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
