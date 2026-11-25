// Package plugins installs, trusts and runs the plugins a workspace declares.
// A plugin is a git repository holding an awake-plugin.yaml manifest and a
// JavaScript bundle; marketplaces are git repositories that list plugins.
package plugins

import (
	"bytes"
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/awake-client/awake/src/shared/semver"
)

// ManifestFileName is the file at the root of a plugin repository that
// describes the plugin.
const ManifestFileName = "awake-plugin.yaml"

// SupportedManifestVersion is the newest manifest format this build reads.
const SupportedManifestVersion = 1

const (
	maximumIdentifierLength = 64
	engineMinimumPrefix     = ">="
)

var (
	// ErrInvalidManifest wraps every problem found in a manifest.
	ErrInvalidManifest = errors.New("plugins: invalid manifest")
	// ErrUnsupportedManifestVersion is returned for a manifest written for a
	// newer Awake.
	ErrUnsupportedManifestVersion = errors.New("plugins: unsupported manifest version")
)

var (
	identifierPattern      = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	symbolPattern          = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)
	dynamicVariablePattern = regexp.MustCompile(`^\$[A-Za-z][A-Za-z0-9_]*$`)
)

// Permission is a capability a plugin asks for. The user grants all of them,
// or none, when approving the plugin.
type Permission string

const (
	// PermissionNetwork lets the plugin send HTTP requests with awake.http.
	PermissionNetwork Permission = "network"
	// PermissionSecrets lets the plugin read secret variable values.
	PermissionSecrets Permission = "secrets"
	// PermissionWorkspaceWrite lets the plugin change files of the workspace.
	PermissionWorkspaceWrite Permission = "workspace:write"
	// PermissionClipboard lets the plugin write to the clipboard.
	PermissionClipboard Permission = "clipboard"
)

var knownPermissions = []Permission{
	PermissionNetwork,
	PermissionSecrets,
	PermissionWorkspaceWrite,
	PermissionClipboard,
}

// Hook names a moment of the request lifecycle a plugin runs at.
type Hook string

const (
	// HookBeforeRequest runs before the pre-request script of the request.
	HookBeforeRequest Hook = "beforeRequest"
	// HookAfterResponse runs before the post-response script of the request.
	HookAfterResponse Hook = "afterResponse"
)

var knownHooks = []Hook{HookBeforeRequest, HookAfterResponse}

// SettingType tells the interface which control edits a setting.
type SettingType string

const (
	// SettingString is a single line of text.
	SettingString SettingType = "string"
	// SettingSecret is text kept in the keychain and masked in the interface.
	SettingSecret SettingType = "secret"
	// SettingBoolean is a checkbox.
	SettingBoolean SettingType = "boolean"
	// SettingSelect is a choice among Options.
	SettingSelect SettingType = "select"
)

var knownSettingTypes = []SettingType{SettingString, SettingSecret, SettingBoolean, SettingSelect}

// Manifest mirrors awake-plugin.yaml.
type Manifest struct {
	Version       int    `yaml:"version" json:"version"`
	ID            string `yaml:"id" json:"id"`
	Name          string `yaml:"name" json:"name"`
	Description   string `yaml:"description,omitempty" json:"description,omitempty"`
	PluginVersion string `yaml:"pluginVersion" json:"pluginVersion"`
	// Engine is the oldest Awake release the plugin runs on, written ">=1.2.0".
	Engine      string        `yaml:"engine,omitempty" json:"engine,omitempty"`
	Main        string        `yaml:"main" json:"main"`
	Permissions []Permission  `yaml:"permissions,omitempty" json:"permissions,omitempty"`
	Settings    []Setting     `yaml:"settings,omitempty" json:"settings,omitempty"`
	Contributes Contributions `yaml:"contributes,omitempty" json:"contributes"`
}

