package servercoretest

import "testing"

func TestMarketDataQuoteReadRehearsalPreservesWireAndRequiresRestartForGoRollback(t *testing.T) {
	operations := []string{
		"GET /api/v1/market-data/broker-queue/{instrumentId}",
		"GET /api/v1/market-data/candles/{market}/{symbol}",
		"GET /api/v1/market-data/capital-flow/{instrumentId}",
		"GET /api/v1/market-data/depth/{market}/{symbol}",
		"GET /api/v1/market-data/instruments/{instrumentId}/profile",
		"GET /api/v1/market-data/intraday/{instrumentId}",
		"GET /api/v1/market-data/securities/{market}/{symbol}",
		"GET /api/v1/market-data/snapshots/{market}/{symbol}",
		"GET /api/v1/market-data/subscriptions",
		"GET /api/v1/market-data/ticks/{instrumentId}",
	}
	runReadRouteRehearsal(t, readRouteRehearsalSpec{
		prefix:     "market-data-quote-read",
		operations: operations,
		paths: []string{
			"/api/v1/market-data/broker-queue/US.AAPL?brokerId=missing&market=US",
			"/api/v1/market-data/candles/US/AAPL?period=bad",
			"/api/v1/market-data/capital-flow/US.AAPL?brokerId=missing&market=US",
			"/api/v1/market-data/depth/US/AAPL?num=bad",
			"/api/v1/market-data/instruments/US.AAPL/profile?brokerId=missing&market=US",
			"/api/v1/market-data/intraday/US.AAPL?brokerId=missing&market=US",
			"/api/v1/market-data/securities/BAD/AAPL",
			"/api/v1/market-data/snapshots/US/AAPL?refresh=maybe",
			"/api/v1/market-data/subscriptions",
			"/api/v1/market-data/ticks/US.AAPL?brokerId=missing&market=US",
		},
		operationPaths: map[string]string{
			operations[0]: "/api/v1/market-data/broker-queue/US.AAPL?brokerId=missing&market=US",
			operations[1]: "/api/v1/market-data/candles/US/AAPL?period=bad",
			operations[2]: "/api/v1/market-data/capital-flow/US.AAPL?brokerId=missing&market=US",
			operations[3]: "/api/v1/market-data/depth/US/AAPL?num=bad",
			operations[4]: "/api/v1/market-data/instruments/US.AAPL/profile?brokerId=missing&market=US",
			operations[5]: "/api/v1/market-data/intraday/US.AAPL?brokerId=missing&market=US",
			operations[6]: "/api/v1/market-data/securities/BAD/AAPL",
			operations[7]: "/api/v1/market-data/snapshots/US/AAPL?refresh=maybe",
			operations[8]: "/api/v1/market-data/subscriptions",
			operations[9]: "/api/v1/market-data/ticks/US.AAPL?brokerId=missing&market=US",
		},
		dynamicJSONKeys: map[string]struct{}{
			"timestamp": {}, "resolvedAt": {}, "observedAt": {}, "quoteAt": {}, "reconciledAt": {},
		},
		prepareStore: prepareDisabledFutuReadRehearsal,
	})
}
