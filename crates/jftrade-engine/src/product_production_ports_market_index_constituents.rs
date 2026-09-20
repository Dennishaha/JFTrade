//! Production adapter for CN index constituent listings.
//!
//! The market-data helper owns the feed
//! (`GET /providers/akshare/index-constituents/{market}/{symbol}`). The
//! reference exposes it as the ADK tool `market.index_constituents` only, so
//! this adapter stays off the public HTTP contract and is consumed by the
//! production MCP executor.

use std::sync::Arc;
use std::thread;

use jftrade_integration_marketdata_helper::{HelperClient, HttpAdapterError};
use jftrade_settings::MarketDataProvider;
use serde_json::{Map, Value, json};

use crate::product::product_production_ports::product_production_ports_helper_runtime::normalize_helper_remote_error;
use crate::product::{
    ActiveProviderState, MarketIndexConstituentsReadError, MarketIndexConstituentsReadPort,
};

/// CN indices covered by the AKShare sidecar.
const CN_INDEX_MARKETS: &[&str] = &["SH", "SZ"];

pub(crate) struct ProductionMarketIndexConstituentsPort {
    pub(crate) active_provider_state: Arc<ActiveProviderState>,
    pub(crate) helper: Option<HelperClient>,
}

impl std::fmt::Debug for ProductionMarketIndexConstituentsPort {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("ProductionMarketIndexConstituentsPort")
            .field("has_helper", &self.helper.is_some())
            .finish()
    }
}

impl MarketIndexConstituentsReadPort for ProductionMarketIndexConstituentsPort {
    fn read(
        &self,
        market: &str,
        symbol: &str,
        limit: usize,
    ) -> Result<Value, MarketIndexConstituentsReadError> {
        let snapshot = self.active_provider_state.snapshot();
        let provider = snapshot.provider.ok_or_else(|| {
            unavailable("active market-data provider is not configured")
        })?;
        if provider != MarketDataProvider::Akshare {
            return Err(capability_unsupported(provider, "index constituents"));
        }
        // Identity and market coverage are answered before the transport
        // readiness gate: an unsupported index stays a capability error even
        // when the helper is still warming.
        let (market, symbol) = resolve_cn_index(market, symbol)?;
        if !snapshot.helper_ready {
            return Err(unavailable("market-data helper is not ready"));
        }
        let helper = self
            .helper
            .as_ref()
            .ok_or_else(|| unavailable("market-data helper is not configured"))?;
        let payload = fetch_json(helper, &market, &symbol, limit)?;
        project_constituents(&payload, &market, &symbol)
    }
}

fn provider_label(provider: MarketDataProvider) -> &'static str {
    match provider {
        MarketDataProvider::Futu => "futu-opend",
        MarketDataProvider::Yfinance => "yahoo-finance",
        MarketDataProvider::Akshare => "akshare",
    }
}

/// Resolve the requested index to the exchange leaf the helper serves.
///
/// The reference facade accepts the `CN` aggregate only when the symbol
/// carries its exchange prefix (`CN.SH.000300`) and answers a capability error
/// for every market AKShare does not index.  A bare CN code stays an invalid
/// instrument because the feed cannot infer its exchange.
fn resolve_cn_index(
    market: &str,
    symbol: &str,
) -> Result<(String, String), MarketIndexConstituentsReadError> {
    let market = market.trim().to_ascii_uppercase();
    let symbol = symbol.trim().to_ascii_uppercase();
    if market == "CN" {
        let Some((prefix, code)) = symbol.split_once('.') else {
            return Err(invalid_request(
                "CN symbols must use SH.<code> or SZ.<code>",
            ));
        };
        let prefix = prefix.trim().to_ascii_uppercase();
        let code = code.trim();
        if !CN_INDEX_MARKETS.contains(&prefix.as_str()) || code.is_empty() {
            return Err(invalid_request(
                "CN symbols must use SH.<code> or SZ.<code>",
            ));
        }
        return Ok((prefix, code.to_owned()));
    }
    if !CN_INDEX_MARKETS.contains(&market.as_str()) {
        return Err(capability_unsupported(
            MarketDataProvider::Akshare,
            &format!("index constituents for market \"{market}\""),
        ));
    }
    Ok((market, symbol))
}

fn fetch_json(
    helper: &HelperClient,
    market: &str,
    symbol: &str,
    limit: usize,
) -> Result<Value, MarketIndexConstituentsReadError> {
    let helper = helper.clone();
    let market = market.to_owned();
    let symbol = symbol.to_owned();
    let limit = limit.to_string();
    let result = thread::spawn(move || {
        let runtime = tokio::runtime::Builder::new_current_thread()
            .enable_all()
            .build()
            .map_err(|error| HttpAdapterError::Unavailable(error.to_string()))?;
        let segments = ["index-constituents", market.as_str(), symbol.as_str()];
        runtime.block_on(
            helper.get_provider_json_with_query::<Value>(
                "akshare",
                &segments,
                &[("limit", limit.as_str())],
            ),
        )
    })
    .join()
    .map_err(|_| unavailable("index constituents helper task panicked"))?;
    result.map_err(map_helper_error)
}

