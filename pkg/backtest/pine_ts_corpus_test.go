package backtest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/types"

	strategypine "github.com/jftrade/jftrade-main/pkg/strategy/pine"
	"github.com/jftrade/jftrade-main/pkg/strategy/pineengine"
	"github.com/jftrade/jftrade-main/pkg/strategy/pinespec"
)

const (
	pinetsShadowDefaultMaxCases = 40
	pinetsShadowParityThreshold = 1e-6
)

func TestPinetsShadowCorpusReport(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skipf("node unavailable: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	dataSet := loadPinetsShadowDataSet(t)
	cases := limitPinetsShadowCases(pinetsShadowCorpusCases(), pinetsShadowMaxCases())
	client := pineengine.NewPinetsWorkerClient("", "")
	defer func() { _ = client.Close() }()

	info, err := client.EngineInfo(ctx)
	if err != nil {
		t.Fatalf("pinets engineInfo: %v", err)
	}
	if info.Engine != pineengine.PinetsShadowEngineID {
		t.Fatalf("pinets engine = %s, want %s", info.Engine, pineengine.PinetsShadowEngineID)
	}
	if info.License != "AGPL-3.0-only" {
		t.Fatalf("pinets license = %s, want AGPL-3.0-only", info.License)
	}

	report := pinetsShadowCorpusReport{
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339Nano),
		Engine:         info,
		ConfiguredMode: pineengine.ExternalModeFromEnv(),
		RunMode:        pinetsShadowReportRunMode(),
		DataSource:     dataSet.Source,
		Cases:          make([]pinetsShadowCorpusCaseResult, 0, len(cases)),
	}
	for _, corpusCase := range cases {
		result := runPinetsShadowCorpusCase(ctx, client, corpusCase, dataSet)
		report.Cases = append(report.Cases, result)
		report.Summary.add(result)
	}

	path := pinetsShadowReportPath(t)
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("marshal pinets shadow report: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create pinets shadow report dir: %v", err)
	}
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		t.Fatalf("write pinets shadow report: %v", err)
	}
	assertPinetsShadowReportFile(t, path, report.Summary.Total)
	t.Logf("pinets shadow corpus report: %s", path)
	t.Logf("pinets shadow summary: total=%d pinetsOK=%d pinetsFailed=%d unsupported=%d plotComparable=%d plotMismatched=%d",
		report.Summary.Total,
		report.Summary.PinetsOK,
		report.Summary.PinetsFailed,
		report.Summary.Unsupported,
		report.Summary.PlotComparable,
		report.Summary.PlotMismatched,
	)
}

type pinetsShadowDataSet struct {
	Source  pinetsShadowDataSource
	KLines  []types.KLine
	Candles []pineengine.Candle
}

type pinetsShadowDataSource struct {
	Kind       string `json:"kind"`
	DBPath     string `json:"dbPath"`
	Symbol     string `json:"symbol"`
	Timeframe  string `json:"timeframe"`
	StartTime  string `json:"startTime"`
	EndTime    string `json:"endTime"`
	CandleRows int    `json:"candleRows"`
	Limit      int    `json:"limit"`
}

type pinetsShadowCorpusCase struct {
	ID          string
	Suite       string
	Title       string
	Script      string
	WarmupBars  int
	MTF         bool
	Aggregation string
	Expected    map[string][]float64
}

type pinetsShadowCorpusReport struct {
	GeneratedAt    string                         `json:"generatedAt"`
	Engine         pineengine.EngineInfo          `json:"engine"`
	ConfiguredMode string                         `json:"configuredMode"`
	RunMode        string                         `json:"runMode"`
	DataSource     pinetsShadowDataSource         `json:"dataSource"`
	Summary        pinetsShadowCorpusSummary      `json:"summary"`
	Cases          []pinetsShadowCorpusCaseResult `json:"cases"`
}

