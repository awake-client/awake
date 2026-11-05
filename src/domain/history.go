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
}
