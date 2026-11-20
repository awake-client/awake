package app

import (
	"github.com/awake-client/awake/src/domain"
	"github.com/awake-client/awake/src/features/collections"
)

// collectionWriter saves the variables scripts store with awake.collection.
// It lives in the application layer so the HTTP client only depends on its
// own port.
type collectionWriter struct{}

// ApplyCollectionChanges writes to the variables of the request's collection,
// in the tracked collection.yaml the team shares through git.
func (collectionWriter) ApplyCollectionChanges(workspaceRoot string, requestPath string, changes []domain.CollectionChange) error {
	repository := collections.NewRepository(workspaceRoot)
	collectionPath, err := repository.CollectionOfRequest(requestPath)
	if err != nil {
		return err
	}
	return repository.ApplyVariableChanges(collectionPath, changes)
}
