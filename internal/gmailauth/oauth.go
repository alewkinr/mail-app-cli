package gmailauth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/intelligrit/mail-app-cli/internal/gmailapi"
	"github.com/intelligrit/mail-app-cli/pkg/mail"
	"golang.org/x/oauth2"
)

const (
	oauthCallbackPath   = "/oauth2/callback"
	defaultLoginTimeout = 2 * time.Minute
)

type ProfileClient interface {
	GetProfile(context.Context) (gmailapi.Profile, error)
}

type ProfileClientFactory interface {
	NewProfileClient(context.Context, oauth2.TokenSource) ProfileClient
}

type ListenerFactory interface {
	Listen(network, address string) (net.Listener, error)
}

type Authorizer interface {
	Authorize(context.Context, LoginRequest) (AuthorizationRecord, error)
}

type LoginRequest struct {
	MailAccount mail.Account
	OAuth       OAuthConfig
	Output      io.Writer
	Timeout     time.Duration
}

type oauthAuthorizer struct {
	profiles  ProfileClientFactory
	listeners ListenerFactory
}

type netListenerFactory struct{}

func NewAuthorizer(
	profiles ProfileClientFactory,
	listeners ListenerFactory,
) Authorizer {
	return &oauthAuthorizer{
		profiles:  profiles,
		listeners: listeners,
	}
}

func NewListenerFactory() ListenerFactory {
	return netListenerFactory{}
}

func (netListenerFactory) Listen(network, address string) (net.Listener, error) {
	return net.Listen(network, address)
}

type oauthCallbackResult struct {
	code string
	err  error
}

func (authorizer *oauthAuthorizer) Authorize(
	ctx context.Context,
	request LoginRequest,
) (AuthorizationRecord, error) {
	if err := validateLoginRequest(request); err != nil {
		return AuthorizationRecord{}, err
	}
	if authorizer.profiles == nil ||
		authorizer.listeners == nil {
		return AuthorizationRecord{}, errors.New("Gmail OAuth dependencies are incomplete")
	}

	timeout := request.Timeout
	if timeout <= 0 {
		timeout = defaultLoginTimeout
	}
	authorizationContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := authorizationContext.Err(); err != nil {
		return AuthorizationRecord{}, err
	}

	listener, err := authorizer.listeners.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return AuthorizationRecord{}, fmt.Errorf("start Gmail OAuth callback listener: %w", err)
	}

	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		_ = listener.Close()
		return AuthorizationRecord{}, errors.New("generate Gmail OAuth state failed")
	}
	encodedState := base64.RawURLEncoding.EncodeToString(stateBytes)
	verifier := oauth2.GenerateVerifier()
	redirectURL := "http://" + listener.Addr().String() + oauthCallbackPath
	oauthConfig, err := newOAuth2Config(request.OAuth, redirectURL)
	if err != nil {
		_ = listener.Close()
		return AuthorizationRecord{}, err
	}

	authorizationURL := oauthConfig.AuthCodeURL(
		encodedState,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", "consent select_account"),
		oauth2.SetAuthURLParam(
			"code_challenge",
			oauth2.S256ChallengeFromVerifier(verifier),
		),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)

	callbackResults := make(chan oauthCallbackResult, 1)
	var callbackClaimed atomic.Bool
	handler := http.NewServeMux()
	handler.HandleFunc(oauthCallbackPath, func(writer http.ResponseWriter, callback *http.Request) {
		if !callbackClaimed.CompareAndSwap(false, true) {
			http.Error(writer, "Authorization callback already handled.", http.StatusConflict)
			return
		}

		result := parseOAuthCallback(callback.URL.Query(), stateBytes)
		if result.err != nil {
			http.Error(writer, "Authorization failed. Return to the terminal.", http.StatusBadRequest)
		} else {
			writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = io.WriteString(
				writer,
				"Authorization response received. Return to the terminal to finish setup.\n",
			)
		}
		if flusher, ok := writer.(http.Flusher); ok {
			flusher.Flush()
		}
		callbackResults <- result
	})

	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	serveDone := make(chan struct{})
	var serveErr error
	var serveErrMu sync.Mutex
	go func() {
		err := server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		serveErrMu.Lock()
		serveErr = err
		serveErrMu.Unlock()
		close(serveDone)
	}()

	var cleanupOnce sync.Once
	cleanup := func() {
		cleanupOnce.Do(func() {
			shutdownContext, shutdownCancel := context.WithTimeout(
				context.Background(),
				2*time.Second,
			)
			defer shutdownCancel()
			if err := server.Shutdown(shutdownContext); err != nil {
				_ = server.Close()
			}
			_ = listener.Close()
			<-serveDone
		})
	}
	defer cleanup()

	if _, err := fmt.Fprintf(
		request.Output,
		"Open this URL to authorize mail-app-cli:\n%s\n",
		authorizationURL,
	); err != nil {
		return AuthorizationRecord{}, errors.New("write Gmail authorization URL failed")
	}

	var authorizationCode string
	select {
	case result := <-callbackResults:
		cleanup()
		if result.err != nil {
			return AuthorizationRecord{}, result.err
		}
		authorizationCode = result.code
	case <-authorizationContext.Done():
		return AuthorizationRecord{}, authorizationContext.Err()
	case <-serveDone:
		serveErrMu.Lock()
		err := serveErr
		serveErrMu.Unlock()
		if err == nil {
			return AuthorizationRecord{}, errors.New("Gmail OAuth callback listener stopped")
		}
		return AuthorizationRecord{}, fmt.Errorf("Gmail OAuth callback listener failed: %w", err)
	}

	token, err := oauthConfig.Exchange(
		authorizationContext,
		authorizationCode,
		oauth2.SetAuthURLParam("code_verifier", verifier),
	)
	if err != nil {
		if ctxErr := authorizationContext.Err(); ctxErr != nil {
			return AuthorizationRecord{}, ctxErr
		}
		if isInvalidGrant(err) {
			return AuthorizationRecord{}, NewReauthorizationError(
				request.MailAccount.Name,
				err,
			)
		}
		return AuthorizationRecord{}, redactedOAuthError(
			"exchange Gmail authorization code failed",
			err,
		)
	}
	if strings.TrimSpace(token.RefreshToken) == "" {
		return AuthorizationRecord{}, NewReauthorizationError(
			request.MailAccount.Name,
			errors.New("refresh token is missing"),
		)
	}

	profileClient := authorizer.profiles.NewProfileClient(
		authorizationContext,
		oauth2.StaticTokenSource(token),
	)
	if profileClient == nil {
		return AuthorizationRecord{}, errors.New("create Gmail profile client failed")
	}
	profile, err := profileClient.GetProfile(authorizationContext)
	if err != nil {
		if ctxErr := authorizationContext.Err(); ctxErr != nil {
			return AuthorizationRecord{}, ctxErr
		}
		if errors.Is(err, gmailapi.ErrNotAuthorized) {
			return AuthorizationRecord{}, NewReauthorizationError(
				request.MailAccount.Name,
				err,
			)
		}
		return AuthorizationRecord{}, gmailapi.NewProfileValidationError(err)
	}
	if !accountContainsEmail(request.MailAccount.EmailAddresses, profile.EmailAddress) {
		return AuthorizationRecord{}, errors.New(
			"Gmail profile does not match the selected Mail.app account",
		)
	}

	record := AuthorizationRecord{
		SchemaVersion:             authorizationRecordSchemaVersion,
		MailAccountID:             request.MailAccount.ID,
		MailAccountName:           request.MailAccount.Name,
		MailAccountEmailAddresses: append([]string(nil), request.MailAccount.EmailAddresses...),
		GmailEmail:                profile.EmailAddress,
		OAuthClientID:             request.OAuth.ClientID,
		Token: TokenRecord{
			AccessToken:  token.AccessToken,
			RefreshToken: token.RefreshToken,
			TokenType:    token.TokenType,
			Expiry:       token.Expiry,
		},
	}
	if err := record.Validate(); err != nil {
		return AuthorizationRecord{}, err
	}
	return record, nil
}