type pinetsShadowCorpusSummary struct {
	Total             int `json:"total"`
	GoCompileOK       int `json:"goCompileOk"`
	GoCompileFailed   int `json:"goCompileFailed"`
	GoBacktestSkipped int `json:"goBacktestSkipped"`
	PinetsOK          int `json:"pinetsOk"`
	PinetsFailed      int `json:"pinetsFailed"`
	Unsupported       int `json:"unsupported"`
	PlotComparable    int `json:"plotComparable"`
	PlotMismatched    int `json:"plotMismatched"`
	PlotNotComparable int `json:"plotNotComparable"`
}

type pinetsShadowCorpusCaseResult struct {
	ID                string                             `json:"id"`
	Suite             string                             `json:"suite"`
	Title             string                             `json:"title"`
	ScriptHash        string                             `json:"scriptHash"`
	Symbol            string                             `json:"symbol"`
	Timeframe         string                             `json:"timeframe"`
	CandleCount       int                                `json:"candleCount"`
	WarmupBars        int                                `json:"warmupBars"`
	GoCompile         bool                               `json:"goCompile"`
	GoCompileError    string                             `json:"goCompileError,omitempty"`
	GoBacktest        pinetsShadowGoBacktestResult       `json:"goBacktest"`
	PinetsRun         bool                               `json:"pinetsRun"`
	PinetsError       string                             `json:"pinetsError,omitempty"`
	UnsupportedReason string                             `json:"unsupportedReason,omitempty"`
	Diagnostics       []pineengine.Diagnostic            `json:"diagnostics"`
	Plots             map[string]pinetsShadowPlotSummary `json:"plots"`
	Signals           map[string]any                     `json:"signals"`
	PlotParity        pinetsShadowPlotParity             `json:"plotParity"`
	Metadata          map[string]any                     `json:"metadata"`
	RuntimeMS         int                                `json:"runtimeMs"`
	LicenseMode       string                             `json:"licenseMode"`
	MTF               bool                               `json:"mtf"`
	Aggregation       string                             `json:"aggregation,omitempty"`
}

type pinetsShadowGoBacktestResult struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

type pinetsShadowPlotSummary struct {
	Title  string `json:"title"`
	Points int    `json:"points"`
	Last   any    `json:"last"`
}

type pinetsShadowPlotParity struct {
	Status           string   `json:"status"`
	Threshold        float64  `json:"threshold"`
	WarmupBars       int      `json:"warmupBars"`
	ComparedPoints   int      `json:"comparedPoints"`
	MaxAbsoluteError float64  `json:"maxAbsoluteError,omitempty"`
	MaxRelativeError float64  `json:"maxRelativeError,omitempty"`
	MissingPlots     []string `json:"missingPlots,omitempty"`
	Reason           string   `json:"reason,omitempty"`
}

func (summary *pinetsShadowCorpusSummary) add(result pinetsShadowCorpusCaseResult) {
	summary.Total++
	if result.GoCompile {
		summary.GoCompileOK++
	} else {
		summary.GoCompileFailed++
	}
	if result.GoBacktest.Status == "not_available" {
		summary.GoBacktestSkipped++
	}
	if result.PinetsRun {
		summary.PinetsOK++
	} else {
		summary.PinetsFailed++
		if result.UnsupportedReason != "" {
			summary.Unsupported++
		}
	}
	switch result.PlotParity.Status {
	case "matched":
		summary.PlotComparable++
	case "mismatched":
		summary.PlotComparable++
		summary.PlotMismatched++
	default:
		summary.PlotNotComparable++
	}
}

