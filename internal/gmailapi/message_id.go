package gmailapi

import (
	"errors"
	"strings"
)

// NormalizeRFCMessageID converts an RFC Message-ID into the value expected by
// Gmail's rfc822msgid search operator.
func NormalizeRFCMessageID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("RFC Message-ID is required")
	}

	if strings.HasPrefix(value, "<") && strings.HasSuffix(value, ">") {
		value = strings.TrimSpace(value[1 : len(value)-1])
		if value == "" {
			return "", errors.New("RFC Message-ID is required")
		}
	}

	for _, character := range []byte(value) {
		if character < 0x20 || character == 0x7f {
			return "", errors.New("RFC Message-ID contains an ASCII control character")
		}
	}
	return value, nil
}
