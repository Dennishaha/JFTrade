package servercore

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	btsrv "github.com/jftrade/jftrade-main/internal/backtest"
	"github.com/jftrade/jftrade-main/internal/pineworkerassets"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

func TestServerStartsConfiguredEphemeralPineWorkerRunners(t *testing.T) {
	binaryPath := filepath.Join(t.TempDir(), "worker")
	if err := os.WriteFile(binaryPath, []byte("fake worker"), 0o755); err != nil {
		t.Fatalf("write worker: %v", err)
	}
	t.Setenv(envPineWorkerBundle, binaryPath)
	t.Setenv(envPineWorkerBacktestWorkers, "2")
	t.Setenv(envPineWorkerInstanceWorkers, "3")
	t.Setenv(envPineWorkerStartPort, "56001")

	launcher := &fakeServerPineWorkerLauncher{}
	dialer := newFakeServerPineWorkerDialer()
	restorePineWorkerFactories(t, launcher, dialer)

	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	backtestRunner, instanceRunner := server.runtimes.PineWorkerRunners()
	if backtestRunner == nil || instanceRunner == nil {
		t.Fatalf("pine worker runners = backtest %#v instance %#v, want both configured", backtestRunner, instanceRunner)
	}
	if launcher.startedCount() != 0 {
		t.Fatalf("started workers = %d before use, want no eager start", launcher.startedCount())
	}
	if _, err := backtestRunner.RunScript(context.Background(), validServerPineWorkerRunScriptRequest("ephemeral-backtest")); err != nil {
		t.Fatalf("backtest RunScript: %v", err)
	}
	if launcher.startedCount() != 1 {
		t.Fatalf("started workers after backtest = %d, want 1", launcher.startedCount())
	}
	if launcher.stoppedCount() != 1 {
		t.Fatalf("stopped workers after backtest = %d, want 1", launcher.stoppedCount())
	}
	if _, err := instanceRunner.RunScript(context.Background(), validServerPineWorkerRunScriptRequest("ephemeral-instance")); err != nil {
		t.Fatalf("instance RunScript: %v", err)
	}
	if launcher.startedCount() != 2 {
		t.Fatalf("started workers after instance = %d, want 2", launcher.startedCount())
	}
	if launcher.stoppedCount() != 2 {
		t.Fatalf("stopped workers after instance = %d, want 2", launcher.stoppedCount())
	}
	if _, err := instanceRunner.RunScript(context.Background(), validServerPineWorkerRunScriptRequest("ephemeral-instance-2")); err != nil {
		t.Fatalf("second instance RunScript: %v", err)
	}
	if launcher.startedCount() != 3 {
		t.Fatalf("started workers after second instance = %d, want 3", launcher.startedCount())
	}
	if launcher.stoppedCount() != 3 {
		t.Fatalf("stopped workers after second instance = %d, want 3", launcher.stoppedCount())
	}
}

func TestServerStartsEmbeddedPineWorkerManager(t *testing.T) {
	restorePineWorkerAssetSelector(t, pineworkerassets.Asset{
		Name:   "worker-embedded",
		Data:   []byte("embedded worker"),
		SHA256: "embedded-sha",
	}, true, nil)
	t.Setenv(envPineWorkerBacktestWorkers, "1")
	t.Setenv(envPineWorkerInstanceWorkers, "1")
	t.Setenv(envPineWorkerStartPort, "57001")

	launcher := &fakeServerPineWorkerLauncher{}
	dialer := newFakeServerPineWorkerDialer()
	restorePineWorkerFactories(t, launcher, dialer)

	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	backtestRunner, instanceRunner := server.runtimes.PineWorkerRunners()
	if backtestRunner == nil || instanceRunner == nil {
		t.Fatalf("pine worker runners = backtest %#v instance %#v, want embedded runners", backtestRunner, instanceRunner)
	}
	if launcher.startedCount() != 0 {
		t.Fatalf("started workers = %d before use, want no eager start", launcher.startedCount())
	}
	if _, err := backtestRunner.RunScript(context.Background(), validServerPineWorkerRunScriptRequest("embedded-ephemeral-start")); err != nil {
		t.Fatalf("RunScript: %v", err)
	}
	if launcher.startedCount() != 1 {
		t.Fatalf("started workers = %d, want 1", launcher.startedCount())
	}
	if launcher.stoppedCount() != 1 {
		t.Fatalf("stopped workers = %d, want 1", launcher.stoppedCount())
	}
}

