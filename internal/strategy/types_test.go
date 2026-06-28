package strategy

import (
	"encoding/json"
	"testing"
)

func TestDefinitionViewJSONRemainsFlat(t *testing.T) {
	data, err := json.Marshal(DefinitionView{
		Definition: Definition{
			ID:           "definition-1",
			Name:         "Typed Strategy",
			Version:      "0.1.0",
			Description:  "description",
			Runtime:      "pine-pinets",
			SourceFormat: "pine-v6",
			Script:       "strategy(\"typed\")",
			CreatedAt:    "2026-06-15T00:00:00Z",
			UpdatedAt:    "2026-06-15T00:00:00Z",
		},
		DerivedWarmupBars:     12,
		DerivedWarmupInterval: "5m",
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	for _, key := range []string{"id", "name", "version", "runtime", "sourceFormat", "script", "derivedWarmupBars", "derivedWarmupInterval"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("JSON key %q missing from %s", key, data)
		}
	}
	if _, nested := payload["Definition"]; nested {
		t.Fatalf("DefinitionView JSON unexpectedly nested: %s", data)
	}
}

func TestInstanceBindingJSONContract(t *testing.T) {
	data, err := json.Marshal(InstanceBinding{
		Instruments:   []BindingInstrument{{Market: "US", Code: "AAPL"}},
		Symbols:       []string{"US.AAPL"},
		Interval:      "5m",
		ExecutionMode: "live",
		BrokerAccount: &BrokerAccountBinding{
			BrokerID:           "futu",
			AccountID:          "account-1",
			TradingEnvironment: "SIMULATE",
			Market:             "US",
		},
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var roundTrip InstanceBinding
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(roundTrip.Instruments) != 1 || roundTrip.Instruments[0].Code != "AAPL" {
		t.Fatalf("instruments = %#v", roundTrip.Instruments)
	}
	if roundTrip.BrokerAccount == nil || roundTrip.BrokerAccount.BrokerID != "futu" {
		t.Fatalf("brokerAccount = %#v", roundTrip.BrokerAccount)
	}
}
