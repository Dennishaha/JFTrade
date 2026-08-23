use std::sync::Arc;

use serde_json::{Value, json};
use tempfile::tempdir;

use super::super::product_market_data_subscription_mutation_port::{
    MarketDataSubscriptionMutationPort, MarketDataSubscriptionMutationPortError,
    MarketDataSubscriptionMutationRequest,
};
use super::*;

#[derive(Debug)]
struct FixtureMarketDataSubscriptionMutationPort;

impl MarketDataSubscriptionMutationPort for FixtureMarketDataSubscriptionMutationPort {
    fn dispatch(
        &self,
        request: &MarketDataSubscriptionMutationRequest,
    ) -> Result<Value, MarketDataSubscriptionMutationPortError> {
        Ok(json!({
            "accepted": true,
            "method": request.method,
            "path": request.path,
        }))
    }
}

#[tokio::test]
async fn market_data_subscription_mutations_register_only_with_explicit_test_port() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let base = ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
        .expect("config");
    let handle = start_product(base).await.expect("start base product");
    assert_eq!(handle.startup_record().owned_routes, 48);
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/market-data/subscriptions",
        Some(r#"{"consumerId":"chart","instruments":[]}"#),
        &[],
    )
    .await;
    assert_eq!(status, 404);
    assert_eq!(response["error"]["code"], "NOT_FOUND");
    handle.shutdown().await.expect("shutdown base product");

    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_market_data_subscription_mutation_port(Arc::new(
                FixtureMarketDataSubscriptionMutationPort,
            ));
    let handle = start_product(config)
        .await
        .expect("start market-data subscription product");
    assert_eq!(handle.startup_record().owned_routes, 54);
    assert!(
        handle
            .startup_record()
            .capabilities
            .iter()
            .any(|route| { route == "POST /api/v1/market-data/subscriptions" })
    );
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/market-data/subscriptions",
        Some(r#"{"consumerId":"chart","instruments":[]}"#),
        &[],
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["ok"], true);
    assert_eq!(response["data"]["accepted"], true);
    assert_eq!(
        response["data"]["path"],
        "/api/v1/market-data/subscriptions"
    );
    handle
        .shutdown()
        .await
        .expect("shutdown market-data subscription product");
}
