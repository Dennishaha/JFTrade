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
