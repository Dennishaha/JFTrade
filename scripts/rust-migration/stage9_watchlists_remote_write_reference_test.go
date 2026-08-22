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

const stage9WatchlistsRemoteWriteFixtureVersion = "stage9.watchlists-remote-write.v1"

var stage9WatchlistsRemoteWriteNow = time.Date(2026, 8, 22, 4, 0, 0, 0, time.UTC)

type stage9WatchlistsRemoteWriteFixture struct {
	Version   string                                   `json:"version"`
	Timestamp string                                   `json:"timestamp"`
	Cases     []stage9WatchlistsRemoteWriteFixtureCase `json:"cases"`
}

type stage9WatchlistsRemoteWriteFixtureCase struct {
	Name      string                                      `json:"name"`
	Requests  []stage9WatchlistsRemoteWriteFixtureRequest `json:"requests"`
	FeatureID broker.FeatureID                            `json:"featureId"`
	Action    string                                      `json:"action"`
	PortMode  string                                      `json:"portMode"`
	Expected  []stage9WatchlistsRemoteWriteExpected       `json:"expected"`
	Calls     stage9WatchlistsRemoteWriteCallTrace        `json:"calls"`
}

type stage9WatchlistsRemoteWriteFixtureRequest struct {
	Method      string  `json:"method"`
	RequestPath string  `json:"requestPath"`
	Body        *string `json:"body,omitempty"`
	Context     string  `json:"context,omitempty"`
}

type stage9WatchlistsRemoteWriteExpected struct {
	Status   int               `json:"status"`
	Headers  map[string]string `json:"headers"`
	PortCall bool              `json:"portCall"`
	Envelope map[string]any    `json:"envelope"`
}

type stage9WatchlistsRemoteWriteCallTrace struct {
	Apply        int              `json:"apply"`
	Actions      []map[string]any `json:"actions,omitempty"`
	PayloadState []string         `json:"payloadState,omitempty"`
}

// TestStage9WatchlistsRemoteWriteFixtureMatchesCurrentGoOwner freezes the
// generic Gin customization handler, product-feature router, and broker
// adapter boundary. The fake broker never connects OpenD or mutates a remote
// watchlist; it records only the observable action sent by the Go owner.
func TestStage9WatchlistsRemoteWriteFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 watchlists-remote-write fixture source")
	}
	fixturePath := filepath.Join(
		filepath.Dir(source), "../../tests/fixtures/rust-migration/stage9/watchlists-remote-write.json",
	)

	want := stage9WatchlistsRemoteWriteFixture{
		Version:   stage9WatchlistsRemoteWriteFixtureVersion,
		Timestamp: stage9WatchlistsRemoteWriteNow.Format(time.RFC3339Nano),
		Cases:     make([]stage9WatchlistsRemoteWriteFixtureCase, 0, len(stage9WatchlistsRemoteWriteCases())),
	}
	for _, testCase := range stage9WatchlistsRemoteWriteCases() {
		adapter, registryBroker := stage9WatchlistsRemoteWriteBrokerForCase(testCase)
		registry := broker.NewRegistry()
		registry.Register(adapter)
		service := productfeatures.NewService(registry, "futu", nil, nil)
		router := gin.New()
		productfeaturesapi.RegisterRoutes(router.Group("/api/v1"), service)

		expected := make([]stage9WatchlistsRemoteWriteExpected, 0, len(testCase.Requests))
		for _, request := range testCase.Requests {
			beforeApply := 0
			if registryBroker != nil {
				beforeApply = registryBroker.applyCalls
			}
			response := stage9WatchlistsRemoteWriteRequest(
				t, router, request.Method, request.RequestPath, request.Body, request.Context,
			)
			if response.Code == 0 {
				t.Fatalf("case %s did not produce a response", testCase.Name)
			}
			var envelope map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("case %s decode response: %v", testCase.Name, err)
			}
			stage9NormalizeWatchlistsRemoteWriteEnvelope(t, testCase.Name, envelope)
			afterApply := beforeApply
			if registryBroker != nil {
				afterApply = registryBroker.applyCalls
			}
			headers := map[string]string{
				"Content-Type": response.Header().Get("Content-Type"),
			}
			if retryAfter := response.Header().Get("Retry-After"); retryAfter != "" {
				headers["Retry-After"] = retryAfter
			}
			expected = append(expected, stage9WatchlistsRemoteWriteExpected{
				Status:   response.Code,
				Headers:  headers,
				PortCall: afterApply > beforeApply,
				Envelope: envelope,
			})
		}

		calls := stage9WatchlistsRemoteWriteCallTrace{}
		if registryBroker != nil {
			calls.Apply = registryBroker.applyCalls
			for _, action := range registryBroker.actions {
				mapped, err := stage9WatchlistsRemoteWriteJSONMap(action)
				if err != nil {
					t.Fatalf("case %s encode action: %v", testCase.Name, err)
				}
				calls.Actions = append(calls.Actions, mapped)
				calls.PayloadState = append(calls.PayloadState, stage9WatchlistsRemoteWritePayloadState(action))
			}
		}
		want.Cases = append(want.Cases, stage9WatchlistsRemoteWriteFixtureCase{
			Name:      testCase.Name,
			Requests:  testCase.Requests,
			FeatureID: testCase.FeatureID,
			Action:    testCase.Action,
			PortMode:  testCase.PortMode,
			Expected:  expected,
			Calls:     calls,
		})
	}

	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode watchlists-remote-write fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write watchlists-remote-write fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read watchlists-remote-write fixture: %v", err)
	}
	var got stage9WatchlistsRemoteWriteFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode watchlists-remote-write fixture: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stage 9 watchlists-remote-write fixture drifted from the Go owner")
	}
}

