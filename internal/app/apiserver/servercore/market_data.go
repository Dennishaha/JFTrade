package servercore

import (
	"context"
	"fmt"
	"strings"
	"time"

	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu"
	marketpkg "github.com/jftrade/jftrade-main/pkg/market"
)

const marketSecurityDetailsStreamInterval = 3 * time.Second
const marketDepthStreamRefreshInterval = 15 * time.Second

func marketSecurityDetailsPathTail(path string) (string, string) {
	return pathTail(path, "/api/v1/market-data/securities/")
}

func (s *Server) marketSecurityDetailsResponse(ctx context.Context, path string) (map[string]any, error) {
	market, symbol := marketSecurityDetailsPathTail(path)
	return s.marketSecurityDetailsResponseForInstrument(ctx, market, symbol)
}

func (s *Server) marketSecurityDetailsResponseForInstrument(ctx context.Context, market string, symbol string) (map[string]any, error) {
	market = strings.ToUpper(strings.TrimSpace(market))
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	instrumentID := market + "." + symbol
	exchange, err := s.futuExchangeOrError()
	if err != nil {
		return nil, err
	}
	details, err := exchange.QuerySecurityDetails(ctx, instrumentID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"request":  map[string]any{"market": market, "symbol": symbol, "instrumentId": instrumentID},
		"security": securityDetailsMap(details),
		"meta":     map[string]any{"instrumentId": instrumentID, "source": "bbgo:futu", "resolvedAt": time.Now().UTC().Format(time.RFC3339Nano), "fromCache": false},
	}, nil
}

func (s *Server) marketSnapshotResponse(ctx context.Context, path string, query map[string][]string) (map[string]any, error) {
	market, symbol := pathTail(path, "/api/v1/market-data/snapshots/")
	return s.marketSnapshotResponseForInstrument(ctx, market, symbol, decodeMarketSnapshotQuery(query))
}

func (s *Server) marketSnapshotResponseForInstrument(ctx context.Context, market string, symbol string, query marketSnapshotQuery) (map[string]any, error) {
	response, err := s.marketdataSvc.GetSnapshot(ctx, market, symbol, query.forceRefresh())
	return map[string]any(response), err
}

func (s *Server) marketCandlesResponse(ctx context.Context, path string, query map[string][]string) (map[string]any, error) {
	market, symbol := pathTail(path, "/api/v1/market-data/candles/")
	decoded, err := decodeMarketCandlesQuery(query)
	if err != nil {
		return nil, err
	}
	return s.marketCandlesResponseForInstrument(ctx, market, symbol, decoded)
}

func (s *Server) marketCandlesResponseForInstrument(ctx context.Context, market string, symbol string, query marketCandlesQuery) (map[string]any, error) {
	period := query.normalizedPeriod()
	limit := query.limitOrDefault(200, 1000)
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
	response, err := s.marketdataSvc.GetCandles(ctx, market, symbol, period, limit, fromTime, toTime)
	return map[string]any(response), err
}

func (s *Server) buildKLineCandlesResponse(ctx context.Context, market string, symbol string, instrumentID string, period string, limit int, query marketCandlesQuery) (map[string]any, error) {
	interval := bbgotypes.Interval(period)
	includeSession := shouldAnnotateHistoricalKLineSession(market, interval)
	beginAt, endAt := kLineQueryWindow(query, interval.Duration(), limit)
	exchange, err := s.futuExchangeOrError()
	if err != nil {
		return nil, err
	}
	klines, err := exchange.QueryKLines(ctx, instrumentID, interval, bbgotypes.KLineQueryOptions{Limit: limit, StartTime: &beginAt, EndTime: &endAt})
	if err != nil {
		return nil, err
	}
	candles := make([]map[string]any, 0, len(klines))
	for _, kline := range klines {
		candle := map[string]any{
			"period": period,
			"open":   kline.Open.String(),
			"high":   kline.High.String(),
			"low":    kline.Low.String(),
			"close":  kline.Close.String(),
			"volume": kline.Volume.Float64(),
			"at":     kline.StartTime.Time().UTC().Format(time.RFC3339Nano),
		}
		if includeSession {
			session, ok := exchange.ResolveKLineSession(kline)
			if !ok {
				session = marketpkg.ClassifySession(instrumentID, kline.StartTime.Time().UTC())
			}
			if session != marketpkg.SessionUnknown && session != marketpkg.SessionClosed {
				candle["session"] = string(session)
			}
		}
		candles = append(candles, candle)
	}
	extendedHours := includeSession

	return map[string]any{
		"request":       marketCandlesRequest(market, symbol, instrumentID, period, limit),
		"candles":       candles,
		"totalReturned": len(candles),
		"meta":          candleMeta(instrumentID, false, extendedHours, includeSession),
	}, nil
}

func marketCandlesRequest(market string, symbol string, instrumentID string, period string, limit int) map[string]any {
	return map[string]any{
		"instrument": map[string]any{
			"market":       market,
			"symbol":       symbol,
			"instrumentId": instrumentID,
		},
		"period": period,
		"limit":  limit,
	}
}

func candleMeta(instrumentID string, fromCache bool, extendedHours bool, includeSession bool) map[string]any {
	meta := map[string]any{
		"instrumentId":  instrumentID,
		"source":        "bbgo:futu",
		"resolvedAt":    time.Now().UTC().Format(time.RFC3339Nano),
		"fromCache":     fromCache,
		"extendedHours": extendedHours,
	}
	if includeSession {
		session := "regular"
		if extendedHours {
			session = "all"
		}
		meta["session"] = session
	}
	return meta
}

func shouldAnnotateHistoricalKLineSession(market string, interval bbgotypes.Interval) bool {
	resolvedMarket, preferredPrefix, err := marketpkg.NormalizeMarketInput(market)
	return err == nil && resolvedMarket == "US" && preferredPrefix == "US" && interval.Duration() > 0 && interval.Duration() <= time.Hour
}

func (s *Server) futuExchange() *futu.Exchange {
	if s == nil || s.marketdataRuntime == nil {
		return nil
	}
	return s.marketdataRuntime.Exchange()
}

// --- Depth (Order Book) ---

func (s *Server) marketDepthResponseForInstrument(ctx context.Context, market string, symbol string, query marketDepthQuery) (map[string]any, error) {
	market = strings.ToUpper(strings.TrimSpace(market))
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	instrumentID := market + "." + symbol
	num := query.numOrDefault(10, 50)

	b, err := s.futuBrokerOrError()
	if err != nil {
		return nil, err
	}
	reader := b.MarketData()
	if reader == nil {
		return nil, fmt.Errorf("broker market data not available")
	}

	brokerResult, err := reader.QueryOrderBook(ctx, broker.OrderBookQuery{
		ReadQuery: brokerReadQuery(instrumentID),
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

func (s *Server) futuBroker() broker.Broker {
	if !s.futuIntegrationEnabled() {
		return nil
	}
	if s.brokers != nil {
		if b := s.brokers.Lookup(string(futu.Name)); b != nil {
			return b
		}
	}
	exchange := s.futuExchange()
	if exchange == nil {
		return nil
	}
	return futu.NewBrokerAdapter(exchange)
}

func brokerReadQuery(instrumentID string) broker.ReadQuery {
	parts := strings.SplitN(instrumentID, ".", 2)
	market := ""
	if len(parts) == 2 {
		market = parts[0]
	}
	return broker.ReadQuery{
		Market: market,
	}
}
