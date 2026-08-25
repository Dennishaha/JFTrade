use std::collections::VecDeque;
use std::sync::{Arc, Mutex};

use rusqlite::Connection;
use serde_json::{Value, json};
use tempfile::tempdir;

use super::super::product_research_preset_write_port::{
    ResearchPresetWriteMutation, ResearchPresetWritePortError,
};
use super::super::product_strategy_definition_write_port::{
    StrategyDefinitionWriteInput, StrategyDefinitionWritePortError,
};
use super::*;

#[derive(Debug)]
struct FixtureResearchPresetWritePort;

impl ResearchPresetWritePort for FixtureResearchPresetWritePort {
    fn mutate(
        &self,
        mutation: &ResearchPresetWriteMutation,
    ) -> Result<Value, ResearchPresetWritePortError> {
        Ok(json!({
            "operation": format!("{:?}", mutation.operation()),
            "presetId": "preset-test",
            "revision": 2,
            "deleted": matches!(mutation, ResearchPresetWriteMutation::Delete { .. }),
        }))
    }
}

#[derive(Debug)]
struct FixtureStrategyDefinitionWritePort;

impl StrategyDefinitionWritePort for FixtureStrategyDefinitionWritePort {
    fn mutate(
        &self,
        input: &StrategyDefinitionWriteInput,
    ) -> Result<Value, StrategyDefinitionWritePortError> {
        Ok(json!({
            "operation": input.operation.name(),
            "definitionId": input.definition_id,
            "hasDefinition": input.definition.is_some(),
            "hasBinding": input.binding.is_some(),
        }))
    }
}

#[derive(Debug)]
struct FailingStrategyDefinitionWritePort;

impl StrategyDefinitionWritePort for FailingStrategyDefinitionWritePort {
    fn mutate(
        &self,
        _input: &StrategyDefinitionWriteInput,
    ) -> Result<Value, StrategyDefinitionWritePortError> {
        Err(StrategyDefinitionWritePortError::Failed {
            status: 500,
            code: "STRATEGY_FAILED".to_owned(),
            message: "fixture mutation failed".to_owned(),
        })
    }
}

#[derive(Debug)]
struct SequencedStrategyDefinitionWritePort {
    responses: Mutex<VecDeque<Result<Value, StrategyDefinitionWritePortError>>>,
    calls: Mutex<Vec<StrategyDefinitionWriteInput>>,
}

impl SequencedStrategyDefinitionWritePort {
    fn new(
        responses: impl IntoIterator<Item = Result<Value, StrategyDefinitionWritePortError>>,
    ) -> Self {
        Self {
            responses: Mutex::new(responses.into_iter().collect()),
            calls: Mutex::new(Vec::new()),
        }
    }

    fn calls(&self) -> Vec<StrategyDefinitionWriteInput> {
        self.calls
            .lock()
            .expect("strategy definition write calls lock")
            .clone()
    }
}

impl StrategyDefinitionWritePort for SequencedStrategyDefinitionWritePort {
    fn mutate(
        &self,
        input: &StrategyDefinitionWriteInput,
    ) -> Result<Value, StrategyDefinitionWritePortError> {
        self.calls
            .lock()
            .expect("strategy definition write calls lock")
            .push(input.clone());
        self.responses
            .lock()
            .expect("strategy definition write responses lock")
            .pop_front()
            .expect("strategy definition write rehearsal response")
    }
}

