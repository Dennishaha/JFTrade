package rustmigration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/gin-gonic/gin"
	marketdataapi "github.com/jftrade/jftrade-main/internal/api/marketdata"
	srv "github.com/jftrade/jftrade-main/internal/marketdata"
)

const stage9MarketDataCatalogReadFixtureVersion = "stage9.market-data-catalog-read.v1"

type stage9MarketDataCatalogReadCase struct {
	Name           string          `json:"name"`
	Method         string          `json:"method"`
	RequestPath    string          `json:"requestPath"`
	ExpectedStatus int             `json:"expectedStatus"`
	Data           json.RawMessage `json:"data,omitempty"`
	ErrorStatus    int             `json:"errorStatus,omitempty"`
	ErrorCode      string          `json:"errorCode,omitempty"`
	ErrorMessage   string          `json:"errorMessage,omitempty"`
}

type stage9MarketDataCatalogReadFixture struct {
	Version string                            `json:"version"`
	Cases   []stage9MarketDataCatalogReadCase `json:"cases"`
}

// TestStage9MarketDataCatalogReadFixtureMatchesCurrentGoOwner freezes the
// provider-backed markets and instrument-search projections.
func TestStage9MarketDataCatalogReadFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 market-data catalog fixture source")
	}
	fixturePath := filepath.Join(filepath.Dir(source), "../../tests/fixtures/rust-migration/stage9/market-data-catalog-read.json")
	cases := []struct {
		name     string
		path     string
		provider *stage9MarketDataCatalogProvider
	}{
		{name: "markets-ready", path: "/api/v1/market-data/markets", provider: stage9MarketDataCatalogProviderReady()},
		{name: "markets-provider-failure", path: "/api/v1/market-data/markets?fixture=markets-error", provider: stage9MarketDataCatalogProviderReadyWithMarketsError()},
		{name: "markets-descriptor-failure", path: "/api/v1/market-data/markets?fixture=descriptor-error", provider: stage9MarketDataCatalogProviderReadyWithDescriptorError()},
		{name: "instruments-ready", path: "/api/v1/market-data/instruments?query=AAPL&market=US&limit=2", provider: stage9MarketDataCatalogProviderReady()},
		{name: "instruments-invalid-query", path: "/api/v1/market-data/instruments?market=US", provider: stage9MarketDataCatalogProviderReady()},
		{name: "instruments-invalid-limit", path: "/api/v1/market-data/instruments?query=AAPL&limit=101", provider: stage9MarketDataCatalogProviderReady()},
		{name: "instruments-provider-failure", path: "/api/v1/market-data/instruments?query=FAIL", provider: stage9MarketDataCatalogProviderReadyWithSearchError()},
	}
	want := stage9MarketDataCatalogReadFixture{
		Version: stage9MarketDataCatalogReadFixtureVersion,
		Cases:   make([]stage9MarketDataCatalogReadCase, 0, len(cases)),
	}
	for _, testCase := range cases {
		gin.SetMode(gin.TestMode)
		router := gin.New()
		marketdataapi.RegisterRoutes(router.Group("/api/v1"), srv.NewService(testCase.provider))
		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, testCase.path, nil)
		router.ServeHTTP(recorder, request)
		entry := stage9MarketDataCatalogReadCase{
			Name: testCase.name, Method: http.MethodGet, RequestPath: testCase.path, ExpectedStatus: recorder.Code,
		}
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
			entry.ErrorStatus = recorder.Code
			entry.ErrorCode, entry.ErrorMessage = envelope.Error.Code, envelope.Error.Message
		} else {
			entry.Data = compactMarketDataCatalogJSON(envelope.Data)
		}
		want.Cases = append(want.Cases, entry)
	}
	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode market-data catalog fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write market-data catalog fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read market-data catalog fixture: %v", err)
	}
	var got stage9MarketDataCatalogReadFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode market-data catalog fixture: %v", err)
	}
	for index := range got.Cases {
		got.Cases[index].Data = compactMarketDataCatalogJSON(got.Cases[index].Data)
		want.Cases[index].Data = compactMarketDataCatalogJSON(want.Cases[index].Data)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stage 9 market-data catalog fixture drifted from the Go owner: got=%#v want=%#v", got, want)
	}
}

func compactMarketDataCatalogJSON(data json.RawMessage) json.RawMessage {
	if len(data) == 0 {
		return data
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, data); err != nil {
		return data
	}
	return compact.Bytes()
}