// Setting describes one value the user configures for the plugin, or one field
// of an authentication the plugin provides.
type Setting struct {
	Key         string      `yaml:"key" json:"key"`
	Type        SettingType `yaml:"type" json:"type"`
	Label       string      `yaml:"label" json:"label"`
	Description string      `yaml:"description,omitempty" json:"description,omitempty"`
	Default     string      `yaml:"default,omitempty" json:"default,omitempty"`
	Options     []string    `yaml:"options,omitempty" json:"options,omitempty"`
	Required    bool        `yaml:"required,omitempty" json:"required,omitempty"`
}

// Contributions lists what the plugin adds to Awake.
type Contributions struct {
	DynamicVariables []DynamicVariableContribution `yaml:"dynamicVariables,omitempty" json:"dynamicVariables,omitempty"`
	Authentication   []AuthenticationContribution  `yaml:"authentication,omitempty" json:"authentication,omitempty"`
	ImportFormats    []FormatContribution          `yaml:"importFormats,omitempty" json:"importFormats,omitempty"`
	ExportFormats    []FormatContribution          `yaml:"exportFormats,omitempty" json:"exportFormats,omitempty"`
	Hooks            []Hook                        `yaml:"hooks,omitempty" json:"hooks,omitempty"`
	Commands         []CommandContribution         `yaml:"commands,omitempty" json:"commands,omitempty"`
	ResponseViewers  []ResponseViewerContribution  `yaml:"responseViewers,omitempty" json:"responseViewers,omitempty"`
	RequestTabs      []RequestTabContribution      `yaml:"requestTabs,omitempty" json:"requestTabs,omitempty"`
}

// DynamicVariableContribution is a {{$name}} placeholder the plugin computes.
type DynamicVariableContribution struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

// AuthenticationContribution is an authentication type the plugin applies to
// requests. Requests select it as plugin:<plugin id>/<type>.
type AuthenticationContribution struct {
	Type   string    `yaml:"type" json:"type"`
	Label  string    `yaml:"label" json:"label"`
	Fields []Setting `yaml:"fields,omitempty" json:"fields,omitempty"`
}

// FormatContribution is a document format the plugin imports or exports.
type FormatContribution struct {
	Format     string   `yaml:"format" json:"format"`
	Label      string   `yaml:"label" json:"label"`
	Extensions []string `yaml:"extensions,omitempty" json:"extensions,omitempty"`
}

// CommandContribution is an entry of the command palette.
type CommandContribution struct {
	ID    string `yaml:"id" json:"id"`
	Title string `yaml:"title" json:"title"`
}

// ResponseViewerContribution renders responses of the given content types in
// a sandboxed view.
type ResponseViewerContribution struct {
	ID           string   `yaml:"id" json:"id"`
	Title        string   `yaml:"title" json:"title"`
	ContentTypes []string `yaml:"contentTypes" json:"contentTypes"`
	View         string   `yaml:"view" json:"view"`
}

// RequestTabContribution adds a tab to the request editor, rendered in a
// sandboxed view.
type RequestTabContribution struct {
	ID    string `yaml:"id" json:"id"`
	Title string `yaml:"title" json:"title"`
	View  string `yaml:"view" json:"view"`
}

