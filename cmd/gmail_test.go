package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/intelligrit/mail-app-cli/internal/gmailapi"
	"github.com/intelligrit/mail-app-cli/internal/gmailauth"
	"github.com/intelligrit/mail-app-cli/pkg/mail"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/oauth2"
)

type gmailCommandMailAccounts struct {
	account *mail.Account
	err     error
	calls   int
	order   *[]string
}

func (accounts *gmailCommandMailAccounts) GetAccountByName(string) (*mail.Account, error) {
	accounts.calls++
	appendOrder(accounts.order, "mail")
	if accounts.err != nil {
		return nil, accounts.err
	}
	return accounts.account, nil
}

type gmailCommandStore struct {
	getRecord   gmailauth.AuthorizationRecord
	getErr      error
	listRecords []gmailauth.AuthorizationRecord
	listErr     error
	putErr      error
	deleteErr   error
	puts        []gmailauth.AuthorizationRecord
	deletes     []string
	gets        []string
	order       *[]string
}

func (store *gmailCommandStore) Get(
	_ context.Context,
	accountID string,
) (gmailauth.AuthorizationRecord, error) {
	store.gets = append(store.gets, accountID)
	appendOrder(store.order, "get")
	return store.getRecord, store.getErr
}

func (store *gmailCommandStore) Put(
	_ context.Context,
	record gmailauth.AuthorizationRecord,
) error {
	appendOrder(store.order, "put")
	if store.putErr != nil {
		return store.putErr
	}
	store.puts = append(store.puts, record)
	return nil
}

func (store *gmailCommandStore) Delete(_ context.Context, accountID string) error {
	appendOrder(store.order, "delete")
	if store.deleteErr != nil {
		return store.deleteErr
	}
	store.deletes = append(store.deletes, accountID)
	return nil
}

func (store *gmailCommandStore) List(
	context.Context,
) ([]gmailauth.AuthorizationRecord, error) {
	appendOrder(store.order, "list")
	return append([]gmailauth.AuthorizationRecord(nil), store.listRecords...), store.listErr
}

type gmailCommandAuthorizer struct {
	record gmailauth.AuthorizationRecord
	err    error
	calls  int
	order  *[]string
	output io.Writer
}

func (authorizer *gmailCommandAuthorizer) Authorize(
	_ context.Context,
	request gmailauth.LoginRequest,
) (gmailauth.AuthorizationRecord, error) {
	authorizer.calls++
	authorizer.output = request.Output
	appendOrder(authorizer.order, "authorize")
	return authorizer.record, authorizer.err
}

type gmailCommandProfileClient struct {
	profile gmailapi.Profile
	err     error
}

func (client gmailCommandProfileClient) GetProfile(
	context.Context,
) (gmailapi.Profile, error) {
	return client.profile, client.err
}

type gmailCommandProfiles struct {
	client gmailauth.ProfileClient
	calls  int
}

func (profiles *gmailCommandProfiles) NewProfileClient(
	context.Context,
	oauth2.TokenSource,
) gmailauth.ProfileClient {
	profiles.calls++
	return profiles.client
}

type gmailCommandRevoker struct {
	err    error
	calls  int
	tokens []string
	order  *[]string
}

func (revoker *gmailCommandRevoker) Revoke(
	_ context.Context,
	token string,
) error {
	revoker.calls++
	revoker.tokens = append(revoker.tokens, token)
	appendOrder(revoker.order, "revoke")
	return revoker.err
}

