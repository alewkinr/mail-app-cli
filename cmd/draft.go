package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/intelligrit/mail-app-cli/pkg/mail"
	"github.com/spf13/cobra"
)

type draftReader interface {
	ListDrafts(account string) ([]mail.Message, error)
	ShowDraft(account, localID string) (*mail.MessageDetail, error)
}

func newDraftCmd(reader draftReader) *cobra.Command {
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
			messages, err := reader.ListDrafts(account)
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
			draft, err := reader.ShowDraft(account, args[0])
			if err != nil {
				return err
			}
			encoder := json.NewEncoder(cmd.OutOrStdout())
			encoder.SetIndent("", "  ")
			return encoder.Encode(draft)
		},
	})
	return drafts
}