func loadPinetsShadowDataSet(t *testing.T) pinetsShadowDataSet {
	t.Helper()
	dbPath := strings.TrimSpace(os.Getenv("JFTRADE_PINETS_REPORT_DB"))
	symbol := pinetsShadowEnvDefault("JFTRADE_PINETS_REPORT_SYMBOL", "US.AAPL")
	interval := types.Interval(pinetsShadowEnvDefault("JFTRADE_PINETS_REPORT_TIMEFRAME", string(types.Interval1m)))
	limit := pinetsShadowEnvInt("JFTRADE_PINETS_REPORT_LIMIT", 512)
	kind := "fixture-backtest-store"
	var since time.Time
	var until time.Time

	if dbPath == "" {
		dbPath = filepath.Join(t.TempDir(), "pinets-shadow-backtest.db")
		store, err := NewFutuKLineStore(dbPath)
		if err != nil {
			t.Fatalf("NewFutuKLineStore() error = %v", err)
		}
		baseStart := time.Date(2026, time.May, 26, 9, 30, 0, 0, time.UTC)
		klines := buildBenchmarkKLines(baseStart, 2048)
		if err := store.InsertKLines(klines, "forward"); err != nil {
			jftradeCheckTestError(t, store.Close())
			t.Fatalf("InsertKLines() error = %v", err)
		}
		if err := store.Close(); err != nil {
			t.Fatalf("store.Close() error = %v", err)
		}
		startIndex := 512
		since = klines[startIndex].StartTime.Time()
		until = klines[min(len(klines)-1, startIndex+limit-1)].EndTime.Time()
	} else {
		kind = "operator-backtest-store"
		until = pinetsShadowEnvTime("JFTRADE_PINETS_REPORT_UNTIL", time.Now().UTC())
	}

	store, err := NewFutuKLineStore(dbPath)
	if err != nil {
		t.Fatalf("open pinets shadow backtest store: %v", err)
	}
	defer func() { jftradeCheckTestError(t, store.Close()) }()
	store.SetRehabType("forward")
	if since.IsZero() {
		rows, err := store.QueryKLinesBackward(nil, symbol, interval, until, limit)
		if err != nil {
			t.Fatalf("query pinets shadow backtest store: %v", err)
		}
		if len(rows) == 0 {
			t.Fatalf("query pinets shadow backtest store returned no rows for %s %s before %s", symbol, interval, until.Format(time.RFC3339Nano))
		}
		sort.Slice(rows, func(i, j int) bool {
			return rows[i].StartTime.Time().Before(rows[j].StartTime.Time())
		})
		since = rows[0].StartTime.Time()
		until = rows[len(rows)-1].EndTime.Time()
	}

	klines := make([]types.KLine, 0, limit)
	if err := store.StreamKLines(since, until, nil, []string{symbol}, []types.Interval{interval}, func(kline types.KLine) {
		if kline.Symbol == symbol && kline.Interval == interval {
			klines = append(klines, kline)
		}
	}); err != nil {
		t.Fatalf("stream pinets shadow K lines: %v", err)
	}
	if len(klines) == 0 {
		t.Fatalf("stream pinets shadow K lines returned no rows for %s %s %s..%s", symbol, interval, since.Format(time.RFC3339Nano), until.Format(time.RFC3339Nano))
	}
	candles := CandlesFromKLines(klines)
	return pinetsShadowDataSet{
		Source: pinetsShadowDataSource{
			Kind:       kind,
			DBPath:     dbPath,
			Symbol:     symbol,
			Timeframe:  string(interval),
			StartTime:  klines[0].StartTime.Time().UTC().Format(time.RFC3339Nano),
			EndTime:    klines[len(klines)-1].EndTime.Time().UTC().Format(time.RFC3339Nano),
			CandleRows: len(klines),
			Limit:      limit,
		},
		KLines:  klines,
		Candles: candles,
	}
}

func pinetsShadowCorpusCases() []pinetsShadowCorpusCase {
	cases := make([]pinetsShadowCorpusCase, 0)
	for _, example := range pinespec.Examples() {
		cases = append(cases, pinetsShadowCorpusCase{
			ID:     example.ID,
			Suite:  "strategy_corpus",
			Title:  example.Title,
			Script: example.Script,
		})
	}
	for _, example := range pinespec.GoldenExamples() {
		cases = append(cases, pinetsShadowCorpusCase{
			ID:     example.ID,
			Suite:  "strategy_corpus",
			Title:  example.Title,
			Script: example.Script,
			MTF:    strings.Contains(example.Script, "request.security"),
		})
	}
	cases = append(cases, pinetsShadowIndicatorProbeCases()...)
	return cases
}

