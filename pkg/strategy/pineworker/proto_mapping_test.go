package pineworker

import (
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
	protoRequest.Params["threshold"] = "99"
	if request.Params["threshold"] != "10" {
		t.Fatal("request params were aliased")
	}

	response := responseFromProto(&pineworkerpb.RunScriptResponse{
		JobId: "job-1",
		Outputs: []*pineworkerpb.SeriesOutput{{
			Name:   "out",
			Kind:   "plot",
			Values: []float64{1, 2},
		}},
		Plots: []*pineworkerpb.PlotOutput{{
			Name:   "plot",
			Values: []float64{3},
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

	if response.JobID != "job-1" || response.Outputs[0].Values[1] != 2 || response.Plots[0].Values[0] != 3 {
		t.Fatalf("unexpected mapped response: %#v", response)
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

func TestProtoMappingHandlesNilResponses(t *testing.T) {
	if response := responseFromProto(nil); response.JobID != "" {
		t.Fatalf("nil response mapped to %#v", response)
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
