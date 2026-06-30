package servercore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
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
		log.Printf("JFTrade PineTS worker manager not started: %s is not configured and no embedded worker asset is available; run `npm run dev:api:pineworker` or set %s=/absolute/path/to/worker.mjs", envPineWorkerBundle, envPineWorkerBundle)
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
	retirePineWorkerRunner(s.backtestPineWorkerRunner)
	retirePineWorkerRunner(s.instancePineWorkerRunner)
	s.backtestPineWorkerRunner = nil
	s.instancePineWorkerRunner = nil
	backtestRunner, instanceRunner := s.startPineWorkerManagers()
	s.backtestPineWorkerRunner = backtestRunner
	s.instancePineWorkerRunner = instanceRunner
	if s.strategyRuntimeManager != nil {
		s.strategyRuntimeManager.pineWorkerRunner = instanceRunner
	}
	if s.backtestSvc != nil {
		s.backtestSvc.SetPineWorkerRunner(backtestRunner)
	}
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

func (runner *ephemeralPineWorkerRunner) Close(context.Context) error {
	return nil
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
	defer listener.Close()
	tcpAddr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("allocate pine worker port: unexpected address %s", listener.Addr())
	}
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

func resolvePineWorkerRuntimeConfig(settingsProvider func() jftsettings.PineWorkerSettings) (pineWorkerRuntimeConfig, bool, error) {
	if envBool(envPineWorkerDisabled, false) {
		return pineWorkerRuntimeConfig{}, false, nil
	}
	bundlePath := strings.TrimSpace(os.Getenv(envPineWorkerBundle))
	var embeddedAsset pineworkerassets.Asset
	var embedded bool
	if bundlePath == "" {
		var err error
		embeddedAsset, embedded, err = selectPineWorkerAsset()
		if err != nil {
			return pineWorkerRuntimeConfig{}, false, err
		}
		if !embedded {
			return pineWorkerRuntimeConfig{}, false, nil
		}
		bundlePath = embeddedAsset.Name
	}
	defaultSettings := settingsfile.DefaultPineWorkerSettings()
	if settingsProvider != nil {
		defaultSettings = settingsfile.NormalizePineWorkerSettings(settingsProvider())
	}
	defaultWorkerConfig := pineworker.DefaultWorkerConfig(runtime.NumCPU())
	backtestWorkers, err := envIntInRange(envPineWorkerBacktestWorkers, defaultSettings.BacktestWorkerLimit, 1, 1000)
	if err != nil {
		return pineWorkerRuntimeConfig{}, false, err
	}
	instanceWorkers, err := envIntInRange(envPineWorkerInstanceWorkers, defaultSettings.InstanceWorkerLimit, 1, 1000)
	if err != nil {
		return pineWorkerRuntimeConfig{}, false, err
	}
	startPort, err := envPositiveInt(envPineWorkerStartPort, 50051)
	if err != nil {
		return pineWorkerRuntimeConfig{}, false, err
	}
	requestTimeout, err := envDuration(envPineWorkerRequestTimeout, defaultWorkerConfig.RequestTimeout)
	if err != nil {
		return pineWorkerRuntimeConfig{}, false, err
	}
	healthTimeout, err := envDuration(envPineWorkerHealthTimeout, 5*time.Second)
	if err != nil {
		return pineWorkerRuntimeConfig{}, false, err
	}
	maxMessageBytes, err := envPositiveInt(envPineWorkerMaxMessageBytes, defaultWorkerConfig.MaxMessageBytes)
	if err != nil {
		return pineWorkerRuntimeConfig{}, false, err
	}
	maxCandles, err := envPositiveInt(envPineWorkerMaxCandles, defaultWorkerConfig.MaxCandlesPerRequest)
	if err != nil {
		return pineWorkerRuntimeConfig{}, false, err
	}
	gate := pineworker.DefaultPerformanceGate()
	maxDuration, err := envDuration(envPineWorkerMaxDuration, gate.MaxDuration)
	if err != nil {
		return pineWorkerRuntimeConfig{}, false, err
	}
	maxDurationPerBar, err := envDuration(envPineWorkerMaxDurationPerBar, gate.MaxDurationPerBar)
	if err != nil {
		return pineWorkerRuntimeConfig{}, false, err
	}
	minCandlesPerSec, err := envPositiveFloat(envPineWorkerMinCandlesPerSec, gate.MinCandlesPerSec)
	if err != nil {
		return pineWorkerRuntimeConfig{}, false, err
	}
	maxPeakRSSBytes, err := envPositiveInt64(envPineWorkerMaxPeakRSSBytes, gate.MaxPeakRSSBytes)
	if err != nil {
		return pineWorkerRuntimeConfig{}, false, err
	}
	host := strings.TrimSpace(os.Getenv(envPineWorkerHost))
	if host == "" {
		host = "127.0.0.1"
	}
	workDir := resolvePineWorkerWorkDir(bundlePath)
	protoPath := resolvePineWorkerProtoPath(workDir)
	runtimePath := resolvePineWorkerRuntime(defaultSettings)
	return pineWorkerRuntimeConfig{
		BundlePath:        bundlePath,
		RuntimePath:       runtimePath,
		SHA256:            firstNonEmpty(strings.TrimSpace(os.Getenv(envPineWorkerSHA256)), embeddedAsset.SHA256),
		BacktestWorkers:   backtestWorkers,
		InstanceWorkers:   instanceWorkers,
		Host:              host,
		StartPort:         startPort,
		TempDir:           strings.TrimSpace(os.Getenv(envPineWorkerTempDir)),
		WorkDir:           workDir,
		ProtoPath:         protoPath,
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
		embedded:          embedded,
		bundleData:        embeddedAsset.Data,
	}, true, nil
}

func resolvePineWorkerRuntime(settings jftsettings.PineWorkerSettings) string {
	if value := settingsfile.NormalizeNodeBinaryPath(settings.NodeBinaryPath); value != "" {
		return value
	}
	if value := settingsfile.NormalizeNodeBinaryPath(os.Getenv(envPineWorkerRuntime)); value != "" {
		return value
	}
	if value := settingsfile.NormalizeNodeBinaryPath(os.Getenv("JFTRADE_NODE_BINARY")); value != "" {
		return value
	}
	return "node"
}

func resolvePineWorkerWorkDir(bundlePath string) string {
	wd, err := os.Getwd()
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
	absolute, err := filepath.Abs(value)
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
