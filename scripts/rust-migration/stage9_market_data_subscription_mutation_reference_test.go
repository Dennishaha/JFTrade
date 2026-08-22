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
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	marketdataapi "github.com/jftrade/jftrade-main/internal/api/marketdata"
	productfeaturesapi "github.com/jftrade/jftrade-main/internal/api/productfeatures"
	marketdatasrv "github.com/jftrade/jftrade-main/internal/marketdata"
	productfeatures "github.com/jftrade/jftrade-main/internal/productfeatures"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

const stage9MarketDataSubscriptionMutationFixtureVersion = "stage9.market-data-subscription-mutation.v1"

type stage9MarketDataSubscriptionMutationFixture struct {
	Version string                                     `json:"version"`
	Cases   []stage9MarketDataSubscriptionMutationCase `json:"cases"`
}

type stage9MarketDataSubscriptionMutationCase struct {
	Name           string            `json:"name"`
	Method         string            `json:"method"`
	RequestPath    string            `json:"requestPath"`
	Body           string            `json:"body,omitempty"`
	ContextError   string            `json:"contextError,omitempty"`
	ExpectedStatus int               `json:"expectedStatus"`
	Headers        map[string]string `json:"headers,omitempty"`
	Data           json.RawMessage   `json:"data,omitempty"`
	ErrorCode      string            `json:"errorCode,omitempty"`
	ErrorMessage   string            `json:"errorMessage,omitempty"`
	ProviderCall   map[string]any    `json:"providerCall,omitempty"`
}

type stage9MarketDataSubscriptionMutationCaseSpec struct {
	Name         string
	Method       string
	Path         string
	Body         string
	ContextError string
	Setup        func(*testing.T, *stage9MarketDataSubscriptionMutationHarness)
	ActualPath   func(*stage9MarketDataSubscriptionMutationHarness) string
}

type stage9MarketDataSubscriptionMutationHarness struct {
	router            *gin.Engine
	market            *marketdatasrv.Service
	prediction        *stage9MarketDataSubscriptionMutationBroker
	predictionSvc     *productfeatures.Service
	predictionLeaseID string
}

// stage9MarketDataSubscriptionMutationBroker is a broker-neutral fixture
// double. It implements only the prediction subscription interface required by
// the handler; no OpenD, Futu SDK or external process is started.
type stage9MarketDataSubscriptionMutationBroker struct {
	id               string
	accounts         []broker.Account
	subscribeErr     error
	unsubscribeErr   error
	honorContext     bool
	subscribeCalls   []broker.PredictionSubscription
	unsubscribeCalls []broker.PredictionSubscription
}

func (b *stage9MarketDataSubscriptionMutationBroker) ID() string { return b.id }

func (b *stage9MarketDataSubscriptionMutationBroker) Descriptor() broker.Descriptor {
	return broker.Descriptor{
		ID:           b.id,
		SecurityFirm: "Moomoo US",
		Capabilities: []broker.MarketCapability{{
			Market: "US",
			Features: []broker.FeatureCapability{
				{
					ID: broker.FeaturePredictionDepth, Markets: []string{"US"},
					Access: broker.FeatureAccessRead, State: broker.CapabilityAvailable,
				},
				{
					ID: broker.FeaturePredictionHistory, Markets: []string{"US"},
					Access: broker.FeatureAccessRead, State: broker.CapabilityAvailable,
				},
			},
		}},
	}
}

func (b *stage9MarketDataSubscriptionMutationBroker) DiscoverAccounts(
	context.Context,
) ([]broker.Account, error) {
	return b.accounts, nil
}

func (b *stage9MarketDataSubscriptionMutationBroker) Trading() broker.TradingService {
	return nil
}

func (b *stage9MarketDataSubscriptionMutationBroker) MarketData() broker.MarketDataReader {
	return nil
}

func (b *stage9MarketDataSubscriptionMutationBroker) QueryPredictionMarket(
	context.Context,
	broker.FeatureQuery,
) (*broker.FeatureResult, error) {
	return &broker.FeatureResult{}, nil
}

