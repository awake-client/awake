package history

import (
	"testing"

	"github.com/clementherve/awake/src/domain"
)

func TestRecordStoresEntriesNewestFirst(t *testing.T) {
	service := NewService()
	workspaceRoot := t.TempDir()

	for _, name := range []string{"First", "Second", "Third"} {
		if err := service.Record(workspaceRoot, domain.HistoryEntry{
			RequestPath:          "collections/platform/users/list.yaml",
			RequestName:          name,
			Method:               "GET",
			URL:                  "https://example.com/users",
			StatusCode:           200,
			DurationMilliseconds: 12,
			SizeBytes:            34,
			Environment:          "development",
			ExecutedAt:           "2026-11-05T10:00:00Z",
		}); err != nil {
			t.Fatalf("record history: %v", err)
		}
	}

	entries, err := service.List(workspaceRoot, 2, 0)
	if err != nil {
		t.Fatalf("list history: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].RequestName != "Third" || entries[1].RequestName != "Second" {
		t.Errorf("unexpected order: %+v", entries)
	}
	if entries[0].StatusCode != 200 || entries[0].Environment != "development" {
		t.Errorf("unexpected entry: %+v", entries[0])
	}

	nextPage, err := service.List(workspaceRoot, 2, entries[1].ID)
	if err != nil {
		t.Fatalf("list next page: %v", err)
	}
	if len(nextPage) != 1 || nextPage[0].RequestName != "First" {
		t.Errorf("unexpected next page: %+v", nextPage)
	}
}

func TestRecordFillsExecutedAt(t *testing.T) {
	service := NewService()
	workspaceRoot := t.TempDir()

	if err := service.Record(workspaceRoot, domain.HistoryEntry{
		RequestPath: "collections/platform/users/list.yaml",
		RequestName: "List users",
		Method:      "GET",
		URL:         "https://example.com/users",
		StatusCode:  200,
	}); err != nil {
		t.Fatalf("record history: %v", err)
	}

	entries, err := service.List(workspaceRoot, 0, 0)
	if err != nil {
		t.Fatalf("list history: %v", err)
	}
	if len(entries) != 1 || entries[0].ExecutedAt == "" {
		t.Fatalf("expected a timestamp, got %+v", entries)
	}
}

func TestWorkspacesKeepSeparateHistories(t *testing.T) {
	service := NewService()
	firstWorkspace := t.TempDir()
	secondWorkspace := t.TempDir()

	if err := service.Record(firstWorkspace, domain.HistoryEntry{
		RequestPath: "collections/first.yaml",
		RequestName: "First workspace",
		Method:      "GET",
		URL:         "https://example.com/first",
	}); err != nil {
		t.Fatalf("record first history: %v", err)
	}

	entries, err := service.List(secondWorkspace, 0, 0)
	if err != nil {
		t.Fatalf("list second history: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected an empty second history, got %+v", entries)
	}
}

func TestClearRemovesEntries(t *testing.T) {
	service := NewService()
	workspaceRoot := t.TempDir()

	if err := service.Record(workspaceRoot, domain.HistoryEntry{
		RequestPath: "collections/first.yaml",
		RequestName: "First",
		Method:      "GET",
		URL:         "https://example.com",
	}); err != nil {
		t.Fatalf("record history: %v", err)
	}

	if err := service.Clear(workspaceRoot); err != nil {
		t.Fatalf("clear history: %v", err)
	}

	entries, err := service.List(workspaceRoot, 0, 0)
	if err != nil {
		t.Fatalf("list history: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected no entries, got %+v", entries)
	}
}
