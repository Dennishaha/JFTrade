package pineworker

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestNormalizeRuntimeMigratesLegacyRuntime(t *testing.T) {
	for _, input := range []string{"", "pine-go-plan", " Pine-Go-Plan ", "pine-pinets"} {
		if got := NormalizeRuntime(input); got != RuntimeID {
			t.Fatalf("NormalizeRuntime(%q) = %q, want %q", input, got, RuntimeID)
		}
	}
	if SupportsRuntime("custom") {
		t.Fatal("SupportsRuntime(custom) = true, want false")
	}
}

func TestDefaultWorkerConfigScalesByCPU(t *testing.T) {
	one := DefaultWorkerConfig(1)
	if one.LiveWorkers != 2 || one.BacktestWorkers != 1 || one.OptimizationWorkers != 1 {
		t.Fatalf("DefaultWorkerConfig(1) = %#v", one)
	}
	eight := DefaultWorkerConfig(8)
	if eight.LiveWorkers != 4 || eight.BacktestWorkers != 4 || eight.OptimizationWorkers != 8 {
		t.Fatalf("DefaultWorkerConfig(8) = %#v", eight)
	}
	if eight.RequestTimeout <= 0 || eight.HealthCheckInterval <= 0 {
		t.Fatalf("default config missing positive operational limits: %#v", eight)
	}
	if eight.MaxMessageBytes != 0 {
		t.Fatalf("default MaxMessageBytes = %d, want unlimited", eight.MaxMessageBytes)
	}
	if eight.MaxCandlesPerRequest != 0 {
		t.Fatalf("default MaxCandlesPerRequest = %d, want unlimited", eight.MaxCandlesPerRequest)
	}
}

