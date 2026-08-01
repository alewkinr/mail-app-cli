package mail

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestEmbeddedScripts(t *testing.T) {
	tests := []struct {
		name     string
		language string
		source   string
	}{
		{name: "get_accounts", language: "AppleScript", source: getAccountsAppleScript},
		{name: "get_mailboxes", language: "AppleScript", source: getMailboxesAppleScript},
		{name: "get_all_mailboxes", language: "AppleScript", source: getAllMailboxesAppleScript},
		{name: "get_messages", language: "AppleScript", source: getMessagesAppleScript},
		{name: "search_messages", language: "AppleScript", source: searchMessagesAppleScript},
		{name: "mark_message_read", language: "JavaScript", source: markMessageReadJXAScript},
		{name: "flag_message", language: "JavaScript", source: flagMessageJXAScript},
		{name: "delete_message", language: "JavaScript", source: deleteMessageJXAScript},
		{name: "send_message", language: "AppleScript", source: sendMessageAppleScript},
		{name: "get_unread_count", language: "AppleScript", source: getUnreadCountAppleScript},
		{name: "get_accounts_json", language: "JavaScript", source: getAccountsJXAScript},
		{name: "sync_account", language: "AppleScript", source: syncAccountAppleScript},
		{name: "sync_all_accounts", language: "AppleScript", source: syncAllAccountsAppleScript},
		{name: "get_mailboxes_json", language: "JavaScript", source: getMailboxesJXAScript},
		{name: "get_messages_json", language: "JavaScript", source: getMessagesJXAScript},
		{name: "get_message_details_json", language: "JavaScript", source: getMessageDetailsJXAScript},
		{name: "archive", language: "JavaScript", source: archiveJXAScript},
		{name: "move_message", language: "JavaScript", source: moveMessageJXAScript},
		{name: "get_attachments_json", language: "JavaScript", source: getAttachmentsJXAScript},
		{name: "save_attachment", language: "JavaScript", source: saveAttachmentJXAScript},
		{name: "search_messages_json", language: "JavaScript", source: searchMessagesJXAScript},
		{name: "get_special_mailbox_json", language: "JavaScript", source: getSpecialMailboxJXAScript},
		{name: "resolve_message_identity", language: "JavaScript", source: resolveMessageIdentityJXAScript},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if strings.TrimSpace(test.source) == "" {
				t.Fatal("embedded script is empty")
			}
			if runtime.GOOS != "darwin" {
				return
			}

			outputPath := filepath.Join(t.TempDir(), test.name+".scpt")
			cmd := exec.Command(
				"osacompile",
				"-l", test.language,
				"-o", outputPath,
				"-e", test.source,
			)
			if output, err := cmd.CombinedOutput(); err != nil {
				if test.language == "AppleScript" &&
					strings.Contains(string(output), "com.apple.hiservices-xpcservice") {
					t.Skip("Mail.app terminology is unavailable in this sandbox")
				}
				t.Fatalf("failed to compile embedded script: %v\n%s", err, output)
			}
		})
	}
}

func TestRunJXA(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("osascript is only available on macOS")
	}

	t.Run("without arguments", func(t *testing.T) {
		const script = `JSON.stringify([]);`

		output, err := NewClient().runJXA(script)
		if err != nil {
			t.Fatalf("runJXA() error = %v", err)
		}
		if output != "[]" {
			t.Fatalf("runJXA() output = %q, want %q", output, "[]")
		}
	})

	t.Run("with arguments", func(t *testing.T) {
		const script = `
function run(argv) {
	return JSON.stringify(argv);
}
`

		want := []string{"Account Name", "INBOX", "message-id@example.com"}
		output, err := NewClient().runJXA(script, want...)
		if err != nil {
			t.Fatalf("runJXA() error = %v", err)
		}

		var got []string
		if err := json.Unmarshal([]byte(output), &got); err != nil {
			t.Fatalf("failed to parse JXA output %q: %v", output, err)
		}

		if len(got) != len(want) {
			t.Fatalf("runJXA() returned %d args, want %d", len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("arg %d = %q, want %q", i, got[i], want[i])
			}
		}
	})
}

func TestRunAppleScriptWithArguments(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("osascript is only available on macOS")
	}

	const script = `
on run argv
	return item 1 of argv & "|" & item 2 of argv
end run
`

	output, err := NewClient().runAppleScript(script, "first", "second")
	if err != nil {
		t.Fatalf("runAppleScript() error = %v", err)
	}
	if output != "first|second" {
		t.Fatalf("runAppleScript() output = %q, want %q", output, "first|second")
	}
}
