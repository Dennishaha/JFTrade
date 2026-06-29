package backtest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/types"

	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

func TestRealPineTSBacktestSmoke(t *testing.T) {
	if os.Getenv("JFTRADE_PINETS_BACKTEST_SMOKE") != "1" {
		t.Skip("JFTRADE_PINETS_BACKTEST_SMOKE=1 is required for real PineTS backtest smoke")
	}
	isolateBacktestHome(t)

	root := backtestSmokeRepoRoot(t)
	workerPath := strings.TrimSpace(os.Getenv("JFTRADE_PINEWORKER_BUNDLE"))
	if workerPath == "" {
		t.Fatal("JFTRADE_PINEWORKER_BUNDLE is required for real PineTS backtest smoke")
	}
	bundleData, err := os.ReadFile(workerPath)
	if err != nil {
		t.Fatalf("read worker binary: %v", err)
	}
	sum := sha256.Sum256(bundleData)
	stdout := pineworker.NewTailBuffer(8192)
	stderr := pineworker.NewTailBuffer(8192)
	launcher, err := pineworker.NewBunWorkerLauncher(pineworker.BunWorkerLauncherConfig{
		Bundle: pineworker.WorkerBundle{
			Name:   filepath.Base(workerPath),
			Data:   bundleData,
			SHA256: hex.EncodeToString(sum[:]),
		},
		RuntimePath:     firstNonEmptyString(strings.TrimSpace(os.Getenv("JFTRADE_PINEWORKER_RUNTIME")), "bun"),
		WorkDir:         root,
		ProtoPath:       filepath.Join(root, "pkg", "strategy", "pineworker", "proto", "pineworker.proto"),
		MaxMessageBytes: 64 * 1024 * 1024,
		StopTimeout:     2 * time.Second,
		PineTSVersion:   "backtest-smoke",
		Stdout:          stdout,
		Stderr:          stderr,
	})
	if err != nil {
		t.Fatalf("NewBunWorkerLauncher: %v", err)
	}

	workerConfig := pineworker.DefaultWorkerConfig(1)
	workerConfig.RequestTimeout = 15 * time.Second
	manager, err := pineworker.NewWorkerManager(pineworker.ManagerConfig{
		Workers:       1,
		Host:          "127.0.0.1",
		StartPort:     backtestSmokeFreeTCPPort(t),
		HealthTimeout: 10 * time.Second,
		WorkerConfig:  workerConfig,
		Gate: pineworker.PerformanceGate{
			MaxDuration:       15 * time.Second,
			MaxDurationPerBar: 50 * time.Millisecond,
			MaxRequestBytes:   64 * 1024 * 1024,
			MaxResponseBytes:  64 * 1024 * 1024,
			MaxPeakRSSBytes:   1024 * 1024 * 1024,
		},
	}, launcher, pineworker.NewGRPCDialer(pineworker.GRPCDialerConfig{MaxMessageBytes: workerConfig.MaxMessageBytes}))
	if err != nil {
		t.Fatalf("NewWorkerManager: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("WorkerManager.Start: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	t.Cleanup(func() {
		stopCtx, cancelStop := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelStop()
		if err := manager.Stop(stopCtx); err != nil {
			t.Fatalf("WorkerManager.Stop: %v", err)
		}
	})
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("pine worker stdout:\n%s", stdout.String())
			t.Logf("pine worker stderr:\n%s", stderr.String())
		}
	})

	dbPath := filepath.Join(t.TempDir(), "pinets-real-backtest-smoke.db")
	klines := seedRealPineTSBacktestSmokeKLines(t, dbPath)
	result := RunWithPineWorker(ctx, RunConfig{
		DBPath:         dbPath,
		Symbol:         "US.AAPL",
		Interval:       string(types.Interval1m),
		SourceFormat:   strategydefinition.SourceFormatPineV6,
		StartTime:      klines[0].StartTime.Time(),
		EndTime:        klines[len(klines)-1].EndTime.Time(),
		StrategyScript: realPineTSBacktestSmokeScript(),
		InitialBalance: 10000,
		RehabType:      "forward",
	}, manager)

	if result == nil {
		t.Fatal("RunWithPineWorker returned nil")
	}
	if result.Error != "" {
		t.Fatalf("RunWithPineWorker error = %s", result.Error)
	}
	if result.TotalTrades == 0 {
		t.Fatalf("TotalTrades = %d, want at least one completed trade", result.TotalTrades)
	}
	if len(result.OrderBook) == 0 {
		t.Fatal("OrderBook is empty, want PineTS-generated orders")
	}
	if len(result.Candles) == 0 || len(result.PnLCurve) == 0 {
		t.Fatalf("missing replay result series: candles=%d pnl=%d", len(result.Candles), len(result.PnLCurve))
	}
	t.Logf("PineTS backtest smoke passed: trades=%d orders=%d candles=%d finalBalance=%.2f", result.TotalTrades, len(result.OrderBook), len(result.Candles), result.FinalBalance)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func seedRealPineTSBacktestSmokeKLines(t *testing.T, dbPath string) []types.KLine {
	t.Helper()
	store, err := NewFutuKLineStore(dbPath)
	if err != nil {
		t.Fatalf("NewFutuKLineStore: %v", err)
	}
	baseStart := time.Date(2026, time.June, 29, 9, 30, 0, 0, time.UTC)
	klines := buildBenchmarkKLines(baseStart, 64)
	if err := store.InsertKLines(klines, "forward"); err != nil {
		jftradeCheckTestError(t, store.Close())
		t.Fatalf("InsertKLines: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("store.Close: %v", err)
	}
	return klines
}

func realPineTSBacktestSmokeScript() string {
	return `//@version=6
strategy("PineTS Backtest Smoke", overlay=true, default_qty_type=strategy.fixed, default_qty_value=1)
fast = ta.ema(close, 3)
slow = ta.ema(close, 8)
nowValue = timenow
if bar_index == 10 and nowValue >= time
    strategy.entry("SmokeLong", strategy.long, qty=1)
if bar_index == 20
    strategy.close("SmokeLong")
if bar_index == 30
    strategy.entry("SmokeShort", strategy.short, qty=2)
if bar_index == 40
    strategy.close("SmokeShort")
plot(fast)
plot(slow)`
}

func backtestSmokeRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find repo root from %s", wd)
		}
		dir = parent
	}
}

func backtestSmokeFreeTCPPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen free port: %v", err)
	}
	defer func() { jftradeCheckTestError(t, listener.Close()) }()
	return listener.Addr().(*net.TCPAddr).Port
}
