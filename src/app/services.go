package app

import (
	"github.com/awake-client/awake/src/features/assistant"
	"github.com/awake-client/awake/src/features/collections"
	"github.com/awake-client/awake/src/features/environments"
	"github.com/awake-client/awake/src/features/git"
	"github.com/awake-client/awake/src/features/history"
	"github.com/awake-client/awake/src/features/httpclient"
	"github.com/awake-client/awake/src/features/interop"
	"github.com/awake-client/awake/src/features/oauth2"
	"github.com/awake-client/awake/src/features/platform"
	"github.com/awake-client/awake/src/features/scripts"
	"github.com/awake-client/awake/src/features/secrets"
	"github.com/awake-client/awake/src/features/updates"
	"github.com/awake-client/awake/src/features/workspace"
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
	OAuth2       *oauth2.Service
	Platform     *platform.Service
	Secrets      *secrets.Service
	Updates      *updates.Service
	Workspace    *workspace.Service
	Assistant    *assistant.Service
}

func newServices() (*services, error) {
	historyService := history.NewService()

	keychain := secrets.NewKeyringKeychain()
	secretRepository := secrets.NewRepository(keychain)
	source := requestSource{secrets: secretRepository}

	httpClient, err := httpclient.NewService(source, historyService)
	if err != nil {
		return nil, err
	}

	oauth2Service := oauth2.NewService(source)
	httpclient.SetOAuth2Provider(httpClient, oauth2.NewProvider(oauth2Service))
	httpclient.SetScriptRunner(httpClient, scripts.NewRunner())
	httpclient.SetEnvironmentWriter(httpClient, environmentWriter{secrets: secretRepository})

	return &services{
		Collections:  collections.NewService(),
		Environments: environments.NewService(),
		Git:          git.NewService(),
		History:      historyService,
		HTTPClient:   httpClient,
		Interop:      interop.NewService(interopAdapter{}, interopAdapter{}),
		OAuth2:       oauth2Service,
		Platform:     platform.NewService(),
		Secrets:      secrets.NewServiceWithKeychain(keychain),
		Updates:      updates.NewService(),
		Workspace:    workspace.NewService(),
		Assistant:    assistant.NewService(),
	}, nil
}
