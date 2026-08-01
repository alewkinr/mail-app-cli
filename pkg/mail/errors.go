package mail

import (
	"errors"
	"fmt"
)

var (
	ErrAccountNotFound  = errors.New("mail account not found")
	ErrAccountAmbiguous = errors.New("mail account name is ambiguous")
	ErrMailboxNotFound  = errors.New("mailbox not found")
	ErrMessageNotFound  = errors.New("message not found")
	ErrMessageIDMissing = errors.New("message is missing its RFC Message-ID")
)

func resolverError(code string) error {
	var err error
	switch code {
	case "account_not_found":
		err = ErrAccountNotFound
	case "account_ambiguous":
		err = ErrAccountAmbiguous
	case "mailbox_not_found":
		err = ErrMailboxNotFound
	case "message_not_found":
		err = ErrMessageNotFound
	case "message_id_missing":
		err = ErrMessageIDMissing
	default:
		return errors.New("Mail.app identity resolution failed")
	}

	return fmt.Errorf("Mail.app identity resolution failed: %w", err)
}
