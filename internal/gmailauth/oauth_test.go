package gmailauth

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/intelligrit/mail-app-cli/internal/gmailapi"
	"github.com/intelligrit/mail-app-cli/pkg/mail"
	"golang.org/x/oauth2"
)

type authorizationOutputFunc func(string)

func (function authorizationOutputFunc) Write(data []byte) (int, error) {
	const prefix = "Open this URL to authorize mail-app-cli:\n"
	value := string(data)
	if !strings.HasPrefix(value, prefix) || !strings.HasSuffix(value, "\n") {
		return 0, errors.New("unexpected authorization URL output")
	}
	function(strings.TrimSuffix(strings.TrimPrefix(value, prefix), "\n"))
	return len(data), nil
}

type oauthTestProfileClient struct {
	profile gmailapi.Profile
	err     error
}

func (client oauthTestProfileClient) GetProfile(context.Context) (gmailapi.Profile, error) {
	return client.profile, client.err
}

type oauthTestProfileFactory struct {
	mu      sync.Mutex
	client  ProfileClient
	sources []oauth2.TokenSource
}

func (factory *oauthTestProfileFactory) NewProfileClient(
	_ context.Context,
	source oauth2.TokenSource,
) ProfileClient {
	factory.mu.Lock()
	defer factory.mu.Unlock()
	factory.sources = append(factory.sources, source)
	return factory.client
}

func (factory *oauthTestProfileFactory) sourceCount() int {
	factory.mu.Lock()
	defer factory.mu.Unlock()
	return len(factory.sources)
}

type trackingListener struct {
	net.Listener
	closed atomic.Bool
}

func (listener *trackingListener) Close() error {
	listener.closed.Store(true)
	return listener.Listener.Close()
}

type trackingListenerFactory struct {
	mu       sync.Mutex
	listener *trackingListener
}

func (factory *trackingListenerFactory) Listen(network, address string) (net.Listener, error) {
	if network != "tcp" || address != "127.0.0.1:0" {
		return nil, errors.New("unexpected listener request")
	}
	listener, err := net.Listen(network, address)
	if err != nil {
		return nil, err
	}
	tracked := &trackingListener{Listener: listener}
	factory.mu.Lock()
	factory.listener = tracked
	factory.mu.Unlock()
	return tracked, nil
}

func (factory *trackingListenerFactory) isClosed() bool {
	factory.mu.Lock()
	defer factory.mu.Unlock()
	return factory.listener != nil && factory.listener.closed.Load()
}

