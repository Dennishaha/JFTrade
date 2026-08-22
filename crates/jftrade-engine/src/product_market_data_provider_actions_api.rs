use std::sync::Arc;

use jftrade_api::{ApiFailure, ApiOutput, ApiRequest};
use serde_json::Value;

use super::product_market_data_provider_actions_port::{
    BATCH_SNAPSHOTS_PATH, MarketDataProviderActionsPort, MarketDataProviderActionsPortError,
    MarketDataProviderActionsRequest, NORMALIZE_INSTRUMENT_PATH, PREDICTION_COMBO_QUOTES_PATH,
    ZERO_DTE_CONTRACTS_PATH, is_market_data_provider_action_path,
};

pub const MARKET_DATA_PROVIDER_ACTIONS_UNAVAILABLE_CODE: &str =
    "MARKET_DATA_PROVIDER_ACTIONS_UNAVAILABLE";

pub struct MarketDataProviderActionsApi {
    port: Option<Arc<dyn MarketDataProviderActionsPort>>,
}

impl MarketDataProviderActionsApi {
    pub fn new(port: Option<Arc<dyn MarketDataProviderActionsPort>>) -> Self {
        Self { port }
    }

    pub fn dispatch(&self, request: &ApiRequest) -> Result<ApiOutput, ApiFailure> {
        dispatch_market_data_provider_actions(
            self.port.as_deref(),
            &request.method,
            &request.path,
            &request.query,
            &request.body,
        )
    }
}

pub fn dispatch_market_data_provider_actions(
    port: Option<&dyn MarketDataProviderActionsPort>,
    method: &str,
    path: &str,
    query: &str,
    body: &[u8],
) -> Result<ApiOutput, ApiFailure> {
    if method != "POST" || !is_market_data_provider_action_path(path) {
        return Err(ApiFailure::new(
            404,
            "NOT_FOUND",
            format!("unknown endpoint {path}"),
        ));
    }
    validate_json_body(path, body)?;
    let Some(port) = port else {
        return Err(ApiFailure::new(
            503,
            MARKET_DATA_PROVIDER_ACTIONS_UNAVAILABLE_CODE,
            "market-data provider actions port is not configured",
        ));
    };
    let request = MarketDataProviderActionsRequest {
        method: method.to_owned(),
        path: path.to_owned(),
        query: query.to_owned(),
        body: body.to_vec(),
    };
    port.dispatch(&request)
        .map(ApiOutput::Json)
        .map_err(market_data_provider_actions_failure)
}

fn validate_json_body(path: &str, body: &[u8]) -> Result<(), ApiFailure> {
    if serde_json::from_slice::<Value>(body).is_ok() {
        return Ok(());
    }
    let (code, message) = if path == NORMALIZE_INSTRUMENT_PATH {
        ("BAD_REQUEST", "invalid normalize request")
    } else if has_instrument_suffix(path, "/api/v1/market-data/options/analysis/") {
        ("BAD_REQUEST", "invalid request body")
    } else if path == ZERO_DTE_CONTRACTS_PATH {
        (
            "OPTION_CHAIN_CONTEXT_REQUIRED",
            "invalid 0DTE chain context",
        )
    } else if path == PREDICTION_COMBO_QUOTES_PATH {
        ("BAD_REQUEST", "invalid prediction combo quote payload")
    } else if path == BATCH_SNAPSHOTS_PATH {
        ("BAD_REQUEST", "invalid request body")
    } else {
        ("BAD_REQUEST", "invalid request body")
    };
    Err(ApiFailure::new(400, code, message))
}

fn has_instrument_suffix(path: &str, prefix: &str) -> bool {
    path.strip_prefix(prefix)
        .is_some_and(|suffix| !suffix.is_empty() && !suffix.contains('/'))
}

fn market_data_provider_actions_failure(error: MarketDataProviderActionsPortError) -> ApiFailure {
    match error {
        MarketDataProviderActionsPortError::Unavailable(message) => {
            ApiFailure::new(503, MARKET_DATA_PROVIDER_ACTIONS_UNAVAILABLE_CODE, message)
        }
        MarketDataProviderActionsPortError::Failed {
            status,
            code,
            message,
            retry_after_seconds,
        } => {
            let failure = ApiFailure::new(status, code, message);
            match retry_after_seconds {
                Some(seconds) => failure.with_retry_after(seconds),
                None => failure,
            }
        }
    }
}
