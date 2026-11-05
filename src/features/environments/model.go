// Package environments reads environment files and resolves the variables used
// in requests.
package environments

import (
	"errors"
	"regexp"
	"strings"
)

const (
	directoryName = "environments"
	fileExtension = ".yaml"
	localSuffix   = ".local" + fileExtension
)

var (
	// ErrEnvironmentNotFound is returned when the shared environment file does
	// not exist.
	ErrEnvironmentNotFound = errors.New("environments: environment not found")
	// ErrInvalidEnvironment is returned when an environment file is malformed.
	ErrInvalidEnvironment = errors.New("environments: invalid environment")
)

var (
	fileNamePattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
	variableNameRegex = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// Environment mirrors an environments/<name>.yaml file.
type Environment struct {
	Name            string            `yaml:"name"`
	Variables       map[string]string `yaml:"variables,omitempty"`
	SecretVariables []string          `yaml:"secretVariables,omitempty"`
}

// Summary describes an environment listed in the interface.
type Summary struct {
	Name              string
	FileName          string
	HasLocalOverrides bool
}

// ResolvedEnvironment combines a shared environment with its local overrides.
type ResolvedEnvironment struct {
	Name   string
	Shared Environment
	Local  *Environment
}

// Layers returns the variable maps in increasing precedence order.
func (environment ResolvedEnvironment) Layers() []map[string]string {
	layers := []map[string]string{environment.Shared.Variables}
	if environment.Local != nil {
		layers = append(layers, environment.Local.Variables)
	}
	return layers
}

// SecretNames returns the secret variable names declared by both the shared
// environment and its local overrides.
func (environment ResolvedEnvironment) SecretNames() []string {
	seen := make(map[string]bool)
	names := make([]string, 0)

	appendNames := func(environment Environment) {
		for _, name := range environment.SecretVariables {
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}

	appendNames(environment.Shared)
	if environment.Local != nil {
		appendNames(*environment.Local)
	}

	return names
}

func (environment Environment) validate() error {
	if strings.TrimSpace(environment.Name) == "" {
		return ErrInvalidEnvironment
	}

	for name := range environment.Variables {
		if !variableNameRegex.MatchString(name) {
			return ErrInvalidEnvironment
		}
	}

	seen := make(map[string]bool)
	for _, name := range environment.SecretVariables {
		if !variableNameRegex.MatchString(name) || seen[name] {
			return ErrInvalidEnvironment
		}
		if _, exposed := environment.Variables[name]; exposed {
			return ErrInvalidEnvironment
		}
		seen[name] = true
	}

	return nil
}

func validateFileName(name string) error {
	if !fileNamePattern.MatchString(name) {
		return ErrInvalidEnvironment
	}
	return nil
}
