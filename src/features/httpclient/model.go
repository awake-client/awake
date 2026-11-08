// Package httpclient executes requests and streams their responses.
package httpclient

import (
	"errors"
	"time"

	"github.com/awake-client/awake/src/domain"
)

const (
	// DefaultTimeout bounds a single exchange.
	DefaultTimeout = 60 * time.Second
	// MaximumBodySize stops writing a response that would fill the disk.
	MaximumBodySize int64 = 512 << 20
	// PreviewSize is the number of leading bytes returned with the metadata.
	PreviewSize = 256 << 10
	// MaximumChunkSize bounds one call to ReadChunk.
	MaximumChunkSize = 1 << 20
)

var (
	// ErrUnsupportedBody is returned for body types not handled yet.
	ErrUnsupportedBody = errors.New("httpclient: unsupported body type")
	// ErrResponseNotFound is returned when a response expired or was closed.
	ErrResponseNotFound = errors.New("httpclient: response not found")
	// ErrMissingPathParameter is returned when a :name segment of the URL
	// has no value.
	ErrMissingPathParameter = errors.New("httpclient: path parameter has no value")
	// ErrInvalidChunk is returned for an out-of-range chunk request.
	ErrInvalidChunk = errors.New("httpclient: invalid chunk")
	// ErrOAuth2AuthorizationRequired is returned when an OAuth 2 request needs
	// the user to authorize it before it can be sent.
	ErrOAuth2AuthorizationRequired = errors.New("httpclient: oauth2 authorization required")
)

// Response describes an exchange. The body itself stays on disk and is read
// through chunks, so a large payload never crosses the bridge at once.
type Response struct {
	ID                   string
	StatusCode           int
	Status               string
	Headers              map[string][]string
	DurationMilliseconds int64
	Size                 int64
	ContentType          string
	Preview              string
	Encoding             string
	Truncated            bool
	HistoryWarning       string
	PreRequestOutcome    domain.ScriptOutcome
	PostResponseOutcome  domain.ScriptOutcome
}

// Chunk is one slice of a stored response body.
type Chunk struct {
	Content    string
	Encoding   string
	NextOffset int64
	HasMore    bool
}
