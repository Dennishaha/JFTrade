use std::collections::VecDeque;
use std::sync::{Arc, Mutex};

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

#[derive(Debug)]
struct SequencedResearchScreenWritePort {
    responses: Mutex<VecDeque<Result<Value, ResearchScreenWritePortError>>>,
}

impl SequencedResearchScreenWritePort {
    fn new(
        responses: impl IntoIterator<Item = Result<Value, ResearchScreenWritePortError>>,
    ) -> Self {
        Self {
            responses: Mutex::new(responses.into_iter().collect()),
        }
    }
}

impl ResearchScreenWritePort for SequencedResearchScreenWritePort {
    fn query(
        &self,
        _request: &ResearchScreenWriteQuery,
    ) -> Result<Value, ResearchScreenWritePortError> {
        self.responses
            .lock()
            .expect("research screen response lock")
            .pop_front()
            .unwrap_or_else(|| Ok(json!({"entries": [], "hasMore": false})))
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

#[tokio::test]
async fn research_screen_product_recovers_after_port_failure_and_restart_without_local_state_side_effects()
 {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, b"{\"seed\":\"research-screen\"}\n").expect("seed settings");
    let settings_before = std::fs::read(&settings_path).expect("read seeded settings");
    let body = r#"{"brokerId":"api-test","market":"US","catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}"#;
    let port = Arc::new(SequencedResearchScreenWritePort::new([
        Err(ResearchScreenWritePortError::Failed(
            "provider failed before recovery".to_owned(),
        )),
        Ok(json!({"entries": [], "hasMore": false})),
    ]));
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_research_screen_write_port(port);
    let handle = start_product(config).await.expect("start product");
    let address = handle.startup_record().address;

    let (status, response) =
        request_json_with_status(address, "POST", "/api/v1/research/screens", Some(body), &[])
            .await;
    assert_eq!(status, 502);
    assert_eq!(response["error"]["code"], "BROKER_FEATURE_FAILED");
    assert_eq!(
        response["error"]["message"],
        "provider failed before recovery"
    );

    let (status, response) =
        request_json_with_status(address, "POST", "/api/v1/research/screens", Some(body), &[])
            .await;
    assert_eq!(status, 200);
    assert_eq!(response["data"]["entries"], json!([]));
    assert_eq!(response["data"]["hasMore"], false);
    handle.shutdown().await.expect("shutdown product");

    let restarted =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("restarted config")
            .with_research_screen_write_port(Arc::new(FixtureResearchScreenWritePort));
    let handle = start_product(restarted).await.expect("restart product");
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/research/screens",
        Some(body),
        &[],
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["data"]["entries"], json!([]));
    handle.shutdown().await.expect("shutdown restarted product");

    let settings_after = std::fs::read(&settings_path).expect("read settings after restart");
    assert_eq!(settings_after, settings_before);
}

#[tokio::test]
async fn research_screen_product_maps_cancel_and_deadline_failures_without_fallback() {
    for message in ["context canceled", "context deadline exceeded"] {
        let directory = tempdir().expect("temporary directory");
        let settings_path = directory.path().join("settings.json");
        let config =
            ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
                .expect("config")
                .with_research_screen_write_port(Arc::new(SequencedResearchScreenWritePort::new(
                    [Err(ResearchScreenWritePortError::Failed(
                        message.to_owned(),
                    ))],
                )));
        let handle = start_product(config).await.expect("start product");
        let (status, response) = request_json_with_status(
            handle.startup_record().address,
            "POST",
            "/api/v1/research/screens",
            Some(
                r#"{"brokerId":"api-test","market":"US","catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}"#,
            ),
            &[],
        )
        .await;
        assert_eq!(status, 502);
        assert_eq!(response["error"]["code"], "BROKER_FEATURE_FAILED");
        assert_eq!(response["error"]["message"], message);
        handle.shutdown().await.expect("shutdown product");
    }
}
