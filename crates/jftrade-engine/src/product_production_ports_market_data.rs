//! Production market-data adapters bundle.
//!
//! Connects catalog reads, quote reads, subscription mutations, and provider
//! actions to real production state without mock fixtures or dummy arrays.

#[path = "product_production_ports_market_data_actions.rs"]
mod product_production_ports_market_data_actions;
#[path = "product_production_ports_market_data_catalog.rs"]
mod product_production_ports_market_data_catalog;
#[path = "product_production_ports_market_data_futures.rs"]
mod product_production_ports_market_data_futures;
#[path = "product_production_ports_market_data_news_actions.rs"]
pub(super) mod product_production_ports_market_data_news_actions;
#[path = "product_production_ports_market_data_news_search.rs"]
mod product_production_ports_market_data_news_search;
#[cfg(test)]
#[path = "product_production_ports_market_data_news_tests.rs"]
mod product_production_ports_market_data_news_tests;
#[path = "product_production_ports_market_data_options_analysis.rs"]
mod product_production_ports_market_data_options_analysis;
#[path = "product_production_ports_market_data_options_common.rs"]
mod product_production_ports_market_data_options_common;
#[path = "product_production_ports_market_data_options_contract_rank.rs"]
mod product_production_ports_market_data_options_contract_rank;
#[path = "product_production_ports_market_data_options_events.rs"]
mod product_production_ports_market_data_options_events;
#[path = "product_production_ports_market_data_options_historical_volatility.rs"]
mod product_production_ports_market_data_options_historical_volatility;
#[path = "product_production_ports_market_data_options_screen.rs"]
mod product_production_ports_market_data_options_screen;
#[path = "product_production_ports_market_data_options_strategy.rs"]
mod product_production_ports_market_data_options_strategy;
#[path = "product_production_ports_market_data_options_strategy_analysis.rs"]
mod product_production_ports_market_data_options_strategy_analysis;
#[path = "product_production_ports_market_data_options_strategy_spread.rs"]
mod product_production_ports_market_data_options_strategy_spread;
#[cfg(test)]
#[path = "product_production_ports_market_data_options_tests.rs"]
mod product_production_ports_market_data_options_tests;
#[path = "product_production_ports_market_data_prediction.rs"]
mod product_production_ports_market_data_prediction;
#[path = "product_production_ports_market_data_projection.rs"]
pub(crate) mod product_production_ports_market_data_projection;
#[path = "product_production_ports_market_data_quote.rs"]
mod product_production_ports_market_data_quote;
#[path = "product_production_ports_market_data_subscription.rs"]
mod product_production_ports_market_data_subscription;

pub(crate) use product_production_ports_market_data_actions::ProductionMarketDataProviderActionsPort;
pub(crate) use product_production_ports_market_data_catalog::ProductionMarketDataCatalogPort;
pub(crate) use product_production_ports_market_data_quote::ProductionMarketDataQuotePort;
pub(crate) use product_production_ports_market_data_subscription::ProductionMarketDataSubscriptionMutationPort;

use product_production_ports_market_data_options_common::{
    map_option_chain_error, map_option_expiration_error, options_bad_request,
};

use super::product_production_ports_trade::SharedTradeReadRuntime;
use crate::product::product_active_provider_state::ActiveProviderState;
use crate::product::{
    MarketDataDerivativeReadSnapshotError, MarketDataDerivativeReadSnapshotPort,
    MarketDataNewsActionsReadSnapshotError, MarketDataNewsActionsReadSnapshotPort,
    MarketDataNewsSearchReadSnapshotError, MarketDataNewsSearchReadSnapshotPort,
    MarketDataOptionsReadSnapshotError, MarketDataOptionsReadSnapshotPort,
};
use jftrade_integration_marketdata_helper::{HelperClient, HttpAdapterError};
use jftrade_settings::MarketDataProvider;
use serde_json::Value;
use std::sync::Arc;
use std::thread;

#[derive(Debug)]
pub(crate) struct ProductionMarketDataDerivativePort {
    pub(crate) active_provider_state: Arc<ActiveProviderState>,
    pub(crate) trade_runtime: Option<Arc<SharedTradeReadRuntime>>,
}

