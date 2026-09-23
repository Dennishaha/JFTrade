//! Regression coverage for Go's `Runtime.runSem` admission gate and for the
//! provider default-selection repair that `StoreCore.ListProviders` performs.
//!
//! Both behaviors were missing in the Rust port: the port had no concurrency
//! limit at all, and provider resolution required an explicit `default: true`
//! row, so a table written without the flag failed chat with
//! "default agent provider is not configured" instead of using the first
//! provider like Go's `DefaultProvider` does.

use std::fs::File;
use std::sync::Arc;

use rusqlite::Connection;
use serde_json::{Value, json};
use tempfile::tempdir;

use jftrade_store_sqlite::{AdkSessionStore, AdkStore, initialize_current};

use super::{
    MAX_CONCURRENT_RUNS, ProductionAdkChatRuntime, RunCancellationRegistry, RunGate, chat_failed,
};
use crate::product::product_adk_chat_stream_port::{
    AdkChatInput, AdkChatPortError, AdkChatRoute, AdkChatStreamPort,
};

fn initialized_stores() -> (tempfile::TempDir, Arc<AdkStore>, Arc<AdkSessionStore>) {
    let directory = tempdir().expect("temporary directory");
    let adk_path = directory.path().join("adk.db");
    let session_path = directory.path().join("adk-session.db");
    File::create(&adk_path).expect("create ADK database");
    File::create(&session_path).expect("create ADK session database");
    initialize_current(
        &Connection::open(&adk_path).expect("initialize ADK database"),
        "adk",
    )
    .expect("initialize ADK schema");
    initialize_current(
        &Connection::open(&session_path).expect("initialize ADK session database"),
        "adk-session",
    )
    .expect("initialize ADK session schema");
    (
        directory,
        Arc::new(AdkStore::open(&adk_path).expect("open ADK store")),
        Arc::new(AdkSessionStore::open(&session_path).expect("open session store")),
    )
}

/// Go's `prepareChatRequest` rejects the request after the limit rather than
/// queueing it, and reports the same wording as the reference implementation:
/// `maximum concurrent runs (10) reached, please try again later`.
#[test]
fn run_gate_rejects_the_eleventh_concurrent_run_and_releases_on_drop() {
    assert_eq!(MAX_CONCURRENT_RUNS, 10);

    let gate = Arc::new(RunGate::default());
    assert_eq!(gate.active(), 0);
    let mut held = Vec::new();
    for index in 0..MAX_CONCURRENT_RUNS {
        held.push(
            gate.try_acquire()
                .unwrap_or_else(|error| panic!("slot {index} must be admitted: {error:?}")),
        );
        assert_eq!(gate.active(), index + 1);
    }

    let rejected = gate
        .try_acquire()
        .expect_err("the run after the limit must fail closed");
    match rejected {
        super::AdkChatPortError::Failed {
            status,
            ref code,
            ref message,
        } => {
            assert_eq!(status, 400);
            assert_eq!(code, "ADK_CHAT_FAILED");
            assert_eq!(
                message,
                "maximum concurrent runs (10) reached, please try again later"
            );
        }
        other => panic!("expected a failed chat port error, got {other:?}"),
    }
    assert_eq!(gate.active(), MAX_CONCURRENT_RUNS);

    // Each finished run returns its slot, so a later request is admitted again.
    let released = held.pop().expect("a held slot");
    drop(released);
    assert_eq!(gate.active(), MAX_CONCURRENT_RUNS - 1);
    let readmitted = gate.try_acquire().expect("slot released by a finished run");
    assert_eq!(gate.active(), MAX_CONCURRENT_RUNS);
    drop(readmitted);
    drop(held);
    assert_eq!(gate.active(), 0);
}

/// A drained gate admits a fresh burst; Go reuses one `runSem` for the whole
/// process, so the limiter is shared rather than per request.
#[test]
fn run_gate_is_shared_across_runtime_facades() {
    let (_directory, store, session_store) = initialized_stores();
    let settings_path = _directory.path().join("settings.json");
    std::fs::write(&settings_path, "{}").expect("write settings");
    let runtime = ProductionAdkChatRuntime::new(
        Arc::clone(&store),
        session_store,
        &settings_path,
        Arc::new(RunCancellationRegistry::default()),
        Arc::new(crate::product::product_production_ports::ProductionToolCatalog::empty_for_test()),
    );
    let facade = runtime.as_ref();
    let slots = (0..MAX_CONCURRENT_RUNS)
        .map(|_| {
            runtime
                .run_gate
                .try_acquire()
                .expect("runtime gate admits up to the limit")
        })
        .collect::<Vec<_>>();
    assert_eq!(runtime.run_gate.active(), MAX_CONCURRENT_RUNS);
    assert_eq!(facade.run_gate.active(), MAX_CONCURRENT_RUNS);
    let error = runtime
        .run_gate
        .try_acquire()
        .expect_err("shared gate stays closed for every facade");
    assert_eq!(
        error,
        chat_failed("maximum concurrent runs (10) reached, please try again later")
    );
    drop(slots);
    assert_eq!(runtime.run_gate.active(), 0);
}

