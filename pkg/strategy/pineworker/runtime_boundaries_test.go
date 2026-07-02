package pineworker

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker/pineworkerpb"
	"google.golang.org/grpc"
)

func TestTailBufferRetainsOnlyConfiguredProcessLogTail(t *testing.T) {
	buffer := NewTailBuffer(5)
	if n, err := buffer.Write([]byte("abc")); err != nil || n != 3 {
		t.Fatalf("Write(first) = (%d, %v)", n, err)
	}
	if n, err := buffer.Write([]byte("defg")); err != nil || n != 4 {
		t.Fatalf("Write(second) = (%d, %v)", n, err)
	}
	if got := buffer.String(); got != "cdefg" {
		t.Fatalf("String() = %q, want cdefg", got)
	}

	defaultBuffer := NewTailBuffer(0)
	if defaultBuffer.maxBytes != 4096 {
		t.Fatalf("default max bytes = %d, want 4096", defaultBuffer.maxBytes)
	}
	var nilBuffer *TailBuffer
	if n, err := nilBuffer.Write([]byte("discarded")); err != nil || n != len("discarded") {
		t.Fatalf("nil Write() = (%d, %v)", n, err)
	}
	if got := nilBuffer.String(); got != "" {
		t.Fatalf("nil String() = %q", got)
	}
}

func TestPerformanceGateRejectsLatencyAndThroughputBoundaries(t *testing.T) {
	if got := DefaultPerformanceGate(); got != (PerformanceGate{}) {
		t.Fatalf("DefaultPerformanceGate() = %#v", got)
	}
	tests := []struct {
		name   string
		sample PerformanceSample
		gate   PerformanceGate
		want   string
	}{
		{name: "duration required", sample: PerformanceSample{Candles: 1}, want: "duration must be positive"},
		{name: "per candle latency", sample: PerformanceSample{Candles: 2, Duration: 6 * time.Millisecond}, gate: PerformanceGate{MaxDurationPerBar: 2 * time.Millisecond}, want: "duration per candle"},
		{name: "throughput", sample: PerformanceSample{Candles: 2, Duration: 2 * time.Second}, gate: PerformanceGate{MinCandlesPerSec: 2}, want: "throughput 1.00 candles/sec below gate 2.00"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckPerformanceGate(tt.sample, tt.gate)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("CheckPerformanceGate() error = %v, want containing %q", err, tt.want)
			}
		})
	}
	if got := CandlesPerSecond(PerformanceSample{Candles: 10}); got != 0 {
		t.Fatalf("CandlesPerSecond(zero duration) = %f, want 0", got)
	}
}

func TestWorkerConfigAndCandleTimeBoundaries(t *testing.T) {
	config := DefaultWorkerConfig(0)
	if config.LiveWorkers != 2 || config.BacktestWorkers != 1 || config.OptimizationWorkers != 1 {
		t.Fatalf("DefaultWorkerConfig(0) = %#v", config)
	}
	if err := validateCandle(Candle{High: 1, Low: 0}); err == nil || err.Error() != "open time is required" {
		t.Fatalf("validateCandle(missing open time) error = %v", err)
	}
	if err := validateCandle(Candle{OpenTime: 2, CloseTime: 1, High: 1, Low: 0}); err == nil || err.Error() != "close time is before open time" {
		t.Fatalf("validateCandle(reversed time) error = %v", err)
	}
}

func TestClientDefaultsAndResponseIdentityBoundaries(t *testing.T) {
	transport := fakeTransport{run: func(_ context.Context, request RunScriptRequest) (RunScriptResponse, error) {
		return RunScriptResponse{Metadata: WorkerMetadata{Duration: time.Millisecond}}, nil
	}}
	client, err := NewClient(transport, WorkerConfig{}, WithNow(nil))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if client.config.RequestTimeout != 30*time.Second || client.now == nil {
		t.Fatalf("client defaults = timeout:%s nowNil:%t", client.config.RequestTimeout, client.now == nil)
	}
	response, err := client.RunScript(t.Context(), validClientRequest())
	if err != nil {
		t.Fatalf("RunScript() error = %v", err)
	}
	if response.JobID != "job-1" {
		t.Fatalf("RunScript() job ID = %q, want job-1", response.JobID)
	}

	var nilClient *Client
	if _, err := nilClient.RunScript(t.Context(), validClientRequest()); err == nil || err.Error() != "pine worker client is nil" {
		t.Fatalf("nil client RunScript() error = %v", err)
	}
	if err := mapTransportError(nil); err != nil {
		t.Fatalf("mapTransportError(nil) = %v", err)
	}
}

