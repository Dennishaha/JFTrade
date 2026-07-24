package futu

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
)

var featureProtocols = map[broker.FeatureID]map[string]string{
	broker.FeatureMarketIntraday: {
		"intraday": "Qot_GetRT",
	},
	broker.FeatureMarketTicks: {
		"ticks": "Qot_GetTicker",
	},
	broker.FeatureMarketBrokerQueue: {
		"broker_queue": "Qot_GetBroker",
	},
	broker.FeatureMarketCapitalFlow: {
		"flow":         "Qot_GetCapitalFlow",
		"distribution": "Qot_GetCapitalDistribution",
	},
	broker.FeatureOptionChain: {
		"expirations": "Qot_GetOptionExpirationDate",
		"chain":       "Qot_GetOptionChain",
	},
	broker.FeatureOptionScreen: {
		"screen": "Qot_OptionScreen",
	},
	broker.FeatureOptionAnalysis: {
		"quote":                 "Qot_GetOptionQuote",
		"volatility":            "Qot_GetOptionVolatility",
		"exercise_probability":  "Qot_GetOptionExerciseProbability",
		"strategy":              "Qot_GetOptionStrategy",
		"strategy_analysis":     "Qot_GetOptionStrategyAnalysis",
		"strategy_spread":       "Qot_GetOptionStrategySpread",
		"market_statistics":     "Qot_GetOptionMarketStatistic",
		"historical_statistics": "Qot_GetOptionUnderlyingHisStatistic",
		"underlying_overview":   "Qot_GetOptionUnderlyingOverview",
		"historical_volatility": "Qot_GetOptionUnderlyingHisVolatility",
		"underlying_rank":       "Qot_GetOptionUnderlyingRank",
		"contract_rank":         "Qot_GetOptionRank",
	},
	broker.FeatureOptionEvents: {
		"unusual":           "Qot_GetOptionEvent",
		"zero_dte":          "Qot_GetOptionZeroDteScreener",
		"zero_dte_contract": "Qot_GetOptionZeroDteContract",
		"earnings":          "Qot_GetOptionEarningsScreener",
		"seller":            "Qot_GetOptionSellerScreener",
	},
	broker.FeatureWarrants: {
		"related": "Qot_GetReference",
		"list":    "Qot_GetWarrant",
		"screen":  "Qot_WarrantScreen",
	},
	broker.FeatureFutures: {
		"contracts": "Qot_GetFutureInfo",
	},
	broker.FeatureResearchInstrument: {
		"profile":                "Qot_GetCompanyProfile",
		"executives":             "Qot_GetCompanyExecutives",
		"executive_background":   "Qot_GetCompanyExecutiveBackground",
		"operational_efficiency": "Qot_GetCompanyOperationalEfficiency",
		"top_brokers":            "Qot_GetTopTenBuySellBrokers",
	},
	broker.FeatureResearchFinancials: {
		"statements":             "Qot_GetFinancialsStatements",
		"revenue_breakdown":      "Qot_GetFinancialsRevenueBreakdown",
		"earnings_price_move":    "Qot_GetFinancialsEarningsPriceMove",
		"earnings_price_history": "Qot_GetFinancialsEarningsPriceHistory",
	},
	broker.FeatureResearchValuation: {
		"detail":       "Qot_GetValuationDetail",
		"constituents": "Qot_GetValuationPlateStockList",
	},
	broker.FeatureResearchAnalyst: {
		"consensus":   "Qot_GetResearchAnalystConsensus",
		"ratings":     "Qot_GetResearchRatingSummary",
		"morningstar": "Qot_GetResearchMorningstarReport",
		"changes":     "Qot_GetRatingChange",
	},
	broker.FeatureResearchOwnership: {
		"overview":             "Qot_GetShareholdersOverview",
		"changes":              "Qot_GetShareholdersHoldingChanges",
		"holders":              "Qot_GetShareholdersHolderDetail",
		"institutional":        "Qot_GetShareholdersInstitutional",
		"insider_holders":      "Qot_GetInsiderHolderList",
		"insider_transactions": "Qot_GetInsiderTradeList",
		"management_changes":   "Qot_GetHoldingChangeList",
	},
	broker.FeatureResearchCorporateAction: {
		"dividends":    "Qot_GetCorporateActionsDividends",
		"buybacks":     "Qot_GetCorporateActionsBuybacks",
		"splits":       "Qot_GetCorporateActionsStockSplits",
		"code_changes": "Qot_GetCodeChange",
	},
	broker.FeatureResearchShortInterest: {
		"daily_volume":   "Qot_GetDailyShortVolume",
		"short_interest": "Qot_GetShortInterest",
	},
	broker.FeatureResearchNews: {
		"search": "Qot_GetSearchNews",
	},
	broker.FeatureResearchScreen: {
		"stock_v1": "Qot_StockFilter",
		"stock_v2": "Qot_StockScreen",
	},
	broker.FeatureResearchCalendar: {
		"earnings":    "Qot_GetEarningsCalendar",
		"dividends":   "Qot_GetDividendCalendar",
		"economic":    "Qot_GetEconomicCalendar",
		"ipos":        "Qot_GetIpoList",
		"trade_dates": "Qot_RequestTradeDate",
	},
	broker.FeatureResearchMacro: {
		"indicators":        "Qot_GetMacroIndicatorList",
		"indicator_history": "Qot_GetMacroIndicatorHistory",
		"fed_target_rate":   "Qot_GetFedWatchTargetRate",
		"fed_dot_plot":      "Qot_GetFedWatchDotPlot",
	},
	broker.FeatureResearchRankings: {
		"earnings_beat":          "Qot_GetEarningsBeatRank",
		"dividend":               "Qot_GetDividendRank",
		"pre_market":             "Qot_GetUSPreMarketRank",
		"after_hours":            "Qot_GetUSAfterHoursRank",
		"overnight":              "Qot_GetUSOvernightRank",
		"top_movers":             "Qot_GetTopMoversRank",
		"hot":                    "Qot_GetHotList",
		"short_selling":          "Qot_GetShortSellingRank",
		"period_change":          "Qot_GetPeriodChangeRank",
		"high_dividend_state":    "Qot_GetHighDividendSOERank",
		"heatmap":                "Qot_GetHeatMapData",
		"rise_fall_distribution": "Qot_GetRiseFallDistribution",
		"market_state":           "Qot_GetMarketState",
		"fund_catalog":           "Qot_GetStaticInfo",
	},
	broker.FeatureResearchInstitutions: {
		"list":               "Qot_GetInstitutionList",
		"profile":            "Qot_GetInstitutionProfile",
		"distribution":       "Qot_GetInstitutionDistribution",
		"holding_changes":    "Qot_GetInstitutionHoldingChange",
		"holdings":           "Qot_GetInstitutionHoldingList",
		"ark_fund_holdings":  "Qot_GetArkFundHolding",
		"ark_stock_activity": "Qot_GetArkStockDynamic",
		"ark_transactions":   "Qot_GetArkActiveTransaction",
	},
	broker.FeatureResearchIndustry: {
		"chains":          "Qot_GetIndustrialChainList",
		"chain_detail":    "Qot_GetIndustrialChainDetail",
		"chains_by_plate": "Qot_GetIndustrialChainByPlate",
		"plate":           "Qot_GetIndustrialPlateInfo",
		"plate_stocks":    "Qot_GetIndustrialPlateStock",
		"owner_plates":    "Qot_GetOwnerPlate",
		"plate_list":      "Qot_GetPlateSet",
		"plate_members":   "Qot_GetPlateSecurity",
	},
	broker.FeatureTechnicalIndicator: {
		"list":      "Qot_GetIndicatorList",
		"calculate": "Qot_RequestIndicatorCalc",
	},
	broker.FeaturePredictionDiscover: {
		"categories":   "Qot_GetEventContractCategory",
		"competitions": "Qot_FilterCompetition",
		"series":       "Qot_GetEventContractSeriesList",
		"events":       "Qot_GetEventContractEventList",
		"contracts":    "Qot_GetEventContract",
		"milestones":   "Qot_GetEventContractMilestoneList",
	},
	broker.FeaturePredictionSnapshot: {
		"snapshot": "Qot_GetEventContractSnapshot",
	},
	broker.FeaturePredictionDepth: {
		"order_book": "Qot_GetEventContractOrderBook",
	},
	broker.FeaturePredictionHistory: {
		"candles":    "Qot_GetEventContractKline",
		"historical": "Qot_RequestHistoryEventContractKL",
		"ticks":      "Qot_GetEventContractTicker",
		"subscribe":  "Qot_SubEventContract",
	},
	broker.FeaturePredictionComboEligible: {
		"eligible_events": "Qot_GetEventContractComboList",
	},
	broker.FeaturePredictionComboQuote: {
		"quote": "Qot_GetEventContractComboRfq",
	},
	broker.FeaturePriceAlertList: {
		"list": "Qot_GetPriceReminder",
	},
	broker.FeaturePriceAlertSet: {
		"set": "Qot_SetPriceReminder",
	},
	broker.FeatureOptionEventAlertList: {
		"list": "Qot_GetOptionEventAlert",
	},
	broker.FeatureOptionEventAlertSet: {
		"set": "Qot_SetOptionEventAlert",
	},
	broker.FeatureRemoteWatchlistModify: {
		"modify": "Qot_ModifyUserSecurity",
	},
}

