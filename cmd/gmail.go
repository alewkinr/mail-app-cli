package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/intelligrit/mail-app-cli/internal/gmailapi"
	"github.com/intelligrit/mail-app-cli/internal/gmailauth"
	"github.com/intelligrit/mail-app-cli/pkg/mail"
	"github.com/spf13/cobra"
	"golang.org/x/oauth2"
)

type MailAccountClient interface {
	GetAccountByName(string) (*mail.Account, error)
}

type AuthorizationStore interface {
	Get(context.Context, string) (gmailauth.AuthorizationRecord, error)
	Put(context.Context, gmailauth.AuthorizationRecord) error
	Delete(context.Context, string) error
	List(context.Context) ([]gmailauth.AuthorizationRecord, error)
}

type Authorizer interface {
	Authorize(
		context.Context,
		gmailauth.LoginRequest,
	) (gmailauth.AuthorizationRecord, error)
}

type ProfileClientFactory interface {
	NewProfileClient(context.Context, oauth2.TokenSource) gmailauth.ProfileClient
}

type gmailCommandDeps struct {
	MailAccounts    MailAccountClient
	Store           AuthorizationStore
	LoadOAuthConfig func() (gmailauth.OAuthConfig, error)
	Authorizer      Authorizer
	Profiles        ProfileClientFactory
	Revoker         gmailauth.Revoker
	Stdout          io.Writer
}

type gmailProfileClientFactory struct{}

func (gmailProfileClientFactory) NewProfileClient(
	ctx context.Context,
	source oauth2.TokenSource,
) gmailauth.ProfileClient {
	return gmailapi.NewClient(oauth2.NewClient(ctx, source))
}

type gmailAuthStatus struct {
	MailAccountID             string   `json:"mailAccountID"`
	MailAccountName           string   `json:"mailAccountName"`
	MailAccountEmailAddresses []string `json:"mailAccountEmailAddresses"`
	GmailEmail                string   `json:"gmailEmail"`
	Authorized                bool     `json:"authorized"`
	TokenExpiry               *string  `json:"tokenExpiry"`
}

func newProductionGmailCommandDeps() gmailCommandDeps {
	store := gmailauth.NewStore()
	profiles := gmailProfileClientFactory{}
	return gmailCommandDeps{
		MailAccounts:    mail.NewClient(),
		Store:           store,
		LoadOAuthConfig: gmailauth.AppOAuthConfig,
		Authorizer: gmailauth.NewAuthorizer(
			profiles,
			gmailauth.NewListenerFactory(),
		),
		Profiles: profiles,
		Revoker:  gmailauth.NewGoogleRevoker(http.DefaultClient),
		Stdout:   os.Stdout,
	}
}

func newGmailCmd(deps gmailCommandDeps) *cobra.Command {
	command := &cobra.Command{
		Use:   "gmail",
		Short: "Manage Gmail API authorization for Mail.app accounts",
	}
	command.AddCommand(newGmailAuthCmd(deps))
	return command
}

func newGmailAuthCmd(deps gmailCommandDeps) *cobra.Command {
	command := &cobra.Command{
		Use:   "auth",
		Short: "Manage Gmail OAuth authorization",
	}
	command.AddCommand(newGmailAuthLoginCmd(deps))
	command.AddCommand(newGmailAuthStatusCmd(deps))
	command.AddCommand(newGmailAuthLogoutCmd(deps))
	return command
}

