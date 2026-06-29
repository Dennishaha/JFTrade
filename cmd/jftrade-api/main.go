// Command jftrade-api runs the JFTrade sidecar without the bbgo CLI runtime.
//
// @title JFTrade Debug API
// @version 0.1.0
// @description Local JFTrade API sidecar for strategy authoring, market data, backtests, settings, and runtime operations.
// @BasePath /
package main

import (
	"context"
	"fmt"
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
	args := os.Args[1:]
	if isHelpArgs(args) {
		printUsage()
		return
	}
	if err := validateArgs(args); err != nil {
		log.Fatalf("%v", err)
	}

	if os.Getenv("DISABLE_MARKETS_CACHE") == "" {
		jftradeErr1 := os.Setenv("DISABLE_MARKETS_CACHE", "1")
		jftradeLogError(jftradeErr1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := apiserver.RunAPIOnly(ctx); err != nil {
		log.Fatalf("JFTrade API-only server failed: %v", err)
	}
}

func isHelpArgs(args []string) bool {
	return len(args) == 1 && (args[0] == "help" || args[0] == "--help" || args[0] == "-h")
}

func printUsage() {
	_, _ = fmt.Fprintln(os.Stdout, "Usage: jftrade-api")
}

func validateArgs(args []string) error {
	if len(args) == 0 {
		return nil
	}
	return fmt.Errorf("unsupported command %q; run `jftrade-api` without subcommands", args[0])
}

func jftradeLogError(values ...any) {
	for _, value := range values {
		if err, ok := value.(error); ok && err != nil {
			log.Printf("best-effort operation failed: %v", err)
		}
	}
}
