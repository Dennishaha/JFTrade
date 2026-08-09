package futu

import (
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
)

func TestFutuOptionComboHelperBusinessBoundaries(t *testing.T) {
	strategies := map[string]qotcommonpb.OptionStrategyType{
		" vertical ": qotcommonpb.OptionStrategyType_OptionStrategyType_Spread,
		"straddle":   qotcommonpb.OptionStrategyType_OptionStrategyType_Straddle,
		"strangle":   qotcommonpb.OptionStrategyType_OptionStrategyType_Strangle,
		"butterfly":  qotcommonpb.OptionStrategyType_OptionStrategyType_Butterfly,
		"calendar":   qotcommonpb.OptionStrategyType_OptionStrategyType_CalendarSpread,
	}
	for name, expected := range strategies {
		got, err := futuOptionStrategyValue(name)
		if err != nil || got != int32(expected) {
			t.Errorf("strategy %q = %d, %v", name, got, err)
		}
	}
	if _, err := futuOptionStrategyValue("unsupported"); err == nil {
		t.Fatal("unsupported strategy succeeded")
	}
	for _, strategy := range []qotcommonpb.OptionStrategyType{
		qotcommonpb.OptionStrategyType_OptionStrategyType_Spread,
		qotcommonpb.OptionStrategyType_OptionStrategyType_Strangle,
		qotcommonpb.OptionStrategyType_OptionStrategyType_Butterfly,
	} {
		if !optionStrategySupportsSpread(int32(strategy)) {
			t.Errorf("%s should require a spread", strategy)
		}
	}
	if optionStrategySupportsSpread(
		int32(qotcommonpb.OptionStrategyType_OptionStrategyType_Straddle),
	) {
		t.Fatal("straddle unexpectedly requires a spread")
	}

	buy := int32(trdcommonpb.TrdSide_TrdSide_Buy)
	sell := int32(trdcommonpb.TrdSide_TrdSide_Sell)
	one := 1.0
	two := 2.0
	expected := []*qotcommonpb.ComboLeg{
		{
			Security: &qotcommonpb.Security{Market: new(int32(11)), Code: new("AAPL.C1")},
			Side:     &buy, QtyRatio: &one,
		},
		{
			Security: &qotcommonpb.Security{Market: new(int32(11)), Code: new("AAPL.C2")},
			Side:     &sell, QtyRatio: &two,
		},
	}
	mapped := comboLegMaps(expected)
	if len(mapped) != 2 {
		t.Fatalf("combo leg maps = %#v", mapped)
	}
	strategy := map[string]any{"multiLegs": mapped}
	if !containsOptionStrategyLegs([]map[string]any{strategy}, expected) ||
		containsOptionStrategyLegs([]map[string]any{{"multiLegs": []any{}}}, expected) ||
		containsOptionStrategyLegs(nil, expected) {
		t.Fatal("option strategy leg comparison changed")
	}
	if len(comboLegKeys(mapped)) != 2 ||
		len(comboLegKeys(objectSlice(mapped))) != 2 ||
		len(comboLegKeys("invalid")) != 0 {
		t.Fatal("combo leg canonicalization changed")
	}
	if keys := comboLegKeys([]any{"invalid"}); len(keys) != 1 {
		t.Fatalf("invalid leg canonical key = %#v", keys)
	}

	values := numberSlice([]any{1.0, "skip", 2.0})
	if len(values) != 2 || len(numberSlice("invalid")) != 0 {
		t.Fatalf("number slice = %#v", values)
	}
	if !containsFloat(values, 1.0+1e-9) ||
		!containsFloat(values, 2.0-1e-9) ||
		containsFloat(values, 3) ||
		containsFloat(nil, 1) {
		t.Fatal("floating spread matching changed")
	}
	numbers := []struct {
		value any
		want  float64
	}{
		{float64(1), 1}, {float32(2), 2}, {int(3), 3},
		{int32(4), 4}, {int64(5), 5}, {"invalid", 0},
	}
	for _, test := range numbers {
		if got := numericValue(test.value); got != test.want {
			t.Errorf("numericValue(%T) = %v", test.value, got)
		}
	}
	if !equalStringSlices([]string{"a"}, []string{"a"}) ||
		equalStringSlices([]string{"a"}, nil) ||
		equalStringSlices([]string{"a"}, []string{"b"}) {
		t.Fatal("string-slice equality changed")
	}

	if optionComboAnalysis("vertical", nil) != nil {
		t.Fatal("nil analysis payload produced a result")
	}
	analysis := optionComboAnalysis("vertical", map[string]any{
		"bid1": 1.0, "ask1": "invalid",
		"maxProfit": 10.0, "maxLoss": 2.0,
		"breakevenPoints": []any{3.0, 4.0},
		"probOfProfit":    0.5, "delta": 0.2, "theta": -0.1,
	})
	if analysis == nil || analysis.Bid == nil || analysis.Ask != nil ||
		len(analysis.BreakevenPoints) != 2 {
		t.Fatalf("option analysis = %#v", analysis)
	}
	unlimited := optionComboAnalysis("strangle", map[string]any{
		"maxProfit": 9_999_999.0, "maxLoss": 10_000_000.0,
	})
	if unlimited == nil || !unlimited.MaxProfitUnlimited ||
		!unlimited.MaxLossUnlimited || unlimited.MaxProfit != nil ||
		unlimited.MaxLoss != nil {
		t.Fatalf("unlimited option analysis = %#v", unlimited)
	}
	if value, unbounded := optionComboBound("invalid"); value != nil || unbounded {
		t.Fatalf("invalid combo bound = %#v, %t", value, unbounded)
	}
	if numberPointer("invalid") != nil || numberPointer(1.0) == nil {
		t.Fatal("number pointer conversion changed")
	}
}

