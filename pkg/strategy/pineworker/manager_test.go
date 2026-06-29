package pineworker

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestWorkerManagerStartStopAndSnapshot(t *testing.T) {
	launcher := &fakeWorkerLauncher{}
	dialer := newFakeManagerDialer()
	manager := newTestManager(t, ManagerConfig{Workers: 2, StartPort: 51000}, launcher, dialer)

	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start error = %v", err)
	}
	if len(launcher.started) != 2 {
		t.Fatalf("started workers = %d, want 2", len(launcher.started))
	}
	if launcher.started[0].Address != "127.0.0.1:51000" || launcher.started[1].Address != "127.0.0.1:51001" {
		t.Fatalf("unexpected worker specs: %#v", launcher.started)
	}
	snapshot := manager.Snapshot()
	if len(snapshot) != 2 || !snapshot[0].Healthy || snapshot[0].WorkerID != "pineworker-1" {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}

	if err := manager.Stop(context.Background()); err != nil {
		t.Fatalf("Stop error = %v", err)
	}
	if launcher.processes[0].stops != 1 || launcher.processes[1].stops != 1 {
		t.Fatalf("process stops = %d/%d, want 1/1", launcher.processes[0].stops, launcher.processes[1].stops)
	}
	for address, transport := range dialer.transports {
		if transport.closes != 1 {
			t.Fatalf("transport %s closes = %d, want 1", address, transport.closes)
		}
	}
}

func TestWorkerManagerRunScriptRoundRobinsHealthyWorkers(t *testing.T) {
	launcher := &fakeWorkerLauncher{}
	dialer := newFakeManagerDialer()
	manager := newTestManager(t, ManagerConfig{Workers: 2}, launcher, dialer)
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	for index := 0; index < 3; index++ {
		response, err := manager.RunScript(context.Background(), validClientRequest())
		if err != nil {
			t.Fatalf("RunScript %d error = %v", index, err)
		}
		if response.JobID != "job-1" {
			t.Fatalf("RunScript %d response = %#v", index, response)
		}
	}

	first := dialer.transports["127.0.0.1:50051"]
	second := dialer.transports["127.0.0.1:50052"]
	if first.runs != 2 || second.runs != 1 {
		t.Fatalf("runs = %d/%d, want 2/1", first.runs, second.runs)
	}
}

func TestWorkerManagerCheckHealthRestartsFailedWorker(t *testing.T) {
	launcher := &fakeWorkerLauncher{}
	dialer := newFakeManagerDialer()
	manager := newTestManager(t, ManagerConfig{Workers: 2}, launcher, dialer)
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	failed := dialer.transports["127.0.0.1:50051"]
	failed.healthErr = errors.New("worker crashed")

	if err := manager.CheckHealth(context.Background()); err != nil {
		t.Fatalf("CheckHealth error = %v", err)
	}
	if failed.closes != 1 || launcher.processes[0].stops != 1 {
		t.Fatalf("failed worker was not stopped: closes=%d stops=%d", failed.closes, launcher.processes[0].stops)
	}
	restarted := dialer.latest["127.0.0.1:50051"]
	if restarted == failed || restarted.healthChecks != 1 {
		t.Fatalf("restart did not install a fresh transport")
	}
	snapshot := manager.Snapshot()
	if snapshot[0].Restarts != 1 || !snapshot[0].Healthy {
		t.Fatalf("snapshot after restart = %#v", snapshot)
	}
}

func TestWorkerManagerCheckHealthReportsRestartFailure(t *testing.T) {
	launcher := &fakeWorkerLauncher{failAfterStarts: 1}
	dialer := newFakeManagerDialer()
	manager := newTestManager(t, ManagerConfig{Workers: 1}, launcher, dialer)
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	dialer.latest["127.0.0.1:50051"].healthy = false

	err := manager.CheckHealth(context.Background())
	if err == nil || !strings.Contains(err.Error(), "start failed") {
		t.Fatalf("CheckHealth error = %v, want start failed", err)
	}
	snapshot := manager.Snapshot()
	if len(snapshot) != 1 || snapshot[0].Healthy || !strings.Contains(snapshot[0].LastError, "restart failed") {
		t.Fatalf("snapshot after failed restart = %#v", snapshot)
	}
}

