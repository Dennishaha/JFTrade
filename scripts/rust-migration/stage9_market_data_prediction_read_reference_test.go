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
	productfeatures "github.com/jftrade/jftrade-main/internal/api/productfeatures"
	marketdatasrv "github.com/jftrade/jftrade-main/internal/marketdata"
	service "github.com/jftrade/jftrade-main/internal/productfeatures"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

const stage9MarketDataPredictionReadFixtureVersion = "stage9.market-data-prediction-read.v1"

type stage9MarketDataPredictionReadCase struct {
	Name           string                                  `json:"name"`
	Method         string                                  `json:"method"`
	RequestPath    string                                  `json:"requestPath"`
	ExpectedStatus int                                     `json:"expectedStatus"`
	Headers        map[string]string                       `json:"headers,omitempty"`
	Data           json.RawMessage                         `json:"data,omitempty"`
	ErrorCode      string                                  `json:"errorCode,omitempty"`
	ErrorMessage   string                                  `json:"errorMessage,omitempty"`
	ProviderCall   *stage9MarketDataPredictionProviderCall `json:"providerCall,omitempty"`
}

type stage9MarketDataPredictionReadFixture struct {
	Version string                               `json:"version"`
	Cases   []stage9MarketDataPredictionReadCase `json:"cases"`
}

// stage9MarketDataPredictionProviderCall is fixture evidence for the
// broker-neutral query after Go route binding and service normalization. It is
// not part of the public HTTP response.
type stage9MarketDataPredictionProviderCall struct {
	FeatureID     string         `json:"featureId"`
	Market        string         `json:"market"`
	MarketSegment string         `json:"marketSegment"`
	ProductClass  string         `json:"productClass"`
	InstrumentID  string         `json:"instrumentId"`
	PageSize      int            `json:"pageSize"`
	Params        map[string]any `json:"params,omitempty"`
}

type stage9MarketDataPredictionReadInput struct {
	name     string
	path     string
	router   *gin.Engine
	provider *stage9MarketDataPredictionBroker
}

// TestStage9MarketDataPredictionReadFixtureMatchesCurrentGoOwner freezes all
// twelve prediction-market GET projections without starting Provider/OpenD or
// creating a subscription.
func TestStage9MarketDataPredictionReadFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 market-data prediction fixture source")
	}
	fixturePath := filepath.Join(
		filepath.Dir(source),
		"../../tests/fixtures/rust-migration/stage9/market-data-prediction-read.json",
	)
	gin.SetMode(gin.TestMode)
	inputs := stage9MarketDataPredictionReadInputs()
	want := stage9MarketDataPredictionReadFixture{
		Version: stage9MarketDataPredictionReadFixtureVersion,
		Cases:   make([]stage9MarketDataPredictionReadCase, 0, len(inputs)),
	}
	for _, testCase := range inputs {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, testCase.path, nil)
		testCase.router.ServeHTTP(recorder, request)
		entry := stage9MarketDataPredictionReadCase{
			Name:           testCase.name,
			Method:         http.MethodGet,
			RequestPath:    testCase.path,
			ExpectedStatus: recorder.Code,
		}
		if testCase.provider != nil {
			entry.ProviderCall = testCase.provider.providerCall()
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
			entry.Data = normalizeStage9MarketDataPredictionJSON(envelope.Data)
		}
		want.Cases = append(want.Cases, entry)
	}
	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode market-data prediction fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write market-data prediction fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read market-data prediction fixture: %v", err)
	}
	var got stage9MarketDataPredictionReadFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode market-data prediction fixture: %v", err)
	}
	for index := range got.Cases {
		got.Cases[index].Data = compactStage9MarketDataPredictionJSON(got.Cases[index].Data)
		want.Cases[index].Data = compactStage9MarketDataPredictionJSON(want.Cases[index].Data)
	}
	if !reflect.DeepEqual(got, want) {
		for index := range want.Cases {
			if !reflect.DeepEqual(got.Cases[index], want.Cases[index]) {
				t.Fatalf(
					"stage 9 market-data prediction case %s drifted: got=%#v want=%#v",
					want.Cases[index].Name,
					got.Cases[index],
					want.Cases[index],
				)
			}
		}
		t.Fatal("stage 9 market-data prediction fixture drifted from the Go owner")
	}
}

