package variables

import (
	"errors"
	"regexp"
	"testing"
	"time"
)

func TestInterpolateResolvesDynamicVariables(t *testing.T) {
	now = func() time.Time { return time.Date(2026, 11, 6, 9, 30, 0, 0, time.UTC) }
	t.Cleanup(func() { now = time.Now })

	result, err := Interpolate("{{$timestamp}}|{{ $isoTimestamp }}|{{$date}}")
	if err != nil {
		t.Fatalf("interpolate: %v", err)
	}
	if result != "1793957400|2026-11-06T09:30:00Z|2026-11-06" {
		t.Errorf("unexpected result %q", result)
	}
}

func TestInterpolateGeneratesAFreshValuePerOccurrence(t *testing.T) {
	result, err := Interpolate("{{$uuid}} {{$uuid}}")
	if err != nil {
		t.Fatalf("interpolate: %v", err)
	}

	uuidPattern := `[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}`
	matches := regexp.MustCompile(uuidPattern).FindAllString(result, -1)
	if len(matches) != 2 || matches[0] == matches[1] {
		t.Errorf("expected two different UUIDs, got %q", result)
	}
}

func TestInterpolateReportsUnknownDynamicVariables(t *testing.T) {
	_, err := Interpolate("{{$nope}}")

	var missingError *MissingVariablesError
	if !errors.As(err, &missingError) || missingError.Names[0] != "$nope" {
		t.Fatalf("expected the unknown dynamic variable to be reported, got %v", err)
	}
}

func TestDynamicVariablesAreNotOverriddenByLayers(t *testing.T) {
	result, err := Interpolate("{{$randomInt}}", map[string]string{"$randomInt": "fixed"})
	if err != nil {
		t.Fatalf("interpolate: %v", err)
	}
	if result == "fixed" {
		t.Error("a layer must not shadow a dynamic variable")
	}
}
