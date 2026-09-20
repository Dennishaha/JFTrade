//! Session-context projection parity for the production ADK adapter.
//!
//! Reference: go:452dea11:internal/assistant/engine/session_context_test.go
//! `TestSessionContextUsesSessionProviderOverrideWindow` (line 120),
//! `TestSessionContextViewDoesNotAutoCompact` (line 651),
//! `TestSessionContextCompactionShrinksSessionView` (line 15) and
//! `TestSessionContextCompactionCreatesCurrentRevision` (line 183).

use std::collections::BTreeMap;
use std::sync::Arc;

use rusqlite::Connection;
use serde_json::{Value, json};
use tempfile::tempdir;

use jftrade_store_sqlite::{
    AdkArtifactStore, AdkSessionStore, AdkStore, RecordAdkEventParams, initialize_current,
};

use crate::product::product_adk_mutation_port::{
    AdkMutationInput, AdkMutationOperation, AdkMutationPort,
};
use crate::product::product_production_ports::ProductionAdkPort;
use crate::product::{AdkReadSnapshot, AdkReadSnapshotPort};

fn context_port() -> (tempfile::TempDir, ProductionAdkPort) {
    let directory = tempdir().expect("temporary directory");
    let adk_path = directory.path().join("adk.db");
    let session_path = directory.path().join("adk-session.db");
    let artifact_path = directory.path().join("adk-artifact.db");
    for (path, component) in [
        (&adk_path, "adk"),
        (&session_path, "adk-session"),
        (&artifact_path, "adk-artifact"),
    ] {
        let connection = Connection::open(path).expect("create ADK database");
        initialize_current(&connection, component).expect("initialize ADK schema");
    }
    let port = ProductionAdkPort::new_for_test(
        Arc::new(AdkStore::open(&adk_path).expect("open adk store")),
        Arc::new(AdkSessionStore::open(&session_path).expect("open adk session store")),
        Arc::new(AdkArtifactStore::open(&artifact_path).expect("open adk artifact store")),
        directory.path().join("settings.json"),
    );
    (directory, port)
}

fn seed_provider(port: &ProductionAdkPort, id: &str, window_tokens: i64) {
    port.store
        .upsert_provider(
            id,
            &json!({
                "id": id,
                "displayName": id,
                "baseUrl": "https://provider.example.test",
                "model": format!("{id}-model"),
                "contextWindowTokens": window_tokens,
                "enabled": true,
            })
            .to_string(),
        )
        .expect("seed provider");
}

/// The chat entry point resolves a credential before it prepares the turn, so
/// the seeded provider carries one (the read projection redacts it).
fn seed_provider_with_key(port: &ProductionAdkPort, id: &str, window_tokens: i64) {
    port.store
        .upsert_provider(
            id,
            &json!({
                "id": id,
                "displayName": id,
                "baseUrl": "https://provider.example.test",
                "model": format!("{id}-model"),
                "contextWindowTokens": window_tokens,
                "apiKey": "sk-turn-fixture",
                "enabled": true,
            })
            .to_string(),
        )
        .expect("seed provider with credential");
}

fn seed_agent(port: &ProductionAdkPort, id: &str, provider_id: &str, recent_user_window: i64) {
    port.store
        .upsert_agent(
            id,
            &json!({
                "id": id,
                "name": id,
                "instruction": "Test agent",
                "providerId": provider_id,
                "recentUserWindow": recent_user_window,
                "permissionMode": "approval",
                "status": "ENABLED",
            })
            .to_string(),
        )
        .expect("seed agent");
}

fn seed_session(port: &ProductionAdkPort, id: &str, agent_id: &str) {
    port.store
        .upsert_session(
            id,
            agent_id,
            &json!({"id": id, "agentId": agent_id, "title": id}).to_string(),
        )
        .expect("seed session");
    // Transcript events cascade off the session store's own session row.
    port.session_store
        .upsert_session("jftrade", "local", id, "{}")
        .expect("seed session transcript row");
}

/// Append `count` alternating user/model events, the way Go's
/// `appendContextEvents` seeds a transcript.
fn append_context_events(port: &ProductionAdkPort, session_id: &str, start: usize, count: usize) {
    for index in start..start + count {
        let author = if index % 2 == 1 { "assistant" } else { "user" };
        port.session_store
            .record_event(RecordAdkEventParams {
                id: &format!("event-{index:03}"),
                app_name: "jftrade",
                user_id: "local",
                session_id,
                invocation_id: &format!("inv-{index:03}"),
                author,
                content: &format!("message {index}"),
            })
            .expect("append context event");
    }
}

