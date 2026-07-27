package pineruntime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/pineworkerassets"
	"github.com/jftrade/jftrade-main/pkg/jftsettings"
	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

func TestResolveConfigUsesEnvironmentAndSettings(t *testing.T) {
	bundlePath := filepath.Join(t.TempDir(), "worker.mjs")
	t.Setenv(EnvBundle, bundlePath)
	t.Setenv(EnvSHA256, "abc123")
	t.Setenv(EnvBacktestWorkers, "3")
	t.Setenv(EnvInstanceWorkers, "7")
	t.Setenv(EnvHost, "localhost")
	t.Setenv(EnvStartPort, "55001")
	t.Setenv(EnvTempDir, t.TempDir())
	t.Setenv(EnvProto, "proto/pineworker.proto")
	t.Setenv(EnvPineTSVersion, "1.2.3")
	t.Setenv(EnvMock, "true")
	t.Setenv(EnvRequestTimeout, "2s")
	t.Setenv(EnvHealthTimeout, "500ms")
	t.Setenv(EnvMaxMessageBytes, "1048576")
	t.Setenv(EnvMaxCandles, "1000")
	t.Setenv(EnvMaxDuration, "5s")
	t.Setenv(EnvMaxDurationPerBar, "1ms")
	t.Setenv(EnvMinCandlesPerSec, "2500")
	t.Setenv(EnvMaxPeakRSSBytes, "33554432")

	config, enabled, err := ResolveConfig(jftsettings.PineWorkerSettings{NodeBinaryPath: ` "custom-node" `})
	if err != nil || !enabled {
		t.Fatalf("ResolveConfig enabled=%v error=%v", enabled, err)
	}
	if config.BundlePath != bundlePath || config.RuntimePath != "custom-node" || config.SHA256 != "abc123" {
		t.Fatalf("identity config = %#v", config)
	}
	if config.BacktestWorkers != 3 || config.InstanceWorkers != 7 || config.Host != "localhost" || config.StartPort != 55001 {
		t.Fatalf("worker config = %#v", config)
	}
	if !config.Mock || config.RequestTimeout != 2*time.Second || config.HealthTimeout != 500*time.Millisecond {
		t.Fatalf("runtime config = %#v", config)
	}
	if config.MaxMessageBytes != 1048576 || config.MaxCandles != 1000 || config.MaxPeakRSSBytes != 33554432 {
		t.Fatalf("resource config = %#v", config)
	}
	if config.MaxDuration != 5*time.Second || config.MaxDurationPerBar != time.Millisecond || config.MinCandlesPerSec != 2500 {
		t.Fatalf("gate config = %#v", config)
	}
}

func TestResolveConfigSelectsEmbeddedAssetAndExternalOverride(t *testing.T) {
	selector := func() (pineworkerassets.Asset, bool, error) {
		return pineworkerassets.Asset{Name: "embedded-worker.mjs", Data: []byte("worker"), SHA256: "embedded-sha"}, true, nil
	}
	config, enabled, err := ResolveConfig(jftsettings.PineWorkerSettings{}, WithAssetSelector(selector))
	if err != nil || !enabled || config.Source() != "embedded" {
		t.Fatalf("embedded config = %#v enabled=%v error=%v", config, enabled, err)
	}
	if config.BundlePath != "embedded-worker.mjs" || string(config.bundleData) != "worker" || config.SHA256 != "embedded-sha" {
		t.Fatalf("embedded identity = %#v", config)
	}

	t.Setenv(EnvBundle, "/tmp/external-worker.mjs")
	config, enabled, err = ResolveConfig(jftsettings.PineWorkerSettings{}, WithAssetSelector(selector))
	if err != nil || !enabled || config.Source() != "external" || len(config.bundleData) != 0 {
		t.Fatalf("external config = %#v enabled=%v error=%v", config, enabled, err)
	}
}

