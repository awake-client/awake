package httpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/clementherve/awake/src/domain"
)

func TestOpenStreamReadsServerSentEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		writer.WriteHeader(http.StatusOK)

		flusher, _ := writer.(http.Flusher)
		_, _ = writer.Write([]byte(": keep-alive\n\n"))
		_, _ = writer.Write([]byte("event: greeting\ndata: hello\ndata: world\n\n"))
		_, _ = writer.Write([]byte("data: second\n\n"))
		flusher.Flush()
	}))
	defer server.Close()

	service := newTestService(t, fakeSource{
		request: domain.Request{
			Name:     "Events",
			Method:   domain.MethodGet,
			URL:      server.URL,
			Protocol: domain.ProtocolSSE,
		},
	})

	stream, err := service.OpenStream(t.TempDir(), "collections/events.yaml", "")
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer service.ReleaseStream(stream.ID)

	if stream.Protocol != domain.ProtocolSSE || stream.StatusCode != http.StatusOK {
		t.Fatalf("unexpected stream: %+v", stream)
	}

	page := waitForStreamEvent(t, service, stream.ID, "hello\nworld")
	greeting := findEvent(page, "hello\nworld")
	if greeting == nil || greeting.Name != "greeting" {
		t.Fatalf("unexpected greeting event: %+v", greeting)
	}

	page = waitForStreamClosed(t, service, stream.ID)
	if !page.Closed {
		t.Fatalf("expected a closed stream")
	}
	if findKind(page, "close") == nil {
		t.Errorf("expected a close event, got %+v", page.Events)
	}
	if findEvent(page, "second") == nil {
		t.Errorf("expected the unnamed event, got %+v", page.Events)
	}
}

func TestOpenStreamReadsWebSocketMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connection, err := websocket.Accept(writer, request, nil)
		if err != nil {
			return
		}
		defer connection.Close(websocket.StatusNormalClosure, "done")

		for {
			messageType, data, err := connection.Read(context.Background())
			if err != nil {
				return
			}
			if err := connection.Write(context.Background(), messageType, []byte("echo: "+string(data))); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	service := newTestService(t, fakeSource{
		request: domain.Request{
			Name:     "Socket",
			Method:   domain.MethodGet,
			URL:      strings.Replace(server.URL, "http", "ws", 1),
			Protocol: domain.ProtocolWebSocket,
		},
	})

	stream, err := service.OpenStream(t.TempDir(), "collections/socket.yaml", "")
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer service.ReleaseStream(stream.ID)

	if stream.Protocol != domain.ProtocolWebSocket || stream.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("unexpected stream: %+v", stream)
	}

	if err := service.SendStreamMessage(stream.ID, "ping", "utf-8"); err != nil {
		t.Fatalf("send message: %v", err)
	}

	page := waitForStreamEvent(t, service, stream.ID, "echo: ping")
	message := findEvent(page, "echo: ping")
	if message == nil || message.Direction != "in" || message.Encoding != "utf-8" {
		t.Fatalf("unexpected message event: %+v", message)
	}

	if err := service.CloseStream(stream.ID); err != nil {
		t.Fatalf("close stream: %v", err)
	}

	closed := waitForStreamClosed(t, service, stream.ID)
	if !closed.Closed {
		t.Fatalf("expected the stream to close")
	}
}

func TestOpenStreamRejectsHTTPRequests(t *testing.T) {
	service := newTestService(t, fakeSource{
		request: domain.Request{
			Name:   "Plain",
			Method: domain.MethodGet,
			URL:    "https://example.com",
		},
	})

	if _, err := service.OpenStream(t.TempDir(), "collections/plain.yaml", ""); err != ErrNotStreaming {
		t.Fatalf("expected ErrNotStreaming, got %v", err)
	}
}

func TestReadStreamEventsRejectsUnknownStream(t *testing.T) {
	service := newTestService(t, fakeSource{})

	if _, err := service.ReadStreamEvents("missing", 0); err != ErrStreamNotFound {
		t.Fatalf("expected ErrStreamNotFound, got %v", err)
	}
}

func waitForStreamEvent(t *testing.T, service *Service, identifier string, content string) StreamEventPage {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	nextIndex := int64(0)
	collected := make([]StreamEvent, 0)

	for time.Now().Before(deadline) {
		page, err := service.ReadStreamEvents(identifier, nextIndex)
		if err != nil {
			t.Fatalf("read stream events: %v", err)
		}

		collected = append(collected, page.Events...)
		nextIndex = page.NextIndex

		if findEvent(StreamEventPage{Events: collected}, content) != nil {
			return StreamEventPage{Events: collected, NextIndex: nextIndex, Closed: page.Closed}
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("event %q not received: %+v", content, collected)
	return StreamEventPage{}
}

func waitForStreamClosed(t *testing.T, service *Service, identifier string) StreamEventPage {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	nextIndex := int64(0)
	collected := make([]StreamEvent, 0)

	for time.Now().Before(deadline) {
		page, err := service.ReadStreamEvents(identifier, nextIndex)
		if err != nil {
			t.Fatalf("read stream events: %v", err)
		}

		collected = append(collected, page.Events...)
		nextIndex = page.NextIndex
		if page.Closed {
			return StreamEventPage{Events: collected, NextIndex: nextIndex, Closed: true, Error: page.Error}
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("stream did not close: %+v", collected)
	return StreamEventPage{}
}

func findEvent(page StreamEventPage, content string) *StreamEvent {
	for index := range page.Events {
		if page.Events[index].Data == content {
			return &page.Events[index]
		}
	}
	return nil
}

func findKind(page StreamEventPage, kind string) *StreamEvent {
	for index := range page.Events {
		if page.Events[index].Kind == kind {
			return &page.Events[index]
		}
	}
	return nil
}
