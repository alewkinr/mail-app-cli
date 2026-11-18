package mail

import (
	"errors"
	"runtime"
	"strings"
	"testing"
)

func TestShowMessagesScript(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("osascript is only available on macOS")
	}

	const mock = `
function mockApplication() {
	function mockMessage(id, header) {
		let currentID = id;
		return {
			id: () => currentID,
			messageId: () => header,
			subject: () => '',
			sender: () => '',
			dateReceived: () => new Date(0),
			dateSent: () => new Date(0),
			readStatus: () => true,
			flaggedStatus: () => false,
			content: () => {
				if (id === 90) {
					throw new Error('read body before resolving batch');
				}
				if (id === 91) currentID = 92;
				return 'body ' + id;
			},
			messageSize: () => 10,
			toRecipients: () => [],
			ccRecipients: () => [],
			bccRecipients: () => []
		};
	}
	const fixtures = [
		mockMessage(42, 'one@example.com'),
		mockMessage(7, '<two@example.com>'),
		mockMessage(8, 'two@example.com'),
		mockMessage(90, 'do-not-read@example.com'),
		mockMessage(91, 'moving@example.com')
	];
	const messages = () => fixtures;
	messages.id = () => fixtures.map(message => message.id());
	messages.at = index => fixtures[index];
	const mailbox = {name: () => 'INBOX', messages: messages};
	const account = {name: () => 'Personal', mailboxes: () => [mailbox]};
	return {accounts: () => [account]};
}
`
	// Provide a mock bridge inside a private scope because JXA owns the global Application.
	script := mock + "function run(argv) { return invokeMock(argv); }\n" +
		"const invokeMock = (function(Application) {\n" + readMessagesJXAScript + "\nreturn run; })(mockApplication);"

	tests := []struct {
		name, selector string
		ids            []string
		wantIDs        []string
		wantErrors     []string
	}{
		{name: "local batch in request order", selector: "local", ids: []string{"7", "42", "7"}, wantIDs: []string{"7", "42", "7"}},
		{
			name: "mailbox changes during detail read", selector: "local",
			ids: []string{"42", "91"}, wantErrors: []string{`"91" (mailbox changed during detail read)`},
		},
		{name: "RFC angle brackets", selector: "rfc", ids: []string{"<one@example.com>"}, wantIDs: []string{"42"}},
		{
			name: "missing and ambiguous before body read", selector: "rfc",
			ids:        []string{"do-not-read@example.com", "missing@example.com", "two@example.com"},
			wantErrors: []string{`account "Personal" mailbox "INBOX"`, `"missing@example.com" (missing)`, `"two@example.com" (ambiguous; local IDs: ["7" "8"])`},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args := append([]string{"show", "Personal", "INBOX", test.selector}, test.ids...)
			output, err := NewClient().runJXA(script, args...)
			if err != nil {
				t.Fatalf("run script: %v", err)
			}

			messages, err := decodeMessageDetailsBatch(output, "Personal", "INBOX", len(test.ids))
			if len(test.wantErrors) > 0 {
				if err == nil || len(messages) > 0 || strings.Contains(output, "messages") {
					t.Fatalf("failed batch got messages %v, error %v, script result %s", messages, err, output)
				}
				for _, want := range test.wantErrors {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q does not contain %q", err, want)
					}
				}
				return
			}

			if err != nil || len(messages) != len(test.wantIDs) {
				t.Fatalf("got %d messages and error %v", len(messages), err)
			}
			for i, want := range test.wantIDs {
				if messages[i].ID != want || messages[i].Content != "body "+want {
					t.Errorf("message %d ID = %q, content = %q, want ID %q", i, messages[i].ID, messages[i].Content, want)
				}
			}
		})
	}

	for _, test := range []struct {
		name, account, mailbox string
		want                   error
	}{
		{"missing account", "Unknown", "INBOX", ErrAccountNotFound},
		{"missing mailbox", "Personal", "Unknown", ErrMailboxNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			output, err := NewClient().runJXA(script, "show", test.account, test.mailbox, "rfc", "one@example.com")
			if !errors.Is(err, test.want) || output != "" {
				t.Fatalf("got output %q and error %v, want %v", output, err, test.want)
			}
		})
	}
}