func (b *stage9MarketDataSubscriptionMutationBroker) SubscribePredictionMarket(
	ctx context.Context,
	subscription broker.PredictionSubscription,
) error {
	if b.honorContext && ctx.Err() != nil {
		return ctx.Err()
	}
	b.subscribeCalls = append(b.subscribeCalls, subscription)
	return b.subscribeErr
}

func (b *stage9MarketDataSubscriptionMutationBroker) UnsubscribePredictionMarket(
	ctx context.Context,
	subscription broker.PredictionSubscription,
) error {
	if b.honorContext && ctx.Err() != nil {
		return ctx.Err()
	}
	b.unsubscribeCalls = append(b.unsubscribeCalls, subscription)
	return b.unsubscribeErr
}

// TestStage9MarketDataSubscriptionMutationFixtureMatchesCurrentGoOwner freezes
// all six subscription mutation routes below the shared authenticated
// transport. It records only observable HTTP behavior and non-wire provider
// call evidence; no real broker or market-data helper is used.
func TestStage9MarketDataSubscriptionMutationFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 market-data subscription mutation fixture source")
	}
	fixturePath := filepath.Join(
		filepath.Dir(source),
		"../../tests/fixtures/rust-migration/stage9/market-data-subscription-mutation.json",
	)
	gin.SetMode(gin.TestMode)
	inputs := stage9MarketDataSubscriptionMutationInputs()
	want := stage9MarketDataSubscriptionMutationFixture{
		Version: stage9MarketDataSubscriptionMutationFixtureVersion,
		Cases:   make([]stage9MarketDataSubscriptionMutationCase, 0, len(inputs)),
	}
	for _, input := range inputs {
		harness := newStage9MarketDataSubscriptionMutationHarness()
		if input.Setup != nil {
			input.Setup(t, harness)
		}
		requestPath := input.Path
		if input.ActualPath != nil {
			requestPath = input.ActualPath(harness)
		}
		status, headers, envelope := stage9MarketDataSubscriptionMutationRequest(
			t, harness, input.Method, requestPath, input.Body, input.ContextError,
		)
		entry := stage9MarketDataSubscriptionMutationCase{
			Name:           input.Name,
			Method:         input.Method,
			RequestPath:    input.Path,
			Body:           input.Body,
			ContextError:   input.ContextError,
			ExpectedStatus: status,
			Headers:        headers,
			ProviderCall:   stage9MarketDataSubscriptionMutationProviderCall(harness),
		}
		if input.ContextError == "" {
			entry.ContextError = ""
		}
		if envelope.Error != nil {
			entry.ErrorCode = envelope.Error.Code
			entry.ErrorMessage = envelope.Error.Message
		} else {
			entry.Data = normalizeStage9MarketDataSubscriptionMutationJSON(envelope.Data)
		}
		want.Cases = append(want.Cases, entry)
	}
	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode market-data subscription mutation fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write market-data subscription mutation fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read market-data subscription mutation fixture: %v", err)
	}
	var got stage9MarketDataSubscriptionMutationFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode market-data subscription mutation fixture: %v", err)
	}
	for index := range got.Cases {
		got.Cases[index].Data = compactStage9MarketDataSubscriptionMutationJSON(got.Cases[index].Data)
		want.Cases[index].Data = compactStage9MarketDataSubscriptionMutationJSON(want.Cases[index].Data)
	}
	if !reflect.DeepEqual(got, want) {
		for index := range want.Cases {
			if !reflect.DeepEqual(got.Cases[index], want.Cases[index]) {
				t.Fatalf(
					"stage 9 market-data subscription mutation case %s drifted: got=%#v want=%#v",
					want.Cases[index].Name,
					got.Cases[index],
					want.Cases[index],
				)
			}
		}
		t.Fatal("stage 9 market-data subscription mutation fixture drifted from the Go owner")
	}
}

