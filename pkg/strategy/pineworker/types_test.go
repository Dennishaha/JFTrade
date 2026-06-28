package pineworker

import (
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
	if eight.RequestTimeout <= 0 || eight.HealthCheckInterval <= 0 || eight.MaxMessageBytes <= 0 {
		t.Fatalf("default config missing positive operational limits: %#v", eight)
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
