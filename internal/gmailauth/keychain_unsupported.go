//go:build !darwin || !cgo

package gmailauth

import (
	"context"
	"fmt"
)

type unsupportedStore struct{}

// NewStore returns a store that reports the native platform requirement.
func NewStore() Store {
	return unsupportedStore{}
}

func (unsupportedStore) Get(context.Context, string) (AuthorizationRecord, error) {
	return AuthorizationRecord{}, unsupportedPlatformError()
}

func (unsupportedStore) Put(context.Context, AuthorizationRecord) error {
	return unsupportedPlatformError()
}

func (unsupportedStore) Delete(context.Context, string) error {
	return unsupportedPlatformError()
}

func (unsupportedStore) List(context.Context) ([]AuthorizationRecord, error) {
	return nil, unsupportedPlatformError()
}

func unsupportedPlatformError() error {
	return fmt.Errorf(
		"Gmail authorization storage requires macOS with CGO_ENABLED=1: %w",
		ErrUnsupportedPlatform,
	)
}