fn context_snapshot(port: &ProductionAdkPort, session_id: &str) -> Value {
    match port
        .read(&format!("/api/v1/adk/sessions/{session_id}/context"), "")
        .expect("read session context")
    {
        AdkReadSnapshot::Json(value) => value,
        other => panic!("expected JSON session context, got {other:?}"),
    }
}

fn compact(port: &ProductionAdkPort, session_id: &str, body: Value) -> Value {
    port.mutate(&AdkMutationInput {
        operation: AdkMutationOperation::CompactSessionContext,
        identifiers: BTreeMap::from([("sessionId".to_owned(), session_id.to_owned())]),
        body,
        webhook_secret: None,
    })
    .expect("compact session context")
}

fn segments_for_revision(port: &ProductionAdkPort, session_id: &str, revision: &str) -> Vec<Value> {
    port.store
        .list_handoff_segments(session_id, true)
        .expect("list handoff segments")
        .into_iter()
        .map(|segment| serde_json::from_str::<Value>(&segment.payload_json).expect("segment JSON"))
        .filter(|segment| segment["contextRevisionId"].as_str() == Some(revision))
        .collect()
}

fn estimate_handoff_tokens(segments: &[Value]) -> usize {
    let text = segments
        .iter()
        .filter_map(|segment| segment["summary"].as_str())
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .collect::<Vec<_>>()
        .join("\n\n");
    if text.is_empty() {
        return 0;
    }
    let bytes = format!("Session handoff summaries:\n{text}").trim().len();
    bytes.div_ceil(4)
}

fn session_timeline(port: &ProductionAdkPort, session_id: &str) -> Vec<Value> {
    match port
        .read(&format!("/api/v1/adk/sessions/{session_id}"), "")
        .expect("read session")
    {
        AdkReadSnapshot::Json(value) => value["timeline"].as_array().cloned().unwrap_or_default(),
        other => panic!("expected JSON session, got {other:?}"),
    }
}

fn context_notices(entries: &[Value]) -> Vec<Value> {
    entries
        .iter()
        .filter(|entry| entry["kind"] == "context_notice")
        .cloned()
        .collect()
}

/// A runtime over the same stores as the port, the way the composition root
/// wires them.
fn runtime_for(
    directory: &tempfile::TempDir,
    port: &ProductionAdkPort,
) -> Arc<super::ProductionAdkChatRuntime> {
    super::ProductionAdkChatRuntime::new(
        Arc::clone(&port.store),
        Arc::clone(&port.session_store),
        &directory.path().join("settings.json"),
        Arc::new(super::RunCancellationRegistry::default()),
        Arc::new(crate::product::product_production_ports::ProductionToolCatalog::empty_for_test()),
    )
}

/// Go `appendLargeContextEvents`: 80 events whose combined text dwarfs the
/// provider's 80-token window.
fn append_large_context_events(port: &ProductionAdkPort, session_id: &str, count: usize) {
    for index in 0..count {
        let author = if index % 2 == 1 { "assistant" } else { "user" };
        port.session_store
            .record_event(RecordAdkEventParams {
                id: &format!("large-{index:03}"),
                app_name: "jftrade",
                user_id: "local",
                session_id,
                invocation_id: &format!("inv-large-{index:03}"),
                author,
                content: &"message pressure ".repeat(50),
            })
            .expect("append large context event");
    }
}

fn pending_user_text() -> String {
    "pending input ".repeat(200)
}