impl MarketDataDerivativeReadSnapshotPort for ProductionMarketDataDerivativePort {
    fn read(
        &self,
        path: &str,
        query: &str,
    ) -> Result<Value, MarketDataDerivativeReadSnapshotError> {
        let snapshot = self.active_provider_state.snapshot();
        if snapshot.provider != Some(MarketDataProvider::Futu) || !snapshot.opend_ready {
            return Err(MarketDataDerivativeReadSnapshotError::Unavailable(
                "Futu derivatives market-data provider is not ready".to_owned(),
            ));
        }
        if path == "/api/v1/market-data/futures" {
            return product_production_ports_market_data_futures::read_runtime(
                self.trade_runtime.as_ref(),
                path,
                query,
            );
        }
        Err(MarketDataDerivativeReadSnapshotError::Unavailable(
            "Futu warrants market-data reader is not ready".to_owned(),
        ))
    }
}

#[derive(Debug)]
pub(crate) struct ProductionMarketDataOptionsPort {
    pub(crate) active_provider_state: Arc<ActiveProviderState>,
    pub(crate) trade_runtime: Option<Arc<SharedTradeReadRuntime>>,
}

impl MarketDataOptionsReadSnapshotPort for ProductionMarketDataOptionsPort {
    fn read(&self, path: &str, query: &str) -> Result<Value, MarketDataOptionsReadSnapshotError> {
        let snapshot = self.active_provider_state.snapshot();
        if snapshot.provider != Some(MarketDataProvider::Futu) || !snapshot.opend_ready {
            return Err(MarketDataOptionsReadSnapshotError::Unavailable(
                "Futu options market-data provider is not ready".to_owned(),
            ));
        }
        let runtime = self.trade_runtime.as_ref().ok_or_else(|| {
            MarketDataOptionsReadSnapshotError::Unavailable(
                "Futu options market-data runtime is not configured".to_owned(),
            )
        })?;
        if path == "/api/v1/market-data/options/screens" {
            return product_production_ports_market_data_options_screen::read(Some(runtime), query);
        }
        if path.starts_with("/api/v1/market-data/options/analysis/") {
            return product_production_ports_market_data_options_analysis::read(
                Some(runtime),
                path,
                query,
            );
        }
        if path == "/api/v1/market-data/options/events" {
            return product_production_ports_market_data_options_events::read(Some(runtime), query);
        }
        if path.starts_with("/api/v1/market-data/options/chains/") {
            if !runtime.option_chains_available() {
                return Err(MarketDataOptionsReadSnapshotError::Unavailable(
                    "Futu option chain reader is not ready".to_owned(),
                ));
            }
            let request = parse_option_chain_request(path, query)?;
            let dates = runtime
                .option_chains(&request)
                .map_err(map_option_chain_error)?;
            if dates.is_empty() {
                return Err(MarketDataOptionsReadSnapshotError::Failed {
                    status: 404,
                    code: "OPTION_CHAIN_NOT_FOUND".to_owned(),
                    message: "OpenD returned no option chain dates".to_owned(),
                });
            }
            return option_chain_result(dates);
        }
        if !runtime.option_expirations_available() {
            return Err(MarketDataOptionsReadSnapshotError::Unavailable(
                "Futu option expiration reader is not ready".to_owned(),
            ));
        }
        let request = parse_option_expiration_request(path, query)?;
        let dates = runtime
            .option_expirations(&request)
            .map_err(map_option_expiration_error)?;
        if dates.is_empty() {
            return Err(MarketDataOptionsReadSnapshotError::Failed {
                status: 404,
                code: "OPTION_EXPIRATIONS_NOT_FOUND".to_owned(),
                message: "OpenD returned no option expiration dates".to_owned(),
            });
        }
        Ok(option_expiration_result(dates))
    }
}

