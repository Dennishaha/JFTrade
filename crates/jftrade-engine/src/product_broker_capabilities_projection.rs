//! Production projection for the broker capability endpoint.
//!
//! The OpenD integration descriptor is intentionally a transport descriptor:
//! it contains read-feature hints but not the product capability catalog that
//! the HTTP API exposes.  This module is the composition-root adapter between
//! those two contracts.  The catalog metadata is static (and versioned), while
//! descriptor feature state and runtime evaluations are derived from the
//! currently installed Provider/OpenD/readers on every request.

use serde_json::{Value, json};
use time::OffsetDateTime;
use time::format_description::well_known::Rfc3339;

use super::{SharedTradeReadRuntime, checked_at};
use crate::product::product_active_provider_state::ProviderRuntimeSnapshot;
use crate::product::product_query::QueryMap;

#[path = "product_broker_capabilities_operations.rs"]
mod operations;

const CATALOG_VERSION: &str = "2026-07-17.opend-10.9.6908";
const FUTU_MARKETS: &[&str] = &["HK", "US", "SH", "SZ"];
const CANDLE_PERIODS: &[&str] = &["1m", "3m", "5m", "10m", "15m", "30m", "1h", "1d", "1w", "1mo"];

#[derive(Clone, Copy)]
struct FeatureSpec {
    id: &'static str,
    adapter: &'static str,
    api: &'static str,
    ui: &'static str,
    tool: &'static str,
    access: &'static str,
    method: &'static str,
}