var defaultFeatureOperations = map[broker.FeatureID]string{
	broker.FeatureMarketIntraday: "intraday", broker.FeatureMarketTicks: "ticks",
	broker.FeatureMarketBrokerQueue: "broker_queue", broker.FeatureMarketCapitalFlow: "flow",
	broker.FeatureOptionChain: "chain", broker.FeatureOptionScreen: "screen",
	broker.FeatureOptionAnalysis: "underlying_overview", broker.FeatureOptionEvents: "unusual",
	broker.FeatureWarrants: "list", broker.FeatureFutures: "contracts",
	broker.FeatureResearchInstrument: "profile", broker.FeatureResearchFinancials: "statements",
	broker.FeatureResearchValuation: "detail", broker.FeatureResearchAnalyst: "consensus",
	broker.FeatureResearchOwnership: "overview", broker.FeatureResearchCorporateAction: "dividends",
	broker.FeatureResearchShortInterest: "daily_volume", broker.FeatureResearchNews: "search",
	broker.FeatureResearchScreen: "stock_v2", broker.FeatureResearchCalendar: "earnings",
	broker.FeatureResearchMacro: "indicators", broker.FeatureResearchRankings: "top_movers",
	broker.FeatureResearchInstitutions: "list", broker.FeatureResearchIndustry: "chains",
	broker.FeatureTechnicalIndicator: "list",
	broker.FeaturePredictionDiscover: "categories", broker.FeaturePredictionSnapshot: "snapshot",
	broker.FeaturePredictionDepth: "order_book", broker.FeaturePredictionHistory: "candles",
	broker.FeaturePredictionComboEligible: "eligible_events", broker.FeaturePredictionComboQuote: "quote",
	broker.FeaturePriceAlertList: "list", broker.FeatureOptionEventAlertList: "list",
}

