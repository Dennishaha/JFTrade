package pineworker

import (
	"encoding/binary"
	"fmt"
	"math"
	"runtime"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker/pineworkerpb"
)

func TestProtoMappingRoundTripRequestAndResponse(t *testing.T) {
	request := validClientRequest()
	protoRequest := requestToProto(request)
	if protoRequest.GetJobId() != request.JobID || protoRequest.GetParams()["threshold"] != "10" {
		t.Fatalf("unexpected proto request: %#v", protoRequest)
	}
	if protoRequest.GetIncludePlots() {
		t.Fatalf("backtest request include_plots = true, want false")
	}
	batch := protoRequest.GetCandles()
	if batch.GetEncodingVersion() != candleBatchEncodingVersion || len(batch.GetPayload()) != candleBatchRecordBytes {
		t.Fatalf("unexpected candle batch: %#v", protoRequest.GetCandles())
	}
	if got := int64(binary.LittleEndian.Uint64(batch.GetPayload())); got != request.Candles[0].OpenTime {
		t.Fatalf("encoded open time = %d, want %d", got, request.Candles[0].OpenTime)
	}
	if got := math.Float64frombits(binary.LittleEndian.Uint64(batch.GetPayload()[40:])); got != request.Candles[0].Close {
		t.Fatalf("encoded close = %v, want %v", got, request.Candles[0].Close)
	}
	protoRequest.Params["threshold"] = "99"
	if request.Params["threshold"] != "10" {
		t.Fatal("request params were aliased")
	}
	request.Mode = ModeLive
	if !requestToProto(request).GetIncludePlots() {
		t.Fatal("live request include_plots = false, want true")
	}

	response := responseFromProto(&pineworkerpb.RunScriptResponse{
		JobId: "job-1",
		Plots: []*pineworkerpb.PlotOutput{{
			Name:   "plot",
			Values: []float64{1, 2},
		}},
		OrderIntents: []*pineworkerpb.OrderIntent{{
			Kind:           "entry",
			Id:             "long",
			FromEntry:      "base",
			Direction:      "long",
			Quantity:       1,
			QuantityPct:    25,
			LimitPrice:     10,
			StopPrice:      9,
			Comment:        "c",
			AlertMessage:   "a",
			DisableAlert:   true,
			BarIndex:       2,
			Time:           123,
			HasQuantity:    true,
			HasQuantityPct: true,
			HasLimitPrice:  true,
			HasStopPrice:   true,
		}},
		Alerts: []*pineworkerpb.AlertEvent{{
			Type:      "alertcondition",
			Id:        "alert-1",
			Message:   "crossed",
			Title:     "Cross",
			Frequency: "all",
			BarIndex:  2,
			Time:      123,
		}},
		VisualOutputs: []*pineworkerpb.VisualOutput{{
			Kind:        "label",
			Name:        "entry-label",
			PayloadJson: `{"text":"Long"}`,
		}},
		StrategyMetrics: &pineworkerpb.StrategyMetrics{
			BuyAndHoldPnl:             0,
			BuyAndHoldPerGain:         12.5,
			StrategyOutperformance:    -3.25,
			HasBuyAndHoldPnl:          true,
			HasBuyAndHoldPerGain:      true,
			HasStrategyOutperformance: true,
		},
		Logs:        []string{"log"},
		Warnings:    []string{"warn"},
		Diagnostics: []*pineworkerpb.Diagnostic{{Severity: "warning", Code: "x", Message: "m", Line: 1, Column: 2}},
		Metadata: &pineworkerpb.WorkerMetadata{
			WorkerId:      "worker-1",
			Version:       "0.1.0",
			PinetsVersion: "pinets",
			ScriptHash:    "script",
			DataHash:      "data",
			DurationMs:    7,
			RequestBytes:  8,
			ResponseBytes: 9,
			PeakRssBytes:  10,
		},
		Error: "worker error",
	})

	if response.JobID != "job-1" || response.Outputs[0].Values[1] != 2 || response.Plots[0].Values[0] != 1 {
		t.Fatalf("unexpected mapped response: %#v", response)
	}
	if response.Outputs[0].Kind != "plot" || response.Outputs[0].Name != response.Plots[0].Name {
		t.Fatalf("outputs were not rebuilt from plots: %#v", response.Outputs)
	}
	intent := response.OrderIntents[0]
	if intent.ID != "long" || !intent.HasStopPrice || !intent.DisableAlert {
		t.Fatalf("unexpected mapped order intent: %#v", intent)
	}
	if response.Alerts[0].ID != "alert-1" || response.Alerts[0].Frequency != "all" {
		t.Fatalf("unexpected mapped alerts: %#v", response.Alerts)
	}
	if response.VisualOutputs[0].Kind != "label" || response.VisualOutputs[0].PayloadJSON == "" {
		t.Fatalf("unexpected mapped visual outputs: %#v", response.VisualOutputs)
	}
	if response.StrategyMetrics == nil || !response.StrategyMetrics.HasBuyAndHoldPnL || response.StrategyMetrics.BuyAndHoldPerGain != 12.5 {
		t.Fatalf("unexpected mapped strategy metrics: %#v", response.StrategyMetrics)
	}
	if response.Diagnostics[0].Line != 1 || response.Metadata.Duration != 7*time.Millisecond {
		t.Fatalf("unexpected diagnostics/metadata: %#v", response)
	}
}

