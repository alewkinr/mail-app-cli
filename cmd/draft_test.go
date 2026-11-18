package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/intelligrit/mail-app-cli/pkg/mail"
)

type stubDraftReader struct {
	list func(account string) ([]mail.Message, error)
	show func(account, localID string) (*mail.MessageDetail, error)
}

func (s stubDraftReader) ListDrafts(account string) ([]mail.Message, error) {
	return s.list(account)
}

func (s stubDraftReader) ShowDraft(account, localID string) (*mail.MessageDetail, error) {
	return s.show(account, localID)
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
	for _, command := range messagesCmd.Commands() {
		if command.Name() == "drafts" || command.HasAlias("drafts") {
			t.Fatal("legacy messages drafts command is still registered")
		}
	}
}
