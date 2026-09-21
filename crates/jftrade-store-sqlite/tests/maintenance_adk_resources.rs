//! ADK-owned maintenance boundaries for the managed ADK databases.
//!
//! Parity: go:452dea11:internal/assistant/assembly/maintenance_test.go:12
//! `TestDatabaseMaintenanceOwnsADKBusyPurgeAndCompactPaths` and
//! `go:452dea11:internal/assistant/assembly/maintenance_test.go:77`
//! `TestDatabaseMaintenanceFailsClosedWithoutOwnedRuntime`.
//!
//! Go routes ADK cleanup and compaction through the engine-owned
//! `DatabaseMaintenance` adapter: soft-deleted agents, workflows and triggers
//! are purged only for the exact approved candidate set, the runtime/session/
//! artifact databases each compact, and a request without that owner fails
//! closed. In Rust the same work belongs to `ManagedDatabaseMaintenanceStore`,
//! whose writer lease makes the owning engine the only maintenance client.

use std::path::Path;

use jftrade_datamanagement::{
    ApprovedCleanupPreview, CLEANUP_SOFT_DELETED, CleanupCandidate, CleanupCandidatePort,
    CleanupCandidateQuery, CleanupCandidateRecord, CleanupPreviewResponse, DATABASE_ADK,
    DATABASE_ADK_ARTIFACT, DATABASE_ADK_SESSION, DatabaseDescriptor, DatabaseMaintenancePort,
    MaintenanceOperationError, preview_cleanup,
};
use jftrade_store_sqlite::{
    AdkStore, ManagedDatabaseCleanupCandidateStore, ManagedDatabaseMaintenanceStore,
    initialize_current,
};
use rusqlite::Connection;
use serde_json::json;
use tempfile::tempdir;

fn initialize_database(path: &Path, component: &str) -> i64 {
    let connection = Connection::open(path).expect("create database");
    initialize_current(&connection, component).expect("initialize schema");
    let version = connection
        .query_row(
            "SELECT version FROM jftrade_schema_meta WHERE component_id = ?1",
            [component],
            |row| row.get::<_, i64>(0),
        )
        .expect("read schema version");
    drop(connection);
    version
}

fn descriptor(id: &str, path: &Path, expected_version: i64) -> DatabaseDescriptor {
    DatabaseDescriptor {
        id: id.to_owned(),
        name: id.to_owned(),
        path: path.to_string_lossy().into_owned(),
        description: String::new(),
        features: Vec::new(),
        expected_version,
    }
}

fn soft_deleted_query() -> CleanupCandidateQuery {
    CleanupCandidateQuery {
        kind: CLEANUP_SOFT_DELETED.to_owned(),
        older_than_days: 0,
        keep_latest: 0,
        cutoff: None,
    }
}

fn approved_preview(
    database_id: &str,
    candidates: Vec<CleanupCandidateRecord>,
    query: CleanupCandidateQuery,
) -> ApprovedCleanupPreview {
    let approved_candidates = candidates
        .iter()
        .map(|candidate| CleanupCandidate {
            id: candidate.id.clone(),
            category: candidate.category.clone(),
        })
        .collect::<Vec<_>>();
    let fingerprint = preview_cleanup(database_id, approved_candidates, None)
        .expect("candidate fingerprint")
        .fingerprint;
    ApprovedCleanupPreview {
        response: CleanupPreviewResponse {
            preview_id: "preview-adk-maintenance".to_owned(),
            expires_at: "2026-09-21T00:10:00Z".to_owned(),
            kind: query.kind.clone(),
            database_id: database_id.to_owned(),
            candidate_count: i64::try_from(candidates.len()).expect("candidate count"),
            estimated_bytes: 0,
            items: Vec::new(),
            confirmation_text: format!("PURGE {database_id}"),
            will_compact: true,
        },
        candidates,
        query,
        fingerprint,
    }
}

