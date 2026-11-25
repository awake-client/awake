package updates

import (
	"reflect"
	"testing"
)

func TestParseChangelogReadsVersionsAndSections(t *testing.T) {
	versions := parseChangelog(sampleChangelog)

	if len(versions) != 4 {
		t.Fatalf("got %d versions, want 4", len(versions))
	}
	want := ChangelogVersion{
		Version: "0.9.0",
		Date:    "2026-11-20",
		Sections: []ChangelogSection{
			{Title: "Added", Items: []string{"Request scripts store collection variables."}},
			{Title: "Fixed", Items: []string{"No more console window on Windows."}},
		},
	}
	if !reflect.DeepEqual(versions[1], want) {
		t.Errorf("got %+v\nwant %+v", versions[1], want)
	}
	if last := versions[3]; len(last.Sections) != 1 || len(last.Sections[0].Items) != 1 {
		t.Errorf("link references leaked into the last version: %+v", last)
	}
}

func TestChangesBetween(t *testing.T) {
	versions := parseChangelog(sampleChangelog)

	cases := []struct {
		name    string
		current string
		latest  string
		want    []string
	}{
		{"skipped versions", "0.8.0", "0.9.0", []string{"0.9.0", "0.8.1"}},
		{"prefixed current", "v0.8.1", "0.9.0", []string{"0.9.0"}},
		{"newer changelog", "0.8.0", "0.8.1", []string{"0.8.1"}},
		{"development build", "dev", "0.9.0", []string{"0.9.0"}},
		{"up to date", "0.9.0", "0.9.0", nil},
	}
	for _, test := range cases {
		var got []string
		for _, version := range changesBetween(versions, test.current, test.latest) {
			got = append(got, version.Version)
		}
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("%s: got %v, want %v", test.name, got, test.want)
		}
	}
}

func TestChangesBetweenUsesUnreleasedChangesForAnUnlistedRelease(t *testing.T) {
	versions := parseChangelog(sampleChangelog)

	changes := changesBetween(versions, "0.8.1", "0.9.1")
	if len(changes) != 2 || changes[0].Version != "0.9.1" || changes[1].Version != "0.9.0" {
		t.Fatalf("got %+v", changes)
	}
	if got := changes[0].Sections[0].Items[0]; got != "Something not shipped yet." {
		t.Errorf("got %q, want the unreleased change", got)
	}
}

func TestParseReleaseNotes(t *testing.T) {
	notes := "## What's Changed\r\n* Fix things by @someone in https://example.test/1\r\n\r\n" +
		"**Full Changelog**: https://example.test/compare"

	got := parseReleaseNotes("1.0.0", notes)
	want := []ChangelogVersion{{
		Version: "1.0.0",
		Sections: []ChangelogSection{{
			Title: "What's Changed",
			Items: []string{
				"Fix things by @someone in https://example.test/1",
				"**Full Changelog**: https://example.test/compare",
			},
		}},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
	if parseReleaseNotes("1.0.0", "  ") != nil {
		t.Error("expected empty notes to give no changes")
	}
}
