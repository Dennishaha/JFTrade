package futu

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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