func TestFutuComboIntentValidOptionCalendarAndEventForms(t *testing.T) {
	quantity := 2.0
	optionLegs := []broker.OrderLegIntent{
		{
			InstrumentID: "US.C1", ProductClass: broker.ProductClassOption,
			Side: "BUY", Ratio: 1, Quantity: &quantity,
		},
		{
			InstrumentID: "US.C2", ProductClass: broker.ProductClassOption,
			Side: "SELL", Ratio: 2, Quantity: &quantity,
		},
	}
	option := broker.ComboOrderIntent{
		OrderKind: broker.OrderKindOptionCombo, UnderlyingID: "US.AAPL",
		OptionStrategy: "straddle", NearExpiry: "2026-07-17", Legs: optionLegs,
	}
	if kind, err := validateComboIntent(option); err != nil ||
		kind != broker.OrderKindOptionCombo {
		t.Fatalf("valid option combo = %q, %v", kind, err)
	}
	calendar := option
	calendar.OptionStrategy = "calendar"
	calendar.FarExpiry = "2026-08-21"
	if _, err := validateComboIntent(calendar); err != nil {
		t.Fatalf("valid calendar combo: %v", err)
	}
	calendar.FarExpiry = ""
	if _, err := validateComboIntent(calendar); err == nil {
		t.Fatal("calendar without far expiry succeeded")
	}
	option.NearExpiry = ""
	if _, err := validateComboIntent(option); err == nil {
		t.Fatal("option combo without near expiry succeeded")
	}
	option.NearExpiry = "2026-07-17"
	option.OptionStrategy = "unsupported"
	if _, err := validateComboIntent(option); err == nil {
		t.Fatal("unknown option combo strategy succeeded")
	}

	amount := 20.0
	expires := time.Now().Add(time.Minute)
	event := broker.ComboOrderIntent{
		OrderKind: broker.OrderKindEventParlay, RFQID: "rfq",
		QuoteExpiresAt: &expires, Amount: &amount,
		Legs: []broker.OrderLegIntent{
			{
				InstrumentID: "US.E1", ProductClass: broker.ProductClassEventContract,
				Side: "BUY", Ratio: 1, PredictionSide: " yes ",
			},
			{
				InstrumentID: "US.E2", ProductClass: broker.ProductClassEventContract,
				Side: "BUY", Ratio: 1, PredictionSide: "NO",
			},
		},
	}
	if kind, err := validateComboIntent(event); err != nil ||
		kind != broker.OrderKindEventParlay {
		t.Fatalf("valid event parlay = %q, %v", kind, err)
	}
	if ids := comboInstrumentIDs(event.Legs); len(ids) != 2 ||
		ids[0] != "US.E1" {
		t.Fatalf("event ids = %#v", ids)
	}
}
