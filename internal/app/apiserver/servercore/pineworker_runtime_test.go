package servercore

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	bbgotypes "github.com/c9s/bbgo/pkg/types"

	btsrv "github.com/jftrade/jftrade-main/internal/backtest"
	"github.com/jftrade/jftrade-main/internal/pineworkerassets"
	bt "github.com/jftrade/jftrade-main/pkg/backtest"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

func TestResolvePineWorkerRuntimeConfigFromEnv(t *testing.T) {
	binaryPath := filepath.Join(t.TempDir(), "worker")
	t.Setenv(envPineWorkerBinary, binaryPath)
	t.Setenv(envPineWorkerSHA256, "abc123")
	t.Setenv(envPineWorkerWorkers, "3")
	t.Setenv(envPineWorkerHost, "localhost")
	t.Setenv(envPineWorkerStartPort, "55001")
	t.Setenv(envPineWorkerTempDir, t.TempDir())
	t.Setenv(envPineWorkerProto, "proto/pineworker.proto")
	t.Setenv(envPineWorkerPineTSVersion, "1.2.3")
	t.Setenv(envPineWorkerMock, "true")
	t.Setenv(envPineWorkerRequestTimeout, "2s")
	t.Setenv(envPineWorkerHealthTimeout, "500ms")
	t.Setenv(envPineWorkerMaxMessageBytes, "1048576")
	t.Setenv(envPineWorkerMaxCandles, "1000")
	t.Setenv(envPineWorkerMaxDuration, "5s")
	t.Setenv(envPineWorkerMaxDurationPerBar, "1ms")
	t.Setenv(envPineWorkerMinCandlesPerSec, "2500")
	t.Setenv(envPineWorkerMaxPeakRSSBytes, "33554432")

	config, enabled, err := resolvePineWorkerRuntimeConfig(nil)
	if err != nil {
		t.Fatalf("resolvePineWorkerRuntimeConfig error = %v", err)
	}
	if !enabled {
		t.Fatal("resolvePineWorkerRuntimeConfig enabled = false, want true")
	}
	if config.BinaryPath != binaryPath || config.SHA256 != "abc123" || config.Workers != 3 {
		t.Fatalf("unexpected identity config: %#v", config)
	}
	if config.Host != "localhost" || config.StartPort != 55001 || !config.Mock {
		t.Fatalf("unexpected connection config: %#v", config)
	}
	if config.RequestTimeout != 2*time.Second || config.HealthTimeout != 500*time.Millisecond {
		t.Fatalf("unexpected timeout config: %#v", config)
	}
	if config.MaxMessageBytes != 1048576 || config.MaxCandles != 1000 || config.MaxPeakRSSBytes != 33554432 {
		t.Fatalf("unexpected size config: %#v", config)
	}
	if config.MaxDuration != 5*time.Second || config.MaxDurationPerBar != time.Millisecond || config.MinCandlesPerSec != 2500 {
		t.Fatalf("unexpected gate config: %#v", config)
	}
}

func TestResolvePineWorkerRuntimeConfigDefaultsToRealPineTSWorker(t *testing.T) {
	binaryPath := filepath.Join(t.TempDir(), "worker")
	t.Setenv(envPineWorkerBinary, binaryPath)

	config, enabled, err := resolvePineWorkerRuntimeConfig(nil)
	if err != nil {
		t.Fatalf("resolvePineWorkerRuntimeConfig error = %v", err)
	}
	if !enabled {
		t.Fatal("resolvePineWorkerRuntimeConfig enabled = false, want true")
	}
	if config.Mock {
		t.Fatal("Mock = true by default; production worker must require explicit mock opt-in")
	}
	if !filepath.IsAbs(config.ProtoPath) || !strings.HasSuffix(filepath.ToSlash(config.ProtoPath), defaultPineWorkerProtoPath) {
		t.Fatalf("ProtoPath = %q, want absolute repo proto path", config.ProtoPath)
	}
	if config.WorkDir == "" || !filepath.IsAbs(config.WorkDir) {
		t.Fatalf("WorkDir = %q, want absolute worker cwd", config.WorkDir)
	}
	wantWorkers := runtime.NumCPU()
	if wantWorkers < 1 {
		wantWorkers = 1
	}
	if wantWorkers > 1000 {
		wantWorkers = 1000
	}
	if config.Workers != wantWorkers {
		t.Fatalf("Workers = %d, want default CPU worker limit %d", config.Workers, wantWorkers)
	}
}