func TestResolveConfigDisabledAndInvalidLimits(t *testing.T) {
	t.Setenv(EnvDisabled, "true")
	if config, enabled, err := ResolveConfig(jftsettings.PineWorkerSettings{}); err != nil || enabled || config.BundlePath != "" {
		t.Fatalf("disabled config = %#v enabled=%v error=%v", config, enabled, err)
	}
	t.Setenv(EnvDisabled, "false")
	t.Setenv(EnvBundle, filepath.Join(t.TempDir(), "worker.mjs"))
	cases := []struct {
		key   string
		value string
	}{
		{EnvBacktestWorkers, "0"}, {EnvInstanceWorkers, "1001"}, {EnvStartPort, "0"},
		{EnvRequestTimeout, "bad"}, {EnvHealthTimeout, "0s"}, {EnvMaxMessageBytes, "-1"},
		{EnvMaxCandles, "0"}, {EnvMaxDuration, "bad"}, {EnvMaxDurationPerBar, "0s"},
		{EnvMinCandlesPerSec, "0"}, {EnvMaxPeakRSSBytes, "bad"},
	}
	for _, testCase := range cases {
		t.Run(testCase.key, func(t *testing.T) {
			t.Setenv(testCase.key, testCase.value)
			if _, enabled, err := ResolveConfig(jftsettings.PineWorkerSettings{}); err == nil || enabled {
				t.Fatalf("enabled=%v error=%v, want invalid config", enabled, err)
			}
		})
	}
}

func TestResolveConfigUsesWorkerDefaultsAndRuntimePrecedence(t *testing.T) {
	t.Setenv(EnvBundle, filepath.Join(t.TempDir(), "worker.mjs"))
	t.Setenv(EnvRuntime, "env-node")
	t.Setenv("JFTRADE_NODE_BINARY", "legacy-node")
	config, enabled, err := ResolveConfig(jftsettings.PineWorkerSettings{NodeBinaryPath: ` 'settings-node' `})
	if err != nil || !enabled {
		t.Fatalf("ResolveConfig enabled=%v error=%v", enabled, err)
	}
	if config.RuntimePath != "settings-node" || config.BacktestWorkers != 2 || config.InstanceWorkers != 10 {
		t.Fatalf("defaults config = %#v", config)
	}
}

func TestResolveConfigFindsRepositoryAndHonorsProtoOverride(t *testing.T) {
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
	nested := filepath.Join(root, "var", "worker")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvBundle, filepath.Join(nested, "worker.mjs"))
	customProto := filepath.Join(t.TempDir(), "custom.proto")
	t.Setenv(EnvProto, customProto)
	deps := defaultDependencies()
	deps.getwd = func() (string, error) { return nested, nil }
	config, enabled, err := resolveConfig(jftsettings.PineWorkerSettings{}, deps)
	if err != nil || !enabled || config.WorkDir != root || config.ProtoPath != customProto {
		t.Fatalf("config = %#v enabled=%v error=%v", config, enabled, err)
	}
}

func TestRuntimeDependencyOptionsAndDefaultFactories(t *testing.T) {
	t.Setenv(EnvBundle, filepath.Join(t.TempDir(), "worker.mjs"))
	config, enabled, err := ResolveConfig(
		jftsettings.PineWorkerSettings{},
		WithRuntimeResolver(func(jftsettings.PineWorkerSettings) string { return "resolved-node" }),
	)
	if err != nil || !enabled || config.RuntimePath != "resolved-node" {
		t.Fatalf("config = %#v enabled=%v error=%v", config, enabled, err)
	}
	launcher, err := NewNodeLauncher(Config{
		BundlePath: "worker.mjs", RuntimePath: "node", ProtoPath: "pineworker.proto", MaxMessageBytes: 1024,
	}, []byte("console.log('worker')"))
	if err != nil || launcher == nil {
		t.Fatalf("NewNodeLauncher = %#v, %v", launcher, err)
	}
	if dialer := NewGRPCDialer(1024); dialer == nil {
		t.Fatal("NewGRPCDialer returned nil")
	}
}

func TestRuntimePathAndWorkDirFallbacks(t *testing.T) {
	deps := defaultDependencies()
	deps.getwd = func() (string, error) { return "", errors.New("getwd failed") }
	deps.abs = func(string) (string, error) { return "", errors.New("abs failed") }
	if got := resolveWorkDir("", deps); got != "" {
		t.Fatalf("work dir = %q, want empty", got)
	}
	if got := resolvePath("relative.proto", "", deps.abs); got != "relative.proto" {
		t.Fatalf("relative fallback = %q", got)
	}
	plainDir := t.TempDir()
	deps.getwd = func() (string, error) { return plainDir, nil }
	if got := resolveWorkDir("", deps); got != plainDir {
		t.Fatalf("plain work dir = %q, want %q", got, plainDir)
	}
	if got := resolvePath("child.proto", plainDir, filepath.Abs); got != filepath.Join(plainDir, "child.proto") {
		t.Fatalf("based path = %q", got)
	}
}

