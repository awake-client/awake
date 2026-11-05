// Package history records executed requests in a per-workspace SQLite
// database. History is personal: it lives under .awake and is never committed.
package history

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/clementherve/awake/src/domain"

	_ "modernc.org/sqlite"
)

const (
	directoryName     = ".awake"
	databaseFileName  = "history.db"
	defaultPageSize   = 50
	maximumPageSize   = 500
	maximumEntryCount = 10000
)

const schema = `
CREATE TABLE IF NOT EXISTS history (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	request_path TEXT NOT NULL,
	request_name TEXT NOT NULL,
	method TEXT NOT NULL,
	url TEXT NOT NULL,
	status_code INTEGER NOT NULL,
	duration_milliseconds INTEGER NOT NULL,
	size_bytes INTEGER NOT NULL,
	environment TEXT NOT NULL DEFAULT '',
	executed_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS history_by_id ON history(id DESC);
`

// Service records and lists the requests executed in a workspace.
type Service struct {
	mutex     sync.Mutex
	databases map[string]*sql.DB
}

// NewService returns a history service.
func NewService() *Service {
	return &Service{databases: make(map[string]*sql.DB)}
}

// Record appends an executed request to the history.
func (service *Service) Record(workspaceRoot string, entry domain.HistoryEntry) error {
	if entry.ExecutedAt == "" {
		entry.ExecutedAt = time.Now().UTC().Format(time.RFC3339)
	}

	database, err := service.databaseFor(workspaceRoot)
	if err != nil {
		return err
	}

	transaction, err := database.Begin()
	if err != nil {
		return fmt.Errorf("begin history transaction: %w", err)
	}
	defer transaction.Rollback()

	_, err = transaction.Exec(`
INSERT INTO history (
	request_path, request_name, method, url, status_code,
	duration_milliseconds, size_bytes, environment, executed_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.RequestPath,
		entry.RequestName,
		entry.Method,
		entry.URL,
		entry.StatusCode,
		entry.DurationMilliseconds,
		entry.SizeBytes,
		entry.Environment,
		entry.ExecutedAt,
	)
	if err != nil {
		return fmt.Errorf("insert history entry: %w", err)
	}

	_, err = transaction.Exec(
		`DELETE FROM history WHERE id <= (SELECT COALESCE(MAX(id), 0) FROM history) - ?`,
		maximumEntryCount,
	)
	if err != nil {
		return fmt.Errorf("prune history: %w", err)
	}

	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit history entry: %w", err)
	}

	return nil
}

// List returns history entries from newest to oldest. Pass a zero beforeID for
// the first page and the last seen identifier for the next one.
func (service *Service) List(workspaceRoot string, limit int, beforeID int64) ([]domain.HistoryEntry, error) {
	if limit <= 0 {
		limit = defaultPageSize
	}
	if limit > maximumPageSize {
		limit = maximumPageSize
	}

	database, err := service.databaseFor(workspaceRoot)
	if err != nil {
		return nil, err
	}

	query := `SELECT
	id, request_path, request_name, method, url, status_code,
	duration_milliseconds, size_bytes, environment, executed_at
FROM history`
	arguments := make([]any, 0, 2)

	if beforeID > 0 {
		query += ` WHERE id < ?`
		arguments = append(arguments, beforeID)
	}

	query += ` ORDER BY id DESC LIMIT ?`
	arguments = append(arguments, limit)

	rows, err := database.Query(query, arguments...)
	if err != nil {
		return nil, fmt.Errorf("query history: %w", err)
	}
	defer rows.Close()

	entries := make([]domain.HistoryEntry, 0, limit)
	for rows.Next() {
		var entry domain.HistoryEntry
		if err := rows.Scan(
			&entry.ID,
			&entry.RequestPath,
			&entry.RequestName,
			&entry.Method,
			&entry.URL,
			&entry.StatusCode,
			&entry.DurationMilliseconds,
			&entry.SizeBytes,
			&entry.Environment,
			&entry.ExecutedAt,
		); err != nil {
			return nil, fmt.Errorf("scan history entry: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate history: %w", err)
	}

	return entries, nil
}

// Clear removes every history entry of a workspace.
func (service *Service) Clear(workspaceRoot string) error {
	database, err := service.databaseFor(workspaceRoot)
	if err != nil {
		return err
	}

	if _, err := database.Exec(`DELETE FROM history`); err != nil {
		return fmt.Errorf("clear history: %w", err)
	}

	return nil
}

func (service *Service) databaseFor(workspaceRoot string) (*sql.DB, error) {
	root := filepath.Clean(workspaceRoot)

	service.mutex.Lock()
	defer service.mutex.Unlock()

	if database, found := service.databases[root]; found {
		return database, nil
	}

	directory := filepath.Join(root, directoryName)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return nil, fmt.Errorf("create history directory: %w", err)
	}

	database, err := sql.Open("sqlite", filepath.Join(directory, databaseFileName))
	if err != nil {
		return nil, fmt.Errorf("open history database: %w", err)
	}
	database.SetMaxOpenConns(1)

	for _, pragma := range []string{
		`PRAGMA journal_mode = WAL`,
		`PRAGMA busy_timeout = 5000`,
	} {
		if _, err := database.Exec(pragma); err != nil {
			database.Close()
			return nil, fmt.Errorf("configure history database: %w", err)
		}
	}

	if _, err := database.Exec(schema); err != nil {
		database.Close()
		return nil, fmt.Errorf("create history schema: %w", err)
	}

	service.databases[root] = database

	return database, nil
}
