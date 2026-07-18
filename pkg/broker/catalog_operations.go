package broker

import (
	"net/url"
	"slices"
	"strings"
)

type catalogProtocolSpec struct {
	key  string
	id   uint32
	kind string
}

func capabilityTestID(id FeatureID) string {
	return "TestCapabilityOperationContracts/" + string(id)
}

func capabilityOperations(
	featureID FeatureID,
	defaultMethod, api, ui, tool, testID string,
) []CapabilityOperation {
	method := capabilityHTTPMethod(featureID, defaultMethod)
	surfaceID := capabilityUISurfaceID(ui)
	specs := capabilityProtocolSpecs[featureID]
	if len(specs) == 0 {
		return []CapabilityOperation{{
			ID: string(featureID), HTTPMethod: method, API: api,
			UISurfaceID: surfaceID, Tool: tool, TestID: testID,
		}}
	}
	operationIDs := make([]string, 0, len(specs))
	for operationID := range specs {
		operationIDs = append(operationIDs, operationID)
	}
	slices.Sort(operationIDs)
	operations := make([]CapabilityOperation, 0, len(specs))
	for _, operationID := range operationIDs {
		protocols := specs[operationID]
		operationMethod := method
		operationAPI := api
		operationSurfaceID := surfaceID
		operationTool := tool
		if override, ok := capabilityOperationSurfaceOverrides[featureID][operationID]; ok {
			if override.method != "" {
				operationMethod = override.method
			}
			if override.api != "" {
				operationAPI = override.api
			}
			if override.ui != "" {
				operationSurfaceID = capabilityUISurfaceID(override.ui)
			}
			if override.noUI {
				operationSurfaceID = ""
			}
			if override.noTool {
				operationTool = ""
			}
		}
		refs := make([]CapabilityProtocol, 0, len(protocols))
		for _, protocol := range protocols {
			refs = append(refs, CapabilityProtocol{
				BrokerID: "futu", Key: protocol.key, ID: protocol.id, Kind: protocol.kind,
			})
		}
		operations = append(operations, CapabilityOperation{
			ID: operationID, HTTPMethod: operationMethod, API: operationAPI,
			UISurfaceID: operationSurfaceID, Tool: operationTool,
			Protocols: refs, TestID: testID + "/" + operationID,
		})
	}
	return operations
}

func capabilityHTTPMethod(id FeatureID, fallback string) string {
	switch id {
	case FeatureMarketSnapshots, FeaturePredictionComboQuote, FeatureExecutionBuyingPower:
		return "POST"
	default:
		return fallback
	}
}

func capabilityUISurfaceID(route string) string {
	normalized := strings.TrimSpace(route)
	if normalized == "" {
		return ""
	}
	parsed, err := url.Parse(normalized)
	if err == nil {
		switch parsed.Path {
		case "/workspace":
			if tab := strings.TrimSpace(parsed.Query().Get("tab")); tab != "" {
				return "workspace." + tab
			}
			if surface := strings.TrimSpace(parsed.Query().Get("surface")); surface != "" {
				return "workspace." + surface
			}
			return "workspace.root"
		case "/research":
			if section := strings.TrimSpace(parsed.Query().Get("section")); section != "" {
				return "research." + section
			}
			return "research.market"
		case "/watchlist":
			return "watchlist.root"
		}
	}
	normalized = strings.TrimPrefix(normalized, "/")
	normalized = strings.ReplaceAll(normalized, "/", ".")
	normalized = strings.ReplaceAll(normalized, "?", ".")
	normalized = strings.ReplaceAll(normalized, "=", ".")
	if normalized == "" {
		return "app.root"
	}
	return normalized
}

type capabilityOperationSurfaceOverride struct {
	method string
	api    string
	ui     string
	noUI   bool
	noTool bool
}