func newGmailAuthLoginCmd(deps gmailCommandDeps) *cobra.Command {
	var accountName string
	command := &cobra.Command{
		Use:   "login",
		Short: "Authorize Gmail access for one Mail.app account",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			account, err := deps.MailAccounts.GetAccountByName(accountName)
			if err != nil {
				return fmt.Errorf("resolve Mail.app account: %w", err)
			}
			oauthConfig, err := deps.LoadOAuthConfig()
			if err != nil {
				return fmt.Errorf("load Gmail OAuth configuration: %w", err)
			}
			record, err := deps.Authorizer.Authorize(command.Context(), gmailauth.LoginRequest{
				MailAccount: *account,
				OAuth:       oauthConfig,
				Output:      writerOrDiscard(deps.Stdout),
			})
			if err != nil {
				return fmt.Errorf("authorize Gmail account: %w", err)
			}
			if err := record.Validate(); err != nil {
				return err
			}
			if err := deps.Store.Put(command.Context(), record); err != nil {
				return fmt.Errorf("store Gmail authorization: %w", err)
			}
			_, err = fmt.Fprintf(
				writerOrDiscard(deps.Stdout),
				"%s\t%s\n",
				record.MailAccountName,
				record.GmailEmail,
			)
			return err
		},
	}
	command.Flags().StringVarP(
		&accountName,
		"account",
		"a",
		"",
		"Mail.app account name",
	)
	mustMarkFlagRequired(command, "account")
	return command
}

func newGmailAuthStatusCmd(deps gmailCommandDeps) *cobra.Command {
	var accountName string
	command := &cobra.Command{
		Use:   "status",
		Short: "Validate Gmail authorization status",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			ctx := command.Context()
			if accountName != "" {
				account, err := deps.MailAccounts.GetAccountByName(accountName)
				if err != nil {
					return fmt.Errorf("resolve Mail.app account: %w", err)
				}
				record, err := deps.Store.Get(ctx, account.ID)
				if errors.Is(err, gmailauth.ErrNotFound) {
					return writeStatusJSON(writerOrDiscard(deps.Stdout), gmailAuthStatus{
						MailAccountID:             account.ID,
						MailAccountName:           account.Name,
						MailAccountEmailAddresses: append([]string(nil), account.EmailAddresses...),
						GmailEmail:                "",
						Authorized:                false,
						TokenExpiry:               nil,
					})
				}
				if err != nil {
					return fmt.Errorf("load Gmail authorization: %w", err)
				}
				status, err := buildLinkedStatus(ctx, deps, record, account.ID)
				if err != nil {
					return err
				}
				return writeStatusJSON(writerOrDiscard(deps.Stdout), status)
			}

			records, err := deps.Store.List(ctx)
			if err != nil {
				return fmt.Errorf("list Gmail authorizations: %w", err)
			}
			sort.Slice(records, func(left, right int) bool {
				leftName := strings.ToLower(records[left].MailAccountName)
				rightName := strings.ToLower(records[right].MailAccountName)
				if leftName != rightName {
					return leftName < rightName
				}
				return records[left].MailAccountID < records[right].MailAccountID
			})

			statuses := make([]gmailAuthStatus, 0, len(records))
			for _, record := range records {
				status, err := buildLinkedStatus(ctx, deps, record, record.MailAccountID)
				if err != nil {
					return err
				}
				statuses = append(statuses, status)
			}
			return writeStatusJSON(writerOrDiscard(deps.Stdout), statuses)
		},
	}
	command.Flags().StringVarP(
		&accountName,
		"account",
		"a",
		"",
		"Mail.app account name",
	)
	return command
}

func newGmailAuthLogoutCmd(deps gmailCommandDeps) *cobra.Command {
	var accountName string
	var revoke bool
	command := &cobra.Command{
		Use:   "logout",
		Short: "Remove Gmail authorization for one Mail.app account",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			ctx := command.Context()
			account, err := deps.MailAccounts.GetAccountByName(accountName)
			if err != nil {
				return fmt.Errorf("resolve Mail.app account: %w", err)
			}
			if revoke {
				record, err := deps.Store.Get(ctx, account.ID)
				if err != nil {
					return fmt.Errorf("load Gmail authorization: %w", err)
				}
				if record.MailAccountID != account.ID {
					return fmt.Errorf(
						"%w: Mail.app account ID does not match stored authorization",
						gmailauth.ErrInvalidRecord,
					)
				}
				if err := record.Validate(); err != nil {
					return err
				}
				if err := deps.Revoker.Revoke(ctx, record.Token.RefreshToken); err != nil {
					return fmt.Errorf("revoke Gmail authorization: %w", err)
				}
			}
			if err := deps.Store.Delete(ctx, account.ID); err != nil {
				return fmt.Errorf("delete Gmail authorization: %w", err)
			}
			_, err = fmt.Fprintf(
				writerOrDiscard(deps.Stdout),
				"Gmail authorization removed for %s\n",
				account.Name,
			)
			return err
		},
	}
	command.Flags().StringVarP(
		&accountName,
		"account",
		"a",
		"",
		"Mail.app account name",
	)
	command.Flags().BoolVar(&revoke, "revoke", false, "Revoke the Google OAuth grant first")
	mustMarkFlagRequired(command, "account")
	return command
}

