package futu

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetsearchquotepb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsearchquote"
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
	if limit > 1000 {
		limit = 1000
	}
	location := time.UTC
	if profile, ok := market.ProfileForSymbol(query.Symbol); ok && profile.Location != nil {
		location = profile.Location
	}
	lowerBound := r.klineListingLowerBound(ctx, query.Symbol, location)
	extendedHours := shouldRequestExtendedKLines(query.Symbol, interval)
	requestedSessions, err := resolveBrokerKLineSessions(query.Sessions, extendedHours)
	if err != nil {
		return nil, err
	}
	session := brokerKLineSessionLabel(requestedSessions, extendedHours)

	var klines []bbgotypes.KLine
	hasMore := false
	beforeTime := strings.TrimSpace(query.BeforeTime)
	if beforeTime != "" {
		beforeAt, parseErr := parseFutuKLineQueryTime(beforeTime, location)
		if parseErr != nil {
			return nil, fmt.Errorf("futu: invalid beforeTime: %w", parseErr)
		}
		klines, hasMore, err = r.queryAdaptiveKLinePage(
			ctx, query.Symbol, interval, lowerBound, beforeAt, limit, requestedSessions,
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
		if beginAt.After(endAt) {
			return nil, fmt.Errorf("futu: fromTime must be earlier than or equal to toTime")
		}
		klines, err = r.exchange.QueryKLinesForSessions(
			ctx, query.Symbol, interval, bbgotypes.KLineQueryOptions{
				StartTime: &beginAt, EndTime: &endAt, Limit: limit,
			}, requestedSessions,
		)
		klines = normalizeBrokerKLineRange(klines, beginAt, endAt, limit)
	} else {
		klines, hasMore, err = r.queryAdaptiveKLinePage(
			ctx, query.Symbol, interval, lowerBound, time.Now().In(location), limit, requestedSessions,
		)
	}
	if err != nil {
		return nil, err
	}

	return r.buildBrokerKLineSnapshot(query, interval, klines, hasMore, hasNonRegularBrokerSession(requestedSessions, extendedHours), session, requestedSessions), nil
}

func hasNonRegularBrokerSession(sessions []market.Session, extendedHours bool) bool {
	if sessions == nil {
		return extendedHours
	}
	for _, session := range sessions {
		if session != market.SessionRegular {
			return true
		}
	}
	return false
}

func resolveBrokerKLineSessions(values []string, extendedHours bool) ([]market.Session, error) {
	if len(values) == 0 {
		if !extendedHours {
			return []market.Session{market.SessionRegular}, nil
		}
		return []market.Session{
			market.SessionRegular,
			market.SessionPre,
			market.SessionAfter,
			market.SessionOvernight,
		}, nil
	}
	seen := make(map[market.Session]struct{}, 3)
	for _, value := range values {
		for _, token := range strings.Split(value, ",") {
			switch strings.ToLower(strings.TrimSpace(token)) {
			case "regular":
				seen[market.SessionRegular] = struct{}{}
			case "extended":
				if !extendedHours {
					return nil, fmt.Errorf("%w: extended is unsupported", broker.ErrInvalidCandleSessions)
				}
				seen[market.SessionPre] = struct{}{}
				seen[market.SessionAfter] = struct{}{}
			case "overnight":
				if !extendedHours {
					return nil, fmt.Errorf("%w: overnight is unsupported", broker.ErrInvalidCandleSessions)
				}
				seen[market.SessionOvernight] = struct{}{}
			default:
				return nil, fmt.Errorf("%w: %q", broker.ErrInvalidCandleSessions, token)
			}
		}
	}
	result := make([]market.Session, 0, len(seen))
	for _, session := range []market.Session{market.SessionRegular, market.SessionPre, market.SessionAfter, market.SessionOvernight} {
		if _, ok := seen[session]; ok {
			result = append(result, session)
		}
	}
	return result, nil
}

func brokerKLineSessionLabel(sessions []market.Session, extendedHours bool) string {
	if len(sessions) == 0 && extendedHours {
		return "all"
	}
	for _, session := range sessions {
		if session != market.SessionRegular {
			return "all"
		}
	}
	return "regular"
}

func (r *futuMarketDataReader) buildBrokerKLineSnapshot(
	query broker.KLineQuery,
	interval bbgotypes.Interval,
	klines []bbgotypes.KLine,
	hasMore bool,
	extendedHours bool,
	session string,
	requestedSessions ...[]market.Session,
) *broker.KLineSnapshot {
	var sessions []market.Session
	if len(requestedSessions) > 0 {
		sessions = requestedSessions[0]
	}
	snapshot := &broker.KLineSnapshot{
		AccountID:     query.AccountID,
		Symbol:        strings.ToUpper(strings.TrimSpace(query.Symbol)),
		Period:        string(interval),
		ExtendedHours: extendedHours,
		Session:       session,
		Sessions:      brokerSessionStrings(sessions),
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

func brokerSessionStrings(sessions []market.Session) []string {
	if len(sessions) == 0 {
		return nil
	}
	result := make([]string, 0, len(sessions))
	hasRegular, hasExtended, hasOvernight := false, false, false
	for _, session := range sessions {
		switch session {
		case market.SessionRegular:
			hasRegular = true
		case market.SessionPre, market.SessionAfter:
			hasExtended = true
		case market.SessionOvernight:
			hasOvernight = true
		}
	}
	if hasRegular {
		result = append(result, "regular")
	}
	if hasExtended {
		result = append(result, "extended")
	}
	if hasOvernight {
		result = append(result, "overnight")
	}
	return result
}

func (r *futuMarketDataReader) queryAdaptiveKLinePage(
	ctx context.Context,
	symbol string,
	interval bbgotypes.Interval,
	lowerBound time.Time,
	endExclusive time.Time,
	limit int,
	sessions ...[]market.Session,
) ([]bbgotypes.KLine, bool, error) {
	var requested []market.Session
	if len(sessions) > 0 {
		requested = sessions[0]
	}
	location := endExclusive.Location()
	lowerBound = lowerBound.In(location)
	if !lowerBound.Before(endExclusive) {
		return []bbgotypes.KLine{}, false, nil
	}
	lookback := max(interval.Duration()*time.Duration(limit+1)*2, 7*24*time.Hour)

	for {
		beginAt := endExclusive.Add(-lookback)
		reachedLowerBound := !beginAt.After(lowerBound)
		if reachedLowerBound {
			beginAt = lowerBound
		}
		klines, err := r.exchange.QueryAllKLinesForSessions(
			ctx, symbol, interval, beginAt, endExclusive,
			qotcommonpb.RehabType_RehabType_Forward, requested,
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

func normalizeBrokerKLineRange(
	klines []bbgotypes.KLine,
	beginInclusive time.Time,
	endInclusive time.Time,
	limit int,
) []bbgotypes.KLine {
	byStart := make(map[int64]bbgotypes.KLine, len(klines))
	for _, kline := range klines {
		at := kline.StartTime.Time().UTC()
		if at.Before(beginInclusive.UTC()) || at.After(endInclusive.UTC()) {
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
		return result[:limit]
	}
	return result
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
	if err := r.exchange.withRetryingClient(ctx, func(client *opend.Client) error {
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
	if err := r.exchange.withRetryingClient(ctx, func(client *opend.Client) error {
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