func TestOAuthAuthorizerSuccess(t *testing.T) {
	accessToken := "access-" + randomTestHex(t)
	refreshToken := "refresh-" + randomTestHex(t)
	authorizationCode := "code-" + randomTestHex(t)
	clientID := "client-" + randomTestHex(t) + ".apps.googleusercontent.com"
	clientSecret := "desktop-secret-" + randomTestHex(t)
	var authorizationQuery url.Values
	var exchangeRequests atomic.Int32

	tokenServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		exchangeRequests.Add(1)
		if request.Header.Get("Authorization") != "" {
			t.Errorf("token exchange sent HTTP Basic Authorization")
		}
		if err := request.ParseForm(); err != nil {
			t.Errorf("ParseForm() error = %v", err)
		}
		if request.PostForm.Get("client_id") != clientID {
			t.Errorf("client_id does not match")
		}
		if request.PostForm.Get("client_secret") != clientSecret {
			t.Errorf("client_secret does not match")
		}
		assertExactOAuthFormKeys(
			t,
			request.PostForm,
			"client_id",
			"client_secret",
			"code",
			"code_verifier",
			"grant_type",
			"redirect_uri",
		)
		if request.PostForm.Get("code") != authorizationCode {
			t.Errorf("authorization code does not match")
		}
		verifier := request.PostForm.Get("code_verifier")
		if verifier == "" {
			t.Errorf("code_verifier is blank")
		}
		if oauth2.S256ChallengeFromVerifier(verifier) != authorizationQuery.Get("code_challenge") {
			t.Errorf("PKCE verifier does not match authorization challenge")
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(
			writer,
			`{"access_token":"`+accessToken+
				`","refresh_token":"`+refreshToken+
				`","token_type":"Bearer","expires_in":3600}`,
		)
	}))
	t.Cleanup(tokenServer.Close)

	output := authorizationOutputFunc(func(authorizationURL string) {
		parsed, err := url.Parse(authorizationURL)
		if err != nil {
			t.Fatalf("url.Parse() error = %v", err)
		}
		authorizationQuery = parsed.Query()
		assertAuthorizationURLSecurity(t, authorizationQuery, clientID)
		stateBytes, err := base64.RawURLEncoding.DecodeString(authorizationQuery.Get("state"))
		if err != nil || len(stateBytes) != 32 {
			t.Fatalf("OAuth state is not 32 base64url bytes")
		}
		sendOAuthCallback(t, authorizationQuery, url.Values{
			"state": {authorizationQuery.Get("state")},
			"code":  {authorizationCode},
		})
	})

	profiles := &oauthTestProfileFactory{
		client: oauthTestProfileClient{
			profile: gmailapi.Profile{EmailAddress: "PRIMARY@example.com"},
		},
	}
	listeners := &trackingListenerFactory{}
	request := validOAuthLoginRequest(tokenServer.URL, clientID)
	request.OAuth.ClientSecret = clientSecret
	request.Output = output
	record, err := NewAuthorizer(profiles, listeners).Authorize(
		context.Background(),
		request,
	)
	if err != nil {
		t.Fatalf("Authorize() error = %v", err)
	}
	if exchangeRequests.Load() != 1 {
		t.Fatalf("token exchanges = %d, want 1", exchangeRequests.Load())
	}
	if profiles.sourceCount() != 1 {
		t.Fatalf("profile clients = %d, want 1", profiles.sourceCount())
	}
	if !listeners.isClosed() {
		t.Fatal("callback listener was not closed")
	}
	if record.SchemaVersion != 1 ||
		record.MailAccountID != request.MailAccount.ID ||
		record.MailAccountName != request.MailAccount.Name ||
		record.GmailEmail != "PRIMARY@example.com" ||
		record.OAuthClientID != clientID ||
		record.Token.AccessToken != accessToken ||
		record.Token.RefreshToken != refreshToken ||
		record.Token.TokenType != "Bearer" ||
		record.Token.Expiry.IsZero() {
		t.Fatal("authorization record fields do not match validated OAuth result")
	}
	if len(record.MailAccountEmailAddresses) != len(request.MailAccount.EmailAddresses) {
		t.Fatal("authorization record did not copy every Mail.app email address")
	}
}

func TestOAuthAuthorizerPrintsURLAndContinues(t *testing.T) {
	tokenServer := successfulTokenServer(t, true)
	var output bytes.Buffer
	callbackDone := make(chan struct{})
	var callbackStatus int
	var callbackBody string
	callbackWriter := authorizationOutputFunc(func(authorizationURL string) {
		go func() {
			defer close(callbackDone)
			parsed, _ := url.Parse(authorizationURL)
			callbackStatus, callbackBody = getOAuthCallback(t, parsed.Query(), url.Values{
				"state": {parsed.Query().Get("state")},
				"code":  {"authorization-code"},
			})
		}()
	})
	profiles := &oauthTestProfileFactory{
		client: oauthTestProfileClient{
			profile: gmailapi.Profile{EmailAddress: "primary@example.com"},
		},
	}
	request := validOAuthLoginRequest(tokenServer.URL, "client-id")
	request.Output = io.MultiWriter(&output, callbackWriter)
	listeners := &trackingListenerFactory{}

	if _, err := NewAuthorizer(profiles, listeners).Authorize(
		context.Background(),
		request,
	); err != nil {
		t.Fatalf("Authorize() error = %v", err)
	}
	<-callbackDone
	if !strings.HasPrefix(output.String(), "Open this URL to authorize mail-app-cli:\nhttps://") {
		t.Fatalf("authorization output = %q", output.String())
	}
	if callbackStatus != http.StatusOK {
		t.Fatalf("callback status = %d, want 200", callbackStatus)
	}
	if callbackBody != "Authorization response received. Return to the terminal to finish setup.\n" {
		t.Fatalf("callback body = %q", callbackBody)
	}
	if !listeners.isClosed() {
		t.Fatal("callback listener was not closed")
	}
}

