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
