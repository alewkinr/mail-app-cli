package mail

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestResolveMessageIdentity(t *testing.T) {
	var gotScript string
	var gotArgs []string
	run := func(script string, args ...string) (string, error) {
		gotScript = script
		gotArgs = append([]string(nil), args...)
		return `{
			"localID":"42",
			"rfcMessageID":"message@example.com",
			"accountID":"account-id",
			"accountName":"Personal",
			"accountEmailAddresses":["primary@example.com","alias@example.com"],
			"mailboxName":"INBOX"
		}`, nil
	}

	identity, err := resolveMessageIdentity(run, "Personal", "INBOX", "42")
	if err != nil {
		t.Fatalf("resolveMessageIdentity() error = %v", err)
	}
	if gotScript != resolveMessageIdentityJXAScript {
		t.Fatal("resolveMessageIdentity() did not pass the embedded resolver script")
	}
	wantArgs := []string{"Personal", "INBOX", "42"}
	if fmt.Sprint(gotArgs) != fmt.Sprint(wantArgs) {
		t.Fatalf("resolver args = %#v, want %#v", gotArgs, wantArgs)
	}
	if identity.LocalID != "42" ||
		identity.RFCMessageID != "message@example.com" ||
		identity.AccountID != "account-id" ||
		identity.AccountName != "Personal" ||
		identity.MailboxName != "INBOX" {
		t.Fatalf("resolved identity = %#v", identity)
	}
	wantAddresses := []string{"primary@example.com", "alias@example.com"}
	if fmt.Sprint(identity.AccountEmailAddresses) != fmt.Sprint(wantAddresses) {
		t.Fatalf(
			"AccountEmailAddresses = %#v, want %#v",
			identity.AccountEmailAddresses,
			wantAddresses,
		)
	}
}

func TestResolveMessageIdentityAllowsEmptyAccountEmails(t *testing.T) {
	run := func(string, ...string) (string, error) {
		return `{
			"localID":"42",
			"rfcMessageID":"message@example.com",
			"accountID":"account-id",
			"accountName":"Local",
			"accountEmailAddresses":[],
			"mailboxName":"INBOX"
		}`, nil
	}

	identity, err := resolveMessageIdentity(run, "Local", "INBOX", "42")
	if err != nil {
		t.Fatalf("resolveMessageIdentity() error = %v", err)
	}
	if identity.AccountEmailAddresses == nil {
		t.Fatal("AccountEmailAddresses = nil, want preserved empty array")
	}
	if len(identity.AccountEmailAddresses) != 0 {
		t.Fatalf("AccountEmailAddresses = %#v, want empty", identity.AccountEmailAddresses)
	}
}

func TestResolveMessageIdentityErrors(t *testing.T) {
	tests := []struct {
		code string
		want error
	}{
		{code: "account_not_found", want: ErrAccountNotFound},
		{code: "account_ambiguous", want: ErrAccountAmbiguous},
		{code: "mailbox_not_found", want: ErrMailboxNotFound},
		{code: "message_not_found", want: ErrMessageNotFound},
		{code: "message_id_missing", want: ErrMessageIDMissing},
	}

	for _, test := range tests {
		t.Run(test.code, func(t *testing.T) {
			run := func(string, ...string) (string, error) {
				return fmt.Sprintf(`{"error":%q}`, test.code), nil
			}
			_, err := resolveMessageIdentity(run, "Personal", "INBOX", "42")
			if !errors.Is(err, test.want) {
				t.Fatalf("resolveMessageIdentity() error = %v, want %v", err, test.want)
			}
		})
	}

	t.Run("unknown code is redacted", func(t *testing.T) {
		generatedCode := "unknown-runtime-secret-" + randomMailTestHex(t)
		run := func(string, ...string) (string, error) {
			return fmt.Sprintf(`{"error":%q}`, generatedCode), nil
		}
		_, err := resolveMessageIdentity(run, "Personal", "INBOX", "42")
		if err == nil {
			t.Fatal("resolveMessageIdentity() error = nil")
		}
		if strings.Contains(err.Error(), generatedCode) {
			t.Fatalf("error exposed resolver output: %v", err)
		}
	})

	t.Run("runner failure", func(t *testing.T) {
		runnerErr := errors.New("runner unavailable")
		run := func(string, ...string) (string, error) {
			return "", runnerErr
		}
		_, err := resolveMessageIdentity(run, "Personal", "INBOX", "42")
		if !errors.Is(err, runnerErr) {
			t.Fatalf("resolveMessageIdentity() error = %v, want runner error", err)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		run := func(string, ...string) (string, error) {
			return "not-json", nil
		}
		_, err := resolveMessageIdentity(run, "Personal", "INBOX", "42")
		if err == nil {
			t.Fatal("resolveMessageIdentity() error = nil")
		}
		if strings.Contains(err.Error(), "not-json") {
			t.Fatalf("error exposed resolver output: %v", err)
		}
	})
}

func TestResolveMessageIdentityRejectsBlankFields(t *testing.T) {
	tests := []struct {
		name  string
		field string
	}{
		{name: "local ID", field: "localID"},
		{name: "RFC Message-ID", field: "rfcMessageID"},
		{name: "account ID", field: "accountID"},
		{name: "account name", field: "accountName"},
		{name: "mailbox name", field: "mailboxName"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			values := map[string]string{
				"localID":      "42",
				"rfcMessageID": "message@example.com",
				"accountID":    "account-id",
				"accountName":  "Personal",
				"mailboxName":  "INBOX",
			}
			values[test.field] = " \t "
			output := fmt.Sprintf(
				`{"localID":%q,"rfcMessageID":%q,"accountID":%q,"accountName":%q,"accountEmailAddresses":[],"mailboxName":%q}`,
				values["localID"],
				values["rfcMessageID"],
				values["accountID"],
				values["accountName"],
				values["mailboxName"],
			)
			run := func(string, ...string) (string, error) {
				return output, nil
			}

			if _, err := resolveMessageIdentity(run, "Personal", "INBOX", "42"); err == nil {
				t.Fatal("resolveMessageIdentity() error = nil")
			}
		})
	}
}

func randomMailTestHex(t *testing.T) string {
	t.Helper()
	var value [8]byte
	if _, err := rand.Read(value[:]); err != nil {
		t.Fatalf("rand.Read() error = %v", err)
	}
	return hex.EncodeToString(value[:])
}
