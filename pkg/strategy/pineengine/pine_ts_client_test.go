package pineengine

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

func TestDisabledPayloadDocumentsCommunityAGPL(t *testing.T) {
	payload := DisabledPayload()
	if payload.Enabled {
		t.Fatal("DisabledPayload().Enabled = true, want false")
	}
	if payload.Compliance["license"] != "AGPL-3.0-only" {
		t.Fatalf("license = %#v", payload.Compliance["license"])
	}
}

func TestPinetsWorkerClientEngineInfoAndRunIndicator(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skipf("node unavailable: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client := NewPinetsWorkerClient("", "")
	defer func() { _ = client.Close() }()

	info, err := client.EngineInfo(ctx)
	if err != nil {
		t.Fatalf("EngineInfo: %v", err)
	}
	if info.Engine != PinetsShadowEngineID || info.License != "AGPL-3.0-only" {
		t.Fatalf("EngineInfo = %#v", info)
	}

	result, err := client.RunIndicator(ctx, RunIndicatorRequest{
		Script: `//@version=6
indicator("SMA")
plot(ta.sma(close, 3), "SMA")`,
		Symbol:    "JF.TEST",
		Timeframe: "1m",
		TimeoutMS: 10_000,
	})
	if err != nil {
		t.Fatalf("RunIndicator: %v", err)
	}
	if !result.OK || result.Engine != PinetsShadowEngineID {
		t.Fatalf("RunIndicator result = %#v", result)
	}
	if len(result.Plots) == 0 || len(result.Signals) == 0 {
		t.Fatalf("RunIndicator plots=%#v signals=%#v, want non-empty", result.Plots, result.Signals)
	}
}

func TestPinetsWorkerClientMapsRuntimeErrors(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skipf("node unavailable: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client := NewPinetsWorkerClient("", "")
	defer func() { _ = client.Close() }()

	_, err := client.RunIndicator(ctx, RunIndicatorRequest{
		Script: `//@version=6
indicator("empty")
plot(close)`,
		Symbol:    "JF.TEST",
		Timeframe: "1m",
		Candles:   []Candle{},
		TimeoutMS: 10_000,
	})
	if err == nil {
		t.Fatal("RunIndicator(empty candles) error = nil, want worker error")
	}
}
