// Command jftrade wraps the bbgo CLI with non-invasive plugins (Futu exchange).
package main

import (
	"context"
	"log"
	"os"

	"github.com/c9s/bbgo/pkg/cmd"
	"github.com/jftrade/jftrade-main/pkg/jftradeapi"

	// Side-effect imports register plugins with bbgo at init() time.
	_ "github.com/jftrade/jftrade-main/pkg/futu"
)

func main() {
	if os.Getenv("DISABLE_MARKETS_CACHE") == "" {
		_ = os.Setenv("DISABLE_MARKETS_CACHE", "1")
	}
	if _, err := jftradeapi.StartForRunArgs(context.Background(), os.Args[1:]); err != nil {
		log.Printf("JFTrade API adapter disabled: %v", err)
	}
	cmd.Execute()
}
