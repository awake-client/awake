package plugins

import (
	"errors"
	"strings"
	"testing"
)

func TestParseMarketplaceReadsItsPlugins(t *testing.T) {
	marketplace, err := ParseMarketplace(readTestData(t, "marketplace.yaml"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if marketplace.Name != "awake-official" || len(marketplace.Plugins) != 2 {
		t.Fatalf("unexpected marketplace: %+v", marketplace)
	}
	entry, found := marketplace.Find("aws-sigv4")
	if !found || entry.Ref != "v1.2.0" || entry.Tags[0] != "auth" {
		t.Errorf("unexpected entry: %+v, found %v", entry, found)
	}
	if _, found := marketplace.Find("missing"); found {
		t.Error("expected an unknown plugin not to be found")
	}
}

func TestParseMarketplaceReportsProblems(t *testing.T) {
	cases := []struct {
		name     string
		change   func(string) string
		expected string
	}{
		{"unknown field", replace("    tags: [auth]", "    tags: [auth]\n    stars: 5"), "field stars not found"},
		{"bad name", replace("name: awake-official", "name: Awake Official"), "name:"},
		{"duplicate plugin", replace("id: insomnia", "id: aws-sigv4"), `plugin "aws-sigv4" is declared twice`},
		{"plain http", replace("https://github.com/awake-plugins/aws-sigv4.git", "http://example.com/x.git"), "unsupported repository URL"},
		{"option ref", replace("ref: v1.2.0", "ref: --upload-pack=evil"), "not a valid tag or branch name"},
		{"range ref", replace("ref: v1.2.0", "ref: main..dev"), "not a valid tag or branch name"},
	}

	base := string(readTestData(t, "marketplace.yaml"))
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			changed := test.change(base)
			if changed == base {
				t.Fatal("the change did not apply to the fixture")
			}

			_, err := ParseMarketplace([]byte(changed))

			if !errors.Is(err, ErrInvalidMarketplace) {
				t.Fatalf("expected ErrInvalidMarketplace, got %v", err)
			}
			if !strings.Contains(err.Error(), test.expected) {
				t.Errorf("expected %q in %q", test.expected, err.Error())
			}
		})
	}
}

func TestParseMarketplaceRefusesANewerVersion(t *testing.T) {
	_, err := ParseMarketplace([]byte("version: 3\nname: future\nplugins: []\n"))

	if !errors.Is(err, ErrUnsupportedMarketplaceVersion) {
		t.Fatalf("expected ErrUnsupportedMarketplaceVersion, got %v", err)
	}
}

func TestValidateRepositoryURL(t *testing.T) {
	accepted := []string{
		"https://github.com/awake-plugins/aws-sigv4.git",
		"ssh://git@github.com/awake-plugins/aws-sigv4.git",
		"git@github.com:awake-plugins/aws-sigv4.git",
		"file:///Users/ada/plugins/aws-sigv4",
		"git@example.com:/srv/git/plugin.git",
	}
	for _, url := range accepted {
		if err := ValidateRepositoryURL(url); err != nil {
			t.Errorf("expected %q to be accepted: %v", url, err)
		}
	}

	refused := []string{
		"",
		"http://example.com/plugin.git",
		"--upload-pack=touch /tmp/pwned",
		"ext::sh -c touch% /tmp/pwned",
		"fd::17",
		"https://",
		" https://example.com/plugin.git",
		"https://example.com/plugin.git\n--config",
		"/Users/ada/plugins/aws-sigv4",
		"../plugin",
	}
	for _, url := range refused {
		if err := ValidateRepositoryURL(url); !errors.Is(err, ErrUnsupportedRepositoryURL) {
			t.Errorf("expected %q to be refused, got %v", url, err)
		}
	}
}
