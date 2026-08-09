package marketdataapp

import (
	"context"
	"errors"
	"strings"
	"time"

	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"

	futuintegration "github.com/jftrade/jftrade-main/internal/integration/futu"
	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/pkg/broker"
	marketpkg "github.com/jftrade/jftrade-main/pkg/market"
)

var errFutuIntegrationNotEnabled = errors.New("futu integration is not enabled")

const MarketSecurityDetailsStreamInterval = 3 * time.Second
const MarketDepthStreamRefreshInterval = 15 * time.Second

// HTTPAdapterDependencies supplies the application services behind market-data
// HTTP response mapping.
type HTTPAdapterDependencies struct {
	Service           func() *mdsrv.Service
	MarketDataRuntime func() *futuintegration.MarketDataRuntime
	FutuEnabled       func() bool
}

// HTTPAdapters maps market-data service results into the console HTTP payloads
// without coupling the transport to servercore.
type HTTPAdapters struct {
	dependencies HTTPAdapterDependencies
}

func NewHTTPAdapters(dependencies HTTPAdapterDependencies) *HTTPAdapters {
	return &HTTPAdapters{dependencies: dependencies}
}

func (a *HTTPAdapters) SecurityDetailsResponse(ctx context.Context, path string) (map[string]any, error) {
	market, symbol := PathTail(path, "/api/v1/market-data/securities/")
	return a.SecurityDetailsResponseForInstrument(ctx, market, symbol)
}

func (a *HTTPAdapters) SecurityDetailsResponseForInstrument(ctx context.Context, market string, symbol string) (map[string]any, error) {
	market = strings.ToUpper(strings.TrimSpace(market))
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	instrumentID := market + "." + symbol
	marketDataRuntime := a.marketDataRuntime()
	if marketDataRuntime == nil || !a.futuEnabled() {
		return nil, errFutuIntegrationNotEnabled
	}
	details, err := marketDataRuntime.QuerySecurityDetails(ctx, instrumentID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"request":  map[string]any{"market": market, "symbol": symbol, "instrumentId": instrumentID},
		"security": details,
		"meta":     map[string]any{"instrumentId": instrumentID, "source": "bbgo:futu", "resolvedAt": time.Now().UTC().Format(time.RFC3339Nano), "fromCache": false},
	}, nil
}

func (a *HTTPAdapters) SnapshotResponse(ctx context.Context, path string, query map[string][]string) (map[string]any, error) {
	market, symbol := PathTail(path, "/api/v1/market-data/snapshots/")
	decoded, err := DecodeSnapshotQuery(query)
	if err != nil {
		return nil, err
	}
	return a.SnapshotResponseForInstrument(ctx, market, symbol, decoded)
}

func (a *HTTPAdapters) SnapshotResponseForInstrument(ctx context.Context, market string, symbol string, query SnapshotQuery) (map[string]any, error) {
	response, err := a.service().GetSnapshot(ctx, market, symbol, query.ForceRefresh())
	return map[string]any(response), err
}

func (a *HTTPAdapters) CandlesResponse(ctx context.Context, path string, query map[string][]string) (map[string]any, error) {
	market, symbol := PathTail(path, "/api/v1/market-data/candles/")
	decoded, err := DecodeCandlesQuery(query)
	if err != nil {
		return nil, err
	}
	return a.CandlesResponseForInstrument(ctx, market, symbol, decoded)
}

func (a *HTTPAdapters) CandlesResponseForInstrument(ctx context.Context, market string, symbol string, query CandlesQuery) (map[string]any, error) {
	period := query.NormalizedPeriod()
	limit := query.LimitOrDefault(200, 1000)
	fromTime := ""
	if !query.FromTime.IsZero() {
		fromTime = query.FromTime.UTC().Format(time.RFC3339Nano)
	}
	if !query.From.IsZero() {
		fromTime = query.From.UTC().Format(time.RFC3339Nano)
	}
	toTime := ""
	if !query.ToTime.IsZero() {
		toTime = query.ToTime.UTC().Format(time.RFC3339Nano)
	}
	if !query.To.IsZero() {
		toTime = query.To.UTC().Format(time.RFC3339Nano)
	}
	response, err := a.service().GetCandles(ctx, mdsrv.HistoricalCandlesQuery{
		Market: market, Symbol: symbol, Period: period, Limit: limit,
		FromTime: fromTime, ToTime: toTime,
		Sessions: query.Sessions, SessionsSpecified: query.SessionsSpecified,
	})
	return map[string]any(response), err
}

func ShouldAnnotateHistoricalKLineSession(market string, interval bbgotypes.Interval) bool {
	resolvedMarket, preferredPrefix, err := marketpkg.NormalizeMarketInput(market)
	return err == nil && resolvedMarket == "US" && preferredPrefix == "US" && interval.Duration() > 0 && interval.Duration() <= time.Hour
}

// --- Depth (Order Book) ---

func (a *HTTPAdapters) DepthResponseForInstrument(ctx context.Context, market string, symbol string, query DepthQuery) (map[string]any, error) {
	market = strings.ToUpper(strings.TrimSpace(market))
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	instrumentID := market + "." + symbol
	num := query.NumOrDefault(10, 50)

	marketDataRuntime := a.marketDataRuntime()
	if marketDataRuntime == nil || !a.futuEnabled() {
		return nil, errFutuIntegrationNotEnabled
	}

	brokerResult, err := marketDataRuntime.QueryOrderBook(ctx, broker.OrderBookQuery{
		ReadQuery: ReadQueryForInstrument(instrumentID),
		Symbol:    instrumentID,
		Num:       num,
	})
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"request": map[string]any{"market": market, "symbol": symbol, "instrumentId": instrumentID, "num": num},
		"depth":   brokerResult,
		"meta": map[string]any{
			"instrumentId": instrumentID,
			"source":       "bbgo:futu",
			"resolvedAt":   time.Now().UTC().Format(time.RFC3339Nano),
			"fromCache":    false,
		},
	}, nil
}

func (a *HTTPAdapters) service() *mdsrv.Service {
	if a == nil || a.dependencies.Service == nil {
		return nil
	}
	return a.dependencies.Service()
}

func (a *HTTPAdapters) marketDataRuntime() *futuintegration.MarketDataRuntime {
	if a == nil || a.dependencies.MarketDataRuntime == nil {
		return nil
	}
	return a.dependencies.MarketDataRuntime()
}

func (a *HTTPAdapters) futuEnabled() bool {
	if a == nil || a.dependencies.FutuEnabled == nil {
		return false
	}
	return a.dependencies.FutuEnabled()
}

func ReadQueryForInstrument(instrumentID string) broker.ReadQuery {
	parts := strings.SplitN(instrumentID, ".", 2)
	market := ""
	if len(parts) == 2 {
		market = parts[0]
	}
	return broker.ReadQuery{
		Market: market,
	}
}
