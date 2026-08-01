package gmailapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	gmailAPIBaseURL      = "https://gmail.googleapis.com/gmail/v1"
	maxResponseBodyBytes = 1 << 20
)

type Client struct {
	httpClient *http.Client
	baseURL    *url.URL
}

type Profile struct {
	EmailAddress string `json:"emailAddress"`
}

// NewClient creates a Gmail REST client using Google's production API.
func NewClient(httpClient *http.Client) *Client {
	baseURL, err := url.Parse(gmailAPIBaseURL)
	if err != nil {
		panic("invalid Gmail API base URL")
	}
	return newClient(httpClient, baseURL)
}

func newClient(httpClient *http.Client, baseURL *url.URL) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	baseURLCopy := *baseURL
	return &Client{
		httpClient: httpClient,
		baseURL:    &baseURLCopy,
	}
}

func (client *Client) GetProfile(ctx context.Context) (Profile, error) {
	var profile Profile
	if err := client.doJSON(
		ctx,
		http.MethodGet,
		client.endpoint("users", "me", "profile"),
		"get profile",
		nil,
		&profile,
	); err != nil {
		return Profile{}, err
	}
	if strings.TrimSpace(profile.EmailAddress) == "" {
		return Profile{}, fmt.Errorf("%w: profile email address is blank", ErrInvalidResponse)
	}
	return profile, nil
}

func (client *Client) FindInboxMessageByRFCMessageID(
	ctx context.Context,
	rfcMessageID string,
) (string, error) {
	normalizedMessageID, err := NormalizeRFCMessageID(rfcMessageID)
	if err != nil {
		return "", err
	}

	endpoint := client.endpoint("users", "me", "messages")
	query := url.Values{}
	query.Set("labelIds", "INBOX")
	query.Set("maxResults", "2")
	query.Set("q", "rfc822msgid:"+normalizedMessageID)
	endpoint.RawQuery = query.Encode()

	var response struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := client.doJSON(
		ctx,
		http.MethodGet,
		endpoint,
		"find inbox message",
		nil,
		&response,
	); err != nil {
		return "", err
	}

	if len(response.Messages) > 1 || response.NextPageToken != "" {
		return "", fmt.Errorf("%w: inbox lookup was not unique", ErrAmbiguousMessage)
	}
	if len(response.Messages) == 0 {
		return "", ErrMessageNotFound
	}
	if strings.TrimSpace(response.Messages[0].ID) == "" {
		return "", fmt.Errorf("%w: message ID is blank", ErrInvalidResponse)
	}
	return response.Messages[0].ID, nil
}

func (client *Client) RemoveInboxLabel(ctx context.Context, gmailMessageID string) error {
	if strings.TrimSpace(gmailMessageID) == "" {
		return invalidInput("Gmail message ID")
	}

	endpoint := client.endpoint(
		"users",
		"me",
		"messages",
		gmailMessageID,
		"modify",
	)
	body := []byte(`{"removeLabelIds":["INBOX"]}`)
	var response struct {
		ID       string   `json:"id"`
		LabelIDs []string `json:"labelIds"`
	}
	if err := client.doJSON(
		ctx,
		http.MethodPost,
		endpoint,
		"remove inbox label",
		body,
		&response,
	); err != nil {
		return err
	}

	if strings.TrimSpace(response.ID) == "" {
		return fmt.Errorf("%w: modified message ID is blank", ErrInvalidResponse)
	}
	if response.ID != gmailMessageID {
		return fmt.Errorf("%w: modified message ID does not match request", ErrInvalidResponse)
	}
	for _, labelID := range response.LabelIDs {
		if labelID == "INBOX" {
			return fmt.Errorf("%w: INBOX label remains", ErrInvalidResponse)
		}
	}
	return nil
}

func (client *Client) endpoint(pathSegments ...string) *url.URL {
	endpoint := *client.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/")
	escapedPath := strings.TrimRight(endpoint.EscapedPath(), "/")
	for _, segment := range pathSegments {
		endpoint.Path += "/" + segment
		escapedPath += "/" + url.PathEscape(segment)
	}
	endpoint.RawPath = escapedPath
	endpoint.RawQuery = ""
	endpoint.Fragment = ""
	return &endpoint
}

func (client *Client) doJSON(
	ctx context.Context,
	method string,
	endpoint *url.URL,
	operation string,
	body []byte,
	result any,
) error {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), bodyReader)
	if err != nil {
		return fmt.Errorf("build Gmail API %s request: %w", operation, err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := client.httpClient.Do(request)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("Gmail API %s request failed: %w", operation, err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return &HTTPError{
			StatusCode: response.StatusCode,
			RetryAfter: parseRetryAfter(response.Header.Get("Retry-After"), time.Now()),
			Operation:  operation,
		}
	}

	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBodyBytes+1))
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("%w: read response", ErrInvalidResponse)
	}
	if len(data) > maxResponseBodyBytes {
		return fmt.Errorf("%w: response exceeds 1 MiB", ErrInvalidResponse)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if err := json.Unmarshal(data, result); err != nil {
		return fmt.Errorf("%w: decode response", ErrInvalidResponse)
	}
	return nil
}

func invalidInput(field string) error {
	return errors.New(field + " is required")
}
