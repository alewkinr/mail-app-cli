package gmailapi

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestNewProfileValidationError(t *testing.T) {
	t.Run("forbidden is actionable", func(t *testing.T) {
		cause := &HTTPError{
			StatusCode: http.StatusForbidden,
			Operation:  "get profile",
		}
		err := NewProfileValidationError(cause)
		if !errors.Is(err, cause) ||
			!strings.Contains(err.Error(), "enable the Gmail API") ||
			!strings.Contains(err.Error(), "HTTP status 403") {
			t.Fatalf("NewProfileValidationError() = %v", err)
		}
	})

	t.Run("unknown cause is redacted", func(t *testing.T) {
		secret := "provider-response-secret"
		err := NewProfileValidationError(errors.New(secret))
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("NewProfileValidationError() exposed cause: %v", err)
		}
	})
}
