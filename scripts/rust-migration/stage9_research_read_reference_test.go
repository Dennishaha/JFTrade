package rustmigration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	productfeatures "github.com/jftrade/jftrade-main/internal/api/productfeatures"
	service "github.com/jftrade/jftrade-main/internal/productfeatures"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

const stage9ResearchReadFixtureVersion = "stage9.research-read.v1"

type stage9ResearchReadCase struct {
	Name           string          `json:"name"`
	Method         string          `json:"method"`
	RequestPath    string          `json:"requestPath"`
	ExpectedStatus int             `json:"expectedStatus"`
	Data           json.RawMessage `json:"data,omitempty"`
	ErrorCode      string          `json:"errorCode,omitempty"`
	ErrorMessage   string          `json:"errorMessage,omitempty"`
}

type stage9ResearchReadFixture struct {
	Version string                   `json:"version"`
	Cases   []stage9ResearchReadCase `json:"cases"`
}

// TestStage9ResearchReadFixtureMatchesCurrentGoOwner freezes provider-backed
// research GET projections together. The fixture broker is used only to
// capture the existing HTTP wire; Rust never starts a provider runtime.
func TestStage9ResearchReadFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 research fixture source")
	}
	fixturePath := filepath.Join(filepath.Dir(source), "../../tests/fixtures/rust-migration/stage9/research-read.json")
	gin.SetMode(gin.TestMode)
	adapter := &stage9ResearchBroker{}
	registry := broker.NewRegistry()
	registry.Register(adapter)
	svc := service.NewService(registry, adapter.ID(), nil, nil)
	router := gin.New()
	productfeatures.RegisterRoutes(router.Group("/api/v1"), svc)
	cases := []struct {
		name string
		path string
	}{
		{"instrument", "/api/v1/research/instruments/US.AAPL?brokerId=api-test&operation=profile"},
		{"financials", "/api/v1/research/financials/SH.600519?brokerId=api-test&operation=statements&statement=cashflow"},
		{"valuation", "/api/v1/research/valuation/US.AAPL?brokerId=api-test&operation=valuation"},
		{"analyst", "/api/v1/research/analyst/US.AAPL?brokerId=api-test&operation=consensus"},
		{"ownership", "/api/v1/research/ownership/SH.600519?brokerId=api-test&operation=overview"},
		{"corporate-actions", "/api/v1/research/corporate-actions/US.AAPL?brokerId=api-test"},
		{"short-interest", "/api/v1/research/short-interest/US.AAPL?brokerId=api-test"},
		{"technical-indicators", "/api/v1/research/technical-indicators/US.AAPL?brokerId=api-test&operation=momentum"},
		{"screens", "/api/v1/research/screens?brokerId=api-test&market=US&operation=screen"},
		{"calendars", "/api/v1/research/calendars?brokerId=api-test&market=US&operation=earnings&beginDate=2026-08-01&endDate=2026-08-31"},
		{"macro", "/api/v1/research/macro?brokerId=api-test&market=US&operation=indicators"},
		{"rankings", "/api/v1/research/rankings?brokerId=api-test&market=US&operation=top_movers&direction=up&pageSize=10"},
		{"institutions", "/api/v1/research/institutions?brokerId=api-test&market=US&operation=holding_changes&institutionId=202"},
		{"industries", "/api/v1/research/industries?brokerId=api-test&market=US&operation=plate_list&plateType=concept"},
	}
	want := stage9ResearchReadFixture{Version: stage9ResearchReadFixtureVersion, Cases: make([]stage9ResearchReadCase, 0, len(cases))}
	for _, testCase := range cases {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, testCase.path, nil)
		router.ServeHTTP(recorder, request)
		entry := stage9ResearchReadCase{Name: testCase.name, Method: http.MethodGet, RequestPath: testCase.path, ExpectedStatus: recorder.Code}
		var envelope struct {
			Data  json.RawMessage `json:"data"`
			Error *struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("decode %s response: %v", testCase.name, err)
		}
		if envelope.Error != nil {
			entry.ErrorCode, entry.ErrorMessage = envelope.Error.Code, envelope.Error.Message
		} else {
			entry.Data = normalizeResearchReadData(envelope.Data)
		}
		want.Cases = append(want.Cases, entry)
	}
	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode research fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write research fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read research fixture: %v", err)
	}
	var got stage9ResearchReadFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode research fixture: %v", err)
	}
	for index := range got.Cases {
		got.Cases[index].Data = compactResearchJSON(got.Cases[index].Data)
		want.Cases[index].Data = compactResearchJSON(want.Cases[index].Data)
	}
	if !reflect.DeepEqual(got, want) {
		for index := range want.Cases {
			if !reflect.DeepEqual(got.Cases[index], want.Cases[index]) {
				t.Fatalf("stage 9 research case %s drifted: got=%s want=%s", want.Cases[index].Name, got.Cases[index].Data, want.Cases[index].Data)
			}
		}
		t.Fatalf("stage 9 research read fixture drifted from the Go owner")
	}
}

