package futu

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetsearchquotepb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsearchquote"
	qotgetsecuritysnapshotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsecuritysnapshot"
	"github.com/jftrade/jftrade-main/pkg/market"
)

// --- broker.MarketDataReader extended methods (futuMarketDataReader) ---

func (r *futuMarketDataReader) QueryQuote(ctx context.Context, query broker.QuoteQuery) (*broker.QuoteSnapshot, error) {
	if len(query.Symbols) == 0 {
		return nil, fmt.Errorf("futu: QueryQuote requires at least one symbol")
	}
	quotes, err := r.exchange.queryBasicQotList(ctx, query.Symbols)
	if err != nil {
		return nil, err
	}
	ordered := make([]*qotcommonpb.BasicQot, 0, len(query.Symbols))
	for _, symbol := range query.Symbols {
		quote, err := basicQotForSymbol(quotes, symbol)
		if err != nil {
			return nil, err
		}
		ordered = append(ordered, quote)
	}
	return quoteSnapshotFromProtoList(query.AccountID, ordered), nil
}

func quoteSnapshotFromProtoList(accountID string, qots []*qotcommonpb.BasicQot) *broker.QuoteSnapshot {
	snapshot := &broker.QuoteSnapshot{AccountID: accountID}
	for _, qot := range qots {
		if qot == nil {
			continue
		}
		item := broker.QuoteItem{
			Symbol:     securitySymbol(qot.GetSecurity()),
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
	return snapshot
}

func (r *futuMarketDataReader) QueryKLines(ctx context.Context, query broker.KLineQuery) (*broker.KLineSnapshot, error) {
	if query.Symbol == "" {
		return nil, fmt.Errorf("futu: QueryKLines requires a symbol")
	}
	if strings.TrimSpace(query.BeforeTime) != "" &&
		(strings.TrimSpace(query.FromTime) != "" || strings.TrimSpace(query.ToTime) != "") {
		return nil, fmt.Errorf("futu: beforeTime cannot be combined with fromTime or toTime")
	}

	interval, err := futuIntervalFromPeriod(query.Period)
	if err != nil {
		return nil, err
	}
	limit := int(query.Limit)
	if limit < 1 {
		limit = 500
	}
	if limit > 500 {
		limit = 500
	}
	location := time.UTC
	if profile, ok := market.ProfileForSymbol(query.Symbol); ok && profile.Location != nil {
		location = profile.Location
	}
	lowerBound := r.klineListingLowerBound(ctx, query.Symbol, location)
	extendedHours := shouldRequestExtendedKLines(query.Symbol, interval)
	session := "regular"
	if extendedHours {
		session = "all"
	}

	var klines []bbgotypes.KLine
	hasMore := false
	beforeTime := strings.TrimSpace(query.BeforeTime)
	if beforeTime != "" {
		beforeAt, parseErr := parseFutuKLineQueryTime(beforeTime, location)
		if parseErr != nil {
			return nil, fmt.Errorf("futu: invalid beforeTime: %w", parseErr)
		}
		klines, hasMore, err = r.queryAdaptiveKLinePage(
			ctx, query.Symbol, interval, lowerBound, beforeAt, limit,
		)
	} else if strings.TrimSpace(query.FromTime) != "" || strings.TrimSpace(query.ToTime) != "" {
		beginAt := lowerBound
		endAt := time.Now().In(location)
		if value := strings.TrimSpace(query.FromTime); value != "" {
			beginAt, err = parseFutuKLineQueryTime(value, location)
			if err != nil {
				return nil, fmt.Errorf("futu: invalid fromTime: %w", err)
			}
		}
		if value := strings.TrimSpace(query.ToTime); value != "" {
			endAt, err = parseFutuKLineQueryTime(value, location)
			if err != nil {
				return nil, fmt.Errorf("futu: invalid toTime: %w", err)
			}
		}
		if !beginAt.Before(endAt) {
			return nil, fmt.Errorf("futu: fromTime must be before toTime")
		}
		klines, err = r.exchange.QueryAllKLines(
			ctx, query.Symbol, interval, beginAt.In(location), endAt.In(location),
			qotcommonpb.RehabType_RehabType_Forward,
		)
		klines = normalizeBrokerKLinePage(klines, beginAt, endAt, limit, false)
	} else {
		klines, hasMore, err = r.queryAdaptiveKLinePage(
			ctx, query.Symbol, interval, lowerBound, time.Now().In(location), limit,
		)
	}
	if err != nil {
		return nil, err
	}

	return r.buildBrokerKLineSnapshot(query, interval, klines, hasMore, extendedHours, session), nil
}

func (r *futuMarketDataReader) buildBrokerKLineSnapshot(
	query broker.KLineQuery,
	interval bbgotypes.Interval,
	klines []bbgotypes.KLine,
	hasMore bool,
	extendedHours bool,
	session string,
) *broker.KLineSnapshot {
	snapshot := &broker.KLineSnapshot{
		AccountID:     query.AccountID,
		Symbol:        strings.ToUpper(strings.TrimSpace(query.Symbol)),
		Period:        string(interval),
		ExtendedHours: extendedHours,
		Session:       session,
		Pagination: broker.KLinePagination{
			HasMore: hasMore,
		},
		KLines: make([]broker.KLineItem, 0, len(klines)),
	}
	for _, kline := range klines {
		open := kline.Open.Float64()
		closePrice := kline.Close.Float64()
		high := kline.High.Float64()
		low := kline.Low.Float64()
		volume := kline.Volume.Float64()
		turnover := kline.QuoteVolume.Float64()
		item := broker.KLineItem{
			Time:     kline.StartTime.Time().UTC().Format(time.RFC3339Nano),
			Open:     &open,
			Close:    &closePrice,
			High:     &high,
			Low:      &low,
			Volume:   &volume,
			Turnover: &turnover,
		}
		if resolvedSession, ok := r.exchange.ResolveKLineSession(kline); extendedHours && ok {
			item.Session = string(resolvedSession)
		}
		snapshot.KLines = append(snapshot.KLines, item)
	}
	if hasMore && len(snapshot.KLines) > 0 {
		snapshot.Pagination.NextBefore = snapshot.KLines[0].Time
	}
	return snapshot
}

func (r *futuMarketDataReader) queryAdaptiveKLinePage(
	ctx context.Context,
	symbol string,
	interval bbgotypes.Interval,
	lowerBound time.Time,
	endExclusive time.Time,
	limit int,
) ([]bbgotypes.KLine, bool, error) {
	location := endExclusive.Location()
	lowerBound = lowerBound.In(location)
	if !lowerBound.Before(endExclusive) {
		return []bbgotypes.KLine{}, false, nil
	}
	lookback := interval.Duration() * time.Duration(limit+1) * 2
	if lookback < 7*24*time.Hour {
		lookback = 7 * 24 * time.Hour
	}

	for {
		beginAt := endExclusive.Add(-lookback)
		reachedLowerBound := !beginAt.After(lowerBound)
		if reachedLowerBound {
			beginAt = lowerBound
		}
		requestEnd := endExclusive.Add(-time.Nanosecond)
		klines, err := r.exchange.QueryAllKLines(
			ctx, symbol, interval, beginAt, requestEnd,
			qotcommonpb.RehabType_RehabType_Forward,
		)
		if err != nil {
			return nil, false, err
		}
		normalized := normalizeBrokerKLinePage(
			klines, beginAt, endExclusive, limit+1, true,
		)
		if len(normalized) >= limit+1 {
			return normalized[len(normalized)-limit:], true, nil
		}
		if reachedLowerBound {
			return normalized, false, nil
		}
		if lookback > 100*365*24*time.Hour {
			lookback = endExclusive.Sub(lowerBound)
		} else {
			lookback *= 2
		}
	}
}

func normalizeBrokerKLinePage(
	klines []bbgotypes.KLine,
	beginInclusive time.Time,
	endExclusive time.Time,
	limit int,
	keepLatest bool,
) []bbgotypes.KLine {
	byStart := make(map[int64]bbgotypes.KLine, len(klines))
	for _, kline := range klines {
		at := kline.StartTime.Time().UTC()
		if at.Before(beginInclusive.UTC()) || !at.Before(endExclusive.UTC()) {
			continue
		}
		byStart[at.UnixNano()] = kline
	}
	result := make([]bbgotypes.KLine, 0, len(byStart))
	for _, kline := range byStart {
		result = append(result, kline)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].StartTime.Time().Before(result[j].StartTime.Time())
	})
	if limit > 0 && len(result) > limit {
		if keepLatest {
			return result[len(result)-limit:]
		}
		return result[:limit]
	}
	return result
}

