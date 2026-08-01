package gmailauth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/oauth2"
)

type persistingTokenSource struct {
	mu     sync.Mutex
	ctx    context.Context
	record AuthorizationRecord
	store  Store
	source oauth2.TokenSource
}

func NewPersistingTokenSource(
	ctx context.Context,
	record AuthorizationRecord,
	store Store,
	oauthConfig OAuthConfig,
) (oauth2.TokenSource, error) {
	if strings.TrimSpace(record.Token.RefreshToken) == "" {
		return nil, NewReauthorizationError(
			record.MailAccountName,
			errors.New("refresh token is missing"),
		)
	}
	if err := record.Validate(); err != nil {
		return nil, err
	}
	if store == nil {
		return nil, errors.New("Gmail authorization store is required")
	}

	if strings.TrimSpace(oauthConfig.ClientID) != record.OAuthClientID {
		return nil, NewReauthorizationError(
			record.MailAccountName,
			errors.New("stored OAuth client does not match this build"),
		)
	}
	config, err := newOAuth2Config(oauthConfig, "")
	if err != nil {
		return nil, err
	}
	token := &oauth2.Token{
		AccessToken:  record.Token.AccessToken,
		RefreshToken: record.Token.RefreshToken,
		TokenType:    record.Token.TokenType,
		Expiry:       record.Token.Expiry,
	}
	return &persistingTokenSource{
		ctx:    ctx,
		record: record,
		store:  store,
		source: config.TokenSource(ctx, token),
	}, nil
}

func (source *persistingTokenSource) Token() (*oauth2.Token, error) {
	source.mu.Lock()
	defer source.mu.Unlock()

	if err := source.ctx.Err(); err != nil {
		return nil, err
	}
	token, err := source.source.Token()
	if err != nil {
		if ctxErr := source.ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		if isInvalidGrant(err) {
			return nil, NewReauthorizationError(source.record.MailAccountName, err)
		}
		return nil, redactedOAuthError("refresh Gmail authorization failed", err)
	}

	updatedToken := *token
	if strings.TrimSpace(updatedToken.RefreshToken) == "" {
		updatedToken.RefreshToken = source.record.Token.RefreshToken
	}
	if strings.TrimSpace(updatedToken.RefreshToken) == "" {
		return nil, NewReauthorizationError(
			source.record.MailAccountName,
			errors.New("refresh token is missing"),
		)
	}
	if strings.TrimSpace(updatedToken.TokenType) == "" {
		updatedToken.TokenType = source.record.Token.TokenType
	}

	updatedRecord := source.record
	updatedRecord.Token = TokenRecord{
		AccessToken:  updatedToken.AccessToken,
		RefreshToken: updatedToken.RefreshToken,
		TokenType:    updatedToken.TokenType,
		Expiry:       updatedToken.Expiry,
	}
	if updatedRecord.Token != source.record.Token {
		if err := source.store.Put(source.ctx, updatedRecord); err != nil {
			return nil, fmt.Errorf("persist refreshed Gmail authorization: %w", err)
		}
		source.record = updatedRecord
	}
	return &updatedToken, nil
}

func NewReauthorizationError(accountName string, cause error) error {
	if errors.Is(cause, ErrReauthorizationRequired) {
		return cause
	}
	command := `mail-app-cli gmail auth login --account "` +
		escapeDoubleQuotedShellValue(accountName) +
		`"`
	return fmt.Errorf("%w; run %s", ErrReauthorizationRequired, command)
}

func escapeDoubleQuotedShellValue(value string) string {
	var escaped strings.Builder
	for _, character := range value {
		switch character {
		case '\\', '"', '$', '`':
			escaped.WriteByte('\\')
			escaped.WriteRune(character)
		case '\n':
			escaped.WriteString(`\n`)
		case '\r':
			escaped.WriteString(`\r`)
		default:
			escaped.WriteRune(character)
		}
	}
	return escaped.String()
}

func isInvalidGrant(err error) bool {
	return oauthErrorCode(err) == "invalid_grant"
}

func redactedOAuthError(operation string, err error) error {
	if code := oauthErrorCode(err); code != "" {
		return fmt.Errorf("%s: Google OAuth error %s", operation, code)
	}
	return errors.New(operation)
}

func oauthErrorCode(err error) string {
	var retrieveError *oauth2.RetrieveError
	if !errors.As(err, &retrieveError) {
		return ""
	}
	code := strings.TrimSpace(retrieveError.ErrorCode)
	if code == "" || len(code) > 64 {
		return ""
	}
	for _, character := range code {
		if (character < 'a' || character > 'z') &&
			(character < '0' || character > '9') &&
			character != '_' &&
			character != '-' {
			return ""
		}
	}
	return code
}
