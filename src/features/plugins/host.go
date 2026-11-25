package plugins

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"sync"

	"github.com/awake-client/awake/src/domain"
)

// keychainServicePrefix names the keychain entries holding the secret
// settings of the plugins of one workspace.
const keychainServicePrefix = "awake-plugins:"

var (
	// ErrPluginNotEnabled is returned when a request uses a plugin that is not
	// approved, or did not load, on this computer.
	ErrPluginNotEnabled = errors.New("plugins: the plugin is not enabled on this computer")
	// ErrUnknownContribution is returned for a contribution no enabled plugin
	// provides.
	ErrUnknownContribution = errors.New("plugins: no enabled plugin provides this")
)

// Keychain stores the secret settings of plugins. The secrets feature
// provides the operating system keychain.
type Keychain interface {
	Get(service string, account string) (string, bool, error)
	Set(service string, account string, value string) error
	Delete(service string, account string) error
}

// FormatInfo describes a document format an enabled plugin imports or
// exports, under the name plugin:<plugin id>/<format>.
type FormatInfo struct {
	Format     string
	PluginID   string
	Label      string
	Extensions []string
}

// Host runs the enabled plugins of each workspace. A plugin is loaded on
// first use and kept until awake.yaml, the approvals or its secret settings
// change, so sending a request does not evaluate bundles again.
type Host struct {
	cache        *Cache
	trust        *TrustStore
	workspace    WorkspaceConfiguration
	keychain     Keychain
	awakeVersion string
	httpClient   *http.Client

	mutex      sync.Mutex
	workspaces map[string]*loadedWorkspace
	generation int
}

type loadedWorkspace struct {
	fingerprint string
	plugins     []*loadedPlugin
	// problems explains, by plugin identifier, why an approved plugin did
	// not load.
	problems map[string]string
}

type loadedPlugin struct {
	declaration domain.PluginDeclaration
	runtime     *Runtime
}

// NewHost returns a host. httpClient sends the requests of awake.http.fetch.
func NewHost(cache *Cache, trust *TrustStore, workspace WorkspaceConfiguration, keychain Keychain, awakeVersion string, httpClient *http.Client) *Host {
	return &Host{
		cache:        cache,
		trust:        trust,
		workspace:    workspace,
		keychain:     keychain,
		awakeVersion: awakeVersion,
		httpClient:   httpClient,
		workspaces:   map[string]*loadedWorkspace{},
	}
}

// BeforeRequest runs the beforeRequest hook of every enabled plugin, in the
// order of awake.yaml. A plugin without the secrets permission does not see
// the variables named in secretNames. A failing hook is reported in the logs
// and never stops the request.
func (host *Host) BeforeRequest(workspaceRoot string, request *domain.Request, variables map[string]string, secretNames []string) []string {
	logs := []string{}
	for _, plugin := range host.pluginsWithHook(workspaceRoot, HookBeforeRequest) {
		visible := visibleVariables(plugin.runtime.Manifest(), variables, secretNames)
		before := maps.Clone(visible)
		log, err := plugin.runtime.BeforeRequest(request, visible)
		logs = append(logs, prefixedLogs(plugin.declaration.ID, log, err)...)
		applyVariableChanges(variables, before, visible)
	}
	return logs
}

// AfterResponse runs the afterResponse hook of every enabled plugin.
func (host *Host) AfterResponse(workspaceRoot string, request domain.Request, response domain.ScriptResponse, variables map[string]string, secretNames []string) []string {
	logs := []string{}
	for _, plugin := range host.pluginsWithHook(workspaceRoot, HookAfterResponse) {
		visible := visibleVariables(plugin.runtime.Manifest(), variables, secretNames)
		before := maps.Clone(visible)
		log, err := plugin.runtime.AfterResponse(request, response, visible)
		logs = append(logs, prefixedLogs(plugin.declaration.ID, log, err)...)
		applyVariableChanges(variables, before, visible)
	}
	return logs
}