// Keep this list in the same order as broker.BuiltinCapabilityCatalog.  The
// operation/protocol details live in `product_broker_capabilities_operations`
// so this projection remains focused on runtime state and wire shaping.
const FEATURE_SPECS: &[FeatureSpec] = &[
    FeatureSpec { id: "market.search", adapter: "InstrumentProfileReader", api: "/api/v1/market-data/instruments", ui: "/workspace", tool: "market.search", access: "read", method: "GET" },
    FeatureSpec { id: "market.instrument_profile", adapter: "InstrumentProfileReader", api: "/api/v1/market-data/instruments/{instrumentId}/profile", ui: "/workspace?tab=company", tool: "market.instrument_profile", access: "read", method: "GET" },
    FeatureSpec { id: "market.snapshot", adapter: "BatchSnapshotSource", api: "/api/v1/market-data/snapshots/{market}/{symbol}", ui: "/workspace?tab=chart", tool: "market.snapshot", access: "read", method: "GET" },
    FeatureSpec { id: "market.snapshots", adapter: "BatchSnapshotSource", api: "/api/v1/market-data/snapshots", ui: "/workspace", tool: "market.snapshots", access: "read", method: "POST" },
    FeatureSpec { id: "market.candles", adapter: "MarketDataReader", api: "/api/v1/market-data/candles/{market}/{symbol}", ui: "/workspace?tab=chart", tool: "market.candles", access: "read", method: "GET" },
    FeatureSpec { id: "market.intraday", adapter: "MarketMicrostructureReader", api: "/api/v1/market-data/intraday/{instrumentId}", ui: "/workspace?tab=chart", tool: "market.intraday", access: "read", method: "GET" },
    FeatureSpec { id: "market.ticks", adapter: "MarketMicrostructureReader", api: "/api/v1/market-data/ticks/{instrumentId}", ui: "/workspace?tab=chart", tool: "market.ticks", access: "read", method: "GET" },
    FeatureSpec { id: "market.depth", adapter: "MarketMicrostructureReader", api: "/api/v1/market-data/depth/{market}/{symbol}", ui: "/workspace?tab=chart", tool: "market.depth", access: "read", method: "GET" },
    FeatureSpec { id: "market.broker_queue", adapter: "MarketMicrostructureReader", api: "/api/v1/market-data/broker-queue/{instrumentId}", ui: "/workspace?tab=chart", tool: "market.broker_queue", access: "read", method: "GET" },
    FeatureSpec { id: "market.capital_flow", adapter: "MarketMicrostructureReader", api: "/api/v1/market-data/capital-flow/{instrumentId}", ui: "/workspace?tab=company", tool: "market.capital_flow", access: "read", method: "GET" },
    FeatureSpec { id: "derivatives.option_chain", adapter: "DerivativeCatalogReader", api: "/api/v1/market-data/options/chains/{instrumentId}", ui: "/workspace?tab=options", tool: "derivatives.option_chain", access: "read", method: "GET" },
    FeatureSpec { id: "derivatives.option_screen", adapter: "DerivativeCatalogReader", api: "/api/v1/market-data/options/screens", ui: "/research?section=options", tool: "derivatives.option_screen", access: "read", method: "GET" },
    FeatureSpec { id: "derivatives.option_analysis", adapter: "OptionAnalyticsReader", api: "/api/v1/market-data/options/analysis/{instrumentId}", ui: "/workspace?tab=options", tool: "derivatives.option_analysis", access: "read", method: "GET" },
    FeatureSpec { id: "derivatives.option_events", adapter: "OptionAnalyticsReader", api: "/api/v1/market-data/options/events", ui: "/research?section=options", tool: "derivatives.option_events", access: "read", method: "GET" },
    FeatureSpec { id: "derivatives.warrants", adapter: "DerivativeCatalogReader", api: "/api/v1/market-data/warrants", ui: "/workspace?tab=warrants", tool: "derivatives.warrants", access: "read", method: "GET" },
    FeatureSpec { id: "derivatives.futures", adapter: "DerivativeCatalogReader", api: "/api/v1/market-data/futures", ui: "/workspace", tool: "derivatives.futures", access: "read", method: "GET" },
    FeatureSpec { id: "research.instrument", adapter: "InstrumentResearchReader", api: "/api/v1/research/instruments/{instrumentId}", ui: "/workspace?tab=company", tool: "research.instrument", access: "read", method: "GET" },
    FeatureSpec { id: "research.financials", adapter: "InstrumentResearchReader", api: "/api/v1/research/financials/{instrumentId}", ui: "/workspace?tab=company", tool: "research.financials", access: "read", method: "GET" },
    FeatureSpec { id: "research.valuation", adapter: "InstrumentResearchReader", api: "/api/v1/research/valuation/{instrumentId}", ui: "/workspace?tab=company", tool: "research.valuation", access: "read", method: "GET" },
    FeatureSpec { id: "research.analyst", adapter: "InstrumentResearchReader", api: "/api/v1/research/analyst/{instrumentId}", ui: "/workspace?tab=company", tool: "research.analyst", access: "read", method: "GET" },
    FeatureSpec { id: "research.ownership", adapter: "InstrumentResearchReader", api: "/api/v1/research/ownership/{instrumentId}", ui: "/workspace?tab=company", tool: "research.ownership", access: "read", method: "GET" },
    FeatureSpec { id: "research.corporate_actions", adapter: "InstrumentResearchReader", api: "/api/v1/research/corporate-actions/{instrumentId}", ui: "/workspace?tab=company", tool: "research.corporate_actions", access: "read", method: "GET" },
    FeatureSpec { id: "research.short_interest", adapter: "InstrumentResearchReader", api: "/api/v1/research/short-interest/{instrumentId}", ui: "/workspace?tab=company", tool: "research.short_interest", access: "read", method: "GET" },
    FeatureSpec { id: "research.news", adapter: "InstrumentResearchReader", api: "/api/v1/market-data/news", ui: "/workspace?tab=news", tool: "research.news", access: "read", method: "GET" },
    FeatureSpec { id: "research.screen", adapter: "MarketResearchReader", api: "/api/v1/research/screens", ui: "/research?section=screens", tool: "research.screen", access: "read", method: "GET" },
    FeatureSpec { id: "research.calendar", adapter: "MarketResearchReader", api: "/api/v1/research/calendars", ui: "/research?section=calendar", tool: "research.calendar", access: "read", method: "GET" },
    FeatureSpec { id: "research.macro", adapter: "MarketResearchReader", api: "/api/v1/research/macro", ui: "/research?section=macro", tool: "research.macro", access: "read", method: "GET" },
    FeatureSpec { id: "research.rankings", adapter: "MarketResearchReader", api: "/api/v1/research/rankings", ui: "/research?section=market", tool: "research.rankings", access: "read", method: "GET" },
    FeatureSpec { id: "research.institutions", adapter: "MarketResearchReader", api: "/api/v1/research/institutions", ui: "/research?section=institutions", tool: "research.institutions", access: "read", method: "GET" },
    FeatureSpec { id: "research.industry", adapter: "MarketResearchReader", api: "/api/v1/research/industries", ui: "/research?section=industries", tool: "research.industry", access: "read", method: "GET" },
    FeatureSpec { id: "research.technical_indicators", adapter: "TechnicalIndicatorReader", api: "/api/v1/research/technical-indicators/{instrumentId}", ui: "/research?section=market", tool: "research.technical_indicators", access: "read", method: "GET" },
    FeatureSpec { id: "prediction.discover", adapter: "PredictionMarketReader", api: "/api/v1/market-data/prediction/categories", ui: "/research?section=prediction", tool: "prediction.discover", access: "read", method: "GET" },
    FeatureSpec { id: "prediction.snapshot", adapter: "PredictionMarketReader", api: "/api/v1/market-data/prediction/contracts/{code}/snapshot", ui: "/workspace?tab=contract&marketSegment=prediction", tool: "prediction.snapshot", access: "read", method: "GET" },
    FeatureSpec { id: "prediction.depth", adapter: "PredictionMarketReader", api: "/api/v1/market-data/prediction/contracts/{code}/order-book", ui: "/workspace?tab=depth&marketSegment=prediction", tool: "prediction.depth", access: "read", method: "GET" },
    FeatureSpec { id: "prediction.history", adapter: "PredictionMarketReader", api: "/api/v1/market-data/prediction/contracts/{code}/candles", ui: "/workspace?tab=chart&marketSegment=prediction", tool: "prediction.history", access: "read", method: "GET" },
    FeatureSpec { id: "prediction.combo_eligible", adapter: "PredictionMarketReader", api: "/api/v1/market-data/prediction/combos/eligible-events", ui: "/research?section=prediction", tool: "prediction.combo_eligible", access: "read", method: "GET" },
    FeatureSpec { id: "prediction.combo_quote", adapter: "PredictionMarketReader", api: "/api/v1/market-data/prediction/combos/quotes", ui: "/research?section=prediction", tool: "prediction.combo_quote", access: "read", method: "POST" },
    FeatureSpec { id: "execution.order_preview", adapter: "ProductRuleProvider", api: "/api/v1/execution/previews", ui: "/workspace?surface=order", tool: "execution.order_preview", access: "trade", method: "POST" },
    FeatureSpec { id: "execution.order_place", adapter: "TradingService", api: "/api/v1/execution/orders", ui: "/workspace?surface=order", tool: "execution.order_place", access: "trade", method: "POST" },
    FeatureSpec { id: "execution.order_cancel", adapter: "TradingService", api: "/api/v1/execution/orders/{internalOrderId}/cancel", ui: "/workspace?surface=order", tool: "execution.order_cancel", access: "trade", method: "POST" },
    FeatureSpec { id: "execution.combo_preview", adapter: "ComboTradingService", api: "/api/v1/execution/combos/previews", ui: "/workspace?tab=options", tool: "execution.combo_preview", access: "trade", method: "POST" },
    FeatureSpec { id: "execution.combo_place", adapter: "ComboTradingService", api: "/api/v1/execution/combos", ui: "/workspace?tab=options", tool: "execution.combo_place", access: "trade", method: "POST" },
    FeatureSpec { id: "execution.combo_cancel", adapter: "ComboTradingService", api: "/api/v1/execution/combos/{internalOrderId}/cancel", ui: "/workspace?tab=options", tool: "execution.combo_cancel", access: "trade", method: "POST" },
    FeatureSpec { id: "execution.buying_power", adapter: "ProductRuleProvider", api: "/api/v1/execution/buying-power", ui: "/workspace?surface=order", tool: "execution.buying_power", access: "read", method: "POST" },
    FeatureSpec { id: "alerts.price.list", adapter: "CustomizationService", api: "/api/v1/alerts/price", ui: "", tool: "alerts.price.list", access: "read", method: "GET" },
    FeatureSpec { id: "alerts.price.set", adapter: "CustomizationService", api: "/api/v1/alerts/price", ui: "", tool: "alerts.price.set", access: "write", method: "POST" },
    FeatureSpec { id: "alerts.option_event.list", adapter: "CustomizationService", api: "/api/v1/alerts/option-events", ui: "/workspace?tab=options", tool: "alerts.option_event.list", access: "read", method: "GET" },
    FeatureSpec { id: "alerts.option_event.set", adapter: "CustomizationService", api: "/api/v1/alerts/option-events", ui: "/workspace?tab=options", tool: "alerts.option_event.set", access: "write", method: "POST" },
    FeatureSpec { id: "watchlist.remote.list", adapter: "CustomizationService", api: "/api/v1/watchlists/remote", ui: "/watchlist", tool: "watchlist.remote.list", access: "read", method: "GET" },
    FeatureSpec { id: "watchlist.remote.modify", adapter: "CustomizationService", api: "/api/v1/watchlists/remote", ui: "/watchlist", tool: "watchlist.remote.modify", access: "write", method: "POST" },
];

