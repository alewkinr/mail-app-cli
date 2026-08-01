package cmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/intelligrit/mail-app-cli/internal/archive"
	"github.com/spf13/cobra"
)

type commandArchiveService struct {
	result  archive.Result
	err     error
	calls   int
	request archive.Request
	ctx     context.Context
}

func (service *commandArchiveService) Archive(
	ctx context.Context,
	request archive.Request,
) (archive.Result, error) {
	service.calls++
	service.ctx = ctx
	service.request = request
	return service.result, service.err
}

func TestMessagesArchivePublicShape(t *testing.T) {
	if messagesArchiveCmd.Use != "archive [message-id]" {
		t.Fatalf("Use = %q", messagesArchiveCmd.Use)
	}
	if err := messagesArchiveCmd.Args(messagesArchiveCmd, []string{"local-id"}); err != nil {
		t.Fatalf("one archive argument rejected: %v", err)
	}
	if err := messagesArchiveCmd.Args(messagesArchiveCmd, nil); err == nil {
		t.Fatal("missing archive argument accepted")
	}
	if err := messagesArchiveCmd.Args(
		messagesArchiveCmd,
		[]string{"one", "two"},
	); err == nil {
		t.Fatal("multiple archive arguments accepted")
	}

	accountFlag := messagesArchiveCmd.Flag("account")
	mailboxFlag := messagesArchiveCmd.Flag("mailbox")
	if accountFlag == nil || accountFlag.Shorthand != "a" {
		t.Fatal("archive account flag is not -a/--account")
	}
	if mailboxFlag == nil || mailboxFlag.Shorthand != "m" {
		t.Fatal("archive mailbox flag is not -m/--mailbox")
	}
	if !strings.Contains(messagesArchiveCmd.Long, "Gmail API") ||
		!strings.Contains(messagesArchiveCmd.Long, "do not require Accessibility") ||
		!strings.Contains(messagesArchiveCmd.Long, "require Accessibility") {
		t.Fatalf("archive help does not explain linked/unlinked Accessibility behavior")
	}
	if strings.Contains(messagesArchiveCmd.Long, "RFC Message-ID") {
		t.Fatal("archive help still advertises direct RFC Message-ID input")
	}
}

func TestRunArchiveSuccess(t *testing.T) {
	service := &commandArchiveService{
		result: archive.Result{Backend: archive.BackendGmailAPI},
	}
	var invalidations []string
	var stdout bytes.Buffer
	deps := archiveCommandDeps{
		Service: service,
		InvalidateMailboxCache: func(account, mailbox string) {
			invalidations = append(invalidations, account+"/"+mailbox)
		},
		Stdout: &stdout,
	}
	ctx := context.WithValue(context.Background(), archiveContextKey{}, "value")

	if err := runArchive(ctx, deps, "Personal", "INBOX", "local-42"); err != nil {
		t.Fatalf("runArchive() error = %v", err)
	}
	if service.calls != 1 || service.ctx != ctx {
		t.Fatal("runArchive() did not call service once with command context")
	}
	if service.request != (archive.Request{
		AccountName:    "Personal",
		MailboxName:    "INBOX",
		LocalMessageID: "local-42",
	}) {
		t.Fatalf("archive request = %#v", service.request)
	}
	wantInvalidations := "Personal/INBOX,Personal/Archive,Personal/All Mail"
	if strings.Join(invalidations, ",") != wantInvalidations {
		t.Fatalf("invalidations = %v, want %s", invalidations, wantInvalidations)
	}
	if stdout.String() != "Message archived\n" {
		t.Fatalf("stdout = %q, want exact success output", stdout.String())
	}
}

func TestRunArchiveFailureHasNoSuccessEffects(t *testing.T) {
	sentinel := errors.New("archive failed")
	service := &commandArchiveService{err: sentinel}
	var invalidations int
	var stdout bytes.Buffer
	err := runArchive(context.Background(), archiveCommandDeps{
		Service: service,
		InvalidateMailboxCache: func(string, string) {
			invalidations++
		},
		Stdout: &stdout,
	}, "Personal", "INBOX", "local-42")
	if !errors.Is(err, sentinel) {
		t.Fatalf("runArchive() error = %v, want sentinel", err)
	}
	if invalidations != 0 {
		t.Fatalf("cache invalidations = %d, want 0", invalidations)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
}

func TestMessagesArchiveCommandStillRequiresAccountAndMailbox(t *testing.T) {
	previousAccount := msgAccount
	previousMailbox := msgMailbox
	t.Cleanup(func() {
		msgAccount = previousAccount
		msgMailbox = previousMailbox
	})
	msgAccount = ""
	msgMailbox = ""

	err := messagesArchiveCmd.RunE(&cobra.Command{}, []string{"local-42"})
	if err == nil || !strings.Contains(err.Error(), "--account") {
		t.Fatalf("archive error = %v, want required flags error", err)
	}
}

type archiveContextKey struct{}
