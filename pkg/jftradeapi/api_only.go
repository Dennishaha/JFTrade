package jftradeapi

import (
	"context"
	"time"
)

// RunAPIOnly starts the sidecar in API-only mode and waits for the caller's
// context to be cancelled before shutting the servers down.
func RunAPIOnly(ctx context.Context) error {
	shutdown, err := StartForRunArgs(ctx, []string{"api"})
	if err != nil {
		return err
	}

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return shutdown(shutdownCtx)
}
