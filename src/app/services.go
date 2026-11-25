package app

import (
	"net/http"
	"time"

	"github.com/awake-client/awake/src/features/assistant"
	"github.com/awake-client/awake/src/features/collections"
	"github.com/awake-client/awake/src/features/environments"
	"github.com/awake-client/awake/src/features/git"
	"github.com/awake-client/awake/src/features/history"
	"github.com/awake-client/awake/src/features/httpclient"
	"github.com/awake-client/awake/src/features/interop"
	"github.com/awake-client/awake/src/features/mcp"
	"github.com/awake-client/awake/src/features/oauth2"
	"github.com/awake-client/awake/src/features/platform"
	"github.com/awake-client/awake/src/features/plugins"
	"github.com/awake-client/awake/src/features/scripts"
	"github.com/awake-client/awake/src/features/secrets"
	"github.com/awake-client/awake/src/features/updates"
	"github.com/awake-client/awake/src/features/workspace"
)

// pluginFetchTimeout bounds one request a plugin sends with awake.http.fetch.
const pluginFetchTimeout = 30 * time.Second

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
	Plugins      *plugins.Service
	Secrets      *secrets.Service
	Updates      *updates.Service
	Workspace    *workspace.Service
	Assistant    *assistant.Service
	MCPSettings  *mcp.SettingsService
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
	httpclient.SetCollectionWriter(httpClient, collectionWriter{})

	pluginService, pluginHost, err := newPlugins(keychain)
	if err != nil {
		return nil, err
	}
	httpclient.SetPluginHost(httpClient, pluginHost)
	interopService := interop.NewService(interopAdapter{}, interopAdapter{})
	interop.SetPluginFormats(interopService, pluginFormats{host: pluginHost})

	return &services{
		Collections:  collections.NewService(),
		Environments: environments.NewService(),
		Git:          git.NewService(),
		History:      historyService,
		HTTPClient:   httpClient,
		Interop:      interopService,
		OAuth2:       oauth2Service,
		Platform:     platform.NewService(),
		Plugins:      pluginService,
		Secrets:      secrets.NewServiceWithKeychain(keychain),
		Updates:      updates.NewService(),
		Workspace:    workspace.NewService(),
		Assistant:    assistant.NewService(),
		MCPSettings:  mcp.NewSettingsService(),
	}, nil
}

// newPlugins keeps plugin repositories in the user cache directory and
// approvals in the user configuration directory, outside every workspace. The
// host runs the enabled plugins while requests are sent or imported.
func newPlugins(keychain plugins.Keychain) (*plugins.Service, *plugins.Host, error) {
	cacheRoot, err := plugins.DefaultCacheRoot()
	if err != nil {
		return nil, nil, err
	}
	trustPath, err := plugins.DefaultTrustPath()
	if err != nil {
		return nil, nil, err
	}
	cache := plugins.NewCache(cacheRoot, git.BareRepositories{})
	trust := plugins.NewTrustStore(trustPath)
	host := plugins.NewHost(cache, trust, pluginWorkspace{}, keychain, updates.Version, &http.Client{Timeout: pluginFetchTimeout})
	service := plugins.NewService(cache, trust, pluginWorkspace{}, updates.Version)
	plugins.ConnectHost(service, host)
	return service, host, nil
}
