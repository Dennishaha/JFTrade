#[path = "../src/product_alerts_write_port.rs"]
mod product_alerts_write_port;

use std::cell::{Cell, RefCell};

use product_alerts_write_port::{
    dispatch_alert_write, AlertWriteAction, AlertWritePort, AlertWritePortError, AlertWriteRequest,
    AlertWriteResolution, AlertWriteRoute,
};
use serde_json::{json, Value};

const FIXTURE_TIMESTAMP: &str = "2026-08-22T04:00:00Z";

#[test]
fn alerts_write_routes_match_go_fixture_in_cutover_only() {
    let fixture: Value = serde_json::from_str(include_str!(
        "../../../tests/fixtures/rust-migration/stage9/alerts-write.json"
    ))
    .expect("alerts-write fixture");
    assert_eq!(fixture["version"], "stage9.alerts-write.v1");
    let cases = fixture["cases"].as_array().expect("alerts-write cases");
    assert_eq!(cases.len(), 14);

    for case in cases {
        let mode = case["portMode"].as_str().expect("port mode");
        let port = FixturePort::new(mode);
        let request = AlertWriteRequest {
            method: case["method"].as_str().expect("method").to_owned(),
            path: case["requestPath"]
                .as_str()
                .expect("request path")
                .to_owned(),
            body: case
                .get("body")
                .and_then(Value::as_str)
                .map(|body| body.as_bytes().to_vec()),
        };
        let response = dispatch_alert_write(&request, Some(&port), FIXTURE_TIMESTAMP);
        let expected_status = case["expected"]["status"]
            .as_u64()
            .expect("expected status") as u16;
        assert_eq!(response.status, expected_status, "case {case:?}");
        assert_eq!(response.body, case["expected"]["envelope"], "case {case:?}");
        assert_eq!(
            serde_json::to_value(&response.headers).expect("headers JSON"),
            case["expected"]["headers"],
            "case {case:?}"
        );

        let mut calls = json!({"apply": port.apply_calls.get()});
        if let Some(action) = port.last_action.borrow().as_ref() {
            calls["action"] = action_json(action);
            calls["payloadState"] = json!(payload_state_name(action));
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
    let port = FixturePort::new("unavailable");
    let response = dispatch_alert_write(&request, Some(&port), FIXTURE_TIMESTAMP);
    assert_eq!(response.status, 503);
    assert_eq!(response.body["error"]["code"], "ALERTS_UNAVAILABLE");
    assert_eq!(port.apply_calls.get(), 0);
}

#[test]
fn alerts_write_leaf_only_accepts_exact_post_paths() {
    let port = FixturePort::new("success");
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
    assert_eq!(port.apply_calls.get(), 0);
}

#[derive(Debug)]
struct FixturePort {
    mode: String,
    apply_calls: Cell<usize>,
    last_action: RefCell<Option<AlertWriteAction>>,
}

impl FixturePort {
    fn new(mode: &str) -> Self {
        Self {
            mode: mode.to_owned(),
            apply_calls: Cell::new(0),
            last_action: RefCell::new(None),
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
            "capability-unavailable" => Err(AlertWritePortError::CapabilityUnavailable(
                format!(
                    "broker feature capability is unavailable: broker \"futu\" feature \"{feature_id}\" is unavailable: "
                ),
            )),
            "adapter-unavailable" => Err(AlertWritePortError::CapabilityUnavailable(
                format!(
                    "broker feature capability is unavailable: broker \"futu\" feature \"{feature_id}\" is unavailable: adapter interface CustomizationService is not implemented"
                ),
            )),
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
        self.apply_calls.set(self.apply_calls.get() + 1);
        *self.last_action.borrow_mut() = Some(action.clone());
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