func newStage9MarketDataSubscriptionMutationHarness() *stage9MarketDataSubscriptionMutationHarness {
	firm := "FUTUINC"
	prediction := &stage9MarketDataSubscriptionMutationBroker{
		id: "api-test",
		accounts: []broker.Account{{
			ID: "eligible", SecurityFirm: &firm, MarketAuthorities: []string{"US"},
		}},
	}
	registry := broker.NewRegistry()
	registry.Register(prediction)
	predictionService := productfeatures.NewService(registry, prediction.id, nil, nil)
	market := marketdatasrv.NewService(nil)
	router := gin.New()
	marketdataapi.RegisterRoutes(router.Group("/api/v1"), market)
	productfeaturesapi.RegisterRoutes(router.Group("/api/v1"), predictionService)
	return &stage9MarketDataSubscriptionMutationHarness{
		router: router, market: market, prediction: prediction,
		predictionSvc: predictionService,
	}
}

func stage9MarketDataSubscriptionMutationInputs() []stage9MarketDataSubscriptionMutationCaseSpec {
	marketSetup := func(consumer string, refs ...marketdatasrv.InstrumentRef) func(*testing.T, *stage9MarketDataSubscriptionMutationHarness) {
		return func(t *testing.T, harness *stage9MarketDataSubscriptionMutationHarness) {
			t.Helper()
			if _, err := harness.market.AcquireSubscription(context.Background(), consumer, refs); err != nil {
				t.Fatalf("seed market-data subscriptions: %v", err)
			}
		}
	}
	predictionSetup := func(code string, dataTypes ...string) func(*testing.T, *stage9MarketDataSubscriptionMutationHarness) {
		return func(t *testing.T, harness *stage9MarketDataSubscriptionMutationHarness) {
			t.Helper()
			lease, err := harness.acquirePredictionLease(code, dataTypes...)
			if err != nil {
				t.Fatalf("seed prediction subscription: %v", err)
			}
			harness.predictionLeaseID = lease
		}
	}

	return []stage9MarketDataSubscriptionMutationCaseSpec{
		{
			Name: "acquire-success-normalizes-instruments", Method: http.MethodPost,
			Path: "/api/v1/market-data/subscriptions",
			Body: `{"consumerId":"chart-main","instruments":[{"market":"hk","symbol":"00700"},{"channel":"KLINE","market":"us","symbol":"aapl","interval":"1m"}]}`,
		},
		{
			Name: "acquire-success-duplicate-target", Method: http.MethodPost,
			Path: "/api/v1/market-data/subscriptions",
			Body: `{"consumerId":"chart-main","instruments":[{"market":"US","symbol":"AAPL"},{"market":"US","symbol":"AAPL"}]}`,
		},
		{
			Name: "acquire-polling-fallback", Method: http.MethodPost,
			Path: "/api/v1/market-data/subscriptions",
			Body: `{"consumerId":"chart-alpha","providerBrokerId":" Alpha ","instruments":[{"market":"US","symbol":"AAPL","channel":"KLINE","interval":"1m"}]}`,
		},
		{
			Name: "acquire-unknown-fields", Method: http.MethodPost,
			Path: "/api/v1/market-data/subscriptions",
			Body: `{"consumerId":"chart","instruments":[{"market":"US","symbol":"AAPL"}],"ignored":true}`,
		},
		{
			Name: "acquire-null-body", Method: http.MethodPost,
			Path: "/api/v1/market-data/subscriptions", Body: "null",
		},
		{
			Name: "acquire-malformed-json", Method: http.MethodPost,
			Path: "/api/v1/market-data/subscriptions", Body: "{",
		},
		{
			Name: "acquire-trailing-json", Method: http.MethodPost,
			Path: "/api/v1/market-data/subscriptions", Body: `{"consumerId":"chart","instruments":[]} {}`,
		},
		{
			Name: "acquire-missing-consumer", Method: http.MethodPost,
			Path: "/api/v1/market-data/subscriptions",
			Body: `{"instruments":[{"market":"US","symbol":"AAPL"}]}`,
		},
		{
			Name: "acquire-missing-instruments", Method: http.MethodPost,
			Path: "/api/v1/market-data/subscriptions", Body: `{"consumerId":"chart"}`,
		},
		{
			Name: "acquire-all-instruments-invalid", Method: http.MethodPost,
			Path: "/api/v1/market-data/subscriptions",
			Body: `{"consumerId":"chart","instruments":[{"market":"US"},{"symbol":"AAPL"}]}`,
		},
		{
			Name: "acquire-invalid-channel", Method: http.MethodPost,
			Path: "/api/v1/market-data/subscriptions",
			Body: `{"consumerId":"chart","instruments":[{"market":"US","symbol":"AAPL","channel":"NEWS"}]}`,
		},
		{
			Name: "acquire-invalid-interval", Method: http.MethodPost,
			Path: "/api/v1/market-data/subscriptions",
			Body: `{"consumerId":"chart","instruments":[{"market":"US","symbol":"AAPL","channel":"KLINE","interval":"2m"}]}`,
		},
		{
			Name: "acquire-canceled", Method: http.MethodPost,
			Path:         "/api/v1/market-data/subscriptions",
			Body:         `{"consumerId":"chart","instruments":[{"market":"US","symbol":"AAPL"}]}`,
			ContextError: "canceled",
		},
		{
			Name: "release-target-success", Method: http.MethodPost,
			Path: "/api/v1/market-data/subscriptions/release",
			Body: `{"consumerId":"chart","instruments":[{"market":"HK","symbol":"00700","channel":"KLINE","interval":"1m"}]}`,
			Setup: marketSetup("chart",
				marketdatasrv.InstrumentRef{Market: "HK", Symbol: "00700", Channel: "KLINE", Interval: "1m"},
				marketdatasrv.InstrumentRef{Market: "US", Symbol: "AAPL"},
			),
		},
		{
			Name: "release-consumer-success", Method: http.MethodPost,
			Path: "/api/v1/market-data/subscriptions/release",
			Body: `{"consumerId":"chart"}`,
			Setup: marketSetup("chart",
				marketdatasrv.InstrumentRef{Market: "HK", Symbol: "00700"},
				marketdatasrv.InstrumentRef{Market: "US", Symbol: "AAPL"},
			),
		},
		{
			Name: "release-only-first-target", Method: http.MethodPost,
			Path: "/api/v1/market-data/subscriptions/release",
			Body: `{"consumerId":"chart","instruments":[{"market":"US","symbol":"AAPL"},{"market":"HK","symbol":"00700"}]}`,
			Setup: marketSetup("chart",
				marketdatasrv.InstrumentRef{Market: "US", Symbol: "AAPL"},
				marketdatasrv.InstrumentRef{Market: "HK", Symbol: "00700"},
			),
		},
		{
			Name: "release-polling-fallback", Method: http.MethodPost,
			Path: "/api/v1/market-data/subscriptions/release",
			Body: `{"consumerId":"chart","providerBrokerId":" Alpha ","instruments":[{"market":"US","symbol":"AAPL"}]}`,
		},
		{
			Name: "release-null-body", Method: http.MethodPost,
			Path: "/api/v1/market-data/subscriptions/release", Body: "null",
		},
		{
			Name: "release-malformed-json", Method: http.MethodPost,
			Path: "/api/v1/market-data/subscriptions/release", Body: "{",
		},
		{
			Name: "release-trailing-json", Method: http.MethodPost,
			Path: "/api/v1/market-data/subscriptions/release", Body: `{"consumerId":"chart"} {}`,
		},
		{
			Name: "release-missing-consumer", Method: http.MethodPost,
			Path: "/api/v1/market-data/subscriptions/release", Body: `{}`,
		},
		{
			Name: "release-incomplete-target", Method: http.MethodPost,
			Path: "/api/v1/market-data/subscriptions/release",
			Body: `{"consumerId":"chart","instruments":[{"market":"US"}]}`,
		},
		{
			Name: "release-invalid-interval", Method: http.MethodPost,
			Path: "/api/v1/market-data/subscriptions/release",
			Body: `{"consumerId":"chart","instruments":[{"market":"US","symbol":"AAPL","channel":"KLINE","interval":"2m"}]}`,
		},
		{
			Name: "release-canceled", Method: http.MethodPost,
			Path:         "/api/v1/market-data/subscriptions/release",
			Body:         `{"consumerId":"chart"}`,
			ContextError: "canceled",
			Setup:        marketSetup("chart", marketdatasrv.InstrumentRef{Market: "US", Symbol: "AAPL"}),
		},
		{
			Name: "heartbeat-success", Method: http.MethodPost,
			Path:  "/api/v1/market-data/subscriptions/heartbeat",
			Body:  `{"consumerId":"chart"}`,
			Setup: marketSetup("chart", marketdatasrv.InstrumentRef{Market: "US", Symbol: "AAPL"}),
		},
		{
			Name: "heartbeat-empty", Method: http.MethodPost,
			Path: "/api/v1/market-data/subscriptions/heartbeat",
			Body: `{"consumerId":"chart"}`,
		},
		{
			Name: "heartbeat-polling-fallback", Method: http.MethodPost,
			Path: "/api/v1/market-data/subscriptions/heartbeat",
			Body: `{"consumerId":"chart","providerBrokerId":" Alpha "}`,
		},
		{
			Name: "heartbeat-null-body", Method: http.MethodPost,
			Path: "/api/v1/market-data/subscriptions/heartbeat", Body: "null",
		},
		{
			Name: "heartbeat-malformed-json", Method: http.MethodPost,
			Path: "/api/v1/market-data/subscriptions/heartbeat", Body: "{",
		},
		{
			Name: "heartbeat-trailing-json", Method: http.MethodPost,
			Path: "/api/v1/market-data/subscriptions/heartbeat", Body: `{"consumerId":"chart"} {}`,
		},
		{
			Name: "heartbeat-blank-consumer", Method: http.MethodPost,
			Path: "/api/v1/market-data/subscriptions/heartbeat", Body: `{"consumerId":" "}`,
		},
		{
			Name: "heartbeat-canceled", Method: http.MethodPost,
			Path:         "/api/v1/market-data/subscriptions/heartbeat",
			Body:         `{"consumerId":"chart"}`,
			ContextError: "canceled",
			Setup:        marketSetup("chart", marketdatasrv.InstrumentRef{Market: "US", Symbol: "AAPL"}),
		},
		{
			Name: "clear-consumer-success", Method: http.MethodDelete,
			Path: "/api/v1/market-data/subscriptions?consumerId=chart",
			Setup: func(t *testing.T, h *stage9MarketDataSubscriptionMutationHarness) {
				marketSetup("chart", marketdatasrv.InstrumentRef{Market: "US", Symbol: "AAPL"})(t, h)
				marketSetup("other", marketdatasrv.InstrumentRef{Market: "HK", Symbol: "00700"})(t, h)
			},
		},
		{
			Name: "clear-all-success", Method: http.MethodDelete,
			Path: "/api/v1/market-data/subscriptions",
			Setup: func(t *testing.T, h *stage9MarketDataSubscriptionMutationHarness) {
				marketSetup("chart", marketdatasrv.InstrumentRef{Market: "US", Symbol: "AAPL"})(t, h)
				marketSetup("other", marketdatasrv.InstrumentRef{Market: "HK", Symbol: "00700"})(t, h)
			},
		},
		{
			Name: "clear-blank-consumer-means-all", Method: http.MethodDelete,
			Path: "/api/v1/market-data/subscriptions?consumerId=%20",
			Setup: func(t *testing.T, h *stage9MarketDataSubscriptionMutationHarness) {
				marketSetup("chart", marketdatasrv.InstrumentRef{Market: "US", Symbol: "AAPL"})(t, h)
				marketSetup("other", marketdatasrv.InstrumentRef{Market: "HK", Symbol: "00700"})(t, h)
			},
		},
		{
			Name: "clear-empty-success", Method: http.MethodDelete,
			Path: "/api/v1/market-data/subscriptions",
		},
		{
			Name: "clear-canceled", Method: http.MethodDelete,
			Path:         "/api/v1/market-data/subscriptions?consumerId=chart",
			ContextError: "canceled",
			Setup:        marketSetup("chart", marketdatasrv.InstrumentRef{Market: "US", Symbol: "AAPL"}),
		},
		{
			Name: "prediction-acquire-normalizes-types", Method: http.MethodPost,
			Path: "/api/v1/market-data/prediction/contracts/ec-42/subscriptions?brokerId=api-test&accountId=eligible",
			Body: `{"dataTypes":[" ticker ","KLINE","ticker"]}`,
		},
		{
			Name: "prediction-acquire-order-book", Method: http.MethodPost,
			Path: "/api/v1/market-data/prediction/contracts/US.EC-42/subscriptions?brokerId=api-test&accountId=eligible",
			Body: `{"dataTypes":["order_book"]}`,
		},
		{
			Name: "prediction-acquire-unknown-fields", Method: http.MethodPost,
			Path: "/api/v1/market-data/prediction/contracts/EC-42/subscriptions?brokerId=api-test&accountId=eligible",
			Body: `{"dataTypes":["KLINE"],"ignored":true}`,
		},
		{
			Name: "prediction-acquire-null-body", Method: http.MethodPost,
			Path: "/api/v1/market-data/prediction/contracts/EC-42/subscriptions?brokerId=api-test&accountId=eligible",
			Body: "null",
		},
		{
			Name: "prediction-acquire-malformed-json", Method: http.MethodPost,
			Path: "/api/v1/market-data/prediction/contracts/EC-42/subscriptions?brokerId=api-test&accountId=eligible",
			Body: "{",
		},
		{
			Name: "prediction-acquire-trailing-json", Method: http.MethodPost,
			Path: "/api/v1/market-data/prediction/contracts/EC-42/subscriptions?brokerId=api-test&accountId=eligible",
			Body: `{"dataTypes":["KLINE"]} {}`,
		},
		{
			Name: "prediction-acquire-missing-types", Method: http.MethodPost,
			Path: "/api/v1/market-data/prediction/contracts/EC-42/subscriptions?brokerId=api-test&accountId=eligible",
			Body: `{}`,
		},
		{
			Name: "prediction-acquire-invalid-type", Method: http.MethodPost,
			Path: "/api/v1/market-data/prediction/contracts/EC-42/subscriptions?brokerId=api-test&accountId=eligible",
			Body: `{"dataTypes":["NEWS"]}`,
		},
		{
			Name: "prediction-acquire-missing-broker", Method: http.MethodPost,
			Path: "/api/v1/market-data/prediction/contracts/EC-42/subscriptions?brokerId=missing&accountId=eligible",
			Body: `{"dataTypes":["KLINE"]}`,
		},
		{
			Name: "prediction-acquire-ineligible-account", Method: http.MethodPost,
			Path: "/api/v1/market-data/prediction/contracts/EC-42/subscriptions?brokerId=api-test&accountId=blocked",
			Body: `{"dataTypes":["KLINE"]}`,
			Setup: func(_ *testing.T, h *stage9MarketDataSubscriptionMutationHarness) {
				firm := "OTHER"
				h.prediction.accounts = []broker.Account{{
					ID: "blocked", SecurityFirm: &firm, MarketAuthorities: []string{"US"},
				}}
			},
		},
		{
			Name: "prediction-acquire-provider-failure", Method: http.MethodPost,
			Path: "/api/v1/market-data/prediction/contracts/EC-42/subscriptions?brokerId=api-test&accountId=eligible",
			Body: `{"dataTypes":["KLINE"]}`,
			Setup: func(_ *testing.T, h *stage9MarketDataSubscriptionMutationHarness) {
				h.prediction.subscribeErr = errors.New("fixture prediction subscribe failed")
			},
		},
		{
			Name: "prediction-acquire-canceled", Method: http.MethodPost,
			Path:         "/api/v1/market-data/prediction/contracts/EC-42/subscriptions?brokerId=api-test&accountId=eligible",
			Body:         `{"dataTypes":["KLINE"]}`,
			ContextError: "canceled",
			Setup: func(_ *testing.T, h *stage9MarketDataSubscriptionMutationHarness) {
				h.prediction.honorContext = true
			},
		},
		{
			Name: "prediction-release-success", Method: http.MethodDelete,
			Path:  "/api/v1/market-data/prediction/contracts/EC-42/subscriptions/fixture-lease",
			Setup: predictionSetup("EC-42", "KLINE"),
			ActualPath: func(h *stage9MarketDataSubscriptionMutationHarness) string {
				return strings.Replace(h.predictionReleasePath("EC-42"), "fixture-lease", h.predictionLeaseID, 1)
			},
		},
		{
			Name: "prediction-release-unknown-is-idempotent", Method: http.MethodDelete,
			Path: "/api/v1/market-data/prediction/contracts/EC-42/subscriptions/missing-lease",
		},
		{
			Name: "prediction-release-blank-lease", Method: http.MethodDelete,
			Path: "/api/v1/market-data/prediction/contracts/EC-42/subscriptions/%20",
		},
		{
			Name: "prediction-release-code-does-not-rebind-lease", Method: http.MethodDelete,
			Path:  "/api/v1/market-data/prediction/contracts/OTHER/subscriptions/fixture-lease",
			Setup: predictionSetup("EC-42", "ORDER_BOOK"),
			ActualPath: func(h *stage9MarketDataSubscriptionMutationHarness) string {
				return strings.Replace(h.predictionReleasePath("OTHER"), "fixture-lease", h.predictionLeaseID, 1)
			},
		},
		{
			Name: "prediction-release-provider-failure", Method: http.MethodDelete,
			Path: "/api/v1/market-data/prediction/contracts/EC-42/subscriptions/fixture-lease",
			Setup: func(t *testing.T, h *stage9MarketDataSubscriptionMutationHarness) {
				predictionSetup("EC-42", "KLINE")(t, h)
				h.prediction.unsubscribeErr = errors.New("fixture prediction unsubscribe failed")
			},
			ActualPath: func(h *stage9MarketDataSubscriptionMutationHarness) string {
				return strings.Replace(h.predictionReleasePath("EC-42"), "fixture-lease", h.predictionLeaseID, 1)
			},
		},
		{
			Name: "prediction-release-canceled", Method: http.MethodDelete,
			Path:         "/api/v1/market-data/prediction/contracts/EC-42/subscriptions/fixture-lease",
			ContextError: "canceled",
			Setup: func(t *testing.T, h *stage9MarketDataSubscriptionMutationHarness) {
				predictionSetup("EC-42", "KLINE")(t, h)
				h.prediction.honorContext = true
			},
			ActualPath: func(h *stage9MarketDataSubscriptionMutationHarness) string {
				return strings.Replace(h.predictionReleasePath("EC-42"), "fixture-lease", h.predictionLeaseID, 1)
			},
		},
	}
}

