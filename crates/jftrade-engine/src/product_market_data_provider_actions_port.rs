// Production boundary for provider-backed market-data actions.

use serde_json::Value;
use thiserror::Error;

pub const NORMALIZE_INSTRUMENT_PATH: &str = "/api/v1/market-data/instruments/normalize";
pub const OPTION_ANALYSIS_PATH: &str = "/api/v1/market-data/options/analysis/{instrumentId}";
pub const ZERO_DTE_CONTRACTS_PATH: &str = "/api/v1/market-data/options/events/zero-dte-contracts";
pub const PREDICTION_COMBO_QUOTES_PATH: &str = "/api/v1/market-data/prediction/combos/quotes";
pub const BATCH_SNAPSHOTS_PATH: &str = "/api/v1/market-data/snapshots";

pub const MARKET_DATA_PROVIDER_ACTIONS_ROUTES: [(&str, &str); 5] = [
    ("POST", NORMALIZE_INSTRUMENT_PATH),
    ("POST", OPTION_ANALYSIS_PATH),
    ("POST", ZERO_DTE_CONTRACTS_PATH),
    ("POST", PREDICTION_COMBO_QUOTES_PATH),
    ("POST", BATCH_SNAPSHOTS_PATH),
];

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct MarketDataProviderActionsRequest {
    pub method: String,
    pub path: String,
    pub query: String,
    pub body: Vec<u8>,
}

#[allow(dead_code)]
#[derive(Clone, Debug, Error, Eq, PartialEq)]
pub enum MarketDataProviderActionsPortError {
    #[error("market-data provider actions snapshot is unavailable: {0}")]
    Unavailable(String),
    #[error("market-data provider actions snapshot failed: {code}: {message}")]
    Failed {
        status: u16,
        code: String,
        message: String,
        retry_after_seconds: Option<u64>,
    },
}

use std::future::Future;
use std::pin::Pin;

pub type MarketDataProviderActionsFuture<'a> =
    Pin<Box<dyn Future<Output = Result<Value, MarketDataProviderActionsPortError>> + Send + 'a>>;

/// Go-owned adapter boundary for the five provider-backed POST operations.
///
/// Implementations used by tests may use fixture data or a mock broker. A
/// production implementation is intentionally not supplied by this slice.
pub trait MarketDataProviderActionsPort: Send + Sync + std::fmt::Debug {
    fn dispatch<'a>(
        &'a self,
        request: &'a MarketDataProviderActionsRequest,
    ) -> MarketDataProviderActionsFuture<'a>;
}

#[allow(dead_code)]
pub fn market_data_provider_actions_routes() -> &'static [(&'static str, &'static str); 5] {
    &MARKET_DATA_PROVIDER_ACTIONS_ROUTES
}

pub fn is_market_data_provider_action_path(path: &str) -> bool {
    matches!(
        path,
        NORMALIZE_INSTRUMENT_PATH
            | ZERO_DTE_CONTRACTS_PATH
            | PREDICTION_COMBO_QUOTES_PATH
            | BATCH_SNAPSHOTS_PATH
    ) || has_instrument_suffix(path, "/api/v1/market-data/options/analysis/")
}

fn has_instrument_suffix(path: &str, prefix: &str) -> bool {
    path.strip_prefix(prefix)
        .is_some_and(|suffix| !suffix.is_empty() && !suffix.contains('/'))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn provider_actions_route_contract_has_five_unique_post_operations() {
        assert_eq!(market_data_provider_actions_routes().len(), 5);
        assert_eq!(
            MARKET_DATA_PROVIDER_ACTIONS_ROUTES
                .iter()
                .filter(|(method, _)| *method == "POST")
                .count(),
            5
        );
        assert!(is_market_data_provider_action_path(
            "/api/v1/market-data/options/analysis/US.AAPL"
        ));
        assert!(!is_market_data_provider_action_path(
            "/api/v1/market-data/options/analysis/US.AAPL/extra"
        ));
    }
}
