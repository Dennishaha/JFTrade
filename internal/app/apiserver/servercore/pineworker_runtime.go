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
	"time"

	"github.com/jftrade/jftrade-main/internal/pineworkerassets"
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

func (s *Server) startPineWorkerManager(ctx context.Context) *pineworker.WorkerManager {
	config, enabled, err := resolvePineWorkerRuntimeConfig()
	if err != nil {
		log.Printf("JFTrade PineTS worker manager disabled by invalid config: %v", err)
		return nil
	}
	if !enabled {
		log.Printf("JFTrade PineTS worker manager not started: %s is not configured and no embedded worker asset is available; run `npm run dev:api:pineworker` or set %s=/absolute/path/to/worker", envPineWorkerBinary, envPineWorkerBinary)
		return nil
	}

	binaryData := config.binaryData
	if len(binaryData) == 0 {
		var err error
		binaryData, err = os.ReadFile(config.BinaryPath)
		if err != nil {
			log.Printf("JFTrade PineTS worker manager disabled: read worker binary: %v", err)
			return nil
		}
	}
	workerConfig := pineworker.DefaultWorkerConfig(runtime.NumCPU())
	workerConfig.BacktestWorkers = config.Workers
	workerConfig.RequestTimeout = config.RequestTimeout
	workerConfig.MaxMessageBytes = config.MaxMessageBytes
	workerConfig.MaxCandlesPerRequest = config.MaxCandles

	launcher, err := newPineWorkerLauncher(config, binaryData)
	if err != nil {
		log.Printf("JFTrade PineTS worker manager disabled: create launcher: %v", err)
		return nil
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
		log.Printf("JFTrade PineTS worker manager disabled: create manager: %v", err)
		return nil
	}
	if err := manager.Start(ctx); err != nil {
		log.Printf("JFTrade PineTS worker manager disabled: start manager: %v", err)
		return nil
	}
	s.pineWorkerManager = manager
	source := "external"
	if config.embedded {
		source = "embedded"
	}
	log.Printf("JFTrade PineTS worker manager started: source=%s workers=%d host=%s startPort=%d", source, config.Workers, config.Host, config.StartPort)
	return manager
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

func resolvePineWorkerRuntimeConfig() (pineWorkerRuntimeConfig, bool, error) {
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
	defaultWorkerConfig := pineworker.DefaultWorkerConfig(runtime.NumCPU())
	workers, err := envPositiveInt(envPineWorkerWorkers, defaultWorkerConfig.BacktestWorkers)
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
