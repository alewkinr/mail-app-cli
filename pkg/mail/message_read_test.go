package mail

import (
	"bytes"
	"encoding/json"
	"runtime"
	"strings"
	"testing"
)

const messageReadFixture = `
function fixtureMail() {
	const recipients = () => [{address: () => 'reader@example.com'}];
	const first = {
		id: () => 42,
		messageId: () => 'first@example.com',
		subject: () => 'Example',
		sender: () => 'sender@example.com',
		dateReceived: () => new Date('2026-11-17T00:00:00Z'),
		dateSent: () => new Date('2026-11-17T00:00:00Z'),
		readStatus: () => false,
		flaggedStatus: () => true,
		content: () => {
			if (!fixtureReadsDetail) throw new Error('unexpected body read');
			return 'Message body';
		},
		messageSize: () => 12,
		toRecipients: recipients,
		ccRecipients: () => [],
		bccRecipients: () => [],
		deletedStatus: () => false
	};
	const second = Object.assign({}, first, {
		id: () => 43,
		messageId: () => { throw new Error('RFC header unavailable'); }
	});
	const messages = () => [first, second];
	messages.id = () => [42, 43];
	messages.at = index => messages()[index];
	const mailbox = {name: () => 'INBOX', messages: messages, account: () => account};
	const mailboxes = () => [mailbox];
	mailboxes.byName = () => mailbox;
	const account = {name: () => 'Test', mailboxes: mailboxes};
	const accounts = () => [account];
	accounts.byName = () => account;
	return {accounts: accounts, sentMailboxes: () => [mailbox]};
}
`

func TestMessageReaderMetadata(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("osascript is only available on macOS")
	}

	tests := []struct {
		name string
		args []string
	}{
		{name: "list", args: []string{"list", "Test", "INBOX", "25", "0", "false", "false", ""}},
		{name: "search", args: []string{"search", "example", "Test", "INBOX", "25"}},
		{name: "special mailbox", args: []string{"special", "sent", "25"}},
	}
	script := strings.Replace(readMessagesJXAScript, "Application('Mail')", "fixtureMail()", 1) +
		messageReadFixture + "\nconst fixtureReadsDetail = false;"
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output, err := NewClient().runJXA(script, test.args...)
			if err != nil {
				t.Fatal(err)
			}
			var rows []Message
			if err := json.Unmarshal([]byte(output), &rows); err != nil {
				t.Fatal(err)
			}
			if len(rows) != 2 {
				t.Fatalf("got %d rows, want 2", len(rows))
			}
			if rows[0].ID != "42" || rows[0].RFCMessageID == nil || *rows[0].RFCMessageID != "first@example.com" {
				t.Fatalf("unexpected first row: %+v", rows[0])
			}
			if rows[1].ID != "43" || rows[1].RFCMessageID != nil {
				t.Fatalf("unexpected second row: %+v", rows[1])
			}

			encoded, err := json.Marshal(rows)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(encoded, []byte(`"rfcMessageID":null`)) {
				t.Fatalf("metadata output omitted the missing RFC header: %s", encoded)
			}
			for _, key := range []string{`"content"`, `"messageSize"`, `"toRecipients"`} {
				if strings.Contains(output, key) || bytes.Contains(encoded, []byte(key)) {
					t.Fatalf("metadata output unexpectedly included %s", key)
				}
			}
		})
	}
}

func TestSingleMessageReaderDetail(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("osascript is only available on macOS")
	}
	script := strings.Replace(readMessagesJXAScript, "Application('Mail')", "fixtureMail()", 1) +
		messageReadFixture + "\nconst fixtureReadsDetail = true;"
	output, err := NewClient().runJXA(script, "show", "Test", "INBOX", "local", "42")
	if err != nil {
		t.Fatal(err)
	}

	details, err := decodeMessageDetailsBatch(output, "Test", "INBOX", 1)
	if err != nil {
		t.Fatal(err)
	}
	detail := details[0]
	if detail.ID != "42" || detail.RFCMessageID == nil || *detail.RFCMessageID != "first@example.com" {
		t.Fatalf("unexpected message identity: %+v", detail.Message)
	}
	if detail.Content != "Message body" || detail.MessageSize != 12 {
		t.Fatalf("unexpected detail: %+v", detail)
	}
	if len(detail.ToRecipients) != 1 || detail.ToRecipients[0] != "reader@example.com" {
		t.Fatalf("unexpected recipients: %v", detail.ToRecipients)
	}
}
