package scripts

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/awake-client/awake/src/domain"
)

func TestBeforeRequestMutatesRequestAndVariables(t *testing.T) {
	request := domain.Request{
		Name:   "List users",
		Method: domain.MethodGet,
		URL:    "https://example.com/users",
		Headers: []domain.Header{
			{Name: "Accept", Value: "application/json", Enabled: true},
			{Name: "X-Disabled", Value: "kept", Enabled: false},
		},
		Parameters: []domain.Parameter{
			{Name: "page", Value: "1", Enabled: true},
		},
		Body: &domain.Body{Type: domain.BodyNone},
	}
	variables := map[string]string{
		"token": "secret-token",
	}

	outcome := NewRunner().BeforeRequest(`
		request.method = "POST";
		request.url = request.url + "/search";
		request.headers["Authorization"] = "Bearer " + awake.getVariable("token");
		delete request.headers["Accept"];
		request.query["limit"] = "25";
		request.body = JSON.stringify({ query: "ada" });
		awake.setVariable("greeting", "hello");
		awake.log("preparing", request.method);
		awake.test("token is available", awake.getVariable("token") === "secret-token");
	`, &request, variables, nil, nil)

	if outcome.Error != "" {
		t.Fatalf("unexpected script error: %s", outcome.Error)
	}
	if !outcome.Tests[0].Passed {
		t.Errorf("expected the test to pass: %+v", outcome.Tests[0])
	}
	if len(outcome.Logs) != 1 || outcome.Logs[0] != "preparing POST" {
		t.Errorf("unexpected logs: %v", outcome.Logs)
	}

	if request.Method != domain.MethodPost {
		t.Errorf("unexpected method: %q", request.Method)
	}
	if request.URL != "https://example.com/users/search" {
		t.Errorf("unexpected URL: %q", request.URL)
	}
	if request.Body == nil || request.Body.Content != `{"query":"ada"}` {
		t.Errorf("unexpected body: %+v", request.Body)
	}
	if request.Body.Type != domain.BodyText {
		t.Errorf("expected the body type to become text: %q", request.Body.Type)
	}

	headers := headerValues(request.Headers)
	if headers["Authorization"] != "Bearer secret-token" {
		t.Errorf("unexpected authorization: %q", headers["Authorization"])
	}
	if _, found := headers["Accept"]; found {
		t.Error("expected the deleted header to be removed")
	}
	if headers["X-Disabled"] != "kept" {
		t.Error("expected disabled headers to be preserved")
	}

	if len(request.Parameters) != 2 {
		t.Fatalf("unexpected parameters: %+v", request.Parameters)
	}
	if request.Parameters[0].Name != "page" || request.Parameters[0].Value != "1" {
		t.Errorf("unexpected existing parameter: %+v", request.Parameters[0])
	}
	if request.Parameters[1].Name != "limit" || request.Parameters[1].Value != "25" {
		t.Errorf("unexpected added parameter: %+v", request.Parameters[1])
	}

	if variables["greeting"] != "hello" {
		t.Errorf("unexpected variables: %v", variables)
	}
}

func TestBeforeRequestReportsFailingAssertion(t *testing.T) {
	request := domain.Request{Name: "List users", Method: domain.MethodGet, URL: "https://example.com"}

	outcome := NewRunner().BeforeRequest(`
		awake.test("always false", false);
		awake.test("throws", () => { throw new Error("boom"); });
		awake.test("returns false", () => false);
		awake.test("returns true", () => true);
	`, &request, map[string]string{}, nil, nil)

	if len(outcome.Tests) != 4 {
		t.Fatalf("unexpected tests: %+v", outcome.Tests)
	}
	if outcome.Tests[0].Passed || outcome.Tests[0].Error == "" {
		t.Errorf("expected the first test to fail: %+v", outcome.Tests[0])
	}
	if outcome.Tests[1].Passed || !strings.Contains(outcome.Tests[1].Error, "boom") {
		t.Errorf("expected the second test to fail: %+v", outcome.Tests[1])
	}
	if outcome.Tests[2].Passed {
		t.Errorf("expected the third test to fail: %+v", outcome.Tests[2])
	}
	if !outcome.Tests[3].Passed {
		t.Errorf("expected the fourth test to pass: %+v", outcome.Tests[3])
	}
}