#[tokio::test]
async fn strategy_and_research_write_routes_are_opt_in_and_counted_exactly() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let base = ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
        .expect("config");
    let handle = start_product(base).await.expect("start base product");
    assert_eq!(handle.startup_record().owned_routes, 48);
    let response = request_json(
        handle.startup_record().address,
        "POST",
        "/api/v1/research/screens/presets",
        Some("{}"),
    )
    .await;
    assert_eq!(response["error"]["code"], "NOT_FOUND");
    handle.shutdown().await.expect("shutdown base product");

    let research =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_research_preset_write_port(Arc::new(FixtureResearchPresetWritePort));
    let handle = start_product(research)
        .await
        .expect("start research product");
    assert_eq!(handle.startup_record().owned_routes, 51);
    let response = request_json(
        handle.startup_record().address,
        "PATCH",
        "/api/v1/research/screens/presets/preset-test",
        Some(r#"{"name":"Updated","expectedRevision":1}"#),
    )
    .await;
    assert_eq!(response["ok"], true);
    assert_eq!(response["data"]["presetId"], "preset-test");
    handle.shutdown().await.expect("shutdown research product");

    let strategy =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_strategy_definition_write_port(Arc::new(FixtureStrategyDefinitionWritePort));
    let handle = start_product(strategy)
        .await
        .expect("start strategy product");
    assert_eq!(handle.startup_record().owned_routes, 53);
    let response = request_json(
        handle.startup_record().address,
        "POST",
        "/api/v1/strategy-definitions/definition-1/instantiate",
        None,
    )
    .await;
    assert_eq!(response["ok"], true);
    assert_eq!(response["data"]["operation"], "instantiate");
    handle.shutdown().await.expect("shutdown strategy product");
}

#[tokio::test]
async fn strategy_and_research_write_routes_preserve_failure_and_combined_registration() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_research_preset_write_port(Arc::new(FixtureResearchPresetWritePort))
            .with_strategy_definition_write_port(Arc::new(FailingStrategyDefinitionWritePort));
    let handle = start_product(config).await.expect("start combined product");
    assert_eq!(handle.startup_record().owned_routes, 56);
    let response = request_json(
        handle.startup_record().address,
        "POST",
        "/api/v1/strategy-definitions",
        Some(r#"{"name":"Draft"}"#),
    )
    .await;
    assert_eq!(response["error"]["code"], "STRATEGY_FAILED");
    let response = request_json(
        handle.startup_record().address,
        "DELETE",
        "/api/v1/research/screens/presets/preset-test",
        Some("not-json"),
    )
    .await;
    assert_eq!(response["ok"], true);
    handle.shutdown().await.expect("shutdown combined product");
}

