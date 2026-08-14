package assembly

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"testing"

	"github.com/jftrade/jftrade-main/internal/productfeatures"
	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/researchscreen"
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

type typedProductFixture struct {
	ProductFeatureService
	screenQuery   broker.ScreenQueryV2
	calendarQuery productfeatures.CalendarRequest
	screenErr     error
	calendarErr   error
}

func (s *typedProductFixture) QueryScreen(_ context.Context, query broker.ScreenQueryV2) (broker.ResearchScreenResult, error) {
	s.screenQuery = query
	if s.screenErr != nil {
		return broker.ResearchScreenResult{}, s.screenErr
	}
	return broker.ResearchScreenResult{Entries: []broker.ResearchScreenRow{{StockID: "1"}}}, nil
}

func (s *typedProductFixture) QueryCalendar(_ context.Context, query productfeatures.CalendarRequest) (*productfeatures.DocumentResult, error) {
	s.calendarQuery = query
	if s.calendarErr != nil {
		return nil, s.calendarErr
	}
	return &productfeatures.DocumentResult{Entries: []json.RawMessage{}}, nil
}

func (s *typedProductFixture) CapabilitiesContext(context.Context, productfeatures.CapabilityQuery) map[string]any {
	return map[string]any{"ok": true}
}

func (s *typedProductFixture) Query(context.Context, broker.FeatureQuery) (*broker.FeatureResult, error) {
	return &broker.FeatureResult{Entries: []map[string]any{{"ok": true}}}, nil
}

func (s *typedProductFixture) BatchSnapshots(context.Context, broker.FeatureQuery, []string) (*broker.FeatureResult, error) {
	return &broker.FeatureResult{Entries: []map[string]any{{"ok": true}}}, nil
}

func (s *typedProductFixture) ApplyCustomization(context.Context, broker.CustomizationAction) (*broker.CustomizationResult, error) {
	return &broker.CustomizationResult{}, nil
}

func TestProductExecutionAdapterNormalizesScreenAndCalendarV2Inputs(t *testing.T) {
	service := &typedProductFixture{}
	adapter := NewProductExecutionAdapter(service, nil)
	screen := map[string]any{
		"market": "us", "catalogVersion": researchscreen.CatalogVersion,
		"querySchemaVersion": broker.ScreenQuerySchemaVersionV2,
		"pool":               map[string]any{"watchlistStockIds": []string{"1"}},
		"columns":            []any{map[string]any{"columnId": "close", "factor": map[string]any{"factorKey": "simple.last_close"}}},
		"page":               map[string]any{"offset": 2},
	}
	result, err := adapter.InvokeProductTool(t.Context(), "research.screen", screen)
	if err != nil {
		t.Fatalf("research.screen: %v", err)
	}
	projected := result.(broker.ResearchScreenResult)
	if projected.CatalogVersion != researchscreen.CatalogVersion || len(projected.Columns) != 1 || projected.Columns[0].FactorKey != "simple.last_close" || service.screenQuery.Page.Limit != 50 || service.screenQuery.Page.Offset != 2 {
		t.Fatalf("screen projection = %#v query=%#v", projected, service.screenQuery)
	}

	_, err = adapter.InvokeProductTool(t.Context(), "research.calendar", map[string]any{
		"market": "US", "sort": "iv_desc", "stockScope": "optionable",
		"marketCapMin": "100", "optionVolumeMax": "500", "ivMin": "0.2", "ivRankMax": "80", "ivPercentileMin": "50",
	})
	if err != nil {
		t.Fatalf("research.calendar: %v", err)
	}
	if service.calendarQuery.Sort != "iv_desc" || service.calendarQuery.StockScope != "optionable" || service.calendarQuery.MarketCapMin != "100" || service.calendarQuery.OptionVolumeMax != "500" || service.calendarQuery.IVMin != "0.2" || service.calendarQuery.IVRankMax != "80" || service.calendarQuery.IVPercentileMin != "50" {
		t.Fatalf("calendar advanced fields = %#v", service.calendarQuery)
	}
}