var protocolInstrumentField = map[string]string{
	"Qot_GetOptionExpirationDate":          "owner",
	"Qot_GetOptionChain":                   "owner",
	"Qot_GetOptionStrategy":                "owner",
	"Qot_GetOptionStrategySpread":          "owner",
	"Qot_GetOptionUnderlyingOverview":      "ownerList",
	"Qot_GetOptionUnderlyingHisStatistic":  "owner",
	"Qot_GetOptionUnderlyingHisVolatility": "owner",
	"Qot_GetOptionZeroDteContract":         "owner",
	"Qot_GetWarrant":                       "owner",
	"Qot_GetEventContractEventList":        "series",
	"Qot_GetEventContract":                 "event",
	"Qot_GetEventContractMilestoneList":    "relatedEvent",
	"Qot_GetEventContractComboList":        "series",
	"Qot_GetEventContractSnapshot":         "securityList",
	"Qot_GetFutureInfo":                    "securityList",
	"Qot_GetOptionQuote":                   "multi_legs",
	"Qot_GetPlateSecurity":                 "plate",
}

func (a *futuAdapter) QueryMarketMicrostructure(ctx context.Context, query broker.FeatureQuery) (*broker.FeatureResult, error) {
	if query.FeatureID == broker.FeatureMarketDepth {
		return a.queryDepthFeature(ctx, query)
	}
	return a.queryAdvancedFeature(ctx, query)
}

