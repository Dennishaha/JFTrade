package servercore

import (
	"context"
	"net/http"
	"time"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
)

type sseWriter = httpserver.SSEWriter
type sseStreamLoopOptions = httpserver.SSEStreamLoopOptions

func writeSSEHeaders(w http.ResponseWriter) {
	httpserver.WriteSSEHeaders(w)
}

func newSSEWriter(w http.ResponseWriter, flusher http.Flusher) sseWriter {
	return httpserver.NewSSEWriter(w, flusher, int(defaultSSEClientRetry/time.Millisecond))
}

func prepareSSEWriter(w http.ResponseWriter) (sseWriter, bool) {
	return httpserver.PrepareSSEWriter(w)
}

func runSSEStreamLoop(ctx context.Context, options sseStreamLoopOptions) error {
	return httpserver.RunSSEStreamLoop(ctx, httpserver.SSEStreamLoopOptions{
		Initial:       options.Initial,
		WriteInterval: options.WriteInterval,
		OnTick:        options.OnTick,
		Trigger:       options.Trigger,
	})
}
