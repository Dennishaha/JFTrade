package trading

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	srv "github.com/jftrade/jftrade-main/internal/trading"
)

func TestTradingOpenAPIDocumentationRoutesMatchGinRegistration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api/v1")
	service := srv.NewService()
	RegisterPortfolioRoutes(api, service)
	RegisterRoutes(api, service)
	RegisterExecutionRoutes(api, service)

	registered := make(map[string]struct{})
	for _, route := range router.Routes() {
		registered[route.Method+" "+route.Path] = struct{}{}
	}

	documentedRoutes := []struct {
		name   string
		docID  func() string
		method string
		path   string
	}{
		{"portfolio cash balances", documentPortfolioCashBalancesRoute, http.MethodGet, "/api/v1/portfolio/:brokerId/cash-balances"},
		{"portfolio positions", documentPortfolioPositionsRoute, http.MethodGet, "/api/v1/portfolio/:brokerId/positions"},
		{"broker funds", documentBrokerFundsRoute, http.MethodGet, "/api/v1/brokers/:brokerId/funds"},
		{"broker positions", documentBrokerPositionsRoute, http.MethodGet, "/api/v1/brokers/:brokerId/positions"},
		{"broker orders", documentBrokerOrdersRoute, http.MethodGet, "/api/v1/brokers/:brokerId/orders"},
		{"broker fills", documentBrokerFillsRoute, http.MethodGet, "/api/v1/brokers/:brokerId/fills"},
		{"broker cash flows", documentBrokerCashFlowsRoute, http.MethodGet, "/api/v1/brokers/:brokerId/cash-flows"},
		{"broker order fees", documentBrokerOrderFeesRoute, http.MethodGet, "/api/v1/brokers/:brokerId/order-fees"},
		{"broker margin ratios", documentBrokerMarginRatiosRoute, http.MethodGet, "/api/v1/brokers/:brokerId/margin-ratios"},
		{"broker max trade quantity", documentBrokerMaxTradeQuantityRoute, http.MethodGet, "/api/v1/brokers/:brokerId/max-trade-qtys"},
		{"broker quote", documentBrokerQuoteRoute, http.MethodGet, "/api/v1/brokers/:brokerId/quote"},
		{"broker klines", documentBrokerKLinesRoute, http.MethodGet, "/api/v1/brokers/:brokerId/klines"},
		{"broker securities", documentBrokerSecuritiesRoute, http.MethodGet, "/api/v1/brokers/:brokerId/securities"},
		{"broker runtime", documentBrokerRuntimeRoute, http.MethodGet, "/api/v1/brokers/:brokerId/runtime"},
		{"broker place order", documentBrokerPlaceOrderRoute, http.MethodPost, "/api/v1/brokers/:brokerId/orders"},
		{"broker cancel orders", documentBrokerCancelOrdersRoute, http.MethodDelete, "/api/v1/brokers/:brokerId/orders"},
		{"broker unlock trade", documentBrokerUnlockTradeRoute, http.MethodPost, "/api/v1/brokers/:brokerId/unlock"},
		{"execution orders", documentExecutionOrdersRoute, http.MethodGet, "/api/v1/execution/orders"},
		{"execution order details", documentExecutionOrderDetailsRoute, http.MethodGet, "/api/v1/execution/orders/:internalOrderId"},
		{"execution place", documentExecutionPlaceRoute, http.MethodPost, "/api/v1/execution/orders"},
		{"execution cancel", documentExecutionCancelRoute, http.MethodPost, "/api/v1/execution/orders/:internalOrderId/cancel"},
		{"execution events", documentExecutionEventsRoute, http.MethodGet, "/api/v1/execution/orders/:internalOrderId/events"},
	}

	documentIDs := make(map[string]string, len(documentedRoutes))
	for _, route := range documentedRoutes {
		t.Run(route.name, func(t *testing.T) {
			id := route.docID()
			require.NotEmpty(t, id, "OpenAPI route documentation needs a stable identifier")
			if prior, duplicate := documentIDs[id]; duplicate {
				t.Fatalf("OpenAPI route identifier %q is shared by %q and %q", id, prior, route.name)
			}
			documentIDs[id] = route.name
			assert.Contains(t, registered, route.method+" "+route.path)
		})
	}
}
