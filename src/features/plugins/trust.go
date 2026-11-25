package plugins

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/awake-client/awake/src/shared/files"
	"github.com/awake-client/awake/src/shared/yamlcodec"
)

const (
	trustFileName      = "trust.yaml"
	trustFileVersion   = 1
	trustDirectoryMode = 0o700
)

// ErrIncompleteApproval is returned for an approval missing the workspace,
// the plugin or the commit it covers.
var ErrIncompleteApproval = errors.New("plugins: an approval needs a workspace, a plugin and a commit")

// Approval records that the user allowed one commit of a plugin to run in one
// workspace with the listed permissions. A plugin pulled from git is only run
// once its exact commit is approved on this machine, since a teammate could
// otherwise make it run any code by changing awake.yaml.
type Approval struct {
	WorkspaceID string       `yaml:"workspaceId" json:"workspaceId"`
	PluginID    string       `yaml:"pluginId" json:"pluginId"`
	Repository  string       `yaml:"repository" json:"repository"`
	Commit      string       `yaml:"commit" json:"commit"`
	Permissions []Permission `yaml:"permissions,omitempty" json:"permissions,omitempty"`
	ApprovedAt  time.Time    `yaml:"approvedAt" json:"approvedAt"`
}

type trustFile struct {
	Version   int        `yaml:"version"`
	Approvals []Approval `yaml:"approvals"`
}

// TrustStore keeps the approvals of this machine in the user configuration
// directory, outside every workspace, so git can never grant one.
type TrustStore struct {
	path  string
	now   func() time.Time
	mutex sync.Mutex
}

// NewTrustStore returns a store reading and writing the file at path.
func NewTrustStore(path string) *TrustStore {
	return &TrustStore{path: path, now: time.Now}
}

// DefaultTrustPath returns the file the store uses outside tests.
func DefaultTrustPath() (string, error) {
	configurationDirectory, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("plugins: locate the user configuration directory: %w", err)
	}
	return filepath.Join(configurationDirectory, "awake", "plugins", trustFileName), nil
}

// IsTrusted reports whether the user approved this commit of the plugin in the
// workspace, with at least the permissions it now asks for. A new commit or a
// new permission needs a new approval.
func (store *TrustStore) IsTrusted(workspaceID string, pluginID string, commit string, permissions []Permission) (bool, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	content, err := store.read()
	if err != nil {
		return false, err
	}
	approval, found := findApproval(content.Approvals, workspaceID, pluginID)
	if !found || approval.Commit != commit {
		return false, nil
	}
	for _, permission := range permissions {
		if !slices.Contains(approval.Permissions, permission) {
			return false, nil
		}
	}
	return true, nil
}

// Approve records the approval, replacing the previous one of the same plugin
// in the same workspace.
func (store *TrustStore) Approve(approval Approval) error {
	if approval.WorkspaceID == "" || approval.PluginID == "" || approval.Commit == "" {
		return ErrIncompleteApproval
	}
	if !fullCommitHash.MatchString(approval.Commit) {
		return fmt.Errorf("%w: %q", ErrInvalidCommit, approval.Commit)
	}

	store.mutex.Lock()
	defer store.mutex.Unlock()

	content, err := store.read()
	if err != nil {
		return err
	}
	approval.ApprovedAt = store.now().UTC().Truncate(time.Second)
	approval.Permissions = slices.Clone(approval.Permissions)
	slices.Sort(approval.Permissions)
	approval.Permissions = slices.Compact(approval.Permissions)

	content.Approvals = withoutApproval(content.Approvals, approval.WorkspaceID, approval.PluginID)
	content.Approvals = append(content.Approvals, approval)
	return store.write(content)
}

// Revoke forgets the approval of the plugin in the workspace, if any.
func (store *TrustStore) Revoke(workspaceID string, pluginID string) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	content, err := store.read()
	if err != nil {
		return err
	}
	remaining := withoutApproval(content.Approvals, workspaceID, pluginID)
	if len(remaining) == len(content.Approvals) {
		return nil
	}
	content.Approvals = remaining
	return store.write(content)
}

// Approvals lists the approvals recorded for the workspace.
func (store *TrustStore) Approvals(workspaceID string) ([]Approval, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	content, err := store.read()
	if err != nil {
		return nil, err
	}
	result := []Approval{}
	for _, approval := range content.Approvals {
		if approval.WorkspaceID == workspaceID {
			result = append(result, approval)
		}
	}
	return result, nil
}

func (store *TrustStore) read() (trustFile, error) {
	content, err := os.ReadFile(store.path)
	if errors.Is(err, fs.ErrNotExist) {
		return trustFile{Version: trustFileVersion}, nil
	}
	if err != nil {
		return trustFile{}, fmt.Errorf("plugins: read approvals: %w", err)
	}

	var decoded trustFile
	if err := yaml.Unmarshal(content, &decoded); err != nil {
		return trustFile{}, fmt.Errorf("plugins: read approvals in %s: %w", store.path, err)
	}
	if decoded.Version > trustFileVersion {
		return trustFile{}, fmt.Errorf("plugins: approvals in %s were written by a newer Awake", store.path)
	}
	decoded.Version = trustFileVersion
	return decoded, nil
}

func (store *TrustStore) write(content trustFile) error {
	if err := os.MkdirAll(filepath.Dir(store.path), trustDirectoryMode); err != nil {
		return fmt.Errorf("plugins: create the approvals directory: %w", err)
	}
	encoded, err := yamlcodec.EncodeValue(content)
	if err != nil {
		return fmt.Errorf("plugins: encode approvals: %w", err)
	}
	if err := files.WriteAtomically(store.path, encoded); err != nil {
		return fmt.Errorf("plugins: save approvals: %w", err)
	}
	return nil
}

func findApproval(approvals []Approval, workspaceID string, pluginID string) (Approval, bool) {
	for _, approval := range approvals {
		if approval.WorkspaceID == workspaceID && approval.PluginID == pluginID {
			return approval, true
		}
	}
	return Approval{}, false
}

func withoutApproval(approvals []Approval, workspaceID string, pluginID string) []Approval {
	return slices.DeleteFunc(slices.Clone(approvals), func(approval Approval) bool {
		return approval.WorkspaceID == workspaceID && approval.PluginID == pluginID
	})
}
