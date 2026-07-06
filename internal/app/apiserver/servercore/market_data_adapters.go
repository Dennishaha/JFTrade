package servercore

import (
	"context"
	"strings"
	"time"

	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/pkg/market"
)

// newMarketdataProvider 创建一个 marketdata.Provider 实现，通过闭包委托到 Server
// 尚未迁出的外部行情能力。
func newMarketdataProvider(s *Server) mdsrv.Provider {
	return &marketdataProvider{
		descriptor:          marketdataProviderDescriptor,
		getMarkets:          marketdataProviderMarkets,
		normalizeInstrument: marketdataProviderNormalizeInstrument,
		getSecurityDetails: func(ctx context.Context, market, symbol string) (mdsrv.SecurityDetails, error) {
			return s.marketSecurityDetailsResponseForInstrument(ctx, market, symbol)
		},

		querySnapshot: func(ctx context.Context, instrumentID string) (*mdsrv.Tick, error) {
			return s.marketdataRuntime.QuerySnapshot(ctx, instrumentID)
		},

		queryTicker: func(ctx context.Context, instrumentID string) (*mdsrv.Tick, error) {
			return s.marketdataRuntime.QueryTicker(ctx, instrumentID)
		},
		getHistoricalCandles: s.marketdataProviderHistoricalCandles,
		getDepth: func(ctx context.Context, market, symbol string, num int) (mdsrv.DepthResponse, error) {
			// Always set Num so that Server.numOrDefault handles clamping (<=0→1, >50→50).
			query := marketDepthQuery{}
			query.Num = optionalIntValue{Value: num, Set: true, Valid: true}
			return s.marketDepthResponseForInstrument(ctx, market, symbol, query)
		},

		health: func(ctx context.Context) (mdsrv.HealthStatus, error) {
			return mdsrv.HealthStatus{}, nil
		},
	}
}

func marketdataProviderDescriptor(context.Context) (mdsrv.ProviderDescriptor, error) {
	dtos := marketProfileDTOs()
	supportedMarkets := make([]string, 0, len(dtos))
	for _, profile := range dtos {
		supportedMarkets = append(supportedMarkets, strings.ToUpper(strings.TrimSpace(profile.Code)))
	}
	return mdsrv.ProviderDescriptor{
		ProviderID:       "futu-opend",
		DisplayName:      "Futu OpenD",
		BrokerID:         "futu",
		Source:           "bbgo:futu",
		DefaultMarket:    "HK",
		SupportedMarkets: supportedMarkets,
		Transports:       []string{"opend-tcp", "push-stream", "snapshot-poll-fallback"},
		Capabilities: mdsrv.ProviderCapabilities{
			Snapshots:         true,
			StreamingQuotes:   true,
			StreamingDepth:    true,
			HistoricalCandles: true,
			TickCandles:       true,
			OrderBookDepth:    true,
			InstrumentSearch:  true,
			ExtendedHours:     true,
			CandleIntervals:   []string{"tick", "1m", "5m", "15m", "30m", "1h", "1d", "1w", "1mo"},
			OrderBookLevels:   []int{1, 5, 10, 25, 50},
			Sessions:          []string{"RTH", "ETH", "ALL", "OVERNIGHT"},
		},
		Constraints: mdsrv.ProviderConstraints{
			RequiresOpenD:           true,
			RequiresMarketDataRight: true,
			UsesSubscriptionQuota:   true,
		},
		Notes: []string{
			"Futu-first provider; data entitlement and subscription quota are enforced by Futu OpenD.",
			"Historical candles and real-time pushes can diverge during extended sessions; UI surfaces observed timestamps and transport mode.",
		},
	}, nil
}

func marketdataProviderMarkets(context.Context) ([]mdsrv.MarketProfile, error) {
	dtos := marketProfileDTOs()
	profiles := make([]mdsrv.MarketProfile, 0, len(dtos))
	for _, d := range dtos {
		profiles = append(profiles, mdsrv.MarketProfile{
			"code":                   d.Code,
			"resolvedMarket":         d.ResolvedMarket,
			"preferredPrefix":        d.PreferredPrefix,
			"displayName":            d.DisplayName,
			"quoteCurrency":          d.QuoteCurrency,
			"timezone":               d.Timezone,
			"supportsExtendedHours":  d.SupportsExtendedHours,
			"requiresExchangePrefix": d.RequiresExchangePrefix,
			"aliases":                d.Aliases,
			"regularSessions":        d.RegularSessions,
			"precision":              d.Precision,
			"tickSize":               d.TickSize,
		})
	}
	return profiles, nil
}

