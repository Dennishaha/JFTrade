package servercore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

func TestStartPineWorkerManagersRejectsInvalidRuntimeConfiguration(t *testing.T) {
	t.Setenv(envPineWorkerBundle, filepath.Join(t.TempDir(), "worker.mjs"))
	t.Setenv(envPineWorkerRequestTimeout, "not-a-duration")

	backtest, instance := (&Server{}).startPineWorkerManagers()
	if backtest != nil || instance != nil {
		t.Fatalf("runners = %#v/%#v, want both disabled for invalid runtime configuration", backtest, instance)
	}
}

func TestStartPineWorkerManagersDoesNotPublishPartialRunnerSet(t *testing.T) {
	bundlePath := filepath.Join(t.TempDir(), "worker.mjs")
	if err := os.WriteFile(bundlePath, []byte("worker"), 0o600); err != nil {
		t.Fatalf("write worker bundle: %v", err)
	}
	t.Setenv(envPineWorkerBundle, bundlePath)

	for _, failCall := range []int{1, 2} {
		t.Run(string(rune('0'+failCall)), func(t *testing.T) {
			previous := newPineWorkerLauncher
			calls := 0
			newPineWorkerLauncher = func(pineWorkerRuntimeConfig, []byte) (pineworker.WorkerLauncher, error) {
				calls++
				if calls == failCall {
					return nil, errors.New("launcher unavailable")
				}
				return &fakeServerPineWorkerLauncher{}, nil
			}
			t.Cleanup(func() { newPineWorkerLauncher = previous })

			backtest, instance := (&Server{}).startPineWorkerManagers()
			if backtest != nil || instance != nil {
				t.Fatalf("runners = %#v/%#v, want atomic startup failure", backtest, instance)
			}
			if calls != failCall {
				t.Fatalf("launcher factory calls = %d, want %d", calls, failCall)
			}
		})
	}
}

func TestNewEphemeralPineWorkerRunnerReportsBundleAndLauncherFailures(t *testing.T) {
	if _, err := newEphemeralPineWorkerRunner(pineWorkerRuntimeConfig{
		BundlePath: filepath.Join(t.TempDir(), "missing-worker.mjs"),
	}, pineWorkerRunnerBacktest); err == nil || !strings.Contains(err.Error(), "read worker bundle") {
		t.Fatalf("missing bundle error = %v", err)
	}

	previous := newPineWorkerLauncher
	newPineWorkerLauncher = func(pineWorkerRuntimeConfig, []byte) (pineworker.WorkerLauncher, error) {
		return nil, errors.New("runtime unavailable")
	}
	t.Cleanup(func() { newPineWorkerLauncher = previous })
	if _, err := newEphemeralPineWorkerRunner(pineWorkerRuntimeConfig{
		BundlePath: "embedded-worker.mjs",
		bundleData: []byte("worker"),
	}, pineWorkerRunnerBacktest); err == nil || !strings.Contains(err.Error(), "create launcher") {
		t.Fatalf("launcher creation error = %v", err)
	}
}

func TestEphemeralPineWorkerRunnerCancellationAndHostFailureBoundaries(t *testing.T) {
	var nilRunner *ephemeralPineWorkerRunner
	if _, err := nilRunner.RunScript(context.Background(), pineworker.RunScriptRequest{}); err == nil || !strings.Contains(err.Error(), "runner is nil") {
		t.Fatalf("nil runner error = %v", err)
	}

	runner := &ephemeralPineWorkerRunner{busy: make(chan struct{}, 1)}
	if err := runner.acquire(context.Background()); err != nil {
		t.Fatalf("occupy backtest capacity: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runner.acquire(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled capacity wait error = %v, want context.Canceled", err)
	}
	runner.release()
	runner.release()

	ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := freePineWorkerPort(ctx, "256.256.256.256"); err == nil || !strings.Contains(err.Error(), "allocate pine worker port") {
		t.Fatalf("invalid worker host error = %v", err)
	}

	if got := (&ephemeralPineWorkerRunner{}).stopTimeout(); got != 5*time.Second {
		t.Fatalf("default stop timeout = %v, want 5s", got)
	}
	if got := (&ephemeralPineWorkerRunner{config: pineWorkerRuntimeConfig{RequestTimeout: 30 * time.Second}}).stopTimeout(); got != 10*time.Second {
		t.Fatalf("capped stop timeout = %v, want 10s", got)
	}
}

func TestResolvePineWorkerRuntimeConfigRejectsEveryInvalidOperationalLimit(t *testing.T) {
	t.Setenv(envPineWorkerBundle, filepath.Join(t.TempDir(), "worker.mjs"))
	cases := []struct {
		name  string
		key   string
		value string
	}{
		{name: "instance workers", key: envPineWorkerInstanceWorkers, value: "1001"},
		{name: "start port", key: envPineWorkerStartPort, value: "0"},
		{name: "request timeout", key: envPineWorkerRequestTimeout, value: "0s"},
		{name: "health timeout", key: envPineWorkerHealthTimeout, value: "broken"},
		{name: "message bytes", key: envPineWorkerMaxMessageBytes, value: "-1"},
		{name: "candles", key: envPineWorkerMaxCandles, value: "none"},
		{name: "duration", key: envPineWorkerMaxDuration, value: "-1s"},
		{name: "duration per bar", key: envPineWorkerMaxDurationPerBar, value: "0s"},
		{name: "throughput", key: envPineWorkerMinCandlesPerSec, value: "0"},
		{name: "memory", key: envPineWorkerMaxPeakRSSBytes, value: "invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(tc.key, tc.value)
			_, enabled, err := resolvePineWorkerRuntimeConfig(nil)
			if err == nil {
				t.Fatalf("resolve config with %s=%q: error = nil", tc.key, tc.value)
			}
			if enabled {
				t.Fatal("enabled = true for invalid operational limit")
			}
		})
	}
}
