package trading

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	srv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestExecutionProductRoutesBuyingPowerComboLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	selected := &comboRouteBroker{id: "route-combo"}
	gateway := &comboRouteGateway{}
	service := srv.NewService(
		srv.WithActiveBroker(func() broker.Broker { return selected }),
		srv.WithComboOrderGateway(gateway),
	)
	router := gin.New()
	RegisterExecutionRoutes(router.Group("/api/v1"), service)

	buyingPower := performExecutionProductRequest(
		t, router, http.MethodPost, "/api/v1/execution/buying-power",
		`{"brokerId":"route-combo","market":"US","instrument":{"instrumentId":"US.AAPL","productClass":"option"}}`,
	)
	if buyingPower.Code != http.StatusOK || selected.ruleCalls != 1 {
		t.Fatalf("buying power status=%d body=%s calls=%d", buyingPower.Code, buyingPower.Body.String(), selected.ruleCalls)
	}

	request := `{
		"brokerId":"route-combo",
		"market":"US",
		"clientOrderId":"route-client",
		"orderKind":"option_combo",
		"underlyingInstrumentId":"US.AAPL",
		"optionStrategy":"vertical",
		"nearExpiry":"2026-07-17",
		"spread":10,
		"legs":[
			{"instrumentId":"US.OPTION.ONE","side":"BUY","ratio":1},
			{"instrumentId":"US.OPTION.TWO","side":"SELL","ratio":1}
		]
	}`
	preview := performExecutionProductRequest(
		t, router, http.MethodPost, "/api/v1/execution/combos/previews", request,
	)
	if preview.Code != http.StatusOK || selected.previewCalls != 1 {
		t.Fatalf("preview status=%d body=%s calls=%d", preview.Code, preview.Body.String(), selected.previewCalls)
	}
	previewID := executionProductDataString(t, preview.Body.Bytes(), "previewId")
	placeBody := strings.Replace(request, `"orderKind":"option_combo",`, `"orderKind":"option_combo","previewId":"`+previewID+`",`, 1)
	placed := performExecutionProductRequest(
		t, router, http.MethodPost, "/api/v1/execution/combos", placeBody,
	)
	if placed.Code != http.StatusOK || gateway.placeCalls != 1 {
		t.Fatalf("place status=%d body=%s calls=%d", placed.Code, placed.Body.String(), gateway.placeCalls)
	}
	canceled := performExecutionProductRequest(
		t, router, http.MethodPost, "/api/v1/execution/combos/combo-internal/cancel", "",
	)
	if canceled.Code != http.StatusOK || gateway.cancelCalls != 1 {
		t.Fatalf("cancel status=%d body=%s calls=%d", canceled.Code, canceled.Body.String(), gateway.cancelCalls)
	}
}

