package httpclient

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/coder/websocket"

	"github.com/awake-client/awake/src/domain"
	"github.com/awake-client/awake/src/shared/variables"
)

const (
	// MaximumStreamEventSize truncates one event so a single message cannot
	// exhaust the frontend.
	MaximumStreamEventSize = 1 << 20
	// MaximumRetainedEvents bounds the events kept for one live stream.
	MaximumRetainedEvents = 2000
	// MaximumWebSocketMessageSize bounds an incoming WebSocket message.
	MaximumWebSocketMessageSize int64 = 8 << 20
	// streamHeaderTimeout bounds the wait for response headers.
	streamHeaderTimeout = 30 * time.Second
	// staleStreamAge is how long a finished stream stays readable.
	staleStreamAge = 2 * time.Hour
	// maximumOpenStreams bounds concurrent live connections.
	maximumOpenStreams = 32
)

var (
	// ErrNotStreaming is returned when a request uses the HTTP protocol.
	ErrNotStreaming = errors.New("httpclient: request does not use a streaming protocol")
	// ErrStreamNotFound is returned when a stream was released or expired.
	ErrStreamNotFound = errors.New("httpclient: stream not found")
	// ErrStreamClosed is returned when writing to a closed stream.
	ErrStreamClosed = errors.New("httpclient: stream closed")
	// ErrNotWebSocket is returned when writing to a non-WebSocket stream.
	ErrNotWebSocket = errors.New("httpclient: stream is not a WebSocket")
)

// StreamEvent is one event of a live SSE or WebSocket session.
type StreamEvent struct {
	Index                 int64
	Kind                  string
	Direction             string
	Name                  string
	EventID               string
	Data                  string
	Encoding              string
	TimestampMilliseconds int64
}

// Stream describes a live SSE or WebSocket session.
type Stream struct {
	ID                    string
	Protocol              domain.Protocol
	StatusCode            int
	Status                string
	Headers               map[string][]string
	StartedAtMilliseconds int64
	Closed                bool
	Error                 string
	PreRequestOutcome     domain.ScriptOutcome
}

// StreamEventPage is an incremental slice of a stream event log.
type StreamEventPage struct {
	Events    []StreamEvent
	NextIndex int64
	Closed    bool
	Error     string
}

type streamSession struct {
	id        string
	protocol  domain.Protocol
	startedAt time.Time

	mutex      sync.Mutex
	events     []StreamEvent
	baseIndex  int64
	nextIndex  int64
	closed     bool
	stopped    bool
	lastError  string
	statusCode int
	status     string
	headers    map[string][]string
	preRequest domain.ScriptOutcome

	cancel     context.CancelFunc
	context    context.Context
	closer     io.Closer
	connection *websocket.Conn
	writeMutex sync.Mutex
}

type streamRegistry struct {
	mutex    sync.Mutex
	sessions map[string]*streamSession
}

func newStreamRegistry() *streamRegistry {
	return &streamRegistry{sessions: make(map[string]*streamSession)}
}

func newStreamClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = streamHeaderTimeout

	return &http.Client{Transport: transport}
}

// OpenStream starts a server-sent events or WebSocket session. The connection
// stays open until CloseStream or ReleaseStream is called.
func (service *Service) OpenStream(workspaceRoot string, requestPath string, environmentFileName string) (Stream, error) {
	request, err := service.source.LoadRequest(workspaceRoot, requestPath)
	if err != nil {
		return Stream{}, err
	}

	labeledLayers, err := service.source.LoadVariableLayers(workspaceRoot, requestPath, environmentFileName)
	if err != nil {
		return Stream{}, err
	}
	layers := variables.Maps(labeledLayers)

	if err := applyEnvironmentAuthentication(&request, layers); err != nil {
		return Stream{}, err
	}

	layers, preRequestOutcome := service.runPreRequest(
		workspaceRoot,
		requestPath,
		environmentFileName,
		&request,
		layers,
		service.environmentResolver(workspaceRoot, requestPath),
		sendOptions{chain: []string{requestPath}},
	)
	service.saveEnvironmentChanges(workspaceRoot, requestPath, environmentFileName, &preRequestOutcome)

	oauth2Token, err := service.oauth2Token(workspaceRoot, requestPath, environmentFileName, request, layers)
	if err != nil {
		return Stream{}, err
	}

	switch request.Protocol {
	case domain.ProtocolSSE:
		return service.openServerSentEvents(request, layers, oauth2Token, preRequestOutcome)
	case domain.ProtocolWebSocket:
		return service.openWebSocket(request, layers, oauth2Token, preRequestOutcome)
	default:
		return Stream{}, ErrNotStreaming
	}
}

