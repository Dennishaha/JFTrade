// Command jftrade wraps the bbgo CLI with non-invasive plugins (Futu exchange).
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/c9s/bbgo/pkg/cmd"

	"github.com/jftrade/jftrade-main/internal/app/apiserver"

	// Side-effect imports register plugins with bbgo at init() time.
	_ "github.com/jftrade/jftrade-main/pkg/futu"
	_ "github.com/jftrade/jftrade-main/pkg/strategy/pineruntime"

	// Embed IANA timezone database so time.LoadLocation works in minimal
	// environments (Docker images, CI runners) that lack the system tz data.
	_ "time/tzdata"
)

func main() {
	if os.Getenv("DISABLE_MARKETS_CACHE") == "" {
		jftradeErr1 := os.Setenv("DISABLE_MARKETS_CACHE", "1")
		jftradeLogError(jftradeErr1)
	}
	if shouldRunAPIOnly(os.Args[1:]) {
		runAPIOnly()
		return
	}
	if _, err := apiserver.StartForRunArgs(context.Background(), os.Args[1:]); err != nil {
		log.Printf("JFTrade API adapter disabled: %v", err)
	}
	cmd.Execute()
}

func shouldRunAPIOnly(args []string) bool {
	value := strings.TrimSpace(os.Getenv("JFTRADE_API_ONLY"))
	if strings.EqualFold(value, "1") || strings.EqualFold(value, "true") {
		return true
	}
	if len(args) == 0 {
		return true
	}
	return len(args) > 0 && (args[0] == "api" || args[0] == "serve-api")
}

func runAPIOnly() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := apiserver.RunAPIOnly(ctx); err != nil {
		log.Fatalf("JFTrade API adapter failed: %v", err)
	}
}

func jftradeLogError(values ...any) {
	for _, value := range values {
		if err, ok := value.(error); ok && err != nil {
			log.Printf("best-effort operation failed: %v", err)
		}
	}
}