func stage9MarketDataPredictionReadInputs() []stage9MarketDataPredictionReadInput {
	ready := stage9MarketDataPredictionReadBrokerWith(func(broker *stage9MarketDataPredictionBroker) {
		broker.accounts = stage9MarketDataPredictionEligibleAccounts()
	})
	empty := stage9MarketDataPredictionReadBrokerWith(func(broker *stage9MarketDataPredictionBroker) {
		broker.accounts = stage9MarketDataPredictionEligibleAccounts()
		broker.empty = true
	})
	ineligible := stage9MarketDataPredictionReadBrokerWith(func(fixtureBroker *stage9MarketDataPredictionBroker) {
		firm := "OTHER"
		fixtureBroker.accounts = []broker.Account{{
			ID: "ineligible-account", SecurityFirm: &firm, MarketAuthorities: []string{"US"},
		}}
	})
	failed := stage9MarketDataPredictionReadBrokerWith(func(broker *stage9MarketDataPredictionBroker) {
		broker.accounts = stage9MarketDataPredictionEligibleAccounts()
		broker.resultErr = errors.New("fixture prediction provider failed")
	})
	warming := stage9MarketDataPredictionReadBrokerWith(func(broker *stage9MarketDataPredictionBroker) {
		broker.accounts = stage9MarketDataPredictionEligibleAccounts()
		broker.resultErr = marketdatasrv.ErrProviderWarming
	})
	busy := stage9MarketDataPredictionReadBrokerWith(func(broker *stage9MarketDataPredictionBroker) {
		broker.accounts = stage9MarketDataPredictionEligibleAccounts()
		broker.resultErr = marketdatasrv.ErrProviderBusy
	})
	missingRouter := stage9MarketDataPredictionReadRouter(nil)
	return []stage9MarketDataPredictionReadInput{
		{name: "categories-success", path: "/api/v1/market-data/prediction/categories?brokerId=api-test&market=US&operation=categories&pageSize=25&active=true", router: stage9MarketDataPredictionReadRouter(ready), provider: ready},
		{name: "combo-eligible-events-success", path: "/api/v1/market-data/prediction/combos/eligible-events?brokerId=api-test&market=US&operation=eligible_events&seriesId=SERIES-1", router: stage9MarketDataPredictionReadRouter(ready), provider: ready},
		{name: "competitions-success", path: "/api/v1/market-data/prediction/competitions?brokerId=api-test&market=US&operation=competitions&status=open", router: stage9MarketDataPredictionReadRouter(ready), provider: ready},
		{name: "candles-success", path: "/api/v1/market-data/prediction/contracts/US.EC-42/candles?brokerId=api-test&market=US&operation=candles&period=1m&pageSize=5", router: stage9MarketDataPredictionReadRouter(ready), provider: ready},
		{name: "historical-candles-success", path: "/api/v1/market-data/prediction/contracts/US.EC-42/candles/history?brokerId=api-test&market=US&operation=historical&from=2026-08-01T00%3A00%3A00Z&to=2026-08-02T00%3A00%3A00Z", router: stage9MarketDataPredictionReadRouter(ready), provider: ready},
		{name: "milestones-success", path: "/api/v1/market-data/prediction/contracts/US.EC-42/milestones?brokerId=api-test&market=US&operation=milestones&includeResolved=false", router: stage9MarketDataPredictionReadRouter(ready), provider: ready},
		{name: "order-book-success", path: "/api/v1/market-data/prediction/contracts/US.EC-42/order-book?brokerId=api-test&market=US&operation=order_book&depth=10", router: stage9MarketDataPredictionReadRouter(ready), provider: ready},
		{name: "snapshot-success", path: "/api/v1/market-data/prediction/contracts/US.EC-42/snapshot?brokerId=api-test&market=US&operation=snapshot&refresh=true", router: stage9MarketDataPredictionReadRouter(ready), provider: ready},
		{name: "ticks-success", path: "/api/v1/market-data/prediction/contracts/US.EC-42/ticks?brokerId=api-test&market=US&operation=ticks&pageSize=20", router: stage9MarketDataPredictionReadRouter(ready), provider: ready},
		{name: "events-success", path: "/api/v1/market-data/prediction/events?brokerId=api-test&market=US&operation=events&status=open", router: stage9MarketDataPredictionReadRouter(ready), provider: ready},
		{name: "event-contracts-success", path: "/api/v1/market-data/prediction/events/EVENT-42/contracts?brokerId=api-test&market=US&operation=contracts", router: stage9MarketDataPredictionReadRouter(ready), provider: ready},
		{name: "series-success", path: "/api/v1/market-data/prediction/series?brokerId=api-test&market=US&operation=series&category=macro", router: stage9MarketDataPredictionReadRouter(ready), provider: ready},
		{name: "events-empty-result", path: "/api/v1/market-data/prediction/events?brokerId=api-test&market=US&operation=events&empty=true", router: stage9MarketDataPredictionReadRouter(empty), provider: empty},
		{name: "categories-ineligible-account", path: "/api/v1/market-data/prediction/categories?brokerId=api-test&market=US", router: stage9MarketDataPredictionReadRouter(ineligible), provider: ineligible},
		{name: "categories-capability-unavailable", path: "/api/v1/market-data/prediction/categories?brokerId=missing&market=US", router: missingRouter},
		{name: "snapshot-provider-failure", path: "/api/v1/market-data/prediction/contracts/US.EC-42/snapshot?brokerId=api-test&market=US", router: stage9MarketDataPredictionReadRouter(failed), provider: failed},
		{name: "events-provider-warming", path: "/api/v1/market-data/prediction/events?brokerId=api-test&market=US", router: stage9MarketDataPredictionReadRouter(warming), provider: warming},
		{name: "series-provider-busy", path: "/api/v1/market-data/prediction/series?brokerId=api-test&market=US", router: stage9MarketDataPredictionReadRouter(busy), provider: busy},
	}
}