func TestGmailCommandShapeAndFlags(t *testing.T) {
	command := newGmailCmd(validGmailCommandDeps())
	login, _, err := command.Find([]string{"auth", "login"})
	if err != nil {
		t.Fatalf("find login command: %v", err)
	}
	status, _, err := command.Find([]string{"auth", "status"})
	if err != nil {
		t.Fatalf("find status command: %v", err)
	}
	logout, _, err := command.Find([]string{"auth", "logout"})
	if err != nil {
		t.Fatalf("find logout command: %v", err)
	}

	for name, subcommand := range map[string]*cobra.Command{
		"login":  login,
		"status": status,
		"logout": logout,
	} {
		flag := subcommand.Flags().Lookup("account")
		if flag == nil || flag.Shorthand != "a" {
			t.Fatalf("%s --account shorthand is not -a", name)
		}
	}
	if login.Flag("account").Annotations[cobra.BashCompOneRequiredFlag] == nil {
		t.Fatal("login --account is not required")
	}
	if logout.Flag("account").Annotations[cobra.BashCompOneRequiredFlag] == nil {
		t.Fatal("logout --account is not required")
	}
	if status.Flag("account").Annotations[cobra.BashCompOneRequiredFlag] != nil {
		t.Fatal("status --account is unexpectedly required")
	}
	if logout.Flags().Lookup("revoke") == nil {
		t.Fatal("logout --revoke flag is missing")
	}

	assertExactCommandFlags(t, login, "account")
	assertExactCommandFlags(t, status, "account")
	assertExactCommandFlags(t, logout, "account", "revoke")
}

func TestGmailAuthLoginWritesOnceAfterValidation(t *testing.T) {
	order := []string{}
	deps := validGmailCommandDeps()
	deps.MailAccounts.(*gmailCommandMailAccounts).order = &order
	deps.Store.(*gmailCommandStore).order = &order
	deps.Authorizer.(*gmailCommandAuthorizer).order = &order
	deps.LoadOAuthConfig = func() (gmailauth.OAuthConfig, error) {
		appendOrder(&order, "config")
		return testOAuthConfig(), nil
	}
	var stdout bytes.Buffer
	deps.Stdout = &stdout

	command := newGmailCmd(deps)
	if err := executeCobra(command, "auth", "login", "-a", "Personal"); err != nil {
		t.Fatalf("login error = %v", err)
	}
	store := deps.Store.(*gmailCommandStore)
	if len(store.puts) != 1 {
		t.Fatalf("Store.Put calls = %d, want 1", len(store.puts))
	}
	if strings.Join(order, ",") != "mail,config,authorize,put" {
		t.Fatalf("operation order = %v", order)
	}
	if deps.Authorizer.(*gmailCommandAuthorizer).output != deps.Stdout {
		t.Fatal("authorization URL output was not wired to command stdout")
	}
	if stdout.String() != "Personal\tprimary@example.com\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestGmailAuthLoginDoesNotWriteAfterFailures(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*gmailCommandDeps)
	}{
		{name: "Mail account resolution", mutate: func(deps *gmailCommandDeps) {
			deps.MailAccounts.(*gmailCommandMailAccounts).err = errors.New("mail error")
		}},
		{name: "missing app configuration", mutate: func(deps *gmailCommandDeps) {
			deps.LoadOAuthConfig = func() (gmailauth.OAuthConfig, error) {
				return gmailauth.OAuthConfig{}, gmailauth.ErrOAuthClientNotConfigured
			}
		}},
		{name: "authorization", mutate: func(deps *gmailCommandDeps) {
			deps.Authorizer.(*gmailCommandAuthorizer).err = errors.New("authorization error")
		}},
		{name: "record validation", mutate: func(deps *gmailCommandDeps) {
			deps.Authorizer.(*gmailCommandAuthorizer).record.Token.RefreshToken = ""
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			deps := validGmailCommandDeps()
			test.mutate(&deps)
			var stdout bytes.Buffer
			deps.Stdout = &stdout
			err := executeCobra(newGmailCmd(deps), "auth", "login", "--account", "Personal")
			if err == nil {
				t.Fatal("login error = nil")
			}
			if len(deps.Store.(*gmailCommandStore).puts) != 0 {
				t.Fatal("Store.Put was called after login failure")
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", stdout.String())
			}
		})
	}
}

func TestGmailAuthLoginRequiresAccount(t *testing.T) {
	deps := validGmailCommandDeps()
	err := executeCobra(newGmailCmd(deps), "auth", "login")
	if err == nil {
		t.Fatal("login without --account error = nil")
	}
	if deps.MailAccounts.(*gmailCommandMailAccounts).calls != 0 {
		t.Fatal("Mail account lookup ran without required flag")
	}
}

