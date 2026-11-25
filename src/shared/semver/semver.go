// Package semver compares the semantic versions of Awake releases and
// plugins, with or without a "v" prefix.
package semver

import (
	"regexp"
	"strconv"
	"strings"
)

var versionNumber = regexp.MustCompile(`^v?(\d+(?:\.\d+)*)(?:-([0-9A-Za-z.-]+))?(?:\+.*)?$`)

// IsRelease reports whether version reads as a semantic version rather
// than a development label such as "dev".
func IsRelease(version string) bool {
	return versionNumber.MatchString(strings.TrimSpace(version))
}

// Compare orders two semantic versions, with or without a "v" prefix.
// A pre-release sorts before its release. Unreadable versions sort first.
func Compare(left, right string) int {
	leftMatch := versionNumber.FindStringSubmatch(strings.TrimSpace(left))
	rightMatch := versionNumber.FindStringSubmatch(strings.TrimSpace(right))
	switch {
	case leftMatch == nil && rightMatch == nil:
		return strings.Compare(left, right)
	case leftMatch == nil:
		return -1
	case rightMatch == nil:
		return 1
	}

	if order := compareNumbers(leftMatch[1], rightMatch[1]); order != 0 {
		return order
	}
	leftPrerelease, rightPrerelease := leftMatch[2], rightMatch[2]
	switch {
	case leftPrerelease == rightPrerelease:
		return 0
	case leftPrerelease == "":
		return 1
	case rightPrerelease == "":
		return -1
	}
	return strings.Compare(leftPrerelease, rightPrerelease)
}

func compareNumbers(left, right string) int {
	leftParts := strings.Split(left, ".")
	rightParts := strings.Split(right, ".")
	for position := 0; position < max(len(leftParts), len(rightParts)); position++ {
		leftPart, rightPart := numberAt(leftParts, position), numberAt(rightParts, position)
		if leftPart != rightPart {
			if leftPart < rightPart {
				return -1
			}
			return 1
		}
	}
	return 0
}

func numberAt(parts []string, position int) int {
	if position >= len(parts) {
		return 0
	}
	number, _ := strconv.Atoi(parts[position])
	return number
}