#[test]
fn auto_compaction_emits_streaming_then_final_notice_and_context_delta() {
    let (directory, port) = context_port();
    seed_provider(&port, "provider-auto", 80);
    seed_agent(&port, "agent-auto", "provider-auto", 1);
    seed_session(&port, "session-auto", "agent-auto");
    append_large_context_events(&port, "session-auto", 80);
    let runtime = runtime_for(&directory, &port);
    let (before, projected_ratio, _) = runtime
        .projected_context_projection("session-auto", &pending_user_text())
        .expect("projected snapshot");
    assert!(projected_ratio >= 0.85, "{before}");

    let mut deltas = Vec::new();
    runtime
        .maybe_auto_compact_session("session-auto", &pending_user_text(), false, |delta| {
            deltas.push(delta);
            Ok(())
        })
        .expect("auto compaction");

    let timeline = deltas
        .iter()
        .filter_map(|delta| match delta {
            super::SessionContextDelta::Timeline(value) => Some(value.clone()),
            super::SessionContextDelta::Context(_) => None,
        })
        .collect::<Vec<_>>();
    assert_eq!(timeline.len(), 2, "{timeline:?}");
    assert_eq!(timeline[0]["kind"], "context_notice", "{timeline:?}");
    assert_eq!(timeline[0]["status"], "streaming", "{timeline:?}");
    assert_eq!(timeline[1]["status"], "final", "{timeline:?}");
    assert_eq!(timeline[0]["id"], timeline[1]["id"], "{timeline:?}");
    assert_eq!(
        timeline[1]["text"], "已压缩上下文，继续使用最新摘要。",
        "{timeline:?}"
    );
    let compacted = deltas
        .iter()
        .find_map(|delta| match delta {
            super::SessionContextDelta::Context(value) => Some(value.clone()),
            super::SessionContextDelta::Timeline(_) => None,
        })
        .expect("context delta");
    assert!(
        compacted["currentInputTokens"].as_u64().unwrap_or(u64::MAX)
            < before["projectedNextTurnTokens"].as_u64().unwrap_or(0),
        "compaction must shrink the projection: {compacted} vs {before}"
    );
    assert_eq!(compacted["autoCompacted"], true, "{compacted}");
    assert!(
        compacted["compactedEventCount"].as_u64().unwrap_or(0) > 0,
        "{compacted}"
    );
    let saved = port
        .store
        .list_session_notices("session-auto")
        .expect("list notices");
    assert_eq!(saved.len(), 1, "one notice row per compaction");
    let seen = context_notices(&session_timeline(&port, "session-auto"));
    assert_eq!(seen.len(), 1, "{seen:?}");
    assert_eq!(seen[0]["status"], "final", "{seen:?}");
}

#[test]
fn auto_compaction_skips_while_another_compaction_holds_the_session_gate() {
    let (directory, port) = context_port();
    seed_provider(&port, "provider-gate-auto", 80);
    seed_agent(&port, "agent-gate-auto", "provider-gate-auto", 1);
    seed_session(&port, "session-gate-auto", "agent-gate-auto");
    append_large_context_events(&port, "session-gate-auto", 80);
    let runtime = runtime_for(&directory, &port);

    let (guard, acquired) =
        crate::product::product_adk_session_compaction_gate::begin_session_compaction(
            "session-gate-auto",
        );
    assert!(acquired);
    let mut gated = Vec::new();
    runtime
        .maybe_auto_compact_session("session-gate-auto", &pending_user_text(), true, |delta| {
            gated.push(delta);
            Ok(())
        })
        .expect("gated auto compaction");
    assert!(
        gated.is_empty(),
        "a held gate publishes no delta: {gated:?}"
    );
    assert!(
        context_notices(&session_timeline(&port, "session-gate-auto")).is_empty(),
        "a held gate writes no notice"
    );
    drop(guard);

    let mut released = Vec::new();
    runtime
        .maybe_auto_compact_session("session-gate-auto", &pending_user_text(), true, |delta| {
            released.push(delta);
            Ok(())
        })
        .expect("auto compaction after release");
    assert!(!released.is_empty(), "release enables the compaction");
}