type stage9WatchlistsRemoteWriteInput struct {
	Name      string
	Requests  []stage9WatchlistsRemoteWriteFixtureRequest
	FeatureID broker.FeatureID
	Action    string
	PortMode  string
}

func stage9WatchlistsRemoteWriteCases() []stage9WatchlistsRemoteWriteInput {
	body := func(value string) *string { return &value }
	request := func(path string, value *string) stage9WatchlistsRemoteWriteFixtureRequest {
		return stage9WatchlistsRemoteWriteFixtureRequest{
			Method: http.MethodPost, RequestPath: path, Body: value,
		}
	}
	return []stage9WatchlistsRemoteWriteInput{
		{
			Name: "explicit-success",
			Requests: []stage9WatchlistsRemoteWriteFixtureRequest{request(
				"/api/v1/watchlists/remote?brokerId=futu&accountId=acct-1",
				body(`{"groupName":"Favorites","op":1,"securityList":[{"market":11,"code":"AAPL"}]}`),
			)},
			FeatureID: broker.FeatureRemoteWatchlistModify, Action: "modify", PortMode: "success",
		},
		{
			Name: "default-broker-success",
			Requests: []stage9WatchlistsRemoteWriteFixtureRequest{request(
				"/api/v1/watchlists/remote",
				body(`{"groupName":"Tech","op":2,"securityList":[]}`),
			)},
			FeatureID: broker.FeatureRemoteWatchlistModify, Action: "modify", PortMode: "success",
		},
		{
			Name: "repeated-broker-query-first-value",
			Requests: []stage9WatchlistsRemoteWriteFixtureRequest{request(
				"/api/v1/watchlists/remote?brokerId=futu&brokerId=ignored&accountId=acct-2&accountId=ignored",
				body(`{"groupName":"Favorites","op":3,"securityList":[{"market":11,"code":"MSFT"}]}`),
			)},
			FeatureID: broker.FeatureRemoteWatchlistModify, Action: "modify", PortMode: "success",
		},
		{
			Name: "null-body-nil-payload",
			Requests: []stage9WatchlistsRemoteWriteFixtureRequest{request(
				"/api/v1/watchlists/remote?brokerId=futu",
				body("null"),
			)},
			FeatureID: broker.FeatureRemoteWatchlistModify, Action: "modify", PortMode: "nil-result",
		},
		{
			Name: "empty-object-payload",
			Requests: []stage9WatchlistsRemoteWriteFixtureRequest{request(
				"/api/v1/watchlists/remote?brokerId=futu",
				body("{}"),
			)},
			FeatureID: broker.FeatureRemoteWatchlistModify, Action: "modify", PortMode: "empty-result",
		},
		{
			Name: "empty-body-wins-over-capability",
			Requests: []stage9WatchlistsRemoteWriteFixtureRequest{request(
				"/api/v1/watchlists/remote?brokerId=missing",
				nil,
			)},
			FeatureID: broker.FeatureRemoteWatchlistModify, Action: "modify", PortMode: "success",
		},
		{
			Name: "malformed-body-wins-over-capability",
			Requests: []stage9WatchlistsRemoteWriteFixtureRequest{request(
				"/api/v1/watchlists/remote?brokerId=missing",
				body("{"),
			)},
			FeatureID: broker.FeatureRemoteWatchlistModify, Action: "modify", PortMode: "success",
		},
		{
			Name: "array-body-rejected",
			Requests: []stage9WatchlistsRemoteWriteFixtureRequest{request(
				"/api/v1/watchlists/remote?brokerId=futu",
				body("[]"),
			)},
			FeatureID: broker.FeatureRemoteWatchlistModify, Action: "modify", PortMode: "success",
		},
		{
			Name: "missing-broker-capability",
			Requests: []stage9WatchlistsRemoteWriteFixtureRequest{request(
				"/api/v1/watchlists/remote?brokerId=missing",
				body(`{"groupName":"Favorites","op":1}`),
			)},
			FeatureID: broker.FeatureRemoteWatchlistModify, Action: "modify", PortMode: "missing-broker",
		},
		{
			Name: "declared-capability-unavailable",
			Requests: []stage9WatchlistsRemoteWriteFixtureRequest{request(
				"/api/v1/watchlists/remote?brokerId=futu",
				body(`{"groupName":"Favorites","op":1}`),
			)},
			FeatureID: broker.FeatureRemoteWatchlistModify, Action: "modify", PortMode: "capability-unavailable",
		},
		{
			Name: "customization-adapter-unavailable",
			Requests: []stage9WatchlistsRemoteWriteFixtureRequest{request(
				"/api/v1/watchlists/remote?brokerId=futu",
				body(`{"groupName":"Favorites","op":1}`),
			)},
			FeatureID: broker.FeatureRemoteWatchlistModify, Action: "modify", PortMode: "adapter-unavailable",
		},
		{
			Name: "provider-http-forbidden",
			Requests: []stage9WatchlistsRemoteWriteFixtureRequest{request(
				"/api/v1/watchlists/remote?brokerId=futu",
				body(`{"groupName":"Favorites","op":1}`),
			)},
			FeatureID: broker.FeatureRemoteWatchlistModify, Action: "modify", PortMode: "provider-http-403",
		},
		{
			Name: "provider-unavailable",
			Requests: []stage9WatchlistsRemoteWriteFixtureRequest{request(
				"/api/v1/watchlists/remote?brokerId=futu",
				body(`{"groupName":"Favorites","op":1}`),
			)},
			FeatureID: broker.FeatureRemoteWatchlistModify, Action: "modify", PortMode: "provider-unavailable",
		},
		{
			Name: "internal-write-failure",
			Requests: []stage9WatchlistsRemoteWriteFixtureRequest{request(
				"/api/v1/watchlists/remote?brokerId=futu",
				body(`{"groupName":"Favorites","op":1}`),
			)},
			FeatureID: broker.FeatureRemoteWatchlistModify, Action: "modify", PortMode: "internal-failure",
		},
		{
			Name: "generic-rate-limit-retry-after",
			Requests: []stage9WatchlistsRemoteWriteFixtureRequest{request(
				"/api/v1/watchlists/remote?brokerId=futu",
				body(`{"groupName":"Favorites","op":1}`),
			)},
			FeatureID: broker.FeatureRemoteWatchlistModify, Action: "modify", PortMode: "rate-limit",
		},
		{
			Name: "cancelled-request-defaults-to-broker-failure",
			Requests: []stage9WatchlistsRemoteWriteFixtureRequest{{
				Method: http.MethodPost, RequestPath: "/api/v1/watchlists/remote?brokerId=futu",
				Body: body(`{"groupName":"Favorites","op":1}`), Context: "canceled",
			}},
			FeatureID: broker.FeatureRemoteWatchlistModify, Action: "modify", PortMode: "context-error",
		},
		{
			Name: "deadline-request-defaults-to-broker-failure",
			Requests: []stage9WatchlistsRemoteWriteFixtureRequest{{
				Method: http.MethodPost, RequestPath: "/api/v1/watchlists/remote?brokerId=futu",
				Body: body(`{"groupName":"Favorites","op":1}`), Context: "deadline",
			}},
			FeatureID: broker.FeatureRemoteWatchlistModify, Action: "modify", PortMode: "context-error",
		},
		{
			Name: "repeated-write-is-forwarded-twice",
			Requests: []stage9WatchlistsRemoteWriteFixtureRequest{
				request("/api/v1/watchlists/remote?brokerId=futu&accountId=acct-3", body(`{"groupName":"Favorites","op":1}`)),
				request("/api/v1/watchlists/remote?brokerId=futu&accountId=acct-3", body(`{"groupName":"Favorites","op":1}`)),
			},
			FeatureID: broker.FeatureRemoteWatchlistModify, Action: "modify", PortMode: "success",
		},
		{
			Name: "failed-write-recovers-on-next-request",
			Requests: []stage9WatchlistsRemoteWriteFixtureRequest{
				request("/api/v1/watchlists/remote?brokerId=futu", body(`{"groupName":"Favorites","op":1}`)),
				request("/api/v1/watchlists/remote?brokerId=futu", body(`{"groupName":"Favorites","op":1}`)),
			},
			FeatureID: broker.FeatureRemoteWatchlistModify, Action: "modify", PortMode: "failure-then-success",
		},
	}
}

