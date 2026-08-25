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
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	productfeaturesapi "github.com/jftrade/jftrade-main/internal/api/productfeatures"
	productfeatures "github.com/jftrade/jftrade-main/internal/productfeatures"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

const stage9AlertsWriteFixtureVersion = "stage9.alerts-write.v1"

var stage9AlertsWriteNow = time.Date(2026, 8, 22, 4, 0, 0, 0, time.UTC)

type stage9AlertsWriteCase struct {
	Name      string                            `json:"name"`
	Requests  []stage9AlertsWriteFixtureRequest `json:"requests"`
	FeatureID broker.FeatureID                  `json:"featureId"`
	Action    string                            `json:"action"`
	PortMode  string                            `json:"portMode"`
	Expected  []stage9AlertsWriteExpected       `json:"expected"`
	Calls     stage9AlertsWriteCallTrace        `json:"calls"`
}

type stage9AlertsWriteFixtureRequest struct {
	Method      string  `json:"method"`
	RequestPath string  `json:"requestPath"`
	Body        *string `json:"body,omitempty"`
	Context     string  `json:"context,omitempty"`
}

type stage9AlertsWriteExpected struct {
	Status   int               `json:"status"`
	Headers  map[string]string `json:"headers,omitempty"`
	PortCall bool              `json:"portCall"`
	Envelope map[string]any    `json:"envelope"`
}

type stage9AlertsWriteCallTrace struct {
	Apply        int              `json:"apply"`
	Actions      []map[string]any `json:"actions,omitempty"`
	PayloadState []string         `json:"payloadState,omitempty"`
}

type stage9AlertsWriteFixture struct {
	Version string                  `json:"version"`
	Cases   []stage9AlertsWriteCase `json:"cases"`
}

// TestStage9AlertsWriteFixtureMatchesCurrentGoOwner freezes both mutation
// routes through the real Gin handler and product feature service. The fake
// broker records the action only; it never connects OpenD or writes product
// state.
func TestStage9AlertsWriteFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 alerts-write fixture source")
	}
	fixturePath := filepath.Join(
		filepath.Dir(source),
		"../../tests/fixtures/rust-migration/stage9/alerts-write.json",
	)

	want := stage9AlertsWriteFixture{
		Version: stage9AlertsWriteFixtureVersion,
		Cases:   make([]stage9AlertsWriteCase, 0, len(stage9AlertsWriteCases())),
	}
	for _, testCase := range stage9AlertsWriteCases() {
		adapter, registryBroker := stage9AlertsWriteBrokerForCase(testCase)
		registry := broker.NewRegistry()
		registry.Register(adapter)
		service := productfeatures.NewService(registry, "futu", nil, nil)
		router := gin.New()
		productfeaturesapi.RegisterRoutes(router.Group("/api/v1"), service)

		requests := testCase.Requests
		if len(requests) == 0 {
			requests = []stage9AlertsWriteFixtureRequest{{
				Method:      testCase.Method,
				RequestPath: testCase.RequestPath,
				Body:        testCase.Body,
			}}
		}
		expected := make([]stage9AlertsWriteExpected, 0, len(requests))
		for _, request := range requests {
			beforeApply := 0
			if registryBroker != nil {
				beforeApply = registryBroker.applyCalls
			}
			response := stage9AlertsWriteRequest(t, router, request)
			if response.Code == 0 {
				t.Fatalf("case %s did not produce a response", testCase.Name)
			}
			var envelope map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("case %s decode response: %v", testCase.Name, err)
			}
			stage9NormalizeAlertsWriteEnvelope(t, testCase.Name, envelope)
			afterApply := beforeApply
			if registryBroker != nil {
				afterApply = registryBroker.applyCalls
			}
			headers := map[string]string{
				"Content-Type": "application/json; charset=utf-8",
			}
			if retryAfter := response.Header().Values("Retry-After"); len(retryAfter) > 0 {
				headers["Retry-After"] = retryAfter[0]
			}
			expected = append(expected, stage9AlertsWriteExpected{
				Status:   response.Code,
				Headers:  headers,
				PortCall: afterApply > beforeApply,
				Envelope: envelope,
			})
		}

		callTrace := stage9AlertsWriteCallTrace{}
		if registryBroker != nil {
			callTrace.Apply = registryBroker.applyCalls
			for _, action := range registryBroker.actions {
				mapped, err := stage9AlertsWriteJSONMap(action)
				if err != nil {
					t.Fatalf("case %s encode action: %v", testCase.Name, err)
				}
				callTrace.Actions = append(callTrace.Actions, mapped)
				callTrace.PayloadState = append(
					callTrace.PayloadState,
					stage9AlertsWritePayloadState(action),
				)
			}
		}
		want.Cases = append(want.Cases, stage9AlertsWriteCase{
			Name:      testCase.Name,
			Requests:  requests,
			FeatureID: testCase.FeatureID,
			Action:    testCase.Action,
			PortMode:  testCase.PortMode,
			Expected:  expected,
			Calls:     callTrace,
		})
	}

	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode alerts-write fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write alerts-write fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read alerts-write fixture: %v", err)
	}
	var got stage9AlertsWriteFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode alerts-write fixture: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stage 9 alerts-write fixture drifted from the Go owner")
	}
}

