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

	productfeaturesapi "github.com/jftrade/jftrade-main/internal/api/productfeatures"
	productfeatures "github.com/jftrade/jftrade-main/internal/productfeatures"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

const stage9AlertsReadFixtureVersion = "stage9.alerts-read.v1"

var stage9AlertsReadNow = time.Date(2026, 8, 21, 4, 0, 0, 0, time.UTC)

type stage9AlertsReadCase struct {
	Name        string           `json:"name"`
	Method      string           `json:"method"`
	RequestPath string           `json:"requestPath"`
	FeatureID   broker.FeatureID `json:"featureId"`
	Query       map[string]any   `json:"query"`
	Response    map[string]any   `json:"response"`
}

type stage9AlertsReadFixture struct {
	Version   string                     `json:"version"`
	Cases     []stage9AlertsReadCase     `json:"cases"`
	WireCases []stage9AlertsReadWireCase `json:"wireCases"`
}

type stage9AlertsReadWireCase struct {
	Name           string            `json:"name"`
	Method         string            `json:"method"`
	RequestPath    string            `json:"requestPath"`
	ExpectedStatus int               `json:"expectedStatus"`
	Headers        map[string]string `json:"headers"`
	Envelope       map[string]any    `json:"envelope"`
}

// TestStage9AlertsReadFixtureMatchesCurrentGoOwner freezes both read-only
// customization projections as one route-group fixture. The fake broker keeps
// OpenD out of the migration rehearsal while the Gin route and product service
// still perform the real query normalization and capability resolution.
func TestStage9AlertsReadFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 alerts fixture source")
	}
	fixturePath := filepath.Join(
		filepath.Dir(source),
		"../../tests/fixtures/rust-migration/stage9/alerts-read.json",
	)
	adapter := &stage9AlertsReadBroker{}
	registry := broker.NewRegistry()
	registry.Register(adapter)
	service := productfeatures.NewService(registry, adapter.ID(), nil, nil)
	router := gin.New()
	productfeaturesapi.RegisterRoutes(router.Group("/api/v1"), service)

	cases := []struct {
		name      string
		path      string
		featureID broker.FeatureID
		assert    func(t *testing.T, query broker.FeatureQuery)
	}{
		{
			name:      "price-list",
			path:      "/api/v1/alerts/price?brokerId=futu&market=us&pageSize=2&enabled=true&threshold=100&tag=one&tag=two",
			featureID: broker.FeaturePriceAlertList,
			assert: func(t *testing.T, query broker.FeatureQuery) {
				t.Helper()
				if query.BrokerID != "futu" || query.Market != "US" || query.PageSize != 2 ||
					query.Params["enabled"] != true || query.Params["threshold"] != int64(100) {
					t.Fatalf("price query = %#v", query)
				}
				tags, ok := query.Params["tag"].([]any)
				if !ok || !reflect.DeepEqual(tags, []any{"one", "two"}) {
					t.Fatalf("price tags = %#v", query.Params["tag"])
				}
			},
		},
		{
			name:      "option-events-list",
			path:      "/api/v1/alerts/option-events?brokerId=futu&market=us&cursor=next&pageSize=3&operation=list&enabled=false",
			featureID: broker.FeatureOptionEventAlertList,
			assert: func(t *testing.T, query broker.FeatureQuery) {
				t.Helper()
				if query.BrokerID != "futu" || query.Market != "US" || query.Cursor != "next" ||
					query.PageSize != 3 || query.Params["operation"] != "list" ||
					query.Params["enabled"] != false {
					t.Fatalf("option event query = %#v", query)
				}
			},
		},
	}

	want := stage9AlertsReadFixture{
		Version:   stage9AlertsReadFixtureVersion,
		Cases:     make([]stage9AlertsReadCase, 0, len(cases)),
		WireCases: make([]stage9AlertsReadWireCase, 0, len(stage9AlertsReadWireCases())),
	}
	for _, testCase := range cases {
		adapter.lastQuery = broker.FeatureQuery{}
		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, testCase.path, nil)
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("case %s status=%d body=%s", testCase.name, recorder.Code, recorder.Body.String())
		}
		var envelope map[string]any
		if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("case %s decode response: %v", testCase.name, err)
		}
		if envelope["ok"] != true {
			t.Fatalf("case %s envelope = %#v", testCase.name, envelope)
		}
		testCase.assert(t, adapter.lastQuery)
		queryValue, err := jsonObject(adapter.lastQuery)
		if err != nil {
			t.Fatalf("case %s encode query: %v", testCase.name, err)
		}
		response, ok := envelope["data"].(map[string]any)
		if !ok {
			t.Fatalf("case %s data = %#v", testCase.name, envelope["data"])
		}
		normalizeAlertTimestamps(response)
		want.Cases = append(want.Cases, stage9AlertsReadCase{
			Name:        testCase.name,
			Method:      http.MethodGet,
			RequestPath: testCase.path,
			FeatureID:   testCase.featureID,
			Query:       queryValue,
			Response:    response,
		})
	}
	for _, testCase := range stage9AlertsReadWireCases() {
		adapter := &stage9AlertsReadBroker{
			queryErr: testCase.queryErr,
			empty:    testCase.empty,
		}
		registry := broker.NewRegistry()
		registry.Register(adapter)
		service := productfeatures.NewService(registry, adapter.ID(), nil, nil)
		router := gin.New()
		productfeaturesapi.RegisterRoutes(router.Group("/api/v1"), service)
		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(t.Context(), testCase.method, testCase.path, nil)
		router.ServeHTTP(recorder, request)
		var envelope map[string]any
		if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("wire case %s decode response: %v", testCase.name, err)
		}
		normalizeAlertEnvelope(envelope)
		headers := map[string]string{
			"Content-Type": recorder.Header().Get("Content-Type"),
		}
		if retryAfter := recorder.Header().Get("Retry-After"); retryAfter != "" {
			headers["Retry-After"] = retryAfter
		}
		want.WireCases = append(want.WireCases, stage9AlertsReadWireCase{
			Name:           testCase.name,
			Method:         testCase.method,
			RequestPath:    testCase.path,
			ExpectedStatus: recorder.Code,
			Headers:        headers,
			Envelope:       envelope,
		})
	}

	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode alerts fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write alerts fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read alerts fixture: %v", err)
	}
	var got stage9AlertsReadFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode alerts fixture: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stage 9 alerts fixture drifted from the Go owner")
	}
}

