package gmailauth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

type tokenTestStore struct {
	mu      sync.Mutex
	records []AuthorizationRecord
	putErr  error
}

func (store *tokenTestStore) Get(context.Context, string) (AuthorizationRecord, error) {
	return AuthorizationRecord{}, ErrNotFound
}

func (store *tokenTestStore) Put(_ context.Context, record AuthorizationRecord) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.putErr != nil {
		return store.putErr
	}
	store.records = append(store.records, record)
	return nil
}

func (store *tokenTestStore) Delete(context.Context, string) error {
	return nil
}

func (store *tokenTestStore) List(context.Context) ([]AuthorizationRecord, error) {
	return nil, nil
}

func (store *tokenTestStore) putRecords() []AuthorizationRecord {
	store.mu.Lock()
	defer store.mu.Unlock()
	return append([]AuthorizationRecord(nil), store.records...)
}

func TestPersistingTokenSourceRefreshesAndPreservesRefreshToken(t *testing.T) {
	record := validAuthorizationRecord()
	record.Token.AccessToken = "expired-access-" + randomTestHex(t)
	record.Token.RefreshToken = "existing-refresh-" + randomTestHex(t)
	record.Token.Expiry = time.Now().Add(-time.Hour)
	newAccessToken := "new-access-" + randomTestHex(t)
	var requests atomic.Int32

	ctx := tokenServerContext(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", request.Method)
		}
		if request.Header.Get("Authorization") != "" {
			t.Errorf("token request sent an HTTP Authorization header")
		}
		if err := request.ParseForm(); err != nil {
			t.Errorf("ParseForm() error = %v", err)
		}
		if request.PostForm.Get("client_id") != record.OAuthClientID {
			t.Errorf("client_id does not match stored public client ID")
		}
		if request.PostForm.Get("client_secret") != "desktop-client-secret" {
			t.Errorf("client_secret does not match the configured Desktop client")
		}
		assertExactOAuthFormKeys(
			t,
			request.PostForm,
			"client_id",
			"client_secret",
			"grant_type",
			"refresh_token",
		)
		if request.PostForm.Get("grant_type") != "refresh_token" {
			t.Errorf("grant_type = %q, want refresh_token", request.PostForm.Get("grant_type"))
		}
		if request.PostForm.Get("refresh_token") != record.Token.RefreshToken {
			t.Errorf("refresh_token does not match stored token")
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(
			writer,
			`{"access_token":"`+newAccessToken+`","token_type":"Bearer","expires_in":3600}`,
		)
	}))

	store := &tokenTestStore{}
	source, err := NewPersistingTokenSource(ctx, record, store, tokenTestOAuthConfig(record))
	if err != nil {
		t.Fatalf("NewPersistingTokenSource() error = %v", err)
	}
	token, err := source.Token()
	if err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	if token.AccessToken != newAccessToken {
		t.Fatal("Token() did not return refreshed access token")
	}
	if token.RefreshToken != record.Token.RefreshToken {
		t.Fatal("Token() did not preserve the existing refresh token")
	}
	if requests.Load() != 1 {
		t.Fatalf("refresh requests = %d, want 1", requests.Load())
	}
	records := store.putRecords()
	if len(records) != 1 {
		t.Fatalf("Store.Put calls = %d, want 1", len(records))
	}
	if records[0].Token.AccessToken != newAccessToken ||
		records[0].Token.RefreshToken != record.Token.RefreshToken {
		t.Fatal("persisted token fields do not match refreshed token")
	}
}

func TestPersistingTokenSourcePersistsRotatedRefreshToken(t *testing.T) {
	record := validAuthorizationRecord()
	record.Token.Expiry = time.Now().Add(-time.Hour)
	rotatedRefreshToken := "rotated-refresh-" + randomTestHex(t)
	ctx := tokenServerContext(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(
			writer,
			`{"access_token":"new-access","refresh_token":"`+
				rotatedRefreshToken+
				`","token_type":"Bearer","expires_in":3600}`,
		)
	}))
	store := &tokenTestStore{}
	source, err := NewPersistingTokenSource(ctx, record, store, tokenTestOAuthConfig(record))
	if err != nil {
		t.Fatalf("NewPersistingTokenSource() error = %v", err)
	}
	token, err := source.Token()
	if err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	if token.RefreshToken != rotatedRefreshToken {
		t.Fatal("Token() did not return rotated refresh token")
	}
	records := store.putRecords()
	if len(records) != 1 || records[0].Token.RefreshToken != rotatedRefreshToken {
		t.Fatal("Store did not persist rotated refresh token")
	}
}

func TestPersistingTokenSourceUsesValidStoredTokenWithoutWrite(t *testing.T) {
	record := validAuthorizationRecord()
	record.Token.Expiry = time.Now().Add(time.Hour)
	store := &tokenTestStore{}
	source, err := NewPersistingTokenSource(
		context.Background(),
		record,
		store,
		tokenTestOAuthConfig(record),
	)
	if err != nil {
		t.Fatalf("NewPersistingTokenSource() error = %v", err)
	}
	token, err := source.Token()
	if err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	if token.AccessToken != record.Token.AccessToken {
		t.Fatal("Token() did not return stored access token")
	}
	if len(store.putRecords()) != 0 {
		t.Fatal("valid unchanged token was persisted unnecessarily")
	}
}

func TestPersistingTokenSourceRequiresRefreshToken(t *testing.T) {
	record := validAuthorizationRecord()
	record.Token.RefreshToken = ""
	_, err := NewPersistingTokenSource(
		context.Background(),
		record,
		&tokenTestStore{},
		tokenTestOAuthConfig(record),
	)
	assertReauthorizationError(t, err, record.MailAccountName)
}