#[test]
fn workflow_auto_compaction_proceeds_under_an_active_run_while_chat_waits() {
    let (directory, port) = context_port();
    seed_provider(&port, "provider-workflow-auto", 80);
    seed_agent(&port, "agent-workflow-auto", "provider-workflow-auto", 1);
    seed_session(&port, "session-workflow-auto", "agent-workflow-auto");
    append_large_context_events(&port, "session-workflow-auto", 80);
    port.store
        .create_run(jftrade_store_sqlite::CreateAdkRunParams {
            id: "run-active-workflow-parent",
            session_id: "session-workflow-auto",
            agent_id: "agent-workflow-auto",
            status: "RUNNING",
            client_request_id: "request-active-workflow-parent",
            request_fingerprint: "fingerprint-active-workflow-parent",
            payload_json: r#"{"id":"run-active-workflow-parent","sessionId":"session-workflow-auto","agentId":"agent-workflow-auto","status":"RUNNING"}"#,
        })
        .expect("seed active parent run");
    let runtime = runtime_for(&directory, &port);

    let mut skipped = Vec::new();
    runtime
        .maybe_auto_compact_session(
            "session-workflow-auto",
            &pending_user_text(),
            false,
            |delta| {
                skipped.push(delta);
                Ok(())
            },
        )
        .expect("chat auto compaction");
    assert!(
        skipped.is_empty(),
        "a live run keeps the chat entry point from compacting: {skipped:?}"
    );

    let mut workflow = Vec::new();
    runtime
        .maybe_auto_compact_session(
            "session-workflow-auto",
            &pending_user_text(),
            true,
            |delta| {
                workflow.push(delta);
                Ok(())
            },
        )
        .expect("workflow auto compaction");
    assert!(
        workflow
            .iter()
            .any(|delta| matches!(delta, super::SessionContextDelta::Context(_))),
        "the workflow entry point publishes the compacted snapshot: {workflow:?}"
    );
    let snapshot = context_snapshot(&port, "session-workflow-auto");
    assert_eq!(snapshot["autoCompacted"], true, "{snapshot}");
    assert!(
        snapshot["activeHandoffCount"].as_u64().unwrap_or(0) > 0,
        "{snapshot}"
    );
}

#[test]
fn model_context_read_compacts_only_when_the_session_gate_is_free() {
    let (directory, port) = context_port();
    seed_provider(&port, "provider-model-auto", 80);
    seed_agent(&port, "agent-model-auto", "provider-model-auto", 1);
    seed_session(&port, "session-model-auto", "agent-model-auto");
    append_large_context_events(&port, "session-model-auto", 80);
    let runtime = runtime_for(&directory, &port);

    let (guard, acquired) =
        crate::product::product_adk_session_compaction_gate::begin_session_compaction(
            "session-model-auto",
        );
    assert!(acquired);
    runtime
        .auto_compact_for_model_context("session-model-auto", &pending_user_text())
        .expect("gated model-context read");
    let gated = context_snapshot(&port, "session-model-auto");
    assert_eq!(gated["activeHandoffCount"], 0, "{gated}");
    assert_eq!(gated["autoCompacted"], false, "{gated}");
    drop(guard);

    runtime
        .auto_compact_for_model_context("session-model-auto", &pending_user_text())
        .expect("model-context read after release");
    let released = context_snapshot(&port, "session-model-auto");
    assert_eq!(released["autoCompacted"], true, "{released}");
    assert!(
        released["activeHandoffCount"].as_u64().unwrap_or(0) > 0,
        "{released}"
    );
}

#[test]
fn model_context_autocompacts_before_the_provider_payload() {
    let (directory, port) = context_port();
    seed_provider(&port, "provider-payload", 80);
    seed_agent(&port, "agent-payload", "provider-payload", 1);
    seed_session(&port, "session-payload", "agent-payload");
    append_large_context_events(&port, "session-payload", 80);
    let runtime = runtime_for(&directory, &port);
    let before = super::durable_context_items(
        port.store.as_ref(),
        port.session_store.as_ref(),
        "session-payload",
        None,
    )
    .expect("durable context before");
    assert_eq!(before.len(), 80, "the raw transcript is the model context");

    runtime
        .auto_compact_for_model_context("session-payload", &pending_user_text())
        .expect("model-context auto compaction");
    let after = super::durable_context_items(
        port.store.as_ref(),
        port.session_store.as_ref(),
        "session-payload",
        None,
    )
    .expect("durable context after");
    assert!(
        after.len() < before.len(),
        "the model payload must shrink: {after:?}"
    );
    let segments = port
        .store
        .list_handoff_segments("session-payload", true)
        .expect("list handoff segments");
    assert!(!segments.is_empty(), "an active handoff segment exists");
}