func TestOAuthCallbackValidation(t *testing.T) {
	tests := []struct {
		name   string
		values func(url.Values) url.Values
	}{
		{name: "missing state", values: func(query url.Values) url.Values {
			return url.Values{"code": {"code"}}
		}},
		{name: "duplicate state", values: func(query url.Values) url.Values {
			return url.Values{
				"state": {query.Get("state"), query.Get("state")},
				"code":  {"code"},
			}
		}},
		{name: "mismatched state", values: func(query url.Values) url.Values {
			return url.Values{
				"state": {base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{9}, 32))},
				"code":  {"code"},
			}
		}},
		{name: "malformed state", values: func(query url.Values) url.Values {
			return url.Values{"state": {"***"}, "code": {"code"}}
		}},
		{name: "provider error after valid state", values: func(query url.Values) url.Values {
			return url.Values{"state": {query.Get("state")}, "error": {"access_denied"}}
		}},
		{name: "missing code", values: func(query url.Values) url.Values {
			return url.Values{"state": {query.Get("state")}}
		}},
		{name: "duplicate code", values: func(query url.Values) url.Values {
			return url.Values{
				"state": {query.Get("state")},
				"code":  {"one", "two"},
			}
		}},
		{name: "blank code", values: func(query url.Values) url.Values {
			return url.Values{"state": {query.Get("state")}, "code": {" \t "}}
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var exchangeRequests atomic.Int32
			tokenServer := httptest.NewServer(http.HandlerFunc(func(
				writer http.ResponseWriter,
				_ *http.Request,
			) {
				exchangeRequests.Add(1)
				writer.WriteHeader(http.StatusInternalServerError)
			}))
			t.Cleanup(tokenServer.Close)
			output := authorizationOutputFunc(func(authorizationURL string) {
				parsed, _ := url.Parse(authorizationURL)
				sendOAuthCallback(t, parsed.Query(), test.values(parsed.Query()))
			})
			listeners := &trackingListenerFactory{}
			profiles := &oauthTestProfileFactory{client: oauthTestProfileClient{}}
			request := validOAuthLoginRequest(tokenServer.URL, "client-id")
			request.Output = output

			_, err := NewAuthorizer(profiles, listeners).Authorize(
				context.Background(),
				request,
			)
			if err == nil {
				t.Fatal("Authorize() error = nil")
			}
			if exchangeRequests.Load() != 0 {
				t.Fatalf("token exchanges = %d, want 0", exchangeRequests.Load())
			}
			if !listeners.isClosed() {
				t.Fatal("callback listener was not closed")
			}
		})
	}
}

func TestOAuthAuthorizerTimeoutAndCancellation(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		listeners := &trackingListenerFactory{}
		request := validOAuthLoginRequest("https://token.invalid", "client-id")
		request.Timeout = 20 * time.Millisecond
		_, err := NewAuthorizer(
			&oauthTestProfileFactory{},
			listeners,
		).Authorize(context.Background(), request)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Authorize() error = %v, want DeadlineExceeded", err)
		}
		if !listeners.isClosed() {
			t.Fatal("callback listener was not closed after timeout")
		}
	})

	t.Run("cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		listeners := &trackingListenerFactory{}
		request := validOAuthLoginRequest("https://token.invalid", "client-id")
		request.Output = authorizationOutputFunc(func(string) {
			cancel()
		})
		_, err := NewAuthorizer(
			&oauthTestProfileFactory{},
			listeners,
		).Authorize(ctx, request)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Authorize() error = %v, want context.Canceled", err)
		}
		if !listeners.isClosed() {
			t.Fatal("callback listener was not closed after cancellation")
		}
	})
}

func TestOAuthAuthorizerReauthorizationFailures(t *testing.T) {
	t.Run("initial invalid grant", func(t *testing.T) {
		bodySecret := "provider-body-" + randomTestHex(t)
		tokenServer := httptest.NewServer(http.HandlerFunc(func(
			writer http.ResponseWriter,
			_ *http.Request,
		) {
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(
				writer,
				`{"error":"invalid_grant","error_description":"`+bodySecret+`"}`,
			)
		}))
		t.Cleanup(tokenServer.Close)
		err := authorizeWithTokenServerError(t, tokenServer.URL, oauthTestProfileClient{})
		assertReauthorizationError(t, err, "Personal")
		if strings.Contains(err.Error(), bodySecret) {
			t.Fatalf("Authorize() error exposed token response: %v", err)
		}
	})

	t.Run("missing refresh token", func(t *testing.T) {
		tokenServer := successfulTokenServer(t, false)
		err := authorizeWithTokenServerError(t, tokenServer.URL, oauthTestProfileClient{})
		assertReauthorizationError(t, err, "Personal")
	})

	t.Run("profile unauthorized", func(t *testing.T) {
		tokenServer := successfulTokenServer(t, true)
		err := authorizeWithTokenServerError(t, tokenServer.URL, oauthTestProfileClient{
			err: gmailapi.ErrNotAuthorized,
		})
		assertReauthorizationError(t, err, "Personal")
	})
}

