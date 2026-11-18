package mail

import (
	"errors"
	"os"
	"slices"
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
			}, len(test.observations), 0)
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