func TestGRPCTransportPropagatesRPCFailures(t *testing.T) {
	rpcErr := errors.New("worker rpc unavailable")
	transport, err := NewGRPCTransportWithClient(failingPineWorkerClient{err: rpcErr})
	if err != nil {
		t.Fatalf("NewGRPCTransportWithClient() error = %v", err)
	}
	if _, err := transport.RunScript(t.Context(), validClientRequest()); !errors.Is(err, rpcErr) {
		t.Fatalf("RunScript() error = %v, want rpc error", err)
	}
	if _, err := transport.HealthCheck(t.Context()); !errors.Is(err, rpcErr) {
		t.Fatalf("HealthCheck() error = %v, want rpc error", err)
	}
	var nilTransport *GRPCTransport
	if _, err := nilTransport.HealthCheck(t.Context()); err == nil || !strings.Contains(err.Error(), "not initialized") {
		t.Fatalf("nil HealthCheck() error = %v", err)
	}

	dialer := NewGRPCDialer(GRPCDialerConfig{})
	if dialer.config.MaxMessageBytes != grpcUnlimitedMessageBytes {
		t.Fatalf("default max message bytes = %d", dialer.config.MaxMessageBytes)
	}
	var nilManaged *managedGRPCTransport
	if err := nilManaged.Close(); err != nil {
		t.Fatalf("nil managed transport Close() error = %v", err)
	}
}

func TestWorkerManagerSelectionCapacityAndErrorBoundaries(t *testing.T) {
	if got := (CapacityExceededError{}).Error(); got != ErrCapacityExceeded.Error() {
		t.Fatalf("zero-worker capacity error = %q", got)
	}
	if _, err := NewWorkerManager(ManagerConfig{Workers: 1}, &fakeWorkerLauncher{}, nil); err == nil || err.Error() != "pine worker dialer is required" {
		t.Fatalf("NewWorkerManager(nil dialer) error = %v", err)
	}

	manager := newTestManager(t, ManagerConfig{Workers: 1}, &fakeWorkerLauncher{}, newFakeManagerDialer())
	if err := manager.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := manager.Start(t.Context()); err != nil {
		t.Fatalf("Start(second) error = %v", err)
	}

	capacity := &WorkerManager{config: ManagerConfig{}, busy: make(chan struct{}, 1)}
	capacity.busy <- struct{}{}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if err := capacity.acquireRunSlot(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("acquireRunSlot(canceled) error = %v", err)
	}
	<-capacity.busy
	capacity.releaseRunSlot()

	empty := &WorkerManager{}
	if _, err := empty.pickWorker(); err == nil || !strings.Contains(err.Error(), "not started") {
		t.Fatalf("pickWorker(empty) error = %v", err)
	}
	unhealthy := &WorkerManager{
		config:  ManagerConfig{WorkerConfig: DefaultWorkerConfig(1)},
		busy:    make(chan struct{}, 1),
		workers: []*managedWorker{{healthy: false}},
	}
	if _, err := unhealthy.RunScript(t.Context(), validClientRequest()); err == nil || !strings.Contains(err.Error(), "no healthy") {
		t.Fatalf("RunScript(unhealthy) error = %v", err)
	}

	failingClient, err := NewClient(fakeTransport{run: func(context.Context, RunScriptRequest) (RunScriptResponse, error) {
		return RunScriptResponse{}, errors.New("worker execution failed")
	}}, DefaultWorkerConfig(1))
	if err != nil {
		t.Fatalf("NewClient(failing) error = %v", err)
	}
	failingManager := &WorkerManager{
		config:  ManagerConfig{WorkerConfig: DefaultWorkerConfig(1)},
		busy:    make(chan struct{}, 1),
		workers: []*managedWorker{{healthy: true, client: failingClient}},
	}
	if _, err := failingManager.RunScript(t.Context(), validClientRequest()); err == nil || !strings.Contains(err.Error(), "transport error") {
		t.Fatalf("RunScript(failing worker) error = %v", err)
	}
}

func TestWorkerManagerDiagnosticErrorFormatting(t *testing.T) {
	if got := processDiagnostics(noDiagnosticsProcess{}); got != "" {
		t.Fatalf("processDiagnostics() = %q", got)
	}
	tests := []struct {
		name       string
		healthErr  error
		restartErr error
		status     HealthStatus
		want       string
	}{
		{name: "both failures", healthErr: errors.New("health failed"), restartErr: errors.New("restart failed"), want: "health check failed: health failed; restart failed: restart failed"},
		{name: "unhealthy restart failure", restartErr: errors.New("restart failed"), status: HealthStatus{OK: false}, want: "worker unhealthy: ok=false; restart failed: restart failed"},
		{name: "health only", healthErr: errors.New("health failed"), want: "health failed"},
		{name: "unhealthy only", want: "worker unhealthy"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := joinHealthError(tt.healthErr, tt.restartErr, tt.status); got != tt.want {
				t.Fatalf("joinHealthError() = %q, want %q", got, tt.want)
			}
		})
	}
}

type failingPineWorkerClient struct{ err error }

func (client failingPineWorkerClient) HealthCheck(context.Context, *pineworkerpb.HealthCheckRequest, ...grpc.CallOption) (*pineworkerpb.HealthCheckResponse, error) {
	return nil, client.err
}

func (client failingPineWorkerClient) AnalyzeScript(context.Context, *pineworkerpb.AnalyzeScriptRequest, ...grpc.CallOption) (*pineworkerpb.AnalyzeScriptResponse, error) {
	return nil, client.err
}

func (client failingPineWorkerClient) RunScript(context.Context, *pineworkerpb.RunScriptRequest, ...grpc.CallOption) (*pineworkerpb.RunScriptResponse, error) {
	return nil, client.err
}

type noDiagnosticsProcess struct{}

func (noDiagnosticsProcess) Stop(context.Context) error { return nil }
