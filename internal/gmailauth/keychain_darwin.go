//go:build darwin && cgo

package gmailauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	keychain "github.com/keybase/go-keychain"
)

const keychainService = "com.alewkinr.mail-app-cli.gmail-oauth"

type keychainStore struct {
	service string
}

// NewStore returns the native macOS Keychain-backed authorization store.
func NewStore() Store {
	return newKeychainStore(keychainService)
}

func newKeychainStore(service string) Store {
	return &keychainStore{service: service}
}

func (store *keychainStore) Get(ctx context.Context, mailAccountID string) (AuthorizationRecord, error) {
	if err := ctx.Err(); err != nil {
		return AuthorizationRecord{}, err
	}
	if err := validateMailAccountID(mailAccountID); err != nil {
		return AuthorizationRecord{}, err
	}

	query := store.query(mailAccountID)
	query.SetMatchLimit(keychain.MatchLimitOne)
	query.SetReturnData(true)
	results, err := keychain.QueryItem(query)
	if err != nil {
		return AuthorizationRecord{}, keychainError("get authorization", err)
	}
	if len(results) == 0 {
		return AuthorizationRecord{}, fmt.Errorf("get Gmail authorization: %w", ErrNotFound)
	}
	if len(results) != 1 {
		return AuthorizationRecord{}, fmt.Errorf("%w: Keychain returned multiple records", ErrInvalidRecord)
	}

	record, err := decodeAuthorizationRecord(results[0].Data)
	if err != nil {
		return AuthorizationRecord{}, err
	}
	if err := validateRecordAccountKey(record, mailAccountID); err != nil {
		return AuthorizationRecord{}, err
	}
	return record, nil
}

func (store *keychainStore) Put(ctx context.Context, record AuthorizationRecord) error {
	data, err := encodeAuthorizationRecord(record)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	item := store.item(record.MailAccountID)
	item.SetData(data)
	if err := keychain.AddItem(item); err != nil {
		if !errors.Is(err, keychain.ErrorDuplicateItem) {
			return keychainError("add authorization", err)
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		update := keychain.NewItem()
		update.SetData(data)
		update.SetSynchronizable(keychain.SynchronizableNo)
		update.SetAccessible(keychain.AccessibleWhenUnlocked)
		if err := keychain.UpdateItem(store.query(record.MailAccountID), update); err != nil {
			return keychainError("update authorization", err)
		}
	}

	return nil
}

func (store *keychainStore) Delete(ctx context.Context, mailAccountID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateMailAccountID(mailAccountID); err != nil {
		return err
	}

	if err := keychain.DeleteItem(store.query(mailAccountID)); err != nil {
		if errors.Is(err, keychain.ErrorItemNotFound) {
			return fmt.Errorf("delete Gmail authorization: %w", ErrNotFound)
		}
		return keychainError("delete authorization", err)
	}
	return nil
}

func (store *keychainStore) List(ctx context.Context) ([]AuthorizationRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	query := store.query("")
	query.SetMatchLimit(keychain.MatchLimitAll)
	query.SetReturnAttributes(true)
	query.SetReturnData(true)
	results, err := keychain.QueryItem(query)
	if err != nil {
		return nil, keychainError("list authorizations", err)
	}

	records := make([]AuthorizationRecord, 0, len(results))
	for _, result := range results {
		record, err := decodeAuthorizationRecord(result.Data)
		if err != nil {
			return nil, err
		}
		if err := validateRecordAccountKey(record, result.Account); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func (store *keychainStore) item(mailAccountID string) keychain.Item {
	item := store.query(mailAccountID)
	item.SetLabel("mail-app-cli Gmail OAuth")
	item.SetSynchronizable(keychain.SynchronizableNo)
	item.SetAccessible(keychain.AccessibleWhenUnlocked)
	return item
}

func (store *keychainStore) query(mailAccountID string) keychain.Item {
	item := keychain.NewItem()
	item.SetSecClass(keychain.SecClassGenericPassword)
	item.SetService(store.service)
	item.SetAccount(mailAccountID)
	return item
}

func encodeAuthorizationRecord(record AuthorizationRecord) ([]byte, error) {
	if err := record.Validate(); err != nil {
		return nil, err
	}

	data, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("%w: encode record", ErrInvalidRecord)
	}
	return data, nil
}

func decodeAuthorizationRecord(data []byte) (AuthorizationRecord, error) {
	var record AuthorizationRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return AuthorizationRecord{}, fmt.Errorf("%w: decode Keychain record", ErrInvalidRecord)
	}
	if err := record.Validate(); err != nil {
		return AuthorizationRecord{}, fmt.Errorf("validate Keychain record: %w", err)
	}
	return record, nil
}

func keychainError(operation string, err error) error {
	if errors.Is(err, keychain.ErrorParam) ||
		errors.Is(err, keychain.ErrorNotAvailable) ||
		errors.Is(err, keychain.ErrorAuthFailed) {
		return fmt.Errorf(
			"macOS Keychain %s failed: the login Keychain is unavailable; "+
				"lock and unlock the login Keychain in Keychain Access, then retry: %w",
			operation,
			err,
		)
	}
	return fmt.Errorf("macOS Keychain %s failed: %w", operation, err)
}
