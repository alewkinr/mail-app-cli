package gmailapi

import "testing"

func TestNormalizeRFCMessageID(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "plain", input: "message@example.com", want: "message@example.com"},
		{name: "trim whitespace", input: "  message@example.com \t", want: "message@example.com"},
		{name: "angle brackets", input: "<message@example.com>", want: "message@example.com"},
		{name: "trim around brackets", input: " \n<message@example.com>\t", want: "message@example.com"},
		{name: "remove one pair", input: "<<message@example.com>>", want: "<message@example.com>"},
		{name: "unpaired bracket preserved", input: "<message@example.com", want: "<message@example.com"},
		{name: "non ASCII allowed", input: "méssage@example.com", want: "méssage@example.com"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := NormalizeRFCMessageID(test.input)
			if err != nil {
				t.Fatalf("NormalizeRFCMessageID() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("NormalizeRFCMessageID() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestNormalizeRFCMessageIDRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "empty", input: ""},
		{name: "whitespace", input: " \t\n"},
		{name: "empty brackets", input: "<>"},
		{name: "blank brackets", input: "< \t >"},
		{name: "newline control", input: "message\nid@example.com"},
		{name: "tab control", input: "message\tid@example.com"},
		{name: "NUL control", input: "message\x00id@example.com"},
		{name: "DEL control", input: "message\x7fid@example.com"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NormalizeRFCMessageID(test.input); err == nil {
				t.Fatal("NormalizeRFCMessageID() error = nil")
			}
		})
	}
}
