#[path = "../src/product_strategy_runtime_write_port.rs"]
mod product_strategy_runtime_write_port;
#[path = "../src/product_strategy_runtime_write_test_cutover.rs"]
mod product_strategy_runtime_write_test_cutover;

use std::collections::{BTreeMap, VecDeque};
use std::sync::{Arc, Mutex};

use jftrade_api::ApiRequest;
use product_strategy_runtime_write_port::{
    StrategyRuntimeWriteInput, StrategyRuntimeWriteOperation, StrategyRuntimeWritePort,
    StrategyRuntimeWritePortError, dispatch_strategy_runtime_write, strategy_runtime_write_routes,
};
use product_strategy_runtime_write_test_cutover::StrategyRuntimeSqliteTestCutoverPort;
use serde::Deserialize;
use serde_json::Value;

const FIXTURE_TIMESTAMP: &str = "2026-08-23T06:00:00Z";

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct Fixture {
    version: String,
    cases: Vec<FixtureCase>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct FixtureCase {
    name: String,
    method: String,
    request_paths: Vec<String>,
    request_bodies: Vec<String>,
    #[serde(default)]
    expected_statuses: Vec<u16>,
    port_calls: Vec<bool>,
    responses: Vec<Value>,
    expected_observation: Value,
}

#[derive(Debug)]
struct FixturePort {
    responses: Mutex<VecDeque<Result<Value, StrategyRuntimeWritePortError>>>,
    calls: Mutex<Vec<StrategyRuntimeWriteInput>>,
}

impl FixturePort {
    fn from_case(case: &FixtureCase) -> Self {
        let responses = case
            .responses
            .iter()
            .enumerate()
            .filter_map(|(index, response)| {
                if !case.port_calls[index] {
                    return None;
                }
                Some(if response["ok"] == true {
                    Ok(response["data"].clone())
                } else {
                    Err(error_from_response(response, case.expected_statuses[index]))
                })
            })
            .collect();
        Self {
            responses: Mutex::new(responses),
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

impl StrategyRuntimeWritePort for FixturePort {
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
            .unwrap_or_else(|| {
                Err(StrategyRuntimeWritePortError::Unavailable(
                    "fixture response missing".to_owned(),
                ))
            })
    }
}

fn fixture() -> Fixture {
    let fixture: Fixture = serde_json::from_str(include_str!(
        "../../../tests/fixtures/rust-migration/stage9/strategies-write.json"
    ))
    .expect("strategies-write fixture");
    assert_eq!(fixture.version, "stage9.strategies-write.v1");
    fixture
}

fn request(case: &FixtureCase, index: usize) -> ApiRequest {
    ApiRequest {
        method: case.method.clone(),
        path: case.request_paths[index].clone(),
        query: String::new(),
        body: case.request_bodies[index].as_bytes().to_vec(),
        request_id: "stage9-strategies-write".to_owned(),
        desktop_trusted: true,
        origin_provided: false,
        origin_allowed: true,
        browser_authenticated: true,
        csrf_valid: false,
        session_cookie: None,
    }
}

fn error_from_response(response: &Value, status: u16) -> StrategyRuntimeWritePortError {
    let error = &response["error"];
    StrategyRuntimeWritePortError::Failed {
        status,
        code: error["code"]
            .as_str()
            .unwrap_or("STRATEGY_FAILED")
            .to_owned(),
        message: error["message"].as_str().unwrap_or_default().to_owned(),
    }
}

#[test]
fn strategies_runtime_write_fixture_matches_go_owner_for_all_seven_routes() {
    let fixture = fixture();
    for case in &fixture.cases {
        assert_eq!(
            case.request_paths.len(),
            case.request_bodies.len(),
            "{}",
            case.name
        );
        assert_eq!(
            case.request_paths.len(),
            case.expected_statuses.len(),
            "{}",
            case.name
        );
        assert_eq!(
            case.request_paths.len(),
            case.port_calls.len(),
            "{}",
            case.name
        );
        assert_eq!(
            case.request_paths.len(),
            case.responses.len(),
            "{}",
            case.name
        );

        let port = Arc::new(FixturePort::from_case(case));
        let mut previous_calls = 0;
        for index in 0..case.request_paths.len() {
            let response = dispatch_strategy_runtime_write(
                &request(case, index),
                Some(port.as_ref()),
                FIXTURE_TIMESTAMP,
            );
            assert_eq!(
                response.status, case.expected_statuses[index],
                "{}",
                case.name
            );
            assert_eq!(
                response.headers,
                BTreeMap::from([(
                    "Content-Type".to_owned(),
                    "application/json; charset=utf-8".to_owned(),
                )]),
                "{}",
                case.name
            );
            let mut expected = case.responses[index].clone();
            expected["timestamp"] = Value::String(FIXTURE_TIMESTAMP.to_owned());
            assert_eq!(response.body, expected, "{}", case.name);

            let calls = port.calls();
            let actual_delta = calls.len() - previous_calls;
            assert_eq!(
                actual_delta,
                usize::from(case.port_calls[index]),
                "{}",
                case.name
            );
            if case.port_calls[index] {
                assert_runtime_write_input(case, &calls[previous_calls], index);
            }
            previous_calls = calls.len();
        }
        assert_eq!(
            port.calls().len(),
            case.port_calls.iter().filter(|called| **called).count()
        );
        assert_boundary_observation(case, &port.calls());
    }
}

fn assert_runtime_write_input(case: &FixtureCase, input: &StrategyRuntimeWriteInput, index: usize) {
    let path = &case.request_paths[index];
    let suffix = path
        .strip_prefix("/api/v1/strategies/")
        .expect("strategy route prefix");
    let mut parts = suffix.split('/');
    let instance_id = parts.next().expect("instance id");
    assert_eq!(input.instance_id, instance_id, "{}", case.name);
    let operation = match parts.next() {
        None => match case.method.as_str() {
            "PUT" => StrategyRuntimeWriteOperation::Update,
            "DELETE" => StrategyRuntimeWriteOperation::Delete,
            method => panic!("unexpected base method {method} in {}", case.name),
        },
        Some("runtime-risk") => StrategyRuntimeWriteOperation::UpdateRuntimeRisk,
        Some("pause") => StrategyRuntimeWriteOperation::Pause,
        Some("stop") => StrategyRuntimeWriteOperation::Stop,
        Some("start") => StrategyRuntimeWriteOperation::Start,
        Some("refresh-definition") => StrategyRuntimeWriteOperation::RefreshDefinition,
        Some(action) => panic!("unexpected action {action} in {}", case.name),
    };
    assert_eq!(input.operation, operation, "{}", case.name);

    if operation == StrategyRuntimeWriteOperation::Update {
        let expected = case.expected_observation["updateCalls"]
            .as_array()
            .expect("update observation")
            .first()
            .map(|call| call["binding"].clone())
            .expect("update binding observation");
        assert_eq!(input.binding.as_ref(), Some(&expected), "{}", case.name);
    }
    if operation == StrategyRuntimeWriteOperation::UpdateRuntimeRisk {
        let expected = case.expected_observation["runtimeRiskCalls"]
            .as_array()
            .expect("risk observation")
            .first()
            .map(|call| call["risk"].clone())
            .expect("risk observation value");
        assert_eq!(
            input.runtime_risk.as_ref(),
            Some(&expected),
            "{}",
            case.name
        );
    }
}

fn assert_boundary_observation(case: &FixtureCase, calls: &[StrategyRuntimeWriteInput]) {
    let expected = &case.expected_observation;
    let expected_update_count = expected["updateCalls"].as_array().map_or(0, Vec::len);
    let expected_risk_count = expected["runtimeRiskCalls"].as_array().map_or(0, Vec::len);
    let expected_delete_count = expected["deleteCalls"].as_array().map_or(0, Vec::len);
    let expected_refresh_count = expected["refreshCalls"].as_array().map_or(0, Vec::len);
    assert_eq!(
        calls
            .iter()
            .filter(|call| call.operation == StrategyRuntimeWriteOperation::Update)
            .count(),
        expected_update_count,
        "{}",
        case.name
    );
    assert_eq!(
        calls
            .iter()
            .filter(|call| call.operation == StrategyRuntimeWriteOperation::UpdateRuntimeRisk)
            .count(),
        expected_risk_count,
        "{}",
        case.name
    );
    assert_eq!(
        calls
            .iter()
            .filter(|call| call.operation == StrategyRuntimeWriteOperation::Delete)
            .count(),
        expected_delete_count,
        "{}",
        case.name
    );
    assert_eq!(
        calls
            .iter()
            .filter(|call| call.operation == StrategyRuntimeWriteOperation::RefreshDefinition)
            .count(),
        expected_refresh_count,
        "{}",
        case.name
    );
}

#[test]
fn strategies_runtime_write_leaf_has_exact_route_inventory_and_isolates_reads() {
    assert_eq!(strategy_runtime_write_routes().len(), 7);
    assert!(
        strategy_runtime_write_routes()
            .contains(&("POST", "/api/v1/strategies/{instanceId}/refresh-definition"))
    );
    let read = ApiRequest {
        method: "GET".to_owned(),
        path: "/api/v1/strategies".to_owned(),
        query: String::new(),
        body: Vec::new(),
        request_id: "strategies-runtime-write-read-isolation".to_owned(),
        desktop_trusted: true,
        origin_provided: false,
        origin_allowed: true,
        browser_authenticated: true,
        csrf_valid: false,
        session_cookie: None,
    };
    let response = dispatch_strategy_runtime_write(&read, None, FIXTURE_TIMESTAMP);
    assert_eq!(response.status, 404);
    assert_eq!(response.body["error"]["code"], "NOT_FOUND");
}

#[test]
fn strategies_runtime_write_leaf_fails_closed_without_test_port() {
    let request = ApiRequest {
        method: "POST".to_owned(),
        path: "/api/v1/strategies/fixture-instance/start".to_owned(),
        query: String::new(),
        body: b"malformed body is ignored by Go control routes".to_vec(),
        request_id: "strategies-runtime-write-unavailable".to_owned(),
        desktop_trusted: true,
        origin_provided: false,
        origin_allowed: true,
        browser_authenticated: true,
        csrf_valid: false,
        session_cookie: None,
    };
    let response = dispatch_strategy_runtime_write(&request, None, FIXTURE_TIMESTAMP);
    assert_eq!(response.status, 503);
    assert_eq!(response.body["error"]["code"], "STRATEGY_UNAVAILABLE");
}

#[test]
fn sqlite_test_cutover_preserves_repeated_transitions_rollback_and_restart() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let database_path = directory.path().join("strategy-runtime-test-cutover.db");
    seed_strategy_runtime_schema(&database_path);
    let port = Arc::new(
        StrategyRuntimeSqliteTestCutoverPort::open(&database_path).expect("open fixture adapter"),
    );
    port.seed_instance("durable-instance", "STOPPED")
        .expect("seed instance");

    let pause = StrategyRuntimeWriteInput {
        operation: StrategyRuntimeWriteOperation::Pause,
        instance_id: "durable-instance".to_owned(),
        binding: None,
        runtime_risk: None,
    };
    let threads = (0..2)
        .map(|_| {
            let port = Arc::clone(&port);
            let pause = pause.clone();
            std::thread::spawn(move || port.mutate(&pause).expect("pause transition"))
        })
        .collect::<Vec<_>>();
    for thread in threads {
        assert_eq!(thread.join().expect("join pause")["status"], "PAUSED");
    }
    assert_eq!(
        port.event_count("durable-instance", "pause")
            .expect("pause count"),
        2
    );

    port.reject_status("RUNNING").expect("install rejection");
    let start = StrategyRuntimeWriteInput {
        operation: StrategyRuntimeWriteOperation::Start,
        instance_id: "durable-instance".to_owned(),
        binding: None,
        runtime_risk: None,
    };
    let error = port.mutate(&start).expect_err("start rollback");
    assert!(matches!(
        error,
        StrategyRuntimeWritePortError::Failed {
            status: 502,
            ref code,
            ..
        } if code == "STRATEGY_RUNTIME_START_FAILED"
    ));
    let snapshot = port
        .snapshot("durable-instance")
        .expect("snapshot")
        .expect("instance");
    assert_eq!(snapshot["status"], "PAUSED");
    assert_eq!(snapshot["runtimeActive"], false);
    assert_eq!(
        port.event_count("durable-instance", "start")
            .expect("start count"),
        0
    );
    port.clear_rejection().expect("clear rejection");
    assert_eq!(port.mutate(&start).expect("start")["status"], "RUNNING");

    drop(port);
    let reopened =
        StrategyRuntimeSqliteTestCutoverPort::open(&database_path).expect("reopen fixture adapter");
    let snapshot = reopened
        .snapshot("durable-instance")
        .expect("reopened snapshot")
        .expect("reopened instance");
    assert_eq!(snapshot["status"], "RUNNING");
    assert_eq!(snapshot["runtimeActive"], true);

    let delete = StrategyRuntimeWriteInput {
        operation: StrategyRuntimeWriteOperation::Delete,
        instance_id: "durable-instance".to_owned(),
        binding: None,
        runtime_risk: None,
    };
    assert!(matches!(
        reopened.mutate(&delete),
        Err(StrategyRuntimeWritePortError::Failed { status: 400, .. })
    ));
    let stop = StrategyRuntimeWriteInput {
        operation: StrategyRuntimeWriteOperation::Stop,
        ..start
    };
    reopened.mutate(&stop).expect("stop before delete");
    assert_eq!(reopened.mutate(&delete).expect("delete")["deleted"], true);
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
