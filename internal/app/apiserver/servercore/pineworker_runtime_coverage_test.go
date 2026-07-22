package servercore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

type pineWorkerLiveBoundaryDialer struct {
	transport *pineWorkerLiveBoundaryTransport
}

func (dialer pineWorkerLiveBoundaryDialer) Dial(context.Context, string) (pineworker.ManagedTransport, error) {
	return dialer.transport, nil
}

type pineWorkerLiveBoundaryTransport struct {
	run func(pineworker.RunScriptRequest) (pineworker.RunScriptResponse, error)
}

func (transport *pineWorkerLiveBoundaryTransport) RunScript(_ context.Context, request pineworker.RunScriptRequest) (pineworker.RunScriptResponse, error) {
	if transport.run != nil {
		return transport.run(request)
	}
	return pineworker.RunScriptResponse{JobID: request.JobID}, nil
}

func (*pineWorkerLiveBoundaryTransport) HealthCheck(context.Context) (pineworker.HealthStatus, error) {
	return pineworker.HealthStatus{OK: true, WorkerID: "live-boundary"}, nil
}

func (*pineWorkerLiveBoundaryTransport) Close() error { return nil }

func TestPineWorkerRuntimeApplyAndMinimumConcurrencyBoundaries(t *testing.T) {
	var nilServer *Server
	nilServer.applyPineWorkerSettings(PineWorkerSettings{})

	t.Setenv(envPineWorkerDisabled, "true")
	service := btsrv.NewService()
	server := &Server{serverFacades: serverFacades{backtestSvc: service}}
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

func TestEphemeralPineWorkerLiveSessionLifecycle(t *testing.T) {
	var nilRunner *ephemeralPineWorkerRunner
	if _, _, err := nilRunner.OpenLiveSession(t.Context(), pineworker.RunScriptRequest{}); err == nil {
		t.Fatal("nil runner opened a live session")
	}
	if err := nilRunner.Close(t.Context()); err != nil {
		t.Fatalf("nil runner close: %v", err)
	}

	closedRunner := &ephemeralPineWorkerRunner{busy: make(chan struct{}, 1), closed: true}
	if _, _, err := closedRunner.OpenLiveSession(t.Context(), pineworker.RunScriptRequest{}); err == nil {
		t.Fatal("closed runner opened a live session")
	}
	busyRunner := &ephemeralPineWorkerRunner{busy: make(chan struct{}, 1), rejectWhenBusy: true}
	busyRunner.busy <- struct{}{}
	if _, _, err := busyRunner.OpenLiveSession(t.Context(), pineworker.RunScriptRequest{}); !errors.Is(err, pineworker.ErrCapacityExceeded) {
		t.Fatalf("busy live-session open error = %v", err)
	}

	launcher := &fakeServerPineWorkerLauncher{}
	dialer := newFakeServerPineWorkerDialer()
	restorePineWorkerFactories(t, launcher, dialer)
	newRunner := func(t *testing.T) *ephemeralPineWorkerRunner {
		t.Helper()
		runner, err := newEphemeralPineWorkerRunner(pineWorkerRuntimeConfig{
			bundleData: []byte("fake worker"), InstanceWorkers: 1, Host: "127.0.0.1",
			RequestTimeout: time.Second, HealthTimeout: 100 * time.Millisecond,
		}, pineWorkerRunnerInstance)
		if err != nil {
			t.Fatalf("new live runner: %v", err)
		}
		return runner
	}

	runner := newRunner(t)
	sessionCtx, cancelSession := context.WithCancel(t.Context())
	session, opened, err := runner.OpenLiveSession(sessionCtx, validServerPineWorkerRunScriptRequest("live-open"))
	if err != nil || session == nil || opened.SessionID == "" || opened.SessionRevision != 1 {
		t.Fatalf("OpenLiveSession = session %#v response %#v err=%v", session, opened, err)
	}
	appended, err := session.Append(sessionCtx, validServerPineWorkerRunScriptRequest("live-append"))
	if err != nil || appended.SessionID != opened.SessionID || appended.SessionRevision != 2 {
		t.Fatalf("Append = %#v err=%v", appended, err)
	}
	if err := session.Close(t.Context()); err != nil {
		t.Fatalf("session Close: %v", err)
	}
	if err := session.Close(t.Context()); err != nil {
		t.Fatalf("idempotent session Close: %v", err)
	}
	if _, err := session.Append(t.Context(), validServerPineWorkerRunScriptRequest("live-after-close")); err == nil {
		t.Fatal("closed session accepted append")
	}
	cancelSession()
	if err := runner.Close(t.Context()); err != nil {
		t.Fatalf("runner Close after session: %v", err)
	}

	runner = newRunner(t)
	closeCtx, cancelClose := context.WithCancel(t.Context())
	if _, _, err := runner.OpenLiveSession(closeCtx, validServerPineWorkerRunScriptRequest("live-runner-close")); err != nil {
		t.Fatalf("open before runner close: %v", err)
	}
	if err := runner.Close(t.Context()); err != nil {
		t.Fatalf("runner closes active session: %v", err)
	}
	cancelClose()
}

func TestEphemeralPineWorkerLiveSessionFailureBoundaries(t *testing.T) {
	request := validServerPineWorkerRunScriptRequest("live-failure")

	badHost := &ephemeralPineWorkerRunner{
		config: pineWorkerRuntimeConfig{Host: "256.256.256.256"}, busy: make(chan struct{}, 1),
	}
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	if _, _, err := badHost.OpenLiveSession(ctx, request); err == nil {
		t.Fatal("invalid host allocated a live worker")
	}

	startErr := errors.New("forced live worker start failure")
	startRunner := &ephemeralPineWorkerRunner{
		config:   pineWorkerRuntimeConfig{Host: "127.0.0.1", HealthTimeout: time.Millisecond, RequestTimeout: time.Second},
		launcher: pineWorkerLauncherFailureStub{err: startErr}, dialer: newFakeServerPineWorkerDialer(), busy: make(chan struct{}, 1),
	}
	if _, _, err := startRunner.OpenLiveSession(t.Context(), request); !errors.Is(err, startErr) {
		t.Fatalf("live worker start error = %v", err)
	}

	rpcErr := errors.New("forced live worker RPC failure")
	transport := &pineWorkerLiveBoundaryTransport{run: func(pineworker.RunScriptRequest) (pineworker.RunScriptResponse, error) {
		return pineworker.RunScriptResponse{}, rpcErr
	}}
	rpcRunner := &ephemeralPineWorkerRunner{
		config:   pineWorkerRuntimeConfig{Host: "127.0.0.1", HealthTimeout: time.Second, RequestTimeout: time.Second},
		launcher: &fakeServerPineWorkerLauncher{}, dialer: pineWorkerLiveBoundaryDialer{transport: transport}, busy: make(chan struct{}, 1),
	}
	if _, _, err := rpcRunner.OpenLiveSession(t.Context(), request); !errors.Is(err, rpcErr) {
		t.Fatalf("live worker RPC error = %v", err)
	}

	var closingRunner *ephemeralPineWorkerRunner
	transport = &pineWorkerLiveBoundaryTransport{}
	transport.run = func(request pineworker.RunScriptRequest) (pineworker.RunScriptResponse, error) {
		revision := request.ExpectedRevision
		if request.SessionOperation == pineworker.SessionOperationOpen {
			revision = 1
			closingRunner.mu.Lock()
			closingRunner.closed = true
			closingRunner.mu.Unlock()
		}
		return pineworker.RunScriptResponse{JobID: request.JobID, SessionID: request.SessionID, SessionRevision: revision}, nil
	}
	closingRunner = &ephemeralPineWorkerRunner{
		config:   pineWorkerRuntimeConfig{Host: "127.0.0.1", HealthTimeout: time.Second, RequestTimeout: time.Second},
		launcher: &fakeServerPineWorkerLauncher{}, dialer: pineWorkerLiveBoundaryDialer{transport: transport}, busy: make(chan struct{}, 1),
		sessions: make(map[*ephemeralPineWorkerSession]struct{}),
	}
	if _, _, err := closingRunner.OpenLiveSession(t.Context(), request); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("close-during-open error = %v", err)
	}

	var nilSession *ephemeralPineWorkerSession
	if _, err := nilSession.Append(t.Context(), request); err == nil {
		t.Fatal("nil session accepted append")
	}
	if err := nilSession.Close(t.Context()); err != nil {
		t.Fatalf("nil session close: %v", err)
	}
}
