//! Production prediction-market read projection.

use std::sync::Arc;

use jftrade_settings::MarketDataProvider;
use percent_encoding::percent_decode_str;
use serde_json::Value;

use crate::product::product_active_provider_state::ActiveProviderState;
use crate::product::product_production_ports::SharedTradeReadRuntime;
use crate::product::product_query::QueryMap;
use crate::product::{MarketDataPredictionReadSnapshotError, MarketDataPredictionReadSnapshotPort};

/// Go `productfeatures.ErrPredictionIneligible` message. The HTTP edge maps this
/// sentinel to `403 PREDICTION_MARKET_INELIGIBLE`, so the text and the code are one
/// contract shared by the read route, the extension executor and the mutation port.
pub(crate) const PREDICTION_INELIGIBLE_MESSAGE: &str =
    "prediction market requires an eligible Moomoo US account";
pub(crate) const PREDICTION_INELIGIBLE_CODE: &str = "PREDICTION_MARKET_INELIGIBLE";

#[derive(Debug)]
pub(crate) struct ProductionMarketDataPredictionPort {
    pub(crate) active_provider_state: Arc<ActiveProviderState>,
    pub(crate) trade_runtime: Option<Arc<SharedTradeReadRuntime>>,
}

impl MarketDataPredictionReadSnapshotPort for ProductionMarketDataPredictionPort {
    fn read(
        &self,
        path: &str,
        query: &str,
    ) -> Result<Value, MarketDataPredictionReadSnapshotError> {
        validate_prediction_read_request(path, query)?;
        let snapshot = self.active_provider_state.snapshot();
        if snapshot.provider.is_none() {
            return Err(MarketDataPredictionReadSnapshotError::Unavailable(
                "prediction market-data provider is not configured".to_owned(),
            ));
        }
        if snapshot.provider != Some(MarketDataProvider::Futu) || !snapshot.opend_ready {
            return Err(MarketDataPredictionReadSnapshotError::Unavailable(
                "Futu prediction market-data provider is not ready".to_owned(),
            ));
        }
        let runtime = self.trade_runtime.as_ref().ok_or_else(|| {
            MarketDataPredictionReadSnapshotError::Unavailable(
                "Futu prediction market-data runtime is not configured".to_owned(),
            )
        })?;
        // Go `ProductFeatureService.Query` resolves the broker capability and then
        // runs `predictionEligibility` for every `prediction.*` read before the
        // reader runs, so an HK-authority or non-FUTUINC account can never receive
        // prediction data. Keeping the check here gives the HTTP route, the
        // extension executor and the subscription mutation port one owner.
        if let Err(detail) = prediction_account_eligibility(runtime, query) {
            return Err(MarketDataPredictionReadSnapshotError::Failed {
                status: 403,
                code: PREDICTION_INELIGIBLE_CODE.to_owned(),
                message: format!("{PREDICTION_INELIGIBLE_MESSAGE}: {detail}"),
                retry_after_seconds: None,
            });
        }
        if !runtime.prediction_reader_available() {
            return Err(MarketDataPredictionReadSnapshotError::Unavailable(
                "Futu prediction market-data reader is not ready".to_owned(),
            ));
        }
        runtime
            .prediction_read(path, query)
            .map_err(|message| {
                if message.contains("invalid prediction request")
                    || message.contains("required")
                    || message.contains("invalid")
                    || message.contains("must be")
                {
                    MarketDataPredictionReadSnapshotError::Invalid(message)
                } else if message.contains("rejected") || message.contains("decode") {
                    MarketDataPredictionReadSnapshotError::Failed {
                        status: 502,
                        code: "BROKER_FEATURE_FAILED".to_owned(),
                        message,
                        retry_after_seconds: None,
                    }
                } else {
                    MarketDataPredictionReadSnapshotError::Unavailable(message)
                }
            })
    }
}

