//! Production prediction-market read projection.

use std::sync::Arc;

use jftrade_settings::MarketDataProvider;
use percent_encoding::percent_decode_str;
use serde_json::Value;

use crate::product::product_active_provider_state::ActiveProviderState;
use crate::product::{MarketDataPredictionReadSnapshotError, MarketDataPredictionReadSnapshotPort};

#[derive(Debug)]
pub(crate) struct ProductionMarketDataPredictionPort {
    pub(crate) active_provider_state: Arc<ActiveProviderState>,
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
        Err(MarketDataPredictionReadSnapshotError::Unavailable(
            "Futu prediction market-data adapter is not configured".to_owned(),
        ))
    }
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
mod tests {
    use super::*;

    fn port(
        provider: Option<MarketDataProvider>,
        opend_ready: bool,
    ) -> ProductionMarketDataPredictionPort {
        let state = Arc::new(ActiveProviderState::new(provider));
        state.set_readiness(false, opend_ready, false);
        ProductionMarketDataPredictionPort {
            active_provider_state: state,
        }
    }

    #[test]
    fn prediction_read_query_accepts_go_route_defaults() {
        validate_prediction_read_request(
            "/api/v1/market-data/prediction/events",
            "brokerId=futu&accountId=acct-1&tradingEnvironment=SIMULATE&category=politics&pageSize=100&refresh=true",
        ).expect("valid prediction query");
    }

    #[test]
    fn prediction_read_query_rejects_invalid_schema_before_provider_check() {
        let invalid = [
            (
                "/api/v1/market-data/prediction/events",
                "pageSize=0",
                "pageSize must be between 1 and 300",
            ),
            (
                "/api/v1/market-data/prediction/events",
                "refresh=maybe",
                "refresh must be true or false",
            ),
            (
                "/api/v1/market-data/prediction/events",
                "category=%FF",
                "invalid prediction query encoding",
            ),
            (
                "/api/v1/market-data/prediction/contracts/US.EC-42/snapshot",
                "operation=events",
                "operation must be snapshot",
            ),
        ];
        for (path, query, message) in invalid {
            assert!(matches!(
                validate_prediction_read_request(path, query),
                Err(MarketDataPredictionReadSnapshotError::Invalid(actual)) if actual == message
            ));
        }
    }

    #[test]
    fn prediction_read_query_rejects_invalid_path_segments() {
        for path in [
            "/api/v1/market-data/prediction/contracts//snapshot",
            "/api/v1/market-data/prediction/contracts/US.EC%2F42/snapshot",
            "/api/v1/market-data/prediction/events/EVENT%2042/contracts",
        ] {
            assert!(matches!(
                validate_prediction_read_request(path, ""),
                Err(MarketDataPredictionReadSnapshotError::Invalid(_))
            ));
        }
    }

    #[test]
    fn prediction_read_port_fails_closed_for_missing_or_unready_provider() {
        assert!(matches!(
            port(None, false).read("/api/v1/market-data/prediction/categories", ""),
            Err(MarketDataPredictionReadSnapshotError::Unavailable(message))
                if message == "prediction market-data provider is not configured"
        ));
        assert!(matches!(
            port(Some(MarketDataProvider::Futu), false)
                .read("/api/v1/market-data/prediction/categories", ""),
            Err(MarketDataPredictionReadSnapshotError::Unavailable(message))
                if message == "Futu prediction market-data provider is not ready"
        ));
    }
}
