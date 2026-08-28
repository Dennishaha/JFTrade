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
	"time"

	"github.com/gin-gonic/gin"
	marketdataapi "github.com/jftrade/jftrade-main/internal/api/marketdata"
	productfeaturesapi "github.com/jftrade/jftrade-main/internal/api/productfeatures"
	marketsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	productservice "github.com/jftrade/jftrade-main/internal/productfeatures"
	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/shopspring/decimal"
)

const stage9MarketDataQuoteReadFixtureVersion = "stage9.market-data-quote-read.v1"

type stage9MarketDataQuoteReadCase struct {
	Name           string            `json:"name"`
	Method         string            `json:"method"`
	RequestPath    string            `json:"requestPath"`
	ExpectedStatus int               `json:"expectedStatus"`
	Headers        map[string]string `json:"headers,omitempty"`
	Data           json.RawMessage   `json:"data,omitempty"`
	ErrorCode      string            `json:"errorCode,omitempty"`
	ErrorMessage   string            `json:"errorMessage,omitempty"`
}

type stage9MarketDataQuoteReadFixture struct {
	Version string                          `json:"version"`
	Cases   []stage9MarketDataQuoteReadCase `json:"cases"`
}

type stage9MarketDataQuoteReadInput struct {
	name           string
	path           string
	marketProvider *stage9MarketDataQuoteProvider
	featureBroker  *stage9MarketDataQuoteFeatureBroker
}

// TestStage9MarketDataQuoteReadFixtureMatchesCurrentGoOwner freezes the ten
// quote-read GET projections without starting Provider/OpenD or a broker
// runtime. Each case creates a fresh fixture-backed owner so error and cache
// branches cannot leak state across operations.
func TestStage9MarketDataQuoteReadFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve quote-read fixture source")
	}
	fixturePath := filepath.Join(
		filepath.Dir(source),
		"../../tests/fixtures/rust-migration/stage9/market-data-quote-read.json",
	)
	gin.SetMode(gin.TestMode)
	want := stage9MarketDataQuoteReadFixture{
		Version: stage9MarketDataQuoteReadFixtureVersion,
		Cases:   make([]stage9MarketDataQuoteReadCase, 0),
	}
	for _, testCase := range stage9MarketDataQuoteReadInputs() {
		router := stage9MarketDataQuoteReadRouter(testCase)
		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, testCase.path, nil)
		router.ServeHTTP(recorder, request)
		entry := stage9MarketDataQuoteReadCase{
			Name:           testCase.name,
			Method:         http.MethodGet,
			RequestPath:    testCase.path,
			ExpectedStatus: recorder.Code,
		}
		if retryAfter := recorder.Header().Get("Retry-After"); retryAfter != "" {
			entry.Headers = map[string]string{"Retry-After": retryAfter}
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
			entry.ErrorCode = envelope.Error.Code
			entry.ErrorMessage = envelope.Error.Message
		} else {
			entry.Data = normalizeStage9MarketDataQuoteReadData(envelope.Data)
		}
		want.Cases = append(want.Cases, entry)
	}
	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode quote-read fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write quote-read fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read quote-read fixture: %v", err)
	}
	var got stage9MarketDataQuoteReadFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode quote-read fixture: %v", err)
	}
	for index := range got.Cases {
		got.Cases[index].Data = compactStage9MarketDataQuoteReadJSON(got.Cases[index].Data)
		want.Cases[index].Data = compactStage9MarketDataQuoteReadJSON(want.Cases[index].Data)
	}
	if !reflect.DeepEqual(got, want) {
		for index := range want.Cases {
			if !reflect.DeepEqual(got.Cases[index], want.Cases[index]) {
				t.Fatalf(
					"stage 9 quote-read case %s drifted: got=%#v want=%#v",
					want.Cases[index].Name,
					got.Cases[index],
					want.Cases[index],
				)
			}
		}
		t.Fatal("stage 9 market-data quote-read fixture drifted from the Go owner")
	}
}

func stage9MarketDataQuoteReadRouter(input stage9MarketDataQuoteReadInput) *gin.Engine {
	router := gin.New()
	marketProvider := input.marketProvider
	if marketProvider == nil {
		marketProvider = stage9MarketDataQuoteProviderReady()
	}
	marketdataapi.RegisterRoutes(
		router.Group("/api/v1"),
		marketsrv.NewService(marketProvider),
	)
	registry := broker.NewRegistry()
	defaultBroker := ""
	if input.featureBroker != nil {
		registry.Register(input.featureBroker)
		defaultBroker = input.featureBroker.ID()
	}
	productfeaturesapi.RegisterRoutes(
		router.Group("/api/v1"),
		productservice.NewService(registry, defaultBroker, nil, nil),
	)
	return router
}

