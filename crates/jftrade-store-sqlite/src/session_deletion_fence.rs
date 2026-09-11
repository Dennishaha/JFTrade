//! Process-local fencing for session-scoped writes during cascade deletion.
//!
//! The three ADK databases are deliberately kept as separate SQLite files for
//! compatibility.  A cascade therefore cannot use one SQLite transaction to
//! cover all of them.  This registry provides the missing linearization point:
//! once deletion starts, every writer that acquires its database mutex after
//! that point is rejected for the affected session.  Entries are intentionally
//! retained for the lifetime of the process so a partially completed cascade
//! can be retried without allowing a stale worker to recreate an orphan row.

use std::collections::HashSet;
use std::path::{Path, PathBuf};
use std::sync::{Mutex, OnceLock};

#[derive(Clone, Debug, Eq, Hash, PartialEq)]
struct FenceKey {
    path: PathBuf,
    session_id: String,
}

static FENCES: OnceLock<Mutex<HashSet<FenceKey>>> = OnceLock::new();

fn registry() -> &'static Mutex<HashSet<FenceKey>> {
    FENCES.get_or_init(|| Mutex::new(HashSet::new()))
}

fn key(path: &Path, session_id: &str) -> FenceKey {
    let canonical_path = path.canonicalize().unwrap_or_else(|_| path.to_path_buf());
    FenceKey {
        path: canonical_path,
        session_id: session_id.to_owned(),
    }
}

/// Begin fencing writes for one database/session pair.
///
/// A poisoned registry is treated as an unavailable safety boundary.  Callers
/// must fail closed rather than continue a destructive operation without a
/// reliable writer fence.
pub(crate) fn begin(path: &Path, session_id: &str) -> Result<(), ()> {
    registry()
        .lock()
        .map_err(|_| ())?
        .insert(key(path, session_id));
    Ok(())
}

/// Return whether a session-scoped write is fenced.  A poisoned registry is
/// fail-closed because allowing a write would risk recreating deleted state.
pub(crate) fn is_fenced(path: &Path, session_id: &str) -> bool {
    registry()
        .lock()
        .map(|fences| fences.contains(&key(path, session_id)))
        .unwrap_or(true)
}
