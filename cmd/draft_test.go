package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/intelligrit/mail-app-cli/pkg/mail"
)

type stubDraftReader struct {
	list   func(account string) ([]mail.Message, error)
	show   func(account, localID string) (*mail.MessageDetail, error)
	create func(account string, options mail.DraftOptions) (*mail.Message, error)
	delete func(account, localID string) (mail.DraftMove, error)
}

func (s stubDraftReader) ListDrafts(account string) ([]mail.Message, error) {
	return s.list(account)
}

func (s stubDraftReader) ShowDraft(account, localID string) (*mail.MessageDetail, error) {
	return s.show(account, localID)
}

func (s stubDraftReader) CreateDraft(account string, options mail.DraftOptions) (*mail.Message, error) {
	return s.create(account, options)
}

func TestDraftList(t *testing.T) {
	for _, account := range []string{"", "Work"} {
		t.Run(account, func(t *testing.T) {
			command := newDraftCmd(stubDraftReader{list: func(gotAccount string) ([]mail.Message, error) {
				if gotAccount != account {
					t.Errorf("got account %q, want %q", gotAccount, account)
				}
				return []mail.Message{}, nil
			}})
			var output bytes.Buffer
			command.SetOut(&output)
			args := []string{"list"}
			if account != "" {
				args = append(args, "--account", account)
			}
			command.SetArgs(args)
			if err := command.Execute(); err != nil {
				t.Fatal(err)
			}
			if got := output.String(); got != "[]\n" {
				t.Errorf("got %q, want an empty JSON array", got)
			}
		})
	}
}

func TestDraftShow(t *testing.T) {
	command := newDraftCmd(stubDraftReader{show: func(account, localID string) (*mail.MessageDetail, error) {
		if account != "Work" || localID != "42" {
			t.Errorf("got account %q and ID %q", account, localID)
		}
		return &mail.MessageDetail{Message: mail.Message{ID: localID}, Content: "draft body"}, nil
	}})
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"show", "42", "--account", "Work"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); !strings.HasPrefix(got, "{\n") || !strings.Contains(got, `"content": "draft body"`) {
		t.Errorf("got %q, want a JSON detail object", got)
	}
}

func TestDraftShowFailuresLeaveStdoutEmpty(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "missing account", args: []string{"show", "42"}, want: "--account is required"},
		{name: "empty local ID", args: []string{"show", " ", "--account", "Work"}, want: "draft ID must be nonempty"},
		{name: "lookup failure", args: []string{"show", "42", "--account", "Work"}, want: "draft not found"},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := newDraftCmd(stubDraftReader{show: func(account, localID string) (*mail.MessageDetail, error) {
				if test.name != "lookup failure" {
					t.Fatal("invalid input reached draft lookup")
				}
				return nil, errors.New("draft not found")
			}})
			var output bytes.Buffer
			command.SetOut(&output)
			command.SetArgs(test.args)
			err := command.Execute()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Errorf("got error %v, want %q", err, test.want)
			}
			if output.Len() != 0 {
				t.Errorf("got stdout %q, want empty", output.String())
			}
		})
	}
}

func TestMessagesDraftsCommandRemoved(t *testing.T) {
	if !messagesCmd.Runnable() {
		t.Fatal("messages must validate arguments before falling back to help")
	}
	if err := messagesCmd.ValidateArgs([]string{"drafts"}); err == nil {
		t.Fatal("legacy messages drafts invocation is accepted")
	}
	for _, command := range messagesCmd.Commands() {
		if command.Name() == "drafts" || command.HasAlias("drafts") {
			t.Fatal("legacy messages drafts command is still registered")
		}
	}
}

func TestDraftCreate(t *testing.T) {
	command := newDraftCmd(stubDraftReader{create: func(account string, options mail.DraftOptions) (*mail.Message, error) {
		if account != "Work" || options.Body != "body only" || options.Subject != "" || len(options.To) != 0 {
			t.Errorf("unexpected account %q or options %+v", account, options)
		}
		return &mail.Message{ID: "42", Mailbox: "Drafts"}, nil
	}})
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"create", "--account", "Work", "--body", "body only"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"id": "42"`) {
		t.Errorf("got %q, want saved metadata", output.String())
	}
}

func (s stubDraftReader) DeleteDraft(account, localID string) (mail.DraftMove, error) {
	return s.delete(account, localID)
}

func TestDraftDelete(t *testing.T) {
	command := newDraftCmd(stubDraftReader{delete: func(account, localID string) (mail.DraftMove, error) {
		if account != "Work" || localID != "42" {
			t.Errorf("got account %q and ID %q", account, localID)
		}
		return mail.DraftMove{Account: account, DraftID: localID}, nil
	}})
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"delete", "42", "--account", "Work"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"draftId": "42"`) {
		t.Errorf("got %q, want deleted draft ID", output.String())
	}
}

func TestDraftDeleteFailuresLeaveStdoutEmpty(t *testing.T) {
	tests := []struct {
		name, account, id string
		want              string
	}{
		{name: "missing account", id: "42", want: "--account is required"},
		{name: "blank ID", account: "Work", id: " ", want: "draft ID must be nonempty"},
		{name: "uncertain result", account: "Work", id: "42", want: "trash move is uncertain"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := newDraftCmd(stubDraftReader{delete: func(account, localID string) (mail.DraftMove, error) {
				if test.name != "uncertain result" {
					t.Fatal("invalid arguments reached draft deletion")
				}
				return mail.DraftMove{}, errors.New("trash move is uncertain")
			}})
			var output bytes.Buffer
			command.SetOut(&output)
			command.SetArgs([]string{"delete", test.id, "--account", test.account})
			err := command.Execute()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Errorf("got error %v, want %q", err, test.want)
			}
			if output.Len() != 0 {
				t.Errorf("got stdout %q, want empty", output.String())
			}
		})
	}
}

func TestDraftCreateInvalidatesAccountCacheOnError(t *testing.T) {
	account := "Work"
	want := errors.New("close failed")
	var invalidated []string
	command := newDraftCreateCmd(stubDraftReader{create: func(string, mail.DraftOptions) (*mail.Message, error) {
		return nil, want
	}}, &account, func(account, mailbox string) {
		invalidated = append(invalidated, account, mailbox)
	})
	command.SilenceErrors = true
	command.SilenceUsage = true
	var output bytes.Buffer
	command.SetOut(&output)
	if err := command.Execute(); !errors.Is(err, want) {
		t.Fatalf("create error = %v, want %v", err, want)
	}
	if strings.Join(invalidated, "/") != "Work/" {
		t.Errorf("invalidated %v, want Work with all mailboxes", invalidated)
	}
	if output.Len() != 0 {
		t.Errorf("stdout = %q, want empty", output.String())
	}
}
