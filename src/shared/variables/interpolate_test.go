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

func TestInterpolateReadsDynamicVariablesComputedByPlugins(t *testing.T) {
	result, err := Interpolate("{{$awsDate}}/{{name}}", map[string]string{"name": "ada"}, map[string]string{"$awsDate": "20261125"})
	if err != nil {
		t.Fatalf("interpolate: %v", err)
	}
	if result != "20261125/ada" {
		t.Errorf("unexpected result %q", result)
	}

	var missing *MissingVariablesError
	if _, err := Interpolate("{{$unknown}}"); !errors.As(err, &missing) || missing.Names[0] != "$unknown" {
		t.Errorf("expected $unknown to be missing, got %v", err)
	}
}

func TestInterpolateKeepsBuiltInDynamicVariablesFirst(t *testing.T) {
	result, err := Interpolate("{{$timestamp}}", map[string]string{"$timestamp": "overridden"})
	if err != nil {
		t.Fatalf("interpolate: %v", err)
	}
	if result == "overridden" {
		t.Error("a layer must not override a built-in dynamic variable")
	}
}

func TestExternalDynamicNames(t *testing.T) {
	names := ExternalDynamicNames(`{{$awsDate}} {{ $uuid }} {{name}} {{$awsDate}} {{$region}}`)

	if !reflect.DeepEqual(names, []string{"$awsDate", "$region"}) {
		t.Errorf("unexpected names %v", names)
	}
	if names := ExternalDynamicNames("no placeholder"); len(names) != 0 {
		t.Errorf("expected no name, got %v", names)
	}
}