func (r *futuMarketDataReader) klineListingLowerBound(
	ctx context.Context,
	symbol string,
	location *time.Location,
) time.Time {
	fallback := time.Date(1900, time.January, 1, 0, 0, 0, 0, location)
	info, err := r.exchange.queryStaticInfo(ctx, symbol)
	if err != nil || info == nil || info.GetBasic() == nil {
		return fallback
	}
	basic := info.GetBasic()
	if timestamp := basic.GetListTimestamp(); timestamp > 0 {
		return time.Unix(int64(timestamp), 0).In(location)
	}
	if listTime := strings.TrimSpace(basic.GetListTime()); listTime != "" {
		if parsed, parseErr := parseFutuKLineQueryTime(listTime, location); parseErr == nil {
			return parsed
		}
	}
	return fallback
}

func parseFutuKLineQueryTime(value string, location *time.Location) (time.Time, error) {
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02",
	} {
		parsed, err := time.ParseInLocation(layout, strings.TrimSpace(value), location)
		if err == nil {
			return parsed.In(location), nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported time value %q", value)
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
		result = securityInfoSnapshotFromProtoList(query.AccountID, staticInfos)
		return nil
	}); err != nil {
		return nil, err
	}
	return result, nil
}

func securityInfoSnapshotFromProtoList(accountID string, staticInfos []*qotcommonpb.SecurityStaticInfo) *broker.SecurityInfoSnapshot {
	snapshot := &broker.SecurityInfoSnapshot{AccountID: accountID}
	for _, info := range staticInfos {
		if info == nil || info.GetBasic() == nil {
			continue
		}
		basic := info.GetBasic()
		snapshot.Securities = append(snapshot.Securities, broker.SecurityInfoItem{
			Symbol:       securitySymbol(basic.GetSecurity()),
			Name:         cloneStringPtr(basic.Name),
			SecurityType: new(enumName(basic.GetSecType(), qotcommonpb.SecurityType_name)),
			LotSize:      cloneInt32Ptr(basic.LotSize),
			ListTime:     cloneStringPtr(basic.ListTime),
			IsDelisted:   cloneBoolPtr(basic.Delisting),
		})
	}
	return snapshot
}