fn parse_option_chain_request(
    path: &str,
    query: &str,
) -> Result<jftrade_integration_futu::OptionChainQuery, MarketDataOptionsReadSnapshotError> {
    let instrument = path
        .strip_prefix("/api/v1/market-data/options/chains/")
        .filter(|value| !value.is_empty() && !value.contains('/'))
        .ok_or_else(|| options_bad_request("unsupported options route"))?;
    let (market, symbol) = instrument
        .split_once('.')
        .filter(|(market, symbol)| {
            !market.is_empty() && !symbol.is_empty() && !symbol.contains('.')
        })
        .ok_or_else(|| options_bad_request("instrumentId must be MARKET.CODE"))?;
    let market = market.trim().to_ascii_uppercase();
    let market_code = match market.as_str() {
        "HK" => 1,
        "US" => 11,
        _ => return Err(options_bad_request("option chain market must be HK or US")),
    };
    let symbol = symbol.trim();
    if symbol.is_empty() || symbol.chars().any(char::is_whitespace) {
        return Err(options_bad_request("option chain symbol is invalid"));
    }
    let query_map = crate::product::product_query::QueryMap::parse(query)
        .map_err(|_| options_bad_request("invalid URL escape"))?;
    if let Some(operation) = query_map.get_first("operation")
        && !operation.trim().is_empty()
        && operation.trim() != "chain"
    {
        return Err(options_bad_request("operation must be chain"));
    }
    if let Some(requested_market) = query_map.get_first("market")
        && !requested_market.trim().is_empty()
        && requested_market.trim().to_ascii_uppercase() != market
    {
        return Err(options_bad_request("market does not match instrumentId"));
    }
    // The Go adapter has historically defaulted an omitted range to today
    // through today + 30 days. Keep that observable request contract while
    // still rejecting an explicitly blank date value.
    let today = time::OffsetDateTime::now_utc().date();
    let begin_time = chain_date_or_default(&query_map, "beginTime", format_chain_date(today))?;
    let end_time = chain_date_or_default(
        &query_map,
        "endTime",
        format_chain_date(today.saturating_add(time::Duration::days(30))),
    )?;
    let index_option_type = optional_chain_enum(&query_map, "indexOptionType", "indexOptionType")?;
    let option_type = optional_chain_enum(&query_map, "type", "type")?;
    let condition = optional_chain_enum(&query_map, "condition", "condition")?;
    let data_filter = parse_option_chain_filter(&query_map)?;
    Ok(jftrade_integration_futu::OptionChainQuery {
        market: market_code,
        symbol: symbol.to_ascii_uppercase(),
        index_option_type,
        option_type,
        condition,
        begin_time,
        end_time,
        data_filter,
    })
}

fn chain_date_or_default(
    query: &crate::product::product_query::QueryMap,
    key: &str,
    default: String,
) -> Result<String, MarketDataOptionsReadSnapshotError> {
    match query.get_first(key) {
        None => Ok(default),
        Some(value) if value.trim().is_empty() => {
            Err(options_bad_request(&format!("{key} must be YYYY-MM-DD")))
        }
        Some(value) => Ok(value.trim().to_owned()),
    }
}

fn format_chain_date(date: time::Date) -> String {
    format!(
        "{:04}-{:02}-{:02}",
        date.year(),
        u8::from(date.month()),
        date.day()
    )
}

fn optional_chain_enum(
    query: &crate::product::product_query::QueryMap,
    key: &str,
    label: &str,
) -> Result<Option<i32>, MarketDataOptionsReadSnapshotError> {
    query
        .get_first(key)
        .filter(|value| !value.trim().is_empty())
        .map(|value| {
            value
                .trim()
                .parse::<i32>()
                .ok()
                .filter(|value| (0..=2).contains(value))
                .ok_or_else(|| options_bad_request(&format!("{label} must be 0, 1, or 2")))
        })
        .transpose()
}

fn parse_option_chain_filter(
    query: &crate::product::product_query::QueryMap,
) -> Result<
    Option<jftrade_integration_futu::OptionChainDataFilter>,
    MarketDataOptionsReadSnapshotError,
> {
    let mut filter = jftrade_integration_futu::OptionChainDataFilter::default();
    let mut present = false;
    macro_rules! field {
        ($name:literal, $slot:ident) => {
            if let Some(value) = query
                .get_first($name)
                .filter(|value| !value.trim().is_empty())
            {
                filter.$slot = Some(parse_chain_filter_number(value, $name)?);
                present = true;
            }
        };
    }
    field!("impliedVolatilityMin", implied_volatility_min);
    field!("impliedVolatilityMax", implied_volatility_max);
    field!("deltaMin", delta_min);
    field!("deltaMax", delta_max);
    field!("gammaMin", gamma_min);
    field!("gammaMax", gamma_max);
    field!("vegaMin", vega_min);
    field!("vegaMax", vega_max);
    field!("thetaMin", theta_min);
    field!("thetaMax", theta_max);
    field!("rhoMin", rho_min);
    field!("rhoMax", rho_max);
    field!("netOpenInterestMin", net_open_interest_min);
    field!("netOpenInterestMax", net_open_interest_max);
    field!("openInterestMin", open_interest_min);
    field!("openInterestMax", open_interest_max);
    field!("volMin", vol_min);
    field!("volMax", vol_max);
    Ok(present.then_some(filter))
}