/// Resolve the prediction account verdict the same way Go's
/// `productfeatures.predictionEligibility` does.
///
/// The account id is optional: when it is absent any discovered account may
/// satisfy the check, which is what a discovery query without a selected
/// account relies on. Only `FUTUINC` accounts with US authority (or no
/// authority list at all) qualify; everything else — including a discovery
/// failure — is ineligible, because prediction data is gated on the Moomoo US
/// entitlement and not on the market-data provider alone.
pub(crate) fn prediction_account_eligibility(
    runtime: &SharedTradeReadRuntime,
    query: &str,
) -> Result<String, String> {
    let query_map =
        QueryMap::parse(query).map_err(|_| "invalid prediction query encoding".to_owned())?;
    let requested = query_map
        .get_first("accountId")
        .map(str::trim)
        .filter(|value| !value.is_empty());
    let client = runtime
        .prediction_account_source()
        .ok_or_else(|| {
            "account eligibility could not be verified: Futu trade read client is unavailable"
                .to_owned()
        })?;
    let accounts = client
        .read_accounts(0, None, None)
        .map_err(|error| format!("account eligibility could not be verified: {error}"))?;
    for account in accounts {
        let identity = super::super::product_production_ports_trade::account_identity(&account);
        if let Some(requested) = requested
            && identity.as_deref() != Some(requested)
        {
            continue;
        }
        let firm = account
            .security_firm
            .and_then(super::super::product_production_ports_trade::trade_projection::security_firm_label)
            .unwrap_or_default();
        if firm != "FUTUINC" {
            continue;
        }
        if !account.trd_market_auth_list.is_empty()
            && !account.trd_market_auth_list.iter().any(|market| {
                super::super::product_production_ports_trade::trade_projection::trade_market_authority(
                    *market,
                )
                .is_some_and(|label| label.eq_ignore_ascii_case("US"))
            })
        {
            continue;
        }
        return Ok(firm.to_owned());
    }
    Err("no eligible Moomoo US (FUTUINC) account was found".to_owned())
}

const PREDICTION_READ_QUERY_MAX_BYTES: usize = 8 * 1024;
const PREDICTION_READ_VALUE_MAX_BYTES: usize = 512;

pub(crate) fn validate_prediction_read_request(
    path: &str,
    query: &str,
) -> Result<(), MarketDataPredictionReadSnapshotError> {
    let operation = prediction_read_operation(path)?;
    if query.len() > PREDICTION_READ_QUERY_MAX_BYTES {
        return Err(prediction_read_invalid("prediction query is too long"));
    }
    let query_map = crate::product::product_query::QueryMap::parse(query)
        .map_err(|_| prediction_read_invalid("invalid prediction query encoding"))?;

    for key in [
        "brokerId",
        "accountId",
        "tradingEnvironment",
        "market",
        "marketSegment",
        "productClass",
        "category",
        "tag",
        "seriesId",
        "cursor",
        "eventId",
        "code",
        "instrumentId",
        "underlying",
    ] {
        if let Some(value) = query_map.get_first(key) {
            validate_prediction_scalar(key, value)?;
        }
    }

    if let Some(value) = query_map.get_first("pageSize") {
        let page_size = value.trim().parse::<u16>().ok();
        if !page_size.is_some_and(|value| (1..=300).contains(&value)) {
            return Err(prediction_read_invalid(
                "pageSize must be between 1 and 300",
            ));
        }
    }
    if let Some(value) = query_map.get_first("count") {
        let count = value.trim().parse::<u16>().ok();
        if !count.is_some_and(|value| (1..=300).contains(&value)) {
            return Err(prediction_read_invalid("count must be between 1 and 300"));
        }
    }
    if let Some(value) = query_map.get_first("refresh")
        && !matches!(
            value.trim().to_ascii_lowercase().as_str(),
            "true" | "false" | "1" | "0"
        )
    {
        return Err(prediction_read_invalid("refresh must be true or false"));
    }
    if let Some(value) = query_map.get_first("operation") {
        let requested = value.trim().to_ascii_lowercase();
        if !requested.is_empty() && requested != operation {
            return Err(prediction_read_invalid(&format!(
                "operation must be {operation}"
            )));
        }
    }
    if let Some(value) = query_map.get_first("market")
        && !value.trim().is_empty()
        && !value.trim().eq_ignore_ascii_case("US")
    {
        return Err(prediction_read_invalid("prediction market must be US"));
    }
    Ok(())
}

