package pineruntime

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/internal/pineworkerassets"
	"github.com/jftrade/jftrade-main/pkg/jftsettings"
	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

const (
	EnvDisabled          = "JFTRADE_PINEWORKER_DISABLED"
	EnvBundle            = "JFTRADE_PINEWORKER_BUNDLE"
	EnvRuntime           = "JFTRADE_PINEWORKER_RUNTIME"
	EnvSHA256            = "JFTRADE_PINEWORKER_SHA256"
	EnvBacktestWorkers   = "JFTRADE_PINEWORKER_BACKTEST_WORKERS"
	EnvInstanceWorkers   = "JFTRADE_PINEWORKER_INSTANCE_WORKERS"
	EnvHost              = "JFTRADE_PINEWORKER_HOST"
	EnvStartPort         = "JFTRADE_PINEWORKER_START_PORT"
	EnvTempDir           = "JFTRADE_PINEWORKER_TEMP_DIR"
	EnvProto             = "JFTRADE_PINEWORKER_PROTO"
	EnvPineTSVersion     = "JFTRADE_PINEWORKER_PINETS_VERSION"
	EnvMock              = "JFTRADE_PINEWORKER_MOCK"
	EnvRequestTimeout    = "JFTRADE_PINEWORKER_REQUEST_TIMEOUT"
	EnvHealthTimeout     = "JFTRADE_PINEWORKER_HEALTH_TIMEOUT"
	EnvMaxMessageBytes   = "JFTRADE_PINEWORKER_MAX_MESSAGE_BYTES"
	EnvMaxCandles        = "JFTRADE_PINEWORKER_MAX_CANDLES"
	EnvMaxDuration       = "JFTRADE_PINEWORKER_MAX_DURATION"
	EnvMaxDurationPerBar = "JFTRADE_PINEWORKER_MAX_DURATION_PER_BAR"
	EnvMinCandlesPerSec  = "JFTRADE_PINEWORKER_MIN_CANDLES_PER_SEC"
	EnvMaxPeakRSSBytes   = "JFTRADE_PINEWORKER_MAX_PEAK_RSS_BYTES"

	DefaultProtoPath = "pkg/strategy/pineworker/proto/pineworker.proto"
	logTailBytes     = 8192
)

