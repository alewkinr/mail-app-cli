package mail

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// Client provides an interface to interact with Mail.app via AppleScript
type Client struct{}

// NewClient creates a new Mail.app client
func NewClient() *Client {
	return &Client{}
}

// runOSA executes an Open Scripting Architecture source string.
func (c *Client) runOSA(language, errorLabel, script string, args ...string) (string, error) {
	cmdArgs := []string{"-l", language, "-e", script}
	cmdArgs = append(cmdArgs, args...)

	cmd := exec.Command("osascript", cmdArgs...)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("%s error: %v - %s", errorLabel, err, stderr.String())
	}

	return strings.TrimSpace(out.String()), nil
}

// runAppleScript executes AppleScript, optionally passing arguments to run argv.
func (c *Client) runAppleScript(script string, args ...string) (string, error) {
	return c.runOSA("AppleScript", "applescript", script, args...)
}

// runJXA executes JavaScript for Automation (JXA), optionally passing
// arguments to its run(argv) function, and returns the output.
func (c *Client) runJXA(script string, args ...string) (string, error) {
	return c.runOSA("JavaScript", "jxa", script, args...)
}

// Account represents a Mail.app account
type Account struct {
	ID             string
	Name           string
	EmailAddress   string
	EmailAddresses []string
	AccountType    string
	UserName       string
	Enabled        bool
}

// Mailbox represents a Mail.app mailbox
type Mailbox struct {
	Name        string
	UnreadCount int
	TotalCount  int
	Account     string
}

// Message represents an email message
type Message struct {
	ID            string
	Subject       string
	Sender        string
	DateSent      string
	DateReceived  string
	Read          bool
	Flagged       bool
	Deleted       bool
	MessageSize   int
	Content       string
	Mailbox       string
	Account       string
	ToRecipients  []string
	CcRecipients  []string
	BccRecipients []string
}

// Attachment represents an email attachment
type Attachment struct {
	Name     string
	FileSize int
	MimeType string
}

// GetAccounts retrieves all Mail.app accounts
func (c *Client) GetAccounts() ([]Account, error) {
	output, err := c.runAppleScript(getAccountsAppleScript)
	if err != nil {
		return nil, err
	}

	// Parse AppleScript list output
	accounts, err := c.parseAccounts(output)
	return accounts, err
}

// GetMailboxes retrieves all mailboxes for a specific account
func (c *Client) GetMailboxes(accountName string) ([]Mailbox, error) {
	output, err := c.runAppleScript(getMailboxesAppleScript, accountName)
	if err != nil {
		return nil, err
	}

	mailboxes, err := c.parseMailboxes(output)
	return mailboxes, err
}

// GetAllMailboxes retrieves all mailboxes across all accounts
func (c *Client) GetAllMailboxes() ([]Mailbox, error) {
	output, err := c.runAppleScript(getAllMailboxesAppleScript)
	if err != nil {
		return nil, err
	}

	mailboxes, err := c.parseMailboxes(output)
	return mailboxes, err
}

// GetMessages retrieves messages from a mailbox
func (c *Client) GetMessages(accountName, mailboxName string, limit int) ([]Message, error) {
	output, err := c.runAppleScript(
		getMessagesAppleScript,
		accountName,
		mailboxName,
		strconv.Itoa(limit),
	)
	if err != nil {
		return nil, err
	}

	messages, err := c.parseMessages(output)
	return messages, err
}

// SearchMessages searches for messages matching a query
func (c *Client) SearchMessages(query string, limit int) ([]Message, error) {
	output, err := c.runAppleScript(
		searchMessagesAppleScript,
		query,
		strconv.Itoa(limit),
	)
	if err != nil {
		return nil, err
	}

	messages, err := c.parseMessages(output)
	return messages, err
}

// MarkMessageAsRead marks a message as read
func (c *Client) MarkMessageAsRead(accountName, mailboxName, messageID string, read bool) error {
	output, err := c.runJXA(
		markMessageReadJXAScript,
		accountName,
		mailboxName,
		messageID,
		strconv.FormatBool(read),
	)
	if err != nil {
		return err
	}
	if strings.Contains(output, "Error") {
		return fmt.Errorf("%s", output)
	}
	return nil
}