func TestAfterResponseExposesTheResponse(t *testing.T) {
	response := domain.ScriptResponse{
		StatusCode:           200,
		Status:               "200 OK",
		Headers:              map[string][]string{"Content-Type": {"application/json"}},
		Body:                 `{"name":"Ada"}`,
		Encoding:             "utf-8",
		DurationMilliseconds: 42,
	}

	variables := map[string]string{}
	outcome := NewRunner().AfterResponse(`
		awake.log("status", response.status);
		awake.log("type", response.headers["Content-Type"]);
		awake.setVariable("userName", JSON.parse(response.body).name);
		awake.test("successful", response.status === 200);
		awake.test("has a body", response.body.indexOf("Ada") >= 0);
	`, domain.Request{Name: "List users"}, response, variables, nil)

	if outcome.Error != "" {
		t.Fatalf("unexpected script error: %s", outcome.Error)
	}
	if len(outcome.Logs) != 2 || outcome.Logs[0] != "status 200" {
		t.Errorf("unexpected logs: %v", outcome.Logs)
	}
	if len(outcome.Tests) != 2 {
		t.Fatalf("unexpected tests: %+v", outcome.Tests)
	}
	for _, test := range outcome.Tests {
		if !test.Passed {
			t.Errorf("expected %q to pass: %+v", test.Name, test)
		}
	}
	if variables["userName"] != "Ada" {
		t.Errorf("unexpected variables: %v", variables)
	}
}

func TestRunnerInterruptsEndlessScripts(t *testing.T) {
	runner := &Runner{timeout: 50 * time.Millisecond}
	request := domain.Request{Name: "List users", Method: domain.MethodGet, URL: "https://example.com"}

	startedAt := time.Now()
	outcome := runner.BeforeRequest("while (true) {}", &request, map[string]string{}, nil, nil)

	if outcome.Error == "" {
		t.Fatal("expected the script to be interrupted")
	}
	if time.Since(startedAt) > 5*time.Second {
		t.Errorf("the script took too long: %s", time.Since(startedAt))
	}
}

func TestEmptyScriptReturnsAnEmptyOutcome(t *testing.T) {
	request := domain.Request{Name: "List users", Method: domain.MethodGet, URL: "https://example.com"}

	outcome := NewRunner().BeforeRequest("  \n ", &request, map[string]string{}, nil, nil)
	if outcome.Error != "" || len(outcome.Logs) != 0 || len(outcome.Tests) != 0 {
		t.Errorf("unexpected outcome: %+v", outcome)
	}
}

func headerValues(headers []domain.Header) map[string]string {
	values := make(map[string]string, len(headers))
	for _, header := range headers {
		values[header.Name] = header.Value
	}
	return values
}

func TestScriptsStoreEnvironmentChanges(t *testing.T) {
	request := domain.Request{Method: domain.MethodGet, URL: "https://example.com"}
	variables := map[string]string{"token": "old", "stale": "value"}

	outcome := NewRunner().AfterResponse(`
		awake.environment.set("token", "new");
		awake.environment.set("count", 3);
		awake.environment.unset("stale");
		awake.test("reads its own write", awake.environment.get("token") === "new");
		awake.test("removed value reads as null", awake.environment.get("stale") === null);
	`, request, domain.ScriptResponse{StatusCode: 200}, variables, nil)

	if outcome.Error != "" {
		t.Fatalf("unexpected script error: %s", outcome.Error)
	}
	for _, test := range outcome.Tests {
		if !test.Passed {
			t.Errorf("test %q failed: %s", test.Name, test.Error)
		}
	}

	expected := []domain.EnvironmentChange{
		{Name: "token", Value: "new"},
		{Name: "count", Value: "3"},
		{Name: "stale", Removed: true},
	}
	if len(outcome.EnvironmentChanges) != len(expected) {
		t.Fatalf("unexpected changes: %+v", outcome.EnvironmentChanges)
	}
	for index, change := range expected {
		if outcome.EnvironmentChanges[index] != change {
			t.Errorf("change %d: expected %+v, got %+v", index, change, outcome.EnvironmentChanges[index])
		}
	}
	if variables["token"] != "new" {
		t.Errorf("expected the variable map to follow the script, got %q", variables["token"])
	}
	if _, found := variables["stale"]; found {
		t.Error("expected the removed variable to leave the map")
	}
}

