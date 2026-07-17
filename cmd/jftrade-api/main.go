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
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/jftrade/jftrade-main/internal/app/apiserver"

	// Embed IANA timezone database so API-only releases still work in
	// minimal environments that lack the system tz data.
	_ "time/tzdata"
)

type apiOnlyRunner func(context.Context) error

type signalContextFunc func(context.Context, ...os.Signal) (context.Context, context.CancelFunc)

type apiCommandRunner func(
	[]string,
	io.Writer,
	func(string) string,
	func(string, string) error,
	apiOnlyRunner,
	signalContextFunc,
) error

var executeAPICommand apiCommandRunner = runAPICommand

func main() {
	reportFatal(executeAPICommand(
		os.Args[1:],
		os.Stdout,
		os.Getenv,
		os.Setenv,
		apiserver.RunAPIOnly,
		signal.NotifyContext,
	), log.Fatalf)
}

func runAPICommand(
	args []string,
	stdout io.Writer,
	getenv func(string) string,
	setenv func(string, string) error,
	runAPI apiOnlyRunner,
	notifyContext signalContextFunc,
) error {
	if isHelpArgs(args) {
		printUsage(stdout)
		return nil
	}
	if err := validateArgs(args); err != nil {
		return err
	}

	if getenv("DISABLE_MARKETS_CACHE") == "" {
		jftradeLogError(setenv("DISABLE_MARKETS_CACHE", "1"))
	}

	ctx, stop := notifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := runAPI(ctx); err != nil {
		return fmt.Errorf("JFTrade API-only server failed: %w", err)
	}
	return nil
}

func isHelpArgs(args []string) bool {
	return len(args) == 1 && (args[0] == "help" || args[0] == "--help" || args[0] == "-h")
}

func printUsage(writer io.Writer) {
	_, _ = fmt.Fprintln(writer, "Usage: jftrade-api")
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

func reportFatal(err error, fatalf func(string, ...any)) {
	if err != nil {
		fatalf("%v", err)
	}
}
