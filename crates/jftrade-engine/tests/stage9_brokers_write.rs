#[path = "../src/product_brokers_write_port.rs"]
mod product_brokers_write_port;

use std::collections::{BTreeMap, VecDeque};
use std::sync::{Arc, Mutex};

use product_brokers_write_port::test_cutover::BrokersWriteSqliteTestCutoverPort;
use product_brokers_write_port::{
    BROKERS_WRITE_ROUTES, BrokersWriteContext, BrokersWriteInput, BrokersWriteOperation,
    BrokersWritePort, BrokersWritePortError, BrokersWriteQuery, BrokersWriteRequest,
    brokers_write_routes, dispatch_brokers_write,
};
use serde::Deserialize;
use serde_json::{Value, json};

const FIXTURE_TIMESTAMP: &str = "2026-08-23T10:00:00Z";

#[derive(Debug, Deserialize)]
struct Fixture {
    version: String,
    cases: Vec<FixtureCase>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct FixtureCase {
    name: String,
    requests: Vec<FixtureRequest>,
    expected: Vec<FixtureExpected>,
    #[serde(default)]
    go_calls: Vec<Value>,
    observation: FixtureObservation,
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

#[derive(Debug, Deserialize, Eq, PartialEq)]
#[serde(rename_all = "camelCase")]
struct FixtureObservation {
    place_calls: usize,
    cancel_calls: usize,
    unlock_calls: usize,
}

#[derive(Debug)]
struct FixturePort {
    responses: Mutex<VecDeque<Result<Value, BrokersWritePortError>>>,
    calls: Mutex<Vec<BrokersWriteInput>>,
}

impl FixturePort {
    fn from_case(case: &FixtureCase) -> Self {
        let responses = case
            .expected
            .iter()
            .filter(|expected| expected.port_call)
            .map(|expected| {
                if expected.envelope["ok"] == true {
                    Ok(expected.envelope["data"].clone())
                } else {
                    Err(error_from_envelope(expected))
                }
            })
            .collect();
        Self {
            responses: Mutex::new(responses),
            calls: Mutex::new(Vec::new()),
        }
    }

    fn calls(&self) -> Vec<BrokersWriteInput> {
        self.calls.lock().expect("brokers write calls lock").clone()
    }

    fn assert_drained(&self, case_name: &str) {
        assert!(
            self.responses
                .lock()
                .expect("brokers write responses lock")
                .is_empty(),
            "fixture responses remain for {case_name}"
        );
    }
}

impl BrokersWritePort for FixturePort {
    fn mutate(&self, input: &BrokersWriteInput) -> Result<Value, BrokersWritePortError> {
        self.calls
            .lock()
            .expect("brokers write calls lock")
            .push(input.clone());
        self.responses
            .lock()
            .expect("brokers write responses lock")
            .pop_front()
            .unwrap_or_else(|| {
                Err(BrokersWritePortError::Unavailable(
                    "fixture response missing".to_owned(),
                ))
            })
    }
}

#[test]
fn brokers_write_fixture_replays_go_wire_for_all_three_routes() {
    let fixture = fixture();
    assert_eq!(fixture.version, "stage9.brokers-write.v1");
    assert_eq!(fixture.cases.len(), 65);
    assert_eq!(brokers_write_routes(), &BROKERS_WRITE_ROUTES);

    let mut requests = 0;
    for case in &fixture.cases {
        assert_eq!(
            case.requests.len(),
            case.expected.len(),
            "case {}",
            case.name
        );
        requests += case.requests.len();
        let port = FixturePort::from_case(case);
        for (index, (request, expected)) in case.requests.iter().zip(&case.expected).enumerate() {
            let response =
                dispatch_brokers_write(&to_request(request), Some(&port), FIXTURE_TIMESTAMP);
            assert_eq!(
                response.status, expected.status,
                "case {} request {index}",
                case.name
            );
            assert_eq!(
                response.headers, expected.headers,
                "case {} request {index}",
                case.name
            );
            assert_eq!(
                response.body, expected.envelope,
                "case {} request {index}",
                case.name
            );
        }
        let calls = port.calls();
        assert_eq!(
            calls.len(),
            case.expected
                .iter()
                .filter(|expected| expected.port_call)
                .count(),
            "case {} port-call count",
            case.name
        );
        assert_eq!(
            brokers_write_observation(&case.go_calls),
            case.observation,
            "case {}",
            case.name
        );
        assert_input_shape(case, &calls);
        port.assert_drained(&case.name);
    }
    assert_eq!(requests, 68);
}

#[test]
fn brokers_write_fixture_covers_exact_route_inventory_and_rejects_reads() {
    let fixture = fixture();
    for (method, template) in brokers_write_routes() {
        assert!(
            fixture
                .cases
                .iter()
                .flat_map(|case| case.requests.iter())
                .any(|request| {
                    request.method == *method
                        && route_template_matches(&request.request_path, template)
                }),
            "fixture does not cover {method} {template}"
        );
    }

    let read = BrokersWriteRequest {
        method: "GET".to_owned(),
        path: "/api/v1/brokers/futu/orders".to_owned(),
        body: Some(b"{}".to_vec()),
        context: BrokersWriteContext::Normal,
    };
    let response = dispatch_brokers_write(&read, None, FIXTURE_TIMESTAMP);
    assert_eq!(response.status, 404);
    assert_eq!(response.body["error"]["code"], "NOT_FOUND");
}

#[test]
fn brokers_write_leaf_fails_closed_after_shape_validation() {
    let valid = BrokersWriteRequest {
        method: "POST".to_owned(),
        path: "/api/v1/brokers/futu/orders".to_owned(),
        body: Some(
            br#"{"symbol":"US.AAPL","side":"BUY","orderType":"LIMIT","quantity":1}"#.to_vec(),
        ),
        context: BrokersWriteContext::Normal,
    };
    let unavailable = dispatch_brokers_write(&valid, None, FIXTURE_TIMESTAMP);
    assert_eq!(unavailable.status, 503);
    assert_eq!(
        unavailable.body["error"]["code"],
        "BROKERS_WRITE_UNAVAILABLE"
    );

    let malformed = BrokersWriteRequest {
        body: Some(b"{".to_vec()),
        ..valid.clone()
    };
    let response = dispatch_brokers_write(&malformed, None, FIXTURE_TIMESTAMP);
    assert_eq!(response.status, 400);
    assert_eq!(response.body["error"]["code"], "BAD_REQUEST");

    let malformed_query = BrokersWriteRequest {
        path: "/api/v1/brokers/futu/orders?market=%zz".to_owned(),
        ..valid
    };
    let response = dispatch_brokers_write(&malformed_query, None, FIXTURE_TIMESTAMP);
    assert_eq!(response.status, 400);
    assert_eq!(
        response.body["error"]["message"],
        "invalid broker write query"
    );
}

#[test]
fn brokers_write_leaf_preserves_query_defaults_null_trailing_and_context() {
    let port = RecordingPort::default();
    let place = BrokersWriteRequest {
        method: "POST".to_owned(),
        path: "/api/v1/brokers/futu/orders?tradingEnvironment=%20simulate%20&accountId=+ACC-1+&market=us".to_owned(),
        body: Some(br#"{"symbol":"US.AAPL","quantity":1}{"ignored":true}"#.to_vec()),
        context: BrokersWriteContext::Normal,
    };
    let response = dispatch_brokers_write(&place, Some(&port), FIXTURE_TIMESTAMP);
    assert_eq!(response.status, 200);
    let input = port.last();
    assert_eq!(input.operation, BrokersWriteOperation::PlaceOrder);
    assert_eq!(input.query.broker_id, "futu");
    assert_eq!(input.query.account_id, "ACC-1");
    assert_eq!(input.query.trading_environment, "SIMULATE");
    assert_eq!(input.query.market, "us");
    assert_eq!(input.payload["symbol"], "US.AAPL");

    let null = BrokersWriteRequest {
        method: "POST".to_owned(),
        path: "/api/v1/brokers/futu/unlock".to_owned(),
        body: Some(b"null".to_vec()),
        context: BrokersWriteContext::Canceled,
    };
    let response = dispatch_brokers_write(&null, Some(&port), FIXTURE_TIMESTAMP);
    assert_eq!(response.status, 200);
    assert_eq!(port.last().operation, BrokersWriteOperation::Unlock);
    assert_eq!(port.last().payload, Value::Null);
    assert_eq!(port.last().context, BrokersWriteContext::Canceled);
}

#[test]
fn sqlite_test_cutover_preserves_place_cancel_unlock_rollback_and_restart() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let database_path = directory.path().join("brokers-test-cutover.db");
    let port = Arc::new(
        BrokersWriteSqliteTestCutoverPort::open(&database_path).expect("open durable adapter"),
    );
    let place = durable_input(
        BrokersWriteOperation::PlaceOrder,
        json!({"symbol":"US.AAPL","side":"BUY","quantity":1}),
        BrokersWriteContext::Normal,
    );
    port.reject_next_event().expect("install event rejection");
    assert!(matches!(
        port.mutate(&place),
        Err(BrokersWritePortError::Failed { .. })
    ));
    assert_eq!(port.order_count().expect("orders after rollback"), 0);
    assert_eq!(
        port.event_count("place-order")
            .expect("events after rollback"),
        0
    );

    let first = port.mutate(&place).expect("first place");
    let second = port.mutate(&place).expect("duplicate place");
    assert_eq!(first["orderId"], 1);
    assert_eq!(second["orderId"], 2);

    let cancel = durable_input(
        BrokersWriteOperation::CancelOrders,
        json!({"orders":[{"orderId":1}]}),
        BrokersWriteContext::Normal,
    );
    let attempts = (0..2)
        .map(|_| {
            let port = Arc::clone(&port);
            let cancel = cancel.clone();
            std::thread::spawn(move || port.mutate(&cancel))
        })
        .collect::<Vec<_>>();
    let mut transitioned = 0;
    let mut fenced = 0;
    for attempt in attempts {
        let response = attempt.join().expect("join cancel").expect("cancel result");
        if response["cancelled"] == 1 {
            transitioned += 1;
        } else {
            fenced += 1;
        }
    }
    assert_eq!((transitioned, fenced), (1, 1));
    assert_eq!(
        port.order_status(1).expect("cancelled order status"),
        Some("cancelled".to_owned())
    );
    assert_eq!(port.event_count("cancel-orders").expect("cancel events"), 1);

    let unlock = durable_input(
        BrokersWriteOperation::Unlock,
        json!({"unlock":false}),
        BrokersWriteContext::Normal,
    );
    assert_eq!(port.mutate(&unlock).expect("unlock")["unlocked"], true);
    assert_eq!(
        port.session_unlocked("fixture-broker")
            .expect("session state"),
        Some(true)
    );
    assert!(matches!(
        port.mutate(&durable_input(
            BrokersWriteOperation::PlaceOrder,
            json!({"symbol":"US.AAPL"}),
            BrokersWriteContext::Canceled,
        )),
        Err(BrokersWritePortError::Failed { status: 499, .. })
    ));
    assert_eq!(port.order_count().expect("orders after cancellation"), 2);

    drop(port);
    let reopened =
        BrokersWriteSqliteTestCutoverPort::open(&database_path).expect("reopen durable adapter");
    assert_eq!(
        reopened.order_status(1).expect("reopened order status"),
        Some("cancelled".to_owned())
    );
    assert_eq!(
        reopened
            .session_unlocked("fixture-broker")
            .expect("reopened session"),
        Some(true)
    );
    let restarted = reopened.mutate(&place).expect("post-restart place");
    assert_eq!(restarted["orderId"], 3);
}

fn durable_input(
    operation: BrokersWriteOperation,
    payload: Value,
    context: BrokersWriteContext,
) -> BrokersWriteInput {
    BrokersWriteInput {
        operation,
        query: BrokersWriteQuery {
            broker_id: "fixture-broker".to_owned(),
            account_id: "acct-1".to_owned(),
            trading_environment: "SIMULATE".to_owned(),
            market: "US".to_owned(),
        },
        payload,
        context,
    }
}

#[derive(Debug, Default)]
struct RecordingPort {
    inputs: Mutex<Vec<BrokersWriteInput>>,
}

impl RecordingPort {
    fn last(&self) -> BrokersWriteInput {
        self.inputs
            .lock()
            .expect("recording brokers write inputs lock")
            .last()
            .cloned()
            .expect("recorded brokers write input")
    }
}

impl BrokersWritePort for RecordingPort {
    fn mutate(&self, input: &BrokersWriteInput) -> Result<Value, BrokersWritePortError> {
        self.inputs
            .lock()
            .expect("recording brokers write inputs lock")
            .push(input.clone());
        Ok(json!({"accepted": true}))
    }
}

fn fixture() -> Fixture {
    serde_json::from_str(include_str!(
        "../../../tests/fixtures/rust-migration/stage9/brokers-write.json"
    ))
    .expect("brokers-write fixture")
}

fn to_request(request: &FixtureRequest) -> BrokersWriteRequest {
    BrokersWriteRequest {
        method: request.method.clone(),
        path: request.request_path.clone(),
        body: request.body.as_ref().map(|body| body.as_bytes().to_vec()),
        context: match request.context.as_str() {
            "canceled" => BrokersWriteContext::Canceled,
            "deadline" => BrokersWriteContext::Deadline,
            _ => BrokersWriteContext::Normal,
        },
    }
}

fn error_from_envelope(expected: &FixtureExpected) -> BrokersWritePortError {
    BrokersWritePortError::Failed {
        status: expected.status,
        code: expected.envelope["error"]["code"]
            .as_str()
            .unwrap_or("BROKERS_WRITE_FAILED")
            .to_owned(),
        message: expected.envelope["error"]["message"]
            .as_str()
            .unwrap_or_default()
            .to_owned(),
    }
}

fn brokers_write_observation(calls: &[Value]) -> FixtureObservation {
    let mut observation = FixtureObservation {
        place_calls: 0,
        cancel_calls: 0,
        unlock_calls: 0,
    };
    for call in calls {
        match call["kind"].as_str() {
            Some("place") => observation.place_calls += 1,
            Some("cancel") => observation.cancel_calls += 1,
            Some("unlock") => observation.unlock_calls += 1,
            kind => panic!("unknown Go broker call kind {kind:?}"),
        }
    }
    observation
}

fn assert_input_shape(case: &FixtureCase, calls: &[BrokersWriteInput]) {
    assert!(
        case.go_calls.len() <= calls.len(),
        "case {} has more Go broker calls than adapter calls",
        case.name
    );
    for (call, input) in case.go_calls.iter().zip(calls) {
        let expected_query = query_value(&input.query);
        for field in ["brokerId", "accountId", "tradingEnvironment", "market"] {
            assert_eq!(
                call["query"][field], expected_query[field],
                "case {} query field {field}",
                case.name
            );
        }
        assert_eq!(
            call["kind"],
            operation_kind(input.operation),
            "case {}",
            case.name
        );
        match input.operation {
            BrokersWriteOperation::CancelOrders => {
                let orders = input.payload["orders"]
                    .as_array()
                    .map(|items| {
                        items
                            .iter()
                            .map(|item| {
                                json!({
                                    "BrokerOrderID": item["brokerOrderId"],
                                    "OrderID": item["orderId"],
                                    "Symbol": item["symbol"],
                                })
                            })
                            .collect::<Vec<_>>()
                    })
                    .unwrap_or_default();
                assert_eq!(call["orders"], json!(orders), "case {}", case.name);
            }
            BrokersWriteOperation::Unlock => {
                assert_eq!(
                    call["request"]["unlock"],
                    input.payload["unlock"].as_bool().unwrap_or(false),
                    "case {}",
                    case.name
                );
                assert_eq!(
                    call["request"]["passwordMd5"],
                    input.payload["passwordMd5"]
                        .as_str()
                        .filter(|value| !value.is_empty())
                        .map_or(Value::Null, |value| json!(value)),
                    "case {}",
                    case.name
                );
            }
            BrokersWriteOperation::PlaceOrder => {
                assert!(input.payload.is_null() || input.payload.is_object());
                for field in ["symbol", "side", "orderType"] {
                    let expected = input.payload[field].as_str().unwrap_or_default();
                    assert_eq!(
                        call["query"][field], expected,
                        "case {} field {field}",
                        case.name
                    );
                }
                assert_eq!(
                    call["query"]["quantity"],
                    input.payload["quantity"].as_f64().unwrap_or(0.0),
                    "case {} quantity",
                    case.name
                );
            }
        }
    }
}

fn query_value(query: &product_brokers_write_port::BrokersWriteQuery) -> Value {
    json!({
        "brokerId": query.broker_id,
        "accountId": query.account_id,
        "tradingEnvironment": query.trading_environment,
        "market": query.market,
    })
}

fn operation_kind(operation: BrokersWriteOperation) -> Value {
    Value::String(
        match operation {
            BrokersWriteOperation::PlaceOrder => "place",
            BrokersWriteOperation::CancelOrders => "cancel",
            BrokersWriteOperation::Unlock => "unlock",
        }
        .to_owned(),
    )
}

fn route_template_matches(path: &str, template: &str) -> bool {
    let path = path.split('?').next().unwrap_or(path);
    let actual = path.split('/').collect::<Vec<_>>();
    let expected = template.split('/').collect::<Vec<_>>();
    actual.len() == expected.len()
        && actual
            .iter()
            .zip(expected)
            .all(|(actual, expected)| expected.starts_with('{') || actual == &expected)
}
