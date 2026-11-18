package mail

import (
	"errors"
	"os"

	"runtime"
	"strconv"
	"strings"

	"testing"
)

func TestFindSavedDraft(t *testing.T) {
	makeDraft := func(id, subject string, recipients ...string) savedDraft {
		return savedDraft{
			Message:      Message{ID: id, Account: "Work", Mailbox: "Drafts", Subject: subject, Sender: "Draft Sender <test@example.com>"},
			ToRecipients: recipients,
		}
	}
	original := makeDraft("old", "", "someone@example.com")
	newDraft := makeDraft("new", "")
	otherDraft := makeDraft("other", "other")
	ambiguous := makeDraft("duplicate", "")
	scope := draftMailboxInfo{AccountID: "account-id", Mailbox: "Drafts", Sender: "test@example.com"}
	for _, test := range []struct {
		name         string
		observations []draftCandidates
		wantID       string
		wantReads    int
		wantError    string
	}{
		{
			name: "returns the first complete match immediately",
			observations: []draftCandidates{
				{Drafts: []savedDraft{original, otherDraft, newDraft}},
				{Drafts: []savedDraft{original, otherDraft, newDraft}},
			},
			wantID: "new", wantReads: 1,
		},
		{
			name: "matching blank subject and recipients with nullable RFC ID",
			observations: []draftCandidates{
				{Drafts: []savedDraft{original, otherDraft, newDraft}, Pending: true},
				{Drafts: []savedDraft{original, otherDraft, newDraft}},
				{Drafts: []savedDraft{original, otherDraft, newDraft}},
			},
			wantID: "new", wantReads: 2,
		},
		{
			name:         "ambiguous",
			observations: []draftCandidates{{Drafts: []savedDraft{newDraft, ambiguous}}},
			wantError:    "multiple matching drafts",
		},
		{
			name:         "timeout",
			observations: []draftCandidates{{Drafts: []savedDraft{original, otherDraft}}},
			wantError:    "no unique saved draft confirmed",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			index := 0
			draft, err := findSavedDraft("Work", scope, DraftOptions{Body: "body"}, func() (draftCandidates, error) {
				candidates := test.observations[index]
				index++
				return candidates, nil
			}, len(test.observations), 0)
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), "uncertain") || !strings.Contains(err.Error(), test.wantError) || draft != nil {
					t.Errorf("got draft %+v, error %v, want uncertain %q", draft, err, test.wantError)
				}
				return
			}
			if index != test.wantReads {
				t.Errorf("read Drafts %d times, want %d", index, test.wantReads)
			}
			if err != nil || draft == nil || draft.ID != test.wantID || draft.RFCMessageID != nil {
				t.Errorf("got draft %+v error %v, want ID %q and nil RFC ID", draft, err, test.wantID)
			}
		})
	}
}

func TestMatchesDraftEnvelope(t *testing.T) {
	options := DraftOptions{
		Subject: "test subject",
		To:      []string{"Example <one@example.com>"}, Cc: []string{"two@example.com"}, Bcc: []string{"three@example.com"},
	}
	draft := savedDraft{
		Message:       Message{Subject: options.Subject, Sender: "Sender <test@example.com>"},
		ToRecipients:  []string{"one@example.com"},
		CcRecipients:  options.Cc,
		BccRecipients: options.Bcc,
	}
	if !matchesDraftEnvelope(draft, "test@example.com", options) {
		t.Fatal("saved draft with matching subject, sender, and recipients did not match")
	}
	draft.CcRecipients, draft.BccRecipients = draft.BccRecipients, draft.CcRecipients
	if matchesDraftEnvelope(draft, "test@example.com", options) {
		t.Fatal("saved draft with swapped Cc and Bcc matched")
	}
}

func TestCreateDraftRejectsMissingAttachment(t *testing.T) {
	path := t.TempDir() + "/missing.txt"
	draft, err := NewClient().CreateDraft("Work", DraftOptions{Body: "body", Attachments: []string{path}})
	if !errors.Is(err, os.ErrNotExist) || draft != nil {
		t.Errorf("got draft %+v, error %v, want missing attachment", draft, err)
	}
}

