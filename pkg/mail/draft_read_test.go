package mail

import (
	"encoding/json"
	"errors"
	"runtime"
	"strings"
	"testing"
)

const draftReadFixture = `
function draftFixture(scenario) {
	const sample = fixtureMail().accounts()[0].mailboxes()[0].messages()[0];
	const work = {id: () => '1', name: () => 'Work'};
	const personal = {id: () => '2', name: () => scenario === 'ambiguous account' ? 'Work' : 'Personal'};
	function mailbox(account, name, rows) {
		const messages = () => rows;
		messages.id = () => rows.map(row => row.id());
		messages.at = index => rows[index];
		return {account: () => account, name: () => name, messages};
	}
	work.mailboxes = () => [mailbox(work, 'Drafts', [Object.assign({}, sample, {id: () => 99})])];
	const deleted = Object.assign({}, sample, {id: () => 98, deletedStatus: () => true});
	const personalDraft = Object.assign({}, sample, {id: () => 43, messageId: () => ''});
	const draftMailboxes = [
		mailbox(work, 'Borradores', scenario === 'ambiguous ID' ? [sample, sample] : [sample, deleted]),
		mailbox(personal, 'Entwürfe', scenario === 'empty' ? [] : [personalDraft])
	];
	if (scenario === 'missing mailbox') draftMailboxes.shift();
	if (scenario === 'ambiguous mailbox') draftMailboxes.push(mailbox(work, 'Other Drafts', []));
	return {
		accounts: () => [work, personal],
		draftsMailbox: () => ({mailboxes: () => draftMailboxes})
	};
}
`

func runDraftReadFixture(t *testing.T, scenario string, readsDetail bool, args ...string) (string, error) {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Skip("osascript is only available on macOS")
	}
	fixtureArgs, err := json.Marshal(scenario)
	if err != nil {
		t.Fatal(err)
	}
	detailFlag := "false"
	if readsDetail {
		detailFlag = "true"
	}
	script := strings.Replace(readMessagesJXAScript, "Application('Mail')", "draftFixture("+string(fixtureArgs)+")", 1) +
		messageReadFixture + draftReadFixture + "\nconst fixtureReadsDetail = " + detailFlag + ";"
	return NewClient().runJXA(script, args...)
}

func TestDraftReaderList(t *testing.T) {
	for _, test := range []struct {
		name, scenario, account string
		wantIDs                 string
	}{
		{name: "all accounts", wantIDs: "42,43"},
		{name: "scoped", account: "Work", wantIDs: "42"},
		{name: "empty", scenario: "empty", account: "Personal"},
	} {
		t.Run(test.name, func(t *testing.T) {
			output, err := runDraftReadFixture(t, test.scenario, false, "draft-list", test.account)
			if err != nil {
				t.Fatal(err)
			}
			var rows []Message
			if err := json.Unmarshal([]byte(output), &rows); err != nil {
				t.Fatal(err)
			}
			if rows == nil || strings.Contains(output, "content") {
				t.Fatalf("want an array of metadata, got %s", output)
			}
			var ids []string
			for _, row := range rows {
				ids = append(ids, row.ID)
				if row.ID == "42" && (row.Account != "Work" || row.Mailbox != "Borradores") {
					t.Errorf("wrong draft scope: %+v", row)
				}
				if row.ID == "43" && row.RFCMessageID != nil {
					t.Errorf("want null RFC ID, got %+v", row)
				}
			}
			if got := strings.Join(ids, ","); got != test.wantIDs {
				t.Errorf("got IDs %q, want %q", got, test.wantIDs)
			}
		})
	}
}

func TestDraftReaderShow(t *testing.T) {
	output, err := runDraftReadFixture(t, "", true, "draft-show", "Work", "42")
	if err != nil {
		t.Fatal(err)
	}
	drafts, err := decodeMessageDetailsBatch(output, "Work", "Drafts", 1)
	if err != nil {
		t.Fatal(err)
	}
	if draft := drafts[0]; draft.ID != "42" || draft.Mailbox != "Borradores" || draft.Content != "Message body" {
		t.Errorf("unexpected draft detail: %+v", draft)
	}
}

func TestDraftReaderScopeFailures(t *testing.T) {
	for _, test := range []struct {
		scenario string
		account  string
		want     error
	}{
		{scenario: "missing account", account: "Missing", want: ErrAccountNotFound},
		{scenario: "ambiguous account", account: "Work", want: ErrAccountAmbiguous},
		{scenario: "missing mailbox", account: "Work", want: ErrMailboxNotFound},
		{scenario: "ambiguous mailbox", account: "Work", want: ErrMailboxAmbiguous},
	} {
		t.Run(test.scenario, func(t *testing.T) {
			for _, args := range [][]string{{"draft-list", test.account}, {"draft-show", test.account, "42"}} {
				if _, err := runDraftReadFixture(t, test.scenario, false, args...); !errors.Is(err, test.want) {
					t.Errorf("%s got %v, want %v", args[0], err, test.want)
				}
			}
		})
	}
}

func TestDraftReaderShowRejectsMissingAndAmbiguousIDs(t *testing.T) {
	for _, test := range []struct{ scenario, id, want string }{
		{scenario: "missing", id: "99", want: "missing"},
		{scenario: "deleted", id: "98", want: "missing"},
		{scenario: "ambiguous ID", id: "42", want: "ambiguous"},
	} {
		t.Run(test.scenario, func(t *testing.T) {
			output, err := runDraftReadFixture(t, test.scenario, false, "draft-show", "Work", test.id)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := decodeMessageDetailsBatch(output, "Work", "Drafts", 1); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Errorf("got %v, want %q", err, test.want)
			}
		})
	}
}
