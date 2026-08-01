//go:build darwin && cgo

package gmailauth

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	keychain "github.com/keybase/go-keychain"
)

func TestKeychainRecordEncodingEnforcesValidation(t *testing.T) {
	record := validAuthorizationRecord()
	record.SchemaVersion = 0
	record.Token.RefreshToken = "runtime-refresh-secret-" + randomTestHex(t)

	_, err := encodeAuthorizationRecord(record)
	if !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("encodeAuthorizationRecord() error = %v, want ErrInvalidRecord", err)
	}
	if strings.Contains(err.Error(), record.Token.RefreshToken) {
		t.Fatalf("encodeAuthorizationRecord() error exposed token material: %v", err)
	}
}

func TestKeychainRecordDecodingEnforcesValidation(t *testing.T) {
	generatedSecret := "runtime-decode-secret-" + randomTestHex(t)
	_, err := decodeAuthorizationRecord([]byte(generatedSecret))
	if !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("decodeAuthorizationRecord() error = %v, want ErrInvalidRecord", err)
	}
	if strings.Contains(err.Error(), generatedSecret) {
		t.Fatalf("decodeAuthorizationRecord() error exposed serialized data: %v", err)
	}

	record := validAuthorizationRecord()
	record.SchemaVersion = 0
	record.Token.RefreshToken = generatedSecret
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	_, err = decodeAuthorizationRecord(data)
	if !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("decodeAuthorizationRecord() validation error = %v, want ErrInvalidRecord", err)
	}
	if strings.Contains(err.Error(), generatedSecret) {
		t.Fatalf("decodeAuthorizationRecord() error exposed token material: %v", err)
	}
}

func TestKeychainRecordRoundTrip(t *testing.T) {
	record := validAuthorizationRecord()
	data, err := encodeAuthorizationRecord(record)
	if err != nil {
		t.Fatalf("encodeAuthorizationRecord() error = %v", err)
	}
	got, err := decodeAuthorizationRecord(data)
	if err != nil {
		t.Fatalf("decodeAuthorizationRecord() error = %v", err)
	}
	if got.MailAccountID != record.MailAccountID ||
		got.GmailEmail != record.GmailEmail ||
		got.Token.RefreshToken != record.Token.RefreshToken {
		t.Fatal("decoded record does not match encoded authorization")
	}
}

func TestKeychainStoreChecksContextBeforeOSCalls(t *testing.T) {
	store := newKeychainStore(keychainService + ".context-test")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := store.Get(ctx, "account-id"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Get() error = %v, want context.Canceled", err)
	}
	if err := store.Put(ctx, validAuthorizationRecord()); !errors.Is(err, context.Canceled) {
		t.Fatalf("Put() error = %v, want context.Canceled", err)
	}
	if err := store.Delete(ctx, "account-id"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Delete() error = %v, want context.Canceled", err)
	}
	if _, err := store.List(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("List() error = %v, want context.Canceled", err)
	}
}

func TestKeychainStoreRejectsBlankAccountKeys(t *testing.T) {
	store := newKeychainStore(keychainService + ".blank-key-test")

	if _, err := store.Get(context.Background(), " "); !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("Get() error = %v, want ErrInvalidRecord", err)
	}
	if err := store.Delete(context.Background(), ""); !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("Delete() error = %v, want ErrInvalidRecord", err)
	}
}

func TestKeychainUnavailableErrorsHaveRecoveryGuidance(t *testing.T) {
	for _, keychainErr := range []error{
		keychain.ErrorParam,
		keychain.ErrorNotAvailable,
		keychain.ErrorAuthFailed,
	} {
		err := keychainError("store authorization", keychainErr)
		if !errors.Is(err, keychainErr) {
			t.Fatalf("keychainError() did not preserve %v", keychainErr)
		}
		if !strings.Contains(err.Error(), "lock and unlock the login Keychain") {
			t.Fatalf("keychainError() lacks recovery guidance: %v", err)
		}
	}
}

func TestKeychainStoreCRUD(t *testing.T) {
	if os.Getenv("MAIL_APP_CLI_KEYCHAIN_TEST") != "1" {
		t.Skip("set MAIL_APP_CLI_KEYCHAIN_TEST=1 to run real Keychain CRUD")
	}

	suffix := randomTestHex(t)
	service := keychainService + ".test." + suffix
	store := newKeychainStore(service)
	record := validAuthorizationRecord()
	record.MailAccountID += "-" + suffix

	t.Cleanup(func() {
		_ = store.Delete(context.Background(), record.MailAccountID)
		_ = store.Delete(context.Background(), "malformed-"+suffix)
	})

	if err := store.Put(context.Background(), record); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	got, err := store.Get(context.Background(), record.MailAccountID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.MailAccountID != record.MailAccountID ||
		got.Token.RefreshToken != record.Token.RefreshToken {
		t.Fatal("Get() did not return the stored authorization")
	}

	record.GmailEmail = "updated@example.com"
	if err := store.Put(context.Background(), record); err != nil {
		t.Fatalf("update Put() error = %v", err)
	}
	got, err = store.Get(context.Background(), record.MailAccountID)
	if err != nil {
		t.Fatalf("Get() after update error = %v", err)
	}
	if got.GmailEmail != record.GmailEmail {
		t.Fatalf("GmailEmail = %q, want %q", got.GmailEmail, record.GmailEmail)
	}

	records, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(records) != 1 || records[0].MailAccountID != record.MailAccountID {
		t.Fatalf("List() returned %d records with unexpected account IDs", len(records))
	}

	malformedAccountID := "malformed-" + suffix
	malformedItem := keychain.NewGenericPassword(
		service,
		malformedAccountID,
		"mail-app-cli Gmail OAuth",
		[]byte("runtime-malformed-secret-"+suffix),
		"",
	)
	malformedItem.SetSynchronizable(keychain.SynchronizableNo)
	malformedItem.SetAccessible(keychain.AccessibleWhenUnlocked)
	if err := keychain.AddItem(malformedItem); err != nil {
		t.Fatalf("keychain.AddItem() error = %v", err)
	}
	if _, err := store.Get(context.Background(), malformedAccountID); !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("Get() malformed error = %v, want ErrInvalidRecord", err)
	}
	if _, err := store.List(context.Background()); !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("List() malformed error = %v, want ErrInvalidRecord", err)
	}

	if err := store.Delete(context.Background(), record.MailAccountID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := store.Get(context.Background(), record.MailAccountID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() after delete error = %v, want ErrNotFound", err)
	}
	if err := store.Delete(context.Background(), record.MailAccountID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete() missing error = %v, want ErrNotFound", err)
	}
}