func (a *futuAdapter) QueryInstrumentProfile(ctx context.Context, query broker.FeatureQuery) (*broker.FeatureResult, error) {
	switch query.FeatureID {
	case broker.FeatureMarketSearch:
		return a.queryInstrumentSearchFeature(ctx, query)
	case broker.FeatureInstrumentProfile:
		return a.queryInstrumentSnapshotFeature(ctx, query)
	default:
		return a.queryAdvancedFeature(ctx, query)
	}
}

func (a *futuAdapter) QueryDerivativeCatalog(ctx context.Context, query broker.FeatureQuery) (*broker.FeatureResult, error) {
	return a.queryAdvancedFeature(ctx, query)
}

func (a *futuAdapter) QueryOptionAnalytics(ctx context.Context, query broker.FeatureQuery) (*broker.FeatureResult, error) {
	return a.queryAdvancedFeature(ctx, query)
}

func (a *futuAdapter) QueryInstrumentResearch(ctx context.Context, query broker.FeatureQuery) (*broker.FeatureResult, error) {
	return a.queryAdvancedFeature(ctx, query)
}

func (a *futuAdapter) QueryMarketResearch(ctx context.Context, query broker.FeatureQuery) (*broker.FeatureResult, error) {
	return a.queryAdvancedFeature(ctx, query)
}

func (a *futuAdapter) QueryPredictionMarket(ctx context.Context, query broker.FeatureQuery) (*broker.FeatureResult, error) {
	if !strings.EqualFold(query.Market, "US") {
		return nil, fmt.Errorf("futu: prediction market is available only in the US market")
	}
	return a.queryAdvancedFeature(ctx, query)
}

func (a *futuAdapter) SubscribePredictionMarket(ctx context.Context, subscription broker.PredictionSubscription) error {
	return a.updatePredictionSubscription(ctx, subscription, true)
}

func (a *futuAdapter) UnsubscribePredictionMarket(ctx context.Context, subscription broker.PredictionSubscription) error {
	return a.updatePredictionSubscription(ctx, subscription, false)
}

func (a *futuAdapter) updatePredictionSubscription(
	ctx context.Context,
	subscription broker.PredictionSubscription,
	subscribe bool,
) error {
	params, err := predictionSubscriptionParams(subscription, subscribe)
	if err != nil {
		return err
	}
	err = a.exchange.withRetryingClient(ctx, func(client *opend.Client) error {
		if ensureErr := a.ensurePredictionPushHandlers(ctx, client); ensureErr != nil {
			return ensureErr
		}
		_, callErr := client.CallAdvanced(ctx, "Qot_SubEventContract", params)
		return callErr
	})
	if err != nil {
		return err
	}
	key := predictionSubscriptionKey(subscription)
	a.predictionStreamMu.Lock()
	if subscribe {
		subscription.InstrumentID = strings.ToUpper(strings.TrimSpace(subscription.InstrumentID))
		subscription.DataTypes = append([]string(nil), subscription.DataTypes...)
		a.predictionSubscriptions[key] = subscription
	} else {
		delete(a.predictionSubscriptions, key)
	}
	a.predictionStreamMu.Unlock()
	return nil
}

