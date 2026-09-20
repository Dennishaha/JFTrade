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
    pub updated_at: String,
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
            updated_at: String::new(),
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
    let saved = ContextCompactionNotice {
        id,
        session_id: notice.session_id,
        status: notice.status,
        text: notice.text,
        created_at,
        updated_at: now,
    };
    let entry = notice_delta_value(&saved);
    let payload_json = entry.to_string();
    store
        .save_session_notice(
            &saved.session_id,
            &saved.id,
            "",
            TIMELINE_KIND_CONTEXT_NOTICE,
            &saved.status,
            &payload_json,
        )
        .ok()?;
    Some(saved)
}

/// The wire value shared by the persisted row and the streaming delta.
pub(crate) fn notice_delta_value(notice: &ContextCompactionNotice) -> Value {
    json!({
        "id": notice.id,
        "sessionId": notice.session_id,
        "kind": TIMELINE_KIND_CONTEXT_NOTICE,
        "createdAt": notice.created_at,
        "updatedAt": notice.updated_at,
        "sequence": 0,
        "status": notice.status,
        "text": notice.text,
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

#[cfg(test)]
mod tests {
    use super::*;
    use jftrade_store_sqlite::{AdkStore, initialize_current};

    fn notice_store() -> (tempfile::TempDir, std::path::PathBuf, AdkStore) {
        let directory = tempfile::tempdir().expect("temporary directory");
        let path = directory.path().join("adk.db");
        let connection = rusqlite::Connection::open(&path).expect("create ADK database");
        initialize_current(&connection, "adk").expect("initialize ADK schema");
        drop(connection);
        let store = AdkStore::open(&path).expect("open ADK store");
        (directory, path, store)
    }

    fn stored_notice(session_id: &str) -> ContextCompactionNotice {
        ContextCompactionNotice {
            id: String::new(),
            session_id: session_id.to_owned(),
            status: TIMELINE_STATUS_STREAMING.to_owned(),
            text: CONTEXT_COMPACTION_STARTED_TEXT.to_owned(),
            created_at: String::new(),
            updated_at: String::new(),
        }
    }

    /// Parity: go:452dea11:internal/assistant/engine/adk_edges_test.go:105
    /// `TestContextCompactionNoticeBoundaryBranches`: an empty identity is a
    /// no-op, an unwritable store degrades to an empty notice instead of an
    /// error, and a stored notice keeps its id and `createdAt` while moving
    /// from `streaming`/started to `final`/done.
    #[test]
    fn context_notices_are_best_effort_and_keep_one_identity_across_updates() {
        let (_directory, _path, store) = notice_store();

        let blank_session = create_context_compaction_notice(&store, "   ");
        assert!(blank_session.is_none(), "a blank session cannot be notified");

        let mut unnamed = stored_notice("session-1");
        unnamed.id = " ".to_owned();
        assert!(
            update_context_compaction_notice(
                &store,
                &unnamed,
                TIMELINE_STATUS_FINAL,
                CONTEXT_COMPACTION_DONE_TEXT,
            )
            .is_none(),
            "an update without a notice id must not create a second row"
        );

        let created =
            create_context_compaction_notice(&store, "session-1").expect("created notice");
        assert!(created.id.starts_with("notice-session-1-"));
        assert_eq!(created.status, TIMELINE_STATUS_STREAMING);
        assert_eq!(created.text, CONTEXT_COMPACTION_STARTED_TEXT);
        let started_delta = notice_delta_value(&created);
        assert_eq!(started_delta["kind"], json!(TIMELINE_KIND_CONTEXT_NOTICE));
        assert_eq!(started_delta["status"], json!(TIMELINE_STATUS_STREAMING));

        let updated = update_context_compaction_notice(
            &store,
            &created,
            TIMELINE_STATUS_FINAL,
            CONTEXT_COMPACTION_DONE_TEXT,
        )
        .expect("finalized notice");
        assert_eq!(updated.id, created.id);
        assert_eq!(updated.created_at, created.created_at);
        assert_eq!(updated.status, TIMELINE_STATUS_FINAL);
        assert_eq!(updated.text, CONTEXT_COMPACTION_DONE_TEXT);
        assert_eq!(notice_delta_value(&updated)["id"], json!(updated.id));

        // A store whose notices table is gone behaves like Go's closed store:
        // the announcement is dropped, not surfaced as an error.
        let (_failed_directory, failed_path, failed_store) = notice_store();
        let connection = rusqlite::Connection::open(&failed_path).expect("reopen ADK database");
        connection
            .execute("DROP TABLE adk_session_notices", [])
            .expect("drop notices table");
        drop(connection);
        assert!(create_context_compaction_notice(&failed_store, "session-2").is_none());
    }
}
