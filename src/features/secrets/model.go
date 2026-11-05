// Package secrets stores environment secret values in the operating system
// keychain. Secret names live in YAML; values never touch the workspace.
package secrets

import "errors"

var (
	// ErrSecretNotDeclared is returned when a name is not listed in the
	// environment secretVariables.
	ErrSecretNotDeclared = errors.New("secrets: variable is not declared as a secret")
	// ErrInvalidSecretName is returned for a malformed variable name.
	ErrInvalidSecretName = errors.New("secrets: invalid secret name")
)

// Keychain is the storage backend. It is an interface so tests can replace the
// operating system keychain with an in-memory map.
type Keychain interface {
	// Get returns the stored value and whether it exists.
	Get(service string, account string) (string, bool, error)
	// Set stores or replaces a value.
	Set(service string, account string, value string) error
	// Delete removes a value.
	Delete(service string, account string) error
}

// SecretState reports whether one declared secret has a stored value.
type SecretState struct {
	Name  string
	IsSet bool
}
