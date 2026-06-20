package backtest

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

const (
	strategyBlockBenchmarkBaselinePath             = "testdata/strategy_block_benchmark_baseline.json"
	strategyBlockBenchmarkMaxNSOpRegressionRatio   = 1.40
	strategyBlockBenchmarkMaxBytesRegressionRatio  = 1.15
	strategyBlockBenchmarkMaxAllocsRegressionRatio = 1.10
	strategyBlockBenchmarkBaselineVersion          = 1
	strategyBlockBenchmarkSampleCount              = 5
	strategyBlockBenchmarkEnforceBaselineEnv       = "JFTRADE_ENFORCE_STRATEGY_BLOCK_BASELINE"
	strategyBlockBenchmarkUpdateBaselineEnv        = "JFTRADE_UPDATE_STRATEGY_BLOCK_BASELINE"
	strategyBlockBenchmarkAuditCommitEnv           = "JFTRADE_BENCHMARK_AUDIT_COMMIT"
	strategyBlockBenchmarkAuditHardwareEnv         = "JFTRADE_BENCHMARK_AUDIT_HARDWARE"
)

type strategyBlockBenchmarkBaseline struct {
	Version             int                                      `json:"version"`
	UpdatedAt           string                                   `json:"updatedAt"`
	WorkloadFingerprint string                                   `json:"workloadFingerprint,omitempty"`
	SourceFormat        string                                   `json:"sourceFormat,omitempty"`
	SampleCount         int                                      `json:"sampleCount,omitempty"`
	AuditCommit         string                                   `json:"auditCommit,omitempty"`
	AuditHardware       string                                   `json:"auditHardware,omitempty"`
	Cases               map[string]strategyBlockBenchmarkMetrics `json:"cases"`
}

type strategyBlockBenchmarkMetrics struct {
	NSPerOp     int64 `json:"nsPerOp"`
	BytesPerOp  int64 `json:"bytesPerOp"`
	AllocsPerOp int64 `json:"allocsPerOp"`
}

func TestStrategyBlockBenchmarkBaseline(t *testing.T) {
	updateBaseline := strings.TrimSpace(os.Getenv(strategyBlockBenchmarkUpdateBaselineEnv)) != ""
	enforceBaseline := updateBaseline || strings.TrimSpace(os.Getenv(strategyBlockBenchmarkEnforceBaselineEnv)) != ""
	if !enforceBaseline {
		t.Skipf("set %s=1 to compare matrix against baseline or %s=1 to rewrite baseline", strategyBlockBenchmarkEnforceBaselineEnv, strategyBlockBenchmarkUpdateBaselineEnv)
	}

	baselinePath := filepath.Join("testdata", filepath.Base(strategyBlockBenchmarkBaselinePath))
	actual := collectStrategyBlockBenchmarkBaseline(t)
	if updateBaseline {
		actual.AuditCommit = strings.TrimSpace(os.Getenv(strategyBlockBenchmarkAuditCommitEnv))
		actual.AuditHardware = strings.TrimSpace(os.Getenv(strategyBlockBenchmarkAuditHardwareEnv))
		if actual.AuditCommit == "" || actual.AuditHardware == "" {
			t.Fatalf("audited baseline update requires %s and %s", strategyBlockBenchmarkAuditCommitEnv, strategyBlockBenchmarkAuditHardwareEnv)
		}
		writeStrategyBlockBenchmarkBaseline(t, baselinePath, actual)
		t.Logf("updated strategy block benchmark baseline at %s", baselinePath)
		return
	}

	expected := readStrategyBlockBenchmarkBaseline(t, baselinePath)
	compareStrategyBlockBenchmarkBaseline(t, expected, actual)
}

func collectStrategyBlockBenchmarkBaseline(t *testing.T) strategyBlockBenchmarkBaseline {
	t.Helper()
	isolateBacktestHome(t)
	dbPath, startTime, endTime := seedStrategyBlockBenchmarkStore(t)
	restoreLogs := suppressBacktestRunLogs(t)
	defer restoreLogs()

	ctx := context.Background()
	baseline := strategyBlockBenchmarkBaseline{
		Version:             strategyBlockBenchmarkBaselineVersion,
		UpdatedAt:           time.Now().UTC().Format(time.RFC3339),
		WorkloadFingerprint: strategyBlockBenchmarkWorkloadFingerprint(),
		SourceFormat:        "pine-v6",
		SampleCount:         strategyBlockBenchmarkSampleCount,
		Cases:               make(map[string]strategyBlockBenchmarkMetrics, len(strategyBlockBenchmarkCases())),
	}
	for _, benchmarkCase := range strategyBlockBenchmarkCases() {
		cfg := strategyBlockBenchmarkRunConfig(dbPath, startTime, endTime, benchmarkCase.script)
		result := collectStrategyBlockBenchmarkSamples(ctx, cfg)
		baseline.Cases[benchmarkCase.name] = strategyBlockBenchmarkMetrics{
			NSPerOp:     result.NsPerOp(),
			BytesPerOp:  result.AllocedBytesPerOp(),
			AllocsPerOp: result.AllocsPerOp(),
		}
	}
	return baseline
}

