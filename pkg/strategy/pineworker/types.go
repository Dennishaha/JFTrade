package pineworker

import (
	"fmt"
	"strings"
	"time"
)

const (
	RuntimeID       = "pine-pinets"
	LegacyRuntimeID = "pine-go-plan"

	ModeBacktest = "backtest"
	ModeLive     = "live"
	ModeAnalyze  = "analyze"

	SessionOperationOpen   = "open"
	SessionOperationAppend = "append"
	SessionOperationClose  = "close"
)

type WorkerConfig struct {
	LiveWorkers          int
	BacktestWorkers      int
	OptimizationWorkers  int
	RequestTimeout       time.Duration
	HealthCheckInterval  time.Duration
	MaxMessageBytes      int
	MaxCandlesPerRequest int
}

func DefaultWorkerConfig(cpuCount int) WorkerConfig {
	if cpuCount < 1 {
		cpuCount = 1
	}
	return WorkerConfig{
		LiveWorkers:          clamp(cpuCount, 2, 4),
		BacktestWorkers:      max(1, cpuCount/2),
		OptimizationWorkers:  cpuCount,
		RequestTimeout:       30 * time.Second,
		HealthCheckInterval:  5 * time.Second,
		MaxMessageBytes:      0,
		MaxCandlesPerRequest: 0,
	}
}

func NormalizeRuntime(value string) string {
	normalized := strings.TrimSpace(strings.ToLower(value))
	if normalized == "" || normalized == LegacyRuntimeID || normalized == RuntimeID {
		return RuntimeID
	}
	return normalized
}

func SupportsRuntime(value string) bool {
	return NormalizeRuntime(value) == RuntimeID
}

// Candle is the canonical numeric K-line of the Pine worker wire protocol
// (RunScriptRequest.Candles). All producers build it from bbgo K-lines via the
// single shared converter backtest.CandleFromKLine; do not grow ad-hoc copies.
//
// Semantics:
//   - OpenTime and CloseTime are epoch milliseconds in UTC.
//   - OpenTime is the bar's inclusive open. CloseTime is the bar's inclusive
//     close timestamp (the bbgo "binance rule": typically OpenTime + interval - 1ms,
//     never overlapping the next bar's open). CloseTime may be 0 when unknown;
//     the TypeScript worker treats it as optional.
//   - OHLCV are float64 approximations of the source fixed-point values: fine
//     for indicator math and plotting, not decimal-exact.
//
// The production transport is gRPC with a binary candle batch (see
// proto_mapping.go), so these JSON tags are not on that wire. They exist
// because JSON-RPC peers — the PineTS shadow worker (scripts/pinets-worker.mjs)
// via pineengine.RunIndicatorRequest — consume this same struct shape with
// camelCase field names; keep the tags stable.
type Candle struct {
	OpenTime  int64   `json:"openTime"`
	CloseTime int64   `json:"closeTime"`
	Open      float64 `json:"open"`
	High      float64 `json:"high"`
	Low       float64 `json:"low"`
	Close     float64 `json:"close"`
	Volume    float64 `json:"volume"`
}

type RunScriptRequest struct {
	JobID            string
	ScriptID         string
	Source           string
	Symbol           string
	Timeframe        string
	Mode             string
	Candles          []Candle
	Params           map[string]string
	SessionID        string
	SessionOperation string
	ExpectedRevision uint64
}

type Diagnostic struct {
	Severity string
	Code     string
	Message  string
	Line     int
	Column   int
}

type SeriesOutput struct {
	Name   string
	Kind   string
	Values []float64
}

type PlotOutput struct {
	Name   string
	Values []float64
}

type AlertEvent struct {
	Type      string
	ID        string
	Message   string
	Title     string
	Frequency string
	BarIndex  int
	Time      int64
}

type VisualOutput struct {
	Kind        string
	Name        string
	PayloadJSON string
}

type OrderIntent struct {
	Kind           string
	ID             string
	FromEntry      string
	Direction      string
	Quantity       float64
	QuantityPct    float64
	LimitPrice     float64
	StopPrice      float64
	Comment        string
	AlertMessage   string
	DisableAlert   bool
	BarIndex       int
	Time           int64
	HasQuantity    bool
	HasQuantityPct bool
	HasLimitPrice  bool
	HasStopPrice   bool
	ParentID       string
	AtomicGroupID  string
	OCOGroupID     string
	ReduceOnly     bool
}