// FlagMessage sets or unsets the flagged status of a message
func (c *Client) FlagMessage(accountName, mailboxName, messageID string, flagged bool) error {
	output, err := c.runJXA(
		flagMessageJXAScript,
		accountName,
		mailboxName,
		messageID,
		strconv.FormatBool(flagged),
	)
	if err != nil {
		return err
	}
	if strings.Contains(output, "Error") {
		return fmt.Errorf("%s", output)
	}
	return nil
}

// DeleteMessage moves a message to trash
func (c *Client) DeleteMessage(accountName, mailboxName, messageID string) error {
	output, err := c.runJXA(
		deleteMessageJXAScript,
		accountName,
		mailboxName,
		messageID,
	)
	if err != nil {
		return err
	}
	if strings.Contains(output, "Error") {
		return fmt.Errorf("%s", output)
	}
	return nil
}

// SendMessage sends a new email message
func (c *Client) SendMessage(accountName, subject, body string, to, cc, bcc, attachments []string) error {
	args := []string{accountName, subject, body, strconv.Itoa(len(to))}
	args = append(args, to...)
	args = append(args, strconv.Itoa(len(cc)))
	args = append(args, cc...)
	args = append(args, strconv.Itoa(len(bcc)))
	args = append(args, bcc...)
	args = append(args, strconv.Itoa(len(attachments)))
	args = append(args, attachments...)

	_, err := c.runAppleScript(sendMessageAppleScript, args...)
	return err
}

// Helper function to parse accounts from AppleScript output
func (c *Client) parseAccounts(_ string) ([]Account, error) {
	// TODO: Implement proper parsing based on AppleScript record format
	return []Account{}, nil
}

// Helper function to parse mailboxes from AppleScript output
func (c *Client) parseMailboxes(_ string) ([]Mailbox, error) {
	// TODO: Implement proper parsing based on AppleScript record format
	return []Mailbox{}, nil
}

// Helper function to parse messages from AppleScript output
func (c *Client) parseMessages(_ string) ([]Message, error) {
	// TODO: Implement proper parsing based on AppleScript record format
	return []Message{}, nil
}

// GetUnreadCount gets the total unread message count
func (c *Client) GetUnreadCount() (int, error) {
	output, err := c.runAppleScript(getUnreadCountAppleScript)
	if err != nil {
		return 0, err
	}

	var count int
	fmt.Sscanf(output, "%d", &count)
	return count, nil
}

// GetAccountsJSON retrieves accounts as JSON using JXA
func (c *Client) GetAccountsJSON() ([]Account, error) {
	output, err := c.runJXA(getAccountsJXAScript)
	if err != nil {
		return nil, err
	}

	var accounts []Account
	if err := json.Unmarshal([]byte(output), &accounts); err != nil {
		return nil, fmt.Errorf("failed to parse accounts JSON: %w", err)
	}

	return accounts, nil
}

// SyncAccount forces Mail.app to check for new mail (syncs all accounts)
// Note: Mail.app's AppleScript doesn't support per-account sync, so this syncs all accounts
func (c *Client) SyncAccount(accountName string) error {
	// Verify account exists
	_, err := c.runAppleScript(syncAccountAppleScript, accountName)
	if err != nil {
		return err
	}

	// Check for new mail (syncs all accounts)
	return c.SyncAllAccounts()
}

// SyncAllAccounts forces Mail.app to check for new mail across all accounts
func (c *Client) SyncAllAccounts() error {
	_, err := c.runAppleScript(syncAllAccountsAppleScript)
	return err
}

// GetMailboxesJSON retrieves mailboxes as JSON using JXA
func (c *Client) GetMailboxesJSON(accountName string) ([]Mailbox, error) {
	// If specific account requested, use single JXA call
	if accountName != "" {
		return c.getMailboxesForSingleAccount(accountName)
	}

	// For all accounts, fetch in parallel for better performance
	accounts, err := c.GetAccountsJSON()
	if err != nil {
		return nil, fmt.Errorf("failed to get accounts: %w", err)
	}

	if len(accounts) == 0 {
		return []Mailbox{}, nil
	}

	// If only one account total, no need for parallelization
	if len(accounts) == 1 {
		return c.getMailboxesForSingleAccount(accounts[0].Name)
	}

	// Use channel to collect results from goroutines
	type result struct {
		mailboxes []Mailbox
		err       error
	}
	results := make(chan result, len(accounts))

	// Launch goroutine for each account
	for _, account := range accounts {
		go func(accName string) {
			mailboxes, err := c.getMailboxesForSingleAccount(accName)
			results <- result{mailboxes: mailboxes, err: err}
		}(account.Name)
	}

	// Collect results
	var allMailboxes []Mailbox
	var errors []error
	for i := 0; i < len(accounts); i++ {
		res := <-results
		if res.err != nil {
			errors = append(errors, res.err)
		} else {
			allMailboxes = append(allMailboxes, res.mailboxes...)
		}
	}

	// Return partial results even if some accounts failed
	if len(errors) > 0 && len(allMailboxes) == 0 {
		return nil, fmt.Errorf("failed to get mailboxes from all accounts: %v", errors)
	}

	return allMailboxes, nil
}