/// Go's `SaveProvider` marks the first stored provider as default, and
/// `NormalizeDefaultProviderSelection` repairs a table that lost the flag.
#[test]
fn provider_list_reports_the_repaired_default_selection() {
    let (_directory, store, _session_store) = initialized_stores();
    store
        .upsert_provider(
            "provider-first",
            &json!({
                "displayName": "First",
                "baseUrl": "https://first.example/v1",
                "model": "first-model",
                "enabled": true,
            })
            .to_string(),
        )
        .expect("persist first provider");
    store
        .upsert_provider(
            "provider-second",
            &json!({
                "displayName": "Second",
                "baseUrl": "https://second.example/v1",
                "model": "second-model",
                "enabled": true,
            })
            .to_string(),
        )
        .expect("persist second provider");

    // Simulate a legacy/corrupt table whose rows carry no `default` key at all.
    let connection = Connection::open(_directory.path().join("adk.db")).expect("open ADK database");
    connection
        .execute(
            "UPDATE adk_providers SET payload_json = json_remove(payload_json, '$.default')",
            [],
        )
        .expect("strip default flags");

    let listed = store.list_providers().expect("list providers");
    let ids = listed.iter().map(|row| row.id.as_str()).collect::<Vec<_>>();
    assert_eq!(
        ids,
        vec!["provider-first", "provider-second"],
        "Go lists created_at ASC with the repaired default first"
    );
    let defaults = listed
        .iter()
        .map(|row| {
            let value: Value = serde_json::from_str(&row.payload_json).expect("provider payload");
            (
                row.id.clone(),
                value
                    .get("default")
                    .and_then(Value::as_bool)
                    .unwrap_or(false),
            )
        })
        .collect::<Vec<_>>();
    assert_eq!(
        defaults,
        vec![
            ("provider-first".to_owned(), true),
            ("provider-second".to_owned(), false)
        ],
        "exactly one provider is repaired as the default"
    );

    // The repair is persisted, so a second read is stable and the surviving
    // `default` row is the one provider resolution selects.
    let again = store.list_providers().expect("list providers again");
    assert_eq!(again[0].id, "provider-first");
    let payload: Value = serde_json::from_str(&again[0].payload_json).expect("provider payload");
    assert_eq!(payload["default"], json!(true));
}

/// Go resolves an agent whose `providerId` is empty through
/// `Runtime.effectiveProvider(ctx, "")` -> `StoreCore.DefaultProvider`, so the
/// provider used by a run follows the *current* default selection rather than
/// anything frozen on the agent.
#[test]
fn resolve_provider_follows_the_default_selection_and_its_repair() {
    let (directory, store, session_store) = initialized_stores();
    let secrets = directory.path().join("secrets");
    std::fs::create_dir_all(&secrets).expect("create secrets directory");
    std::fs::write(
        secrets.join("adk-secrets.json"),
        r#"{"provider-first":"sk-first","provider-second":"sk-second"}"#,
    )
    .expect("write provider secrets");
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, "{}").expect("write settings");

    store
        .upsert_provider(
            "provider-first",
            &json!({
                "displayName": "First",
                "baseUrl": "https://first.example/v1",
                "model": "first-model",
                "enabled": true,
            })
            .to_string(),
        )
        .expect("persist first provider");
    store
        .upsert_provider(
            "provider-second",
            &json!({
                "displayName": "Second",
                "baseUrl": "https://second.example/v1",
                "model": "second-model",
                "enabled": true,
            })
            .to_string(),
        )
        .expect("persist second provider");
    store
        .upsert_agent(
            "agent-default-provider",
            &json!({
                "id": "agent-default-provider",
                "name": "Dynamic Default Provider",
                "providerId": "",
                "status": "ENABLED",
            })
            .to_string(),
        )
        .expect("persist agent without a provider");

    let runtime = ProductionAdkChatRuntime::new(
        Arc::clone(&store),
        session_store,
        &settings_path,
        Arc::new(RunCancellationRegistry::default()),
        Arc::new(crate::product::product_production_ports::ProductionToolCatalog::empty_for_test()),
    );

    // Even with no persisted `default` flag, Go's repaired selection resolves
    // the first provider instead of failing with
    // "default agent provider is not configured".
    let mut request = serde_json::Map::new();
    request.insert("agentId".to_owned(), json!("agent-default-provider"));
    let resolved = runtime
        .resolve_provider(&request)
        .expect("agent without provider falls back to the default provider");
    assert_eq!(resolved.id, "provider-first");
    assert_eq!(resolved.agent_id, "agent-default-provider");
    assert_eq!(resolved.model, "first-model");

    // Switching the default changes what the same agent resolves to, matching
    // Go's `TestAgentWithoutProviderDynamicallyUsesDefaultProvider`.
    let listed = store.list_providers().expect("list providers");
    let mut updates = Vec::new();
    for row in &listed {
        let mut payload: Value = serde_json::from_str(&row.payload_json).expect("provider payload");
        payload["default"] = json!(row.id == "provider-second");
        updates.push((row.id.clone(), payload.to_string()));
    }
    let update_refs = updates
        .iter()
        .map(|(id, payload)| (id.as_str(), payload.as_str()))
        .collect::<Vec<_>>();
    store
        .set_default_provider_atomic("provider-second", &update_refs)
        .expect("switch the default provider");

    let switched = runtime
        .resolve_provider(&request)
        .expect("switched default resolves for the same agent");
    assert_eq!(switched.id, "provider-second");
    assert_eq!(switched.model, "second-model");
}

