package instancebinding

import (
	"strings"
	"testing"

	strategy "github.com/jftrade/jftrade-main/internal/strategy"
)

func TestNormalizeBindingPrefersExplicitInstruments(t *testing.T) {
	got := NormalizeBinding(strategy.InstanceBinding{
		Instruments: []strategy.BindingInstrument{
			{Market: "us", Code: "aapl"},
			{Market: "hk", Code: "00700"},
		},
		Symbols: []string{"US.MSFT"},
	}, nil)

	if len(got.Symbols) != 2 || got.Symbols[0] != "US.AAPL" || got.Symbols[1] != "HK.00700" {
		t.Fatalf("normalized symbols = %+v", got.Symbols)
	}
	if len(got.Instruments) != 2 || got.Instruments[0].Market != "US" || got.Instruments[0].Code != "AAPL" || got.Instruments[1].Market != "HK" || got.Instruments[1].Code != "00700" {
		t.Fatalf("normalized instruments = %+v", got.Instruments)
	}
}

func TestNormalizeBindingBackfillsLegacyParams(t *testing.T) {
	got := NormalizeBinding(strategy.InstanceBinding{}, map[string]any{
		"symbol":        "us:aapl",
		"interval":      " 1m ",
		"executionMode": "notify_only",
		"brokerAccount": map[string]any{
			"brokerId":           " Futu ",
			"accountId":          " 123 ",
			"tradingEnvironment": "simulate",
			"market":             "us",
		},
		"runtimeRisk": map[string]any{
			"mode":             "enforce",
			"closeOnly":        true,
			"maxOrderQuantity": float64(100),
			"dailyMaxOrders":   float64(3),
			"pauseOnReject":    true,
		},
	})

	if len(got.Symbols) != 1 || got.Symbols[0] != "US.AAPL" {
		t.Fatalf("symbols = %+v", got.Symbols)
	}
	if got.Interval != "1m" || got.ExecutionMode != ExecutionModeNotifyOnly {
		t.Fatalf("interval/mode = %q/%q", got.Interval, got.ExecutionMode)
	}
	if got.BrokerAccount == nil || got.BrokerAccount.BrokerID != "futu" || got.BrokerAccount.TradingEnvironment != "SIMULATE" || got.BrokerAccount.Market != "US" {
		t.Fatalf("broker account = %+v", got.BrokerAccount)
	}
	if got.RuntimeRisk.Mode != "enforce" || !got.RuntimeRisk.CloseOnly || got.RuntimeRisk.MaxOrderQuantity == nil || *got.RuntimeRisk.MaxOrderQuantity != 100 || got.RuntimeRisk.DailyMaxOrders == nil || *got.RuntimeRisk.DailyMaxOrders != 3 || !got.RuntimeRisk.PauseOnReject {
		t.Fatalf("runtime risk = %+v", got.RuntimeRisk)
	}
}

func TestApplyParamsWritesCanonicalBindingFields(t *testing.T) {
	instance := strategy.ManagedInstance{
		Binding: strategy.InstanceBinding{
			Symbols:       []string{"hk:00700"},
			Interval:      "15m",
			ExecutionMode: "bad-mode",
			BrokerAccount: &strategy.BrokerAccountBinding{BrokerID: " FUTU ", AccountID: " 88 ", TradingEnvironment: "real", Market: "hk"},
		},
		Params: map[string]any{},
	}
	ApplyParams(&instance)

	if got := instance.Params["symbol"]; got != "HK.00700" {
		t.Fatalf("symbol param = %#v", got)
	}
	if got := instance.Params["executionMode"]; got != ExecutionModeLive {
		t.Fatalf("executionMode param = %#v", got)
	}
	account, ok := instance.Params["brokerAccount"].(map[string]any)
	if !ok || account["brokerId"] != "futu" || account["tradingEnvironment"] != "REAL" || account["market"] != "HK" {
		t.Fatalf("brokerAccount param = %#v", instance.Params["brokerAccount"])
	}
	instruments, ok := instance.Params["instruments"].([]map[string]any)
	if !ok || len(instruments) != 1 || instruments[0]["market"] != "HK" || instruments[0]["code"] != "00700" {
		t.Fatalf("instruments param = %#v", instance.Params["instruments"])
	}
}

func TestRiskAndBindingAuditDetails(t *testing.T) {
	maxQuantity := 10.0
	maxOrders := 2
	risk := strategy.RuntimeRiskSettings{
		Mode:             "monitor",
		CloseOnly:        true,
		MaxOrderQuantity: &maxQuantity,
		DailyMaxOrders:   &maxOrders,
		PauseOnReject:    true,
	}
	detail := RiskAuditDetail(risk)
	for _, want := range []string{"mode=monitor", "closeOnly=true", "maxOrderQuantity=10", "dailyMaxOrders=2", "pauseOnReject=true"} {
		if !strings.Contains(detail, want) {
			t.Fatalf("risk detail %q missing %q", detail, want)
		}
	}

	binding := NormalizeBinding(strategy.InstanceBinding{
		Symbols:       []string{"US.AAPL"},
		Interval:      "5m",
		ExecutionMode: ExecutionModeNotifyOnly,
		BrokerAccount: &strategy.BrokerAccountBinding{BrokerID: "futu", AccountID: "acc-1", TradingEnvironment: "SIMULATE", Market: "US"},
	}, nil)
	bindingDetail := BindingAuditDetail("definition-1", binding)
	for _, want := range []string{"definition-1", "symbols=US.AAPL", "interval=5m", "mode=notify_only", "account=futu/SIMULATE/acc-1/US"} {
		if !strings.Contains(bindingDetail, want) {
			t.Fatalf("binding detail %q missing %q", bindingDetail, want)
		}
	}
}