fn parse_chain_filter_number(
    value: &str,
    name: &str,
) -> Result<f64, MarketDataOptionsReadSnapshotError> {
    let parsed = value
        .trim()
        .parse::<f64>()
        .ok()
        .filter(|value| value.is_finite())
        .ok_or_else(|| options_bad_request(&format!("{name} must be a finite number")))?;
    Ok(parsed)
}

fn option_chain_result(
    dates: Vec<jftrade_integration_futu::OptionChainDate>,
) -> Result<Value, MarketDataOptionsReadSnapshotError> {
    let entries = dates
        .into_iter()
        .map(|date| {
            serde_json::to_value(date).map_err(|error| MarketDataOptionsReadSnapshotError::Failed {
                status: 502,
                code: "BAD_GATEWAY".to_owned(),
                message: format!("failed to serialize OpenD option chain: {error}"),
            })
        })
        .collect::<Result<Vec<_>, _>>()?;
    let as_of = super::provider_now_rfc3339();
    let count = entries.len();
    Ok(serde_json::json!({
        "provider": {
            "brokerId": "futu",
            "securityFirm": "Futu/Moomoo via OpenD",
            "featureId": "derivatives.option_chain",
            "capability": "available",
            "selectionReason": "adapter_request",
            "resolvedAt": as_of,
            "asOf": as_of,
        },
        "asOf": as_of,
        "entries": entries,
        "hasMore": false,
        "total": count,
    }))
}

fn parse_option_expiration_request(
    path: &str,
    query: &str,
) -> Result<jftrade_integration_futu::OptionExpirationQuery, MarketDataOptionsReadSnapshotError> {
    let instrument = path
        .strip_prefix("/api/v1/market-data/options/expirations/")
        .filter(|value| !value.is_empty() && !value.contains('/'))
        .ok_or_else(|| options_bad_request("unsupported options route"))?;
    let (market, symbol) = instrument
        .split_once('.')
        .filter(|(market, symbol)| {
            !market.is_empty() && !symbol.is_empty() && !symbol.contains('.')
        })
        .ok_or_else(|| options_bad_request("instrumentId must be MARKET.CODE"))?;
    let market = market.trim().to_ascii_uppercase();
    let market_code = match market.as_str() {
        "HK" => 1,
        "US" => 11,
        _ => {
            return Err(options_bad_request(
                "option expiration market must be HK or US",
            ));
        }
    };
    let symbol = symbol.trim();
    if symbol.is_empty() || symbol.chars().any(char::is_whitespace) {
        return Err(options_bad_request("option expiration symbol is invalid"));
    }
    let query_map = crate::product::product_query::QueryMap::parse(query)
        .map_err(|_| options_bad_request("invalid URL escape"))?;
    if let Some(operation) = query_map.get_first("operation")
        && !operation.trim().is_empty()
        && operation.trim() != "expirations"
    {
        return Err(options_bad_request("operation must be expirations"));
    }
    if let Some(requested_market) = query_map.get_first("market")
        && !requested_market.trim().is_empty()
        && requested_market.trim().to_ascii_uppercase() != market
    {
        return Err(options_bad_request("market does not match instrumentId"));
    }
    let index_option_type = query_map
        .get_first("indexOptionType")
        .filter(|value| !value.trim().is_empty())
        .map(|value| {
            value
                .trim()
                .parse::<i32>()
                .ok()
                .filter(|value| (0..=2).contains(value))
                .ok_or_else(|| options_bad_request("indexOptionType must be 0, 1, or 2"))
        })
        .transpose()?;
    Ok(jftrade_integration_futu::OptionExpirationQuery {
        market: market_code,
        symbol: symbol.to_ascii_uppercase(),
        index_option_type,
    })
}