/// Parity: go:452dea11:internal/assistant/engine/runner_chat_test.go:541
/// `TestRunChatRejectsInvalidPermissionModeOverride` plus the override table in
/// `internal/assistant/model/runner_state.go:78` `ValidateChatOverrides`.
///
/// Go validates the per-request overrides before any agent or provider lookup:
/// an unknown `permissionModeOverride`, `workModeOverride` or
/// `reasoningEffortOverride` is rejected with `invalid ... mode %q` /
/// `invalid reasoning effort %q`, and a blank override means "absent".  The Rust
/// port accepted every override string silently, so a request that asked for
/// `"root"` ran with the agent's own mode instead of failing closed.
#[test]
fn chat_rejects_invalid_permission_work_mode_and_reasoning_overrides() {
    let (directory, store, session_store) = initialized_stores();
    let secrets = directory.path().join("secrets");
    std::fs::create_dir_all(&secrets).expect("create secrets directory");
    std::fs::write(
        secrets.join("adk-secrets.json"),
        r#"{"provider-override":"sk-override"}"#,
    )
    .expect("write provider secrets");
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, "{}").expect("write settings");

    store
        .upsert_provider(
            "provider-override",
            &json!({
                "id": "provider-override",
                "displayName": "Override Provider",
                "baseUrl": "https://override.example/v1",
                "model": "override-model",
                "enabled": true,
            })
            .to_string(),
        )
        .expect("persist provider");
    store
        .upsert_agent(
            "agent-override",
            &json!({
                "id": "agent-override",
                "name": "Override Agent",
                "providerId": "provider-override",
                "permissionMode": "approval",
                "status": "ENABLED",
            })
            .to_string(),
        )
        .expect("persist agent");

    let runtime = ProductionAdkChatRuntime::new(
        Arc::clone(&store),
        session_store,
        &settings_path,
        Arc::new(RunCancellationRegistry::default()),
        Arc::new(crate::product::product_production_ports::ProductionToolCatalog::empty_for_test()),
    );

    // Every rejected override reports the reference wording and the
    // `400 ADK_CHAT_FAILED` classification Go's handler uses.  The assertions
    // drive the real chat entry point so the validation order is exercised too:
    // Go rejects the override *before* resolving the agent, so the same error
    // comes back even for an agent id that does not exist.
    // Each probe needs its own identity: a repeated `clientRequestId` with a
    // changed fingerprint is the idempotency conflict the reference also
    // reports, which would mask the override validation under test.
    let mut sequence = 0_u32;
    let mut dispatch = |agent_id: &str, field: &str, value: &str| {
        sequence += 1;
        let request_id = format!("888888{:02}-8888-4888-8888-888888888888", sequence + 10);
        let body = serde_json::json!({
            "clientRequestId": request_id,
            "agentId": agent_id,
            "message": "override probe",
            field: value,
        })
        .to_string();
        runtime.dispatch(
            AdkChatRoute::Chat,
            &AdkChatInput {
                body: body.into_bytes(),
                client_request_id: request_id,
            },
        )
    };

    for (field, value, expected) in [
        (
            "permissionModeOverride",
            "root",
            "invalid permission mode \"root\"",
        ),
        (
            "workModeOverride",
            "workflow",
            "invalid work mode \"workflow\"",
        ),
        (
            "reasoningEffortOverride",
            "turbo",
            "invalid reasoning effort \"turbo\"",
        ),
        (
            "reasoningEffortOverride",
            "default",
            "invalid reasoning effort \"default\"",
        ),
    ] {
        for agent_id in ["agent-override", "agent-that-does-not-exist"] {
            match dispatch(agent_id, field, value)
                .expect_err("an unknown override must fail closed")
            {
                AdkChatPortError::Failed {
                    status,
                    code,
                    message,
                } => {
                    assert_eq!(status, 400, "{field} status for {agent_id}");
                    assert_eq!(code, "ADK_CHAT_FAILED", "{field} code for {agent_id}");
                    assert_eq!(message, expected, "{field} message for {agent_id}");
                }
                other => panic!("{field} must classify as a chat failure: {other:?}"),
            }
        }
    }

    // A valid override wins over the agent's own mode on the run snapshot, and
    // the provider display name is captured alongside it.  The model endpoint is
    // a closed loopback port, so the run reaches the provider boundary and fails
    // after the snapshot is written.
    let override_run_id = "88888888-8888-4888-8888-888888888888";
    let override_body = serde_json::json!({
        "clientRequestId": override_run_id,
        "agentId": "agent-override",
        "message": "override probe",
        "permissionModeOverride": "less_approval",
    })
    .to_string();
    let _ = runtime.dispatch(
        AdkChatRoute::Chat,
        &AdkChatInput {
            body: override_body.into_bytes(),
            client_request_id: override_run_id.to_owned(),
        },
    );
    let snapshot: Value = serde_json::from_str(
        &store
            .get_run(&format!("run-{override_run_id}"))
            .expect("read override run")
            .expect("override run row")
            .payload_json,
    )
    .expect("decode override run payload");
    assert_eq!(
        snapshot["permissionMode"], "less_approval",
        "the validated override must win over the agent's own mode: {snapshot}"
    );
    assert_eq!(
        snapshot["providerName"], "Override Provider",
        "the snapshot must carry the resolved provider display name: {snapshot}"
    );

    // Without an override the agent's own normalized mode is kept.
    let plain_run_id = "88888889-8888-4888-8888-888888888888";
    let plain_body = serde_json::json!({
        "clientRequestId": plain_run_id,
        "agentId": "agent-override",
        "message": "override probe",
    })
    .to_string();
    let _ = runtime.dispatch(
        AdkChatRoute::Chat,
        &AdkChatInput {
            body: plain_body.into_bytes(),
            client_request_id: plain_run_id.to_owned(),
        },
    );
    let plain: Value = serde_json::from_str(
        &store
            .get_run(&format!("run-{plain_run_id}"))
            .expect("read plain run")
            .expect("plain run row")
            .payload_json,
    )
    .expect("decode plain run payload");
    assert_eq!(plain["permissionMode"], "approval");

    // Blank overrides are "absent" rather than invalid, exactly like Go's
    // trimmed comparisons, and every documented value is accepted.
    for accepted in [
        ("permissionModeOverride", "all"),
        ("permissionModeOverride", "  approval  "),
        ("workModeOverride", "loop"),
        ("workModeOverride", ""),
        ("reasoningEffortOverride", "xhigh"),
        ("reasoningEffortOverride", "MAX"),
    ] {
        if let Err(error) = dispatch("agent-override", accepted.0, accepted.1) {
            panic!("{accepted:?} must be accepted, got {error:?}");
        }
    }
}

