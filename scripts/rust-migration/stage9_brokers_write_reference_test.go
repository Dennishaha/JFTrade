package rustmigration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	tradingapi "github.com/jftrade/jftrade-main/internal/api/trading"
	srv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

const (
	stage9BrokersWriteFixtureVersion = "stage9.brokers-write.v1"
	stage9BrokersWriteTimestamp      = "2026-08-23T10:00:00Z"
)

type stage9BrokersWriteFixture struct {
	Version string                          `json:"version"`
	Cases   []stage9BrokersWriteFixtureCase `json:"cases"`
}

type stage9BrokersWriteFixtureCase struct {
	Name        string                             `json:"name"`
	PortMode    string                             `json:"portMode"`
	Requests    []stage9BrokersWriteFixtureRequest `json:"requests"`
	Expected    []stage9BrokersWriteExpected       `json:"expected"`
	GoCalls     []map[string]any                   `json:"goCalls,omitempty"`
	Observation stage9BrokersWriteObservation      `json:"observation"`
}

type stage9BrokersWriteFixtureRequest struct {
	Method      string  `json:"method"`
	RequestPath string  `json:"requestPath"`
	Body        *string `json:"body,omitempty"`
	Context     string  `json:"context,omitempty"`
}

type stage9BrokersWriteExpected struct {
	Status   int               `json:"status"`
	Headers  map[string]string `json:"headers"`
	PortCall bool              `json:"portCall"`
	Envelope map[string]any    `json:"envelope"`
}

type stage9BrokersWriteObservation struct {
	PlaceCalls  int `json:"placeCalls"`
	CancelCalls int `json:"cancelCalls"`
	UnlockCalls int `json:"unlockCalls"`
}

type stage9BrokersWriteCaseSpec struct {
	Name     string
	PortMode string
	Requests []stage9BrokersWriteFixtureRequest
}

// TestStage9BrokersWriteFixtureMatchesCurrentGoOwner freezes all three broker
// mutation routes through the real Gin handlers and trading service. The
// broker doubles record the command boundary without opening OpenD, SQLite,
// or any production trading session.
func TestStage9BrokersWriteFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 brokers-write fixture source")
	}
	fixturePath := filepath.Join(
		filepath.Dir(source), "../../tests/fixtures/rust-migration/stage9/brokers-write.json",
	)
	gin.SetMode(gin.TestMode)
	want := stage9BrokersWriteFixture{
		Version: stage9BrokersWriteFixtureVersion,
		Cases:   make([]stage9BrokersWriteFixtureCase, 0, len(stage9BrokersWriteCases())),
	}
	for _, spec := range stage9BrokersWriteCases() {
		router, trace := stage9BrokersWriteRouter(spec.PortMode)
		expected := make([]stage9BrokersWriteExpected, 0, len(spec.Requests))
		for _, request := range spec.Requests {
			response := stage9BrokersWriteRequest(t, router, request)
			var envelope map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("case %s decode response: %v", spec.Name, err)
			}
			stage9NormalizeBrokersWriteEnvelope(t, spec.Name, envelope)
			expected = append(expected, stage9BrokersWriteExpected{
				Status: response.Code, Headers: stage9BrokersWriteHeaders(response),
				PortCall: stage9BrokersWritePortCall(response.Code), Envelope: envelope,
			})
		}
		goCalls := trace.calls
		if len(goCalls) == 0 {
			goCalls = nil
		} else {
			goCalls = stage9CanonicalBrokersWriteCalls(t, goCalls)
		}
		want.Cases = append(want.Cases, stage9BrokersWriteFixtureCase{
			Name: spec.Name, PortMode: spec.PortMode, Requests: spec.Requests,
			Expected: expected, GoCalls: goCalls, Observation: trace.observation(),
		})
	}

	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode brokers-write fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write brokers-write fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read brokers-write fixture: %v", err)
	}
	var got stage9BrokersWriteFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode brokers-write fixture: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		for index := range want.Cases {
			if !reflect.DeepEqual(got.Cases[index], want.Cases[index]) {
				t.Fatalf("stage 9 brokers-write case %s drifted: got=%#v want=%#v", want.Cases[index].Name, got.Cases[index], want.Cases[index])
			}
		}
		t.Fatalf("stage 9 brokers-write fixture drifted from the Go owner")
	}
}

