//! HTTP parity for the screen parser and catalog. Catalog uses the concrete
//! production composition. Recording-provider requests use the explicit
//! rehearsal listener; production intentionally fences caller route ports.

use super::*;
use crate::product::product_research_screen_write_port::{
    ResearchScreenWritePort, ResearchScreenWritePortError, ResearchScreenWriteQuery,
};
use std::sync::Mutex;

const DESKTOP_HEADERS: &[(&str, &str)] =
    &[("Authorization", "Bearer aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")];

#[derive(Debug, Default)]
struct RecordingScreenPort {
    queries: Mutex<Vec<ResearchScreenWriteQuery>>,
}

impl ResearchScreenWritePort for RecordingScreenPort {
    fn query(
        &self,
        request: &ResearchScreenWriteQuery,
    ) -> Result<Value, ResearchScreenWritePortError> {
        self.queries.lock().expect("queries").push(request.clone());
        Ok(json!({"entries": [], "hasMore": false}))
    }
}

fn production_config(directory: &tempfile::TempDir) -> ProductConfig {
    let path = directory.path().join("settings.json");
    std::fs::write(&path, "{}").expect("settings");
    product_data_management::initialize_production_databases(&path).expect("production databases");
    let mut config = ProductConfig::desktop_production(
        "127.0.0.1:0".parse().expect("address"),
        &path,
        "a".repeat(32),
    )
    .expect("production config");
    config.capabilities = ProductCapabilities::all();
    config
}

async fn recording_product(
    directory: &tempfile::TempDir,
    port: Arc<RecordingScreenPort>,
) -> ProductHandle {
    let mut config = ProductConfig::test_cutover(
        "127.0.0.1:0".parse().expect("address"),
        directory.path().join("settings.json"),
    )
    .expect("rehearsal config")
    .with_research_screen_write_port(port);
    config.access = AccessPolicy::desktop(Some("a".repeat(32)));
    start_product(config)
        .await
        .expect("rehearsal listener with recording provider boundary")
}

// Parity: go:452dea11:internal/api/productfeatures/research_screen_test.go:19 TestResearchScreenCatalogRouteAndValidation
#[tokio::test]
async fn production_http_screen_catalog_keeps_display_semantics_and_rejects_invalid_market() {
    let directory = tempdir().expect("directory");
    let handle = start_product(production_config(&directory))
        .await
        .expect("production product");
    let address = handle.startup_record().address;
    let (status, response) = request_json_with_status(
        address,
        "GET",
        "/api/v1/research/screens/catalog?brokerId=futu&market=US",
        None,
        DESKTOP_HEADERS,
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["data"]["version"], "futu-stock-screen-v1");
    assert!(response["data"]["factors"].is_array());
    let body = response.to_string();
    assert!(body.contains("\"currencyBasis\":\"quote\""));
    assert!(body.contains("\"displayFormat\":\"compact_amount\""));
    assert!(!body.contains("providerId"));
    let (status, response) = request_json_with_status(
        address,
        "GET",
        "/api/v1/research/screens/catalog?market=SG",
        None,
        DESKTOP_HEADERS,
    )
    .await;
    assert_eq!(status, 400, "{response}");
    handle.shutdown().await.expect("shutdown");
}

// Parity: go:452dea11:internal/api/productfeatures/research_screen_test.go:47 TestNormalizeResearchScreenQueryDefaultsAndRejectsNonV2Input
#[tokio::test]
async fn http_screen_query_normalizes_broker_market_and_default_limit() {
    let directory = tempdir().expect("directory");
    let port = Arc::new(RecordingScreenPort::default());
    let handle = recording_product(&directory, Arc::clone(&port)).await;
    let mut body = json!({"brokerId": " FUTU ", "market": "us",
        "catalogVersion": "futu-stock-screen-v1", "querySchemaVersion": 2,
        "columns": [{"columnId": "price", "factor": {"instanceId": "price", "factorKey": "simple.price"}}]});
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/research/screens",
        Some(&body.to_string()),
        DESKTOP_HEADERS,
    )
    .await;
    assert_eq!(status, 200, "{response}");
    let queries = port.queries.lock().expect("queries").clone();
    assert_eq!(queries.len(), 1);
    assert_eq!(queries[0].broker_id, "futu");
    assert_eq!(queries[0].market, "US");
    assert_eq!(queries[0].limit, 50);
    assert_eq!(queries[0].definition["brokerId"], "futu");
    assert_eq!(queries[0].definition["market"], "US");
    body["querySchemaVersion"] = json!(1);
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/research/screens",
        Some(&body.to_string()),
        DESKTOP_HEADERS,
    )
    .await;
    assert_eq!(status, 400, "{response}");
    assert_eq!(port.queries.lock().expect("queries").len(), 1);
    handle.shutdown().await.expect("shutdown");
}

