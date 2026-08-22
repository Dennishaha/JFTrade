package rustmigration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	marketdataapi "github.com/jftrade/jftrade-main/internal/api/marketdata"
	productfeaturesapi "github.com/jftrade/jftrade-main/internal/api/productfeatures"
	marketdataapp "github.com/jftrade/jftrade-main/internal/app/apiserver/marketdataapp"
	marketdatasrv "github.com/jftrade/jftrade-main/internal/marketdata"
	productservice "github.com/jftrade/jftrade-main/internal/productfeatures"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

const stage9MarketDataProviderActionsFixtureVersion = "stage9.market-data-provider-actions.v1"

type stage9MarketDataProviderActionsCase struct {
	Name           string            `json:"name"`
	Method         string            `json:"method"`
	RequestPath    string            `json:"requestPath"`
	Body           string            `json:"body,omitempty"`
	ExpectedStatus int               `json:"expectedStatus"`
	Headers        map[string]string `json:"headers,omitempty"`
	Data           json.RawMessage   `json:"data,omitempty"`
	ErrorCode      string            `json:"errorCode,omitempty"`
	ErrorMessage   string            `json:"errorMessage,omitempty"`
	ProviderCall   json.RawMessage   `json:"providerCall,omitempty"`
}

type stage9MarketDataProviderActionsFixture struct {
	Version string                                `json:"version"`
	Cases   []stage9MarketDataProviderActionsCase `json:"cases"`
}

type stage9MarketDataProviderActionsInput struct {
	name               string
	path               string
	body               string
	marketProvider     *stage9MarketDataProviderActionsMarketProvider
	featureBroker      *stage9MarketDataProviderActionsBroker
	quoteStore         broker.PredictionQuoteStore
	expectProviderCall bool
}

// TestStage9MarketDataProviderActionsFixtureMatchesCurrentGoOwner freezes the
// five non-subscription provider-backed POST operations. Every request uses a
// fixture provider/broker; no Provider/OpenD, helper, subscription lease, or
// production store is started.
func TestStage9MarketDataProviderActionsFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve provider-actions fixture source")
	}
	fixturePath := filepath.Join(
		filepath.Dir(source),
		"../../tests/fixtures/rust-migration/stage9/market-data-provider-actions.json",
	)
	gin.SetMode(gin.TestMode)
	want := stage9MarketDataProviderActionsFixture{
		Version: stage9MarketDataProviderActionsFixtureVersion,
		Cases:   make([]stage9MarketDataProviderActionsCase, 0),
	}
	for _, testCase := range stage9MarketDataProviderActionsInputs() {
		router := stage9MarketDataProviderActionsRouter(testCase)
		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodPost,
			testCase.path,
			strings.NewReader(testCase.body),
		)
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)

		providerCall := stage9MarketDataProviderActionsProviderCall(testCase)
		if (providerCall != nil) != testCase.expectProviderCall {
			t.Fatalf(
				"%s provider call presence = %t, want %t (%s)",
				testCase.name,
				providerCall != nil,
				testCase.expectProviderCall,
				recorder.Body.String(),
			)
		}
		entry := stage9MarketDataProviderActionsCase{
			Name:           testCase.name,
			Method:         http.MethodPost,
			RequestPath:    testCase.path,
			Body:           testCase.body,
			ExpectedStatus: recorder.Code,
			ProviderCall:   providerCall,
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
			entry.Data = normalizeStage9MarketDataProviderActionsJSON(envelope.Data)
		}
		want.Cases = append(want.Cases, entry)
	}
	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode provider-actions fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write provider-actions fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read provider-actions fixture: %v", err)
	}
	var got stage9MarketDataProviderActionsFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode provider-actions fixture: %v", err)
	}
	for index := range got.Cases {
		got.Cases[index].Data = compactStage9MarketDataProviderActionsJSON(got.Cases[index].Data)
		got.Cases[index].ProviderCall = compactStage9MarketDataProviderActionsJSON(got.Cases[index].ProviderCall)
		want.Cases[index].Data = compactStage9MarketDataProviderActionsJSON(want.Cases[index].Data)
		want.Cases[index].ProviderCall = compactStage9MarketDataProviderActionsJSON(want.Cases[index].ProviderCall)
	}
	if !reflect.DeepEqual(got, want) {
		for index := range want.Cases {
			if !reflect.DeepEqual(got.Cases[index], want.Cases[index]) {
				t.Fatalf(
					"stage 9 provider-actions case %s drifted: got=%#v want=%#v",
					want.Cases[index].Name,
					got.Cases[index],
					want.Cases[index],
				)
			}
		}
		t.Fatal("stage 9 market-data provider-actions fixture drifted from the Go owner")
	}
}

