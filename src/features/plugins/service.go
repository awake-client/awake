package plugins

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/awake-client/awake/src/domain"
)

// networkTimeout bounds one operation that may clone or fetch a repository.
const networkTimeout = 2 * time.Minute

var (
	// ErrMarketplaceExists is returned when adding a marketplace the workspace
	// already declares, by name or by repository.
	ErrMarketplaceExists = errors.New("plugins: the workspace already uses this marketplace")
	// ErrMarketplaceNotDeclared is returned for a marketplace the workspace
	// does not declare.
	ErrMarketplaceNotDeclared = errors.New("plugins: the workspace does not use this marketplace")
	// ErrPluginNotListed is returned for a plugin a marketplace does not list.
	ErrPluginNotListed = errors.New("plugins: the marketplace does not list this plugin")
	// ErrPluginNotDeclared is returned for a plugin the workspace does not
	// declare.
	ErrPluginNotDeclared = errors.New("plugins: the workspace does not use this plugin")
	// ErrPluginMismatch is returned when a repository holds another plugin
	// than the one the marketplace or the workspace names.
	ErrPluginMismatch = errors.New("plugins: the repository holds another plugin")
	// ErrIncompatiblePlugin is returned for a plugin that needs a newer Awake.
	ErrIncompatiblePlugin = errors.New("plugins: the plugin needs a newer Awake")
	// ErrDeclarationChanged is returned when awake.yaml moved the plugin to
	// another commit than the one the user reviewed.
	ErrDeclarationChanged = errors.New("plugins: the workspace now declares another commit of this plugin")
	// ErrIncompleteSource is returned for a source naming neither a
	// marketplace plugin nor a repository.
	ErrIncompleteSource = errors.New("plugins: name a marketplace and a plugin, or a repository")
	// ErrInvalidSetting is returned for a setting the manifest does not
	// declare, a secret setting, or a value it does not accept.
	ErrInvalidSetting = errors.New("plugins: invalid setting")
)

// WorkspaceConfiguration reads and writes the plugin section of awake.yaml.
// The workspace feature owns the file; src/app adapts it.
type WorkspaceConfiguration interface {
	LoadPlugins(workspaceRoot string) (WorkspacePlugins, error)
	SavePlugins(workspaceRoot string, marketplaces []domain.MarketplaceReference, plugins []domain.PluginDeclaration) error
}

// WorkspacePlugins is the plugin section of awake.yaml with the identifier of
// the workspace, which keys the approvals of this machine.
type WorkspacePlugins struct {
	WorkspaceID  string
	Marketplaces []domain.MarketplaceReference
	Plugins      []domain.PluginDeclaration
}

// PluginState tells the interface what a declared plugin needs before it runs.
type PluginState string

const (
	// PluginNotDownloaded means the declared commit is not on this machine.
	PluginNotDownloaded PluginState = "notDownloaded"
	// PluginNeedsApproval means the user has not approved the declared commit
	// or a permission it asks for.
	PluginNeedsApproval PluginState = "needsApproval"
	// PluginApproved means the plugin runs in this workspace.
	PluginApproved PluginState = "approved"
	// PluginIncompatible means the plugin needs a newer Awake.
	PluginIncompatible PluginState = "incompatible"
	// PluginInvalid means the declared commit holds no valid plugin.
	PluginInvalid PluginState = "invalid"
)

// PluginView is a declared plugin as the interface lists it.
type PluginView struct {
	Declaration domain.PluginDeclaration `json:"declaration"`
	State       PluginState              `json:"state"`
	// Problem explains an invalid or incompatible plugin.
	Problem string `json:"problem,omitempty"`
	// Manifest is missing until the commit is downloaded.
	Manifest *Manifest `json:"manifest,omitempty"`
}

// MarketplaceView is a declared marketplace with the plugins it lists.
type MarketplaceView struct {
	Name        string         `json:"name"`
	Repository  string         `json:"repository"`
	Description string         `json:"description,omitempty"`
	Plugins     []CatalogEntry `json:"plugins"`
	// Problem explains why the catalog could not be read, for instance
	// because it was never downloaded on this machine.
	Problem string `json:"problem,omitempty"`
}

