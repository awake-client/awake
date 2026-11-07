package app

import (
	"errors"
	"os"
	"strings"

	"github.com/clementherve/awake/src/domain"
	"github.com/clementherve/awake/src/features/collections"
	"github.com/clementherve/awake/src/features/environments"
	"github.com/clementherve/awake/src/features/interop"
)

// interopAdapter connects the interop feature to collection files. It lives in
// the application layer so interop only depends on its own ports.
type interopAdapter struct{}

func (interopAdapter) CreateCollection(workspaceRoot string, relativePath string, collection domain.Collection) error {
	return collections.NewRepository(workspaceRoot).CreateCollection(relativePath, collection)
}

func (interopAdapter) CreateEnvironment(workspaceRoot string, scope string, fileName string, environment interop.Environment) error {
	repository, err := environments.NewScopedRepository(workspaceRoot, scope)
	if err != nil {
		return err
	}
	if _, err := os.Stat(repository.SharedPath(fileName)); err == nil {
		return os.ErrExist
	}
	return repository.Save(fileName, environments.Environment{
		Name:            environment.Name,
		Variables:       environment.Variables,
		SecretVariables: environment.SecretVariables,
	})
}

func (interopAdapter) CreateRequest(workspaceRoot string, relativePath string, request domain.Request) error {
	return collections.NewRepository(workspaceRoot).CreateRequest(relativePath, request)
}

func (interopAdapter) LoadCollections(workspaceRoot string) ([]interop.CollectionSnapshot, error) {
	repository := collections.NewRepository(workspaceRoot)

	entries, err := repository.List("")
	if err != nil {
		return nil, err
	}

	snapshots := make([]interop.CollectionSnapshot, 0, len(entries))
	for _, entry := range entries {
		if entry.Kind != collections.KindCollection {
			continue
		}

		snapshot := interop.CollectionSnapshot{
			Name:     entry.Name,
			Requests: []interop.RequestSnapshot{},
		}

		metadata, err := repository.LoadCollection(entry.Path)
		if err != nil && !errors.Is(err, collections.ErrEntryNotFound) {
			return nil, err
		}
		if err == nil {
			snapshot.Name = metadata.Name
			snapshot.Documentation = metadata.Documentation
			snapshot.Variables = metadata.Variables
			snapshot.Authentication = metadata.Authentication
		}

		if err := collectRequests(repository, entry.Path, entry.Path, &snapshot.Requests); err != nil {
			return nil, err
		}

		snapshots = append(snapshots, snapshot)
	}

	return snapshots, nil
}

func collectRequests(
	repository *collections.Repository,
	collectionPath string,
	directoryPath string,
	requests *[]interop.RequestSnapshot,
) error {
	entries, err := repository.List(directoryPath)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.Kind == collections.KindRequest {
			request, err := repository.LoadRequest(entry.Path)
			if err != nil {
				return err
			}

			*requests = append(*requests, interop.RequestSnapshot{
				RelativePath: strings.TrimPrefix(entry.Path, collectionPath+"/"),
				Request:      request,
			})
			continue
		}

		if err := collectRequests(repository, collectionPath, entry.Path, requests); err != nil {
			return err
		}
	}

	return nil
}
