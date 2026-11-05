package app

import (
	"github.com/clementherve/awake/src/domain"
	"github.com/clementherve/awake/src/features/collections"
	"github.com/clementherve/awake/src/features/environments"
	"github.com/clementherve/awake/src/features/secrets"
	"github.com/clementherve/awake/src/features/workspace"
)

// requestSource adapts the storage features to the HTTP client. It lives in
// the application layer so the HTTP client only depends on its own port.
type requestSource struct {
	secrets *secrets.Repository
}

func (requestSource) LoadRequest(workspaceRoot string, relativePath string) (domain.Request, error) {
	return collections.NewRepository(workspaceRoot).LoadRequest(relativePath)
}

func (source requestSource) LoadVariableLayers(workspaceRoot string, environmentFileName string) ([]map[string]string, error) {
	configuration, err := workspace.NewRepository(workspaceRoot).Load()
	if err != nil {
		return nil, err
	}

	layers := []map[string]string{configuration.Variables}

	if environmentFileName != "" {
		resolved, err := environments.NewRepository(workspaceRoot).Load(environmentFileName)
		if err != nil {
			return nil, err
		}
		layers = append(layers, resolved.Layers()...)

		if source.secrets != nil {
			secretValues, err := source.secrets.Values(workspaceRoot, environmentFileName)
			if err != nil {
				return nil, err
			}
			layers = append(layers, secretValues)
		}
	}

	return layers, nil
}
