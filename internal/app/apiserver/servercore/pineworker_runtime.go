package servercore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jftrade/jftrade-main/internal/pineworkerassets"
	"github.com/jftrade/jftrade-main/internal/store/settingsfile"
	"github.com/jftrade/jftrade-main/pkg/jftsettings"
	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

var (
	newPineWorkerLauncher = defaultNewPineWorkerLauncher
	newPineWorkerDialer   = defaultNewPineWorkerDialer
	selectPineWorkerAsset = pineworkerassets.Select
	pineWorkerGetwd       = os.Getwd
	pineWorkerAbs         = filepath.Abs
)

const (
	envPineWorkerDisabled          = "JFTRADE_PINEWORKER_DISABLED"
	envPineWorkerBundle            = "JFTRADE_PINEWORKER_BUNDLE"
	envPineWorkerRuntime           = "JFTRADE_PINEWORKER_RUNTIME"
	envPineWorkerSHA256            = "JFTRADE_PINEWORKER_SHA256"
	envPineWorkerBacktestWorkers   = "JFTRADE_PINEWORKER_BACKTEST_WORKERS"
	envPineWorkerInstanceWorkers   = "JFTRADE_PINEWORKER_INSTANCE_WORKERS"
	envPineWorkerHost              = "JFTRADE_PINEWORKER_HOST"
	envPineWorkerStartPort         = "JFTRADE_PINEWORKER_START_PORT"
	envPineWorkerTempDir           = "JFTRADE_PINEWORKER_TEMP_DIR"
	envPineWorkerProto             = "JFTRADE_PINEWORKER_PROTO"
	envPineWorkerPineTSVersion     = "JFTRADE_PINEWORKER_PINETS_VERSION"
	envPineWorkerMock              = "JFTRADE_PINEWORKER_MOCK"
	envPineWorkerRequestTimeout    = "JFTRADE_PINEWORKER_REQUEST_TIMEOUT"
	envPineWorkerHealthTimeout     = "JFTRADE_PINEWORKER_HEALTH_TIMEOUT"
	envPineWorkerMaxMessageBytes   = "JFTRADE_PINEWORKER_MAX_MESSAGE_BYTES"
	envPineWorkerMaxCandles        = "JFTRADE_PINEWORKER_MAX_CANDLES"
	envPineWorkerMaxDuration       = "JFTRADE_PINEWORKER_MAX_DURATION"
	envPineWorkerMaxDurationPerBar = "JFTRADE_PINEWORKER_MAX_DURATION_PER_BAR"
	envPineWorkerMinCandlesPerSec  = "JFTRADE_PINEWORKER_MIN_CANDLES_PER_SEC"
	envPineWorkerMaxPeakRSSBytes   = "JFTRADE_PINEWORKER_MAX_PEAK_RSS_BYTES"

	defaultPineWorkerProtoPath = "pkg/strategy/pineworker/proto/pineworker.proto"
	pineWorkerLogTailBytes     = 8192
)

type pineWorkerRuntimeConfig struct {
	BundlePath        string
	RuntimePath       string
	SHA256            string
	BacktestWorkers   int
	InstanceWorkers   int
	Host              string
	StartPort         int
	TempDir           string
	WorkDir           string
	ProtoPath         string
	PineTSVersion     string
	Mock              bool
	RequestTimeout    time.Duration
	HealthTimeout     time.Duration
	MaxMessageBytes   int
	MaxCandles        int
	MaxDuration       time.Duration
	MaxDurationPerBar time.Duration
	MinCandlesPerSec  float64
	MaxPeakRSSBytes   int64
	embedded          bool
	bundleData        []byte
}

type pineWorkerRunner interface {
	RunScript(context.Context, pineworker.RunScriptRequest) (pineworker.RunScriptResponse, error)
}

type pineWorkerLiveSession interface {
	Append(context.Context, pineworker.RunScriptRequest) (pineworker.RunScriptResponse, error)
	Close(context.Context) error
}

type pineWorkerLiveSessionOpener interface {
	OpenLiveSession(context.Context, pineworker.RunScriptRequest) (pineWorkerLiveSession, pineworker.RunScriptResponse, error)
}

type pineWorkerRunnerKind string

const (
	pineWorkerRunnerBacktest pineWorkerRunnerKind = "backtest"
	pineWorkerRunnerInstance pineWorkerRunnerKind = "instance"
)