// Regression: setVariable on an existing name was overwritten by the stale
// value of the variables object when the script ended.
func TestSetVariableOverridesAnExistingValue(t *testing.T) {
	request := domain.Request{Method: domain.MethodGet, URL: "https://example.com"}
	variables := map[string]string{"token": "old"}

	outcome := NewRunner().BeforeRequest(`awake.setVariable("token", "new");`, &request, variables, nil, nil)

	if outcome.Error != "" {
		t.Fatalf("unexpected script error: %s", outcome.Error)
	}
	if variables["token"] != "new" {
		t.Errorf("expected token to be new, got %q", variables["token"])
	}
	if len(outcome.EnvironmentChanges) != 0 {
		t.Errorf("setVariable must not touch the environment: %+v", outcome.EnvironmentChanges)
	}
}

func TestResponseJSONParsesTheBody(t *testing.T) {
	request := domain.Request{Method: domain.MethodGet, URL: "https://example.com"}
	response := domain.ScriptResponse{StatusCode: 200, Body: `{"token":"abc"}`}

	outcome := NewRunner().AfterResponse(`
		awake.test("parsed", response.json().token === "abc");
	`, request, response, map[string]string{}, nil)
	if outcome.Error != "" || !outcome.Tests[0].Passed {
		t.Fatalf("unexpected outcome: %+v", outcome)
	}

	invalid := NewRunner().AfterResponse(`response.json();`, request, domain.ScriptResponse{Body: "not json"}, map[string]string{}, nil)
	if !strings.Contains(invalid.Error, "SyntaxError") {
		t.Errorf("expected a syntax error, got %q", invalid.Error)
	}
}

// fakeEnvironments resolves display names to environments, like the
// application matches "Production" to a file with another name.
type fakeEnvironments map[string]domain.ScriptEnvironment

func (environments fakeEnvironments) ResolveEnvironment(name string) (domain.ScriptEnvironment, error) {
	for displayName, environment := range environments {
		if strings.EqualFold(displayName, name) {
			return environment, nil
		}
	}
	return domain.ScriptEnvironment{}, errors.New("environment not found")
}

func TestScriptsChangeNamedEnvironments(t *testing.T) {
	request := domain.Request{Method: domain.MethodGet, URL: "https://example.com"}
	response := domain.ScriptResponse{
		StatusCode: 200,
		Headers: map[string][]string{
			"Set-Cookie": {"session=abc; Path=/; Expires=Wed, 21 Oct 2026 07:28:00 GMT", "theme=dark"},
		},
	}
	environments := fakeEnvironments{"Production": {
		FileName:  "base-url",
		Name:      "Production",
		Variables: map[string]string{"region": "eu"},
	}}
	variables := map[string]string{}

	outcome := NewRunner().AfterResponse(`
		const production = awake.environments.get("Production");
		awake.test("reads", production.get("region") === "eu");
		production.set("cookies", response.cookieHeader);
		awake.test("reads its write", production.get("cookies") === "session=abc; theme=dark");
		awake.test("parses cookies", response.cookies.session === "abc");
		console.log("saved", production.name, production.fileName);
		console.error("boom");
	`, request, response, variables, environments)

	if outcome.Error != "" {
		t.Fatalf("unexpected script error: %s", outcome.Error)
	}
	for _, test := range outcome.Tests {
		if !test.Passed {
			t.Errorf("test %q failed: %s", test.Name, test.Error)
		}
	}
	expected := domain.EnvironmentChange{Environment: "base-url", EnvironmentName: "Production", Name: "cookies", Value: "session=abc; theme=dark"}
	if len(outcome.EnvironmentChanges) != 1 || outcome.EnvironmentChanges[0] != expected {
		t.Errorf("unexpected changes: %+v", outcome.EnvironmentChanges)
	}
	if _, found := variables["cookies"]; found {
		t.Error("a named environment must not change the running request's variables")
	}
	if strings.Join(outcome.Logs, "|") != "saved Production base-url|error: boom" {
		t.Errorf("unexpected logs: %v", outcome.Logs)
	}

	missing := NewRunner().AfterResponse(`awake.environments.get("staging");`, request, response, variables, environments)
	if !strings.Contains(missing.Error, "environment not found") {
		t.Errorf("expected a missing environment error, got %q", missing.Error)
	}
}

