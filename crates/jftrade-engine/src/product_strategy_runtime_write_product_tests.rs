use std::collections::VecDeque;
use std::sync::{Arc, Mutex};

use serde_json::{Value, json};
use tempfile::tempdir;

use super::super::product_strategy_runtime_write_port::{
    StrategyRuntimeWriteInput, StrategyRuntimeWritePort, StrategyRuntimeWritePortError,
};
use super::super::product_strategy_runtime_write_test_cutover::StrategyRuntimeSqliteTestCutoverPort;
use super::*;

#[derive(Debug)]
struct FixtureStrategyRuntimeWritePort;

impl StrategyRuntimeWritePort for FixtureStrategyRuntimeWritePort {
    fn mutate(
        &self,
        input: &StrategyRuntimeWriteInput,
    ) -> Result<Value, StrategyRuntimeWritePortError> {
        Ok(json!({
            "accepted": true,
            "operation": input.operation.name(),
            "instanceId": input.instance_id,
        }))
    }
}

#[derive(Debug)]
struct SequencedStrategyRuntimeWritePort {
    responses: Mutex<VecDeque<Result<Value, StrategyRuntimeWritePortError>>>,
    calls: Mutex<Vec<StrategyRuntimeWriteInput>>,
}

impl SequencedStrategyRuntimeWritePort {
    fn new(
        responses: impl IntoIterator<Item = Result<Value, StrategyRuntimeWritePortError>>,
    ) -> Self {
        Self {
            responses: Mutex::new(responses.into_iter().collect()),
            calls: Mutex::new(Vec::new()),
        }
    }

    fn calls(&self) -> Vec<StrategyRuntimeWriteInput> {
        self.calls
            .lock()
            .expect("strategy runtime write calls lock")
            .clone()
    }
}

impl StrategyRuntimeWritePort for SequencedStrategyRuntimeWritePort {
    fn mutate(
        &self,
        input: &StrategyRuntimeWriteInput,
    ) -> Result<Value, StrategyRuntimeWritePortError> {
        self.calls
            .lock()
            .expect("strategy runtime write calls lock")
            .push(input.clone());
        self.responses
            .lock()
            .expect("strategy runtime write responses lock")
            .pop_front()
            .expect("strategy runtime write rehearsal response")
    }
}

#[tokio::test]
async fn strategy_runtime_write_routes_register_only_with_explicit_test_port() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let base = ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
        .expect("config");
    let handle = start_product(base).await.expect("start base product");
    assert_eq!(handle.startup_record().owned_routes, 48);
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/strategies/fixture-instance/start",
        None,
        &[],
    )
    .await;
    assert_eq!(status, 404);
    assert_eq!(response["error"]["code"], "NOT_FOUND");
    handle.shutdown().await.expect("shutdown base product");

    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_strategy_runtime_write_port(Arc::new(FixtureStrategyRuntimeWritePort));
    let handle = start_product(config)
        .await
        .expect("start strategy runtime product");
    assert_eq!(handle.startup_record().owned_routes, 55);
    assert!(
        handle
            .startup_record()
            .capabilities
            .iter()
            .any(|route| { route == "POST /api/v1/strategies/{instanceId}/start" })
    );
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/strategies/fixture-instance/start",
        None,
        &[],
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["ok"], true);
    assert_eq!(response["data"]["accepted"], true);
    assert_eq!(response["data"]["operation"], "start");
    handle
        .shutdown()
        .await
        .expect("shutdown strategy runtime product");
}