var capabilityOperationSurfaceOverrides = map[FeatureID]map[string]capabilityOperationSurfaceOverride{
	FeatureOptionChain: {
		"expirations": {api: "/api/v1/market-data/options/expirations/{instrumentId}"},
	},
	FeatureOptionAnalysis: {
		"strategy":          {method: "POST"},
		"strategy_analysis": {method: "POST"},
		"strategy_spread":   {method: "POST"},
	},
	FeatureOptionEvents: {
		"zero_dte_contract": {
			method: "POST",
			api:    "/api/v1/market-data/options/events/zero-dte-contracts",
		},
	},
	FeaturePredictionDiscover: {
		"categories":   {api: "/api/v1/market-data/prediction/categories"},
		"competitions": {api: "/api/v1/market-data/prediction/competitions"},
		"series":       {api: "/api/v1/market-data/prediction/series"},
		"events":       {api: "/api/v1/market-data/prediction/events"},
		"contracts":    {api: "/api/v1/market-data/prediction/events/{eventId}/contracts"},
		"milestones": {
			api: "/api/v1/market-data/prediction/contracts/{code}/milestones",
			ui:  "/workspace?tab=rules&marketSegment=prediction",
		},
	},
	FeaturePredictionHistory: {
		"candles": {
			api: "/api/v1/market-data/prediction/contracts/{code}/candles",
			ui:  "/workspace?tab=chart&marketSegment=prediction",
		},
		"historical": {
			api: "/api/v1/market-data/prediction/contracts/{code}/candles/history",
			ui:  "/workspace?tab=chart&marketSegment=prediction",
		},
		"ticks": {
			api: "/api/v1/market-data/prediction/contracts/{code}/ticks",
			ui:  "/workspace?tab=ticks&marketSegment=prediction",
		},
		"subscribe": {
			method: "POST",
			api:    "/api/v1/market-data/prediction/contracts/{code}/subscriptions",
			ui:     "/workspace?tab=chart&marketSegment=prediction",
			noTool: true,
		},
	},
}

func request(key string, id uint32) catalogProtocolSpec {
	return catalogProtocolSpec{key: key, id: id, kind: "request"}
}

func push(key string, id uint32) catalogProtocolSpec {
	return catalogProtocolSpec{key: key, id: id, kind: "push"}
}

