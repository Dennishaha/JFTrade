package httpserver

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

type panicSSEWriter struct {
	header  http.Header
	onWrite func()
	onFlush func()
	body    strings.Builder
}

func (w *panicSSEWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *panicSSEWriter) WriteHeader(int) {}

func (w *panicSSEWriter) Write(p []byte) (int, error) {
	if w.onWrite != nil {
		w.onWrite()
	}
	return w.body.Write(p)
}

func (w *panicSSEWriter) Flush() {
	if w.onFlush != nil {
		w.onFlush()
	}
}

func TestSSEWriterReturnsFlushPanicAsError(t *testing.T) {
	resp := &panicSSEWriter{
		onFlush: func() { panic("flush failed") },
	}
	writer, ok := PrepareSSEWriter(resp)
	if !ok {
		t.Fatal("PrepareSSEWriter returned ok=false")
	}

	if err := writer.WriteEvent(map[string]string{"type": "delta"}); err == nil || !strings.Contains(err.Error(), "flush failed") {
		t.Fatalf("WriteEvent error = %v, want recovered flush panic", err)
	}
}

func TestSSEWriterReturnsWritePanicAsError(t *testing.T) {
	resp := &panicSSEWriter{
		onWrite: func() { panic("write failed") },
	}
	writer, ok := PrepareSSEWriter(resp)
	if !ok {
		t.Fatal("PrepareSSEWriter returned ok=false")
	}

	if err := writer.WriteRetryDirective(); err == nil || !strings.Contains(err.Error(), "write failed") {
		t.Fatalf("WriteRetryDirective error = %v, want recovered write panic", err)
	}
}

type plainResponseWriter struct {
	header http.Header
	body   strings.Builder
}

func (w *plainResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *plainResponseWriter) WriteHeader(int) {}

func (w *plainResponseWriter) Write(p []byte) (int, error) {
	return w.body.Write(p)
}

func TestPrepareSSEWriterAndFrameFormatting(t *testing.T) {
	resp := &panicSSEWriter{}
	writer, ok := PrepareSSEWriter(resp)
	if !ok {
		t.Fatal("PrepareSSEWriter returned ok=false")
	}

	if got := resp.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := resp.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("Cache-Control = %q", got)
	}
	if got := resp.Header().Get("Connection"); got != "keep-alive" {
		t.Fatalf("Connection = %q", got)
	}

	if err := writer.WriteRetryDirective(); err != nil {
		t.Fatalf("WriteRetryDirective error = %v", err)
	}
	if err := writer.WriteEventID("evt-1", struct {
		Status string `json:"status"`
	}{Status: "ready"}); err != nil {
		t.Fatalf("WriteEventID error = %v", err)
	}
	if err := writer.WriteComment("heartbeat"); err != nil {
		t.Fatalf("WriteComment error = %v", err)
	}

	want := "retry: 3000\n\nid: evt-1\ndata: {\"status\":\"ready\"}\n\n: heartbeat\n\n"
	if got := resp.body.String(); got != want {
		t.Fatalf("SSE body = %q, want %q", got, want)
	}

	if err := NewSSEWriter(resp, resp, 0).WriteRetryDirective(); err != nil {
		t.Fatalf("WriteRetryDirective(no retry) error = %v", err)
	}
	if got := resp.body.String(); got != want {
		t.Fatalf("no-retry writer should not append output: %q", got)
	}
}

func TestPrepareSSEWriterRejectsWriterWithoutFlusher(t *testing.T) {
	writer, ok := PrepareSSEWriter(&plainResponseWriter{})
	if ok {
		t.Fatalf("PrepareSSEWriter returned ok=true with writer %#v", writer)
	}
}

func TestRunSSEStreamLoopPropagatesInitialError(t *testing.T) {
	want := errors.New("initial failed")
	err := RunSSEStreamLoop(context.Background(), SSEStreamLoopOptions{
		Initial: func(context.Context) error { return want },
	})
	if !errors.Is(err, want) {
		t.Fatalf("RunSSEStreamLoop error = %v, want %v", err, want)
	}
}

func TestRunSSEStreamLoopRunsTriggerAndTickerTicks(t *testing.T) {
	t.Run("trigger path drives resumable writes until cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		trigger := make(chan struct{}, 2)
		trigger <- struct{}{}
		trigger <- struct{}{}
		ticks := 0

		err := RunSSEStreamLoop(ctx, SSEStreamLoopOptions{
			Trigger: trigger,
			OnTick: func(context.Context) error {
				ticks++
				if ticks == 2 {
					cancel()
				}
				return nil
			},
		})
		if err != nil {
			t.Fatalf("RunSSEStreamLoop(trigger) error = %v", err)
		}
		if ticks != 2 {
			t.Fatalf("trigger ticks = %d, want 2", ticks)
		}
	})

	t.Run("ticker path fires when interval is configured", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ticks := 0
		err := RunSSEStreamLoop(ctx, SSEStreamLoopOptions{
			WriteInterval: 5 * time.Millisecond,
			OnTick: func(context.Context) error {
				ticks++
				cancel()
				return nil
			},
		})
		if err != nil {
			t.Fatalf("RunSSEStreamLoop(ticker) error = %v", err)
		}
		if ticks != 1 {
			t.Fatalf("ticker ticks = %d, want 1", ticks)
		}
	})
}

func TestTickerCHandlesNilTicker(t *testing.T) {
	if tickerC(nil) != nil {
		t.Fatal("tickerC(nil) should return nil")
	}
}
