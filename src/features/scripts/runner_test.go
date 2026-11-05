package scripts

import (
	"strings"
	"testing"
	"time"

	"github.com/clementherve/awake/src/domain"
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
	`, &request, variables)

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
	`, &request, map[string]string{})

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
	`, domain.Request{Name: "List users"}, response, variables)

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
	outcome := runner.BeforeRequest("while (true) {}", &request, map[string]string{})

	if outcome.Error == "" {
		t.Fatal("expected the script to be interrupted")
	}
	if time.Since(startedAt) > 5*time.Second {
		t.Errorf("the script took too long: %s", time.Since(startedAt))
	}
}

func TestEmptyScriptReturnsAnEmptyOutcome(t *testing.T) {
	request := domain.Request{Name: "List users", Method: domain.MethodGet, URL: "https://example.com"}

	outcome := NewRunner().BeforeRequest("  \n ", &request, map[string]string{})
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
