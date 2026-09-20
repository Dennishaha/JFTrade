//! Cleanup-candidate boundaries for the managed ADK database.
//!
//! Parity: go:452dea11:internal/assistant/engine/adk_edges_test.go:237
//! `TestStoreMaintenanceBoundaryBranches`: only soft-deleted rows may be
//! purged, a candidate set that changed after the preview is rejected instead
//! of deleting something else, and an active run keeps maintenance busy.

use jftrade_datamanagement::{
    CLEANUP_SOFT_DELETED, CleanupCandidate, CleanupCandidatePort, CleanupCandidateQuery,
    DATABASE_ADK, DatabaseDescriptor, MaintenanceError, preview_cleanup, verify_execute,
};
use jftrade_store_sqlite::{AdkStore, ManagedDatabaseCleanupCandidateStore, initialize_current};
use rusqlite::Connection;
use serde_json::json;
use tempfile::tempdir;

fn agent_payload(id: &str, status: &str) -> String {
    json!({
        "id": id,
        "name": id,
        "providerId": "provider-maintenance",
        "status": status,
    })
    .to_string()
}

#[test]
fn soft_deleted_adk_rows_are_the_only_candidates_and_changes_reject_execute() {
    let directory = tempdir().expect("temporary directory");
    let adk_path = directory.path().join("adk.db");
    let connection = Connection::open(&adk_path).expect("create ADK database");
    initialize_current(&connection, "adk").expect("initialize ADK schema");
    drop(connection);

    let store = AdkStore::open(&adk_path).expect("open ADK store");
    store
        .upsert_agent("agent-active", &agent_payload("agent-active", "ENABLED"))
        .expect("persist active agent");
    store
        .upsert_agent("agent-deleted", &agent_payload("agent-deleted", "DELETED"))
        .expect("persist deleted agent");
    assert!(
        store
            .delete_agent("agent-deleted")
            .expect("soft-delete agent")
    );

    let descriptor = DatabaseDescriptor {
        id: DATABASE_ADK.to_owned(),
        name: "ADK".to_owned(),
        path: adk_path.to_string_lossy().into_owned(),
        description: String::new(),
        features: Vec::new(),
        expected_version: 0,
    };
    let query = CleanupCandidateQuery {
        kind: CLEANUP_SOFT_DELETED.to_owned(),
        older_than_days: 0,
        keep_latest: 0,
        cutoff: None,
    };
    let candidates = ManagedDatabaseCleanupCandidateStore
        .candidates(&descriptor, &query)
        .expect("query cleanup candidates");
    let ids = candidates
        .iter()
        .map(|candidate| candidate.id.as_str())
        .collect::<Vec<_>>();
    assert_eq!(
        ids,
        ["agent-deleted"],
        "only the soft-deleted row may be purged: {candidates:?}"
    );

    let approved = candidates
        .iter()
        .map(|candidate| CleanupCandidate {
            id: candidate.id.clone(),
            category: candidate.category.clone(),
        })
        .collect::<Vec<_>>();
    let preview = preview_cleanup(DATABASE_ADK, approved.clone(), None).expect("cleanup preview");

    // A candidate set that no longer matches the approved preview is rejected.
    assert_eq!(
        verify_execute(&preview, Vec::new(), None),
        Err(MaintenanceError::CandidatesChanged)
    );
    assert_eq!(
        verify_execute(&preview, approved.clone(), None).expect("matching candidates"),
        approved
    );

    // An active run keeps maintenance busy, like the reference's
    // `HasDatabaseActivity` gate.
    assert_eq!(
        preview_cleanup(DATABASE_ADK, approved, Some("active run")),
        Err(MaintenanceError::Busy("active run".to_owned()))
    );
}