func (r *futuMarketDataReader) QuerySecuritySearch(ctx context.Context, query broker.SecuritySearchQuery) (*broker.SecuritySearchSnapshot, error) {
	keyword := strings.TrimSpace(query.Keyword)
	if keyword == "" {
		return nil, fmt.Errorf("futu: QuerySecuritySearch requires a keyword")
	}
	limit := query.Limit
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > 100 {
		return nil, fmt.Errorf("futu: QuerySecuritySearch limit must be between 1 and 100")
	}

	var result *broker.SecuritySearchSnapshot
	if err := r.exchange.withClient(ctx, func(client *opend.Client) error {
		matches, err := client.GetSearchQuote(ctx, keyword, limit)
		if err != nil {
			return err
		}
		result = securitySearchSnapshotFromProtoList(query.AccountID, matches)
		return nil
	}); err != nil {
		return nil, err
	}
	return result, nil
}

func securitySearchSnapshotFromProtoList(accountID string, matches []*qotgetsearchquotepb.SearchQuote) *broker.SecuritySearchSnapshot {
	snapshot := &broker.SecuritySearchSnapshot{AccountID: accountID}
	for _, match := range matches {
		if match == nil {
			continue
		}
		marketCode := futuSearchMarketCode(qotcommonpb.QotMarket(match.GetMarket()))
		symbol := canonicalSearchQuoteSymbol(marketCode, match.GetCode())
		if symbol == "" {
			continue
		}
		snapshot.Entries = append(snapshot.Entries, broker.SecuritySearchItem{
			Market:       marketCode,
			Symbol:       symbol,
			Name:         strings.TrimSpace(match.GetName()),
			SecurityType: enumName(match.GetSecType(), qotcommonpb.SecurityType_name),
			IsWatched:    match.GetIsWatched(),
		})
	}
	return snapshot
}

