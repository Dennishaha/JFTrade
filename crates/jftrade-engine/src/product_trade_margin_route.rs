//! Production margin-ratio route behavior and its rate-limit resilience policy.

use std::sync::Arc;

use jftrade_integration_futu::{TradeMarginRatioSnapshot, TradeReadPort, TradeSecurity};
use serde_json::{Value, json};

use super::product_trade_margin_cache::{
    MARGIN_RATIO_CACHE_FALLBACK_TTL, MARGIN_RATIO_CACHE_TTL,
};
use super::{
    BrokerReadSnapshotError, ResolvedTradeRequest,
    SharedTradeReadRuntime, TradeRequest, checked_at, map_broker_header_error,
    margin_ratio_value, session_error, qot_market_label,
};

impl SharedTradeReadRuntime {
    pub(crate) fn margin_ratio_cache_get(
        &self,
        key: &str,
        max_age: std::time::Duration,
    ) -> Option<Vec<TradeMarginRatioSnapshot>> {
        self.margin_ratio_cache.get(key, max_age)
    }

    pub(crate) fn margin_ratio_cache_put(
        &self,
        key: String,
        snapshots: Vec<TradeMarginRatioSnapshot>,
    ) {
        self.margin_ratio_cache.put(key, snapshots);
    }
}

pub(super) fn read_margin_ratios(
    request: &TradeRequest,
    client: &dyn TradeReadPort,
    runtime: Option<&Arc<SharedTradeReadRuntime>>,
) -> Result<Value, BrokerReadSnapshotError> {
    let securities = request
        .securities()
        .map_err(BrokerReadSnapshotError::Invalid)?;
    let resolved = request
        .resolve_account_real_for_market(client, securities[0].market)
        .map_err(map_broker_header_error)?;
    let cache_key = margin_ratio_cache_key(&resolved, &securities);
    if let Some(ratios) = runtime.and_then(|runtime| {
        runtime.margin_ratio_cache_get(&cache_key, MARGIN_RATIO_CACHE_TTL)
    }) {
        return Ok(project_margin_ratios(&resolved, ratios));
    }

    let ratios = match client.read_margin_ratios(resolved.header.clone(), securities) {
        Ok(ratios) => {
            if let Some(runtime) = runtime {
                runtime.margin_ratio_cache_put(cache_key.clone(), ratios.clone());
            }
            ratios
        }
        Err(error) if is_margin_ratio_rate_limited(&error.to_string()) => runtime
            .and_then(|runtime| {
                runtime.margin_ratio_cache_get(&cache_key, MARGIN_RATIO_CACHE_FALLBACK_TTL)
            })
            .ok_or_else(|| session_error(error))?,
        Err(error) => return Err(session_error(error)),
    };
    Ok(project_margin_ratios(&resolved, ratios))
}

fn project_margin_ratios(
    resolved: &ResolvedTradeRequest,
    ratios: Vec<TradeMarginRatioSnapshot>,
) -> Value {
    let ratios = ratios
        .into_iter()
        .map(|ratio| margin_ratio_value(resolved, ratio))
        .collect::<Vec<_>>();
    json!({
        "checkedAt": checked_at(),
        "connectivity": "connected",
        "marginRatios": ratios
    })
}

fn margin_ratio_cache_key(
    resolved: &ResolvedTradeRequest,
    securities: &[TradeSecurity],
) -> String {
    let mut symbols = securities
        .iter()
        .map(|security| {
            let market = qot_market_label(security.market).unwrap_or("UNKNOWN");
            format!("{market}.{}", security.code.trim().to_ascii_uppercase())
        })
        .collect::<Vec<_>>();
    symbols.sort_unstable();
    format!(
        "{}|REAL|{}|{}",
        resolved.account_id.to_ascii_uppercase(),
        resolved.market.to_ascii_uppercase(),
        symbols.join(",")
    )
}

fn is_margin_ratio_rate_limited(message: &str) -> bool {
    let lower = message.to_ascii_lowercase();
    lower.contains("频率太高")
        || lower.contains("每30秒最多10次")
        || (lower.contains("too high") && lower.contains("request"))
        || lower.contains("rate limit")
        || lower.contains("rate-limit")
}
