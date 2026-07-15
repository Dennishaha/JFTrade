package servercore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	btsrv "github.com/jftrade/jftrade-main/internal/backtest"
	"github.com/jftrade/jftrade-main/internal/pineworkerassets"
	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

type pineWorkerLauncherFailureStub struct{ err error }

func (s pineWorkerLauncherFailureStub) Start(context.Context, pineworker.WorkerSpec) (pineworker.WorkerProcess, error) {
	return nil, s.err
}

type pineWorkerStopFailureLauncher struct{ err error }

func (s pineWorkerStopFailureLauncher) Start(context.Context, pineworker.WorkerSpec) (pineworker.WorkerProcess, error) {
	return pineWorkerStopFailureProcess(s), nil
}

type pineWorkerStopFailureProcess struct{ err error }

func (s pineWorkerStopFailureProcess) Stop(context.Context) error { return s.err }

func TestPineWorkerRuntimeApplyAndMinimumConcurrencyBoundaries(t *testing.T) {
	var nilServer *Server
	nilServer.applyPineWorkerSettings(PineWorkerSettings{})

	t.Setenv(envPineWorkerDisabled, "true")
	service := btsrv.NewService()
	server := &Server{backtestSvc: service}
	server.applyPineWorkerSettings(PineWorkerSettings{})

	launcher := &fakeServerPineWorkerLauncher{}
	dialer := newFakeServerPineWorkerDialer()
	restorePineWorkerFactories(t, launcher, dialer)
	runner, err := newEphemeralPineWorkerRunner(pineWorkerRuntimeConfig{bundleData: []byte("worker")}, pineWorkerRunnerBacktest)
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}
	if cap(runner.busy) != 1 {
		t.Fatalf("minimum concurrency = %d", cap(runner.busy))
	}
}

func TestPineWorkerRuntimeRunScriptFailureBoundaries(t *testing.T) {
	busyRunner := &ephemeralPineWorkerRunner{busy: make(chan struct{}, 1), rejectWhenBusy: true}
	busyRunner.busy <- struct{}{}
	if _, err := busyRunner.RunScript(context.Background(), pineworker.RunScriptRequest{}); !errors.Is(err, pineworker.ErrCapacityExceeded) {
		t.Fatalf("busy RunScript error = %v", err)
	}

	badHost := &ephemeralPineWorkerRunner{
		config: pineWorkerRuntimeConfig{Host: "256.256.256.256"},
		busy:   make(chan struct{}, 1),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := badHost.RunScript(ctx, pineworker.RunScriptRequest{}); err == nil {
		t.Fatal("expected manager allocation failure")
	}

	startErr := errors.New("forced worker start failure")
	startRunner := &ephemeralPineWorkerRunner{
		config:   pineWorkerRuntimeConfig{Host: "127.0.0.1", HealthTimeout: time.Millisecond, RequestTimeout: time.Second},
		launcher: pineWorkerLauncherFailureStub{err: startErr},
		dialer:   newFakeServerPineWorkerDialer(),
		busy:     make(chan struct{}, 1),
	}
	if _, err := startRunner.RunScript(context.Background(), pineworker.RunScriptRequest{}); !errors.Is(err, startErr) {
		t.Fatalf("manager start error = %v", err)
	}

	stopErr := errors.New("forced worker stop failure")
	stopRunner := &ephemeralPineWorkerRunner{
		config:   pineWorkerRuntimeConfig{Host: "127.0.0.1", HealthTimeout: time.Second, RequestTimeout: time.Second},
		launcher: pineWorkerStopFailureLauncher{err: stopErr},
		dialer:   newFakeServerPineWorkerDialer(),
		busy:     make(chan struct{}, 1),
	}
	if _, err := stopRunner.RunScript(context.Background(), validServerPineWorkerRunScriptRequest("stop-error")); err != nil {
		t.Fatalf("RunScript before stop failure: %v", err)
	}
}

func TestPineWorkerRuntimeResolutionRemainingBoundaries(t *testing.T) {
	if port, err := freePineWorkerPort(context.Background(), "  "); err != nil || port <= 0 {
		t.Fatalf("blank-host free port = %d, %v", port, err)
	}

	restorePineWorkerAssetSelector(t, pineworkerassets.Asset{}, false, errors.New("forced asset selection failure"))
	if _, enabled, err := resolvePineWorkerBundleConfig(); err == nil || enabled {
		t.Fatalf("asset selection = enabled %v, err %v", enabled, err)
	}

	previousGetwd := pineWorkerGetwd
	previousAbs := pineWorkerAbs
	t.Cleanup(func() {
		pineWorkerGetwd = previousGetwd
		pineWorkerAbs = previousAbs
	})

	getwdErr := errors.New("forced getwd failure")
	pineWorkerGetwd = func() (string, error) { return "", getwdErr }
	if got := resolvePineWorkerWorkDir(""); got != "" {
		t.Fatalf("workdir after getwd failure = %q", got)
	}
	repoRoot := t.TempDir()
	protoPath := filepath.Join(repoRoot, filepath.FromSlash(defaultPineWorkerProtoPath))
	if err := os.MkdirAll(filepath.Dir(protoPath), 0o755); err != nil {
		t.Fatalf("create proto directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module coverage"), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(protoPath, []byte("syntax = \"proto3\";"), 0o600); err != nil {
		t.Fatalf("write proto: %v", err)
	}
	if got := resolvePineWorkerWorkDir(filepath.Join(repoRoot, "dist", "worker.mjs")); got != repoRoot {
		t.Fatalf("bundle-derived repo workdir = %q", got)
	}

	nonRepo := t.TempDir()
	pineWorkerGetwd = func() (string, error) { return nonRepo, nil }
	if got := resolvePineWorkerWorkDir(""); got != nonRepo {
		t.Fatalf("non-repo workdir = %q", got)
	}
	if got := findPineWorkerRepoRoot(filepath.Join(nonRepo, "missing", "child")); got != "" {
		t.Fatalf("missing repo root = %q", got)
	}

	pineWorkerAbs = func(string) (string, error) { return "", errors.New("forced abs failure") }
	if got := resolvePineWorkerRuntimePath("relative-worker", ""); got != filepath.Clean("relative-worker") {
		t.Fatalf("fallback runtime path = %q", got)
	}
	pineWorkerAbs = previousAbs
	if got := resolvePineWorkerRuntimePath("relative-worker", ""); !filepath.IsAbs(got) {
		t.Fatalf("absolute runtime path = %q", got)
	}

	if got := clampInt(-1, 1, 10); got != 1 {
		t.Fatalf("lower clamp = %d", got)
	}
	if got := clampInt(11, 1, 10); got != 10 {
		t.Fatalf("upper clamp = %d", got)
	}
}