// DynamicValues computes the {{$name}} placeholders enabled plugins provide.
// Names no plugin provides are left out, so interpolation reports them as
// missing.
func (host *Host) DynamicValues(workspaceRoot string, names []string) (map[string]string, []string, error) {
	values := map[string]string{}
	logs := []string{}
	plugins := host.loaded(workspaceRoot).plugins
	for _, name := range names {
		for _, plugin := range plugins {
			if !slices.ContainsFunc(plugin.runtime.Manifest().Contributes.DynamicVariables, func(variable DynamicVariableContribution) bool {
				return variable.Name == name
			}) {
				continue
			}
			value, log, err := plugin.runtime.DynamicVariable(name)
			logs = append(logs, prefixedLogs(plugin.declaration.ID, log, nil)...)
			if err != nil {
				return nil, logs, fmt.Errorf("%s: %w", name, err)
			}
			values[name] = value
			break
		}
	}
	return values, logs, nil
}

// DynamicVariables lists the placeholders the enabled plugins provide, for
// autocompletion.
func (host *Host) DynamicVariables(workspaceRoot string) []DynamicVariableContribution {
	result := []DynamicVariableContribution{}
	for _, plugin := range host.loaded(workspaceRoot).plugins {
		result = append(result, plugin.runtime.Manifest().Contributes.DynamicVariables...)
	}
	return result
}

// Authenticate applies the authentication type a plugin provides to a request
// about to be sent. Unlike a hook, a failure stops the request: sending it
// unsigned would only hide the problem.
func (host *Host) Authenticate(workspaceRoot string, pluginID string, authenticationType string, httpRequest *http.Request, fields map[string]string) ([]string, error) {
	plugin, err := host.enabledPlugin(workspaceRoot, pluginID)
	if err != nil {
		return nil, err
	}
	if !slices.ContainsFunc(plugin.runtime.Manifest().Contributes.Authentication, func(authentication AuthenticationContribution) bool {
		return authentication.Type == authenticationType
	}) {
		return nil, fmt.Errorf("%w: authentication %s/%s", ErrUnknownContribution, pluginID, authenticationType)
	}
	log, err := plugin.runtime.Authenticate(authenticationType, httpRequest, fields)
	return prefixedLogs(pluginID, log, nil), err
}

// Formats lists the import and export formats of the enabled plugins.
func (host *Host) Formats(workspaceRoot string) (imports []FormatInfo, exports []FormatInfo) {
	imports, exports = []FormatInfo{}, []FormatInfo{}
	for _, plugin := range host.loaded(workspaceRoot).plugins {
		contributions := plugin.runtime.Manifest().Contributes
		for _, format := range contributions.ImportFormats {
			imports = append(imports, formatInfo(plugin.declaration.ID, format))
		}
		for _, format := range contributions.ExportFormats {
			exports = append(exports, formatInfo(plugin.declaration.ID, format))
		}
	}
	return imports, exports
}

// Import converts a document with the import format of a plugin.
func (host *Host) Import(workspaceRoot string, pluginID string, format string, fileName string, content []byte) ([]byte, error) {
	plugin, err := host.enabledPlugin(workspaceRoot, pluginID)
	if err != nil {
		return nil, err
	}
	if !slices.ContainsFunc(plugin.runtime.Manifest().Contributes.ImportFormats, func(contribution FormatContribution) bool {
		return contribution.Format == format
	}) {
		return nil, fmt.Errorf("%w: import format %s/%s", ErrUnknownContribution, pluginID, format)
	}
	collections, _, err := plugin.runtime.Import(format, fileName, string(content))
	return collections, err
}

// Export converts collections with the export format of a plugin.
func (host *Host) Export(workspaceRoot string, pluginID string, format string, collections []byte) (string, error) {
	plugin, err := host.enabledPlugin(workspaceRoot, pluginID)
	if err != nil {
		return "", err
	}
	if !slices.ContainsFunc(plugin.runtime.Manifest().Contributes.ExportFormats, func(contribution FormatContribution) bool {
		return contribution.Format == format
	}) {
		return "", fmt.Errorf("%w: export format %s/%s", ErrUnknownContribution, pluginID, format)
	}
	content, _, err := plugin.runtime.Export(format, collections)
	return content, err
}

// RunCommand runs a command of an enabled plugin and returns what it logged.
func (host *Host) RunCommand(workspaceRoot string, pluginID string, commandID string) ([]string, error) {
	plugin, err := host.enabledPlugin(workspaceRoot, pluginID)
	if err != nil {
		return nil, err
	}
	if !slices.ContainsFunc(plugin.runtime.Manifest().Contributes.Commands, func(command CommandContribution) bool {
		return command.ID == commandID
	}) {
		return nil, fmt.Errorf("%w: command %s/%s", ErrUnknownContribution, pluginID, commandID)
	}
	log, err := plugin.runtime.RunCommand(commandID)
	return log.Lines, err
}

