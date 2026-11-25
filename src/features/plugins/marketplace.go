package plugins

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// MarketplaceFileName is the file at the root of a marketplace repository
// that lists its plugins.
const MarketplaceFileName = "marketplace.yaml"

// SupportedMarketplaceVersion is the newest marketplace format this build
// reads.
const SupportedMarketplaceVersion = 1

var (
	// ErrInvalidMarketplace wraps every problem found in a marketplace index.
	ErrInvalidMarketplace = errors.New("plugins: invalid marketplace")
	// ErrUnsupportedMarketplaceVersion is returned for an index written for a
	// newer Awake.
	ErrUnsupportedMarketplaceVersion = errors.New("plugins: unsupported marketplace version")
	// ErrUnsupportedRepositoryURL is returned for a repository URL Awake will
	// not hand to git.
	ErrUnsupportedRepositoryURL = errors.New("plugins: unsupported repository URL")
)

// scpLikeURL matches the user@host:path form git accepts for SSH.
var scpLikeURL = regexp.MustCompile(`^[A-Za-z0-9._-]+@[A-Za-z0-9.-]+:.`)

var allowedURLSchemes = []string{"https://", "ssh://", "file://"}

// Marketplace mirrors marketplace.yaml.
type Marketplace struct {
	Version     int                `yaml:"version" json:"version"`
	Name        string             `yaml:"name" json:"name"`
	Description string             `yaml:"description,omitempty" json:"description,omitempty"`
	Plugins     []MarketplaceEntry `yaml:"plugins" json:"plugins"`
}

// MarketplaceEntry points at the repository of one plugin.
type MarketplaceEntry struct {
	ID         string `yaml:"id" json:"id"`
	Repository string `yaml:"repository" json:"repository"`
	// Ref is the tag or branch to install; empty means the default branch.
	Ref         string   `yaml:"ref,omitempty" json:"ref,omitempty"`
	Description string   `yaml:"description,omitempty" json:"description,omitempty"`
	Tags        []string `yaml:"tags,omitempty" json:"tags,omitempty"`
}

// ParseMarketplace decodes and validates marketplace.yaml.
func ParseMarketplace(content []byte) (Marketplace, error) {
	var marketplace Marketplace
	if err := decodeStrict(content, &marketplace); err != nil {
		return Marketplace{}, fmt.Errorf("%w: %w", ErrInvalidMarketplace, err)
	}
	if err := marketplace.Validate(); err != nil {
		return Marketplace{}, err
	}
	return marketplace, nil
}

// Validate checks the index and reports every problem at once.
func (marketplace Marketplace) Validate() error {
	if marketplace.Version > SupportedMarketplaceVersion {
		return fmt.Errorf("%w: %d, this Awake reads version %d", ErrUnsupportedMarketplaceVersion, marketplace.Version, SupportedMarketplaceVersion)
	}

	problems := &problemList{sentinel: ErrInvalidMarketplace}
	if marketplace.Version < 1 {
		problems.add("version must be %d", SupportedMarketplaceVersion)
	}
	problems.check(validateIdentifier(marketplace.Name), "name")

	ids := uniqueNames{problems: problems, kind: "plugin"}
	for _, entry := range marketplace.Plugins {
		problems.check(validateIdentifier(entry.ID), "plugin id")
		problems.check(ValidateRepositoryURL(entry.Repository), "plugin "+entry.ID+" repository")
		problems.check(validateRef(entry.Ref), "plugin "+entry.ID+" ref")
		ids.add(entry.ID)
	}

	return problems.err()
}

// Find returns the entry with the given plugin id.
func (marketplace Marketplace) Find(id string) (MarketplaceEntry, bool) {
	for _, entry := range marketplace.Plugins {
		if entry.ID == id {
			return entry, true
		}
	}
	return MarketplaceEntry{}, false
}

// ValidateRepositoryURL accepts the URL forms a marketplace or a workspace may
// point git at: HTTPS, SSH in both spellings, and file:// for plugins under
// development. Plain HTTP and anything git could read as an option are
// refused.
func ValidateRepositoryURL(url string) error {
	trimmed := strings.TrimSpace(url)
	switch {
	case trimmed == "":
		return fmt.Errorf("%w: a repository URL is required", ErrUnsupportedRepositoryURL)
	case trimmed != url, strings.ContainsAny(url, " \t\r\n"):
		return fmt.Errorf("%w: %q contains white space", ErrUnsupportedRepositoryURL, url)
	case strings.HasPrefix(url, "-"):
		return fmt.Errorf("%w: %q starts with a dash", ErrUnsupportedRepositoryURL, url)
	case strings.Contains(url, "::"):
		// git reads transport::address as a remote helper invocation.
		return fmt.Errorf("%w: %q names a remote helper", ErrUnsupportedRepositoryURL, url)
	}

	for _, scheme := range allowedURLSchemes {
		if rest, found := strings.CutPrefix(url, scheme); found {
			if rest == "" {
				return fmt.Errorf("%w: %q has no address", ErrUnsupportedRepositoryURL, url)
			}
			return nil
		}
	}
	if scpLikeURL.MatchString(url) {
		return nil
	}

	return fmt.Errorf("%w: %q, use https://, ssh://, git@host:path or file://", ErrUnsupportedRepositoryURL, url)
}

// validateRef refuses refs git could read as an option or a revision range.
func validateRef(ref string) error {
	if ref == "" {
		return nil
	}
	if strings.HasPrefix(ref, "-") || strings.Contains(ref, "..") || strings.ContainsAny(ref, " \t\r\n~^:?*[\\") {
		return fmt.Errorf("%q is not a valid tag or branch name", ref)
	}
	return nil
}