type stage9AlertsWriteInput struct {
	Name        string
	Requests    []stage9AlertsWriteFixtureRequest
	Method      string
	RequestPath string
	Body        *string
	FeatureID   broker.FeatureID
	Action      string
	PortMode    string
}

func stage9AlertsWriteCases() []stage9AlertsWriteInput {
	body := func(value string) *string { return &value }
	request := func(path string, value *string) stage9AlertsWriteFixtureRequest {
		return stage9AlertsWriteFixtureRequest{
			Method:      http.MethodPost,
			RequestPath: path,
			Body:        value,
		}
	}
	requestWithContext := func(path string, value *string, contextMode string) stage9AlertsWriteFixtureRequest {
		result := request(path, value)
		result.Context = contextMode
		return result
	}
	return []stage9AlertsWriteInput{
		{
			Name:        "price-success",
			Method:      http.MethodPost,
			RequestPath: "/api/v1/alerts/price?brokerId=futu&accountId=acct-1",
			Body:        body(`{"symbol":"US.AAPL","price":190.5,"enabled":true}`),
			FeatureID:   broker.FeaturePriceAlertSet,
			Action:      "set",
			PortMode:    "success",
		},
		{
			Name:        "option-events-success-repeated-broker-query",
			Method:      http.MethodPost,
			RequestPath: "/api/v1/alerts/option-events?brokerId=futu&brokerId=ignored&accountId=acct-2",
			Body:        body(`{"operation":"add","alertList":[{"key":202}]}`),
			FeatureID:   broker.FeatureOptionEventAlertSet,
			Action:      "set",
			PortMode:    "success",
		},
		{
			Name:        "price-null-body-nil-result",
			Method:      http.MethodPost,
			RequestPath: "/api/v1/alerts/price?brokerId=futu",
			Body:        body("null"),
			FeatureID:   broker.FeaturePriceAlertSet,
			Action:      "set",
			PortMode:    "nil-result",
		},
		{
			Name:        "empty-body-wins-over-capability",
			Method:      http.MethodPost,
			RequestPath: "/api/v1/alerts/price?brokerId=missing",
			FeatureID:   broker.FeaturePriceAlertSet,
			Action:      "set",
			PortMode:    "success",
		},
		{
			Name:        "option-events-empty-object",
			Method:      http.MethodPost,
			RequestPath: "/api/v1/alerts/option-events?brokerId=futu",
			Body:        body("{}"),
			FeatureID:   broker.FeatureOptionEventAlertSet,
			Action:      "set",
			PortMode:    "empty-result",
		},
		{
			Name:        "malformed-body-wins-over-capability",
			Method:      http.MethodPost,
			RequestPath: "/api/v1/alerts/price?brokerId=missing",
			Body:        body("{"),
			FeatureID:   broker.FeaturePriceAlertSet,
			Action:      "set",
			PortMode:    "success",
		},
		{
			Name:        "array-body-rejected",
			Method:      http.MethodPost,
			RequestPath: "/api/v1/alerts/option-events?brokerId=futu",
			Body:        body("[]"),
			FeatureID:   broker.FeatureOptionEventAlertSet,
			Action:      "set",
			PortMode:    "success",
		},
		{
			Name:        "missing-broker-capability",
			Method:      http.MethodPost,
			RequestPath: "/api/v1/alerts/price?brokerId=missing",
			Body:        body(`{"symbol":"US.AAPL","price":100}`),
			FeatureID:   broker.FeaturePriceAlertSet,
			Action:      "set",
			PortMode:    "missing-broker",
		},
		{
			Name:        "declared-capability-unavailable",
			Method:      http.MethodPost,
			RequestPath: "/api/v1/alerts/option-events?brokerId=futu",
			Body:        body(`{"operation":"disable"}`),
			FeatureID:   broker.FeatureOptionEventAlertSet,
			Action:      "set",
			PortMode:    "capability-unavailable",
		},
		{
			Name:        "customization-adapter-unavailable",
			Method:      http.MethodPost,
			RequestPath: "/api/v1/alerts/price?brokerId=futu",
			Body:        body(`{"symbol":"US.AAPL","price":100}`),
			FeatureID:   broker.FeaturePriceAlertSet,
			Action:      "set",
			PortMode:    "adapter-unavailable",
		},
		{
			Name:        "provider-http-forbidden",
			Method:      http.MethodPost,
			RequestPath: "/api/v1/alerts/option-events?brokerId=futu",
			Body:        body(`{"operation":"delete","key":202}`),
			FeatureID:   broker.FeatureOptionEventAlertSet,
			Action:      "set",
			PortMode:    "provider-http-403",
		},
		{
			Name:        "provider-unavailable",
			Method:      http.MethodPost,
			RequestPath: "/api/v1/alerts/price?brokerId=futu",
			Body:        body(`{"symbol":"US.AAPL","price":100}`),
			FeatureID:   broker.FeaturePriceAlertSet,
			Action:      "set",
			PortMode:    "provider-unavailable",
		},
		{
			Name:        "internal-write-failure",
			Method:      http.MethodPost,
			RequestPath: "/api/v1/alerts/option-events?brokerId=futu",
			Body:        body(`{"operation":"modify","key":202}`),
			FeatureID:   broker.FeatureOptionEventAlertSet,
			Action:      "set",
			PortMode:    "internal-failure",
		},
		{
			Name:        "generic-rate-limit-retry-after",
			Method:      http.MethodPost,
			RequestPath: "/api/v1/alerts/price?brokerId=futu",
			Body:        body(`{"symbol":"US.AAPL","price":100}`),
			FeatureID:   broker.FeaturePriceAlertSet,
			Action:      "set",
			PortMode:    "rate-limit",
		},
		{
			Name: "price-repeated-write-is-forwarded-twice",
			Requests: []stage9AlertsWriteFixtureRequest{
				request("/api/v1/alerts/price?brokerId=futu&accountId=acct-3", body(`{"symbol":"US.AAPL","price":100}`)),
				request("/api/v1/alerts/price?brokerId=futu&accountId=acct-3", body(`{"symbol":"US.AAPL","price":100}`)),
			},
			FeatureID: broker.FeaturePriceAlertSet,
			Action:    "set",
			PortMode:  "success",
		},
		{
			Name: "option-events-failed-write-recovers-on-next-request",
			Requests: []stage9AlertsWriteFixtureRequest{
				request("/api/v1/alerts/option-events?brokerId=futu", body(`{"operation":"modify","key":202}`)),
				request("/api/v1/alerts/option-events?brokerId=futu", body(`{"operation":"modify","key":202}`)),
			},
			FeatureID: broker.FeatureOptionEventAlertSet,
			Action:    "set",
			PortMode:  "failure-then-success",
		},
		{
			Name: "price-cancelled-request-defaults-to-broker-failure",
			Requests: []stage9AlertsWriteFixtureRequest{
				requestWithContext(
					"/api/v1/alerts/price?brokerId=futu",
					body(`{"symbol":"US.AAPL","price":100}`),
					"canceled",
				),
			},
			FeatureID: broker.FeaturePriceAlertSet,
			Action:    "set",
			PortMode:  "context-error",
		},
		{
			Name: "option-events-deadline-request-defaults-to-broker-failure",
			Requests: []stage9AlertsWriteFixtureRequest{
				requestWithContext(
					"/api/v1/alerts/option-events?brokerId=futu",
					body(`{"operation":"modify","key":202}`),
					"deadline",
				),
			},
			FeatureID: broker.FeatureOptionEventAlertSet,
			Action:    "set",
			PortMode:  "context-error",
		},
	}
}

