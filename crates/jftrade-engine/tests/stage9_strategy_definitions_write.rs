#[path = "../src/product_strategy_definition_write_port.rs"]
mod product_strategy_definition_write_port;
#[path = "../src/product_strategy_definition_write_test_cutover.rs"]
mod product_strategy_definition_write_test_cutover;

use std::collections::{BTreeMap, VecDeque};
use std::sync::{Arc, Mutex};

use jftrade_api::ApiRequest;
use product_strategy_definition_write_port::{
    StrategyDefinitionWriteInput, StrategyDefinitionWriteOperation, StrategyDefinitionWritePort,
    StrategyDefinitionWritePortError, dispatch_strategy_definition_write,
    strategy_definition_write_routes,
};
use product_strategy_definition_write_test_cutover::StrategyDefinitionSqliteTestCutoverPort;
use serde::Deserialize;
use serde_json::{Value, json};

const FIXTURE_TIMESTAMP: &str = "2026-08-22T06:00:00Z";

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
    expected_statuses: Vec<u16>,
    responses: Vec<Value>,
}

#[derive(Debug)]
struct FixturePort {
    responses: Mutex<VecDeque<Result<Value, StrategyDefinitionWritePortError>>>,
    calls: Mutex<Vec<StrategyDefinitionWriteInput>>,
}

impl FixturePort {
    fn from_case(case: &FixtureCase) -> Self {
        let responses = case
            .responses
            .iter()
            .enumerate()
            .filter_map(|(index, response)| {
                if !case_port_call(case, index) {
                    return None;
                }
                Some(if response["ok"] == true {
                    Ok(response["data"].clone())
                } else {
                    Err(error_from_response(response, case.expected_statuses[index]))
                })
            })
            .collect::<VecDeque<_>>();
        Self {
            responses: Mutex::new(responses),
            calls: Mutex::new(Vec::new()),
        }
    }

    fn call_count(&self) -> usize {
        self.calls.lock().expect("strategy write calls lock").len()
    }
}

impl StrategyDefinitionWritePort for FixturePort {
    fn mutate(
        &self,
        input: &StrategyDefinitionWriteInput,
    ) -> Result<Value, StrategyDefinitionWritePortError> {
        self.calls
            .lock()
            .expect("strategy write calls lock")
            .push(input.clone());
        self.responses
            .lock()
            .expect("strategy write responses lock")
            .pop_front()
            .unwrap_or_else(|| {
                Err(StrategyDefinitionWritePortError::Unavailable(
                    "fixture response missing".to_owned(),
                ))
            })
    }
}

fn fixture() -> Fixture {
    let fixture: Fixture = serde_json::from_str(include_str!(
        "../../../tests/fixtures/rust-migration/stage9/strategy-definitions-write.json"
    ))
    .expect("strategy-definitions-write fixture");
    assert_eq!(fixture.version, "stage9.strategy-definitions-write.v1");
    fixture
}

fn request(case: &FixtureCase, index: usize) -> ApiRequest {
    ApiRequest {
        method: case.method.clone(),
        path: case.request_paths[index].clone(),
        query: String::new(),
        body: case.request_bodies[index].as_bytes().to_vec(),
        request_id: "stage9-strategy-definitions-write".to_owned(),
        desktop_trusted: true,
        origin_provided: false,
        origin_allowed: true,
        browser_authenticated: true,
        csrf_valid: false,
        session_cookie: None,
    }
}

fn raw_request(method: &str, path: &str, body: &[u8]) -> ApiRequest {
    ApiRequest {
        method: method.to_owned(),
        path: path.to_owned(),
        query: String::new(),
        body: body.to_vec(),
        request_id: "strategy-definition-write-durable".to_owned(),
        desktop_trusted: true,
        origin_provided: false,
        origin_allowed: true,
        browser_authenticated: true,
        csrf_valid: false,
        session_cookie: None,
    }
}

fn case_port_call(case: &FixtureCase, _index: usize) -> bool {
    !matches!(
        case.name.as_str(),
        "create-malformed-body"
            | "create-malformed-body-before-unavailable"
            | "update-malformed-body"
    )
}

fn error_from_response(response: &Value, status: u16) -> StrategyDefinitionWritePortError {
    let error = &response["error"];
    StrategyDefinitionWritePortError::Failed {
        status,
        code: error["code"]
            .as_str()
            .unwrap_or("STRATEGY_FAILED")
            .to_owned(),
        message: error["message"].as_str().unwrap_or_default().to_owned(),
    }
}