func TestRunRequestExposesTheNestedResponse(t *testing.T) {
	request := domain.Request{Name: "List users", Method: domain.MethodGet, URL: "https://example.com"}
	variables := map[string]string{}

	var calledPath string
	var calledTimeout time.Duration
	execute := func(path string, timeout time.Duration) (domain.ScriptResponse, error) {
		calledPath = path
		calledTimeout = timeout
		return domain.ScriptResponse{
			StatusCode: 200,
			Status:     "200 OK",
			Headers:    map[string][]string{"Content-Type": {"application/json"}},
			Body:       `{"access_token":"abc"}`,
			Encoding:   "utf-8",
		}, nil
	}

	outcome := NewRunner().BeforeRequest(`
		const login = awake.runRequest("collections/auth/login.yaml", { timeout: 1500 });
		request.headers["Authorization"] = "Bearer " + login.json().access_token;
		awake.setVariable("loginStatus", login.status);
	`, &request, variables, nil, execute)

	if outcome.Error != "" {
		t.Fatalf("unexpected script error: %s", outcome.Error)
	}
	if calledPath != "collections/auth/login.yaml" {
		t.Errorf("unexpected path: %q", calledPath)
	}
	if calledTimeout != 1500*time.Millisecond {
		t.Errorf("unexpected timeout: %s", calledTimeout)
	}
	if headerValues(request.Headers)["Authorization"] != "Bearer abc" {
		t.Errorf("unexpected authorization: %q", headerValues(request.Headers)["Authorization"])
	}
	if variables["loginStatus"] != "200" {
		t.Errorf("unexpected variables: %v", variables)
	}
}

func TestRunRequestDefaultsToTheClientTimeout(t *testing.T) {
	request := domain.Request{Method: domain.MethodGet, URL: "https://example.com"}

	var called bool
	execute := func(_ string, timeout time.Duration) (domain.ScriptResponse, error) {
		called = true
		if timeout != 0 {
			t.Errorf("expected no timeout, got %s", timeout)
		}
		return domain.ScriptResponse{StatusCode: 204}, nil
	}

	outcome := NewRunner().BeforeRequest(
		`awake.runRequest("collections/auth/login.yaml");`,
		&request,
		map[string]string{},
		nil,
		execute,
	)
	if outcome.Error != "" {
		t.Fatalf("unexpected script error: %s", outcome.Error)
	}
	if !called {
		t.Error("expected the request to run")
	}
}

func TestRunRequestReportsAFailure(t *testing.T) {
	request := domain.Request{Method: domain.MethodGet, URL: "https://example.com"}

	outcome := NewRunner().BeforeRequest(
		`awake.runRequest("collections/auth/login.yaml"); awake.setVariable("unreachable", "yes");`,
		&request,
		map[string]string{},
		nil,
		func(string, time.Duration) (domain.ScriptResponse, error) {
			return domain.ScriptResponse{}, errors.New("collections: entry not found")
		},
	)

	if !strings.Contains(outcome.Error, "entry not found") {
		t.Fatalf("expected the failure to surface, got %q", outcome.Error)
	}
}

func TestRunRequestValidatesItsArguments(t *testing.T) {
	request := domain.Request{Method: domain.MethodGet, URL: "https://example.com"}
	execute := func(string, time.Duration) (domain.ScriptResponse, error) {
		t.Error("the request must not run")
		return domain.ScriptResponse{}, nil
	}

	for _, script := range []string{
		`awake.runRequest();`,
		`awake.runRequest("");`,
		`awake.runRequest("collections/auth/login.yaml", { timeout: 0 });`,
		`awake.runRequest("collections/auth/login.yaml", { timeout: -5 });`,
	} {
		outcome := NewRunner().BeforeRequest(script, &request, map[string]string{}, nil, execute)
		if outcome.Error == "" {
			t.Errorf("expected %q to fail", script)
		}
	}
}

// Regression: a request run from a script has its own timeout, so its duration
// must not trip the five second budget of the script.
func TestRunRequestPausesTheScriptTimeout(t *testing.T) {
	runner := &Runner{timeout: 40 * time.Millisecond}
	request := domain.Request{Method: domain.MethodGet, URL: "https://example.com"}
	variables := map[string]string{}

	outcome := runner.BeforeRequest(
		`awake.runRequest("collections/auth/login.yaml", { timeout: 1000 }); awake.setVariable("done", "yes");`,
		&request,
		variables,
		nil,
		func(string, time.Duration) (domain.ScriptResponse, error) {
			time.Sleep(120 * time.Millisecond)
			return domain.ScriptResponse{StatusCode: 200}, nil
		},
	)

	if outcome.Error != "" {
		t.Fatalf("expected the request to pause the script timeout: %s", outcome.Error)
	}
	if variables["done"] != "yes" {
		t.Errorf("expected the script to finish, got %v", variables)
	}
}
