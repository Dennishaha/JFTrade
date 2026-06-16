package httpserver

import (
	"net/http"
	"strings"
	"testing"
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