fn prediction_read_operation(
    path: &str,
) -> Result<&'static str, MarketDataPredictionReadSnapshotError> {
    if !crate::product::is_market_data_prediction_read_path(path) {
        return Err(prediction_read_invalid("unsupported prediction read route"));
    }
    match path {
        "/api/v1/market-data/prediction/categories" => Ok("categories"),
        "/api/v1/market-data/prediction/competitions" => Ok("competitions"),
        "/api/v1/market-data/prediction/series" => Ok("series"),
        "/api/v1/market-data/prediction/events" => Ok("events"),
        "/api/v1/market-data/prediction/combos/eligible-events" => Ok("eligible_events"),
        _ if path.starts_with("/api/v1/market-data/prediction/events/") => {
            let event_id = path
                .strip_prefix("/api/v1/market-data/prediction/events/")
                .and_then(|value| value.strip_suffix("/contracts"))
                .ok_or_else(|| prediction_read_invalid("eventId is invalid"))?;
            validate_prediction_path_segment(event_id, "eventId")?;
            Ok("contracts")
        }
        _ if path.starts_with("/api/v1/market-data/prediction/contracts/") => {
            let suffix = path
                .strip_prefix("/api/v1/market-data/prediction/contracts/")
                .ok_or_else(|| prediction_read_invalid("contract code is invalid"))?;
            let (code, operation) = suffix
                .split_once('/')
                .ok_or_else(|| prediction_read_invalid("contract code is invalid"))?;
            validate_prediction_path_segment(code, "code")?;
            match operation {
                "snapshot" => Ok("snapshot"),
                "order-book" => Ok("order_book"),
                "candles" => Ok("candles"),
                "candles/history" => Ok("historical"),
                "ticks" => Ok("ticks"),
                "milestones" => Ok("milestones"),
                _ => Err(prediction_read_invalid("unsupported prediction read route")),
            }
        }
        _ => Err(prediction_read_invalid("unsupported prediction read route")),
    }
}

fn validate_prediction_path_segment(
    encoded: &str,
    label: &str,
) -> Result<(), MarketDataPredictionReadSnapshotError> {
    if encoded.is_empty() || crate::product::product_query::has_invalid_percent_escape(encoded) {
        return Err(prediction_read_invalid(&format!("{label} is invalid")));
    }
    let decoded = percent_decode_str(encoded)
        .decode_utf8()
        .map_err(|_| prediction_read_invalid(&format!("{label} is invalid")))?;
    if decoded.is_empty()
        || decoded.len() > PREDICTION_READ_VALUE_MAX_BYTES
        || decoded.chars().any(|value| {
            value.is_control() || value.is_whitespace() || matches!(value, '/' | '\\' | '?' | '#')
        })
    {
        return Err(prediction_read_invalid(&format!("{label} is invalid")));
    }
    Ok(())
}

fn validate_prediction_scalar(
    key: &str,
    value: &str,
) -> Result<(), MarketDataPredictionReadSnapshotError> {
    if value.len() > PREDICTION_READ_VALUE_MAX_BYTES || value.chars().any(char::is_control) {
        return Err(prediction_read_invalid(&format!("{key} is invalid")));
    }
    Ok(())
}

fn prediction_read_invalid(message: &str) -> MarketDataPredictionReadSnapshotError {
    MarketDataPredictionReadSnapshotError::Invalid(message.to_owned())
}

#[cfg(test)]
#[path = "product_production_ports_market_data_prediction_tests.rs"]
mod tests;