pub(super) fn project(
    runtime: &SharedTradeReadRuntime,
    provider: &ProviderRuntimeSnapshot,
    query: &str,
) -> Result<Value, String> {
    let query = QueryMap::parse(query).map_err(|_| "invalid URL escape".to_owned())?;
    let broker_filter = query.get_first("brokerId").map(str::trim).filter(|v| !v.is_empty());
    if broker_filter.is_some_and(|value| !value.eq_ignore_ascii_case("futu")) {
        return Ok(json!({
            "catalog": catalog(),
            "brokers": [],
            "runtime": [],
        }));
    }
    let market_filter = query.get_first("market").map(str::trim).filter(|v| !v.is_empty());
    let feature_filter = query.get_first("featureId").map(str::trim).filter(|v| !v.is_empty());
    let product_filter = query.get_first("productClass").map(str::trim).filter(|v| !v.is_empty());
    let segment_filter = query.get_first("marketSegment").map(str::trim).filter(|v| !v.is_empty());
    let descriptor = descriptor(
        provider,
        runtime,
        market_filter,
        feature_filter,
        product_filter,
        segment_filter,
    );
    let statuses = runtime_statuses(
        provider,
        runtime,
        market_filter,
        feature_filter,
        product_filter,
        segment_filter,
    );
    Ok(json!({"catalog": catalog(), "brokers": [descriptor], "runtime": statuses}))
}