func TestProductExecutionAdapterRejectsInvalidScreenPageAndValue(t *testing.T) {
	adapter := NewProductExecutionAdapter(&typedProductFixture{}, nil)
	base := map[string]any{
		"market": "US", "catalogVersion": researchscreen.CatalogVersion,
		"querySchemaVersion": broker.ScreenQuerySchemaVersionV2,
		"pool":               map[string]any{},
	}
	for _, input := range []map[string]any{
		{"market": "US", "catalogVersion": researchscreen.CatalogVersion, "querySchemaVersion": broker.ScreenQuerySchemaVersionV2, "pool": map[string]any{}, "page": map[string]any{"limit": 101}},
		{"market": "US", "catalogVersion": "wrong", "querySchemaVersion": broker.ScreenQuerySchemaVersionV2, "pool": map[string]any{}},
	} {
		if _, err := adapter.InvokeProductTool(t.Context(), "research.screen", input); err == nil {
			t.Fatalf("invalid screen input %#v returned nil error", input)
		}
	}
	if _, err := adapter.productScreen(t.Context(), base, &typedProductFixture{}); err != nil {
		t.Fatalf("base screen normalization: %v", err)
	}
	if err := decodeToolInputValue(map[string]any{"value": "ok"}, &struct {
		Value string `json:"value"`
	}{}); err != nil {
		t.Fatalf("decodeToolInputValue: %v", err)
	}
	if err := decodeToolInputValue(make(chan int), &struct{}{}); err == nil {
		t.Fatal("decodeToolInputValue marshaled channel")
	}
	if err := decodeToolInputValue(map[string]any{"value": "ok"}, nil); err == nil {
		t.Fatal("decodeToolInputValue decoded into nil")
	}
}

func TestProductExecutionAdapterCoversSpecialDispatchFailuresAndSnapshots(t *testing.T) {
	if _, err := NewProductExecutionAdapter(nil, nil).InvokeProductTool(t.Context(), "market.capabilities", nil); err == nil {
		t.Fatal("market.capabilities nil service error = nil")
	}
	fixture := &typedProductFixture{}
	adapter := NewProductExecutionAdapter(fixture, nil)
	for _, name := range []string{"market.snapshot", "market.snapshots"} {
		if _, err := adapter.InvokeProductTool(t.Context(), name, map[string]any{"instrumentId": "US.AAPL"}); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if _, err := adapter.InvokeProductTool(t.Context(), "research.screen", map[string]any{"bad": make(chan int)}); err == nil {
		t.Fatal("research.screen accepted malformed input")
	}
	if _, err := adapter.InvokeProductTool(t.Context(), "research.calendar", nil); err != nil {
		t.Fatalf("research.calendar default input: %v", err)
	}
	plain := &plainProductFixture{}
	if _, err := NewProductExecutionAdapter(plain, nil).InvokeProductTool(t.Context(), "research.screen", map[string]any{}); err == nil {
		t.Fatal("plain product service passed typed screen dispatch")
	}
	if _, err := NewProductExecutionAdapter(nil, nil).InvokeProductTool(t.Context(), "research.screen", nil); err == nil {
		t.Fatal("nil product service passed typed screen dispatch")
	}
	wantScreenErr := errors.New("screen query failed")
	fixture.screenErr = wantScreenErr
	if _, err := adapter.InvokeProductTool(t.Context(), "research.screen", map[string]any{
		"market": "US", "catalogVersion": researchscreen.CatalogVersion,
		"querySchemaVersion": broker.ScreenQuerySchemaVersionV2, "pool": map[string]any{},
	}); !errors.Is(err, wantScreenErr) {
		t.Fatalf("screen query error = %v", err)
	}
	wantCalendarErr := errors.New("calendar query failed")
	fixture.calendarErr = wantCalendarErr
	if _, err := adapter.InvokeProductTool(t.Context(), "research.calendar", nil); !errors.Is(err, wantCalendarErr) {
		t.Fatalf("calendar query error = %v", err)
	}
}

type plainProductFixture struct {
	ProductFeatureService
}

func (plainProductFixture) CapabilitiesContext(context.Context, productfeatures.CapabilityQuery) map[string]any {
	return nil
}

func (plainProductFixture) Query(context.Context, broker.FeatureQuery) (*broker.FeatureResult, error) {
	return &broker.FeatureResult{}, nil
}

func (plainProductFixture) BatchSnapshots(context.Context, broker.FeatureQuery, []string) (*broker.FeatureResult, error) {
	return &broker.FeatureResult{}, nil
}

func (plainProductFixture) ApplyCustomization(context.Context, broker.CustomizationAction) (*broker.CustomizationResult, error) {
	return &broker.CustomizationResult{}, nil
}