func TestPersistingTokenSourceMapsInvalidGrantSafely(t *testing.T) {
	record := validAuthorizationRecord()
	record.Token.Expiry = time.Now().Add(-time.Hour)
	record.Token.RefreshToken = "runtime-refresh-" + randomTestHex(t)
	bodySecret := "provider-body-" + randomTestHex(t)
	ctx := tokenServerContext(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(
			writer,
			`{"error":"invalid_grant","error_description":"`+bodySecret+`"}`,
		)
	}))
	source, err := NewPersistingTokenSource(
		ctx,
		record,
		&tokenTestStore{},
		tokenTestOAuthConfig(record),
	)
	if err != nil {
		t.Fatalf("NewPersistingTokenSource() error = %v", err)
	}
	_, err = source.Token()
	assertReauthorizationError(t, err, record.MailAccountName)
	if strings.Contains(err.Error(), record.Token.RefreshToken) ||
		strings.Contains(err.Error(), bodySecret) {
		t.Fatalf("Token() error exposed token or provider response: %v", err)
	}
}

func TestPersistingTokenSourceReportsSafeOAuthErrorCode(t *testing.T) {
	record := validAuthorizationRecord()
	record.Token.Expiry = time.Now().Add(-time.Hour)
	bodySecret := "provider-body-" + randomTestHex(t)
	ctx := tokenServerContext(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(
			writer,
			`{"error":"server_error","error_description":"`+bodySecret+`"}`,
		)
	}))
	source, err := NewPersistingTokenSource(
		ctx,
		record,
		&tokenTestStore{},
		tokenTestOAuthConfig(record),
	)
	if err != nil {
		t.Fatalf("NewPersistingTokenSource() error = %v", err)
	}
	_, err = source.Token()
	if err == nil ||
		!strings.Contains(err.Error(), "server_error") ||
		strings.Contains(err.Error(), bodySecret) {
		t.Fatalf("Token() error lacks safe code or exposed provider response: %v", err)
	}
}

func TestPersistingTokenSourceRejectsDifferentOAuthClient(t *testing.T) {
	record := validAuthorizationRecord()
	config := tokenTestOAuthConfig(record)
	config.ClientID = "different-client.apps.googleusercontent.com"
	_, err := NewPersistingTokenSource(context.Background(), record, &tokenTestStore{}, config)
	assertReauthorizationError(t, err, record.MailAccountName)
}

func TestPersistingTokenSourcePropagatesCanceledContext(t *testing.T) {
	record := validAuthorizationRecord()
	record.Token.Expiry = time.Now().Add(-time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	source, err := NewPersistingTokenSource(
		ctx,
		record,
		&tokenTestStore{},
		tokenTestOAuthConfig(record),
	)
	if err != nil {
		t.Fatalf("NewPersistingTokenSource() error = %v", err)
	}
	if _, err := source.Token(); !errors.Is(err, context.Canceled) {
		t.Fatalf("Token() error = %v, want context.Canceled", err)
	}
}

func TestNewReauthorizationErrorQuotesAccountOnce(t *testing.T) {
	accountName := `Work "$HOME"` + "\n"
	causeSecret := "cause-secret-" + randomTestHex(t)
	err := NewReauthorizationError(accountName, errors.New(causeSecret))
	if !errors.Is(err, ErrReauthorizationRequired) {
		t.Fatalf("error = %v, want ErrReauthorizationRequired", err)
	}
	if strings.Count(err.Error(), "mail-app-cli gmail auth login --account") != 1 {
		t.Fatalf("reauthorization error does not contain exactly one login command: %v", err)
	}
	if strings.Contains(err.Error(), causeSecret) {
		t.Fatalf("reauthorization error exposed cause: %v", err)
	}
	if !strings.Contains(err.Error(), `--account "Work \"\$HOME\"\n"`) {
		t.Fatalf("reauthorization command is not safely quoted: %v", err)
	}
}

func assertReauthorizationError(t *testing.T, err error, accountName string) {
	t.Helper()
	if !errors.Is(err, ErrReauthorizationRequired) {
		t.Fatalf("error = %v, want ErrReauthorizationRequired", err)
	}
	wantCommand := `mail-app-cli gmail auth login --account "` + accountName + `"`
	if strings.Count(err.Error(), wantCommand) != 1 {
		t.Fatalf("error = %v, want exactly one %q command", err, wantCommand)
	}
}

func tokenTestOAuthConfig(record AuthorizationRecord) OAuthConfig {
	return OAuthConfig{
		ClientID:     record.OAuthClientID,
		ClientSecret: "desktop-client-secret",
		AuthURL:      googleAuthorizationURL,
		TokenURL:     googleTokenURL,
	}
}

func tokenServerContext(t *testing.T, handler http.Handler) context.Context {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	transport := tokenRoundTripperFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != googleTokenURL {
			t.Errorf("token URL = %q, want fixed Google token URL", request.URL.String())
		}
		clone := request.Clone(request.Context())
		clonedURL := *request.URL
		clonedURL.Scheme = target.Scheme
		clonedURL.Host = target.Host
		clone.URL = &clonedURL
		return server.Client().Transport.RoundTrip(clone)
	})
	client := &http.Client{Transport: transport}
	return context.WithValue(context.Background(), oauth2.HTTPClient, client)
}

type tokenRoundTripperFunc func(*http.Request) (*http.Response, error)

func (function tokenRoundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