func pinetsShadowIndicatorProbeCases() []pinetsShadowCorpusCase {
	return []pinetsShadowCorpusCase{
		{
			ID:         "indicator-probe-sma",
			Suite:      "indicator_probe_corpus",
			Title:      "SMA probe",
			WarmupBars: 8,
			Script: `//@version=6
indicator("JFTrade SMA Probe")
plot(ta.sma(close, 8), "sma8")`,
		},
		{
			ID:         "indicator-probe-ema",
			Suite:      "indicator_probe_corpus",
			Title:      "EMA probe",
			WarmupBars: 8,
			Script: `//@version=6
indicator("JFTrade EMA Probe")
plot(ta.ema(close, 8), "ema8")`,
		},
		{
			ID:         "indicator-probe-rsi",
			Suite:      "indicator_probe_corpus",
			Title:      "RSI probe",
			WarmupBars: 14,
			Script: `//@version=6
indicator("JFTrade RSI Probe")
plot(ta.rsi(close, 14), "rsi14")`,
		},
		{
			ID:         "indicator-probe-macd",
			Suite:      "indicator_probe_corpus",
			Title:      "MACD probe",
			WarmupBars: 35,
			Script: `//@version=6
indicator("JFTrade MACD Probe")
[macdLine, signalLine, histLine] = ta.macd(close, 12, 26, 9)
plot(macdLine, "macd")
plot(signalLine, "signal")
plot(histLine, "hist")`,
		},
		{
			ID:         "indicator-probe-bollinger",
			Suite:      "indicator_probe_corpus",
			Title:      "Bollinger probe",
			WarmupBars: 20,
			Script: `//@version=6
indicator("JFTrade Bollinger Probe")
[basis, upper, lower] = ta.bb(close, 20, 2)
plot(basis, "basis")
plot(upper, "upper")
plot(lower, "lower")`,
		},
		{
			ID:          "indicator-probe-request-security",
			Suite:       "indicator_probe_corpus",
			Title:       "request.security probe",
			WarmupBars:  20,
			MTF:         true,
			Aggregation: "same-symbol static 15m request.security over 1m report candles",
			Script: `//@version=6
indicator("JFTrade request.security Probe")
mtfClose = request.security("US.AAPL", "15", close)
plot(mtfClose, "mtfClose")`,
		},
	}
}

func limitPinetsShadowCases(cases []pinetsShadowCorpusCase, maxCases int) []pinetsShadowCorpusCase {
	if maxCases <= 0 || maxCases >= len(cases) {
		return cases
	}
	return cases[:maxCases]
}

