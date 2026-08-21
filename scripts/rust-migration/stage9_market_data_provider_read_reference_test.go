package rustmigration

import (
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

	marketdataapi "github.com/jftrade/jftrade-main/internal/api/marketdata"
	srv "github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/gin-gonic/gin"
)

const stage9MarketDataProviderReadFixtureVersion = "stage9.market-data-provider-read.v1"

type stage9MarketDataProviderReadCase struct {
	Name           string          `json:"name"`
	Method         string          `json:"method"`
	RequestPath    string          `json:"requestPath"`
	ExpectedStatus int             `json:"expectedStatus"`
	Data           json.RawMessage `json:"data,omitempty"`
	ErrorCode      string          `json:"errorCode,omitempty"`
	ErrorMessage   string          `json:"errorMessage,omitempty"`
}

type stage9MarketDataProviderReadFixture struct {
	Version string                              `json:"version"`
	Cases   []stage9MarketDataProviderReadCase `json:"cases"`
}

// TestStage9MarketDataProviderReadFixtureMatchesCurrentGoOwner freezes the
// provider descriptor, health fallback, runtime, and subscription projection.
func TestStage9MarketDataProviderReadFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 market-data provider fixture source")
	}
	fixturePath := filepath.Join(filepath.Dir(source), "../../tests/fixtures/rust-migration/stage9/market-data-provider-read.json")
	cases := []struct {
		name    string
		path    string
		service *srv.Service
	}{
		{name: "provider-ready", path: "/api/v1/market-data/provider", service: stage9ProviderStatusService(nil, nil)},
		{name: "provider-degraded", path: "/api/v1/market-data/provider?fixture=degraded", service: stage9ProviderStatusService(nil, errors.New("provider warming"))},
		{name: "provider-failed", path: "/api/v1/market-data/provider?fixture=error", service: stage9ProviderStatusService(errors.New("provider unavailable"), nil)},
	}
	want := stage9MarketDataProviderReadFixture{
		Version: stage9MarketDataProviderReadFixtureVersion,
		Cases:   make([]stage9MarketDataProviderReadCase, 0, len(cases)),
	}
	for _, testCase := range cases {
		gin.SetMode(gin.TestMode)
		router := gin.New()
		marketdataapi.RegisterRoutes(router.Group("/api/v1"), testCase.service)
		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, testCase.path, nil)
		router.ServeHTTP(recorder, request)
		entry := stage9MarketDataProviderReadCase{
			Name: testCase.name, Method: http.MethodGet, RequestPath: testCase.path, ExpectedStatus: recorder.Code,
		}
		var envelope struct {
			Data  json.RawMessage                 `json:"data"`
			Error *struct{ Code, Message string } `json:"error"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("decode %s response: %v", testCase.name, err)
		}
		if envelope.Error != nil {
			entry.ErrorCode, entry.ErrorMessage = envelope.Error.Code, envelope.Error.Message
		} else {
			entry.Data = normalizeMarketDataProviderFixtureData(envelope.Data)
		}
		want.Cases = append(want.Cases, entry)
	}
	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode market-data provider fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write market-data provider fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read market-data provider fixture: %v", err)
	}
	var got stage9MarketDataProviderReadFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode market-data provider fixture: %v", err)
	}
	for index := range got.Cases {
		got.Cases[index].Data = normalizeMarketDataProviderFixtureData(got.Cases[index].Data)
		want.Cases[index].Data = normalizeMarketDataProviderFixtureData(want.Cases[index].Data)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stage 9 market-data provider fixture drifted from the Go owner: got=%#v want=%#v", got, want)
	}
}

func normalizeMarketDataProviderFixtureData(data json.RawMessage) json.RawMessage {
	if len(data) == 0 {
		return data
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return data
	}
	if object, ok := value.(map[string]any); ok {
		if _, exists := object["checkedAt"]; exists {
			object["checkedAt"] = "fixture-time"
		}
	}
	contents, err := json.Marshal(value)
	if err != nil {
		return data
	}
	return contents
}

func stage9ProviderStatusService(descriptorErr, healthErr error) *srv.Service {
	return srv.NewService(&stage9ProviderStatusProvider{
		descriptorErr: descriptorErr,
		healthErr:     healthErr,
	})
}

type stage9ProviderStatusProvider struct {
	descriptorErr error
	healthErr     error
}

func (p *stage9ProviderStatusProvider) Descriptor(context.Context) (srv.ProviderDescriptor, error) {
	if p.descriptorErr != nil {
		return srv.ProviderDescriptor{}, p.descriptorErr
	}
	return srv.ProviderDescriptor{
		SelectionID:      "futu",
		ProviderID:       "futu-opend",
		DisplayName:      "Futu OpenD",
		BrokerID:         "futu",
		Source:           "bbgo:futu",
		DefaultMarket:    "HK",
		SupportedMarkets: []string{"HK", "US"},
		Transports:       []string{"opend"},
		Capabilities: srv.ProviderCapabilities{
			Snapshots: true, StreamingQuotes: true, StreamingCandles: true,
			StreamingDepth: true, HistoricalCandles: true, TickCandles: true,
			OrderBookDepth: true, InstrumentSearch: true, ExtendedHours: true,
			CandleIntervals: []string{"1m", "1d"}, OrderBookLevels: []int{5, 10},
			Sessions: []string{"regular", "extended"}, PriceAdjustments: []string{"none"},
			HistoricalLookbackDays: map[string]int{"1d": 3650},
		},
		Constraints: srv.ProviderConstraints{
			RequiresOpenD: true, RequiresMarketDataRight: true, UsesSubscriptionQuota: true,
		},
		Notes: []string{"fixture provider"},
	}, nil
}

func (p *stage9ProviderStatusProvider) GetMarkets(context.Context) ([]srv.MarketProfile, error) {
	return nil, nil
}

func (p *stage9ProviderStatusProvider) GetSecurityDetails(context.Context, string, string) (srv.SecurityDetails, error) {
	return nil, nil
}

func (p *stage9ProviderStatusProvider) LookupInstrument(context.Context, string, string) ([]srv.InstrumentCandidate, error) {
	return nil, nil
}

func (p *stage9ProviderStatusProvider) SearchInstruments(context.Context, string, int) ([]srv.InstrumentCandidate, error) {
	return nil, nil
}

func (p *stage9ProviderStatusProvider) QuerySnapshot(context.Context, string) (*srv.Tick, error) {
	return nil, nil
}

func (p *stage9ProviderStatusProvider) QueryTicker(context.Context, string) (*srv.Tick, error) {
	return nil, nil
}

func (p *stage9ProviderStatusProvider) GetHistoricalCandles(context.Context, srv.HistoricalCandlesQuery) (srv.CandlesResponse, error) {
	return nil, nil
}

func (p *stage9ProviderStatusProvider) GetDepth(context.Context, string, string, int) (srv.DepthResponse, error) {
	return nil, nil
}

func (p *stage9ProviderStatusProvider) NormalizeInstrument(context.Context, map[string]any) (map[string]any, error) {
	return nil, nil
}

func (p *stage9ProviderStatusProvider) Health(context.Context) (srv.HealthStatus, error) {
	if p.healthErr != nil {
		return srv.HealthStatus{}, p.healthErr
	}
	return srv.HealthStatus{Connected: true, Readiness: srv.ProviderReadinessReady}, nil
}
