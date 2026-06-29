package servercore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
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
	envPineWorkerBinary            = "JFTRADE_PINEWORKER_BINARY"
	envPineWorkerSHA256            = "JFTRADE_PINEWORKER_SHA256"
	envPineWorkerWorkers           = "JFTRADE_PINEWORKER_WORKERS"
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

	defaultPineWorkerIdleTimeout = 60 * time.Second
)

type pineWorkerRuntimeConfig struct {
	BinaryPath        string
	SHA256            string
	Workers           int
	Host              string
	StartPort         int
	TempDir           string
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
	binaryData        []byte
}

type pineWorkerRunner interface {
	RunScript(context.Context, pineworker.RunScriptRequest) (pineworker.RunScriptResponse, error)
}

func (s *Server) startPineWorkerManager() pineWorkerRunner {
	config, enabled, err := resolvePineWorkerRuntimeConfig(s.pineWorkerSettings)
	if err != nil {
		log.Printf("JFTrade PineTS worker manager disabled by invalid config: %v", err)
		return nil
	}
	if !enabled {
		log.Printf("JFTrade PineTS worker manager not started: %s is not configured and no embedded worker asset is available; run `npm run dev:api:pineworker` or set %s=/absolute/path/to/worker", envPineWorkerBinary, envPineWorkerBinary)
		return nil
	}

	manager, err := newPineWorkerManagerFromConfig(config)
	if err != nil {
		log.Printf("JFTrade PineTS worker manager disabled: create manager: %v", err)
		return nil
	}
	runner := newLazyPineWorkerRunner(config, manager, defaultPineWorkerIdleTimeout)
	s.pineWorkerRunner = runner
	source := "external"
	if config.embedded {
		source = "embedded"
	}
	log.Printf("JFTrade PineTS worker manager configured: source=%s workers=%d host=%s startPort=%d idleTimeout=%s", source, config.Workers, config.Host, config.StartPort, defaultPineWorkerIdleTimeout)
	return runner
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
	previous := s.pineWorkerRunner
	if previous != nil {
		if runner, ok := previous.(*lazyPineWorkerRunner); ok {
			_ = runner.RetireWhenIdle(context.Background())
		} else if closer, ok := previous.(interface{ Close(context.Context) error }); ok {
			_ = closer.Close(context.Background())
		}
	}
	s.pineWorkerRunner = nil
	next := s.startPineWorkerManager()
	if next == nil {
		return
	}
	s.pineWorkerRunner = next
	if s.strategyRuntimeManager != nil {
		s.strategyRuntimeManager.pineWorkerRunner = next
	}
	if s.backtestSvc != nil {
		s.backtestSvc.SetPineWorkerRunner(next)
	}
}

type lazyPineWorkerRunner struct {
	mu          sync.Mutex
	config      pineWorkerRuntimeConfig
	manager     *pineworker.WorkerManager
	idleTimeout time.Duration
	idleTimer   *time.Timer
	active      int
	running     bool
	closed      bool
}

func newLazyPineWorkerRunner(config pineWorkerRuntimeConfig, manager *pineworker.WorkerManager, idleTimeout time.Duration) *lazyPineWorkerRunner {
	if idleTimeout <= 0 {
		idleTimeout = defaultPineWorkerIdleTimeout
	}
	return &lazyPineWorkerRunner{
		config:      config,
		manager:     manager,
		idleTimeout: idleTimeout,
	}
}

func (runner *lazyPineWorkerRunner) RunScript(ctx context.Context, request pineworker.RunScriptRequest) (pineworker.RunScriptResponse, error) {
	manager, err := runner.acquire(ctx)
	if err != nil {
		return pineworker.RunScriptResponse{}, err
	}
	defer runner.release()
	return manager.RunScript(ctx, request)
}

func (runner *lazyPineWorkerRunner) Close(ctx context.Context) error {
	return runner.retire(ctx, true)
}

func (runner *lazyPineWorkerRunner) RetireWhenIdle(ctx context.Context) error {
	return runner.retire(ctx, false)
}

func (runner *lazyPineWorkerRunner) retire(ctx context.Context, immediate bool) error {
	runner.mu.Lock()
	runner.closed = true
	if runner.idleTimer != nil {
		runner.idleTimer.Stop()
		runner.idleTimer = nil
	}
	manager := runner.manager
	running := runner.running && (immediate || runner.active == 0)
	if running {
		runner.running = false
	}
	if immediate {
		runner.active = 0
	}
	runner.mu.Unlock()
	if running {
		return manager.Stop(ctx)
	}
	return nil
}

func (runner *lazyPineWorkerRunner) IsRunning() bool {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.running
}

func (runner *lazyPineWorkerRunner) acquire(ctx context.Context) (*pineworker.WorkerManager, error) {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if runner.closed {
		return nil, fmt.Errorf("pine worker runner is closed")
	}
	if runner.idleTimer != nil {
		runner.idleTimer.Stop()
		runner.idleTimer = nil
	}
	if !runner.running {
		if err := runner.manager.Start(ctx); err != nil {
			return nil, err
		}
		runner.running = true
	}
	runner.active++
	return runner.manager, nil
}

