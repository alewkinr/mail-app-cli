package mail

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	netmail "net/mail"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
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

type draftSnapshot struct {
	AccountID string
	Mailbox   string
	Sender    string
	IDs       []string
}

type savedDraft struct {
	MessageDetail
	Attachments []string
}

type draftObservation struct {
	Drafts  []savedDraft
	Pending bool
}

// CreateDraft saves a standalone draft and returns its persisted metadata.
func (c *Client) CreateDraft(account string, options DraftOptions) (*Message, error) {
	if strings.TrimSpace(account) == "" {
		return nil, fmt.Errorf("account is required")
	}
	if strings.ContainsRune(options.Body, '\uFFFC') {
		return nil, fmt.Errorf("draft body cannot contain a Unicode object replacement character")
	}
	for _, recipients := range [][]string{options.To, options.Cc, options.Bcc} {
		for _, recipient := range recipients {
			if _, err := netmail.ParseAddress(recipient); err != nil {
				return nil, fmt.Errorf("invalid draft recipient %q", recipient)
			}
		}
	}
	attachmentPaths := make([]string, 0, len(options.Attachments))
	attachmentDigests := make(map[draftAttachment]int)
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
		digest, err := draftFileDigest(absolutePath)
		if err != nil {
			return nil, fmt.Errorf("hash draft attachment %q: %w", path, err)
		}
		attachmentDigests[draftAttachment{name: filepath.Base(path), digest: digest}]++
	}
	options.Attachments = attachmentPaths

	// Resolve the stable account ID before locking, then take a fresh snapshot
	// under the lock so another MailCLI process cannot race our baseline.
	snapshot, err := c.takeDraftSnapshot(account)
	if err != nil {
		return nil, err
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("locate cache for draft creation: %w", err)
	}
	lock, err := lockDraftAccount(filepath.Join(cacheDir, "mail-app-cli", "draft-locks"), snapshot.AccountID)
	if err != nil {
		return nil, fmt.Errorf("lock draft creation for account %q: %w", account, err)
	}
	defer func() { _ = lock.Close() }()

	baseline, err := c.takeDraftSnapshot(account)
	if err != nil {
		return nil, err
	}
	if baseline.AccountID != snapshot.AccountID {
		return nil, fmt.Errorf("draft account %q changed before composition", account)
	}

	args := []string{baseline.Sender, options.Subject, options.Body}
	for _, values := range [][]string{options.To, options.Cc, options.Bcc, options.Attachments} {
		args = append(args, strconv.Itoa(len(values)))
		args = append(args, values...)
	}
	output, err := c.runAppleScript(createDraftAppleScript, args...)
	if err != nil {
		return nil, uncertainDraftSave(account, baseline.Mailbox, "compose, save, or close failed")
	}
	if output != "saved and closed" {
		switch output {
		case "compose failed; composer discarded", "compose failed; composer cleanup unverified",
			"attach failed; composer discarded", "attach failed; composer cleanup unverified", "save failed", "close failed":
			return nil, uncertainDraftSave(account, baseline.Mailbox, output)
		default:
			return nil, uncertainDraftSave(account, baseline.Mailbox, "unexpected response from Mail.app")
		}
	}
	baselineIDs, err := json.Marshal(baseline.IDs)
	if err != nil {
		return nil, uncertainDraftSave(account, baseline.Mailbox, "encode saved-record baseline")
	}
	return reconcileDraft(account, baseline, options, func() (draftObservation, error) {
		var observation draftObservation
		output, err := c.runJXA(readMessagesJXAScript, "draft-observe", account, baseline.AccountID, baseline.Mailbox, string(baselineIDs))
		if err != nil {
			return observation, err
		}
		err = json.Unmarshal([]byte(output), &observation)
		return observation, err
	}, func(draft savedDraft) (bool, error) {
		return c.verifyDraftAttachments(baseline.AccountID, draft, attachmentDigests, len(options.Attachments))
	}, 10, 350*time.Millisecond)
}

func (c *Client) takeDraftSnapshot(account string) (draftSnapshot, error) {
	var snapshot draftSnapshot
	output, err := c.runJXA(readMessagesJXAScript, "draft-snapshot", account)
	if err != nil {
		return snapshot, fmt.Errorf("snapshot drafts for account %q: %w", account, err)
	}
	if err := json.Unmarshal([]byte(output), &snapshot); err != nil {
		return snapshot, fmt.Errorf("decode draft snapshot for account %q: %w", account, err)
	}
	if snapshot.AccountID == "" || snapshot.Mailbox == "" {
		return snapshot, fmt.Errorf("incomplete Mail.app draft scope for account %q", account)
	}
	if _, err := netmail.ParseAddress(snapshot.Sender); err != nil {
		return snapshot, fmt.Errorf("account %q has no valid Mail.app sender", account)
	}
	return snapshot, nil
}