func futuSearchMarketCode(value qotcommonpb.QotMarket) string {
	if marketCode, err := futuMarketCodeFromQotMarket(value); err == nil {
		return marketCode
	}
	switch value {
	case qotcommonpb.QotMarket_QotMarket_HK_Future:
		return "HK_FUTURE"
	case qotcommonpb.QotMarket_QotMarket_FX_Security:
		return "FX"
	case qotcommonpb.QotMarket_QotMarket_CC_Security:
		return "CRYPTO"
	default:
		return "UNKNOWN"
	}
}

func canonicalSearchQuoteSymbol(marketCode, rawCode string) string {
	marketCode = strings.ToUpper(strings.TrimSpace(marketCode))
	code := strings.ToUpper(strings.TrimSpace(rawCode))
	code = strings.ReplaceAll(code, ":", ".")
	if marketCode == "" || code == "" {
		return ""
	}
	if separator := strings.Index(code, "."); separator > 0 {
		prefix := strings.TrimSpace(code[:separator])
		bareCode := strings.TrimSpace(code[separator+1:])
		if canonicalSearchQuoteMarketPrefix(prefix) == marketCode && bareCode != "" {
			return marketCode + "." + bareCode
		}
	}
	return marketCode + "." + code
}

func canonicalSearchQuoteMarketPrefix(value string) string {
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
	default:
		return normalized
	}
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
	snapshotsBySymbol, err := r.exchange.querySecuritySnapshotList(ctx, query.Symbols)
	if err != nil {
		if errors.Is(err, errNoSecuritySnapshots) {
			return &broker.SecuritySnapshotResult{AccountID: query.AccountID}, nil
		}
		return nil, err
	}
	// querySecuritySnapshotList canonicalizes the same symbol set before it
	// reaches OpenD, so this second pass cannot fail after a successful read.
	canonical, _ := canonicalSecuritySnapshotSymbols(query.Symbols)
	snapshots := make([]*qotgetsecuritysnapshotpb.Snapshot, 0, len(canonical))
	for _, symbol := range canonical {
		if snapshot := snapshotsBySymbol[symbol]; snapshot != nil {
			snapshots = append(snapshots, snapshot)
		}
	}
	return securitySnapshotResultFromProtoList(query.AccountID, snapshots, time.Now().UTC()), nil
}

func securitySnapshotResultFromProtoList(accountID string, snapshots []*qotgetsecuritysnapshotpb.Snapshot, observedAt time.Time) *broker.SecuritySnapshotResult {
	result := &broker.SecuritySnapshotResult{AccountID: accountID}
	for _, snapshot := range snapshots {
		item, ok := securitySnapshotItemFromProto(snapshot, observedAt)
		if !ok {
			continue
		}
		result.Snapshots = append(result.Snapshots, item)
	}
	return result
}

func securitySnapshotItemFromProto(snap *qotgetsecuritysnapshotpb.Snapshot, observedAt time.Time) (broker.SecuritySnapshotItem, bool) {
	if snap == nil || snap.Basic == nil {
		return broker.SecuritySnapshotItem{}, false
	}
	basic := snap.Basic
	item := broker.SecuritySnapshotItem{
		Symbol:       securitySymbol(basic.GetSecurity()),
		Name:         cloneStringPtr(basic.Name),
		SecurityType: new(enumName(basic.GetType(), qotcommonpb.SecurityType_name)),
		ProductClass: productClassFromSecurityType(basic.GetType()),
		IsSuspended:  cloneBoolPtr(basic.IsSuspend),
		LastPrice:    cloneFloat64Ptr(basic.CurPrice),
		BidPrice:     cloneFloat64Ptr(basic.BidPrice),
		AskPrice:     cloneFloat64Ptr(basic.AskPrice),
		// Keep OpenD's raw LastClosePrice here. Watchlist regular-session
		// change is measured against that value even while the US market is closed.
		PreviousClose: cloneFloat64Ptr(basic.LastClosePrice),
		OpenPrice:     cloneFloat64Ptr(basic.OpenPrice),
		HighPrice:     cloneFloat64Ptr(basic.HighPrice),
		LowPrice:      cloneFloat64Ptr(basic.LowPrice),
		Volume:        int64AsFloat64Ptr(basic.Volume),
		Turnover:      cloneFloat64Ptr(basic.Turnover),
		LotSize:       cloneInt32Ptr(basic.LotSize),
		UpdateTime:    cloneStringPtr(basic.UpdateTime),
		ObservedAt:    observedAt,
		PreMarket:     extendedSessionSnapshotFromProto(basic.GetPreMarket()),
		AfterMarket:   extendedSessionSnapshotFromProto(basic.GetAfterMarket()),
		Overnight:     extendedSessionSnapshotFromProto(basic.GetOvernight()),
	}
	item.MarketSegment = marketSegmentFromProductClass(item.ProductClass)
	preQuote := extendedMarketQuoteFromProto(basic.GetPreMarket(), basic.GetUpdateTime())
	afterQuote := extendedMarketQuoteFromProto(basic.GetAfterMarket(), basic.GetUpdateTime())
	overnightQuote := extendedMarketQuoteFromProto(basic.GetOvernight(), basic.GetUpdateTime())
	session := sessionFromExtendedBlocksAt(item.Symbol, preQuote, afterQuote, overnightQuote, observedAt)
	if session != market.SessionUnknown {
		item.Session = new(string(session))
	}
	applySecuritySnapshotExtensions(&item, snap)
	return item, true
}

