package rustmigration

import (
	"context"
	"encoding/json"
	"errors"
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
	trading "github.com/jftrade/jftrade-main/internal/api/trading"
	srv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

const (
	stage9ExecutionWriteFixtureVersion = "stage9.execution-write.v1"
	stage9ExecutionWriteTimestamp      = "2026-08-23T10:00:00Z"
)

type stage9ExecutionWriteFixture struct {
	Version string                            `json:"version"`
	Cases   []stage9ExecutionWriteFixtureCase `json:"cases"`
}

type stage9ExecutionWriteFixtureCase struct {
	Name        string                               `json:"name"`
	PortMode    string                               `json:"portMode"`
	Requests    []stage9ExecutionWriteFixtureRequest `json:"requests"`
	Expected    []stage9ExecutionWriteExpected       `json:"expected"`
	GoCalls     []map[string]any                     `json:"goCalls,omitempty"`
	Observation stage9ExecutionWriteObservation      `json:"observation"`
}

type stage9ExecutionWriteFixtureRequest struct {
	Method      string  `json:"method"`
	RequestPath string  `json:"requestPath"`
	Body        *string `json:"body,omitempty"`
	Context     string  `json:"context,omitempty"`
}

type stage9ExecutionWriteExpected struct {
	Status   int               `json:"status"`
	Headers  map[string]string `json:"headers"`
	PortCall bool              `json:"portCall"`
	Envelope map[string]any    `json:"envelope"`
}

type stage9ExecutionWriteObservation struct {
	BuyingPowerCalls  int `json:"buyingPowerCalls"`
	ComboPreviewCalls int `json:"comboPreviewCalls"`
	ComboPlaceCalls   int `json:"comboPlaceCalls"`
	ComboCancelCalls  int `json:"comboCancelCalls"`
	OrderPreviewCalls int `json:"orderPreviewCalls"`
	OrderPlaceCalls   int `json:"orderPlaceCalls"`
	OrderCancelCalls  int `json:"orderCancelCalls"`
}

// TestStage9ExecutionWriteFixtureMatchesCurrentGoOwner freezes all seven
// execution mutation routes through the real Gin handlers and the existing
// execution service. The broker/gateway doubles record the Go side-effect
// boundary without opening SQLite, connecting OpenD, or submitting an order.
func TestStage9ExecutionWriteFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 execution-write fixture source")
	}
	fixturePath := filepath.Join(
		filepath.Dir(source), "../../tests/fixtures/rust-migration/stage9/execution-write.json",
	)
	want := stage9ExecutionWriteFixture{
		Version: stage9ExecutionWriteFixtureVersion,
		Cases:   make([]stage9ExecutionWriteFixtureCase, 0, len(stage9ExecutionWriteCases())),
	}
	for _, spec := range stage9ExecutionWriteCases() {
		router, trace := stage9ExecutionWriteRouter(spec.PortMode)
		expected := make([]stage9ExecutionWriteExpected, 0, len(spec.Requests))
		for _, request := range spec.Requests {
			response := stage9ExecutionWriteRequest(t, router, request)
			var envelope map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("case %s decode response: %v", spec.Name, err)
			}
			stage9NormalizeExecutionWriteEnvelope(t, spec.Name, envelope)
			expected = append(expected, stage9ExecutionWriteExpected{
				Status: response.Code, Headers: stage9ExecutionWriteHeaders(response),
				PortCall: stage9ExecutionWritePortCall(request), Envelope: envelope,
			})
		}
		want.Cases = append(want.Cases, stage9ExecutionWriteFixtureCase{
			Name: spec.Name, PortMode: spec.PortMode, Requests: spec.Requests,
			Expected: expected, GoCalls: trace.calls,
			Observation: trace.observation(),
		})
	}

	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode execution-write fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write execution-write fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read execution-write fixture: %v", err)
	}
	var got stage9ExecutionWriteFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode execution-write fixture: %v", err)
	}
	canonicalWantBytes, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("canonicalize execution-write fixture: %v", err)
	}
	var canonicalWant stage9ExecutionWriteFixture
	if err := json.Unmarshal(canonicalWantBytes, &canonicalWant); err != nil {
		t.Fatalf("decode canonical execution-write fixture: %v", err)
	}
	if !reflect.DeepEqual(got, canonicalWant) {
		t.Fatalf("stage 9 execution-write fixture drifted from the Go owner")
	}
}

type stage9ExecutionWriteCaseSpec struct {
	Name     string
	PortMode string
	Requests []stage9ExecutionWriteFixtureRequest
}

