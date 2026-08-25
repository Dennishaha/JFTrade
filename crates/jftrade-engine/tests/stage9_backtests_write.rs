#[path = "../src/product_backtests_write_port.rs"]
mod product_backtests_write_port;
#[path = "../src/product_backtests_write_test_cutover.rs"]
mod product_backtests_write_test_cutover;

use std::collections::{BTreeMap, VecDeque};
use std::sync::{Arc, Mutex};

use product_backtests_write_port::{
    BACKTEST_DELETE_PATH, BACKTEST_START_PATH, BACKTEST_SYNC_CANCEL_PATH, BACKTEST_SYNC_START_PATH,
    BacktestsWriteDeleteResult, BacktestsWriteInput, BacktestsWritePort, BacktestsWritePortError,
    BacktestsWritePortResult, BacktestsWriteRequest, backtests_write_routes,
    dispatch_backtests_write,
};
use product_backtests_write_test_cutover::BacktestsSqliteTestCutoverPort;
use serde::Deserialize;
use serde_json::{Value, json};

const FIXTURE_TIMESTAMP: &str = "2026-08-23T04:00:00Z";

#[derive(Debug, Deserialize)]
struct Fixture {
    version: String,
    timestamp: String,
    cases: Vec<FixtureCase>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct FixtureCase {
    name: String,
    requests: Vec<FixtureRequest>,
    port_mode: String,
    #[serde(default)]
    restart_after_first: bool,
    expected: Vec<FixtureExpected>,
    #[serde(default)]
    calls: Vec<Value>,
    effects: FixtureEffects,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct FixtureRequest {
    method: String,
    request_path: String,
    body: Option<String>,
    #[serde(default)]
    context: String,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct FixtureExpected {
    status: u16,
    headers: BTreeMap<String, String>,
    port_call: bool,
    envelope: Value,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct FixtureEffects {
    strategy_lookups: usize,
    run_adds: usize,
    sync_adapter_opens: usize,
    sync_task_adds: usize,
    sync_cancels: usize,
    run_status_reads: usize,
    run_deletes: usize,
}

#[derive(Debug)]
struct FixturePort {
    responses: Mutex<VecDeque<Result<BacktestsWritePortResult, BacktestsWritePortError>>>,
    calls: Mutex<Vec<BacktestsWriteInput>>,
}

impl FixturePort {
    fn from_case(case: &FixtureCase) -> Self {
        let responses = case
            .requests
            .iter()
            .zip(&case.expected)
            .filter_map(|(request, expected)| {
                if !expected.port_call {
                    return None;
                }
                Some(fixture_port_response(request, expected))
            })
            .collect();
        Self {
            responses: Mutex::new(responses),
            calls: Mutex::new(Vec::new()),
        }
    }

    fn assert_drained(&self, case_name: &str) {
        assert!(
            self.responses
                .lock()
                .expect("backtests write response lock")
                .is_empty(),
            "fixture responses remain for {case_name}"
        );
    }
}

impl BacktestsWritePort for FixturePort {
    fn mutate(
        &self,
        input: &BacktestsWriteInput,
    ) -> Result<BacktestsWritePortResult, BacktestsWritePortError> {
        self.calls
            .lock()
            .expect("backtests write call lock")
            .push(input.clone());
        self.responses
            .lock()
            .expect("backtests write response lock")
            .pop_front()
            .unwrap_or_else(|| {
                Err(BacktestsWritePortError::Unavailable(
                    "fixture response missing".to_owned(),
                ))
            })
    }
}

#[test]
fn backtests_write_fixture_replays_all_four_go_owned_mutations() {
    let fixture = fixture();
    assert_eq!(fixture.version, "stage9.backtests-write.v1");
    assert_eq!(fixture.timestamp, FIXTURE_TIMESTAMP);
    assert_eq!(fixture.cases.len(), 38);

    for case in &fixture.cases {
        assert!(!case.port_mode.is_empty(), "case {} mode", case.name);
        if case.restart_after_first {
            assert!(
                case.requests.len() >= 2,
                "case {} restart sequence",
                case.name
            );
        }
        assert_eq!(
            case.requests.len(),
            case.expected.len(),
            "case {}",
            case.name
        );
        let port = FixturePort::from_case(case);
        for (request, expected) in case.requests.iter().zip(&case.expected) {
            let response =
                dispatch_backtests_write(&to_request(request), Some(&port), FIXTURE_TIMESTAMP);
            assert_eq!(response.status, expected.status, "case {}", case.name);
            assert_eq!(response.headers, expected.headers, "case {}", case.name);
            assert_eq!(response.body, expected.envelope, "case {}", case.name);
        }
        let calls = port.calls.lock().expect("backtests write calls");
        let actual_calls = calls.iter().map(input_json).collect::<Vec<_>>();
        assert_eq!(
            actual_calls, case.calls,
            "case {} delegation trace",
            case.name
        );
        assert_eq!(
            actual_calls.len(),
            case.expected
                .iter()
                .filter(|expected| expected.port_call)
                .count(),
            "case {} port calls",
            case.name
        );
        drop(calls);
        port.assert_drained(&case.name);
        assert_effects_are_well_formed(case);
    }
}

#[test]
fn backtests_write_leaf_has_exact_route_inventory_and_read_isolation() {
    assert_eq!(backtests_write_routes().len(), 4);
    assert!(backtests_write_routes().contains(&("POST", BACKTEST_START_PATH)));
    assert!(backtests_write_routes().contains(&("POST", BACKTEST_SYNC_START_PATH)));
    assert!(backtests_write_routes().contains(&("DELETE", BACKTEST_SYNC_CANCEL_PATH)));
    assert!(backtests_write_routes().contains(&("DELETE", BACKTEST_DELETE_PATH)));

    let read = BacktestsWriteRequest {
        method: "GET".to_owned(),
        path: BACKTEST_START_PATH.to_owned(),
        body: None,
    };
    let response = dispatch_backtests_write(&read, None, FIXTURE_TIMESTAMP);
    assert_eq!(response.status, 404);
    assert_eq!(response.body["error"]["code"], "NOT_FOUND");
}

#[test]
fn backtests_write_cancel_blank_id_preserves_go_route_branch() {
    let case = fixture()
        .cases
        .into_iter()
        .find(|case| case.name == "cancel-blank-id")
        .expect("blank task-id fixture");
    let port = FixturePort::from_case(&case);
    let response = dispatch_backtests_write(
        &to_request(&case.requests[0]),
        Some(&port),
        FIXTURE_TIMESTAMP,
    );
    assert_eq!(response.status, 404);
    assert_eq!(response.body["error"]["code"], "NOT_FOUND");
    assert_eq!(
        port.calls
            .lock()
            .expect("blank task-id calls")
            .iter()
            .map(input_json)
            .collect::<Vec<_>>(),
        case.calls
    );
}

#[test]
fn backtests_write_leaf_fails_closed_without_a_test_port_after_shape_validation() {
    let requests = [
        BacktestsWriteRequest {
            method: "POST".to_owned(),
            path: BACKTEST_START_PATH.to_owned(),
            body: Some(br#"{"definitionId":"def-1"}"#.to_vec()),
        },
        BacktestsWriteRequest {
            method: "POST".to_owned(),
            path: BACKTEST_SYNC_START_PATH.to_owned(),
            body: Some(br#"{"market":"US"}"#.to_vec()),
        },
        BacktestsWriteRequest {
            method: "DELETE".to_owned(),
            path: "/api/v1/backtests/sync/fixture-task".to_owned(),
            body: None,
        },
        BacktestsWriteRequest {
            method: "DELETE".to_owned(),
            path: "/api/v1/backtests/fixture-run".to_owned(),
            body: None,
        },
    ];
    for request in requests {
        let response = dispatch_backtests_write(&request, None, FIXTURE_TIMESTAMP);
        assert_eq!(response.status, 503, "path {}", request.path);
        assert_eq!(
            response.body["error"]["code"], "BACKTESTS_WRITE_UNAVAILABLE",
            "path {}",
            request.path
        );
    }

    let malformed = BacktestsWriteRequest {
        method: "POST".to_owned(),
        path: BACKTEST_START_PATH.to_owned(),
        body: Some(b"{".to_vec()),
    };
    let response = dispatch_backtests_write(&malformed, None, FIXTURE_TIMESTAMP);
    assert_eq!(response.status, 400);
    assert_eq!(response.body["error"]["code"], "BAD_REQUEST");
}

#[test]
fn backtests_write_leaf_preserves_trailing_json_and_error_precedence() {
    let fixture = fixture();
    let trailing = fixture
        .cases
        .iter()
        .find(|case| case.name == "start-trailing-json-is-ignored")
        .expect("trailing-json fixture");
    let port = FixturePort::from_case(trailing);
    let response = dispatch_backtests_write(
        &to_request(&trailing.requests[0]),
        Some(&port),
        FIXTURE_TIMESTAMP,
    );
    assert_eq!(response.status, 200);
    assert_eq!(response.body["data"]["status"], "queued");

    let malformed = BacktestsWriteRequest {
        method: "POST".to_owned(),
        path: "/api/v1/backtests?ignored=true".to_owned(),
        body: Some(b"{".to_vec()),
    };
    let response = dispatch_backtests_write(&malformed, None, FIXTURE_TIMESTAMP);
    assert_eq!(response.status, 400);
    assert_eq!(
        response.body["error"]["message"],
        "invalid backtest request"
    );
}

#[test]
fn sqlite_test_cutover_preserves_rollback_duplicate_fencing_and_restart() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let database_path = directory.path().join("backtests-test-cutover.db");
    let port = Arc::new(
        BacktestsSqliteTestCutoverPort::open(&database_path).expect("open durable adapter"),
    );
    let start = BacktestsWriteInput::Start {
        payload: json!({"definitionId":"definition-1"}),
    };
    port.reject_start_event().expect("install rejection");
    assert!(matches!(
        port.mutate(&start),
        Err(BacktestsWritePortError::Failed(_))
    ));
    assert_eq!(port.run_count().expect("runs after rollback"), 0);
    assert_eq!(port.event_count("start").expect("events after rollback"), 0);
    port.clear_rejection().expect("clear rejection");

    let first = port.mutate(&start).expect("first start");
    let second = port.mutate(&start).expect("duplicate start");
    assert_eq!(data_id(&first, "id"), "run-test-1");
    assert_eq!(data_id(&second, "id"), "run-test-2");
    assert_eq!(port.run_count().expect("duplicate runs"), 2);

    let sync = BacktestsWriteInput::Sync {
        payload: json!({"market":"US","symbol":"AAPL"}),
    };
    let first_task = port.mutate(&sync).expect("first sync");
    let second_task = port.mutate(&sync).expect("duplicate sync");
    assert_eq!(data_id(&first_task, "taskId"), "task-test-3");
    assert_eq!(data_id(&second_task, "taskId"), "task-test-4");

    let cancel = BacktestsWriteInput::CancelSync {
        task_id: "task-test-3".to_owned(),
    };
    let attempts = (0..2)
        .map(|_| {
            let port = Arc::clone(&port);
            let cancel = cancel.clone();
            std::thread::spawn(move || port.mutate(&cancel))
        })
        .collect::<Vec<_>>();
    let mut cancelled = 0;
    let mut fenced = 0;
    for attempt in attempts {
        match attempt
            .join()
            .expect("join cancellation")
            .expect("cancel result")
        {
            BacktestsWritePortResult::SyncCancelled(true) => cancelled += 1,
            BacktestsWritePortResult::SyncCancelled(false) => fenced += 1,
            result => panic!("unexpected cancel result: {result:?}"),
        }
    }
    assert_eq!((cancelled, fenced), (1, 1));
    assert_eq!(
        port.task_status("task-test-3").expect("task status"),
        Some("cancelled".to_owned())
    );
    assert_eq!(port.event_count("cancel-sync").expect("cancel events"), 1);

    port.seed_run("terminal-run", "completed")
        .expect("seed terminal run");
    port.seed_run("active-run", "running")
        .expect("seed active run");
    let delete_terminal = BacktestsWriteInput::Delete {
        run_id: "terminal-run".to_owned(),
    };
    assert_eq!(
        port.mutate(&delete_terminal).expect("delete terminal"),
        BacktestsWritePortResult::RunDeleted(BacktestsWriteDeleteResult::Deleted)
    );
    assert_eq!(
        port.mutate(&delete_terminal).expect("repeat delete"),
        BacktestsWritePortResult::RunDeleted(BacktestsWriteDeleteResult::Missing)
    );
    assert_eq!(
        port.mutate(&BacktestsWriteInput::Delete {
            run_id: "active-run".to_owned(),
        })
        .expect("active delete"),
        BacktestsWritePortResult::RunDeleted(BacktestsWriteDeleteResult::NotTerminal)
    );
    assert_eq!(port.event_count("delete").expect("delete events"), 1);

    drop(port);
    let reopened = BacktestsSqliteTestCutoverPort::open(&database_path).expect("reopen adapter");
    assert_eq!(
        reopened.task_status("task-test-3").expect("reopened task"),
        Some("cancelled".to_owned())
    );
    assert!(!reopened.run_exists("terminal-run").expect("deleted run"));
    assert!(reopened.run_exists("active-run").expect("active run"));
    let restarted = reopened.mutate(&start).expect("post-restart start");
    assert_eq!(data_id(&restarted, "id"), "run-test-5");
}

fn data_id<'a>(result: &'a BacktestsWritePortResult, key: &str) -> &'a str {
    let BacktestsWritePortResult::Data(data) = result else {
        panic!("expected data result: {result:?}");
    };
    data[key].as_str().expect("result id")
}

fn fixture() -> Fixture {
    serde_json::from_str(include_str!(
        "../../../tests/fixtures/rust-migration/stage9/backtests-write.json"
    ))
    .expect("backtests-write fixture")
}

fn to_request(request: &FixtureRequest) -> BacktestsWriteRequest {
    let _ = &request.context;
    BacktestsWriteRequest {
        method: request.method.clone(),
        path: request.request_path.clone(),
        body: request.body.as_ref().map(|body| body.as_bytes().to_vec()),
    }
}

fn fixture_port_response(
    request: &FixtureRequest,
    expected: &FixtureExpected,
) -> Result<BacktestsWritePortResult, BacktestsWritePortError> {
    let path = request
        .request_path
        .split_once('?')
        .map_or(request.request_path.as_str(), |pair| pair.0);
    let error_message = expected.envelope["error"]["message"]
        .as_str()
        .unwrap_or("fixture backtests-write error")
        .to_owned();
    if request.method == "POST" {
        if expected.envelope["ok"] == true {
            return Ok(BacktestsWritePortResult::Data(
                expected.envelope["data"].clone(),
            ));
        }
        if path == BACKTEST_START_PATH {
            return Err(match expected.envelope["error"]["code"].as_str() {
                Some("BAD_REQUEST") => BacktestsWritePortError::BadRequest(error_message),
                Some("NOT_FOUND") => BacktestsWritePortError::StrategyNotFound(error_message),
                _ => BacktestsWritePortError::Failed(error_message),
            });
        }
        return Err(match expected.envelope["error"]["code"].as_str() {
            Some("BAD_REQUEST") => BacktestsWritePortError::BadRequest(error_message),
            _ => BacktestsWritePortError::Failed(error_message),
        });
    }
    if path.starts_with("/api/v1/backtests/sync/") {
        if expected.envelope["ok"] == true {
            return Ok(BacktestsWritePortResult::SyncCancelled(true));
        }
        return Ok(BacktestsWritePortResult::SyncCancelled(false));
    }
    if expected.envelope["ok"] == true {
        return Ok(BacktestsWritePortResult::RunDeleted(
            BacktestsWriteDeleteResult::Deleted,
        ));
    }
    match expected.status {
        400 => Ok(BacktestsWritePortResult::RunDeleted(
            BacktestsWriteDeleteResult::NotTerminal,
        )),
        404 => Ok(BacktestsWritePortResult::RunDeleted(
            BacktestsWriteDeleteResult::Missing,
        )),
        _ => Err(BacktestsWritePortError::Failed(error_message)),
    }
}

fn input_json(input: &BacktestsWriteInput) -> Value {
    match input {
        BacktestsWriteInput::Start { payload } => {
            json!({"operation":"start", "payload": payload})
        }
        BacktestsWriteInput::Sync { payload } => {
            json!({"operation":"sync", "payload": payload})
        }
        BacktestsWriteInput::CancelSync { task_id } => {
            json!({"operation":"cancel-sync", "taskId": task_id})
        }
        BacktestsWriteInput::Delete { run_id } => {
            json!({"operation":"delete", "runId": run_id})
        }
    }
}

fn assert_effects_are_well_formed(case: &FixtureCase) {
    let effects = &case.effects;
    let expected_calls = case
        .expected
        .iter()
        .filter(|expected| expected.port_call)
        .count();
    assert!(
        effects.strategy_lookups <= expected_calls,
        "case {}",
        case.name
    );
    assert!(effects.run_adds <= expected_calls, "case {}", case.name);
    assert!(
        effects.sync_adapter_opens <= expected_calls,
        "case {}",
        case.name
    );
    assert!(
        effects.sync_task_adds <= expected_calls,
        "case {}",
        case.name
    );
    assert!(effects.sync_cancels <= expected_calls, "case {}", case.name);
    assert!(
        effects.run_status_reads <= expected_calls,
        "case {}",
        case.name
    );
    assert!(effects.run_deletes <= expected_calls, "case {}", case.name);
}
