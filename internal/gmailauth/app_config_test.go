package gmailauth

import (
	"errors"
	"testing"

	"golang.org/x/oauth2"
)

func TestNewOAuthConfig(t *testing.T) {
	for _, clientID := range []string{"", " \t "} {
		if _, err := newOAuthConfig(clientID, "desktop-client-secret"); !errors.Is(err, ErrOAuthClientNotConfigured) {
			t.Fatalf("newOAuthConfig(%q) error = %v, want ErrOAuthClientNotConfigured", clientID, err)
		}
	}
	for _, clientSecret := range []string{"", " \t "} {
		if _, err := newOAuthConfig("public-client-id.apps.googleusercontent.com", clientSecret); !errors.Is(err, ErrOAuthClientNotConfigured) {
			t.Fatalf("newOAuthConfig() error = %v, want ErrOAuthClientNotConfigured", err)
		}
	}

	config, err := newOAuthConfig(
		"  public-client-id.apps.googleusercontent.com  ",
		"  desktop-client-secret  ",
	)
	if err != nil {
		t.Fatalf("newOAuthConfig() error = %v", err)
	}
	if config.ClientID != "public-client-id.apps.googleusercontent.com" {
		t.Fatalf("ClientID was not trimmed")
	}
	if config.ClientSecret != "desktop-client-secret" {
		t.Fatalf("ClientSecret was not trimmed")
	}
	if config.AuthURL != googleAuthorizationURL || config.TokenURL != googleTokenURL {
		t.Fatalf("OAuth endpoints = %#v, want fixed Google endpoints", config)
	}
}

func TestAppOAuthConfigUsesInjectedClientID(t *testing.T) {
	previousID := appOAuthClientID
	previousSecret := appOAuthClientSecret
	t.Cleanup(func() {
		appOAuthClientID = previousID
		appOAuthClientSecret = previousSecret
	})

	appOAuthClientID = "injected-client-id.apps.googleusercontent.com"
	appOAuthClientSecret = "injected-client-secret"
	config, err := AppOAuthConfig()
	if err != nil {
		t.Fatalf("AppOAuthConfig() error = %v", err)
	}
	if config.ClientID != appOAuthClientID {
		t.Fatalf("ClientID = %q, want injected value", config.ClientID)
	}
	if config.ClientSecret != appOAuthClientSecret {
		t.Fatal("ClientSecret does not match injected value")
	}

	appOAuthClientID = ""
	if _, err := AppOAuthConfig(); !errors.Is(err, ErrOAuthClientNotConfigured) {
		t.Fatalf("AppOAuthConfig() error = %v, want ErrOAuthClientNotConfigured", err)
	}
	appOAuthClientID = "injected-client-id.apps.googleusercontent.com"
	appOAuthClientSecret = ""
	if _, err := AppOAuthConfig(); !errors.Is(err, ErrOAuthClientNotConfigured) {
		t.Fatalf("AppOAuthConfig() error = %v, want ErrOAuthClientNotConfigured", err)
	}
}

func TestOAuth2ConfigSecurityProperties(t *testing.T) {
	if _, err := newOAuth2Config(OAuthConfig{
		ClientID: "public-client-id.apps.googleusercontent.com",
		AuthURL:  googleAuthorizationURL,
		TokenURL: googleTokenURL,
	}, "http://127.0.0.1/callback"); !errors.Is(err, ErrOAuthClientNotConfigured) {
		t.Fatalf("newOAuth2Config() missing-secret error = %v", err)
	}

	config, err := newOAuth2Config(OAuthConfig{
		ClientID:     "public-client-id.apps.googleusercontent.com",
		ClientSecret: "desktop-client-secret",
		AuthURL:      googleAuthorizationURL,
		TokenURL:     googleTokenURL,
	}, "http://127.0.0.1/callback")
	if err != nil {
		t.Fatalf("newOAuth2Config() error = %v", err)
	}
	if config.Endpoint.AuthStyle != oauth2.AuthStyleInParams {
		t.Fatalf("AuthStyle = %v, want AuthStyleInParams", config.Endpoint.AuthStyle)
	}
	if config.ClientSecret != "desktop-client-secret" {
		t.Fatal("oauth2.Config does not contain the Desktop client secret")
	}
	if len(config.Scopes) != 1 || config.Scopes[0] != gmailModifyScope {
		t.Fatalf("Scopes = %#v, want exact gmail.modify scope", config.Scopes)
	}
}