func stage9ExecutionWriteCases() []stage9ExecutionWriteCaseSpec {
	body := func(value string) *string { return &value }
	post := func(path, value string) stage9ExecutionWriteFixtureRequest {
		return stage9ExecutionWriteFixtureRequest{Method: http.MethodPost, RequestPath: path, Body: body(value)}
	}
	postContext := func(path, value, requestContext string) stage9ExecutionWriteFixtureRequest {
		request := post(path, value)
		request.Context = requestContext
		return request
	}
	postEmpty := func(path string) stage9ExecutionWriteFixtureRequest {
		return stage9ExecutionWriteFixtureRequest{Method: http.MethodPost, RequestPath: path}
	}
	cancel := func(path string) stage9ExecutionWriteFixtureRequest {
		return stage9ExecutionWriteFixtureRequest{Method: http.MethodPost, RequestPath: path}
	}
	withTrailing := func(value string) string { return value + ` {"ignored":true}` }

	return []stage9ExecutionWriteCaseSpec{
		{Name: "buying-power-success", PortMode: "success", Requests: []stage9ExecutionWriteFixtureRequest{
			post("/api/v1/execution/buying-power", stage9ExecutionWriteBuyingPowerBody()),
		}},
		{Name: "buying-power-trailing-json-first-value-wins", PortMode: "success", Requests: []stage9ExecutionWriteFixtureRequest{
			post("/api/v1/execution/buying-power", withTrailing(stage9ExecutionWriteBuyingPowerBody())),
		}},
		{Name: "buying-power-null-body", PortMode: "success", Requests: []stage9ExecutionWriteFixtureRequest{
			post("/api/v1/execution/buying-power", "null"),
		}},
		{Name: "buying-power-empty-body", PortMode: "success", Requests: []stage9ExecutionWriteFixtureRequest{
			postEmpty("/api/v1/execution/buying-power"),
		}},
		{Name: "buying-power-malformed-body", PortMode: "success", Requests: []stage9ExecutionWriteFixtureRequest{
			post("/api/v1/execution/buying-power", "{"),
		}},
		{Name: "buying-power-timeout", PortMode: "buying-power-timeout", Requests: []stage9ExecutionWriteFixtureRequest{
			post("/api/v1/execution/buying-power", stage9ExecutionWriteBuyingPowerBody()),
		}},
		{Name: "buying-power-missing-broker", PortMode: "success", Requests: []stage9ExecutionWriteFixtureRequest{
			post("/api/v1/execution/buying-power", strings.Replace(stage9ExecutionWriteBuyingPowerBody(), `"brokerId":"FUTU"`, `"brokerId":"missing"`, 1)),
		}},
		{Name: "combo-preview-success", PortMode: "success", Requests: []stage9ExecutionWriteFixtureRequest{
			post("/api/v1/execution/combos/previews", stage9ExecutionWriteComboBody()),
		}},
		{Name: "combo-preview-trailing-json-first-value-wins", PortMode: "success", Requests: []stage9ExecutionWriteFixtureRequest{
			post("/api/v1/execution/combos/previews", withTrailing(stage9ExecutionWriteComboBody())),
		}},
		{Name: "combo-preview-repeated-request-replays", PortMode: "success", Requests: []stage9ExecutionWriteFixtureRequest{
			post("/api/v1/execution/combos/previews", stage9ExecutionWriteComboBody()),
			post("/api/v1/execution/combos/previews", stage9ExecutionWriteComboBody()),
		}},
		{Name: "combo-preview-null-body", PortMode: "success", Requests: []stage9ExecutionWriteFixtureRequest{
			post("/api/v1/execution/combos/previews", "null"),
		}},
		{Name: "combo-preview-empty-body", PortMode: "success", Requests: []stage9ExecutionWriteFixtureRequest{
			postEmpty("/api/v1/execution/combos/previews"),
		}},
		{Name: "combo-preview-malformed-body", PortMode: "success", Requests: []stage9ExecutionWriteFixtureRequest{
			post("/api/v1/execution/combos/previews", "{"),
		}},
		{Name: "combo-preview-mixed-legs", PortMode: "success", Requests: []stage9ExecutionWriteFixtureRequest{
			post("/api/v1/execution/combos/previews", strings.Replace(stage9ExecutionWriteComboBody(), `"instrumentId":"US.OPTION.TWO","side":"SELL"`, `"instrumentId":"US.EVENT.TWO","side":"SELL","productClass":"event_contract"`, 1)),
		}},
		{Name: "combo-preview-rate-limited", PortMode: "combo-preview-rate", Requests: []stage9ExecutionWriteFixtureRequest{
			post("/api/v1/execution/combos/previews", stage9ExecutionWriteComboBody()),
		}},
		{Name: "combo-preview-cancelled", PortMode: "combo-preview-cancel", Requests: []stage9ExecutionWriteFixtureRequest{
			postContext("/api/v1/execution/combos/previews", stage9ExecutionWriteComboBody(), "canceled"),
		}},
		{Name: "combo-preview-event-parlay-price-is-rejected", PortMode: "success", Requests: []stage9ExecutionWriteFixtureRequest{
			post("/api/v1/execution/combos/previews", stage9ExecutionWriteEventParlayBody(true)),
		}},
		{Name: "combo-preview-event-parlay-success", PortMode: "success", Requests: []stage9ExecutionWriteFixtureRequest{
			post("/api/v1/execution/combos/previews", stage9ExecutionWriteEventParlayBody(false)),
		}},
		{Name: "combo-place-success", PortMode: "success", Requests: []stage9ExecutionWriteFixtureRequest{
			post("/api/v1/execution/combos", stage9ExecutionWriteComboPlaceBody()),
		}},
		{Name: "combo-place-trailing-json-first-value-wins", PortMode: "success", Requests: []stage9ExecutionWriteFixtureRequest{
			post("/api/v1/execution/combos", withTrailing(stage9ExecutionWriteComboPlaceBody())),
		}},
		{Name: "combo-place-null-body", PortMode: "success", Requests: []stage9ExecutionWriteFixtureRequest{
			post("/api/v1/execution/combos", "null"),
		}},
		{Name: "combo-place-empty-body", PortMode: "success", Requests: []stage9ExecutionWriteFixtureRequest{
			postEmpty("/api/v1/execution/combos"),
		}},
		{Name: "combo-place-missing-preview", PortMode: "success", Requests: []stage9ExecutionWriteFixtureRequest{
			post("/api/v1/execution/combos", strings.Replace(stage9ExecutionWriteComboPlaceBody(), `"previewId":"preview-fixed",`, "", 1)),
		}},
		{Name: "combo-place-broker-timeout", PortMode: "combo-place-timeout", Requests: []stage9ExecutionWriteFixtureRequest{
			post("/api/v1/execution/combos", stage9ExecutionWriteComboPlaceBody()),
		}},
		{Name: "combo-place-repeated-write-submits-twice", PortMode: "combo-place-repeat", Requests: []stage9ExecutionWriteFixtureRequest{
			post("/api/v1/execution/combos", stage9ExecutionWriteComboPlaceBody()),
			post("/api/v1/execution/combos", stage9ExecutionWriteComboPlaceBody()),
		}},
		{Name: "combo-place-cancelled", PortMode: "combo-place-cancel", Requests: []stage9ExecutionWriteFixtureRequest{
			postContext("/api/v1/execution/combos", stage9ExecutionWriteComboPlaceBody(), "canceled"),
		}},
		{Name: "combo-place-event-parlay-success", PortMode: "success", Requests: []stage9ExecutionWriteFixtureRequest{
			post("/api/v1/execution/combos", stage9ExecutionWriteEventParlayPlaceBody()),
		}},
		{Name: "combo-cancel-success-trims-id", PortMode: "success", Requests: []stage9ExecutionWriteFixtureRequest{
			cancel("/api/v1/execution/combos/%20combo-1%20/cancel"),
		}},
		{Name: "combo-cancel-blank-id-reaches-service", PortMode: "success", Requests: []stage9ExecutionWriteFixtureRequest{
			cancel("/api/v1/execution/combos/%20/cancel"),
		}},
		{Name: "combo-cancel-not-found", PortMode: "combo-cancel-not-found", Requests: []stage9ExecutionWriteFixtureRequest{
			cancel("/api/v1/execution/combos/unknown/cancel"),
		}},
		{Name: "combo-cancel-timeout", PortMode: "combo-cancel-timeout", Requests: []stage9ExecutionWriteFixtureRequest{
			cancel("/api/v1/execution/combos/combo-1/cancel"),
		}},
		{Name: "combo-cancel-cancelled", PortMode: "combo-cancel-cancel", Requests: []stage9ExecutionWriteFixtureRequest{
			postContext("/api/v1/execution/combos/combo-1/cancel", "", "canceled"),
		}},
		{Name: "combo-cancel-repeated-write-is-not-idempotent", PortMode: "combo-cancel-repeat", Requests: []stage9ExecutionWriteFixtureRequest{
			cancel("/api/v1/execution/combos/combo-1/cancel"),
			cancel("/api/v1/execution/combos/combo-1/cancel"),
		}},
		{Name: "order-preview-success", PortMode: "success", Requests: []stage9ExecutionWriteFixtureRequest{
			post("/api/v1/execution/previews", stage9ExecutionWriteOrderBody()),
		}},
		{Name: "order-preview-trailing-json-first-value-wins", PortMode: "success", Requests: []stage9ExecutionWriteFixtureRequest{
			post("/api/v1/execution/previews", withTrailing(stage9ExecutionWriteOrderBody())),
		}},
		{Name: "order-preview-null-body", PortMode: "success", Requests: []stage9ExecutionWriteFixtureRequest{
			post("/api/v1/execution/previews", "null"),
		}},
		{Name: "order-preview-empty-body", PortMode: "success", Requests: []stage9ExecutionWriteFixtureRequest{
			postEmpty("/api/v1/execution/previews"),
		}},
		{Name: "order-preview-malformed-body", PortMode: "success", Requests: []stage9ExecutionWriteFixtureRequest{
			post("/api/v1/execution/previews", "{"),
		}},
		{Name: "order-preview-option-requires-client-id", PortMode: "success", Requests: []stage9ExecutionWriteFixtureRequest{
			post("/api/v1/execution/previews", stage9ExecutionWriteOptionPreviewBody()),
		}},
		{Name: "order-preview-option-provider-timeout", PortMode: "order-preview-timeout", Requests: []stage9ExecutionWriteFixtureRequest{
			post("/api/v1/execution/previews", strings.Replace(stage9ExecutionWriteOptionPreviewBody(), `"price":2.25`, `"price":2.25,"clientOrderId":"option-client"`, 1)),
		}},
		{Name: "order-place-success-env-precedence", PortMode: "success", Requests: []stage9ExecutionWriteFixtureRequest{
			post("/api/v1/execution/orders", stage9ExecutionWriteOrderBodyWithEnvironment()),
		}},
		{Name: "order-place-trailing-json-first-value-wins", PortMode: "success", Requests: []stage9ExecutionWriteFixtureRequest{
			post("/api/v1/execution/orders", withTrailing(stage9ExecutionWriteOrderBody())),
		}},
		{Name: "order-place-null-body", PortMode: "success", Requests: []stage9ExecutionWriteFixtureRequest{
			post("/api/v1/execution/orders", "null"),
		}},
		{Name: "order-place-empty-body", PortMode: "success", Requests: []stage9ExecutionWriteFixtureRequest{
			postEmpty("/api/v1/execution/orders"),
		}},
		{Name: "order-place-malformed-body", PortMode: "success", Requests: []stage9ExecutionWriteFixtureRequest{
			post("/api/v1/execution/orders", "{"),
		}},
		{Name: "order-place-missing-broker", PortMode: "success", Requests: []stage9ExecutionWriteFixtureRequest{
			post("/api/v1/execution/orders", strings.Replace(stage9ExecutionWriteOrderBody(), `"brokerId":"futu",`, `"brokerId":"missing",`, 1)),
		}},
		{Name: "order-place-broker-timeout", PortMode: "order-place-timeout", Requests: []stage9ExecutionWriteFixtureRequest{
			post("/api/v1/execution/orders", stage9ExecutionWriteOrderBody()),
		}},
		{Name: "order-place-risk-rejected", PortMode: "order-place-risk", Requests: []stage9ExecutionWriteFixtureRequest{
			post("/api/v1/execution/orders", strings.Replace(stage9ExecutionWriteOrderBody(), `"tradingEnvironment":"SIMULATE"`, `"tradingEnvironment":"REAL"`, 1)),
		}},
		{Name: "order-place-cancelled", PortMode: "order-place-cancel", Requests: []stage9ExecutionWriteFixtureRequest{
			postContext("/api/v1/execution/orders", stage9ExecutionWriteOrderBody(), "canceled"),
		}},
		{Name: "order-place-repeated-write-submits-twice", PortMode: "order-place-repeat", Requests: []stage9ExecutionWriteFixtureRequest{
			post("/api/v1/execution/orders", stage9ExecutionWriteOrderBody()),
			post("/api/v1/execution/orders", stage9ExecutionWriteOrderBody()),
		}},
		{Name: "order-cancel-success-trims-id", PortMode: "success", Requests: []stage9ExecutionWriteFixtureRequest{
			cancel("/api/v1/execution/orders/%20order-1%20/cancel"),
		}},
		{Name: "order-cancel-blank-id-reaches-service", PortMode: "success", Requests: []stage9ExecutionWriteFixtureRequest{
			cancel("/api/v1/execution/orders/%20/cancel"),
		}},
		{Name: "order-cancel-invalid-percent-is-bound-as-literal", PortMode: "success", Requests: []stage9ExecutionWriteFixtureRequest{
			cancel("/api/v1/execution/orders/%zz/cancel"),
		}},
		{Name: "order-cancel-not-found", PortMode: "order-cancel-not-found", Requests: []stage9ExecutionWriteFixtureRequest{
			cancel("/api/v1/execution/orders/unknown/cancel"),
		}},
		{Name: "order-cancel-timeout", PortMode: "order-cancel-timeout", Requests: []stage9ExecutionWriteFixtureRequest{
			cancel("/api/v1/execution/orders/order-1/cancel"),
		}},
		{Name: "order-cancel-cancelled", PortMode: "order-cancel-cancel", Requests: []stage9ExecutionWriteFixtureRequest{
			postContext("/api/v1/execution/orders/order-1/cancel", "", "canceled"),
		}},
		{Name: "order-cancel-repeated-write-is-not-idempotent", PortMode: "order-cancel-repeat", Requests: []stage9ExecutionWriteFixtureRequest{
			cancel("/api/v1/execution/orders/order-1/cancel"),
			cancel("/api/v1/execution/orders/order-1/cancel"),
		}},
	}
}

