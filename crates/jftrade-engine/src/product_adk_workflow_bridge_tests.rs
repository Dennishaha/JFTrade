//! Fail-closed coverage for the workflow bridge read/mutation ports.
//!
//! Owner: `internal/assistant/assembly/workflow_bridge_contracts_test.go` on
//! the `go` branch. Go's `WorkflowToolManager` answers `unavailable` for every
//! operation when its service is missing, so no console reader can mistake an
//! unwired runtime for an empty page or a synthetic accepted run. Rust keeps
//! the same boundary per port: the read routes require the ADK read snapshot
//! port and the mutation routes require the ADK mutation port.

use std::collections::BTreeMap;

use serde_json::{Value, json};

use crate::product::dispatch_adk_read;
use crate::product::product_adk_mutation_port::{AdkMutationRequest, dispatch_adk_mutation};

/// Parity: go:452dea11:internal/assistant/assembly/workflow_bridge_contracts_test.go:103
/// `TestWorkflowManagerRejectsUnavailableServicesAcrossOperations`: Go closes
/// list/get/save/delete for workflows and triggers, list/get for runs and both
/// start operations, and the nil-service/closed-facade providers fail the same
/// way.
///
/// Rust splits that manager across two ports, so the equivalent assertion is
/// that every workflow route fails closed on the port it belongs to: the two
/// workflow read routes answer `503 ADK_READ_UNAVAILABLE` without a snapshot
/// port, and all eight workflow mutation routes answer
/// `503 ADK_MUTATIONS_UNAVAILABLE` without a mutation port. Go's closed facade
/// (`assistant.NewService(nil)`) is the runtime-less port whose workflow run
/// fails closed with `ADK_WORKFLOW_RUNTIME_UNAVAILABLE`, covered by
/// `workflow_run_without_a_model_runtime_fails_closed_and_finalises_the_invocation`.
#[test]
fn workflow_bridge_operations_fail_closed_without_their_ports() {
    for path in [
        "/api/v1/adk/workflows",
        "/api/v1/adk/workflows/workflow-1",
        "/api/v1/adk/workflows/workflow-1/triggers",
        "/api/v1/adk/workflow-trigger-logs",
    ] {
        let failure = dispatch_adk_read(None, "GET", path, "")
            .expect_err("a workflow read without a snapshot port must fail closed");
        assert_eq!(failure.status, 503, "{path}");
        assert_eq!(failure.code, "ADK_READ_UNAVAILABLE", "{path}");
        assert_eq!(
            failure.message, "ADK read snapshot port is not configured",
            "{path}"
        );
    }

    let routes: [(&str, &str, Value); 8] = [
        ("POST", "/api/v1/adk/workflows", json!({"name": "Bridge"})),
        (
            "PUT",
            "/api/v1/adk/workflows/workflow-1",
            json!({"name": "Bridge"}),
        ),
        ("DELETE", "/api/v1/adk/workflows/workflow-1", json!({})),
        (
            "POST",
            "/api/v1/adk/workflows/workflow-1/triggers",
            json!({"type": "manual", "title": "Bridge"}),
        ),
        (
            "PUT",
            "/api/v1/adk/workflows/workflow-1/triggers/trigger-1",
            json!({"title": "Bridge"}),
        ),
        (
            "DELETE",
            "/api/v1/adk/workflows/workflow-1/triggers/trigger-1",
            json!({}),
        ),
        (
            "POST",
            "/api/v1/adk/workflows/workflow-1/run",
            json!({"inputs": {"symbol": "US.AAPL"}}),
        ),
        (
            "POST",
            "/api/v1/adk/workflow-triggers/trigger-1/run",
            json!({}),
        ),
    ];
    for (method, path, body) in routes {
        let request = AdkMutationRequest {
            method: method.to_owned(),
            path: path.to_owned(),
            body: Some(body.to_string().into_bytes()),
            headers: BTreeMap::new(),
        };
        let response = dispatch_adk_mutation(&request, None, "2026-01-01T00:00:00Z");
        assert_eq!(
            response.status, 503,
            "{method} {path} must fail closed: {response:?}"
        );
        assert_eq!(
            response.body["error"]["code"], "ADK_MUTATIONS_UNAVAILABLE",
            "{method} {path}: {response:?}"
        );
        assert_eq!(
            response.body["error"]["message"], "ADK mutation port is unavailable",
            "{method} {path}: {response:?}"
        );
    }
}
