package futu

import (
	"context"
	"fmt"
	"strings"

	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
)

var futuCandleIntervalByPeriod = map[string]bbgotypes.Interval{
	"1m":  bbgotypes.Interval1m,
	"3m":  bbgotypes.Interval3m,
	"5m":  bbgotypes.Interval5m,
	"10m": bbgotypes.Interval10m,
	"15m": bbgotypes.Interval15m,
	"30m": bbgotypes.Interval30m,
	"1h":  bbgotypes.Interval1h,
	"1d":  bbgotypes.Interval1d,
	"1w":  bbgotypes.Interval1w,
	"1mo": bbgotypes.Interval1mo,
}

func futuCandlePeriods() []string {
	periods := make([]string, 0, len(futuCandleIntervalByPeriod))
	for _, period := range broker.SupportedHistoricalCandlePeriods() {
		interval, ok := futuCandleIntervalByPeriod[period]
		if !ok {
			continue
		}
		if _, _, err := futuKLineTypesFromInterval(interval); err == nil {
			periods = append(periods, period)
		}
	}
	return periods
}

func futuIntervalFromPeriod(period string) (bbgotypes.Interval, error) {
	normalized, err := broker.NormalizeCandlePeriod(period)
	if err != nil || normalized == "tick" {
		return "", fmt.Errorf("futu: unsupported kline period %q", period)
	}
	interval, ok := futuCandleIntervalByPeriod[normalized]
	if !ok {
		return "", fmt.Errorf("futu: unsupported kline period %q", period)
	}
	if _, _, err := futuKLineTypesFromInterval(interval); err != nil {
		return "", fmt.Errorf("futu: unsupported kline period %q", period)
	}
	return interval, nil
}

// --- broker.QuoteSubscriber implementation ---

func (a *futuAdapter) SubscribeQuotes(ctx context.Context, req broker.QuoteSubscribeRequest) error {
	requests, err := basicQotRequestsFromSymbols(req.Symbols)
	if err != nil {
		return err
	}
	return a.exchange.withClient(ctx, func(client *opend.Client) error {
		return a.exchange.ensureBasicQotPushSubscriptions(ctx, client, requests)
	})
}

// --- broker.UnlockTrader implementation ---

func (a *futuAdapter) UnlockTrade(ctx context.Context, req broker.UnlockTradeRequest) error {
	return a.exchange.withClient(ctx, func(client *opend.Client) error {
		return client.UnlockTrade(ctx, req.Unlock, req.PasswordMD5, nil)
	})
}

// --- internal helpers for the adapter methods ---

// securitiesFromSymbols parses a list of "MARKET.CODE" symbols into Security protobufs.
func securitiesFromSymbols(symbols []string) ([]*qotcommonpb.Security, error) {
	securities := make([]*qotcommonpb.Security, 0, len(symbols))
	for _, symbol := range symbols {
		security, _, err := futuSecurityFromSymbol(symbol)
		if err != nil {
			return nil, err
		}
		securities = append(securities, security)
	}
	return securities, nil
}

// securitySymbol converts a qotcommonpb.Security to "MARKET.CODE" string.
// Returns empty string if the security is nil or conversion fails.
func securitySymbol(security *qotcommonpb.Security) string {
	if security == nil {
		return ""
	}
	sym, err := futuSymbolFromSecurity(security)
	if err != nil {
		return ""
	}
	return sym
}

// futuKLTypeFromIntervalString converts a period string to a qotcommonpb.KLType.
func futuKLTypeFromIntervalString(period string) (qotcommonpb.KLType, error) {
	trimmed := strings.TrimSpace(period)
	// Special-case month "1M" before lowering, since ToLower("1M") == "1m"
	switch trimmed {
	case "1M", "month", "monthly":
		return qotcommonpb.KLType_KLType_Month, nil
	}
	switch strings.ToLower(trimmed) {
	case "1m", "1min":
		return qotcommonpb.KLType_KLType_1Min, nil
	case "3m", "3min":
		return qotcommonpb.KLType_KLType_3Min, nil
	case "5m", "5min":
		return qotcommonpb.KLType_KLType_5Min, nil
	case "10m", "10min":
		return qotcommonpb.KLType_KLType_10Min, nil
	case "15m", "15min":
		return qotcommonpb.KLType_KLType_15Min, nil
	case "30m", "30min":
		return qotcommonpb.KLType_KLType_30Min, nil
	case "60m", "60min", "1h", "1hour":
		return qotcommonpb.KLType_KLType_60Min, nil
	case "120m", "120min", "2h":
		return qotcommonpb.KLType_KLType_120Min, nil
	case "180m", "180min", "3h":
		return qotcommonpb.KLType_KLType_180Min, nil
	case "240m", "240min", "4h":
		return qotcommonpb.KLType_KLType_240Min, nil
	case "1d", "day", "daily":
		return qotcommonpb.KLType_KLType_Day, nil
	case "1w", "week", "weekly":
		return qotcommonpb.KLType_KLType_Week, nil
	case "1q", "quarter":
		return qotcommonpb.KLType_KLType_Quarter, nil
	case "1y", "year", "yearly":
		return qotcommonpb.KLType_KLType_Year, nil
	default:
		return 0, fmt.Errorf("futu: unsupported kline period %q", period)
	}
}

func int64AsFloat64Ptr(value *int64) *float64 {
	if value == nil {
		return nil
	}
	return new(float64(*value))
}

// Ensure adapter implements new interfaces at compile time.
var (
	_ broker.QuoteSubscriber     = (*futuAdapter)(nil)
	_ broker.UnlockTrader        = (*futuAdapter)(nil)
	_ broker.OrderBookSubscriber = (*futuAdapter)(nil)
)