fn catalog() -> Value {
    let features = FEATURE_SPECS.iter().map(catalog_feature).collect::<Vec<_>>();
    json!({"version": CATALOG_VERSION, "features": features})
}

fn catalog_feature(spec: &FeatureSpec) -> Value {
    let operations = operations::catalog_operations(spec.id, spec.method, spec.api, spec.ui, spec.tool);
    let permission = match spec.access {
        "trade" => "live_trading",
        "write" => "write_external",
        _ => "read_only",
    };
    let approval = match spec.access {
        "trade" => "critical",
        "write" => "high",
        _ => "none",
    };
    let mut surface = json!({"api": spec.api, "tool": spec.tool});
    if !spec.ui.is_empty() {
        surface["ui"] = Value::String(spec.ui.to_owned());
    }
    if spec.access == "read" {
        surface["readOnlyMcp"] = Value::Bool(true);
    }
    json!({
        "id": spec.id,
        "adapterInterface": spec.adapter,
        "access": spec.access,
        "permission": permission,
        "approval": approval,
        "surface": surface,
        "testMapping": format!("TestCapabilityOperationContracts/{}", spec.id),
        "operations": operations,
    })
}

fn descriptor(
    provider: &ProviderRuntimeSnapshot,
    runtime: &SharedTradeReadRuntime,
    market_filter: Option<&str>,
    feature_filter: Option<&str>,
    product_filter: Option<&str>,
    segment_filter: Option<&str>,
) -> Value {
    let features = FEATURE_SPECS
        .iter()
        .filter(|spec| feature_filter.is_none_or(|id| spec.id.eq_ignore_ascii_case(id)))
        .filter(|spec| feature_matches_filters(spec.id, product_filter, segment_filter))
        .flat_map(|spec| {
            FUTU_MARKETS
                .iter()
                .filter(move |&market| feature_supported_in_market(spec.id, market))
                .map(move |market| feature_capability(spec, market, static_state(spec.id)))
        })
        .collect::<Vec<_>>();
    let read_features = serde_json::to_value(jftrade_integration_futu::broker_descriptor())
        .ok()
        .and_then(|value| {
            value["capabilities"]
                .get(0)
                .and_then(|cap| cap["readFeatures"].as_object().cloned())
        })
        .unwrap_or_default();
    let read_features = complete_read_features(read_features);
    let capabilities = FUTU_MARKETS
        .iter()
        .filter(|market| market_filter.is_none_or(|filter| market.eq_ignore_ascii_case(filter)))
        .map(|market| {
            let market_features = features
                .iter()
                .filter(|feature| feature["markets"].as_array().is_some_and(|markets| markets.iter().any(|item| item == market)))
                .cloned()
                .collect::<Vec<_>>();
            json!({
                "market": market,
                "supportsQuote": true,
                "supportsTrade": true,
                "readFeatures": read_features,
                "features": market_features,
            })
        })
        .collect::<Vec<_>>();
    let _ = (provider, runtime);
    json!({
        "id": "futu",
        "displayName": "Futu",
        "securityFirm": "Futu/Moomoo via OpenD",
        "capabilityVersion": CATALOG_VERSION,
        "environments": ["SIMULATE", "REAL"],
        "capabilities": capabilities,
        "notes": [
            "OpenD 10.9.6908 product and research protocols are broker-neutral above the adapter boundary.",
            "Prediction-market availability is determined at runtime and currently requires an eligible Moomoo US environment.",
            "Snapshot polling does not consume Basic Quote subscriptions; streaming is restricted to visible instruments."
        ],
    })
}