type WorkerMetadata struct {
	WorkerID      string
	Version       string
	PineTSVersion string
	ScriptHash    string
	DataHash      string
	Duration      time.Duration
	RequestBytes  int
	ResponseBytes int
	PeakRSSBytes  int64
}

type StrategyMetrics struct {
	BuyAndHoldPnL             float64
	BuyAndHoldPerGain         float64
	StrategyOutperformance    float64
	HasBuyAndHoldPnL          bool
	HasBuyAndHoldPerGain      bool
	HasStrategyOutperformance bool
}

type RunScriptResponse struct {
	JobID           string
	Outputs         []SeriesOutput
	Plots           []PlotOutput
	OrderIntents    []OrderIntent
	Alerts          []AlertEvent
	VisualOutputs   []VisualOutput
	Logs            []string
	Warnings        []string
	Diagnostics     []Diagnostic
	Error           string
	Metadata        WorkerMetadata
	StrategyMetrics *StrategyMetrics
	SessionID       string
	SessionRevision uint64
}

func ValidateRunScriptRequest(request RunScriptRequest, config WorkerConfig) error {
	_, err := validateAndMeasureRunScriptRequest(request, config)
	return err
}

func validateRunScriptRequestBasics(request RunScriptRequest, config WorkerConfig) error {
	if strings.TrimSpace(request.JobID) == "" {
		return fmt.Errorf("job id is required")
	}
	operation := normalizeSessionOperation(request.SessionOperation)
	if strings.TrimSpace(request.SessionOperation) != "" && operation == "" {
		return fmt.Errorf("unsupported pine worker session operation: %s", request.SessionOperation)
	}
	if operation != "" && strings.TrimSpace(request.SessionID) == "" {
		return fmt.Errorf("session id is required for %s", operation)
	}
	mode := normalizeMode(request.Mode)
	if mode == "" {
		return fmt.Errorf("unsupported pine worker mode: %s", request.Mode)
	}
	if operation != "" && mode != ModeLive {
		return fmt.Errorf("pine worker sessions require live mode")
	}
	if operation == SessionOperationOpen && request.ExpectedRevision != 0 {
		return fmt.Errorf("pine worker session open requires expected revision 0")
	}
	if operation == SessionOperationAppend && request.ExpectedRevision == 0 {
		return fmt.Errorf("pine worker session append requires a positive expected revision")
	}
	if operation == SessionOperationClose {
		return nil
	}
	if strings.TrimSpace(request.Source) == "" {
		return fmt.Errorf("source is required")
	}
	if strings.TrimSpace(request.Symbol) == "" {
		return fmt.Errorf("symbol is required")
	}
	if strings.TrimSpace(request.Timeframe) == "" {
		return fmt.Errorf("timeframe is required")
	}
	if len(request.Candles) == 0 && mode != ModeAnalyze {
		return fmt.Errorf("candles are required")
	}
	if config.MaxCandlesPerRequest > 0 && len(request.Candles) > config.MaxCandlesPerRequest {
		return fmt.Errorf("too many candles: %d > %d", len(request.Candles), config.MaxCandlesPerRequest)
	}
	return nil
}

func normalizeSessionOperation(operation string) string {
	switch strings.TrimSpace(strings.ToLower(operation)) {
	case "":
		return ""
	case SessionOperationOpen:
		return SessionOperationOpen
	case SessionOperationAppend:
		return SessionOperationAppend
	case SessionOperationClose:
		return SessionOperationClose
	default:
		return ""
	}
}

func normalizeMode(mode string) string {
	switch strings.TrimSpace(strings.ToLower(mode)) {
	case "", ModeBacktest:
		return ModeBacktest
	case ModeLive:
		return ModeLive
	case ModeAnalyze:
		return ModeAnalyze
	default:
		return ""
	}
}

func validateCandle(candle Candle) error {
	if candle.OpenTime <= 0 {
		return fmt.Errorf("open time is required")
	}
	if candle.CloseTime > 0 && candle.CloseTime < candle.OpenTime {
		return fmt.Errorf("close time is before open time")
	}
	if candle.High < candle.Low {
		return fmt.Errorf("high is below low")
	}
	for name, value := range map[string]float64{
		"open":  candle.Open,
		"close": candle.Close,
	} {
		if value < candle.Low || value > candle.High {
			return fmt.Errorf("%s is outside high/low range", name)
		}
	}
	if candle.Volume < 0 {
		return fmt.Errorf("volume is negative")
	}
	return nil
}

func clamp(value int, minValue int, maxValue int) int {
	return min(max(value, minValue), maxValue)
}
