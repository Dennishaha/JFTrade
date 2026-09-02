//! Operation-level capability metadata shared by the production broker
//! projection.  This mirrors the operation/protocol map in
//! `pkg/broker/catalog_operations.go`; the values are catalog metadata only
//! and never bypass the concrete Rust adapter.

use serde_json::{Value, json};

#[derive(Clone, Copy)]
struct Protocol {
    key: &'static str,
    id: u32,
    kind: &'static str,
}

const fn request(key: &'static str, id: u32) -> Protocol {
    Protocol { key, id, kind: "request" }
}

const fn push(key: &'static str, id: u32) -> Protocol {
    Protocol { key, id, kind: "push" }
}

#[allow(clippy::too_many_arguments)]
fn operation(
    feature_id: &str,
    operation_id: &str,
    default_method: &str,
    default_api: &str,
    default_ui: &str,
    default_tool: &str,
    method_override: Option<&str>,
    api_override: Option<&str>,
    ui_override: Option<&str>,
    no_tool: bool,
    protocols: &[Protocol],
) -> Value {
    let method = method_override.unwrap_or(default_method);
    let api = api_override.unwrap_or(default_api);
    let ui = ui_override.unwrap_or(default_ui);
    let mut value = json!({
        "id": operation_id,
        "httpMethod": method,
        "api": api,
        "uiSurfaceId": super::ui_surface_id(ui),
        "testId": format!("TestCapabilityOperationContracts/{feature_id}/{operation_id}"),
    });
    if !no_tool && !default_tool.is_empty() {
        value["tool"] = Value::String(default_tool.to_owned());
    }
    if ui.is_empty() {
        value["uiSurfaceId"] = Value::String(String::new());
    }
    if !protocols.is_empty() {
        value["protocols"] = Value::Array(
            protocols
                .iter()
                .map(|protocol| {
                    json!({
                        "brokerId": "futu",
                        "key": protocol.key,
                        "id": protocol.id,
                        "kind": protocol.kind,
                    })
                })
                .collect(),
        );
    }
    value
}

