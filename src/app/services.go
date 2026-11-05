package app

import (
	"github.com/clementherve/awake/src/features/collections"
	"github.com/clementherve/awake/src/features/environments"
	"github.com/clementherve/awake/src/features/git"
	"github.com/clementherve/awake/src/features/history"
	"github.com/clementherve/awake/src/features/httpclient"
	"github.com/clementherve/awake/src/features/interop"
	"github.com/clementherve/awake/src/features/platform"
	"github.com/clementherve/awake/src/features/secrets"
	"github.com/clementherve/awake/src/features/workspace"
)

// services holds the feature services exposed to the frontend. Dependencies are
// wired here so features never construct each other directly.
type services struct {
	Collections  *collections.Service
	Environments *environments.Service
	Git          *git.Service
	History      *history.Service
	HTTPClient   *httpclient.Service
	Interop      *interop.Service
	Platform     *platform.Service
	Secrets      *secrets.Service
	Workspace    *workspace.Service
}

func newServices() (*services, error) {
	historyService := history.NewService()

	keychain := secrets.NewKeyringKeychain()
	secretRepository := secrets.NewRepository(keychain)

	httpClient, err := httpclient.NewService(requestSource{secrets: secretRepository}, historyService)
	if err != nil {
		return nil, err
	}

	return &services{
		Collections:  collections.NewService(),
		Environments: environments.NewService(),
		Git:          git.NewService(),
		History:      historyService,
		HTTPClient:   httpClient,
		Interop:      interop.NewService(interopAdapter{}, interopAdapter{}),
		Platform:     platform.NewService(),
		Secrets:      secrets.NewServiceWithKeychain(keychain),
		Workspace:    workspace.NewService(),
	}, nil
}