func runPinetsShadowCorpusCase(
	ctx context.Context,
	client *pineengine.PinetsWorkerClient,
	corpusCase pinetsShadowCorpusCase,
	dataSet pinetsShadowDataSet,
) pinetsShadowCorpusCaseResult {
	result := pinetsShadowCorpusCaseResult{
		ID:          corpusCase.ID,
		Suite:       corpusCase.Suite,
		Title:       corpusCase.Title,
		ScriptHash:  pinetsShadowScriptHash(corpusCase.Script),
		Symbol:      dataSet.Source.Symbol,
		Timeframe:   dataSet.Source.Timeframe,
		CandleCount: len(dataSet.Candles),
		WarmupBars:  corpusCase.WarmupBars,
		GoBacktest: pinetsShadowGoBacktestResult{
			Status: "not_available",
			Reason: "direct Go Pine backtest runner has been removed in the current pine-pinets hard-cut path",
		},
		LicenseMode: pineengine.ExternalModeFromEnv(),
		MTF:         corpusCase.MTF,
		Aggregation: corpusCase.Aggregation,
		PlotParity: pinetsShadowPlotParity{
			Status:     "not_comparable",
			Threshold:  pinetsShadowParityThreshold,
			WarmupBars: corpusCase.WarmupBars,
			Reason:     "no comparable Go-side plot series is available for this corpus case",
		},
		Diagnostics: []pineengine.Diagnostic{},
		Plots:       map[string]pinetsShadowPlotSummary{},
		Signals:     map[string]any{},
		Metadata:    map[string]any{},
	}
	if _, err := strategypine.Compile(corpusCase.Script); err != nil {
		result.GoCompileError = err.Error()
	} else {
		result.GoCompile = true
	}
	if !pinetsShadowCaseRunnable(corpusCase) {
		result.UnsupportedReason = "strategy corpus is compile-only until the stdio shadow harness supports strategy declarations and order APIs"
		return result
	}

	expected := pinetsShadowExpectedPlots(corpusCase.ID, dataSet.Candles)
	caseCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	response, err := client.RunIndicator(caseCtx, pineengine.RunIndicatorRequest{
		Script:     corpusCase.Script,
		Symbol:     dataSet.Source.Symbol,
		Timeframe:  dataSet.Source.Timeframe,
		Candles:    dataSet.Candles,
		WarmupBars: corpusCase.WarmupBars,
		Mode:       pinetsShadowReportRunMode(),
		TimeoutMS:  10_000,
	})
	if err != nil {
		result.PinetsError = err.Error()
		result.UnsupportedReason = err.Error()
		if errors.Is(caseCtx.Err(), context.DeadlineExceeded) {
			result.UnsupportedReason = "pinets shadow corpus context deadline exceeded"
		}
		return result
	}
	result.PinetsRun = response.OK
	result.Diagnostics = response.Diagnostics
	result.Plots = summarizePinetsShadowPlots(response.Plots)
	result.Signals = response.Signals
	result.Metadata = response.Metadata
	result.RuntimeMS = response.RuntimeMS
	if !response.OK {
		result.UnsupportedReason = "pinets worker returned ok=false"
		return result
	}
	if corpusCase.MTF && len(expected) == 0 {
		result.PlotParity = pinetsShadowPlotParity{
			Status:     "not_comparable",
			Threshold:  pinetsShadowParityThreshold,
			WarmupBars: corpusCase.WarmupBars,
			Reason:     "MTF aggregation recorded for report-only analysis",
		}
		return result
	}
	result.PlotParity = comparePinetsShadowPlots(response.Plots, expected, corpusCase.WarmupBars)
	return result
}

func pinetsShadowCaseRunnable(corpusCase pinetsShadowCorpusCase) bool {
	return corpusCase.Suite == "indicator_probe_corpus"
}

func assertPinetsShadowReportFile(t *testing.T, path string, wantCases int) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read pinets shadow report: %v", err)
	}
	var report pinetsShadowCorpusReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("decode pinets shadow report: %v", err)
	}
	if report.Engine.Engine != pineengine.PinetsShadowEngineID || report.Engine.License != "AGPL-3.0-only" {
		t.Fatalf("report engine metadata = %#v", report.Engine)
	}
	if report.DataSource.Symbol == "" || report.DataSource.Timeframe == "" || report.DataSource.CandleRows == 0 {
		t.Fatalf("report dataSource = %#v, want symbol/timeframe/candleRows", report.DataSource)
	}
	if report.Summary.Total != wantCases || len(report.Cases) != wantCases {
		t.Fatalf("report cases total=%d len=%d, want %d", report.Summary.Total, len(report.Cases), wantCases)
	}
	for _, item := range report.Cases {
		if item.ID == "" || item.Suite == "" || item.ScriptHash == "" {
			t.Fatalf("report case has unstable identity fields: %#v", item)
		}
		if item.Diagnostics == nil || item.Plots == nil || item.Signals == nil || item.Metadata == nil {
			t.Fatalf("report case %s has nil collection fields", item.ID)
		}
	}
}

func summarizePinetsShadowPlots(plots map[string]pineengine.Plot) map[string]pinetsShadowPlotSummary {
	out := make(map[string]pinetsShadowPlotSummary, len(plots))
	for name, plot := range plots {
		var last any
		if len(plot.Data) > 0 {
			last = plot.Data[len(plot.Data)-1]
		}
		out[name] = pinetsShadowPlotSummary{
			Title:  plot.Title,
			Points: len(plot.Data),
			Last:   last,
		}
	}
	return out
}