func buildLinkedStatus(
	ctx context.Context,
	deps gmailCommandDeps,
	record gmailauth.AuthorizationRecord,
	expectedMailAccountID string,
) (gmailAuthStatus, error) {
	if strings.TrimSpace(record.Token.RefreshToken) == "" {
		return gmailAuthStatus{}, gmailauth.NewReauthorizationError(
			record.MailAccountName,
			errors.New("refresh token is missing"),
		)
	}
	if err := record.Validate(); err != nil {
		return gmailAuthStatus{}, err
	}
	if record.MailAccountID != expectedMailAccountID {
		return gmailAuthStatus{}, fmt.Errorf(
			"%w: Mail.app account ID does not match stored authorization",
			gmailauth.ErrInvalidRecord,
		)
	}

	oauthConfig, err := deps.LoadOAuthConfig()
	if err != nil {
		return gmailAuthStatus{}, fmt.Errorf("load Gmail OAuth configuration: %w", err)
	}
	source, err := gmailauth.NewPersistingTokenSource(ctx, record, deps.Store, oauthConfig)
	if err != nil {
		return gmailAuthStatus{}, err
	}
	profileClient := deps.Profiles.NewProfileClient(ctx, source)
	if profileClient == nil {
		return gmailAuthStatus{}, errors.New("create Gmail profile client failed")
	}
	token, err := source.Token()
	if err != nil {
		return gmailAuthStatus{}, err
	}
	profile, err := profileClient.GetProfile(ctx)
	if err != nil {
		if errors.Is(err, gmailauth.ErrReauthorizationRequired) {
			return gmailAuthStatus{}, err
		}
		if errors.Is(err, gmailapi.ErrNotAuthorized) {
			return gmailAuthStatus{}, gmailauth.NewReauthorizationError(
				record.MailAccountName,
				err,
			)
		}
		return gmailAuthStatus{}, gmailapi.NewProfileValidationError(err)
	}
	if !strings.EqualFold(
		strings.TrimSpace(profile.EmailAddress),
		strings.TrimSpace(record.GmailEmail),
	) {
		return gmailAuthStatus{}, fmt.Errorf(
			"%w: Gmail profile does not match stored authorization",
			gmailauth.ErrInvalidRecord,
		)
	}

	var tokenExpiry *string
	if !token.Expiry.IsZero() {
		formatted := token.Expiry.UTC().Format(time.RFC3339)
		tokenExpiry = &formatted
	}
	return gmailAuthStatus{
		MailAccountID:             record.MailAccountID,
		MailAccountName:           record.MailAccountName,
		MailAccountEmailAddresses: append([]string(nil), record.MailAccountEmailAddresses...),
		GmailEmail:                record.GmailEmail,
		Authorized:                true,
		TokenExpiry:               tokenExpiry,
	}, nil
}

func writeStatusJSON(writer io.Writer, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return errors.New("encode Gmail authorization status failed")
	}
	if _, err := fmt.Fprintln(writer, string(data)); err != nil {
		return fmt.Errorf("write Gmail authorization status: %w", err)
	}
	return nil
}

func writerOrDiscard(writer io.Writer) io.Writer {
	if writer == nil {
		return io.Discard
	}
	return writer
}

func mustMarkFlagRequired(command *cobra.Command, name string) {
	if err := command.MarkFlagRequired(name); err != nil {
		panic(err)
	}
}