// ReadStreamEvents returns the events starting at fromIndex. Callers pass the
// NextIndex of the previous page, and zero for the first read.
func (service *Service) ReadStreamEvents(identifier string, fromIndex int64) (StreamEventPage, error) {
	session, found := service.streams.find(identifier)
	if !found {
		return StreamEventPage{}, ErrStreamNotFound
	}

	session.mutex.Lock()
	defer session.mutex.Unlock()

	start := fromIndex
	if start < session.baseIndex {
		start = session.baseIndex
	}

	events := make([]StreamEvent, 0, len(session.events))
	for _, event := range session.events {
		if event.Index >= start {
			events = append(events, event)
		}
	}

	return StreamEventPage{
		Events:    events,
		NextIndex: session.nextIndex,
		Closed:    session.closed,
		Error:     session.lastError,
	}, nil
}

// SendStreamMessage writes a WebSocket message. encoding is utf-8 for text or
// base64 for binary payloads.
func (service *Service) SendStreamMessage(identifier string, message string, encoding string) error {
	session, found := service.streams.find(identifier)
	if !found {
		return ErrStreamNotFound
	}
	if session.protocol != domain.ProtocolWebSocket {
		return ErrNotWebSocket
	}
	if session.connection == nil || session.isClosed() {
		return ErrStreamClosed
	}

	payload := []byte(message)
	messageType := websocket.MessageText
	if encoding == "base64" {
		decoded, err := base64.StdEncoding.DecodeString(message)
		if err != nil {
			return fmt.Errorf("decode message: %w", err)
		}
		payload = decoded
		messageType = websocket.MessageBinary
	}

	writeContext, cancel := context.WithTimeout(session.context, streamHeaderTimeout)
	defer cancel()

	session.writeMutex.Lock()
	err := session.connection.Write(writeContext, messageType, payload)
	session.writeMutex.Unlock()
	if err != nil {
		return fmt.Errorf("send message: %w", err)
	}

	session.append("message", "out", "", message, encoding)
	return nil
}

// CloseStream stops a live stream but keeps its events readable until they are
// released.
func (service *Service) CloseStream(identifier string) error {
	session, found := service.streams.find(identifier)
	if !found {
		return ErrStreamNotFound
	}

	session.stop()
	return nil
}

// ReleaseStream stops a stream and forgets its events.
func (service *Service) ReleaseStream(identifier string) error {
	session, found := service.streams.remove(identifier)
	if !found {
		return ErrStreamNotFound
	}

	session.stop()
	return nil
}

func (service *Service) openServerSentEvents(request domain.Request, layers []map[string]string, oauth2Token string, preRequestOutcome domain.ScriptOutcome) (Stream, error) {
	streamContext, cancel := context.WithCancel(context.Background())

	httpRequest, err := buildHTTPRequestWithContext(streamContext, request, layers, oauth2Token)
	if err != nil {
		cancel()
		return Stream{}, err
	}
	if httpRequest.Header.Get("Accept") == "" {
		httpRequest.Header.Set("Accept", "text/event-stream")
	}

	httpResponse, err := service.streamClient.Do(httpRequest)
	if err != nil {
		cancel()
		return Stream{}, fmt.Errorf("open event stream: %w", err)
	}

	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		defer httpResponse.Body.Close()
		defer cancel()

		preview, _ := io.ReadAll(io.LimitReader(httpResponse.Body, 8<<10))
		message := strings.TrimSpace(string(preview))
		if message == "" {
			return Stream{}, fmt.Errorf("open event stream: server responded %s", httpResponse.Status)
		}
		return Stream{}, fmt.Errorf("open event stream: server responded %s: %s", httpResponse.Status, message)
	}

	session := service.streams.register(domain.ProtocolSSE, httpResponse.StatusCode, httpResponse.Status, httpResponse.Header)
	session.context = streamContext
	session.cancel = cancel
	session.closer = httpResponse.Body
	session.preRequest = preRequestOutcome

	go service.readServerSentEvents(session, httpResponse.Body)

	return session.snapshot(), nil
}