func stage9MarketDataQuoteReadInputs() []stage9MarketDataQuoteReadInput {
	return []stage9MarketDataQuoteReadInput{
		{
			name:           "subscriptions-empty",
			path:           "/api/v1/market-data/subscriptions",
			marketProvider: stage9MarketDataQuoteProviderReady(),
		},
		{
			name: "securities-ready",
			path: "/api/v1/market-data/securities/US/AAPL",
			marketProvider: stage9MarketDataQuoteProviderWith(func(provider *stage9MarketDataQuoteProvider) {
				provider.security = marketsrv.SecurityDetails{
					"symbol": "US.AAPL", "name": "Fixture Apple", "lotSize": 1,
				}
			}),
		},
		{
			name: "securities-cn-qualified-normalizes-to-leaf",
			path: "/api/v1/market-data/securities/CN/SH.600519",
			marketProvider: stage9MarketDataQuoteProviderWith(func(provider *stage9MarketDataQuoteProvider) {
				provider.security = marketsrv.SecurityDetails{
					"symbol": "SH.600519", "name": "Fixture Moutai", "lotSize": 100,
				}
			}),
		},
		{
			name: "securities-provider-failure",
			path: "/api/v1/market-data/securities/US/FAIL",
			marketProvider: stage9MarketDataQuoteProviderWith(func(provider *stage9MarketDataQuoteProvider) {
				provider.securityErr = errors.New("fixture security unavailable")
			}),
		},
		{
			name:           "snapshots-ready",
			path:           "/api/v1/market-data/snapshots/US/AAPL?refresh=true",
			marketProvider: stage9MarketDataQuoteProviderReady(),
		},
		{
			name:           "snapshots-invalid-refresh",
			path:           "/api/v1/market-data/snapshots/US/AAPL?refresh=maybe",
			marketProvider: stage9MarketDataQuoteProviderReady(),
		},
		{
			name: "snapshots-capability-unsupported",
			path: "/api/v1/market-data/snapshots/US/AAPL",
			marketProvider: stage9MarketDataQuoteProviderWith(func(provider *stage9MarketDataQuoteProvider) {
				provider.snapshotsSupported = false
			}),
		},
		{
			name: "snapshots-provider-warming-retry",
			path: "/api/v1/market-data/snapshots/SH/600519",
			marketProvider: stage9MarketDataQuoteProviderWith(func(provider *stage9MarketDataQuoteProvider) {
				provider.snapshotErr = marketsrv.ErrProviderWarming
			}),
		},
		{
			name: "candles-ready-with-range-and-sessions",
			path: "/api/v1/market-data/candles/US/AAPL?period=5m&limit=2&fromTime=2026-08-01T00:00:00Z&toTime=2026-08-02T00:00:00Z&sessions=regular&sessions=extended",
			marketProvider: stage9MarketDataQuoteProviderWith(func(provider *stage9MarketDataQuoteProvider) {
				provider.candles = marketsrv.CandlesResponse{
					"candles": []map[string]any{{"at": "2026-08-01T13:30:00Z", "open": "100.0", "close": "101.0"}},
					"source":  "fixture-candles", "pagination": map[string]any{"hasMore": false},
				}
			}),
		},
		{
			name: "candles-limit-zero-clamped",
			path: "/api/v1/market-data/candles/US/AAPL?period=1d&limit=0",
			marketProvider: stage9MarketDataQuoteProviderWith(func(provider *stage9MarketDataQuoteProvider) {
				provider.candles = marketsrv.CandlesResponse{
					"candles": []map[string]any{{"at": "2026-08-01T00:00:00Z", "open": "100.0", "close": "101.0"}},
					"source":  "fixture-candles", "pagination": map[string]any{"hasMore": false},
				}
			}),
		},
		{
			name: "candles-limit-overflow-clamped",
			path: "/api/v1/market-data/candles/US/AAPL?period=1d&limit=2000",
			marketProvider: stage9MarketDataQuoteProviderWith(func(provider *stage9MarketDataQuoteProvider) {
				provider.candles = marketsrv.CandlesResponse{
					"candles": []map[string]any{{"at": "2026-08-01T00:00:00Z", "open": "100.0", "close": "101.0"}},
					"source":  "fixture-candles", "pagination": map[string]any{"hasMore": false},
				}
			}),
		},
		{
			name: "candles-limit-negative-clamped",
			path: "/api/v1/market-data/candles/US/AAPL?period=1d&limit=-10",
			marketProvider: stage9MarketDataQuoteProviderWith(func(provider *stage9MarketDataQuoteProvider) {
				provider.candles = marketsrv.CandlesResponse{
					"candles": []map[string]any{{"at": "2026-08-01T00:00:00Z", "open": "100.0", "close": "101.0"}},
					"source":  "fixture-candles", "pagination": map[string]any{"hasMore": false},
				}
			}),
		},
		{
			name: "candles-paged-with-next-before",
			path: "/api/v1/market-data/candles/US/AAPL?period=1d&limit=2&before=2026-08-28T00:00:00Z",
			marketProvider: stage9MarketDataQuoteProviderWith(func(provider *stage9MarketDataQuoteProvider) {
				provider.candles = marketsrv.CandlesResponse{
					"candles": []map[string]any{
						{"at": "2026-08-26T00:00:00Z", "open": "100.0", "close": "101.0"},
						{"at": "2026-08-27T00:00:00Z", "open": "101.0", "close": "102.0"},
					},
					"source":     "fixture-candles",
					"pagination": map[string]any{"hasMore": true, "nextBefore": "2026-08-26T00:00:00Z"},
				}
			}),
		},
		{
			name:           "candles-invalid-sessions",
			path:           "/api/v1/market-data/candles/US/AAPL?period=1d&sessions=bad_session",
			marketProvider: stage9MarketDataQuoteProviderReady(),
		},
		{
			name:           "candles-invalid-before-timestamp",
			path:           "/api/v1/market-data/candles/US/AAPL?period=1d&before=2026-08-28",
			marketProvider: stage9MarketDataQuoteProviderReady(),
		},
		{
			name: "candles-empty-result",
			path: "/api/v1/market-data/candles/HK/00700?period=1d&limit=10",
			marketProvider: stage9MarketDataQuoteProviderWith(func(provider *stage9MarketDataQuoteProvider) {
				provider.candles = marketsrv.CandlesResponse{
					"candles": []map[string]any{}, "source": "fixture-empty",
					"pagination": map[string]any{"hasMore": false},
				}
			}),
		},
		{
			name:           "candles-invalid-period",
			path:           "/api/v1/market-data/candles/US/AAPL?period=bad",
			marketProvider: stage9MarketDataQuoteProviderReady(),
		},
		{
			name: "candles-provider-failure",
			path: "/api/v1/market-data/candles/US/FAIL?period=1m",
			marketProvider: stage9MarketDataQuoteProviderWith(func(provider *stage9MarketDataQuoteProvider) {
				provider.candlesErr = errors.New("fixture candles unavailable")
			}),
		},
		{
			name: "depth-ready",
			path: "/api/v1/market-data/depth/US/AAPL?num=2",
			marketProvider: stage9MarketDataQuoteProviderWith(func(provider *stage9MarketDataQuoteProvider) {
				provider.depth = marketsrv.DepthResponse{
					"symbol": "US.AAPL",
					"bids":   []map[string]any{{"price": "100.0", "volume": "10"}},
					"asks":   []map[string]any{{"price": "101.0", "volume": "12"}},
				}
			}),
		},
		{
			name:           "depth-invalid-num",
			path:           "/api/v1/market-data/depth/US/AAPL?num=bad",
			marketProvider: stage9MarketDataQuoteProviderReady(),
		},
		{
			name: "depth-provider-busy-retry",
			path: "/api/v1/market-data/depth/SZ/000001",
			marketProvider: stage9MarketDataQuoteProviderWith(func(provider *stage9MarketDataQuoteProvider) {
				provider.depthErr = marketsrv.ErrProviderBusy
			}),
		},
		{
			name:          "profile-ready",
			path:          "/api/v1/market-data/instruments/US.AAPL/profile?brokerId=api-test&market=US&pageSize=5",
			featureBroker: stage9MarketDataQuoteFeatureBrokerReady(),
		},
		{
			name:          "intraday-ready",
			path:          "/api/v1/market-data/intraday/US.AAPL?brokerId=api-test&market=US&pageSize=3",
			featureBroker: stage9MarketDataQuoteFeatureBrokerReady(),
		},
		{
			name:          "ticks-ready",
			path:          "/api/v1/market-data/ticks/US.AAPL?brokerId=api-test&market=US&pageSize=3",
			featureBroker: stage9MarketDataQuoteFeatureBrokerReady(),
		},
		{
			name:          "broker-queue-ready",
			path:          "/api/v1/market-data/broker-queue/US.AAPL?brokerId=api-test&market=US",
			featureBroker: stage9MarketDataQuoteFeatureBrokerReady(),
		},
		{
			name:          "capital-flow-ready",
			path:          "/api/v1/market-data/capital-flow/US.AAPL?brokerId=api-test&market=US",
			featureBroker: stage9MarketDataQuoteFeatureBrokerReady(),
		},
		{
			name: "intraday-capability-unavailable",
			path: "/api/v1/market-data/intraday/US.AAPL?brokerId=missing&market=US",
		},
		{
			name:          "ticks-provider-failure",
			path:          "/api/v1/market-data/ticks/US.AAPL?brokerId=api-test&market=US",
			featureBroker: stage9MarketDataQuoteFeatureBrokerWith(errors.New("fixture ticks unavailable")),
		},
		{
			name:          "broker-queue-provider-warming-retry",
			path:          "/api/v1/market-data/broker-queue/US.AAPL?brokerId=api-test&market=US",
			featureBroker: stage9MarketDataQuoteFeatureBrokerWith(marketsrv.ErrProviderWarming),
		},
	}
}