type stage9MarketDataCatalogProvider struct {
	descriptor    srv.ProviderDescriptor
	descriptorErr error
	markets       []srv.MarketProfile
	marketsErr    error
	candidates    []srv.InstrumentCandidate
	searchErr     error
}

func stage9MarketDataCatalogProviderReady() *stage9MarketDataCatalogProvider {
	return &stage9MarketDataCatalogProvider{
		descriptor: srv.ProviderDescriptor{
			SelectionID: "futu", ProviderID: "futu-opend", DisplayName: "Futu OpenD", BrokerID: "futu",
			Source: "bbgo:futu", DefaultMarket: "US", SupportedMarkets: []string{"US", "HK"}, Transports: []string{"opend"},
			Capabilities: srv.ProviderCapabilities{InstrumentSearch: true},
			Constraints:  srv.ProviderConstraints{RequiresOpenD: true, RequiresMarketDataRight: true, UsesSubscriptionQuota: true},
			Notes:        []string{"fixture catalog"},
		},
		markets: []srv.MarketProfile{
			{"market": "US", "name": "United States", "timezone": "America/New_York"},
			{"market": "HK", "name": "Hong Kong", "timezone": "Asia/Hong_Kong"},
		},
		candidates: []srv.InstrumentCandidate{{
			Market: "US", ResolvedMarket: "US", InstrumentID: "US.AAPL", Code: "AAPL", Symbol: "AAPL",
			Name: "Apple Inc.", SecurityType: "stock", SupportedPeriods: []string{"1d"}, LotSize: 1,
			Source: "fixture", Selectable: true,
		}},
	}
}

func stage9MarketDataCatalogProviderReadyWithMarketsError() *stage9MarketDataCatalogProvider {
	provider := stage9MarketDataCatalogProviderReady()
	provider.marketsErr = errors.New("market catalog unavailable")
	return provider
}

func stage9MarketDataCatalogProviderReadyWithDescriptorError() *stage9MarketDataCatalogProvider {
	provider := stage9MarketDataCatalogProviderReady()
	provider.descriptorErr = errors.New("provider descriptor unavailable")
	return provider
}

func stage9MarketDataCatalogProviderReadyWithSearchError() *stage9MarketDataCatalogProvider {
	provider := stage9MarketDataCatalogProviderReady()
	provider.searchErr = errors.New("instrument search unavailable")
	return provider
}

func (p *stage9MarketDataCatalogProvider) Descriptor(context.Context) (srv.ProviderDescriptor, error) {
	if p.descriptorErr != nil {
		return srv.ProviderDescriptor{}, p.descriptorErr
	}
	return p.descriptor, nil
}

func (p *stage9MarketDataCatalogProvider) GetMarkets(context.Context) ([]srv.MarketProfile, error) {
	return p.markets, p.marketsErr
}

func (p *stage9MarketDataCatalogProvider) LookupInstrument(context.Context, string, string) ([]srv.InstrumentCandidate, error) {
	return p.candidates, p.searchErr
}

func (p *stage9MarketDataCatalogProvider) SearchInstruments(context.Context, string, int) ([]srv.InstrumentCandidate, error) {
	return p.candidates, p.searchErr
}

func (p *stage9MarketDataCatalogProvider) GetSecurityDetails(context.Context, string, string) (srv.SecurityDetails, error) {
	return nil, nil
}

func (p *stage9MarketDataCatalogProvider) QuerySnapshot(context.Context, string) (*srv.Tick, error) {
	return nil, nil
}

func (p *stage9MarketDataCatalogProvider) QueryTicker(context.Context, string) (*srv.Tick, error) {
	return nil, nil
}

func (p *stage9MarketDataCatalogProvider) GetHistoricalCandles(context.Context, srv.HistoricalCandlesQuery) (srv.CandlesResponse, error) {
	return nil, nil
}

func (p *stage9MarketDataCatalogProvider) GetDepth(context.Context, string, string, int) (srv.DepthResponse, error) {
	return nil, nil
}

func (p *stage9MarketDataCatalogProvider) NormalizeInstrument(context.Context, map[string]any) (map[string]any, error) {
	return nil, nil
}

func (p *stage9MarketDataCatalogProvider) Health(context.Context) (srv.HealthStatus, error) {
	return srv.HealthStatus{Connected: true, Readiness: srv.ProviderReadinessReady}, nil
}
