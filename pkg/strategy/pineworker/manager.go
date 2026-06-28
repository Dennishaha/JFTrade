package pineworker

import (
	"context"
	"fmt"
	"sync"
	"time"
)

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
	Workers       int
	Host          string
	StartPort     int
	HealthTimeout time.Duration
	WorkerConfig  WorkerConfig
	Gate          PerformanceGate
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
	mu       sync.Mutex
	config   ManagerConfig
	launcher WorkerLauncher
	dialer   TransportDialer
	workers  []*managedWorker
	next     int
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
	return &WorkerManager{config: config, launcher: launcher, dialer: dialer}, nil
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
	worker, err := manager.pickWorker()
	if err != nil {
		return RunScriptResponse{}, err
	}
	return worker.client.RunScript(ctx, request)
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
		WorkerID: fmt.Sprintf("pineworker-%d", index+1),
		Port:     manager.config.StartPort + index,
	}
	spec.Address = fmt.Sprintf("%s:%d", manager.config.Host, spec.Port)
	process, err := manager.launcher.Start(ctx, spec)
	if err != nil {
		return nil, fmt.Errorf("start %s: %w", spec.WorkerID, err)
	}
	transport, err := manager.dialer.Dial(ctx, spec.Address)
	if err != nil {
		_ = process.Stop(ctx)
		return nil, fmt.Errorf("dial %s: %w", spec.WorkerID, err)
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
		spec:      spec,
		process:   process,
		transport: transport,
		client:    client,
		healthy:   true,
		restarts:  restarts,
	}, nil
}

func (manager *WorkerManager) restartWorkerLocked(ctx context.Context, index int, worker *managedWorker) (*managedWorker, error) {
	restarts := worker.restarts + 1
	_ = worker.transport.Close()
	_ = worker.process.Stop(ctx)
	return manager.startWorkerLocked(ctx, index, restarts)
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
	return firstErr
}

func (manager *WorkerManager) pickWorker() (*managedWorker, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
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
