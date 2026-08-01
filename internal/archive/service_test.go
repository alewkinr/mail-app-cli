package archive

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/intelligrit/mail-app-cli/internal/gmailapi"
	"github.com/intelligrit/mail-app-cli/internal/gmailauth"
	"github.com/intelligrit/mail-app-cli/pkg/mail"
)

type archiveTestResolver struct {
	identity *mail.MessageIdentity
	err      error
	calls    int
	account  string
	mailbox  string
	localID  string
}

func (resolver *archiveTestResolver) ResolveMessageIdentity(
	account,
	mailbox,
	localID string,
) (*mail.MessageIdentity, error) {
	resolver.calls++
	resolver.account = account
	resolver.mailbox = mailbox
	resolver.localID = localID
	return resolver.identity, resolver.err
}

type archiveTestMailArchiver struct {
	err     error
	calls   int
	account string
	mailbox string
	localID string
}

func (archiver *archiveTestMailArchiver) ArchiveMessage(
	account,
	mailbox,
	localID string,
) error {
	archiver.calls++
	archiver.account = account
	archiver.mailbox = mailbox
	archiver.localID = localID
	return archiver.err
}

type archiveTestStore struct {
	record gmailauth.AuthorizationRecord
	err    error
	calls  int
	key    string
	onGet  func()
}

func (store *archiveTestStore) Get(
	_ context.Context,
	key string,
) (gmailauth.AuthorizationRecord, error) {
	store.calls++
	store.key = key
	if store.onGet != nil {
		store.onGet()
	}
	return store.record, store.err
}

type archiveTestClientFactory struct {
	client GmailClient
	err    error
	calls  int
	record gmailauth.AuthorizationRecord
}

func (factory *archiveTestClientFactory) NewClient(
	_ context.Context,
	record gmailauth.AuthorizationRecord,
) (GmailClient, error) {
	factory.calls++
	factory.record = record
	return factory.client, factory.err
}

type archiveTestGmailClient struct {
	findID         string
	findErr        error
	removeErr      error
	findCalls      int
	removeCalls    int
	rfcMessageID   string
	gmailMessageID string
}

func (client *archiveTestGmailClient) FindInboxMessageByRFCMessageID(
	_ context.Context,
	rfcMessageID string,
) (string, error) {
	client.findCalls++
	client.rfcMessageID = rfcMessageID
	return client.findID, client.findErr
}

func (client *archiveTestGmailClient) RemoveInboxLabel(
	_ context.Context,
	gmailMessageID string,
) error {
	client.removeCalls++
	client.gmailMessageID = gmailMessageID
	return client.removeErr
}

func TestArchiveBackendConstants(t *testing.T) {
	if BackendGmailAPI != "gmail-api" || BackendMailApp != "mail-app" {
		t.Fatalf("backend constants = %q, %q", BackendGmailAPI, BackendMailApp)
	}
}

func TestArchiveLinkedAccountUsesGmailAPI(t *testing.T) {
	resolver, mailArchiver, store, factory, gmailClient := validArchiveTestDeps()
	service := NewService(resolver, mailArchiver, store, factory)

	result, err := service.Archive(context.Background(), archiveTestRequest())
	if err != nil {
		t.Fatalf("Archive() error = %v", err)
	}
	if result.Backend != BackendGmailAPI {
		t.Fatalf("Backend = %q, want gmail-api", result.Backend)
	}
	if resolver.localID != "local-42" {
		t.Fatalf("resolver local ID = %q, want local-42", resolver.localID)
	}
	if store.key != "mail-account-id" {
		t.Fatalf("Store key = %q, want stable Mail.app account ID", store.key)
	}
	if gmailClient.rfcMessageID != "rfc-message@example.com" {
		t.Fatalf("Gmail lookup ID = %q, want resolved RFC Message-ID", gmailClient.rfcMessageID)
	}
	if gmailClient.gmailMessageID != "gmail-message-id" {
		t.Fatalf("Gmail mutation ID = %q, want lookup result", gmailClient.gmailMessageID)
	}
	if mailArchiver.calls != 0 {
		t.Fatal("Mail.app fallback ran for linked account")
	}
}

