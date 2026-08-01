package archive

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/intelligrit/mail-app-cli/internal/gmailapi"
	"github.com/intelligrit/mail-app-cli/internal/gmailauth"
	"github.com/intelligrit/mail-app-cli/pkg/mail"
)

type MessageResolver interface {
	ResolveMessageIdentity(
		accountName,
		mailboxName,
		localMessageID string,
	) (*mail.MessageIdentity, error)
}

type MailArchiver interface {
	ArchiveMessage(accountName, mailboxName, localMessageID string) error
}

type AuthorizationStore interface {
	Get(context.Context, string) (gmailauth.AuthorizationRecord, error)
}

type TokenClientFactory interface {
	NewClient(
		context.Context,
		gmailauth.AuthorizationRecord,
	) (GmailClient, error)
}

type GmailClient interface {
	FindInboxMessageByRFCMessageID(context.Context, string) (string, error)
	RemoveInboxLabel(context.Context, string) error
}

type Backend string

const (
	BackendGmailAPI Backend = "gmail-api"
	BackendMailApp  Backend = "mail-app"
)

type Request struct {
	AccountName    string
	MailboxName    string
	LocalMessageID string
}

type Result struct {
	Backend Backend
}

type Service struct {
	resolver      MessageResolver
	mailArchiver  MailArchiver
	store         AuthorizationStore
	clientFactory TokenClientFactory
}

func NewService(
	resolver MessageResolver,
	mailArchiver MailArchiver,
	store AuthorizationStore,
	clientFactory TokenClientFactory,
) *Service {
	return &Service{
		resolver:      resolver,
		mailArchiver:  mailArchiver,
		store:         store,
		clientFactory: clientFactory,
	}
}

func (service *Service) Archive(
	ctx context.Context,
	request Request,
) (Result, error) {
	if strings.TrimSpace(request.LocalMessageID) == "" {
		return Result{}, errors.New("local Mail.app message ID is required")
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	identity, err := service.resolver.ResolveMessageIdentity(
		request.AccountName,
		request.MailboxName,
		request.LocalMessageID,
	)
	if err != nil {
		return Result{}, fmt.Errorf("resolve Mail.app message identity: %w", err)
	}
	if identity == nil ||
		strings.TrimSpace(identity.AccountID) == "" ||
		strings.TrimSpace(identity.RFCMessageID) == "" {
		return Result{}, errors.New("Mail.app returned an incomplete message identity")
	}

	record, err := service.store.Get(ctx, identity.AccountID)
	if errors.Is(err, gmailauth.ErrNotFound) {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		if err := service.mailArchiver.ArchiveMessage(
			request.AccountName,
			request.MailboxName,
			request.LocalMessageID,
		); err != nil {
			return Result{}, fmt.Errorf("archive through Mail.app: %w", err)
		}
		return Result{Backend: BackendMailApp}, nil
	}
	if err != nil {
		return Result{}, fmt.Errorf("load Gmail authorization: %w", err)
	}

	if strings.TrimSpace(record.Token.RefreshToken) == "" {
		return Result{}, gmailauth.NewReauthorizationError(
			identity.AccountName,
			errors.New("refresh token is missing"),
		)
	}
	if err := record.Validate(); err != nil {
		return Result{}, err
	}
	if record.MailAccountID != identity.AccountID {
		return Result{}, fmt.Errorf(
			"%w: stored Mail.app account ID does not match resolved identity",
			gmailauth.ErrInvalidRecord,
		)
	}

	gmailClient, err := service.clientFactory.NewClient(ctx, record)
	if err != nil {
		return Result{}, linkedAuthorizationError(
			identity.AccountName,
			"create Gmail client",
			err,
		)
	}
	gmailMessageID, err := gmailClient.FindInboxMessageByRFCMessageID(
		ctx,
		identity.RFCMessageID,
	)
	if err != nil {
		return Result{}, linkedAuthorizationError(
			identity.AccountName,
			"find Gmail inbox message",
			err,
		)
	}
	if err := gmailClient.RemoveInboxLabel(ctx, gmailMessageID); err != nil {
		return Result{}, linkedAuthorizationError(
			identity.AccountName,
			"remove Gmail INBOX label",
			err,
		)
	}
	return Result{Backend: BackendGmailAPI}, nil
}

func linkedAuthorizationError(accountName, operation string, err error) error {
	if errors.Is(err, gmailauth.ErrReauthorizationRequired) {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if errors.Is(err, gmailapi.ErrNotAuthorized) {
		return fmt.Errorf(
			"%s: %w",
			operation,
			gmailauth.NewReauthorizationError(accountName, err),
		)
	}
	return fmt.Errorf("%s: %w", operation, err)
}