func predictionSubscriptionParams(
	subscription broker.PredictionSubscription,
	subscribe bool,
) (map[string]any, error) {
	code := strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(subscription.InstrumentID)), "US.")
	if code == "" {
		return nil, fmt.Errorf("futu: event contract code is required")
	}
	subTypes := make([]any, 0, len(subscription.DataTypes))
	klineSources := make([]any, 0, 1)
	for _, dataType := range subscription.DataTypes {
		switch strings.ToUpper(strings.TrimSpace(dataType)) {
		case "ORDER_BOOK":
			subTypes = append(subTypes, 2)
		case "KLINE":
			subTypes = append(subTypes, 11)
			klineSources = append(klineSources, 1)
		case "TICKER":
			subTypes = append(subTypes, 4)
		default:
			return nil, fmt.Errorf("futu: unsupported event contract subscription type %q", dataType)
		}
	}
	params := map[string]any{
		"securityList":     []any{map[string]any{"market": 101, "code": code}},
		"subTypeList":      subTypes,
		"isSubOrUnSub":     subscribe,
		"isRegOrUnRegPush": subscribe,
		"isFirstPush":      subscribe,
	}
	if len(klineSources) > 0 {
		params["klineSource"] = klineSources
	}
	return params, nil
}

func (a *futuAdapter) QueryTechnicalIndicator(ctx context.Context, query broker.FeatureQuery) (*broker.FeatureResult, error) {
	return a.queryAdvancedFeature(ctx, query)
}

func (a *futuAdapter) QueryCustomization(ctx context.Context, query broker.FeatureQuery) (*broker.FeatureResult, error) {
	if query.FeatureID == broker.FeatureRemoteWatchlistList {
		return a.queryRemoteWatchlistFeature(ctx, query)
	}
	return a.queryAdvancedFeature(ctx, query)
}

func (a *futuAdapter) ApplyCustomization(ctx context.Context, action broker.CustomizationAction) (*broker.CustomizationResult, error) {
	query := broker.FeatureQuery{
		BrokerID:  action.BrokerID,
		AccountID: action.AccountID,
		FeatureID: action.FeatureID,
		Params:    cloneMap(action.Payload),
	}
	query.Params["operation"] = action.Action
	result, err := a.queryAdvancedFeature(ctx, query)
	if err != nil {
		return nil, err
	}
	return &broker.CustomizationResult{Provider: result.Provider, Entries: result.Entries}, nil
}

func (a *futuAdapter) queryAdvancedFeature(ctx context.Context, query broker.FeatureQuery) (*broker.FeatureResult, error) {
	protocols := featureProtocols[query.FeatureID]
	if len(protocols) == 0 {
		return nil, fmt.Errorf("futu: feature %q has no OpenD protocol mapping", query.FeatureID)
	}
	if strings.TrimSpace(stringValue(query.Params["operation"])) == "" {
		query.Params = cloneMap(query.Params)
		query.Params["operation"] = defaultFeatureOperations[query.FeatureID]
	}
	return a.queryAdvancedFeatureWithProtocols(ctx, query, protocols)
}