#[test]
fn strategy_definition_write_fixture_matches_go_owner_for_all_five_routes() {
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
            case.responses.len(),
            "{}",
            case.name
        );
        let port = Arc::new(FixturePort::from_case(case));
        for index in 0..case.request_paths.len() {
            let response = dispatch_strategy_definition_write(
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
        }
        assert_eq!(
            port.call_count(),
            case_port_call_count(case),
            "{}",
            case.name
        );
    }
}

#[test]
fn strategy_definition_write_leaf_has_exact_route_inventory_and_isolates_reads() {
    assert_eq!(strategy_definition_write_routes().len(), 5);
    assert!(strategy_definition_write_routes().contains(&(
        "POST",
        "/api/v1/strategy-definitions/{definitionId}/apply-linked-instances"
    )));
    let read = ApiRequest {
        method: "GET".to_owned(),
        path: "/api/v1/strategy-definitions".to_owned(),
        query: String::new(),
        body: Vec::new(),
        request_id: "strategy-definition-write-read-isolation".to_owned(),
        desktop_trusted: true,
        origin_provided: false,
        origin_allowed: true,
        browser_authenticated: true,
        csrf_valid: false,
        session_cookie: None,
    };
    let response = dispatch_strategy_definition_write(&read, None, FIXTURE_TIMESTAMP);
    assert_eq!(response.status, 404);
    assert_eq!(response.body["error"]["code"], "NOT_FOUND");
}

#[test]
fn strategy_definition_write_leaf_fails_closed_without_test_port() {
    let request = ApiRequest {
        method: "POST".to_owned(),
        path: "/api/v1/strategy-definitions".to_owned(),
        query: String::new(),
        body: br#"{"name":"Draft"}"#.to_vec(),
        request_id: "strategy-definition-write-unavailable".to_owned(),
        desktop_trusted: true,
        origin_provided: false,
        origin_allowed: true,
        browser_authenticated: true,
        csrf_valid: false,
        session_cookie: None,
    };
    let response = dispatch_strategy_definition_write(&request, None, FIXTURE_TIMESTAMP);
    assert_eq!(response.status, 503);
    assert_eq!(
        response.body["error"]["code"],
        "STRATEGY_DEFINITIONS_UNAVAILABLE"
    );
}

