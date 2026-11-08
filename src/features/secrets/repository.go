package secrets

import (
	"fmt"

	"github.com/awake-client/awake/src/features/environments"
	"github.com/awake-client/awake/src/features/workspace"
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

// Copy stores, for the target environment, the value of every secret of the
// source environment that the target also declares.
func (repository *Repository) Copy(workspaceRoot string, scope string, sourceFileName string, targetFileName string) error {
	source, err := repository.loadContext(workspaceRoot, scope, sourceFileName)
	if err != nil {
		return err
	}
	target, err := repository.loadContext(workspaceRoot, scope, targetFileName)
	if err != nil {
		return err
	}

	for _, name := range source.names {
		if target.requireDeclared(name) != nil {
			continue
		}
		value, found, err := repository.keychain.Get(source.service, source.account(name))
		if err != nil {
			return err
		}
		if !found {
			continue
		}
		if err := repository.keychain.Set(target.service, target.account(name), value); err != nil {
			return err
		}
	}
	return nil
}

// Move transfers the stored secret values of a renamed environment from its
// former file name to its current one.
func (repository *Repository) Move(workspaceRoot string, scope string, formerFileName string, fileName string) error {
	target, err := repository.loadContext(workspaceRoot, scope, fileName)
	if err != nil {
		return err
	}
	source := environmentContext{service: target.service, environmentName: accountPrefix(scope, formerFileName)}

	for _, name := range target.names {
		value, found, err := repository.keychain.Get(source.service, source.account(name))
		if err != nil {
			return err
		}
		if !found {
			continue
		}
		if err := repository.keychain.Set(target.service, target.account(name), value); err != nil {
			return err
		}
		if err := repository.keychain.Delete(source.service, source.account(name)); err != nil {
			return err
		}
	}
	return nil
}

// accountPrefix names an environment in the keychain. Global environments
// keep their historical account names; collection environments are prefixed
// by their scope so they never share values.
func accountPrefix(scope string, environmentFileName string) string {
	if scope == "" {
		return environmentFileName
	}
	return scope + ":" + environmentFileName
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

	for _, name := range resolved.SecretNames() {
		if !environments.IsVariableName(name) {
			return environmentContext{}, ErrInvalidSecretName
		}
	}

	return environmentContext{
		service:         keychainServicePrefix + configuration.ID,
		environmentName: accountPrefix(scope, environmentFileName),
		names:           resolved.SecretNames(),
	}, nil
}
