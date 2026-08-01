package gmailauth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

const authorizationRecordSchemaVersion = 1

var (
	ErrNotFound                = errors.New("Gmail authorization not found")
	ErrInvalidRecord           = errors.New("invalid Gmail authorization record")
	ErrUnsupportedPlatform     = errors.New("Gmail authorization storage is unsupported on this platform")
	ErrReauthorizationRequired = errors.New("Gmail reauthorization required")
)

// TokenRecord contains the OAuth token material stored for one authorization.
type TokenRecord struct {
	AccessToken  string    `json:"accessToken"`
	RefreshToken string    `json:"refreshToken"`
	TokenType    string    `json:"tokenType"`
	Expiry       time.Time `json:"expiry"`
}

// AuthorizationRecord binds one Mail.app account identity to one Gmail OAuth
// authorization.
type AuthorizationRecord struct {
	SchemaVersion             int         `json:"schemaVersion"`
	MailAccountID             string      `json:"mailAccountID"`
	MailAccountName           string      `json:"mailAccountName"`
	MailAccountEmailAddresses []string    `json:"mailAccountEmailAddresses"`
	GmailEmail                string      `json:"gmailEmail"`
	OAuthClientID             string      `json:"oauthClientID"`
	Token                     TokenRecord `json:"token"`
}

// Validate rejects records that cannot be safely associated with one Mail.app
// account and refreshed through the Gmail API.
func (record AuthorizationRecord) Validate() error {
	if record.SchemaVersion != authorizationRecordSchemaVersion {
		return fmt.Errorf("%w: unsupported schema version", ErrInvalidRecord)
	}
	if strings.TrimSpace(record.MailAccountID) == "" {
		return fmt.Errorf("%w: missing Mail.app account ID", ErrInvalidRecord)
	}
	if strings.TrimSpace(record.MailAccountName) == "" {
		return fmt.Errorf("%w: missing Mail.app account name", ErrInvalidRecord)
	}
	if len(record.MailAccountEmailAddresses) == 0 {
		return fmt.Errorf("%w: missing Mail.app account email addresses", ErrInvalidRecord)
	}
	for _, address := range record.MailAccountEmailAddresses {
		if strings.TrimSpace(address) == "" {
			return fmt.Errorf("%w: blank Mail.app account email address", ErrInvalidRecord)
		}
	}
	if strings.TrimSpace(record.GmailEmail) == "" {
		return fmt.Errorf("%w: missing Gmail email", ErrInvalidRecord)
	}
	if strings.TrimSpace(record.OAuthClientID) == "" {
		return fmt.Errorf("%w: missing OAuth client ID", ErrInvalidRecord)
	}
	if strings.TrimSpace(record.Token.RefreshToken) == "" {
		return fmt.Errorf("%w: missing refresh token", ErrInvalidRecord)
	}

	return nil
}

// Store persists per-account Gmail authorization records.
type Store interface {
	Get(context.Context, string) (AuthorizationRecord, error)
	Put(context.Context, AuthorizationRecord) error
	Delete(context.Context, string) error
	List(context.Context) ([]AuthorizationRecord, error)
}

func validateMailAccountID(mailAccountID string) error {
	if strings.TrimSpace(mailAccountID) == "" {
		return fmt.Errorf("%w: missing Mail.app account ID", ErrInvalidRecord)
	}
	return nil
}

func validateRecordAccountKey(record AuthorizationRecord, mailAccountID string) error {
	if err := validateMailAccountID(mailAccountID); err != nil {
		return err
	}
	if record.MailAccountID != mailAccountID {
		return fmt.Errorf("%w: Mail.app account ID does not match Keychain account", ErrInvalidRecord)
	}
	return nil
}