// getMailboxesForSingleAccount retrieves mailboxes for a specific account
func (c *Client) getMailboxesForSingleAccount(accountName string) ([]Mailbox, error) {
	output, err := c.runJXA(getMailboxesJXAScript, accountName)
	if err != nil {
		return nil, err
	}

	var mailboxes []Mailbox
	if err := json.Unmarshal([]byte(output), &mailboxes); err != nil {
		return nil, fmt.Errorf("failed to parse mailboxes JSON: %w", err)
	}

	return mailboxes, nil
}

// GetMessagesJSON retrieves messages from a mailbox using JXA
func (c *Client) GetMessagesJSON(accountName, mailboxName string, limit, offset int, unreadOnly, flaggedOnly, withContent bool, since string) ([]Message, error) {
	output, err := c.runJXA(
		getMessagesJXAScript,
		accountName,
		mailboxName,
		strconv.Itoa(limit),
		strconv.Itoa(offset),
		strconv.FormatBool(unreadOnly),
		strconv.FormatBool(flaggedOnly),
		strconv.FormatBool(withContent),
		since,
	)
	if err != nil {
		return nil, err
	}

	var messages []Message
	if err := json.Unmarshal([]byte(output), &messages); err != nil {
		return nil, fmt.Errorf("failed to parse messages JSON: %w", err)
	}

	return messages, nil
}

// GetMessageDetailsJSON retrieves full details of a specific message
func (c *Client) GetMessageDetailsJSON(accountName, mailboxName, messageID string) (*Message, error) {
	output, err := c.runJXA(
		getMessageDetailsJXAScript,
		accountName,
		mailboxName,
		messageID,
	)
	if err != nil {
		return nil, err
	}

	var message Message
	if err := json.Unmarshal([]byte(output), &message); err != nil {
		return nil, fmt.Errorf("failed to parse message JSON: %w", err)
	}

	return &message, nil
}

// ArchiveMessage moves a message to the account's Archive mailbox through
// Mail.app's object model. The script accepts Mail's local numeric ID and
// uses the stable Message-ID to verify source removal.
func (c *Client) ArchiveMessage(accountName, mailboxName, messageID string) error {
	output, err := c.runJXA(
		archiveJXAScript,
		accountName,
		mailboxName,
		messageID,
	)
	if err != nil {
		return err
	}
	if strings.HasPrefix(output, "Error:") {
		return fmt.Errorf("%s", output)
	}
	if output != "Success" {
		return fmt.Errorf("unexpected archive script output: %q", output)
	}
	return nil
}

// MoveMessage moves a message to a different mailbox
func (c *Client) MoveMessage(accountName, sourceMailbox, messageID, targetMailbox string) error {
	output, err := c.runJXA(
		moveMessageJXAScript,
		accountName,
		sourceMailbox,
		messageID,
		targetMailbox,
	)
	if err != nil {
		return err
	}
	if strings.Contains(output, "Error") {
		return fmt.Errorf("%s", output)
	}
	return nil
}

// GetAttachmentsJSON retrieves attachments from a message
func (c *Client) GetAttachmentsJSON(accountName, mailboxName, messageID string) ([]Attachment, error) {
	output, err := c.runJXA(
		getAttachmentsJXAScript,
		accountName,
		mailboxName,
		messageID,
	)
	if err != nil {
		return nil, err
	}

	var attachments []Attachment
	if err := json.Unmarshal([]byte(output), &attachments); err != nil {
		return nil, fmt.Errorf("failed to parse attachments JSON: %w", err)
	}

	return attachments, nil
}