func (a *futuAdapter) queryAdvancedFeatureWithProtocols(
	ctx context.Context,
	query broker.FeatureQuery,
	protocols map[string]string,
) (*broker.FeatureResult, error) {
	operation := strings.TrimSpace(stringValue(query.Params["operation"]))
	if operation == "" {
		operation = defaultOperation(protocols)
	}
	protocol := protocols[operation]
	if protocol == "" {
		return nil, fmt.Errorf("futu: unsupported %s operation %q", query.FeatureID, operation)
	}
	params := cloneMap(query.Params)
	delete(params, "operation")
	if protocol == "Qot_GetEventContractMilestoneList" {
		eventID, err := a.resolvePredictionEventID(ctx, query.InstrumentID)
		if err != nil {
			return nil, err
		}
		query.InstrumentID = eventID
	}
	if protocol == "Qot_GetEventContractComboRfq" {
		if err := translatePredictionComboQuoteParams(params); err != nil {
			return nil, err
		}
	}
	if query.Cursor != "" {
		injectAdvancedCursor(params, protocol, query.Cursor)
	}
	if query.PageSize > 0 {
		injectAdvancedPageSize(params, protocol, query.PageSize)
	}
	if err := injectFeatureInstrument(params, protocol, query.InstrumentID); err != nil {
		return nil, err
	}
	if err := injectAdvancedDefaults(params, protocol, query); err != nil {
		return nil, err
	}
	if protocol == earningsCalendarProtocol {
		return a.queryEarningsCalendarFeature(ctx, query, params)
	}

	var payload map[string]any
	if err := a.withAdvancedClient(ctx, protocol, func(client *opend.Client) error {
		if protocol == "Qot_StockScreen" {
			if retryAfter := a.researchScreenRetryAfter(client); retryAfter > 0 {
				return broker.NewResearchScreenRateLimitError(retryAfter)
			}
		}
		if strings.Contains(protocol, "EventContract") {
			if err := a.ensurePredictionPushHandlers(ctx, client); err != nil {
				return err
			}
		}
		var err error
		payload, err = client.CallAdvanced(ctx, protocol, params)
		if err == nil && protocol == "Qot_StockScreen" {
			err = resolveStockScreenIdentities(ctx, client, query, payload)
		}
		return err
	}); err != nil {
		return nil, err
	}
	result := featureResultFromProtocolPayload(query, protocol, payload)
	if err := applyResearchLocalPagination(result, query, protocol); err != nil {
		return nil, err
	}
	return result, nil
}

func (a *futuAdapter) withAdvancedClient(
	ctx context.Context,
	protocol string,
	fn func(*opend.Client) error,
) error {
	if advancedProtocolReplaySafe(protocol) {
		return a.exchange.withRetryingClient(ctx, fn)
	}
	return a.exchange.withClient(ctx, fn)
}

func advancedProtocolReplaySafe(protocol string) bool {
	// Despite its Get prefix, an RFQ request creates a short-lived quote and is
	// not safe to duplicate when the first response outcome is unknown.
	if protocol == "Qot_GetEventContractComboRfq" {
		return false
	}
	if strings.HasPrefix(protocol, "Qot_Get") ||
		strings.HasPrefix(protocol, "Qot_Request") ||
		strings.HasPrefix(protocol, "Qot_Filter") {
		return true
	}
	switch protocol {
	case "Qot_OptionScreen", "Qot_WarrantScreen", "Qot_StockFilter", "Qot_StockScreen", "Qot_SubEventContract":
		return true
	default:
		return false
	}
}

func injectAdvancedCursor(params map[string]any, protocol, cursor string) {
	for _, field := range []string{"nextPage", "page", "nextKey"} {
		if opend.AdvancedC2SHasField(protocol, field) {
			if _, exists := params[field]; !exists {
				params[field] = cursor
			}
			return
		}
	}
}

func injectAdvancedPageSize(params map[string]any, protocol string, pageSize int) {
	pageSize = min(max(pageSize, 1), advancedPageSizeLimit(protocol))
	for _, field := range []string{"count", "pageCount", "num", "maxCount", "maxRetNum"} {
		if opend.AdvancedC2SHasField(protocol, field) {
			if _, exists := params[field]; !exists {
				params[field] = pageSize
			}
			return
		}
	}
}

func advancedPageSizeLimit(protocol string) int {
	switch protocol {
	case "Qot_GetIndustrialChainList":
		// OpenD rejects count outside [1, 50]. Keep the protocol limit at the
		// adapter boundary so every caller, not just the research UI, is safe.
		return 50
	default:
		return 100
	}
}

func injectAdvancedDefaults(params map[string]any, protocol string, query broker.FeatureQuery) error {
	marketValue, err := futuQotMarketForCode(query.Market)
	if err != nil && query.Market != "" {
		return err
	}
	if opend.AdvancedC2SHasField(protocol, "market") && params["market"] == nil {
		params["market"] = int32(marketValue)
	}
	if opend.AdvancedC2SHasField(protocol, "offset") && params["offset"] == nil {
		params["offset"] = 0
	}
	if opend.AdvancedC2SHasField(protocol, "pageFrom") && params["pageFrom"] == nil {
		params["pageFrom"] = 0
	}
	return injectAdvancedProtocolDefaults(params, protocol, query)
}

