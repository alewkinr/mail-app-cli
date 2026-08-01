package gmailauth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func validAuthorizationRecord() AuthorizationRecord {
	return AuthorizationRecord{
		SchemaVersion:             1,
		MailAccountID:             "mail-account-id",
		MailAccountName:           "Personal",
		MailAccountEmailAddresses: []string{"primary@example.com", "alias@example.com"},
		GmailEmail:                "primary@example.com",
		OAuthClientID:             "public-client-id.apps.googleusercontent.com",
		Token: TokenRecord{
			AccessToken:  "access-token",
			RefreshToken: "refresh-token",
			TokenType:    "Bearer",
			Expiry:       time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC),
		},
	}
}

func TestAuthorizationRecordValidate(t *testing.T) {
	if err := validAuthorizationRecord().Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*AuthorizationRecord)
	}{
		{name: "schema zero", mutate: func(record *AuthorizationRecord) {
			record.SchemaVersion = 0
		}},
		{name: "future schema", mutate: func(record *AuthorizationRecord) {
			record.SchemaVersion = 2
		}},
		{name: "Mail account ID", mutate: func(record *AuthorizationRecord) {
			record.MailAccountID = " "
		}},
		{name: "Mail account name", mutate: func(record *AuthorizationRecord) {
			record.MailAccountName = ""
		}},
		{name: "Mail account emails missing", mutate: func(record *AuthorizationRecord) {
			record.MailAccountEmailAddresses = nil
		}},
		{name: "Mail account email blank", mutate: func(record *AuthorizationRecord) {
			record.MailAccountEmailAddresses = []string{"primary@example.com", " "}
		}},
		{name: "Gmail email", mutate: func(record *AuthorizationRecord) {
			record.GmailEmail = ""
		}},
		{name: "OAuth client ID", mutate: func(record *AuthorizationRecord) {
			record.OAuthClientID = "\t"
		}},
		{name: "refresh token", mutate: func(record *AuthorizationRecord) {
			record.Token.RefreshToken = ""
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			record := validAuthorizationRecord()
			test.mutate(&record)
			err := record.Validate()
			if !errors.Is(err, ErrInvalidRecord) {
				t.Fatalf("Validate() error = %v, want ErrInvalidRecord", err)
			}
		})
	}
}

func TestAuthorizationRecordJSONSchema(t *testing.T) {
	data, err := json.Marshal(validAuthorizationRecord())
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if value["oauthClientID"] == nil {
		t.Fatal("JSON does not contain oauthClientID")
	}
	for _, forbidden := range []string{
		"clientSecret",
		"client-secret",
		"clientSecretFile",
		"credentialFile",
		"authURL",
		"tokenURL",
	} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("authorization JSON contains forbidden field %q", forbidden)
		}
	}
}

func TestAuthorizationRecordValidationRedactsSecrets(t *testing.T) {
	generatedSecret := "runtime-refresh-secret-" + randomTestHex(t)
	record := validAuthorizationRecord()
	record.SchemaVersion = 999
	record.Token.RefreshToken = generatedSecret

	err := record.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil")
	}
	if strings.Contains(err.Error(), generatedSecret) {
		t.Fatalf("Validate() error exposed token material: %v", err)
	}
}

func TestAuthorizationRecordAccountKeyValidation(t *testing.T) {
	record := validAuthorizationRecord()
	if err := validateRecordAccountKey(record, record.MailAccountID); err != nil {
		t.Fatalf("validateRecordAccountKey() error = %v", err)
	}
	if err := validateRecordAccountKey(record, "other-account-id"); !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("validateRecordAccountKey() error = %v, want ErrInvalidRecord", err)
	}
}

func randomTestHex(t *testing.T) string {
	t.Helper()
	var value [8]byte
	if _, err := rand.Read(value[:]); err != nil {
		t.Fatalf("rand.Read() error = %v", err)
	}
	return hex.EncodeToString(value[:])
}