func TestWorkerManagerStartCleansUpAfterDialFailure(t *testing.T) {
	launcher := &fakeWorkerLauncher{}
	dialer := newFakeManagerDialer()
	dialer.failAddress = "127.0.0.1:50052"
	manager := newTestManager(t, ManagerConfig{Workers: 2, HealthTimeout: 20 * time.Millisecond}, launcher, dialer)

	err := manager.Start(context.Background())
	if err == nil || !strings.Contains(err.Error(), "dial pineworker-2") {
		t.Fatalf("Start error = %v, want dial pineworker-2", err)
	}
	if len(manager.Snapshot()) != 0 {
		t.Fatalf("snapshot after failed start = %#v, want empty", manager.Snapshot())
	}
	if launcher.processes[0].stops != 1 || launcher.processes[1].stops != 1 {
		t.Fatalf("process stops after failed start = %d/%d, want 1/1", launcher.processes[0].stops, launcher.processes[1].stops)
	}
}

func TestWorkerManagerStartDialFailureIncludesProcessDiagnostics(t *testing.T) {
	launcher := &fakeWorkerLauncher{diagnostics: "binary=/tmp/worker; cwd=/repo; stderr=proto load failed"}
	dialer := newFakeManagerDialer()
	dialer.failAddress = "127.0.0.1:50051"
	manager := newTestManager(t, ManagerConfig{Workers: 1, HealthTimeout: 20 * time.Millisecond}, launcher, dialer)

	err := manager.Start(context.Background())
	if err == nil {
		t.Fatal("Start error = nil, want dial failure")
	}
	for _, want := range []string{
		"dial pineworker-1 at 127.0.0.1:50051",
		"pine worker process did not become ready",
		"stderr=proto load failed",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("Start error = %q, want %q", err.Error(), want)
		}
	}
	if launcher.processes[0].stops != 1 {
		t.Fatalf("failed worker stops = %d, want 1", launcher.processes[0].stops)
	}
}

func TestWorkerManagerStartRetriesDialUntilWorkerReady(t *testing.T) {
	launcher := &fakeWorkerLauncher{}
	dialer := newFakeManagerDialer()
	dialer.failDialAttempts["127.0.0.1:50051"] = 2
	manager := newTestManager(t, ManagerConfig{Workers: 1, HealthTimeout: time.Second}, launcher, dialer)

	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start error = %v", err)
	}
	if dialer.dialAttempts["127.0.0.1:50051"] != 3 {
		t.Fatalf("dial attempts = %d, want 3", dialer.dialAttempts["127.0.0.1:50051"])
	}
	if snapshot := manager.Snapshot(); len(snapshot) != 1 || !snapshot[0].Healthy || snapshot[0].PineTSVersion != "pinets-test" {
		t.Fatalf("snapshot after retry start = %#v", snapshot)
	}
}

func TestWorkerManagerStopReturnsFirstCloseError(t *testing.T) {
	launcher := &fakeWorkerLauncher{}
	dialer := newFakeManagerDialer()
	manager := newTestManager(t, ManagerConfig{Workers: 1}, launcher, dialer)
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	dialer.latest["127.0.0.1:50051"].closeErr = errors.New("close failed")

	err := manager.Stop(context.Background())
	if err == nil || !strings.Contains(err.Error(), "close failed") {
		t.Fatalf("Stop error = %v, want close failed", err)
	}
	if len(manager.Snapshot()) != 0 {
		t.Fatalf("snapshot after Stop = %#v, want empty", manager.Snapshot())
	}
}

func TestWorkerManagerRequiresDependenciesAndStart(t *testing.T) {
	if _, err := NewWorkerManager(ManagerConfig{Workers: 0}, &fakeWorkerLauncher{}, newFakeManagerDialer()); err == nil {
		t.Fatal("NewWorkerManager with zero workers error = nil, want error")
	}
	if _, err := NewWorkerManager(ManagerConfig{Workers: 1}, nil, newFakeManagerDialer()); err == nil {
		t.Fatal("NewWorkerManager without launcher error = nil, want error")
	}
	manager := newTestManager(t, ManagerConfig{Workers: 1}, &fakeWorkerLauncher{}, newFakeManagerDialer())
	if _, err := manager.RunScript(context.Background(), validClientRequest()); err == nil {
		t.Fatal("RunScript before Start error = nil, want error")
	}
}