/// Parity: go:452dea11:internal/assistant/engine/runner_chat_test.go:552
/// `TestRunStoresResolvedModelSnapshot`.
///
/// Go's `startRun` freezes the resolved provider/model/permission selection onto
/// the run row, so renaming the provider or changing its model afterwards does
/// not rewrite the history of an existing run.  The Rust run payload carried
/// `providerId` and `model` but omitted `providerName` and `permissionMode`, so
/// the console could not render the same snapshot the reference stores.
#[test]
fn run_snapshot_freezes_provider_name_model_and_permission_mode() {
    let (directory, store, session_store) = initialized_stores();
    let secrets = directory.path().join("secrets");
    std::fs::create_dir_all(&secrets).expect("create secrets directory");
    std::fs::write(
        secrets.join("adk-secrets.json"),
        r#"{"snapshot-provider":"sk-snapshot"}"#,
    )
    .expect("write provider secrets");
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, "{}").expect("write settings");

    store
        .upsert_provider(
            "snapshot-provider",
            &json!({
                "id": "snapshot-provider",
                "displayName": "Snapshot Provider",
                "baseUrl": "https://snapshot.example/v1",
                "model": "snapshot-model-v1",
                "enabled": true,
            })
            .to_string(),
        )
        .expect("persist provider");
    store
        .upsert_agent(
            "agent-model-snapshot",
            &json!({
                "id": "agent-model-snapshot",
                "name": "Snapshot Agent",
                "providerId": "snapshot-provider",
                "permissionMode": "approval",
                "status": "ENABLED",
            })
            .to_string(),
        )
        .expect("persist agent");

    let runtime = ProductionAdkChatRuntime::new(
        Arc::clone(&store),
        Arc::clone(&session_store),
        &settings_path,
        Arc::new(RunCancellationRegistry::default()),
        Arc::new(crate::product::product_production_ports::ProductionToolCatalog::empty_for_test()),
    );

    let request_id = "77777777-7777-4777-8777-777777777777";
    let body = format!(
        r#"{{"clientRequestId":"{request_id}","agentId":"agent-model-snapshot","message":"hello"}}"#
    );
    // The provider endpoint is never contacted: `prepare_chat` persists the run
    // snapshot before the model call, and the closed loopback port fails the
    // call afterwards, which leaves the frozen snapshot on the row.
    let _ = runtime.dispatch(
        AdkChatRoute::Chat,
        &AdkChatInput {
            body: body.into_bytes(),
            client_request_id: request_id.to_owned(),
        },
    );

    let run_id = format!("run-{request_id}");
    let snapshot = |store: &Arc<AdkStore>| -> Value {
        let run = store
            .get_run(&run_id)
            .expect("read run")
            .expect("run row exists");
        serde_json::from_str(&run.payload_json).expect("decode run payload")
    };

    let first = snapshot(&store);
    assert_eq!(first["model"], "snapshot-model-v1");
    assert_eq!(
        first["providerName"], "Snapshot Provider",
        "the run must freeze the resolved provider display name: {first}"
    );
    assert_eq!(
        first["permissionMode"], "approval",
        "the run must freeze the effective permission mode: {first}"
    );

    // Renaming the provider and changing its model must not rewrite the run.
    store
        .upsert_provider(
            "snapshot-provider",
            &json!({
                "id": "snapshot-provider",
                "displayName": "Snapshot Provider Renamed",
                "baseUrl": "https://snapshot.example/v1",
                "model": "snapshot-model-v2",
                "enabled": true,
            })
            .to_string(),
        )
        .expect("rename provider");
    let stored = snapshot(&store);
    assert_eq!(
        stored["model"], "snapshot-model-v1",
        "the stored model must stay the started snapshot: {stored}"
    );
    assert_eq!(
        stored["providerName"], "Snapshot Provider",
        "the stored provider name must stay the started snapshot: {stored}"
    );
}

/// Go's `resolveAgentDefinition` checks `status` before the soft-delete marker,
/// so a row that is both disabled and deleted reports "agent is disabled".
#[test]
fn agent_unavailable_reason_prefers_status_over_the_delete_marker() {
    assert_eq!(
        super::agent_unavailable_reason(&json!({"status": "ENABLED"})),
        "agent is disabled",
        "an enabled payload without a delete marker keeps the default wording"
    );
    assert_eq!(
        super::agent_unavailable_reason(&json!({"status": "DISABLED"})),
        "agent is disabled"
    );
    assert_eq!(
        super::agent_unavailable_reason(&json!({
            "status": "ENABLED",
            "deletedAt": "2026-09-19T00:00:00Z",
        })),
        "agent is deleted"
    );
    assert_eq!(
        super::agent_unavailable_reason(&json!({
            "status": "DISABLED",
            "deletedAt": "2026-09-19T00:00:00Z",
        })),
        "agent is disabled",
        "Go checks the status first, so the disabled wording wins"
    );
    assert_eq!(
        super::agent_unavailable_reason(&json!({"status": "DISABLED", "deletedAt": null})),
        "agent is disabled"
    );
}