func stage9ExecutionWriteBuyingPowerBody() string {
	return `{"brokerId":"FUTU","accountId":" acct-1 ","tradingEnvironment":"simulate","market":"us","featureId":"ignored","instrument":{"instrumentId":"US.AAPL","productClass":"option"},"orderKind":"single","orderType":"LIMIT","quantity":2,"price":100}`
}

func stage9ExecutionWriteComboBody() string {
	return `{"brokerId":"FUTU","accountId":" acct-1 ","market":"us","clientOrderId":"combo-client","orderKind":"option_combo","underlyingInstrumentId":"us.aapl","optionStrategy":"vertical","nearExpiry":"2026-07-17","spread":10,"legs":[{"instrumentId":" us.option.one ","side":" buy ","ratio":1},{"instrumentId":"US.OPTION.TWO","side":"SELL","ratio":1}]}`
}

func stage9ExecutionWriteComboPlaceBody() string {
	return strings.Replace(stage9ExecutionWriteComboBody(), `"clientOrderId":"combo-client"`, `"clientOrderId":"combo-client","previewId":"preview-fixed"`, 1)
}

func stage9ExecutionWriteEventParlayBody(includePrice bool) string {
	price := ""
	if includePrice {
		price = `,"price":0.42`
	}
	return `{"brokerId":"futu","accountId":"acct-1","market":"us","clientOrderId":"parlay-client","orderKind":"event_parlay","productClass":"event_contract","rfqId":"rfq-1","mvc":"mvc-1","amount":25,"quoteExpiresAt":"2030-01-01T00:00:00Z"` + price + `,"legs":[{"instrumentId":"US.EVENT.ONE","side":"buy","ratio":1,"predictionSide":"yes"},{"instrumentId":"US.EVENT.TWO","side":"buy","ratio":1,"predictionSide":"no"}]}`
}