func stage9MarketDataProviderActionsRouter(
	input stage9MarketDataProviderActionsInput,
) *gin.Engine {
	router := gin.New()
	marketProvider := input.marketProvider
	if marketProvider == nil {
		marketProvider = stage9MarketDataProviderActionsMarketProviderReady()
	}
	marketdataapi.RegisterRoutes(
		router.Group("/api/v1"),
		marketdatasrv.NewService(marketProvider),
	)

	registry := broker.NewRegistry()
	defaultBroker := ""
	if input.featureBroker != nil {
		registry.Register(input.featureBroker)
		defaultBroker = input.featureBroker.ID()
	}
	options := make([]productservice.Option, 0, 1)
	if input.quoteStore != nil {
		options = append(options, productservice.WithPredictionQuoteStore(input.quoteStore))
	}
	productfeaturesapi.RegisterRoutes(
		router.Group("/api/v1"),
		productservice.NewService(registry, defaultBroker, nil, nil, options...),
	)
	return router
}

func stage9MarketDataProviderActionsInputs() []stage9MarketDataProviderActionsInput {
	return []stage9MarketDataProviderActionsInput{
		{
			name:               "normalize-success-alias",
			path:               "/api/v1/market-data/instruments/normalize",
			body:               `{"market":"us","symbol":"aapl"}`,
			marketProvider:     stage9MarketDataProviderActionsMarketProviderReady(),
			expectProviderCall: true,
		},
		{
			name:               "normalize-empty-object",
			path:               "/api/v1/market-data/instruments/normalize",
			body:               `{}`,
			marketProvider:     stage9MarketDataProviderActionsMarketProviderReady(),
			expectProviderCall: true,
		},
		{
			name:               "normalize-null-body",
			path:               "/api/v1/market-data/instruments/normalize",
			body:               `null`,
			marketProvider:     stage9MarketDataProviderActionsMarketProviderReady(),
			expectProviderCall: true,
		},
		{
			name:           "normalize-invalid-json",
			path:           "/api/v1/market-data/instruments/normalize",
			body:           `{`,
			marketProvider: stage9MarketDataProviderActionsMarketProviderReady(),
		},
		{
			name:               "normalize-invalid-instrument",
			path:               "/api/v1/market-data/instruments/normalize",
			body:               `{"market":"US","symbol":""}`,
			marketProvider:     stage9MarketDataProviderActionsMarketProviderReady(),
			expectProviderCall: true,
		},
		{
			name: "normalize-provider-error-maps-to-invalid",
			path: "/api/v1/market-data/instruments/normalize",
			body: `{"market":"US","symbol":"AAPL"}`,
			marketProvider: stage9MarketDataProviderActionsMarketProviderWith(func(p *stage9MarketDataProviderActionsMarketProvider) {
				p.normalizeErr = errors.New("fixture normalization provider failure")
			}),
			expectProviderCall: true,
		},

		{
			name:               "analysis-body-overrides-query-operation",
			path:               "/api/v1/market-data/options/analysis/US.AAPL?brokerId=api-test&market=US&operation=volatility&operation=quote&custom=query",
			body:               `{"operation":"strategy_analysis","market":"HK","custom":"body","bodyOnly":true}`,
			featureBroker:      stage9MarketDataProviderActionsFeatureBrokerReady(),
			expectProviderCall: true,
		},
		{
			name:               "analysis-empty-object",
			path:               "/api/v1/market-data/options/analysis/US.AAPL?brokerId=api-test&market=US",
			body:               `{}`,
			featureBroker:      stage9MarketDataProviderActionsFeatureBrokerReady(),
			expectProviderCall: true,
		},
		{
			name:          "analysis-invalid-json",
			path:          "/api/v1/market-data/options/analysis/US.AAPL?brokerId=api-test&market=US",
			body:          `{`,
			featureBroker: stage9MarketDataProviderActionsFeatureBrokerReady(),
		},
		{
			name:          "analysis-invalid-contract-operation",
			path:          "/api/v1/market-data/options/analysis/US.AAPL?brokerId=api-test&market=US",
			body:          `{"operation":"quote"}`,
			featureBroker: stage9MarketDataProviderActionsFeatureBrokerReady(),
		},
		{
			name:          "analysis-capability-unsupported",
			path:          "/api/v1/market-data/options/analysis/US.AAPL?brokerId=missing&market=US",
			body:          `{}`,
			featureBroker: nil,
		},
		{
			name: "analysis-provider-warming",
			path: "/api/v1/market-data/options/analysis/US.AAPL260821C00100000?brokerId=api-test&market=US",
			body: `{"operation":"volatility"}`,
			featureBroker: stage9MarketDataProviderActionsFeatureBrokerWith(func(b *stage9MarketDataProviderActionsBroker) {
				b.queryErr = marketdatasrv.ErrProviderWarming
			}),
			expectProviderCall: true,
		},
		{
			name: "analysis-provider-busy",
			path: "/api/v1/market-data/options/analysis/US.AAPL260821C00100000?brokerId=api-test&market=US",
			body: `{"operation":"volatility"}`,
			featureBroker: stage9MarketDataProviderActionsFeatureBrokerWith(func(b *stage9MarketDataProviderActionsBroker) {
				b.queryErr = marketdatasrv.ErrProviderBusy
			}),
			expectProviderCall: true,
		},
		{
			name: "analysis-provider-403",
			path: "/api/v1/market-data/options/analysis/US.AAPL260821C00100000?brokerId=api-test&market=US",
			body: `{"operation":"volatility"}`,
			featureBroker: stage9MarketDataProviderActionsFeatureBrokerWith(func(b *stage9MarketDataProviderActionsBroker) {
				b.queryErr = &stage9MarketDataProviderActionsHTTPError{status: 403, message: "option analysis entitlement denied"}
			}),
			expectProviderCall: true,
		},
		{
			name: "analysis-provider-422",
			path: "/api/v1/market-data/options/analysis/US.AAPL260821C00100000?brokerId=api-test&market=US",
			body: `{"operation":"volatility"}`,
			featureBroker: stage9MarketDataProviderActionsFeatureBrokerWith(func(b *stage9MarketDataProviderActionsBroker) {
				b.queryErr = &stage9MarketDataProviderActionsHTTPError{status: 422, message: "option analysis input rejected"}
			}),
			expectProviderCall: true,
		},
		{
			name: "analysis-provider-502",
			path: "/api/v1/market-data/options/analysis/US.AAPL260821C00100000?brokerId=api-test&market=US",
			body: `{"operation":"volatility"}`,
			featureBroker: stage9MarketDataProviderActionsFeatureBrokerWith(func(b *stage9MarketDataProviderActionsBroker) {
				b.queryErr = errors.New("option analysis upstream failed")
			}),
			expectProviderCall: true,
		},

		{
			name:               "zero-dte-success-body-context-wins",
			path:               "/api/v1/market-data/options/events/zero-dte-contracts?brokerId=query-broker&accountId=query-account&tradingEnvironment=simulate",
			body:               `{"brokerId":"api-test","accountId":"body-account","tradingEnvironment":"real","market":"us","underlyingInstrumentId":"us.spx","underlyingProductClass":"index","expiryTimestamp":1787395200,"chain":{"productCode":"SPX","multiplier":100},"sort":"volume","optionType":"call"}`,
			featureBroker:      stage9MarketDataProviderActionsFeatureBrokerReady(),
			expectProviderCall: true,
		},
		{
			name:               "zero-dte-empty-object",
			path:               "/api/v1/market-data/options/events/zero-dte-contracts?brokerId=api-test",
			body:               `{}`,
			featureBroker:      stage9MarketDataProviderActionsFeatureBrokerReady(),
			expectProviderCall: false,
		},
		{
			name:          "zero-dte-invalid-json",
			path:          "/api/v1/market-data/options/events/zero-dte-contracts?brokerId=api-test",
			body:          `{`,
			featureBroker: stage9MarketDataProviderActionsFeatureBrokerReady(),
		},
		{
			name:          "zero-dte-non-us",
			path:          "/api/v1/market-data/options/events/zero-dte-contracts?brokerId=api-test",
			body:          `{"market":"hk","underlyingInstrumentId":"HK.00700","expiryTimestamp":1787395200,"chain":{"productCode":"HSI"}}`,
			featureBroker: stage9MarketDataProviderActionsFeatureBrokerReady(),
		},
		{
			name:          "zero-dte-missing-chain-context",
			path:          "/api/v1/market-data/options/events/zero-dte-contracts?brokerId=api-test",
			body:          `{"market":"US","underlyingInstrumentId":"US.SPX","expiryTimestamp":1787395200}`,
			featureBroker: stage9MarketDataProviderActionsFeatureBrokerReady(),
		},
		{
			name: "zero-dte-capability-unsupported",
			path: "/api/v1/market-data/options/events/zero-dte-contracts?brokerId=missing",
			body: `{"market":"US","underlyingInstrumentId":"US.SPX","expiryTimestamp":1787395200,"chain":{"productCode":"SPX"}}`,
		},
		{
			name: "zero-dte-provider-warming",
			path: "/api/v1/market-data/options/events/zero-dte-contracts?brokerId=api-test",
			body: `{"market":"US","underlyingInstrumentId":"US.SPX","expiryTimestamp":1787395200,"chain":{"productCode":"SPX"}}`,
			featureBroker: stage9MarketDataProviderActionsFeatureBrokerWith(func(b *stage9MarketDataProviderActionsBroker) {
				b.queryErr = marketdatasrv.ErrProviderWarming
			}),
			expectProviderCall: true,
		},
		{
			name: "zero-dte-provider-busy",
			path: "/api/v1/market-data/options/events/zero-dte-contracts?brokerId=api-test",
			body: `{"market":"US","underlyingInstrumentId":"US.SPX","expiryTimestamp":1787395200,"chain":{"productCode":"SPX"}}`,
			featureBroker: stage9MarketDataProviderActionsFeatureBrokerWith(func(b *stage9MarketDataProviderActionsBroker) {
				b.queryErr = marketdatasrv.ErrProviderBusy
			}),
			expectProviderCall: true,
		},
		{
			name: "zero-dte-provider-403",
			path: "/api/v1/market-data/options/events/zero-dte-contracts?brokerId=api-test",
			body: `{"market":"US","underlyingInstrumentId":"US.SPX","expiryTimestamp":1787395200,"chain":{"productCode":"SPX"}}`,
			featureBroker: stage9MarketDataProviderActionsFeatureBrokerWith(func(b *stage9MarketDataProviderActionsBroker) {
				b.queryErr = &stage9MarketDataProviderActionsHTTPError{status: 403, message: "0DTE entitlement denied"}
			}),
			expectProviderCall: true,
		},
		{
			name: "zero-dte-provider-422",
			path: "/api/v1/market-data/options/events/zero-dte-contracts?brokerId=api-test",
			body: `{"market":"US","underlyingInstrumentId":"US.SPX","expiryTimestamp":1787395200,"chain":{"productCode":"SPX"}}`,
			featureBroker: stage9MarketDataProviderActionsFeatureBrokerWith(func(b *stage9MarketDataProviderActionsBroker) {
				b.queryErr = &stage9MarketDataProviderActionsHTTPError{status: 422, message: "0DTE contract filters rejected"}
			}),
			expectProviderCall: true,
		},
		{
			name: "zero-dte-provider-502",
			path: "/api/v1/market-data/options/events/zero-dte-contracts?brokerId=api-test",
			body: `{"market":"US","underlyingInstrumentId":"US.SPX","expiryTimestamp":1787395200,"chain":{"productCode":"SPX"}}`,
			featureBroker: stage9MarketDataProviderActionsFeatureBrokerWith(func(b *stage9MarketDataProviderActionsBroker) {
				b.queryErr = errors.New("0DTE upstream failed")
			}),
			expectProviderCall: true,
		},

		{
			name: "combo-success-query-fallback",
			path: "/api/v1/market-data/prediction/combos/quotes?brokerId=api-test&accountId=query-account&tradingEnvironment=simulate",
			body: `{"accountId":"body-account","mvc":"0.50","legs":[{"instrumentId":"us.ec-yes","side":"buy","predictionSide":"yes","ratio":1},{"instrumentId":"us.ec-no","side":"sell","predictionSide":"no","ratio":2}]}`,
			featureBroker: stage9MarketDataProviderActionsFeatureBrokerWith(func(b *stage9MarketDataProviderActionsBroker) {
				b.accounts = stage9MarketDataProviderActionsEligibleAccounts("body-account")
			}),
			quoteStore:         stage9MarketDataProviderActionsQuoteStoreReady(),
			expectProviderCall: true,
		},
		{
			name: "combo-empty-object",
			path: "/api/v1/market-data/prediction/combos/quotes?brokerId=api-test&accountId=account-1&tradingEnvironment=simulate",
			body: `{}`,
			featureBroker: stage9MarketDataProviderActionsFeatureBrokerWith(func(b *stage9MarketDataProviderActionsBroker) {
				b.accounts = stage9MarketDataProviderActionsEligibleAccounts("account-1")
			}),
			quoteStore: stage9MarketDataProviderActionsQuoteStoreReady(),
		},
		{
			name:          "combo-invalid-json",
			path:          "/api/v1/market-data/prediction/combos/quotes?brokerId=api-test&accountId=account-1&tradingEnvironment=simulate",
			body:          `{`,
			featureBroker: stage9MarketDataProviderActionsFeatureBrokerReady(),
			quoteStore:    stage9MarketDataProviderActionsQuoteStoreReady(),
		},
		{
			name:          "combo-invalid-leg",
			path:          "/api/v1/market-data/prediction/combos/quotes?brokerId=api-test&accountId=account-1&tradingEnvironment=simulate",
			body:          `{"mvc":"0.50","legs":[{"instrumentId":"US.EC-YES","side":"buy","predictionSide":"yes","ratio":0}]}`,
			featureBroker: stage9MarketDataProviderActionsFeatureBrokerReady(),
			quoteStore:    stage9MarketDataProviderActionsQuoteStoreReady(),
		},
		{
			name: "combo-capability-unsupported",
			path: "/api/v1/market-data/prediction/combos/quotes?brokerId=missing&accountId=account-1&tradingEnvironment=simulate",
			body: `{"mvc":"0.50","legs":[{"instrumentId":"US.EC-YES","side":"buy","predictionSide":"yes","ratio":1},{"instrumentId":"US.EC-NO","side":"sell","predictionSide":"no","ratio":1}]}`,
		},
		{
			name: "combo-ineligible-account",
			path: "/api/v1/market-data/prediction/combos/quotes?brokerId=api-test&accountId=hk-account&tradingEnvironment=simulate",
			body: `{"mvc":"0.50","legs":[{"instrumentId":"US.EC-YES","side":"buy","predictionSide":"yes","ratio":1},{"instrumentId":"US.EC-NO","side":"sell","predictionSide":"no","ratio":1}]}`,
			featureBroker: stage9MarketDataProviderActionsFeatureBrokerWith(func(b *stage9MarketDataProviderActionsBroker) {
				b.accounts = stage9MarketDataProviderActionsIneligibleAccounts()
			}),
			quoteStore: stage9MarketDataProviderActionsQuoteStoreReady(),
		},
		{
			name: "combo-provider-warming",
			path: "/api/v1/market-data/prediction/combos/quotes?brokerId=api-test&accountId=account-1&tradingEnvironment=simulate",
			body: `{"mvc":"0.50","legs":[{"instrumentId":"US.EC-YES","side":"buy","predictionSide":"yes","ratio":1},{"instrumentId":"US.EC-NO","side":"sell","predictionSide":"no","ratio":1}]}`,
			featureBroker: stage9MarketDataProviderActionsFeatureBrokerWith(func(b *stage9MarketDataProviderActionsBroker) {
				b.queryErr = marketdatasrv.ErrProviderWarming
			}),
			quoteStore:         stage9MarketDataProviderActionsQuoteStoreReady(),
			expectProviderCall: true,
		},
		{
			name: "combo-provider-busy",
			path: "/api/v1/market-data/prediction/combos/quotes?brokerId=api-test&accountId=account-1&tradingEnvironment=simulate",
			body: `{"mvc":"0.50","legs":[{"instrumentId":"US.EC-YES","side":"buy","predictionSide":"yes","ratio":1},{"instrumentId":"US.EC-NO","side":"sell","predictionSide":"no","ratio":1}]}`,
			featureBroker: stage9MarketDataProviderActionsFeatureBrokerWith(func(b *stage9MarketDataProviderActionsBroker) {
				b.queryErr = marketdatasrv.ErrProviderBusy
			}),
			quoteStore:         stage9MarketDataProviderActionsQuoteStoreReady(),
			expectProviderCall: true,
		},
		{
			name: "combo-provider-rate-limited",
			path: "/api/v1/market-data/prediction/combos/quotes?brokerId=api-test&accountId=account-1&tradingEnvironment=simulate",
			body: `{"mvc":"0.50","legs":[{"instrumentId":"US.EC-YES","side":"buy","predictionSide":"yes","ratio":1},{"instrumentId":"US.EC-NO","side":"sell","predictionSide":"no","ratio":1}]}`,
			featureBroker: stage9MarketDataProviderActionsFeatureBrokerWith(func(b *stage9MarketDataProviderActionsBroker) {
				b.queryErr = broker.NewSnapshotRateLimitError(6500*time.Millisecond, nil)
			}),
			quoteStore:         stage9MarketDataProviderActionsQuoteStoreReady(),
			expectProviderCall: true,
		},
		{
			name: "combo-provider-422",
			path: "/api/v1/market-data/prediction/combos/quotes?brokerId=api-test&accountId=account-1&tradingEnvironment=simulate",
			body: `{"mvc":"0.50","legs":[{"instrumentId":"US.EC-YES","side":"buy","predictionSide":"yes","ratio":1},{"instrumentId":"US.EC-NO","side":"sell","predictionSide":"no","ratio":1}]}`,
			featureBroker: stage9MarketDataProviderActionsFeatureBrokerWith(func(b *stage9MarketDataProviderActionsBroker) {
				b.queryErr = &stage9MarketDataProviderActionsHTTPError{status: 422, message: "combo quote rejected"}
			}),
			quoteStore:         stage9MarketDataProviderActionsQuoteStoreReady(),
			expectProviderCall: true,
		},
		{
			name: "combo-provider-502",
			path: "/api/v1/market-data/prediction/combos/quotes?brokerId=api-test&accountId=account-1&tradingEnvironment=simulate",
			body: `{"mvc":"0.50","legs":[{"instrumentId":"US.EC-YES","side":"buy","predictionSide":"yes","ratio":1},{"instrumentId":"US.EC-NO","side":"sell","predictionSide":"no","ratio":1}]}`,
			featureBroker: stage9MarketDataProviderActionsFeatureBrokerWith(func(b *stage9MarketDataProviderActionsBroker) {
				b.queryErr = errors.New("combo quote upstream failed")
			}),
			quoteStore:         stage9MarketDataProviderActionsQuoteStoreReady(),
			expectProviderCall: true,
		},

		{
			name:               "batch-success-order-and-deduplicate",
			path:               "/api/v1/market-data/snapshots?brokerId=api-test&market=US",
			body:               `{"instrumentIds":["us.aapl","US.MSFT","US.AAPL"],"symbols":["HK.00700","hk.00700"]}`,
			featureBroker:      stage9MarketDataProviderActionsFeatureBrokerReady(),
			expectProviderCall: true,
		},
		{
			name:               "batch-null-instrument-ids-with-symbols",
			path:               "/api/v1/market-data/snapshots?brokerId=api-test&market=US",
			body:               `{"instrumentIds":null,"symbols":["US.AAPL"]}`,
			featureBroker:      stage9MarketDataProviderActionsFeatureBrokerReady(),
			expectProviderCall: true,
		},
		{
			name:               "batch-omitted-symbols",
			path:               "/api/v1/market-data/snapshots?brokerId=api-test&market=US",
			body:               `{"instrumentIds":["US.AAPL"]}`,
			featureBroker:      stage9MarketDataProviderActionsFeatureBrokerReady(),
			expectProviderCall: true,
		},
		{
			name:          "batch-invalid-json",
			path:          "/api/v1/market-data/snapshots?brokerId=api-test",
			body:          `{`,
			featureBroker: stage9MarketDataProviderActionsFeatureBrokerReady(),
		},
		{
			name:          "batch-empty-arrays",
			path:          "/api/v1/market-data/snapshots?brokerId=api-test",
			body:          `{"instrumentIds":[],"symbols":[]}`,
			featureBroker: stage9MarketDataProviderActionsFeatureBrokerReady(),
		},
		{
			name: "batch-capability-unsupported",
			path: "/api/v1/market-data/snapshots?brokerId=api-test",
			body: `{"instrumentIds":["US.AAPL"]}`,
			featureBroker: stage9MarketDataProviderActionsFeatureBrokerWith(func(b *stage9MarketDataProviderActionsBroker) {
				b.supported[broker.FeatureMarketSnapshots] = false
			}),
		},
		{
			name: "batch-provider-warming",
			path: "/api/v1/market-data/snapshots?brokerId=api-test",
			body: `{"instrumentIds":["US.AAPL"]}`,
			featureBroker: stage9MarketDataProviderActionsFeatureBrokerWith(func(b *stage9MarketDataProviderActionsBroker) {
				b.snapshotErr = marketdatasrv.ErrProviderWarming
			}),
			expectProviderCall: true,
		},
		{
			name: "batch-provider-busy",
			path: "/api/v1/market-data/snapshots?brokerId=api-test",
			body: `{"instrumentIds":["US.AAPL"]}`,
			featureBroker: stage9MarketDataProviderActionsFeatureBrokerWith(func(b *stage9MarketDataProviderActionsBroker) {
				b.snapshotErr = marketdatasrv.ErrProviderBusy
			}),
			expectProviderCall: true,
		},
		{
			name: "batch-provider-rate-limited",
			path: "/api/v1/market-data/snapshots?brokerId=api-test",
			body: `{"instrumentIds":["US.AAPL"]}`,
			featureBroker: stage9MarketDataProviderActionsFeatureBrokerWith(func(b *stage9MarketDataProviderActionsBroker) {
				b.snapshotErr = broker.NewSnapshotRateLimitError(6500*time.Millisecond, nil)
			}),
			expectProviderCall: true,
		},
		{
			name: "batch-provider-403",
			path: "/api/v1/market-data/snapshots?brokerId=api-test",
			body: `{"instrumentIds":["US.AAPL"]}`,
			featureBroker: stage9MarketDataProviderActionsFeatureBrokerWith(func(b *stage9MarketDataProviderActionsBroker) {
				b.snapshotErr = &stage9MarketDataProviderActionsHTTPError{status: 403, message: "snapshot entitlement denied"}
			}),
			expectProviderCall: true,
		},
		{
			name: "batch-provider-422",
			path: "/api/v1/market-data/snapshots?brokerId=api-test",
			body: `{"instrumentIds":["US.AAPL"]}`,
			featureBroker: stage9MarketDataProviderActionsFeatureBrokerWith(func(b *stage9MarketDataProviderActionsBroker) {
				b.snapshotErr = &stage9MarketDataProviderActionsHTTPError{status: 422, message: "snapshot symbol rejected"}
			}),
			expectProviderCall: true,
		},
		{
			name: "batch-provider-502",
			path: "/api/v1/market-data/snapshots?brokerId=api-test",
			body: `{"instrumentIds":["US.AAPL"]}`,
			featureBroker: stage9MarketDataProviderActionsFeatureBrokerWith(func(b *stage9MarketDataProviderActionsBroker) {
				b.snapshotErr = errors.New("snapshot upstream failed")
			}),
			expectProviderCall: true,
		},
	}
}

