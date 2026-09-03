#[path = "../src/product_alerts_write_port.rs"]
mod product_alerts_write_port;

use std::sync::Mutex;

use product_alerts_write_port::{
    AlertWriteAction, AlertWritePort, AlertWritePortError, AlertWriteRequest, AlertWriteResolution,
    AlertWriteRoute, dispatch_alert_write,
};
use serde_json::{Value, json};

const FIXTURE_TIMESTAMP: &str = "2026-08-22T04:00:00Z";

#[test]
fn alerts_write_routes_match_go_fixture_in_cutover_only() {
    let fixture: Value = serde_json::from_str(include_str!(
        "../../../tests/fixtures/compatibility/api-transport/alerts-write.json"
    ))
    .expect("alerts-write fixture");
    assert_eq!(fixture["version"], "stage9.alerts-write.v1");
    let cases = fixture["cases"].as_array().expect("alerts-write cases");
    assert_eq!(cases.len(), 18);

    for case in cases {
        let mode = case["portMode"].as_str().expect("port mode");
        let requests = case["requests"].as_array().expect("requests");
        let expected = case["expected"].as_array().expect("expected responses");
        assert_eq!(requests.len(), expected.len(), "case {case:?}");
        let context = requests
            .first()
            .and_then(|request| request.get("context"))
            .and_then(Value::as_str)
            .unwrap_or("");
        let port = FixturePort::new(mode, context);
        for (index, (request_case, expected_case)) in requests.iter().zip(expected).enumerate() {
            let before_apply = *port.apply_calls.lock().expect("apply call lock");
            let request = AlertWriteRequest {
                method: request_case["method"].as_str().expect("method").to_owned(),
                path: request_case["requestPath"]
                    .as_str()
                    .expect("request path")
                    .to_owned(),
                body: request_case
                    .get("body")
                    .and_then(Value::as_str)
                    .map(|body| body.as_bytes().to_vec()),
            };
            let response = dispatch_alert_write(&request, Some(&port), FIXTURE_TIMESTAMP);
            let expected_status = expected_case["status"].as_u64().expect("expected status") as u16;
            assert_eq!(
                response.status, expected_status,
                "case {case:?}, request {index}"
            );
            assert_eq!(
                response.body, expected_case["envelope"],
                "case {case:?}, request {index}"
            );
            assert_eq!(
                serde_json::to_value(&response.headers).expect("headers JSON"),
                expected_case["headers"],
                "case {case:?}, request {index}"
            );
            let after_apply = *port.apply_calls.lock().expect("apply call lock");
            assert_eq!(
                after_apply > before_apply,
                expected_case["portCall"].as_bool().expect("port call"),
                "case {case:?}, request {index}"
            );
        }

        let apply_calls = *port.apply_calls.lock().expect("apply call lock");
        let actions = port.actions.lock().expect("action lock");
        let payload_states = port.payload_states.lock().expect("payload state lock");
        let mut calls = json!({"apply": apply_calls});
        if !actions.is_empty() {
            calls["actions"] = Value::Array(actions.iter().map(action_json).collect());
            calls["payloadState"] =
                Value::Array(payload_states.iter().map(|state| json!(state)).collect());
        }
        assert_eq!(calls, case["calls"], "call trace for case {case:?}");
    }
}