func (s *Server) startPineWorkerManagers() (pineWorkerRunner, pineWorkerRunner) {
	config, enabled, err := resolvePineWorkerRuntimeConfig(s.pineWorkerSettings)
	if err != nil {
		log.Printf("JFTrade PineTS worker manager disabled by invalid config: %v", err)
		return nil, nil
	}
	if !enabled {
		log.Printf("JFTrade PineTS worker manager not started: %s is not configured and no embedded worker asset is available; run `pnpm run dev:api:pineworker` or set %s=/absolute/path/to/worker.mjs", envPineWorkerBundle, envPineWorkerBundle)
		return nil, nil
	}

	backtestRunner, err := newEphemeralPineWorkerRunner(config, pineWorkerRunnerBacktest)
	if err != nil {
		log.Printf("JFTrade PineTS worker manager disabled: create backtest runner: %v", err)
		return nil, nil
	}
	instanceRunner, err := newEphemeralPineWorkerRunner(config, pineWorkerRunnerInstance)
	if err != nil {
		log.Printf("JFTrade PineTS worker manager disabled: create instance runner: %v", err)
		return nil, nil
	}
	source := "external"
	if config.embedded {
		source = "embedded"
	}
	log.Printf("JFTrade PineTS worker managers configured: source=%s runtime=%s backtestLimit=%d instanceLimit=%d host=%s mode=ephemeral proto=%s cwd=%s", source, config.RuntimePath, config.BacktestWorkers, config.InstanceWorkers, config.Host, config.ProtoPath, config.WorkDir)
	return backtestRunner, instanceRunner
}

func (s *Server) pineWorkerSettings() jftsettings.PineWorkerSettings {
	if s == nil || s.store == nil {
		return settingsfile.DefaultPineWorkerSettings()
	}
	return persistenceOnlySettingsStore(s.store).PineWorkerSettings()
}

func (s *Server) applyPineWorkerSettings(_ jftsettings.PineWorkerSettings) {
	if s == nil {
		return
	}
	backtestRunner, instanceRunner := s.startPineWorkerManagers()

	s.pineWorkerMu.Lock()
	previousBacktestRunner := s.backtestPineWorkerRunner
	previousInstanceRunner := s.instancePineWorkerRunner
	s.backtestPineWorkerRunner = backtestRunner
	s.instancePineWorkerRunner = instanceRunner
	if s.strategyRuntimeManager != nil {
		s.strategyRuntimeManager.setPineWorkerRunner(instanceRunner)
	}
	if s.backtestSvc != nil {
		s.backtestSvc.SetPineWorkerRunner(backtestRunner)
	}
	s.pineWorkerMu.Unlock()

	retirePineWorkerRunner(previousBacktestRunner)
	retirePineWorkerRunner(previousInstanceRunner)
}

func retirePineWorkerRunner(runner pineWorkerRunner) {
	if runner == nil {
		return
	}
	if closer, ok := runner.(interface{ Close(context.Context) error }); ok {
		_ = closer.Close(context.Background())
	}
}

type ephemeralPineWorkerRunner struct {
	config         pineWorkerRuntimeConfig
	kind           pineWorkerRunnerKind
	launcher       pineworker.WorkerLauncher
	dialer         pineworker.TransportDialer
	busy           chan struct{}
	rejectWhenBusy bool
	nextID         atomic.Uint64
	mu             sync.Mutex
	sessions       map[*ephemeralPineWorkerSession]struct{}
	closed         bool
}

func newEphemeralPineWorkerRunner(config pineWorkerRuntimeConfig, kind pineWorkerRunnerKind) (*ephemeralPineWorkerRunner, error) {
	bundleData := config.bundleData
	if len(bundleData) == 0 {
		var err error
		bundleData, err = os.ReadFile(config.BundlePath)
		if err != nil {
			return nil, fmt.Errorf("read worker bundle: %w", err)
		}
	}
	launcher, err := newPineWorkerLauncher(config, bundleData)
	if err != nil {
		return nil, fmt.Errorf("create launcher: %w", err)
	}
	workers := pineWorkerConcurrencyLimit(config, kind)
	if workers <= 0 {
		workers = 1
	}
	return &ephemeralPineWorkerRunner{
		config:         config,
		kind:           kind,
		launcher:       launcher,
		dialer:         newPineWorkerDialer(config.MaxMessageBytes),
		busy:           make(chan struct{}, workers),
		rejectWhenBusy: kind == pineWorkerRunnerInstance,
		sessions:       make(map[*ephemeralPineWorkerSession]struct{}),
	}, nil
}

