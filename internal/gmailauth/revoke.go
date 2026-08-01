package gmailauth

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const (
	googleRevocationURL       = "https://oauth2.googleapis.com/revoke"
	maxRevocationResponseSize = 64 << 10
)

type Revoker interface {
	Revoke(context.Context, string) error
}

type RevocationHTTPError struct {
	StatusCode int
}

func (err *RevocationHTTPError) Error() string {
	return fmt.Sprintf(
		"Google OAuth revocation failed with HTTP status %d",
		err.StatusCode,
	)
}

type googleRevoker struct {
	httpClient *http.Client
	endpoint   *url.URL
}

func NewGoogleRevoker(httpClient *http.Client) Revoker {
	endpoint, err := url.Parse(googleRevocationURL)
	if err != nil {
		panic("invalid Google OAuth revocation URL")
	}
	return newGoogleRevoker(httpClient, endpoint)
}

func newGoogleRevoker(httpClient *http.Client, endpoint *url.URL) Revoker {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	endpointCopy := *endpoint
	return &googleRevoker{
		httpClient: httpClient,
		endpoint:   &endpointCopy,
	}
}

func (revoker *googleRevoker) Revoke(ctx context.Context, refreshToken string) error {
	if strings.TrimSpace(refreshToken) == "" {
		return errors.New("refresh token is required for revocation")
	}

	form := url.Values{}
	form.Set("token", refreshToken)
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		revoker.endpoint.String(),
		bytes.NewBufferString(form.Encode()),
	)
	if err != nil {
		return errors.New("build Google OAuth revocation request failed")
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	response, err := revoker.httpClient.Do(request)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return errors.New("Google OAuth revocation request failed")
	}
	defer response.Body.Close()

	data, err := io.ReadAll(io.LimitReader(response.Body, maxRevocationResponseSize+1))
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return errors.New("read Google OAuth revocation response failed")
	}
	if response.StatusCode != http.StatusOK {
		return &RevocationHTTPError{StatusCode: response.StatusCode}
	}
	if len(data) > maxRevocationResponseSize {
		return errors.New("Google OAuth revocation response exceeds 64 KiB")
	}
	return nil
}