/// Go's `FirstToolCallFailure` picks the first TIMED_OUT/FAILED/CANCELLED tool
/// call and falls back to a status-derived message when the call carries no
/// error text.  `RUNNING`/`PENDING_APPROVAL` calls are not failures.
#[test]
fn first_tool_call_failure_matches_the_go_selection_rules() {
    let failure = |payload: serde_json::Value| super::first_tool_call_failure(&payload.to_string());

    // A clean run reports no failure, which keeps `degraded` false.
    assert_eq!(failure(json!({"toolCalls": []})), None);
    assert_eq!(failure(json!({})), None);
    assert_eq!(
        failure(json!({"toolCalls": [
            {"id": "call-1", "status": "SUCCEEDED"},
            {"id": "call-2", "status": "RUNNING"},
            {"id": "call-3", "status": "PENDING_APPROVAL"},
        ]})),
        None,
        "only terminal tool failures mark a run degraded"
    );

    // The call's own error text wins, matching `ToolCallFailureMessage`.
    assert_eq!(
        failure(json!({"toolCalls": [
            {"id": "call-1", "status": "SUCCEEDED"},
            {"id": "call-2", "status": "FAILED", "error": "disk full"},
        ]})),
        Some("disk full".to_owned())
    );
    // The first failing call wins, in list order.
    assert_eq!(
        failure(json!({"toolCalls": [
            {"id": "call-1", "status": "TIMED_OUT", "error": "first"},
            {"id": "call-2", "status": "FAILED", "error": "second"},
        ]})),
        Some("first".to_owned())
    );
    // A blank/missing error falls back to the status-derived message.
    assert_eq!(
        failure(json!({"toolCalls": [{"id": "c", "status": "FAILED", "error": "   "}]})),
        Some("tool execution failed".to_owned())
    );
    assert_eq!(
        failure(json!({"toolCalls": [{"id": "c", "status": "TIMED_OUT"}]})),
        Some("tool execution timed out".to_owned())
    );
    assert_eq!(
        failure(json!({"toolCalls": [{"id": "c", "status": "CANCELLED"}]})),
        Some("tool execution cancelled".to_owned())
    );
    // Status matching is case-insensitive, like Go's ToUpper/TrimSpace.
    assert_eq!(
        failure(json!({"toolCalls": [{"id": "c", "status": " failed ", "error": "boom"}]})),
        Some("boom".to_owned())
    );
    assert_eq!(
        failure(json!({"toolCalls": [{"id": "c", "status": "failed"}]})),
        Some("tool execution failed".to_owned())
    );
}
/// Go's `Runtime.resolveSession` refuses to reuse an explicit session id that
/// belongs to another agent, and reports a provided-but-missing session id as
/// "session not found".  Both failures surface as `400 ADK_CHAT_FAILED`.
///
/// Reference: go:452dea11:internal/assistant/engine/store_ops_test.go
/// `TestResolveSessionRejectsDifferentAgent`.
#[test]
fn chat_rejects_a_session_owned_by_a_different_agent() {
    let (directory, store, session_store) = initialized_stores();
    let secrets_dir = directory.path().join("secrets");
    std::fs::create_dir_all(&secrets_dir).expect("create secrets directory");
    std::fs::write(
        secrets_dir.join("adk-secrets.json"),
        r#"{"provider-session":"sk-session"}"#,
    )
    .expect("write provider secrets");
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, "{}").expect("write settings");

    store
        .upsert_provider(
            "provider-session",
            &json!({
                "displayName": "Session fixture",
                "baseUrl": "http://127.0.0.1:9/v1",
                "model": "fixture-model",
                "enabled": true,
            })
            .to_string(),
        )
        .expect("persist provider");
    for agent_id in ["agent-a", "agent-b"] {
        store
            .upsert_agent(
                agent_id,
                &json!({
                    "id": agent_id,
                    "name": agent_id,
                    "providerId": "provider-session",
                    "status": "ENABLED",
                })
                .to_string(),
            )
            .expect("persist agent");
    }
    store
        .upsert_session(
            "session-owned-by-a",
            "agent-a",
            &json!({"id": "session-owned-by-a", "agentId": "agent-a", "title": "A"}).to_string(),
        )
        .expect("persist session for agent-a");

    let runtime = ProductionAdkChatRuntime::new(
        Arc::clone(&store),
        session_store,
        &settings_path,
        Arc::new(RunCancellationRegistry::default()),
        Arc::new(crate::product::product_production_ports::ProductionToolCatalog::empty_for_test()),
    );

    let request = |agent_id: &str, session_id: &str| AdkChatInput {
        body: json!({
            "agentId": agent_id,
            "sessionId": session_id,
            "message": "hello",
        })
        .to_string()
        .into_bytes(),
        client_request_id: format!("request-{agent_id}-{session_id}"),
    };

    // The owner may reuse the session; the mismatch fails before any model call.
    let mismatched = runtime
        .dispatch(
            AdkChatRoute::Chat,
            &request("agent-b", "session-owned-by-a"),
        )
        .expect_err("a session owned by another agent must be rejected");
    match mismatched {
        AdkChatPortError::Failed {
            status,
            ref code,
            ref message,
        } => {
            assert_eq!(status, 400);
            assert_eq!(code, "ADK_CHAT_FAILED");
            assert_eq!(message, "session belongs to a different agent");
        }
        other => panic!("expected 400 ADK_CHAT_FAILED, got {other:?}"),
    }

    // A provided session id that does not exist is "session not found" rather
    // than being created on the fly.
    let missing = runtime
        .dispatch(AdkChatRoute::Chat, &request("agent-a", "session-missing"))
        .expect_err("a provided missing session must be rejected");
    match missing {
        AdkChatPortError::Failed {
            status,
            code,
            ref message,
        } => {
            assert_eq!(status, 400);
            assert_eq!(code, "ADK_CHAT_FAILED");
            assert_eq!(message, "session not found");
        }
        other => panic!("expected 400 ADK_CHAT_FAILED, got {other:?}"),
    }

    // Neither rejection rewrote the stored session ownership.
    assert_eq!(
        store
            .get_session_agent_id("session-owned-by-a")
            .expect("read session owner"),
        Some("agent-a".to_owned())
    );
}
/// Go's `Runtime.prepareAgent` appends the durable `JFTrade memory:` block to
/// the instruction only when the agent opted in (`memoryEnabled`), combining
/// workspace rows with the agent's own rows and bounded to 4000 runes.
///
/// Reference: go:452dea11:internal/assistant/engine/store_ops_test.go
/// `TestPrepareAgentInjectsMemoryOnlyWhenEnabled`.
#[test]
fn chat_injects_memory_into_the_instruction_only_when_enabled() {
    let (directory, store, session_store) = initialized_stores();
    let secrets_dir = directory.path().join("secrets");
    std::fs::create_dir_all(&secrets_dir).expect("create secrets directory");
    std::fs::write(
        secrets_dir.join("adk-secrets.json"),
        r#"{"provider-memory":"sk-memory"}"#,
    )
    .expect("write provider secrets");
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, "{}").expect("write settings");

    store
        .upsert_provider(
            "provider-memory",
            &json!({
                "displayName": "Memory fixture",
                "baseUrl": "http://127.0.0.1:9/v1",
                "model": "fixture-model",
                "enabled": true,
            })
            .to_string(),
        )
        .expect("persist provider");
    for (agent_id, memory_enabled, instruction) in [
        ("agent-memory-off", false, "base"),
        ("agent-memory-on", true, "base"),
    ] {
        store
            .upsert_agent(
                agent_id,
                &json!({
                    "id": agent_id,
                    "name": agent_id,
                    "providerId": "provider-memory",
                    "status": "ENABLED",
                    "instruction": instruction,
                    "memoryEnabled": memory_enabled,
                })
                .to_string(),
            )
            .expect("persist agent");
    }
    store
        .upsert_memory(
            "memory-workspace",
            "",
            "workspace",
            "preference",
            &json!({
                "id": "memory-workspace",
                "scope": "workspace",
                "agentId": "",
                "key": "preference",
                "value": "use HK market",
            })
            .to_string(),
        )
        .expect("persist workspace memory");

    let runtime = ProductionAdkChatRuntime::new(
        Arc::clone(&store),
        session_store,
        &settings_path,
        Arc::new(RunCancellationRegistry::default()),
        Arc::new(crate::product::product_production_ports::ProductionToolCatalog::empty_for_test()),
    );

    let resolve = |agent_id: &str| {
        let mut request = serde_json::Map::new();
        request.insert("agentId".to_owned(), json!(agent_id));
        runtime
            .resolve_provider(&request)
            .unwrap_or_else(|error| panic!("resolve {agent_id}: {error:?}"))
    };

    let disabled = resolve("agent-memory-off");
    let instruction = disabled.instruction.expect("instruction");
    assert_eq!(
        instruction, "base",
        "disabled agents keep their instruction"
    );
    assert!(
        !instruction.contains("JFTrade memory"),
        "memory must not be injected when disabled: {instruction}"
    );

    let enabled = resolve("agent-memory-on");
    let instruction = enabled.instruction.expect("instruction");
    assert!(
        instruction.contains("JFTrade memory:"),
        "enabled agents must carry the memory header: {instruction}"
    );
    assert!(
        instruction.contains("use HK market"),
        "the workspace memory value must be injected: {instruction}"
    );
    assert!(
        instruction.starts_with("base"),
        "the original instruction must be preserved: {instruction}"
    );
}
/// Go's `ToolDescriptorsForAgent` scopes the model-visible tool list by the
/// resolved agent: an explicit `toolAccessMode` wins, otherwise a non-empty
/// `tools` list is an allowlist and an empty list exposes every registered
/// tool.  `strategy.research_backtest` / `strategy.optimize` also imply
/// `backtest.kline_sync_status`.
///
/// Reference: go:452dea11:internal/assistant/engine/tools.go
/// `TestToolsSearchReturnsOnlyCurrentAgentTools`.
#[test]
fn agent_tool_scope_follows_the_go_access_mode_normalization() {
    let scope = ProductionAdkChatRuntime::agent_tool_scope;

    let selected = scope(&json!({
        "tools": ["research.instrument"],
        "toolAccessMode": "selected",
    }));
    assert!(selected.exposes("research.instrument"));
    assert!(
        !selected.exposes("market.search"),
        "a selected agent must not see tools outside its allowlist"
    );
    // `interaction.request_user` is granted separately by the run admission
    // path and is not part of the agent allowlist.
    assert!(!selected.exposes("interaction.request_user"));
    // The implicit backtest companion only appears with its parent tools.
    assert!(!selected.exposes("backtest.kline_sync_status"));

    let none = scope(&json!({
        "tools": ["research.instrument"],
        "toolAccessMode": "none",
    }));
    assert!(
        !none.exposes("research.instrument"),
        "toolAccessMode=none hides even the explicit allowlist"
    );

    let all = scope(&json!({
        "tools": [],
    }));
    assert!(
        all.exposes("research.instrument") && all.exposes("market.search"),
        "an empty tool list keeps the legacy unrestricted behavior"
    );

    let explicit_all = scope(&json!({
        "tools": ["research.instrument"],
        "toolAccessMode": "all",
    }));
    assert!(
        explicit_all.exposes("market.search"),
        "an explicit toolAccessMode=all ignores the stale allowlist"
    );

    let implicit_selected = scope(&json!({
        "tools": ["strategy.research_backtest"],
    }));
    assert!(
        implicit_selected.exposes("backtest.kline_sync_status"),
        "research_backtest implies the kline sync companion, like Go"
    );
    assert!(!implicit_selected.exposes("strategy.optimize"));

    let optimize = scope(&json!({
        "tools": ["strategy.optimize"],
        "toolAccessMode": "selected",
    }));
    assert!(
        optimize.exposes("backtest.kline_sync_status"),
        "strategy.optimize implies the kline sync companion too"
    );
}

