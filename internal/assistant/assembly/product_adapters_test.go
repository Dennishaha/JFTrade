package assembly

import (
	"maps"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestCustomizationToolsMapToOpenDOperations(t *testing.T) {
	want := map[string]string{
		"alerts.price.set":        "set",
		"alerts.option_event.set": "set",
		"watchlist.remote.modify": "modify",
	}
	if !maps.Equal(customizationToolActions, want) {
		t.Fatalf("customization tool actions = %v, want %v", customizationToolActions, want)
	}
}

func TestProductToolInputHelpersCompleteBranches(t *testing.T) {
	var decoded struct {
		Value string `json:"value"`
	}
	if err := decodeToolInput(map[string]any{"value": "ok"}, &decoded); err != nil || decoded.Value != "ok" {
		t.Fatalf("decodeToolInput success = %#v, %v", decoded, err)
	}
	if err := decodeToolInput(map[string]any{"bad": make(chan int)}, &decoded); err == nil {
		t.Fatal("decodeToolInput marshaled channel")
	}
	if err := decodeToolInput(map[string]any{"value": "ok"}, nil); err == nil {
		t.Fatal("decodeToolInput decoded into nil")
	}

	if got := toolInstrumentID(map[string]any{"instrumentId": " us.aapl "}); got != "US.AAPL" {
		t.Fatalf("explicit tool instrument = %q", got)
	}
	if got := toolInstrumentID(map[string]any{"market": "us", "symbol": "aapl"}); got != "US.AAPL" {
		t.Fatalf("market symbol instrument = %q", got)
	}
	if got := toolInstrumentID(map[string]any{"symbol": "aapl"}); got != "AAPL" {
		t.Fatalf("bare symbol instrument = %q", got)
	}
	if got := toolMapString(map[string]any{}, "missing"); got != "" {
		t.Fatalf("missing tool string = %q", got)
	}
	if got := toolMapString(map[string]any{"nil": nil}, "nil"); got != "" {
		t.Fatalf("nil tool string = %q", got)
	}
	if got := toolMapString(map[string]any{"number": 12}, "number"); got != "12" {
		t.Fatalf("numeric tool string = %q", got)
	}
	if got := toolMapInt(map[string]any{"page": "25"}, "page", 5); got != 25 {
		t.Fatalf("tool integer = %d", got)
	}
	if got := toolMapInt(map[string]any{"page": "bad"}, "page", 5); got != 5 {
		t.Fatalf("fallback tool integer = %d", got)
	}
	if got := toolMapStrings(map[string]any{
		"symbols": []any{" A ", "", 2},
	}, "symbols"); len(got) != 2 || got[0] != "A" || got[1] != "2" {
		t.Fatalf("interface tool strings = %#v", got)
	}
	direct := []string{"A", "B"}
	gotDirect := toolMapStrings(map[string]any{"symbols": direct}, "symbols")
	if len(gotDirect) != 2 || &gotDirect[0] == &direct[0] {
		t.Fatalf("direct tool strings = %#v", gotDirect)
	}
	if got := toolMapStrings(map[string]any{"symbols": 1}, "symbols"); got != nil {
		t.Fatalf("invalid tool strings = %#v", got)
	}
	cloned := cloneToolInput(map[string]any{"a": 1})
	if cloned["a"] != 1 {
		t.Fatalf("cloned tool input = %#v", cloned)
	}
}

func TestProductAndExecutionDispatchFailureBoundaries(t *testing.T) {
	adapter := NewProductExecutionAdapter(nil, nil)
	if _, err := adapter.InvokeProductTool(t.Context(), "unknown.product.tool", nil); err == nil {
		t.Fatal("unknown product tool succeeded")
	}
	if _, err := adapter.InvokeExecutionTool(t.Context(), "unknown.execution.tool", nil); err == nil {
		t.Fatal("unknown execution tool succeeded")
	}
	for _, name := range []string{
		"execution.order_preview",
		"execution.order_place",
		"execution.combo_preview",
		"execution.combo_place",
	} {
		if _, err := adapter.InvokeExecutionTool(t.Context(), name, map[string]any{
			"invalid": make(chan int),
		}); err == nil {
			t.Errorf("%s accepted unmarshalable input", name)
		}
	}
	if _, err := adapter.productSnapshots(
		t.Context(), map[string]any{}, broker.FeatureMarketSnapshots,
	); err == nil {
		t.Fatal("snapshot tool without symbols succeeded")
	}
	if _, err := adapter.productBuyingPower(t.Context(), map[string]any{
		"invalid": make(chan int),
	}); err == nil {
		t.Fatal("buying-power tool accepted unmarshalable input")
	}
}