fn option_expiration_result(dates: Vec<jftrade_integration_futu::OptionExpirationDate>) -> Value {
    let as_of = super::provider_now_rfc3339();
    let entries = dates
        .iter()
        .map(|date| {
            let mut entry = serde_json::Map::new();
            if let Some(strike_time) = date.strike_time.as_ref() {
                entry.insert("strikeTime".to_owned(), Value::String(strike_time.clone()));
            }
            if let Some(strike_timestamp) = date.strike_timestamp {
                entry.insert(
                    "strikeTimestamp".to_owned(),
                    serde_json::json!(strike_timestamp),
                );
            }
            entry.insert(
                "optionExpiryDateDistance".to_owned(),
                serde_json::json!(date.expiry_date_distance),
            );
            if let Some(cycle) = date.cycle {
                entry.insert("cycle".to_owned(), serde_json::json!(cycle));
            }
            entry
        })
        .map(Value::Object)
        .collect::<Vec<_>>();
    let count = entries.len();
    let broker_id = "futu";
    serde_json::json!({
        "provider": {
            "brokerId": broker_id,
            "securityFirm": "Futu/Moomoo via OpenD",
            "featureId": "derivatives.option_chain",
            "capability": "available",
            "selectionReason": "adapter_request",
            "resolvedAt": as_of,
            "asOf": as_of,
        },
        "asOf": as_of,
        "entries": entries,
        "hasMore": false,
        "total": count,
    })
}

pub(crate) struct ProductionMarketDataNewsPort {
    pub(crate) active_provider_state: Arc<ActiveProviderState>,
    pub(crate) helper: Option<HelperClient>,
    pub(crate) trade_runtime: Option<Arc<SharedTradeReadRuntime>>,
}

impl std::fmt::Debug for ProductionMarketDataNewsPort {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("ProductionMarketDataNewsPort")
            .field("has_helper", &self.helper.is_some())
            .field("has_trade_runtime", &self.trade_runtime.is_some())
            .finish()
    }
}

impl MarketDataNewsActionsReadSnapshotPort for ProductionMarketDataNewsPort {
    fn read(
        &self,
        path: &str,
        query: &str,
    ) -> Result<Value, MarketDataNewsActionsReadSnapshotError> {
        let snapshot = self.active_provider_state.snapshot();
        let provider_kind = snapshot.provider.ok_or_else(|| {
            MarketDataNewsActionsReadSnapshotError::Unavailable(
                "active market-data provider is not configured".to_owned(),
            )
        })?;
        let query_map = crate::product::product_query::QueryMap::parse(query)
            .map_err(|_| news_actions_bad_request("invalid URL escape"))?;
        if !super::provider_request_matches(provider_kind, &query_map) {
            let requested = query_map
                .get_first("brokerId")
                .or_else(|| query_map.get_first("providerBrokerId"))
                .unwrap_or_default();
            return Err(news_actions_capability(&format!(
                "requested broker {requested:?} does not match active provider"
            )));
        }
        if provider_kind == MarketDataProvider::Futu {
            if !snapshot.opend_ready {
                return Err(MarketDataNewsActionsReadSnapshotError::Unavailable(
                    "Futu OpenD news/actions runtime is not ready".to_owned(),
                ));
            }
            return product_production_ports_market_data_news_actions::read_futu(
                self.trade_runtime.as_ref(),
                path,
                query,
            );
        }
        let provider = match provider_kind {
            MarketDataProvider::Yfinance => "yfinance",
            MarketDataProvider::Akshare => "akshare",
            MarketDataProvider::Futu => unreachable!("Futu handled by OpenD branch above"),
        };
        if !snapshot.helper_ready {
            return Err(MarketDataNewsActionsReadSnapshotError::Unavailable(
                "market-data helper is not ready".to_owned(),
            ));
        }
        let Some(helper) = self.helper.clone() else {
            return Err(MarketDataNewsActionsReadSnapshotError::Unavailable(
                "market-data helper is not configured".to_owned(),
            ));
        };
        let (operation, market, symbol, query_pairs) = news_actions_helper_request(path, query)?;
        if provider == "akshare" && !matches!(market.as_str(), "SH" | "SZ") {
            return Err(news_actions_capability(
                "AKShare news/actions is only available for CN markets",
            ));
        }
        let expected_market = market.clone();
        let expected_symbol = symbol.clone();
        let result = thread::spawn(move || {
            let query_refs = query_pairs
                .iter()
                .map(|(key, value)| (*key, value.as_str()))
                .collect::<Vec<_>>();
            let runtime = tokio::runtime::Builder::new_current_thread()
                .enable_all()
                .build()
                .map_err(|error| HttpAdapterError::Unavailable(error.to_string()))?;
            runtime.block_on(helper.get_provider_json_with_query::<Value>(
                provider,
                &[operation, market.as_str(), symbol.as_str()],
                &query_refs,
            ))
        })
        .join()
        .map_err(|_| {
            MarketDataNewsActionsReadSnapshotError::Unavailable(
                "market-data helper task panicked".to_owned(),
            )
        })?;
        let payload = result.map_err(map_news_actions_helper_error)?;
        product_production_ports_market_data_news_actions::validate_news_actions_payload(
            payload,
            operation,
            &expected_market,
            &expected_symbol,
        )
    }
}