func applySecuritySnapshotExtensions(
	item *broker.SecuritySnapshotItem,
	snap *qotgetsecuritysnapshotpb.Snapshot,
) {
	if snap.EquityExData != nil {
		item.PERate = cloneFloat64Ptr(snap.EquityExData.PeRate)
		item.PBRate = cloneFloat64Ptr(snap.EquityExData.PbRate)
	}
	applyOptionSnapshotData(item, snap.GetOptionExData())
	applyWarrantSnapshotData(item, snap.GetWarrantExData())
	applyFutureSnapshotData(item, snap.GetFutureExData())
	applyFundSnapshotData(item, snap.GetTrustExData())
}

func applyOptionSnapshotData(item *broker.SecuritySnapshotItem, option *qotgetsecuritysnapshotpb.OptionSnapshotExData) {
	if option == nil {
		return
	}
	contractSize := float64(option.GetContractSize())
	if option.ContractSizeFloat != nil {
		contractSize = option.GetContractSizeFloat()
	}
	item.Option = &broker.OptionSnapshotData{
		OptionType:           enumName(option.GetType(), qotcommonpb.OptionType_name),
		UnderlyingCode:       securitySymbol(option.GetOwner()),
		ExpiryDate:           option.GetStrikeTime(),
		StrikePrice:          option.GetStrikePrice(),
		ContractSize:         contractSize,
		ContractMultiplier:   cloneFloat64Ptr(option.ContractMultiplier),
		OpenInterest:         option.GetOpenInterest(),
		NetOpenInterest:      cloneInt32Ptr(option.NetOpenInterest),
		ImpliedVolatility:    option.GetImpliedVolatility(),
		Premium:              option.GetPremium(),
		Delta:                option.GetDelta(),
		Gamma:                option.GetGamma(),
		Vega:                 option.GetVega(),
		Theta:                option.GetTheta(),
		Rho:                  option.GetRho(),
		DaysToExpiry:         cloneInt32Ptr(option.ExpiryDateDistance),
		ContractNominalValue: cloneFloat64Ptr(option.ContractNominalValue),
	}
	item.ProductClass = broker.ProductClassOption
	item.MarketSegment = broker.MarketSegmentDerivatives
}

