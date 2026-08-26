// Stage 9 test-cutover boundary for market-data subscription mutations.
//
// Go remains the only owner of subscription demand, prediction eligibility,
// Provider/OpenD lifecycle, lease state, persistence, and cleanup.  This
// port deliberately carries raw HTTP inputs to an explicitly injected test
// adapter so the Rust leaf cannot create a second subscription owner.

use serde_json::Value;
use thiserror::Error;

pub const MARKET_DATA_SUBSCRIPTION_ACQUIRE_PATH: &str = "/api/v1/market-data/subscriptions";
pub const MARKET_DATA_SUBSCRIPTION_CLEAR_PATH: &str = "/api/v1/market-data/subscriptions";
pub const MARKET_DATA_SUBSCRIPTION_RELEASE_PATH: &str = "/api/v1/market-data/subscriptions/release";
pub const MARKET_DATA_SUBSCRIPTION_HEARTBEAT_PATH: &str =
    "/api/v1/market-data/subscriptions/heartbeat";
pub const MARKET_DATA_PREDICTION_SUBSCRIPTION_ACQUIRE_PATH: &str =
    "/api/v1/market-data/prediction/contracts/{code}/subscriptions";
pub const MARKET_DATA_PREDICTION_SUBSCRIPTION_RELEASE_PATH: &str =
    "/api/v1/market-data/prediction/contracts/{code}/subscriptions/{leaseId}";

pub const MARKET_DATA_SUBSCRIPTION_MUTATION_ROUTES: [(&str, &str); 6] = [
    ("DELETE", MARKET_DATA_PREDICTION_SUBSCRIPTION_RELEASE_PATH),
    ("DELETE", MARKET_DATA_SUBSCRIPTION_CLEAR_PATH),
    ("POST", MARKET_DATA_PREDICTION_SUBSCRIPTION_ACQUIRE_PATH),
    ("POST", MARKET_DATA_SUBSCRIPTION_ACQUIRE_PATH),
    ("POST", MARKET_DATA_SUBSCRIPTION_HEARTBEAT_PATH),
    ("POST", MARKET_DATA_SUBSCRIPTION_RELEASE_PATH),
];

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct MarketDataSubscriptionMutationRequest {
    pub method: String,
    pub path: String,
    pub query: String,
    pub body: Vec<u8>,
}

#[allow(dead_code)]
#[derive(Clone, Debug, Error, Eq, PartialEq)]
pub enum MarketDataSubscriptionMutationPortError {
    #[error("market-data subscription mutation port is unavailable: {0}")]
    Unavailable(String),
    #[error("market-data subscription mutation failed: {code}: {message}")]
    Failed {
        status: u16,
        code: String,
        message: String,
        retry_after_seconds: Option<u64>,
    },
}

/// Go-owned adapter boundary for all six subscription mutation operations.
///
/// Test implementations may replay frozen Go responses or use an isolated
/// mock.  No production implementation is supplied by this slice.
pub trait MarketDataSubscriptionMutationPort: Send + Sync + std::fmt::Debug {
    fn dispatch(
        &self,
        request: &MarketDataSubscriptionMutationRequest,
    ) -> Result<Value, MarketDataSubscriptionMutationPortError>;
}

pub fn market_data_subscription_mutation_routes() -> &'static [(&'static str, &'static str); 6] {
    &MARKET_DATA_SUBSCRIPTION_MUTATION_ROUTES
}

pub fn is_market_data_subscription_mutation_path(method: &str, path: &str) -> bool {
    match method {
        "DELETE" => {
            path == MARKET_DATA_SUBSCRIPTION_CLEAR_PATH || has_prediction_lease_suffix(path)
        }
        "POST" => {
            path == MARKET_DATA_SUBSCRIPTION_ACQUIRE_PATH
                || path == MARKET_DATA_SUBSCRIPTION_RELEASE_PATH
                || path == MARKET_DATA_SUBSCRIPTION_HEARTBEAT_PATH
                || has_prediction_subscription_suffix(path)
        }
        _ => false,
    }
}

fn has_prediction_subscription_suffix(path: &str) -> bool {
    has_prediction_suffix(path, "/subscriptions")
}

fn has_prediction_lease_suffix(path: &str) -> bool {
    has_prediction_suffix(path, "/subscriptions/")
}

fn has_prediction_suffix(path: &str, suffix: &str) -> bool {
    let Some(prefix) = path.strip_prefix("/api/v1/market-data/prediction/contracts/") else {
        return false;
    };
    let mut segments = prefix.split('/');
    let Some(code) = segments.next() else {
        return false;
    };
    if code.is_empty() {
        return false;
    }
    if suffix == "/subscriptions" {
        return segments.next() == Some("subscriptions") && segments.next().is_none();
    }
    segments.next() == Some("subscriptions")
        && segments.next().is_some_and(|lease_id| !lease_id.is_empty())
        && segments.next().is_none()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn subscription_mutation_route_contract_has_six_unique_operations() {
        assert_eq!(market_data_subscription_mutation_routes().len(), 6);
        assert_eq!(
            MARKET_DATA_SUBSCRIPTION_MUTATION_ROUTES
                .iter()
                .filter(|(method, _)| *method == "POST")
                .count(),
            4
        );
        assert!(is_market_data_subscription_mutation_path(
            "POST",
            "/api/v1/market-data/prediction/contracts/US.EC-42/subscriptions"
        ));
        assert!(is_market_data_subscription_mutation_path(
            "DELETE",
            "/api/v1/market-data/prediction/contracts/US.EC-42/subscriptions/lease-1"
        ));
        assert!(!is_market_data_subscription_mutation_path(
            "GET",
            "/api/v1/market-data/subscriptions"
        ));
        assert!(!is_market_data_subscription_mutation_path(
            "DELETE",
            "/api/v1/market-data/prediction/contracts/US.EC-42/subscriptions/"
        ));
    }
}

#[cfg(test)]
pub(crate) mod test_cutover {
    include!("product_market_data_subscription_mutation_test_cutover.rs");
}
