package servercore

import (
	"context"
	"fmt"
	"strings"
	"time"

	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/pkg/broker"
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
		lookupInstrument:  s.marketdataProviderLookupInstrument,
		searchInstruments: s.marketdataProviderSearchInstruments,

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
	dtos := userMarketProfileDTOs()
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

func (s *Server) marketdataProviderLookupInstrument(ctx context.Context, marketCode, code string) ([]mdsrv.InstrumentCandidate, error) {
	instrument, err := market.ParseInstrument(market.InstrumentInput{Market: marketCode, Code: code})
	if err != nil {
		return nil, err
	}
	b, err := s.futuBrokerOrError()
	if err != nil {
		return nil, err
	}
	reader := b.MarketData()
	if reader == nil {
		return nil, fmt.Errorf("broker market data not available")
	}
	staticInfo, err := reader.QuerySecurityInfo(ctx, broker.SecurityInfoQuery{
		ReadQuery: brokerReadQuery(instrument.Symbol),
		Symbols:   []string{instrument.Symbol},
	})
	if err != nil {
		return nil, err
	}
	if staticInfo == nil {
		return []mdsrv.InstrumentCandidate{}, nil
	}

	candidates := make([]mdsrv.InstrumentCandidate, 0, len(staticInfo.Securities))
	for _, security := range staticInfo.Securities {
		parsed, parseErr := market.ParseQualifiedInstrumentSymbol(security.Symbol)
		if parseErr != nil || parsed.Prefix != instrument.Prefix || !strings.EqualFold(parsed.Code, instrument.Code) {
			continue
		}
		candidate := mdsrv.InstrumentCandidate{
			Market:         parsed.Prefix,
			ResolvedMarket: parsed.Market,
			InstrumentID:   parsed.Symbol,
			Code:           parsed.Code,
			Symbol:         parsed.Code,
			Source:         "bbgo:futu",
			Selectable:     isSelectableInstrumentMarketCode(parsed.Prefix),
		}
		if security.Name != nil {
			candidate.Name = strings.TrimSpace(*security.Name)
		}
		if security.SecurityType != nil {
			candidate.SecurityType = strings.TrimSpace(*security.SecurityType)
		}
		if security.LotSize != nil && *security.LotSize > 0 {
			candidate.LotSize = *security.LotSize
		}
		candidates = append(candidates, candidate)
	}
	return candidates, nil
}

func (s *Server) marketdataProviderSearchInstruments(ctx context.Context, query string, limit int) ([]mdsrv.InstrumentCandidate, error) {
	b, err := s.futuBrokerOrError()
	if err != nil {
		return nil, err
	}
	reader := b.MarketData()
	if reader == nil {
		return nil, fmt.Errorf("broker market data not available")
	}
	snapshot, err := reader.QuerySecuritySearch(ctx, broker.SecuritySearchQuery{
		Keyword: strings.TrimSpace(query),
		Limit:   int32(limit),
	})
	if err != nil {
		return nil, err
	}
	if snapshot == nil {
		return []mdsrv.InstrumentCandidate{}, nil
	}

	candidates := make([]mdsrv.InstrumentCandidate, 0, len(snapshot.Entries))
	for _, entry := range snapshot.Entries {
		marketCode := strings.ToUpper(strings.TrimSpace(entry.Market))
		symbol := strings.ToUpper(strings.TrimSpace(entry.Symbol))
		marketCode, code := brokerSearchInstrumentParts(marketCode, symbol)
		if marketCode == "" || code == "" {
			continue
		}
		resolvedMarket := marketCode
		if marketCode == "SH" || marketCode == "SZ" {
			resolvedMarket = "CN"
		}
		selectable := isSelectableInstrumentMarketCode(marketCode)
		candidate := mdsrv.InstrumentCandidate{
			Market:         marketCode,
			ResolvedMarket: resolvedMarket,
			InstrumentID:   marketCode + "." + code,
			Code:           code,
			Symbol:         code,
			Name:           strings.TrimSpace(entry.Name),
			SecurityType:   strings.TrimSpace(entry.SecurityType),
			Source:         "bbgo:futu-search",
			IsWatched:      entry.IsWatched,
			Selectable:     selectable,
		}
		if !selectable {
			candidate.UnavailableReason = fmt.Sprintf("当前版本暂不支持 %s 市场", marketCode)
		}
		candidates = append(candidates, candidate)
	}
	return candidates, nil
}

func brokerSearchInstrumentParts(marketCode, symbol string) (string, string) {
	if separator := strings.Index(symbol, "."); separator > 0 {
		prefix := canonicalBrokerSearchMarketPrefix(symbol[:separator])
		if marketCode == "" {
			marketCode = prefix
		}
		if prefix != "" && prefix == marketCode {
			return marketCode, strings.TrimSpace(symbol[separator+1:])
		}
	}
	return marketCode, symbol
}

func canonicalBrokerSearchMarketPrefix(value string) string {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	switch normalized {
	case "CNSH":
		return "SH"
	case "CNSZ":
		return "SZ"
	case "HKFUTURE", "HK_FUTURES":
		return "HK_FUTURE"
	case "CC":
		return "CRYPTO"
	case "HK", "US", "SH", "SZ", "SG", "JP", "AU", "MY", "CA", "FX", "CRYPTO", "HK_FUTURE", "UNKNOWN":
		return normalized
	default:
		return ""
	}
}

func isSelectableInstrumentMarketCode(marketCode string) bool {
	switch strings.ToUpper(strings.TrimSpace(marketCode)) {
	case "HK", "US", "SH", "SZ":
		return true
	default:
		return false
	}
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
	return optionalTimeValue{}
}

// marketdataProvider 闭包式 Provider 实现——每个方法通过闭包委托到 Server。
type marketdataProvider struct {
	descriptor           func(context.Context) (mdsrv.ProviderDescriptor, error)
	getMarkets           func(context.Context) ([]mdsrv.MarketProfile, error)
	normalizeInstrument  func(context.Context, map[string]any) (map[string]any, error)
	getSecurityDetails   func(context.Context, string, string) (mdsrv.SecurityDetails, error)
	lookupInstrument     func(context.Context, string, string) ([]mdsrv.InstrumentCandidate, error)
	searchInstruments    func(context.Context, string, int) ([]mdsrv.InstrumentCandidate, error)
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

func (p *marketdataProvider) LookupInstrument(ctx context.Context, market, code string) ([]mdsrv.InstrumentCandidate, error) {
	if p.lookupInstrument == nil {
		return nil, fmt.Errorf("market-data exact instrument lookup is unavailable")
	}
	return p.lookupInstrument(ctx, market, code)
}

func (p *marketdataProvider) SearchInstruments(ctx context.Context, query string, limit int) ([]mdsrv.InstrumentCandidate, error) {
	if p.searchInstruments == nil {
		return nil, fmt.Errorf("market-data instrument search is unavailable")
	}
	return p.searchInstruments(ctx, query, limit)
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
