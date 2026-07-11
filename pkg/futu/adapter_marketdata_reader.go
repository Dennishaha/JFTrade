package futu

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	"github.com/jftrade/jftrade-main/pkg/market"
)

// --- broker.MarketDataReader extended methods (futuMarketDataReader) ---

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
			historyReq.MaxAckKLNum = new(query.Limit)
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
			item := broker.SecurityInfoItem{
				Symbol:       securitySymbol(basic.GetSecurity()),
				Name:         cloneStringPtr(basic.Name),
				SecurityType: new(enumName(basic.GetSecType(), qotcommonpb.SecurityType_name)),
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

func (r *futuMarketDataReader) QueryMarketRules(ctx context.Context, query broker.MarketRuleQuery) (*broker.MarketRuleSnapshot, error) {
	if len(query.Symbols) == 0 {
		return nil, fmt.Errorf("futu: QueryMarketRules requires at least one symbol")
	}
	info, err := r.QuerySecurityInfo(ctx, broker.SecurityInfoQuery(query))
	if err == nil {
		if snapshot := marketRulesFromSecurityInfo(info); len(snapshot.Rules) > 0 {
			return snapshot, nil
		}
	}
	fallbackReason := "QuerySecurityInfo returned no usable market rules"
	if err != nil {
		fallbackReason = fmt.Sprintf("QuerySecurityInfo failed: %v", err)
	}

	snapshot, fallbackErr := r.QuerySecuritySnapshot(ctx, broker.SecuritySnapshotQuery(query))
	if fallbackErr != nil {
		if err != nil {
			return nil, fmt.Errorf("%w; fallback QuerySecuritySnapshot failed: %v", err, fallbackErr)
		}
		return nil, fallbackErr
	}
	rules := marketRulesFromSecuritySnapshot(snapshot)
	if len(rules.Rules) == 0 {
		if err != nil {
			return nil, fmt.Errorf("%w; fallback QuerySecuritySnapshot returned no market rules", err)
		}
		return nil, fmt.Errorf("futu: QueryMarketRules returned no market rules")
	}
	rules.Warnings = append(rules.Warnings, fmt.Sprintf(
		"futu market rules loaded from QuerySecuritySnapshot fallback because %s",
		fallbackReason,
	))
	return rules, nil
}

func marketRulesFromSecurityInfo(info *broker.SecurityInfoSnapshot) *broker.MarketRuleSnapshot {
	snapshot := &broker.MarketRuleSnapshot{}
	if info == nil {
		return snapshot
	}
	snapshot.AccountID = info.AccountID
	for _, security := range info.Securities {
		if strings.TrimSpace(security.Symbol) == "" || security.LotSize == nil || *security.LotSize <= 0 {
			continue
		}
		snapshot.Rules = append(snapshot.Rules, broker.MarketRuleItem{
			Symbol:  security.Symbol,
			LotSize: cloneInt32Ptr(security.LotSize),
		})
	}
	return snapshot
}

func marketRulesFromSecuritySnapshot(result *broker.SecuritySnapshotResult) *broker.MarketRuleSnapshot {
	snapshot := &broker.MarketRuleSnapshot{}
	if result == nil {
		return snapshot
	}
	snapshot.AccountID = result.AccountID
	for _, security := range result.Snapshots {
		if strings.TrimSpace(security.Symbol) == "" || security.LotSize == nil || *security.LotSize <= 0 {
			continue
		}
		snapshot.Rules = append(snapshot.Rules, broker.MarketRuleItem{
			Symbol:  security.Symbol,
			LotSize: cloneInt32Ptr(security.LotSize),
		})
	}
	return snapshot
}

func (r *futuMarketDataReader) QuerySecuritySnapshot(ctx context.Context, query broker.SecuritySnapshotQuery) (*broker.SecuritySnapshotResult, error) {
	if len(query.Symbols) == 0 {
		return nil, fmt.Errorf("futu: QuerySecuritySnapshot requires at least one symbol")
	}
	var result *broker.SecuritySnapshotResult
	if err := r.exchange.withClient(ctx, func(client *opend.Client) error {
		securities, err := securitiesFromSymbols(query.Symbols)
		if err != nil {
			return broker.NewSymbolScopedSnapshotError(err)
		}
		snapshots, err := client.GetSecuritySnapshot(ctx, securities)
		if err != nil {
			return err
		}
		res := &broker.SecuritySnapshotResult{AccountID: query.AccountID}
		observedAt := time.Now().UTC()
		for _, snap := range snapshots {
			if snap == nil || snap.Basic == nil {
				continue
			}
			basic := snap.Basic
			preMarket := extendedSessionSnapshotFromProto(basic.GetPreMarket())
			afterMarket := extendedSessionSnapshotFromProto(basic.GetAfterMarket())
			overnight := extendedSessionSnapshotFromProto(basic.GetOvernight())
			item := broker.SecuritySnapshotItem{
				Symbol:        securitySymbol(basic.GetSecurity()),
				Name:          cloneStringPtr(basic.Name),
				SecurityType:  new(enumName(basic.GetType(), qotcommonpb.SecurityType_name)),
				IsSuspended:   cloneBoolPtr(basic.IsSuspend),
				LastPrice:     cloneFloat64Ptr(basic.CurPrice),
				PreviousClose: cloneFloat64Ptr(basic.LastClosePrice),
				OpenPrice:     cloneFloat64Ptr(basic.OpenPrice),
				HighPrice:     cloneFloat64Ptr(basic.HighPrice),
				LowPrice:      cloneFloat64Ptr(basic.LowPrice),
				Volume:        int64AsFloat64Ptr(basic.Volume),
				Turnover:      cloneFloat64Ptr(basic.Turnover),
				LotSize:       cloneInt32Ptr(basic.LotSize),
				UpdateTime:    cloneStringPtr(basic.UpdateTime),
				ObservedAt:    observedAt,
				PreMarket:     preMarket,
				AfterMarket:   afterMarket,
				Overnight:     overnight,
			}
			preQuote := extendedMarketQuoteFromProto(basic.GetPreMarket(), basic.GetUpdateTime())
			afterQuote := extendedMarketQuoteFromProto(basic.GetAfterMarket(), basic.GetUpdateTime())
			overnightQuote := extendedMarketQuoteFromProto(basic.GetOvernight(), basic.GetUpdateTime())
			session := sessionFromExtendedBlocksAt(item.Symbol, preQuote, afterQuote, overnightQuote, observedAt)
			if session != market.SessionUnknown {
				item.Session = new(string(session))
			}
			if basic.CurPrice != nil && market.ShouldUseRegularCloseAsPreviousClose(
				item.Symbol,
				session,
				decimal.NewFromFloat(basic.GetCurPrice()),
			) {
				item.PreviousClose = cloneFloat64Ptr(basic.CurPrice)
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

func extendedSessionSnapshotFromProto(data *qotcommonpb.PreAfterMarketData) *broker.ExtendedSessionSnapshot {
	if data == nil {
		return nil
	}
	return &broker.ExtendedSessionSnapshot{
		Price:      cloneFloat64Ptr(data.Price),
		HighPrice:  cloneFloat64Ptr(data.HighPrice),
		LowPrice:   cloneFloat64Ptr(data.LowPrice),
		Volume:     int64AsFloat64Ptr(data.Volume),
		Turnover:   cloneFloat64Ptr(data.Turnover),
		Change:     cloneFloat64Ptr(data.ChangeVal),
		ChangeRate: cloneFloat64Ptr(data.ChangeRate),
		Amplitude:  cloneFloat64Ptr(data.Amplitude),
	}
}

func (r *futuMarketDataReader) QueryOrderBook(ctx context.Context, query broker.OrderBookQuery) (*broker.OrderBookSnapshot, error) {
	if query.Symbol == "" {
		return nil, fmt.Errorf("futu: QueryOrderBook requires a symbol")
	}
	num := query.Num
	if num <= 0 {
		num = 10 // default depth levels
	}
	var result *broker.OrderBookSnapshot
	if err := r.exchange.withClient(ctx, func(client *opend.Client) error {
		res, err := r.exchange.QueryOrderBook(ctx, query.Symbol, num)
		if err != nil {
			return err
		}
		snapshot := orderBookSnapshotFromOpendResult(res, &query)
		result = snapshot
		return nil
	}); err != nil {
		return nil, err
	}
	return result, nil
}
