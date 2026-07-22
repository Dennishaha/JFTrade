package pineworker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jftrade/jftrade-main/pkg/observability"
)

var ErrCapacityExceeded = errors.New("pine worker capacity exceeded")

type CapacityExceededError struct {
	Workers int
}

func (e CapacityExceededError) Error() string {
	if e.Workers > 0 {
		return fmt.Sprintf("pine worker capacity exceeded: %d workers are busy", e.Workers)
	}
	return ErrCapacityExceeded.Error()
}

func (e CapacityExceededError) Is(target error) bool {
	return target == ErrCapacityExceeded
}

type WorkerSpec struct {
	WorkerID string
	Address  string
	Port     int
}

type WorkerProcess interface {
	Stop(context.Context) error
}

type WorkerLauncher interface {
	Start(context.Context, WorkerSpec) (WorkerProcess, error)
}

type ManagedTransport interface {
	Transport
	HealthCheck(context.Context) (HealthStatus, error)
	Close() error
}

type TransportDialer interface {
	Dial(context.Context, string) (ManagedTransport, error)
}

type ManagerConfig struct {
	Workers        int
	WorkerIDPrefix string
	Host           string
	StartPort      int
	HealthTimeout  time.Duration
	WorkerConfig   WorkerConfig
	Gate           PerformanceGate
	RejectWhenBusy bool
}

type WorkerSnapshot struct {
	WorkerID      string
	Address       string
	Port          int
	Healthy       bool
	Restarts      int
	LastError     string
	Version       string
	PineTSVersion string
	Capabilities  []string
}

type WorkerManager struct {
	mu             sync.Mutex
	config         ManagerConfig
	launcher       WorkerLauncher
	dialer         TransportDialer
	workers        []*managedWorker
	next           int
	busy           chan struct{}
	sessionWorkers map[string]*managedWorker
}

func NewWorkerManager(config ManagerConfig, launcher WorkerLauncher, dialer TransportDialer) (*WorkerManager, error) {
	if config.Workers <= 0 {
		return nil, fmt.Errorf("pine worker manager requires at least one worker")
	}
	if config.Host == "" {
		config.Host = "127.0.0.1"
	}
	if config.StartPort <= 0 {
		config.StartPort = 50051
	}
	if config.WorkerIDPrefix == "" {
		config.WorkerIDPrefix = "pineworker"
	}
	if config.HealthTimeout <= 0 {
		config.HealthTimeout = 5 * time.Second
	}
	if config.WorkerConfig.RequestTimeout <= 0 {
		config.WorkerConfig = DefaultWorkerConfig(config.Workers)
	}
	if launcher == nil {
		return nil, fmt.Errorf("pine worker launcher is required")
	}
	if dialer == nil {
		return nil, fmt.Errorf("pine worker dialer is required")
	}
	return &WorkerManager{
		config: config, launcher: launcher, dialer: dialer,
		busy: make(chan struct{}, config.Workers), sessionWorkers: make(map[string]*managedWorker),
	}, nil
}

func (manager *WorkerManager) Start(ctx context.Context) error {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if len(manager.workers) > 0 {
		return nil
	}
	for index := 0; index < manager.config.Workers; index++ {
		worker, err := manager.startWorkerLocked(ctx, index, 0)
		if err != nil {
			_ = manager.stopLocked(ctx)
			return err
		}
		manager.workers = append(manager.workers, worker)
	}
	return nil
}

func (manager *WorkerManager) Stop(ctx context.Context) error {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.stopLocked(ctx)
}

func (manager *WorkerManager) RunScript(ctx context.Context, request RunScriptRequest) (RunScriptResponse, error) {
	ctx = observability.WithFields(ctx, observability.Fields{
		TaskID:       request.JobID,
		InstrumentID: request.Symbol,
		Source:       "pinets",
	})
	startedAt := time.Now()
	if err := manager.ensureStarted(); err != nil {
		observability.ErrorWithImportance(ctx, observability.ImportanceHigh, "pine worker request rejected", err, "mode", request.Mode)
		return RunScriptResponse{}, err
	}
	if err := manager.acquireRunSlot(ctx); err != nil {
		observability.ErrorWithImportance(ctx, observability.ImportanceHigh, "pine worker capacity unavailable", err, "mode", request.Mode)
		return RunScriptResponse{}, err
	}
	defer manager.releaseRunSlot()

	worker, err := manager.workerForRequest(request)
	if err != nil {
		observability.ErrorWithImportance(ctx, observability.ImportanceHigh, "pine worker selection failed", err, "mode", request.Mode)
		return RunScriptResponse{}, err
	}
	response, err := worker.client.RunScript(ctx, request)
	if err != nil {
		manager.recordSessionFailure(request, worker)
		observability.ErrorWithImportance(ctx, observability.ImportanceHigh, "pine worker request failed", err, "mode", request.Mode, "latency_ms", time.Since(startedAt).Milliseconds())
		return RunScriptResponse{}, err
	}
	if err := manager.recordSessionResult(request, worker); err != nil {
		return response, err
	}
	observability.InfoWithImportance(ctx, observability.ImportanceLow, "pine worker request completed", "mode", request.Mode, "latency_ms", time.Since(startedAt).Milliseconds())
	return response, nil
}