fn row_exists(path: &Path, table: &str, id: &str) -> bool {
    let connection = Connection::open(path).expect("reopen database");
    let count: i64 = connection
        .query_row(
            &format!("SELECT COUNT(*) FROM {table} WHERE id = ?1"),
            [id],
            |row| row.get(0),
        )
        .expect("count rows");
    count > 0
}

#[test]
fn adk_soft_deleted_configs_purge_only_for_the_approved_candidate_set() {
    let directory = tempdir().expect("temporary directory");
    let adk_path = directory.path().join("adk.db");
    let expected_version = initialize_database(&adk_path, DATABASE_ADK);

    let store = AdkStore::open(&adk_path).expect("open ADK store");
    store
        .upsert_agent(
            "agent-active",
            &json!({"id": "agent-active", "name": "Active Agent", "status": "ENABLED"}).to_string(),
        )
        .expect("persist active agent");
    store
        .upsert_agent(
            "agent-deleted",
            &json!({"id": "agent-deleted", "name": "Deleted Agent", "status": "DELETED"})
                .to_string(),
        )
        .expect("persist deleted agent");
    assert!(
        store
            .delete_agent("agent-deleted")
            .expect("soft-delete agent")
    );
    store
        .upsert_workflow(
            "workflow-deleted",
            "DISABLED",
            &json!({
                "id": "workflow-deleted",
                "name": "Deleted Workflow",
                "status": "DISABLED",
                "agentId": "agent-active",
                "promptTemplate": "prompt",
                "deletedAt": "2026-09-01T00:00:00Z",
            })
            .to_string(),
        )
        .expect("persist deleted workflow");
    store
        .upsert_workflow(
            "workflow-live",
            "ENABLED",
            &json!({
                "id": "workflow-live",
                "name": "Live Workflow",
                "status": "ENABLED",
                "agentId": "agent-active",
                "promptTemplate": "prompt",
            })
            .to_string(),
        )
        .expect("persist live workflow");
    store
        .upsert_workflow_trigger(
            "trigger-deleted",
            "workflow-live",
            "manual",
            "DISABLED",
            "",
            &json!({"id": "trigger-deleted", "deletedAt": "2026-09-01T00:00:00Z"}).to_string(),
        )
        .expect("persist deleted trigger");
    store
        .upsert_workflow_trigger(
            "trigger-cascade",
            "workflow-deleted",
            "manual",
            "ENABLED",
            "",
            &json!({"id": "trigger-cascade", "workflowId": "workflow-deleted"}).to_string(),
        )
        .expect("persist cascade trigger");
    drop(store);

    let descriptor = descriptor(DATABASE_ADK, &adk_path, expected_version);
    let query = soft_deleted_query();
    let candidates = ManagedDatabaseCleanupCandidateStore
        .candidates(&descriptor, &query)
        .expect("query ADK cleanup candidates");
    let mut listed = candidates
        .iter()
        .map(|candidate| (candidate.id.clone(), candidate.category.clone()))
        .collect::<Vec<_>>();
    listed.sort();
    assert_eq!(
        listed,
        [
            ("agent-deleted".to_owned(), "智能体".to_owned()),
            ("trigger-cascade".to_owned(), "触发器".to_owned()),
            ("trigger-deleted".to_owned(), "触发器".to_owned()),
            ("workflow-deleted".to_owned(), "工作流".to_owned()),
        ],
        "only soft-deleted rows and a deleted workflow's triggers are candidates"
    );

    let maintenance = ManagedDatabaseMaintenanceStore::new(
        vec![descriptor],
        directory.path().join("database-rebuild.json"),
        "test-maintenance",
    );
    let approved = approved_preview(DATABASE_ADK, candidates, query.clone());
    let result = maintenance
        .execute_cleanup(&approved)
        .expect("purge the approved ADK configs");
    assert_eq!(result.database_id, DATABASE_ADK);
    assert_eq!(result.deleted_count, 4);
    assert!(result.compacted, "cleanup compacts the ADK database");
    for (table, id) in [
        ("adk_agents", "agent-deleted"),
        ("adk_workflows", "workflow-deleted"),
        ("adk_workflow_triggers", "trigger-deleted"),
        ("adk_workflow_triggers", "trigger-cascade"),
    ] {
        assert!(
            !row_exists(&adk_path, table, id),
            "{table}.{id} must be purged"
        );
    }
    assert!(
        row_exists(&adk_path, "adk_agents", "agent-active"),
        "a live agent must survive the purge"
    );
    assert!(
        row_exists(&adk_path, "adk_workflows", "workflow-live"),
        "a live workflow must survive the purge"
    );

    // An approved candidate the database can never produce (unknown category)
    // is a changed candidate set: the purge fails closed and mutates nothing.
    let store = AdkStore::open(&adk_path).expect("reopen ADK store");
    store
        .upsert_agent(
            "agent-late",
            &json!({"id": "agent-late", "name": "Deleted Later", "status": "DELETED"}).to_string(),
        )
        .expect("persist late agent");
    assert!(
        store
            .delete_agent("agent-late")
            .expect("soft-delete late agent")
    );
    drop(store);
    let unknown_category = approved_preview(
        DATABASE_ADK,
        vec![CleanupCandidateRecord {
            id: "unknown".to_owned(),
            category: "未来类型".to_owned(),
            estimated_bytes: 0,
        }],
        query,
    );
    assert_eq!(
        maintenance.execute_cleanup(&unknown_category),
        Err(MaintenanceOperationError::Stale)
    );
    assert!(
        row_exists(&adk_path, "adk_agents", "agent-late"),
        "a rejected purge must not delete candidates behind the caller's back"
    );
}

