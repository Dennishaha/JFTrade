#[path = "../src/product_plugins_write_port.rs"]
mod product_plugins_write_port;

use std::collections::BTreeMap;
use std::sync::{Arc, Mutex};
use std::thread;

use product_plugins_write_port::{
    PluginWriteOperation, PluginWritePort, PluginWritePortError, PluginWriteRequest,
    dispatch_plugin_write,
};
use serde_json::{Value, json};

const FIXTURE_TIMESTAMP: &str = "2026-08-22T00:00:00Z";

#[test]
fn plugins_write_routes_match_go_fixture_in_cutover_only() {
    let fixture: Value = serde_json::from_str(include_str!(
        "../../../tests/fixtures/compatibility/api-transport/plugins-write.json"
    ))
    .expect("plugins-write fixture");
    assert_eq!(fixture["version"], "stage9.plugins-write.v1");
    let cases = fixture["cases"].as_array().expect("plugins-write cases");
    assert_eq!(cases.len(), 12);

    for case in cases {
        let port = FixturePort::from_case(case);
        let paths = case["requestPaths"].as_array().expect("request paths");
        let bodies = case["requestBodies"].as_array().expect("request bodies");
        let expected_statuses = case["expectedStatuses"]
            .as_array()
            .expect("expected statuses");
        let expected_responses = case["responses"].as_array().expect("responses");
        assert_eq!(paths.len(), bodies.len(), "case {case:?}");
        assert_eq!(paths.len(), expected_statuses.len(), "case {case:?}");
        assert_eq!(paths.len(), expected_responses.len(), "case {case:?}");

        let mut actual = Vec::with_capacity(paths.len());
        for index in 0..paths.len() {
            let request = PluginWriteRequest {
                method: case["method"].as_str().expect("method").to_owned(),
                path: paths[index].as_str().expect("request path").to_owned(),
                body: Some(
                    bodies[index]
                        .as_str()
                        .expect("request body")
                        .as_bytes()
                        .to_vec(),
                ),
            };
            let response = dispatch_plugin_write(&request, Some(&port), FIXTURE_TIMESTAMP);
            assert_eq!(
                response.status,
                expected_statuses[index].as_u64().unwrap() as u16
            );
            assert_eq!(response.headers, fixture_header_expectations());
            assert_eq!(response.body["timestamp"], FIXTURE_TIMESTAMP);
            let mut body = response.body;
            body.as_object_mut()
                .expect("response object")
                .remove("timestamp");
            actual.push(body);
        }

        assert_eq!(actual, *expected_responses, "case {case:?}");
        assert_eq!(port.observation(), case["expectedObservation"]);
    }
}

#[test]
fn plugins_write_routes_fail_closed_without_port_and_preserve_route_isolation() {
    let request = PluginWriteRequest {
        method: "POST".to_owned(),
        path: "/api/v1/plugins/alpha/install".to_owned(),
        body: Some(Vec::new()),
    };
    let response = dispatch_plugin_write(&request, None, FIXTURE_TIMESTAMP);
    assert_eq!(response.status, 503);
    assert_eq!(response.body["error"]["code"], "PLUGINS_UNAVAILABLE");

    let port = FixturePort::success();
    for request in [
        PluginWriteRequest {
            method: "GET".to_owned(),
            path: "/api/v1/plugins/alpha/install".to_owned(),
            body: Some(Vec::new()),
        },
        PluginWriteRequest {
            method: "POST".to_owned(),
            path: "/api/v1/plugins/alpha/install/extra".to_owned(),
            body: Some(Vec::new()),
        },
        PluginWriteRequest {
            method: "POST".to_owned(),
            path: "/api/v1/plugins//install".to_owned(),
            body: Some(Vec::new()),
        },
    ] {
        let response = dispatch_plugin_write(&request, Some(&port), FIXTURE_TIMESTAMP);
        assert_eq!(response.status, 404);
    }
    assert_eq!(port.mutation_count(), 0);
}

