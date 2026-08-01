//go:build !darwin || !cgo

package gmailauth

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestUnsupportedStore(t *testing.T) {
	store := NewStore()
	record := validAuthorizationRecord()
	record.Token.RefreshToken = "runtime-refresh-secret-" + randomTestHex(t)

	tests := []struct {
		name string
		run  func() error
	}{
		{name: "Get", run: func() error {
			_, err := store.Get(context.Background(), record.MailAccountID)
			return err
		}},
		{name: "Put", run: func() error {
			return store.Put(context.Background(), record)
		}},
		{name: "Delete", run: func() error {
			return store.Delete(context.Background(), record.MailAccountID)
		}},
		{name: "List", run: func() error {
			_, err := store.List(context.Background())
			return err
		}},
	}

	var wantMessage string
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.run()
			if !errors.Is(err, ErrUnsupportedPlatform) {
				t.Fatalf("%s() error = %v, want ErrUnsupportedPlatform", test.name, err)
			}
			if !strings.Contains(err.Error(), "macOS") ||
				!strings.Contains(err.Error(), "CGO_ENABLED=1") {
				t.Fatalf("%s() error = %q, want platform requirements", test.name, err)
			}
			if strings.Contains(err.Error(), record.Token.RefreshToken) {
				t.Fatalf("%s() error exposed token material: %v", test.name, err)
			}
			if wantMessage == "" {
				wantMessage = err.Error()
			} else if err.Error() != wantMessage {
				t.Fatalf("%s() error = %q, want deterministic %q", test.name, err, wantMessage)
			}
		})
	}
}