func TestOAuthAuthorizerRejectsProfileMismatch(t *testing.T) {
	tokenServer := successfulTokenServer(t, true)
	err := authorizeWithTokenServerError(t, tokenServer.URL, oauthTestProfileClient{
		profile: gmailapi.Profile{EmailAddress: "different@example.com"},
	})
	if err == nil || errors.Is(err, ErrReauthorizationRequired) {
		t.Fatalf("Authorize() error = %v, want non-reauthorization mismatch", err)
	}
}

func TestOAuthAuthorizerRedactsExchangeAndProfileFailures(t *testing.T) {
	t.Run("exchange", func(t *testing.T) {
		bodySecret := "exchange-secret-" + randomTestHex(t)
		tokenServer := httptest.NewServer(http.HandlerFunc(func(
			writer http.ResponseWriter,
			_ *http.Request,
		) {
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(
				writer,
				`{"error":"server_error","error_description":"`+bodySecret+`"}`,
			)
		}))
		t.Cleanup(tokenServer.Close)
		err := authorizeWithTokenServerError(t, tokenServer.URL, oauthTestProfileClient{})
		if err == nil ||
			strings.Contains(err.Error(), bodySecret) ||
			!strings.Contains(err.Error(), "server_error") {
			t.Fatalf("Authorize() exchange error was nil or exposed response: %v", err)
		}
	})

	t.Run("profile", func(t *testing.T) {
		profileSecret := "profile-secret-" + randomTestHex(t)
		tokenServer := successfulTokenServer(t, true)
		err := authorizeWithTokenServerError(t, tokenServer.URL, oauthTestProfileClient{
			err: errors.New(profileSecret),
		})
		if err == nil || strings.Contains(err.Error(), profileSecret) {
			t.Fatalf("Authorize() profile error was nil or exposed cause: %v", err)
		}
	})

	t.Run("profile forbidden", func(t *testing.T) {
		tokenServer := successfulTokenServer(t, true)
		err := authorizeWithTokenServerError(t, tokenServer.URL, oauthTestProfileClient{
			err: &gmailapi.HTTPError{
				StatusCode: http.StatusForbidden,
				Operation:  "get profile",
			},
		})
		if err == nil ||
			!strings.Contains(err.Error(), "HTTP status 403") ||
			!strings.Contains(err.Error(), "enable the Gmail API") {
			t.Fatalf("Authorize() profile error = %v, want actionable HTTP 403", err)
		}
	})
}

func TestOAuthAuthorizerRejectsSecondCallback(t *testing.T) {
	tokenServer := successfulTokenServer(t, true)
	var secondStatus atomic.Int32
	output := authorizationOutputFunc(func(authorizationURL string) {
		parsed, _ := url.Parse(authorizationURL)
		values := url.Values{
			"state": {parsed.Query().Get("state")},
			"code":  {"authorization-code"},
		}
		sendOAuthCallback(t, parsed.Query(), values)
		secondStatus.Store(int32(sendOAuthCallback(t, parsed.Query(), values)))
	})
	profiles := &oauthTestProfileFactory{
		client: oauthTestProfileClient{
			profile: gmailapi.Profile{EmailAddress: "primary@example.com"},
		},
	}
	request := validOAuthLoginRequest(tokenServer.URL, "client-id")
	request.Output = output
	if _, err := NewAuthorizer(
		profiles,
		&trackingListenerFactory{},
	).Authorize(
		context.Background(),
		request,
	); err != nil {
		t.Fatalf("Authorize() error = %v", err)
	}
	if secondStatus.Load() != http.StatusConflict {
		t.Fatalf("second callback status = %d, want 409", secondStatus.Load())
	}
}

func TestConstantTimeStateEqual(t *testing.T) {
	state := bytes.Repeat([]byte{7}, 32)
	if !constantTimeStateEqual(state, append([]byte(nil), state...)) {
		t.Fatal("equal states did not compare equal")
	}
	different := append([]byte(nil), state...)
	different[0] = 8
	if constantTimeStateEqual(state, different) {
		t.Fatal("different states compared equal")
	}
	if constantTimeStateEqual(state, state[:31]) {
		t.Fatal("different-length states compared equal")
	}
}

func validOAuthLoginRequest(tokenURL, clientID string) LoginRequest {
	return LoginRequest{
		MailAccount: mail.Account{
			ID:             "mail-account-id",
			Name:           "Personal",
			EmailAddresses: []string{"primary@example.com", "alias@example.com"},
		},
		OAuth: OAuthConfig{
			ClientID:     clientID,
			ClientSecret: "desktop-client-secret",
			AuthURL:      "https://accounts.example.test/authorize",
			TokenURL:     tokenURL,
		},
		Output:  io.Discard,
		Timeout: 2 * time.Second,
	}
}

