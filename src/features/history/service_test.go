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

func TestHistoryKeepsTheExchangeOfEachRequest(t *testing.T) {
	service := NewService()
	workspaceRoot := t.TempDir()

	record := func(path string, status int, body string) {
		t.Helper()
		err := service.Record(workspaceRoot, domain.HistoryEntry{
			RequestPath:          path,
			RequestName:          "Request",
			Method:               "POST",
			URL:                  "https://example.com/orders",
			StatusCode:           status,
			DurationMilliseconds: 42,
			SizeBytes:            int64(len(body)),
			Exchange: &domain.HistoryExchange{
				RequestHeaders:  map[string][]string{"Content-Type": {"application/json"}},
				RequestBody:     `{"amount":42}`,
				ResponseHeaders: map[string][]string{"X-Request-Id": {"abc"}},
				ResponseBody:    body,
			},
		})
		if err != nil {
			t.Fatalf("record: %v", err)
		}
	}
	record("collections/shop/orders/create.yaml", 201, `{"id":1}`)
	record("collections/shop/ping.yaml", 200, "pong")
	record("collections/shop/orders/create.yaml", 409, `{"error":"duplicate"}`)

	entries, err := service.ListForRequest(workspaceRoot, "collections/shop/orders/create.yaml", 10, 0)
	if err != nil {
		t.Fatalf("list for request: %v", err)
	}
	if len(entries) != 2 || entries[0].StatusCode != 409 || entries[1].StatusCode != 201 {
		t.Fatalf("expected the two executions of the request, newest first: %+v", entries)
	}
	if entries[0].Exchange != nil {
		t.Error("lists must stay light and leave the exchange out")
	}

	entry, err := service.Entry(workspaceRoot, entries[0].ID)
	if err != nil {
		t.Fatalf("entry: %v", err)
	}
	if entry.Exchange == nil ||
		entry.Exchange.RequestBody != `{"amount":42}` ||
		entry.Exchange.ResponseBody != `{"error":"duplicate"}` ||
		entry.Exchange.RequestHeaders["Content-Type"][0] != "application/json" ||
		entry.DurationMilliseconds != 42 {
		t.Fatalf("unexpected exchange: %+v %+v", entry, entry.Exchange)
	}

	if err := service.RenameRequests(workspaceRoot, "collections/shop/orders", "local/scratch/orders"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	moved, err := service.ListForRequest(workspaceRoot, "local/scratch/orders/create.yaml", 10, 0)
	if err != nil || len(moved) != 2 {
		t.Fatalf("expected the history to follow the moved request, got %+v, %v", moved, err)
	}
	untouched, err := service.ListForRequest(workspaceRoot, "collections/shop/ping.yaml", 10, 0)
	if err != nil || len(untouched) != 1 {
		t.Fatalf("other requests keep their history, got %+v, %v", untouched, err)
	}

	if _, err := service.Entry(workspaceRoot, 9999); err != ErrEntryNotFound {
		t.Fatalf("expected ErrEntryNotFound, got %v", err)
	}
	if err := service.Clear(workspaceRoot); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if _, err := service.Entry(workspaceRoot, entries[0].ID); err != ErrEntryNotFound {
		t.Fatalf("clear must remove entries, got %v", err)
	}
}

func TestDeleteOneEntryOrTheHistoryOfOneRequest(t *testing.T) {
	service := NewService()
	workspaceRoot := t.TempDir()
	record := func(path string) {
		t.Helper()
		err := service.Record(workspaceRoot, domain.HistoryEntry{
			RequestPath: path,
			RequestName: "Request",
			Method:      "GET",
			URL:         "https://example.com",
			StatusCode:  200,
			Exchange:    &domain.HistoryExchange{ResponseBody: "ok"},
		})
		if err != nil {
			t.Fatalf("record: %v", err)
		}
	}
	record("collections/shop/a.yaml")
	record("collections/shop/a.yaml")
	record("collections/shop/a.yaml")
	record("collections/shop/b.yaml")

	entries, err := service.ListForRequest(workspaceRoot, "collections/shop/a.yaml", 10, 0)
	if err != nil || len(entries) != 3 {
		t.Fatalf("expected three entries, got %+v, %v", entries, err)
	}

	if err := service.Delete(workspaceRoot, entries[1].ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	remaining, _ := service.ListForRequest(workspaceRoot, "collections/shop/a.yaml", 10, 0)
	if len(remaining) != 2 || remaining[0].ID != entries[0].ID || remaining[1].ID != entries[2].ID {
		t.Fatalf("only the deleted entry must go, got %+v", remaining)
	}
	if _, err := service.Entry(workspaceRoot, entries[1].ID); err != ErrEntryNotFound {
		t.Fatalf("expected the deleted entry to be gone, got %v", err)
	}

	if err := service.ClearForRequest(workspaceRoot, "collections/shop/a.yaml"); err != nil {
		t.Fatalf("clear for request: %v", err)
	}
	cleared, _ := service.ListForRequest(workspaceRoot, "collections/shop/a.yaml", 10, 0)
	other, _ := service.ListForRequest(workspaceRoot, "collections/shop/b.yaml", 10, 0)
	if len(cleared) != 0 || len(other) != 1 {
		t.Fatalf("clearing a request must keep the others: %d left, %d other", len(cleared), len(other))
	}
}
