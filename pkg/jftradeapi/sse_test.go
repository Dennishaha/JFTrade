package jftradeapi

import (
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type concurrentDetectingResponseWriter struct {
	header     http.Header
	mu         sync.Mutex
	body       strings.Builder
	active     int32
	concurrent int32
}

func (w *concurrentDetectingResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *concurrentDetectingResponseWriter) WriteHeader(int) {}

func (w *concurrentDetectingResponseWriter) Write(p []byte) (int, error) {
	if !atomic.CompareAndSwapInt32(&w.active, 0, 1) {
		atomic.StoreInt32(&w.concurrent, 1)
	}
	defer atomic.StoreInt32(&w.active, 0)
	time.Sleep(2 * time.Millisecond)
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.body.Write(p)
}

func (w *concurrentDetectingResponseWriter) Flush() {}

func TestSSEWriterSerializesConcurrentWrites(t *testing.T) {
	resp := &concurrentDetectingResponseWriter{}
	writer, ok := prepareSSEWriter(resp)
	if !ok {
		t.Fatal("prepareSSEWriter returned ok=false")
	}

	var wg sync.WaitGroup
	for index := 0; index < 8; index++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := writer.WriteEvent(map[string]any{"type": "delta", "toolProgress": i}); err != nil {
				t.Errorf("WriteEvent(%d): %v", i, err)
			}
		}(index)
	}
	wg.Wait()

	if atomic.LoadInt32(&resp.concurrent) != 0 {
		t.Fatal("detected concurrent writes to the underlying response writer")
	}
	body := resp.body.String()
	if count := strings.Count(body, "data: "); count != 8 {
		t.Fatalf("frame count = %d, want 8; body=%q", count, body)
	}
	if count := strings.Count(body, "\n\n"); count != 8 {
		t.Fatalf("frame terminator count = %d, want 8; body=%q", count, body)
	}
}