/// Go's `TestBacktestToolsIncludeRequiredKLineSyncStatusCompanion`: an agent
/// that selects `strategy.research_backtest` (or `strategy.optimize`) must also
/// receive `backtest.kline_sync_status`, while an unrelated selection must not.
#[test]
fn selected_backtest_tools_gain_the_kline_sync_companion() {
    let scope = ProductionAdkChatRuntime::agent_tool_scope;
    for parent in ["strategy.research_backtest", "strategy.optimize"] {
        let selected = scope(&json!({
            "tools": [parent],
            "toolAccessMode": "selected",
        }));
        assert!(
            selected.exposes(parent),
            "{parent} must stay visible to its own selection"
        );
        assert!(
            selected.exposes("backtest.kline_sync_status"),
            "{parent} must imply the kline sync companion"
        );
    }

    let unrelated = scope(&json!({
        "tools": ["market.snapshot"],
        "toolAccessMode": "selected",
    }));
    assert!(
        !unrelated.exposes("backtest.kline_sync_status"),
        "the companion is not granted to unrelated selections"
    );

    // An empty allowlist has no parent, so the companion stays hidden too.
    let none = scope(&json!({
        "tools": [],
        "toolAccessMode": "none",
    }));
    assert!(!none.exposes("backtest.kline_sync_status"));
}