func TestResolvePineWorkerRuntimeConfigKeepsProtoEnvOverride(t *testing.T) {
	binaryPath := filepath.Join(t.TempDir(), "worker")
	customProtoPath := filepath.Join(t.TempDir(), "custom-pineworker.proto")
	t.Setenv(envPineWorkerBinary, binaryPath)
	t.Setenv(envPineWorkerProto, customProtoPath)

	config, enabled, err := resolvePineWorkerRuntimeConfig(nil)
	if err != nil {
		t.Fatalf("resolvePineWorkerRuntimeConfig error = %v", err)
	}
	if !enabled {
		t.Fatal("resolvePineWorkerRuntimeConfig enabled = false, want true")
	}
	if config.ProtoPath != filepath.Clean(customProtoPath) {
		t.Fatalf("ProtoPath = %q, want env override %q", config.ProtoPath, customProtoPath)
	}
}

func TestFindPineWorkerRepoRootFromNestedWorkerPath(t *testing.T) {
	root := t.TempDir()
	protoPath := filepath.Join(root, filepath.FromSlash(defaultPineWorkerProtoPath))
	if err := os.MkdirAll(filepath.Dir(protoPath), 0o755); err != nil {
		t.Fatalf("mkdir proto dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module test\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(protoPath, []byte("syntax = \"proto3\";\n"), 0o644); err != nil {
		t.Fatalf("write proto: %v", err)
	}
	nested := filepath.Join(root, "var", "pineworker")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	if got := findPineWorkerRepoRoot(nested); got != root {
		t.Fatalf("findPineWorkerRepoRoot() = %q, want %q", got, root)
	}
}

func TestResolvePineWorkerRuntimeConfigUsesSettingsWorkerLimit(t *testing.T) {
	binaryPath := filepath.Join(t.TempDir(), "worker")
	t.Setenv(envPineWorkerBinary, binaryPath)

	config, enabled, err := resolvePineWorkerRuntimeConfig(func() PineWorkerSettings {
		return PineWorkerSettings{WorkerLimit: 4}
	})
	if err != nil {
		t.Fatalf("resolvePineWorkerRuntimeConfig error = %v", err)
	}
	if !enabled || config.Workers != 4 {
		t.Fatalf("enabled=%v Workers=%d, want settings worker limit 4", enabled, config.Workers)
	}
}

func TestResolvePineWorkerRuntimeConfigEnvOverridesSettingsWorkerLimit(t *testing.T) {
	binaryPath := filepath.Join(t.TempDir(), "worker")
	t.Setenv(envPineWorkerBinary, binaryPath)
	t.Setenv(envPineWorkerWorkers, "5")

	config, enabled, err := resolvePineWorkerRuntimeConfig(func() PineWorkerSettings {
		return PineWorkerSettings{WorkerLimit: 2}
	})
	if err != nil {
		t.Fatalf("resolvePineWorkerRuntimeConfig error = %v", err)
	}
	if !enabled || config.Workers != 5 {
		t.Fatalf("enabled=%v Workers=%d, want env worker limit 5", enabled, config.Workers)
	}
}

func TestResolvePineWorkerRuntimeConfigDisabledWithoutBinary(t *testing.T) {
	restorePineWorkerAssetSelector(t, pineworkerassets.Asset{}, false, nil)

	config, enabled, err := resolvePineWorkerRuntimeConfig(nil)
	if err != nil {
		t.Fatalf("resolvePineWorkerRuntimeConfig error = %v", err)
	}
	if enabled || config.BinaryPath != "" || len(config.binaryData) != 0 {
		t.Fatalf("config = %#v enabled=%v, want disabled empty config", config, enabled)
	}
}