/// The native Futu descriptor is also exposed by the lower-level integration
/// API and intentionally keeps its historical nine-field fixture.  The
/// product capability endpoint has always exposed the five market-data read
/// hints below as well, so add them at this projection boundary instead of
/// changing that separate descriptor contract.
fn complete_read_features(
    mut features: serde_json::Map<String, Value>,
) -> Value {
    let environments = json!(["SIMULATE", "REAL"]);
    for (id, requirements) in [
        (
            "quote",
            json!({"supportedEnvironments": environments.clone(), "requiresSymbols": true}),
        ),
        (
            "klines",
            json!({"supportedEnvironments": environments.clone(), "requiresSymbol": true}),
        ),
        (
            "securityInfo",
            json!({"supportedEnvironments": environments.clone(), "requiresSymbols": true}),
        ),
        (
            "securitySnapshot",
            json!({"supportedEnvironments": environments, "requiresSymbols": true}),
        ),
        (
            "unlockTrade",
            json!({"supportedEnvironments": ["REAL"], "requiresPassword": true}),
        ),
    ] {
        features.entry(id.to_owned()).or_insert(requirements);
    }
    Value::Object(features)
}

fn runtime_statuses(
    provider: &ProviderRuntimeSnapshot,
    runtime: &SharedTradeReadRuntime,
    market_filter: Option<&str>,
    feature_filter: Option<&str>,
    product_filter: Option<&str>,
    segment_filter: Option<&str>,
) -> Vec<Value> {
    FEATURE_SPECS
        .iter()
        .filter(|spec| feature_filter.is_none_or(|id| spec.id.eq_ignore_ascii_case(id)))
        .filter(|spec| feature_matches_filters(spec.id, product_filter, segment_filter))
        .flat_map(|spec| {
            FUTU_MARKETS.iter().filter_map(move |market| {
                if !feature_supported_in_market(spec.id, market)
                    || market_filter.is_some_and(|filter| !market.eq_ignore_ascii_case(filter))
                {
                    return None;
                }
                let capability = feature_capability(spec, market, static_state(spec.id));
                let available = feature_runtime_available(spec.id, provider, runtime);
                let evaluation = evaluation(spec, provider, runtime, available);
                let mut evaluated_capability = capability;
                evaluated_capability["state"] = evaluation["state"].clone();
                evaluated_capability["reasonCode"] = evaluation["code"].clone();
                evaluated_capability["reason"] = evaluation["reason"].clone();
                Some(json!({
                    "brokerId": "futu",
                    "securityFirm": "Futu/Moomoo via OpenD",
                    "market": market,
                    "featureId": spec.id,
                    "capability": evaluated_capability,
                    "evaluation": evaluation,
                }))
            })
        })
        .collect()
}