func marketdataProviderNormalizeInstrument(_ context.Context, input map[string]any) (map[string]any, error) {
	marketStr := jftradeOptionalTypeAssertion[string](input["market"])
	symbolStr := jftradeOptionalTypeAssertion[string](input["symbol"])
	codeStr := jftradeOptionalTypeAssertion[string](input["code"])
	instrumentIDStr := jftradeOptionalTypeAssertion[string](input["instrumentId"])

	instrument, err := market.ParseInstrument(market.InstrumentInput{
		Market:       marketStr,
		Symbol:       symbolStr,
		Code:         codeStr,
		InstrumentID: instrumentIDStr,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"market":         instrument.Market,
		"prefix":         instrument.Prefix,
		"code":           instrument.Code,
		"symbol":         instrument.Symbol,
		"instrumentId":   instrument.Symbol,
		"resolvedMarket": instrument.Market,
	}, nil
}

func (s *Server) marketdataProviderHistoricalCandles(ctx context.Context, market string, symbol string, period string, limit int, fromTime string, toTime string) (mdsrv.CandlesResponse, error) {
	query := marketdataProviderCandlesQuery(period, limit, fromTime, toTime)
	normalizedPeriod := query.normalizedPeriod()
	if normalizedPeriod == "tick" {
		normalizedPeriod = "1m"
	}
	resolvedMarket := strings.ToUpper(strings.TrimSpace(market))
	resolvedSymbol := strings.ToUpper(strings.TrimSpace(symbol))
	instrumentID := resolvedMarket + "." + resolvedSymbol
	resp, err := s.buildKLineCandlesResponse(ctx, resolvedMarket, resolvedSymbol, instrumentID, normalizedPeriod, query.limitOrDefault(200, 1000), query)
	if err != nil {
		return nil, err
	}
	return mdsrv.CandlesResponse(resp), nil
}

func marketdataProviderCandlesQuery(period string, limit int, fromTime string, toTime string) marketCandlesQuery {
	query := marketCandlesQuery{}
	if period != "" {
		query.Period = candlePeriodValue(period)
	}
	if limit > 0 {
		query.Limit = optionalIntValue{Value: limit, Set: true, Valid: true}
	}
	query.FromTime = marketdataProviderOptionalTime(fromTime)
	query.ToTime = marketdataProviderOptionalTime(toTime)
	return query
}

func marketdataProviderOptionalTime(value string) optionalTimeValue {
	if value == "" {
		return optionalTimeValue{}
	}
	if t, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return optionalTimeValue{Time: t}
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return optionalTimeValue{Time: t}
	}
	return optionalTimeValue{}
}

// marketdataProvider 闭包式 Provider 实现——每个方法通过闭包委托到 Server。
type marketdataProvider struct {
	descriptor           func(context.Context) (mdsrv.ProviderDescriptor, error)
	getMarkets           func(context.Context) ([]mdsrv.MarketProfile, error)
	normalizeInstrument  func(context.Context, map[string]any) (map[string]any, error)
	getSecurityDetails   func(context.Context, string, string) (mdsrv.SecurityDetails, error)
	querySnapshot        func(context.Context, string) (*mdsrv.Tick, error)
	queryTicker          func(context.Context, string) (*mdsrv.Tick, error)
	getHistoricalCandles func(context.Context, string, string, string, int, string, string) (mdsrv.CandlesResponse, error)
	getDepth             func(context.Context, string, string, int) (mdsrv.DepthResponse, error)
	health               func(context.Context) (mdsrv.HealthStatus, error)
}

// compile-time interface check
var _ mdsrv.Provider = (*marketdataProvider)(nil)

func (p *marketdataProvider) Descriptor(ctx context.Context) (mdsrv.ProviderDescriptor, error) {
	return p.descriptor(ctx)
}

func (p *marketdataProvider) GetMarkets(ctx context.Context) ([]mdsrv.MarketProfile, error) {
	return p.getMarkets(ctx)
}

func (p *marketdataProvider) NormalizeInstrument(ctx context.Context, input map[string]any) (map[string]any, error) {
	return p.normalizeInstrument(ctx, input)
}

func (p *marketdataProvider) GetSecurityDetails(ctx context.Context, market, symbol string) (mdsrv.SecurityDetails, error) {
	return p.getSecurityDetails(ctx, market, symbol)
}

func (p *marketdataProvider) QuerySnapshot(ctx context.Context, instrumentID string) (*mdsrv.Tick, error) {
	return p.querySnapshot(ctx, instrumentID)
}

func (p *marketdataProvider) QueryTicker(ctx context.Context, instrumentID string) (*mdsrv.Tick, error) {
	return p.queryTicker(ctx, instrumentID)
}

func (p *marketdataProvider) GetHistoricalCandles(ctx context.Context, market, symbol, period string, limit int, fromTime, toTime string) (mdsrv.CandlesResponse, error) {
	return p.getHistoricalCandles(ctx, market, symbol, period, limit, fromTime, toTime)
}

func (p *marketdataProvider) GetDepth(ctx context.Context, market, symbol string, num int) (mdsrv.DepthResponse, error) {
	return p.getDepth(ctx, market, symbol, num)
}

func (p *marketdataProvider) Health(ctx context.Context) (mdsrv.HealthStatus, error) {
	return p.health(ctx)
}