func stage9WatchlistsRemoteWriteRequest(
	t *testing.T,
	router http.Handler,
	method string,
	path string,
	body *string,
	contextMode string,
) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body == nil {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(*body)
	}
	requestContext := t.Context()
	var cancel context.CancelFunc
	switch contextMode {
	case "canceled":
		requestContext, cancel = context.WithCancel(requestContext)
		cancel()
	case "deadline":
		requestContext, cancel = context.WithDeadline(requestContext, time.Unix(1, 0))
		defer cancel()
	}
	request := httptest.NewRequestWithContext(requestContext, method, path, reader)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func stage9NormalizeWatchlistsRemoteWriteEnvelope(t *testing.T, name string, envelope map[string]any) {
	t.Helper()
	timestamp, ok := envelope["timestamp"].(string)
	if !ok {
		t.Fatalf("case %s response timestamp = %#v", name, envelope["timestamp"])
	}
	if _, err := time.Parse(time.RFC3339Nano, timestamp); err != nil {
		t.Fatalf("case %s response timestamp = %q: %v", name, timestamp, err)
	}
	envelope["timestamp"] = stage9WatchlistsRemoteWriteNow.Format(time.RFC3339Nano)
	if data, ok := envelope["data"].(map[string]any); ok {
		if provider, ok := data["provider"].(map[string]any); ok {
			stamp := stage9WatchlistsRemoteWriteNow.Format(time.RFC3339Nano)
			provider["resolvedAt"] = stamp
			provider["asOf"] = stamp
		}
	}
}