func TestManagerBuildsPublishesAndRetiresRunnerPairs(t *testing.T) {
	launcher := &fakeLauncher{}
	dialer := newFakeDialer()
	manager := newTestManager(launcher, dialer)
	var publishedBacktest Runner
	var publishedInstance Runner
	config, enabled, err := manager.Reconfigure(jftsettings.PineWorkerSettings{}, func(backtest Runner, instance Runner) {
		publishedBacktest = backtest
		publishedInstance = instance
	})
	if err != nil || !enabled || config.Source() != "embedded" {
		t.Fatalf("Reconfigure enabled=%v config=%#v error=%v", enabled, config, err)
	}
	if publishedBacktest == nil || publishedInstance == nil {
		t.Fatal("published runner pair is incomplete")
	}
	retiredBacktest := publishedBacktest
	if _, err := publishedBacktest.RunScript(context.Background(), validRequest("backtest")); err != nil {
		t.Fatalf("RunScript: %v", err)
	}
	if launcher.startedCount() != 1 || launcher.stoppedCount() != 1 {
		t.Fatalf("worker lifecycle = started %d stopped %d", launcher.startedCount(), launcher.stoppedCount())
	}

	t.Setenv(EnvDisabled, "true")
	_, enabled, err = manager.Reconfigure(jftsettings.PineWorkerSettings{}, func(backtest Runner, instance Runner) {
		publishedBacktest = backtest
		publishedInstance = instance
	})
	if err != nil || enabled || publishedBacktest != nil || publishedInstance != nil {
		t.Fatalf("disabled publish = %#v/%#v enabled=%v error=%v", publishedBacktest, publishedInstance, enabled, err)
	}
	if err := manager.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := manager.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if _, err := retiredBacktest.RunScript(context.Background(), validRequest("retired")); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("retired runner error = %v", err)
	}
}

func TestManagerDoesNotPublishPartialPairAndRollsBack(t *testing.T) {
	calls := 0
	manager := NewManager(
		WithAssetSelector(embeddedAsset),
		WithLauncherFactory(func(Config, []byte) (pineworker.WorkerLauncher, error) {
			calls++
			if calls == 2 {
				return nil, errors.New("launcher unavailable")
			}
			return &fakeLauncher{}, nil
		}),
		WithDialerFactory(func(int) pineworker.TransportDialer { return newFakeDialer() }),
	)
	published := false
	_, enabled, err := manager.Reconfigure(jftsettings.PineWorkerSettings{}, func(backtest Runner, instance Runner) {
		published = backtest != nil || instance != nil
	})
	if err == nil || enabled || published || calls != 2 {
		t.Fatalf("enabled=%v published=%v calls=%d error=%v", enabled, published, calls, err)
	}
}

func TestManagerNilClosedAndNonClosableRunnerBoundaries(t *testing.T) {
	var nilManager *Manager
	if _, _, err := nilManager.Reconfigure(jftsettings.PineWorkerSettings{}, nil); err == nil {
		t.Fatal("nil manager Reconfigure should fail")
	}
	if backtest, instance := nilManager.Runners(); backtest != nil || instance != nil {
		t.Fatalf("nil manager runners = %#v/%#v", backtest, instance)
	}
	if err := nilManager.Close(); err != nil {
		t.Fatalf("nil manager Close: %v", err)
	}
	if err := closeRunner(nonClosableRunner{}); err != nil {
		t.Fatalf("non-closable runner close: %v", err)
	}
	closeErr := CloseRunners(closeErrorRunner{err: errors.New("backtest close")}, closeErrorRunner{err: errors.New("instance close")})
	if closeErr == nil || !strings.Contains(closeErr.Error(), "backtestPineWorkerRunner close") || !strings.Contains(closeErr.Error(), "instancePineWorkerRunner close") {
		t.Fatalf("CloseRunners error = %v", closeErr)
	}

	manager := newTestManager(&fakeLauncher{}, newFakeDialer())
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}
	if _, enabled, err := manager.Reconfigure(jftsettings.PineWorkerSettings{}, nil); err == nil || enabled {
		t.Fatalf("closed manager enabled=%v error=%v", enabled, err)
	}
}