// The Rust adapter call represents dispatch into the Go service boundary. A
// validly bound mutation still crosses that boundary when broker resolution,
// risk, or the broker itself returns an error; only transport/body/query
// binding errors stop before the adapter.
func stage9BrokersWritePortCall(status int) bool {
	return status != http.StatusBadRequest
}

func stage9CanonicalBrokersWriteCalls(
	t *testing.T,
	calls []map[string]any,
) []map[string]any {
	t.Helper()
	canonical := make([]map[string]any, 0, len(calls))
	for _, call := range calls {
		encoded, err := json.Marshal(call)
		if err != nil {
			t.Fatalf("encode brokers-write call: %v", err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatalf("decode brokers-write call: %v", err)
		}
		canonical = append(canonical, decoded)
	}
	return canonical
}

func stage9BrokersWriteCases() []stage9BrokersWriteCaseSpec {
	body := func(value string) *string { return &value }
	post := func(path, value string) stage9BrokersWriteFixtureRequest {
		return stage9BrokersWriteFixtureRequest{Method: http.MethodPost, RequestPath: path, Body: body(value)}
	}
	postContext := func(path, value, requestContext string) stage9BrokersWriteFixtureRequest {
		request := post(path, value)
		request.Context = requestContext
		return request
	}
	postEmpty := func(path string) stage9BrokersWriteFixtureRequest {
		return stage9BrokersWriteFixtureRequest{Method: http.MethodPost, RequestPath: path}
	}
	del := func(path, value string) stage9BrokersWriteFixtureRequest {
		return stage9BrokersWriteFixtureRequest{Method: http.MethodDelete, RequestPath: path, Body: body(value)}
	}
	delContext := func(path, value, requestContext string) stage9BrokersWriteFixtureRequest {
		request := del(path, value)
		request.Context = requestContext
		return request
	}
	delEmpty := func(path string) stage9BrokersWriteFixtureRequest {
		return stage9BrokersWriteFixtureRequest{Method: http.MethodDelete, RequestPath: path}
	}
	placeBody := `{"symbol":"US.AAPL","side":"BUY","orderType":"LIMIT","price":100.25,"quantity":2,"timeInForce":"DAY","clientOrderId":"client-1","remark":"fixture","session":"RTH","fillOutsideRTH":false}`
	cancelBody := `{"orders":[{"orderId":7,"brokerOrderId":"broker-7","symbol":"US.AAPL"},{"orderId":8,"brokerOrderId":"broker-8","symbol":"US.MSFT"}]}`
	unlockBody := `{"unlock":true,"passwordMd5":"abc123"}`

	return []stage9BrokersWriteCaseSpec{
		{Name: "place-success-query-normalization", PortMode: "success", Requests: []stage9BrokersWriteFixtureRequest{
			post("/api/v1/brokers/futu/orders?tradingEnvironment=%20simulate%20&accountId=+ACC-1+&market=us", placeBody),
		}},
		{Name: "place-success-query-defaults", PortMode: "success", Requests: []stage9BrokersWriteFixtureRequest{
			post("/api/v1/brokers/futu/orders", `{"symbol":"AAPL","side":"BUY","orderType":"MARKET","quantity":1}`),
		}},
		{Name: "place-duplicate-query-first-values-win", PortMode: "success", Requests: []stage9BrokersWriteFixtureRequest{
			post("/api/v1/brokers/futu/orders?tradingEnvironment=&tradingEnvironment=REAL&accountId=&accountId=ACC-2&market=&market=US", `{"symbol":"AAPL","side":"BUY","orderType":"MARKET","quantity":1}`),
		}},
		{Name: "place-null-body-reaches-broker", PortMode: "success", Requests: []stage9BrokersWriteFixtureRequest{
			post("/api/v1/brokers/futu/orders", "null"),
		}},
		{Name: "place-trailing-json-first-value-wins", PortMode: "success", Requests: []stage9BrokersWriteFixtureRequest{
			post("/api/v1/brokers/futu/orders", placeBody+` {"symbol":"ignored"}`),
		}},
		{Name: "place-unknown-fields-ignored", PortMode: "success", Requests: []stage9BrokersWriteFixtureRequest{
			post("/api/v1/brokers/futu/orders", `{"symbol":"US.AAPL","side":"BUY","orderType":"LIMIT","quantity":1,"unknownField":true}`),
		}},
		{Name: "place-malformed-field-body-rejected", PortMode: "success", Requests: []stage9BrokersWriteFixtureRequest{
			post("/api/v1/brokers/futu/orders", `{"symbol":`),
		}},
		{Name: "place-string-quantity-rejected", PortMode: "success", Requests: []stage9BrokersWriteFixtureRequest{
			post("/api/v1/brokers/futu/orders", `{"quantity":"1"}`),
		}},
		{Name: "place-number-symbol-rejected", PortMode: "success", Requests: []stage9BrokersWriteFixtureRequest{
			post("/api/v1/brokers/futu/orders", `{"symbol":1}`),
		}},
		{Name: "place-scalar-body-rejected", PortMode: "success", Requests: []stage9BrokersWriteFixtureRequest{
			post("/api/v1/brokers/futu/orders", "1"),
		}},
		{Name: "place-empty-body-rejected", PortMode: "success", Requests: []stage9BrokersWriteFixtureRequest{
			postEmpty("/api/v1/brokers/futu/orders"),
		}},
		{Name: "place-malformed-body-rejected", PortMode: "success", Requests: []stage9BrokersWriteFixtureRequest{
			post("/api/v1/brokers/futu/orders", "{"),
		}},
		{Name: "place-array-body-rejected", PortMode: "success", Requests: []stage9BrokersWriteFixtureRequest{
			post("/api/v1/brokers/futu/orders", "[]"),
		}},
		{Name: "place-malformed-query-precedes-body", PortMode: "success", Requests: []stage9BrokersWriteFixtureRequest{
			post("/api/v1/brokers/futu/orders?market=%zz", "{"),
		}},
		{Name: "place-broker-not-found", PortMode: "missing-broker", Requests: []stage9BrokersWriteFixtureRequest{
			post("/api/v1/brokers/futu/orders", placeBody),
		}},
		{Name: "place-no-active-broker", PortMode: "no-broker", Requests: []stage9BrokersWriteFixtureRequest{
			post("/api/v1/brokers/futu/orders", placeBody),
		}},
		{Name: "place-trading-unsupported", PortMode: "no-trading", Requests: []stage9BrokersWriteFixtureRequest{
			post("/api/v1/brokers/futu/orders", placeBody),
		}},
		{Name: "place-pre-trade-risk-rejected", PortMode: "place-risk", Requests: []stage9BrokersWriteFixtureRequest{
			post("/api/v1/brokers/futu/orders", placeBody),
		}},
		{Name: "place-broker-failure", PortMode: "place-error", Requests: []stage9BrokersWriteFixtureRequest{
			post("/api/v1/brokers/futu/orders", placeBody),
		}},
		{Name: "place-broker-timeout-uses-generic-failure", PortMode: "place-timeout", Requests: []stage9BrokersWriteFixtureRequest{
			post("/api/v1/brokers/futu/orders", placeBody),
		}},
		{Name: "place-cancelled-uses-generic-failure", PortMode: "place-cancel", Requests: []stage9BrokersWriteFixtureRequest{
			postContext("/api/v1/brokers/futu/orders", placeBody, "canceled"),
		}},
		{Name: "place-deadline-uses-generic-failure", PortMode: "place-deadline", Requests: []stage9BrokersWriteFixtureRequest{
			postContext("/api/v1/brokers/futu/orders", placeBody, "deadline"),
		}},
		{Name: "place-repeat-submits-twice", PortMode: "place-repeat", Requests: []stage9BrokersWriteFixtureRequest{
			post("/api/v1/brokers/futu/orders", placeBody),
			post("/api/v1/brokers/futu/orders", placeBody),
		}},
		{Name: "place-nil-order-result-is-success", PortMode: "place-nil", Requests: []stage9BrokersWriteFixtureRequest{
			post("/api/v1/brokers/futu/orders", placeBody),
		}},

		{Name: "cancel-success-query-and-orders", PortMode: "success", Requests: []stage9BrokersWriteFixtureRequest{
			del("/api/v1/brokers/futu/orders?tradingEnvironment=REAL&accountId=ACC-1&market=US", cancelBody),
		}},
		{Name: "cancel-empty-object-is-zero-operation", PortMode: "success", Requests: []stage9BrokersWriteFixtureRequest{
			del("/api/v1/brokers/futu/orders", "{}"),
		}},
		{Name: "cancel-null-body-is-zero-operation", PortMode: "success", Requests: []stage9BrokersWriteFixtureRequest{
			del("/api/v1/brokers/futu/orders", "null"),
		}},
		{Name: "cancel-trailing-json-first-value-wins", PortMode: "success", Requests: []stage9BrokersWriteFixtureRequest{
			del("/api/v1/brokers/futu/orders", cancelBody+` {"orders":null}`),
		}},
		{Name: "cancel-unknown-fields-ignored", PortMode: "success", Requests: []stage9BrokersWriteFixtureRequest{
			del("/api/v1/brokers/futu/orders", `{"orders":[{"orderId":9,"brokerOrderId":"broker-9","symbol":"US.AAPL"}],"unknownField":true}`),
		}},
		{Name: "cancel-number-order-id-rejected", PortMode: "success", Requests: []stage9BrokersWriteFixtureRequest{
			del("/api/v1/brokers/futu/orders", `{"orders":[{"orderId":1.5}]}`),
		}},
		{Name: "cancel-number-broker-order-id-rejected", PortMode: "success", Requests: []stage9BrokersWriteFixtureRequest{
			del("/api/v1/brokers/futu/orders", `{"orders":[{"brokerOrderId":1}]}`),
		}},
		{Name: "cancel-scalar-body-rejected", PortMode: "success", Requests: []stage9BrokersWriteFixtureRequest{
			del("/api/v1/brokers/futu/orders", "1"),
		}},
		{Name: "cancel-empty-body-rejected", PortMode: "success", Requests: []stage9BrokersWriteFixtureRequest{
			delEmpty("/api/v1/brokers/futu/orders"),
		}},
		{Name: "cancel-malformed-body-rejected", PortMode: "success", Requests: []stage9BrokersWriteFixtureRequest{
			del("/api/v1/brokers/futu/orders", "{"),
		}},
		{Name: "cancel-array-body-rejected", PortMode: "success", Requests: []stage9BrokersWriteFixtureRequest{
			del("/api/v1/brokers/futu/orders", "[]"),
		}},
		{Name: "cancel-invalid-order-id-rejected", PortMode: "success", Requests: []stage9BrokersWriteFixtureRequest{
			del("/api/v1/brokers/futu/orders", `{"orders":[{"orderId":"bad"} ]}`),
		}},
		{Name: "cancel-malformed-query-precedes-body", PortMode: "success", Requests: []stage9BrokersWriteFixtureRequest{
			del("/api/v1/brokers/futu/orders?accountId=%zz", "{"),
		}},
		{Name: "cancel-broker-not-found", PortMode: "missing-broker", Requests: []stage9BrokersWriteFixtureRequest{
			del("/api/v1/brokers/futu/orders", cancelBody),
		}},
		{Name: "cancel-no-active-broker", PortMode: "no-broker", Requests: []stage9BrokersWriteFixtureRequest{
			del("/api/v1/brokers/futu/orders", cancelBody),
		}},
		{Name: "cancel-trading-unsupported", PortMode: "no-trading", Requests: []stage9BrokersWriteFixtureRequest{
			del("/api/v1/brokers/futu/orders", cancelBody),
		}},
		{Name: "cancel-broker-failure", PortMode: "cancel-error", Requests: []stage9BrokersWriteFixtureRequest{
			del("/api/v1/brokers/futu/orders", cancelBody),
		}},
		{Name: "cancel-broker-timeout-uses-generic-failure", PortMode: "cancel-timeout", Requests: []stage9BrokersWriteFixtureRequest{
			del("/api/v1/brokers/futu/orders", cancelBody),
		}},
		{Name: "cancel-cancelled-uses-generic-failure", PortMode: "cancel-cancel", Requests: []stage9BrokersWriteFixtureRequest{
			delContext("/api/v1/brokers/futu/orders", cancelBody, "canceled"),
		}},
		{Name: "cancel-deadline-uses-generic-failure", PortMode: "cancel-deadline", Requests: []stage9BrokersWriteFixtureRequest{
			delContext("/api/v1/brokers/futu/orders", cancelBody, "deadline"),
		}},
		{Name: "cancel-repeat-is-not-idempotent", PortMode: "cancel-repeat", Requests: []stage9BrokersWriteFixtureRequest{
			del("/api/v1/brokers/futu/orders", cancelBody),
			del("/api/v1/brokers/futu/orders", cancelBody),
		}},

		{Name: "unlock-success", PortMode: "success", Requests: []stage9BrokersWriteFixtureRequest{
			post("/api/v1/brokers/futu/unlock?tradingEnvironment=REAL&accountId=ACC-1&market=US", unlockBody),
		}},
		{Name: "unlock-false-is-still-submitted", PortMode: "success", Requests: []stage9BrokersWriteFixtureRequest{
			post("/api/v1/brokers/futu/unlock", `{"unlock":false}`),
		}},
		{Name: "unlock-null-body-reaches_broker", PortMode: "success", Requests: []stage9BrokersWriteFixtureRequest{
			post("/api/v1/brokers/futu/unlock", "null"),
		}},
		{Name: "unlock-trailing-json-first-value-wins", PortMode: "success", Requests: []stage9BrokersWriteFixtureRequest{
			post("/api/v1/brokers/futu/unlock", unlockBody+` {"unlock":false}`),
		}},
		{Name: "unlock-unknown-fields-ignored", PortMode: "success", Requests: []stage9BrokersWriteFixtureRequest{
			post("/api/v1/brokers/futu/unlock", `{"unlock":true,"passwordMd5":"abc","unknownField":true}`),
		}},
		{Name: "unlock-string-flag-rejected", PortMode: "success", Requests: []stage9BrokersWriteFixtureRequest{
			post("/api/v1/brokers/futu/unlock", `{"unlock":"true"}`),
		}},
		{Name: "unlock-number-password-rejected", PortMode: "success", Requests: []stage9BrokersWriteFixtureRequest{
			post("/api/v1/brokers/futu/unlock", `{"passwordMd5":1}`),
		}},
		{Name: "unlock-scalar-body-rejected", PortMode: "success", Requests: []stage9BrokersWriteFixtureRequest{
			post("/api/v1/brokers/futu/unlock", "true"),
		}},
		{Name: "unlock-empty-body-rejected", PortMode: "success", Requests: []stage9BrokersWriteFixtureRequest{
			postEmpty("/api/v1/brokers/futu/unlock"),
		}},
		{Name: "unlock-malformed-body-rejected", PortMode: "success", Requests: []stage9BrokersWriteFixtureRequest{
			post("/api/v1/brokers/futu/unlock", "{"),
		}},
		{Name: "unlock-array-body-rejected", PortMode: "success", Requests: []stage9BrokersWriteFixtureRequest{
			post("/api/v1/brokers/futu/unlock", "[]"),
		}},
		{Name: "unlock-malformed-query-precedes-body", PortMode: "success", Requests: []stage9BrokersWriteFixtureRequest{
			post("/api/v1/brokers/futu/unlock?market=%zz", "{"),
		}},
		{Name: "unlock-broker-not-found", PortMode: "missing-broker", Requests: []stage9BrokersWriteFixtureRequest{
			post("/api/v1/brokers/futu/unlock", unlockBody),
		}},
		{Name: "unlock-no-active-broker", PortMode: "no-broker", Requests: []stage9BrokersWriteFixtureRequest{
			post("/api/v1/brokers/futu/unlock", unlockBody),
		}},
		{Name: "unlock-unsupported", PortMode: "unlock-unsupported", Requests: []stage9BrokersWriteFixtureRequest{
			post("/api/v1/brokers/futu/unlock", unlockBody),
		}},
		{Name: "unlock-broker-failure", PortMode: "unlock-error", Requests: []stage9BrokersWriteFixtureRequest{
			post("/api/v1/brokers/futu/unlock", unlockBody),
		}},
		{Name: "unlock-broker-timeout-uses-generic-failure", PortMode: "unlock-timeout", Requests: []stage9BrokersWriteFixtureRequest{
			post("/api/v1/brokers/futu/unlock", unlockBody),
		}},
		{Name: "unlock-cancelled-uses-generic-failure", PortMode: "unlock-cancel", Requests: []stage9BrokersWriteFixtureRequest{
			postContext("/api/v1/brokers/futu/unlock", unlockBody, "canceled"),
		}},
		{Name: "unlock-deadline-uses-generic-failure", PortMode: "unlock-deadline", Requests: []stage9BrokersWriteFixtureRequest{
			postContext("/api/v1/brokers/futu/unlock", unlockBody, "deadline"),
		}},
		{Name: "unlock-repeat-is-not-idempotent", PortMode: "unlock-repeat", Requests: []stage9BrokersWriteFixtureRequest{
			post("/api/v1/brokers/futu/unlock", unlockBody),
			post("/api/v1/brokers/futu/unlock", unlockBody),
		}},
	}
}

type stage9BrokersWriteTrace struct {
	mode  string
	calls []map[string]any
}

func (trace *stage9BrokersWriteTrace) add(kind string, fields map[string]any) {
	call := map[string]any{"kind": kind}
	for key, value := range fields {
		call[key] = value
	}
	trace.calls = append(trace.calls, call)
}

func (trace *stage9BrokersWriteTrace) observation() stage9BrokersWriteObservation {
	var observation stage9BrokersWriteObservation
	for _, call := range trace.calls {
		switch call["kind"] {
		case "place":
			observation.PlaceCalls++
		case "cancel":
			observation.CancelCalls++
		case "unlock":
			observation.UnlockCalls++
		}
	}
	return observation
}

func stage9BrokersWriteRouter(mode string) (*gin.Engine, *stage9BrokersWriteTrace) {
	trace := &stage9BrokersWriteTrace{mode: mode, calls: make([]map[string]any, 0)}
	activeID := "futu"
	if mode == "missing-broker" {
		activeID = "ib"
	}
	trading := broker.TradingService(&stage9BrokersWriteTrading{mode: mode, trace: trace})
	if mode == "no-trading" {
		trading = nil
	}
	active := &stage9BrokersWriteBroker{id: activeID, trading: trading}
	var selected broker.Broker = active
	if mode == "no-broker" {
		selected = nil
	}
	serviceOptions := []srv.Option{
		srv.WithActiveBroker(func() broker.Broker { return selected }),
		srv.WithDefaultTradingEnvironment(func() string { return "SIMULATE" }),
	}
	if mode == "place-risk" {
		serviceOptions[1] = srv.WithDefaultTradingEnvironment(func() string { return "REAL" })
		serviceOptions = append(serviceOptions, srv.WithPreTradeRiskGateway(
			srv.NewStaticPreTradeRiskGateway(func() srv.PreTradeRiskConfig { return srv.PreTradeRiskConfig{} }),
		))
	}
	if selected != nil && mode != "unlock-unsupported" {
		selected = &stage9BrokersWriteUnlockBroker{stage9BrokersWriteBroker: active, mode: mode, trace: trace}
		serviceOptions[0] = srv.WithActiveBroker(func() broker.Broker { return selected })
	}
	service := srv.NewService(serviceOptions...)
	router := gin.New()
	tradingapi.RegisterRoutes(router.Group("/api/v1"), service)
	return router, trace
}

type stage9BrokersWriteBroker struct {
	id      string
	trading broker.TradingService
}

func (adapter *stage9BrokersWriteBroker) ID() string { return adapter.id }
func (adapter *stage9BrokersWriteBroker) Descriptor() broker.Descriptor {
	return broker.Descriptor{ID: adapter.id}
}
func (*stage9BrokersWriteBroker) DiscoverAccounts(context.Context) ([]broker.Account, error) {
	return nil, nil
}
func (adapter *stage9BrokersWriteBroker) Trading() broker.TradingService {
	return adapter.trading
}
func (*stage9BrokersWriteBroker) MarketData() broker.MarketDataReader { return nil }

type stage9BrokersWriteUnlockBroker struct {
	*stage9BrokersWriteBroker
	mode  string
	trace *stage9BrokersWriteTrace
}

func (adapter *stage9BrokersWriteUnlockBroker) UnlockTrade(ctx context.Context, request broker.UnlockTradeRequest) error {
	adapter.trace.add("unlock", map[string]any{"query": request.ReadQuery, "request": request})
	switch adapter.mode {
	case "unlock-error":
		return errors.New("broker unlock failed")
	case "unlock-timeout":
		return broker.NewBrokerError(adapter.id, broker.ErrCodeTimeout, "unlock timed out")
	case "unlock-cancel", "unlock-deadline":
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return nil
}

type stage9BrokersWriteTrading struct {
	mode        string
	trace       *stage9BrokersWriteTrace
	placeCount  int
	cancelCount int
}

func (trading *stage9BrokersWriteTrading) PlaceOrder(ctx context.Context, query broker.PlaceOrderQuery) (*broker.PlaceOrderResult, error) {
	trading.placeCount++
	trading.trace.add("place", map[string]any{"query": query})
	switch trading.mode {
	case "place-error":
		return nil, errors.New("broker place failed")
	case "place-timeout":
		return nil, broker.NewBrokerError("futu", broker.ErrCodeTimeout, "order timed out")
	case "place-cancel", "place-deadline":
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	case "place-nil":
		return nil, nil
	}
	orderID := trading.placeCount
	return &broker.PlaceOrderResult{
		AccountID: query.AccountID, TradingEnvironment: query.TradingEnvironment, Market: query.Market,
		BrokerOrderID:   fmt.Sprintf("broker-order-%d", orderID),
		BrokerOrderIDEx: new(fmt.Sprintf("broker-order-%d-ex", orderID)), Status: "SUBMITTED",
	}, nil
}

func (trading *stage9BrokersWriteTrading) CancelOrders(ctx context.Context, query broker.ReadQuery, orders ...broker.CancelOrder) error {
	trading.cancelCount++
	trading.trace.add("cancel", map[string]any{"query": query, "orders": orders})
	switch trading.mode {
	case "cancel-error":
		return errors.New("broker cancel failed")
	case "cancel-timeout":
		return broker.NewBrokerError("futu", broker.ErrCodeTimeout, "cancel timed out")
	case "cancel-cancel", "cancel-deadline":
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return nil
}

func stage9BrokersWriteRequest(
	t *testing.T,
	router http.Handler,
	fixtureRequest stage9BrokersWriteFixtureRequest,
) *httptest.ResponseRecorder {
	t.Helper()
	requestContext := context.Background()
	var cancel context.CancelFunc
	switch fixtureRequest.Context {
	case "canceled":
		requestContext, cancel = context.WithCancel(requestContext)
		cancel()
	case "deadline":
		requestContext, cancel = context.WithDeadline(requestContext, time.Unix(1, 0))
		defer cancel()
	}
	var body io.Reader
	if fixtureRequest.Body != nil {
		body = strings.NewReader(*fixtureRequest.Body)
	}
	request := httptest.NewRequestWithContext(
		requestContext, fixtureRequest.Method, fixtureRequest.RequestPath, body,
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func stage9BrokersWriteHeaders(response *httptest.ResponseRecorder) map[string]string {
	return map[string]string{"Content-Type": response.Header().Get("Content-Type")}
}

func stage9NormalizeBrokersWriteEnvelope(t *testing.T, name string, envelope map[string]any) {
	t.Helper()
	rawTimestamp, ok := envelope["timestamp"].(string)
	if !ok {
		t.Fatalf("case %s response has no timestamp", name)
	}
	if _, err := time.Parse(time.RFC3339Nano, rawTimestamp); err != nil {
		t.Fatalf("case %s timestamp %q is not RFC3339Nano: %v", name, rawTimestamp, err)
	}
	envelope["timestamp"] = stage9BrokersWriteTimestamp
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		return
	}
	for _, field := range []string{"placedAt", "cancelledAt", "unlockedAt"} {
		if value, exists := data[field]; exists {
			if _, ok := value.(string); ok {
				data[field] = stage9BrokersWriteTimestamp
			}
		}
	}
}

var _ broker.Broker = (*stage9BrokersWriteBroker)(nil)
var _ broker.TradingService = (*stage9BrokersWriteTrading)(nil)
var _ broker.UnlockTrader = (*stage9BrokersWriteUnlockBroker)(nil)