func stage9WatchlistsRemoteWriteJSONMap(value any) (map[string]any, error) {
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

func stage9WatchlistsRemoteWritePayloadState(action broker.CustomizationAction) string {
	if action.Payload == nil {
		return "nil"
	}
	if len(action.Payload) == 0 {
		return "empty_object"
	}
	return "object"
}

func stage9WatchlistsRemoteWriteBrokerForCase(
	testCase stage9WatchlistsRemoteWriteInput,
) (broker.Broker, *stage9WatchlistsRemoteWriteBroker) {
	if testCase.PortMode == "adapter-unavailable" {
		return &stage9WatchlistsRemoteWriteBareBroker{}, nil
	}
	state := broker.CapabilityAvailable
	if testCase.PortMode == "capability-unavailable" {
		state = broker.CapabilityUnavailable
	}
	adapter := &stage9WatchlistsRemoteWriteBroker{mode: testCase.PortMode, state: state}
	return adapter, adapter
}

type stage9WatchlistsRemoteWriteBroker struct {
	mode       string
	state      broker.CapabilityState
	applyCalls int
	actions    []broker.CustomizationAction
}

func (b *stage9WatchlistsRemoteWriteBroker) ID() string { return "futu" }

func (b *stage9WatchlistsRemoteWriteBroker) Descriptor() broker.Descriptor {
	return stage9WatchlistsRemoteWriteDescriptor(b.ID(), b.state)
}

func (*stage9WatchlistsRemoteWriteBroker) DiscoverAccounts(context.Context) ([]broker.Account, error) {
	return nil, nil
}

func (*stage9WatchlistsRemoteWriteBroker) Trading() broker.TradingService      { return nil }
func (*stage9WatchlistsRemoteWriteBroker) MarketData() broker.MarketDataReader { return nil }

func (*stage9WatchlistsRemoteWriteBroker) QueryCustomization(
	context.Context,
	broker.FeatureQuery,
) (*broker.FeatureResult, error) {
	return nil, nil
}

func (b *stage9WatchlistsRemoteWriteBroker) ApplyCustomization(
	ctx context.Context,
	action broker.CustomizationAction,
) (*broker.CustomizationResult, error) {
	b.applyCalls++
	b.actions = append(b.actions, action)
	switch b.mode {
	case "provider-http-403":
		return nil, &stage9WatchlistsRemoteWriteHTTPError{
			status: http.StatusForbidden, message: "provider denied remote watchlist write",
		}
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

type stage9WatchlistsRemoteWriteBareBroker struct{}

func (*stage9WatchlistsRemoteWriteBareBroker) ID() string { return "futu" }

func (*stage9WatchlistsRemoteWriteBareBroker) Descriptor() broker.Descriptor {
	return stage9WatchlistsRemoteWriteDescriptor("futu", broker.CapabilityAvailable)
}

func (*stage9WatchlistsRemoteWriteBareBroker) DiscoverAccounts(context.Context) ([]broker.Account, error) {
	return nil, nil
}

func (*stage9WatchlistsRemoteWriteBareBroker) Trading() broker.TradingService      { return nil }
func (*stage9WatchlistsRemoteWriteBareBroker) MarketData() broker.MarketDataReader { return nil }

func stage9WatchlistsRemoteWriteDescriptor(
	id string,
	state broker.CapabilityState,
) broker.Descriptor {
	return broker.Descriptor{
		ID:                id,
		DisplayName:       "Futu fixture",
		SecurityFirm:      "Futu/Moomoo via OpenD",
		CapabilityVersion: "stage9-watchlists-remote-write-fixture",
		Environments:      []string{"SIMULATE"},
		Capabilities: []broker.MarketCapability{{
			Market: "US",
			Features: []broker.FeatureCapability{{
				ID: broker.FeatureRemoteWatchlistModify, Markets: []string{"US"},
				Access: broker.FeatureAccessWrite, State: state,
			}},
		}},
	}
}

type stage9WatchlistsRemoteWriteHTTPError struct {
	status  int
	message string
}

func (e *stage9WatchlistsRemoteWriteHTTPError) Error() string   { return e.message }
func (e *stage9WatchlistsRemoteWriteHTTPError) HTTPStatus() int { return e.status }

var _ broker.Broker = (*stage9WatchlistsRemoteWriteBroker)(nil)
var _ broker.CustomizationService = (*stage9WatchlistsRemoteWriteBroker)(nil)
var _ broker.Broker = (*stage9WatchlistsRemoteWriteBareBroker)(nil)