func TestArchiveUnlinkedAccountUsesOriginalLocalID(t *testing.T) {
	resolver, mailArchiver, store, factory, gmailClient := validArchiveTestDeps()
	store.err = gmailauth.ErrNotFound
	service := NewService(resolver, mailArchiver, store, factory)

	result, err := service.Archive(context.Background(), archiveTestRequest())
	if err != nil {
		t.Fatalf("Archive() error = %v", err)
	}
	if result.Backend != BackendMailApp {
		t.Fatalf("Backend = %q, want mail-app", result.Backend)
	}
	if mailArchiver.calls != 1 || mailArchiver.localID != "local-42" {
		t.Fatalf(
			"Mail.app calls = %d with ID %q, want original local ID",
			mailArchiver.calls,
			mailArchiver.localID,
		)
	}
	if factory.calls != 0 || gmailClient.findCalls != 0 || gmailClient.removeCalls != 0 {
		t.Fatal("Gmail client ran for unlinked account")
	}
}

func TestArchiveRejectsBlankLocalIDBeforeCalls(t *testing.T) {
	resolver, mailArchiver, store, factory, _ := validArchiveTestDeps()
	_, err := NewService(resolver, mailArchiver, store, factory).Archive(
		context.Background(),
		Request{AccountName: "Personal", MailboxName: "INBOX", LocalMessageID: " \t "},
	)
	if err == nil {
		t.Fatal("Archive() error = nil")
	}
	if resolver.calls != 0 || store.calls != 0 || mailArchiver.calls != 0 || factory.calls != 0 {
		t.Fatal("blank local ID triggered downstream calls")
	}
}

func TestArchiveStoreFailureDoesNotFallback(t *testing.T) {
	resolver, mailArchiver, store, factory, gmailClient := validArchiveTestDeps()
	store.err = errors.New("Keychain unavailable")
	_, err := NewService(resolver, mailArchiver, store, factory).Archive(
		context.Background(),
		archiveTestRequest(),
	)
	if err == nil {
		t.Fatal("Archive() error = nil")
	}
	assertNoArchiveMutation(t, mailArchiver, factory, gmailClient)
}

func TestArchiveRejectsInvalidLinkedRecordsBeforeClient(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*gmailauth.AuthorizationRecord)
		want   error
	}{
		{
			name: "missing refresh",
			mutate: func(record *gmailauth.AuthorizationRecord) {
				record.Token.RefreshToken = ""
			},
			want: gmailauth.ErrReauthorizationRequired,
		},
		{
			name: "malformed",
			mutate: func(record *gmailauth.AuthorizationRecord) {
				record.GmailEmail = ""
			},
			want: gmailauth.ErrInvalidRecord,
		},
		{
			name: "account ID mismatch",
			mutate: func(record *gmailauth.AuthorizationRecord) {
				record.MailAccountID = "different-account"
			},
			want: gmailauth.ErrInvalidRecord,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolver, mailArchiver, store, factory, gmailClient := validArchiveTestDeps()
			test.mutate(&store.record)
			_, err := NewService(resolver, mailArchiver, store, factory).Archive(
				context.Background(),
				archiveTestRequest(),
			)
			if !errors.Is(err, test.want) {
				t.Fatalf("Archive() error = %v, want %v", err, test.want)
			}
			if errors.Is(err, gmailauth.ErrReauthorizationRequired) {
				assertOneLoginCommand(t, err)
			}
			assertNoArchiveMutation(t, mailArchiver, factory, gmailClient)
		})
	}
}

func TestArchiveGmailFailuresNeverFallback(t *testing.T) {
	tests := []struct {
		name       string
		factoryErr error
		findErr    error
		removeErr  error
		want       error
	}{
		{
			name:    "zero Gmail matches",
			findErr: gmailapi.ErrMessageNotFound,
			want:    gmailapi.ErrMessageNotFound,
		},
		{
			name:    "multiple Gmail matches",
			findErr: gmailapi.ErrAmbiguousMessage,
			want:    gmailapi.ErrAmbiguousMessage,
		},
		{
			name:       "existing reauthorization error",
			factoryErr: gmailauth.NewReauthorizationError("Personal", errors.New("invalid grant")),
			want:       gmailauth.ErrReauthorizationRequired,
		},
		{
			name:    "lookup unauthorized",
			findErr: gmailapi.ErrNotAuthorized,
			want:    gmailauth.ErrReauthorizationRequired,
		},
		{
			name:      "mutation unauthorized",
			removeErr: gmailapi.ErrNotAuthorized,
			want:      gmailauth.ErrReauthorizationRequired,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolver, mailArchiver, store, factory, gmailClient := validArchiveTestDeps()
			factory.err = test.factoryErr
			gmailClient.findErr = test.findErr
			gmailClient.removeErr = test.removeErr
			_, err := NewService(resolver, mailArchiver, store, factory).Archive(
				context.Background(),
				archiveTestRequest(),
			)
			if !errors.Is(err, test.want) {
				t.Fatalf("Archive() error = %v, want %v", err, test.want)
			}
			if errors.Is(err, gmailauth.ErrReauthorizationRequired) {
				assertOneLoginCommand(t, err)
			}
			if mailArchiver.calls != 0 {
				t.Fatal("linked Gmail failure invoked Mail.app fallback")
			}
			if test.findErr != nil && gmailClient.removeCalls != 0 {
				t.Fatal("lookup failure invoked Gmail mutation")
			}
		})
	}
}

