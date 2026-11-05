package interop

import (
	"strings"

	"strconv"
)

// slugify turns a display name into a portable file name.
func slugify(value string) string {
	normalized := strings.ToLower(value)
	normalized = strings.NewReplacer(
		"à", "a", "á", "a", "â", "a", "ä", "a",
		"ç", "c", "è", "e", "é", "e", "ê", "e", "ë", "e",
		"î", "i", "ï", "i", "ô", "o", "ö", "o", "ù", "u", "û", "u", "ü", "u",
		"ß", "ss",
	).Replace(normalized)

	var builder strings.Builder
	lastWasSeparator := true

	for _, character := range normalized {
		switch {
		case character >= 'a' && character <= 'z',
			character >= '0' && character <= '9':
			builder.WriteRune(character)
			lastWasSeparator = false
		default:
			if !lastWasSeparator {
				builder.WriteByte('-')
				lastWasSeparator = true
			}
		}
	}

	return strings.Trim(builder.String(), "-")
}

// uniqueName returns a slug that is not already present in used, recording it.
func uniqueName(used map[string]int, candidate string, fallback string) string {
	if candidate == "" {
		candidate = fallback
	}

	count := used[candidate]
	used[candidate] = count + 1
	if count == 0 {
		return candidate
	}

	return candidate + "-" + strconv.Itoa(count+1)
}