#[tokio::test]
async fn strategy_definition_write_product_replays_browser_failure_recovery_and_restart() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    std::fs::write(
        &settings_path,
        b"{\"seed\":\"strategy-definitions-write\"}\n",
    )
    .expect("seed settings");
    let settings_before = std::fs::read(&settings_path).expect("read settings before replay");
    let success = |operation: &str| {
        Ok(json!({
            "accepted": true,
            "source": "rust-product",
            "operation": operation,
        }))
    };
    let port = Arc::new(SequencedStrategyDefinitionWritePort::new([
        Err(StrategyDefinitionWritePortError::Unavailable(
            "fixture strategy definition owner unavailable".to_owned(),
        )),
        success("create"),
        Err(StrategyDefinitionWritePortError::Failed {
            status: 500,
            code: "STRATEGY_FAILED".to_owned(),
            message: "fixture definition update failure".to_owned(),
        }),
        success("update"),
        success("delete"),
        success("apply-linked-instances"),
        success("instantiate"),
        success("update"),
        success("instantiate"),
    ]));
    let mut config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_strategy_definition_write_port(port.clone());
    config.access = AccessPolicy {
        session_token: Some("fixture-browser-session".to_owned()),
        csrf_token: Some("fixture-csrf".to_owned()),
        enforce_access: true,
        desktop_mode: false,
        ..AccessPolicy::default()
    }
    .with_allowed_origins(["https://fixture.jftrade.local".to_owned()]);
    let handle = start_product(config)
        .await
        .expect("start strategy definition write product");
    assert_eq!(handle.startup_record().owned_routes, 53);
    let address = handle.startup_record().address;
    let browser_headers = [
        ("Cookie", "jftrade_web_session=fixture-browser-session"),
        ("Origin", "https://fixture.jftrade.local"),
        (
            "Referer",
            "https://fixture.jftrade.local/strategy-definitions",
        ),
        ("X-CSRF-Token", "fixture-csrf"),
        ("X-Request-ID", "strategy-definitions-write-product"),
    ];
    let create_path = "/api/v1/strategy-definitions";
    let create_body = r#"{"name":"Draft"}"#;

    let unauthorized =
        request_json_with_status(address, "POST", create_path, Some(create_body), &[]).await;
    assert_eq!(unauthorized.0, 401);
    let csrf_missing = request_json_with_status(
        address,
        "POST",
        create_path,
        Some(create_body),
        &[
            ("Cookie", "jftrade_web_session=fixture-browser-session"),
            ("Origin", "https://fixture.jftrade.local"),
            (
                "Referer",
                "https://fixture.jftrade.local/strategy-definitions",
            ),
        ],
    )
    .await;
    assert_eq!(csrf_missing.0, 403);
    assert_eq!(csrf_missing.1["error"]["code"], "CSRF_FAILED");

    let unavailable = request_json_with_status(
        address,
        "POST",
        create_path,
        Some(create_body),
        &browser_headers,
    )
    .await;
    assert_eq!(unavailable.0, 503);
    assert_eq!(
        unavailable.1["error"]["code"],
        "STRATEGY_DEFINITIONS_UNAVAILABLE"
    );
    let recovered = request_json_with_status(
        address,
        "POST",
        create_path,
        Some(create_body),
        &browser_headers,
    )
    .await;
    assert_eq!(recovered.0, 200);
    assert_eq!(recovered.1["data"]["operation"], "create");

    let update_path = "/api/v1/strategy-definitions/definition-1";
    let update_body = r#"{"id":"body-id","name":"Updated"}"#;
    let failed = request_json_with_status(
        address,
        "PUT",
        update_path,
        Some(update_body),
        &browser_headers,
    )
    .await;
    assert_eq!(failed.0, 500);
    assert_eq!(failed.1["error"]["code"], "STRATEGY_FAILED");
    let recovered = request_json_with_status(
        address,
        "PUT",
        update_path,
        Some(update_body),
        &browser_headers,
    )
    .await;
    assert_eq!(recovered.0, 200);
    assert_eq!(recovered.1["data"]["operation"], "update");

    for (method, path, body, operation) in [
        ("DELETE", update_path, None, "delete"),
        (
            "POST",
            "/api/v1/strategy-definitions/definition-1/apply-linked-instances",
            Some("not-json"),
            "apply-linked-instances",
        ),
        (
            "POST",
            "/api/v1/strategy-definitions/definition-1/instantiate",
            Some(r#"{"symbols":["AAPL"]}"#),
            "instantiate",
        ),
    ] {
        let (status, response) =
            request_json_with_status(address, method, path, body, &browser_headers).await;
        assert_eq!(status, 200, "{method} {path}");
        assert_eq!(response["data"]["operation"], operation, "{method} {path}");
    }
    let duplicate_update = request_json_with_status(
        address,
        "PUT",
        update_path,
        Some(update_body),
        &browser_headers,
    )
    .await;
    assert_eq!(duplicate_update.0, 200);
    let duplicate_instantiate = request_json_with_status(
        address,
        "POST",
        "/api/v1/strategy-definitions/definition-1/instantiate",
        Some(r#"{"symbols":["AAPL"]}"#),
        &browser_headers,
    )
    .await;
    assert_eq!(duplicate_instantiate.0, 200);

    let calls = port.calls();
    assert_eq!(calls.len(), 9);
    assert_eq!(
        calls
            .iter()
            .filter(|call| call.operation.name() == "update")
            .count(),
        3
    );
    assert_eq!(
        calls
            .iter()
            .filter(|call| call.operation.name() == "instantiate")
            .count(),
        2
    );
    handle
        .shutdown()
        .await
        .expect("shutdown strategy definition write product");
    assert_eq!(
        std::fs::read(&settings_path).expect("read settings after shutdown"),
        settings_before
    );

    let restarted_config = ProductConfig::test_cutover(
        "127.0.0.1:0".parse().expect("restarted address"),
        &settings_path,
    )
    .expect("restarted config")
    .with_strategy_definition_write_port(Arc::new(FixtureStrategyDefinitionWritePort));
    let restarted = start_product(restarted_config)
        .await
        .expect("restart strategy definition write product");
    let restarted_response = request_json_with_status(
        restarted.startup_record().address,
        "POST",
        "/api/v1/strategy-definitions/definition-1/instantiate",
        None,
        &[],
    )
    .await;
    assert_eq!(restarted_response.0, 200);
    assert_eq!(restarted_response.1["data"]["operation"], "instantiate");
    restarted
        .shutdown()
        .await
        .expect("shutdown restarted strategy definition write product");
    assert_eq!(
        std::fs::read(&settings_path).expect("read settings after restart"),
        settings_before
    );
}

#[tokio::test]
async fn explicit_sqlite_test_cutover_config_registers_durable_preset_routes() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let database_path = directory.path().join("research.db");
    let connection = Connection::open(&database_path).expect("open research database");
    connection
        .execute_batch(
            "CREATE TABLE research_screen_presets (
                preset_id TEXT PRIMARY KEY,
                name TEXT NOT NULL,
                name_key TEXT NOT NULL UNIQUE,
                query_schema_version INTEGER NOT NULL,
                query_json TEXT NOT NULL,
                revision INTEGER NOT NULL DEFAULT 1,
                created_at TEXT NOT NULL,
                updated_at TEXT NOT NULL
            );
            CREATE INDEX research_screen_presets_updated_at
                ON research_screen_presets(updated_at DESC, preset_id);
            CREATE TABLE jftrade_schema_meta (
                component_id TEXT PRIMARY KEY,
                version INTEGER NOT NULL,
                created_at TEXT NOT NULL
            );
            INSERT INTO jftrade_schema_meta (component_id, version, created_at)
                VALUES ('research', 1, '2026-08-24T04:00:00Z');",
        )
        .expect("seed research database");
    drop(connection);

    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_research_preset_sqlite_test_cutover(&database_path)
            .expect("test-cutover adapter");
    let handle = start_product(config).await.expect("start product");
    assert_eq!(handle.startup_record().owned_routes, 51);
    let response = request_json(
        handle.startup_record().address,
        "POST",
        "/api/v1/research/screens/presets",
        Some(
            r#"{"name":"Value","definition":{"market":"US","pool":{},"catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}}"#,
        ),
    )
    .await;
    assert_eq!(response["ok"], true);
    assert!(response["data"]["presetId"].as_str().is_some());
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn authenticated_sqlite_test_cutover_replays_mutations_and_recovers_across_restart() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let database_path = directory.path().join("research.db");
    seed_research_preset_schema(&database_path);

    let token = "research-preset-product-auth-token-012345678901234567890";
    let config =
        authenticated_research_preset_product_config(&settings_path, &database_path, token);
    let handle = start_product(config)
        .await
        .expect("start authenticated product");
    assert_eq!(handle.startup_record().owned_routes, 51);
    assert_eq!(
        handle.startup_record().route_profile,
        PRODUCT_TEST_CUTOVER_ROUTE_PROFILE
    );

    let create_body = r#"{"name":"  Value  ","definition":{"brokerId":"futu","market":"US","catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2,"columns":[{"columnId":"price","factor":{"instanceId":"price","factorKey":"simple.price"}}]}}"#;
    let authorization = format!("Bearer {token}");
    let authorized_headers = [("Authorization", authorization.as_str())];

    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/research/screens/presets",
        Some("{}"),
        &[],
    )
    .await;
    assert_eq!(status, 401);
    assert_eq!(response["error"]["code"], "WEB_AUTH_REQUIRED");

    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/research/screens/presets",
        Some("{}"),
        &authorized_headers,
    )
    .await;
    assert_eq!(status, 400);
    assert_eq!(response["error"]["code"], "RESEARCH_PRESET_INVALID");

    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/research/screens/presets",
        Some(create_body),
        &authorized_headers,
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["ok"], true);
    assert_eq!(response["data"]["name"], "Value");
    assert_eq!(response["data"]["revision"], 1);
    let preset_id = response["data"]["presetId"]
        .as_str()
        .expect("created preset id")
        .to_owned();

    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/research/screens/presets",
        Some(create_body),
        &authorized_headers,
    )
    .await;
    assert_eq!(status, 409);
    assert_eq!(response["error"]["code"], "RESEARCH_PRESET_CONFLICT");

    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "PATCH",
        "/api/v1/research/screens/presets/missing",
        Some(r#"{"name":"Missing","expectedRevision":1}"#),
        &authorized_headers,
    )
    .await;
    assert_eq!(status, 404);
    assert_eq!(response["error"]["code"], "RESEARCH_PRESET_NOT_FOUND");

    let patch_path = format!("/api/v1/research/screens/presets/{preset_id}");
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "PATCH",
        &patch_path,
        Some(r#"{"name":"Stale","expectedRevision":2}"#),
        &authorized_headers,
    )
    .await;
    assert_eq!(status, 409);
    assert_eq!(response["error"]["code"], "RESEARCH_PRESET_CONFLICT");

    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "PATCH",
        &patch_path,
        Some(r#"{"definition":{"brokerId":"futu","market":"US","catalogVersion":"futu-stock-screen-v1","querySchemaVersion":1},"expectedRevision":1}"#),
        &authorized_headers,
    )
    .await;
    assert_eq!(status, 400);
    assert_eq!(response["error"]["code"], "RESEARCH_PRESET_INVALID");

    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "PATCH",
        &patch_path,
        Some(r#"{"name":" Updated ","expectedRevision":1}"#),
        &authorized_headers,
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["data"]["name"], "Updated");
    assert_eq!(response["data"]["revision"], 2);

    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "DELETE",
        &patch_path,
        Some("not-json"),
        &authorized_headers,
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["data"], json!({"deleted": true}));

    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "DELETE",
        &patch_path,
        None,
        &authorized_headers,
    )
    .await;
    assert_eq!(status, 404);
    assert_eq!(response["error"]["code"], "RESEARCH_PRESET_NOT_FOUND");

    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/research/screens/presets",
        Some(
            r#"{"name":"Recovery","definition":{"brokerId":"futu","market":"US","catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2,"columns":[]}}"#,
        ),
        &authorized_headers,
    )
    .await;
    assert_eq!(status, 200);
    let recovery_id = response["data"]["presetId"]
        .as_str()
        .expect("recovery preset id")
        .to_owned();
    handle.shutdown().await.expect("shutdown first product");

    let handle = start_product(authenticated_research_preset_product_config(
        &settings_path,
        &database_path,
        token,
    ))
    .await
    .expect("restart authenticated product");
    let recovery_path = format!("/api/v1/research/screens/presets/{recovery_id}");
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "PATCH",
        &recovery_path,
        Some(r#"{"name":"Recovered","expectedRevision":1}"#),
        &authorized_headers,
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["data"]["name"], "Recovered");
    assert_eq!(response["data"]["revision"], 2);

    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "DELETE",
        &recovery_path,
        None,
        &authorized_headers,
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["data"], json!({"deleted": true}));
    handle.shutdown().await.expect("shutdown restarted product");
}