func TestGmailAuthStatusSingleUnlinkedAccount(t *testing.T) {
	deps := validGmailCommandDeps()
	deps.Store.(*gmailCommandStore).getErr = gmailauth.ErrNotFound
	var stdout bytes.Buffer
	deps.Stdout = &stdout
	if err := executeCobra(
		newGmailCmd(deps),
		"auth",
		"status",
		"--account",
		"Personal",
	); err != nil {
		t.Fatalf("status error = %v", err)
	}

	var status map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if len(status) != 6 ||
		status["authorized"] != false ||
		status["gmailEmail"] != "" ||
		status["tokenExpiry"] != nil {
		t.Fatalf("unlinked status = %#v", status)
	}
}

func TestGmailAuthStatusListsLinkedAccountsSorted(t *testing.T) {
	deps := validGmailCommandDeps()
	alpha := testAuthorizationRecord()
	alpha.MailAccountID = "id-2"
	alpha.MailAccountName = "alpha"
	beta := testAuthorizationRecord()
	beta.MailAccountID = "id-1"
	beta.MailAccountName = "Beta"
	deps.Store.(*gmailCommandStore).listRecords = []gmailauth.AuthorizationRecord{beta, alpha}
	deps.Profiles = &gmailCommandProfiles{
		client: gmailCommandProfileClient{
			profile: gmailapi.Profile{EmailAddress: "primary@example.com"},
		},
	}
	var stdout bytes.Buffer
	deps.Stdout = &stdout

	if err := executeCobra(newGmailCmd(deps), "auth", "status"); err != nil {
		t.Fatalf("status error = %v", err)
	}
	var statuses []gmailAuthStatus
	if err := json.Unmarshal(stdout.Bytes(), &statuses); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if len(statuses) != 2 ||
		statuses[0].MailAccountName != "alpha" ||
		statuses[1].MailAccountName != "Beta" {
		t.Fatalf("sorted statuses = %#v", statuses)
	}
	if !statuses[0].Authorized || statuses[0].TokenExpiry == nil {
		t.Fatalf("linked status missing authorization fields: %#v", statuses[0])
	}
}

func TestGmailAuthStatusEmptyListIsJSONArray(t *testing.T) {
	deps := validGmailCommandDeps()
	var stdout bytes.Buffer
	deps.Stdout = &stdout
	if err := executeCobra(newGmailCmd(deps), "auth", "status"); err != nil {
		t.Fatalf("status error = %v", err)
	}
	if strings.TrimSpace(stdout.String()) != "[]" {
		t.Fatalf("stdout = %q, want []", stdout.String())
	}
}

func TestGmailAuthStatusFailuresProduceNoPartialJSON(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(gmailCommandDeps) context.Context
		want    error
	}{
		{
			name: "list inaccessible",
			prepare: func(deps gmailCommandDeps) context.Context {
				deps.Store.(*gmailCommandStore).listErr = errors.New("Keychain unavailable")
				return context.Background()
			},
		},
		{
			name: "malformed second record",
			prepare: func(deps gmailCommandDeps) context.Context {
				valid := testAuthorizationRecord()
				invalid := testAuthorizationRecord()
				invalid.MailAccountID = ""
				deps.Store.(*gmailCommandStore).listRecords = []gmailauth.AuthorizationRecord{
					valid,
					invalid,
				}
				return context.Background()
			},
			want: gmailauth.ErrInvalidRecord,
		},
		{
			name: "missing refresh token",
			prepare: func(deps gmailCommandDeps) context.Context {
				record := testAuthorizationRecord()
				record.Token.RefreshToken = ""
				deps.Store.(*gmailCommandStore).listRecords = []gmailauth.AuthorizationRecord{record}
				return context.Background()
			},
			want: gmailauth.ErrReauthorizationRequired,
		},
		{
			name: "refresh invalid grant",
			prepare: func(deps gmailCommandDeps) context.Context {
				record := testAuthorizationRecord()
				record.Token.Expiry = time.Now().Add(-time.Hour)
				deps.Store.(*gmailCommandStore).listRecords = []gmailauth.AuthorizationRecord{record}
				return invalidGrantContext()
			},
			want: gmailauth.ErrReauthorizationRequired,
		},
		{
			name: "profile unauthorized",
			prepare: func(deps gmailCommandDeps) context.Context {
				deps.Store.(*gmailCommandStore).listRecords = []gmailauth.AuthorizationRecord{
					testAuthorizationRecord(),
				}
				deps.Profiles.(*gmailCommandProfiles).client = gmailCommandProfileClient{
					err: gmailapi.ErrNotAuthorized,
				}
				return context.Background()
			},
			want: gmailauth.ErrReauthorizationRequired,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			deps := validGmailCommandDeps()
			var stdout bytes.Buffer
			deps.Stdout = &stdout
			ctx := test.prepare(deps)
			command := newGmailCmd(deps)
			command.SetContext(ctx)
			err := executeCobra(command, "auth", "status")
			if err == nil {
				t.Fatal("status error = nil")
			}
			if test.want != nil && !errors.Is(err, test.want) {
				t.Fatalf("status error = %v, want %v", err, test.want)
			}
			if errors.Is(err, gmailauth.ErrReauthorizationRequired) &&
				strings.Count(
					err.Error(),
					`mail-app-cli gmail auth login --account "Personal"`,
				) != 1 {
				t.Fatalf("reauthorization guidance = %v", err)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want no partial JSON", stdout.String())
			}
		})
	}
}