/// Go's `TestToolDescriptorsRespectExplicitAccessModes`: the three access
/// modes project exactly the declared descriptor sets.  `tools.search` backs
/// the reference test, but the projection rule is the access mode itself, so
/// this asserts the mode matrix directly.
#[test]
fn explicit_access_modes_project_their_declared_tool_sets() {
    let scope = ProductionAdkChatRuntime::agent_tool_scope;
    let declared = ["market.snapshot", "orders.place"];

    let all = scope(&json!({
        "tools": declared,
        "toolAccessMode": "all",
    }));
    for name in declared {
        assert!(all.exposes(name), "all mode exposes {name}");
    }
    assert!(
        all.exposes("market.search"),
        "all mode ignores the stale allowlist"
    );

    let selected = scope(&json!({
        "tools": ["orders.place"],
        "toolAccessMode": "selected",
    }));
    assert!(selected.exposes("orders.place"));
    assert!(
        !selected.exposes("market.snapshot"),
        "selected mode exposes only the declared tools"
    );
    assert!(!selected.exposes("market.search"));

    let none = scope(&json!({
        "tools": declared,
        "toolAccessMode": "none",
    }));
    for name in declared {
        assert!(!none.exposes(name), "none mode hides {name}");
    }
    assert!(!none.exposes("market.search"));
}

/// Parity: go:452dea11:internal/assistant/engine/adk_edges_test.go:25
/// `TestGoogleADKMemoryServiceBoundaryBranches`: Rust has no Google ADK memory
/// service, so the durable contract is `Runtime.agentMemoryPrompt`: a workspace
/// row is shared with every agent, an agent's own rows stay private to it, an
/// agent without rows yields an empty prompt, and an unreadable store fails the
/// resolution instead of silently dropping memory.
#[test]
fn agent_memory_prompt_scopes_workspace_rows_and_fails_closed() {
    let (directory, store, session_store) = initialized_stores();
    std::fs::create_dir_all(directory.path().join("secrets")).expect("create secrets directory");
    std::fs::write(
        directory.path().join("secrets/adk-secrets.json"),
        r#"{"provider-memory-scope":"sk-memory"}"#,
    )
    .expect("write provider secrets");
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, "{}").expect("write settings");

    store
        .upsert_provider(
            "provider-memory-scope",
            &json!({
                "displayName": "Memory scope fixture",
                "baseUrl": "http://127.0.0.1:9/v1",
                "model": "fixture-model",
                "enabled": true,
            })
            .to_string(),
        )
        .expect("persist provider");
    for agent_id in ["agent-memory-owner", "agent-memory-peer"] {
        store
            .upsert_agent(
                agent_id,
                &json!({
                    "id": agent_id,
                    "name": agent_id,
                    "providerId": "provider-memory-scope",
                    "status": "ENABLED",
                    "memoryEnabled": true,
                })
                .to_string(),
            )
            .expect("persist agent");
    }
    let memory = |id: &str, agent_id: &str, scope: &str, key: &str, value: &str| {
        json!({
            "id": id,
            "agentId": agent_id,
            "scope": scope,
            "key": key,
            "value": value,
        })
        .to_string()
    };
    store
        .upsert_memory(
            "memory-shared",
            "",
            "workspace",
            "preference",
            &memory(
                "memory-shared",
                "",
                "workspace",
                "preference",
                "use HK market",
            ),
        )
        .expect("persist workspace memory");
    store
        .upsert_memory(
            "memory-owner",
            "agent-memory-owner",
            "agent",
            "risk",
            &memory(
                "memory-owner",
                "agent-memory-owner",
                "agent",
                "risk",
                "small",
            ),
        )
        .expect("persist owner memory");
    store
        .upsert_memory(
            "memory-peer",
            "agent-memory-peer",
            "agent",
            "peer",
            &memory(
                "memory-peer",
                "agent-memory-peer",
                "agent",
                "peer",
                "private",
            ),
        )
        .expect("persist peer memory");

    let runtime = ProductionAdkChatRuntime::new(
        Arc::clone(&store),
        Arc::clone(&session_store),
        &settings_path,
        Arc::new(RunCancellationRegistry::default()),
        Arc::new(crate::product::product_production_ports::ProductionToolCatalog::empty_for_test()),
    );

    let owner_prompt = runtime
        .agent_memory_prompt("agent-memory-owner")
        .expect("owner memory prompt");
    assert!(
        owner_prompt.contains("preference: use HK market"),
        "the workspace row is shared: {owner_prompt}"
    );
    assert!(
        owner_prompt.contains("risk: small"),
        "the agent's own row is included: {owner_prompt}"
    );
    assert!(
        !owner_prompt.contains("peer: private"),
        "another agent's row stays private: {owner_prompt}"
    );

    store
        .upsert_agent(
            "agent-memory-empty",
            &json!({
                "id": "agent-memory-empty",
                "name": "empty",
                "providerId": "provider-memory-scope",
                "status": "ENABLED",
                "memoryEnabled": true,
            })
            .to_string(),
        )
        .expect("persist memory-less agent");
    let empty_prompt = runtime
        .agent_memory_prompt("agent-memory-empty")
        .expect("memory prompt without rows");
    assert!(
        empty_prompt.contains("preference: use HK market"),
        "workspace rows still reach an agent without private rows: {empty_prompt}"
    );

    // An unreadable store must fail the resolution instead of dropping memory.
    let connection =
        Connection::open(directory.path().join("adk.db")).expect("reopen ADK database");
    connection
        .execute("DROP TABLE adk_memory", [])
        .expect("drop memory table");
    drop(connection);
    assert!(
        runtime.agent_memory_prompt("agent-memory-owner").is_err(),
        "a missing memory table is a store failure, not empty memory"
    );
}