func TestArchiveChecksCanceledContextBeforeMailAppCalls(t *testing.T) {
	t.Run("resolver", func(t *testing.T) {
		resolver, mailArchiver, store, factory, _ := validArchiveTestDeps()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := NewService(resolver, mailArchiver, store, factory).Archive(
			ctx,
			archiveTestRequest(),
		)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Archive() error = %v, want context.Canceled", err)
		}
		if resolver.calls != 0 {
			t.Fatal("resolver ran with already-canceled context")
		}
	})

	t.Run("fallback archiver", func(t *testing.T) {
		resolver, mailArchiver, store, factory, _ := validArchiveTestDeps()
		ctx, cancel := context.WithCancel(context.Background())
		store.err = gmailauth.ErrNotFound
		store.onGet = cancel
		_, err := NewService(resolver, mailArchiver, store, factory).Archive(
			ctx,
			archiveTestRequest(),
		)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Archive() error = %v, want context.Canceled", err)
		}
		if mailArchiver.calls != 0 {
			t.Fatal("Mail.app archiver ran after context cancellation")
		}
	})
}

func validArchiveTestDeps() (
	*archiveTestResolver,
	*archiveTestMailArchiver,
	*archiveTestStore,
	*archiveTestClientFactory,
	*archiveTestGmailClient,
) {
	identity := &mail.MessageIdentity{
		LocalID:               "local-42",
		RFCMessageID:          "rfc-message@example.com",
		AccountID:             "mail-account-id",
		AccountName:           "Personal",
		AccountEmailAddresses: []string{"primary@example.com"},
		MailboxName:           "INBOX",
	}
	record := gmailauth.AuthorizationRecord{
		SchemaVersion:             1,
		MailAccountID:             identity.AccountID,
		MailAccountName:           identity.AccountName,
		MailAccountEmailAddresses: append([]string(nil), identity.AccountEmailAddresses...),
		GmailEmail:                "primary@example.com",
		OAuthClientID:             "public-client-id.apps.googleusercontent.com",
		Token: gmailauth.TokenRecord{
			AccessToken:  "access-token",
			RefreshToken: "refresh-token",
			TokenType:    "Bearer",
			Expiry:       time.Now().Add(time.Hour),
		},
	}
	gmailClient := &archiveTestGmailClient{findID: "gmail-message-id"}
	factory := &archiveTestClientFactory{client: gmailClient}
	return &archiveTestResolver{identity: identity},
		&archiveTestMailArchiver{},
		&archiveTestStore{record: record},
		factory,
		gmailClient
}

func archiveTestRequest() Request {
	return Request{
		AccountName:    "Personal",
		MailboxName:    "INBOX",
		LocalMessageID: "local-42",
	}
}

func assertNoArchiveMutation(
	t *testing.T,
	mailArchiver *archiveTestMailArchiver,
	factory *archiveTestClientFactory,
	gmailClient *archiveTestGmailClient,
) {
	t.Helper()
	if mailArchiver.calls != 0 ||
		factory.calls != 0 ||
		gmailClient.findCalls != 0 ||
		gmailClient.removeCalls != 0 {
		t.Fatal("failure triggered Mail.app or Gmail downstream mutation path")
	}
}

func assertOneLoginCommand(t *testing.T, err error) {
	t.Helper()
	if strings.Count(
		err.Error(),
		`mail-app-cli gmail auth login --account "Personal"`,
	) != 1 {
		t.Fatalf("reauthorization error = %v, want exactly one login command", err)
	}
}