// Config is the resolved process and resource policy for an ephemeral worker.
type Config struct {
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

// Source reports whether the selected worker bundle is embedded or external.
func (config Config) Source() string {
	if config.embedded {
		return "embedded"
	}
	return "external"
}

type AssetSelector func() (pineworkerassets.Asset, bool, error)
type LauncherFactory func(Config, []byte) (pineworker.WorkerLauncher, error)
type DialerFactory func(int) pineworker.TransportDialer
type RuntimeResolver func(jftsettings.PineWorkerSettings) string

type dependencies struct {
	selectAsset    AssetSelector
	newLauncher    LauncherFactory
	newDialer      DialerFactory
	resolveRuntime RuntimeResolver
	getwd          func() (string, error)
	abs            func(string) (string, error)
}

type Option func(*dependencies)

func WithAssetSelector(selector AssetSelector) Option {
	return func(deps *dependencies) { deps.selectAsset = selector }
}

func WithLauncherFactory(factory LauncherFactory) Option {
	return func(deps *dependencies) { deps.newLauncher = factory }
}

func WithDialerFactory(factory DialerFactory) Option {
	return func(deps *dependencies) { deps.newDialer = factory }
}

func WithRuntimeResolver(resolver RuntimeResolver) Option {
	return func(deps *dependencies) { deps.resolveRuntime = resolver }
}

func defaultDependencies() dependencies {
	return dependencies{
		selectAsset:    pineworkerassets.Select,
		newLauncher:    NewNodeLauncher,
		newDialer:      NewGRPCDialer,
		resolveRuntime: defaultRuntimePath,
		getwd:          os.Getwd,
		abs:            filepath.Abs,
	}
}

func applyOptions(options []Option) dependencies {
	deps := defaultDependencies()
	for _, option := range options {
		if option != nil {
			option(&deps)
		}
	}
	return deps
}

// ResolveConfig resolves settings and environment overrides without starting a process.
func ResolveConfig(settings jftsettings.PineWorkerSettings, options ...Option) (Config, bool, error) {
	return resolveConfig(settings, applyOptions(options))
}

func resolveConfig(settings jftsettings.PineWorkerSettings, deps dependencies) (Config, bool, error) {
	if envBool(EnvDisabled, false) {
		return Config{}, false, nil
	}
	bundle, enabled, err := resolveBundle(deps.selectAsset)
	if err != nil || !enabled {
		return Config{}, enabled, err
	}
	workerDefaults := pineworker.DefaultWorkerConfig(runtime.NumCPU())
	backtestWorkers, err := envIntInRange(EnvBacktestWorkers, normalizedWorkerLimit(settings.BacktestWorkerLimit, 2), 1, 1000)
	if err != nil {
		return Config{}, false, err
	}
	instanceWorkers, err := envIntInRange(EnvInstanceWorkers, normalizedWorkerLimit(settings.InstanceWorkerLimit, 10), 1, 1000)
	if err != nil {
		return Config{}, false, err
	}
	config, err := resolveLimits(workerDefaults)
	if err != nil {
		return Config{}, false, err
	}
	workDir := resolveWorkDir(bundle.path, deps)
	config.BundlePath = bundle.path
	config.RuntimePath = deps.resolveRuntime(settings)
	config.SHA256 = firstNonEmpty(strings.TrimSpace(os.Getenv(EnvSHA256)), bundle.asset.SHA256)
	config.BacktestWorkers = backtestWorkers
	config.InstanceWorkers = instanceWorkers
	config.WorkDir = workDir
	config.ProtoPath = resolvePath(envOrDefault(EnvProto, DefaultProtoPath), workDir, deps.abs)
	config.embedded = bundle.embedded
	config.bundleData = bundle.asset.Data
	return config, true, nil
}

type bundleConfig struct {
	path     string
	asset    pineworkerassets.Asset
	embedded bool
}

func resolveBundle(selector AssetSelector) (bundleConfig, bool, error) {
	if path := strings.TrimSpace(os.Getenv(EnvBundle)); path != "" {
		return bundleConfig{path: path}, true, nil
	}
	asset, embedded, err := selector()
	if err != nil || !embedded {
		return bundleConfig{}, false, err
	}
	return bundleConfig{path: asset.Name, asset: asset, embedded: true}, true, nil
}

func resolveLimits(defaults pineworker.WorkerConfig) (Config, error) {
	startPort, err := envPositiveInt(EnvStartPort, 50051)
	if err != nil {
		return Config{}, err
	}
	requestTimeout, err := envDuration(EnvRequestTimeout, defaults.RequestTimeout)
	if err != nil {
		return Config{}, err
	}
	healthTimeout, err := envDuration(EnvHealthTimeout, 5*time.Second)
	if err != nil {
		return Config{}, err
	}
	maxMessageBytes, err := envPositiveInt(EnvMaxMessageBytes, defaults.MaxMessageBytes)
	if err != nil {
		return Config{}, err
	}
	maxCandles, err := envPositiveInt(EnvMaxCandles, defaults.MaxCandlesPerRequest)
	if err != nil {
		return Config{}, err
	}
	maxDuration, maxDurationPerBar, minCandlesPerSec, maxPeakRSSBytes, err := resolveGate()
	if err != nil {
		return Config{}, err
	}
	host := strings.TrimSpace(os.Getenv(EnvHost))
	if host == "" {
		host = "127.0.0.1"
	}
	return Config{
		Host: host, StartPort: startPort, TempDir: strings.TrimSpace(os.Getenv(EnvTempDir)),
		PineTSVersion: strings.TrimSpace(os.Getenv(EnvPineTSVersion)), Mock: envBool(EnvMock, false),
		RequestTimeout: requestTimeout, HealthTimeout: healthTimeout, MaxMessageBytes: maxMessageBytes,
		MaxCandles: maxCandles, MaxDuration: maxDuration, MaxDurationPerBar: maxDurationPerBar,
		MinCandlesPerSec: minCandlesPerSec, MaxPeakRSSBytes: maxPeakRSSBytes,
	}, nil
}

func resolveGate() (time.Duration, time.Duration, float64, int64, error) {
	gate := pineworker.DefaultPerformanceGate()
	maxDuration, err := envDuration(EnvMaxDuration, gate.MaxDuration)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	maxDurationPerBar, err := envDuration(EnvMaxDurationPerBar, gate.MaxDurationPerBar)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	minCandlesPerSec, err := envPositiveFloat(EnvMinCandlesPerSec, gate.MinCandlesPerSec)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	maxPeakRSSBytes, err := envPositiveInt64(EnvMaxPeakRSSBytes, gate.MaxPeakRSSBytes)
	return maxDuration, maxDurationPerBar, minCandlesPerSec, maxPeakRSSBytes, err
}

func NewNodeLauncher(config Config, bundleData []byte) (pineworker.WorkerLauncher, error) {
	if config.SHA256 == "" {
		sum := sha256.Sum256(bundleData)
		config.SHA256 = hex.EncodeToString(sum[:])
	}
	return pineworker.NewNodeWorkerLauncher(pineworker.NodeWorkerLauncherConfig{
		Bundle:      pineworker.WorkerBundle{Name: filepath.Base(config.BundlePath), Data: bundleData, SHA256: config.SHA256},
		RuntimePath: config.RuntimePath, TempDir: config.TempDir, WorkDir: config.WorkDir, ProtoPath: config.ProtoPath,
		MaxMessageBytes: config.MaxMessageBytes, PineTSVersion: config.PineTSVersion, Mock: config.Mock,
		Stdout: pineworker.NewTailBuffer(logTailBytes), Stderr: pineworker.NewTailBuffer(logTailBytes),
	})
}

func NewGRPCDialer(maxMessageBytes int) pineworker.TransportDialer {
	return pineworker.NewGRPCDialer(pineworker.GRPCDialerConfig{MaxMessageBytes: maxMessageBytes})
}

func defaultRuntimePath(settings jftsettings.PineWorkerSettings) string {
	if path := normalizePath(settings.NodeBinaryPath); path != "" {
		return path
	}
	if path := normalizePath(os.Getenv(EnvRuntime)); path != "" {
		return path
	}
	if path := normalizePath(os.Getenv("JFTRADE_NODE_BINARY")); path != "" {
		return path
	}
	return "node"
}

func normalizePath(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
		value = strings.TrimSpace(value[1 : len(value)-1])
	}
	return value
}