func TestValidateRunScriptRequest(t *testing.T) {
	valid := RunScriptRequest{
		JobID:     "job-1",
		Source:    `//@version=6 strategy("x")`,
		Symbol:    "US.AAPL",
		Timeframe: "1",
		Mode:      ModeBacktest,
		Candles: []Candle{{
			OpenTime:  1_700_000_000_000,
			CloseTime: 1_700_000_060_000,
			Open:      10,
			High:      12,
			Low:       9,
			Close:     11,
			Volume:    100,
		}},
	}
	if err := ValidateRunScriptRequest(valid, DefaultWorkerConfig(4)); err != nil {
		t.Fatalf("ValidateRunScriptRequest(valid) error = %v", err)
	}

	tests := []struct {
		name    string
		mutate  func(*RunScriptRequest)
		wantErr string
	}{
		{"missing job", func(r *RunScriptRequest) { r.JobID = "" }, "job id is required"},
		{"missing source", func(r *RunScriptRequest) { r.Source = "" }, "source is required"},
		{"missing symbol", func(r *RunScriptRequest) { r.Symbol = "" }, "symbol is required"},
		{"missing timeframe", func(r *RunScriptRequest) { r.Timeframe = "" }, "timeframe is required"},
		{"unsupported mode", func(r *RunScriptRequest) { r.Mode = "scan" }, "unsupported pine worker mode"},
		{"missing candles", func(r *RunScriptRequest) { r.Candles = nil }, "candles are required"},
		{"bad range", func(r *RunScriptRequest) { r.Candles[0].High = 8 }, "high is below low"},
		{"open outside range", func(r *RunScriptRequest) { r.Candles[0].Open = 99 }, "open is outside"},
		{"negative volume", func(r *RunScriptRequest) { r.Candles[0].Volume = -1 }, "volume is negative"},
		{"non-finite high", func(r *RunScriptRequest) { r.Candles[0].High = math.Inf(1) }, "unsupported value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := valid
			request.Candles = append([]Candle(nil), valid.Candles...)
			tt.mutate(&request)
			err := ValidateRunScriptRequest(request, DefaultWorkerConfig(4))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateRunScriptRequestAnalyzeModeAllowsNoCandles(t *testing.T) {
	request := RunScriptRequest{
		JobID:     "analyze-1",
		Source:    `//@version=6 indicator("x")`,
		Symbol:    "US.AAPL",
		Timeframe: "1",
		Mode:      ModeAnalyze,
	}
	if err := ValidateRunScriptRequest(request, DefaultWorkerConfig(4)); err != nil {
		t.Fatalf("ValidateRunScriptRequest(analyze) error = %v", err)
	}
}

func TestValidateRunScriptRequestLiveSessionContract(t *testing.T) {
	base := RunScriptRequest{
		JobID: "live-1", Source: `//@version=6 strategy("x")`, Symbol: "US.AAPL", Timeframe: "1", Mode: ModeLive,
		Candles:   []Candle{{OpenTime: 1, CloseTime: 2, Open: 1, High: 1, Low: 1, Close: 1}},
		SessionID: "session-1", SessionOperation: SessionOperationOpen,
	}
	if err := ValidateRunScriptRequest(base, DefaultWorkerConfig(1)); err != nil {
		t.Fatalf("open validation: %v", err)
	}
	appendRequest := base
	appendRequest.SessionOperation = SessionOperationAppend
	appendRequest.ExpectedRevision = 1
	if err := ValidateRunScriptRequest(appendRequest, DefaultWorkerConfig(1)); err != nil {
		t.Fatalf("append validation: %v", err)
	}
	closeRequest := RunScriptRequest{
		JobID: "close-1", Mode: ModeLive, SessionID: "session-1", SessionOperation: SessionOperationClose, ExpectedRevision: 2,
	}
	if err := ValidateRunScriptRequest(closeRequest, DefaultWorkerConfig(1)); err != nil {
		t.Fatalf("close validation: %v", err)
	}

	tests := []struct {
		name    string
		request RunScriptRequest
		want    string
	}{
		{name: "unsupported session operation", request: func() RunScriptRequest { next := base; next.SessionOperation = "replace"; return next }(), want: "unsupported pine worker session operation"},
		{name: "session requires live mode", request: func() RunScriptRequest { next := base; next.Mode = ModeBacktest; return next }(), want: "require live mode"},
		{name: "open starts at zero", request: func() RunScriptRequest { next := base; next.ExpectedRevision = 1; return next }(), want: "expected revision 0"},
		{name: "append requires revision", request: func() RunScriptRequest { next := base; next.SessionOperation = SessionOperationAppend; return next }(), want: "positive expected revision"},
		{name: "operation requires id", request: func() RunScriptRequest { next := base; next.SessionID = ""; return next }(), want: "session id is required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateRunScriptRequest(test.request, DefaultWorkerConfig(1)); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validation error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateRunScriptRequestRejectsTooManyCandles(t *testing.T) {
	request := RunScriptRequest{
		JobID:     "job-1",
		Source:    `//@version=6 strategy("x")`,
		Symbol:    "US.AAPL",
		Timeframe: "1",
		Candles: []Candle{
			{OpenTime: 1, Open: 1, High: 1, Low: 1, Close: 1},
			{OpenTime: 2, Open: 1, High: 1, Low: 1, Close: 1},
		},
	}
	err := ValidateRunScriptRequest(request, WorkerConfig{MaxCandlesPerRequest: 1})
	if err == nil || !strings.Contains(err.Error(), "too many candles") {
		t.Fatalf("error = %v, want too many candles", err)
	}
}

func TestRunScriptPayloadSizeRejectsNonFiniteCandle(t *testing.T) {
	_, err := jsonSize(RunScriptRequest{Candles: []Candle{{Open: math.NaN()}}})
	if err == nil || !strings.Contains(err.Error(), "unsupported value") {
		t.Fatalf("jsonSize error = %v, want unsupported value", err)
	}
}

func TestCheckPerformanceGate(t *testing.T) {
	gate := PerformanceGate{
		MaxDuration:       time.Second,
		MaxDurationPerBar: time.Millisecond,
		MinCandlesPerSec:  100,
		MaxRequestBytes:   1000,
		MaxResponseBytes:  1000,
		MaxPeakRSSBytes:   1000,
	}
	sample := PerformanceSample{
		Candles:       1000,
		Duration:      100 * time.Millisecond,
		RequestBytes:  500,
		ResponseBytes: 400,
		PeakRSSBytes:  900,
	}
	if err := CheckPerformanceGate(sample, gate); err != nil {
		t.Fatalf("CheckPerformanceGate(valid) error = %v", err)
	}

	tests := []struct {
		name    string
		mutate  func(*PerformanceSample)
		wantErr string
	}{
		{"zero candles", func(s *PerformanceSample) { s.Candles = 0 }, "candles must be positive"},
		{"slow duration", func(s *PerformanceSample) { s.Duration = 2 * time.Second }, "duration"},
		{"large request", func(s *PerformanceSample) { s.RequestBytes = 1001 }, "request bytes"},
		{"large response", func(s *PerformanceSample) { s.ResponseBytes = 1001 }, "response bytes"},
		{"large rss", func(s *PerformanceSample) { s.PeakRSSBytes = 1001 }, "peak RSS"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next := sample
			tt.mutate(&next)
			err := CheckPerformanceGate(next, gate)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func BenchmarkCheckPerformanceGate(b *testing.B) {
	gate := DefaultPerformanceGate()
	sample := PerformanceSample{
		Candles:       10_000,
		Duration:      200 * time.Millisecond,
		RequestBytes:  1024 * 1024,
		ResponseBytes: 256 * 1024,
		PeakRSSBytes:  128 * 1024 * 1024,
	}
	for range b.N {
		if err := CheckPerformanceGate(sample, gate); err != nil {
			b.Fatal(err)
		}
	}
}