func TestResolvePineWorkerRuntimeConfigUsesEmbeddedAsset(t *testing.T) {
	restorePineWorkerAssetSelector(t, pineworkerassets.Asset{
		Name:   "worker-test",
		Data:   []byte("embedded"),
		SHA256: "embedded-sha",
	}, true, nil)

	config, enabled, err := resolvePineWorkerRuntimeConfig(nil)
	if err != nil {
		t.Fatalf("resolvePineWorkerRuntimeConfig error = %v", err)
	}
	if !enabled || !config.embedded {
		t.Fatalf("enabled=%v embedded=%v, want embedded config", enabled, config.embedded)
	}
	if config.BinaryPath != "worker-test" || config.SHA256 != "embedded-sha" || string(config.binaryData) != "embedded" {
		t.Fatalf("config = %#v, want embedded asset metadata", config)
	}
}

func TestResolvePineWorkerRuntimeConfigPrefersExternalBinaryOverEmbeddedAsset(t *testing.T) {
	restorePineWorkerAssetSelector(t, pineworkerassets.Asset{
		Name:   "worker-embedded",
		Data:   []byte("embedded"),
		SHA256: "embedded-sha",
	}, true, nil)
	t.Setenv(envPineWorkerBinary, "/tmp/external-worker")

	config, enabled, err := resolvePineWorkerRuntimeConfig(nil)
	if err != nil {
		t.Fatalf("resolvePineWorkerRuntimeConfig error = %v", err)
	}
	if !enabled || config.embedded {
		t.Fatalf("enabled=%v embedded=%v, want external config", enabled, config.embedded)
	}
	if config.BinaryPath != "/tmp/external-worker" || len(config.binaryData) != 0 {
		t.Fatalf("config = %#v, want external binary path without embedded data", config)
	}
}

func TestResolvePineWorkerRuntimeConfigRejectsInvalidNumericEnv(t *testing.T) {
	t.Setenv(envPineWorkerBinary, "/tmp/worker")
	t.Setenv(envPineWorkerWorkers, "0")
	_, enabled, err := resolvePineWorkerRuntimeConfig(nil)
	if err == nil || !strings.Contains(err.Error(), "between 1 and 1000") {
		t.Fatalf("resolvePineWorkerRuntimeConfig error = %v, want workers range validation", err)
	}
	if enabled {
		t.Fatal("enabled = true, want false on invalid config")
	}
}

func TestServerStartsConfiguredPineWorkerManagerAndStopsOnClose(t *testing.T) {
	binaryPath := filepath.Join(t.TempDir(), "worker")
	if err := os.WriteFile(binaryPath, []byte("fake worker"), 0o755); err != nil {
		t.Fatalf("write worker: %v", err)
	}
	t.Setenv(envPineWorkerBinary, binaryPath)
	t.Setenv(envPineWorkerWorkers, "2")
	t.Setenv(envPineWorkerStartPort, "56001")

	launcher := &fakeServerPineWorkerLauncher{}
	dialer := newFakeServerPineWorkerDialer()
	restorePineWorkerFactories(t, launcher, dialer)

	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	if server.pineWorkerRunner == nil {
		t.Fatal("pineWorkerRunner = nil, want configured runner")
	}
	if launcher.startedCount() != 0 {
		t.Fatalf("started workers = %d before use, want lazy start", launcher.startedCount())
	}
	if _, err := server.pineWorkerRunner.RunScript(context.Background(), validServerPineWorkerRunScriptRequest("lazy-start")); err != nil {
		t.Fatalf("RunScript: %v", err)
	}
	if launcher.startedCount() != 2 {
		t.Fatalf("started workers = %d, want 2", launcher.startedCount())
	}
	if _, ok := dialer.transport("127.0.0.1:56001"); !ok {
		t.Fatalf("expected first worker transport, got %#v", dialer.addresses())
	}
}