#[test]
fn sqlite_test_cutover_preserves_versions_rollback_linked_delete_and_restart() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let database_path = directory.path().join("strategy-definitions.db");
    let port = Arc::new(
        StrategyDefinitionSqliteTestCutoverPort::open(&database_path)
            .expect("open strategy definition test-cutover port"),
    );
    port.seed_definition(
        "fixture-concurrent",
        json!({"id":"fixture-concurrent","name":"Before"}),
        &[],
    )
    .expect("seed concurrent definition");

    let mut updates = Vec::new();
    for _ in 0..8 {
        let port = Arc::clone(&port);
        updates.push(std::thread::spawn(move || {
            port.mutate(&StrategyDefinitionWriteInput {
                operation: StrategyDefinitionWriteOperation::Update,
                definition_id: Some("fixture-concurrent".to_owned()),
                definition: Some(json!({"id":"body-id","name":"Same update"})),
                binding: None,
                binding_error: None,
            })
        }));
    }
    for update in updates {
        let result = update.join().expect("join concurrent strategy update");
        assert_eq!(result.expect("concurrent update")["version"], "0.1.1");
    }
    assert_eq!(
        port.version_count("fixture-concurrent")
            .expect("version count"),
        2
    );
    assert_eq!(
        port.current("fixture-concurrent")
            .expect("current concurrent definition")
            .expect("concurrent definition")
            .0,
        "0.1.1"
    );

    let instantiate = StrategyDefinitionWriteInput {
        operation: StrategyDefinitionWriteOperation::Instantiate,
        definition_id: Some("fixture-concurrent".to_owned()),
        definition: None,
        binding: Some(json!({"symbols":["US.AAPL"]})),
        binding_error: None,
    };
    port.reject_instance_create()
        .expect("install instance rollback trigger");
    assert!(matches!(
        port.mutate(&instantiate),
        Err(StrategyDefinitionWritePortError::Failed { status: 500, .. })
    ));
    assert_eq!(
        port.instance_count("fixture-concurrent")
            .expect("instance count after rollback"),
        0
    );
    assert!(
        port.linked_ids("fixture-concurrent")
            .expect("linked IDs after rollback")
            .is_empty()
    );
    port.clear_instance_rejection()
        .expect("clear instance rollback trigger");
    let first_instance = port
        .mutate(&instantiate)
        .expect("persist linked strategy instance");
    let first_instance_id = first_instance["id"]
        .as_str()
        .expect("instance ID")
        .to_owned();
    assert_eq!(
        port.instance_ids("fixture-concurrent")
            .expect("linked instance IDs"),
        vec![first_instance_id.clone()]
    );
    assert_eq!(
        port.linked_ids("fixture-concurrent")
            .expect("stored linked instance IDs"),
        vec![first_instance_id.clone()]
    );
    assert!(matches!(
        port.mutate(&StrategyDefinitionWriteInput {
            operation: StrategyDefinitionWriteOperation::Delete,
            definition_id: Some("fixture-concurrent".to_owned()),
            definition: None,
            binding: None,
            binding_error: None,
        }),
        Err(StrategyDefinitionWritePortError::Failed { status: 400, .. })
    ));

    port.seed_definition(
        "fixture-rollback",
        json!({"id":"fixture-rollback","name":"Before rollback"}),
        &[],
    )
    .expect("seed rollback definition");
    port.reject_version("0.1.1")
        .expect("install rollback trigger");
    let rollback = port.mutate(&StrategyDefinitionWriteInput {
        operation: StrategyDefinitionWriteOperation::Update,
        definition_id: Some("fixture-rollback".to_owned()),
        definition: Some(json!({"name":"Rejected update"})),
        binding: None,
        binding_error: None,
    });
    assert!(matches!(
        rollback,
        Err(StrategyDefinitionWritePortError::Failed { status: 500, .. })
    ));
    assert_eq!(
        port.current("fixture-rollback")
            .expect("current rollback definition")
            .expect("rollback definition")
            .1["name"],
        "Before rollback"
    );
    port.clear_version_rejection()
        .expect("clear version rollback trigger");

    port.seed_definition(
        "fixture-delete",
        json!({"id":"fixture-delete","name":"Delete"}),
        &["inst-1"],
    )
    .expect("seed linked definition");
    let delete = StrategyDefinitionWriteInput {
        operation: StrategyDefinitionWriteOperation::Delete,
        definition_id: Some("fixture-delete".to_owned()),
        definition: None,
        binding: None,
        binding_error: None,
    };
    assert!(matches!(
        port.mutate(&delete),
        Err(StrategyDefinitionWritePortError::Failed { status: 400, .. })
    ));
    port.set_linked_ids("fixture-delete", &[])
        .expect("clear linked definition instances");
    port.mutate(&delete).expect("soft delete definition");
    assert!(
        port.current("fixture-delete")
            .expect("current deleted definition")
            .expect("deleted definition")
            .2
    );

    let missing_instantiate = dispatch_strategy_definition_write(
        &raw_request(
            "POST",
            "/api/v1/strategy-definitions/missing/instantiate",
            b"{",
        ),
        Some(port.as_ref()),
        FIXTURE_TIMESTAMP,
    );
    assert_eq!(missing_instantiate.status, 404);
    assert_eq!(
        missing_instantiate.body["error"]["message"],
        "strategy resource not found"
    );

    drop(port);
    let reopened = StrategyDefinitionSqliteTestCutoverPort::open(&database_path)
        .expect("reopen strategy definition test-cutover port");
    assert_eq!(
        reopened
            .current("fixture-concurrent")
            .expect("reopened concurrent definition")
            .expect("persisted concurrent definition")
            .0,
        "0.1.1"
    );
    assert_eq!(
        reopened
            .instance_ids("fixture-concurrent")
            .expect("reopened linked instances"),
        vec![first_instance_id.clone()]
    );
    let second_instance = reopened
        .mutate(&instantiate)
        .expect("persist instance after restart");
    assert_ne!(second_instance["id"], first_instance_id);
    assert_eq!(
        reopened
            .instance_count("fixture-concurrent")
            .expect("reopened instance count"),
        2
    );
    assert!(
        reopened
            .current("fixture-delete")
            .expect("reopened deleted definition")
            .expect("persisted deleted definition")
            .2
    );
}

#[test]
fn sqlite_test_cutover_writer_lease_rejects_a_second_owner() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let database_path = directory.path().join("strategy-definitions-lease.db");
    let first = StrategyDefinitionSqliteTestCutoverPort::open(&database_path)
        .expect("open first strategy definition owner");
    let second = StrategyDefinitionSqliteTestCutoverPort::open(&database_path);
    assert!(
        second
            .expect_err("second strategy definition owner must be fenced")
            .contains("writer lease is already held")
    );
    drop(first);
    StrategyDefinitionSqliteTestCutoverPort::open(&database_path)
        .expect("writer lease becomes available after owner close");
}

fn case_port_call_count(case: &FixtureCase) -> usize {
    (0..case.request_paths.len())
        .filter(|index| case_port_call(case, *index))
        .count()
}
