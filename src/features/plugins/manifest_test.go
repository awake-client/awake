package plugins

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseManifestReadsEveryContribution(t *testing.T) {
	manifest, err := ParseManifest(readTestData(t, "complete-manifest.yaml"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if manifest.ID != "aws-sigv4" || manifest.Main != "dist/plugin.js" {
		t.Errorf("unexpected identity: %+v", manifest)
	}
	if !manifest.HasPermission(PermissionNetwork) || manifest.HasPermission(PermissionWorkspaceWrite) {
		t.Errorf("unexpected permissions: %v", manifest.Permissions)
	}
	contributions := manifest.Contributes
	counts := map[string]int{
		"dynamic variables": len(contributions.DynamicVariables),
		"authentication":    len(contributions.Authentication),
		"import formats":    len(contributions.ImportFormats),
		"export formats":    len(contributions.ExportFormats),
		"hooks":             len(contributions.Hooks),
		"commands":          len(contributions.Commands),
		"response viewers":  len(contributions.ResponseViewers),
		"request tabs":      len(contributions.RequestTabs),
	}
	expected := map[string]int{
		"dynamic variables": 1, "authentication": 1, "import formats": 1, "export formats": 1,
		"hooks": 2, "commands": 1, "response viewers": 1, "request tabs": 1,
	}
	for kind, count := range expected {
		if counts[kind] != count {
			t.Errorf("expected %d %s, got %d", count, kind, counts[kind])
		}
	}
	if fields := contributions.Authentication[0].Fields; len(fields) != 2 || fields[1].Type != SettingSecret {
		t.Errorf("unexpected authentication fields: %+v", fields)
	}
}

func TestParseManifestReportsProblems(t *testing.T) {
	cases := []struct {
		name     string
		change   func(string) string
		expected string
	}{
		{"unknown field", replace("main: dist/plugin.js", "main: dist/plugin.js\nmian: typo.js"), "field mian not found"},
		{"missing version", replace("version: 1\n", ""), "version must be 1"},
		{"upper case id", replace("id: aws-sigv4", "id: AWS"), "must be lower case"},
		{"long id", replace("id: aws-sigv4", "id: "+strings.Repeat("a", 65)), "longer than 64"},
		{"missing name", replace("name: AWS SigV4", "name: ' '"), "name is required"},
		{"bad plugin version", replace("pluginVersion: 1.2.0", "pluginVersion: latest"), "not a semantic version"},
		{"bad engine", replace(`engine: ">=0.11.0"`, `engine: "^0.11"`), "must read >="},
		{"absolute main", replace("main: dist/plugin.js", "main: /etc/passwd"), "must be relative"},
		{"escaping main", replace("main: dist/plugin.js", "main: ../outside.js"), "leaves the plugin repository"},
		{"windows main", replace("main: dist/plugin.js", `main: 'dist\plugin.js'`), "forward slashes"},
		{"drive main", replace("main: dist/plugin.js", "main: 'C:/plugin.js'"), "must be relative"},
		{"unknown permission", replace("permissions: [network, secrets]", "permissions: [filesystem]"), `permission "filesystem" is unknown`},
		{"select without options", replace("    options: [eu-west-3, us-east-1]\n", ""), "lists no option"},
		{"unknown setting type", replace("type: select", "type: color"), `unknown type "color"`},
		{"dynamic variable without dollar", replace("name: $awsDate", "name: awsDate"), "must look like $name"},
		{"unknown hook", replace("hooks: [beforeRequest, afterResponse]", "hooks: [onSave]"), `hook "onSave" is unknown`},
		{"duplicate hook free command", replace("    - id: refresh-token\n      title: \"AWS: refresh token\"", "    - id: refresh-token\n      title: one\n    - id: refresh-token\n      title: two"), `command "refresh-token" is declared twice`},
		{"duplicate view across kinds", replace("    - id: notes", "    - id: protobuf"), `view "protobuf" is declared twice`},
		{"extension without dot", replace("extensions: [.har]", "extensions: [har]"), "must start with a dot"},
		{"viewer without content type", replace("      contentTypes: [application/x-protobuf]\n", ""), "lists no content type"},
		{"escaping view", replace("view: ui/tab.html", "view: ../../tab.html"), "leaves the plugin repository"},
	}

	base := string(readTestData(t, "complete-manifest.yaml"))
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			changed := test.change(base)
			if changed == base {
				t.Fatal("the change did not apply to the fixture")
			}

			_, err := ParseManifest([]byte(changed))

			if !errors.Is(err, ErrInvalidManifest) {
				t.Fatalf("expected ErrInvalidManifest, got %v", err)
			}
			if !strings.Contains(err.Error(), test.expected) {
				t.Errorf("expected %q in %q", test.expected, err.Error())
			}
		})
	}
}

func TestParseManifestReportsEveryProblemAtOnce(t *testing.T) {
	_, err := ParseManifest([]byte("version: 1\nid: Bad\nname: ''\npluginVersion: x\nmain: ''\n"))

	for _, expected := range []string{"id:", "name is required", "pluginVersion", "main:"} {
		if err == nil || !strings.Contains(err.Error(), expected) {
			t.Errorf("expected %q in %v", expected, err)
		}
	}
}

func TestParseManifestRefusesANewerVersion(t *testing.T) {
	_, err := ParseManifest([]byte("version: 2\nid: future\n"))

	if !errors.Is(err, ErrUnsupportedManifestVersion) {
		t.Fatalf("expected ErrUnsupportedManifestVersion, got %v", err)
	}
}

func TestManifestSupportsAwake(t *testing.T) {
	cases := []struct {
		engine, awakeVersion string
		expected             bool
	}{
		{"", "0.1.0", true},
		{">=0.11.0", "0.11.0", true},
		{">=0.11.0", "v1.0.0", true},
		{">=0.11.0", "0.10.1", false},
		{">=0.11.0", "0.11.0-rc.1", false},
		{">=0.11.0", "dev", true},
	}
	for _, test := range cases {
		manifest := Manifest{Engine: test.engine}
		if actual := manifest.SupportsAwake(test.awakeVersion); actual != test.expected {
			t.Errorf("engine %q on %q: expected %v, got %v", test.engine, test.awakeVersion, test.expected, actual)
		}
	}
}

func replace(old string, new string) func(string) string {
	return func(content string) string {
		return strings.Replace(content, old, new, 1)
	}
}

func readTestData(t *testing.T, name string) []byte {
	t.Helper()

	content, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return content
}