func (manager *WorkerManager) workerForRequest(request RunScriptRequest) (*managedWorker, error) {
	operation := normalizeSessionOperation(request.SessionOperation)
	if operation == SessionOperationAppend || operation == SessionOperationClose {
		manager.mu.Lock()
		worker := manager.sessionWorkers[strings.TrimSpace(request.SessionID)]
		manager.mu.Unlock()
		if worker == nil {
			return nil, fmt.Errorf("pine worker live session %q is not pinned to an active worker", request.SessionID)
		}
		return worker, nil
	}
	if operation == SessionOperationOpen {
		sessionID := strings.TrimSpace(request.SessionID)
		manager.mu.Lock()
		defer manager.mu.Unlock()
		if _, exists := manager.sessionWorkers[sessionID]; exists {
			return nil, fmt.Errorf("pine worker live session %q is already open", request.SessionID)
		}
		worker, err := manager.pickWorkerLocked()
		if err != nil {
			return nil, err
		}
		// Reserve before the RPC so concurrent opens cannot create duplicate
		// state under the same logical session id on different workers.
		manager.sessionWorkers[sessionID] = worker
		return worker, nil
	}
	return manager.pickWorker()
}

func (manager *WorkerManager) recordSessionFailure(request RunScriptRequest, worker *managedWorker) {
	if normalizeSessionOperation(request.SessionOperation) != SessionOperationOpen {
		return
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	sessionID := strings.TrimSpace(request.SessionID)
	if manager.sessionWorkers[sessionID] == worker {
		delete(manager.sessionWorkers, sessionID)
	}
}

func (manager *WorkerManager) recordSessionResult(request RunScriptRequest, worker *managedWorker) error {
	operation := normalizeSessionOperation(request.SessionOperation)
	if operation == "" {
		return nil
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	sessionID := strings.TrimSpace(request.SessionID)
	if operation == SessionOperationClose {
		delete(manager.sessionWorkers, sessionID)
		return nil
	}
	if operation == SessionOperationOpen {
		if manager.sessionWorkers[sessionID] != worker {
			return fmt.Errorf("pine worker live session %q lost its pinned worker while opening", request.SessionID)
		}
	}
	return nil
}

func (manager *WorkerManager) ensureStarted() error {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if len(manager.workers) == 0 {
		return fmt.Errorf("pine worker manager is not started")
	}
	return nil
}

func (manager *WorkerManager) acquireRunSlot(ctx context.Context) error {
	if manager.config.RejectWhenBusy {
		select {
		case manager.busy <- struct{}{}:
			return nil
		default:
			return CapacityExceededError{Workers: cap(manager.busy)}
		}
	}
	select {
	case manager.busy <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (manager *WorkerManager) releaseRunSlot() {
	select {
	case <-manager.busy:
	default:
	}
}

func (manager *WorkerManager) CheckHealth(ctx context.Context) error {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	for index, worker := range manager.workers {
		healthCtx, cancel := context.WithTimeout(ctx, manager.config.HealthTimeout)
		status, err := worker.transport.HealthCheck(healthCtx)
		cancel()
		if err != nil || !status.OK {
			restarted, restartErr := manager.restartWorkerLocked(ctx, index, worker)
			if restartErr != nil {
				worker.healthy = false
				worker.lastError = joinHealthError(err, restartErr, status)
				return restartErr
			}
			manager.workers[index] = restarted
			continue
		}
		worker.healthy = true
		worker.lastError = ""
		worker.version = status.Version
		worker.pineTSVersion = status.PineTSVersion
		worker.capabilities = append([]string(nil), status.Capabilities...)
	}
	return nil
}

func (manager *WorkerManager) Snapshot() []WorkerSnapshot {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	snapshot := make([]WorkerSnapshot, 0, len(manager.workers))
	for _, worker := range manager.workers {
		snapshot = append(snapshot, WorkerSnapshot{
			WorkerID:      worker.spec.WorkerID,
			Address:       worker.spec.Address,
			Port:          worker.spec.Port,
			Healthy:       worker.healthy,
			Restarts:      worker.restarts,
			LastError:     worker.lastError,
			Version:       worker.version,
			PineTSVersion: worker.pineTSVersion,
			Capabilities:  append([]string(nil), worker.capabilities...),
		})
	}
	return snapshot
}

func (manager *WorkerManager) startWorkerLocked(ctx context.Context, index int, restarts int) (*managedWorker, error) {
	spec := WorkerSpec{
		WorkerID: fmt.Sprintf("%s-%d", manager.config.WorkerIDPrefix, index+1),
		Port:     manager.config.StartPort + index,
	}
	spec.Address = fmt.Sprintf("%s:%d", manager.config.Host, spec.Port)
	process, err := manager.launcher.Start(ctx, spec)
	if err != nil {
		return nil, fmt.Errorf("start %s: %w", spec.WorkerID, err)
	}
	transport, status, err := manager.dialWorkerUntilReady(ctx, spec.Address)
	if err != nil {
		_ = process.Stop(ctx)
		return nil, fmt.Errorf("dial %s at %s: pine worker process did not become ready: %w%s", spec.WorkerID, spec.Address, err, processDiagnostics(process))
	}
	options := []ClientOption(nil)
	if manager.config.Gate != (PerformanceGate{}) {
		options = append(options, WithPerformanceGate(manager.config.Gate))
	}
	client, err := NewClient(transport, manager.config.WorkerConfig, options...)
	if err != nil {
		_ = transport.Close()
		_ = process.Stop(ctx)
		return nil, err
	}
	return &managedWorker{
		spec:          spec,
		process:       process,
		transport:     transport,
		client:        client,
		healthy:       true,
		restarts:      restarts,
		version:       status.Version,
		pineTSVersion: status.PineTSVersion,
		capabilities:  append([]string(nil), status.Capabilities...),
	}, nil
}

func processDiagnostics(process WorkerProcess) string {
	diagnostics, ok := process.(interface{ Diagnostics() string })
	if !ok {
		return ""
	}
	text := diagnostics.Diagnostics()
	if text == "" {
		return ""
	}
	return "; " + text
}

func (manager *WorkerManager) dialWorkerUntilReady(ctx context.Context, address string) (ManagedTransport, HealthStatus, error) {
	deadline := time.Now().Add(manager.config.HealthTimeout)
	var lastErr error
	for {
		dialCtx, cancelDial := context.WithTimeout(ctx, manager.config.HealthTimeout)
		transport, err := manager.dialer.Dial(dialCtx, address)
		cancelDial()
		if err == nil {
			healthCtx, cancelHealth := context.WithTimeout(ctx, manager.config.HealthTimeout)
			status, healthErr := transport.HealthCheck(healthCtx)
			cancelHealth()
			if healthErr == nil && status.OK {
				return transport, status, nil
			}
			_ = transport.Close()
			if healthErr != nil {
				lastErr = healthErr
			} else {
				lastErr = fmt.Errorf("worker unhealthy: ok=%v", status.OK)
			}
		} else {
			lastErr = err
		}
		if time.Now().After(deadline) {
			if lastErr == nil {
				lastErr = context.DeadlineExceeded
			}
			return nil, HealthStatus{}, lastErr
		}
		select {
		case <-ctx.Done():
			return nil, HealthStatus{}, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func (manager *WorkerManager) restartWorkerLocked(ctx context.Context, index int, worker *managedWorker) (*managedWorker, error) {
	restarts := worker.restarts + 1
	manager.dropWorkerSessionsLocked(worker)
	_ = worker.transport.Close()
	_ = worker.process.Stop(ctx)
	return manager.startWorkerLocked(ctx, index, restarts)
}

func (manager *WorkerManager) dropWorkerSessionsLocked(worker *managedWorker) {
	for sessionID, candidate := range manager.sessionWorkers {
		if candidate == worker {
			delete(manager.sessionWorkers, sessionID)
		}
	}
}

func (manager *WorkerManager) stopLocked(ctx context.Context) error {
	var firstErr error
	for _, worker := range manager.workers {
		if err := worker.transport.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		if err := worker.process.Stop(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	manager.workers = nil
	manager.next = 0
	clear(manager.sessionWorkers)
	return firstErr
}

func (manager *WorkerManager) pickWorker() (*managedWorker, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.pickWorkerLocked()
}

func (manager *WorkerManager) pickWorkerLocked() (*managedWorker, error) {
	if len(manager.workers) == 0 {
		return nil, fmt.Errorf("pine worker manager is not started")
	}
	for attempts := 0; attempts < len(manager.workers); attempts++ {
		index := (manager.next + attempts) % len(manager.workers)
		worker := manager.workers[index]
		if worker.healthy {
			manager.next = (index + 1) % len(manager.workers)
			return worker, nil
		}
	}
	return nil, fmt.Errorf("no healthy pine workers available")
}

func joinHealthError(healthErr error, restartErr error, status HealthStatus) string {
	if restartErr != nil {
		if healthErr != nil {
			return fmt.Sprintf("health check failed: %v; restart failed: %v", healthErr, restartErr)
		}
		return fmt.Sprintf("worker unhealthy: ok=%v; restart failed: %v", status.OK, restartErr)
	}
	if healthErr != nil {
		return healthErr.Error()
	}
	return "worker unhealthy"
}

type managedWorker struct {
	spec          WorkerSpec
	process       WorkerProcess
	transport     ManagedTransport
	client        *Client
	healthy       bool
	restarts      int
	lastError     string
	version       string
	pineTSVersion string
	capabilities  []string
}
