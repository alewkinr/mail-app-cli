package archive

import (
	"context"

	"github.com/intelligrit/mail-app-cli/internal/gmailapi"
	"github.com/intelligrit/mail-app-cli/internal/gmailauth"
	"golang.org/x/oauth2"
)

type gmailClientFactory struct {
	store           gmailauth.Store
	loadOAuthConfig func() (gmailauth.OAuthConfig, error)
}

func NewGmailClientFactory(store gmailauth.Store) TokenClientFactory {
	return &gmailClientFactory{
		store:           store,
		loadOAuthConfig: gmailauth.AppOAuthConfig,
	}
}

func (factory *gmailClientFactory) NewClient(
	ctx context.Context,
	record gmailauth.AuthorizationRecord,
) (GmailClient, error) {
	oauthConfig, err := factory.loadOAuthConfig()
	if err != nil {
		return nil, err
	}
	source, err := gmailauth.NewPersistingTokenSource(ctx, record, factory.store, oauthConfig)
	if err != nil {
		return nil, err
	}
	httpClient := oauth2.NewClient(ctx, source)
	return gmailapi.NewClient(httpClient), nil
}
