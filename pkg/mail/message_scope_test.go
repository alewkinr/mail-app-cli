package mail

import (
	"errors"
	"runtime"
	"strings"
	"testing"
)

func TestScopedScriptsRejectMissingAccountOrMailbox(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("osascript is only available on macOS")
	}

	const fixture = `
function fixtureMail() {
	const inbox = {name: () => 'INBOX', messages: () => { throw new Error('unexpected message read'); }};
	inbox.messages.id = inbox.messages;
	const account = {name: () => 'Personal', mailboxes: () => [inbox]};
	return {accounts: () => [account]};
}
`
	tests := []struct {
		name, script string
		args         []string
		want         error
	}{
		{"list", readMessagesJXAScript, []string{"list", "Unknown", "INBOX"}, ErrAccountNotFound},
		{"search", readMessagesJXAScript, []string{"search", "query", "Personal", "Unknown"}, ErrMailboxNotFound},
		{"mailboxes", getMailboxesJXAScript, []string{"Unknown"}, ErrAccountNotFound},
		{"mark", markMessageReadJXAScript, []string{"Unknown", "INBOX", "42", "true"}, ErrAccountNotFound},
		{"flag", flagMessageJXAScript, []string{"Personal", "Unknown", "42", "true"}, ErrMailboxNotFound},
		{"delete", deleteMessageJXAScript, []string{"Personal", "Unknown", "42"}, ErrMailboxNotFound},
		{"archive", archiveJXAScript, []string{"Personal", "Unknown", "42"}, ErrMailboxNotFound},
		{"move", moveMessageJXAScript, []string{"Personal", "INBOX", "42", "Unknown"}, ErrMailboxNotFound},
		{"attachments list", getAttachmentsJXAScript, []string{"Personal", "Unknown", "42"}, ErrMailboxNotFound},
		{"attachments save", saveAttachmentJXAScript, []string{"Personal", "Unknown", "42", "file", "/tmp/file"}, ErrMailboxNotFound},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			script := strings.Replace(test.script, "Application('Mail')", "fixtureMail()", 1) + fixture
			output, err := NewClient().runJXA(script, test.args...)
			if output != "" || !errors.Is(err, test.want) {
				t.Fatalf("got output %q and error %v, want empty output and %v", output, err, test.want)
			}
		})
	}
}
