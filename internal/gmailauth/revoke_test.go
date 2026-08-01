package gmailauth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestGoogleRevoker(t *testing.T) {
	refreshToken := "runtime-refresh-" + randomTestHex(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", request.Method)
		}
		if request.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Errorf("Content-Type = %q", request.Header.Get("Content-Type"))
		}
		if strings.Contains(request.URL.RawQuery, refreshToken) {
			t.Errorf("refresh token appeared in revocation URL")
		}
		if err := request.ParseForm(); err != nil {
			t.Errorf("ParseForm() error = %v", err)
		}
		if request.PostForm.Get("token") != refreshToken {
			t.Errorf("revocation body token does not match input")
		}
		writer.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	endpoint, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	revoker := newGoogleRevoker(server.Client(), endpoint)
	if err := revoker.Revoke(context.Background(), refreshToken); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	if requests.Load() != 1 {
		t.Fatalf("requests = %d, want 1", requests.Load())
	}
}

func TestGoogleRevokerStatusErrorsAreTypedAndRedacted(t *testing.T) {
	refreshToken := "runtime-refresh-" + randomTestHex(t)
	bodySecret := "runtime-response-" + randomTestHex(t)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(writer, bodySecret)
	}))
	t.Cleanup(server.Close)
	endpoint, _ := url.Parse(server.URL)
	revoker := newGoogleRevoker(server.Client(), endpoint)

	err := revoker.Revoke(context.Background(), refreshToken)
	var statusErr *RevocationHTTPError
	if !errors.As(err, &statusErr) {
		t.Fatalf("Revoke() error = %v, want RevocationHTTPError", err)
	}
	if statusErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("StatusCode = %d, want 400", statusErr.StatusCode)
	}
	if strings.Contains(err.Error(), refreshToken) || strings.Contains(err.Error(), bodySecret) {
		t.Fatalf("Revoke() error exposed token or response: %v", err)
	}
}

func TestGoogleRevokerLimitsSuccessResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, strings.Repeat("x", maxRevocationResponseSize+1))
	}))
	t.Cleanup(server.Close)
	endpoint, _ := url.Parse(server.URL)
	revoker := newGoogleRevoker(server.Client(), endpoint)

	if err := revoker.Revoke(context.Background(), "refresh-token"); err == nil {
		t.Fatal("Revoke() error = nil for oversized response")
	}
}

func TestGoogleRevokerContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	endpoint, _ := url.Parse("https://oauth.test/revoke")
	client := &http.Client{
		Transport: tokenRoundTripperFunc(func(request *http.Request) (*http.Response, error) {
			<-request.Context().Done()
			return nil, request.Context().Err()
		}),
	}
	revoker := newGoogleRevoker(client, endpoint)
	if err := revoker.Revoke(ctx, "refresh-token"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Revoke() error = %v, want context.Canceled", err)
	}
}

func TestGoogleRevokerRejectsBlankTokenWithoutRequest(t *testing.T) {
	var requests atomic.Int32
	client := &http.Client{
		Transport: tokenRoundTripperFunc(func(*http.Request) (*http.Response, error) {
			requests.Add(1)
			return nil, errors.New("unexpected request")
		}),
	}
	endpoint, _ := url.Parse("https://oauth.test/revoke")
	revoker := newGoogleRevoker(client, endpoint)
	if err := revoker.Revoke(context.Background(), " \t "); err == nil {
		t.Fatal("Revoke() error = nil")
	}
	if requests.Load() != 0 {
		t.Fatalf("requests = %d, want 0", requests.Load())
	}
}
