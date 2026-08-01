package mail

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// MessageIdentity connects Mail.app's local message identity to the RFC
// Message-ID required by the Gmail API.
type MessageIdentity struct {
	LocalID               string
	RFCMessageID          string
	AccountID             string
	AccountName           string
	AccountEmailAddresses []string
	MailboxName           string
}

type jxaRunFunc func(script string, args ...string) (string, error)

type messageIdentityResult struct {
	LocalID               string   `json:"localID"`
	RFCMessageID          string   `json:"rfcMessageID"`
	AccountID             string   `json:"accountID"`
	AccountName           string   `json:"accountName"`
	AccountEmailAddresses []string `json:"accountEmailAddresses"`
	MailboxName           string   `json:"mailboxName"`
	Error                 string   `json:"error"`
}

// ResolveMessageIdentity resolves a Mail.app-local message ID into the stable
// identity needed by provider-specific operations.
func (c *Client) ResolveMessageIdentity(accountName, mailboxName, localMessageID string) (*MessageIdentity, error) {
	return resolveMessageIdentity(c.runJXA, accountName, mailboxName, localMessageID)
}

func resolveMessageIdentity(run jxaRunFunc, accountName, mailboxName, localMessageID string) (*MessageIdentity, error) {
	output, err := run(
		resolveMessageIdentityJXAScript,
		accountName,
		mailboxName,
		localMessageID,
	)
	if err != nil {
		return nil, fmt.Errorf("resolve Mail.app message identity: %w", err)
	}

	var result messageIdentityResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		return nil, errors.New("invalid response from Mail.app identity resolver")
	}
	if result.Error != "" {
		return nil, resolverError(result.Error)
	}

	if strings.TrimSpace(result.LocalID) == "" ||
		strings.TrimSpace(result.RFCMessageID) == "" ||
		strings.TrimSpace(result.AccountID) == "" ||
		strings.TrimSpace(result.AccountName) == "" ||
		strings.TrimSpace(result.MailboxName) == "" {
		return nil, errors.New("Mail.app identity resolver returned incomplete identity")
	}

	return &MessageIdentity{
		LocalID:               result.LocalID,
		RFCMessageID:          result.RFCMessageID,
		AccountID:             result.AccountID,
		AccountName:           result.AccountName,
		AccountEmailAddresses: result.AccountEmailAddresses,
		MailboxName:           result.MailboxName,
	}, nil
}