func TestServerStartsEmbeddedPineWorkerManager(t *testing.T) {
	restorePineWorkerAssetSelector(t, pineworkerassets.Asset{
		Name:   "worker-embedded",
		Data:   []byte("embedded worker"),
		SHA256: "embedded-sha",
	}, true, nil)
	t.Setenv(envPineWorkerWorkers, "1")
	t.Setenv(envPineWorkerStartPort, "57001")

	launcher := &fakeServerPineWorkerLauncher{}
	dialer := newFakeServerPineWorkerDialer()
	restorePineWorkerFactories(t, launcher, dialer)

	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	if server.pineWorkerRunner == nil {
		t.Fatal("pineWorkerRunner = nil, want embedded runner")
	}
	if launcher.startedCount() != 0 {
		t.Fatalf("started workers = %d before use, want lazy start", launcher.startedCount())
	}
	if _, err := server.pineWorkerRunner.RunScript(context.Background(), validServerPineWorkerRunScriptRequest("embedded-lazy-start")); err != nil {
		t.Fatalf("RunScript: %v", err)
	}
	if launcher.startedCount() != 1 {
		t.Fatalf("started workers = %d, want 1", launcher.startedCount())
	}
	if _, ok := dialer.transport("127.0.0.1:57001"); !ok {
		t.Fatalf("expected embedded worker transport, got %#v", dialer.addresses())
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
	if server.pineWorkerRunner != nil {
		t.Fatal("pineWorkerRunner != nil without worker binary")
	}
	if _, err := server.designStore.saveDefinition(strategyDesignDefinition{
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

func TestLazyPineWorkerRunnerStopsIdleWorkers(t *testing.T) {
	binaryPath := filepath.Join(t.TempDir(), "worker")
	if err := os.WriteFile(binaryPath, []byte("fake worker"), 0o755); err != nil {
		t.Fatalf("write worker: %v", err)
	}
	launcher := &fakeServerPineWorkerLauncher{}
	dialer := newFakeServerPineWorkerDialer()
	restorePineWorkerFactories(t, launcher, dialer)

	manager, err := newPineWorkerManagerFromConfig(pineWorkerRuntimeConfig{
		BinaryPath:      binaryPath,
		Workers:         1,
		Host:            "127.0.0.1",
		StartPort:       58001,
		RequestTimeout:  time.Second,
		HealthTimeout:   100 * time.Millisecond,
		MaxMessageBytes: 1024 * 1024,
		MaxCandles:      1000,
	})
	if err != nil {
		t.Fatalf("newPineWorkerManagerFromConfig: %v", err)
	}
	runner := newLazyPineWorkerRunner(pineWorkerRuntimeConfig{Workers: 1}, manager, 10*time.Millisecond)
	if _, err := runner.RunScript(context.Background(), validServerPineWorkerRunScriptRequest("idle")); err != nil {
		t.Fatalf("RunScript: %v", err)
	}
	if launcher.startedCount() != 1 || !runner.IsRunning() {
		t.Fatalf("runner did not start lazily; started=%d running=%v", launcher.startedCount(), runner.IsRunning())
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if !runner.IsRunning() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("runner still running after idle timeout")
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
	store, err := bt.NewFutuKLineStore(dbPath)
	if err != nil {
		t.Fatalf("NewFutuKLineStore: %v", err)
	}
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
		run, ok, err := server.backtestRuns.getFull(runID)
		if err != nil {
			t.Fatalf("getFull: %v", err)
		}
		if ok && run.Status == want {
			return run
		}
		time.Sleep(10 * time.Millisecond)
	}
	run, _, _ := server.backtestRuns.getFull(runID)
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
	stops int
}

func (process *fakeServerPineWorkerProcess) Stop(context.Context) error {
	process.stops++
	return nil
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

func (dialer *fakeServerPineWorkerDialer) transport(address string) (*fakeServerPineWorkerTransport, bool) {
	dialer.mu.Lock()
	defer dialer.mu.Unlock()
	transport, ok := dialer.transports[address]
	return transport, ok
}

func (dialer *fakeServerPineWorkerDialer) addresses() []string {
	dialer.mu.Lock()
	defer dialer.mu.Unlock()
	addresses := make([]string, 0, len(dialer.transports))
	for address := range dialer.transports {
		addresses = append(addresses, address)
	}
	return addresses
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
	return pineworker.RunScriptResponse{
		JobID: request.JobID,
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
