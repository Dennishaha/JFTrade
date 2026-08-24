package servercoretest

import "testing"

func TestBrokerReadRehearsalPreservesWireAndRequiresRestartForGoRollback(t *testing.T) {
	operations := []string{
		"GET /api/v1/brokers/capabilities",
		"GET /api/v1/brokers/{brokerId}/runtime",
		"GET /api/v1/brokers/{brokerId}/funds",
		"GET /api/v1/brokers/{brokerId}/positions",
		"GET /api/v1/brokers/{brokerId}/orders",
		"GET /api/v1/brokers/{brokerId}/fills",
		"GET /api/v1/brokers/{brokerId}/cash-flows",
		"GET /api/v1/brokers/{brokerId}/order-fees",
		"GET /api/v1/brokers/{brokerId}/margin-ratios",
		"GET /api/v1/brokers/{brokerId}/max-trade-qtys",
		"GET /api/v1/brokers/{brokerId}/quote",
		"GET /api/v1/brokers/{brokerId}/klines",
		"GET /api/v1/brokers/{brokerId}/securities",
	}
	paths := []string{
		"/api/v1/brokers/capabilities",
		"/api/v1/brokers/missing/runtime",
		"/api/v1/brokers/missing/funds?market=US",
		"/api/v1/brokers/missing/positions?market=US",
		"/api/v1/brokers/missing/orders?scope=current&symbol=US.AAPL",
		"/api/v1/brokers/missing/fills?scope=current&symbol=US.AAPL",
		"/api/v1/brokers/missing/cash-flows?clearingDate=2026-08-21",
		"/api/v1/brokers/missing/order-fees?orderIdEx=OID-1",
		"/api/v1/brokers/missing/margin-ratios?market=US&symbol=US.AAPL",
		"/api/v1/brokers/missing/max-trade-qtys?market=US&symbol=US.AAPL&orderType=LIMIT&price=100",
		"/api/v1/brokers/missing/quote?symbol=US.AAPL",
		"/api/v1/brokers/missing/klines?symbol=US.AAPL&period=1d&limit=10",
		"/api/v1/brokers/missing/securities?symbol=US.AAPL",
	}
	operationPaths := make(map[string]string, len(operations))
	for index, operation := range operations {
		operationPaths[operation] = paths[index]
	}
	runReadRouteRehearsal(t, readRouteRehearsalSpec{
		prefix:         "broker-read",
		operations:     operations,
		paths:          paths,
		operationPaths: operationPaths,
		dynamicJSONKeys: map[string]struct{}{
			"timestamp": {}, "checkedAt": {}, "observedAt": {}, "quoteAt": {},
		},
		prepareStore: prepareDisabledFutuReadRehearsal,
	})
}
