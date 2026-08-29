//! Production market-data provider actions adapter.

use std::sync::Arc;
use serde::Deserialize;
use serde_json::{Value, json};

use crate::product::product_market_data_provider_actions_port::{
    BATCH_SNAPSHOTS_PATH, MarketDataProviderActionsFuture, MarketDataProviderActionsPort,
    MarketDataProviderActionsPortError, MarketDataProviderActionsRequest,
    NORMALIZE_INSTRUMENT_PATH, OPTION_ANALYSIS_PATH, PREDICTION_COMBO_QUOTES_PATH,
    ZERO_DTE_CONTRACTS_PATH, is_market_data_provider_action_path,
};
use crate::product::{MarketDataQuoteReadSnapshotError, MarketDataQuoteReadSnapshotPort};
use crate::product::product_active_provider_state::ActiveProviderState;
use super::super::product_production_ports_trade::SharedTradeReadRuntime;

#[derive(Clone, Default)]
pub(crate) struct ProductionMarketDataProviderActionsPort {
    quote_port: Option<Arc<dyn MarketDataQuoteReadSnapshotPort>>,
    trade_runtime: Option<Arc<SharedTradeReadRuntime>>,
    active_provider_state: Option<Arc<ActiveProviderState>>,
}

impl std::fmt::Debug for ProductionMarketDataProviderActionsPort {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("ProductionMarketDataProviderActionsPort")
            .field("has_quote_port", &self.quote_port.is_some())
            .field("has_trade_runtime", &self.trade_runtime.is_some())
            .field("has_active_provider_state", &self.active_provider_state.is_some())
            .finish()
    }
}

impl ProductionMarketDataProviderActionsPort {
    pub(crate) fn new(quote_port: Option<Arc<dyn MarketDataQuoteReadSnapshotPort>>) -> Self {
        Self {
            quote_port,
            trade_runtime: None,
            active_provider_state: None,
        }
    }

    pub(crate) fn with_trade_runtime(
        mut self,
        trade_runtime: Option<Arc<SharedTradeReadRuntime>>,
    ) -> Self {
        self.trade_runtime = trade_runtime;
        self
    }