// LoadProblem explains why an approved plugin did not load, or returns an
// empty string.
func (host *Host) LoadProblem(workspaceRoot string, pluginID string) string {
	return host.loaded(workspaceRoot).problems[pluginID]
}

// Invalidate drops the loaded plugins of every workspace, so the next use
// loads them again, for instance after a secret setting changed.
func (host *Host) Invalidate() {
	host.mutex.Lock()
	defer host.mutex.Unlock()
	host.generation++
	clear(host.workspaces)
}

// SecretSettingStates reports which secret settings of a plugin have a value
// on this computer. The values themselves never leave the backend.
func (host *Host) SecretSettingStates(workspaceID string, manifest Manifest) ([]SecretSettingState, error) {
	states := []SecretSettingState{}
	for _, setting := range manifest.Settings {
		if setting.Type != SettingSecret {
			continue
		}
		_, found, err := host.keychain.Get(keychainServicePrefix+workspaceID, secretAccount(manifest.ID, setting.Key))
		if err != nil {
			return nil, err
		}
		states = append(states, SecretSettingState{Key: setting.Key, IsSet: found})
	}
	return states, nil
}

// SetSecretSetting stores a secret setting in the keychain, or removes it
// when value is empty.
func (host *Host) SetSecretSetting(workspaceID string, manifest Manifest, key string, value string) error {
	if !slices.ContainsFunc(manifest.Settings, func(setting Setting) bool {
		return setting.Key == key && setting.Type == SettingSecret
	}) {
		return fmt.Errorf("%w: %s declares no secret setting %q", ErrInvalidSetting, manifest.ID, key)
	}
	service, account := keychainServicePrefix+workspaceID, secretAccount(manifest.ID, key)
	var err error
	if value == "" {
		err = host.keychain.Delete(service, account)
	} else {
		err = host.keychain.Set(service, account, value)
	}
	if err != nil {
		return err
	}
	host.Invalidate()
	return nil
}

// SecretSettingState reports whether a secret setting has a value.
type SecretSettingState struct {
	Key   string `json:"key"`
	IsSet bool   `json:"isSet"`
}

func (host *Host) pluginsWithHook(workspaceRoot string, hook Hook) []*loadedPlugin {
	result := []*loadedPlugin{}
	for _, plugin := range host.loaded(workspaceRoot).plugins {
		if slices.Contains(plugin.runtime.Manifest().Contributes.Hooks, hook) {
			result = append(result, plugin)
		}
	}
	return result
}

func (host *Host) enabledPlugin(workspaceRoot string, pluginID string) (*loadedPlugin, error) {
	loaded := host.loaded(workspaceRoot)
	for _, plugin := range loaded.plugins {
		if plugin.declaration.ID == pluginID {
			return plugin, nil
		}
	}
	if problem := loaded.problems[pluginID]; problem != "" {
		return nil, fmt.Errorf("%w: %s: %s", ErrPluginNotEnabled, pluginID, problem)
	}
	return nil, fmt.Errorf("%w: %s", ErrPluginNotEnabled, pluginID)
}

// loaded returns the enabled plugins of a workspace, loading them again when
// its declarations or approvals changed. A workspace that cannot be read runs
// no plugin.
func (host *Host) loaded(workspaceRoot string) *loadedWorkspace {
	declared, err := host.workspace.LoadPlugins(workspaceRoot)
	if err != nil {
		return &loadedWorkspace{problems: map[string]string{}}
	}
	approvals, err := host.trust.Approvals(declared.WorkspaceID)
	if err != nil {
		return &loadedWorkspace{problems: map[string]string{}}
	}

	host.mutex.Lock()
	defer host.mutex.Unlock()
	fingerprint := workspaceFingerprint(declared, approvals, host.generation)
	if current, found := host.workspaces[workspaceRoot]; found && current.fingerprint == fingerprint {
		return current
	}

	loaded := &loadedWorkspace{fingerprint: fingerprint, problems: map[string]string{}}
	for _, declaration := range declared.Plugins {
		runtime, problem := host.load(declared.WorkspaceID, declaration)
		switch {
		case problem != "":
			loaded.problems[declaration.ID] = problem
		case runtime != nil:
			loaded.plugins = append(loaded.plugins, &loadedPlugin{declaration: declaration, runtime: runtime})
		}
	}
	host.workspaces[workspaceRoot] = loaded
	return loaded
}

