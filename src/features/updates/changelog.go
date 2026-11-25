package updates

import (
	"regexp"
	"strings"

	"github.com/awake-client/awake/src/shared/semver"
)

// ChangelogVersion lists the changes one version brought.
type ChangelogVersion struct {
	Version  string             `json:"version"`
	Date     string             `json:"date,omitempty"`
	Sections []ChangelogSection `json:"sections"`
}

// ChangelogSection groups the changes of a version under a title such as
// "Added" or "Fixed". The title is empty for changes listed without one.
type ChangelogSection struct {
	Title string   `json:"title"`
	Items []string `json:"items"`
}

const unreleasedVersion = "unreleased"

var (
	versionHeading   = regexp.MustCompile(`^##\s+\[?([^\]\s]+)\]?(?:\s+-\s+(\S+))?`)
	anyHeading       = regexp.MustCompile(`^#{1,6}\s+(.+?)\s*#*\s*$`)
	sectionHeading   = regexp.MustCompile(`^###\s+(.+?)\s*$`)
	listItem         = regexp.MustCompile(`^\s*[-*+]\s+(.*)$`)
	linkReference    = regexp.MustCompile(`^\s*\[[^\]]+\]:\s+\S+`)
	markdownComments = regexp.MustCompile(`(?s)<!--.*?-->`)
)

// parseChangelog reads a changelog that follows Keep a Changelog: a level two
// heading per version, a level three heading per kind of change and a bullet
// per change. Versions come back in the order of the file.
func parseChangelog(markdown string) []ChangelogVersion {
	var versions []ChangelogVersion
	var builder sectionBuilder
	inVersion := false

	flush := func() {
		if inVersion {
			versions[len(versions)-1].Sections = builder.finish()
		}
		builder = sectionBuilder{}
	}

	for _, line := range strings.Split(markdownComments.ReplaceAllString(markdown, ""), "\n") {
		if match := versionHeading.FindStringSubmatch(line); match != nil {
			flush()
			versions = append(versions, ChangelogVersion{Version: match[1], Date: match[2]})
			inVersion = true
			continue
		}
		if !inVersion {
			continue
		}
		if match := sectionHeading.FindStringSubmatch(line); match != nil {
			builder.startSection(match[1])
			continue
		}
		builder.addLine(line)
	}
	flush()
	return versions
}

// parseReleaseNotes reads free-form release notes, such as the ones GitHub
// generates, as the changes of a single version. Any heading starts a section.
func parseReleaseNotes(version, notes string) []ChangelogVersion {
	var builder sectionBuilder
	for _, line := range strings.Split(markdownComments.ReplaceAllString(notes, ""), "\n") {
		if match := anyHeading.FindStringSubmatch(line); match != nil {
			builder.startSection(match[1])
			continue
		}
		builder.addLine(line)
	}
	sections := builder.finish()
	if len(sections) == 0 {
		return nil
	}
	return []ChangelogVersion{{Version: version, Sections: sections}}
}

// changesBetween keeps the released versions newer than current and not newer
// than latest. A development build only gets the changes of latest.
//
// A release tagged before its changes moved out of the Unreleased section
// still lists them there; they are then shown as the changes of latest.
func changesBetween(versions []ChangelogVersion, current, latest string) []ChangelogVersion {
	kept := releasedBetween(versions, current, latest)
	if len(kept) > 0 && semver.Compare(kept[0].Version, latest) == 0 {
		return kept
	}
	if semver.IsRelease(current) && semver.Compare(latest, current) <= 0 {
		return kept
	}
	for _, version := range versions {
		if strings.EqualFold(version.Version, unreleasedVersion) && len(version.Sections) > 0 {
			unreleased := ChangelogVersion{Version: latest, Sections: version.Sections}
			return append([]ChangelogVersion{unreleased}, kept...)
		}
	}
	return kept
}

func releasedBetween(versions []ChangelogVersion, current, latest string) []ChangelogVersion {
	var kept []ChangelogVersion
	for _, version := range versions {
		if strings.EqualFold(version.Version, unreleasedVersion) {
			continue
		}
		if semver.Compare(version.Version, latest) > 0 {
			continue
		}
		if semver.IsRelease(current) {
			if semver.Compare(version.Version, current) <= 0 {
				continue
			}
		} else if semver.Compare(version.Version, latest) != 0 {
			continue
		}
		kept = append(kept, version)
	}
	return kept
}

// sectionBuilder collects bullets, and the lines that wrap them, into
// sections.
type sectionBuilder struct {
	sections []ChangelogSection
	item     []string
}

func (builder *sectionBuilder) startSection(title string) {
	builder.endItem()
	builder.sections = append(builder.sections, ChangelogSection{Title: strings.TrimSpace(title)})
}

func (builder *sectionBuilder) addLine(line string) {
	if linkReference.MatchString(line) {
		builder.endItem()
		return
	}
	if match := listItem.FindStringSubmatch(line); match != nil {
		builder.endItem()
		builder.item = []string{strings.TrimSpace(match[1])}
		return
	}
	text := strings.TrimSpace(line)
	if text == "" {
		builder.endItem()
		return
	}
	builder.item = append(builder.item, text)
}

func (builder *sectionBuilder) endItem() {
	if len(builder.item) == 0 {
		return
	}
	text := strings.Join(builder.item, " ")
	builder.item = nil
	if len(builder.sections) == 0 {
		builder.sections = append(builder.sections, ChangelogSection{})
	}
	current := &builder.sections[len(builder.sections)-1]
	current.Items = append(current.Items, text)
}

func (builder *sectionBuilder) finish() []ChangelogSection {
	builder.endItem()
	sections := make([]ChangelogSection, 0, len(builder.sections))
	for _, section := range builder.sections {
		if len(section.Items) > 0 {
			sections = append(sections, section)
		}
	}
	return sections
}
