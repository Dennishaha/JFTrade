package assistant

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	assistantservice "github.com/jftrade/jftrade-main/internal/assistant"
	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

// coverage98FailingSSEWriter models a client that disconnects while the
// response headers have already been accepted. It is deliberately a real
// http.Flusher so Gin takes the normal SSE path instead of an artificial
// unsupported-writer branch.
type coverage98FailingSSEWriter struct {
	header http.Header
	status int
	writes int
}

func newCoverage98FailingSSEWriter() *coverage98FailingSSEWriter {
	return &coverage98FailingSSEWriter{header: make(http.Header)}
}

func (w *coverage98FailingSSEWriter) Header() http.Header {
	return w.header
}

func (w *coverage98FailingSSEWriter) WriteHeader(status int) {
	w.status = status
}

func (w *coverage98FailingSSEWriter) Write([]byte) (int, error) {
	w.writes++
	return 0, errors.New("stream client disconnected")
}

func (*coverage98FailingSSEWriter) Flush() {}

func coverage98SSEContext(t *testing.T, writer http.ResponseWriter, method string, target string, body string) *gin.Context {
	t.Helper()
	context, _ := gin.CreateTestContext(writer)
	context.Request = httptest.NewRequestWithContext(t.Context(), method, target, strings.NewReader(body))
	return context
}

func TestCoverage98ChatStreamTransportHandlesDisconnectedClients(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := &Handler{
		service: assistantservice.NewService(nil),
		streams: newADKChatStreamHub(),
	}

	t.Run("invalid payload stops after failed retry handshake", func(t *testing.T) {
		writer := newCoverage98FailingSSEWriter()
		handler.handleADKChatStream(coverage98SSEContext(t, writer, http.MethodPost, "/api/v1/adk/chat/stream", `{"message":`))

		if writer.writes != 1 {
			t.Fatalf("writes = %d, want only failed retry directive", writer.writes)
		}
		if got := writer.Header().Get("Content-Type"); got != "text/event-stream" {
			t.Fatalf("Content-Type = %q, want SSE response", got)
		}
	})

	t.Run("valid request keeps terminal execution state after client disconnect", func(t *testing.T) {
		writer := newCoverage98FailingSSEWriter()
		handler.handleADKChatStream(coverage98SSEContext(t, writer, http.MethodPost, "/api/v1/adk/chat/stream", `{"message":"hello"}`))

		streamID := writer.Header().Get("X-ADK-Stream-ID")
		if streamID == "" {
			t.Fatal("stream id was not assigned before SSE handshake")
		}
		if writer.writes != 1 {
			t.Fatalf("writes = %d, want failed retry directive only", writer.writes)
		}
		record, ok := handler.streams.get(streamID)
		if !ok {
			t.Fatalf("stream %q was not retained for reconnect", streamID)
		}

		deadline := time.After(time.Second)
		for {
			events, terminal, notify := record.snapshot(0)
			if terminal {
				if len(events) == 0 || events[len(events)-1].Type != "error" {
					t.Fatalf("terminal events = %#v, want unavailable-runtime error", events)
				}
				return
			}
			select {
			case <-notify:
			case <-deadline:
				t.Fatal("background chat execution did not publish a terminal event")
			}
		}
	})
}

func TestCoverage98ChatStreamReconnectAndReplayRespectClientDisconnect(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := &Handler{
		service: assistantservice.NewService(nil),
		streams: newADKChatStreamHub(),
	}
	record := handler.streams.create()
	handler.streams.publish(record, adkChatStreamEvent{
		Type:  "final",
		RunID: "run-disconnected-client",
		Response: &jfadk.ChatResponse{
			Run: jfadk.Run{ID: "run-disconnected-client"},
		},
	})

	t.Run("stream reconnect exits cleanly when retry write fails", func(t *testing.T) {
		writer := newCoverage98FailingSSEWriter()
		context := coverage98SSEContext(t, writer, http.MethodGet, "/api/v1/adk/streams/"+record.id, "")
		context.Params = gin.Params{{Key: "streamId", Value: record.id}}
		handler.handleADKChatStreamReconnect(context)

		if got := writer.Header().Get("X-ADK-Stream-ID"); got != record.id {
			t.Fatalf("reconnect stream id = %q, want %q", got, record.id)
		}
		if writer.writes != 1 {
			t.Fatalf("reconnect writes = %d, want failed retry only", writer.writes)
		}
	})

	t.Run("run reconnect exits cleanly when retry write fails", func(t *testing.T) {
		writer := newCoverage98FailingSSEWriter()
		context := coverage98SSEContext(t, writer, http.MethodGet, "/api/v1/adk/runs/run-disconnected-client/stream", "")
		context.Params = gin.Params{{Key: "runId", Value: "run-disconnected-client"}}
		handler.handleADKRunStreamReconnect(context)

		if got := writer.Header().Get("X-ADK-Stream-ID"); got != record.id {
			t.Fatalf("run reconnect stream id = %q, want %q", got, record.id)
		}
		if writer.writes != 1 {
			t.Fatalf("run reconnect writes = %d, want failed retry only", writer.writes)
		}
	})

	t.Run("event replay stops when an event frame cannot be written", func(t *testing.T) {
		writer := newCoverage98FailingSSEWriter()
		context := coverage98SSEContext(t, writer, http.MethodGet, "/api/v1/adk/streams/"+record.id, "")
		handler.streamADKChatRecord(context, httpserver.NewSSEWriter(writer, writer, 0), record, 0, true)
		if writer.writes != 1 {
			t.Fatalf("replay writes = %d, want one failed event frame", writer.writes)
		}
	})

	t.Run("idle record returns as soon as the request context is cancelled", func(t *testing.T) {
		idle := handler.streams.create()
		writer := httptest.NewRecorder()
		context, cancel := context.WithCancel(t.Context())
		cancel()
		ginContext, _ := gin.CreateTestContext(writer)
		ginContext.Request = httptest.NewRequestWithContext(context, http.MethodGet, "/api/v1/adk/streams/"+idle.id, nil)
		sseWriter, ok := httpserver.PrepareSSEWriter(ginContext.Writer)
		if !ok {
			t.Fatal("Gin response writer should support SSE")
		}
		handler.streamADKChatRecord(ginContext, sseWriter, idle, 0, false)
		if writer.Body.Len() != 0 {
			t.Fatalf("cancelled idle stream wrote body %q", writer.Body.String())
		}
	})
}