// SaveAttachment saves an attachment to disk
func (c *Client) SaveAttachment(accountName, mailboxName, messageID, attachmentName, savePath string) error {
	output, err := c.runJXA(
		saveAttachmentJXAScript,
		accountName,
		mailboxName,
		messageID,
		attachmentName,
		savePath,
	)
	if err != nil {
		return err
	}
	if strings.Contains(output, "Error") {
		return fmt.Errorf("%s", output)
	}
	return nil
}

// SearchMessagesJSON searches for messages across mailboxes
// Note: By default only searches INBOX mailboxes for performance reasons
func (c *Client) SearchMessagesJSON(query string, accountName string, mailboxName string, limit int) ([]Message, error) {
	// Set a reasonable default limit if none specified
	if limit == 0 {
		limit = 50
	}

	// If specific mailbox requested, use single JXA call for simplicity
	if mailboxName != "" {
		return c.searchMessagesInSingleMailbox(query, accountName, mailboxName, limit)
	}

	// Get list of mailboxes to search
	mailboxes, err := c.GetMailboxesJSON(accountName)
	if err != nil {
		return nil, fmt.Errorf("failed to get mailboxes: %w", err)
	}

	// Filter to only INBOX mailboxes for performance (unless specific account given)
	var mailboxesToSearch []Mailbox
	for _, mbox := range mailboxes {
		if mbox.Name == "INBOX" || mbox.Name == "Inbox" {
			mailboxesToSearch = append(mailboxesToSearch, mbox)
		}
	}

	if len(mailboxesToSearch) == 0 {
		return []Message{}, nil
	}

	// If only one mailbox, no need for parallelization
	if len(mailboxesToSearch) == 1 {
		return c.searchMessagesInSingleMailbox(query, mailboxesToSearch[0].Account, mailboxesToSearch[0].Name, limit)
	}

	// Search mailboxes in parallel
	type result struct {
		messages []Message
		err      error
	}
	results := make(chan result, len(mailboxesToSearch))

	// Launch goroutine for each mailbox
	for _, mbox := range mailboxesToSearch {
		go func(accName, mboxName string) {
			messages, err := c.searchMessagesInSingleMailbox(query, accName, mboxName, limit)
			results <- result{messages: messages, err: err}
		}(mbox.Account, mbox.Name)
	}

	// Collect results
	var allMessages []Message
	var errors []error
	for i := 0; i < len(mailboxesToSearch); i++ {
		res := <-results
		if res.err != nil {
			errors = append(errors, res.err)
		} else {
			allMessages = append(allMessages, res.messages...)
		}
	}

	// Return partial results even if some mailboxes failed
	if len(errors) > 0 && len(allMessages) == 0 {
		return nil, fmt.Errorf("failed to search all mailboxes: %v", errors)
	}

	// Sort by date received (newest first) and apply limit
	sort.Slice(allMessages, func(i, j int) bool {
		return allMessages[i].DateReceived > allMessages[j].DateReceived
	})

	if len(allMessages) > limit {
		allMessages = allMessages[:limit]
	}

	return allMessages, nil
}

// searchMessagesInSingleMailbox searches for messages in a specific mailbox
func (c *Client) searchMessagesInSingleMailbox(query, accountName, mailboxName string, limit int) ([]Message, error) {
	output, err := c.runJXA(
		searchMessagesJXAScript,
		query,
		accountName,
		mailboxName,
		strconv.Itoa(limit),
	)
	if err != nil {
		return nil, err
	}

	var messages []Message
	if err := json.Unmarshal([]byte(output), &messages); err != nil {
		return nil, fmt.Errorf("failed to parse search results JSON: %w", err)
	}

	return messages, nil
}