    pub(crate) fn with_active_provider_state(
        mut self,
        active_provider_state: Option<Arc<ActiveProviderState>>,
    ) -> Self {
        self.active_provider_state = active_provider_state;
        self
    }
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct NormalizeRequestBody {
    market: Option<String>,
    symbol: Option<String>,
    code: Option<String>,
    instrument_id: Option<String>,
    instrument: Option<String>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct BatchSnapshotsRequestBody {
    instrument_ids: Option<Vec<String>>,
    symbols: Option<Vec<String>>,
    instruments: Option<Vec<String>>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct ZeroDteContractsRequest {
    market: Option<String>,
    underlying_instrument_id: Option<String>,
    expiry_timestamp: Option<i64>,
    chain: Option<ZeroDteChainLocator>,
    sort: Option<String>,
    option_type: Option<String>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct ZeroDteChainLocator {
    product_code: String,
    multiplier: Option<f64>,
    contract_size: Option<f64>,
    expiration_type: Option<i32>,
}

fn parse_zero_dte_owner(
    value: Option<&str>,
) -> Result<jftrade_integration_futu::OptionEventSecurity, MarketDataProviderActionsPortError> {
    let value = value.ok_or_else(|| {
        action_bad_request(
            "OPTION_CHAIN_CONTEXT_REQUIRED",
            "invalid 0DTE chain context",
        )
    })?;
    let (market, code) = value.trim().split_once('.').ok_or_else(|| {
        action_bad_request(
            "OPTION_CHAIN_CONTEXT_REQUIRED",
            "invalid 0DTE chain context",
        )
    })?;
    let market = market.trim().to_ascii_uppercase();
    let code = code.trim().to_ascii_uppercase();
    if market != "US" || code.is_empty() || code.contains('.') || code.chars().any(char::is_whitespace)
    {
        return Err(action_bad_request(
            "OPTION_CHAIN_CONTEXT_REQUIRED",
            "invalid 0DTE chain context",
        ));
    }
    Ok(jftrade_integration_futu::OptionEventSecurity {
        market: market.clone(),
        code: code.clone(),
        quote_market: market.clone(),
        trade_market: market.clone(),
        instrument_id: format!("{market}.{code}"),
    })
}

fn parse_zero_dte_contract_sort(
    value: Option<&str>,
) -> Result<Option<i32>, MarketDataProviderActionsPortError> {
    let Some(value) = value.map(str::trim).filter(|value| !value.is_empty()) else {
        return Ok(None);
    };
    let sort = match value.to_ascii_lowercase().as_str() {
        "default" => return Ok(None),
        "volume" => 1,
        "open_interest" | "oi" => 2,
        "iv" => 3,
        "delta" => 4,
        _ => {
            return Err(action_bad_request(
                "BAD_REQUEST",
                "unsupported 0DTE contract sort",
            ));
        }
    };
    Ok(Some(sort))
}

fn parse_zero_dte_option_type(
    value: Option<&str>,
) -> Result<Vec<jftrade_integration_futu::EventIndicator>, MarketDataProviderActionsPortError> {
    let Some(value) = value.map(str::trim).filter(|value| !value.is_empty()) else {
        return Ok(Vec::new());
    };
    let option_type = match value.to_ascii_lowercase().as_str() {
        "all" => return Ok(Vec::new()),
        "call" => 1,
        "put" => 2,
        _ => {
            return Err(action_bad_request(
                "BAD_REQUEST",
                "optionType must be all, call, or put",
            ));
        }
    };
    Ok(vec![jftrade_integration_futu::EventIndicator {
        indicator_type: 1,
        value: Some(jftrade_integration_futu::EventIndicatorValue {
            value_list: vec![option_type],
            value_interval: None,
            string_value_list: Vec::new(),
            security_list: Vec::new(),
        }),
    }])
}

fn action_bad_request(code: &str, message: &str) -> MarketDataProviderActionsPortError {
    MarketDataProviderActionsPortError::Failed {
        status: 400,
        code: code.to_owned(),
        message: message.to_owned(),
        retry_after_seconds: None,
    }
}

fn map_zero_dte_contract_action_error(
    error: jftrade_integration_futu::OptionZeroDteContractQueryError,
) -> MarketDataProviderActionsPortError {
    use jftrade_integration_futu::OptionZeroDteContractQueryError as Error;
    match error {
        Error::InvalidQuery(message) => action_bad_request("BAD_REQUEST", &message),
        other => MarketDataProviderActionsPortError::Failed {
            status: 502,
            code: "BAD_GATEWAY".to_owned(),
            message: other.to_string(),
            retry_after_seconds: None,
        },
    }
}

impl MarketDataProviderActionsPort for ProductionMarketDataProviderActionsPort {
    fn dispatch<'a>(
        &'a self,
        request: &'a MarketDataProviderActionsRequest,
    ) -> MarketDataProviderActionsFuture<'a> {
        Box::pin(async move {
            let path = request.path.as_str();

            if path == NORMALIZE_INSTRUMENT_PATH {
                return self.normalize_instrument(request);
            }
            if path == BATCH_SNAPSHOTS_PATH {
                return self.batch_snapshots(request).await;
            }
            if path == ZERO_DTE_CONTRACTS_PATH
                || path == OPTION_ANALYSIS_PATH
                || path.starts_with("/api/v1/market-data/options/analysis/")
            {
                if path == ZERO_DTE_CONTRACTS_PATH {
                    return self.zero_dte_contracts(request);
                }
                return Err(MarketDataProviderActionsPortError::Unavailable(
                    "options analysis provider is not configured".to_owned(),
                ));
            }
            if path == PREDICTION_COMBO_QUOTES_PATH {
                return Err(MarketDataProviderActionsPortError::Unavailable(
                    "prediction combo quotes provider is not configured".to_owned(),
                ));
            }

            if !is_market_data_provider_action_path(path) {
                return Err(MarketDataProviderActionsPortError::Unavailable(format!(
                    "unsupported provider action path: {path}"
                )));
            }

            Err(MarketDataProviderActionsPortError::Unavailable(
                "market-data provider action is not configured".to_owned(),
            ))
        })
    }
}

impl ProductionMarketDataProviderActionsPort {
    fn zero_dte_contracts(
        &self,
        request: &MarketDataProviderActionsRequest,
    ) -> Result<Value, MarketDataProviderActionsPortError> {
        let body: ZeroDteContractsRequest = serde_json::from_slice(&request.body).map_err(|_| {
            action_bad_request("OPTION_CHAIN_CONTEXT_REQUIRED", "invalid 0DTE chain context")
        })?;
        let market = body
            .market
            .as_deref()
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .ok_or_else(|| {
                action_bad_request("OPTION_CHAIN_CONTEXT_REQUIRED", "invalid 0DTE chain context")
            })?
            .to_ascii_uppercase();
        if market != "US" {
            return Err(action_bad_request(
                "BAD_REQUEST",
                "0DTE option research is available only in US market",
            ));
        }
        let owner = parse_zero_dte_owner(body.underlying_instrument_id.as_deref())?;
        let expiry = body.expiry_timestamp.unwrap_or_default();
        let locator = body.chain.ok_or_else(|| {
            action_bad_request("OPTION_CHAIN_CONTEXT_REQUIRED", "invalid 0DTE chain context")
        })?;
        let product_code = locator.product_code.trim().to_owned();
        if expiry <= 0 || product_code.is_empty() {
            return Err(action_bad_request(
                "OPTION_CHAIN_CONTEXT_REQUIRED",
                "invalid 0DTE chain context",
            ));
        }
        let sort_type = parse_zero_dte_contract_sort(body.sort.as_deref())?;
        let filters = parse_zero_dte_option_type(body.option_type.as_deref())?;
        let query = jftrade_integration_futu::OptionZeroDteContractQuery {
            owner: owner.clone(),
            strike_date_timestamp: expiry,
            chain_info: jftrade_integration_futu::OptionZeroDteChainInfo {
                strike_date_timestamp: Some(expiry),
                product_code: Some(product_code),
                multiplier: locator.multiplier,
                contract_share_size: locator.contract_size,
                expiration_type: locator.expiration_type,
                underlying: Some(owner),
            },
            sort_type,
            is_asc: None,
            filters,
        };
        let runtime = self.trade_runtime.as_ref().ok_or_else(|| {
            MarketDataProviderActionsPortError::Unavailable(
                "Futu 0DTE contract reader is not configured".to_owned(),
            )
        })?;
        if !runtime.option_zero_dte_contract_available() {
            return Err(MarketDataProviderActionsPortError::Unavailable(
                "Futu 0DTE contract reader is not ready".to_owned(),
            ));
        }
        if let Some(active_provider_state) = self.active_provider_state.as_ref() {
            let snapshot = active_provider_state.snapshot();
            if snapshot.provider != Some(jftrade_settings::MarketDataProvider::Futu)
                || !snapshot.opend_ready
            {
                return Err(MarketDataProviderActionsPortError::Unavailable(
                    "Futu 0DTE contract provider is not ready".to_owned(),
                ));
            }
        }
        let items = runtime
            .option_zero_dte_contract(&query)
            .map_err(map_zero_dte_contract_action_error)?;
        let entries = items
            .into_iter()
            .map(|item| {
                serde_json::to_value(item).map_err(|error| {
                    MarketDataProviderActionsPortError::Failed {
                        status: 502,
                        code: "BAD_GATEWAY".to_owned(),
                        message: format!("failed to serialize OpenD 0DTE contracts: {error}"),
                        retry_after_seconds: None,
                    }
                })
            })
            .collect::<Result<Vec<_>, _>>()?;
        let total = entries.len();
        let as_of = super::super::provider_now_rfc3339();
        Ok(json!({
            "provider": {
                "brokerId": "futu",
                "securityFirm": "Futu/Moomoo via OpenD",
                "featureId": "derivatives.option_events",
                "capability": "available",
                "selectionReason": "adapter_request",
                "resolvedAt": as_of,
                "asOf": as_of,
            },
            "asOf": as_of,
            "entries": entries,
            "hasMore": false,
            "total": total,
        }))
    }

    fn normalize_instrument(
        &self,
        request: &MarketDataProviderActionsRequest,
    ) -> Result<Value, MarketDataProviderActionsPortError> {
        let body: NormalizeRequestBody = serde_json::from_slice(&request.body).map_err(|_| {
            MarketDataProviderActionsPortError::Failed {
                status: 400,
                code: "BAD_REQUEST".to_owned(),
                message: "invalid normalize request".to_owned(),
                retry_after_seconds: None,
            }
        })?;

        let candidate = body
            .instrument_id
            .clone()
            .or_else(|| body.instrument.clone())
            .or_else(|| body.code.clone())
            .or_else(|| body.symbol.clone());

        let (market, symbol) = if let Some(candidate) = candidate {
            let trimmed = candidate.trim();
            if trimmed.contains('.') {
                let mut parts = trimmed.splitn(2, '.');
                let m = parts.next().unwrap_or("US").to_ascii_uppercase();
                let s = parts.next().unwrap_or("").to_ascii_uppercase();
                (m, s)
            } else {
                let m = body
                    .market
                    .as_deref()
                    .map(str::trim)
                    .filter(|m| !m.is_empty())
                    .unwrap_or("US")
                    .to_ascii_uppercase();
                (m, trimmed.to_ascii_uppercase())
            }
        } else if let (Some(m), Some(s)) = (body.market.as_deref(), body.symbol.as_deref()) {
            (m.trim().to_ascii_uppercase(), s.trim().to_ascii_uppercase())
        } else {
            return Err(MarketDataProviderActionsPortError::Failed {
                status: 400,
                code: "MARKET_INSTRUMENT_INVALID".to_owned(),
                message: "symbol or code is required".to_owned(),
                retry_after_seconds: None,
            });
        };

        if symbol.is_empty() {
            return Err(MarketDataProviderActionsPortError::Failed {
                status: 400,
                code: "MARKET_INSTRUMENT_INVALID".to_owned(),
                message: "symbol or code is required".to_owned(),
                retry_after_seconds: None,
            });
        }

        let instrument_id = format!("{market}.{symbol}");
        Ok(json!({
            "code": symbol,
            "instrumentId": instrument_id,
            "market": market,
            "prefix": market,
            "resolvedMarket": market,
            "symbol": format!("{market}.{symbol}"),
        }))
    }

    async fn batch_snapshots(
        &self,
        request: &MarketDataProviderActionsRequest,
    ) -> Result<Value, MarketDataProviderActionsPortError> {
        let body: BatchSnapshotsRequestBody =
            serde_json::from_slice(&request.body).map_err(|_| {
                MarketDataProviderActionsPortError::Failed {
                    status: 400,
                    code: "BAD_REQUEST".to_owned(),
                    message: "invalid request body".to_owned(),
                    retry_after_seconds: None,
                }
            })?;

        let quote_port = self.quote_port.as_ref().ok_or_else(|| {
            MarketDataProviderActionsPortError::Unavailable(
                "market-data quote provider is not configured".to_owned(),
            )
        })?;

        let mut raw_items = Vec::new();
        if let Some(list) = body.instrument_ids {
            raw_items.extend(list);
        }
        if let Some(list) = body.symbols {
            raw_items.extend(list);
        }
        if let Some(list) = body.instruments {
            raw_items.extend(list);
        }

        let mut requested_symbols = Vec::new();
        for item in raw_items {
            let trimmed = item.trim();
            if trimmed.is_empty() {
                continue;
            }
            let formatted = if trimmed.contains('.') {
                let mut parts = trimmed.splitn(2, '.');
                let m = parts.next().unwrap_or("US").to_ascii_uppercase();
                let s = parts.next().unwrap_or("").to_ascii_uppercase();
                format!("{m}.{s}")
            } else {
                format!("US.{}", trimmed.to_ascii_uppercase())
            };
            if !requested_symbols.contains(&formatted) {
                requested_symbols.push(formatted);
            }
        }

        if requested_symbols.is_empty() {
            return Err(MarketDataProviderActionsPortError::Failed {
                status: 400,
                code: "BAD_REQUEST".to_owned(),
                message: "invalid product feature query: at least one instrumentId is required".to_owned(),
                retry_after_seconds: None,
            });
        }

        let mut entries = Vec::new();
        let mut snapshots_map = serde_json::Map::new();

        for symbol_str in &requested_symbols {
            let (market, symbol) = if symbol_str.contains('.') {
                let mut parts = symbol_str.splitn(2, '.');
                let m = parts.next().unwrap_or("US");
                let s = parts.next().unwrap_or(symbol_str);
                (m, s)
            } else {
                ("US", symbol_str.as_str())
            };

            let snapshot_path = format!("/api/v1/market-data/snapshots/{market}/{symbol}");
            let snap_res = quote_port
                .read(&snapshot_path, "")
                .await
                .map_err(|error| match error {
                    MarketDataQuoteReadSnapshotError::Unavailable(msg) => {
                        MarketDataProviderActionsPortError::Unavailable(msg)
                    }
                    MarketDataQuoteReadSnapshotError::Failed {
                        status,
                        code,
                        message,
                        retry_after_seconds,
                    } => MarketDataProviderActionsPortError::Failed {
                        status,
                        code,
                        message,
                        retry_after_seconds,
                    },
                })?;

            let snap_obj = snap_res.get("snapshot").ok_or_else(|| {
                MarketDataProviderActionsPortError::Failed {
                    status: 502,
                    code: "BROKER_FEATURE_FAILED".to_owned(),
                    message: format!("snapshot payload missing for {symbol_str}"),
                    retry_after_seconds: None,
                }
            })?;

            let price = snap_obj
                .get("price")
                .and_then(|p| p.as_str())
                .and_then(|p| p.parse::<f64>().ok())
                .or_else(|| snap_obj.get("price").and_then(Value::as_f64))
                .ok_or_else(|| MarketDataProviderActionsPortError::Failed {
                    status: 502,
                    code: "BROKER_FEATURE_FAILED".to_owned(),
                    message: format!("snapshot price missing for {symbol_str}"),
                    retry_after_seconds: None,
                })?;

            let observed_at = snap_obj
                .get("observedAt")
                .and_then(Value::as_str)
                .filter(|s| !s.is_empty())
                .ok_or_else(|| MarketDataProviderActionsPortError::Failed {
                    status: 502,
                    code: "BROKER_FEATURE_FAILED".to_owned(),
                    message: format!("snapshot observedAt missing for {symbol_str}"),
                    retry_after_seconds: None,
                })?
                .to_owned();

            entries.push(json!({
                "lastPrice": price,
                "observedAt": observed_at,
                "symbol": symbol_str,
            }));

            snapshots_map.insert(
                symbol_str.clone(),
                json!({
                    "instrumentId": symbol_str,
                    "price": price,
                    "observedAt": observed_at,
                }),
            );
        }

        let as_of = current_utc_rfc3339();
        let query_map = parse_query(&request.query);
        let broker_id = query_map
            .get("brokerId")
            .map(String::as_str)
            .unwrap_or("active");
        let selection_reason = if query_map.contains_key("brokerId") {
            "explicit_broker"
        } else {
            "active_provider"
        };

        Ok(json!({
            "asOf": as_of,
            "entries": entries,
            "metadata": {
                "requestedSymbols": requested_symbols,
                "subscriptionCreated": false,
            },
            "provider": {
                "asOf": as_of,
                "brokerId": broker_id,
                "capability": "available",
                "featureId": "market.snapshots",
                "resolvedAt": as_of,
                "selectionReason": selection_reason,
            },
            "snapshots": snapshots_map,
        }))
    }
}

fn parse_query(query_str: &str) -> std::collections::BTreeMap<String, String> {
    let mut map = std::collections::BTreeMap::new();
    for pair in query_str.split('&') {
        if pair.is_empty() {
            continue;
        }
        let mut parts = pair.splitn(2, '=');
        let key = parts.next().unwrap_or_default().trim();
        let value = parts.next().unwrap_or_default().trim();
        if !key.is_empty() {
            map.insert(key.to_owned(), value.to_owned());
        }
    }
    map
}

fn current_utc_rfc3339() -> String {
    time::OffsetDateTime::now_utc()
        .format(&time::format_description::well_known::Rfc3339)
        .unwrap_or_else(|_| "1970-01-01T00:00:00Z".to_owned())
}