func applyWarrantSnapshotData(item *broker.SecuritySnapshotItem, warrant *qotgetsecuritysnapshotpb.WarrantSnapshotExData) {
	if warrant == nil {
		return
	}
	item.Warrant = &broker.WarrantSnapshotData{
		WarrantType:        enumName(warrant.GetWarrantType(), qotcommonpb.WarrantType_name),
		UnderlyingCode:     securitySymbol(warrant.GetOwner()),
		IssuerCode:         cloneStringPtr(warrant.IssuerCode),
		MaturityDate:       warrant.GetMaturityTime(),
		LastTradeDate:      warrant.GetEndTradeTime(),
		StrikePrice:        warrant.GetStrikePrice(),
		RecoveryPrice:      warrant.GetRecoveryPrice(),
		ConversionRate:     warrant.GetConversionRate(),
		StreetVolume:       warrant.GetStreetVolumn(),
		IssueVolume:        warrant.GetIssueVolumn(),
		StreetRate:         warrant.GetStreetRate(),
		ImpliedVolatility:  warrant.GetImpliedVolatility(),
		Premium:            warrant.GetPremium(),
		Delta:              warrant.GetDelta(),
		Leverage:           cloneFloat64Ptr(warrant.Leverage),
		BreakEvenPoint:     cloneFloat64Ptr(warrant.BreakEvenPoint),
		PriceRecoveryRatio: cloneFloat64Ptr(warrant.PriceRecoveryRatio),
	}
	item.ProductClass = broker.ProductClassWarrant
	if warrant.GetWarrantType() == int32(qotcommonpb.WarrantType_WarrantType_Bull) ||
		warrant.GetWarrantType() == int32(qotcommonpb.WarrantType_WarrantType_Bear) {
		item.ProductClass = broker.ProductClassCBBC
	}
	item.MarketSegment = broker.MarketSegmentDerivatives
}

func applyFutureSnapshotData(item *broker.SecuritySnapshotItem, future *qotgetsecuritysnapshotpb.FutureSnapshotExData) {
	if future == nil {
		return
	}
	item.Future = &broker.FutureSnapshotData{
		LastSettlementPrice: future.GetLastSettlePrice(),
		OpenInterest:        future.GetPosition(),
		OpenInterestChange:  future.GetPositionChange(),
		LastTradeDate:       future.GetLastTradeTime(),
		LastTradeTimestamp:  cloneFloat64Ptr(future.LastTradeTimestamp),
		IsMainContract:      future.GetIsMainContract(),
	}
	item.ProductClass = broker.ProductClassFuture
	item.MarketSegment = broker.MarketSegmentDerivatives
}

func applyFundSnapshotData(item *broker.SecuritySnapshotItem, fund *qotgetsecuritysnapshotpb.TrustSnapshotExData) {
	if fund == nil {
		return
	}
	item.Fund = &broker.FundSnapshotData{
		DividendYield:         fund.GetDividendYield(),
		AssetsUnderManagement: fund.GetAum(),
		OutstandingUnits:      fund.GetOutstandingUnits(),
		NetAssetValue:         fund.GetNetAssetValue(),
		Premium:               fund.GetPremium(),
		AssetClass:            enumName(fund.GetAssetClass(), qotcommonpb.AssetClass_name),
	}
	item.ProductClass = broker.ProductClassFund
	item.MarketSegment = broker.MarketSegmentSecurities
}

func productClassFromSecurityType(value int32) broker.ProductClass {
	switch qotcommonpb.SecurityType(value) {
	case qotcommonpb.SecurityType_SecurityType_Bond:
		return broker.ProductClassBond
	case qotcommonpb.SecurityType_SecurityType_Eqty:
		return broker.ProductClassEquity
	case qotcommonpb.SecurityType_SecurityType_Trust:
		return broker.ProductClassFund
	case qotcommonpb.SecurityType_SecurityType_Warrant:
		return broker.ProductClassWarrant
	case qotcommonpb.SecurityType_SecurityType_Index:
		return broker.ProductClassIndex
	case qotcommonpb.SecurityType_SecurityType_Plate,
		qotcommonpb.SecurityType_SecurityType_PlateSet:
		return broker.ProductClassPlate
	case qotcommonpb.SecurityType_SecurityType_Drvt:
		return broker.ProductClassOption
	case qotcommonpb.SecurityType_SecurityType_Future:
		return broker.ProductClassFuture
	default:
		return broker.ProductClassUnknown
	}
}

func marketSegmentFromProductClass(productClass broker.ProductClass) broker.MarketSegment {
	switch productClass {
	case broker.ProductClassOption,
		broker.ProductClassWarrant,
		broker.ProductClassCBBC,
		broker.ProductClassFuture:
		return broker.MarketSegmentDerivatives
	case broker.ProductClassEventContract:
		return broker.MarketSegmentPrediction
	default:
		return broker.MarketSegmentSecurities
	}
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
