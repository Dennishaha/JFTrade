package productfeatures

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/researchscreen"
)

func embeddedScreenDefinition() broker.ScreenDefinitionV2 {
	return broker.ScreenDefinitionV2{
		BrokerID:           "yfinance",
		Market:             "US",
		CatalogVersion:     researchscreen.EmbeddedCatalogVersion,
		QuerySchemaVersion: broker.ScreenQuerySchemaVersionV2,
		Conditions: []broker.ScreenCondition{
			{
				Factor:   broker.FactorRef{FactorKey: "simple.price"},
				Operator: "between",
				Value:    map[string]any{"min": 100.0, "max": 300.0},
			},
			{
				Factor:   broker.FactorRef{FactorKey: "simple.pe_ttm"},
				Operator: "between",
				Value:    map[string]any{"max": json.Number("40")},
			},
		},
		Columns: []broker.ScreenColumn{
			{ID: "col-code", Factor: broker.FactorRef{FactorKey: "basic.code"}},
			{ID: "col-name", Factor: broker.FactorRef{FactorKey: "basic.name"}},
			{ID: "col-industry", Factor: broker.FactorRef{FactorKey: "basic.industry"}},
			{ID: "col-price", Factor: broker.FactorRef{FactorKey: "simple.price"}},
			{ID: "col-volume", Factor: broker.FactorRef{FactorKey: "simple.volume"}},
			{ID: "col-pb", Factor: broker.FactorRef{FactorKey: "simple.pb"}},
		},
		Sorts: []broker.ScreenSort{
			{Factor: broker.FactorRef{FactorKey: "simple.market_cap"}, Direction: "desc"},
		},
	}
}

func embeddedScreenFixtures() *embeddedReaderStub {
	industry := "Technology"
	return &embeddedReaderStub{screenResult: marketdata.ScreenResponse{
		Entries: []marketdata.ScreenEntry{
			{
				InstrumentID:  "US.AAPL",
				Name:          "Apple",
				Industry:      &industry,
				QuoteCurrency: "USD",
				Values: map[string]json.Number{
					"simple.price":  json.Number("189.25"),
					"simple.volume": json.Number("1234567"),
				},
			},
			{
				InstrumentID:  "US.MSFT",
				Name:          "Microsoft",
				QuoteCurrency: "USD",
				Values:        map[string]json.Number{},
			},
		},
		Total:   7,
		HasMore: true,
		AsOf:    "2026-08-15T20:00:00Z",
		Source:  "yfinance-screen-us",
	}}
}

func newEmbeddedScreenService(reader *embeddedReaderStub) (*Service, *featureBroker) {
	adapter := researchBrokerAdapter()
	svc := newEmbeddedResearchService(adapter, reader,
		marketdata.ProviderDescriptor{BrokerID: "yfinance", ProviderID: "yahoo-finance"})
	return svc, adapter
}

