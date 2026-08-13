package marketdataapp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/market"
)

// ProviderOptions supplies the application callbacks behind a market-data
// provider without exposing the composition root to the business service.
type ProviderOptions struct {
	Descriptor          func(context.Context) (mdsrv.ProviderDescriptor, error)
	Markets             func(context.Context) ([]mdsrv.MarketProfile, error)
	NormalizeInstrument func(context.Context, map[string]any) (map[string]any, error)
	SecurityDetails     func(context.Context, string, string) (mdsrv.SecurityDetails, error)
	LookupInstrument    func(context.Context, string, string) ([]mdsrv.InstrumentCandidate, error)
	SearchInstruments   func(context.Context, string, int) ([]mdsrv.InstrumentCandidate, error)
	QuerySnapshot       func(context.Context, string) (*mdsrv.Tick, error)
	QueryTicker         func(context.Context, string) (*mdsrv.Tick, error)
	HistoricalCandles   func(context.Context, mdsrv.HistoricalCandlesQuery) (mdsrv.CandlesResponse, error)
	Depth               func(context.Context, string, string, int) (mdsrv.DepthResponse, error)
	Health              func(context.Context) (mdsrv.HealthStatus, error)
}

type provider struct {
	options ProviderOptions
}

var _ mdsrv.Provider = (*provider)(nil)

func NewProvider(options ProviderOptions) mdsrv.Provider {
	return &provider{options: options}
}

func (p *provider) Descriptor(ctx context.Context) (mdsrv.ProviderDescriptor, error) {
	return p.options.Descriptor(ctx)
}

func (p *provider) GetMarkets(ctx context.Context) ([]mdsrv.MarketProfile, error) {
	return p.options.Markets(ctx)
}

func (p *provider) NormalizeInstrument(ctx context.Context, input map[string]any) (map[string]any, error) {
	return p.options.NormalizeInstrument(ctx, input)
}

func (p *provider) GetSecurityDetails(ctx context.Context, marketCode, symbol string) (mdsrv.SecurityDetails, error) {
	return p.options.SecurityDetails(ctx, marketCode, symbol)
}

func (p *provider) LookupInstrument(ctx context.Context, marketCode, code string) ([]mdsrv.InstrumentCandidate, error) {
	if p == nil || p.options.LookupInstrument == nil {
		return nil, fmt.Errorf("market-data exact instrument lookup is unavailable")
	}
	return p.options.LookupInstrument(ctx, marketCode, code)
}

func (p *provider) SearchInstruments(ctx context.Context, query string, limit int) ([]mdsrv.InstrumentCandidate, error) {
	if p == nil || p.options.SearchInstruments == nil {
		return nil, fmt.Errorf("market-data instrument search is unavailable")
	}
	return p.options.SearchInstruments(ctx, query, limit)
}

func (p *provider) QuerySnapshot(ctx context.Context, instrumentID string) (*mdsrv.Tick, error) {
	return p.options.QuerySnapshot(ctx, instrumentID)
}

func (p *provider) QueryTicker(ctx context.Context, instrumentID string) (*mdsrv.Tick, error) {
	return p.options.QueryTicker(ctx, instrumentID)
}

func (p *provider) GetHistoricalCandles(ctx context.Context, query mdsrv.HistoricalCandlesQuery) (mdsrv.CandlesResponse, error) {
	return p.options.HistoricalCandles(ctx, query)
}

func (p *provider) GetDepth(ctx context.Context, marketCode, symbol string, num int) (mdsrv.DepthResponse, error) {
	result, err := p.options.Depth(ctx, marketCode, symbol, num)
	if err != nil {
		if errors.Is(err, mdsrv.ErrSubscriptionRequired) {
			return nil, mdsrv.NewSubscriptionRequiredError("ORDER_BOOK", marketCode, symbol, "")
		}
		return nil, err
	}
	return result, nil
}

func (p *provider) Health(ctx context.Context) (mdsrv.HealthStatus, error) {
	return p.options.Health(ctx)
}