func newTestManager(t *testing.T, config ManagerConfig, launcher WorkerLauncher, dialer TransportDialer) *WorkerManager {
	t.Helper()
	if config.WorkerConfig.RequestTimeout == 0 {
		config.WorkerConfig = DefaultWorkerConfig(max(1, config.Workers))
		config.WorkerConfig.RequestTimeout = time.Second
	}
	if config.Gate == (PerformanceGate{}) {
		config.Gate = relaxedGate()
	}
	manager, err := NewWorkerManager(config, launcher, dialer)
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

type fakeWorkerLauncher struct {
	started         []WorkerSpec
	processes       []*fakeWorkerProcess
	failAfterStarts int
	diagnostics     string
}

func (launcher *fakeWorkerLauncher) Start(ctx context.Context, spec WorkerSpec) (WorkerProcess, error) {
	if launcher.failAfterStarts > 0 && len(launcher.started) >= launcher.failAfterStarts {
		return nil, errors.New("start failed")
	}
	process := &fakeWorkerProcess{spec: spec, diagnostics: launcher.diagnostics}
	launcher.started = append(launcher.started, spec)
	launcher.processes = append(launcher.processes, process)
	return process, nil
}

type fakeWorkerProcess struct {
	spec        WorkerSpec
	stops       int
	diagnostics string
}

func (process *fakeWorkerProcess) Stop(context.Context) error {
	process.stops++
	return nil
}

func (process *fakeWorkerProcess) Diagnostics() string {
	return process.diagnostics
}

type fakeManagerDialer struct {
	transports       map[string]*fakeManagedTransport
	latest           map[string]*fakeManagedTransport
	failAddress      string
	failDialAttempts map[string]int
	dialAttempts     map[string]int
}

func newFakeManagerDialer() *fakeManagerDialer {
	return &fakeManagerDialer{
		transports:       map[string]*fakeManagedTransport{},
		latest:           map[string]*fakeManagedTransport{},
		failDialAttempts: map[string]int{},
		dialAttempts:     map[string]int{},
	}
}

func (dialer *fakeManagerDialer) Dial(ctx context.Context, address string) (ManagedTransport, error) {
	dialer.dialAttempts[address]++
	if dialer.failAddress == address {
		return nil, errors.New("dial failed")
	}
	if dialer.dialAttempts[address] <= dialer.failDialAttempts[address] {
		return nil, errors.New("dial not ready")
	}
	transport := &fakeManagedTransport{
		address: address,
		healthy: true,
		status: HealthStatus{
			OK:            true,
			WorkerID:      address,
			Version:       "0.1.0",
			PineTSVersion: "pinets-test",
			Capabilities:  []string{"run"},
		},
	}
	if _, exists := dialer.transports[address]; !exists {
		dialer.transports[address] = transport
	}
	dialer.latest[address] = transport
	return transport, nil
}

type fakeManagedTransport struct {
	address      string
	healthy      bool
	status       HealthStatus
	healthErr    error
	closeErr     error
	healthChecks int
	runs         int
	closes       int
}

func (transport *fakeManagedTransport) RunScript(ctx context.Context, request RunScriptRequest) (RunScriptResponse, error) {
	transport.runs++
	return RunScriptResponse{
		JobID: request.JobID,
		Metadata: WorkerMetadata{
			WorkerID:      transport.address,
			Duration:      time.Millisecond,
			RequestBytes:  100,
			ResponseBytes: 100,
		},
	}, nil
}

func (transport *fakeManagedTransport) HealthCheck(context.Context) (HealthStatus, error) {
	transport.healthChecks++
	if transport.healthErr != nil {
		return HealthStatus{}, transport.healthErr
	}
	status := transport.status
	status.OK = transport.healthy
	return status, nil
}

func (transport *fakeManagedTransport) Close() error {
	transport.closes++
	return transport.closeErr
}