// GetMessagesFromMultipleMailboxes loads messages from multiple mailboxes concurrently
func (c *Client) GetMessagesFromMultipleMailboxes(requests []struct {
	AccountName string
	MailboxName string
	Limit       int
	Offset      int
	UnreadOnly  bool
	FlaggedOnly bool
	WithContent bool
	Since       string
}) ([]Message, error) {
	if len(requests) == 0 {
		return []Message{}, nil
	}

	// If only one request, no need for parallelization
	if len(requests) == 1 {
		req := requests[0]
		return c.GetMessagesJSON(req.AccountName, req.MailboxName, req.Limit, req.Offset, req.UnreadOnly, req.FlaggedOnly, req.WithContent, req.Since)
	}

	// Load messages from multiple mailboxes in parallel
	type result struct {
		messages []Message
		err      error
	}
	results := make(chan result, len(requests))

	// Launch goroutine for each mailbox
	for _, req := range requests {
		go func(r struct {
			AccountName string
			MailboxName string
			Limit       int
			Offset      int
			UnreadOnly  bool
			FlaggedOnly bool
			WithContent bool
			Since       string
		}) {
			messages, err := c.GetMessagesJSON(r.AccountName, r.MailboxName, r.Limit, r.Offset, r.UnreadOnly, r.FlaggedOnly, r.WithContent, r.Since)
			results <- result{messages: messages, err: err}
		}(req)
	}

	// Collect results
	var allMessages []Message
	var errors []error
	for i := 0; i < len(requests); i++ {
		res := <-results
		if res.err != nil {
			errors = append(errors, res.err)
		} else {
			allMessages = append(allMessages, res.messages...)
		}
	}

	// Return partial results even if some mailboxes failed
	if len(errors) > 0 && len(allMessages) == 0 {
		return nil, fmt.Errorf("failed to get messages from all mailboxes: %v", errors)
	}

	return allMessages, nil
}

// GetMultipleMessageDetails loads full details for multiple messages concurrently
func (c *Client) GetMultipleMessageDetails(requests []struct {
	AccountName string
	MailboxName string
	MessageID   string
}) ([]*Message, error) {
	if len(requests) == 0 {
		return []*Message{}, nil
	}

	// If only one request, no need for parallelization
	if len(requests) == 1 {
		req := requests[0]
		msg, err := c.GetMessageDetailsJSON(req.AccountName, req.MailboxName, req.MessageID)
		if err != nil {
			return nil, err
		}
		return []*Message{msg}, nil
	}

	// Load message details in parallel
	type result struct {
		message *Message
		err     error
		index   int
	}
	results := make(chan result, len(requests))

	// Launch goroutine for each message
	for i, req := range requests {
		go func(idx int, r struct {
			AccountName string
			MailboxName string
			MessageID   string
		}) {
			message, err := c.GetMessageDetailsJSON(r.AccountName, r.MailboxName, r.MessageID)
			results <- result{message: message, err: err, index: idx}
		}(i, req)
	}

	// Collect results in original order
	messages := make([]*Message, len(requests))
	var errors []error
	successCount := 0

	for i := 0; i < len(requests); i++ {
		res := <-results
		if res.err != nil {
			errors = append(errors, res.err)
			messages[res.index] = nil
		} else {
			messages[res.index] = res.message
			successCount++
		}
	}

	// Return error if all requests failed
	if successCount == 0 {
		return nil, fmt.Errorf("failed to get all message details: %v", errors)
	}

	return messages, nil
}

// BulkMarkMessages marks multiple messages as read/unread concurrently
func (c *Client) BulkMarkMessages(requests []struct {
	AccountName string
	MailboxName string
	MessageID   string
	Read        bool
}) error {
	if len(requests) == 0 {
		return nil
	}

	// If only one request, no need for parallelization
	if len(requests) == 1 {
		req := requests[0]
		return c.MarkMessageAsRead(req.AccountName, req.MailboxName, req.MessageID, req.Read)
	}

	// Process marks in parallel
	errors := make(chan error, len(requests))

	// Launch goroutine for each mark operation
	for _, req := range requests {
		go func(r struct {
			AccountName string
			MailboxName string
			MessageID   string
			Read        bool
		}) {
			errors <- c.MarkMessageAsRead(r.AccountName, r.MailboxName, r.MessageID, r.Read)
		}(req)
	}

	// Collect results
	var errorList []error
	for i := 0; i < len(requests); i++ {
		if err := <-errors; err != nil {
			errorList = append(errorList, err)
		}
	}

	if len(errorList) > 0 {
		return fmt.Errorf("failed to mark some messages: %v", errorList)
	}

	return nil
}