func normalizeStage9MarketDataQuoteReadData(data json.RawMessage) json.RawMessage {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return data
	}
	normalizeStage9MarketDataQuoteReadTimes(value)
	contents, err := json.Marshal(value)
	if err != nil {
		return data
	}
	return contents
}

func normalizeStage9MarketDataQuoteReadTimes(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if key == "asOf" || key == "resolvedAt" || key == "observedAt" || key == "quoteAt" {
				if _, ok := child.(string); ok {
					typed[key] = "fixture-time"
					continue
				}
			}
			normalizeStage9MarketDataQuoteReadTimes(child)
		}
	case []any:
		for _, child := range typed {
			normalizeStage9MarketDataQuoteReadTimes(child)
		}
	}
}

func compactStage9MarketDataQuoteReadJSON(data json.RawMessage) json.RawMessage {
	if len(data) == 0 {
		return data
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return data
	}
	contents, err := json.Marshal(value)
	if err != nil {
		return data
	}
	return contents
}

type stage9MarketDataQuoteProvider struct {
	stage9MarketDataNewsActionsBaseProvider
	snapshotsSupported bool
	depthSupported     bool
	security           marketsrv.SecurityDetails
	securityErr        error
	snapshotErr        error
	candles            marketsrv.CandlesResponse
	candlesErr         error
	depth              marketsrv.DepthResponse
	depthErr           error
}