func stage9AlertsWriteRequest(
	t *testing.T,
	router http.Handler,
	fixtureRequest stage9AlertsWriteFixtureRequest,
) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if fixtureRequest.Body == nil {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(*fixtureRequest.Body)
	}
	requestContext := t.Context()
	var cancel context.CancelFunc
	switch fixtureRequest.Context {
	case "canceled":
		requestContext, cancel = context.WithCancel(requestContext)
		cancel()
	case "deadline":
		requestContext, cancel = context.WithDeadline(requestContext, time.Unix(1, 0))
		defer cancel()
	}
	request := httptest.NewRequestWithContext(
		requestContext, fixtureRequest.Method, fixtureRequest.RequestPath, reader,
	)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func stage9NormalizeAlertsWriteEnvelope(t *testing.T, name string, envelope map[string]any) {
	t.Helper()
	timestamp, ok := envelope["timestamp"].(string)
	if !ok {
		t.Fatalf("case %s response timestamp = %#v", name, envelope["timestamp"])
	}
	if _, err := time.Parse(time.RFC3339Nano, timestamp); err != nil {
		t.Fatalf("case %s response timestamp = %q: %v", name, timestamp, err)
	}
	envelope["timestamp"] = stage9AlertsWriteNow.Format(time.RFC3339Nano)
	if data, ok := envelope["data"].(map[string]any); ok {
		if provider, ok := data["provider"].(map[string]any); ok {
			stamp := stage9AlertsWriteNow.Format(time.RFC3339Nano)
			provider["resolvedAt"] = stamp
			provider["asOf"] = stamp
		}
	}
}

