package variables

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var placeholderPattern = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)

// MissingVariablesError lists the variables referenced by a template but not
// defined in any layer.
type MissingVariablesError struct {
	Names []string
}

func (missingError *MissingVariablesError) Error() string {
	return fmt.Sprintf("missing variables: %s", strings.Join(missingError.Names, ", "))
}

// Interpolate replaces {{variable}} placeholders using the provided layers,
// ordered from lowest to highest precedence. It fails instead of leaving an
// undefined placeholder behind.
func Interpolate(input string, layers ...map[string]string) (string, error) {
	values := make(map[string]string)
	for _, layer := range layers {
		for name, value := range layer {
			values[name] = value
		}
	}

	missing := make(map[string]bool)

	result := placeholderPattern.ReplaceAllStringFunc(input, func(placeholder string) string {
		matches := placeholderPattern.FindStringSubmatch(placeholder)
		name := matches[1]

		value, found := values[name]
		if !found {
			missing[name] = true
			return placeholder
		}

		return value
	})

	if len(missing) > 0 {
		names := make([]string, 0, len(missing))
		for name := range missing {
			names = append(names, name)
		}
		sort.Strings(names)
		return "", &MissingVariablesError{Names: names}
	}

	return result, nil
}
