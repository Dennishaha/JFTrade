use std::sync::Arc;

use jftrade_api::{ApiFailure, ApiOutput, ApiRequest};
use serde_json::Value;

use super::product_market_data_subscription_mutation_port::{
    MARKET_DATA_PREDICTION_SUBSCRIPTION_ACQUIRE_PATH, MARKET_DATA_SUBSCRIPTION_ACQUIRE_PATH,
    MARKET_DATA_SUBSCRIPTION_HEARTBEAT_PATH, MARKET_DATA_SUBSCRIPTION_RELEASE_PATH,
    MarketDataSubscriptionMutationPort, MarketDataSubscriptionMutationPortError,
    MarketDataSubscriptionMutationRequest, is_market_data_subscription_mutation_path,
};

pub const MARKET_DATA_SUBSCRIPTION_MUTATION_UNAVAILABLE_CODE: &str =
    "MARKET_DATA_SUBSCRIPTION_MUTATION_UNAVAILABLE";

pub struct MarketDataSubscriptionMutationApi {
    port: Option<Arc<dyn MarketDataSubscriptionMutationPort>>,
}

impl MarketDataSubscriptionMutationApi {
    pub fn new(port: Option<Arc<dyn MarketDataSubscriptionMutationPort>>) -> Self {
        Self { port }
    }

    pub fn dispatch(&self, request: &ApiRequest) -> Result<ApiOutput, ApiFailure> {
        dispatch_market_data_subscription_mutation(
            self.port.as_deref(),
            &request.method,
            &request.path,
            &request.query,
            &request.body,
        )
    }
}

pub fn dispatch_market_data_subscription_mutation(
    port: Option<&dyn MarketDataSubscriptionMutationPort>,
    method: &str,
    path: &str,
    query: &str,
    body: &[u8],
) -> Result<ApiOutput, ApiFailure> {
    if !is_market_data_subscription_mutation_path(method, path) {
        return Err(ApiFailure::new(
            404,
            "NOT_FOUND",
            format!("unknown endpoint {path}"),
        ));
    }
    validate_json_body(method, path, body)?;
    let Some(port) = port else {
        return Err(ApiFailure::new(
            503,
            MARKET_DATA_SUBSCRIPTION_MUTATION_UNAVAILABLE_CODE,
            "market-data subscription mutation port is not configured",
        ));
    };
    let request = MarketDataSubscriptionMutationRequest {
        method: method.to_owned(),
        path: path.to_owned(),
        query: query.to_owned(),
        body: body.to_vec(),
    };
    port.dispatch(&request)
        .map(ApiOutput::Json)
        .map_err(market_data_subscription_mutation_failure)
}

fn validate_json_body(method: &str, path: &str, body: &[u8]) -> Result<(), ApiFailure> {
    if method != "POST" || body_starts_with_json_value(body) {
        return Ok(());
    }
    let message = if path == MARKET_DATA_SUBSCRIPTION_ACQUIRE_PATH {
        "invalid subscription request"
    } else if path == MARKET_DATA_SUBSCRIPTION_RELEASE_PATH {
        "invalid release request"
    } else if path == MARKET_DATA_SUBSCRIPTION_HEARTBEAT_PATH {
        "invalid heartbeat request"
    } else if path == MARKET_DATA_PREDICTION_SUBSCRIPTION_ACQUIRE_PATH
        || is_prediction_subscription_path(path)
    {
        "invalid prediction subscription payload"
    } else {
        "invalid request body"
    };
    Err(ApiFailure::new(400, "BAD_REQUEST", message))
}

fn body_starts_with_json_value(body: &[u8]) -> bool {
    serde_json::Deserializer::from_slice(body)
        .into_iter::<Value>()
        .next()
        .is_some_and(|result| result.is_ok())
}

fn is_prediction_subscription_path(path: &str) -> bool {
    path.strip_prefix("/api/v1/market-data/prediction/contracts/")
        .is_some_and(|suffix| suffix.split('/').nth(1) == Some("subscriptions"))
}

fn market_data_subscription_mutation_failure(
    error: MarketDataSubscriptionMutationPortError,
) -> ApiFailure {
    match error {
        MarketDataSubscriptionMutationPortError::Unavailable(message) => ApiFailure::new(
            503,
            MARKET_DATA_SUBSCRIPTION_MUTATION_UNAVAILABLE_CODE,
            message,
        ),
        MarketDataSubscriptionMutationPortError::Failed {
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

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[derive(Debug)]
    struct ErrorPort(MarketDataSubscriptionMutationPortError);

    impl MarketDataSubscriptionMutationPort for ErrorPort {
        fn dispatch(
            &self,
            _request: &MarketDataSubscriptionMutationRequest,
        ) -> Result<Value, MarketDataSubscriptionMutationPortError> {
            Err(self.0.clone())
        }
    }

    fn request() -> ApiRequest {
        ApiRequest {
            method: "POST".to_owned(),
            path: MARKET_DATA_SUBSCRIPTION_ACQUIRE_PATH.to_owned(),
            query: String::new(),
            body: serde_json::to_vec(&json!({
                "consumerId": "chart",
                "instruments": []
            }))
            .expect("request body"),
            request_id: "fixture-request".to_owned(),
            desktop_trusted: false,
            origin_provided: false,
            origin_allowed: false,
            browser_authenticated: false,
        }
    }

    #[test]
    fn subscription_mutation_failure_mapping_preserves_unavailable_and_retry_wire() {
        let unavailable = MarketDataSubscriptionMutationApi::new(Some(Arc::new(ErrorPort(
            MarketDataSubscriptionMutationPortError::Unavailable("warming".to_owned()),
        ))))
        .dispatch(&request())
        .expect_err("unavailable subscription port");
        assert_eq!(unavailable.status, 503);
        assert_eq!(
            unavailable.code,
            MARKET_DATA_SUBSCRIPTION_MUTATION_UNAVAILABLE_CODE
        );

        let failed = MarketDataSubscriptionMutationApi::new(Some(Arc::new(ErrorPort(
            MarketDataSubscriptionMutationPortError::Failed {
                status: 429,
                code: "SUBSCRIPTION_RATE_LIMITED".to_owned(),
                message: "retry later".to_owned(),
                retry_after_seconds: Some(7),
            },
        ))))
        .dispatch(&request())
        .expect_err("rate-limited subscription port");
        assert_eq!(failed.status, 429);
        assert_eq!(failed.code, "SUBSCRIPTION_RATE_LIMITED");
        assert_eq!(failed.retry_after_seconds, Some(7));
    }
}