impl MarketDataNewsSearchReadSnapshotPort for ProductionMarketDataNewsPort {
    fn read(
        &self,
        path: &str,
        query: &str,
    ) -> Result<Value, MarketDataNewsSearchReadSnapshotError> {
        product_production_ports_market_data_news_search::read(self, path, query)
    }
}

type NewsActionsHelperRequest = (&'static str, String, String, Vec<(&'static str, String)>);

fn news_actions_helper_request(
    path: &str,
    query: &str,
) -> Result<NewsActionsHelperRequest, MarketDataNewsActionsReadSnapshotError> {
    let (operation, suffix) = if let Some(value) = path.strip_prefix("/api/v1/market-data/news/") {
        ("news", value)
    } else if let Some(value) = path.strip_prefix("/api/v1/market-data/corporate-actions/") {
        ("corporate-actions", value)
    } else {
        return Err(news_actions_bad_request("unsupported news/actions path"));
    };
    let mut parts = suffix.split('/');
    let market = parts.next().unwrap_or_default().trim();
    let symbol = parts.next().unwrap_or_default().trim();
    if market.is_empty() || symbol.is_empty() || parts.next().is_some() {
        return Err(news_actions_bad_request("invalid instrument"));
    }

    let query_map = crate::product::product_query::QueryMap::parse(query)
        .map_err(|_| news_actions_bad_request("invalid URL escape"))?;
    let mut query_pairs = Vec::new();
    if operation == "news" {
        if let Some(raw_limit) = query_map.get_first("limit") {
            let limit = raw_limit
                .trim()
                .parse::<u16>()
                .ok()
                .filter(|value| (1..=50).contains(value))
                .ok_or_else(|| news_actions_bad_request("limit must be between 1 and 50"))?;
            query_pairs.push(("limit", limit.to_string()));
        }
    } else {
        let from = parse_corporate_action_time(&query_map, "from")?;
        let to = parse_corporate_action_time(&query_map, "to")?;
        if let (Some((from_at, _)), Some((to_at, _))) = (&from, &to)
            && from_at > to_at
        {
            return Err(news_actions_bad_request("from must not be after to"));
        }
        if let Some((_, value)) = from {
            query_pairs.push(("from", value));
        }
        if let Some((_, value)) = to {
            query_pairs.push(("to", value));
        }
    }
    let (market, symbol) = normalize_news_actions_identity(market, symbol)?;
    Ok((operation, market, symbol, query_pairs))
}

fn parse_corporate_action_time(
    query: &crate::product::product_query::QueryMap,
    key: &'static str,
) -> Result<Option<(time::OffsetDateTime, String)>, MarketDataNewsActionsReadSnapshotError> {
    let Some(raw) = query
        .get_first(key)
        .map(str::trim)
        .filter(|value| !value.is_empty())
    else {
        return Ok(None);
    };
    let parsed =
        time::OffsetDateTime::parse(raw, &time::format_description::well_known::Rfc3339)
            .map_err(|_| news_actions_bad_request(&format!("{key} must be a valid timestamp")))?;
    let normalized = parsed
        .to_offset(time::UtcOffset::UTC)
        .format(&time::format_description::well_known::Rfc3339)
        .map_err(|_| news_actions_bad_request(&format!("{key} must be a valid timestamp")))?;
    Ok(Some((parsed, normalized)))
}

