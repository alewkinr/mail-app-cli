package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/intelligrit/mail-app-cli/pkg/mail"
	"github.com/spf13/cobra"
)

type draftClient interface {
	ListDrafts(account string) ([]mail.Message, error)
	ShowDraft(account, localID string) (*mail.MessageDetail, error)
	CreateDraft(account string, options mail.DraftOptions) (*mail.Message, error)
	DeleteDraft(account, localID string) (mail.DraftMove, error)
}

func newDraftCmd(client draftClient) *cobra.Command {
	var account string
	drafts := &cobra.Command{
		Use:           "draft",
		Short:         "Manage Mail.app drafts",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	drafts.PersistentFlags().StringVarP(&account, "account", "a", "", "Mail.app account name")
	drafts.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List saved drafts as JSON",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			messages, err := client.ListDrafts(account)
			if err != nil {
				return err
			}
			encoder := json.NewEncoder(cmd.OutOrStdout())
			encoder.SetIndent("", "  ")
			return encoder.Encode(messages)
		},
	})
	drafts.AddCommand(&cobra.Command{
		Use:   "show <local-id>",
		Short: "Show a saved draft as JSON",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(account) == "" {
				return fmt.Errorf("--account is required")
			}
			if strings.TrimSpace(args[0]) == "" {
				return fmt.Errorf("draft ID must be nonempty")
			}
			draft, err := client.ShowDraft(account, args[0])
			if err != nil {
				return err
			}
			encoder := json.NewEncoder(cmd.OutOrStdout())
			encoder.SetIndent("", "  ")
			return encoder.Encode(draft)
		},
	})
	drafts.AddCommand(newDraftCreateCmd(client, &account, invalidateMailboxCache))
	drafts.AddCommand(newDraftDeleteCmd(client, &account))
	return drafts
}

func newDraftCreateCmd(client draftClient, account *string, invalidate func(account, mailbox string)) *cobra.Command {
	var options mail.DraftOptions
	command := &cobra.Command{
		Use:   "create",
		Short: "Save an unsent draft and print its metadata as JSON",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(*account) == "" {
				return fmt.Errorf("--account is required")
			}
			draft, err := client.CreateDraft(*account, options)
			if err != nil {
				invalidate(*account, "")
				return err
			}
			invalidate(*account, draft.Mailbox)
			encoder := json.NewEncoder(cmd.OutOrStdout())
			encoder.SetIndent("", "  ")
			return encoder.Encode(draft)
		},
	}
	flags := command.Flags()
	flags.StringVarP(&options.Subject, "subject", "s", "", "Draft subject")
	flags.StringVar(&options.Body, "body", "", "Draft body")
	flags.StringArrayVarP(&options.To, "to", "t", nil, "To recipient (repeatable)")
	flags.StringArrayVarP(&options.Cc, "cc", "c", nil, "Cc recipient (repeatable)")
	flags.StringArrayVarP(&options.Bcc, "bcc", "b", nil, "Bcc recipient (repeatable)")
	flags.StringArrayVar(&options.Attachments, "attach", nil, "File to attach (repeatable)")
	return command
}

func newDraftDeleteCmd(client draftClient, account *string) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <local-id>",
		Short: "Move a saved draft to Trash",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(*account) == "" {
				return fmt.Errorf("--account is required")
			}
			if strings.TrimSpace(args[0]) == "" {
				return fmt.Errorf("draft ID must be nonempty")
			}
			move, err := client.DeleteDraft(*account, args[0])
			for _, mailbox := range []string{move.DraftMailbox, move.TrashMailbox} {
				if mailbox != "" {
					invalidateMailboxCache(*account, mailbox)
				}
			}
			if err != nil {
				return err
			}
			encoder := json.NewEncoder(cmd.OutOrStdout())
			encoder.SetIndent("", "  ")
			return encoder.Encode(move)
		},
	}
}