func TestEmbeddedProviderServesScreenAndProjectsRows(t *testing.T) {
	reader := embeddedScreenFixtures()
	svc, adapter := newEmbeddedScreenService(reader)

	result, err := svc.QueryScreen(t.Context(), broker.ScreenQueryV2{
		ScreenDefinitionV2: embeddedScreenDefinition(),
		Page:               broker.ResearchScreenPagination{Offset: 50, Limit: 25},
	})
	if err != nil {
		t.Fatalf("embedded screen query: %v", err)
	}
	if reader.screenCalls != 1 || adapter.queryCalls != 0 {
		t.Fatalf("reader calls = %d, broker calls = %d", reader.screenCalls, adapter.queryCalls)
	}
	request := reader.screenReq
	if request.Market != "US" || request.Offset != 50 || request.Limit != 25 {
		t.Fatalf("screen request envelope = %#v", request)
	}
	if len(request.Conditions) != 2 ||
		request.Conditions[0].FactorKey != "simple.price" ||
		request.Conditions[0].Min == nil || *request.Conditions[0].Min != "100" ||
		request.Conditions[0].Max == nil || *request.Conditions[0].Max != "300" {
		t.Fatalf("condition[0] = %#v", request.Conditions)
	}
	if request.Conditions[1].Min != nil ||
		request.Conditions[1].Max == nil || *request.Conditions[1].Max != "40" {
		t.Fatalf("condition[1] = %#v", request.Conditions[1])
	}
	if len(request.Sorts) != 1 || request.Sorts[0].FactorKey != "simple.market_cap" ||
		request.Sorts[0].Direction != "desc" {
		t.Fatalf("sorts = %#v", request.Sorts)
	}

	if result.Provider.BrokerID != "yfinance" ||
		result.Provider.SelectionReason != embeddedProviderSelectionReason {
		t.Fatalf("provider attribution = %#v", result.Provider)
	}
	if result.Total == nil || *result.Total != 7 || !result.HasMore ||
		result.NextOffset == nil || *result.NextOffset != 52 {
		t.Fatalf("paging = total %v hasMore %v next %v", result.Total, result.HasMore, result.NextOffset)
	}
	if result.AsOf.UTC().Format("2006-01-02T15:04:05Z") != "2026-08-15T20:00:00Z" {
		t.Fatalf("asOf = %v", result.AsOf)
	}
	if len(result.Entries) != 2 {
		t.Fatalf("entries = %#v", result.Entries)
	}

	row := result.Entries[0]
	if row.StockID != "US.AAPL" || row.InstrumentID != "US.AAPL" || row.Market != "US" ||
		row.Symbol != "AAPL" || row.Name != "Apple" || row.Industry != "Technology" ||
		row.QuoteCurrency != "USD" || row.ProductClass != broker.ProductClassEquity {
		t.Fatalf("row identity = %#v", row)
	}
	if cell := row.Cells["col-code"]; cell.Value.Type != "string" ||
		cell.Value.String == nil || *cell.Value.String != "AAPL" {
		t.Fatalf("col-code cell = %#v", cell)
	}
	if cell := row.Cells["col-industry"]; cell.Value.Type != "string" ||
		cell.Value.String == nil || *cell.Value.String != "Technology" {
		t.Fatalf("col-industry cell = %#v", cell)
	}
	if cell := row.Cells["col-price"]; cell.Value.Type != "number" ||
		cell.Value.Number == nil || *cell.Value.Number != 189.25 ||
		cell.Value.Unit != "currency" {
		t.Fatalf("col-price cell = %#v", cell)
	}
	if cell := row.Cells["col-volume"]; cell.Value.Type != "integer" ||
		cell.Value.Integer == nil || *cell.Value.Integer != 1234567 ||
		cell.Value.Unit != "shares" {
		t.Fatalf("col-volume cell = %#v", cell)
	}
	if cell := row.Cells["col-pb"]; cell.Value.Type != "missing" || cell.Value.Number != nil {
		t.Fatalf("col-pb cell = %#v", cell)
	}

	second := result.Entries[1]
	if second.Industry != "" {
		t.Fatalf("row[1].Industry = %q, want empty", second.Industry)
	}
	if cell := second.Cells["col-industry"]; cell.Value.Type != "missing" {
		t.Fatalf("row[1] col-industry cell = %#v", cell)
	}
}

func TestEmbeddedProviderRejectsFutuCatalogScreenWith409(t *testing.T) {
	reader := embeddedScreenFixtures()
	svc, adapter := newEmbeddedScreenService(reader)
	definition := embeddedScreenDefinition()
	definition.CatalogVersion = researchscreen.CatalogVersion

	_, err := svc.QueryScreen(t.Context(), broker.ScreenQueryV2{
		ScreenDefinitionV2: definition,
		Page:               broker.ResearchScreenPagination{Limit: 25},
	})
	if !errors.Is(err, ErrCapabilityUnavailable) {
		t.Fatalf("futu catalog error = %v, want ErrCapabilityUnavailable", err)
	}
	if reader.screenCalls != 0 || adapter.queryCalls != 0 {
		t.Fatalf("reader calls = %d, broker calls = %d", reader.screenCalls, adapter.queryCalls)
	}
}

func TestEmbeddedProviderRejectsNonExecutableScreenShapes(t *testing.T) {
	reader := embeddedScreenFixtures()
	svc, adapter := newEmbeddedScreenService(reader)

	absolute := embeddedScreenDefinition()
	absolute.Sorts[0].Direction = "abs_desc"
	if _, err := svc.QueryScreen(t.Context(), broker.ScreenQueryV2{
		ScreenDefinitionV2: absolute,
		Page:               broker.ResearchScreenPagination{Limit: 25},
	}); !errors.Is(err, ErrCapabilityUnavailable) {
		t.Fatalf("abs_desc error = %v, want ErrCapabilityUnavailable", err)
	}

	intervals := embeddedScreenDefinition()
	intervals.Conditions[0].Value = map[string]any{
		"intervals": []any{map[string]any{"min": 1.0, "max": 2.0}},
	}
	if _, err := svc.QueryScreen(t.Context(), broker.ScreenQueryV2{
		ScreenDefinitionV2: intervals,
		Page:               broker.ResearchScreenPagination{Limit: 25},
	}); !errors.Is(err, ErrCapabilityUnavailable) {
		t.Fatalf("intervals error = %v, want ErrCapabilityUnavailable", err)
	}
	if reader.screenCalls != 0 || adapter.queryCalls != 0 {
		t.Fatalf("reader calls = %d, broker calls = %d", reader.screenCalls, adapter.queryCalls)
	}
}