func (h *stage9MarketDataSubscriptionMutationHarness) acquirePredictionLease(
	code string,
	dataTypes ...string,
) (string, error) {
	lease, err := h.predictionSvc.AcquirePredictionSubscription(
		context.Background(), h.prediction.id, "eligible", code, dataTypes,
	)
	if err != nil {
		return "", err
	}
	return lease.LeaseID, nil
}

func (h *stage9MarketDataSubscriptionMutationHarness) predictionReleasePath(code string) string {
	return "/api/v1/market-data/prediction/contracts/" + code + "/subscriptions/fixture-lease"
}

func stage9MarketDataSubscriptionMutationRequest(
	t *testing.T,
	harness *stage9MarketDataSubscriptionMutationHarness,
	method, path, body, contextError string,
) (int, map[string]string, struct {
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}) {
	t.Helper()
	ctx := context.Background()
	var cancel context.CancelFunc
	if contextError == "canceled" {
		ctx, cancel = context.WithCancel(ctx)
		cancel()
	}
	request := httptest.NewRequestWithContext(ctx, method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	harness.router.ServeHTTP(recorder, request)
	var envelope struct {
		Data  json.RawMessage `json:"data"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode %s %s response: %v (%s)", method, path, err, recorder.Body.String())
	}
	headers := make(map[string]string)
	if retryAfter := recorder.Header().Get("Retry-After"); retryAfter != "" {
		headers["Retry-After"] = retryAfter
		return recorder.Code, headers, envelope
	}
	return recorder.Code, nil, envelope
}

func stage9MarketDataSubscriptionMutationProviderCall(
	harness *stage9MarketDataSubscriptionMutationHarness,
) map[string]any {
	call := map[string]any{}
	if len(harness.prediction.subscribeCalls) > 0 {
		call["subscribeCalls"] = predictionSubscriptionObservations(harness.prediction.subscribeCalls)
	}
	if len(harness.prediction.unsubscribeCalls) > 0 {
		call["unsubscribeCalls"] = predictionSubscriptionObservations(harness.prediction.unsubscribeCalls)
	}
	if len(call) == 0 {
		return nil
	}
	return call
}

func predictionSubscriptionObservations(
	calls []broker.PredictionSubscription,
) []any {
	observations := make([]any, 0, len(calls))
	for _, call := range calls {
		dataTypes := make([]any, 0, len(call.DataTypes))
		for _, dataType := range call.DataTypes {
			dataTypes = append(dataTypes, dataType)
		}
		observations = append(observations, map[string]any{
			"brokerId": call.BrokerID, "accountId": call.AccountID,
			"instrumentId": call.InstrumentID, "dataTypes": dataTypes,
		})
	}
	return observations
}

func normalizeStage9MarketDataSubscriptionMutationJSON(data json.RawMessage) json.RawMessage {
	if len(data) == 0 {
		return data
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return data
	}
	normalizeStage9MarketDataSubscriptionMutationValue(value)
	contents, err := json.Marshal(value)
	if err != nil {
		return data
	}
	return contents
}

func normalizeStage9MarketDataSubscriptionMutationValue(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			switch key {
			case "createdAt", "updatedAt", "asOf", "resolvedAt", "subscribedAt", "unsubscribeEligibleAt":
				if _, ok := child.(string); ok {
					typed[key] = "fixture-time"
					continue
				}
			case "leaseId":
				if _, ok := child.(string); ok {
					typed[key] = "fixture-lease"
					continue
				}
			}
			normalizeStage9MarketDataSubscriptionMutationValue(child)
			if key == "entries" || key == "byMarket" {
				if entries, ok := typed[key].([]any); ok {
					sort.SliceStable(entries, func(i, j int) bool {
						return subscriptionMutationSortKey(entries[i]) < subscriptionMutationSortKey(entries[j])
					})
					typed[key] = entries
				}
			}
		}
	case []any:
		for _, child := range typed {
			normalizeStage9MarketDataSubscriptionMutationValue(child)
		}
	}
}

func subscriptionMutationSortKey(value any) string {
	if object, ok := value.(map[string]any); ok {
		for _, field := range []string{"key", "market", "instrumentId"} {
			if text, ok := object[field].(string); ok {
				return text
			}
		}
	}
	return ""
}

func compactStage9MarketDataSubscriptionMutationJSON(data json.RawMessage) json.RawMessage {
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
