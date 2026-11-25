package plugins

import (
	"errors"
	"fmt"
	"mime"
	"path"
	"strings"

	"github.com/awake-client/awake/src/domain"
)

// ViewPathPrefix starts the URL of a file of a plugin view, followed by the
// repository key, the commit and the path inside the repository.
const ViewPathPrefix = "/plugin-views/"

// ErrViewNotFound is returned for a view file no enabled plugin serves.
var ErrViewNotFound = errors.New("plugins: no enabled plugin serves this view")

// UIContributions lists what the enabled plugins of a workspace add to the
// interface.
type UIContributions struct {
	Commands         []CommandItem                 `json:"commands"`
	Authentication   []AuthenticationItem          `json:"authentication"`
	DynamicVariables []DynamicVariableContribution `json:"dynamicVariables"`
	ResponseViewers  []ViewItem                    `json:"responseViewers"`
	RequestTabs      []ViewItem                    `json:"requestTabs"`
}

// CommandItem is a command of the command palette.
type CommandItem struct {
	PluginID   string `json:"pluginId"`
	PluginName string `json:"pluginName"`
	ID         string `json:"id"`
	Title      string `json:"title"`
}

// AuthenticationItem is an authentication type a plugin provides, with the
// type a request stores.
type AuthenticationItem struct {
	PluginID string                    `json:"pluginId"`
	Type     domain.AuthenticationType `json:"type"`
	Label    string                    `json:"label"`
	Fields   []Setting                 `json:"fields"`
}

// ViewItem is a response viewer or a request tab, with the URL of its page.
type ViewItem struct {
	PluginID string `json:"pluginId"`
	ID       string `json:"id"`
	Title    string `json:"title"`
	// ContentTypes lists the media types a response viewer renders, such as
	// application/x-protobuf or image/*. It is empty for a request tab.
	ContentTypes []string `json:"contentTypes"`
	URL          string   `json:"url"`
}

// Contributions lists what the enabled plugins of the workspace add to the
// interface.
func (host *Host) Contributions(workspaceRoot string) UIContributions {
	result := UIContributions{
		Commands:         []CommandItem{},
		Authentication:   []AuthenticationItem{},
		DynamicVariables: []DynamicVariableContribution{},
		ResponseViewers:  []ViewItem{},
		RequestTabs:      []ViewItem{},
	}
	for _, plugin := range host.loaded(workspaceRoot).plugins {
		manifest := plugin.runtime.Manifest()
		contributions := manifest.Contributes
		for _, command := range contributions.Commands {
			result.Commands = append(result.Commands, CommandItem{PluginID: manifest.ID, PluginName: manifest.Name, ID: command.ID, Title: command.Title})
		}
		for _, authentication := range contributions.Authentication {
			fields := authentication.Fields
			if fields == nil {
				fields = []Setting{}
			}
			result.Authentication = append(result.Authentication, AuthenticationItem{
				PluginID: manifest.ID,
				Type:     domain.PluginAuthenticationType(manifest.ID, authentication.Type),
				Label:    authentication.Label,
				Fields:   fields,
			})
		}
		result.DynamicVariables = append(result.DynamicVariables, contributions.DynamicVariables...)
		for _, viewer := range contributions.ResponseViewers {
			result.ResponseViewers = append(result.ResponseViewers, ViewItem{
				PluginID: manifest.ID, ID: viewer.ID, Title: viewer.Title, ContentTypes: viewer.ContentTypes,
				URL: viewURL(plugin.declaration, viewer.View),
			})
		}
		for _, tab := range contributions.RequestTabs {
			result.RequestTabs = append(result.RequestTabs, ViewItem{
				PluginID: manifest.ID, ID: tab.ID, Title: tab.Title, ContentTypes: []string{},
				URL: viewURL(plugin.declaration, tab.View),
			})
		}
	}
	return result
}

// InvokeView calls a handler the page of a view asks for.
func (host *Host) InvokeView(workspaceRoot string, pluginID string, viewID string, handler string, payload string) (string, error) {
	plugin, err := host.enabledPlugin(workspaceRoot, pluginID)
	if err != nil {
		return "", err
	}
	result, _, err := plugin.runtime.InvokeView(viewID, handler, payload)
	return result, err
}

// ViewFile returns a file of a view page and its media type. The URL names the
// repository and the commit, and an enabled plugin of a workspace loaded in
// this session must run that exact commit: nothing else of the cache is
// served.
func (host *Host) ViewFile(urlPath string) ([]byte, string, error) {
	rest, found := strings.CutPrefix(urlPath, ViewPathPrefix)
	parts := strings.SplitN(rest, "/", 3)
	if !found || len(parts) != 3 || !fullCommitHash.MatchString(parts[1]) {
		return nil, "", ErrViewNotFound
	}
	key, commit, relative := parts[0], parts[1], parts[2]

	repository, enabled := host.enabledCheckout(key, commit)
	if !enabled {
		return nil, "", ErrViewNotFound
	}
	checkout, found := host.cache.CachedCheckout(repository, commit)
	if !found {
		return nil, "", ErrViewNotFound
	}
	content, err := ReadFile(checkout, relative)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrViewNotFound, err)
	}
	mediaType := mime.TypeByExtension(path.Ext(relative))
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}
	return content, mediaType, nil
}

func (host *Host) enabledCheckout(key string, commit string) (string, bool) {
	host.mutex.Lock()
	defer host.mutex.Unlock()
	for _, loaded := range host.workspaces {
		for _, plugin := range loaded.plugins {
			if plugin.declaration.Commit == commit && repositoryKey(plugin.declaration.Repository) == key {
				return plugin.declaration.Repository, true
			}
		}
	}
	return "", false
}

func viewURL(declaration domain.PluginDeclaration, view string) string {
	return ViewPathPrefix + repositoryKey(declaration.Repository) + "/" + declaration.Commit + "/" + view
}