func stage9MarketDataProviderActionsProviderCall(
	input stage9MarketDataProviderActionsInput,
) json.RawMessage {
	if input.marketProvider != nil {
		return input.marketProvider.providerCall()
	}
	if input.featureBroker != nil {
		return input.featureBroker.providerCall()
	}
	return nil
}

func normalizeStage9MarketDataProviderActionsJSON(data json.RawMessage) json.RawMessage {
	if len(data) == 0 {
		return data
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return data
	}
	normalizeStage9MarketDataProviderActionsTimes(value)
	contents, err := json.Marshal(value)
	if err != nil {
		return data
	}
	return contents
}

func normalizeStage9MarketDataProviderActionsTimes(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			switch key {
			case "asOf", "resolvedAt", "receivedAt", "quoteExpiresAt":
				if _, ok := child.(string); ok {
					typed[key] = "fixture-time"
					continue
				}
			}
			normalizeStage9MarketDataProviderActionsTimes(child)
		}
	case []any:
		for _, child := range typed {
			normalizeStage9MarketDataProviderActionsTimes(child)
		}
	}
}

func compactStage9MarketDataProviderActionsJSON(data json.RawMessage) json.RawMessage {
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

type stage9MarketDataProviderActionsMarketProvider struct {
	stage9MarketDataNewsActionsBaseProvider
	normalizeErr    error
	normalizeCalled bool
	normalizeInput  map[string]any
}

func stage9MarketDataProviderActionsMarketProviderReady() *stage9MarketDataProviderActionsMarketProvider {
	return stage9MarketDataProviderActionsMarketProviderWith(nil)
}

func stage9MarketDataProviderActionsMarketProviderWith(
	configure func(*stage9MarketDataProviderActionsMarketProvider),
) *stage9MarketDataProviderActionsMarketProvider {
	provider := &stage9MarketDataProviderActionsMarketProvider{
		stage9MarketDataNewsActionsBaseProvider: stage9MarketDataNewsActionsBaseProvider{
			providerID: "fixture-provider-actions",
		},
	}
	if configure != nil {
		configure(provider)
	}
	return provider
}

func (p *stage9MarketDataProviderActionsMarketProvider) NormalizeInstrument(
	ctx context.Context,
	input map[string]any,
) (map[string]any, error) {
	p.normalizeCalled = true
	p.normalizeInput = input
	if p.normalizeErr != nil {
		return nil, p.normalizeErr
	}
	return marketdataapp.NormalizeInstrument(ctx, input)
}

func (p *stage9MarketDataProviderActionsMarketProvider) providerCall() json.RawMessage {
	if !p.normalizeCalled {
		return nil
	}
	contents, err := json.Marshal(p.normalizeInput)
	if err != nil {
		return nil
	}
	return contents
}

type stage9MarketDataProviderActionsBroker struct {
	id           string
	supported    map[broker.FeatureID]bool
	queryErr     error
	snapshotErr  error
	emptyResult  bool
	accounts     []broker.Account
	lastQuery    *broker.FeatureQuery
	lastSnapshot *broker.SecuritySnapshotQuery
}

func stage9MarketDataProviderActionsFeatureBrokerReady() *stage9MarketDataProviderActionsBroker {
	return stage9MarketDataProviderActionsFeatureBrokerWith(nil)
}

func stage9MarketDataProviderActionsFeatureBrokerWith(
	configure func(*stage9MarketDataProviderActionsBroker),
) *stage9MarketDataProviderActionsBroker {
	firm := "FUTUINC"
	adapter := &stage9MarketDataProviderActionsBroker{
		id: "api-test",
		supported: map[broker.FeatureID]bool{
			broker.FeatureOptionAnalysis:       true,
			broker.FeatureOptionEvents:         true,
			broker.FeaturePredictionComboQuote: true,
			broker.FeatureMarketSnapshots:      true,
		},
		accounts: []broker.Account{{
			ID: "account-1", SecurityFirm: &firm, MarketAuthorities: []string{"US"},
		}},
	}
	if configure != nil {
		configure(adapter)
	}
	return adapter
}

func (b *stage9MarketDataProviderActionsBroker) ID() string { return b.id }

func (b *stage9MarketDataProviderActionsBroker) Descriptor() broker.Descriptor {
	features := make([]broker.FeatureCapability, 0, len(b.supported))
	for feature, supported := range b.supported {
		state := broker.CapabilityAvailable
		if !supported {
			state = broker.CapabilityUnavailable
		}
		features = append(features, broker.FeatureCapability{
			ID: feature, Markets: []string{"US"}, Access: broker.FeatureAccessRead, State: state,
		})
	}
	return broker.Descriptor{
		ID: b.id, SecurityFirm: "Fixture", CapabilityVersion: broker.BuiltinCapabilityCatalog.Version,
		Capabilities: []broker.MarketCapability{
			{Market: "US", Features: features},
			{Market: "HK", Features: []broker.FeatureCapability{{
				ID: broker.FeatureMarketSnapshots, Markets: []string{"HK"},
				Access: broker.FeatureAccessRead, State: broker.CapabilityAvailable,
			}}},
		},
	}
}

func (b *stage9MarketDataProviderActionsBroker) DiscoverAccounts(context.Context) ([]broker.Account, error) {
	return b.accounts, nil
}

func (*stage9MarketDataProviderActionsBroker) Trading() broker.TradingService { return nil }

func (*stage9MarketDataProviderActionsBroker) MarketData() broker.MarketDataReader { return nil }

func (b *stage9MarketDataProviderActionsBroker) QueryOptionAnalytics(
	_ context.Context,
	query broker.FeatureQuery,
) (*broker.FeatureResult, error) {
	b.lastQuery = cloneStage9MarketDataProviderActionsQuery(query)
	return b.queryResult(query)
}

func (b *stage9MarketDataProviderActionsBroker) QueryPredictionMarket(
	_ context.Context,
	query broker.FeatureQuery,
) (*broker.FeatureResult, error) {
	b.lastQuery = cloneStage9MarketDataProviderActionsQuery(query)
	return b.queryResult(query)
}

func (*stage9MarketDataProviderActionsBroker) SubscribePredictionMarket(
	context.Context,
	broker.PredictionSubscription,
) error {
	return nil
}

func (*stage9MarketDataProviderActionsBroker) UnsubscribePredictionMarket(
	context.Context,
	broker.PredictionSubscription,
) error {
	return nil
}

func (b *stage9MarketDataProviderActionsBroker) QuerySecuritySnapshot(
	_ context.Context,
	query broker.SecuritySnapshotQuery,
) (*broker.SecuritySnapshotResult, error) {
	copyQuery := query
	copyQuery.Symbols = append([]string(nil), query.Symbols...)
	b.lastSnapshot = &copyQuery
	if b.snapshotErr != nil {
		return nil, b.snapshotErr
	}
	items := make([]broker.SecuritySnapshotItem, 0, len(query.Symbols))
	for _, symbol := range query.Symbols {
		lastPrice := 101.25
		items = append(items, broker.SecuritySnapshotItem{
			Symbol:     symbol,
			LastPrice:  &lastPrice,
			ObservedAt: time.Date(2026, 8, 21, 13, 30, 0, 0, time.UTC),
		})
	}
	return &broker.SecuritySnapshotResult{AccountID: query.AccountID, Snapshots: items}, nil
}

func (b *stage9MarketDataProviderActionsBroker) queryResult(
	query broker.FeatureQuery,
) (*broker.FeatureResult, error) {
	if b.queryErr != nil {
		return nil, b.queryErr
	}
	if b.emptyResult {
		return &broker.FeatureResult{Entries: []map[string]any{}, AsOf: stage9MarketDataProviderActionsTime()}, nil
	}
	result := &broker.FeatureResult{
		Entries: []map[string]any{{
			"feature":      string(query.FeatureID),
			"market":       query.Market,
			"instrumentId": query.InstrumentID,
			"operation":    query.Params["operation"],
		}},
		AsOf: stage9MarketDataProviderActionsTime(),
	}
	if query.FeatureID == broker.FeaturePredictionComboQuote {
		result.Metadata = map[string]any{
			"quoteId":     "fixture-quote-1",
			"bidPrice":    0.42,
			"askPrice":    0.47,
			"shouldRetry": false,
		}
	}
	return result, nil
}

func (b *stage9MarketDataProviderActionsBroker) providerCall() json.RawMessage {
	if b.lastSnapshot != nil {
		value := map[string]any{
			"kind":      "batchSnapshot",
			"brokerId":  b.lastSnapshot.BrokerID,
			"accountId": b.lastSnapshot.AccountID,
			"market":    b.lastSnapshot.Market,
			"symbols":   b.lastSnapshot.Symbols,
		}
		return stage9MarketDataProviderActionsJSON(value)
	}
	if b.lastQuery == nil {
		return nil
	}
	value := map[string]any{
		"kind":               "featureQuery",
		"brokerId":           b.lastQuery.BrokerID,
		"accountId":          b.lastQuery.AccountID,
		"tradingEnvironment": b.lastQuery.TradingEnvironment,
		"market":             b.lastQuery.Market,
		"marketSegment":      b.lastQuery.MarketSegment,
		"productClass":       b.lastQuery.ProductClass,
		"instrumentId":       b.lastQuery.InstrumentID,
		"featureId":          b.lastQuery.FeatureID,
		"pageSize":           b.lastQuery.PageSize,
		"params":             b.lastQuery.Params,
	}
	return stage9MarketDataProviderActionsJSON(value)
}

func cloneStage9MarketDataProviderActionsQuery(query broker.FeatureQuery) *broker.FeatureQuery {
	copyQuery := query
	if query.Params != nil {
		contents, _ := json.Marshal(query.Params)
		_ = json.Unmarshal(contents, &copyQuery.Params)
	}
	return &copyQuery
}

func stage9MarketDataProviderActionsJSON(value any) json.RawMessage {
	contents, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return contents
}

func stage9MarketDataProviderActionsTime() time.Time {
	return time.Date(2026, 8, 21, 13, 30, 0, 0, time.UTC)
}

func stage9MarketDataProviderActionsEligibleAccounts(accountID string) []broker.Account {
	firm := "FUTUINC"
	return []broker.Account{{ID: accountID, SecurityFirm: &firm, MarketAuthorities: []string{"US"}}}
}

func stage9MarketDataProviderActionsIneligibleAccounts() []broker.Account {
	firm := "FUTUSECURITIES"
	return []broker.Account{{ID: "hk-account", SecurityFirm: &firm, MarketAuthorities: []string{"HK"}}}
}

type stage9MarketDataProviderActionsHTTPError struct {
	status  int
	message string
}

func (e *stage9MarketDataProviderActionsHTTPError) Error() string { return e.message }

func (e *stage9MarketDataProviderActionsHTTPError) HTTPStatus() int { return e.status }

type stage9MarketDataProviderActionsQuoteStore struct {
	saveErr error
}

func stage9MarketDataProviderActionsQuoteStoreReady() broker.PredictionQuoteStore {
	return &stage9MarketDataProviderActionsQuoteStore{}
}

func (s *stage9MarketDataProviderActionsQuoteStore) SavePredictionQuote(
	context.Context,
	broker.PredictionQuoteRecord,
) error {
	return s.saveErr
}

func (*stage9MarketDataProviderActionsQuoteStore) ValidatePredictionQuote(
	context.Context, string, string, string, string, string, string,
) (broker.PredictionQuoteRecord, error) {
	return broker.PredictionQuoteRecord{}, errors.New("fixture validation is not used")
}

func (*stage9MarketDataProviderActionsQuoteStore) ConsumePredictionQuote(
	context.Context, string, string, string, string, string, string, string, string,
) error {
	return errors.New("fixture consumption is not used")
}

var (
	_ marketdatasrv.Provider        = (*stage9MarketDataProviderActionsMarketProvider)(nil)
	_ broker.Broker                 = (*stage9MarketDataProviderActionsBroker)(nil)
	_ broker.BatchSnapshotSource    = (*stage9MarketDataProviderActionsBroker)(nil)
	_ broker.OptionAnalyticsReader  = (*stage9MarketDataProviderActionsBroker)(nil)
	_ broker.PredictionMarketReader = (*stage9MarketDataProviderActionsBroker)(nil)
)

var _ = fmt.Sprint