func TestExecutionProductRoutesValidationAndServiceErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	selected := &comboRouteBroker{id: "route-errors"}
	gateway := &comboRouteGateway{}
	service := srv.NewService(
		srv.WithActiveBroker(func() broker.Broker { return selected }),
		srv.WithComboOrderGateway(gateway),
	)
	router := gin.New()
	RegisterExecutionRoutes(router.Group("/api/v1"), service)

	for _, path := range []string{
		"/api/v1/execution/buying-power",
		"/api/v1/execution/combos/previews",
		"/api/v1/execution/combos",
	} {
		rec := performExecutionProductRequest(t, router, http.MethodPost, path, `{`)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s malformed status=%d body=%s", path, rec.Code, rec.Body.String())
		}
	}
	invalidCombo := performExecutionProductRequest(
		t, router, http.MethodPost, "/api/v1/execution/combos/previews",
		`{"brokerId":"route-errors","market":"US","orderKind":"option_combo","legs":[]}`,
	)
	if invalidCombo.Code != http.StatusBadRequest {
		t.Fatalf("invalid combo status=%d body=%s", invalidCombo.Code, invalidCombo.Body.String())
	}

	selected.ruleErr = broker.NewBrokerError(selected.id, broker.ErrCodeTimeout, "slow")
	buyingPower := performExecutionProductRequest(
		t, router, http.MethodPost, "/api/v1/execution/buying-power",
		`{"brokerId":"route-errors","market":"US"}`,
	)
	if buyingPower.Code != http.StatusGatewayTimeout {
		t.Fatalf("buying power error status=%d body=%s", buyingPower.Code, buyingPower.Body.String())
	}
	selected.ruleErr = nil
	selected.previewErr = broker.NewBrokerError(selected.id, broker.ErrCodeRateLimited, "limited")
	preview := performExecutionProductRequest(
		t, router, http.MethodPost, "/api/v1/execution/combos/previews",
		`{"brokerId":"route-errors","market":"US","clientOrderId":"client","orderKind":"option_combo","underlyingInstrumentId":"US.AAPL","optionStrategy":"vertical","nearExpiry":"2026-07-17","spread":1,"legs":[{"instrumentId":"US.ONE","side":"BUY","ratio":1},{"instrumentId":"US.TWO","side":"SELL","ratio":1}]}`,
	)
	if preview.Code != http.StatusTooManyRequests {
		t.Fatalf("preview error status=%d body=%s", preview.Code, preview.Body.String())
	}

	gateway.placeErr = errors.New("place failed")
	place := performExecutionProductRequest(
		t, router, http.MethodPost, "/api/v1/execution/combos",
		`{"brokerId":"route-errors","market":"US","clientOrderId":"client","previewId":"preview","orderKind":"option_combo","underlyingInstrumentId":"US.AAPL","optionStrategy":"vertical","nearExpiry":"2026-07-17","spread":1,"legs":[{"instrumentId":"US.ONE","side":"BUY","ratio":1},{"instrumentId":"US.TWO","side":"SELL","ratio":1}]}`,
	)
	if place.Code != http.StatusBadGateway {
		t.Fatalf("place error status=%d body=%s", place.Code, place.Body.String())
	}
	gateway.cancelErr = broker.NewBrokerError(selected.id, broker.ErrCodeNotConnected, "offline")
	cancel := performExecutionProductRequest(
		t, router, http.MethodPost, "/api/v1/execution/combos/combo/cancel", "",
	)
	if cancel.Code != http.StatusBadGateway {
		t.Fatalf("cancel error status=%d body=%s", cancel.Code, cancel.Body.String())
	}

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	handleExecutionComboCancel(service)(ctx)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing combo cancel id status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func performExecutionProductRequest(
	t *testing.T,
	router http.Handler,
	method string,
	path string,
	body string,
) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	router.ServeHTTP(rec, req)
	return rec
}

func executionProductDataString(t *testing.T, content []byte, field string) string {
	t.Helper()
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		t.Fatalf("decode %s: %v", string(content), err)
	}
	value, _ := envelope.Data[field].(string)
	if value == "" {
		t.Fatalf("%s missing from %s", field, string(content))
	}
	return value
}

type comboRouteBroker struct {
	id           string
	ruleCalls    int
	previewCalls int
	ruleErr      error
	previewErr   error
}

func (b *comboRouteBroker) ID() string                    { return b.id }
func (b *comboRouteBroker) Descriptor() broker.Descriptor { return broker.Descriptor{ID: b.id} }
func (b *comboRouteBroker) DiscoverAccounts(context.Context) ([]broker.Account, error) {
	return nil, nil
}
func (b *comboRouteBroker) Trading() broker.TradingService      { return nil }
func (b *comboRouteBroker) MarketData() broker.MarketDataReader { return nil }
func (b *comboRouteBroker) ValidateProductOrder(
	context.Context,
	broker.ProductRuleQuery,
) (*broker.ProductRuleResult, error) {
	b.ruleCalls++
	if b.ruleErr != nil {
		return nil, b.ruleErr
	}
	return &broker.ProductRuleResult{Allowed: true}, nil
}
func (b *comboRouteBroker) PreviewComboOrder(
	context.Context,
	broker.ComboOrderIntent,
) (*broker.ProductRuleResult, error) {
	b.previewCalls++
	if b.previewErr != nil {
		return nil, b.previewErr
	}
	return &broker.ProductRuleResult{Allowed: true}, nil
}
func (b *comboRouteBroker) PlaceComboOrder(context.Context, broker.ComboOrderIntent) (*broker.ComboOrderResult, error) {
	return &broker.ComboOrderResult{}, nil
}
func (b *comboRouteBroker) CancelComboOrder(context.Context, broker.ReadQuery, string) error {
	return nil
}

type comboRouteGateway struct {
	placeCalls  int
	cancelCalls int
	placeErr    error
	cancelErr   error
}

func (g *comboRouteGateway) PlaceCombo(
	context.Context,
	broker.ComboOrderIntent,
) (srv.ExecutionOrder, error) {
	g.placeCalls++
	return srv.ExecutionOrder{InternalOrderID: "combo-internal", Status: "SUBMITTED"}, g.placeErr
}
func (g *comboRouteGateway) CancelCombo(
	_ context.Context,
	id string,
) (srv.ExecutionOrder, error) {
	g.cancelCalls++
	return srv.ExecutionOrder{InternalOrderID: id, Status: "CANCEL_SUBMITTED"}, g.cancelErr
}