func TestCandleBatchEncodingGoldenVector(t *testing.T) {
	batch := candlesToProto([]Candle{{
		OpenTime: -2, CloseTime: 3, Open: 1.5, High: 2.5, Low: -0.5, Close: 2, Volume: 0,
	}})
	want := "feffffffffffffff0300000000000000000000000000f83f0000000000000440000000000000e0bf00000000000000400000000000000000"
	if got := fmt.Sprintf("%x", batch.GetPayload()); got != want {
		t.Fatalf("payload = %s, want %s", got, want)
	}
}

func BenchmarkRequestToProto200K(b *testing.B) {
	request := validClientRequest()
	request.Params = nil
	request.Candles = make([]Candle, 200_000)
	for index := range request.Candles {
		request.Candles[index] = Candle{
			OpenTime: int64(index + 1), CloseTime: int64(index + 2),
			Open: 10, High: 12, Low: 9, Close: 11, Volume: 100,
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		result := requestToProto(request)
		runtime.KeepAlive(result)
	}
}

func TestProtoMappingHandlesNilResponses(t *testing.T) {
	if response := responseFromProto(nil); response.JobID != "" {
		t.Fatalf("nil response mapped to %#v", response)
	}
	if response := responseFromProto(&pineworkerpb.RunScriptResponse{JobId: "job-1"}); response.StrategyMetrics != nil {
		t.Fatalf("response without strategy metrics mapped to %#v", response.StrategyMetrics)
	}
	if health := healthFromProto(nil); health.OK {
		t.Fatalf("nil health mapped to %#v", health)
	}
	if metadata := metadataFromProto(nil); metadata.WorkerID != "" {
		t.Fatalf("nil metadata mapped to %#v", metadata)
	}
}

func TestHealthFromProtoCopiesCapabilities(t *testing.T) {
	protoHealth := &pineworkerpb.HealthCheckResponse{
		Ok:            true,
		WorkerId:      "worker-1",
		Version:       "0.1.0",
		PinetsVersion: "pinets",
		Capabilities:  []string{"run"},
	}
	health := healthFromProto(protoHealth)
	protoHealth.Capabilities[0] = "mutated"
	if !health.OK || health.Capabilities[0] != "run" {
		t.Fatalf("unexpected health: %#v", health)
	}
}

func TestRequestToProtoDoesNotAliasNilOrMutableParams(t *testing.T) {
	request := validClientRequest()
	request.Params = nil
	if params := requestToProto(request).GetParams(); params != nil {
		t.Fatalf("nil params mapped to %#v", params)
	}

	request.Params = map[string]string{"risk": "low"}
	protoRequest := requestToProto(request)
	protoRequest.Params["risk"] = "high"
	if request.Params["risk"] != "low" {
		t.Fatalf("request params were aliased after proto mutation: %#v", request.Params)
	}
}