/// The chat entry point itself must compact: a turn started against a session
/// that already exceeds the window leaves a durable automatic projection, which
/// is what Go's `sessionService.Get` path observes before the provider payload
/// is assembled.
#[test]
fn a_chat_turn_autocompacts_the_session_before_the_provider_payload() {
    let (directory, port) = context_port();
    seed_provider_with_key(&port, "provider-turn", 80);
    seed_agent(&port, "agent-turn", "provider-turn", 1);
    seed_session(&port, "session-turn", "agent-turn");
    append_large_context_events(&port, "session-turn", 80);
    let runtime = runtime_for(&directory, &port);

    let input = crate::product::product_adk_chat_stream_port::AdkChatInput {
        body: json!({
            "agentId": "agent-turn",
            "sessionId": "session-turn",
            "message": "current request",
        })
        .to_string()
        .into_bytes(),
        client_request_id: "11111111-1111-4111-8111-111111111199".to_owned(),
    };
    runtime
        .prepare_chat(
            crate::product::product_adk_chat_stream_port::AdkChatRoute::Chat,
            &input,
        )
        .expect("prepare the chat turn");

    let snapshot = context_snapshot(&port, "session-turn");
    assert_eq!(snapshot["autoCompacted"], true, "{snapshot}");
    assert!(
        snapshot["activeHandoffCount"].as_u64().unwrap_or(0) > 0,
        "{snapshot}"
    );
    assert!(
        snapshot["compactedEventCount"].as_u64().unwrap_or(0) > 0,
        "{snapshot}"
    );
}

#[test]
fn manual_context_compaction_writes_the_done_notice_into_the_timeline() {
    let (_directory, port) = context_port();
    seed_provider(&port, "provider-notice", 100_000);
    seed_agent(&port, "agent-notice", "provider-notice", 1);
    seed_session(&port, "session-notice", "agent-notice");
    append_context_events(&port, "session-notice", 0, 6);

    compact(
        &port,
        "session-notice",
        json!({"mode": "normal", "trigger": "manual", "reason": "test notice"}),
    );

    let entries = session_timeline(&port, "session-notice");
    let notices = context_notices(&entries);
    assert_eq!(notices.len(), 1, "{entries:?}");
    assert_eq!(notices[0]["status"], "final", "{notices:?}");
    assert_eq!(
        notices[0]["text"], "已压缩上下文，继续使用最新摘要。",
        "{notices:?}"
    );
    assert_eq!(notices[0]["sessionId"], "session-notice", "{notices:?}");
    assert!(
        entries.iter().any(|entry| entry["kind"] == "user_message"),
        "the transcript stays in the same timeline: {entries:?}"
    );
}

#[test]
fn a_second_compaction_is_rejected_while_the_session_gate_is_held() {
    let (_directory, port) = context_port();
    seed_provider(&port, "provider-gate", 100_000);
    seed_agent(&port, "agent-gate", "provider-gate", 1);
    seed_session(&port, "session-gate", "agent-gate");
    append_context_events(&port, "session-gate", 0, 6);

    let (guard, acquired) =
        crate::product::product_adk_session_compaction_gate::begin_session_compaction(
            "session-gate",
        );
    assert!(acquired);
    let rejected = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::CompactSessionContext,
            identifiers: BTreeMap::from([("sessionId".to_owned(), "session-gate".to_owned())]),
            body: json!({"mode": "normal", "trigger": "auto"}),
            webhook_secret: None,
        })
        .expect_err("a held gate rejects the compaction");
    match rejected {
        crate::product::product_adk_mutation_port::AdkMutationPortError::Failed {
            status,
            code,
            message,
        } => {
            assert_eq!(status, 500);
            assert_eq!(code, "ADK_SESSION_CONTEXT_COMPACT_FAILED");
            assert_eq!(message, "session context compaction already running");
        }
        other => panic!("expected the route's own failure envelope, got {other:?}"),
    }
    assert!(
        context_notices(&session_timeline(&port, "session-gate")).is_empty(),
        "a gated attempt must not announce a compaction"
    );
    drop(guard);

    compact(
        &port,
        "session-gate",
        json!({"mode": "normal", "trigger": "auto"}),
    );
    let notices = context_notices(&session_timeline(&port, "session-gate"));
    assert_eq!(notices.len(), 1, "{notices:?}");
    assert_eq!(notices[0]["status"], "final", "{notices:?}");
}