func stage9MarketDataPredictionReadRouter(
	adapter *stage9MarketDataPredictionBroker,
) *gin.Engine {
	registry := broker.NewRegistry()
	if adapter != nil {
		registry.Register(adapter)
	}
	defaultBroker := ""
	if adapter != nil {
		defaultBroker = adapter.ID()
	}
	router := gin.New()
	productfeatures.RegisterRoutes(
		router.Group("/api/v1"),
		service.NewService(registry, defaultBroker, nil, nil),
	)
	return router
}

func normalizeStage9MarketDataPredictionJSON(data json.RawMessage) json.RawMessage {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return data
	}
	normalizeStage9MarketDataPredictionTimes(value)
	contents, err := json.Marshal(value)
	if err != nil {
		return data
	}
	return contents
}

func normalizeStage9MarketDataPredictionTimes(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if key == "asOf" || key == "resolvedAt" {
				if text, ok := child.(string); ok && text != "" {
					typed[key] = "fixture-time"
					continue
				}
			}
			normalizeStage9MarketDataPredictionTimes(child)
		}
	case []any:
		for _, child := range typed {
			normalizeStage9MarketDataPredictionTimes(child)
		}
	}
}

func compactStage9MarketDataPredictionJSON(data json.RawMessage) json.RawMessage {
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

func stage9MarketDataPredictionEligibleAccounts() []broker.Account {
	firm := "FUTUINC"
	return []broker.Account{{
		ID: "prediction-account", SecurityFirm: &firm, MarketAuthorities: []string{"US"},
	}}
}

type stage9MarketDataPredictionBroker struct {
	accounts  []broker.Account
	resultErr error
	empty     bool
	lastQuery *broker.FeatureQuery
}

func stage9MarketDataPredictionReadBrokerWith(
	configure func(*stage9MarketDataPredictionBroker),
) *stage9MarketDataPredictionBroker {
	adapter := &stage9MarketDataPredictionBroker{}
	configure(adapter)
	return adapter
}

func (b *stage9MarketDataPredictionBroker) ID() string { return "api-test" }

func (b *stage9MarketDataPredictionBroker) Descriptor() broker.Descriptor {
	features := make([]broker.FeatureCapability, 0, len(broker.BuiltinCapabilityCatalog.Features))
	for _, definition := range broker.BuiltinCapabilityCatalog.Features {
		features = append(features, broker.FeatureCapability{
			ID: definition.ID, Markets: []string{"US"}, Access: definition.Access, State: broker.CapabilityAvailable,
		})
	}
	return broker.Descriptor{
		ID: b.ID(), SecurityFirm: "Fixture",
		Capabilities: []broker.MarketCapability{{Market: "US", Features: features}},
	}
}

func (b *stage9MarketDataPredictionBroker) DiscoverAccounts(context.Context) ([]broker.Account, error) {
	return b.accounts, nil
}

func (*stage9MarketDataPredictionBroker) Trading() broker.TradingService      { return nil }
func (*stage9MarketDataPredictionBroker) MarketData() broker.MarketDataReader { return nil }

func (b *stage9MarketDataPredictionBroker) QueryPredictionMarket(
	_ context.Context,
	query broker.FeatureQuery,
) (*broker.FeatureResult, error) {
	b.lastQuery = &query
	if b.resultErr != nil {
		return nil, b.resultErr
	}
	if b.empty {
		return &broker.FeatureResult{Entries: []map[string]any{}, AsOf: time.Now().UTC()}, nil
	}
	return &broker.FeatureResult{
		Entries: []map[string]any{{
			"feature":      string(query.FeatureID),
			"market":       query.Market,
			"instrumentId": query.InstrumentID,
			"operation":    query.Params["operation"],
		}},
		AsOf: time.Now().UTC(),
	}, nil
}

func (*stage9MarketDataPredictionBroker) SubscribePredictionMarket(
	context.Context,
	broker.PredictionSubscription,
) error {
	return nil
}

func (*stage9MarketDataPredictionBroker) UnsubscribePredictionMarket(
	context.Context,
	broker.PredictionSubscription,
) error {
	return nil
}

func (b *stage9MarketDataPredictionBroker) providerCall() *stage9MarketDataPredictionProviderCall {
	if b.lastQuery == nil {
		return nil
	}
	query := b.lastQuery
	params := make(map[string]any)
	if encoded, err := json.Marshal(query.Params); err == nil {
		_ = json.Unmarshal(encoded, &params)
	}
	return &stage9MarketDataPredictionProviderCall{
		FeatureID:     string(query.FeatureID),
		Market:        query.Market,
		MarketSegment: string(query.MarketSegment),
		ProductClass:  string(query.ProductClass),
		InstrumentID:  query.InstrumentID,
		PageSize:      query.PageSize,
		Params:        params,
	}
}