func stage9ExecutionWriteEventParlayPlaceBody() string {
	return strings.Replace(stage9ExecutionWriteEventParlayBody(false), `"clientOrderId":"parlay-client"`, `"clientOrderId":"parlay-client","previewId":"preview-fixed"`, 1)
}

func stage9ExecutionWriteOrderBody() string {
	return `{"brokerId":"futu","accountId":" acct-1 ","tradingEnvironment":"SIMULATE","env":"REAL","market":"us","symbol":"AAPL","side":" buy ","orderType":"limit","timeInForce":"day","quantity":2,"price":100.5,"clientOrderId":"order-client","remark":" fixture order "}`
}

func stage9ExecutionWriteOrderBodyWithEnvironment() string {
	return strings.Replace(stage9ExecutionWriteOrderBody(), `"tradingEnvironment":"SIMULATE","env":"REAL"`, `"tradingEnvironment":"SIMULATE","env":"REAL"`, 1)
}

func stage9ExecutionWriteOptionPreviewBody() string {
	return `{"brokerId":"futu","accountId":"acct-1","market":"US","symbol":"AAPL260717C00200000","productClass":"option","side":"BUY","orderType":"LIMIT","quantity":1,"price":2.25}`
}

type stage9ExecutionWriteTrace struct {
	mode  string
	calls []map[string]any
}