#[test]
fn alerts_write_routes_fail_closed_when_test_port_is_unavailable() {
    let request = AlertWriteRequest {
        method: "POST".to_owned(),
        path: "/api/v1/alerts/price?brokerId=futu".to_owned(),
        body: Some(br#"{"symbol":"US.AAPL","price":100}"#.to_vec()),
    };
    let response = dispatch_alert_write(&request, None, FIXTURE_TIMESTAMP);
    assert_eq!(response.status, 503);
    assert_eq!(response.body["error"]["code"], "ALERTS_UNAVAILABLE");
}

#[test]
fn alerts_write_routes_preserve_injected_port_unavailability() {
    let request = AlertWriteRequest {
        method: "POST".to_owned(),
        path: "/api/v1/alerts/option-events?brokerId=futu".to_owned(),
        body: Some(br#"{"operation":"add"}"#.to_vec()),
    };
    let port = FixturePort::new("unavailable", "");
    let response = dispatch_alert_write(&request, Some(&port), FIXTURE_TIMESTAMP);
    assert_eq!(response.status, 503);
    assert_eq!(response.body["error"]["code"], "ALERTS_UNAVAILABLE");
    assert_eq!(*port.apply_calls.lock().expect("apply call lock"), 0);
}

#[test]
fn alerts_write_leaf_only_accepts_exact_post_paths() {
    let port = FixturePort::new("success", "");
    let get_request = AlertWriteRequest {
        method: "GET".to_owned(),
        path: "/api/v1/alerts/price?brokerId=futu".to_owned(),
        body: Some(br#"{"symbol":"US.AAPL"}"#.to_vec()),
    };
    let unknown_request = AlertWriteRequest {
        method: "POST".to_owned(),
        path: "/api/v1/alerts/price/unknown".to_owned(),
        body: Some(br#"{"symbol":"US.AAPL"}"#.to_vec()),
    };
    assert_eq!(
        dispatch_alert_write(&get_request, Some(&port), FIXTURE_TIMESTAMP).status,
        404
    );
    assert_eq!(
        dispatch_alert_write(&unknown_request, Some(&port), FIXTURE_TIMESTAMP).status,
        404
    );
    assert_eq!(*port.apply_calls.lock().expect("apply call lock"), 0);
}

#[derive(Debug)]
struct FixturePort {
    mode: String,
    context: String,
    apply_calls: Mutex<usize>,
    actions: Mutex<Vec<AlertWriteAction>>,
    payload_states: Mutex<Vec<&'static str>>,
}

impl FixturePort {
    fn new(mode: &str, context: &str) -> Self {
        Self {
            mode: mode.to_owned(),
            context: context.to_owned(),
            apply_calls: Mutex::new(0),
            actions: Mutex::new(Vec::new()),
            payload_states: Mutex::new(Vec::new()),
        }
    }
}

impl AlertWritePort for FixturePort {
    fn resolve(
        &self,
        route: AlertWriteRoute,
        broker_id: Option<&str>,
        _account_id: Option<&str>,
    ) -> Result<AlertWriteResolution, AlertWritePortError> {
        let feature_id = route.feature_id();
        match self.mode.as_str() {
            "unavailable" => Err(AlertWritePortError::Unavailable(
                "alerts write port is unavailable".to_owned(),
            )),
            "missing-broker" => Err(AlertWritePortError::CapabilityUnavailable(
                "broker feature capability is unavailable: broker \"missing\" is not registered"
                    .to_owned(),
            )),
            "capability-unavailable" => Err(AlertWritePortError::CapabilityUnavailable(format!(
                "broker feature capability is unavailable: broker \"futu\" feature \"{feature_id}\" is unavailable: "
            ))),
            "adapter-unavailable" => Err(AlertWritePortError::CapabilityUnavailable(format!(
                "broker feature capability is unavailable: broker \"futu\" feature \"{feature_id}\" is unavailable: adapter interface CustomizationService is not implemented"
            ))),
            _ => Ok(AlertWriteResolution {
                broker_id: "futu".to_owned(),
                security_firm: "Futu/Moomoo via OpenD".to_owned(),
                capability: "available".to_owned(),
                selection_reason: if broker_id.is_some() {
                    "explicit_broker".to_owned()
                } else {
                    "default_broker".to_owned()
                },
            }),
        }
    }

    fn apply(
        &self,
        _resolution: &AlertWriteResolution,
        action: &AlertWriteAction,
    ) -> Result<Option<Value>, AlertWritePortError> {
        *self.apply_calls.lock().expect("apply call lock") += 1;
        self.actions
            .lock()
            .expect("action lock")
            .push(action.clone());
        self.payload_states
            .lock()
            .expect("payload state lock")
            .push(payload_state_name(action));
        match self.mode.as_str() {
            "provider-http-403" => Err(AlertWritePortError::Provider {
                status: Some(403),
                message: "provider denied alert write".to_owned(),
            }),
            "provider-unavailable" => Err(AlertWritePortError::Provider {
                status: None,
                message: "provider unavailable".to_owned(),
            }),
            "internal-failure" => Err(AlertWritePortError::Internal("write failed".to_owned())),
            "rate-limit" => Err(AlertWritePortError::RateLimited {
                retry_after: 3,
                message: "broker snapshot rate limited; retry after 2.5s".to_owned(),
            }),
            "context-error" => Err(AlertWritePortError::Internal(
                match self.context.as_str() {
                    "deadline" => "context deadline exceeded",
                    _ => "context canceled",
                }
                .to_owned(),
            )),
            "failure-then-success" => {
                if *self.apply_calls.lock().expect("apply call lock") == 1 {
                    return Err(AlertWritePortError::Internal("write failed".to_owned()));
                }
                Ok(Some(json!({
                    "entries": [{
                        "accepted": true,
                        "featureId": action.feature_id,
                        "operation": action.action,
                    }]
                })))
            }
            "nil-result" => Ok(None),
            "empty-result" => Ok(Some(json!({}))),
            _ => Ok(Some(json!({
                "entries": [{
                    "accepted": true,
                    "featureId": action.feature_id,
                    "operation": action.action,
                }]
            }))),
        }
    }
}

fn action_json(action: &AlertWriteAction) -> Value {
    let mut result = json!({
        "featureId": action.feature_id,
        "brokerId": action.broker_id,
        "action": action.action,
    });
    if let Some(account_id) = action.account_id.as_deref() {
        result["accountId"] = json!(account_id);
    }
    match action.payload.as_ref() {
        Some(Value::Object(payload)) if !payload.is_empty() => {
            result["payload"] = Value::Object(payload.clone());
        }
        _ => {}
    }
    result
}

fn payload_state_name(action: &AlertWriteAction) -> &'static str {
    match action.payload_state {
        product_alerts_write_port::AlertWritePayloadState::Nil => "nil",
        product_alerts_write_port::AlertWritePayloadState::EmptyObject => "empty_object",
        product_alerts_write_port::AlertWritePayloadState::Object => "object",
    }
}