// BulkFlagMessages flags/unflags multiple messages concurrently
func (c *Client) BulkFlagMessages(requests []struct {
	AccountName string
	MailboxName string
	MessageID   string
	Flagged     bool
}) error {
	if len(requests) == 0 {
		return nil
	}

	// If only one request, no need for parallelization
	if len(requests) == 1 {
		req := requests[0]
		return c.FlagMessage(req.AccountName, req.MailboxName, req.MessageID, req.Flagged)
	}

	// Process flags in parallel
	errors := make(chan error, len(requests))

	// Launch goroutine for each flag operation
	for _, req := range requests {
		go func(r struct {
			AccountName string
			MailboxName string
			MessageID   string
			Flagged     bool
		}) {
			errors <- c.FlagMessage(r.AccountName, r.MailboxName, r.MessageID, r.Flagged)
		}(req)
	}

	// Collect results
	var errorList []error
	for i := 0; i < len(requests); i++ {
		if err := <-errors; err != nil {
			errorList = append(errorList, err)
		}
	}

	if len(errorList) > 0 {
		return fmt.Errorf("failed to flag some messages: %v", errorList)
	}

	return nil
}

// BulkDeleteMessages deletes multiple messages concurrently
func (c *Client) BulkDeleteMessages(requests []struct {
	AccountName string
	MailboxName string
	MessageID   string
}) error {
	if len(requests) == 0 {
		return nil
	}

	// If only one request, no need for parallelization
	if len(requests) == 1 {
		req := requests[0]
		return c.DeleteMessage(req.AccountName, req.MailboxName, req.MessageID)
	}

	// Process deletes in parallel
	errors := make(chan error, len(requests))

	// Launch goroutine for each delete operation
	for _, req := range requests {
		go func(r struct {
			AccountName string
			MailboxName string
			MessageID   string
		}) {
			errors <- c.DeleteMessage(r.AccountName, r.MailboxName, r.MessageID)
		}(req)
	}

	// Collect results
	var errorList []error
	for i := 0; i < len(requests); i++ {
		if err := <-errors; err != nil {
			errorList = append(errorList, err)
		}
	}

	if len(errorList) > 0 {
		return fmt.Errorf("failed to delete some messages: %v", errorList)
	}

	return nil
}

// BulkArchiveMessages archives multiple messages concurrently
func (c *Client) BulkArchiveMessages(requests []struct {
	AccountName string
	MailboxName string
	MessageID   string
}) error {
	if len(requests) == 0 {
		return nil
	}

	// If only one request, no need for parallelization
	if len(requests) == 1 {
		req := requests[0]
		return c.ArchiveMessage(req.AccountName, req.MailboxName, req.MessageID)
	}

	// Process archives in parallel
	errors := make(chan error, len(requests))

	// Launch goroutine for each archive operation
	for _, req := range requests {
		go func(r struct {
			AccountName string
			MailboxName string
			MessageID   string
		}) {
			errors <- c.ArchiveMessage(r.AccountName, r.MailboxName, r.MessageID)
		}(req)
	}

	// Collect results
	var errorList []error
	for i := 0; i < len(requests); i++ {
		if err := <-errors; err != nil {
			errorList = append(errorList, err)
		}
	}

	if len(errorList) > 0 {
		return fmt.Errorf("failed to archive some messages: %v", errorList)
	}

	return nil
}

// BulkMoveMessages moves multiple messages concurrently
// GetUnifiedMessagesJSON retrieves messages from Mail.app's special unified
// mailboxes (inboxes, sentMailboxes, draftMailboxes, trashMailboxes,
// junkMailboxes) across all accounts in a single JXA call.
//
// mailboxType must be one of: "inbox", "unread", "sent", "drafts",
// "trash", "junk", "flagged".
//
// "unread" and "flagged" are treated as inbox views with the appropriate
// filter applied.
// GetUnifiedMessagesJSON retrieves messages from unified views across all accounts.
//
// mailboxType must be one of: "inbox", "unread", "flagged", "sent", "drafts",
// "trash", "junk".
//
// inbox/unread/flagged use the accounts-based path (GetMessagesFromMultipleMailboxes
// → GetMessagesJSON per account INBOX) because mailbox objects from
// mail.inboxes() don't support the same bulk property operations as those
// obtained via acc.mailboxes.byName(), causing unreliable filtering.
//
// sent/drafts/trash/junk use Mail.app's JXA special-mailbox accessors
// (mail.sentMailboxes() etc.) which don't require per-message filtering.
func (c *Client) GetUnifiedMessagesJSON(mailboxType string, limit, offset int, withContent bool) ([]Message, error) {
	switch mailboxType {
	case "inbox", "unread", "flagged":
		return c.getInboxBasedUnified(mailboxType, limit, offset, withContent)
	case "sent", "drafts", "trash", "junk":
		return c.getSpecialMailboxUnified(mailboxType, limit, offset, withContent)
	default:
		return nil, fmt.Errorf("unknown unified mailbox type: %s", mailboxType)
	}
}

