//! Go `Runtime.beginSessionCompaction`: one in-flight context compaction per
//! session.
//!
//! Go keeps the set on the runtime instance.  The production port and the chat
//! runtime are separate objects here, so the set lives in one process-wide map
//! that both entry points share; an entry is removed as soon as the guard
//! drops, so a second compactor for the same session is rejected only while the
//! first one is genuinely running.

use std::collections::HashSet;
use std::sync::{Mutex, OnceLock};

/// Guard returned while a session compaction holds the gate.  Dropping it
/// releases the session for the next compactor.
#[derive(Debug)]
pub(crate) struct SessionCompactionGuard {
    session_id: String,
}

impl Drop for SessionCompactionGuard {
    fn drop(&mut self) {
        let mut sessions = gate()
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        sessions.remove(&self.session_id);
    }
}

fn gate() -> &'static Mutex<HashSet<String>> {
    static GATE: OnceLock<Mutex<HashSet<String>>> = OnceLock::new();
    GATE.get_or_init(|| Mutex::new(HashSet::new()))
}

/// Acquire the session's compaction gate.  A blank session id never blocks and
/// reports success, matching Go's `beginSessionCompaction` fallback.
pub(crate) fn begin_session_compaction(session_id: &str) -> (Option<SessionCompactionGuard>, bool) {
    let session_id = session_id.trim();
    if session_id.is_empty() {
        return (None, true);
    }
    let mut sessions = gate()
        .lock()
        .unwrap_or_else(std::sync::PoisonError::into_inner);
    if !sessions.insert(session_id.to_owned()) {
        return (None, false);
    }
    (
        Some(SessionCompactionGuard {
            session_id: session_id.to_owned(),
        }),
        true,
    )
}