func injectOptionChainDates(params map[string]any) {
	now := time.Now()
	if params["beginTime"] == nil {
		params["beginTime"] = now.Format("2006-01-02")
	}
	if params["endTime"] == nil {
		params["endTime"] = now.AddDate(0, 0, 30).Format("2006-01-02")
	}
}

func injectOptionMarketStatisticDefaults(params map[string]any, market string) {
	now := time.Now()
	if params["optionMarket"] == nil {
		params["optionMarket"] = 1
		if strings.EqualFold(market, "HK") {
			params["optionMarket"] = 3
		}
	}
	if params["dataType"] == nil {
		params["dataType"] = 0
	}
	if params["beginTime"] == nil {
		params["beginTime"] = now.AddDate(0, -1, 0).Format("2006-01-02")
	}
	if params["endTime"] == nil {
		params["endTime"] = now.Format("2006-01-02")
	}
}

func injectHistoricalDateRange(params map[string]any) {
	now := time.Now()
	if params["beginTime"] == nil {
		params["beginTime"] = now.AddDate(-1, 0, 0).Format("2006-01-02")
	}
	if params["endTime"] == nil {
		params["endTime"] = now.Format("2006-01-02")
	}
}

func injectWarrantDefaults(params map[string]any) {
	if params["begin"] == nil {
		params["begin"] = 0
	}
	if params["sortField"] == nil {
		params["sortField"] = 12
	}
	if params["ascend"] == nil {
		params["ascend"] = false
	}
}

func macroRegion(market string) int {
	switch strings.ToUpper(strings.TrimSpace(market)) {
	case "HK":
		return 1
	case "SH", "SZ":
		return 8
	default:
		return 2
	}
}

func (a *futuAdapter) queryDepthFeature(ctx context.Context, query broker.FeatureQuery) (*broker.FeatureResult, error) {
	num := int32(numberValue(query.Params["num"], 10))
	snapshot, err := a.MarketData().QueryOrderBook(ctx, broker.OrderBookQuery{
		ReadQuery: broker.ReadQuery{AccountID: query.AccountID, BrokerID: query.BrokerID, Market: query.Market},
		Symbol:    query.InstrumentID,
		Num:       num,
	})
	if err != nil {
		return nil, err
	}
	entry := jsonSafeStructMap(snapshot)
	return featureResult(query, []map[string]any{entry}, nil), nil
}

func (a *futuAdapter) queryInstrumentSearchFeature(ctx context.Context, query broker.FeatureQuery) (*broker.FeatureResult, error) {
	keyword := stringValue(query.Params["keyword"])
	if keyword == "" {
		// Keep the adapter tolerant of older broker-neutral callers while the
		// public API and tool contract consistently use `query`.
		keyword = stringValue(query.Params["query"])
	}
	snapshot, err := a.MarketData().QuerySecuritySearch(ctx, broker.SecuritySearchQuery{
		ReadQuery: broker.ReadQuery{AccountID: query.AccountID, BrokerID: query.BrokerID, Market: query.Market},
		Keyword:   keyword,
		Limit:     int32(min(query.PageSize, 100)),
	})
	if err != nil {
		return nil, err
	}
	entries := make([]map[string]any, 0, len(snapshot.Entries))
	for _, item := range snapshot.Entries {
		entries = append(entries, jsonSafeStructMap(item))
	}
	return featureResult(query, entries, nil), nil
}

