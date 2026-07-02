package httpserver

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

type failingSSEWriter struct {
	header http.Header
	err    error
}

func (w *failingSSEWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (*failingSSEWriter) WriteHeader(int) {}

func (w *failingSSEWriter) Write([]byte) (int, error) { return 0, w.err }

func (*failingSSEWriter) Flush() {}

func TestSSEWriterPropagatesSerializationAndWriteFailures(t *testing.T) {
	marshalWriter := NewSSEWriter(&failingSSEWriter{}, nil, 3000)
	if err := marshalWriter.WriteEvent(make(chan int)); err == nil {
		t.Fatal("WriteEvent unsupported value error = nil")
	}
	if err := marshalWriter.WriteEventID("evt-1", make(chan int)); err == nil {
		t.Fatal("WriteEventID unsupported value error = nil")
	}

	wantErr := errors.New("client disconnected")
	writeWriter := NewSSEWriter(&failingSSEWriter{err: wantErr}, nil, 3000)
	if err := writeWriter.WriteComment("heartbeat"); !errors.Is(err, wantErr) {
		t.Fatalf("WriteComment error = %v, want %v", err, wantErr)
	}
}

func TestRunSSEStreamLoopHandlesTriggerWithoutCallback(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	trigger := make(chan struct{}, 1)
	trigger <- struct{}{}
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()

	if err := RunSSEStreamLoop(ctx, SSEStreamLoopOptions{Trigger: trigger}); err != nil {
		t.Fatalf("RunSSEStreamLoop: %v", err)
	}
}

func TestRunSSEStreamLoopPropagatesTriggerAndTickerFailures(t *testing.T) {
	wantErr := errors.New("stream write failed")

	t.Run("trigger", func(t *testing.T) {
		trigger := make(chan struct{}, 1)
		trigger <- struct{}{}
		err := RunSSEStreamLoop(t.Context(), SSEStreamLoopOptions{
			Trigger: trigger,
			OnTick:  func(context.Context) error { return wantErr },
		})
		if !errors.Is(err, wantErr) {
			t.Fatalf("trigger error = %v, want %v", err, wantErr)
		}
	})

	t.Run("ticker", func(t *testing.T) {
		err := RunSSEStreamLoop(t.Context(), SSEStreamLoopOptions{
			WriteInterval: time.Millisecond,
			OnTick:        func(context.Context) error { return wantErr },
		})
		if !errors.Is(err, wantErr) {
			t.Fatalf("ticker error = %v, want %v", err, wantErr)
		}
	})
}

func TestFailingSSEWriterIncludesReadableError(t *testing.T) {
	wantErr := errors.New("network closed")
	writer := NewSSEWriter(&failingSSEWriter{err: wantErr}, nil, 3000)
	if err := writer.WriteRetryDirective(); err == nil || !strings.Contains(err.Error(), "network closed") {
		t.Fatalf("WriteRetryDirective error = %v", err)
	}
}