#[test]
fn plugins_write_routes_preserve_blank_encoded_id_and_port_error_mapping() {
    let port = FixturePort::success();
    let blank = PluginWriteRequest {
        method: "POST".to_owned(),
        path: "/api/v1/plugins/%20/uninstall".to_owned(),
        body: Some(br#"not-json"#.to_vec()),
    };
    let response = dispatch_plugin_write(&blank, Some(&port), FIXTURE_TIMESTAMP);
    assert_eq!(response.status, 400);
    assert_eq!(response.body["error"]["code"], "BAD_REQUEST");
    assert_eq!(response.body["error"]["message"], "pluginId is invalid");
    assert_eq!(port.mutation_count(), 0);

    for (error, status, code, message, path) in [
        (
            PluginWritePortError::NotFound("plugin not found".to_owned()),
            404,
            "NOT_FOUND",
            "plugin not found",
            "/api/v1/plugins/alpha/install",
        ),
        (
            PluginWritePortError::Internal("save failed".to_owned()),
            500,
            "INTERNAL_ERROR",
            "plugin install failed",
            "/api/v1/plugins/alpha/install",
        ),
        (
            PluginWritePortError::Unavailable("fixture unavailable".to_owned()),
            503,
            "PLUGINS_UNAVAILABLE",
            "fixture unavailable",
            "/api/v1/plugins/alpha/install",
        ),
        (
            PluginWritePortError::Internal("save failed".to_owned()),
            500,
            "INTERNAL_ERROR",
            "plugin uninstall failed",
            "/api/v1/plugins/alpha/uninstall",
        ),
    ] {
        let port = ErrorPort { error };
        let request = PluginWriteRequest {
            method: "POST".to_owned(),
            path: path.to_owned(),
            body: Some(Vec::new()),
        };
        let response = dispatch_plugin_write(&request, Some(&port), FIXTURE_TIMESTAMP);
        assert_eq!(response.status, status);
        assert_eq!(response.body["error"]["code"], code);
        assert_eq!(response.body["error"]["message"], message);
    }
}

#[derive(Debug)]
struct ErrorPort {
    error: PluginWritePortError,
}

impl PluginWritePort for ErrorPort {
    fn mutate(
        &self,
        _operation: PluginWriteOperation,
        _plugin_id: &str,
    ) -> Result<Value, PluginWritePortError> {
        Err(self.error.clone())
    }
}

#[derive(Debug)]
struct FixturePort {
    case_name: String,
    mode: FixtureMode,
    state: Arc<Mutex<FixtureState>>,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
enum FixtureMode {
    Success,
    PersistFailure,
    Internal,
}

#[derive(Debug)]
struct FixtureState {
    durable_plugin_present: bool,
    durable_installed: bool,
    durable_operation_count: usize,
    memory_plugin_present: bool,
    memory_installed: bool,
    memory_operation_count: usize,
    save_count: usize,
}

impl FixtureState {
    fn new(plugin_present: bool, installed: bool) -> Self {
        Self {
            durable_plugin_present: plugin_present,
            durable_installed: installed,
            durable_operation_count: 0,
            memory_plugin_present: plugin_present,
            memory_installed: installed,
            memory_operation_count: 0,
            save_count: 0,
        }
    }

    fn status(plugin_present: bool, installed: bool) -> &'static str {
        if !plugin_present {
            ""
        } else if installed {
            "INSTALLED"
        } else {
            "NOT_INSTALLED"
        }
    }
}

impl FixturePort {
    fn success() -> Self {
        Self::new("test-success", FixtureMode::Success, true, false)
    }

    fn new(case_name: &str, mode: FixtureMode, plugin_present: bool, installed: bool) -> Self {
        Self {
            case_name: case_name.to_owned(),
            mode,
            state: Arc::new(Mutex::new(FixtureState::new(plugin_present, installed))),
        }
    }

    fn from_case(case: &Value) -> Self {
        let mode = if case["persistFailure"].as_bool().unwrap_or(false) {
            FixtureMode::PersistFailure
        } else if case["name"]
            .as_str()
            .unwrap_or_default()
            .starts_with("missing")
        {
            FixtureMode::Internal
        } else {
            FixtureMode::Success
        };
        let plugin_present = case["initialPluginPresent"].as_bool().unwrap_or(false);
        let installed = plugin_present && case["initialInstalled"].as_bool().unwrap_or(false);
        Self::new(
            case["name"].as_str().expect("case name"),
            mode,
            plugin_present,
            installed,
        )
    }

    fn mutation_count(&self) -> usize {
        self.state
            .lock()
            .expect("fixture state lock")
            .memory_operation_count
    }

    fn observation(&self) -> Value {
        let state = self.state.lock().expect("fixture state lock");
        json!({
            "durablePluginPresent": state.durable_plugin_present,
            "durableInstalled": state.durable_installed,
            "durableStatus": FixtureState::status(
                state.durable_plugin_present,
                state.durable_installed,
            ),
            "durableOperationCount": state.durable_operation_count,
            "memoryPluginPresent": state.memory_plugin_present,
            "memoryInstalled": state.memory_installed,
            "memoryStatus": FixtureState::status(
                state.memory_plugin_present,
                state.memory_installed,
            ),
            "saveCount": state.save_count,
            "resourceEvents": [],
        })
    }
}

impl PluginWritePort for FixturePort {
    fn mutate(
        &self,
        operation: PluginWriteOperation,
        plugin_id: &str,
    ) -> Result<Value, PluginWritePortError> {
        let mut state = self.state.lock().expect("fixture state lock");
        if self.mode == FixtureMode::Internal {
            return Err(PluginWritePortError::Internal(
                "strategy resource not found".to_owned(),
            ));
        }
        let installed = matches!(operation, PluginWriteOperation::Install);
        state.memory_installed = installed;
        state.memory_operation_count += 1;
        state.save_count += 1;
        let operation_number = state.memory_operation_count;
        if self.mode == FixtureMode::PersistFailure {
            return Err(PluginWritePortError::Internal(
                "catalog repository unavailable".to_owned(),
            ));
        }
        state.durable_installed = state.memory_installed;
        state.durable_operation_count = state.memory_operation_count;
        let phase = if installed {
            "installed"
        } else {
            "uninstalled"
        };
        let message = if installed {
            "plugin metadata installed"
        } else {
            "plugin metadata uninstalled"
        };
        Ok(json!({
            "operationId": format!(
                "{}-{}",
                self.case_name,
                operation_number
            ),
            "pluginId": plugin_id,
            "status": "SUCCEEDED",
            "phase": phase,
            "progress": 100,
            "message": message,
            "targetDir": "plugins",
            "installPath": "plugins/alpha.so",
            "startedAt": FIXTURE_TIMESTAMP,
            "updatedAt": FIXTURE_TIMESTAMP,
            "completedAt": FIXTURE_TIMESTAMP,
            "error": null,
        }))
    }
}

#[test]
fn plugins_write_port_serializes_concurrent_mutations_without_resource_lifecycle() {
    let port = Arc::new(FixturePort::success());
    let mut workers = Vec::new();
    for _ in 0..8 {
        let port = Arc::clone(&port);
        workers.push(thread::spawn(move || {
            let request = PluginWriteRequest {
                method: "POST".to_owned(),
                path: "/api/v1/plugins/alpha/install".to_owned(),
                body: Some(Vec::new()),
            };
            dispatch_plugin_write(&request, Some(port.as_ref()), FIXTURE_TIMESTAMP)
        }));
    }
    for worker in workers {
        assert_eq!(worker.join().expect("mutation worker").status, 200);
    }
    assert_eq!(port.mutation_count(), 8);
    assert_eq!(port.observation()["resourceEvents"], json!([]));
}

#[allow(dead_code)]
fn fixture_header_expectations() -> BTreeMap<String, String> {
    BTreeMap::from([(
        "Content-Type".to_owned(),
        "application/json; charset=utf-8".to_owned(),
    )])
}