func lockDraftAccount(dir, accountID string) (*os.File, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	name := fmt.Sprintf("%x.lock", sha256.Sum256([]byte(accountID)))
	file, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	// Leave the lock file in place. Closing the descriptor releases the
	// kernel lock, including when a CLI process exits unexpectedly.
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

// DraftSaveUncertainError reports a draft that may have been saved in Mail.app.
type DraftSaveUncertainError struct {
	Account string
	Mailbox string
	Stage   string
}

func (e *DraftSaveUncertainError) Error() string {
	return fmt.Sprintf("draft save for account %q is uncertain (%s); check draft list before retrying", e.Account, e.Stage)
}

func uncertainDraftSave(account, mailbox, stage string) error {
	return &DraftSaveUncertainError{Account: account, Mailbox: mailbox, Stage: stage}
}

func reconcileDraft(account string, baseline draftSnapshot, options DraftOptions, observe func() (draftObservation, error), verifyAttachments func(savedDraft) (bool, error), attempts int, interval time.Duration) (*Message, error) {
	var previousID string
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			time.Sleep(interval)
		}
		observation, err := observe()
		if err != nil {
			previousID = ""
			continue
		}
		var match *Message
		for _, draft := range observation.Drafts {
			if draft.ID == "" || draft.Account != account || draft.Mailbox != baseline.Mailbox || slices.Contains(baseline.IDs, draft.ID) {
				continue
			}
			if !matchesDraft(draft, baseline.Sender, options) {
				continue
			}
			if len(options.Attachments) != 0 {
				verified, err := verifyAttachments(draft)
				if err != nil {
					observation.Pending = true
					continue
				}
				if !verified {
					continue
				}
			}
			if match != nil {
				return nil, uncertainDraftSave(account, baseline.Mailbox, "multiple matching saved records")
			}
			match = &draft.Message
		}
		if match == nil || observation.Pending {
			previousID = ""
			continue
		}
		if match.ID == previousID {
			return match, nil
		}
		previousID = match.ID
	}
	return nil, uncertainDraftSave(account, baseline.Mailbox, "timed out identifying the saved record")
}

func matchesDraft(draft savedDraft, sender string, options DraftOptions) bool {
	if draft.Subject != options.Subject || addressKey(draft.Sender) != addressKey(sender) {
		return false
	}
	body := strings.ReplaceAll(strings.ReplaceAll(draft.Content, "\r\n", "\n"), "\r", "\n")
	expectedBody := strings.ReplaceAll(strings.ReplaceAll(options.Body, "\r\n", "\n"), "\r", "\n")
	// Mail may pad the body with whitespace and attachment placeholders on save.
	body = strings.ReplaceAll(body, "\uFFFC", "")
	if strings.TrimSpace(body) != strings.TrimSpace(expectedBody) {
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
				keys = append(keys, addressKey(address))
			}
			slices.Sort(keys)
			return keys
		}
		if !slices.Equal(normalize(group.got), normalize(group.want)) {
			return false
		}
	}
	wantNames := make([]string, 0, len(options.Attachments))
	for _, path := range options.Attachments {
		wantNames = append(wantNames, filepath.Base(path))
	}
	gotNames := slices.Clone(draft.Attachments)
	slices.Sort(wantNames)
	slices.Sort(gotNames)
	return slices.Equal(gotNames, wantNames)
}

func addressKey(value string) string {
	if address, err := netmail.ParseAddress(value); err == nil {
		return strings.ToLower(address.Address)
	}
	return strings.ToLower(strings.TrimSpace(value))
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
	return deleteDraft(c.runJXA, account, localID)
}

func deleteDraft(run jxaRunFunc, account, localID string) (DraftMove, error) {
	if strings.TrimSpace(account) == "" {
		return DraftMove{}, fmt.Errorf("account is required")
	}
	if strings.TrimSpace(localID) == "" {
		return DraftMove{}, fmt.Errorf("draft ID must be nonempty")
	}

	uncertain := func(stage string) error {
		return fmt.Errorf("trash move for draft %q in account %q is uncertain (%s); check draft list and Trash before retrying", localID, account, stage)
	}
	output, err := run(deleteDraftJXAScript, account, localID)
	if err != nil {
		for _, scopeError := range []error{ErrAccountNotFound, ErrAccountAmbiguous, ErrMailboxNotFound, ErrMailboxAmbiguous} {
			if errors.Is(err, scopeError) {
				return DraftMove{}, err
			}
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

type draftAttachment struct {
	name   string
	digest [sha256.Size]byte
}

func draftFileDigest(path string) ([sha256.Size]byte, error) {
	var digest [sha256.Size]byte
	file, err := os.Open(path)
	if err != nil {
		return digest, err
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return digest, err
	}
	copy(digest[:], hash.Sum(nil))
	return digest, nil
}

func (c *Client) verifyDraftAttachments(accountID string, draft savedDraft, expected map[draftAttachment]int, count int) (bool, error) {
	dir, err := os.MkdirTemp("", "mailcli-draft-verify-")
	if err != nil {
		return false, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	output, err := c.runJXA(readMessagesJXAScript, "draft-export-attachments", draft.Account, accountID, draft.Mailbox, draft.ID, dir, strconv.Itoa(count))
	if err != nil {
		return false, err
	}
	var names []string
	if err := json.Unmarshal([]byte(output), &names); err != nil {
		return false, err
	}
	return checkDraftAttachmentDigests(dir, names, expected, count)
}

func checkDraftAttachmentDigests(dir string, names []string, expected map[draftAttachment]int, count int) (bool, error) {
	if len(names) != count {
		return false, nil
	}
	observed := make(map[draftAttachment]int)
	for i, name := range names {
		digest, err := draftFileDigest(filepath.Join(dir, strconv.Itoa(i)))
		if err != nil {
			return false, err
		}
		attachment := draftAttachment{name: name, digest: digest}
		observed[attachment]++
		if observed[attachment] > expected[attachment] {
			return false, nil
		}
	}
	return true, nil
}