func normalizeResearchReadData(data json.RawMessage) json.RawMessage {
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		return data
	}
	normalizeResearchTimes(value)
	return mustResearchJSON(value)
}

func normalizeResearchTimes(value map[string]any) {
	for key, item := range value {
		switch typed := item.(type) {
		case string:
			if key == "asOf" || key == "resolvedAt" {
				value[key] = "fixture-time"
			}
		case map[string]any:
			normalizeResearchTimes(typed)
		case []any:
			for _, child := range typed {
				if object, ok := child.(map[string]any); ok {
					normalizeResearchTimes(object)
				}
			}
		}
	}
}

func mustResearchJSON(value any) json.RawMessage {
	contents, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return contents
}

func compactResearchJSON(data json.RawMessage) json.RawMessage {
	if len(data) == 0 {
		return data
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return data
	}
	return mustResearchJSON(value)
}

type stage9ResearchBroker struct{}

func (*stage9ResearchBroker) ID() string { return "api-test" }
func (*stage9ResearchBroker) Descriptor() broker.Descriptor {
	features := make([]broker.FeatureCapability, 0, len(broker.BuiltinCapabilityCatalog.Features))
	for _, definition := range broker.BuiltinCapabilityCatalog.Features {
		features = append(features, broker.FeatureCapability{ID: definition.ID, Markets: []string{"US"}, Access: definition.Access, State: broker.CapabilityAvailable})
	}
	return broker.Descriptor{ID: "api-test", SecurityFirm: "Fixture", Capabilities: []broker.MarketCapability{{Market: "US", Features: features}}}
}
func (*stage9ResearchBroker) DiscoverAccounts(context.Context) ([]broker.Account, error) {
	return nil, nil
}
func (*stage9ResearchBroker) Trading() broker.TradingService      { return nil }
func (*stage9ResearchBroker) MarketData() broker.MarketDataReader { return nil }
func (*stage9ResearchBroker) result(query broker.FeatureQuery) (*broker.FeatureResult, error) {
	return &broker.FeatureResult{Entries: []map[string]any{{"feature": query.FeatureID}}, AsOf: time.Now().UTC()}, nil
}
func (b *stage9ResearchBroker) QueryMarketMicrostructure(_ context.Context, q broker.FeatureQuery) (*broker.FeatureResult, error) {
	return b.result(q)
}
func (b *stage9ResearchBroker) QueryInstrumentProfile(_ context.Context, q broker.FeatureQuery) (*broker.FeatureResult, error) {
	return b.result(q)
}
func (b *stage9ResearchBroker) QueryDerivativeCatalog(_ context.Context, q broker.FeatureQuery) (*broker.FeatureResult, error) {
	return b.result(q)
}
func (b *stage9ResearchBroker) QueryOptionAnalytics(_ context.Context, q broker.FeatureQuery) (*broker.FeatureResult, error) {
	return b.result(q)
}
func (b *stage9ResearchBroker) QueryInstrumentResearch(_ context.Context, q broker.FeatureQuery) (*broker.FeatureResult, error) {
	return b.result(q)
}
func (b *stage9ResearchBroker) QueryMarketResearch(_ context.Context, q broker.FeatureQuery) (*broker.FeatureResult, error) {
	return b.result(q)
}
func (b *stage9ResearchBroker) QueryPredictionMarket(_ context.Context, q broker.FeatureQuery) (*broker.FeatureResult, error) {
	return b.result(q)
}
func (b *stage9ResearchBroker) QueryTechnicalIndicator(_ context.Context, q broker.FeatureQuery) (*broker.FeatureResult, error) {
	return b.result(q)
}
func (b *stage9ResearchBroker) QueryCustomization(_ context.Context, q broker.FeatureQuery) (*broker.FeatureResult, error) {
	return b.result(q)
}
