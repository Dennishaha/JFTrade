package runtimecontrol

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
)

type stage5StrategyDecimal float64

func (value *stage5StrategyDecimal) UnmarshalJSON(data []byte) error {
	parsed, err := strconv.ParseFloat(string(bytes.Trim(data, `"`)), 64)
	if err != nil {
		return err
	}
	*value = stage5StrategyDecimal(parsed)
	return nil
}

type stage5StrategyCorpus struct {
	RiskConfig struct {
		MaxOrderQuantity *stage5StrategyDecimal `json:"maxOrderQuantity"`
		MaxOrderNotional *stage5StrategyDecimal `json:"maxOrderNotional"`
	} `json:"riskConfig"`
	StrategyScenarios []struct {
		Name       string `json:"name"`
		Mode       string `json:"mode"`
		Operations []struct {
			Op     string `json:"op"`
			Signal *struct {
				SignalID string                 `json:"signalId"`
				Symbol   string                 `json:"symbol"`
				Side     string                 `json:"side"`
				Quantity stage5StrategyDecimal  `json:"quantity"`
				Price    *stage5StrategyDecimal `json:"price"`
			} `json:"signal"`
		} `json:"operations"`
	} `json:"strategyScenarios"`
}

type stage5StrategyExpected struct {
	Strategies []struct {
		Name       string `json:"name"`
		Mode       string `json:"mode"`
		Operations []struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
			Op    string `json:"op"`
			Value *struct {
				Duplicate bool `json:"duplicate"`
				TradePlan *struct {
					Accepted   bool    `json:"accepted"`
					Dispatch   bool    `json:"dispatch"`
					ReasonCode *string `json:"reasonCode"`
				} `json:"tradePlan"`
				Notification *struct {
					Dispatch bool `json:"dispatch"`
				} `json:"notification"`
			} `json:"value"`
		} `json:"operations"`
	} `json:"strategies"`
}

func TestRustMigrationStage5StrategyRiskAndNotificationPlansMatchCorpus(t *testing.T) {
	var corpus stage5StrategyCorpus
	var expected stage5StrategyExpected
	readStage5StrategyFixture(t, "trading-strategy-corpus.json", &corpus)
	readStage5StrategyFixture(t, "trading-strategy-corpus.expected.json", &expected)
	if len(corpus.StrategyScenarios) != len(expected.Strategies) {
		t.Fatalf("strategy scenarios = %d, expected %d", len(corpus.StrategyScenarios), len(expected.Strategies))
	}
	settings := RiskSettings{
		Mode:             ModeEnforce,
		MaxOrderQuantity: stage5StrategyFloatPointer(corpus.RiskConfig.MaxOrderQuantity),
		MaxOrderNotional: stage5StrategyFloatPointer(corpus.RiskConfig.MaxOrderNotional),
	}
	for index, scenario := range corpus.StrategyScenarios {
		wantScenario := expected.Strategies[index]
		if scenario.Name != wantScenario.Name || scenario.Mode != wantScenario.Mode || len(scenario.Operations) != len(wantScenario.Operations) {
			t.Fatalf("strategy scenario[%d] shape drifted", index)
		}
		for operationIndex, operation := range scenario.Operations {
			if operation.Op != "signal" || operation.Signal == nil {
				continue
			}
			want := wantScenario.Operations[operationIndex]
			if want.Op != "signal" {
				t.Fatalf("strategy operation[%d][%d] = %q, want signal", index, operationIndex, want.Op)
			}
			if want.Value == nil {
				if want.OK || want.Error == "" {
					t.Fatalf("blocked signal[%d][%d] is not an explicit error", index, operationIndex)
				}
				continue
			}
			if scenario.Mode == "notify_only" {
				if !want.Value.Duplicate && (want.Value.Notification == nil || want.Value.Notification.Dispatch || want.Value.TradePlan != nil) {
					t.Fatalf("notify-only signal[%d][%d] would escape shadow: %#v", index, operationIndex, want.Value)
				}
				continue
			}
			if want.Value.TradePlan == nil || want.Value.TradePlan.Dispatch {
				t.Fatalf("strategy trade signal[%d][%d] lacks a non-dispatch plan", index, operationIndex)
			}
			if scenario.Mode == "paper" {
				if !want.Value.TradePlan.Accepted {
					t.Fatalf("paper signal[%d][%d] was rejected", index, operationIndex)
				}
				continue
			}
			price := stage5StrategyFloatPointer(operation.Signal.Price)
			decision := EvaluateRisk(settings, OrderIntent{
				Symbol: operation.Signal.Symbol, Side: operation.Signal.Side,
				Quantity: float64(operation.Signal.Quantity), Price: price,
			}, RiskContext{})
			gotReason := stage5StrategyReasonCode(decision.Reason)
			if want.Value.TradePlan.Accepted == decision.Rejected || !stage5StrategyOptionalEqual(want.Value.TradePlan.ReasonCode, gotReason) {
				t.Fatalf("live strategy risk[%d][%d] = reject %v code %q, want %#v", index, operationIndex, decision.Rejected, gotReason, want.Value.TradePlan)
			}
		}
	}
}

func stage5StrategyReasonCode(reason string) string {
	switch reason {
	case "max_order_quantity":
		return "MAX_ORDER_QUANTITY_EXCEEDED"
	case "max_order_notional":
		return "MAX_ORDER_NOTIONAL_EXCEEDED"
	case "max_order_notional_missing_price":
		return "RISK_PRICE_UNAVAILABLE"
	default:
		return reason
	}
}

func stage5StrategyFloatPointer(value *stage5StrategyDecimal) *float64 {
	if value == nil {
		return nil
	}
	converted := float64(*value)
	return &converted
}

func stage5StrategyOptionalEqual(expected *string, actual string) bool {
	if expected == nil {
		return actual == ""
	}
	return *expected == actual
}

func readStage5StrategyFixture(t *testing.T, name string, target any) {
	t.Helper()
	directory := os.Getenv("JFTRADE_STAGE5_FIXTURE_ROOT")
	if directory == "" {
		_, source, _, ok := runtime.Caller(0)
		if !ok {
			t.Fatal("resolve stage 5 strategy test source")
		}
		directory = filepath.Join(filepath.Dir(source), "..", "..", "..", "tests", "fixtures", "rust-migration", "stage5")
	}
	content, err := os.ReadFile(filepath.Join(directory, name))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(content, target); err != nil {
		t.Fatal(err)
	}
}