func (service *Service) openWebSocket(request domain.Request, layers []map[string]string, oauth2Token string, preRequestOutcome domain.ScriptOutcome) (Stream, error) {
	streamContext, cancel := context.WithCancel(context.Background())

	httpRequest, err := buildHTTPRequestWithContext(streamContext, request, layers, oauth2Token)
	if err != nil {
		cancel()
		return Stream{}, err
	}

	webSocketURL, err := webSocketURL(httpRequest.URL)
	if err != nil {
		cancel()
		return Stream{}, err
	}

	connection, httpResponse, err := websocket.Dial(streamContext, webSocketURL, &websocket.DialOptions{
		HTTPHeader: httpRequest.Header,
	})
	if err != nil {
		cancel()
		return Stream{}, fmt.Errorf("open websocket: %w", err)
	}
	connection.SetReadLimit(MaximumWebSocketMessageSize)

	statusCode := http.StatusSwitchingProtocols
	status := "101 Switching Protocols"
	headers := map[string][]string{}
	if httpResponse != nil {
		statusCode = httpResponse.StatusCode
		status = httpResponse.Status
		headers = httpResponse.Header
	}

	session := service.streams.register(domain.ProtocolWebSocket, statusCode, status, headers)
	session.context = streamContext
	session.cancel = cancel
	session.connection = connection
	session.preRequest = preRequestOutcome
	session.append("open", "system", "", "Connected", "utf-8")

	if request.Body != nil && request.Body.Type != domain.BodyNone {
		content, err := interpolate(request.Body.Content, layers)
		if err == nil && content != "" {
			writeContext, writeCancel := context.WithTimeout(streamContext, streamHeaderTimeout)
			writeError := connection.Write(writeContext, websocket.MessageText, []byte(content))
			writeCancel()
			if writeError != nil {
				connection.CloseNow()
				cancel()
				service.streams.remove(session.id)
				return Stream{}, fmt.Errorf("send initial message: %w", writeError)
			}
			session.append("message", "out", "", content, "utf-8")
		}
	}

	go service.readWebSocketMessages(session, connection)

	return session.snapshot(), nil
}

func webSocketURL(requestURL *url.URL) (string, error) {
	converted := *requestURL
	switch strings.ToLower(converted.Scheme) {
	case "http":
		converted.Scheme = "ws"
	case "https":
		converted.Scheme = "wss"
	case "ws", "wss":
	default:
		return "", fmt.Errorf("open websocket: unsupported URL scheme %q", converted.Scheme)
	}

	return converted.String(), nil
}

func (service *Service) readServerSentEvents(session *streamSession, body io.ReadCloser) {
	defer body.Close()

	reader := bufio.NewReaderSize(body, 64<<10)
	eventName := ""
	dataLines := make([]string, 0, 4)

	dispatch := func() {
		if len(dataLines) == 0 && eventName == "" {
			return
		}

		name := eventName
		if name == "" {
			name = "message"
		}

		session.append("event", "in", name, strings.Join(dataLines, "\n"), "utf-8")
		eventName = ""
		dataLines = dataLines[:0]
	}

	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			trimmed := strings.TrimRight(line, "\r\n")
			switch {
			case trimmed == "":
				dispatch()
			case strings.HasPrefix(trimmed, ":"):
				// Comments keep the connection alive and carry no data.
			default:
				field, value, found := strings.Cut(trimmed, ":")
				if found {
					value = strings.TrimPrefix(value, " ")
				}
				switch field {
				case "event":
					eventName = value
				case "data":
					dataLines = append(dataLines, value)
				case "retry":
					// Reconnection delays are handled by the server, not Awake.
				}
			}
		}

		if err != nil {
			if session.isStopped() {
				session.finish("close", "closed by client")
			} else if errors.Is(err, io.EOF) {
				session.finish("close", "stream closed by server")
			} else {
				session.finish("error", fmt.Sprintf("read event stream: %v", err))
			}
			return
		}
	}
}

func (service *Service) readWebSocketMessages(session *streamSession, connection *websocket.Conn) {
	for {
		messageType, data, err := connection.Read(session.context)
		if err != nil {
			if session.isStopped() {
				session.finish("close", "closed by client")
				return
			}

			status := websocket.CloseStatus(err)
			switch status {
			case websocket.StatusNormalClosure:
				session.finish("close", "stream closed by server")
			case websocket.StatusGoingAway:
				session.finish("close", "server went away")
			default:
				session.finish("error", fmt.Sprintf("read websocket: %v", err))
			}
			return
		}

		encoding := "utf-8"
		content := string(data)
		if messageType == websocket.MessageBinary || !utf8.Valid(data) {
			encoding = "base64"
			content = base64.StdEncoding.EncodeToString(data)
		}

		session.append("message", "in", "", content, encoding)
	}
}