func TestEmbeddedProviderMapsScreenCapabilityErrors(t *testing.T) {
	reader := &embeddedReaderStub{screenErr: fmt.Errorf(
		"%w: stock screen market %q", marketdata.ErrCapabilityUnsupported, "HK",
	)}
	svc, _ := newEmbeddedScreenService(reader)
	if _, err := svc.QueryScreen(t.Context(), broker.ScreenQueryV2{
		ScreenDefinitionV2: embeddedScreenDefinition(),
		Page:               broker.ResearchScreenPagination{Limit: 25},
	}); !errors.Is(err, ErrCapabilityUnavailable) {
		t.Fatalf("unsupported market error = %v, want ErrCapabilityUnavailable", err)
	}

	reader = &embeddedReaderStub{screenErr: marketdata.ErrProviderWarming}
	svc, _ = newEmbeddedScreenService(reader)
	if _, err := svc.QueryScreen(t.Context(), broker.ScreenQueryV2{
		ScreenDefinitionV2: embeddedScreenDefinition(),
		Page:               broker.ResearchScreenPagination{Limit: 25},
	}); !errors.Is(err, marketdata.ErrProviderWarming) {
		t.Fatalf("warming error = %v", err)
	}
}

func TestEmbeddedScreenDecodesMapDefinitionAndDefaultsPaging(t *testing.T) {
	reader := embeddedScreenFixtures()
	reader.screenResult.HasMore = false
	svc, _ := newEmbeddedScreenService(reader)

	content, err := json.Marshal(embeddedScreenDefinition())
	if err != nil {
		t.Fatalf("marshal definition: %v", err)
	}
	var asMap map[string]any
	if err := json.Unmarshal(content, &asMap); err != nil {
		t.Fatalf("unmarshal definition: %v", err)
	}
	result, err := svc.Query(t.Context(), broker.FeatureQuery{
		BrokerID: "yfinance", FeatureID: broker.FeatureResearchScreen,
		Params: map[string]any{"researchScreenDefinition": asMap},
	})
	if err != nil {
		t.Fatalf("map definition query: %v", err)
	}
	// PageSize flows through the service default (100) before the facade sees
	// the query; the HTTP layer pins its own default before this point.
	if reader.screenReq.Market != "US" || reader.screenReq.Offset != 0 ||
		reader.screenReq.Limit != 100 {
		t.Fatalf("screen request = %#v", reader.screenReq)
	}
	if result.HasMore == nil || *result.HasMore || result.NextCursor != "" {
		t.Fatalf("hasMore/nextCursor = %v %q", result.HasMore, result.NextCursor)
	}
}

func TestEmbeddedProviderServesUSScreenViaAkshare(t *testing.T) {
	adapter := researchBrokerAdapter()
	reader := embeddedScreenFixtures()
	svc := newEmbeddedResearchService(adapter, reader,
		marketdata.ProviderDescriptor{BrokerID: "akshare", ProviderID: "akshare"})
	definition := embeddedScreenDefinition()
	definition.BrokerID = "akshare"

	result, err := svc.QueryScreen(t.Context(), broker.ScreenQueryV2{
		ScreenDefinitionV2: definition,
		Page:               broker.ResearchScreenPagination{Limit: 25},
	})
	if err != nil {
		t.Fatalf("akshare US screen query: %v", err)
	}
	if reader.screenCalls != 1 || reader.screenReq.Market != "US" || adapter.queryCalls != 0 {
		t.Fatalf("reader calls = %d market %q, broker calls = %d",
			reader.screenCalls, reader.screenReq.Market, adapter.queryCalls)
	}
	if result.Provider.BrokerID != "akshare" || len(result.Entries) != 2 {
		t.Fatalf("result = %#v", result.Provider)
	}
}