func TestDeleteDraftBridge(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("osascript is only available on macOS")
	}
	const fixture = `
function fixtureMail() {
	const work = {id: () => '1', name: () => 'Work'};
	const personal = {id: () => '2', name: () => 'Personal'};
	const parent = owner => ({account: () => owner});
	let sourceReads = 0;
	let moved = false;
	const source = {name: () => 'Account Drafts', account: () => work};
	const destination = {name: () => 'Deleted Items', account: () => work};
	const personalDrafts = {name: () => 'Drafts', account: () => personal, messages: {id: () => []}};
	const draft = {id: () => 42, deletedStatus: () => false, messageId: () => '<unique@draft.invalid>'};
	Object.defineProperty(draft, 'mailbox', {
		get() { return () => fixtureCase === 'non-draft reference' ? {name: () => 'Inbox', ...parent(work)} : source; },
		set(value) {
			if (!['verified', 'nullable RFC ID', 'uncertain', 'ambiguous arrival'].includes(fixtureCase) ||
			    value !== destination || moved) throw Error('unexpected mutation');
			moved = true;
		}
	});
	source.messages = {
		id: () => {
			if (fixtureCase === 'ambiguous source') return [42, 42];
			return moved && sourceReads++ > 0 ? [] : [42];
		},
		byId: () => draft
	};
	const trashIDs = () => {
		if (!moved || fixtureCase === 'uncertain') return [1];
		if (fixtureCase === 'nullable RFC ID') return [1, 42];
		if (fixtureCase === 'ambiguous arrival') return [1, 43, 44];
		return [1, 43];
	};
	destination.messages = {
		id: trashIDs,
		byId: () => ({messageId: draft.messageId})
	};
	if (fixtureCase === 'nullable RFC ID') draft.messageId = () => '';
	return {
		accounts: () => [work, personal],
		draftsMailbox: () => ({mailboxes: () => [source, personalDrafts]}),
		trashMailbox: () => ({mailboxes: () => fixtureCase === 'missing Trash' ? [] :
			fixtureCase === 'ambiguous Trash' ? [destination, {...destination}] : [destination, {name: () => "Trash", account: () => personal}]})
	};
}
`
	tests := []struct {
		name, account, id string
		want              error
		wantText          string
	}{
		{name: "verified", account: "Work", id: "42"},
		{name: "nullable RFC ID", account: "Work", id: "42"},
		{name: "wrong account", account: "Personal", id: "42", want: ErrMessageNotFound},
		{name: "non-draft ID", account: "Work", id: "99", want: ErrMessageNotFound},
		{name: "non-draft reference", account: "Work", id: "42", want: ErrMessageNotFound},
		{name: "missing Trash", account: "Work", id: "42", want: ErrMailboxNotFound},
		{name: "ambiguous Trash", account: "Work", id: "42", want: ErrMailboxAmbiguous},
		{name: "ambiguous source", account: "Work", id: "42", wantText: "is ambiguous"},
		{name: "ambiguous arrival", account: "Work", id: "42", wantText: "is uncertain"},
		{name: "uncertain", account: "Work", id: "42", wantText: "is uncertain"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			script := strings.Replace(deleteDraftJXAScript, "Application('Mail')", "fixtureMail()", 1)
			script = strings.Replace(script, "delay(0.35)", "void 0", 1)
			script += fixture + "\nconst fixtureCase = " + strconv.Quote(test.name) + ";"
			output, scriptErr := NewClient().runJXA(script, test.account, test.id)
			move, err := draftMoveResult(test.account, test.id, output, scriptErr)
			switch {
			case test.want != nil:
				if !errors.Is(err, test.want) {
					t.Fatalf("got error %v, want %v", err, test.want)
				}
			case test.wantText != "":
				if err == nil || !strings.Contains(err.Error(), test.wantText) {
					t.Fatalf("got error %v, want %q", err, test.wantText)
				}
			default:
				want := DraftMove{Account: "Work", DraftID: "42", DraftMailbox: "Account Drafts", TrashMailbox: "Deleted Items"}
				if err != nil || move != want {
					t.Fatalf("got move %+v and error %v, want %+v", move, err, want)
				}
			}
		})
	}
}