#[test]
fn a_rejected_compaction_records_the_failed_notice() {
    let (_directory, port) = context_port();
    seed_provider(&port, "provider-failed", 100_000);
    seed_agent(&port, "agent-failed", "provider-failed", 1);
    seed_session(&port, "session-failed", "agent-failed");
    append_context_events(&port, "session-failed", 0, 6);
    port.store
        .create_run(jftrade_store_sqlite::CreateAdkRunParams {
            id: "run-active-failed",
            session_id: "session-failed",
            agent_id: "agent-failed",
            status: "RUNNING",
            client_request_id: "request-active-failed",
            request_fingerprint: "fingerprint-active-failed",
            payload_json: r#"{"id":"run-active-failed","sessionId":"session-failed","agentId":"agent-failed","status":"RUNNING"}"#,
        })
        .expect("seed active run");

    let rejected = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::CompactSessionContext,
            identifiers: BTreeMap::from([("sessionId".to_owned(), "session-failed".to_owned())]),
            body: json!({"mode": "normal", "trigger": "manual"}),
            webhook_secret: None,
        })
        .expect_err("an active run blocks manual compaction");
    match rejected {
        crate::product::product_adk_mutation_port::AdkMutationPortError::Failed {
            status, ..
        } => {
            assert_eq!(status, 409);
        }
        other => panic!("expected 409 for an active run, got {other:?}"),
    }
    let notices = context_notices(&session_timeline(&port, "session-failed"));
    assert_eq!(notices.len(), 1, "{notices:?}");
    assert_eq!(notices[0]["status"], "error", "{notices:?}");
    assert_eq!(
        notices[0]["text"], "上下文压缩失败，将继续使用当前上下文。",
        "{notices:?}"
    );
}

#[test]
fn session_context_window_follows_the_composer_provider_override() {
    let (_directory, port) = context_port();
    seed_provider(&port, "context-base-provider", 1000);
    seed_provider(&port, "context-override-provider", 200_000);
    seed_agent(&port, "agent-window", "context-base-provider", 6);
    seed_session(&port, "session-window", "agent-window");
    append_context_events(&port, "session-window", 0, 1);

    let base = context_snapshot(&port, "session-window");
    assert_eq!(
        base["contextWindowTokens"], 1000,
        "the agent's own provider supplies the window: {base}"
    );

    // A durable projection written before the switch keeps the old window
    // until the next read re-resolves it.
    let compacted = compact(
        &port,
        "session-window",
        json!({"mode": "normal", "trigger": "manual", "reason": "before the switch"}),
    );
    assert_eq!(compacted["contextWindowTokens"], 1000, "{compacted}");
    let stored_base = context_snapshot(&port, "session-window");
    assert_eq!(stored_base["contextWindowTokens"], 1000, "{stored_base}");

    port.store
        .upsert_session_composer_state(
            "session-window",
            &json!({
                "sessionId": "session-window",
                "providerIdOverride": "context-override-provider",
                "modelOverride": "override-model",
            })
            .to_string(),
        )
        .expect("save composer state");

    let overridden = context_snapshot(&port, "session-window");
    assert_eq!(
        overridden["contextWindowTokens"], 200_000,
        "the composer provider override decides the window: {overridden}"
    );
    assert!(
        overridden["usageRatio"]
            .as_f64()
            .is_some_and(|ratio| ratio > 0.0),
        "a resolved window yields a positive usage ratio: {overridden}"
    );
}

#[test]
fn session_context_read_reports_pressure_without_compacting() {
    let (_directory, port) = context_port();
    seed_provider(&port, "provider-small-window", 80);
    seed_agent(&port, "agent-pressure", "provider-small-window", 1);
    seed_session(&port, "session-pressure", "agent-pressure");
    for index in 0..80 {
        let author = if index % 2 == 1 { "assistant" } else { "user" };
        port.session_store
            .record_event(RecordAdkEventParams {
                id: &format!("pressure-{index:03}"),
                app_name: "jftrade",
                user_id: "local",
                session_id: "session-pressure",
                invocation_id: &format!("inv-{index:03}"),
                author,
                content: &"message pressure ".repeat(50),
            })
            .expect("append pressure event");
    }

    let snapshot = context_snapshot(&port, "session-pressure");
    let status = snapshot["status"].as_str().unwrap_or_default();
    assert!(
        matches!(status, "near_limit" | "critical"),
        "an oversized transcript reports pressure, got {status}: {snapshot}"
    );
    assert_eq!(snapshot["autoCompacted"], false, "{snapshot}");
    assert_eq!(snapshot["activeHandoffCount"], 0, "{snapshot}");
    assert!(
        port.store
            .list_handoff_segments("session-pressure", true)
            .expect("list handoff segments")
            .is_empty(),
        "a context read must not compact: {snapshot}"
    );
}

