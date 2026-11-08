package app

import (
	"errors"

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
// collection in its place, or none when the collection defines none.
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

	inherited := &domain.Authentication{Type: domain.AuthenticationNone}
	if scope := environments.ScopeOfRequest(relativePath); scope != "" {
		collection, err := repository.LoadCollection(scope)
		if err != nil && !errors.Is(err, collections.ErrEntryNotFound) {
			return domain.Request{}, err
		}
		if err == nil && collection.Authentication != nil && collection.Authentication.Type != domain.AuthenticationInherit {
			copied := *collection.Authentication
			inherited = &copied
		}
	}
	request.Authentication = inherited
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

	scope := environments.ScopeOfRequest(requestPath)
	if scope != "" {
		collection, err := collections.NewRepository(workspaceRoot).LoadCollection(scope)
		if err != nil && !errors.Is(err, collections.ErrEntryNotFound) {
			return nil, err
		}
		if err == nil && len(collection.Variables) > 0 {
			layers = append(layers, variables.Layer{Source: "collection", Values: collection.Variables})
		}
	}

	if environmentFileName == "" {
		return layers, nil
	}

	scopes := []string{""}
	if scope != "" {
		scopes = append(scopes, scope)
	}

	found := false
	for _, environmentScope := range scopes {
		repository, err := environments.NewScopedRepository(workspaceRoot, environmentScope)
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
		if environmentScope != "" {
			label = "collection environment"
		}
		layers = append(layers, variables.Layer{Source: label, Values: resolved.Shared.Variables})
		if resolved.Local != nil {
			layers = append(layers, variables.Layer{Source: label + " (local)", Values: resolved.Local.Variables})
		}

		if source.secrets != nil {
			secretValues, err := source.secrets.Values(workspaceRoot, environmentScope, environmentFileName)
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
