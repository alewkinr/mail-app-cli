package mail

import (
	"errors"
	"os"
	"path/filepath"

	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

func TestReconcileDraft(t *testing.T) {
	makeDraft := func(id, subject, body string) savedDraft {
		return savedDraft{MessageDetail: MessageDetail{
			Message: Message{ID: id, Account: "Work", Mailbox: "Drafts", Subject: subject, Sender: "Draft Sender <test@example.com>"},
			Content: body,
		}}
	}
	original := makeDraft("old", "", "body")
	newDraft := makeDraft("new", "", "body\r\n")
	otherDraft := makeDraft("other", "other", "body")
	ambiguous := makeDraft("duplicate", "", "body")
	baseline := draftSnapshot{AccountID: "account-id", Mailbox: "Drafts", Sender: "test@example.com", IDs: []string{"old"}}
	for _, test := range []struct {
		name         string
		observations []draftObservation
		wantID       string
		wantError    string
	}{
		{
			name: "fresh ID with identical pre-existing draft and nullable RFC ID",
			observations: []draftObservation{
				{Drafts: []savedDraft{original, otherDraft, newDraft}, Pending: true},
				{Drafts: []savedDraft{original, otherDraft, newDraft}},
				{Drafts: []savedDraft{original, otherDraft, newDraft}},
			},
			wantID: "new",
		},
		{
			name:         "ambiguous",
			observations: []draftObservation{{Drafts: []savedDraft{newDraft, ambiguous}}},
			wantError:    "multiple matching saved records",
		},
		{
			name:         "timeout",
			observations: []draftObservation{{Drafts: []savedDraft{original, otherDraft}}},
			wantError:    "timed out",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			index := 0
			draft, err := reconcileDraft("Work", baseline, DraftOptions{Body: "body"}, func() (draftObservation, error) {
				observation := test.observations[index]
				index++
				return observation, nil
			}, nil, len(test.observations), 0)
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), "uncertain") || !strings.Contains(err.Error(), test.wantError) || draft != nil {
					t.Errorf("got draft %+v, error %v, want uncertain %q", draft, err, test.wantError)
				}
				return
			}
			if err != nil || draft == nil || draft.ID != test.wantID || draft.RFCMessageID != nil {
				t.Errorf("got draft %+v error %v, want ID %q and nil RFC ID", draft, err, test.wantID)
			}
		})
	}
}

func TestMatchesCompleteDraft(t *testing.T) {
	options := DraftOptions{
		Subject: "test subject", Body: "first\nsecond",
		To: []string{"Example <one@example.com>"}, Cc: []string{"two@example.com"}, Bcc: []string{"three@example.com"},
		Attachments: []string{"/files/test.txt", "/files/other.txt"},
	}
	draft := savedDraft{
		MessageDetail: MessageDetail{
			Message:       Message{Subject: options.Subject, Sender: "Sender <test@example.com>"},
			Content:       "first\rsecond\r\n\uFFFC",
			ToRecipients:  []string{"one@example.com"},
			CcRecipients:  options.Cc,
			BccRecipients: options.Bcc,
		},
		Attachments: []string{"other.txt", "test.txt"},
	}
	if !matchesDraft(draft, "test@example.com", options) {
		t.Fatal("complete saved draft did not match its fingerprint")
	}
	draft.Attachments = slices.Delete(draft.Attachments, 0, 1)
	if matchesDraft(draft, "test@example.com", options) {
		t.Error("saved draft with a missing attachment matched")
	}
}

func TestCreateDraftRejectsMissingAttachment(t *testing.T) {
	path := t.TempDir() + "/missing.txt"
	draft, err := NewClient().CreateDraft("Work", DraftOptions{Body: "body", Attachments: []string{path}})
	if !errors.Is(err, os.ErrNotExist) || draft != nil {
		t.Errorf("got draft %+v, error %v, want missing attachment", draft, err)
	}
}

func TestDraftAccountLock(t *testing.T) {
	dir := t.TempDir()
	accountID := "account"
	first, err := lockDraftAccount(dir, accountID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = first.Close()
	})
	second, err := os.OpenFile(first.Name(), os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Close() })
	if err := syscall.Flock(int(second.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); !errors.Is(err, syscall.EWOULDBLOCK) {
		t.Fatalf("same-account lock contention error = %v, want EWOULDBLOCK", err)
	}
	other, err := lockDraftAccount(dir, accountID+"-other")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = other.Close()
	})
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
			move, err := deleteDraft(func(script string, args ...string) (string, error) {
				script = strings.Replace(script, "Application('Mail')", "fixtureMail()", 1)
				script = strings.Replace(script, "delay(0.35)", "void 0", 1)
				script += fixture + "\nconst fixtureCase = " + strconv.Quote(test.name) + ";"
				return NewClient().runJXA(script, args...)
			}, test.account, test.id)
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

func TestCheckDraftAttachmentDigests(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "0")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	digest, err := draftFileDigest(path)
	if err != nil {
		t.Fatal(err)
	}
	expected := map[draftAttachment]int{{name: "same.txt", digest: digest}: 1}
	names := []string{"same.txt"}

	if match, err := checkDraftAttachmentDigests(dir, names, expected, 1); err != nil || !match {
		t.Fatalf("matching file: got %v, %v", match, err)
	}
	if err := os.WriteFile(path, []byte("switched"), 0600); err != nil {
		t.Fatal(err)
	}
	if match, err := checkDraftAttachmentDigests(dir, names, expected, 1); err != nil || match {
		t.Errorf("same name and size with different bytes: got %v, %v", match, err)
	}
}

func TestCreateDraftRejectsObjectReplacementCharacter(t *testing.T) {
	draft, err := NewClient().CreateDraft("Work", DraftOptions{Body: "body \uFFFC"})
	if err == nil || !strings.Contains(err.Error(), "Unicode object replacement character") || draft != nil {
		t.Errorf("got draft %+v, error %v, want explicit body rejection", draft, err)
	}
}