#[test]
fn context_compaction_shrinks_the_projected_session_view() {
    let (_directory, port) = context_port();
    seed_provider(&port, "provider-compact", 100_000);
    seed_agent(&port, "agent-compact", "provider-compact", 2);
    seed_session(&port, "session-compact", "agent-compact");
    append_context_events(&port, "session-compact", 0, 10);

    let before = context_snapshot(&port, "session-compact");
    assert_eq!(before["rawEventCount"], 10, "{before}");

    let after = compact(
        &port,
        "session-compact",
        json!({"mode": "normal", "trigger": "manual", "reason": "test compaction"}),
    );
    assert!(
        after["compactedEventCount"].as_u64().unwrap_or(0) > 0,
        "{after}"
    );
    assert!(
        after["protectedRecentCount"].as_u64().unwrap_or(0)
            < after["rawEventCount"].as_u64().unwrap_or(0),
        "{after}"
    );
    assert!(
        !after["summaryPreview"]
            .as_str()
            .unwrap_or_default()
            .is_empty(),
        "{after}"
    );

    let reread = context_snapshot(&port, "session-compact");
    assert_eq!(
        reread["contextRevisionId"], after["contextRevisionId"],
        "the compacted projection is durable: {reread}"
    );
    assert_eq!(reread["compactedEventCount"], after["compactedEventCount"]);
    assert_eq!(reread["activeHandoffCount"], 1, "{reread}");
}

#[test]
fn each_context_compaction_creates_the_next_current_revision() {
    let (_directory, port) = context_port();
    seed_provider(&port, "provider-revision", 100_000);
    seed_agent(&port, "agent-revision", "provider-revision", 1);
    seed_session(&port, "session-revision", "agent-revision");
    append_context_events(&port, "session-revision", 0, 12);

    let before = context_snapshot(&port, "session-revision");
    let first = compact(
        &port,
        "session-revision",
        json!({"mode": "normal", "trigger": "manual", "reason": "first"}),
    );
    assert_eq!(first["sessionId"], "session-revision", "{first}");
    assert_ne!(
        first["contextRevisionId"], before["contextRevisionId"],
        "the first compaction publishes a new revision: {first}"
    );
    assert_eq!(
        first["previousContextRevisionId"], before["contextRevisionId"],
        "{first}"
    );
    let first_revision = first["contextRevisionId"].as_str().unwrap_or_default();
    assert_eq!(
        segments_for_revision(&port, "session-revision", first_revision).len(),
        1,
        "one handoff segment carries the new revision"
    );

    append_context_events(&port, "session-revision", 12, 4);
    let second = compact(
        &port,
        "session-revision",
        json!({"mode": "normal", "trigger": "manual", "reason": "second"}),
    );
    let second_revision = second["contextRevisionId"].as_str().unwrap_or_default();
    assert_ne!(second_revision, first_revision, "{second}");
    assert_eq!(
        second["previousContextRevisionId"], first_revision,
        "{second}"
    );
    let current_segments = segments_for_revision(&port, "session-revision", second_revision);
    assert_eq!(current_segments.len(), 1, "{second}");
    assert_eq!(
        second["activeHandoffCount"],
        current_segments.len(),
        "{second}"
    );

    let all_active = port
        .store
        .list_handoff_segments("session-revision", true)
        .expect("list active handoff segments")
        .into_iter()
        .map(|segment| serde_json::from_str::<Value>(&segment.payload_json).expect("segment JSON"))
        .collect::<Vec<_>>();
    assert!(
        all_active.len() > current_segments.len(),
        "superseded revisions stay durable: {all_active:?}"
    );
    assert_eq!(
        second["breakdown"]["handoffTokens"],
        estimate_handoff_tokens(&current_segments),
        "the projection counts only the current revision: {second}"
    );
    assert_ne!(
        second["breakdown"]["handoffTokens"],
        estimate_handoff_tokens(&all_active),
        "superseded handoffs must not inflate the window: {second}"
    );
    assert!(
        second["rawEventCount"].as_u64().unwrap_or(0)
            > second["compactedEventCount"].as_u64().unwrap_or(0),
        "raw diagnostics stay separate from the compacted view: {second}"
    );
}
