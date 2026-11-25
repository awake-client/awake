package plugins

import (
	"errors"
	"strings"
	"testing"
)

const viewManifest = `version: 1
id: viewer
name: Viewer
pluginVersion: 1.0.0
main: dist/plugin.js
contributes:
  commands: [{ id: hello, title: "Viewer: hello" }]
  authentication: [{ type: stamp, label: Stamp, fields: [{ key: who, type: string, label: Who }] }]
  dynamicVariables: [{ name: $view }]
  responseViewers: [{ id: lines, title: Lines, contentTypes: [text/plain], view: ui/lines.html }]
  requestTabs: [{ id: notes, title: Notes, view: ui/notes.html }]
`

const viewBundle = `awake.register({
	commands: { hello() {} },
	authentication: { stamp() {} },
	dynamicVariables: { $view: () => "view" },
	views: { lines: { count: (context) => ({ lines: context.payload.text.split("\n").length }) } },
})`

func TestHostListsTheUIContributionsOfEnabledPlugins(t *testing.T) {
	fixture := newHostFixture(t)
	fixture.publishViewPlugin(viewBundle)

	if contributions := fixture.host.Contributions(testWorkspaceRoot); len(contributions.Commands) != 0 {
		t.Fatalf("a plugin not installed contributes nothing, got %+v", contributions)
	}
	if _, err := fixture.service.Install(testWorkspaceRoot, PluginSource{Repository: testRepositoryURL}, firstCommit); err != nil {
		t.Fatalf("install: %v", err)
	}

	contributions, err := fixture.service.Contributions(testWorkspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(contributions.Commands) != 1 || contributions.Commands[0].Title != "Viewer: hello" || contributions.Commands[0].PluginName != "Viewer" {
		t.Errorf("unexpected commands %+v", contributions.Commands)
	}
	if len(contributions.Authentication) != 1 || contributions.Authentication[0].Type != "plugin:viewer/stamp" || contributions.Authentication[0].Fields[0].Key != "who" {
		t.Errorf("unexpected authentication %+v", contributions.Authentication)
	}
	if len(contributions.DynamicVariables) != 1 || contributions.DynamicVariables[0].Name != "$view" {
		t.Errorf("unexpected dynamic variables %+v", contributions.DynamicVariables)
	}
	viewer := contributions.ResponseViewers[0]
	expectedURL := ViewPathPrefix + repositoryKey(testRepositoryURL) + "/" + firstCommit + "/ui/lines.html"
	if viewer.URL != expectedURL || viewer.ContentTypes[0] != "text/plain" || contributions.RequestTabs[0].ID != "notes" {
		t.Errorf("unexpected views %+v %+v", viewer, contributions.RequestTabs)
	}
}

func TestHostServesTheViewsOfEnabledPluginsOnly(t *testing.T) {
	fixture := newHostFixture(t)
	fixture.publishViewPlugin(viewBundle)
	viewPath := ViewPathPrefix + repositoryKey(testRepositoryURL) + "/" + firstCommit + "/ui/lines.html"

	if _, _, err := fixture.host.ViewFile(viewPath); !errors.Is(err, ErrViewNotFound) {
		t.Fatalf("a plugin not installed must not serve views, got %v", err)
	}
	if _, err := fixture.service.Install(testWorkspaceRoot, PluginSource{Repository: testRepositoryURL}, firstCommit); err != nil {
		t.Fatalf("install: %v", err)
	}
	fixture.host.Contributions(testWorkspaceRoot)

	content, mediaType, err := fixture.host.ViewFile(viewPath)
	if err != nil {
		t.Fatalf("view file: %v", err)
	}
	if string(content) != "<p>lines</p>" || !strings.HasPrefix(mediaType, "text/html") {
		t.Errorf("unexpected view %q %q", content, mediaType)
	}

	refused := []string{
		ViewPathPrefix + repositoryKey(testRepositoryURL) + "/" + firstCommit + "/../../trust.yaml",
		ViewPathPrefix + repositoryKey(testRepositoryURL) + "/" + secondCommit + "/ui/lines.html",
		ViewPathPrefix + "0123456789abcdef0123456789abcdef/" + firstCommit + "/ui/lines.html",
		ViewPathPrefix + repositoryKey(testRepositoryURL) + "/main/ui/lines.html",
		"/elsewhere/ui/lines.html",
	}
	for _, path := range refused {
		if _, _, err := fixture.host.ViewFile(path); !errors.Is(err, ErrViewNotFound) {
			t.Errorf("expected %s to be refused, got %v", path, err)
		}
	}

	if err := fixture.service.Revoke(testWorkspaceRoot, "viewer"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, _, err := fixture.host.ViewFile(viewPath); !errors.Is(err, ErrViewNotFound) {
		t.Errorf("a disabled plugin must stop serving its views at once, got %v", err)
	}
}

func TestHostInvokesTheHandlersOfAView(t *testing.T) {
	fixture := newHostFixture(t)
	fixture.publishViewPlugin(viewBundle)
	if _, err := fixture.service.Install(testWorkspaceRoot, PluginSource{Repository: testRepositoryURL}, firstCommit); err != nil {
		t.Fatalf("install: %v", err)
	}

	result, err := fixture.service.InvokeView(testWorkspaceRoot, "viewer", "lines", "count", `{"text":"a\nb\nc"}`)
	if err != nil || result != `{"lines":3}` {
		t.Fatalf("unexpected result %s: %v", result, err)
	}
	if _, err := fixture.service.InvokeView(testWorkspaceRoot, "viewer", "notes", "count", ""); !errors.Is(err, ErrUndeclaredHandler) {
		t.Errorf("expected a view without handlers to refuse calls, got %v", err)
	}
	if _, err := fixture.service.InvokeView(testWorkspaceRoot, "viewer", "lines", "count", "not json"); err == nil {
		t.Error("expected a malformed payload to be refused")
	}
}

func TestLoadRuntimeRefusesHandlersForUndeclaredViews(t *testing.T) {
	manifest := testRuntimeManifest()
	_, err := LoadRuntime(manifest, []byte(`awake.register({ views: { hidden: { run() {} } } })`), RuntimeOptions{})

	if !errors.Is(err, ErrUndeclaredHandler) || !strings.Contains(err.Error(), `views "hidden"`) {
		t.Fatalf("expected the undeclared view to be refused, got %v", err)
	}
}

func (fixture *hostFixture) publishViewPlugin(bundle string) {
	fixture.t.Helper()
	fixture.git.publishTo(testRepositoryURL, firstCommit, "", testArchive(fixture.t, map[string]string{
		ManifestFileName: viewManifest,
		"dist/plugin.js": bundle,
		"ui/lines.html":  "<p>lines</p>",
		"ui/notes.html":  "<p>notes</p>",
	}))
}
