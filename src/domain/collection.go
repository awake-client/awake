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
	// Color names one of the colors listed in Colors.
	Color string `yaml:"color,omitempty"`
}

// Validate checks the collection metadata.
func (collection Collection) Validate() error {
	if strings.TrimSpace(collection.Name) == "" || !IsColor(collection.Color) {
		return ErrInvalidCollection
	}
	if collection.Authentication != nil {
		if err := validateAuthentication(*collection.Authentication); err != nil {
			return ErrInvalidCollection
		}
	}

	return nil
}