pub(super) fn catalog_operations(
    feature_id: &str,
    default_method: &str,
    default_api: &str,
    default_ui: &str,
    default_tool: &str,
) -> Vec<Value> {
    macro_rules! op {
        ($id:literal, [$($protocol:expr),* $(,)?]) => {
            operation(feature_id, $id, default_method, default_api, default_ui, default_tool, None, None, None, false, &[$($protocol),*])
        };
        ($id:literal, method = $method:literal, [$($protocol:expr),* $(,)?]) => {
            operation(feature_id, $id, default_method, default_api, default_ui, default_tool, Some($method), None, None, false, &[$($protocol),*])
        };
        ($id:literal, api = $api:literal, [$($protocol:expr),* $(,)?]) => {
            operation(feature_id, $id, default_method, default_api, default_ui, default_tool, None, Some($api), None, false, &[$($protocol),*])
        };
        ($id:literal, ui = $ui:literal, [$($protocol:expr),* $(,)?]) => {
            operation(feature_id, $id, default_method, default_api, default_ui, default_tool, None, None, Some($ui), false, &[$($protocol),*])
        };
        ($id:literal, method = $method:literal, api = $api:literal, ui = $ui:literal, no_tool, [$($protocol:expr),* $(,)?]) => {
            operation(feature_id, $id, default_method, default_api, default_ui, default_tool, Some($method), Some($api), Some($ui), true, &[$($protocol),*])
        };
        ($id:literal, method = $method:literal, api = $api:literal, [$($protocol:expr),* $(,)?], no_tool) => {
            operation(feature_id, $id, default_method, default_api, default_ui, default_tool, Some($method), Some($api), None, true, &[$($protocol),*])
        };
        ($id:literal, method = $method:literal, api = $api:literal, [$($protocol:expr),* $(,)?]) => {
            operation(feature_id, $id, default_method, default_api, default_ui, default_tool, Some($method), Some($api), None, false, &[$($protocol),*])
        };
        ($id:literal, api = $api:literal, ui = $ui:literal, [$($protocol:expr),* $(,)?]) => {
            operation(feature_id, $id, default_method, default_api, default_ui, default_tool, None, Some($api), Some($ui), false, &[$($protocol),*])
        };
        ($id:literal, api = $api:literal, [$($protocol:expr),* $(,)?], no_tool) => {
            operation(feature_id, $id, default_method, default_api, default_ui, default_tool, None, Some($api), None, true, &[$($protocol),*])
        };
    }

    match feature_id {
        "market.search" => vec![op!("search", [request("Qot_GetSearchQuote", 3262)])],
        "market.instrument_profile" => {
            vec![op!("profile", [request("Qot_GetStaticInfo", 3202), request("Qot_GetSecuritySnapshot", 3203)])]
        }
        "market.snapshot" => vec![op!("snapshot", [request("Qot_GetSecuritySnapshot", 3203)])],
        "market.snapshots" => vec![op!("snapshots", [request("Qot_GetSecuritySnapshot", 3203)])],
        "market.candles" => vec![
            op!("current", [request("Qot_GetKL", 3006), push("Qot_UpdateKL", 3007)]),
            op!("historical", [request("Qot_RequestHistoryKL", 3103)]),
        ],
        "market.intraday" => vec![op!("intraday", [request("Qot_GetRT", 3008)])],
        "market.ticks" => vec![op!("ticks", [request("Qot_GetTicker", 3010)])],
        "market.depth" => vec![op!("depth", [request("Qot_GetOrderBook", 3012), push("Qot_UpdateOrderBook", 3013)])],
        "market.broker_queue" => vec![op!("broker_queue", [request("Qot_GetBroker", 3014)])],
        "market.capital_flow" => vec![
            op!("distribution", [request("Qot_GetCapitalDistribution", 3212)]),
            op!("flow", [request("Qot_GetCapitalFlow", 3211)]),
        ],
        "derivatives.option_chain" => vec![
            op!("chain", [request("Qot_GetOptionChain", 3209)]),
            op!("expirations", api = "/api/v1/market-data/options/expirations/{instrumentId}", [request("Qot_GetOptionExpirationDate", 3224)]),
        ],
        "derivatives.option_screen" => vec![op!("screen", [request("Qot_OptionScreen", 3253)])],
        "derivatives.option_analysis" => vec![
            op!("contract_rank", [request("Qot_GetOptionRank", 3306)]),
            op!("exercise_probability", [request("Qot_GetOptionExerciseProbability", 3251)]),
            op!("historical_statistics", [request("Qot_GetOptionUnderlyingHisStatistic", 3302)]),
            op!("historical_volatility", [request("Qot_GetOptionUnderlyingHisVolatility", 3304)]),
            op!("market_statistics", [request("Qot_GetOptionMarketStatistic", 3301)]),
            op!("quote", [request("Qot_GetOptionQuote", 3255)]),
            op!("strategy", method = "POST", [request("Qot_GetOptionStrategy", 3256)]),
            op!("strategy_analysis", method = "POST", [request("Qot_GetOptionStrategyAnalysis", 3257)]),
            op!("strategy_spread", method = "POST", [request("Qot_GetOptionStrategySpread", 3258)]),
            op!("underlying_overview", [request("Qot_GetOptionUnderlyingOverview", 3303)]),
            op!("underlying_rank", [request("Qot_GetOptionUnderlyingRank", 3305)]),
            op!("volatility", [request("Qot_GetOptionVolatility", 3250)]),
        ],
        "derivatives.option_events" => vec![
            op!("earnings", [request("Qot_GetOptionEarningsScreener", 3313)]),
            op!("seller", [request("Qot_GetOptionSellerScreener", 3314)]),
            op!("unusual", [request("Qot_GetOptionEvent", 3307)]),
            op!("zero_dte", [request("Qot_GetOptionZeroDteScreener", 3311)]),
            op!("zero_dte_contract", method = "POST", api = "/api/v1/market-data/options/events/zero-dte-contracts", [request("Qot_GetOptionZeroDteContract", 3312)]),
        ],
        "derivatives.warrants" => vec![
            op!("list", [request("Qot_GetWarrant", 3210)]),
            op!("related", [request("Qot_GetReference", 3206)]),
            op!("screen", [request("Qot_WarrantScreen", 3254)]),
        ],
        "derivatives.futures" => vec![op!("contracts", [request("Qot_GetFutureInfo", 3218)])],
        "research.instrument" => vec![
            op!("executive_background", [request("Qot_GetCompanyExecutiveBackground", 3245)]),
            op!("executives", [request("Qot_GetCompanyExecutives", 3244)]),
            op!("operational_efficiency", [request("Qot_GetCompanyOperationalEfficiency", 3246)]),
            op!("profile", [request("Qot_GetCompanyProfile", 3243)]),
            op!("top_brokers", [request("Qot_GetTopTenBuySellBrokers", 3247)]),
        ],
        "research.financials" => vec![
            op!("earnings_price_history", [request("Qot_GetFinancialsEarningsPriceHistory", 3226)]),
            op!("earnings_price_move", [request("Qot_GetFinancialsEarningsPriceMove", 3225)]),
            op!("revenue_breakdown", [request("Qot_GetFinancialsRevenueBreakdown", 3228)]),
            op!("statements", [request("Qot_GetFinancialsStatements", 3227)]),
        ],
        "research.valuation" => vec![
            op!("constituents", [request("Qot_GetValuationPlateStockList", 3233)]),
            op!("detail", [request("Qot_GetValuationDetail", 3232)]),
        ],
        "research.analyst" => vec![
            op!("changes", [request("Qot_GetRatingChange", 3426)]),
            op!("consensus", [request("Qot_GetResearchAnalystConsensus", 3229)]),
            op!("morningstar", [request("Qot_GetResearchMorningstarReport", 3231)]),
            op!("ratings", [request("Qot_GetResearchRatingSummary", 3230)]),
        ],
        "research.ownership" => vec![
            op!("changes", [request("Qot_GetShareholdersHoldingChanges", 3238)]),
            op!("holders", [request("Qot_GetShareholdersHolderDetail", 3239)]),
            op!("insider_holders", [request("Qot_GetInsiderHolderList", 3241)]),
            op!("insider_transactions", [request("Qot_GetInsiderTradeList", 3242)]),
            op!("institutional", [request("Qot_GetShareholdersInstitutional", 3240)]),
            op!("management_changes", [request("Qot_GetHoldingChangeList", 3208)]),
            op!("overview", [request("Qot_GetShareholdersOverview", 3237)]),
        ],
        "research.corporate_actions" => vec![
            op!("buybacks", [request("Qot_GetCorporateActionsBuybacks", 3235)]),
            op!("code_changes", [request("Qot_GetCodeChange", 3216)]),
            op!("dividends", [request("Qot_GetCorporateActionsDividends", 3234)]),
            op!("splits", [request("Qot_GetCorporateActionsStockSplits", 3236)]),
        ],
        "research.short_interest" => vec![
            op!("daily_volume", [request("Qot_GetDailyShortVolume", 3248)]),
            op!("short_interest", [request("Qot_GetShortInterest", 3249)]),
        ],
        "research.news" => vec![op!("search", [request("Qot_GetSearchNews", 3263)])],
        "research.screen" => vec![
            op!("stock_v1", api = "/api/v1/research/screens", [request("Qot_StockFilter", 3215)], no_tool),
            op!("stock_v2", [request("Qot_StockScreen", 3252)]),
        ],
        "research.calendar" => vec![
            op!("dividends", [request("Qot_GetDividendCalendar", 3408)]),
            op!("economic", [request("Qot_GetEconomicCalendar", 3409)]),
            op!("earnings", [request("Qot_GetEarningsCalendar", 3401)]),
            op!("ipos", [request("Qot_GetIpoList", 3217)]),
            op!("trade_dates", [request("Qot_RequestTradeDate", 3219)]),
        ],
        "research.macro" => vec![
            op!("fed_dot_plot", [request("Qot_GetFedWatchDotPlot", 3405)]),
            op!("fed_target_rate", [request("Qot_GetFedWatchTargetRate", 3404)]),
            op!("indicator_history", [request("Qot_GetMacroIndicatorHistory", 3403)]),
            op!("indicators", [request("Qot_GetMacroIndicatorList", 3402)]),
        ],
        "research.rankings" => vec![
            op!("after_hours", [request("Qot_GetUSAfterHoursRank", 3411)]),
            op!("dividend", [request("Qot_GetDividendRank", 3407)]),
            op!("earnings_beat", [request("Qot_GetEarningsBeatRank", 3406)]),
            op!("fund_catalog", [request("Qot_GetStaticInfo", 3202)]),
            op!("heatmap", [request("Qot_GetHeatMapData", 3432)]),
            op!("high_dividend_state", [request("Qot_GetHighDividendSOERank", 3417)]),
            op!("hot", [request("Qot_GetHotList", 3414)]),
            op!("market_state", [request("Qot_GetMarketState", 3223)]),
            op!("overnight", [request("Qot_GetUSOvernightRank", 3412)]),
            op!("period_change", [request("Qot_GetPeriodChangeRank", 3416)]),
            op!("pre_market", [request("Qot_GetUSPreMarketRank", 3410)]),
            op!("rise_fall_distribution", [request("Qot_GetRiseFallDistribution", 3433)]),
            op!("short_selling", [request("Qot_GetShortSellingRank", 3415)]),
            op!("top_movers", [request("Qot_GetTopMoversRank", 3413)]),
        ],
        "research.institutions" => vec![
            op!("ark_fund_holdings", [request("Qot_GetArkFundHolding", 3423)]),
            op!("ark_stock_activity", [request("Qot_GetArkStockDynamic", 3424)]),
            op!("ark_transactions", [request("Qot_GetArkActiveTransaction", 3425)]),
            op!("distribution", [request("Qot_GetInstitutionDistribution", 3420)]),
            op!("holding_changes", [request("Qot_GetInstitutionHoldingChange", 3421)]),
            op!("holdings", [request("Qot_GetInstitutionHoldingList", 3422)]),
            op!("list", [request("Qot_GetInstitutionList", 3418)]),
            op!("profile", [request("Qot_GetInstitutionProfile", 3419)]),
        ],
        "research.industry" => vec![
            op!("chain_detail", [request("Qot_GetIndustrialChainDetail", 3428)]),
            op!("chains", [request("Qot_GetIndustrialChainList", 3427)]),
            op!("chains_by_plate", [request("Qot_GetIndustrialChainByPlate", 3429)]),
            op!("owner_plates", [request("Qot_GetOwnerPlate", 3207)]),
            op!("plate", [request("Qot_GetIndustrialPlateInfo", 3430)]),
            op!("plate_list", [request("Qot_GetPlateSet", 3204)]),
            op!("plate_members", [request("Qot_GetPlateSecurity", 3205)]),
            op!("plate_stocks", [request("Qot_GetIndustrialPlateStock", 3431)]),
        ],
        "research.technical_indicators" => vec![
            op!("calculate", [request("Qot_RequestIndicatorCalc", 3260)]),
            op!("list", [request("Qot_GetIndicatorList", 3259)]),
        ],
        "prediction.discover" => vec![
            op!("categories", api = "/api/v1/market-data/prediction/categories", [request("Qot_GetEventContractCategory", 3434)]),
            op!("competitions", api = "/api/v1/market-data/prediction/competitions", [request("Qot_FilterCompetition", 3435)]),
            op!("contracts", api = "/api/v1/market-data/prediction/events/{eventId}/contracts", [request("Qot_GetEventContract", 3438)]),
            op!("events", api = "/api/v1/market-data/prediction/events", [request("Qot_GetEventContractEventList", 3437)]),
            op!("milestones", api = "/api/v1/market-data/prediction/contracts/{code}/milestones", ui = "/workspace?tab=rules&marketSegment=prediction", [request("Qot_GetEventContractMilestoneList", 3439)]),
            op!("series", api = "/api/v1/market-data/prediction/series", [request("Qot_GetEventContractSeriesList", 3436)]),
        ],
        "prediction.snapshot" => vec![op!("snapshot", [request("Qot_GetEventContractSnapshot", 3445)])],
        "prediction.depth" => vec![op!("order_book", [request("Qot_GetEventContractOrderBook", 3446), push("Qot_UpdateEventContractOrderBook", 3450)])],
        "prediction.history" => vec![
            op!("candles", api = "/api/v1/market-data/prediction/contracts/{code}/candles", ui = "/workspace?tab=chart&marketSegment=prediction", [request("Qot_GetEventContractKline", 3447), push("Qot_UpdateEventContractKline", 3451)]),
            op!("historical", api = "/api/v1/market-data/prediction/contracts/{code}/candles/history", ui = "/workspace?tab=chart&marketSegment=prediction", [request("Qot_RequestHistoryEventContractKL", 3456)]),
            op!("subscribe", method = "POST", api = "/api/v1/market-data/prediction/contracts/{code}/subscriptions", ui = "/workspace?tab=chart&marketSegment=prediction", no_tool, [request("Qot_SubEventContract", 3455)]),
            op!("ticks", api = "/api/v1/market-data/prediction/contracts/{code}/ticks", ui = "/workspace?tab=ticks&marketSegment=prediction", [request("Qot_GetEventContractTicker", 3448), push("Qot_UpdateEventContractTicker", 3452)]),
        ],
        "prediction.combo_eligible" => vec![op!("eligible_events", [request("Qot_GetEventContractComboList", 3453)])],
        "prediction.combo_quote" => vec![op!("quote", [request("Qot_GetEventContractComboRfq", 3454)])],
        "execution.order_preview" => vec![op!("rules", [request("Trd_GetMaxTrdQtys", 2111)])],
        "execution.order_place" => vec![op!("place", [request("Trd_PlaceOrder", 2202), push("Trd_UpdateOrder", 2208), push("Trd_UpdateOrderFill", 2218)])],
        "execution.order_cancel" => vec![op!("cancel", [request("Trd_ModifyOrder", 2205)])],
        "execution.combo_preview" => vec![
            op!("buying_power", [request("Trd_GetComboMaxTrdQtys", 2112)]),
            op!("legality", [request("Qot_GetOptionStrategy", 3256), request("Qot_GetOptionStrategyAnalysis", 3257), request("Qot_GetOptionStrategySpread", 3258)]),
        ],
        "execution.combo_place" => vec![op!("place", [request("Trd_PlaceComboOrder", 2227), push("Trd_UpdateOrder", 2208), push("Trd_UpdateOrderFill", 2218)])],
        "execution.combo_cancel" => vec![op!("cancel", [request("Trd_ModifyOrder", 2205)])],
        "execution.buying_power" => vec![
            op!("combo", [request("Trd_GetComboMaxTrdQtys", 2112)]),
            op!("single", [request("Trd_GetMaxTrdQtys", 2111)]),
        ],
        "alerts.price.list" => vec![op!("list", [request("Qot_GetPriceReminder", 3221)])],
        "alerts.price.set" => vec![op!("set", [request("Qot_SetPriceReminder", 3220)])],
        "alerts.option_event.list" => vec![op!("list", [request("Qot_GetOptionEventAlert", 3308)])],
        "alerts.option_event.set" => vec![op!("set", [request("Qot_SetOptionEventAlert", 3309)])],
        "watchlist.remote.list" => vec![
            op!("groups", [request("Qot_GetUserSecurityGroup", 3222)]),
            op!("members", [request("Qot_GetUserSecurity", 3213)]),
        ],
        "watchlist.remote.modify" => vec![op!("modify", [request("Qot_ModifyUserSecurity", 3214)])],
        _ => vec![operation(feature_id, feature_id, default_method, default_api, default_ui, default_tool, None, None, None, false, &[])],
    }
}