func (a *futuAdapter) queryInstrumentSnapshotFeature(ctx context.Context, query broker.FeatureQuery) (*broker.FeatureResult, error) {
	if strings.TrimSpace(query.InstrumentID) == "" {
		return nil, fmt.Errorf("futu: instrumentId is required")
	}
	if query.MarketSegment == broker.MarketSegmentPrediction ||
		query.ProductClass == broker.ProductClassEventContract {
		query.ProductClass = broker.ProductClassEventContract
		query.MarketSegment = broker.MarketSegmentPrediction
		query.Params = cloneMap(query.Params)
		query.Params["operation"] = "snapshot"
		return a.queryAdvancedFeatureWithProtocols(
			ctx,
			query,
			featureProtocols[broker.FeaturePredictionSnapshot],
		)
	}
	details, err := a.exchange.QuerySecurityDetails(ctx, query.InstrumentID)
	if err != nil {
		return nil, err
	}
	entry := jsonSafeStructMap(details)
	return featureResult(query, []map[string]any{entry}, nil), nil
}

func (a *futuAdapter) queryRemoteWatchlistFeature(ctx context.Context, query broker.FeatureQuery) (*broker.FeatureResult, error) {
	groups, err := a.ListWatchlistGroups(ctx)
	if err != nil {
		return nil, err
	}
	entries := make([]map[string]any, 0, len(groups))
	for _, group := range groups {
		entries = append(entries, jsonSafeStructMap(group))
	}
	return featureResult(query, entries, nil), nil
}

func featureResultFromPayload(query broker.FeatureQuery, payload map[string]any) *broker.FeatureResult {
	result := featureResultFromNormalizedPayload(query, normalizeOpenDMap(payload))
	// Compatibility helper retained for adapter unit fixtures. Production RFQ
	// responses use featureResultFromProtocolPayload and receive their
	// authoritative expiry only after the business service persists the quote.
	if query.FeatureID == broker.FeaturePredictionComboQuote {
		if result.Metadata == nil {
			result.Metadata = make(map[string]any)
		}
		result.Metadata["quoteExpiresAt"] = time.Now().UTC().Add(30 * time.Second).Format(time.RFC3339Nano)
		result.Warnings = append(result.Warnings, "Parlay RFQ is short-lived and must be refreshed after expiry.")
	}
	return result
}

func (a *futuAdapter) resolvePredictionEventID(
	ctx context.Context,
	instrumentID string,
) (string, error) {
	code := strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(instrumentID)), "US.")
	if code == "" {
		return "", fmt.Errorf("futu: prediction contract code is required for milestones")
	}
	var payload map[string]any
	err := a.exchange.withRetryingClient(ctx, func(client *opend.Client) error {
		var callErr error
		payload, callErr = client.CallAdvanced(ctx, "Qot_GetEventContractSnapshot", map[string]any{
			"securityList": []any{map[string]any{"market": 101, "code": code}},
		})
		return callErr
	})
	if err != nil {
		return "", fmt.Errorf("futu: resolve prediction event for %s: %w", code, err)
	}
	entries := objectSlice(normalizeOpenDMap(payload)["snapshotList"])
	if len(entries) == 0 {
		return "", fmt.Errorf("futu: prediction contract %s has no snapshot", code)
	}
	eventID := securityInstrumentID(entries[0]["eventCode"])
	if eventID == "" {
		eventID = securityInstrumentID(entries[0]["event_code"])
	}
	if eventID == "" {
		return "", fmt.Errorf("futu: prediction contract %s has no owning event", code)
	}
	return eventID, nil
}

var (
	_ broker.MarketMicrostructureReader = (*futuAdapter)(nil)
	_ broker.InstrumentProfileReader    = (*futuAdapter)(nil)
	_ broker.DerivativeCatalogReader    = (*futuAdapter)(nil)
	_ broker.OptionAnalyticsReader      = (*futuAdapter)(nil)
	_ broker.InstrumentResearchReader   = (*futuAdapter)(nil)
	_ broker.MarketResearchReader       = (*futuAdapter)(nil)
	_ broker.PredictionMarketReader     = (*futuAdapter)(nil)
	_ broker.TechnicalIndicatorReader   = (*futuAdapter)(nil)
	_ broker.CustomizationService       = (*futuAdapter)(nil)
)