func validateLoginRequest(request LoginRequest) error {
	if request.Output == nil {
		return errors.New("Gmail authorization URL output is required")
	}
	if strings.TrimSpace(request.MailAccount.ID) == "" {
		return errors.New("Mail.app account ID is required")
	}
	if strings.TrimSpace(request.MailAccount.Name) == "" {
		return errors.New("Mail.app account name is required")
	}
	if len(request.MailAccount.EmailAddresses) == 0 {
		return errors.New("Mail.app account email addresses are required")
	}
	for _, emailAddress := range request.MailAccount.EmailAddresses {
		if strings.TrimSpace(emailAddress) == "" {
			return errors.New("Mail.app account email address is blank")
		}
	}
	_, err := newOAuth2Config(request.OAuth, "")
	return err
}

func parseOAuthCallback(values url.Values, expectedState []byte) oauthCallbackResult {
	states := values["state"]
	if len(states) != 1 || strings.TrimSpace(states[0]) == "" {
		return oauthCallbackResult{err: errors.New("invalid Gmail OAuth callback state")}
	}
	actualState, err := base64.RawURLEncoding.DecodeString(states[0])
	if err != nil || !constantTimeStateEqual(expectedState, actualState) {
		return oauthCallbackResult{err: errors.New("invalid Gmail OAuth callback state")}
	}

	if providerErrors := values["error"]; len(providerErrors) > 0 &&
		strings.TrimSpace(providerErrors[0]) != "" {
		return oauthCallbackResult{err: errors.New("Google returned an OAuth authorization error")}
	}

	codes := values["code"]
	if len(codes) != 1 || strings.TrimSpace(codes[0]) == "" {
		return oauthCallbackResult{err: errors.New("invalid Gmail OAuth authorization code")}
	}
	return oauthCallbackResult{code: codes[0]}
}

func constantTimeStateEqual(expected, actual []byte) bool {
	if len(expected) != len(actual) {
		return false
	}
	return subtle.ConstantTimeCompare(expected, actual) == 1
}

func accountContainsEmail(accountEmails []string, gmailEmail string) bool {
	gmailEmail = strings.TrimSpace(gmailEmail)
	if gmailEmail == "" {
		return false
	}
	for _, accountEmail := range accountEmails {
		if strings.EqualFold(strings.TrimSpace(accountEmail), gmailEmail) {
			return true
		}
	}
	return false
}