func comparePinetsShadowPlots(
	actual map[string]pineengine.Plot,
	expected map[string][]float64,
	warmupBars int,
) pinetsShadowPlotParity {
	parity := pinetsShadowPlotParity{
		Status:     "not_comparable",
		Threshold:  pinetsShadowParityThreshold,
		WarmupBars: warmupBars,
	}
	if len(expected) == 0 {
		parity.Reason = "no expected plot series for this case"
		return parity
	}
	parity.Status = "matched"
	for name, expectedSeries := range expected {
		plot, ok := actual[name]
		if !ok {
			parity.MissingPlots = append(parity.MissingPlots, name)
			continue
		}
		for index := max(0, warmupBars); index < len(expectedSeries) && index < len(plot.Data); index++ {
			expectedValue := expectedSeries[index]
			if math.IsNaN(expectedValue) {
				continue
			}
			actualValue, ok := pinetsShadowNumericValue(plot.Data[index])
			if !ok {
				continue
			}
			absErr := math.Abs(actualValue - expectedValue)
			relErr := absErr
			if math.Abs(expectedValue) > 1e-12 {
				relErr = absErr / math.Abs(expectedValue)
			}
			parity.ComparedPoints++
			parity.MaxAbsoluteError = math.Max(parity.MaxAbsoluteError, absErr)
			parity.MaxRelativeError = math.Max(parity.MaxRelativeError, relErr)
		}
	}
	if len(parity.MissingPlots) > 0 {
		sort.Strings(parity.MissingPlots)
		parity.Status = "not_comparable"
		parity.Reason = "one or more expected plot names were not returned by Pinets"
		return parity
	}
	if parity.ComparedPoints == 0 {
		parity.Status = "not_comparable"
		parity.Reason = "no numeric plot points were comparable after warmup"
		return parity
	}
	if parity.MaxRelativeError > pinetsShadowParityThreshold && parity.MaxAbsoluteError > pinetsShadowParityThreshold {
		parity.Status = "mismatched"
	}
	return parity
}

func pinetsShadowExpectedPlots(caseID string, candles []pineengine.Candle) map[string][]float64 {
	closes := make([]float64, len(candles))
	for index, candle := range candles {
		closes[index] = candle.Close
	}
	switch caseID {
	case "indicator-probe-sma":
		return map[string][]float64{"sma8": pinetsShadowSMA(closes, 8)}
	case "indicator-probe-ema":
		return map[string][]float64{"ema8": pinetsShadowEMA(closes, 8)}
	case "indicator-probe-rsi":
		return map[string][]float64{"rsi14": pinetsShadowRSI(closes, 14)}
	case "indicator-probe-macd":
		macdLine, signalLine, histLine := pinetsShadowMACD(closes, 12, 26, 9)
		return map[string][]float64{"macd": macdLine, "signal": signalLine, "hist": histLine}
	case "indicator-probe-bollinger":
		basis, upper, lower := pinetsShadowBollinger(closes, 20, 2)
		return map[string][]float64{"basis": basis, "upper": upper, "lower": lower}
	default:
		return nil
	}
}

func pinetsShadowSMA(values []float64, length int) []float64 {
	out := pinetsShadowNaNSeries(len(values))
	var sum float64
	for index, value := range values {
		sum += value
		if index >= length {
			sum -= values[index-length]
		}
		if index >= length-1 {
			out[index] = sum / float64(length)
		}
	}
	return out
}

func pinetsShadowEMA(values []float64, length int) []float64 {
	out := pinetsShadowNaNSeries(len(values))
	if len(values) == 0 {
		return out
	}
	alpha := 2.0 / float64(length+1)
	out[0] = values[0]
	for index := 1; index < len(values); index++ {
		out[index] = alpha*values[index] + (1-alpha)*out[index-1]
	}
	return out
}