func (runner *ephemeralPineWorkerRunner) RunScript(ctx context.Context, request pineworker.RunScriptRequest) (pineworker.RunScriptResponse, error) {
	if runner == nil {
		return pineworker.RunScriptResponse{}, fmt.Errorf("pine worker runner is nil")
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
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), runner.stopTimeout())
		defer cancel()
		if err := manager.Stop(stopCtx); err != nil {
			log.Printf("JFTrade PineTS ephemeral worker stop failed: %v", err)
		}
	}()
	return manager.RunScript(ctx, request)
}

func (runner *ephemeralPineWorkerRunner) OpenLiveSession(
	ctx context.Context,
	request pineworker.RunScriptRequest,
) (pineWorkerLiveSession, pineworker.RunScriptResponse, error) {
	if runner == nil {
		return nil, pineworker.RunScriptResponse{}, fmt.Errorf("pine worker runner is nil")
	}
	runner.mu.Lock()
	closed := runner.closed
	runner.mu.Unlock()
	if closed {
		return nil, pineworker.RunScriptResponse{}, fmt.Errorf("pine worker runner is closed")
	}
	if err := runner.acquire(ctx); err != nil {
		return nil, pineworker.RunScriptResponse{}, err
	}
	manager, err := runner.newManager(ctx)
	if err != nil {
		runner.release()
		return nil, pineworker.RunScriptResponse{}, err
	}
	if err := manager.Start(ctx); err != nil {
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
		stopCtx, cancel := context.WithTimeout(context.Background(), runner.stopTimeout())
		defer cancel()
		_ = manager.Stop(stopCtx)
		runner.release()
		return nil, response, err
	}
	session := &ephemeralPineWorkerSession{
		runner: runner, manager: manager, sessionID: request.SessionID,
		revision: response.SessionRevision,
	}
	runner.mu.Lock()
	if runner.closed {
		runner.mu.Unlock()
		_ = session.Close(context.Background())
		return nil, pineworker.RunScriptResponse{}, fmt.Errorf("pine worker runner is closed")
	}
	runner.sessions[session] = struct{}{}
	runner.mu.Unlock()
	go func() {
		<-ctx.Done()
		stopCtx, cancel := context.WithTimeout(context.Background(), runner.stopTimeout())
		defer cancel()
		_ = session.Close(stopCtx)
	}()
	return session, response, nil
}

func (runner *ephemeralPineWorkerRunner) Close(ctx context.Context) error {
	if runner == nil {
		return nil
	}
	runner.mu.Lock()
	runner.closed = true
	sessions := make([]*ephemeralPineWorkerSession, 0, len(runner.sessions))
	for session := range runner.sessions {
		sessions = append(sessions, session)
	}
	runner.mu.Unlock()
	var closeErr error
	for _, session := range sessions {
		closeErr = errors.Join(closeErr, session.Close(ctx))
	}
	return closeErr
}

type ephemeralPineWorkerSession struct {
	runner    *ephemeralPineWorkerRunner
	manager   *pineworker.WorkerManager
	sessionID string

	mu       sync.Mutex
	revision uint64
	closed   bool
}

func (session *ephemeralPineWorkerSession) Append(
	ctx context.Context,
	request pineworker.RunScriptRequest,
) (pineworker.RunScriptResponse, error) {
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
	if err != nil {
		return response, err
	}
	session.revision = response.SessionRevision
	return response, nil
}

