package pineruntime

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

// Runner is the Pine worker surface consumed by backtest and live strategy services.
type Runner interface {
	RunScript(context.Context, pineworker.RunScriptRequest) (pineworker.RunScriptResponse, error)
}

// LiveSession is a stateful live Pine execution session.
type LiveSession interface {
	Append(context.Context, pineworker.RunScriptRequest) (pineworker.RunScriptResponse, error)
	Close(context.Context) error
}

// LiveSessionOpener is implemented by runners that support stateful live execution.
type LiveSessionOpener interface {
	OpenLiveSession(context.Context, pineworker.RunScriptRequest) (LiveSession, pineworker.RunScriptResponse, error)
}

type runnerKind string

const (
	runnerBacktest runnerKind = "backtest"
	runnerInstance runnerKind = "instance"
)

type ephemeralRunner struct {
	config         Config
	kind           runnerKind
	launcher       pineworker.WorkerLauncher
	dialer         pineworker.TransportDialer
	busy           chan struct{}
	rejectWhenBusy bool
	nextID         atomic.Uint64
	mu             sync.Mutex
	sessions       map[*liveSession]struct{}
	sessionWatchWG sync.WaitGroup
	closed         bool
}

func newEphemeralRunner(config Config, kind runnerKind, deps dependencies) (*ephemeralRunner, error) {
	bundleData := config.bundleData
	if len(bundleData) == 0 {
		var err error
		bundleData, err = os.ReadFile(config.BundlePath)
		if err != nil {
			return nil, fmt.Errorf("read worker bundle: %w", err)
		}
	}
	launcher, err := deps.newLauncher(config, bundleData)
	if err != nil {
		return nil, fmt.Errorf("create launcher: %w", err)
	}
	workers := config.BacktestWorkers
	if kind == runnerInstance {
		workers = config.InstanceWorkers
	}
	if workers <= 0 {
		workers = 1
	}
	return &ephemeralRunner{
		config: config, kind: kind, launcher: launcher, dialer: deps.newDialer(config.MaxMessageBytes),
		busy: make(chan struct{}, workers), rejectWhenBusy: kind == runnerInstance,
		sessions: make(map[*liveSession]struct{}),
	}, nil
}

func (runner *ephemeralRunner) RunScript(ctx context.Context, request pineworker.RunScriptRequest) (pineworker.RunScriptResponse, error) {
	if runner == nil {
		return pineworker.RunScriptResponse{}, fmt.Errorf("pine worker runner is nil")
	}
	if runner.isClosed() {
		return pineworker.RunScriptResponse{}, fmt.Errorf("pine worker runner is closed")
	}
	if err := runner.acquire(ctx); err != nil {
		return pineworker.RunScriptResponse{}, err
	}
	defer runner.release()
	manager, err := runner.newManager(ctx)
	if err != nil {
		return pineworker.RunScriptResponse{}, err
	}
	if err := manager.Start(ctx); err != nil {
		return pineworker.RunScriptResponse{}, err
	}
	defer runner.stopManager(manager)
	return manager.RunScript(ctx, request)
}