func pinetsShadowRSI(values []float64, length int) []float64 {
	out := pinetsShadowNaNSeries(len(values))
	if len(values) <= length {
		return out
	}
	var gainSum float64
	var lossSum float64
	for index := 1; index <= length; index++ {
		change := values[index] - values[index-1]
		if change >= 0 {
			gainSum += change
		} else {
			lossSum -= change
		}
	}
	avgGain := gainSum / float64(length)
	avgLoss := lossSum / float64(length)
	out[length] = pinetsShadowRSIValue(avgGain, avgLoss)
	for index := length + 1; index < len(values); index++ {
		change := values[index] - values[index-1]
		gain := 0.0
		loss := 0.0
		if change >= 0 {
			gain = change
		} else {
			loss = -change
		}
		avgGain = (avgGain*float64(length-1) + gain) / float64(length)
		avgLoss = (avgLoss*float64(length-1) + loss) / float64(length)
		out[index] = pinetsShadowRSIValue(avgGain, avgLoss)
	}
	return out
}

func pinetsShadowRSIValue(avgGain float64, avgLoss float64) float64 {
	if avgLoss == 0 {
		return 100
	}
	rs := avgGain / avgLoss
	return 100 - 100/(1+rs)
}

func pinetsShadowMACD(values []float64, fast int, slow int, signal int) ([]float64, []float64, []float64) {
	fastEMA := pinetsShadowEMA(values, fast)
	slowEMA := pinetsShadowEMA(values, slow)
	macd := pinetsShadowNaNSeries(len(values))
	for index := range values {
		if !math.IsNaN(fastEMA[index]) && !math.IsNaN(slowEMA[index]) {
			macd[index] = fastEMA[index] - slowEMA[index]
		}
	}
	signalLine := pinetsShadowEMA(macd, signal)
	hist := pinetsShadowNaNSeries(len(values))
	for index := range values {
		if !math.IsNaN(macd[index]) && !math.IsNaN(signalLine[index]) {
			hist[index] = macd[index] - signalLine[index]
		}
	}
	return macd, signalLine, hist
}

func pinetsShadowBollinger(values []float64, length int, mult float64) ([]float64, []float64, []float64) {
	basis := pinetsShadowSMA(values, length)
	upper := pinetsShadowNaNSeries(len(values))
	lower := pinetsShadowNaNSeries(len(values))
	for index := length - 1; index < len(values); index++ {
		mean := basis[index]
		var variance float64
		for cursor := index - length + 1; cursor <= index; cursor++ {
			delta := values[cursor] - mean
			variance += delta * delta
		}
		stdev := math.Sqrt(variance / float64(length))
		upper[index] = mean + mult*stdev
		lower[index] = mean - mult*stdev
	}
	return basis, upper, lower
}

func pinetsShadowNaNSeries(length int) []float64 {
	out := make([]float64, length)
	for index := range out {
		out[index] = math.NaN()
	}
	return out
}

func pinetsShadowNumericValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, !math.IsNaN(typed)
	case int:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

func pinetsShadowMaxCases() int {
	return pinetsShadowEnvInt("JFTRADE_PINETS_SHADOW_MAX_CASES", pinetsShadowDefaultMaxCases)
}

func pinetsShadowReportRunMode() string {
	mode := pineengine.ExternalModeFromEnv()
	if mode == pineengine.ModeOff {
		return pineengine.ModeShadow
	}
	return mode
}

func pinetsShadowReportPath(t *testing.T) string {
	t.Helper()
	if path := strings.TrimSpace(os.Getenv("JFTRADE_PINETS_SHADOW_REPORT_PATH")); path != "" {
		return path
	}
	return filepath.Join(t.TempDir(), "pinets_shadow_report.json")
}

func pinetsShadowEnvDefault(key string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func pinetsShadowEnvInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func pinetsShadowEnvTime(key string, fallback time.Time) time.Time {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return fallback
	}
	return parsed.UTC()
}

func pinetsShadowScriptHash(script string) string {
	sum := sha256.Sum256([]byte(script))
	return hex.EncodeToString(sum[:8])
}
