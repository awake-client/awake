package variables

import (
	"errors"
	"reflect"
	"testing"
)

func TestInterpolateAppliesLayerPrecedence(t *testing.T) {
	workspaceDefaults := map[string]string{
		"baseUrl":  "https://default.example.com",
		"pageSize": "10",
	}
	sharedEnvironment := map[string]string{
		"baseUrl": "https://shared.example.com",
	}
	localOverrides := map[string]string{
		"pageSize": "50",
	}

	result, err := Interpolate(
		"{{baseUrl}}/users?pageSize={{pageSize}}",
		workspaceDefaults,
		sharedEnvironment,
		localOverrides,
	)
	if err != nil {
		t.Fatalf("interpolate: %v", err)
	}

	expected := "https://shared.example.com/users?pageSize=50"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestInterpolateAcceptsWhitespaceAroundNames(t *testing.T) {
	result, err := Interpolate("{{ baseUrl }}/users", map[string]string{"baseUrl": "https://example.com"})
	if err != nil {
		t.Fatalf("interpolate: %v", err)
	}
	if result != "https://example.com/users" {
		t.Errorf("unexpected result %q", result)
	}
}

func TestInterpolateReportsMissingVariables(t *testing.T) {
	_, err := Interpolate("{{missingTwo}}/{{missingOne}}", map[string]string{})
	if err == nil {
		t.Fatal("expected an error")
	}

	var missingError *MissingVariablesError
	if !errors.As(err, &missingError) {
		t.Fatalf("expected MissingVariablesError, got %v", err)
	}

	expected := []string{"missingOne", "missingTwo"}
	if !reflect.DeepEqual(missingError.Names, expected) {
		t.Errorf("expected %v, got %v", expected, missingError.Names)
	}
}

func TestInterpolateLeavesMalformedPlaceholdersUntouched(t *testing.T) {
	input := "{{unclosed and {{ }} stay literal"
	result, err := Interpolate(input)
	if err != nil {
		t.Fatalf("interpolate: %v", err)
	}
	if result != input {
		t.Errorf("expected %q, got %q", input, result)
	}
}

func TestInterpolateWithoutPlaceholders(t *testing.T) {
	result, err := Interpolate("https://example.com/users")
	if err != nil {
		t.Fatalf("interpolate: %v", err)
	}
	if result != "https://example.com/users" {
		t.Errorf("unexpected result %q", result)
	}
}