/// Parity: go:452dea11:internal/assistant/engine/adk_edges_test.go:532
/// `TestRuntimeAgentProviderSessionAndMemoryBoundaryBranches`: the reachable
/// resolution boundaries are a missing agent, a disabled agent, a disabled
/// provider, a provider without credentials, and a store whose providers carry
/// no default for an agent that declares none.
#[test]
fn chat_resolution_reports_missing_agents_and_unusable_providers() {
    let (directory, store, session_store) = initialized_stores();
    std::fs::create_dir_all(directory.path().join("secrets")).expect("create secrets directory");
    std::fs::write(
        directory.path().join("secrets/adk-secrets.json"),
        r#"{"provider-keyed":"sk-keyed"}"#,
    )
    .expect("write provider secrets");
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, "{}").expect("write settings");

    for (id, enabled) in [
        ("provider-disabled", false),
        ("provider-keyless", true),
        ("provider-keyed", true),
    ] {
        store
            .upsert_provider(
                id,
                &json!({
                    "displayName": id,
                    "baseUrl": "http://127.0.0.1:9/v1",
                    "model": "fixture-model",
                    "enabled": enabled,
                })
                .to_string(),
            )
            .expect("persist provider");
    }
    for (id, provider_id, status) in [
        ("agent-disabled", "provider-keyed", "DISABLED"),
        ("agent-disabled-provider", "provider-disabled", "ENABLED"),
        ("agent-keyless", "provider-keyless", "ENABLED"),
        ("agent-keyed", "provider-keyed", "ENABLED"),
        ("agent-no-provider", " ", "ENABLED"),
    ] {
        store
            .upsert_agent(
                id,
                &json!({
                    "id": id,
                    "name": id,
                    "providerId": provider_id,
                    "status": status,
                })
                .to_string(),
            )
            .expect("persist agent");
    }

    let runtime = ProductionAdkChatRuntime::new(
        Arc::clone(&store),
        Arc::clone(&session_store),
        &settings_path,
        Arc::new(RunCancellationRegistry::default()),
        Arc::new(crate::product::product_production_ports::ProductionToolCatalog::empty_for_test()),
    );
    let failure = |agent_id: &str| -> String {
        let mut request = serde_json::Map::new();
        request.insert("agentId".to_owned(), json!(agent_id));
        match runtime.resolve_provider(&request) {
            Err(AdkChatPortError::Unavailable(message)) => message,
            Err(AdkChatPortError::Conflict(message)) => message,
            Err(AdkChatPortError::Failed { message, .. }) => message,
            Ok(resolved) => panic!("{agent_id} must not resolve a provider, got {resolved:?}"),
        }
    };

    assert!(
        failure("agent-missing").contains("agent not found"),
        "missing agent = {}",
        failure("agent-missing")
    );
    assert!(
        failure("agent-disabled").contains("agent is disabled"),
        "disabled agent = {}",
        failure("agent-disabled")
    );
    assert!(
        failure("agent-disabled-provider").contains("agent provider is unavailable"),
        "disabled provider = {}",
        failure("agent-disabled-provider")
    );
    assert!(
        failure("agent-keyless").contains("agent provider API keys is not configured"),
        "credential-less provider = {}",
        failure("agent-keyless")
    );
    // The store keeps one provider flagged as the default, so the
    // "no default configured" branch needs a store without any provider row.
    let (empty_directory, empty_store, empty_session_store) = initialized_stores();
    let empty_settings = empty_directory.path().join("settings.json");
    std::fs::write(&empty_settings, "{}").expect("write empty settings");
    empty_store
        .upsert_agent(
            "agent-no-default-provider",
            &json!({
                "id": "agent-no-default-provider",
                "name": "no default",
                "providerId": " ",
                "status": "ENABLED",
            })
            .to_string(),
        )
        .expect("persist provider-less agent");
    let empty_runtime = ProductionAdkChatRuntime::new(
        Arc::clone(&empty_store),
        Arc::clone(&empty_session_store),
        &empty_settings,
        Arc::new(RunCancellationRegistry::default()),
        Arc::new(crate::product::product_production_ports::ProductionToolCatalog::empty_for_test()),
    );
    let mut empty_request = serde_json::Map::new();
    empty_request.insert("agentId".to_owned(), json!("agent-no-default-provider"));
    let empty_failure = match empty_runtime.resolve_provider(&empty_request) {
        Err(AdkChatPortError::Unavailable(message)) => message,
        Err(AdkChatPortError::Conflict(message)) => message,
        Err(AdkChatPortError::Failed { message, .. }) => message,
        Ok(resolved) => panic!("an empty provider store must not resolve, got {resolved:?}"),
    };
    assert!(
        empty_failure.contains("default agent provider is not configured"),
        "agent without a provider and without any store default = {empty_failure}"
    );

    let mut request = serde_json::Map::new();
    request.insert("agentId".to_owned(), json!("agent-keyed"));
    let resolved = runtime
        .resolve_provider(&request)
        .expect("the credential-backed provider resolves");
    assert_eq!(resolved.id, "provider-keyed");
}