func (session *ephemeralPineWorkerSession) Close(ctx context.Context) error {
	if session == nil {
		return nil
	}
	session.mu.Lock()
	if session.closed {
		session.mu.Unlock()
		return nil
	}
	session.closed = true
	revision := session.revision
	session.mu.Unlock()
	var closeErr error
	if session.manager != nil {
		_, closeErr = session.manager.RunScript(ctx, pineworker.RunScriptRequest{
			JobID: fmt.Sprintf("close:%s:%d", session.sessionID, time.Now().UnixNano()),
			Mode:  pineworker.ModeLive, SessionID: session.sessionID,
			SessionOperation: pineworker.SessionOperationClose, ExpectedRevision: revision,
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

func (runner *ephemeralPineWorkerRunner) acquire(ctx context.Context) error {
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

func (runner *ephemeralPineWorkerRunner) release() {
	select {
	case <-runner.busy:
	default:
	}
}

func (runner *ephemeralPineWorkerRunner) newManager(ctx context.Context) (*pineworker.WorkerManager, error) {
	port, err := freePineWorkerPort(ctx, runner.config.Host)
	if err != nil {
		return nil, err
	}
	config := runner.config
	config.StartPort = port
	workerConfig := pineworker.DefaultWorkerConfig(1)
	workerConfig.LiveWorkers = 1
	workerConfig.BacktestWorkers = 1
	workerConfig.RequestTimeout = config.RequestTimeout
	workerConfig.MaxMessageBytes = config.MaxMessageBytes
	workerConfig.MaxCandlesPerRequest = config.MaxCandles
	return pineworker.NewWorkerManager(pineworker.ManagerConfig{
		Workers:        1,
		WorkerIDPrefix: fmt.Sprintf("%s-%d", pineWorkerRunnerIDPrefix(runner.kind), runner.nextID.Add(1)),
		Host:           config.Host,
		StartPort:      config.StartPort,
		HealthTimeout:  config.HealthTimeout,
		WorkerConfig:   workerConfig,
		RejectWhenBusy: runner.rejectWhenBusy,
		Gate: pineworker.PerformanceGate{
			MaxDuration:       config.MaxDuration,
			MaxDurationPerBar: config.MaxDurationPerBar,
			MinCandlesPerSec:  config.MinCandlesPerSec,
			MaxRequestBytes:   config.MaxMessageBytes,
			MaxResponseBytes:  config.MaxMessageBytes,
			MaxPeakRSSBytes:   config.MaxPeakRSSBytes,
		},
	}, runner.launcher, runner.dialer)
}

func (runner *ephemeralPineWorkerRunner) stopTimeout() time.Duration {
	if runner.config.RequestTimeout > 0 {
		return min(runner.config.RequestTimeout, 10*time.Second)
	}
	return 5 * time.Second
}

func freePineWorkerPort(ctx context.Context, host string) (int, error) {
	normalizedHost := strings.TrimSpace(host)
	if normalizedHost == "" {
		normalizedHost = "127.0.0.1"
	}
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", net.JoinHostPort(normalizedHost, "0"))
	if err != nil {
		return 0, fmt.Errorf("allocate pine worker port: %w", err)
	}
	defer func() {
		_ = listener.Close()
	}()
	tcpAddr := listener.Addr().(*net.TCPAddr)
	return tcpAddr.Port, nil
}

func pineWorkerConcurrencyLimit(config pineWorkerRuntimeConfig, kind pineWorkerRunnerKind) int {
	switch kind {
	case pineWorkerRunnerInstance:
		return config.InstanceWorkers
	default:
		return config.BacktestWorkers
	}
}

func pineWorkerRunnerIDPrefix(kind pineWorkerRunnerKind) string {
	switch kind {
	case pineWorkerRunnerInstance:
		return "pineworker-instance"
	default:
		return "pineworker-backtest"
	}
}

func defaultNewPineWorkerLauncher(config pineWorkerRuntimeConfig, bundleData []byte) (pineworker.WorkerLauncher, error) {
	if config.SHA256 == "" {
		sum := sha256.Sum256(bundleData)
		config.SHA256 = hex.EncodeToString(sum[:])
	}
	return pineworker.NewNodeWorkerLauncher(pineworker.NodeWorkerLauncherConfig{
		Bundle:          pineworker.WorkerBundle{Name: filepath.Base(config.BundlePath), Data: bundleData, SHA256: config.SHA256},
		RuntimePath:     config.RuntimePath,
		TempDir:         config.TempDir,
		WorkDir:         config.WorkDir,
		ProtoPath:       config.ProtoPath,
		MaxMessageBytes: config.MaxMessageBytes,
		PineTSVersion:   config.PineTSVersion,
		Mock:            config.Mock,
		Stdout:          pineworker.NewTailBuffer(pineWorkerLogTailBytes),
		Stderr:          pineworker.NewTailBuffer(pineWorkerLogTailBytes),
	})
}

func defaultNewPineWorkerDialer(maxMessageBytes int) pineworker.TransportDialer {
	return pineworker.NewGRPCDialer(pineworker.GRPCDialerConfig{MaxMessageBytes: maxMessageBytes})
}

type pineWorkerBundleConfig struct {
	bundlePath string
	asset      pineworkerassets.Asset
	embedded   bool
}

func resolvePineWorkerRuntimeConfig(settingsProvider func() jftsettings.PineWorkerSettings) (pineWorkerRuntimeConfig, bool, error) {
	if envBool(envPineWorkerDisabled, false) {
		return pineWorkerRuntimeConfig{}, false, nil
	}
	bundleConfig, enabled, err := resolvePineWorkerBundleConfig()
	if err != nil || !enabled {
		return pineWorkerRuntimeConfig{}, enabled, err
	}
	defaultSettings := defaultPineWorkerSettings(settingsProvider)
	defaultWorkerConfig := pineworker.DefaultWorkerConfig(runtime.NumCPU())
	backtestWorkers, instanceWorkers, err := resolvePineWorkerWorkerCounts(defaultSettings)
	if err != nil {
		return pineWorkerRuntimeConfig{}, false, err
	}
	runtimeConfig, err := resolvePineWorkerRuntimeLimits(defaultWorkerConfig)
	if err != nil {
		return pineWorkerRuntimeConfig{}, false, err
	}
	workDir := resolvePineWorkerWorkDir(bundleConfig.bundlePath)
	protoPath := resolvePineWorkerProtoPath(workDir)
	runtimePath := resolvePineWorkerRuntime(defaultSettings)
	runtimeConfig.BundlePath = bundleConfig.bundlePath
	runtimeConfig.RuntimePath = runtimePath
	runtimeConfig.SHA256 = firstNonEmpty(strings.TrimSpace(os.Getenv(envPineWorkerSHA256)), bundleConfig.asset.SHA256)
	runtimeConfig.BacktestWorkers = backtestWorkers
	runtimeConfig.InstanceWorkers = instanceWorkers
	runtimeConfig.WorkDir = workDir
	runtimeConfig.ProtoPath = protoPath
	runtimeConfig.embedded = bundleConfig.embedded
	runtimeConfig.bundleData = bundleConfig.asset.Data
	return runtimeConfig, true, nil
}

func resolvePineWorkerBundleConfig() (pineWorkerBundleConfig, bool, error) {
	bundlePath := strings.TrimSpace(os.Getenv(envPineWorkerBundle))
	if bundlePath != "" {
		return pineWorkerBundleConfig{bundlePath: bundlePath}, true, nil
	}
	asset, embedded, err := selectPineWorkerAsset()
	if err != nil {
		return pineWorkerBundleConfig{}, false, err
	}
	if !embedded {
		return pineWorkerBundleConfig{}, false, nil
	}
	return pineWorkerBundleConfig{
		bundlePath: asset.Name,
		asset:      asset,
		embedded:   true,
	}, true, nil
}

func defaultPineWorkerSettings(settingsProvider func() jftsettings.PineWorkerSettings) jftsettings.PineWorkerSettings {
	settings := settingsfile.DefaultPineWorkerSettings()
	if settingsProvider != nil {
		settings = settingsfile.NormalizePineWorkerSettings(settingsProvider())
	}
	return settings
}

func resolvePineWorkerWorkerCounts(defaultSettings jftsettings.PineWorkerSettings) (int, int, error) {
	backtestWorkers, err := envIntInRange(envPineWorkerBacktestWorkers, defaultSettings.BacktestWorkerLimit, 1, 1000)
	if err != nil {
		return 0, 0, err
	}
	instanceWorkers, err := envIntInRange(envPineWorkerInstanceWorkers, defaultSettings.InstanceWorkerLimit, 1, 1000)
	if err != nil {
		return 0, 0, err
	}
	return backtestWorkers, instanceWorkers, nil
}

func resolvePineWorkerRuntimeLimits(defaultWorkerConfig pineworker.WorkerConfig) (pineWorkerRuntimeConfig, error) {
	startPort, err := envPositiveInt(envPineWorkerStartPort, 50051)
	if err != nil {
		return pineWorkerRuntimeConfig{}, err
	}
	requestTimeout, err := envDuration(envPineWorkerRequestTimeout, defaultWorkerConfig.RequestTimeout)
	if err != nil {
		return pineWorkerRuntimeConfig{}, err
	}
	healthTimeout, err := envDuration(envPineWorkerHealthTimeout, 5*time.Second)
	if err != nil {
		return pineWorkerRuntimeConfig{}, err
	}
	maxMessageBytes, err := envPositiveInt(envPineWorkerMaxMessageBytes, defaultWorkerConfig.MaxMessageBytes)
	if err != nil {
		return pineWorkerRuntimeConfig{}, err
	}
	maxCandles, err := envPositiveInt(envPineWorkerMaxCandles, defaultWorkerConfig.MaxCandlesPerRequest)
	if err != nil {
		return pineWorkerRuntimeConfig{}, err
	}
	maxDuration, maxDurationPerBar, minCandlesPerSec, maxPeakRSSBytes, err := resolvePineWorkerPerformanceGate()
	if err != nil {
		return pineWorkerRuntimeConfig{}, err
	}
	host := strings.TrimSpace(os.Getenv(envPineWorkerHost))
	if host == "" {
		host = "127.0.0.1"
	}
	return pineWorkerRuntimeConfig{
		Host:              host,
		StartPort:         startPort,
		TempDir:           strings.TrimSpace(os.Getenv(envPineWorkerTempDir)),
		PineTSVersion:     strings.TrimSpace(os.Getenv(envPineWorkerPineTSVersion)),
		Mock:              envBool(envPineWorkerMock, false),
		RequestTimeout:    requestTimeout,
		HealthTimeout:     healthTimeout,
		MaxMessageBytes:   maxMessageBytes,
		MaxCandles:        maxCandles,
		MaxDuration:       maxDuration,
		MaxDurationPerBar: maxDurationPerBar,
		MinCandlesPerSec:  minCandlesPerSec,
		MaxPeakRSSBytes:   maxPeakRSSBytes,
	}, nil
}

func resolvePineWorkerPerformanceGate() (time.Duration, time.Duration, float64, int64, error) {
	gate := pineworker.DefaultPerformanceGate()
	maxDuration, err := envDuration(envPineWorkerMaxDuration, gate.MaxDuration)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	maxDurationPerBar, err := envDuration(envPineWorkerMaxDurationPerBar, gate.MaxDurationPerBar)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	minCandlesPerSec, err := envPositiveFloat(envPineWorkerMinCandlesPerSec, gate.MinCandlesPerSec)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	maxPeakRSSBytes, err := envPositiveInt64(envPineWorkerMaxPeakRSSBytes, gate.MaxPeakRSSBytes)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	return maxDuration, maxDurationPerBar, minCandlesPerSec, maxPeakRSSBytes, nil
}

func resolvePineWorkerRuntime(settings jftsettings.PineWorkerSettings) string {
	resolution := resolveNodeDependencyRuntime(settings)
	return resolution.effectivePath
}

func resolvePineWorkerWorkDir(bundlePath string) string {
	wd, err := pineWorkerGetwd()
	if err == nil {
		if root := findPineWorkerRepoRoot(wd); root != "" {
			return root
		}
	}
	if bundlePath != "" {
		if root := findPineWorkerRepoRoot(filepath.Dir(resolvePineWorkerRuntimePath(bundlePath, wd))); root != "" {
			return root
		}
	}
	if err != nil {
		return ""
	}
	return wd
}

func findPineWorkerRepoRoot(start string) string {
	dir := filepath.Clean(start)
	for {
		if fileExists(filepath.Join(dir, "go.mod")) && fileExists(filepath.Join(dir, filepath.FromSlash(defaultPineWorkerProtoPath))) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func resolvePineWorkerProtoPath(workDir string) string {
	value := strings.TrimSpace(os.Getenv(envPineWorkerProto))
	if value == "" {
		value = filepath.FromSlash(defaultPineWorkerProtoPath)
	}
	return resolvePineWorkerRuntimePath(value, workDir)
}

func resolvePineWorkerRuntimePath(value string, base string) string {
	if value == "" || filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	if base != "" {
		return filepath.Join(base, value)
	}
	absolute, err := pineWorkerAbs(value)
	if err != nil {
		return filepath.Clean(value)
	}
	return absolute
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func envIntInRange(key string, defaultValue int, minValue int, maxValue int) (int, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return clampInt(defaultValue, minValue, maxValue), nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < minValue || parsed > maxValue {
		return 0, fmt.Errorf("%s must be an integer between %d and %d", key, minValue, maxValue)
	}
	return parsed, nil
}

func clampInt(value int, minValue int, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func envBool(key string, defaultValue bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return defaultValue
	}
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func envPositiveInt(key string, defaultValue int) (int, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return parsed, nil
}

func envPositiveInt64(key string, defaultValue int64) (int64, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return parsed, nil
}

func envPositiveFloat(key string, defaultValue float64) (float64, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive number", key)
	}
	return parsed, nil
}

func envDuration(key string, defaultValue time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", key)
	}
	return parsed, nil
}