func collectStrategyBlockBenchmarkSamples(ctx context.Context, cfg RunConfig) testing.BenchmarkResult {
	samples := make([]testing.BenchmarkResult, 0, strategyBlockBenchmarkSampleCount)
	for sampleIndex := 0; sampleIndex < strategyBlockBenchmarkSampleCount; sampleIndex++ {
		samples = append(samples, testing.Benchmark(func(b *testing.B) {
			b.ReportAllocs()
			for index := 0; index < b.N; index++ {
				runResult := Run(ctx, cfg)
				if runResult == nil {
					b.Fatal("expected run result")
				}
				if runResult.Error != "" {
					b.Fatalf("Run() error = %s", runResult.Error)
				}
			}
		}))
	}
	sort.Slice(samples, func(left, right int) bool {
		return samples[left].NsPerOp() < samples[right].NsPerOp()
	})
	return samples[strategyBlockBenchmarkRepresentativeSampleIndex(len(samples))]
}

func strategyBlockBenchmarkRepresentativeSampleIndex(sampleCount int) int {
	if sampleCount <= 1 {
		return 0
	}
	return sampleCount / 2
}

func compareStrategyBlockBenchmarkBaseline(t *testing.T, expected, actual strategyBlockBenchmarkBaseline) {
	t.Helper()
	if expected.Version != strategyBlockBenchmarkBaselineVersion {
		t.Fatalf("baseline version = %d, want %d", expected.Version, strategyBlockBenchmarkBaselineVersion)
	}
	if expected.WorkloadFingerprint != actual.WorkloadFingerprint {
		t.Fatalf("baseline workload fingerprint = %q, current = %q; the historical JSON is not comparable and requires an audited regeneration", expected.WorkloadFingerprint, actual.WorkloadFingerprint)
	}
	if expected.SourceFormat != actual.SourceFormat || expected.SampleCount != actual.SampleCount {
		t.Fatalf("baseline metadata sourceFormat/sampleCount = %q/%d, current = %q/%d; audited regeneration required", expected.SourceFormat, expected.SampleCount, actual.SourceFormat, actual.SampleCount)
	}
	for name, actualMetrics := range actual.Cases {
		expectedMetrics, ok := expected.Cases[name]
		if !ok {
			t.Errorf("baseline missing case %s; rerun with %s=1", name, strategyBlockBenchmarkUpdateBaselineEnv)
			continue
		}
		assertStrategyBlockMetricWithinRatio(t, name, "ns/op", expectedMetrics.NSPerOp, actualMetrics.NSPerOp, strategyBlockBenchmarkMaxNSOpRegressionRatio)
		assertStrategyBlockMetricWithinRatio(t, name, "B/op", expectedMetrics.BytesPerOp, actualMetrics.BytesPerOp, strategyBlockBenchmarkMaxBytesRegressionRatio)
		assertStrategyBlockMetricWithinRatio(t, name, "allocs/op", expectedMetrics.AllocsPerOp, actualMetrics.AllocsPerOp, strategyBlockBenchmarkMaxAllocsRegressionRatio)
	}
	for name := range expected.Cases {
		if _, ok := actual.Cases[name]; !ok {
			t.Errorf("current matrix missing baseline case %s", name)
		}
	}
	if t.Failed() {
		t.Logf("rerun with %s=1 to refresh baseline after an intentional performance shift", strategyBlockBenchmarkUpdateBaselineEnv)
	}
}

func strategyBlockBenchmarkWorkloadFingerprint() string {
	hash := sha256.New()
	_, jftradeErr1 := fmt.Fprintln(hash, "sourceFormat=pine-v6")
	jftradePanicOnError(jftradeErr1)
	for _, benchmarkCase := range strategyBlockBenchmarkCases() {
		_, jftradeErr2 := fmt.Fprintln(hash, benchmarkCase.name)
		jftradePanicOnError(jftradeErr2)
		_, jftradeErr3 := fmt.Fprintln(hash, strings.TrimSpace(benchmarkCase.script))
		jftradePanicOnError(jftradeErr3)
	}
	return fmt.Sprintf("sha256:%x", hash.Sum(nil))
}

func assertStrategyBlockMetricWithinRatio(t *testing.T, caseName, metricName string, baselineValue, actualValue int64, maxRatio float64) {
	t.Helper()
	if baselineValue <= 0 || actualValue <= 0 {
		return
	}
	threshold := float64(baselineValue) * maxRatio
	if float64(actualValue) <= threshold {
		return
	}
	t.Errorf("%s %s regressed: got %d, baseline %d, allowed <= %.2f (ratio %.2fx)", caseName, metricName, actualValue, baselineValue, threshold, maxRatio)
}

func readStrategyBlockBenchmarkBaseline(t *testing.T, baselinePath string) strategyBlockBenchmarkBaseline {
	t.Helper()
	content, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatalf("read strategy block benchmark baseline %s: %v", baselinePath, err)
	}
	baseline := strategyBlockBenchmarkBaseline{}
	if err := json.Unmarshal(content, &baseline); err != nil {
		t.Fatalf("parse strategy block benchmark baseline %s: %v", baselinePath, err)
	}
	if baseline.Cases == nil {
		baseline.Cases = map[string]strategyBlockBenchmarkMetrics{}
	}
	return baseline
}

func writeStrategyBlockBenchmarkBaseline(t *testing.T, baselinePath string, baseline strategyBlockBenchmarkBaseline) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(baselinePath), 0o755); err != nil {
		t.Fatalf("mkdir baseline dir: %v", err)
	}
	content, err := json.MarshalIndent(baseline, "", "  ")
	if err != nil {
		t.Fatalf("marshal baseline: %v", err)
	}
	content = append(content, '\n')
	if err := os.WriteFile(baselinePath, content, 0o644); err != nil {
		t.Fatalf("write baseline %s: %v", baselinePath, err)
	}
}

func jftradePanicOnError(err error) {
	if err != nil {
		panic(err)
	}
}
