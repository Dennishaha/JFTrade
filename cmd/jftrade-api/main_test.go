package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func TestValidateArgsAllowsNoArgs(t *testing.T) {
	if err := validateArgs(nil); err != nil {
		t.Fatalf("validateArgs(nil) = %v, want nil", err)
	}
}

func TestValidateArgsRejectsLegacySubcommands(t *testing.T) {
	for _, args := range [][]string{
		{"api"},
		{"serve-api"},
		{"run"},
	} {
		if err := validateArgs(args); err == nil {
			t.Fatalf("validateArgs(%v) = nil, want error", args)
		}
	}
}

func TestIsHelpArgs(t *testing.T) {
	for _, args := range [][]string{
		{"help"},
		{"--help"},
		{"-h"},
	} {
		if !isHelpArgs(args) {
			t.Fatalf("isHelpArgs(%v) = false, want true", args)
		}
	}
	if isHelpArgs(nil) || isHelpArgs([]string{"api"}) || isHelpArgs([]string{"help", "extra"}) {
		t.Fatal("isHelpArgs accepted non-help args")
	}
}

func TestRunAPICommandPrintsUsageWithoutStartingTheServer(t *testing.T) {
	var output bytes.Buffer
	called := false

	err := runAPICommand(
		[]string{"--help"},
		&output,
		func(string) string { t.Fatal("getenv should not be called"); return "" },
		func(string, string) error { t.Fatal("setenv should not be called"); return nil },
		func(context.Context) error { called = true; return nil },
		func(context.Context, ...os.Signal) (context.Context, context.CancelFunc) {
			t.Fatal("notifyContext should not be called")
			return nil, nil
		},
	)
	if err != nil {
		t.Fatalf("runAPICommand() error = %v", err)
	}
	if called || output.String() != "Usage: jftrade-api\n" {
		t.Fatalf("runAPICommand() called=%v output=%q", called, output.String())
	}
}

func TestRunAPICommandRejectsUnsupportedArgsBeforeStartingTheServer(t *testing.T) {
	err := runAPICommand(
		[]string{"serve-api"},
		io.Discard,
		func(string) string { t.Fatal("getenv should not be called"); return "" },
		func(string, string) error { t.Fatal("setenv should not be called"); return nil },
		func(context.Context) error { t.Fatal("runAPI should not be called"); return nil },
		func(context.Context, ...os.Signal) (context.Context, context.CancelFunc) {
			t.Fatal("notifyContext should not be called")
			return nil, nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "unsupported command") {
		t.Fatalf("runAPICommand() error = %v, want unsupported-command error", err)
	}
}

func TestRunAPICommandStartsAndStopsAPI(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	stopCalled := false
	setCalls := make([]string, 0, 1)

	err := runAPICommand(
		nil,
		io.Discard,
		func(string) string { return "" },
		func(name, value string) error {
			setCalls = append(setCalls, name+"="+value)
			return nil
		},
		func(received context.Context) error {
			if received != ctx {
				t.Fatal("runAPI received unexpected context")
			}
			return nil
		},
		func(context.Context, ...os.Signal) (context.Context, context.CancelFunc) {
			return ctx, func() { stopCalled = true }
		},
	)
	if err != nil {
		t.Fatalf("runAPICommand() error = %v", err)
	}
	if got, want := setCalls, []string{"DISABLE_MARKETS_CACHE=1"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("setenv calls = %v, want %v", got, want)
	}
	if !stopCalled {
		t.Fatal("runAPICommand() did not stop the signal context")
	}
}

func TestRunAPICommandPreservesConfiguredCacheAndWrapsStartupErrors(t *testing.T) {
	wantErr := errors.New("startup failed")
	setCalled := false
	err := runAPICommand(
		nil,
		io.Discard,
		func(string) string { return "0" },
		func(string, string) error { setCalled = true; return nil },
		func(context.Context) error { return wantErr },
		func(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
			return parent, func() {}
		},
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("runAPICommand() error = %v, want %v", err, wantErr)
	}
	if setCalled {
		t.Fatal("runAPICommand() overwrote existing DISABLE_MARKETS_CACHE")
	}
}

func TestRunAPICommandContinuesAfterBestEffortEnvironmentFailure(t *testing.T) {
	setErr := errors.New("environment is read-only")
	called := false
	err := runAPICommand(
		nil,
		io.Discard,
		func(string) string { return "" },
		func(string, string) error { return setErr },
		func(context.Context) error { called = true; return nil },
		func(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
			return parent, func() {}
		},
	)
	if err != nil || !called {
		t.Fatalf("runAPICommand() error=%v called=%v, want successful startup", err, called)
	}
}

func TestMainDelegatesToCommandRunner(t *testing.T) {
	originalRunner := executeAPICommand
	originalArgs := os.Args
	t.Cleanup(func() {
		executeAPICommand = originalRunner
		os.Args = originalArgs
	})

	called := false
	executeAPICommand = func(
		args []string,
		_ io.Writer,
		_ func(string) string,
		_ func(string, string) error,
		_ apiOnlyRunner,
		_ signalContextFunc,
	) error {
		called = true
		if strings.Join(args, ",") != "from-main" {
			t.Fatalf("main args = %v", args)
		}
		return nil
	}
	os.Args = []string{"jftrade-api", "from-main"}

	main()
	if !called {
		t.Fatal("main() did not invoke the command runner")
	}
}

func TestReportFatalForwardsErrorsAndIgnoresNil(t *testing.T) {
	var format string
	var args []any
	reportFatal(nil, func(string, ...any) {
		t.Fatal("fatalf should not be called for nil")
	})
	reportFatal(errors.New("boom"), func(receivedFormat string, receivedArgs ...any) {
		format, args = receivedFormat, receivedArgs
	})
	if format != "%v" || len(args) != 1 || args[0] == nil || args[0].(error).Error() != "boom" {
		t.Fatalf("fatalf received format=%q args=%v", format, args)
	}
}