func TestGmailAuthStatusExplainsForbiddenProfile(t *testing.T) {
	deps := validGmailCommandDeps()
	deps.Store.(*gmailCommandStore).listRecords = []gmailauth.AuthorizationRecord{
		testAuthorizationRecord(),
	}
	deps.Profiles.(*gmailCommandProfiles).client = gmailCommandProfileClient{
		err: &gmailapi.HTTPError{
			StatusCode: http.StatusForbidden,
			Operation:  "get profile",
		},
	}
	var stdout bytes.Buffer
	deps.Stdout = &stdout

	err := executeCobra(newGmailCmd(deps), "auth", "status")
	if err == nil ||
		!strings.Contains(err.Error(), "HTTP status 403") ||
		!strings.Contains(err.Error(), "enable the Gmail API") {
		t.Fatalf("status error = %v, want actionable HTTP 403", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want no partial JSON", stdout.String())
	}
}

func TestGmailAuthLogoutRevocationOrder(t *testing.T) {
	order := []string{}
	deps := validGmailCommandDeps()
	deps.Store.(*gmailCommandStore).order = &order
	deps.Revoker.(*gmailCommandRevoker).order = &order
	if err := executeCobra(
		newGmailCmd(deps),
		"auth",
		"logout",
		"-a",
		"Personal",
		"--revoke",
	); err != nil {
		t.Fatalf("logout error = %v", err)
	}
	if strings.Join(order, ",") != "get,revoke,delete" {
		t.Fatalf("operation order = %v", order)
	}
}

func TestGmailAuthLogoutRevocationFailurePreservesRecord(t *testing.T) {
	deps := validGmailCommandDeps()
	deps.Revoker.(*gmailCommandRevoker).err = errors.New("revocation failed")
	err := executeCobra(
		newGmailCmd(deps),
		"auth",
		"logout",
		"--account",
		"Personal",
		"--revoke",
	)
	if err == nil {
		t.Fatal("logout error = nil")
	}
	if len(deps.Store.(*gmailCommandStore).deletes) != 0 {
		t.Fatal("Keychain record was deleted after revocation failure")
	}
}

func TestGmailAuthLogoutWithoutRevokeDeletesOnly(t *testing.T) {
	deps := validGmailCommandDeps()
	if err := executeCobra(
		newGmailCmd(deps),
		"auth",
		"logout",
		"--account",
		"Personal",
	); err != nil {
		t.Fatalf("logout error = %v", err)
	}
	store := deps.Store.(*gmailCommandStore)
	if len(store.gets) != 0 || len(store.deletes) != 1 {
		t.Fatalf("Get calls = %d, Delete calls = %d", len(store.gets), len(store.deletes))
	}
	if deps.Revoker.(*gmailCommandRevoker).calls != 0 {
		t.Fatal("Revoker was called without --revoke")
	}
}

func TestGmailAuthLogoutMissingRecord(t *testing.T) {
	deps := validGmailCommandDeps()
	deps.Store.(*gmailCommandStore).getErr = gmailauth.ErrNotFound
	err := executeCobra(
		newGmailCmd(deps),
		"auth",
		"logout",
		"--account",
		"Personal",
		"--revoke",
	)
	if !errors.Is(err, gmailauth.ErrNotFound) {
		t.Fatalf("logout error = %v, want ErrNotFound", err)
	}
	if deps.Revoker.(*gmailCommandRevoker).calls != 0 ||
		len(deps.Store.(*gmailCommandStore).deletes) != 0 {
		t.Fatal("missing record triggered revocation or deletion")
	}
}

func validGmailCommandDeps() gmailCommandDeps {
	account := &mail.Account{
		ID:             "mail-account-id",
		Name:           "Personal",
		EmailAddresses: []string{"primary@example.com", "alias@example.com"},
	}
	record := testAuthorizationRecord()
	return gmailCommandDeps{
		MailAccounts: &gmailCommandMailAccounts{account: account},
		Store: &gmailCommandStore{
			getRecord: record,
		},
		LoadOAuthConfig: func() (gmailauth.OAuthConfig, error) {
			return testOAuthConfig(), nil
		},
		Authorizer: &gmailCommandAuthorizer{record: record},
		Profiles: &gmailCommandProfiles{
			client: gmailCommandProfileClient{
				profile: gmailapi.Profile{EmailAddress: "primary@example.com"},
			},
		},
		Revoker: &gmailCommandRevoker{},
		Stdout:  io.Discard,
	}
}

func testAuthorizationRecord() gmailauth.AuthorizationRecord {
	return gmailauth.AuthorizationRecord{
		SchemaVersion:             1,
		MailAccountID:             "mail-account-id",
		MailAccountName:           "Personal",
		MailAccountEmailAddresses: []string{"primary@example.com", "alias@example.com"},
		GmailEmail:                "primary@example.com",
		OAuthClientID:             "public-client-id.apps.googleusercontent.com",
		Token: gmailauth.TokenRecord{
			AccessToken:  "access-token",
			RefreshToken: "refresh-token",
			TokenType:    "Bearer",
			Expiry:       time.Now().Add(time.Hour).UTC().Truncate(time.Second),
		},
	}
}

func testOAuthConfig() gmailauth.OAuthConfig {
	return gmailauth.OAuthConfig{
		ClientID:     "public-client-id.apps.googleusercontent.com",
		ClientSecret: "desktop-client-secret",
		AuthURL:      "https://accounts.example.test/authorize",
		TokenURL:     "https://tokens.example.test/token",
	}
}

func executeCobra(command *cobra.Command, args ...string) error {
	command.SilenceUsage = true
	command.SilenceErrors = true
	command.SetArgs(args)
	return command.Execute()
}

func walkCommands(command *cobra.Command, visit func(*cobra.Command)) {
	visit(command)
	for _, child := range command.Commands() {
		walkCommands(child, visit)
	}
}

func assertExactCommandFlags(t *testing.T, command *cobra.Command, expected ...string) {
	t.Helper()
	want := make(map[string]bool, len(expected)+1)
	want["help"] = true
	for _, name := range expected {
		want[name] = true
	}
	command.Flags().VisitAll(func(flag *pflag.Flag) {
		if !want[flag.Name] {
			t.Fatalf("unexpected flag --%s on %s", flag.Name, command.CommandPath())
		}
		delete(want, flag.Name)
	})
	delete(want, "help")
	if len(want) != 0 {
		t.Fatalf("missing expected flags on %s: %#v", command.CommandPath(), want)
	}
}

func appendOrder(order *[]string, value string) {
	if order != nil {
		*order = append(*order, value)
	}
}

func invalidGrantContext() context.Context {
	client := &http.Client{
		Transport: gmailCommandRoundTripper(func(request *http.Request) (*http.Response, error) {
			body := io.NopCloser(strings.NewReader(`{"error":"invalid_grant"}`))
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Status:     "400 Bad Request",
				Header: http.Header{
					"Content-Type": []string{"application/json"},
				},
				Body:    body,
				Request: request,
			}, nil
		}),
	}
	return context.WithValue(context.Background(), oauth2.HTTPClient, client)
}

type gmailCommandRoundTripper func(*http.Request) (*http.Response, error)

func (function gmailCommandRoundTripper) RoundTrip(
	request *http.Request,
) (*http.Response, error) {
	return function(request)
}