func stage9MarketDataQuoteProviderReady() *stage9MarketDataQuoteProvider {
	return stage9MarketDataQuoteProviderWith(nil)
}

func stage9MarketDataQuoteProviderWith(
	configure func(*stage9MarketDataQuoteProvider),
) *stage9MarketDataQuoteProvider {
	provider := &stage9MarketDataQuoteProvider{
		stage9MarketDataNewsActionsBaseProvider: stage9MarketDataNewsActionsBaseProvider{
			providerID: "fixture-quote",
		},
		snapshotsSupported: true,
		depthSupported:     true,
		security: marketsrv.SecurityDetails{
			"symbol": "US.AAPL", "name": "Fixture Apple", "lotSize": 1,
		},
		candles: marketsrv.CandlesResponse{
			"candles": []map[string]any{{"at": "2026-08-01T13:30:00Z", "close": "101.0"}},
			"source":  "fixture-candles", "pagination": map[string]any{"hasMore": false},
		},
		depth: marketsrv.DepthResponse{
			"symbol": "US.AAPL", "bids": []map[string]any{}, "asks": []map[string]any{},
		},
	}
	if configure != nil {
		configure(provider)
	}
	return provider
}

func (p *stage9MarketDataQuoteProvider) Descriptor(context.Context) (marketsrv.ProviderDescriptor, error) {
	return marketsrv.ProviderDescriptor{
		SelectionID:   "futu",
		ProviderID:    p.providerID,
		DisplayName:   "Fixture Quote Provider",
		BrokerID:      "futu",
		Source:        "fixture",
		DefaultMarket: "US",
		Capabilities: marketsrv.ProviderCapabilities{
			Snapshots:         p.snapshotsSupported,
			HistoricalCandles: true,
			TickCandles:       true,
			OrderBookDepth:    p.depthSupported,
		},
	}, nil
}