// getInboxBasedUnified fetches messages from each account's INBOX using the
// proven GetMessagesJSON path, then merges, sorts, and slices globally.
func (c *Client) getInboxBasedUnified(mailboxType string, limit, offset int, withContent bool) ([]Message, error) {
	accounts, err := c.GetAccountsJSON()
	if err != nil {
		return nil, fmt.Errorf("failed to list accounts: %w", err)
	}

	// Over-fetch per account so the global sort+slice is accurate.
	perLimit := limit + offset
	if perLimit < 50 {
		perLimit = 50
	}

	type req = struct {
		AccountName string
		MailboxName string
		Limit       int
		Offset      int
		UnreadOnly  bool
		FlaggedOnly bool
		WithContent bool
		Since       string
	}

	var requests []req
	for _, acc := range accounts {
		if !acc.Enabled {
			continue
		}
		requests = append(requests, req{
			AccountName: acc.Name,
			MailboxName: "INBOX",
			Limit:       perLimit,
			Offset:      0,
			UnreadOnly:  mailboxType == "unread",
			FlaggedOnly: mailboxType == "flagged",
			WithContent: withContent,
		})
	}

	if len(requests) == 0 {
		return []Message{}, nil
	}

	messages, err := c.GetMessagesFromMultipleMailboxes(requests)
	if err != nil {
		return nil, err
	}

	return sortAndSlice(messages, offset, limit), nil
}

// getSpecialMailboxUnified fetches messages from Mail.app's built-in special
// mailbox collections (sentMailboxes, draftMailboxes, trashMailboxes,
// junkMailboxes) via a single JXA call.  No per-message filtering is applied
// since these views don't need unread/flagged filtering.
func (c *Client) getSpecialMailboxUnified(mailboxType string, limit, offset int, withContent bool) ([]Message, error) {
	perLimit := limit + offset
	if perLimit < 50 {
		perLimit = 50
	}

	output, err := c.runJXA(
		getSpecialMailboxJXAScript,
		mailboxType,
		strconv.Itoa(perLimit),
		strconv.FormatBool(withContent),
	)
	if err != nil {
		return nil, err
	}

	var messages []Message
	if err := json.Unmarshal([]byte(output), &messages); err != nil {
		return nil, fmt.Errorf("failed to parse %s messages JSON: %w", mailboxType, err)
	}

	return sortAndSlice(messages, offset, limit), nil
}

// sortAndSlice sorts messages by date descending then applies offset and limit.
func sortAndSlice(messages []Message, offset, limit int) []Message {
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].DateReceived > messages[j].DateReceived
	})
	if offset > 0 {
		if offset >= len(messages) {
			return []Message{}
		}
		messages = messages[offset:]
	}
	if limit > 0 && len(messages) > limit {
		messages = messages[:limit]
	}
	return messages
}

func (c *Client) BulkMoveMessages(requests []struct {
	AccountName   string
	SourceMailbox string
	MessageID     string
	TargetMailbox string
}) error {
	if len(requests) == 0 {
		return nil
	}

	// If only one request, no need for parallelization
	if len(requests) == 1 {
		req := requests[0]
		return c.MoveMessage(req.AccountName, req.SourceMailbox, req.MessageID, req.TargetMailbox)
	}

	// Process moves in parallel
	errors := make(chan error, len(requests))

	// Launch goroutine for each move operation
	for _, req := range requests {
		go func(r struct {
			AccountName   string
			SourceMailbox string
			MessageID     string
			TargetMailbox string
		}) {
			errors <- c.MoveMessage(r.AccountName, r.SourceMailbox, r.MessageID, r.TargetMailbox)
		}(req)
	}

	// Collect results
	var errorList []error
	for i := 0; i < len(requests); i++ {
		if err := <-errors; err != nil {
			errorList = append(errorList, err)
		}
	}

	if len(errorList) > 0 {
		return fmt.Errorf("failed to move some messages: %v", errorList)
	}

	return nil
}