func (trace *stage9ExecutionWriteTrace) add(call map[string]any) {
	trace.calls = append(trace.calls, call)
}

func (trace *stage9ExecutionWriteTrace) observation() stage9ExecutionWriteObservation {
	var observation stage9ExecutionWriteObservation
	for _, call := range trace.calls {
		switch call["kind"] {
		case "buying-power":
			observation.BuyingPowerCalls++
		case "combo-preview":
			observation.ComboPreviewCalls++
		case "combo-place":
			observation.ComboPlaceCalls++
		case "combo-cancel":
			observation.ComboCancelCalls++
		case "order-preview":
			observation.OrderPreviewCalls++
		case "order-place":
			observation.OrderPlaceCalls++
		case "order-cancel":
			observation.OrderCancelCalls++
		}
	}
	return observation
}

func stage9ExecutionWriteRouter(mode string) (*gin.Engine, *stage9ExecutionWriteTrace) {
	trace := &stage9ExecutionWriteTrace{mode: mode}
	selected := &stage9ExecutionWriteBroker{id: "futu", mode: mode, trace: trace}
	placeCount := 0
	cancelCount := 0
	comboPlaceCount := 0
	comboCancelCount := 0
	serviceOptions := []srv.Option{
		srv.WithActiveBroker(func() broker.Broker { return selected }),
		srv.WithDefaultTradingEnvironment(func() string { return "SIMULATE" }),
		srv.WithPlaceOrder(func(ctx context.Context, command srv.ExecutionOrderCommand) (srv.ExecutionOrder, error) {
			trace.add(map[string]any{"kind": "order-place", "command": stage9ExecutionWriteCommandJSON(command)})
			placeCount++
			switch mode {
			case "order-place-timeout":
				return srv.ExecutionOrder{}, broker.NewBrokerError("futu", broker.ErrCodeTimeout, "order timed out")
			case "order-place-cancel":
				if ctx.Err() != nil {
					return srv.ExecutionOrder{}, ctx.Err()
				}
			case "order-place-repeat":
				return stage9ExecutionWriteOrder("order-"+string(rune('1'+placeCount-1)), "SUBMITTED"), nil
			}
			return stage9ExecutionWriteOrder("order-1", "SUBMITTED"), nil
		}),
		srv.WithCancelOrder(func(ctx context.Context, id string) (srv.ExecutionOrder, error) {
			trace.add(map[string]any{"kind": "order-cancel", "internalOrderId": id})
			cancelCount++
			switch mode {
			case "order-cancel-timeout":
				return srv.ExecutionOrder{}, broker.NewBrokerError("futu", broker.ErrCodeTimeout, "cancel timed out")
			case "order-cancel-cancel":
				if ctx.Err() != nil {
					return srv.ExecutionOrder{}, ctx.Err()
				}
			case "order-cancel-not-found":
				return srv.ExecutionOrder{}, errors.New("execution order not found")
			case "order-cancel-repeat":
				if cancelCount > 1 {
					return srv.ExecutionOrder{}, errors.New("execution order is already terminal")
				}
			}
			return stage9ExecutionWriteOrder(id, "CANCEL_SUBMITTED"), nil
		}),
		srv.WithComboOrderGateway(&stage9ExecutionWriteComboGateway{
			trace: trace,
			place: func(ctx context.Context, intent broker.ComboOrderIntent) (srv.ExecutionOrder, error) {
				trace.add(map[string]any{"kind": "combo-place", "intent": intent})
				comboPlaceCount++
				switch mode {
				case "combo-place-timeout":
					return srv.ExecutionOrder{}, broker.NewBrokerError("futu", broker.ErrCodeTimeout, "combo timed out")
				case "combo-place-cancel":
					if ctx.Err() != nil {
						return srv.ExecutionOrder{}, ctx.Err()
					}
				case "combo-place-repeat":
					return stage9ExecutionWriteOrder("combo-order-"+string(rune('1'+comboPlaceCount-1)), "SUBMITTED"), nil
				}
				return stage9ExecutionWriteOrder("combo-order-1", "SUBMITTED"), nil
			},
			cancel: func(ctx context.Context, id string) (srv.ExecutionOrder, error) {
				trace.add(map[string]any{"kind": "combo-cancel", "internalOrderId": id})
				comboCancelCount++
				switch mode {
				case "combo-cancel-timeout":
					return srv.ExecutionOrder{}, broker.NewBrokerError("futu", broker.ErrCodeTimeout, "combo cancel timed out")
				case "combo-cancel-cancel":
					if ctx.Err() != nil {
						return srv.ExecutionOrder{}, ctx.Err()
					}
				case "combo-cancel-not-found":
					return srv.ExecutionOrder{}, errors.New("execution order not found")
				case "combo-cancel-repeat":
					if comboCancelCount > 1 {
						return srv.ExecutionOrder{}, errors.New("execution combo is already terminal")
					}
				}
				return stage9ExecutionWriteOrder(id, "CANCEL_SUBMITTED"), nil
			},
		}),
	}
	if mode == "order-place-risk" {
		serviceOptions = append(serviceOptions, srv.WithPreTradeRiskGateway(
			srv.NewStaticPreTradeRiskGateway(func() srv.PreTradeRiskConfig { return srv.PreTradeRiskConfig{} }),
		))
	}
	service := srv.NewService(serviceOptions...)
	router := gin.New()
	trading.RegisterExecutionRoutes(router.Group("/api/v1"), service)
	return router, trace
}