#[tokio::test]
async fn strategy_runtime_write_product_replays_browser_failure_recovery_and_restart() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, b"{\"seed\":\"strategies-write\"}\n").expect("seed settings");
    let settings_before = std::fs::read(&settings_path).expect("read settings before replay");
    let success = |operation: &str| {
        Ok(json!({
            "accepted": true,
            "source": "rust-product",
            "operation": operation,
        }))
    };
    let port = Arc::new(SequencedStrategyRuntimeWritePort::new([
        Err(StrategyRuntimeWritePortError::Unavailable(
            "fixture strategy runtime unavailable".to_owned(),
        )),
        success("start"),
        Err(StrategyRuntimeWritePortError::Failed {
            status: 502,
            code: "STRATEGY_RUNTIME_START_FAILED".to_owned(),
            message: "context deadline exceeded".to_owned(),
        }),
        success("start"),
        success("update"),
        success("update-runtime-risk"),
        success("pause"),
        success("stop"),
        success("refresh-definition"),
        success("delete"),
        success("pause"),
    ]));
    let mut config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_strategy_runtime_write_port(port.clone());
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
        .expect("start strategy runtime write product");
    assert_eq!(handle.startup_record().owned_routes, 55);
    let address = handle.startup_record().address;
    let browser_headers = [
        ("Cookie", "jftrade_web_session=fixture-browser-session"),
        ("Origin", "https://fixture.jftrade.local"),
        ("Referer", "https://fixture.jftrade.local/strategies"),
        ("X-CSRF-Token", "fixture-csrf"),
        ("X-Request-ID", "strategies-write-product"),
    ];
    let start_path = "/api/v1/strategies/instance-1/start";

    let unauthorized = request_json_with_status(address, "POST", start_path, None, &[]).await;
    assert_eq!(unauthorized.0, 401);
    let csrf_missing = request_json_with_status(
        address,
        "POST",
        start_path,
        None,
        &[
            ("Cookie", "jftrade_web_session=fixture-browser-session"),
            ("Origin", "https://fixture.jftrade.local"),
            ("Referer", "https://fixture.jftrade.local/strategies"),
        ],
    )
    .await;
    assert_eq!(csrf_missing.0, 403);
    assert_eq!(csrf_missing.1["error"]["code"], "CSRF_FAILED");

    let unavailable =
        request_json_with_status(address, "POST", start_path, None, &browser_headers).await;
    assert_eq!(unavailable.0, 503);
    assert_eq!(unavailable.1["error"]["code"], "STRATEGY_UNAVAILABLE");
    let recovered =
        request_json_with_status(address, "POST", start_path, None, &browser_headers).await;
    assert_eq!(recovered.0, 200);
    assert_eq!(recovered.1["data"]["operation"], "start");
    let failed =
        request_json_with_status(address, "POST", start_path, None, &browser_headers).await;
    assert_eq!(failed.0, 502);
    assert_eq!(failed.1["error"]["code"], "STRATEGY_RUNTIME_START_FAILED");
    let recovered =
        request_json_with_status(address, "POST", start_path, None, &browser_headers).await;
    assert_eq!(recovered.0, 200);

    for (method, path, body, operation) in [
        (
            "PUT",
            "/api/v1/strategies/instance-1",
            Some(r#"{"symbols":["AAPL"],"interval":"1m"}"#),
            "update",
        ),
        (
            "PUT",
            "/api/v1/strategies/instance-1/runtime-risk",
            Some(r#"{"mode":"paper","closeOnly":true}"#),
            "update-runtime-risk",
        ),
        (
            "POST",
            "/api/v1/strategies/instance-1/pause",
            Some("ignored-pause-body"),
            "pause",
        ),
        ("POST", "/api/v1/strategies/instance-1/stop", None, "stop"),
        (
            "POST",
            "/api/v1/strategies/instance-1/refresh-definition",
            Some("not-json"),
            "refresh-definition",
        ),
        ("DELETE", "/api/v1/strategies/instance-1", None, "delete"),
    ] {
        let (status, response) =
            request_json_with_status(address, method, path, body, &browser_headers).await;
        assert_eq!(status, 200, "{method} {path}");
        assert_eq!(response["data"]["operation"], operation, "{method} {path}");
    }
    let duplicate = request_json_with_status(
        address,
        "POST",
        "/api/v1/strategies/instance-1/pause",
        Some("ignored-pause-body"),
        &browser_headers,
    )
    .await;
    assert_eq!(duplicate.0, 200);
    assert_eq!(duplicate.1["data"]["operation"], "pause");

    let calls = port.calls();
    assert_eq!(calls.len(), 11);
    assert_eq!(
        calls
            .iter()
            .filter(|call| call.operation.name() == "start")
            .count(),
        4
    );
    assert_eq!(
        calls
            .iter()
            .filter(|call| call.operation.name() == "pause")
            .count(),
        2
    );
    handle
        .shutdown()
        .await
        .expect("shutdown strategy runtime write product");
    assert_eq!(
        std::fs::read(&settings_path).expect("read settings after shutdown"),
        settings_before
    );

    let restarted_config = ProductConfig::test_cutover(
        "127.0.0.1:0".parse().expect("restarted address"),
        &settings_path,
    )
    .expect("restarted config")
    .with_strategy_runtime_write_port(Arc::new(FixtureStrategyRuntimeWritePort));
    let restarted = start_product(restarted_config)
        .await
        .expect("restart strategy runtime write product");
    let restarted_response = request_json_with_status(
        restarted.startup_record().address,
        "POST",
        "/api/v1/strategies/instance-1/start",
        None,
        &[],
    )
    .await;
    assert_eq!(restarted_response.0, 200);
    assert_eq!(restarted_response.1["data"]["operation"], "start");
    restarted
        .shutdown()
        .await
        .expect("shutdown restarted strategy runtime write product");
    assert_eq!(
        std::fs::read(&settings_path).expect("read settings after restart"),
        settings_before
    );
}

#[tokio::test]
async fn strategy_runtime_sqlite_test_cutover_replays_transport_and_restart() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let database_path = directory.path().join("strategy-runtime-test-cutover.db");
    std::fs::write(&settings_path, b"{\"seed\":\"strategy-runtime-durable\"}\n")
        .expect("seed settings");
    let settings_before = std::fs::read(&settings_path).expect("read settings");
    seed_strategy_runtime_schema(&database_path);
    let port = Arc::new(
        StrategyRuntimeSqliteTestCutoverPort::open(&database_path).expect("open fixture adapter"),
    );
    port.seed_instance("durable-instance", "STOPPED")
        .expect("seed instance");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_strategy_runtime_write_port(port.clone());
    let handle = start_product(config).await.expect("start product");
    let address = handle.startup_record().address;

    for (method, path, body, status) in [
        (
            "PUT",
            "/api/v1/strategies/durable-instance",
            Some(r#"{"symbols":["AAPL"],"interval":"1m"}"#),
            "STOPPED",
        ),
        (
            "PUT",
            "/api/v1/strategies/durable-instance/runtime-risk",
            Some(r#"{"mode":"paper","closeOnly":true}"#),
            "STOPPED",
        ),
        (
            "POST",
            "/api/v1/strategies/durable-instance/start",
            None,
            "RUNNING",
        ),
        (
            "POST",
            "/api/v1/strategies/durable-instance/pause",
            None,
            "PAUSED",
        ),
        (
            "POST",
            "/api/v1/strategies/durable-instance/refresh-definition",
            None,
            "PAUSED",
        ),
    ] {
        let response = request_json_with_status(address, method, path, body, &[]).await;
        assert_eq!(response.0, 200, "{method} {path}");
        assert_eq!(response.1["data"]["status"], status, "{method} {path}");
    }
    assert_eq!(
        port.event_count("durable-instance", "pause")
            .expect("pause count"),
        1
    );
    handle.shutdown().await.expect("shutdown product");
    drop(port);

    let reopened = Arc::new(
        StrategyRuntimeSqliteTestCutoverPort::open(&database_path).expect("reopen adapter"),
    );
    let restarted_config = ProductConfig::test_cutover(
        "127.0.0.1:0".parse().expect("restart address"),
        &settings_path,
    )
    .expect("restart config")
    .with_strategy_runtime_write_port(reopened.clone());
    let restarted = start_product(restarted_config)
        .await
        .expect("restart product");
    let stopped = request_json_with_status(
        restarted.startup_record().address,
        "POST",
        "/api/v1/strategies/durable-instance/stop",
        None,
        &[],
    )
    .await;
    assert_eq!(stopped.0, 200);
    assert_eq!(stopped.1["data"]["status"], "STOPPED");
    restarted.shutdown().await.expect("shutdown restart");
    assert_eq!(
        std::fs::read(&settings_path).expect("read settings after restart"),
        settings_before
    );
}

fn seed_strategy_runtime_schema(path: &std::path::Path) {
    let connection = rusqlite::Connection::open(path).expect("open strategy database");
    connection
        .execute_batch(
            "CREATE TABLE strategy_log_events (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                instance_id TEXT NOT NULL,
                at_ms INTEGER NOT NULL,
                raw TEXT NOT NULL,
                level TEXT NOT NULL DEFAULT '',
                source TEXT NOT NULL DEFAULT ''
            );
            CREATE TABLE strategy_audit_events (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                instance_id TEXT NOT NULL,
                kind TEXT NOT NULL,
                detail TEXT NOT NULL DEFAULT '',
                at_ms INTEGER NOT NULL
            );
            CREATE TABLE strategy_runtime_observations (
                instance_id TEXT PRIMARY KEY,
                actual_status_snapshot TEXT NOT NULL DEFAULT '',
                active_symbols_json TEXT NOT NULL DEFAULT '[]',
                last_closed_kline_at_ms INTEGER,
                last_signal_at_ms INTEGER,
                last_order_at_ms INTEGER,
                last_error_at_ms INTEGER,
                last_error TEXT NOT NULL DEFAULT '',
                updated_at_ms INTEGER
            );
            CREATE TABLE strategy_catalog_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '');
            CREATE TABLE strategy_catalog_plugins (id TEXT PRIMARY KEY, payload_json TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL DEFAULT '');
            CREATE TABLE strategy_catalog_strategies (id TEXT PRIMARY KEY, payload_json TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL DEFAULT '');
            CREATE TABLE strategy_catalog_operations (operation_id TEXT PRIMARY KEY, plugin_id TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL DEFAULT '', payload_json TEXT NOT NULL DEFAULT '');
            CREATE TABLE strategy_design_definitions (
                id TEXT PRIMARY KEY,
                name TEXT NOT NULL DEFAULT '',
                version TEXT NOT NULL DEFAULT '',
                description TEXT NOT NULL DEFAULT '',
                runtime TEXT NOT NULL DEFAULT '',
                source_format TEXT NOT NULL DEFAULT '',
                symbol TEXT NOT NULL DEFAULT '',
                interval TEXT NOT NULL DEFAULT '',
                script TEXT NOT NULL DEFAULT '',
                visual_model_json TEXT NOT NULL DEFAULT '',
                created_at TEXT NOT NULL DEFAULT '',
                updated_at TEXT NOT NULL DEFAULT '',
                deleted_at TEXT
            );
            CREATE TABLE strategy_definition_versions (
                definition_id TEXT NOT NULL,
                version TEXT NOT NULL,
                name TEXT NOT NULL DEFAULT '',
                description TEXT NOT NULL DEFAULT '',
                runtime TEXT NOT NULL DEFAULT '',
                source_format TEXT NOT NULL DEFAULT '',
                symbol TEXT NOT NULL DEFAULT '',
                interval TEXT NOT NULL DEFAULT '',
                script TEXT NOT NULL DEFAULT '',
                visual_model_json TEXT NOT NULL DEFAULT '',
                created_at TEXT NOT NULL DEFAULT '',
                updated_at TEXT NOT NULL DEFAULT '',
                saved_at TEXT NOT NULL DEFAULT '',
                PRIMARY KEY (definition_id, version),
                FOREIGN KEY (definition_id) REFERENCES strategy_design_definitions(id) ON DELETE CASCADE
            );
            CREATE INDEX idx_strategy_log_events_instance_at ON strategy_log_events (instance_id, at_ms DESC, id DESC);
            CREATE INDEX idx_strategy_log_events_level ON strategy_log_events (level);
            CREATE INDEX idx_strategy_audit_events_instance_at ON strategy_audit_events (instance_id, at_ms DESC, id DESC);
            CREATE INDEX idx_strategy_audit_events_kind ON strategy_audit_events (kind);
            CREATE INDEX idx_strategy_catalog_strategies_created_at ON strategy_catalog_strategies (created_at ASC, id ASC);
            CREATE INDEX idx_strategy_catalog_operations_updated_at ON strategy_catalog_operations (updated_at DESC, operation_id ASC);
            CREATE INDEX idx_strategy_design_definitions_updated_at ON strategy_design_definitions (updated_at DESC, id ASC);
            CREATE INDEX idx_strategy_design_definitions_deleted_at ON strategy_design_definitions (deleted_at);
            CREATE INDEX idx_strategy_definition_versions_saved_at ON strategy_definition_versions (definition_id, saved_at DESC, version DESC);
            CREATE TRIGGER trg_strategy_definition_versions_immutable
                BEFORE UPDATE ON strategy_definition_versions
                BEGIN
                    SELECT RAISE(ABORT, 'strategy definition versions are immutable');
                END;
            CREATE TABLE jftrade_schema_meta (
                component_id TEXT PRIMARY KEY,
                version INTEGER NOT NULL,
                created_at TEXT NOT NULL
            );
            INSERT INTO jftrade_schema_meta (component_id, version, created_at)
                VALUES ('strategy', 2, '2026-08-22T06:00:00Z');",
        )
        .expect("seed strategy schema");
}
