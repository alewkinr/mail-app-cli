package mail

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestSelectAccountByName(t *testing.T) {
	t.Run("no match", func(t *testing.T) {
		_, err := selectAccountByName(
			[]Account{{Name: "Personal"}},
			"Work",
		)
		if !errors.Is(err, ErrAccountNotFound) {
			t.Fatalf("selectAccountByName() error = %v, want ErrAccountNotFound", err)
		}
	})

	t.Run("one exact match", func(t *testing.T) {
		accounts := []Account{
			{ID: "one", Name: "Personal"},
			{ID: "two", Name: "Work"},
		}
		account, err := selectAccountByName(accounts, "Work")
		if err != nil {
			t.Fatalf("selectAccountByName() error = %v", err)
		}
		if account.ID != "two" {
			t.Fatalf("selectAccountByName() ID = %q, want %q", account.ID, "two")
		}
	})

	t.Run("multiple exact matches", func(t *testing.T) {
		_, err := selectAccountByName(
			[]Account{{Name: "Work"}, {Name: "Work"}},
			"Work",
		)
		if !errors.Is(err, ErrAccountAmbiguous) {
			t.Fatalf("selectAccountByName() error = %v, want ErrAccountAmbiguous", err)
		}
	})
}

func TestAccountJSONBackwardCompatibility(t *testing.T) {
	const input = `{"id":"account-id","name":"Work","emailAddress":"first@example.com"}`

	var account Account
	if err := json.Unmarshal([]byte(input), &account); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if account.EmailAddress != "first@example.com" {
		t.Fatalf("EmailAddress = %q, want %q", account.EmailAddress, "first@example.com")
	}
	if account.EmailAddresses != nil {
		t.Fatalf("EmailAddresses = %#v, want nil for legacy JSON", account.EmailAddresses)
	}
}