func assertAuthorizationURLSecurity(t *testing.T, query url.Values, clientID string) {
	t.Helper()
	if query.Get("client_id") != clientID {
		t.Fatal("authorization URL client_id does not match")
	}
	if query.Get("client_secret") != "" {
		t.Fatal("authorization URL exposed the Desktop client secret")
	}
	if got := query["scope"]; len(got) != 1 || got[0] != gmailModifyScope {
		t.Fatalf("scope = %#v, want exact gmail.modify scope", got)
	}
	if strings.Contains(query.Get("scope"), "https://mail.google.com/") {
		t.Fatal("authorization URL includes broad mail scope")
	}
	if query.Get("access_type") != "offline" {
		t.Fatal("authorization URL does not request offline access")
	}
	if query.Get("prompt") != "consent select_account" {
		t.Fatal("authorization URL prompt is not consent select_account")
	}
	if query.Get("code_challenge_method") != "S256" ||
		query.Get("code_challenge") == "" {
		t.Fatal("authorization URL does not contain PKCE-S256")
	}
	if query.Get("redirect_uri") == "" {
		t.Fatal("authorization URL redirect_uri is blank")
	}
}

func sendOAuthCallback(t *testing.T, authorizationQuery, values url.Values) int {
	t.Helper()
	status, _ := getOAuthCallback(t, authorizationQuery, values)
	return status
}

func getOAuthCallback(
	t *testing.T,
	authorizationQuery url.Values,
	values url.Values,
) (int, string) {
	t.Helper()
	callbackURL, err := url.Parse(authorizationQuery.Get("redirect_uri"))
	if err != nil {
		t.Fatalf("parse redirect URI: %v", err)
	}
	callbackURL.RawQuery = values.Encode()
	response, err := http.Get(callbackURL.String())
	if err != nil {
		t.Fatalf("send OAuth callback: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read OAuth callback response: %v", err)
	}
	return response.StatusCode, string(body)
}

func successfulTokenServer(t *testing.T, includeRefresh bool) *httptest.Server {
	t.Helper()
	refreshField := ""
	if includeRefresh {
		refreshField = `,"refresh_token":"refresh-token"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "" {
			t.Errorf("token exchange sent HTTP Basic Authorization")
		}
		if err := request.ParseForm(); err != nil {
			t.Errorf("ParseForm() error = %v", err)
		}
		if request.PostForm.Get("client_secret") != "desktop-client-secret" {
			t.Errorf("client_secret does not match configured Desktop client")
		}
		assertExactOAuthFormKeys(
			t,
			request.PostForm,
			"client_id",
			"client_secret",
			"code",
			"code_verifier",
			"grant_type",
			"redirect_uri",
		)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(
			writer,
			`{"access_token":"access-token","token_type":"Bearer","expires_in":3600`+
				refreshField+
				`}`,
		)
	}))
	t.Cleanup(server.Close)
	return server
}

func authorizeWithTokenServerError(
	t *testing.T,
	tokenURL string,
	profileClient oauthTestProfileClient,
) error {
	t.Helper()
	output := authorizationOutputFunc(func(authorizationURL string) {
		parsed, _ := url.Parse(authorizationURL)
		sendOAuthCallback(t, parsed.Query(), url.Values{
			"state": {parsed.Query().Get("state")},
			"code":  {"authorization-code"},
		})
	})
	profiles := &oauthTestProfileFactory{client: profileClient}
	listeners := &trackingListenerFactory{}
	request := validOAuthLoginRequest(tokenURL, "client-id")
	request.Output = output
	_, err := NewAuthorizer(profiles, listeners).Authorize(
		context.Background(),
		request,
	)
	if !listeners.isClosed() {
		t.Fatal("callback listener was not closed")
	}
	return err
}

func assertExactOAuthFormKeys(t *testing.T, values url.Values, expected ...string) {
	t.Helper()
	want := make(map[string]bool, len(expected))
	for _, key := range expected {
		want[key] = true
	}
	if len(values) != len(want) {
		t.Fatalf("OAuth form contains %d keys, want %d", len(values), len(want))
	}
	for key := range values {
		if !want[key] {
			t.Fatalf("OAuth form contains unexpected key %q", key)
		}
	}
}
