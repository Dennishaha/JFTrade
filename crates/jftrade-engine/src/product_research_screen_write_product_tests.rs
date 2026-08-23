use std::sync::Arc;

use serde_json::{Value, json};

use crate::product::product_research_screen_write_port::{
    ResearchScreenWritePortError, ResearchScreenWriteQuery,
};

use super::*;

#[derive(Debug)]
struct FixtureResearchScreenWritePort;

impl ResearchScreenWritePort for FixtureResearchScreenWritePort {
    fn query(
        &self,
        _request: &ResearchScreenWriteQuery,
    ) -> Result<Value, ResearchScreenWritePortError> {
        Ok(json!({"entries": [], "hasMore": false}))
    }
}

#[tokio::test]
async fn research_screen_write_route_is_opt_in_and_preserves_success_wire() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let base = ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
        .expect("config");
    let handle = start_product(base).await.expect("start base product");
    assert_eq!(handle.startup_record().owned_routes, 48);
    let body = r#"{"brokerId":"api-test","market":"US","catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}"#;
    let response = request_json(
        handle.startup_record().address,
        "POST",
        "/api/v1/research/screens",
        Some(body),
    )
    .await;
    assert_eq!(response["error"]["code"], "NOT_FOUND");
    handle.shutdown().await.expect("shutdown base product");

    let configured =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_research_screen_write_port(Arc::new(FixtureResearchScreenWritePort));
    let handle = start_product(configured)
        .await
        .expect("start research screen product");
    assert_eq!(handle.startup_record().owned_routes, 49);
    let response = request_json(
        handle.startup_record().address,
        "POST",
        "/api/v1/research/screens",
        Some(body),
    )
    .await;
    assert_eq!(response["ok"], true);
    assert_eq!(response["data"]["catalogVersion"], "futu-stock-screen-v1");
    assert_eq!(response["data"]["entries"], json!([]));
    assert_eq!(response["data"]["hasMore"], false);
    handle
        .shutdown()
        .await
        .expect("shutdown research screen product");
}
