package secrets

import (
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

// keyringKeychain stores secrets with the operating system keychain: Keychain
// on macOS, Credential Manager on Windows and the Secret Service on Linux.
type keyringKeychain struct{}

// NewKeyringKeychain returns the operating system keychain backend.
func NewKeyringKeychain() Keychain {
	return keyringKeychain{}
}

func (keyringKeychain) Get(service string, account string) (string, bool, error) {
	value, err := keyring.Get(service, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read secret: %w", err)
	}

	return value, true, nil
}

func (keyringKeychain) Set(service string, account string, value string) error {
	if err := keyring.Set(service, account, value); err != nil {
		return fmt.Errorf("write secret: %w", err)
	}
	return nil
}

func (keyringKeychain) Delete(service string, account string) error {
	err := keyring.Delete(service, account)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("delete secret: %w", err)
	}
	return nil
}
