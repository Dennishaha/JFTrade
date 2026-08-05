package servercore

import (
	"context"
	"fmt"
	"strings"
	"time"

	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"

	futuintegration "github.com/jftrade/jftrade-main/internal/integration/futu"
	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/pkg/broker"
	marketpkg "github.com/jftrade/jftrade-main/pkg/market"
)

const marketSecurityDetailsStreamInterval = 3 * time.Second
const marketDepthStreamRefreshInterval = 15 * time.Second

func marketSecurityDetailsPathTail(path string) (string, string) {
	return pathTail(path, "/api/v1/market-data/securities/")
}

func (s *serverApplication) marketSecurityDetailsResponse(ctx context.Context, path string) (map[string]any, error) {
	market, symbol := marketSecurityDetailsPathTail(path)
	return s.marketSecurityDetailsResponseForInstrument(ctx, market, symbol)
}

func (s *serverApplication) marketSecurityDetailsResponseForInstrument(ctx context.Context, market string, symbol string) (map[string]any, error) {
	market = strings.ToUpper(strings.TrimSpace(market))
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	instrumentID := market + "." + symbol
	marketDataRuntime := s.runtimes.MarketData()
	if marketDataRuntime == nil || !s.futuIntegrationEnabled() {
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

func (s *serverApplication) marketSnapshotResponse(ctx context.Context, path string, query map[string][]string) (map[string]any, error) {
	market, symbol := pathTail(path, "/api/v1/market-data/snapshots/")
	decoded, err := decodeMarketSnapshotQuery(query)
	if err != nil {
		return nil, err
	}
	return s.marketSnapshotResponseForInstrument(ctx, market, symbol, decoded)
}

func (s *serverApplication) marketSnapshotResponseForInstrument(ctx context.Context, market string, symbol string, query marketSnapshotQuery) (map[string]any, error) {
	response, err := s.marketdataSvc.GetSnapshot(ctx, market, symbol, query.forceRefresh())
	return map[string]any(response), err
}

func (s *serverApplication) marketCandlesResponse(ctx context.Context, path string, query map[string][]string) (map[string]any, error) {
	market, symbol := pathTail(path, "/api/v1/market-data/candles/")
	decoded, err := decodeMarketCandlesQuery(query)
	if err != nil {
		return nil, err
	}
	return s.marketCandlesResponseForInstrument(ctx, market, symbol, decoded)
}

func (s *serverApplication) marketCandlesResponseForInstrument(ctx context.Context, market string, symbol string, query marketCandlesQuery) (map[string]any, error) {
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
	response, err := s.marketdataSvc.GetCandles(ctx, mdsrv.HistoricalCandlesQuery{
		Market: market, Symbol: symbol, Period: period, Limit: limit,
		FromTime: fromTime, ToTime: toTime,
		Sessions: query.Sessions, SessionsSpecified: query.SessionsSpecified,
	})
	return map[string]any(response), err
}

func (s *serverApplication) buildKLineCandlesResponse(ctx context.Context, market string, symbol string, instrumentID string, period string, limit int, query marketCandlesQuery) (map[string]any, error) {
	interval := bbgotypes.Interval(period)
	includeSession := shouldAnnotateHistoricalKLineSession(market, interval)
	availableSessions := []mdsrv.CandleSession{mdsrv.CandleSessionRegular}
	if includeSession {
		availableSessions = append(availableSessions, mdsrv.CandleSessionExtended, mdsrv.CandleSessionOvernight)
	}
	sessions, err := mdsrv.ResolveCandleSessions(
		query.Sessions,
		query.SessionsSpecified,
		availableSessions,
	)
	if err != nil {
		return nil, err
	}
	beginAt, endAt := kLineQueryWindow(query, interval.Duration(), limit)
	marketDataRuntime := s.runtimes.MarketData()
	if marketDataRuntime == nil || !s.futuIntegrationEnabled() {
		return nil, errFutuIntegrationNotEnabled
	}
	requestedFutuSessions := futuintegration.MarketSessionsForCandleSessions(sessions)
	klines, err := marketDataRuntime.QueryKLinesForSessions(ctx, instrumentID, interval, bbgotypes.KLineQueryOptions{Limit: limit, StartTime: &beginAt, EndTime: &endAt}, requestedFutuSessions)
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
			"volume": kline.Volume.String(),
			"at":     kline.StartTime.Time().UTC().Format(time.RFC3339Nano),
		}
		sessionGroup := mdsrv.CandleSessionRegular
		if includeSession {
			session, ok := marketDataRuntime.ResolveKLineSession(kline)
			if !ok {
				session = marketpkg.ClassifySession(instrumentID, kline.StartTime.Time().UTC())
			}
			if session != marketpkg.SessionUnknown && session != marketpkg.SessionClosed {
				candle["session"] = string(session)
			}
			sessionGroup = mdsrv.CandleSessionForLabel(string(session))
			if sessionGroup == "" {
				return nil, fmt.Errorf("unable to classify K-line session at %s", candle["at"])
			}
		}
		if !mdsrv.ContainsCandleSession(sessions, sessionGroup) {
			continue
		}
		candles = append(candles, candle)
	}
	if len(candles) > limit {
		candles = candles[len(candles)-limit:]
	}
	extendedHours := mdsrv.ContainsCandleSession(sessions, mdsrv.CandleSessionExtended) ||
		mdsrv.ContainsCandleSession(sessions, mdsrv.CandleSessionOvernight)
	return mdsrv.CandlesResponseDTO{
		Instrument: mdsrv.InstrumentDTO{Market: market, Symbol: symbol, InstrumentID: instrumentID},
		Period:     period, Limit: limit, Candles: candles, Source: "bbgo:futu",
		ResolvedAt: time.Now().UTC().Format(time.RFC3339Nano), ExtendedHours: extendedHours,
		IncludeSession: includeSession, Sessions: sessions,
	}.JSON(), nil
}

func shouldAnnotateHistoricalKLineSession(market string, interval bbgotypes.Interval) bool {
	resolvedMarket, preferredPrefix, err := marketpkg.NormalizeMarketInput(market)
	return err == nil && resolvedMarket == "US" && preferredPrefix == "US" && interval.Duration() > 0 && interval.Duration() <= time.Hour
}

func (s *serverApplication) futuExchange() futuintegration.RuntimeExchange {
	return s.futuCoordinator().Exchange()
}

// --- Depth (Order Book) ---

func (s *serverApplication) marketDepthResponseForInstrument(ctx context.Context, market string, symbol string, query marketDepthQuery) (map[string]any, error) {
	market = strings.ToUpper(strings.TrimSpace(market))
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	instrumentID := market + "." + symbol
	num := query.numOrDefault(10, 50)

	marketDataRuntime := s.runtimes.MarketData()
	if marketDataRuntime == nil || !s.futuIntegrationEnabled() {
		return nil, errFutuIntegrationNotEnabled
	}

	brokerResult, err := marketDataRuntime.QueryOrderBook(ctx, broker.OrderBookQuery{
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

func (s *serverApplication) futuBroker() broker.Broker {
	return s.futuCoordinator().Broker()
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