func (runner *ephemeralRunner) OpenLiveSession(
	ctx context.Context,
	request pineworker.RunScriptRequest,
) (LiveSession, pineworker.RunScriptResponse, error) {
	if runner == nil {
		return nil, pineworker.RunScriptResponse{}, fmt.Errorf("pine worker runner is nil")
	}
	if runner.isClosed() {
		return nil, pineworker.RunScriptResponse{}, fmt.Errorf("pine worker runner is closed")
	}
	if err := runner.acquire(ctx); err != nil {
		return nil, pineworker.RunScriptResponse{}, err
	}
	manager, err := runner.startManager(ctx)
	if err != nil {
		runner.release()
		return nil, pineworker.RunScriptResponse{}, err
	}
	request.Mode = pineworker.ModeLive
	request.SessionOperation = pineworker.SessionOperationOpen
	request.ExpectedRevision = 0
	if strings.TrimSpace(request.SessionID) == "" {
		request.SessionID = fmt.Sprintf("live-session-%d", runner.nextID.Add(1))
	}
	response, err := manager.RunScript(ctx, request)
	if err != nil {
		runner.stopManager(manager)
		runner.release()
		return nil, response, err
	}
	session := &liveSession{
		runner: runner, manager: manager, sessionID: request.SessionID,
		revision: response.SessionRevision, done: make(chan struct{}),
	}
	watchContext := ctx.Done() != nil
	if err := runner.registerSessionWithWatcher(session, watchContext); err != nil {
		_ = session.Close(context.Background())
		return nil, pineworker.RunScriptResponse{}, err
	}
	if watchContext {
		go func() {
			defer runner.sessionWatchWG.Done()
			session.closeWhenDone(ctx)
		}()
	}
	return session, response, nil
}

func (runner *ephemeralRunner) Close(ctx context.Context) error {
	if runner == nil {
		return nil
	}
	runner.mu.Lock()
	runner.closed = true
	sessions := make([]*liveSession, 0, len(runner.sessions))
	for session := range runner.sessions {
		sessions = append(sessions, session)
	}
	runner.mu.Unlock()
	var closeErr error
	for _, session := range sessions {
		closeErr = errors.Join(closeErr, session.Close(ctx))
	}
	runner.sessionWatchWG.Wait()
	return closeErr
}

func (runner *ephemeralRunner) startManager(ctx context.Context) (*pineworker.WorkerManager, error) {
	manager, err := runner.newManager(ctx)
	if err != nil {
		return nil, err
	}
	if err := manager.Start(ctx); err != nil {
		return nil, err
	}
	return manager, nil
}

func (runner *ephemeralRunner) stopManager(manager *pineworker.WorkerManager) {
	stopCtx, cancel := context.WithTimeout(context.Background(), runner.stopTimeout())
	defer cancel()
	if err := manager.Stop(stopCtx); err != nil {
		log.Printf("JFTrade PineTS ephemeral worker stop failed: %v", err)
	}
}

func (runner *ephemeralRunner) isClosed() bool {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.closed
}

func (runner *ephemeralRunner) registerSession(session *liveSession) error {
	return runner.registerSessionWithWatcher(session, false)
}

func (runner *ephemeralRunner) registerSessionWithWatcher(session *liveSession, watchContext bool) error {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if runner.closed {
		return fmt.Errorf("pine worker runner is closed")
	}
	runner.sessions[session] = struct{}{}
	if watchContext {
		runner.sessionWatchWG.Add(1)
	}
	return nil
}

