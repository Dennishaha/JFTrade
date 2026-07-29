package pineruntime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/internal/jftsettings"
	"github.com/jftrade/jftrade-main/internal/pineworkerassets"
	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

func TestResolveConfigReportsUnavailableEmbeddedBundle(t *testing.T) {
	t.Setenv(EnvBundle, "")
	selectorErr := errors.New("embedded worker unavailable")
	config, enabled, err := ResolveConfig(
		jftsettings.PineWorkerSettings{},
		WithAssetSelector(func() (pineworkerassets.Asset, bool, error) {
			return pineworkerassets.Asset{}, false, selectorErr
		}),
	)
	if enabled || !errors.Is(err, selectorErr) || config.BundlePath != "" {
		t.Fatalf("config=%#v enabled=%v error=%v", config, enabled, err)
	}
}

func TestResolveConfigHonorsRuntimeFallbacksAndConfiguredWorkerLimits(t *testing.T) {
	t.Setenv(EnvBundle, filepath.Join(t.TempDir(), "worker.mjs"))
	t.Setenv(EnvRuntime, "environment-node")
	t.Setenv("JFTRADE_NODE_BINARY", "legacy-node")
	config, enabled, err := ResolveConfig(jftsettings.PineWorkerSettings{
		BacktestWorkerLimit: 4,
		InstanceWorkerLimit: 8,
	})
	if err != nil || !enabled {
		t.Fatalf("ResolveConfig enabled=%v error=%v", enabled, err)
	}
	if config.RuntimePath != "environment-node" || config.BacktestWorkers != 4 || config.InstanceWorkers != 8 {
		t.Fatalf("environment runtime config = %#v", config)
	}

	t.Setenv(EnvRuntime, "")
	config, enabled, err = ResolveConfig(jftsettings.PineWorkerSettings{})
	if err != nil || !enabled || config.RuntimePath != "legacy-node" {
		t.Fatalf("legacy runtime config = %#v enabled=%v error=%v", config, enabled, err)
	}
}