func TestEphemeralRunnerConcurrencyAndLiveSessionLifecycle(t *testing.T) {
	t.Setenv(EnvInstanceWorkers, "1")
	launcher := &fakeLauncher{}
	dialer := newFakeDialer()
	manager := newTestManager(launcher, dialer)
	if _, _, err := manager.Reconfigure(jftsettings.PineWorkerSettings{}, nil); err != nil {
		t.Fatal(err)
	}
	backtestRaw, instanceRaw := manager.Runners()
	backtest := backtestRaw.(*ephemeralRunner)
	instance := instanceRaw.(*ephemeralRunner)
	if err := backtest.acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	backtest.config.BacktestWorkers = 1
	backtest.release()
	if err := instance.acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	var capacityErr pineworker.CapacityExceededError
	if err := instance.acquire(context.Background()); !errors.As(err, &capacityErr) {
		t.Fatalf("second instance acquire error = %v", err)
	}
	instance.release()

	ctx, cancel := context.WithCancel(context.Background())
	opener := instanceRaw.(LiveSessionOpener)
	session, response, err := opener.OpenLiveSession(ctx, validRequest("open"))
	if err != nil || session == nil || response.SessionRevision != 1 {
		t.Fatalf("OpenLiveSession response=%#v error=%v", response, err)
	}
	response, err = session.Append(context.Background(), validRequest("append"))
	if err != nil || response.SessionRevision != 2 {
		t.Fatalf("Append response=%#v error=%v", response, err)
	}
	if err := session.Close(context.Background()); err != nil {
		t.Fatalf("session Close: %v", err)
	}
	if _, err := session.Append(context.Background(), validRequest("closed")); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("Append after close error = %v", err)
	}
	cancel()
	if err := manager.Close(); err != nil {
		t.Fatalf("manager Close: %v", err)
	}
}

func TestEphemeralRunnerFailureBoundaries(t *testing.T) {
	var nilRunner *ephemeralRunner
	if _, err := nilRunner.RunScript(context.Background(), pineworker.RunScriptRequest{}); err == nil {
		t.Fatal("nil runner should fail")
	}
	if _, err := newEphemeralRunner(Config{BundlePath: filepath.Join(t.TempDir(), "missing")}, runnerBacktest, defaultDependencies()); err == nil {
		t.Fatal("missing bundle should fail")
	}
	if _, err := freePort(context.Background(), "256.256.256.256"); err == nil {
		t.Fatal("invalid host should fail")
	}
	if got := (&ephemeralRunner{}).stopTimeout(); got != 5*time.Second {
		t.Fatalf("default stop timeout = %v", got)
	}
	if got := (&ephemeralRunner{config: Config{RequestTimeout: 30 * time.Second}}).stopTimeout(); got != 10*time.Second {
		t.Fatalf("capped stop timeout = %v", got)
	}
}

func TestEphemeralRunnerOpenSessionFailureBoundaries(t *testing.T) {
	closed := &ephemeralRunner{busy: make(chan struct{}, 1), closed: true}
	if _, _, err := closed.OpenLiveSession(context.Background(), validRequest("closed")); err == nil {
		t.Fatal("closed runner should reject session")
	}
	busy := &ephemeralRunner{busy: make(chan struct{}, 1), rejectWhenBusy: true}
	busy.busy <- struct{}{}
	if _, _, err := busy.OpenLiveSession(context.Background(), validRequest("busy")); !errors.Is(err, pineworker.ErrCapacityExceeded) {
		t.Fatalf("busy session error = %v", err)
	}
	badHost := &ephemeralRunner{config: Config{Host: "256.256.256.256"}, busy: make(chan struct{}, 1)}
	if _, _, err := badHost.OpenLiveSession(context.Background(), validRequest("host")); err == nil {
		t.Fatal("invalid host should reject session")
	}
	startFailure := &ephemeralRunner{
		config: Config{Host: "127.0.0.1", HealthTimeout: time.Millisecond}, launcher: failingLauncher{},
		dialer: newFakeDialer(), busy: make(chan struct{}, 1),
	}
	if _, _, err := startFailure.OpenLiveSession(context.Background(), validRequest("start")); err == nil {
		t.Fatal("worker start failure should reject session")
	}
}