fn normalize_news_actions_identity(
    market: &str,
    symbol: &str,
) -> Result<(String, String), MarketDataNewsActionsReadSnapshotError> {
    let mut market = match market.trim().to_ascii_uppercase().as_str() {
        "USA" | "NYSE" | "NASDAQ" | "AMEX" => "US".to_owned(),
        "HKEX" | "HKG" => "HK".to_owned(),
        "CNSH" | "SHH" | "SSE" | "SHSE" => "SH".to_owned(),
        "CNSZ" | "SHZ" | "SZSE" | "SHE" => "SZ".to_owned(),
        value => value.to_owned(),
    };
    let mut symbol = symbol.trim().to_ascii_uppercase();
    if let Some((prefix, code)) = symbol
        .split_once('.')
        .filter(|(prefix, _)| news_actions_market_token(prefix))
    {
        let exchange = match prefix {
            "SH" | "CNSH" | "SHH" | "SSE" | "SHSE" => Some("SH"),
            "SZ" | "CNSZ" | "SHZ" | "SZSE" | "SHE" => Some("SZ"),
            _ => None,
        };
        if market == "CN" {
            if let Some(exchange) = exchange {
                market = exchange.to_owned();
                symbol = code.to_owned();
            } else {
                return Err(news_actions_bad_request("invalid instrument"));
            }
        } else if exchange.is_some_and(|value| value.eq_ignore_ascii_case(&market))
            || prefix.eq_ignore_ascii_case(&market)
        {
            symbol = code.to_owned();
        } else {
            return Err(news_actions_bad_request("invalid instrument"));
        }
    }
    if !matches!(market.as_str(), "US" | "HK" | "SH" | "SZ")
        || symbol.is_empty()
        || symbol.contains('/')
        || symbol.chars().any(char::is_whitespace)
    {
        return Err(news_actions_bad_request("invalid instrument"));
    }
    if market == "HK" && symbol.chars().all(|value| value.is_ascii_digit()) && symbol.len() < 5 {
        symbol = format!("{symbol:0>5}");
    }
    Ok((market, symbol))
}

fn news_actions_market_token(value: &str) -> bool {
    matches!(
        value.to_ascii_uppercase().as_str(),
        "US" | "USA"
            | "NYSE"
            | "NASDAQ"
            | "AMEX"
            | "HK"
            | "HKEX"
            | "HKG"
            | "CN"
            | "SH"
            | "CNSH"
            | "SHH"
            | "SSE"
            | "SHSE"
            | "SZ"
            | "CNSZ"
            | "SHZ"
            | "SZSE"
            | "SHE"
    )
}

fn news_actions_bad_request(message: &str) -> MarketDataNewsActionsReadSnapshotError {
    MarketDataNewsActionsReadSnapshotError::Failed {
        status: 400,
        code: "BAD_REQUEST".to_owned(),
        message: message.to_owned(),
        retry_after_seconds: None,
    }
}

fn map_news_actions_helper_error(
    error: HttpAdapterError,
) -> MarketDataNewsActionsReadSnapshotError {
    match error {
        HttpAdapterError::Remote {
            status,
            code,
            message,
            retry_after_seconds,
        } => MarketDataNewsActionsReadSnapshotError::Failed {
            status,
            code: if code.is_empty() {
                "BAD_GATEWAY".to_owned()
            } else {
                code
            },
            message,
            retry_after_seconds,
        },
        HttpAdapterError::Timeout => MarketDataNewsActionsReadSnapshotError::Failed {
            status: 504,
            code: "GATEWAY_TIMEOUT".to_owned(),
            message: "market-data helper request timed out".to_owned(),
            retry_after_seconds: None,
        },
        HttpAdapterError::InvalidResponse(message) => {
            MarketDataNewsActionsReadSnapshotError::Failed {
                status: 502,
                code: "BAD_GATEWAY".to_owned(),
                message,
                retry_after_seconds: None,
            }
        }
        HttpAdapterError::Unavailable(message) => {
            MarketDataNewsActionsReadSnapshotError::Unavailable(message)
        }
        other => MarketDataNewsActionsReadSnapshotError::Failed {
            status: 500,
            code: "MARKET_DATA_NEWS_FAILED".to_owned(),
            message: other.to_string(),
            retry_after_seconds: None,
        },
    }
}

fn news_actions_capability(message: &str) -> MarketDataNewsActionsReadSnapshotError {
    MarketDataNewsActionsReadSnapshotError::Failed {
        status: 409,
        code: "CAPABILITY_UNAVAILABLE".to_owned(),
        message: message.to_owned(),
        retry_after_seconds: None,
    }
}

pub(crate) use product_production_ports_market_data_prediction::ProductionMarketDataPredictionPort;