// ParseManifest decodes and validates awake-plugin.yaml. Unknown fields are
// refused so a misspelt key does not silently disable a contribution.
func ParseManifest(content []byte) (Manifest, error) {
	var manifest Manifest
	if err := decodeStrict(content, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("%w: %w", ErrInvalidManifest, err)
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

// Validate checks the manifest and reports every problem at once, so an author
// fixes them in one pass.
func (manifest Manifest) Validate() error {
	if manifest.Version > SupportedManifestVersion {
		return fmt.Errorf("%w: %d, this Awake reads version %d", ErrUnsupportedManifestVersion, manifest.Version, SupportedManifestVersion)
	}

	problems := &problemList{}
	if manifest.Version < 1 {
		problems.add("version must be %d", SupportedManifestVersion)
	}
	problems.check(validateIdentifier(manifest.ID), "id")
	if strings.TrimSpace(manifest.Name) == "" {
		problems.add("name is required")
	}
	if !semver.IsRelease(manifest.PluginVersion) {
		problems.add("pluginVersion %q is not a semantic version", manifest.PluginVersion)
	}
	if manifest.Engine != "" {
		if _, err := minimumEngine(manifest.Engine); err != nil {
			problems.add("engine: %v", err)
		}
	}
	problems.check(validateRelativePath(manifest.Main), "main")
	for _, permission := range manifest.Permissions {
		if !slices.Contains(knownPermissions, permission) {
			problems.add("permission %q is unknown", permission)
		}
	}
	validateSettings(problems, "settings", manifest.Settings)
	manifest.Contributes.validate(problems)

	return problems.err()
}

// SupportsAwake reports whether the plugin runs on the given Awake version. A
// development build accepts every plugin.
func (manifest Manifest) SupportsAwake(awakeVersion string) bool {
	if manifest.Engine == "" || !semver.IsRelease(awakeVersion) {
		return true
	}
	minimum, err := minimumEngine(manifest.Engine)
	if err != nil {
		return false
	}
	return semver.Compare(awakeVersion, minimum) >= 0
}

// HasPermission reports whether the manifest asks for permission.
func (manifest Manifest) HasPermission(permission Permission) bool {
	return slices.Contains(manifest.Permissions, permission)
}

func (contributions Contributions) validate(problems *problemList) {
	variableNames := uniqueNames{problems: problems, kind: "dynamic variable"}
	for _, variable := range contributions.DynamicVariables {
		if !dynamicVariablePattern.MatchString(variable.Name) {
			problems.add("dynamic variable %q must look like $name", variable.Name)
		}
		variableNames.add(variable.Name)
	}

	authenticationTypes := uniqueNames{problems: problems, kind: "authentication type"}
	for _, authentication := range contributions.Authentication {
		if !symbolPattern.MatchString(authentication.Type) {
			problems.add("authentication type %q must be letters, digits or underscores", authentication.Type)
		}
		if strings.TrimSpace(authentication.Label) == "" {
			problems.add("authentication type %q needs a label", authentication.Type)
		}
		authenticationTypes.add(authentication.Type)
		validateSettings(problems, "authentication "+authentication.Type+" fields", authentication.Fields)
	}

	validateFormats(problems, "import format", contributions.ImportFormats)
	validateFormats(problems, "export format", contributions.ExportFormats)

	for _, hook := range contributions.Hooks {
		if !slices.Contains(knownHooks, hook) {
			problems.add("hook %q is unknown", hook)
		}
	}

	commandIDs := uniqueNames{problems: problems, kind: "command"}
	for _, command := range contributions.Commands {
		problems.check(validateIdentifier(command.ID), "command id")
		if strings.TrimSpace(command.Title) == "" {
			problems.add("command %q needs a title", command.ID)
		}
		commandIDs.add(command.ID)
	}

	viewIDs := uniqueNames{problems: problems, kind: "view"}
	for _, viewer := range contributions.ResponseViewers {
		problems.check(validateIdentifier(viewer.ID), "response viewer id")
		if len(viewer.ContentTypes) == 0 {
			problems.add("response viewer %q lists no content type", viewer.ID)
		}
		problems.check(validateRelativePath(viewer.View), "response viewer "+viewer.ID+" view")
		viewIDs.add(viewer.ID)
	}
	for _, tab := range contributions.RequestTabs {
		problems.check(validateIdentifier(tab.ID), "request tab id")
		if strings.TrimSpace(tab.Title) == "" {
			problems.add("request tab %q needs a title", tab.ID)
		}
		problems.check(validateRelativePath(tab.View), "request tab "+tab.ID+" view")
		viewIDs.add(tab.ID)
	}
}

func validateFormats(problems *problemList, kind string, formats []FormatContribution) {
	names := uniqueNames{problems: problems, kind: kind}
	for _, format := range formats {
		problems.check(validateIdentifier(format.Format), kind)
		if strings.TrimSpace(format.Label) == "" {
			problems.add("%s %q needs a label", kind, format.Format)
		}
		for _, extension := range format.Extensions {
			if !strings.HasPrefix(extension, ".") {
				problems.add("%s %q: extension %q must start with a dot", kind, format.Format, extension)
			}
		}
		names.add(format.Format)
	}
}

func validateSettings(problems *problemList, context string, settings []Setting) {
	keys := uniqueNames{problems: problems, kind: context + " key"}
	for _, setting := range settings {
		if !symbolPattern.MatchString(setting.Key) {
			problems.add("%s: key %q must be letters, digits or underscores", context, setting.Key)
		}
		keys.add(setting.Key)
		if !slices.Contains(knownSettingTypes, setting.Type) {
			problems.add("%s: %q has unknown type %q", context, setting.Key, setting.Type)
		}
		if setting.Type == SettingSelect && len(setting.Options) == 0 {
			problems.add("%s: select %q lists no option", context, setting.Key)
		}
	}
}

// validateIdentifier checks the lower case, dash separated names used for
// plugins, marketplaces, commands and views.
func validateIdentifier(identifier string) error {
	if len(identifier) > maximumIdentifierLength {
		return fmt.Errorf("%q is longer than %d characters", identifier, maximumIdentifierLength)
	}
	if !identifierPattern.MatchString(identifier) {
		return fmt.Errorf("%q must be lower case letters, digits and dashes", identifier)
	}
	return nil
}

// validateRelativePath checks a manifest path syntactically: it must name a
// file inside the plugin repository. Symbolic links are checked when the file
// is opened, by resolvePluginFile.
func validateRelativePath(relative string) error {
	if strings.TrimSpace(relative) == "" {
		return errors.New("a path is required")
	}
	if strings.Contains(relative, "\\") {
		return fmt.Errorf("%q must use forward slashes", relative)
	}
	if path.IsAbs(relative) || strings.Contains(relative, ":") {
		return fmt.Errorf("%q must be relative to the plugin repository", relative)
	}
	cleaned := path.Clean(relative)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("%q leaves the plugin repository", relative)
	}
	return nil
}

func minimumEngine(engine string) (string, error) {
	version, found := strings.CutPrefix(strings.TrimSpace(engine), engineMinimumPrefix)
	version = strings.TrimSpace(version)
	if !found || !semver.IsRelease(version) {
		return "", fmt.Errorf("%q must read %s followed by a version, such as >=1.2.0", engine, engineMinimumPrefix)
	}
	return version, nil
}

func decodeStrict(content []byte, target any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)
	return decoder.Decode(target)
}

// problemList gathers validation problems into one error wrapping
// ErrInvalidManifest.
type problemList struct {
	sentinel error
	problems []string
}

func (list *problemList) add(format string, arguments ...any) {
	list.problems = append(list.problems, fmt.Sprintf(format, arguments...))
}

func (list *problemList) check(err error, field string) {
	if err != nil {
		list.add("%s: %v", field, err)
	}
}

func (list *problemList) err() error {
	if len(list.problems) == 0 {
		return nil
	}
	sentinel := list.sentinel
	if sentinel == nil {
		sentinel = ErrInvalidManifest
	}
	return fmt.Errorf("%w: %s", sentinel, strings.Join(list.problems, "; "))
}

// uniqueNames reports a name seen twice within one kind of contribution.
type uniqueNames struct {
	problems *problemList
	kind     string
	seen     map[string]bool
}

func (names *uniqueNames) add(name string) {
	if names.seen == nil {
		names.seen = map[string]bool{}
	}
	if names.seen[name] {
		names.problems.add("%s %q is declared twice", names.kind, name)
	}
	names.seen[name] = true
}
