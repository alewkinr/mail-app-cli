package gmailapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestGetProfile(t *testing.T) {
	var gotMethod string
	var gotPath string
	client := newTestClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		gotMethod = request.Method
		gotPath = request.URL.Path
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"emailAddress":"primary@example.com"}`)
	}))

	profile, err := client.GetProfile(context.Background())
	if err != nil {
		t.Fatalf("GetProfile() error = %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("method = %q, want GET", gotMethod)
	}
	if gotPath != "/gmail/v1/users/me/profile" {
		t.Fatalf("path = %q, want profile endpoint", gotPath)
	}
	if profile.EmailAddress != "primary@example.com" {
		t.Fatalf("EmailAddress = %q, want primary@example.com", profile.EmailAddress)
	}
}

func TestGetProfileRejectsInvalidResponses(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "missing email", body: `{}`},
		{name: "blank email", body: `{"emailAddress":" \t "}`},
		{name: "malformed JSON", body: `{"emailAddress":`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := newTestClient(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(writer, test.body)
			}))

			_, err := client.GetProfile(context.Background())
			if !errors.Is(err, ErrInvalidResponse) {
				t.Fatalf("GetProfile() error = %v, want ErrInvalidResponse", err)
			}
			if strings.Contains(err.Error(), test.body) {
				t.Fatalf("GetProfile() error exposed response body: %v", err)
			}
		})
	}
}

func TestResponseBodyLimit(t *testing.T) {
	bodySecret := "response-secret-" + randomTestSecret(t)
	client := newTestClient(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, strings.Repeat("x", maxResponseBodyBytes)+bodySecret)
	}))

	_, err := client.GetProfile(context.Background())
	if !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("GetProfile() error = %v, want ErrInvalidResponse", err)
	}
	if strings.Contains(err.Error(), bodySecret) {
		t.Fatalf("GetProfile() error exposed oversized response body: %v", err)
	}
}

func TestHTTPStatusErrors(t *testing.T) {
	tests := []struct {
		status       int
		wantSentinel error
	}{
		{status: http.StatusBadRequest},
		{status: http.StatusUnauthorized, wantSentinel: ErrNotAuthorized},
		{status: http.StatusForbidden},
		{status: http.StatusNotFound, wantSentinel: ErrMessageNotFound},
		{status: http.StatusTooManyRequests},
		{status: http.StatusInternalServerError},
		{status: http.StatusServiceUnavailable},
	}

	for _, test := range tests {
		t.Run(http.StatusText(test.status), func(t *testing.T) {
			tokenSecret := "authorization-secret-" + randomTestSecret(t)
			bodySecret := "response-secret-" + randomTestSecret(t)
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Header.Get("Authorization") != "Bearer "+tokenSecret {
					t.Errorf("Authorization header was not propagated by test transport")
				}
				writer.WriteHeader(test.status)
				_, _ = io.WriteString(writer, bodySecret)
			}))
			t.Cleanup(server.Close)

			baseURL, err := url.Parse(server.URL + "/gmail/v1")
			if err != nil {
				t.Fatalf("url.Parse() error = %v", err)
			}
			transport := roundTripperFunc(func(request *http.Request) (*http.Response, error) {
				request.Header.Set("Authorization", "Bearer "+tokenSecret)
				return http.DefaultTransport.RoundTrip(request)
			})
			client := newClient(&http.Client{Transport: transport}, baseURL)

			_, err = client.GetProfile(context.Background())
			var httpErr *HTTPError
			if !errors.As(err, &httpErr) {
				t.Fatalf("GetProfile() error = %v, want HTTPError", err)
			}
			if httpErr.StatusCode != test.status {
				t.Fatalf("StatusCode = %d, want %d", httpErr.StatusCode, test.status)
			}
			if httpErr.Operation != "get profile" {
				t.Fatalf("Operation = %q, want get profile", httpErr.Operation)
			}
			if test.wantSentinel != nil && !errors.Is(err, test.wantSentinel) {
				t.Fatalf("GetProfile() error = %v, want %v", err, test.wantSentinel)
			}
			if strings.Contains(err.Error(), tokenSecret) ||
				strings.Contains(err.Error(), bodySecret) {
				t.Fatalf("HTTPError exposed authorization or response data: %v", err)
			}
		})
	}
}

func TestRetryAfter(t *testing.T) {
	t.Run("delta seconds", func(t *testing.T) {
		client := newTestClient(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Retry-After", "17")
			writer.WriteHeader(http.StatusTooManyRequests)
		}))

		_, err := client.GetProfile(context.Background())
		var httpErr *HTTPError
		if !errors.As(err, &httpErr) {
			t.Fatalf("GetProfile() error = %v, want HTTPError", err)
		}
		if httpErr.RetryAfter != 17*time.Second {
			t.Fatalf("RetryAfter = %s, want 17s", httpErr.RetryAfter)
		}
	})

	t.Run("HTTP date", func(t *testing.T) {
		retryAt := time.Now().Add(30 * time.Second).UTC().Truncate(time.Second)
		client := newTestClient(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Retry-After", retryAt.Format(http.TimeFormat))
			writer.WriteHeader(http.StatusServiceUnavailable)
		}))

		_, err := client.GetProfile(context.Background())
		var httpErr *HTTPError
		if !errors.As(err, &httpErr) {
			t.Fatalf("GetProfile() error = %v, want HTTPError", err)
		}
		if httpErr.RetryAfter < 28*time.Second || httpErr.RetryAfter > 31*time.Second {
			t.Fatalf("RetryAfter = %s, want approximately 30s", httpErr.RetryAfter)
		}
	})
}

func TestFindInboxMessageByRFCMessageID(t *testing.T) {
	var gotRawQuery string
	client := newTestClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", request.Method)
		}
		if request.URL.Path != "/gmail/v1/users/me/messages" {
			t.Errorf("path = %q, want messages endpoint", request.URL.Path)
		}
		gotRawQuery = request.URL.RawQuery
		query := request.URL.Query()
		if got := query["labelIds"]; len(got) != 1 || got[0] != "INBOX" {
			t.Errorf("labelIds = %#v, want [INBOX]", got)
		}
		if got := query["maxResults"]; len(got) != 1 || got[0] != "2" {
			t.Errorf("maxResults = %#v, want [2]", got)
		}
		if got := query["q"]; len(got) != 1 || got[0] != "rfc822msgid:message+tag@example.com" {
			t.Errorf("q = %#v, want encoded RFC Message-ID search", got)
		}
		_, _ = io.WriteString(writer, `{"messages":[{"id":"gmail-message-id"}]}`)
	}))

	messageID, err := client.FindInboxMessageByRFCMessageID(
		context.Background(),
		" <message+tag@example.com> ",
	)
	if err != nil {
		t.Fatalf("FindInboxMessageByRFCMessageID() error = %v", err)
	}
	if messageID != "gmail-message-id" {
		t.Fatalf("message ID = %q, want gmail-message-id", messageID)
	}
	if strings.Contains(gotRawQuery, "message+tag") ||
		!strings.Contains(gotRawQuery, "message%2Btag") {
		t.Fatalf("RawQuery = %q, want plus sign encoded through url.Values", gotRawQuery)
	}
}

func TestFindInboxMessageOutcomes(t *testing.T) {
	tests := []struct {
		name string
		body string
		want error
	}{
		{name: "zero results", body: `{}`, want: ErrMessageNotFound},
		{name: "empty messages", body: `{"messages":[]}`, want: ErrMessageNotFound},
		{
			name: "multiple results",
			body: `{"messages":[{"id":"one"},{"id":"two"}]}`,
			want: ErrAmbiguousMessage,
		},
		{
			name: "next page with result",
			body: `{"messages":[{"id":"one"}],"nextPageToken":"next"}`,
			want: ErrAmbiguousMessage,
		},
		{
			name: "next page without result",
			body: `{"messages":[],"nextPageToken":"next"}`,
			want: ErrAmbiguousMessage,
		},
		{name: "blank result ID", body: `{"messages":[{"id":" \t"}]}`, want: ErrInvalidResponse},
		{name: "malformed JSON", body: `{"messages":`, want: ErrInvalidResponse},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := newTestClient(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(writer, test.body)
			}))

			_, err := client.FindInboxMessageByRFCMessageID(
				context.Background(),
				"message@example.com",
			)
			if !errors.Is(err, test.want) {
				t.Fatalf(
					"FindInboxMessageByRFCMessageID() error = %v, want %v",
					err,
					test.want,
				)
			}
		})
	}
}

func TestLookupFailuresNeverTriggerMutation(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		status int
		body   string
	}{
		{name: "invalid RFC Message-ID", input: "message\nid@example.com"},
		{name: "zero results", input: "message@example.com", body: `{}`},
		{
			name:  "multiple results",
			input: "message@example.com",
			body:  `{"messages":[{"id":"one"},{"id":"two"}]}`,
		},
		{
			name:  "next page",
			input: "message@example.com",
			body:  `{"messages":[{"id":"one"}],"nextPageToken":"next"}`,
		},
		{
			name:  "blank result ID",
			input: "message@example.com",
			body:  `{"messages":[{"id":""}]}`,
		},
		{name: "malformed response", input: "message@example.com", body: `{"messages":`},
		{
			name:   "not authorized",
			input:  "message@example.com",
			status: http.StatusUnauthorized,
		},
		{
			name:   "rate limited",
			input:  "message@example.com",
			status: http.StatusTooManyRequests,
		},
		{
			name:  "oversized response",
			input: "message@example.com",
			body:  strings.Repeat("x", maxResponseBodyBytes+1),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var mutationRequests atomic.Int32
			client := newTestClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method == http.MethodPost {
					mutationRequests.Add(1)
					_, _ = io.WriteString(writer, `{"id":"unexpected","labelIds":[]}`)
					return
				}
				if test.status != 0 {
					writer.WriteHeader(test.status)
				}
				_, _ = io.WriteString(writer, test.body)
			}))

			messageID, err := client.FindInboxMessageByRFCMessageID(
				context.Background(),
				test.input,
			)
			if err == nil {
				err = client.RemoveInboxLabel(context.Background(), messageID)
			}
			if err == nil {
				t.Fatal("lookup workflow error = nil")
			}
			if got := mutationRequests.Load(); got != 0 {
				t.Fatalf("mutation requests = %d, want 0", got)
			}
		})
	}
}

func TestRemoveInboxLabel(t *testing.T) {
	gmailMessageID := "gmail/id with space"
	var gotBody string
	var gotEscapedPath string
	var gotContentType string
	client := newTestClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", request.Method)
		}
		gotEscapedPath = request.URL.EscapedPath()
		gotContentType = request.Header.Get("Content-Type")
		data, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		gotBody = string(data)
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"id":       gmailMessageID,
			"labelIds": []string{"STARRED", "IMPORTANT"},
		})
	}))

	if err := client.RemoveInboxLabel(context.Background(), gmailMessageID); err != nil {
		t.Fatalf("RemoveInboxLabel() error = %v", err)
	}
	wantPath := "/gmail/v1/users/me/messages/" + url.PathEscape(gmailMessageID) + "/modify"
	if gotEscapedPath != wantPath {
		t.Fatalf("escaped path = %q, want %q", gotEscapedPath, wantPath)
	}
	if gotContentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", gotContentType)
	}
	if gotBody != `{"removeLabelIds":["INBOX"]}` {
		t.Fatalf("request body = %q, want exact removeLabelIds JSON", gotBody)
	}
}

func TestRemoveInboxLabelRejectsInvalidInputsAndResponses(t *testing.T) {
	t.Run("blank input performs no request", func(t *testing.T) {
		var requests atomic.Int32
		client := newTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			requests.Add(1)
		}))
		if err := client.RemoveInboxLabel(context.Background(), " \t "); err == nil {
			t.Fatal("RemoveInboxLabel() error = nil")
		}
		if got := requests.Load(); got != 0 {
			t.Fatalf("requests = %d, want 0", got)
		}
	})

	tests := []struct {
		name string
		body string
	}{
		{name: "missing response ID", body: `{"labelIds":[]}`},
		{name: "blank response ID", body: `{"id":" ","labelIds":[]}`},
		{name: "mismatched response ID", body: `{"id":"other","labelIds":[]}`},
		{name: "INBOX unchanged", body: `{"id":"gmail-id","labelIds":["INBOX","STARRED"]}`},
		{name: "malformed JSON", body: `{"id":`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := newTestClient(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(writer, test.body)
			}))
			err := client.RemoveInboxLabel(context.Background(), "gmail-id")
			if !errors.Is(err, ErrInvalidResponse) {
				t.Fatalf("RemoveInboxLabel() error = %v, want ErrInvalidResponse", err)
			}
		})
	}
}

func TestGmailClientDoesNotRetry(t *testing.T) {
	tests := []struct {
		name   string
		status int
		run    func(*Client) error
	}{
		{
			name:   "lookup 429",
			status: http.StatusTooManyRequests,
			run: func(client *Client) error {
				_, err := client.FindInboxMessageByRFCMessageID(
					context.Background(),
					"message@example.com",
				)
				return err
			},
		},
		{
			name:   "mutation 503",
			status: http.StatusServiceUnavailable,
			run: func(client *Client) error {
				return client.RemoveInboxLabel(context.Background(), "gmail-id")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var requests atomic.Int32
			client := newTestClient(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				writer.WriteHeader(test.status)
			}))

			var httpErr *HTTPError
			if err := test.run(client); !errors.As(err, &httpErr) {
				t.Fatalf("operation error = %v, want HTTPError", err)
			}
			if got := requests.Load(); got != 1 {
				t.Fatalf("requests = %d, want exactly 1", got)
			}
		})
	}
}

func TestGmailClientPropagatesContextErrors(t *testing.T) {
	operations := []struct {
		name string
		run  func(context.Context, *Client) error
	}{
		{name: "profile", run: func(ctx context.Context, client *Client) error {
			_, err := client.GetProfile(ctx)
			return err
		}},
		{name: "lookup", run: func(ctx context.Context, client *Client) error {
			_, err := client.FindInboxMessageByRFCMessageID(ctx, "message@example.com")
			return err
		}},
		{name: "mutation", run: func(ctx context.Context, client *Client) error {
			return client.RemoveInboxLabel(ctx, "gmail-id")
		}},
	}

	for _, operation := range operations {
		t.Run(operation.name+" canceled", func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			client := clientWithContextTransport(t)
			if err := operation.run(ctx, client); !errors.Is(err, context.Canceled) {
				t.Fatalf("operation error = %v, want context.Canceled", err)
			}
		})

		t.Run(operation.name+" deadline", func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()
			client := clientWithContextTransport(t)
			if err := operation.run(ctx, client); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("operation error = %v, want context.DeadlineExceeded", err)
			}
		})
	}
}

func newTestClient(t *testing.T, handler http.Handler) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	baseURL, err := url.Parse(server.URL + "/gmail/v1")
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	return newClient(server.Client(), baseURL)
}

func clientWithContextTransport(t *testing.T) *Client {
	t.Helper()
	baseURL, err := url.Parse("https://gmail.test/gmail/v1")
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	httpClient := &http.Client{
		Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			<-request.Context().Done()
			return nil, request.Context().Err()
		}),
	}
	return newClient(httpClient, baseURL)
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (function roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func randomTestSecret(t *testing.T) string {
	t.Helper()
	var value [12]byte
	if _, err := rand.Read(value[:]); err != nil {
		t.Fatalf("rand.Read() error = %v", err)
	}
	return hex.EncodeToString(value[:])
}