func (session *streamSession) snapshot() Stream {
	session.mutex.Lock()
	defer session.mutex.Unlock()

	return Stream{
		ID:                    session.id,
		Protocol:              session.protocol,
		StatusCode:            session.statusCode,
		Status:                session.status,
		Headers:               session.headers,
		StartedAtMilliseconds: session.startedAt.UnixMilli(),
		Closed:                session.closed,
		Error:                 session.lastError,
		PreRequestOutcome:     session.preRequest,
	}
}

func (session *streamSession) append(kind string, direction string, name string, data string, encoding string) {
	if len(data) > MaximumStreamEventSize {
		data = data[:MaximumStreamEventSize]
		encoding = "utf-8"
	}

	session.mutex.Lock()
	defer session.mutex.Unlock()

	event := StreamEvent{
		Index:                 session.nextIndex,
		Kind:                  kind,
		Direction:             direction,
		Name:                  name,
		Data:                  data,
		Encoding:              encoding,
		TimestampMilliseconds: time.Now().UnixMilli(),
	}
	session.nextIndex += 1
	session.events = append(session.events, event)

	if len(session.events) > MaximumRetainedEvents {
		drop := len(session.events) - MaximumRetainedEvents/2
		session.events = append([]StreamEvent(nil), session.events[drop:]...)
		session.baseIndex = session.events[0].Index
	}
}

func (session *streamSession) finish(kind string, message string) {
	session.mutex.Lock()
	if session.closed {
		session.mutex.Unlock()
		return
	}
	session.closed = true
	if kind == "error" {
		session.lastError = message
	}
	session.mutex.Unlock()

	session.append(kind, "system", "", message, "utf-8")
}

func (session *streamSession) isClosed() bool {
	session.mutex.Lock()
	defer session.mutex.Unlock()
	return session.closed
}

func (session *streamSession) isStopped() bool {
	session.mutex.Lock()
	defer session.mutex.Unlock()
	return session.stopped
}

func (session *streamSession) stop() {
	session.mutex.Lock()
	if session.stopped {
		session.mutex.Unlock()
		return
	}
	session.stopped = true
	session.mutex.Unlock()

	if session.connection != nil {
		_ = session.connection.CloseNow()
	}
	if session.closer != nil {
		_ = session.closer.Close()
	}
	if session.cancel != nil {
		session.cancel()
	}
}

func (registry *streamRegistry) register(protocol domain.Protocol, statusCode int, status string, headers http.Header) *streamSession {
	registry.prune()

	identifier, err := randomIdentifier()
	if err != nil {
		identifier = fmt.Sprintf("stream-%d", time.Now().UnixNano())
	}

	session := &streamSession{
		id:         identifier,
		protocol:   protocol,
		startedAt:  time.Now(),
		statusCode: statusCode,
		status:     status,
		headers:    headers,
	}
	if session.headers == nil {
		session.headers = map[string][]string{}
	}

	registry.mutex.Lock()
	registry.sessions[identifier] = session
	registry.mutex.Unlock()

	return session
}

func (registry *streamRegistry) find(identifier string) (*streamSession, bool) {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()

	session, found := registry.sessions[identifier]
	return session, found
}

func (registry *streamRegistry) remove(identifier string) (*streamSession, bool) {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()

	session, found := registry.sessions[identifier]
	if found {
		delete(registry.sessions, identifier)
	}

	return session, found
}

func (registry *streamRegistry) prune() {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()

	cutoff := time.Now().Add(-staleStreamAge)
	open := 0
	for identifier, session := range registry.sessions {
		if session.startedAt.Before(cutoff) {
			delete(registry.sessions, identifier)
			session.stop()
			continue
		}
		if !session.isClosed() {
			open += 1
		}
	}

	if open < maximumOpenStreams {
		return
	}

	for identifier, session := range registry.sessions {
		if session.isClosed() {
			delete(registry.sessions, identifier)
			session.stop()
			return
		}
	}
}
