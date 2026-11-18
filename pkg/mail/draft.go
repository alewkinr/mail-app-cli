package mail

import (
	"encoding/json"
	"errors"
	"fmt"
	netmail "net/mail"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"time"
)

// DraftOptions describes the content of a standalone unsent draft.
type DraftOptions struct {
	Subject     string
	Body        string
	To          []string
	Cc          []string
	Bcc         []string
	Attachments []string
}

type draftMailboxInfo struct {
	AccountID string
	Mailbox   string
	Sender    string
}

type savedDraft struct {
	Message
	ToRecipients  []string
	CcRecipients  []string
	BccRecipients []string
}

type draftCandidates struct {
	Drafts  []savedDraft
	Pending bool
}

// CreateDraft saves a standalone draft and returns its persisted metadata.
func (c *Client) CreateDraft(account string, options DraftOptions) (*Message, error) {
	if strings.TrimSpace(account) == "" {
		return nil, fmt.Errorf("account is required")
	}
	for _, recipients := range [][]string{options.To, options.Cc, options.Bcc} {
		for _, recipient := range recipients {
			if mailboxAddress(recipient) == "" {
				return nil, fmt.Errorf("invalid draft recipient %q", recipient)
			}
		}
	}
	attachmentPaths := make([]string, 0, len(options.Attachments))
	for _, path := range options.Attachments {
		absolutePath, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("resolve draft attachment %q: %w", path, err)
		}
		info, err := os.Stat(absolutePath)
		if err != nil {
			return nil, fmt.Errorf("read draft attachment %q: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("draft attachment %q must be a regular file", path)
		}
		attachmentPaths = append(attachmentPaths, absolutePath)
	}
	options.Attachments = attachmentPaths

	scope, err := c.getDraftMailboxAndSender(account)
	if err != nil {
		return nil, err
	}

	args := []string{scope.Sender, options.Subject, options.Body}
	for _, values := range [][]string{options.To, options.Cc, options.Bcc, options.Attachments} {
		args = append(args, strconv.Itoa(len(values)))
		args = append(args, values...)
	}
	output, err := c.runAppleScript(createDraftAppleScript, args...)
	if err != nil {
		return nil, fmt.Errorf("draft save for account %q is uncertain: %w", account, err)
	}
	if output != "saved and closed" {
		return nil, fmt.Errorf("draft save for account %q is uncertain (%s); check draft list before retrying", account, output)
	}
	return findSavedDraft(account, scope, options, func() (draftCandidates, error) {
		var candidates draftCandidates
		output, err := c.runJXA(readDraftsJXAScript, "draft-get-candidates", account, scope.AccountID, scope.Mailbox, options.Subject)
		if err != nil {
			return candidates, err
		}
		err = json.Unmarshal([]byte(output), &candidates)
		return candidates, err
	}, 3, 350*time.Millisecond)
}

func (c *Client) getDraftMailboxAndSender(account string) (draftMailboxInfo, error) {
	var scope draftMailboxInfo
	output, err := c.runJXA(readDraftsJXAScript, "draft-mailbox-info", account)
	if err != nil {
		return scope, fmt.Errorf("read Drafts for account %q: %w", account, err)
	}
	if err := json.Unmarshal([]byte(output), &scope); err != nil {
		return scope, fmt.Errorf("decode Drafts scope for account %q: %w", account, err)
	}
	if scope.AccountID == "" || scope.Mailbox == "" {
		return scope, fmt.Errorf("incomplete Mail.app draft scope for account %q", account)
	}
	if mailboxAddress(scope.Sender) == "" {
		return scope, fmt.Errorf("account %q has no valid Mail.app sender", account)
	}
	return scope, nil
}