fn authenticated_research_preset_product_config(
    settings_path: &std::path::Path,
    database_path: &std::path::Path,
    token: &str,
) -> ProductConfig {
    let mut config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), settings_path)
            .expect("config");
    config.access = jftrade_api::AccessPolicy {
        desktop_token: Some(token.to_owned()),
        enforce_access: true,
        desktop_mode: true,
        ..config.access
    };
    config
        .with_research_preset_sqlite_test_cutover(database_path)
        .expect("research preset test-cutover adapter")
}

fn seed_research_preset_schema(path: &std::path::Path) {
    let connection = Connection::open(path).expect("open research database");
    connection
        .execute_batch(
            "CREATE TABLE research_screen_presets (
                preset_id TEXT PRIMARY KEY,
                name TEXT NOT NULL,
                name_key TEXT NOT NULL UNIQUE,
                query_schema_version INTEGER NOT NULL,
                query_json TEXT NOT NULL,
                revision INTEGER NOT NULL DEFAULT 1,
                created_at TEXT NOT NULL,
                updated_at TEXT NOT NULL
            );
            CREATE INDEX research_screen_presets_updated_at
                ON research_screen_presets(updated_at DESC, preset_id);
            CREATE TABLE jftrade_schema_meta (
                component_id TEXT PRIMARY KEY,
                version INTEGER NOT NULL,
                created_at TEXT NOT NULL
            );
            INSERT INTO jftrade_schema_meta (component_id, version, created_at)
                VALUES ('research', 1, '2026-08-24T04:00:00Z');",
        )
        .expect("seed research schema");
}