// capabilityProtocolSpecs deliberately duplicates the public OpenD wire
// mapping at the broker-neutral catalog boundary. Futu adapter tests compare
// this manifest with its dispatcher so adding a protocol without an operation,
// API, UI/tool surface and test mapping cannot silently pass CI.
var capabilityProtocolSpecs = map[FeatureID]map[string][]catalogProtocolSpec{
	FeatureMarketSearch: {
		"search": {request("Qot_GetSearchQuote", 3262)},
	},
	FeatureInstrumentProfile: {
		"profile": {request("Qot_GetStaticInfo", 3202), request("Qot_GetSecuritySnapshot", 3203)},
	},
	FeatureMarketSnapshot: {
		"snapshot": {request("Qot_GetSecuritySnapshot", 3203)},
	},
	FeatureMarketSnapshots: {
		"snapshots": {request("Qot_GetSecuritySnapshot", 3203)},
	},
	FeatureMarketCandles: {
		"current":    {request("Qot_GetKL", 3006), push("Qot_UpdateKL", 3007)},
		"historical": {request("Qot_RequestHistoryKL", 3103)},
	},
	FeatureMarketIntraday: {
		"intraday": {request("Qot_GetRT", 3008)},
	},
	FeatureMarketTicks: {
		"ticks": {request("Qot_GetTicker", 3010)},
	},
	FeatureMarketDepth: {
		"depth": {request("Qot_GetOrderBook", 3012), push("Qot_UpdateOrderBook", 3013)},
	},
	FeatureMarketBrokerQueue: {
		"broker_queue": {request("Qot_GetBroker", 3014)},
	},
	FeatureMarketCapitalFlow: {
		"flow":         {request("Qot_GetCapitalFlow", 3211)},
		"distribution": {request("Qot_GetCapitalDistribution", 3212)},
	},
	FeatureOptionChain: {
		"expirations": {request("Qot_GetOptionExpirationDate", 3224)},
		"chain":       {request("Qot_GetOptionChain", 3209)},
	},
	FeatureOptionScreen: {
		"screen": {request("Qot_OptionScreen", 3253)},
	},
	FeatureOptionAnalysis: {
		"quote":                 {request("Qot_GetOptionQuote", 3255)},
		"volatility":            {request("Qot_GetOptionVolatility", 3250)},
		"exercise_probability":  {request("Qot_GetOptionExerciseProbability", 3251)},
		"strategy":              {request("Qot_GetOptionStrategy", 3256)},
		"strategy_analysis":     {request("Qot_GetOptionStrategyAnalysis", 3257)},
		"strategy_spread":       {request("Qot_GetOptionStrategySpread", 3258)},
		"market_statistics":     {request("Qot_GetOptionMarketStatistic", 3301)},
		"historical_statistics": {request("Qot_GetOptionUnderlyingHisStatistic", 3302)},
		"underlying_overview":   {request("Qot_GetOptionUnderlyingOverview", 3303)},
		"historical_volatility": {request("Qot_GetOptionUnderlyingHisVolatility", 3304)},
		"underlying_rank":       {request("Qot_GetOptionUnderlyingRank", 3305)},
		"contract_rank":         {request("Qot_GetOptionRank", 3306)},
	},
	FeatureOptionEvents: {
		"unusual":           {request("Qot_GetOptionEvent", 3307)},
		"zero_dte":          {request("Qot_GetOptionZeroDteScreener", 3311)},
		"zero_dte_contract": {request("Qot_GetOptionZeroDteContract", 3312)},
		"earnings":          {request("Qot_GetOptionEarningsScreener", 3313)},
		"seller":            {request("Qot_GetOptionSellerScreener", 3314)},
	},
	FeatureWarrants: {
		"related": {request("Qot_GetReference", 3206)},
		"list":    {request("Qot_GetWarrant", 3210)},
		"screen":  {request("Qot_WarrantScreen", 3254)},
	},
	FeatureFutures: {
		"contracts": {request("Qot_GetFutureInfo", 3218)},
	},
	FeatureResearchInstrument: {
		"profile":                {request("Qot_GetCompanyProfile", 3243)},
		"executives":             {request("Qot_GetCompanyExecutives", 3244)},
		"executive_background":   {request("Qot_GetCompanyExecutiveBackground", 3245)},
		"operational_efficiency": {request("Qot_GetCompanyOperationalEfficiency", 3246)},
		"top_brokers":            {request("Qot_GetTopTenBuySellBrokers", 3247)},
	},
	FeatureResearchFinancials: {
		"statements":             {request("Qot_GetFinancialsStatements", 3227)},
		"revenue_breakdown":      {request("Qot_GetFinancialsRevenueBreakdown", 3228)},
		"earnings_price_move":    {request("Qot_GetFinancialsEarningsPriceMove", 3225)},
		"earnings_price_history": {request("Qot_GetFinancialsEarningsPriceHistory", 3226)},
	},
	FeatureResearchValuation: {
		"detail":       {request("Qot_GetValuationDetail", 3232)},
		"constituents": {request("Qot_GetValuationPlateStockList", 3233)},
	},
	FeatureResearchAnalyst: {
		"consensus":   {request("Qot_GetResearchAnalystConsensus", 3229)},
		"ratings":     {request("Qot_GetResearchRatingSummary", 3230)},
		"morningstar": {request("Qot_GetResearchMorningstarReport", 3231)},
		"changes":     {request("Qot_GetRatingChange", 3426)},
	},
	FeatureResearchOwnership: {
		"overview":             {request("Qot_GetShareholdersOverview", 3237)},
		"changes":              {request("Qot_GetShareholdersHoldingChanges", 3238)},
		"holders":              {request("Qot_GetShareholdersHolderDetail", 3239)},
		"institutional":        {request("Qot_GetShareholdersInstitutional", 3240)},
		"insider_holders":      {request("Qot_GetInsiderHolderList", 3241)},
		"insider_transactions": {request("Qot_GetInsiderTradeList", 3242)},
		"management_changes":   {request("Qot_GetHoldingChangeList", 3208)},
	},
	FeatureResearchCorporateAction: {
		"dividends":    {request("Qot_GetCorporateActionsDividends", 3234)},
		"buybacks":     {request("Qot_GetCorporateActionsBuybacks", 3235)},
		"splits":       {request("Qot_GetCorporateActionsStockSplits", 3236)},
		"code_changes": {request("Qot_GetCodeChange", 3216)},
	},
	FeatureResearchShortInterest: {
		"daily_volume":   {request("Qot_GetDailyShortVolume", 3248)},
		"short_interest": {request("Qot_GetShortInterest", 3249)},
	},
	FeatureResearchNews: {
		"search": {request("Qot_GetSearchNews", 3263)},
	},
	FeatureResearchScreen: {
		"stock_v1": {request("Qot_StockFilter", 3215)},
		"stock_v2": {request("Qot_StockScreen", 3252)},
	},
	FeatureResearchCalendar: {
		"earnings":    {request("Qot_GetEarningsCalendar", 3401)},
		"dividends":   {request("Qot_GetDividendCalendar", 3408)},
		"economic":    {request("Qot_GetEconomicCalendar", 3409)},
		"ipos":        {request("Qot_GetIpoList", 3217)},
		"trade_dates": {request("Qot_RequestTradeDate", 3219)},
	},
	FeatureResearchMacro: {
		"indicators":        {request("Qot_GetMacroIndicatorList", 3402)},
		"indicator_history": {request("Qot_GetMacroIndicatorHistory", 3403)},
		"fed_target_rate":   {request("Qot_GetFedWatchTargetRate", 3404)},
		"fed_dot_plot":      {request("Qot_GetFedWatchDotPlot", 3405)},
	},
	FeatureResearchRankings: {
		"earnings_beat":          {request("Qot_GetEarningsBeatRank", 3406)},
		"dividend":               {request("Qot_GetDividendRank", 3407)},
		"pre_market":             {request("Qot_GetUSPreMarketRank", 3410)},
		"after_hours":            {request("Qot_GetUSAfterHoursRank", 3411)},
		"overnight":              {request("Qot_GetUSOvernightRank", 3412)},
		"top_movers":             {request("Qot_GetTopMoversRank", 3413)},
		"hot":                    {request("Qot_GetHotList", 3414)},
		"short_selling":          {request("Qot_GetShortSellingRank", 3415)},
		"period_change":          {request("Qot_GetPeriodChangeRank", 3416)},
		"high_dividend_state":    {request("Qot_GetHighDividendSOERank", 3417)},
		"heatmap":                {request("Qot_GetHeatMapData", 3432)},
		"rise_fall_distribution": {request("Qot_GetRiseFallDistribution", 3433)},
		"market_state":           {request("Qot_GetMarketState", 3223)},
	},
	FeatureResearchInstitutions: {
		"list":               {request("Qot_GetInstitutionList", 3418)},
		"profile":            {request("Qot_GetInstitutionProfile", 3419)},
		"distribution":       {request("Qot_GetInstitutionDistribution", 3420)},
		"holding_changes":    {request("Qot_GetInstitutionHoldingChange", 3421)},
		"holdings":           {request("Qot_GetInstitutionHoldingList", 3422)},
		"ark_fund_holdings":  {request("Qot_GetArkFundHolding", 3423)},
		"ark_stock_activity": {request("Qot_GetArkStockDynamic", 3424)},
		"ark_transactions":   {request("Qot_GetArkActiveTransaction", 3425)},
	},
	FeatureResearchIndustry: {
		"chains":          {request("Qot_GetIndustrialChainList", 3427)},
		"chain_detail":    {request("Qot_GetIndustrialChainDetail", 3428)},
		"chains_by_plate": {request("Qot_GetIndustrialChainByPlate", 3429)},
		"plate":           {request("Qot_GetIndustrialPlateInfo", 3430)},
		"plate_stocks":    {request("Qot_GetIndustrialPlateStock", 3431)},
		"owner_plates":    {request("Qot_GetOwnerPlate", 3207)},
	},
	FeatureTechnicalIndicator: {
		"list":      {request("Qot_GetIndicatorList", 3259)},
		"calculate": {request("Qot_RequestIndicatorCalc", 3260)},
	},
	FeaturePredictionDiscover: {
		"categories":   {request("Qot_GetEventContractCategory", 3434)},
		"competitions": {request("Qot_FilterCompetition", 3435)},
		"series":       {request("Qot_GetEventContractSeriesList", 3436)},
		"events":       {request("Qot_GetEventContractEventList", 3437)},
		"contracts":    {request("Qot_GetEventContract", 3438)},
		"milestones":   {request("Qot_GetEventContractMilestoneList", 3439)},
	},
	FeaturePredictionSnapshot: {
		"snapshot": {request("Qot_GetEventContractSnapshot", 3445)},
	},
	FeaturePredictionDepth: {
		"order_book": {request("Qot_GetEventContractOrderBook", 3446), push("Qot_UpdateEventContractOrderBook", 3450)},
	},
	FeaturePredictionHistory: {
		"candles":    {request("Qot_GetEventContractKline", 3447), push("Qot_UpdateEventContractKline", 3451)},
		"ticks":      {request("Qot_GetEventContractTicker", 3448), push("Qot_UpdateEventContractTicker", 3452)},
		"historical": {request("Qot_RequestHistoryEventContractKL", 3456)},
		"subscribe":  {request("Qot_SubEventContract", 3455)},
	},
	FeaturePredictionComboEligible: {
		"eligible_events": {request("Qot_GetEventContractComboList", 3453)},
	},
	FeaturePredictionComboQuote: {
		"quote": {request("Qot_GetEventContractComboRfq", 3454)},
	},
	FeatureExecutionOrderPreview: {
		"rules": {request("Trd_GetMaxTrdQtys", 2111)},
	},
	FeatureExecutionOrderPlace: {
		"place": {request("Trd_PlaceOrder", 2202), push("Trd_UpdateOrder", 2208), push("Trd_UpdateOrderFill", 2218)},
	},
	FeatureExecutionOrderCancel: {
		"cancel": {request("Trd_ModifyOrder", 2205)},
	},
	FeatureExecutionComboPreview: {
		"legality": {
			request("Qot_GetOptionStrategy", 3256),
			request("Qot_GetOptionStrategyAnalysis", 3257),
			request("Qot_GetOptionStrategySpread", 3258),
		},
		"buying_power": {request("Trd_GetComboMaxTrdQtys", 2112)},
	},
	FeatureExecutionComboPlace: {
		"place": {request("Trd_PlaceComboOrder", 2227), push("Trd_UpdateOrder", 2208), push("Trd_UpdateOrderFill", 2218)},
	},
	FeatureExecutionComboCancel: {
		"cancel": {request("Trd_ModifyOrder", 2205)},
	},
	FeatureExecutionBuyingPower: {
		"single": {request("Trd_GetMaxTrdQtys", 2111)},
		"combo":  {request("Trd_GetComboMaxTrdQtys", 2112)},
	},
	FeaturePriceAlertList: {
		"list": {request("Qot_GetPriceReminder", 3221)},
	},
	FeaturePriceAlertSet: {
		"set": {request("Qot_SetPriceReminder", 3220)},
	},
	FeatureOptionEventAlertList: {
		"list": {request("Qot_GetOptionEventAlert", 3308)},
	},
	FeatureOptionEventAlertSet: {
		"set": {request("Qot_SetOptionEventAlert", 3309)},
	},
	FeatureRemoteWatchlistList: {
		"groups":  {request("Qot_GetUserSecurityGroup", 3222)},
		"members": {request("Qot_GetUserSecurity", 3213)},
	},
	FeatureRemoteWatchlistModify: {
		"modify": {request("Qot_ModifyUserSecurity", 3214)},
	},
}