func TestNormalizeBrokerAccountDropsEmptyInput(t *testing.T) {
	if got := NormalizeBrokerAccount(&strategy.BrokerAccountBinding{BrokerID: " ", AccountID: " ", TradingEnvironment: " ", Market: " "}); got != nil {
		t.Fatalf("empty broker account = %+v, want nil", got)
	}
}

func TestNormalizeBindingAcceptsLegacyArrayPayloadsAndDropsInvalidEntries(t *testing.T) {
	got := NormalizeBinding(strategy.InstanceBinding{}, map[string]any{
		"instruments": []any{
			map[string]any{"market": "us", "code": "aapl"},
			map[string]any{"market": "US", "code": "AAPL"},
			map[string]any{"market": "", "code": ""},
			"invalid",
		},
		"symbols": []any{"US.MSFT", 42},
	})
	if len(got.Symbols) != 1 || got.Symbols[0] != "US.AAPL" || len(got.Instruments) != 1 {
		t.Fatalf("binding = %#v", got)
	}
	if got.Interval != "5m" || got.ExecutionMode != ExecutionModeLive {
		t.Fatalf("defaults = %#v", got)
	}
}

func TestBindingConversionBoundaryTypes(t *testing.T) {
	if NormalizeInstrumentID(" ") != "" || NormalizeInstrumentID(" us : aapl ") != "US.AAPL" {
		t.Fatal("instrument ID normalization mismatch")
	}
	if _, ok := BindingInstrumentFromSymbol(""); ok {
		t.Fatal("empty symbol should not produce an instrument")
	}
	if _, ok := BindingInstrumentFromSymbol("invalid"); ok {
		t.Fatal("unqualified symbol should not produce an instrument")
	}
	if _, _, ok := NormalizeBindingInstrument(strategy.BindingInstrument{}); ok {
		t.Fatal("empty binding instrument should be invalid")
	}
	if BrokerAccountFromAny("invalid") != nil {
		t.Fatal("non-map broker account should be ignored")
	}
	if got := RiskSettingsFromAny("invalid"); got.Mode != "" {
		t.Fatalf("non-map risk settings = %#v", got)
	}

	for _, value := range []any{float64(1.5), float32(2.5), 3, int64(4)} {
		if result := numberPointerFromAny(value); result == nil || *result <= 0 {
			t.Fatalf("numberPointerFromAny(%T) = %#v", value, result)
		}
	}
	if numberPointerFromAny("5") != nil {
		t.Fatal("string number should not be accepted")
	}
	for _, value := range []any{3, int64(4), float64(5)} {
		if result := intPointerFromAny(value); result == nil || *result <= 0 {
			t.Fatalf("intPointerFromAny(%T) = %#v", value, result)
		}
	}
	if intPointerFromAny(1.5) != nil || intPointerFromAny("5") != nil {
		t.Fatal("non-integral values should not be accepted")
	}
	if optionalString(1) != "" || optionalBool("true") {
		t.Fatal("optional primitive conversion mismatch")
	}
}

func TestApplyParamsHandlesNilAndClearsStaleOptionalFields(t *testing.T) {
	ApplyParams(nil)
	instance := strategy.ManagedInstance{
		Params: map[string]any{
			"symbol":        42,
			"brokerAccount": "stale",
		},
	}
	ApplyParams(&instance)
	if _, exists := instance.Params["symbol"]; exists {
		t.Fatalf("stale symbol remained: %#v", instance.Params)
	}
	if _, exists := instance.Params["brokerAccount"]; exists {
		t.Fatalf("stale broker account remained: %#v", instance.Params)
	}
	if symbols, ok := instance.Params["symbols"].([]string); !ok || len(symbols) != 0 {
		t.Fatalf("symbols = %#v", instance.Params["symbols"])
	}

	maxNotional := 2500.0
	detail := RiskAuditDetail(strategy.RuntimeRiskSettings{Mode: "enforce", MaxOrderNotional: &maxNotional})
	if !strings.Contains(detail, "maxOrderNotional=2500") {
		t.Fatalf("risk detail = %q", detail)
	}
	if detail := BindingAuditDetail(" definition ", strategy.InstanceBinding{Interval: "1m", ExecutionMode: ExecutionModeLive}); strings.Contains(detail, "account=") {
		t.Fatalf("binding detail = %q", detail)
	}
}