#[test]
fn adk_maintenance_compacts_every_owned_database_and_requires_the_writer_lease() {
    let directory = tempdir().expect("temporary directory");
    let adk_path = directory.path().join("adk.db");
    let session_path = directory.path().join("adk-session.db");
    let artifact_path = directory.path().join("adk-artifact.db");
    let descriptors = vec![
        descriptor(
            DATABASE_ADK,
            &adk_path,
            initialize_database(&adk_path, DATABASE_ADK),
        ),
        descriptor(
            DATABASE_ADK_SESSION,
            &session_path,
            initialize_database(&session_path, DATABASE_ADK_SESSION),
        ),
        descriptor(
            DATABASE_ADK_ARTIFACT,
            &artifact_path,
            initialize_database(&artifact_path, DATABASE_ADK_ARTIFACT),
        ),
    ];
    let maintenance = ManagedDatabaseMaintenanceStore::new(
        descriptors,
        directory.path().join("database-rebuild.json"),
        "test-maintenance",
    );

    for database_id in [DATABASE_ADK, DATABASE_ADK_SESSION, DATABASE_ADK_ARTIFACT] {
        let result = maintenance
            .compact(database_id, "2026-09-21T00:00:00Z")
            .expect("compact an ADK-owned database");
        assert_eq!(result.database_id, database_id);
        assert!(result.compacted, "{database_id} must report compaction");
    }

    // Go's unsupported resource fails closed; the Rust lookup rejects an
    // unknown database id before any file is touched.
    assert!(matches!(
        maintenance.compact("unknown", "2026-09-21T00:00:00Z"),
        Err(MaintenanceOperationError::Rejected(_))
    ));

    // The owning engine holds the writer lease. While it is open, maintenance
    // cannot take over: both compaction and purge fail closed instead of
    // rewriting a live database behind its owner.
    let store = AdkStore::open(&adk_path).expect("open ADK store");
    assert!(matches!(
        maintenance.compact(DATABASE_ADK, "2026-09-21T00:00:00Z"),
        Err(MaintenanceOperationError::Conflict(_))
    ));
    let approved = approved_preview(DATABASE_ADK, Vec::new(), soft_deleted_query());
    assert!(matches!(
        maintenance.execute_cleanup(&approved),
        Err(MaintenanceOperationError::Conflict(_))
    ));
    drop(store);

    assert!(
        maintenance
            .compact(DATABASE_ADK, "2026-09-21T00:00:00Z")
            .is_ok(),
        "releasing the owner hands the database back to maintenance"
    );
}