func (runner *lazyPineWorkerRunner) release() {
	runner.mu.Lock()
	if runner.active > 0 {
		runner.active--
	}
	if runner.closed {
		if runner.active > 0 || !runner.running {
			runner.mu.Unlock()
			return
		}
		manager := runner.manager
		runner.running = false
		runner.mu.Unlock()
		if err := manager.Stop(context.Background()); err != nil {
			log.Printf("JFTrade PineTS worker manager retire stop failed: %v", err)
		}
		return
	}
	if runner.active > 0 || !runner.running {
		runner.mu.Unlock()
		return
	}
	runner.idleTimer = time.AfterFunc(runner.idleTimeout, runner.stopIfIdle)
	runner.mu.Unlock()
}

func (runner *lazyPineWorkerRunner) stopIfIdle() {
	runner.mu.Lock()
	if runner.closed || runner.active > 0 || !runner.running {
		runner.mu.Unlock()
		return
	}
	manager := runner.manager
	runner.running = false
	runner.idleTimer = nil
	runner.mu.Unlock()
	if err := manager.Stop(context.Background()); err != nil {
		log.Printf("JFTrade PineTS worker manager idle stop failed: %v", err)
	}
}

func newPineWorkerManagerFromConfig(config pineWorkerRuntimeConfig) (*pineworker.WorkerManager, error) {
	binaryData := config.binaryData
	if len(binaryData) == 0 {
		var err error
		binaryData, err = os.ReadFile(config.BinaryPath)
		if err != nil {
			return nil, fmt.Errorf("read worker binary: %w", err)
		}
	}
	workerConfig := pineworker.DefaultWorkerConfig(runtime.NumCPU())
	workerConfig.BacktestWorkers = config.Workers
	workerConfig.RequestTimeout = config.RequestTimeout
	workerConfig.MaxMessageBytes = config.MaxMessageBytes
	workerConfig.MaxCandlesPerRequest = config.MaxCandles

	launcher, err := newPineWorkerLauncher(config, binaryData)
	if err != nil {
		return nil, fmt.Errorf("create launcher: %w", err)
	}
	manager, err := pineworker.NewWorkerManager(pineworker.ManagerConfig{
		Workers:       config.Workers,
		Host:          config.Host,
		StartPort:     config.StartPort,
		HealthTimeout: config.HealthTimeout,
		WorkerConfig:  workerConfig,
		Gate: pineworker.PerformanceGate{
			MaxDuration:       config.MaxDuration,
			MaxDurationPerBar: config.MaxDurationPerBar,
			MinCandlesPerSec:  config.MinCandlesPerSec,
			MaxRequestBytes:   config.MaxMessageBytes,
			MaxResponseBytes:  config.MaxMessageBytes,
			MaxPeakRSSBytes:   config.MaxPeakRSSBytes,
		},
	}, launcher, newPineWorkerDialer(config.MaxMessageBytes))
	if err != nil {
		return nil, err
	}
	return manager, nil
}

func defaultNewPineWorkerLauncher(config pineWorkerRuntimeConfig, binaryData []byte) (pineworker.WorkerLauncher, error) {
	if config.SHA256 == "" {
		sum := sha256.Sum256(binaryData)
		config.SHA256 = hex.EncodeToString(sum[:])
	}
	return pineworker.NewBinaryWorkerLauncher(pineworker.BinaryWorkerLauncherConfig{
		Binary:          pineworker.WorkerBinary{Name: filepath.Base(config.BinaryPath), Data: binaryData, SHA256: config.SHA256},
		TempDir:         config.TempDir,
		ProtoPath:       config.ProtoPath,
		MaxMessageBytes: config.MaxMessageBytes,
		PineTSVersion:   config.PineTSVersion,
		Mock:            config.Mock,
	})
}

func defaultNewPineWorkerDialer(maxMessageBytes int) pineworker.TransportDialer {
	return pineworker.NewGRPCDialer(pineworker.GRPCDialerConfig{MaxMessageBytes: maxMessageBytes})
}

func resolvePineWorkerRuntimeConfig(settingsProvider func() jftsettings.PineWorkerSettings) (pineWorkerRuntimeConfig, bool, error) {
	if envBool(envPineWorkerDisabled, false) {
		return pineWorkerRuntimeConfig{}, false, nil
	}
	binaryPath := strings.TrimSpace(os.Getenv(envPineWorkerBinary))
	var embeddedAsset pineworkerassets.Asset
	var embedded bool
	if binaryPath == "" {
		var err error
		embeddedAsset, embedded, err = selectPineWorkerAsset()
		if err != nil {
			return pineWorkerRuntimeConfig{}, false, err
		}
		if !embedded {
			return pineWorkerRuntimeConfig{}, false, nil
		}
		binaryPath = embeddedAsset.Name
	}
	defaultWorkers := settingsfile.DefaultPineWorkerSettings().WorkerLimit
	if settingsProvider != nil {
		defaultWorkers = settingsfile.NormalizePineWorkerSettings(settingsProvider()).WorkerLimit
	}
	defaultWorkerConfig := pineworker.DefaultWorkerConfig(runtime.NumCPU())
	workers, err := envIntInRange(envPineWorkerWorkers, defaultWorkers, 1, 1000)
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
	return pineWorkerRuntimeConfig{
		BinaryPath:        binaryPath,
		SHA256:            firstNonEmpty(strings.TrimSpace(os.Getenv(envPineWorkerSHA256)), embeddedAsset.SHA256),
		Workers:           workers,
		Host:              host,
		StartPort:         startPort,
		TempDir:           strings.TrimSpace(os.Getenv(envPineWorkerTempDir)),
		ProtoPath:         strings.TrimSpace(os.Getenv(envPineWorkerProto)),
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
		binaryData:        embeddedAsset.Data,
	}, true, nil
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
