package jftradeapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type sseWriter struct {
	writer      http.ResponseWriter
	flusher     http.Flusher
	retryMillis int
}

type sseStreamLoopOptions struct {
	initial       func(context.Context) error
	writeInterval time.Duration
	onTick        func(context.Context) error
	trigger       <-chan struct{}
}

func writeSSEHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
}

func newSSEWriter(w http.ResponseWriter, flusher http.Flusher) sseWriter {
	return sseWriter{
		writer:      w,
		flusher:     flusher,
		retryMillis: int(defaultSSEClientRetry / time.Millisecond),
	}
}

func prepareSSEWriter(w http.ResponseWriter) (sseWriter, bool) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return sseWriter{}, false
	}
	writeSSEHeaders(w)
	return newSSEWriter(w, flusher), true
}

func (writer sseWriter) WriteRetryDirective() error {
	if writer.retryMillis <= 0 {
		return nil
	}
	if _, err := fmt.Fprintf(writer.writer, "retry: %d\n\n", writer.retryMillis); err != nil {
		return err
	}
	writer.flusher.Flush()
	return nil
}

func (writer sseWriter) WriteEvent(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer.writer, "data: %s\n\n", data); err != nil {
		return err
	}
	writer.flusher.Flush()
	return nil
}

func runSSEStreamLoop(ctx context.Context, options sseStreamLoopOptions) error {
	if options.initial != nil {
		if err := options.initial(ctx); err != nil {
			return err
		}
	}

	var ticker *time.Ticker
	if options.writeInterval > 0 && options.onTick != nil {
		ticker = time.NewTicker(options.writeInterval)
		defer ticker.Stop()
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-options.trigger:
			if options.onTick == nil {
				continue
			}
			if err := options.onTick(ctx); err != nil {
				return err
			}
		case <-tickerC(ticker):
			if options.onTick == nil {
				continue
			}
			if err := options.onTick(ctx); err != nil {
				return err
			}
		}
	}
}

func tickerC(ticker *time.Ticker) <-chan time.Time {
	if ticker == nil {
		return nil
	}
	return ticker.C
}