func normalizeAlertTimestamps(response map[string]any) {
	stamp := stage9AlertsReadNow.Format(time.RFC3339Nano)
	response["asOf"] = stamp
	if provider, ok := response["provider"].(map[string]any); ok {
		provider["resolvedAt"] = stamp
		provider["asOf"] = stamp
	}
}

func normalizeAlertEnvelope(envelope map[string]any) {
	envelope["timestamp"] = stage9AlertsReadNow.Format(time.RFC3339Nano)
	if data, ok := envelope["data"].(map[string]any); ok {
		normalizeAlertTimestamps(data)
	}
}

type stage9AlertsReadWireInput struct {
	name     string
	path     string
	method   string
	queryErr error
	empty    bool
}

func stage9AlertsReadWireCases() []stage9AlertsReadWireInput {
	return []stage9AlertsReadWireInput{
		{
			name:   "price-empty-result",
			method: http.MethodGet,
			path:   "/api/v1/alerts/price?brokerId=futu&market=us",
			empty:  true,
		},
		{
			name:   "option-events-missing-broker",
			method: http.MethodGet,
			path:   "/api/v1/alerts/option-events?brokerId=missing&market=us",
		},
		{
			name:     "price-provider-failure",
			method:   http.MethodGet,
			path:     "/api/v1/alerts/price?brokerId=futu&market=us&fixture=error",
			queryErr: errors.New("fixture provider failed"),
		},
	}
}

func jsonObject(value any) (map[string]any, error) {
	contents, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(contents, &result); err != nil {
		return nil, err
	}
	return result, nil
}

type stage9AlertsReadBroker struct {
	lastQuery broker.FeatureQuery
	queryErr  error
	empty     bool
}

func (b *stage9AlertsReadBroker) ID() string { return "futu" }

func (b *stage9AlertsReadBroker) Descriptor() broker.Descriptor {
	return broker.Descriptor{
		ID:                b.ID(),
		DisplayName:       "Futu fixture",
		SecurityFirm:      "Futu/Moomoo via OpenD",
		CapabilityVersion: "stage9-alerts-fixture",
		Environments:      []string{"SIMULATE"},
		Capabilities: []broker.MarketCapability{{
			Market: "US",
			Features: []broker.FeatureCapability{
				{ID: broker.FeaturePriceAlertList, Markets: []string{"US"}, Access: broker.FeatureAccessRead, State: broker.CapabilityAvailable},
				{ID: broker.FeatureOptionEventAlertList, Markets: []string{"US"}, Access: broker.FeatureAccessRead, State: broker.CapabilityAvailable},
			},
		}},
	}
}

func (*stage9AlertsReadBroker) DiscoverAccounts(context.Context) ([]broker.Account, error) {
	return nil, nil
}

func (*stage9AlertsReadBroker) Trading() broker.TradingService      { return nil }
func (*stage9AlertsReadBroker) MarketData() broker.MarketDataReader { return nil }

func (b *stage9AlertsReadBroker) QueryCustomization(
	_ context.Context,
	query broker.FeatureQuery,
) (*broker.FeatureResult, error) {
	b.lastQuery = query
	if b.queryErr != nil {
		return nil, b.queryErr
	}
	if b.empty {
		hasMore := false
		total := 0
		return &broker.FeatureResult{
			AsOf:     stage9AlertsReadNow,
			Entries:  []map[string]any{},
			HasMore:  &hasMore,
			Total:    &total,
			Metadata: map[string]any{"source": "fixture"},
		}, nil
	}
	var entries []map[string]any
	switch query.FeatureID {
	case broker.FeaturePriceAlertList:
		entries = []map[string]any{{
			"key":          int64(101),
			"enabled":      true,
			"instrumentId": "US.AAPL",
			"type":         "price_up",
			"target":       190.5,
			"frequency":    "once_a_day",
			"sessions":     []any{"regular"},
		}}
	case broker.FeatureOptionEventAlertList:
		entries = []map[string]any{{
			"key":               int64(202),
			"enabled":           false,
			"optionMarket":      "US",
			"underlying":        map[string]any{"market": "US", "code": "AAPL", "instrumentId": "US.AAPL"},
			"optionType":        "call",
			"sideTypeList":      []any{"buy"},
			"orderTypeList":     []any{"limit"},
			"earningsDateBegin": "2026-09-01",
			"note":              "fixture alert",
		}}
	default:
		return nil, nil
	}
	hasMore := false
	total := len(entries)
	return &broker.FeatureResult{
		AsOf:     stage9AlertsReadNow,
		Entries:  entries,
		HasMore:  &hasMore,
		Total:    &total,
		Metadata: map[string]any{"source": "fixture"},
	}, nil
}

func (*stage9AlertsReadBroker) ApplyCustomization(
	context.Context,
	broker.CustomizationAction,
) (*broker.CustomizationResult, error) {
	return nil, nil
}

var _ broker.Broker = (*stage9AlertsReadBroker)(nil)
var _ broker.CustomizationService = (*stage9AlertsReadBroker)(nil)