func (runner *ephemeralRunner) acquire(ctx context.Context) error {
	if runner.rejectWhenBusy {
		select {
		case runner.busy <- struct{}{}:
			return nil
		default:
			return pineworker.CapacityExceededError{Workers: cap(runner.busy)}
		}
	}
	select {
	case runner.busy <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (runner *ephemeralRunner) release() {
	select {
	case <-runner.busy:
	default:
	}
}

func (runner *ephemeralRunner) newManager(ctx context.Context) (*pineworker.WorkerManager, error) {
	port, err := freePort(ctx, runner.config.Host)
	if err != nil {
		return nil, err
	}
	workerConfig := pineworker.DefaultWorkerConfig(1)
	workerConfig.LiveWorkers = 1
	workerConfig.BacktestWorkers = 1
	workerConfig.RequestTimeout = runner.config.RequestTimeout
	workerConfig.MaxMessageBytes = runner.config.MaxMessageBytes
	workerConfig.MaxCandlesPerRequest = runner.config.MaxCandles
	return pineworker.NewWorkerManager(pineworker.ManagerConfig{
		Workers: 1, WorkerIDPrefix: fmt.Sprintf("%s-%d", runner.idPrefix(), runner.nextID.Add(1)),
		Host: runner.config.Host, StartPort: port, HealthTimeout: runner.config.HealthTimeout,
		WorkerConfig: workerConfig, RejectWhenBusy: runner.rejectWhenBusy,
		Gate: pineworker.PerformanceGate{
			MaxDuration: runner.config.MaxDuration, MaxDurationPerBar: runner.config.MaxDurationPerBar,
			MinCandlesPerSec: runner.config.MinCandlesPerSec, MaxRequestBytes: runner.config.MaxMessageBytes,
			MaxResponseBytes: runner.config.MaxMessageBytes, MaxPeakRSSBytes: runner.config.MaxPeakRSSBytes,
		},
	}, runner.launcher, runner.dialer)
}

func (runner *ephemeralRunner) idPrefix() string {
	if runner.kind == runnerInstance {
		return "pineworker-instance"
	}
	return "pineworker-backtest"
}

func (runner *ephemeralRunner) stopTimeout() time.Duration {
	if runner.config.RequestTimeout > 0 {
		return min(runner.config.RequestTimeout, 10*time.Second)
	}
	return 5 * time.Second
}

func freePort(ctx context.Context, host string) (int, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		host = "127.0.0.1"
	}
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", net.JoinHostPort(host, "0"))
	if err != nil {
		return 0, fmt.Errorf("allocate pine worker port: %w", err)
	}
	defer func() { _ = listener.Close() }()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

type liveSession struct {
	runner    *ephemeralRunner
	manager   *pineworker.WorkerManager
	sessionID string
	mu        sync.Mutex
	revision  uint64
	closed    bool
	done      chan struct{}
}

func (session *liveSession) Append(ctx context.Context, request pineworker.RunScriptRequest) (pineworker.RunScriptResponse, error) {
	if session == nil || session.manager == nil {
		return pineworker.RunScriptResponse{}, fmt.Errorf("pine worker live session is unavailable")
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.closed {
		return pineworker.RunScriptResponse{}, fmt.Errorf("pine worker live session %q is closed", session.sessionID)
	}
	request.Mode = pineworker.ModeLive
	request.SessionID = session.sessionID
	request.SessionOperation = pineworker.SessionOperationAppend
	request.ExpectedRevision = session.revision
	response, err := session.manager.RunScript(ctx, request)
	if err == nil {
		session.revision = response.SessionRevision
	}
	return response, err
}

func (session *liveSession) Close(ctx context.Context) error {
	if session == nil {
		return nil
	}
	session.mu.Lock()
	if session.closed {
		session.mu.Unlock()
		return nil
	}
	session.closed = true
	if session.done == nil {
		session.done = make(chan struct{})
	}
	close(session.done)
	revision := session.revision
	session.mu.Unlock()
	var closeErr error
	if session.manager != nil {
		_, closeErr = session.manager.RunScript(ctx, pineworker.RunScriptRequest{
			JobID: fmt.Sprintf("close:%s:%d", session.sessionID, time.Now().UnixNano()), Mode: pineworker.ModeLive,
			SessionID: session.sessionID, SessionOperation: pineworker.SessionOperationClose, ExpectedRevision: revision,
		})
		stopCtx, cancel := context.WithTimeout(context.Background(), session.runner.stopTimeout())
		closeErr = errors.Join(closeErr, session.manager.Stop(stopCtx))
		cancel()
	}
	if session.runner != nil {
		session.runner.mu.Lock()
		delete(session.runner.sessions, session)
		session.runner.mu.Unlock()
		session.runner.release()
	}
	return closeErr
}

func (session *liveSession) closeWhenDone(ctx context.Context) {
	session.mu.Lock()
	if session.done == nil {
		session.done = make(chan struct{})
	}
	done := session.done
	runner := session.runner
	session.mu.Unlock()
	select {
	case <-ctx.Done():
	case <-done:
		return
	}
	if runner == nil {
		return
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), runner.stopTimeout())
	defer cancel()
	_ = session.Close(stopCtx)
}