fn feature_capability(spec: &FeatureSpec, market: &str, state: &'static str) -> Value {
    let mut capability = json!({
        "id": spec.id,
        "markets": [market],
        "access": spec.access,
        "state": state,
        "requiresConnection": true,
        "requiresAccount": spec.access != "read",
        "requiresQuoteRight": spec.access == "read",
        "productClasses": product_classes(spec.id),
        "marketSegments": market_segments(spec.id),
    });
    if spec.id.starts_with("prediction.") {
        capability["reasonCode"] = Value::String("RUNTIME_ELIGIBILITY_REQUIRED".to_owned());
        capability["reason"] = Value::String(
            "Prediction markets require an eligible Moomoo US environment.".to_owned(),
        );
    }
    if spec.id == "market.candles" {
        capability["supportedPeriods"] = json!(CANDLE_PERIODS);
        if market == "US" {
            let regular = [
                "tick", "1m", "3m", "5m", "10m", "15m", "30m", "1h", "1d", "1w", "1mo",
            ];
            let intraday = ["tick", "1m", "3m", "5m", "10m", "15m", "30m", "1h"];
            capability["supportedSessions"] = json!([
                {"id": "regular", "supportedPeriods": regular},
                {"id": "extended", "supportedPeriods": intraday},
                {"id": "overnight", "supportedPeriods": intraday}
            ]);
        } else {
            capability["supportedSessions"] = json!([
                {"id": "regular", "supportedPeriods": CANDLE_PERIODS}
            ]);
        }
    }
    capability
}

fn evaluation(
    spec: &FeatureSpec,
    provider: &ProviderRuntimeSnapshot,
    runtime: &SharedTradeReadRuntime,
    available: bool,
) -> Value {
    let now = checked_at();
    let connection_ready = provider.provider.is_some()
        && provider.opend_ready
        && runtime.snapshot().client.is_some();
    let connection = if connection_ready {
        check("available", "OPEND_CONNECTED", "OpenD session is connected.", &now)
    } else {
        check("unavailable", "OPEND_CONNECTION_UNAVAILABLE", "OpenD session is unavailable.", &now)
    };
    let account = if spec.access == "read" {
        check("available", "NOT_REQUIRED", "This runtime dimension is not required.", &now)
    } else if runtime.snapshot().is_ready() {
        check("available", "ACCOUNT_ELIGIBLE", "The active trade session is eligible.", &now)
    } else {
        check("degraded", "ACCOUNT_CONTEXT_REQUIRED", "A logged-in trade session is required.", &now)
    };
    let quote = if spec.access == "read" {
        if available {
            check("degraded", "QUOTE_RIGHT_UNVERIFIED", "OpenD quote entitlement has not been verified for this session.", &now)
        } else {
            check("unavailable", "CAPABILITY_UNAVAILABLE", "The concrete reader for this capability is unavailable.", &now)
        }
    } else {
        check("available", "NOT_REQUIRED", "This runtime dimension is not required.", &now)
    };
    let (state, code, reason) = if !available {
        ("unavailable", "CAPABILITY_UNAVAILABLE", "The concrete production adapter is unavailable.")
    } else if [connection["state"].as_str(), account["state"].as_str(), quote["state"].as_str()].contains(&Some("unavailable")) {
        ("unavailable", "RUNTIME_UNAVAILABLE", "The capability is not usable in the current runtime.")
    } else if [connection["state"].as_str(), account["state"].as_str(), quote["state"].as_str()].contains(&Some("degraded")) {
        ("degraded", "RUNTIME_STATUS_PARTIAL", "Static support is available but runtime state is incomplete.")
    } else {
        ("available", "RUNTIME_READY", "The capability is ready in the current runtime.")
    };
    json!({
        "state": state,
        "code": code,
        "reason": reason,
        "connection": connection,
        "account": account,
        "quoteRight": quote,
        "checkedAt": now,
    })
}

fn check(state: &str, code: &str, reason: &str, checked_at: &str) -> Value {
    json!({"state": state, "code": code, "reason": reason, "checkedAt": checked_at})
}

