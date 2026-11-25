package domain

// MarketplaceReference names a marketplace a workspace installs plugins from.
// It is stored in awake.yaml so the whole team browses the same catalog.
type MarketplaceReference struct {
	Name       string `yaml:"name" json:"name"`
	Repository string `yaml:"repository" json:"repository"`
}

// PluginDeclaration is a plugin a workspace uses, pinned to one commit of its
// repository. It is stored in awake.yaml, so pulling the workspace brings the
// same plugin version to every teammate; each of them still approves it
// before it runs.
type PluginDeclaration struct {
	ID string `yaml:"id" json:"id"`
	// Marketplace names the marketplace the plugin was installed from, used
	// to look for updates. It is empty for a plugin installed from its
	// repository URL.
	Marketplace string `yaml:"marketplace,omitempty" json:"marketplace,omitempty"`
	Repository  string `yaml:"repository" json:"repository"`
	// Ref is the tag or branch the commit was resolved from, kept to look for
	// updates of a plugin installed from its repository URL.
	Ref    string `yaml:"ref,omitempty" json:"ref,omitempty"`
	Commit string `yaml:"commit" json:"commit"`
	// Settings holds the values shared with the team. Secret settings stay in
	// the keychain of each user.
	Settings map[string]string `yaml:"settings,omitempty" json:"settings,omitempty"`
}
