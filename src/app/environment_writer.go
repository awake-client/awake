package app

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/awake-client/awake/src/domain"
	"github.com/awake-client/awake/src/features/environments"
	"github.com/awake-client/awake/src/features/secrets"
)

// environmentWriter saves the variables scripts store with awake.environment.
// It lives in the application layer so the HTTP client only depends on its
// own port.
type environmentWriter struct {
	secrets *secrets.Repository
}

// ApplyEnvironmentChanges writes to the most specific environment of the
// selected name: the one of the request's collection when it declares it,
// otherwise the global one. Declared secrets go to the keychain; every other
// value goes to the git ignored local overrides, which win over the shared
// file.
func (writer environmentWriter) ApplyEnvironmentChanges(
	workspaceRoot string,
	requestPath string,
	environmentFileName string,
	changes []domain.EnvironmentChange,
) error {
	scope, resolved, err := mostSpecificEnvironment(workspaceRoot, requestPath, environmentFileName)
	if err != nil {
		return err
	}

	secretNames := resolved.SecretNames()
	plain := make([]domain.EnvironmentChange, 0, len(changes))
	for _, change := range changes {
		if !slices.Contains(secretNames, change.Name) || writer.secrets == nil {
			plain = append(plain, change)
			continue
		}
		if err := writer.applySecret(workspaceRoot, scope, environmentFileName, change); err != nil {
			return err
		}
	}

	repository, err := environments.NewScopedRepository(workspaceRoot, scope)
	if err != nil {
		return err
	}
	return repository.ApplyLocalChanges(environmentFileName, plain)
}

func (writer environmentWriter) applySecret(workspaceRoot string, scope string, environmentFileName string, change domain.EnvironmentChange) error {
	if change.Removed {
		return writer.secrets.Delete(workspaceRoot, scope, environmentFileName, change.Name)
	}
	return writer.secrets.Set(workspaceRoot, scope, environmentFileName, change.Name, change.Value)
}

func mostSpecificEnvironment(workspaceRoot string, requestPath string, environmentFileName string) (string, environments.ResolvedEnvironment, error) {
	scopes := []string{""}
	if scope := environments.ScopeOfRequest(requestPath); scope != "" {
		scopes = []string{scope, ""}
	}

	for _, scope := range scopes {
		repository, err := environments.NewScopedRepository(workspaceRoot, scope)
		if err != nil {
			return "", environments.ResolvedEnvironment{}, err
		}
		resolved, err := repository.Load(environmentFileName)
		if errors.Is(err, environments.ErrEnvironmentNotFound) {
			continue
		}
		if err != nil {
			return "", environments.ResolvedEnvironment{}, err
		}
		return scope, resolved, nil
	}
	return "", environments.ResolvedEnvironment{}, environments.ErrEnvironmentNotFound
}

// ResolveEnvironmentName finds an environment visible to the request, in its
// collection or globally, by file name first and then by display name,
// ignoring case.
func (environmentWriter) ResolveEnvironmentName(workspaceRoot string, requestPath string, name string) (string, string, error) {
	scopes := []string{""}
	if scope := environments.ScopeOfRequest(requestPath); scope != "" {
		scopes = append(scopes, scope)
	}

	summaries := make([]environments.Summary, 0)
	for _, scope := range scopes {
		repository, err := environments.NewScopedRepository(workspaceRoot, scope)
		if err != nil {
			return "", "", err
		}
		listed, err := repository.List()
		if err != nil {
			return "", "", err
		}
		summaries = append(summaries, listed...)
	}

	for _, summary := range summaries {
		if summary.FileName == name {
			return summary.FileName, summary.Name, nil
		}
	}
	for _, summary := range summaries {
		if strings.EqualFold(summary.Name, strings.TrimSpace(name)) {
			return summary.FileName, summary.Name, nil
		}
	}
	return "", "", fmt.Errorf("environment %q not found", name)
}
