package workspace

import (
	"time"

	"github.com/clementherve/awake/src/domain"
)

// SupportedVersion is the newest workspace format version this build can read.
const SupportedVersion = 1

// Mode describes who the workspace belongs to.
type Mode string

const (
	// ModeTeam marks a workspace shared with a team through git.
	ModeTeam Mode = "team"
	// ModePersonal marks a workspace that may stay local.
	ModePersonal Mode = "personal"
)

// Configuration mirrors the content of awake.yaml.
type Configuration struct {
	Version            int               `yaml:"version"`
	ID                 string            `yaml:"id"`
	Name               string            `yaml:"name"`
	Mode               Mode              `yaml:"mode"`
	DefaultEnvironment string            `yaml:"defaultEnvironment,omitempty"`
	Variables          map[string]string `yaml:"variables,omitempty"`
	Git                *GitConfiguration `yaml:"git,omitempty"`
	// Color names one of the colors listed in domain.Colors.
	Color string `yaml:"color,omitempty"`
}

// GitConfiguration describes how the workspace synchronizes with its remote.
type GitConfiguration struct {
	Remote                  string `yaml:"remote"`
	Branch                  string `yaml:"branch"`
	AutomaticCommit         bool   `yaml:"automaticCommit"`
	AutomaticPull           bool   `yaml:"automaticPull"`
	AutomaticPush           bool   `yaml:"automaticPush"`
	SynchronizationInterval string `yaml:"synchronizationInterval"`
}

// SynchronizationPeriod parses the configured interval, falling back to five
// minutes when it is empty.
func (configuration GitConfiguration) SynchronizationPeriod() (time.Duration, error) {
	if configuration.SynchronizationInterval == "" {
		return 5 * time.Minute, nil
	}

	return time.ParseDuration(configuration.SynchronizationInterval)
}

func (configuration Configuration) validate() error {
	if configuration.Version < 1 {
		return ErrInvalidConfiguration
	}
	if configuration.Version > SupportedVersion {
		return ErrUnsupportedVersion
	}
	if configuration.ID == "" || configuration.Name == "" {
		return ErrInvalidConfiguration
	}
	if configuration.Mode != ModeTeam && configuration.Mode != ModePersonal {
		return ErrInvalidConfiguration
	}
	if !domain.IsColor(configuration.Color) {
		return ErrInvalidConfiguration
	}

	if configuration.Git != nil && configuration.Git.SynchronizationInterval != "" {
		if _, err := configuration.Git.SynchronizationPeriod(); err != nil {
			return ErrInvalidConfiguration
		}
	}

	return nil
}
