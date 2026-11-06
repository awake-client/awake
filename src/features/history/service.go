// Package history records executed requests in a per-workspace SQLite
// database. History is personal: it lives under .awake and is never committed.
package history

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// ErrEntryNotFound is returned for a history entry that no longer exists.
var ErrEntryNotFound = errors.New("history: entry not found")

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
CREATE INDEX IF NOT EXISTS history_by_request ON history(request_path, id DESC);
CREATE TABLE IF NOT EXISTS history_exchange (
	entry_id INTEGER PRIMARY KEY,
	request_headers TEXT NOT NULL DEFAULT '{}',
	request_body TEXT NOT NULL DEFAULT '',
	response_headers TEXT NOT NULL DEFAULT '{}',
	response_body TEXT NOT NULL DEFAULT '',
	response_encoding TEXT NOT NULL DEFAULT '',
	response_truncated INTEGER NOT NULL DEFAULT 0
);
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

	result, err := transaction.Exec(`
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

	if entry.Exchange != nil {
		identifier, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("read history entry id: %w", err)
		}
		if err := insertExchange(transaction, identifier, *entry.Exchange); err != nil {
			return err
		}
	}

	_, err = transaction.Exec(
		`DELETE FROM history WHERE id <= (SELECT COALESCE(MAX(id), 0) FROM history) - ?`,
		maximumEntryCount,
	)
	if err != nil {
		return fmt.Errorf("prune history: %w", err)
	}
	if _, err := transaction.Exec(`DELETE FROM history_exchange WHERE entry_id NOT IN (SELECT id FROM history)`); err != nil {
		return fmt.Errorf("prune history payloads: %w", err)
	}

	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit history entry: %w", err)
	}

	return nil
}

// List returns history entries from newest to oldest. Pass a zero beforeID for
// the first page and the last seen identifier for the next one.
func (service *Service) List(workspaceRoot string, limit int, beforeID int64) ([]domain.HistoryEntry, error) {
	return service.list(workspaceRoot, "", limit, beforeID)
}

// ListForRequest returns the executions of one request, newest first.
func (service *Service) ListForRequest(workspaceRoot string, requestPath string, limit int, beforeID int64) ([]domain.HistoryEntry, error) {
	if requestPath == "" {
		return []domain.HistoryEntry{}, nil
	}
	return service.list(workspaceRoot, requestPath, limit, beforeID)
}

func (service *Service) list(workspaceRoot string, requestPath string, limit int, beforeID int64) ([]domain.HistoryEntry, error) {
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
	arguments := make([]any, 0, 3)
	conditions := make([]string, 0, 2)

	if requestPath != "" {
		conditions = append(conditions, `request_path = ?`)
		arguments = append(arguments, requestPath)
	}
	if beforeID > 0 {
		conditions = append(conditions, `id < ?`)
		arguments = append(arguments, beforeID)
	}
	if len(conditions) > 0 {
		query += ` WHERE ` + strings.Join(conditions, ` AND `)
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
	if _, err := database.Exec(`DELETE FROM history_exchange`); err != nil {
		return fmt.Errorf("clear history payloads: %w", err)
	}

	return nil
}

// Delete removes one history entry and its payload.
func (service *Service) Delete(workspaceRoot string, identifier int64) error {
	database, err := service.databaseFor(workspaceRoot)
	if err != nil {
		return err
	}
	return deleteWhere(database, `id = ?`, identifier)
}

// ClearForRequest removes every execution of one request.
func (service *Service) ClearForRequest(workspaceRoot string, requestPath string) error {
	if requestPath == "" {
		return nil
	}
	database, err := service.databaseFor(workspaceRoot)
	if err != nil {
		return err
	}
	return deleteWhere(database, `request_path = ?`, requestPath)
}

// deleteWhere removes the matching entries and their payloads atomically.
func deleteWhere(database *sql.DB, condition string, argument any) error {
	transaction, err := database.Begin()
	if err != nil {
		return fmt.Errorf("begin history deletion: %w", err)
	}
	defer transaction.Rollback()

	if _, err := transaction.Exec(`DELETE FROM history_exchange WHERE entry_id IN (SELECT id FROM history WHERE `+condition+`)`, argument); err != nil {
		return fmt.Errorf("delete history payloads: %w", err)
	}
	if _, err := transaction.Exec(`DELETE FROM history WHERE `+condition, argument); err != nil {
		return fmt.Errorf("delete history entries: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit history deletion: %w", err)
	}
	return nil
}

// Entry returns one history entry with the payload sent and the response
// received, when they were recorded.
func (service *Service) Entry(workspaceRoot string, identifier int64) (domain.HistoryEntry, error) {
	database, err := service.databaseFor(workspaceRoot)
	if err != nil {
		return domain.HistoryEntry{}, err
	}

	var entry domain.HistoryEntry
	err = database.QueryRow(`SELECT
	id, request_path, request_name, method, url, status_code,
	duration_milliseconds, size_bytes, environment, executed_at
FROM history WHERE id = ?`, identifier).Scan(
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
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.HistoryEntry{}, ErrEntryNotFound
	}
	if err != nil {
		return domain.HistoryEntry{}, fmt.Errorf("read history entry: %w", err)
	}

	var exchange domain.HistoryExchange
	var requestHeaders, responseHeaders string
	var truncated int
	err = database.QueryRow(`SELECT
	request_headers, request_body, response_headers, response_body,
	response_encoding, response_truncated
FROM history_exchange WHERE entry_id = ?`, identifier).Scan(
		&requestHeaders,
		&exchange.RequestBody,
		&responseHeaders,
		&exchange.ResponseBody,
		&exchange.ResponseEncoding,
		&truncated,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return entry, nil
	}
	if err != nil {
		return domain.HistoryEntry{}, fmt.Errorf("read history payload: %w", err)
	}
	if err := json.Unmarshal([]byte(requestHeaders), &exchange.RequestHeaders); err != nil {
		return domain.HistoryEntry{}, fmt.Errorf("decode request headers: %w", err)
	}
	if err := json.Unmarshal([]byte(responseHeaders), &exchange.ResponseHeaders); err != nil {
		return domain.HistoryEntry{}, fmt.Errorf("decode response headers: %w", err)
	}
	exchange.ResponseTruncated = truncated != 0
	entry.Exchange = &exchange
	return entry, nil
}

// RenameRequests follows requests that moved: entries recorded under a path,
// or under a folder path, are attached to the new location.
func (service *Service) RenameRequests(workspaceRoot string, from string, to string) error {
	if from == "" || to == "" || from == to {
		return nil
	}
	database, err := service.databaseFor(workspaceRoot)
	if err != nil {
		return err
	}
	_, err = database.Exec(`
UPDATE history
SET request_path = ? || substr(request_path, ?)
WHERE request_path = ? OR substr(request_path, 1, ?) = ?`,
		to, len(from)+1, from, len(from)+1, from+"/",
	)
	if err != nil {
		return fmt.Errorf("rename history entries: %w", err)
	}
	return nil
}

func insertExchange(transaction *sql.Tx, identifier int64, exchange domain.HistoryExchange) error {
	requestHeaders, err := json.Marshal(nonNilHeaders(exchange.RequestHeaders))
	if err != nil {
		return fmt.Errorf("encode request headers: %w", err)
	}
	responseHeaders, err := json.Marshal(nonNilHeaders(exchange.ResponseHeaders))
	if err != nil {
		return fmt.Errorf("encode response headers: %w", err)
	}
	truncated := 0
	if exchange.ResponseTruncated {
		truncated = 1
	}
	_, err = transaction.Exec(`
INSERT INTO history_exchange (
	entry_id, request_headers, request_body, response_headers, response_body,
	response_encoding, response_truncated
) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		identifier,
		string(requestHeaders),
		exchange.RequestBody,
		string(responseHeaders),
		exchange.ResponseBody,
		exchange.ResponseEncoding,
		truncated,
	)
	if err != nil {
		return fmt.Errorf("insert history payload: %w", err)
	}
	return nil
}

func nonNilHeaders(headers map[string][]string) map[string][]string {
	if headers == nil {
		return map[string][]string{}
	}
	return headers
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