func newTestManager(launcher pineworker.WorkerLauncher, dialer pineworker.TransportDialer) *Manager {
	return NewManager(
		WithAssetSelector(embeddedAsset),
		WithLauncherFactory(func(Config, []byte) (pineworker.WorkerLauncher, error) { return launcher, nil }),
		WithDialerFactory(func(int) pineworker.TransportDialer { return dialer }),
	)
}

func embeddedAsset() (pineworkerassets.Asset, bool, error) {
	return pineworkerassets.Asset{Name: "worker.mjs", Data: []byte("worker"), SHA256: "sha"}, true, nil
}

func validRequest(jobID string) pineworker.RunScriptRequest {
	return pineworker.RunScriptRequest{
		JobID: jobID, ScriptID: "script", Source: "//@version=6\nstrategy(\"test\")",
		Symbol: "US.AAPL", Timeframe: "1m", Mode: pineworker.ModeBacktest,
		Candles: []pineworker.Candle{{OpenTime: 1, CloseTime: 2, Open: 1, High: 2, Low: 1, Close: 2, Volume: 100}},
	}
}

type fakeLauncher struct {
	mu        sync.Mutex
	processes []*fakeProcess
}

type failingLauncher struct{}

func (failingLauncher) Start(context.Context, pineworker.WorkerSpec) (pineworker.WorkerProcess, error) {
	return nil, errors.New("worker start failed")
}

type nonClosableRunner struct{}

func (nonClosableRunner) RunScript(context.Context, pineworker.RunScriptRequest) (pineworker.RunScriptResponse, error) {
	return pineworker.RunScriptResponse{}, nil
}

type closeErrorRunner struct{ err error }

func (runner closeErrorRunner) RunScript(context.Context, pineworker.RunScriptRequest) (pineworker.RunScriptResponse, error) {
	return pineworker.RunScriptResponse{}, nil
}

func (runner closeErrorRunner) Close(context.Context) error { return runner.err }

func (launcher *fakeLauncher) Start(ctx context.Context, _ pineworker.WorkerSpec) (pineworker.WorkerProcess, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	process := &fakeProcess{}
	launcher.processes = append(launcher.processes, process)
	return process, nil
}

func (launcher *fakeLauncher) startedCount() int {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return len(launcher.processes)
}

func (launcher *fakeLauncher) stoppedCount() int {
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

type fakeProcess struct {
	mu    sync.Mutex
	stops int
}

func (process *fakeProcess) Stop(context.Context) error {
	process.mu.Lock()
	defer process.mu.Unlock()
	process.stops++
	return nil
}

type fakeDialer struct {
	mu         sync.Mutex
	transports map[string]*fakeTransport
}

func newFakeDialer() *fakeDialer {
	return &fakeDialer{transports: make(map[string]*fakeTransport)}
}

func (dialer *fakeDialer) Dial(ctx context.Context, address string) (pineworker.ManagedTransport, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dialer.mu.Lock()
	defer dialer.mu.Unlock()
	transport := &fakeTransport{address: address}
	dialer.transports[address] = transport
	return transport, nil
}

type fakeTransport struct {
	address string
	closed  bool
}

func (transport *fakeTransport) RunScript(_ context.Context, request pineworker.RunScriptRequest) (pineworker.RunScriptResponse, error) {
	revision := request.ExpectedRevision
	switch request.SessionOperation {
	case pineworker.SessionOperationOpen:
		revision = 1
	case pineworker.SessionOperationAppend:
		revision++
	}
	return pineworker.RunScriptResponse{
		JobID: request.JobID, SessionID: request.SessionID, SessionRevision: revision,
		Metadata: pineworker.WorkerMetadata{Duration: 100 * time.Microsecond, RequestBytes: 100, ResponseBytes: 100},
	}, nil
}

func (transport *fakeTransport) HealthCheck(context.Context) (pineworker.HealthStatus, error) {
	return pineworker.HealthStatus{OK: true, WorkerID: transport.address}, nil
}

func (transport *fakeTransport) Close() error {
	transport.closed = true
	return nil
}