fn feature_runtime_available(id: &str, provider: &ProviderRuntimeSnapshot, runtime: &SharedTradeReadRuntime) -> bool {
    if provider.provider != Some(jftrade_settings::MarketDataProvider::Futu) || !provider.opend_ready {
        return false;
    }
    match id {
        "market.snapshot" | "market.snapshots" => runtime.market_data_reader_available(),
        "market.candles" => runtime.historical_klines_available(),
        "derivatives.option_chain" => runtime.option_chains_available() || runtime.option_expirations_available(),
        "derivatives.option_screen" => runtime.option_screens_available(),
        "derivatives.option_analysis" => runtime.option_quotes_available() || runtime.option_volatility_available() || runtime.option_exercise_probability_available() || runtime.option_underlying_overview_available() || runtime.option_underlying_his_volatility_available() || runtime.option_market_statistic_available() || runtime.option_underlying_his_statistic_available() || runtime.option_strategy_spread_available() || runtime.option_strategy_available() || runtime.option_strategy_analysis_available() || runtime.option_underlying_rank_available() || runtime.option_contract_rank_available(),
        "derivatives.option_events" => runtime.option_events_available() || runtime.option_zero_dte_screener_available() || runtime.option_zero_dte_contract_available() || runtime.option_earnings_screener_available() || runtime.option_seller_screener_available(),
        "derivatives.futures" => runtime.future_info_available(),
        "research.valuation" => runtime.valuation_detail_available(),
        "research.news" => runtime.news_reader_available(),
        "research.corporate_actions" => runtime.corporate_actions_reader_available(),
        "alerts.price.list" | "alerts.option_event.list" => runtime.alert_reader().is_some(),
        "alerts.price.set" | "alerts.option_event.set" => runtime.alert_writer().is_some(),
        "watchlist.remote.list" => runtime.remote_watchlist_reader().is_some(),
        "watchlist.remote.modify" => runtime.remote_watchlist_writer().is_some(),
        id if id.starts_with("execution.") => {
            runtime.snapshot().is_ready()
                && (id.ends_with("_preview")
                    || id == "execution.buying_power"
                    || runtime.writer_snapshot().is_some())
        }
        _ => false,
    }
}

fn static_state(id: &str) -> &'static str {
    if id.starts_with("prediction.") { "degraded" } else { "available" }
}

fn feature_supported_in_market(id: &str, market: &str) -> bool {
    if id.starts_with("prediction.") { return market == "US"; }
    if id == "derivatives.warrants" { return market == "HK"; }
    if id == "derivatives.futures" { return market == "HK" || market == "US"; }
    if id.starts_with("derivatives.option") || id.starts_with("execution.combo") { return market == "HK" || market == "US"; }
    true
}

fn product_classes(id: &str) -> Value {
    if id.starts_with("prediction.") { return json!(["event_contract"]); }
    if id.starts_with("execution.combo") { return json!(["option", "event_contract"]); }
    if id.starts_with("execution.order") || id == "execution.buying_power" { return json!(["equity", "fund", "option", "warrant", "cbbc", "future", "event_contract"]); }
    if id.contains("option") { return json!(["option"]); }
    if id == "derivatives.warrants" { return json!(["warrant", "cbbc"]); }
    if id == "derivatives.futures" { return json!(["future"]); }
    json!(["equity", "fund", "index", "bond", "plate"])
}

fn market_segments(id: &str) -> Value {
    if id.starts_with("prediction.") { json!(["prediction"]) }
    else if id.starts_with("execution.") { json!(["securities", "derivatives", "prediction"]) }
    else if id.starts_with("derivatives.") { json!(["derivatives"]) }
    else { json!(["securities"]) }
}

fn feature_matches_filters(
    id: &str,
    product_filter: Option<&str>,
    segment_filter: Option<&str>,
) -> bool {
    let product_match = product_filter.is_none_or(|filter| {
        product_classes(id).as_array().is_some_and(|values| {
            values.iter().any(|value| {
                value
                    .as_str()
                    .is_some_and(|item| item.eq_ignore_ascii_case(filter))
            })
        })
    });
    let segment_match = segment_filter.is_none_or(|filter| {
        market_segments(id).as_array().is_some_and(|values| {
            values.iter().any(|value| {
                value
                    .as_str()
                    .is_some_and(|item| item.eq_ignore_ascii_case(filter))
            })
        })
    });
    product_match && segment_match
}

pub(super) fn ui_surface_id(value: &str) -> String {
    let value = value.trim();
    if value.is_empty() { return String::new(); }
    if value == "/workspace" { return "workspace.root".to_owned(); }
    if value.starts_with("/workspace?") {
        if let Some(tab) = value.split("tab=").nth(1).and_then(|v| v.split('&').next()) { return format!("workspace.{tab}"); }
        if let Some(surface) = value.split("surface=").nth(1).and_then(|v| v.split('&').next()) { return format!("workspace.{surface}"); }
    }
    if value == "/watchlist" { return "watchlist.root".to_owned(); }
    if value.starts_with("/research?")
        && let Some(section) = value.split("section=").nth(1).and_then(|v| v.split('&').next())
    {
        return format!("research.{section}");
    }
    value.trim_start_matches('/').replace(['/', '?', '='], ".")
}

#[allow(dead_code)]
fn now_utc_rfc3339() -> String {
    OffsetDateTime::now_utc().format(&Rfc3339).unwrap_or_else(|_| checked_at())
}
