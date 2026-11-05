package domain

import (
	"errors"
	"strings"
)

// ErrInvalidCollection is returned when a collection file is malformed.
var ErrInvalidCollection = errors.New("domain: invalid collection")

// Collection mirrors a collection.yaml file.
type Collection struct {
	Name           string            `yaml:"name"`
	Variables      map[string]string `yaml:"variables,omitempty"`
	Authentication *Authentication   `yaml:"authentication,omitempty"`
	Documentation  string            `yaml:"documentation,omitempty"`
}

// Validate checks the collection metadata.
func (collection Collection) Validate() error {
	if strings.TrimSpace(collection.Name) == "" {
		return ErrInvalidCollection
	}
	if collection.Authentication != nil && !isSupportedAuthentication(collection.Authentication.Type) {
		return ErrInvalidCollection
	}

	return nil
}