// load returns the runtime of an approved plugin, nothing for a plugin the
// user has not approved, or the reason an approved plugin cannot run.
func (host *Host) load(workspaceID string, declaration domain.PluginDeclaration) (*Runtime, string) {
	checkout, found := host.cache.CachedCheckout(declaration.Repository, declaration.Commit)
	if !found {
		return nil, ""
	}
	manifest, err := ReadManifest(checkout)
	if err != nil || manifest.ID != declaration.ID || !manifest.SupportsAwake(host.awakeVersion) {
		return nil, ""
	}
	trusted, err := host.trust.IsTrusted(workspaceID, declaration.ID, declaration.Commit, manifest.Permissions)
	if err != nil || !trusted {
		return nil, ""
	}

	bundle, err := ReadFile(checkout, manifest.Main)
	if err != nil {
		return nil, err.Error()
	}
	settings, err := host.settings(workspaceID, manifest, declaration)
	if err != nil {
		return nil, err.Error()
	}
	runtime, err := LoadRuntime(manifest, bundle, RuntimeOptions{Settings: settings, HTTPClient: host.httpClient})
	if err != nil {
		return nil, err.Error()
	}
	return runtime, ""
}

// settings merges the defaults of the manifest, the values shared in
// awake.yaml and the secret values of this computer.
func (host *Host) settings(workspaceID string, manifest Manifest, declaration domain.PluginDeclaration) (map[string]string, error) {
	settings := map[string]string{}
	for _, setting := range manifest.Settings {
		switch setting.Type {
		case SettingSecret:
			value, found, err := host.keychain.Get(keychainServicePrefix+workspaceID, secretAccount(manifest.ID, setting.Key))
			if err != nil {
				return nil, fmt.Errorf("plugins: read the secret setting %s: %w", setting.Key, err)
			}
			if found {
				settings[setting.Key] = value
			}
		default:
			settings[setting.Key] = setting.Default
			if value, found := declaration.Settings[setting.Key]; found {
				settings[setting.Key] = value
			}
		}
	}
	return settings, nil
}

func workspaceFingerprint(declared WorkspacePlugins, approvals []Approval, generation int) string {
	encoded, _ := json.Marshal(struct {
		Plugins   []domain.PluginDeclaration
		Approvals []Approval
	}{declared.Plugins, approvals})
	sum := sha256.Sum256(append(encoded, []byte(strconv.Itoa(generation))...))
	return hex.EncodeToString(sum[:])
}

// visibleVariables returns the variables a plugin may read: all of them with
// the secrets permission, the others without it.
func visibleVariables(manifest Manifest, variables map[string]string, secretNames []string) map[string]string {
	visible := maps.Clone(variables)
	if manifest.HasPermission(PermissionSecrets) {
		return visible
	}
	for _, name := range secretNames {
		delete(visible, name)
	}
	return visible
}

// applyVariableChanges copies what a plugin changed in its view of the
// variables back, so hidden secrets are neither revealed nor dropped.
func applyVariableChanges(variables map[string]string, before map[string]string, after map[string]string) {
	for name, value := range after {
		if previous, found := before[name]; !found || previous != value {
			variables[name] = value
		}
	}
	for name := range before {
		if _, found := after[name]; !found {
			delete(variables, name)
		}
	}
}

func prefixedLogs(pluginID string, log CallLog, err error) []string {
	lines := make([]string, 0, len(log.Lines)+1)
	for _, line := range log.Lines {
		lines = append(lines, "["+pluginID+"] "+line)
	}
	if err != nil {
		lines = append(lines, "["+pluginID+"] error: "+err.Error())
	}
	return lines
}

func formatInfo(pluginID string, format FormatContribution) FormatInfo {
	return FormatInfo{
		Format:     "plugin:" + pluginID + "/" + format.Format,
		PluginID:   pluginID,
		Label:      format.Label,
		Extensions: format.Extensions,
	}
}

func secretAccount(pluginID string, key string) string {
	return pluginID + "/" + key
}