type stage9ExecutionWriteComboGateway struct {
	trace  *stage9ExecutionWriteTrace
	place  func(context.Context, broker.ComboOrderIntent) (srv.ExecutionOrder, error)
	cancel func(context.Context, string) (srv.ExecutionOrder, error)
}

func (gateway *stage9ExecutionWriteComboGateway) PlaceCombo(ctx context.Context, intent broker.ComboOrderIntent) (srv.ExecutionOrder, error) {
	return gateway.place(ctx, intent)
}

func (gateway *stage9ExecutionWriteComboGateway) CancelCombo(ctx context.Context, id string) (srv.ExecutionOrder, error) {
	return gateway.cancel(ctx, id)
}

type stage9ExecutionWriteBroker struct {
	id    string
	mode  string
	trace *stage9ExecutionWriteTrace
}

func (brokerAdapter *stage9ExecutionWriteBroker) ID() string { return brokerAdapter.id }
func (brokerAdapter *stage9ExecutionWriteBroker) Descriptor() broker.Descriptor {
	return broker.Descriptor{ID: brokerAdapter.id}
}
func (brokerAdapter *stage9ExecutionWriteBroker) DiscoverAccounts(context.Context) ([]broker.Account, error) {
	return nil, nil
}
func (brokerAdapter *stage9ExecutionWriteBroker) Trading() broker.TradingService      { return nil }
func (brokerAdapter *stage9ExecutionWriteBroker) MarketData() broker.MarketDataReader { return nil }
func (brokerAdapter *stage9ExecutionWriteBroker) ValidateProductOrder(ctx context.Context, query broker.ProductRuleQuery) (*broker.ProductRuleResult, error) {
	kind := "buying-power"
	if query.FeatureID == broker.FeatureExecutionOrderPreview {
		kind = "order-preview"
	}
	brokerAdapter.trace.add(map[string]any{"kind": kind, "query": query})
	if brokerAdapter.mode == "buying-power-timeout" {
		return nil, broker.NewBrokerError(brokerAdapter.id, broker.ErrCodeTimeout, "buying power timed out")
	}
	if brokerAdapter.mode == "order-preview-timeout" {
		return nil, broker.NewBrokerError(brokerAdapter.id, broker.ErrCodeTimeout, "order preview timed out")
	}
	if brokerAdapter.mode == "combo-preview-cancel" && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return stage9ExecutionWriteRuleResult(), nil
}