// Parity: go:452dea11:internal/api/productfeatures/research_screen_test.go:80 TestResearchScreenPostUsesTypedDefinitionAndOffset
#[tokio::test]
async fn http_screen_post_passes_execution_definition_and_offset_to_provider() {
    let directory = tempdir().expect("directory");
    let port = Arc::new(RecordingScreenPort::default());
    let handle = recording_product(&directory, Arc::clone(&port)).await;
    let body = json!({"brokerId": "api-test", "market": "US", "catalogVersion": "futu-stock-screen-v1",
        "querySchemaVersion": 2,
        "conditions": [{"id": "price-filter", "factor": {"instanceId": "price-filter", "factorKey": "simple.price"}, "operator": "between", "value": {"min": 10}}],
        "columns": [{"columnId": "price-column", "factor": {"instanceId": "price-column", "factorKey": "simple.price"}}],
        "sorts": [{"sortId": "market-cap-sort", "factor": {"instanceId": "market-cap-sort", "factorKey": "simple.market_cap"}, "direction": "desc"}],
        "page": {"offset": 50, "limit": 25}});
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/research/screens",
        Some(&body.to_string()),
        DESKTOP_HEADERS,
    )
    .await;
    assert_eq!(status, 200, "{response}");
    let queries = port.queries.lock().expect("queries").clone();
    assert_eq!(queries.len(), 1);
    let query = &queries[0];
    assert_eq!(query.offset, 50);
    assert_eq!(query.limit, 25);
    assert_eq!(query.definition["conditions"], body["conditions"]);
    assert_eq!(query.definition["columns"], body["columns"]);
    assert_eq!(query.definition["sorts"], body["sorts"]);
    assert_eq!(query.columns[0].column_id, "price-column");
    assert_eq!(query.columns[0].factor_key, "simple.price");
    handle.shutdown().await.expect("shutdown");
}

// Parity: go:452dea11:internal/api/productfeatures/research_screen_test.go:130 TestResearchScreenPostPreservesExecutableV2Definition
#[tokio::test]
async fn http_screen_post_preserves_interval_bounds_and_column_identity() {
    let directory = tempdir().expect("directory");
    let port = Arc::new(RecordingScreenPort::default());
    let handle = recording_product(&directory, Arc::clone(&port)).await;
    let body = json!({"brokerId": "api-test", "market": "US", "catalogVersion": "futu-stock-screen-v1",
        "querySchemaVersion": 2,
        "conditions": [{"id": "price-range", "factor": {"instanceId": "price-filter", "factorKey": "simple.price"}, "operator": "between", "value": {"min": 10.5, "minIncludes": false, "max": 120, "maxIncludes": true}}],
        "columns": [{"columnId": "price-column", "factor": {"instanceId": "price-result", "factorKey": "simple.price"}, "label": "最新价"}],
        "page": {"limit": 25}});
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/research/screens",
        Some(&body.to_string()),
        DESKTOP_HEADERS,
    )
    .await;
    assert_eq!(status, 200, "{response}");
    let queries = port.queries.lock().expect("queries").clone();
    assert_eq!(queries.len(), 1);
    let query = &queries[0];
    assert_eq!(query.broker_id, "api-test");
    assert_eq!(query.market, "US");
    assert_eq!(query.definition["querySchemaVersion"], 2);
    assert_eq!(query.definition["conditions"], body["conditions"]);
    assert_eq!(query.definition["columns"], body["columns"]);
    assert_eq!(response["data"]["catalogVersion"], "futu-stock-screen-v1");
    assert_eq!(response["data"]["columns"][0]["columnId"], "price-column");
    assert_eq!(response["data"]["columns"][0]["instanceId"], "price-result");
    handle.shutdown().await.expect("shutdown");
}

// Parity: go:452dea11:internal/api/productfeatures/research_screen_test.go:185 TestResearchScreenPostRejectsV1Payload
#[tokio::test]
async fn http_screen_rejects_legacy_shape_before_provider_execution() {
    let directory = tempdir().expect("directory");
    let handle = start_product(production_config(&directory))
        .await
        .expect("production product");
    let body = json!({"brokerId": "api-test", "market": "US", "catalogVersion": "futu-stock-screen-v1",
        "querySchemaVersion": 2, "filters": [{"factor": "simple.price", "min": {"value": 10}}],
        "columns": [{"factor": "simple.price"}]});
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/research/screens",
        Some(&body.to_string()),
        DESKTOP_HEADERS,
    )
    .await;
    assert_eq!(status, 400, "{response}");
    handle.shutdown().await.expect("shutdown production");
    let recording_directory = tempdir().expect("recording directory");
    let port = Arc::new(RecordingScreenPort::default());
    let handle = recording_product(&recording_directory, Arc::clone(&port)).await;
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/research/screens",
        Some(&body.to_string()),
        DESKTOP_HEADERS,
    )
    .await;
    assert_eq!(status, 400, "{response}");
    assert!(port.queries.lock().expect("queries").is_empty());
    let valid = r#"{"brokerId":"api-test","market":"US","catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}"#;
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/research/screens",
        Some(valid),
        DESKTOP_HEADERS,
    )
    .await;
    assert_eq!(status, 200, "{response}");
    assert_eq!(
        port.queries.lock().expect("queries").len(),
        1,
        "the same listener's provider boundary is actually wired"
    );
    handle.shutdown().await.expect("shutdown");
}