func resolveWorkDir(bundlePath string, deps dependencies) string {
	wd, err := deps.getwd()
	if err == nil {
		if root := findRepoRoot(wd); root != "" {
			return root
		}
	}
	if bundlePath != "" {
		if root := findRepoRoot(filepath.Dir(resolvePath(bundlePath, wd, deps.abs))); root != "" {
			return root
		}
	}
	if err != nil {
		return ""
	}
	return wd
}

func findRepoRoot(start string) string {
	for dir := filepath.Clean(start); ; dir = filepath.Dir(dir) {
		if fileExists(filepath.Join(dir, "go.mod")) && fileExists(filepath.Join(dir, filepath.FromSlash(DefaultProtoPath))) {
			return dir
		}
		if filepath.Dir(dir) == dir {
			return ""
		}
	}
}

func resolvePath(value string, base string, abs func(string) (string, error)) string {
	if value == "" || filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	if base != "" {
		return filepath.Join(base, value)
	}
	absolute, err := abs(value)
	if err != nil {
		return filepath.Clean(value)
	}
	return absolute
}

func fileExists(path string) bool { _, err := os.Stat(path); return err == nil }
func envOrDefault(key string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
func normalizedWorkerLimit(value int, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}
func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	return value == "1" || value == "true" || value == "yes" || value == "on"
}
func envIntInRange(key string, fallback int, minValue int, maxValue int) (int, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return min(max(fallback, minValue), maxValue), nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < minValue || parsed > maxValue {
		return 0, fmt.Errorf("%s must be an integer between %d and %d", key, minValue, maxValue)
	}
	return parsed, nil
}
func envPositiveInt(key string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return parsed, nil
}
func envPositiveInt64(key string, fallback int64) (int64, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return parsed, nil
}
func envPositiveFloat(key string, fallback float64) (float64, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive number", key)
	}
	return parsed, nil
}
func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", key)
	}
	return parsed, nil
}