func TestServerBacktestDoesNotFallbackToGoRuntimeWithoutPineWorker(t *testing.T) {
	restorePineWorkerAssetSelector(t, pineworkerassets.Asset{}, false, nil)

	dbPath := filepath.Join(t.TempDir(), "backtest.db")
	t.Setenv("JFTRADE_BACKTEST_DB", dbPath)
	seedServerPineWorkerTestKLines(t, dbPath)

	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	backtestRunner, instanceRunner := server.runtimes.PineWorkerRunners()
	if backtestRunner != nil || instanceRunner != nil {
		t.Fatalf("pine worker runners = backtest %#v instance %#v without worker binary", backtestRunner, instanceRunner)
	}
	if _, err := server.stores.Design.SaveDefinition(stratsrv.Definition{
		ID:           "pinets-required",
		Name:         "PineTS Required",
		Version:      "0.1.0",
		Runtime:      strategyRuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Symbol:       "US.AAPL",
		Interval:     "1m",
		Script:       `//@version=6` + "\n" + `strategy("PineTS Required", overlay=true)` + "\n" + `strategy.entry("Long", strategy.long, qty=1)`,
	}); err != nil {
		t.Fatalf("saveDefinition: %v", err)
	}
	run, err := server.backtestSvc.Start(context.Background(), btsrv.StartRequest{
		DefinitionID: "pinets-required",
		Symbol:       "US.AAPL",
		Interval:     "1m",
		StartTime:    "2026-05-26T09:30:00Z",
		EndTime:      "2026-05-26T09:31:00Z",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	finished := waitForServerBacktestStatus(t, server, run.ID, "failed")
	if finished.Result == nil || !strings.Contains(finished.Result.Error, "pine worker runner is not configured") {
		t.Fatalf("finished result = %#v, want Pine worker fail-fast", finished.Result)
	}
}

func validServerPineWorkerRunScriptRequest(jobID string) pineworker.RunScriptRequest {
	return pineworker.RunScriptRequest{
		JobID:     jobID,
		ScriptID:  "test-script",
		Source:    `//@version=6` + "\n" + `strategy("test")`,
		Symbol:    "US.AAPL",
		Timeframe: "1m",
		Mode:      pineworker.ModeBacktest,
		Candles: []pineworker.Candle{{
			OpenTime:  1,
			CloseTime: 2,
			Open:      1,
			High:      2,
			Low:       1,
			Close:     2,
			Volume:    100,
		}},
	}
}

func seedServerPineWorkerTestKLines(t *testing.T, dbPath string) {
	t.Helper()
	store := openServerKLineSeedStore(t, dbPath)
	defer func() { jftradeCheckTestError(t, store.Close()) }()
	start := time.Date(2026, time.May, 26, 9, 30, 0, 0, time.UTC)
	klines := []bbgotypes.KLine{
		serverPineWorkerTestKLine(start, 100, 101),
		serverPineWorkerTestKLine(start.Add(time.Minute), 101, 102),
	}
	if err := store.InsertKLines(klines, "forward"); err != nil {
		t.Fatalf("InsertKLines: %v", err)
	}
}

func serverPineWorkerTestKLine(start time.Time, open float64, close float64) bbgotypes.KLine {
	return bbgotypes.KLine{
		StartTime: bbgotypes.Time(start),
		EndTime:   bbgotypes.Time(start.Add(time.Minute - time.Millisecond)),
		Interval:  bbgotypes.Interval1m,
		Symbol:    "US.AAPL",
		Open:      fixedpoint.NewFromFloat(open),
		High:      fixedpoint.NewFromFloat(max(open, close) + 1),
		Low:       fixedpoint.NewFromFloat(min(open, close) - 1),
		Close:     fixedpoint.NewFromFloat(close),
		Volume:    fixedpoint.NewFromFloat(1000),
	}
}

func waitForServerBacktestStatus(t *testing.T, server *Server, runID string, want string) *backtestRunState {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		run, ok, err := server.stores.BacktestRuns.GetFull(runID)
		if err != nil {
			t.Fatalf("getFull: %v", err)
		}
		if ok && run.Status == want {
			return run
		}
		time.Sleep(10 * time.Millisecond)
	}
	run, _, _ := server.stores.BacktestRuns.GetFull(runID)
	t.Fatalf("timed out waiting for run %s status %q; latest = %#v", runID, want, run)
	return nil
}

func restorePineWorkerFactories(t *testing.T, launcher pineworker.WorkerLauncher, dialer pineworker.TransportDialer) {
	t.Helper()
	previousLauncher := newPineWorkerLauncher
	previousDialer := newPineWorkerDialer
	newPineWorkerLauncher = func(pineWorkerRuntimeConfig, []byte) (pineworker.WorkerLauncher, error) {
		return launcher, nil
	}
	newPineWorkerDialer = func(int) pineworker.TransportDialer {
		return dialer
	}
	t.Cleanup(func() {
		newPineWorkerLauncher = previousLauncher
		newPineWorkerDialer = previousDialer
	})
}

func restorePineWorkerAssetSelector(t *testing.T, asset pineworkerassets.Asset, ok bool, err error) {
	t.Helper()
	previous := selectPineWorkerAsset
	selectPineWorkerAsset = func() (pineworkerassets.Asset, bool, error) {
		return asset, ok, err
	}
	t.Cleanup(func() {
		selectPineWorkerAsset = previous
	})
}

type fakeServerPineWorkerLauncher struct {
	mu        sync.Mutex
	started   []pineworker.WorkerSpec
	processes []*fakeServerPineWorkerProcess
}

type closeTrackingPineWorkerRunner struct {
	closed int
}

func (runner *closeTrackingPineWorkerRunner) RunScript(context.Context, pineworker.RunScriptRequest) (pineworker.RunScriptResponse, error) {
	return pineworker.RunScriptResponse{}, nil
}

func (runner *closeTrackingPineWorkerRunner) Close(context.Context) error {
	runner.closed++
	return nil
}

func (launcher *fakeServerPineWorkerLauncher) Start(ctx context.Context, spec pineworker.WorkerSpec) (pineworker.WorkerProcess, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	process := &fakeServerPineWorkerProcess{}
	launcher.started = append(launcher.started, spec)
	launcher.processes = append(launcher.processes, process)
	return process, nil
}

func (launcher *fakeServerPineWorkerLauncher) startedCount() int {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return len(launcher.started)
}

type fakeServerPineWorkerProcess struct {
	mu    sync.Mutex
	stops int
}

func (process *fakeServerPineWorkerProcess) Stop(context.Context) error {
	process.mu.Lock()
	defer process.mu.Unlock()
	process.stops++
	return nil
}

func (launcher *fakeServerPineWorkerLauncher) stoppedCount() int {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	count := 0
	for _, process := range launcher.processes {
		process.mu.Lock()
		count += process.stops
		process.mu.Unlock()
	}
	return count
}

type fakeServerPineWorkerDialer struct {
	mu         sync.Mutex
	transports map[string]*fakeServerPineWorkerTransport
}

func newFakeServerPineWorkerDialer() *fakeServerPineWorkerDialer {
	return &fakeServerPineWorkerDialer{transports: map[string]*fakeServerPineWorkerTransport{}}
}

func (dialer *fakeServerPineWorkerDialer) Dial(ctx context.Context, address string) (pineworker.ManagedTransport, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	dialer.mu.Lock()
	defer dialer.mu.Unlock()
	transport := &fakeServerPineWorkerTransport{address: address}
	dialer.transports[address] = transport
	return transport, nil
}

type fakeServerPineWorkerTransport struct {
	mu      sync.Mutex
	address string
	closed  bool
	runs    int
}

func (transport *fakeServerPineWorkerTransport) RunScript(_ context.Context, request pineworker.RunScriptRequest) (pineworker.RunScriptResponse, error) {
	transport.mu.Lock()
	transport.runs++
	transport.mu.Unlock()
	revision := request.ExpectedRevision
	switch request.SessionOperation {
	case pineworker.SessionOperationOpen:
		revision = 1
	case pineworker.SessionOperationAppend:
		revision++
	}
	return pineworker.RunScriptResponse{
		JobID: request.JobID, SessionID: request.SessionID, SessionRevision: revision,
		Metadata: pineworker.WorkerMetadata{
			Duration:      100 * time.Microsecond,
			RequestBytes:  100,
			ResponseBytes: 100,
		},
	}, nil
}

func (transport *fakeServerPineWorkerTransport) HealthCheck(context.Context) (pineworker.HealthStatus, error) {
	return pineworker.HealthStatus{OK: true, WorkerID: transport.address}, nil
}

func (transport *fakeServerPineWorkerTransport) Close() error {
	transport.closed = true
	return nil
}