func (brokerAdapter *stage9ExecutionWriteBroker) PreviewComboOrder(ctx context.Context, intent broker.ComboOrderIntent) (*broker.ProductRuleResult, error) {
	brokerAdapter.trace.add(map[string]any{"kind": "combo-preview", "intent": intent})
	if brokerAdapter.mode == "combo-preview-rate" {
		return nil, broker.NewBrokerError(brokerAdapter.id, broker.ErrCodeRateLimited, "combo preview limited")
	}
	if brokerAdapter.mode == "combo-preview-cancel" && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return stage9ExecutionWriteRuleResult(), nil
}

func (brokerAdapter *stage9ExecutionWriteBroker) PlaceComboOrder(context.Context, broker.ComboOrderIntent) (*broker.ComboOrderResult, error) {
	return &broker.ComboOrderResult{BrokerOrderID: "unused", Status: "SUBMITTED"}, nil
}

func (brokerAdapter *stage9ExecutionWriteBroker) CancelComboOrder(context.Context, broker.ReadQuery, string) error {
	return nil
}

func stage9ExecutionWriteRuleResult() *broker.ProductRuleResult {
	impact := 37.5
	return &broker.ProductRuleResult{
		Allowed:           true,
		BuyingPowerImpact: &impact,
		Warnings:          []string{"fixture warning"},
		AccountImpact:     &broker.OptionComboAccountImpact{BuyingPowerDecrease: &impact},
		OptionAnalysis:    &broker.OptionComboAnalysis{Strategy: "vertical", Bid: new(1.1), Ask: new(1.3)},
	}
}

func stage9ExecutionWriteOrder(id, status string) srv.ExecutionOrder {
	brokerOrderID := "broker-" + id
	return srv.ExecutionOrder{
		InternalOrderID: id, BrokerOrderID: &brokerOrderID, Status: status,
	}
}

func stage9ExecutionWriteCommandJSON(command srv.ExecutionOrderCommand) map[string]any {
	return map[string]any{
		"brokerId": command.BrokerID, "query": command.Query, "symbol": command.Symbol,
		"side": command.Side, "orderType": command.OrderType, "remark": command.Remark,
		"session": command.Session, "orderKind": command.OrderKind, "productClass": command.ProductClass,
		"quantityMode": command.QuantityMode, "previewId": command.PreviewID,
	}
}