fn map_helper_error(error: HttpAdapterError) -> MarketIndexConstituentsReadError {
    match error {
        HttpAdapterError::Remote {
            status,
            code,
            message,
            retry_after_seconds,
        } => {
            let (status, code, message, retry_after_seconds) = normalize_helper_remote_error(
                status,
                &code,
                message,
                retry_after_seconds,
                "BAD_GATEWAY",
            );
            if matches!(code.as_str(), "AKSHARE_UNSUPPORTED" | "CAPABILITY_UNSUPPORTED") {
                // The reference AKShare client turns the helper's 400
                // capability rejection into `ErrCapabilityUnsupported`, so the
                // tool reports the same 409 class instead of a caller error.
                return MarketIndexConstituentsReadError::Failed {
                    status: 409,
                    code: "MARKET_DATA_CAPABILITY_UNSUPPORTED".to_owned(),
                    message,
                    retry_after_seconds: None,
                };
            }
            MarketIndexConstituentsReadError::Failed {
                status,
                code,
                message,
                retry_after_seconds,
            }
        }
        HttpAdapterError::Timeout => MarketIndexConstituentsReadError::Failed {
            status: 504,
            code: "GATEWAY_TIMEOUT".to_owned(),
            message: "market-data helper request timed out".to_owned(),
            retry_after_seconds: None,
        },
        HttpAdapterError::InvalidResponse(message) => MarketIndexConstituentsReadError::Failed {
            status: 502,
            code: "BAD_GATEWAY".to_owned(),
            message,
            retry_after_seconds: None,
        },
        HttpAdapterError::Unavailable(message) => {
            MarketIndexConstituentsReadError::Unavailable(message)
        }
        other => MarketIndexConstituentsReadError::Failed {
            status: 502,
            code: "BAD_GATEWAY".to_owned(),
            message: other.to_string(),
            retry_after_seconds: None,
        },
    }
}

/// Project the helper payload onto the reference contract.
///
/// The upstream identity has to match the requested index and every entry
/// needs a non-empty code, so a malformed upstream payload can never be
/// presented as a successful empty member list.
fn project_constituents(
    payload: &Value,
    market: &str,
    symbol: &str,
) -> Result<Value, MarketIndexConstituentsReadError> {
    let instrument_id = format!("{market}.{symbol}");
    let upstream_market = required_text(payload, "market")?;
    let upstream_symbol = required_text(payload, "symbol")?;
    let upstream_id = required_text(payload, "instrument_id")?;
    if !upstream_market.eq_ignore_ascii_case(market)
        || upstream_symbol != symbol
        || upstream_id != instrument_id
    {
        return Err(bad_gateway(format!(
            "index constituents identity does not match {instrument_id}"
        )));
    }
    let entries = payload
        .get("constituents")
        .and_then(Value::as_array)
        .ok_or_else(|| bad_gateway("index constituents payload has no constituents array"))?;
    let mut constituents = Vec::with_capacity(entries.len());
    for entry in entries {
        let code = required_text(entry, "code")?;
        let mut item = Map::new();
        item.insert("code".to_owned(), Value::String(code));
        item.insert(
            "name".to_owned(),
            Value::String(optional_text(entry, "name")),
        );
        item.insert(
            "weight".to_owned(),
            entry.get("weight").cloned().unwrap_or(Value::Null),
        );
        constituents.push(Value::Object(item));
    }
    Ok(json!({
        "market": market,
        "symbol": symbol,
        "instrumentId": instrument_id,
        "constituents": constituents,
        "source": optional_text(payload, "source"),
    }))
}

fn required_text(payload: &Value, key: &str) -> Result<String, MarketIndexConstituentsReadError> {
    let text = optional_text(payload, key);
    if text.is_empty() {
        return Err(bad_gateway(format!(
            "index constituents payload field {key} is missing"
        )));
    }
    Ok(text)
}

fn optional_text(payload: &Value, key: &str) -> String {
    payload
        .get(key)
        .and_then(Value::as_str)
        .unwrap_or_default()
        .trim()
        .to_owned()
}

fn unavailable(message: impl Into<String>) -> MarketIndexConstituentsReadError {
    MarketIndexConstituentsReadError::Unavailable(message.into())
}

/// Mirror the shared market-data capability error: the descriptor keeps
/// `409 MARKET_DATA_CAPABILITY_UNSUPPORTED` and names the active provider so a
/// Futu or yfinance deployment sees which feed is missing instead of a generic
/// failure.
fn capability_unsupported(
    provider: MarketDataProvider,
    capability: &str,
) -> MarketIndexConstituentsReadError {
    MarketIndexConstituentsReadError::Failed {
        status: 409,
        code: "MARKET_DATA_CAPABILITY_UNSUPPORTED".to_owned(),
        message: format!(
            "market-data capability is unsupported: active provider \"{}\" does not support {capability}",
            provider_label(provider)
        ),
        retry_after_seconds: None,
    }
}

fn invalid_request(message: impl Into<String>) -> MarketIndexConstituentsReadError {
    MarketIndexConstituentsReadError::Failed {
        status: 400,
        code: "BAD_REQUEST".to_owned(),
        message: message.into(),
        retry_after_seconds: None,
    }
}

fn bad_gateway(message: impl Into<String>) -> MarketIndexConstituentsReadError {
    MarketIndexConstituentsReadError::Failed {
        status: 502,
        code: "BAD_GATEWAY".to_owned(),
        message: message.into(),
        retry_after_seconds: None,
    }
}

#[cfg(test)]
#[path = "product_production_ports_market_index_constituents_tests.rs"]
mod tests;