func TestResolveWorkDirFindsRepositoryFromExternalBundleAfterGetwdFailure(t *testing.T) {
	root := t.TempDir()
	protoPath := filepath.Join(root, filepath.FromSlash(DefaultProtoPath))
	if err := os.MkdirAll(filepath.Dir(protoPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(protoPath, []byte("syntax = \"proto3\";\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bundlePath := filepath.Join(root, "workers", "pineworker", "worker.mjs")
	deps := defaultDependencies()
	deps.getwd = func() (string, error) { return "", errors.New("working directory unavailable") }
	if got := resolveWorkDir(bundlePath, deps); got != root {
		t.Fatalf("work dir = %q, want %q", got, root)
	}
	resolved := filepath.Join(root, "resolved.proto")
	if got := resolvePath("resolved.proto", "", func(string) (string, error) { return resolved, nil }); got != resolved {
		t.Fatalf("resolved path = %q, want %q", got, resolved)
	}
}

func TestManagerDoesNotPublishWhenBacktestRunnerCannotBeBuilt(t *testing.T) {
	launcherErr := errors.New("launcher configuration rejected")
	launcherCalls := 0
	manager := NewManager(
		WithAssetSelector(embeddedAsset),
		WithLauncherFactory(func(Config, []byte) (pineworker.WorkerLauncher, error) {
			launcherCalls++
			return nil, launcherErr
		}),
		WithDialerFactory(func(int) pineworker.TransportDialer { return newFakeDialer() }),
	)
	published := false
	_, enabled, err := manager.Reconfigure(jftsettings.PineWorkerSettings{}, func(backtest Runner, instance Runner) {
		published = backtest != nil || instance != nil
	})
	if enabled || !errors.Is(err, launcherErr) || published || launcherCalls != 1 {
		t.Fatalf("enabled=%v published=%v launcherCalls=%d error=%v", enabled, published, launcherCalls, err)
	}
	if backtest, instance := manager.Runners(); backtest != nil || instance != nil {
		t.Fatalf("failed runner pair retained: %#v/%#v", backtest, instance)
	}
}

func TestBacktestRunnerWaitsForCapacityAndPropagatesStartupFailures(t *testing.T) {
	deps := applyOptions([]Option{
		WithLauncherFactory(func(Config, []byte) (pineworker.WorkerLauncher, error) {
			return &fakeLauncher{}, nil
		}),
		WithDialerFactory(func(int) pineworker.TransportDialer { return newFakeDialer() }),
	})
	runner, err := newEphemeralRunner(Config{bundleData: []byte("worker")}, runnerBacktest, deps)
	if err != nil {
		t.Fatal(err)
	}
	if cap(runner.busy) != 1 {
		t.Fatalf("default worker capacity = %d, want 1", cap(runner.busy))
	}
	runner.busy <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := runner.RunScript(ctx, validRequest("cancelled-capacity")); !errors.Is(err, context.Canceled) {
		t.Fatalf("capacity wait error = %v", err)
	}
	runner.release()

	runner.config.Host = "256.256.256.256"
	if _, err := runner.RunScript(context.Background(), validRequest("invalid-host")); err == nil ||
		!strings.Contains(err.Error(), "allocate pine worker port") {
		t.Fatalf("invalid host error = %v", err)
	}

	startFailure, err := newEphemeralRunner(
		Config{bundleData: []byte("worker"), Host: "127.0.0.1"},
		runnerBacktest,
		applyOptions([]Option{
			WithLauncherFactory(func(Config, []byte) (pineworker.WorkerLauncher, error) {
				return failingLauncher{}, nil
			}),
			WithDialerFactory(func(int) pineworker.TransportDialer { return newFakeDialer() }),
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := startFailure.RunScript(context.Background(), validRequest("startup-failure")); err == nil ||
		!strings.Contains(err.Error(), "worker start failed") {
		t.Fatalf("worker startup error = %v", err)
	}
}

func TestRunnerAndSessionNilAndClosedLifecycleBoundaries(t *testing.T) {
	var nilRunner *ephemeralRunner
	if _, _, err := nilRunner.OpenLiveSession(context.Background(), validRequest("nil-runner")); err == nil {
		t.Fatal("nil runner should reject live session")
	}
	if err := nilRunner.Close(context.Background()); err != nil {
		t.Fatalf("nil runner close: %v", err)
	}
	var nilSession *liveSession
	if _, err := nilSession.Append(context.Background(), validRequest("nil-session")); err == nil {
		t.Fatal("nil session should reject append")
	}
	if err := nilSession.Close(context.Background()); err != nil {
		t.Fatalf("nil session close: %v", err)
	}

	closed := &ephemeralRunner{closed: true, sessions: make(map[*liveSession]struct{})}
	if err := closed.registerSession(&liveSession{}); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("closed runner registration error = %v", err)
	}
	if port, err := freePort(context.Background(), ""); err != nil || port <= 0 {
		t.Fatalf("default-host port = %d error=%v", port, err)
	}
}

func TestManagerCloseDrainsActiveLiveSession(t *testing.T) {
	launcher := &fakeLauncher{}
	manager := newTestManager(launcher, newFakeDialer())
	if _, enabled, err := manager.Reconfigure(jftsettings.PineWorkerSettings{}, nil); err != nil || !enabled {
		t.Fatalf("Reconfigure enabled=%v error=%v", enabled, err)
	}
	_, instance := manager.Runners()
	session, response, err := instance.(LiveSessionOpener).OpenLiveSession(
		context.Background(),
		validRequest("active-until-runtime-close"),
	)
	if err != nil || response.SessionRevision != 1 {
		t.Fatalf("OpenLiveSession response=%#v error=%v", response, err)
	}
	if err := manager.Close(); err != nil {
		t.Fatalf("manager Close: %v", err)
	}
	if _, err := session.Append(context.Background(), validRequest("after-runtime-close")); err == nil ||
		!strings.Contains(err.Error(), "closed") {
		t.Fatalf("append after runtime close error = %v", err)
	}
	if err := session.Close(context.Background()); err != nil {
		t.Fatalf("repeated session Close: %v", err)
	}
	if launcher.stoppedCount() != 1 {
		t.Fatalf("stopped workers = %d, want 1", launcher.stoppedCount())
	}
}
