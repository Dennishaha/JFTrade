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

#[cfg(test)]
mod tests {
    use super::*;

    /// Parity: go:452dea11:internal/assistant/engine/adk_edges_test.go:351
    /// `TestRuntimeConstructionCompactionAndCloseBoundaryBranches`: a blank
    /// session id never blocks, the second compactor of the same session is
    /// rejected while the first guard is alive, other sessions stay
    /// independent, and dropping the guard releases the session.
    #[test]
    fn compaction_gate_serializes_one_compactor_per_session() {
        let (blank, blank_acquired) = begin_session_compaction("  ");
        assert!(
            blank_acquired,
            "a blank session id acquires without blocking"
        );
        assert!(
            blank.is_none(),
            "a blank session id has no guard to release"
        );

        let (first, first_acquired) = begin_session_compaction("session-one");
        assert!(first_acquired);
        let (duplicate, duplicate_acquired) = begin_session_compaction(" session-one ");
        assert!(
            !duplicate_acquired,
            "the same session must not compact twice"
        );
        assert!(duplicate.is_none());

        let (other, other_acquired) = begin_session_compaction("session-two");
        assert!(other_acquired, "unrelated sessions stay independent");
        drop(other);
        drop(first);

        let (again, again_acquired) = begin_session_compaction("session-one");
        assert!(again_acquired, "dropping the guard releases the session");
        drop(again);
    }
}
