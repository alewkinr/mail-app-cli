package gmailauth

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/oauth2"
)

const (
	googleAuthorizationURL = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL         = "https://oauth2.googleapis.com/token"
	gmailModifyScope       = "https://www.googleapis.com/auth/gmail.modify"
)

var (
	appOAuthClientID            string
	appOAuthClientSecret        string
	ErrOAuthClientNotConfigured = errors.New("Gmail OAuth client is not configured")
)

type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	AuthURL      string
	TokenURL     string
}

func newOAuth2Config(config OAuthConfig, redirectURL string) (*oauth2.Config, error) {
	clientID := strings.TrimSpace(config.ClientID)
	if clientID == "" {
		return nil, fmt.Errorf("%w: client ID is blank", ErrOAuthClientNotConfigured)
	}
	clientSecret := strings.TrimSpace(config.ClientSecret)
	if clientSecret == "" {
		return nil, fmt.Errorf(
			"%w: Desktop client secret is blank",
			ErrOAuthClientNotConfigured,
		)
	}
	if strings.TrimSpace(config.AuthURL) == "" || strings.TrimSpace(config.TokenURL) == "" {
		return nil, errors.New("Gmail OAuth endpoints are not configured")
	}
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:   config.AuthURL,
			TokenURL:  config.TokenURL,
			AuthStyle: oauth2.AuthStyleInParams,
		},
		RedirectURL: redirectURL,
		Scopes:      []string{gmailModifyScope},
	}, nil
}

func AppOAuthConfig() (OAuthConfig, error) {
	return newOAuthConfig(appOAuthClientID, appOAuthClientSecret)
}

func newOAuthConfig(clientID, clientSecret string) (OAuthConfig, error) {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return OAuthConfig{}, fmt.Errorf(
			"%w: install a Gmail-enabled maintainer build",
			ErrOAuthClientNotConfigured,
		)
	}
	clientSecret = strings.TrimSpace(clientSecret)
	if clientSecret == "" {
		return OAuthConfig{}, fmt.Errorf(
			"%w: Desktop client secret is blank; install a Gmail-enabled maintainer build",
			ErrOAuthClientNotConfigured,
		)
	}
	return OAuthConfig{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		AuthURL:      googleAuthorizationURL,
		TokenURL:     googleTokenURL,
	}, nil
}
