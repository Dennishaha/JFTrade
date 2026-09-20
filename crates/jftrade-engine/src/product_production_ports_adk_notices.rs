//! Durable context-compaction notices, mirroring Go `context_notice.go`.
//!
//! A compaction announces itself as a `context_notice` timeline entry before it
//! touches the projection: the entry starts `streaming`, then the same id is
//! finalized with the done text (or the error text when the compaction fails).
//! Because the row keeps its original `created_at`, the notice stays where the
//! console first saw it even after the later update.

use super::*;

pub(crate) const CONTEXT_COMPACTION_STARTED_TEXT: &str = "正在压缩上下文...";
pub(crate) const CONTEXT_COMPACTION_DONE_TEXT: &str = "已压缩上下文，继续使用最新摘要。";
pub(crate) const CONTEXT_COMPACTION_FAILED_TEXT: &str = "上下文压缩失败，将继续使用当前上下文。";

/// Go `TimelineKindContextNotice` / `TimelineStatus*`.
pub(crate) const TIMELINE_KIND_CONTEXT_NOTICE: &str = "context_notice";
pub(crate) const TIMELINE_STATUS_STREAMING: &str = "streaming";
pub(crate) const TIMELINE_STATUS_FINAL: &str = "final";
pub(crate) const TIMELINE_STATUS_ERROR: &str = "error";

/// The identity of one notice across its lifecycle updates.
#[derive(Clone, Debug, PartialEq, Eq)]
pub(crate) struct ContextCompactionNotice {
    pub id: String,
    pub session_id: String,
    pub status: String,
    pub text: String,
    pub created_at: String,
}

/// Go `Runtime.createContextCompactionNotice`.
pub(crate) fn create_context_compaction_notice(
    store: &jftrade_store_sqlite::AdkStore,
    session_id: &str,
) -> Option<ContextCompactionNotice> {
    save_context_compaction_notice(
        store,
        ContextCompactionNotice {
            id: String::new(),
            session_id: session_id.trim().to_owned(),
            status: TIMELINE_STATUS_STREAMING.to_owned(),
            text: CONTEXT_COMPACTION_STARTED_TEXT.to_owned(),
            created_at: String::new(),
        },
    )
}

/// Go `Runtime.updateContextCompactionNotice`: an unknown notice id is a no-op
/// rather than a second row.
pub(crate) fn update_context_compaction_notice(
    store: &jftrade_store_sqlite::AdkStore,
    notice: &ContextCompactionNotice,
    status: &str,
    text: &str,
) -> Option<ContextCompactionNotice> {
    if notice.id.trim().is_empty() {
        return None;
    }
    let mut updated = notice.clone();
    updated.status = status.trim().to_owned();
    updated.text = text.trim().to_owned();
    save_context_compaction_notice(store, updated)
}

fn save_context_compaction_notice(
    store: &jftrade_store_sqlite::AdkStore,
    notice: ContextCompactionNotice,
) -> Option<ContextCompactionNotice> {
    if notice.session_id.is_empty() {
        return None;
    }
    let now = super::super::mutation::now_rfc3339();
    let id = if notice.id.trim().is_empty() {
        format!(
            "notice-{}-{}",
            super::super::mutation::helpers::normalize_id(&notice.session_id),
            super::super::mutation::helpers::normalize_id(&now)
        )
    } else {
        notice.id.clone()
    };
    let created_at = if notice.created_at.trim().is_empty() {
        now.clone()
    } else {
        notice.created_at.clone()
    };
    let entry = json!({
        "id": id,
        "sessionId": notice.session_id,
        "kind": TIMELINE_KIND_CONTEXT_NOTICE,
        "createdAt": created_at,
        "updatedAt": now,
        "sequence": 0,
        "status": notice.status,
        "text": notice.text,
    });
    let payload_json = entry.to_string();
    store
        .save_session_notice(
            &notice.session_id,
            &id,
            "",
            TIMELINE_KIND_CONTEXT_NOTICE,
            entry["status"].as_str().unwrap_or_default(),
            &payload_json,
        )
        .ok()?;
    Some(ContextCompactionNotice {
        id,
        session_id: notice.session_id,
        status: entry["status"].as_str().unwrap_or_default().to_owned(),
        text: entry["text"].as_str().unwrap_or_default().to_owned(),
        created_at,
    })
}

/// Project one stored notice for the session timeline read.
pub(crate) fn session_notice_value(
    row: jftrade_store_sqlite::StoredAdkEntity,
) -> Result<Value, AdkReadSnapshotError> {
    let mut value: Value = serde_json::from_str(&row.payload_json)
        .map_err(|error| invalid_payload("session notice", error))?;
    let object = value.as_object_mut().ok_or_else(|| {
        invalid_payload(
            "session notice",
            "stored session notice payload must be a JSON object",
        )
    })?;
    object
        .entry("id".to_owned())
        .or_insert_with(|| Value::String(row.id.clone()));
    object
        .entry("kind".to_owned())
        .or_insert_with(|| Value::String(TIMELINE_KIND_CONTEXT_NOTICE.to_owned()));
    object
        .entry("status".to_owned())
        .or_insert_with(|| Value::String(TIMELINE_STATUS_FINAL.to_owned()));
    object
        .entry("createdAt".to_owned())
        .or_insert_with(|| Value::String(row.created_at.clone()));
    object.insert(
        "updatedAt".to_owned(),
        Value::String(row.updated_at.clone()),
    );
    if object
        .get("runId")
        .and_then(Value::as_str)
        .is_some_and(|run_id| run_id.trim().is_empty())
    {
        object.remove("runId");
    }
    Ok(value)
}

/// Go `BuildSessionTimeline`: notices and messages share one ordered timeline,
/// and the sequence numbers are renumbered after the merge so a notice written
/// between two messages shifts the later entries exactly once.
pub(crate) fn merge_session_timeline(messages: Vec<Value>, notices: Vec<Value>) -> Vec<Value> {
    let mut entries = messages
        .into_iter()
        .chain(notices)
        .map(|value| {
            let created_at = value
                .get("createdAt")
                .and_then(Value::as_str)
                .unwrap_or_default()
                .to_owned();
            let id = value
                .get("id")
                .and_then(Value::as_str)
                .unwrap_or_default()
                .to_owned();
            (created_at, id, value)
        })
        .collect::<Vec<_>>();
    entries.sort_by(|left, right| left.0.cmp(&right.0).then_with(|| left.1.cmp(&right.1)));
    entries
        .into_iter()
        .enumerate()
        .map(|(sequence, (_, _, mut value))| {
            if let Some(object) = value.as_object_mut() {
                object.insert("sequence".to_owned(), Value::from(sequence));
            }
            value
        })
        .collect()
}
