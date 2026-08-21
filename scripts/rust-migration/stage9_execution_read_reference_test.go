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

	"github.com/gin-gonic/gin"
	tradingapi "github.com/jftrade/jftrade-main/internal/api/trading"
	srv "github.com/jftrade/jftrade-main/internal/trading"
)

const stage9ExecutionReadFixtureVersion = "stage9.execution-read.v1"

type stage9ExecutionReadCase struct {
	Name           string          `json:"name"`
	Method         string          `json:"method"`
	RequestPath    string          `json:"requestPath"`
	ExpectedStatus int             `json:"expectedStatus"`
	Data           json.RawMessage `json:"data,omitempty"`
	ErrorCode      string          `json:"errorCode,omitempty"`
	ErrorMessage   string          `json:"errorMessage,omitempty"`
}

type stage9ExecutionReadFixture struct {
	Version string                    `json:"version"`
	Cases   []stage9ExecutionReadCase `json:"cases"`
}

// TestStage9ExecutionReadFixtureMatchesCurrentGoOwner freezes order list,
// receipt, and event projections without connecting OpenD or a real broker.
func TestStage9ExecutionReadFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 execution fixture source")
	}
	fixturePath := filepath.Join(filepath.Dir(source), "../../tests/fixtures/rust-migration/stage9/execution-read.json")
	cases := []struct {
		name    string
		path    string
		service *srv.Service
	}{
		{name: "orders-default", path: "/api/v1/execution/orders", service: executionFixtureService(nil, nil, false)},
		{name: "orders-active", path: "/api/v1/execution/orders?scope=%20ACTIVE%20&brokerId=%20futu%20&accountId=%20ACC-1%20&market=us", service: executionFixtureService(nil, nil, false)},
		{name: "orders-failed", path: "/api/v1/execution/orders?fixture=error", service: executionFixtureService(errors.New("store failed"), nil, false)},
		{name: "order-details", path: "/api/v1/execution/orders/order-1", service: executionFixtureService(nil, nil, false)},
		{name: "order-missing", path: "/api/v1/execution/orders/missing", service: executionFixtureService(nil, nil, true)},
		{name: "order-details-failed", path: "/api/v1/execution/orders/order-1?fixture=error", service: executionFixtureService(errors.New("store failed"), nil, false)},
		{name: "order-events", path: "/api/v1/execution/orders/order-1/events", service: executionFixtureService(nil, nil, false)},
		{name: "order-events-unknown", path: "/api/v1/execution/orders/unknown/events", service: executionFixtureService(nil, nil, false)},
		{name: "order-events-failed", path: "/api/v1/execution/orders/order-1/events?fixture=error", service: executionFixtureService(nil, errors.New("events failed"), false)},
	}
	want := stage9ExecutionReadFixture{Version: stage9ExecutionReadFixtureVersion, Cases: make([]stage9ExecutionReadCase, 0, len(cases))}
	for _, testCase := range cases {
		gin.SetMode(gin.TestMode)
		router := gin.New()
		tradingapi.RegisterExecutionRoutes(router.Group("/api/v1"), testCase.service)
		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, testCase.path, nil)
		router.ServeHTTP(recorder, request)
		entry := stage9ExecutionReadCase{Name: testCase.name, Method: http.MethodGet, RequestPath: testCase.path, ExpectedStatus: recorder.Code}
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
			entry.Data = normalizeExecutionFixtureData(envelope.Data)
		}
		want.Cases = append(want.Cases, entry)
	}
	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode execution fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write execution fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read execution fixture: %v", err)
	}
	var got stage9ExecutionReadFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode execution fixture: %v", err)
	}
	for index := range got.Cases {
		got.Cases[index].Data = normalizeExecutionFixtureData(got.Cases[index].Data)
		want.Cases[index].Data = normalizeExecutionFixtureData(want.Cases[index].Data)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stage 9 execution read fixture drifted from the Go owner: got=%#v want=%#v", got, want)
	}
}

func normalizeExecutionFixtureData(data json.RawMessage) json.RawMessage {
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

func executionFixtureService(listErr, eventErr error, missing bool) *srv.Service {
	return srv.NewService(
		srv.WithListOrders(func(_ context.Context, filter srv.ExecutionOrderFilter) (srv.ExecutionOrders, error) {
			if listErr != nil {
				return srv.ExecutionOrders{}, listErr
			}
			if missing {
				return srv.ExecutionOrders{Orders: []srv.ExecutionOrder{}}, nil
			}
			_ = filter
			return srv.ExecutionOrders{Orders: []srv.ExecutionOrder{executionFixtureOrder("order-1", srv.OrderStatusBrokerAccepted)}}, nil
		}),
		srv.WithGetOrderEvents(func(_ context.Context, id string) (srv.ExecutionOrderEvents, error) {
			if eventErr != nil {
				return srv.ExecutionOrderEvents{}, eventErr
			}
			if id != "order-1" {
				return srv.ExecutionOrderEvents{InternalOrderID: id, Events: []srv.ExecutionOrderEvent{}}, nil
			}
			return srv.ExecutionOrderEvents{InternalOrderID: id, Events: []srv.ExecutionOrderEvent{
				{ID: "event-1", InternalOrderID: id, EventType: "BROKER_PUSH_ORDER", NextStatus: srv.OrderStatusBrokerAccepted, PayloadJSON: `{"status":"SUBMITTED"}`, CreatedAt: "2026-08-15T20:01:00Z"},
			}}, nil
		}),
	)
}

func executionFixtureOrder(id, status string) srv.ExecutionOrder {
	brokerOrderID := "broker-1"
	symbol, side, orderType := "US.AAPL", "BUY", "LIMIT"
	quantity, price, filled := 10.0, 100.5, 2.0
	return srv.ExecutionOrder{
		InternalOrderID: id, BrokerID: "futu", BrokerOrderID: &brokerOrderID, Source: "fixture", SourceDetail: "reference",
		TradingEnvironment: "SIMULATE", AccountID: "ACC-1", Market: "US", ClientOrderID: nil,
		Symbol: &symbol, Side: &side, OrderType: &orderType, Status: status, RawBrokerStatus: new("SUBMITTED"),
		RequestedQuantity: &quantity, RequestedPrice: &price, FilledQuantity: &filled,
		UpdatedAt: "2026-08-15T20:01:00Z", CreatedAt: "2026-08-15T20:00:00Z",
	}
}
