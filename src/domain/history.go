package domain

// HistoryEntry is one executed request recorded in the personal history.
type HistoryEntry struct {
	ID                   int64
	RequestPath          string
	RequestName          string
	Method               string
	URL                  string
	StatusCode           int
	DurationMilliseconds int64
	SizeBytes            int64
	Environment          string
	ExecutedAt           string
	// Exchange holds what was sent and received. Lists leave it empty; it is
	// loaded for a single entry.
	Exchange *HistoryExchange
}

// HistoryExchange is the payload of an executed request and the response it
// received, as stored in the history. Credentials are redacted before they
// are recorded.
type HistoryExchange struct {
	RequestHeaders    map[string][]string
	RequestBody       string
	ResponseHeaders   map[string][]string
	ResponseBody      string
	ResponseEncoding  string
	ResponseTruncated bool
}