// CatalogEntry is a plugin of a marketplace.
type CatalogEntry struct {
	ID          string   `json:"id"`
	Repository  string   `json:"repository"`
	Ref         string   `json:"ref,omitempty"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Installed   bool     `json:"installed"`
}

// PluginSource names a plugin to preview or install: a plugin of a declared
// marketplace, or a repository and a ref.
type PluginSource struct {
	Marketplace string `json:"marketplace,omitempty"`
	PluginID    string `json:"pluginId,omitempty"`
	Repository  string `json:"repository,omitempty"`
	Ref         string `json:"ref,omitempty"`
}

// PluginPreview is what the user reviews before installing, updating or
// approving a plugin: the exact commit and the permissions it asks for.
type PluginPreview struct {
	Source     PluginSource `json:"source"`
	Repository string       `json:"repository"`
	Ref        string       `json:"ref,omitempty"`
	Commit     string       `json:"commit"`
	Manifest   Manifest     `json:"manifest"`
	// InstalledCommit is the commit the workspace declares today, empty for a
	// new plugin.
	InstalledCommit string `json:"installedCommit,omitempty"`
}

// Service exposes plugin management to the frontend. Every method takes the
// root of the workspace, which the backend only knows by its files.
type Service struct {
	cache        *Cache
	trust        *TrustStore
	workspace    WorkspaceConfiguration
	awakeVersion string
}

// NewService returns a plugin service. awakeVersion is checked against the
// engine each manifest asks for.
func NewService(cache *Cache, trust *TrustStore, workspace WorkspaceConfiguration, awakeVersion string) *Service {
	return &Service{cache: cache, trust: trust, workspace: workspace, awakeVersion: awakeVersion}
}

// ListMarketplaces returns the declared marketplaces with the catalog last
// downloaded, without touching the network.
func (service *Service) ListMarketplaces(workspaceRoot string) ([]MarketplaceView, error) {
	operationContext, cancel := context.WithTimeout(context.Background(), networkTimeout)
	defer cancel()

	declared, err := service.workspace.LoadPlugins(workspaceRoot)
	if err != nil {
		return nil, err
	}
	views := make([]MarketplaceView, 0, len(declared.Marketplaces))
	for _, reference := range declared.Marketplaces {
		views = append(views, service.marketplaceView(operationContext, reference, declared.Plugins))
	}
	return views, nil
}

// AddMarketplace downloads the marketplace at repository and adds it to the
// workspace under the name its index declares.
func (service *Service) AddMarketplace(workspaceRoot string, repository string) (MarketplaceView, error) {
	operationContext, cancel := context.WithTimeout(context.Background(), networkTimeout)
	defer cancel()

	declared, err := service.workspace.LoadPlugins(workspaceRoot)
	if err != nil {
		return MarketplaceView{}, err
	}
	marketplace, err := service.downloadMarketplace(operationContext, repository)
	if err != nil {
		return MarketplaceView{}, err
	}
	for _, existing := range declared.Marketplaces {
		if existing.Name == marketplace.Name || existing.Repository == repository {
			return MarketplaceView{}, fmt.Errorf("%w: %s", ErrMarketplaceExists, existing.Name)
		}
	}

	reference := domain.MarketplaceReference{Name: marketplace.Name, Repository: repository}
	marketplaces := append(slices.Clone(declared.Marketplaces), reference)
	if err := service.workspace.SavePlugins(workspaceRoot, marketplaces, declared.Plugins); err != nil {
		return MarketplaceView{}, err
	}
	return catalogView(reference, marketplace, declared.Plugins), nil
}

// RemoveMarketplace removes a marketplace from the workspace. Plugins
// installed from it keep running from their repository.
func (service *Service) RemoveMarketplace(workspaceRoot string, name string) error {
	declared, err := service.workspace.LoadPlugins(workspaceRoot)
	if err != nil {
		return err
	}
	remaining := slices.DeleteFunc(slices.Clone(declared.Marketplaces), func(reference domain.MarketplaceReference) bool {
		return reference.Name == name
	})
	if len(remaining) == len(declared.Marketplaces) {
		return fmt.Errorf("%w: %s", ErrMarketplaceNotDeclared, name)
	}
	return service.workspace.SavePlugins(workspaceRoot, remaining, declared.Plugins)
}

// RefreshMarketplaces fetches every declared marketplace and returns their
// catalogs. A marketplace that cannot be fetched keeps its last catalog and
// reports the problem.
func (service *Service) RefreshMarketplaces(workspaceRoot string) ([]MarketplaceView, error) {
	operationContext, cancel := context.WithTimeout(context.Background(), networkTimeout)
	defer cancel()

	declared, err := service.workspace.LoadPlugins(workspaceRoot)
	if err != nil {
		return nil, err
	}
	views := make([]MarketplaceView, 0, len(declared.Marketplaces))
	for _, reference := range declared.Marketplaces {
		_, fetchError := service.downloadMarketplace(operationContext, reference.Repository)
		view := service.marketplaceView(operationContext, reference, declared.Plugins)
		if fetchError != nil {
			view.Problem = fetchError.Error()
		}
		views = append(views, view)
	}
	return views, nil
}

// ListPlugins returns the declared plugins and what each needs before it
// runs, without touching the network.
func (service *Service) ListPlugins(workspaceRoot string) ([]PluginView, error) {
	declared, err := service.workspace.LoadPlugins(workspaceRoot)
	if err != nil {
		return nil, err
	}
	views := make([]PluginView, 0, len(declared.Plugins))
	for _, declaration := range declared.Plugins {
		view, err := service.pluginView(declared.WorkspaceID, declaration)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

// Preview downloads the commit a source points at today and returns what the
// user reviews before installing it.
func (service *Service) Preview(workspaceRoot string, source PluginSource) (PluginPreview, error) {
	operationContext, cancel := context.WithTimeout(context.Background(), networkTimeout)
	defer cancel()

	declared, err := service.workspace.LoadPlugins(workspaceRoot)
	if err != nil {
		return PluginPreview{}, err
	}
	return service.preview(operationContext, declared, source)
}

// Install adds the reviewed commit of a plugin to the workspace, or moves an
// installed plugin to it, and approves it on this machine: the user installing
// it has just reviewed its permissions.
func (service *Service) Install(workspaceRoot string, source PluginSource, commit string) (PluginView, error) {
	operationContext, cancel := context.WithTimeout(context.Background(), networkTimeout)
	defer cancel()

	declared, err := service.workspace.LoadPlugins(workspaceRoot)
	if err != nil {
		return PluginView{}, err
	}
	repository, ref, err := service.sourceRepository(operationContext, declared, source)
	if err != nil {
		return PluginView{}, err
	}
	manifest, err := service.downloadCommit(operationContext, repository, commit, source.PluginID)
	if err != nil {
		return PluginView{}, err
	}

	declaration := domain.PluginDeclaration{
		ID:          manifest.ID,
		Marketplace: source.Marketplace,
		Repository:  repository,
		Ref:         ref,
		Commit:      commit,
	}
	plugins := slices.Clone(declared.Plugins)
	if position := declarationIndex(plugins, manifest.ID); position >= 0 {
		declaration.Settings = keptSettings(plugins[position].Settings, manifest)
		plugins[position] = declaration
	} else {
		// A setting left out takes its default from the manifest, so a new
		// default reaches the team on update.
		plugins = append(plugins, declaration)
	}

	if err := service.workspace.SavePlugins(workspaceRoot, declared.Marketplaces, plugins); err != nil {
		return PluginView{}, err
	}
	if err := service.approve(declared.WorkspaceID, declaration, manifest); err != nil {
		return PluginView{}, err
	}
	return service.pluginView(declared.WorkspaceID, declaration)
}

// CheckUpdate looks for a newer commit of an installed plugin, at the ref its
// marketplace lists today or at its own ref. It returns nil when the plugin is
// up to date.
func (service *Service) CheckUpdate(workspaceRoot string, pluginID string) (*PluginPreview, error) {
	operationContext, cancel := context.WithTimeout(context.Background(), networkTimeout)
	defer cancel()

	declared, err := service.workspace.LoadPlugins(workspaceRoot)
	if err != nil {
		return nil, err
	}
	declaration, err := findDeclaration(declared.Plugins, pluginID)
	if err != nil {
		return nil, err
	}

	source := PluginSource{PluginID: declaration.ID, Repository: declaration.Repository, Ref: declaration.Ref}
	if marketplace, found := findMarketplace(declared.Marketplaces, declaration.Marketplace); found {
		// The marketplace may have moved the plugin to a new tag.
		if _, err := service.downloadMarketplace(operationContext, marketplace.Repository); err != nil {
			return nil, err
		}
		source = PluginSource{Marketplace: marketplace.Name, PluginID: declaration.ID}
	}

	preview, err := service.preview(operationContext, declared, source)
	if err != nil {
		return nil, err
	}
	if preview.Commit == declaration.Commit {
		return nil, nil
	}
	return &preview, nil
}

// Download fetches the commit the workspace declares for a plugin, typically
// one a teammate added, so the user can review it before approving it.
func (service *Service) Download(workspaceRoot string, pluginID string) (PluginView, error) {
	operationContext, cancel := context.WithTimeout(context.Background(), networkTimeout)
	defer cancel()

	declared, err := service.workspace.LoadPlugins(workspaceRoot)
	if err != nil {
		return PluginView{}, err
	}
	declaration, err := findDeclaration(declared.Plugins, pluginID)
	if err != nil {
		return PluginView{}, err
	}
	if _, err := service.cache.Checkout(operationContext, declaration.Repository, declaration.Commit); err != nil {
		return PluginView{}, err
	}
	return service.pluginView(declared.WorkspaceID, declaration)
}

// Approve lets the declared commit of a plugin run on this machine. commit is
// the one the user reviewed: if awake.yaml changed meanwhile, for instance
// after a pull, nothing is approved.
func (service *Service) Approve(workspaceRoot string, pluginID string, commit string) (PluginView, error) {
	operationContext, cancel := context.WithTimeout(context.Background(), networkTimeout)
	defer cancel()

	declared, err := service.workspace.LoadPlugins(workspaceRoot)
	if err != nil {
		return PluginView{}, err
	}
	declaration, err := findDeclaration(declared.Plugins, pluginID)
	if err != nil {
		return PluginView{}, err
	}
	if declaration.Commit != commit {
		return PluginView{}, fmt.Errorf("%w: %s", ErrDeclarationChanged, declaration.Commit)
	}
	manifest, err := service.downloadCommit(operationContext, declaration.Repository, declaration.Commit, declaration.ID)
	if err != nil {
		return PluginView{}, err
	}
	if err := service.approve(declared.WorkspaceID, declaration, manifest); err != nil {
		return PluginView{}, err
	}
	return service.pluginView(declared.WorkspaceID, declaration)
}

// Revoke stops a plugin from running on this machine. It stays declared for
// the rest of the team.
func (service *Service) Revoke(workspaceRoot string, pluginID string) error {
	declared, err := service.workspace.LoadPlugins(workspaceRoot)
	if err != nil {
		return err
	}
	return service.trust.Revoke(declared.WorkspaceID, pluginID)
}

// Uninstall removes a plugin from the workspace and forgets its approval.
func (service *Service) Uninstall(workspaceRoot string, pluginID string) error {
	declared, err := service.workspace.LoadPlugins(workspaceRoot)
	if err != nil {
		return err
	}
	if _, err := findDeclaration(declared.Plugins, pluginID); err != nil {
		return err
	}
	plugins := slices.DeleteFunc(slices.Clone(declared.Plugins), func(declaration domain.PluginDeclaration) bool {
		return declaration.ID == pluginID
	})
	if err := service.workspace.SavePlugins(workspaceRoot, declared.Marketplaces, plugins); err != nil {
		return err
	}
	return service.trust.Revoke(declared.WorkspaceID, pluginID)
}

// UpdateSettings replaces the shared settings of a plugin. Every key must be
// a non secret setting of its manifest.
func (service *Service) UpdateSettings(workspaceRoot string, pluginID string, settings map[string]string) (PluginView, error) {
	declared, err := service.workspace.LoadPlugins(workspaceRoot)
	if err != nil {
		return PluginView{}, err
	}
	position := declarationIndex(declared.Plugins, pluginID)
	if position < 0 {
		return PluginView{}, fmt.Errorf("%w: %s", ErrPluginNotDeclared, pluginID)
	}
	declaration := declared.Plugins[position]
	checkout, found := service.cache.CachedCheckout(declaration.Repository, declaration.Commit)
	if !found {
		return PluginView{}, fmt.Errorf("%w: %s", ErrNotCached, declaration.Repository)
	}
	manifest, err := ReadManifest(checkout)
	if err != nil {
		return PluginView{}, err
	}
	if err := validateSettingValues(manifest, settings); err != nil {
		return PluginView{}, err
	}

	plugins := slices.Clone(declared.Plugins)
	declaration.Settings = maps.Clone(settings)
	if len(declaration.Settings) == 0 {
		declaration.Settings = nil
	}
	plugins[position] = declaration
	if err := service.workspace.SavePlugins(workspaceRoot, declared.Marketplaces, plugins); err != nil {
		return PluginView{}, err
	}
	return service.pluginView(declared.WorkspaceID, declaration)
}

func (service *Service) preview(operationContext context.Context, declared WorkspacePlugins, source PluginSource) (PluginPreview, error) {
	repository, ref, err := service.sourceRepository(operationContext, declared, source)
	if err != nil {
		return PluginPreview{}, err
	}
	commit, err := service.cache.Resolve(operationContext, repository, ref)
	if err != nil {
		return PluginPreview{}, err
	}
	manifest, err := service.downloadCommit(operationContext, repository, commit, source.PluginID)
	if err != nil {
		return PluginPreview{}, err
	}

	preview := PluginPreview{Source: source, Repository: repository, Ref: ref, Commit: commit, Manifest: manifest}
	if position := declarationIndex(declared.Plugins, manifest.ID); position >= 0 {
		preview.InstalledCommit = declared.Plugins[position].Commit
	}
	return preview, nil
}

// sourceRepository finds the repository and ref a source points at. A
// marketplace plugin is looked up in the catalog last downloaded.
func (service *Service) sourceRepository(operationContext context.Context, declared WorkspacePlugins, source PluginSource) (string, string, error) {
	if source.Marketplace == "" {
		if source.Repository == "" {
			return "", "", ErrIncompleteSource
		}
		if err := ValidateRepositoryURL(source.Repository); err != nil {
			return "", "", err
		}
		return source.Repository, source.Ref, nil
	}
	if source.PluginID == "" {
		return "", "", ErrIncompleteSource
	}

	reference, found := findMarketplace(declared.Marketplaces, source.Marketplace)
	if !found {
		return "", "", fmt.Errorf("%w: %s", ErrMarketplaceNotDeclared, source.Marketplace)
	}
	marketplace, err := service.cachedMarketplace(operationContext, reference.Repository)
	if err != nil {
		return "", "", err
	}
	entry, found := marketplace.Find(source.PluginID)
	if !found {
		return "", "", fmt.Errorf("%w: %s in %s", ErrPluginNotListed, source.PluginID, source.Marketplace)
	}
	return entry.Repository, entry.Ref, nil
}

// downloadCommit extracts commit and checks it holds a plugin this Awake
// runs, with the expected identifier when one is given.
func (service *Service) downloadCommit(operationContext context.Context, repository string, commit string, expectedID string) (Manifest, error) {
	checkout, err := service.cache.Checkout(operationContext, repository, commit)
	if err != nil {
		return Manifest{}, err
	}
	manifest, err := ReadManifest(checkout)
	if err != nil {
		return Manifest{}, err
	}
	if expectedID != "" && manifest.ID != expectedID {
		return Manifest{}, fmt.Errorf("%w: expected %s, found %s", ErrPluginMismatch, expectedID, manifest.ID)
	}
	if !manifest.SupportsAwake(service.awakeVersion) {
		return Manifest{}, fmt.Errorf("%w: %s asks for Awake %s", ErrIncompatiblePlugin, manifest.ID, manifest.Engine)
	}
	return manifest, nil
}

func (service *Service) approve(workspaceID string, declaration domain.PluginDeclaration, manifest Manifest) error {
	return service.trust.Approve(Approval{
		WorkspaceID: workspaceID,
		PluginID:    declaration.ID,
		Repository:  declaration.Repository,
		Commit:      declaration.Commit,
		Permissions: manifest.Permissions,
	})
}

func (service *Service) pluginView(workspaceID string, declaration domain.PluginDeclaration) (PluginView, error) {
	view := PluginView{Declaration: declaration, State: PluginNotDownloaded}
	checkout, found := service.cache.CachedCheckout(declaration.Repository, declaration.Commit)
	if !found {
		return view, nil
	}

	manifest, err := ReadManifest(checkout)
	switch {
	case err != nil:
		view.State, view.Problem = PluginInvalid, err.Error()
		return view, nil
	case manifest.ID != declaration.ID:
		view.State = PluginInvalid
		view.Problem = fmt.Sprintf("%v: expected %s, found %s", ErrPluginMismatch, declaration.ID, manifest.ID)
		return view, nil
	}
	view.Manifest = &manifest
	if !manifest.SupportsAwake(service.awakeVersion) {
		view.State = PluginIncompatible
		view.Problem = fmt.Sprintf("needs Awake %s", manifest.Engine)
		return view, nil
	}

	trusted, err := service.trust.IsTrusted(workspaceID, declaration.ID, declaration.Commit, manifest.Permissions)
	if err != nil {
		return PluginView{}, err
	}
	view.State = PluginNeedsApproval
	if trusted {
		view.State = PluginApproved
	}
	return view, nil
}

func (service *Service) marketplaceView(operationContext context.Context, reference domain.MarketplaceReference, plugins []domain.PluginDeclaration) MarketplaceView {
	marketplace, err := service.cachedMarketplace(operationContext, reference.Repository)
	if err != nil {
		return MarketplaceView{Name: reference.Name, Repository: reference.Repository, Plugins: []CatalogEntry{}, Problem: err.Error()}
	}
	return catalogView(reference, marketplace, plugins)
}

// downloadMarketplace fetches the marketplace and reads the index of its
// default branch.
func (service *Service) downloadMarketplace(operationContext context.Context, repository string) (Marketplace, error) {
	commit, err := service.cache.Resolve(operationContext, repository, "")
	if err != nil {
		return Marketplace{}, err
	}
	return service.readMarketplace(operationContext, repository, commit)
}

func (service *Service) cachedMarketplace(operationContext context.Context, repository string) (Marketplace, error) {
	commit, err := service.cache.ResolveCached(operationContext, repository, "")
	if err != nil {
		return Marketplace{}, err
	}
	return service.readMarketplace(operationContext, repository, commit)
}

func (service *Service) readMarketplace(operationContext context.Context, repository string, commit string) (Marketplace, error) {
	checkout, err := service.cache.Checkout(operationContext, repository, commit)
	if err != nil {
		return Marketplace{}, err
	}
	content, err := ReadFile(checkout, MarketplaceFileName)
	if err != nil {
		return Marketplace{}, err
	}
	return ParseMarketplace(content)
}

func catalogView(reference domain.MarketplaceReference, marketplace Marketplace, plugins []domain.PluginDeclaration) MarketplaceView {
	view := MarketplaceView{
		Name:        reference.Name,
		Repository:  reference.Repository,
		Description: marketplace.Description,
		Plugins:     make([]CatalogEntry, 0, len(marketplace.Plugins)),
	}
	for _, entry := range marketplace.Plugins {
		view.Plugins = append(view.Plugins, CatalogEntry{
			ID:          entry.ID,
			Repository:  entry.Repository,
			Ref:         entry.Ref,
			Description: entry.Description,
			Tags:        entry.Tags,
			Installed:   declarationIndex(plugins, entry.ID) >= 0,
		})
	}
	return view
}

// validateSettingValues accepts the values of declared, non secret settings.
// Secret settings are kept in the keychain of each user, never in awake.yaml.
func validateSettingValues(manifest Manifest, values map[string]string) error {
	for key, value := range values {
		position := slices.IndexFunc(manifest.Settings, func(setting Setting) bool { return setting.Key == key })
		if position < 0 {
			return fmt.Errorf("%w: %s declares no setting %q", ErrInvalidSetting, manifest.ID, key)
		}
		setting := manifest.Settings[position]
		switch setting.Type {
		case SettingSecret:
			return fmt.Errorf("%w: %q is secret and is not stored in awake.yaml", ErrInvalidSetting, key)
		case SettingBoolean:
			if value != "true" && value != "false" {
				return fmt.Errorf("%w: %q must be true or false", ErrInvalidSetting, key)
			}
		case SettingSelect:
			if !slices.Contains(setting.Options, value) {
				return fmt.Errorf("%w: %q must be one of %v", ErrInvalidSetting, key, setting.Options)
			}
		}
	}
	return nil
}

// keptSettings keeps the shared values of an updated plugin that its new
// manifest still accepts.
func keptSettings(values map[string]string, manifest Manifest) map[string]string {
	kept := map[string]string{}
	for key, value := range values {
		if validateSettingValues(manifest, map[string]string{key: value}) == nil {
			kept[key] = value
		}
	}
	if len(kept) == 0 {
		return nil
	}
	return kept
}

func findDeclaration(plugins []domain.PluginDeclaration, pluginID string) (domain.PluginDeclaration, error) {
	position := declarationIndex(plugins, pluginID)
	if position < 0 {
		return domain.PluginDeclaration{}, fmt.Errorf("%w: %s", ErrPluginNotDeclared, pluginID)
	}
	return plugins[position], nil
}

func declarationIndex(plugins []domain.PluginDeclaration, pluginID string) int {
	return slices.IndexFunc(plugins, func(declaration domain.PluginDeclaration) bool {
		return declaration.ID == pluginID
	})
}

func findMarketplace(marketplaces []domain.MarketplaceReference, name string) (domain.MarketplaceReference, bool) {
	position := slices.IndexFunc(marketplaces, func(reference domain.MarketplaceReference) bool {
		return reference.Name == name
	})
	if position < 0 {
		return domain.MarketplaceReference{}, false
	}
	return marketplaces[position], true
}