func (p *stage9MarketDataQuoteProvider) GetSecurityDetails(
	context.Context,
	string,
	string,
) (marketsrv.SecurityDetails, error) {
	return p.security, p.securityErr
}

func (p *stage9MarketDataQuoteProvider) QuerySnapshot(
	context.Context,
	string,
) (*marketsrv.Tick, error) {
	return stage9MarketDataQuoteTick(), p.snapshotErr
}

func (p *stage9MarketDataQuoteProvider) GetHistoricalCandles(
	context.Context,
	marketsrv.HistoricalCandlesQuery,
) (marketsrv.CandlesResponse, error) {
	return p.candles, p.candlesErr
}

func (p *stage9MarketDataQuoteProvider) GetDepth(
	context.Context,
	string,
	string,
	int,
) (marketsrv.DepthResponse, error) {
	return p.depth, p.depthErr
}

func stage9MarketDataQuoteTick() *marketsrv.Tick {
	return &marketsrv.Tick{
		InstrumentID:       "US.AAPL",
		Market:             "US",
		Symbol:             "AAPL",
		Price:              stage9MarketDataQuoteDecimal("101.25"),
		Bid:                stage9MarketDataQuoteDecimal("101.20"),
		Ask:                stage9MarketDataQuoteDecimal("101.30"),
		OpenPrice:          stage9MarketDataQuoteDecimalPointer("100.00"),
		HighPrice:          stage9MarketDataQuoteDecimalPointer("102.00"),
		LowPrice:           stage9MarketDataQuoteDecimalPointer("99.00"),
		PreviousClosePrice: stage9MarketDataQuoteDecimalPointer("100.50"),
		LastClosePrice:     stage9MarketDataQuoteDecimalPointer("100.50"),
		Volume:             stage9MarketDataQuoteDecimal("1000"),
		Turnover:           stage9MarketDataQuoteDecimal("101250"),
		QuoteAt:            "2026-08-22T13:30:00Z",
		ObservedAt:         "2026-08-22T13:30:00Z",
		Source:             "fixture-quote",
		Session:            "regular",
	}
}

func stage9MarketDataQuoteDecimal(value string) decimal.Decimal {
	result, err := decimal.NewFromString(value)
	if err != nil {
		panic(err)
	}
	return result
}

func stage9MarketDataQuoteDecimalPointer(value string) *decimal.Decimal {
	result := stage9MarketDataQuoteDecimal(value)
	return &result
}

type stage9MarketDataQuoteFeatureBroker struct {
	*stage9ResearchBroker
	queryErr error
}

func stage9MarketDataQuoteFeatureBrokerReady() *stage9MarketDataQuoteFeatureBroker {
	return stage9MarketDataQuoteFeatureBrokerWith(nil)
}

func stage9MarketDataQuoteFeatureBrokerWith(err error) *stage9MarketDataQuoteFeatureBroker {
	return &stage9MarketDataQuoteFeatureBroker{
		stage9ResearchBroker: &stage9ResearchBroker{},
		queryErr:             err,
	}
}

func (b *stage9MarketDataQuoteFeatureBroker) QueryMarketMicrostructure(
	ctx context.Context,
	query broker.FeatureQuery,
) (*broker.FeatureResult, error) {
	if b.queryErr != nil {
		return nil, b.queryErr
	}
	return b.stage9ResearchBroker.QueryMarketMicrostructure(ctx, query)
}

func (b *stage9MarketDataQuoteFeatureBroker) QueryInstrumentProfile(
	ctx context.Context,
	query broker.FeatureQuery,
) (*broker.FeatureResult, error) {
	if b.queryErr != nil {
		return nil, b.queryErr
	}
	return b.stage9ResearchBroker.QueryInstrumentProfile(ctx, query)
}

// Keep the compiler checking that the fixture adapters remain valid providers
// and brokers as their public contracts evolve.
var (
	_ marketsrv.Provider = (*stage9MarketDataQuoteProvider)(nil)
	_ broker.Broker      = (*stage9MarketDataQuoteFeatureBroker)(nil)
)

var _ = time.UTC
