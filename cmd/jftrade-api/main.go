// Command jftrade-api runs the JFTrade sidecar without the bbgo CLI runtime.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/jftrade/jftrade-main/internal/app/apiserver"

	// Embed IANA timezone database so API-only releases still work in
	// minimal environments that lack the system tz data.
	_ "time/tzdata"
)

func main() {
	if os.Getenv("DISABLE_MARKETS_CACHE") == "" {
		_ = os.Setenv("DISABLE_MARKETS_CACHE", "1")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := apiserver.RunAPIOnly(ctx); err != nil {
		log.Fatalf("JFTrade API-only server failed: %v", err)
	}
}
