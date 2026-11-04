package git

import (
	"context"
	"strings"
	"testing"
)

func TestOpenReportsMissingGitExecutable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	_, err := Open(context.Background(), t.TempDir())
	if err == nil {
		t.Fatal("expected an error when git is not available")
	}
	if !strings.Contains(err.Error(), "executable not found") {
		t.Fatalf("expected an explicit missing executable error, got %v", err)
	}
}
