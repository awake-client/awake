package secrets

import (
	"fmt"

	"github.com/clementherve/awake/src/features/environments"
	"github.com/clementherve/awake/src/features/workspace"
)

const keychainServicePrefix = "awake/"

// Repository reads and writes the secrets of a workspace environment.
type Repository struct {
	keychain Keychain
}

// NewRepository returns a secret repository backed by keychain.
func NewRepository(keychain Keychain) *Repository {
	return &Repository{keychain: keychain}
}

// States reports which declared secrets have a stored value.
func (repository *Repository) States(workspaceRoot string, scope string, environmentFileName string) ([]SecretState, error) {
	context, err := repository.loadContext(workspaceRoot, scope, environmentFileName)
	if err != nil {
		return nil, err
	}

	states := make([]SecretState, 0, len(context.names))
	for _, name := range context.names {
		_, found, err := repository.keychain.Get(context.service, context.account(name))
		if err != nil {
			return nil, err
		}
		states = append(states, SecretState{Name: name, IsSet: found})
	}

	return states, nil
}

// Values returns every stored secret of the environment. Missing values are
// omitted so the variable resolver can report the name that the request uses.
func (repository *Repository) Values(workspaceRoot string, scope string, environmentFileName string) (map[string]string, error) {
	context, err := repository.loadContext(workspaceRoot, scope, environmentFileName)
	if err != nil {
		return nil, err
	}

	values := make(map[string]string, len(context.names))
	for _, name := range context.names {
		value, found, err := repository.keychain.Get(context.service, context.account(name))
		if err != nil {
			return nil, err
		}
		if found {
			values[name] = value
		}
	}

	return values, nil
}

// Set stores or replaces one declared secret.
func (repository *Repository) Set(workspaceRoot string, scope string, environmentFileName string, name string, value string) error {
	context, err := repository.loadContext(workspaceRoot, scope, environmentFileName)
	if err != nil {
		return err
	}
	if err := context.requireDeclared(name); err != nil {
		return err
	}

	return repository.keychain.Set(context.service, context.account(name), value)
}

// Delete removes one declared secret from the keychain.
func (repository *Repository) Delete(workspaceRoot string, scope string, environmentFileName string, name string) error {
	context, err := repository.loadContext(workspaceRoot, scope, environmentFileName)
	if err != nil {
		return err
	}
	if err := context.requireDeclared(name); err != nil {
		return err
	}

	return repository.keychain.Delete(context.service, context.account(name))
}

type environmentContext struct {
	service         string
	environmentName string
	names           []string
}

// account scopes a secret to its environment, so two environments of the same
// workspace can declare a secret with the same name and different values.
func (context environmentContext) account(name string) string {
	return context.environmentName + "/" + name
}

func (context environmentContext) requireDeclared(name string) error {
	for _, declared := range context.names {
		if declared == name {
			return nil
		}
	}
	return ErrSecretNotDeclared
}

func (repository *Repository) loadContext(workspaceRoot string, scope string, environmentFileName string) (environmentContext, error) {
	configuration, err := workspace.NewRepository(workspaceRoot).Load()
	if err != nil {
		return environmentContext{}, fmt.Errorf("load workspace for secrets: %w", err)
	}

	environmentRepository, err := environments.NewScopedRepository(workspaceRoot, scope)
	if err != nil {
		return environmentContext{}, err
	}
	resolved, err := environmentRepository.Load(environmentFileName)
	if err != nil {
		return environmentContext{}, err
	}

	// Global environments keep their historical account names; collection
	// environments are prefixed by their scope so they never share values.
	accountPrefix := environmentFileName
	if scope != "" {
		accountPrefix = scope + ":" + environmentFileName
	}

	for _, name := range resolved.SecretNames() {
		if !environments.IsVariableName(name) {
			return environmentContext{}, ErrInvalidSecretName
		}
	}

	return environmentContext{
		service:         keychainServicePrefix + configuration.ID,
		environmentName: accountPrefix,
		names:           resolved.SecretNames(),
	}, nil
}
