package app

import (
	"errors"
	"path"
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

	// Default headers of the collection and folders come first, so the
	// headers of the request override them.
	inheritedHeaders, err := repository.ResolveInheritedHeaders(relativePath)
	if err != nil {
		return domain.Request{}, err
	}
	request.Headers = append(inheritedHeaders, request.Headers...)

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

// ResolveAuthentication returns the authentication used by a request,
// collection or folder, and the path of the entry that defines it. An entry
// set to inherit resolves to its closest folder or collection; a request
// without authentication uses none.
func (requestSource) ResolveAuthentication(workspaceRoot string, entryPath string) (domain.Authentication, string, error) {
	repository := collections.NewRepository(workspaceRoot)
	none := domain.Authentication{Type: domain.AuthenticationNone}

	var own *domain.Authentication
	if isRequestEntry(entryPath) {
		request, err := repository.LoadRequest(entryPath)
		if err != nil {
			return domain.Authentication{}, "", err
		}
		if request.Authentication == nil {
			return none, entryPath, nil
		}
		own = request.Authentication
	} else {
		entryPath = path.Clean(entryPath)
		collection, err := repository.LoadCollection(entryPath)
		if err != nil && !errors.Is(err, collections.ErrEntryNotFound) {
			return domain.Authentication{}, "", err
		}
		own = collection.Authentication
	}

	if own != nil && own.Type != domain.AuthenticationInherit {
		return *own, entryPath, nil
	}

	var inherited collections.InheritedAuthentication
	var err error
	if isRequestEntry(entryPath) {
		inherited, err = repository.ResolveInheritedAuthentication(entryPath)
	} else if len(strings.Split(entryPath, "/")) > 2 {
		inherited, err = repository.ResolveInheritedAuthenticationForScope(entryPath)
	} else {
		return none, entryPath, nil
	}
	if err != nil {
		return domain.Authentication{}, "", err
	}
	if inherited.Authentication == nil || inherited.SourcePath == "" {
		return none, entryPath, nil
	}
	return *inherited.Authentication, inherited.SourcePath, nil
}

// isRequestEntry reports whether a path names a request file rather than the
// directory of a collection or folder.
func isRequestEntry(entryPath string) bool {
	return strings.HasSuffix(entryPath, ".yaml")
}

// LoadVariableLayers stacks the variables visible to a request, or to a
// collection or folder given by its directory path, from the weakest to the
// strongest: the workspace, the variables of its collection, the global
// environment and then the environment of the same name declared by the
// collection, each followed by its local overrides and its secrets.
func (source requestSource) LoadVariableLayers(workspaceRoot string, requestPath string, environmentFileName string) ([]variables.Layer, error) {
	if !isRequestEntry(requestPath) {
		// The scopes of a directory are those of a request placed in it.
		requestPath = path.Join(requestPath, "request.yaml")
	}

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
		if resolved.Private && resolved.Local != nil {
			layers = append(layers, variables.Layer{Source: label + " (private)", Values: resolved.Local.Variables})
		} else {
			layers = append(layers, variables.Layer{Source: label, Values: resolved.Shared.Variables})
			if resolved.Local != nil {
				layers = append(layers, variables.Layer{Source: label + " (local)", Values: resolved.Local.Variables})
			}
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