func stage9ExecutionWriteRequest(
	t *testing.T,
	router http.Handler,
	request stage9ExecutionWriteFixtureRequest,
) *httptest.ResponseRecorder {
	t.Helper()
	ctx := context.Background()
	if request.Context == "canceled" {
		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(ctx)
		cancel()
	}
	if request.Context == "deadline" {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, time.Unix(1, 0))
		cancel()
	}
	var body io.Reader
	if request.Body != nil {
		body = strings.NewReader(*request.Body)
	}
	target := request.RequestPath
	if strings.Contains(target, "%zz") {
		target = strings.Replace(target, "%zz", "placeholder", 1)
	}
	httpRequest := httptest.NewRequestWithContext(ctx, request.Method, target, body)
	if target != request.RequestPath {
		httpRequest.URL.Path = request.RequestPath
		httpRequest.URL.RawPath = request.RequestPath
		httpRequest.RequestURI = request.RequestPath
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httpRequest)
	return response
}

func stage9ExecutionWriteHeaders(response *httptest.ResponseRecorder) map[string]string {
	return map[string]string{"Content-Type": response.Header().Get("Content-Type")}
}

func stage9NormalizeExecutionWriteEnvelope(t *testing.T, name string, envelope map[string]any) {
	t.Helper()
	rawTimestamp, ok := envelope["timestamp"].(string)
	if !ok {
		t.Fatalf("case %s response has no timestamp", name)
	}
	if _, err := time.Parse(time.RFC3339Nano, rawTimestamp); err != nil {
		t.Fatalf("case %s timestamp %q is not RFC3339Nano: %v", name, rawTimestamp, err)
	}
	envelope["timestamp"] = stage9ExecutionWriteTimestamp
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		return
	}
	for _, field := range []string{"previewAt", "checkedAt"} {
		if value, exists := data[field]; exists {
			raw, ok := value.(string)
			if !ok {
				t.Fatalf("case %s %s is not a string", name, field)
			}
			if _, err := time.Parse(time.RFC3339Nano, raw); err != nil {
				t.Fatalf("case %s %s=%q is not RFC3339Nano: %v", name, field, raw, err)
			}
			data[field] = stage9ExecutionWriteTimestamp
		}
	}
	if value, exists := data["expiresAt"]; exists {
		raw, ok := value.(string)
		if !ok {
			t.Fatalf("case %s expiresAt is not a string", name)
		}
		if _, err := time.Parse(time.RFC3339Nano, raw); err != nil {
			t.Fatalf("case %s expiresAt=%q is not RFC3339Nano: %v", name, raw, err)
		}
		data["expiresAt"] = "2026-08-23T10:05:00Z"
	}
}

func stage9ExecutionWritePortCall(request stage9ExecutionWriteFixtureRequest) bool {
	path := strings.SplitN(request.RequestPath, "?", 2)[0]
	if request.Method != http.MethodPost {
		return false
	}
	for _, prefix := range []string{
		"/api/v1/execution/combos/",
		"/api/v1/execution/orders/",
	} {
		if suffix, ok := strings.CutPrefix(path, prefix); ok && strings.HasSuffix(suffix, "/cancel") {
			rawID, ok := strings.CutSuffix(suffix, "/cancel")
			return ok && rawID != "" && !strings.Contains(rawID, "/") &&
				!stage9ExecutionWriteInvalidPercent(rawID)
		}
	}
	isBodyRoute := path == "/api/v1/execution/buying-power" ||
		path == "/api/v1/execution/combos/previews" || path == "/api/v1/execution/combos" ||
		path == "/api/v1/execution/orders" || path == "/api/v1/execution/previews"
	if !isBodyRoute {
		return false
	}
	if request.Body == nil || *request.Body == "" {
		return false
	}
	var value json.RawMessage
	decoder := json.NewDecoder(strings.NewReader(*request.Body))
	if decoder.Decode(&value) != nil {
		return false
	}
	trimmed := strings.TrimSpace(string(value))
	return trimmed == "null" || strings.HasPrefix(trimmed, "{")
}

func stage9ExecutionWriteInvalidPercent(value string) bool {
	for index := 0; index < len(value); index++ {
		if value[index] != '%' {
			continue
		}
		if index+2 >= len(value) || !isHexDigit(value[index+1]) || !isHexDigit(value[index+2]) {
			return true
		}
		index += 2
	}
	return false
}

func isHexDigit(value byte) bool {
	return value >= '0' && value <= '9' || value >= 'a' && value <= 'f' || value >= 'A' && value <= 'F'
}
