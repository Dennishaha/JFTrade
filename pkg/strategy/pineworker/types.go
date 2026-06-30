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

type Candle struct {
	OpenTime  int64
	CloseTime int64
	Open      float64
	High      float64
	Low       float64
	Close     float64
	Volume    float64
}

type RunScriptRequest struct {
	JobID     string
	ScriptID  string
	Source    string
	Symbol    string
	Timeframe string
	Mode      string
	Candles   []Candle
	Params    map[string]string
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
}

func ValidateRunScriptRequest(request RunScriptRequest, config WorkerConfig) error {
	_, err := validateAndMeasureRunScriptRequest(request, config)
	return err
}

func validateRunScriptRequestBasics(request RunScriptRequest, config WorkerConfig) error {
	if strings.TrimSpace(request.JobID) == "" {
		return fmt.Errorf("job id is required")
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
	if mode := normalizeMode(request.Mode); mode == "" {
		return fmt.Errorf("unsupported pine worker mode: %s", request.Mode)
	}
	if len(request.Candles) == 0 && normalizeMode(request.Mode) != ModeAnalyze {
		return fmt.Errorf("candles are required")
	}
	if config.MaxCandlesPerRequest > 0 && len(request.Candles) > config.MaxCandlesPerRequest {
		return fmt.Errorf("too many candles: %d > %d", len(request.Candles), config.MaxCandlesPerRequest)
	}
	return nil
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
