package assistant

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestSuggestMergeRunsClaudeWithoutTools(t *testing.T) {
	var arguments []string
	var prompt string
	service := &Service{
		lookup: func() (string, error) { return "/bin/claude", nil },
		run: func(_ context.Context, _ string, _ string, input string, args ...string) (string, error) {
			arguments = args
			prompt = input
			return "SUMMARY: Kept the v2 path and the all filter.\n```yaml\nname: Orders\nurl: https://shop.test/v2/orders?all=true\n```\n", nil
		},
	}

	suggestion, err := service.SuggestMerge("/workspace", "collections/shop/orders.yaml", "url: a", "url: b", "url: c")
	if err != nil {
		t.Fatalf("suggest merge: %v", err)
	}
	if suggestion.Summary != "Kept the v2 path and the all filter." {
		t.Errorf("unexpected summary %q", suggestion.Summary)
	}
	if suggestion.Content != "name: Orders\nurl: https://shop.test/v2/orders?all=true\n" {
		t.Errorf("the code block must be removed, got %q", suggestion.Content)
	}
	joined := strings.Join(arguments, " ")
	if !strings.Contains(joined, "--print") || !strings.Contains(joined, "--tools ") {
		t.Errorf("claude must run non interactively without tools, got %q", joined)
	}
	for _, expected := range []string{"collections/shop/orders.yaml", "url: a", "url: b", "url: c"} {
		if !strings.Contains(prompt, expected) {
			t.Errorf("the prompt must contain %q", expected)
		}
	}
}

func TestSuggestMergeReportsAMissingInstallation(t *testing.T) {
	service := &Service{lookup: func() (string, error) { return "", ErrNotInstalled }}
	if _, err := service.SuggestMerge("/workspace", "a.yaml", "", "", ""); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("expected ErrNotInstalled, got %v", err)
	}
	if service.Status().Installed {
		t.Fatal("a missing binary must not be reported as installed")
	}
}

func TestParseSuggestionRejectsEmptyAnswers(t *testing.T) {
	if _, err := parseSuggestion("SUMMARY: nothing\n"); err == nil {
		t.Fatal("expected an empty merge to be rejected")
	}
}