// findSavedDraft polls until Mail exposes one fully verified matching draft.
func findSavedDraft(account string, scope draftMailboxInfo, options DraftOptions, getCandidates func() (draftCandidates, error), attempts int, interval time.Duration) (*Message, error) {
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			time.Sleep(interval)
		}
		candidates, err := getCandidates()
		if err != nil {
			continue
		}
		var match *Message
		for _, draft := range candidates.Drafts {
			if draft.ID == "" || draft.Account != account || draft.Mailbox != scope.Mailbox {
				continue
			}
			if !matchesDraftEnvelope(draft, scope.Sender, options) {
				continue
			}
			if match != nil {
				return nil, fmt.Errorf("draft save for account %q is uncertain: multiple matching drafts; check draft list before retrying", account)
			}
			match = &draft.Message
		}
		if match != nil && !candidates.Pending {
			return match, nil
		}
	}
	return nil, fmt.Errorf("draft save for account %q is uncertain: no unique saved draft confirmed; check draft list before retrying", account)
}

func matchesDraftEnvelope(draft savedDraft, sender string, options DraftOptions) bool {
	if draft.Subject != options.Subject || mailboxAddress(draft.Sender) != mailboxAddress(sender) {
		return false
	}
	for _, group := range []struct{ got, want []string }{
		{draft.ToRecipients, options.To},
		{draft.CcRecipients, options.Cc},
		{draft.BccRecipients, options.Bcc},
	} {
		normalize := func(addresses []string) []string {
			keys := make([]string, 0, len(addresses))
			for _, address := range addresses {
				keys = append(keys, mailboxAddress(address))
			}
			slices.Sort(keys)
			return keys
		}
		if !slices.Equal(normalize(group.got), normalize(group.want)) {
			return false
		}
	}
	return true
}

// mailboxAddress strips a display name from a formatted Mail.app address.
func mailboxAddress(value string) string {
	if address, err := netmail.ParseAddress(value); err == nil {
		return strings.ToLower(address.Address)
	}
	return ""
}

// DraftMove identifies the saved draft and its source and destination mailboxes.
type DraftMove struct {
	Account      string `json:"account"`
	DraftID      string `json:"draftId"`
	DraftMailbox string `json:"draftMailbox"`
	TrashMailbox string `json:"trashMailbox"`
}

// DeleteDraft moves a saved draft to Trash within the selected account.
func (c *Client) DeleteDraft(account, localID string) (DraftMove, error) {
	if strings.TrimSpace(account) == "" {
		return DraftMove{}, fmt.Errorf("account is required")
	}
	if strings.TrimSpace(localID) == "" {
		return DraftMove{}, fmt.Errorf("draft ID must be nonempty")
	}

	output, err := c.runJXA(deleteDraftJXAScript, account, localID)
	return draftMoveResult(account, localID, output, err)
}

func draftMoveResult(account, localID, output string, err error) (DraftMove, error) {
	uncertain := func(stage string) error {
		return fmt.Errorf("trash move for draft %q in account %q is uncertain (%s); check draft list and Trash before retrying", localID, account, stage)
	}
	if err != nil {
		if errors.Is(err, ErrAccountNotFound) || errors.Is(err, ErrAccountAmbiguous) ||
			errors.Is(err, ErrMailboxNotFound) || errors.Is(err, ErrMailboxAmbiguous) {
			return DraftMove{}, err
		}
		return DraftMove{}, fmt.Errorf("%w: %w", uncertain("Mail.app script failed"), err)
	}
	var result struct {
		Status      string
		Source      string
		Destination string
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		return DraftMove{}, uncertain("invalid Mail.app response")
	}
	move := DraftMove{
		Account: account, DraftID: localID,
		DraftMailbox: result.Source, TrashMailbox: result.Destination,
	}
	switch result.Status {
	case "not_found":
		return DraftMove{}, fmt.Errorf("draft %q in account %q: %w", localID, account, ErrMessageNotFound)
	case "ambiguous":
		return DraftMove{}, fmt.Errorf("draft %q in account %q is ambiguous", localID, account)
	case "moved":
		if move.DraftMailbox != "" && move.TrashMailbox != "" {
			return move, nil
		}
		return move, uncertain("incomplete Mail.app response")
	case "uncertain":
		return move, uncertain("could not verify the move")
	default:
		return move, uncertain("unexpected Mail.app response")
	}
}
