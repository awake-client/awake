package app

import (
	"errors"
	"strings"

	"github.com/awake-client/awake/src/domain"
	"github.com/awake-client/awake/src/features/collections"
	"github.com/awake-client/awake/src/features/environments"
	"github.com/awake-client/awake/src/features/secrets"
	"github.com/awake-client/awake/src/features/workspace"
	"github.com/awake-client/awake/src/shared/variables"
)

// requestSource adapts the storage features to the HTTP client. It lives in
// the application layer so the HTTP client only depends on its own port.
type requestSource struct {
	secrets *secrets.Repository
}

// LoadRequest returns a request ready to be sent: its local overrides
// applied and, when it inherits its authentication, the authentication of its
// closest folder or collection in its place, or none when neither defines one.
func (requestSource) LoadRequest(workspaceRoot string, relativePath string) (domain.Request, error) {
	repository := collections.NewRepository(workspaceRoot)
	request, err := repository.LoadRequest(relativePath)
	if err != nil {
		return domain.Request{}, err
	}
	override, err := repository.LoadRequestOverride(relativePath)
	if err != nil {
		return domain.Request{}, err
	}
	request = override.ApplyTo(request)
	if request.Authentication == nil || request.Authentication.Type != domain.AuthenticationInherit {
		return request, nil
	}

	inherited, err := repository.ResolveInheritedAuthentication(relativePath)
	if err != nil {
		return domain.Request{}, err
	}
	request.Authentication = inherited.Authentication
	return request, nil
}

// LoadVariableLayers stacks the variables visible to a request, from the
// weakest to the strongest: the workspace, the variables of its collection,
// the global environment and then the environment of the same name declared
// by the collection, each followed by its local overrides and its secrets.
func (source requestSource) LoadVariableLayers(workspaceRoot string, requestPath string, environmentFileName string) ([]variables.Layer, error) {
	configuration, err := workspace.NewRepository(workspaceRoot).Load()
	if err != nil {
		return nil, err
	}

	layers := []variables.Layer{{Source: "workspace", Values: configuration.Variables}}

	scopes := environments.ScopesOfRequest(requestPath)

	// Default variables of the collection and of the folder of the request,
	// weakest first, before the environment layers refine them.
	for _, scope := range scopes {
		if scope == "" {
			continue
		}
		collection, err := collections.NewRepository(workspaceRoot).LoadCollection(scope)
		if err != nil && !errors.Is(err, collections.ErrEntryNotFound) {
			return nil, err
		}
		if err == nil && len(collection.Variables) > 0 {
			layers = append(layers, variables.Layer{Source: scope, Values: collection.Variables})
		}
	}

	if environmentFileName == "" {
		return layers, nil
	}

	found := false
	for _, scope := range scopes {
		repository, err := environments.NewScopedRepository(workspaceRoot, scope)
		if err != nil {
			return nil, err
		}
		resolved, err := repository.Load(environmentFileName)
		if errors.Is(err, environments.ErrEnvironmentNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		found = true

		label := "environment"
		if scope != "" {
			label = "collection environment"
			if len(strings.Split(scope, "/")) > 2 {
				label = "folder environment"
			}
		}
		layers = append(layers, variables.Layer{Source: label, Values: resolved.Shared.Variables})
		if resolved.Local != nil {
			layers = append(layers, variables.Layer{Source: label + " (local)", Values: resolved.Local.Variables})
		}

		if source.secrets != nil {
			secretValues, err := source.secrets.Values(workspaceRoot, scope, environmentFileName)
			if err != nil {
				return nil, err
			}
			layers = append(layers, variables.Layer{Source: "secret", Values: secretValues, Secret: true})
		}
	}

	if !found {
		return nil, environments.ErrEnvironmentNotFound
	}
	return layers, nil
}
