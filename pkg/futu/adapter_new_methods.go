package futu

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
)

// --- broker.QuoteSubscriber implementation ---

func (a *futuAdapter) SubscribeQuotes(ctx context.Context, req broker.QuoteSubscribeRequest) error {
	return a.exchange.withClient(ctx, func(client *opend.Client) error {
		securities, err := securitiesFromSymbols(req.Symbols)
		if err != nil {
			return err
		}
		return client.SubscribeQuotes(ctx, opend.QuoteSubRequest{
			Securities:  securities,
			SubTypes:    []qotcommonpb.SubType{qotcommonpb.SubType_SubType_Basic},
			IsSubscribe: true,
			IsRegPush:   proto.Bool(true),
		})
	})
}

// --- broker.UnlockTrader implementation ---

func (a *futuAdapter) UnlockTrade(ctx context.Context, req broker.UnlockTradeRequest) error {
	return a.exchange.withClient(ctx, func(client *opend.Client) error {
		return client.UnlockTrade(ctx, req.Unlock, req.PasswordMD5, nil)
	})
}

// --- broker.MarketDataReader new methods ---

func (r *futuMarketDataReader) QueryQuote(ctx context.Context, query broker.QuoteQuery) (*broker.QuoteSnapshot, error) {
	if len(query.Symbols) == 0 {
		return nil, fmt.Errorf("futu: QueryQuote requires at least one symbol")
	}
	var result *broker.QuoteSnapshot
	if err := r.exchange.withClient(ctx, func(client *opend.Client) error {
		securities, err := securitiesFromSymbols(query.Symbols)
		if err != nil {
			return err
		}
		qots, err := client.GetBasicQot(ctx, securities)
		if err != nil {
			return err
		}
		snapshot := &broker.QuoteSnapshot{AccountID: query.AccountID}
		for _, qot := range qots {
			if qot == nil {
				continue
			}
			sym := securitySymbol(qot.GetSecurity())
			item := broker.QuoteItem{
				Symbol:     sym,
				SymbolName: cloneStringPtr(qot.Name),
				LastPrice:  qot.GetCurPrice(),
				OpenPrice:  cloneFloat64Ptr(qot.OpenPrice),
				HighPrice:  cloneFloat64Ptr(qot.HighPrice),
				LowPrice:   cloneFloat64Ptr(qot.LowPrice),
				Volume:     float64(qot.GetVolume()),
				Turnover:   cloneFloat64Ptr(qot.Turnover),
			}
			if snapshot.Symbol == "" {
				snapshot.Symbol = item.Symbol
				snapshot.SymbolName = item.SymbolName
				snapshot.LastPrice = item.LastPrice
				snapshot.OpenPrice = item.OpenPrice
				snapshot.HighPrice = item.HighPrice
				snapshot.LowPrice = item.LowPrice
				snapshot.Volume = item.Volume
				snapshot.Turnover = item.Turnover
			}
			snapshot.Quotes = append(snapshot.Quotes, item)
		}
		result = snapshot
		return nil
	}); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *futuMarketDataReader) QueryKLines(ctx context.Context, query broker.KLineQuery) (*broker.KLineSnapshot, error) {
	if query.Symbol == "" {
		return nil, fmt.Errorf("futu: QueryKLines requires a symbol")
	}
	var result *broker.KLineSnapshot
	if err := r.exchange.withClient(ctx, func(client *opend.Client) error {
		security, _, err := futuSecurityFromSymbol(query.Symbol)
		if err != nil {
			return err
		}
		klType, err := futuKLTypeFromIntervalString(query.Period)
		if err != nil {
			return err
		}
		fromTime := strings.TrimSpace(query.FromTime)
		toTime := strings.TrimSpace(query.ToTime)
		if fromTime == "" {
			fromTime = "2020-01-01"
		}
		if toTime == "" {
			toTime = "2099-12-31"
		}
		historyReq := opend.HistoryKLineRequest{
			Security:  security,
			RehabType: qotcommonpb.RehabType_RehabType_Forward,
			KLType:    klType,
			BeginTime: fromTime,
			EndTime:   toTime,
		}
		if query.Limit > 0 {
			historyReq.MaxAckKLNum = proto.Int32(query.Limit)
		}
		historyResult, err := client.RequestHistoryKL(ctx, historyReq)
		if err != nil {
			return err
		}
		snapshot := &broker.KLineSnapshot{
			AccountID: query.AccountID,
			Symbol:    query.Symbol,
			Period:    query.Period,
		}
		for _, kl := range historyResult.KLines {
			snapshot.KLines = append(snapshot.KLines, broker.KLineItem{
				Time:       kl.GetTime(),
				Open:       cloneFloat64Ptr(kl.OpenPrice),
				Close:      cloneFloat64Ptr(kl.ClosePrice),
				High:       cloneFloat64Ptr(kl.HighPrice),
				Low:        cloneFloat64Ptr(kl.LowPrice),
				Volume:     int64AsFloat64Ptr(kl.Volume),
				Turnover:   cloneFloat64Ptr(kl.Turnover),
				ChangeRate: cloneFloat64Ptr(kl.ChangeRate),
			})
		}
		result = snapshot
		return nil
	}); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *futuMarketDataReader) QuerySecurityInfo(ctx context.Context, query broker.SecurityInfoQuery) (*broker.SecurityInfoSnapshot, error) {
	if len(query.Symbols) == 0 {
		return nil, fmt.Errorf("futu: QuerySecurityInfo requires at least one symbol")
	}
	var result *broker.SecurityInfoSnapshot
	if err := r.exchange.withClient(ctx, func(client *opend.Client) error {
		securities, err := securitiesFromSymbols(query.Symbols)
		if err != nil {
			return err
		}
		staticInfos, err := client.GetStaticInfo(ctx, securities)
		if err != nil {
			return err
		}
		snapshot := &broker.SecurityInfoSnapshot{AccountID: query.AccountID}
		for _, info := range staticInfos {
			if info == nil || info.GetBasic() == nil {
				continue
			}
			basic := info.GetBasic()
			secTypeName := enumName(basic.GetSecType(), qotcommonpb.SecurityType_name)
			item := broker.SecurityInfoItem{
				Symbol:       securitySymbol(basic.GetSecurity()),
				Name:         cloneStringPtr(basic.Name),
				SecurityType: &secTypeName,
				LotSize:      cloneInt32Ptr(basic.LotSize),
				ListTime:     cloneStringPtr(basic.ListTime),
				IsDelisted:   cloneBoolPtr(basic.Delisting),
			}
			snapshot.Securities = append(snapshot.Securities, item)
		}
		result = snapshot
		return nil
	}); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *futuMarketDataReader) QuerySecuritySnapshot(ctx context.Context, query broker.SecuritySnapshotQuery) (*broker.SecuritySnapshotResult, error) {
	if len(query.Symbols) == 0 {
		return nil, fmt.Errorf("futu: QuerySecuritySnapshot requires at least one symbol")
	}
	var result *broker.SecuritySnapshotResult
	if err := r.exchange.withClient(ctx, func(client *opend.Client) error {
		securities, err := securitiesFromSymbols(query.Symbols)
		if err != nil {
			return err
		}
		snapshots, err := client.GetSecuritySnapshot(ctx, securities)
		if err != nil {
			return err
		}
		res := &broker.SecuritySnapshotResult{AccountID: query.AccountID}
		for _, snap := range snapshots {
			if snap == nil || snap.Basic == nil {
				continue
			}
			basic := snap.Basic
			secTypeName := enumName(basic.GetType(), qotcommonpb.SecurityType_name)
			item := broker.SecuritySnapshotItem{
				Symbol:       securitySymbol(basic.GetSecurity()),
				Name:         cloneStringPtr(basic.Name),
				SecurityType: &secTypeName,
				IsSuspended:  cloneBoolPtr(basic.IsSuspend),
				LastPrice:    cloneFloat64Ptr(basic.CurPrice),
				OpenPrice:    cloneFloat64Ptr(basic.OpenPrice),
				HighPrice:    cloneFloat64Ptr(basic.HighPrice),
				LowPrice:     cloneFloat64Ptr(basic.LowPrice),
				Volume:       int64AsFloat64Ptr(basic.Volume),
				Turnover:     cloneFloat64Ptr(basic.Turnover),
				LotSize:      cloneInt32Ptr(basic.LotSize),
			}
			if snap.EquityExData != nil {
				item.PERate = cloneFloat64Ptr(snap.EquityExData.PeRate)
				item.PBRate = cloneFloat64Ptr(snap.EquityExData.PbRate)
			}
			res.Snapshots = append(res.Snapshots, item)
		}
		result = res
		return nil
	}); err != nil {
		return nil, err
	}
	return result, nil
}

// --- internal helpers for the new adapter methods ---

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
	v := float64(*value)
	return &v
}

// Ensure adapter implements new interfaces at compile time.
var (
	_ broker.QuoteSubscriber = (*futuAdapter)(nil)
	_ broker.UnlockTrader   = (*futuAdapter)(nil)
)