func NormalizeInstrument(_ context.Context, input map[string]any) (map[string]any, error) {
	marketCode, _ := input["market"].(string)
	symbol, _ := input["symbol"].(string)
	code, _ := input["code"].(string)
	instrumentID, _ := input["instrumentId"].(string)
	instrument, err := market.ParseInstrument(market.InstrumentInput{
		Market: marketCode, Symbol: symbol, Code: code, InstrumentID: instrumentID,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"market": instrument.Market, "prefix": instrument.Prefix, "code": instrument.Code,
		"symbol": instrument.Symbol, "instrumentId": instrument.Symbol, "resolvedMarket": instrument.Market,
	}, nil
}

func LookupInstrument(
	ctx context.Context,
	selected broker.Broker,
	marketCode string,
	code string,
	source string,
) ([]mdsrv.InstrumentCandidate, error) {
	instrument, err := market.ParseInstrument(market.InstrumentInput{Market: marketCode, Code: code})
	if err != nil {
		return nil, err
	}
	reader, err := marketDataReader(selected)
	if err != nil {
		return nil, err
	}
	staticInfo, err := reader.QuerySecurityInfo(ctx, broker.SecurityInfoQuery{
		ReadQuery: broker.ReadQuery{Market: instrument.Prefix},
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
			Market: parsed.Prefix, ResolvedMarket: parsed.Market, InstrumentID: parsed.Symbol,
			Code: parsed.Code, Symbol: parsed.Code, Source: source,
			Selectable: isSelectableInstrumentMarketCode(parsed.Prefix),
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

func SearchInstruments(
	ctx context.Context,
	selected broker.Broker,
	query string,
	limit int,
	source string,
) ([]mdsrv.InstrumentCandidate, error) {
	reader, err := marketDataReader(selected)
	if err != nil {
		return nil, err
	}
	snapshot, err := reader.QuerySecuritySearch(ctx, broker.SecuritySearchQuery{
		Keyword: strings.TrimSpace(query), Limit: int32(limit),
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
		marketCode, code := BrokerSearchInstrumentParts(marketCode, symbol)
		if marketCode == "" || code == "" {
			continue
		}
		resolvedMarket := marketCode
		if marketCode == "SH" || marketCode == "SZ" {
			resolvedMarket = "CN"
		}
		selectable := isSelectableInstrumentMarketCode(marketCode)
		candidate := mdsrv.InstrumentCandidate{
			Market: marketCode, ResolvedMarket: resolvedMarket, InstrumentID: marketCode + "." + code,
			Code: code, Symbol: code, Name: strings.TrimSpace(entry.Name),
			SecurityType: strings.TrimSpace(entry.SecurityType), Source: source,
			IsWatched: entry.IsWatched, Selectable: selectable,
		}
		if !selectable {
			candidate.UnavailableReason = fmt.Sprintf("当前版本暂不支持 %s 市场", marketCode)
		}
		candidates = append(candidates, candidate)
	}
	return candidates, nil
}

func BrokerSearchInstrumentParts(marketCode, symbol string) (string, string) {
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

func HistoricalCandles(
	ctx context.Context,
	selected broker.Broker,
	brokerID string,
	request mdsrv.HistoricalCandlesQuery,
	includeSession bool,
	source string,
) (mdsrv.CandlesResponse, error) {
	period := strings.ToLower(strings.TrimSpace(request.Period))
	if period == "tick" {
		period = "1m"
	}
	limit := request.Limit
	if limit < 1 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	instrument, err := market.ParseInstrument(market.InstrumentInput{
		Market: request.Market,
		Symbol: request.Symbol,
	})
	if err != nil {
		return nil, err
	}
	marketCode := instrument.Market
	symbol := instrument.Code
	instrumentID := instrument.Symbol
	sessions, err := resolveCandleSessions(request, includeSession)
	if err != nil {
		return nil, err
	}
	reader, err := marketDataReader(selected)
	if err != nil {
		return nil, err
	}
	snapshot, err := reader.QueryKLines(ctx, broker.KLineQuery{
		ReadQuery: broker.ReadQuery{BrokerID: brokerID, Market: marketCode},
		Symbol:    instrumentID, Period: period, Adjustment: request.Adjustment,
		FromTime: request.FromTime, ToTime: request.ToTime,
		BeforeTime: request.BeforeTime, Limit: int32(limit), Sessions: mdsrv.CandleSessionStrings(sessions),
	})
	if err != nil {
		if errors.Is(err, mdsrv.ErrSubscriptionRequired) {
			return nil, mdsrv.NewSubscriptionRequiredError("KLINE", marketCode, symbol, period)
		}
		return nil, err
	}
	return mdsrv.BrokerKLineCandlesResponse(
		marketCode, symbol, instrumentID, period, limit, request, sessions, includeSession, snapshot, source,
	)
}

func resolveCandleSessions(
	request mdsrv.HistoricalCandlesQuery,
	includeSession bool,
) ([]mdsrv.CandleSession, error) {
	available := []mdsrv.CandleSession{mdsrv.CandleSessionRegular}
	if includeSession {
		available = append(available, mdsrv.CandleSessionExtended, mdsrv.CandleSessionOvernight)
	}
	return mdsrv.ResolveCandleSessions(request.Sessions, request.SessionsSpecified, available)
}

func marketDataReader(selected broker.Broker) (broker.MarketDataReader, error) {
	if selected == nil || selected.MarketData() == nil {
		return nil, fmt.Errorf("broker market data not available")
	}
	return selected.MarketData(), nil
}