func stage9AlertsWriteJSONMap(value any) (map[string]any, error) {
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

func stage9AlertsWritePayloadState(action broker.CustomizationAction) string {
	if action.Payload == nil {
		return "nil"
	}
	if len(action.Payload) == 0 {
		return "empty_object"
	}
	return "object"
}

func stage9AlertsWriteBrokerForCase(
	testCase stage9AlertsWriteInput,
) (broker.Broker, *stage9AlertsWriteBroker) {
	if testCase.PortMode == "adapter-unavailable" {
		return &stage9AlertsWriteBareBroker{}, nil
	}
	state := broker.CapabilityAvailable
	if testCase.PortMode == "capability-unavailable" {
		state = broker.CapabilityUnavailable
	}
	adapter := &stage9AlertsWriteBroker{mode: testCase.PortMode, state: state}
	return adapter, adapter
}

type stage9AlertsWriteBroker struct {
	mode       string
	state      broker.CapabilityState
	applyCalls int
	actions    []broker.CustomizationAction
}

func (b *stage9AlertsWriteBroker) ID() string { return "futu" }

func (b *stage9AlertsWriteBroker) Descriptor() broker.Descriptor {
	return stage9AlertsWriteDescriptor(b.ID(), b.state)
}

func (*stage9AlertsWriteBroker) DiscoverAccounts(context.Context) ([]broker.Account, error) {
	return nil, nil
}

func (*stage9AlertsWriteBroker) Trading() broker.TradingService      { return nil }
func (*stage9AlertsWriteBroker) MarketData() broker.MarketDataReader { return nil }

func (*stage9AlertsWriteBroker) QueryCustomization(
	context.Context,
	broker.FeatureQuery,
) (*broker.FeatureResult, error) {
	return nil, nil
}

func (b *stage9AlertsWriteBroker) ApplyCustomization(
	ctx context.Context,
	action broker.CustomizationAction,
) (*broker.CustomizationResult, error) {
	b.applyCalls++
	b.actions = append(b.actions, action)
	switch b.mode {
	case "provider-http-403":
		return nil, &stage9AlertsWriteHTTPError{status: http.StatusForbidden, message: "provider denied alert write"}
	case "provider-unavailable":
		return nil, errors.New("provider unavailable")
	case "internal-failure":
		return nil, errors.New("write failed")
	case "rate-limit":
		return nil, broker.NewSnapshotRateLimitError(2500*time.Millisecond, nil)
	case "context-error":
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("context canceled")
	case "failure-then-success":
		if b.applyCalls == 1 {
			return nil, errors.New("write failed")
		}
	case "nil-result":
		return nil, nil
	case "empty-result":
		return &broker.CustomizationResult{Entries: []map[string]any{}}, nil
	}
	return &broker.CustomizationResult{Entries: []map[string]any{{
		"accepted":  true,
		"featureId": action.FeatureID,
		"operation": action.Action,
	}}}, nil
}

type stage9AlertsWriteBareBroker struct{}

func (*stage9AlertsWriteBareBroker) ID() string { return "futu" }

func (*stage9AlertsWriteBareBroker) Descriptor() broker.Descriptor {
	return stage9AlertsWriteDescriptor("futu", broker.CapabilityAvailable)
}

func (*stage9AlertsWriteBareBroker) DiscoverAccounts(context.Context) ([]broker.Account, error) {
	return nil, nil
}

func (*stage9AlertsWriteBareBroker) Trading() broker.TradingService      { return nil }
func (*stage9AlertsWriteBareBroker) MarketData() broker.MarketDataReader { return nil }

func stage9AlertsWriteDescriptor(id string, state broker.CapabilityState) broker.Descriptor {
	return broker.Descriptor{
		ID:                id,
		DisplayName:       "Futu fixture",
		SecurityFirm:      "Futu/Moomoo via OpenD",
		CapabilityVersion: "stage9-alerts-write-fixture",
		Environments:      []string{"SIMULATE"},
		Capabilities: []broker.MarketCapability{{
			Market: "US",
			Features: []broker.FeatureCapability{
				{ID: broker.FeaturePriceAlertSet, Markets: []string{"US"}, Access: broker.FeatureAccessWrite, State: state},
				{ID: broker.FeatureOptionEventAlertSet, Markets: []string{"US"}, Access: broker.FeatureAccessWrite, State: state},
			},
		}},
	}
}

type stage9AlertsWriteHTTPError struct {
	status  int
	message string
}

func (e *stage9AlertsWriteHTTPError) Error() string   { return e.message }
func (e *stage9AlertsWriteHTTPError) HTTPStatus() int { return e.status }

var _ broker.Broker = (*stage9AlertsWriteBroker)(nil)
var _ broker.CustomizationService = (*stage9AlertsWriteBroker)(nil)
var _ broker.Broker = (*stage9AlertsWriteBareBroker)(nil)
