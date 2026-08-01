package gmailapi

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var (
	ErrNotAuthorized    = errors.New("Gmail authorization is invalid")
	ErrMessageNotFound  = errors.New("Gmail message not found")
	ErrAmbiguousMessage = errors.New("multiple Gmail messages matched")
	ErrInvalidResponse  = errors.New("invalid Gmail API response")
)

// NewProfileValidationError returns an actionable error without exposing the
// Gmail API response body. HTTPError contains only safe status metadata.
func NewProfileValidationError(err error) error {
	const operation = "validate Gmail profile"

	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		if httpErr.StatusCode == http.StatusForbidden {
			return fmt.Errorf(
				"%s: access forbidden; enable the Gmail API for the OAuth client's Google Cloud project; if it is already enabled, check Workspace API policy and project quota: %w",
				operation,
				httpErr,
			)
		}
		return fmt.Errorf("%s: %w", operation, httpErr)
	}
	if errors.Is(err, ErrInvalidResponse) {
		return fmt.Errorf("%s: %w", operation, ErrInvalidResponse)
	}
	return errors.New(operation + " failed")
}

// HTTPError contains safe status metadata for a failed Gmail API request.
// It intentionally excludes request and response payloads.
type HTTPError struct {
	StatusCode int
	RetryAfter time.Duration
	Operation  string
}

func (err *HTTPError) Error() string {
	if err.RetryAfter > 0 {
		return fmt.Sprintf(
			"Gmail API %s failed with HTTP status %d; retry after %s",
			err.Operation,
			err.StatusCode,
			err.RetryAfter,
		)
	}
	return fmt.Sprintf(
		"Gmail API %s failed with HTTP status %d",
		err.Operation,
		err.StatusCode,
	)
}

func (err *HTTPError) Unwrap() error {
	switch err.StatusCode {
	case http.StatusUnauthorized:
		return ErrNotAuthorized
	case http.StatusNotFound:
		return ErrMessageNotFound
	default:
		return nil
	}
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}

	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		if seconds <= 0 {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}

	retryAt, err := http.ParseTime(value)
	if err != nil {
		return 0
	}
	delay := retryAt.Sub(now)
	if delay <= 0 {
		return 0
	}
	return delay
}
