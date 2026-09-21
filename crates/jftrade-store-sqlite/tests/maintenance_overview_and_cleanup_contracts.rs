//! Overview and cleanup contracts for the managed databases.
//!
//! Parity: `go:452dea11:internal/app/apiserver/datamigration/maintenance_test.go:17`
//! `TestOverviewCountsMainWALSHMAndKeepsDatabaseErrorsLocal`,
//! `.../maintenance_test.go:49` `TestOverviewSupportsSummaryOnlyAndSingleDatabase`,
//! `.../maintenance_test.go:255` `TestOverviewCleanableCategoriesADKPreviewAndCompact`,
//! `.../manager_boundaries_test.go:187`
//! `TestManagerMarkerPersistenceNormalizesAndSurfacesFilesystemErrors`,
//! `.../manager_boundaries_test.go:280`
//! `TestManagerPropagatesUnreadableMarkerAndDatabaseStatErrors`,
//! `.../maintenance_failure_paths_test.go:270`
//! `TestMaintenanceStorageAndCandidateBoundaryErrorsAreContained`,
//! `.../maintenance_failure_paths_test.go:552`
//! `TestMaintenanceCleanupAndCompactionReportActualReclaimedStorage` and
//! `.../maintenance_failure_paths_test.go:167`
//! `TestMaintenancePreviewDefaultsAndConcurrentStateChangesFailClosed`.

use std::fs;
use std::path::{Path, PathBuf};
use std::sync::Arc;

use jftrade_datamanagement::{
    CLEANUP_SOFT_DELETED, CleanupCandidatePort, CleanupCandidateQuery, CleanupPreviewIdPort,
    CleanupPreviewRequest, CleanupPreviewService, DATABASE_ADK, DATABASE_ADK_ARTIFACT,
    DATABASE_ADK_SESSION, DATABASE_BACKTEST, DATABASE_BACKTEST_RUNS, DATABASE_EXECUTION,
    DATABASE_RESEARCH, DATABASE_STRATEGY, DATABASE_WATCHLIST, DatabaseDescriptor,
    DatabaseMaintenancePort, ManagedDatabasePaths, OverviewError, OverviewRequest, OverviewService,
    managed_database_descriptors,
};
use jftrade_store_sqlite::{
    ManagedDatabaseCleanupCandidateStore, ManagedDatabaseMaintenanceStore,
    ManagedDatabaseOverviewStore, initialize_current,
};
use rusqlite::Connection;
use tempfile::{TempDir, tempdir};
use time::OffsetDateTime;

const CHECKED_AT: &str = "2026-09-21T00:00:00Z";

#[derive(Debug)]
struct FixedPreviewIds;

impl CleanupPreviewIdPort for FixedPreviewIds {
    fn new_preview_id(&self) -> Result<String, String> {
        Ok("0123456789abcdef0123456789abcdef".to_owned())
    }
}

struct Fixture {
    _directory: TempDir,
    descriptors: Vec<DatabaseDescriptor>,
    marker_path: PathBuf,
}

impl Fixture {
    fn new() -> Self {
        let directory = tempdir().expect("temporary directory");
        let path = |name: &str| directory.path().join(name).to_string_lossy().into_owned();
        let paths = ManagedDatabasePaths::new([
            (DATABASE_BACKTEST, path("backtest.db")),
            (DATABASE_BACKTEST_RUNS, path("backtest-runs.db")),
            (DATABASE_STRATEGY, path("strategy-runtime.db")),
            (DATABASE_EXECUTION, path("execution-orders.db")),
            (DATABASE_ADK, path("adk.db")),
            (DATABASE_ADK_SESSION, path("adk-session.db")),
            (DATABASE_ADK_ARTIFACT, path("adk-artifact.db")),
            (DATABASE_WATCHLIST, path("watchlists.db")),
            (DATABASE_RESEARCH, path("research.db")),
        ]);
        Self {
            descriptors: managed_database_descriptors(&paths),
            marker_path: directory.path().join("database-rebuild.json"),
            _directory: directory,
        }
    }

    fn descriptor(&self, id: &str) -> &DatabaseDescriptor {
        self.descriptors
            .iter()
            .find(|descriptor| descriptor.id == id)
            .expect("managed descriptor")
    }

    fn directory_path(&self) -> &Path {
        self._directory.path()
    }

    fn initialize(&self, id: &str) -> PathBuf {
        let path = PathBuf::from(&self.descriptor(id).path);
        let connection = Connection::open(&path).expect("create database");
        initialize_current(&connection, id).expect("initialize schema");
        drop(connection);
        path
    }

    fn overview_service(&self) -> OverviewService {
        OverviewService::new(
            self.descriptors.clone(),
            Arc::new(ManagedDatabaseOverviewStore::new(self.marker_path.clone())),
        )
    }

    fn preview_service(&self) -> CleanupPreviewService {
        CleanupPreviewService::new(
            self.descriptors.clone(),
            Arc::new(ManagedDatabaseOverviewStore::new(self.marker_path.clone())),
            Arc::new(ManagedDatabaseCleanupCandidateStore),
            Arc::new(FixedPreviewIds),
        )
    }

    fn maintenance_store(&self) -> ManagedDatabaseMaintenanceStore {
        ManagedDatabaseMaintenanceStore::new(
            self.descriptors.clone(),
            self.marker_path.clone(),
            "overview-cleanup-contract-test",
        )
    }
}

fn execute(path: &Path, sql: &str) {
    Connection::open(path)
        .expect("open managed database")
        .execute_batch(sql)
        .expect("execute fixture statement");
}

fn seed_soft_deleted_definition(path: &Path) {
    execute(
        path,
        "INSERT INTO strategy_design_definitions
             (id, script, visual_model_json, deleted_at)
         VALUES ('deleted', 'script', '{}', '2026-01-01T00:00:00Z');",
    );
}

fn seed_backtest_history(path: &Path, rows: i64) {
    let mut sql = String::from("CREATE TABLE history_payload (value BLOB);");
    for index in 0..rows {
        sql.push_str(&format!(
            "INSERT INTO backtest_runs (id, status, request_json, result_json, created_at, updated_at) \
             VALUES ('run-{index}', 'completed', '{{}}', '{{}}', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');"
        ));
    }
    sql.push_str("DROP TABLE history_payload;");
    execute(path, &sql);
}

fn seed_soft_deleted_adk(path: &Path) {
    execute(
        path,
        "INSERT INTO adk_agents (id, payload_json, created_at, updated_at)
             VALUES ('agent-deleted', '{\"deletedAt\":\"2026-01-01T00:00:00Z\"}', 't0', 't1');
         INSERT INTO adk_workflows (id, status, payload_json, created_at, updated_at)
             VALUES ('workflow-deleted', 'deleted', '{\"deletedAt\":\"2026-01-01T00:00:00Z\"}', 't0', 't1');
         INSERT INTO adk_workflow_triggers
             (id, workflow_id, trigger_type, status, next_run_at, payload_json, created_at, updated_at)
             VALUES ('trigger-child', 'workflow-deleted', 'manual', 'deleted', '', '{}', 't0', 't1');",
    );
}

#[test]
fn overview_counts_main_wal_and_shm_and_keeps_an_unreadable_database_local() {
    let fixture = Fixture::new();
    let backtest = PathBuf::from(&fixture.descriptor(DATABASE_BACKTEST).path);
    fs::write(&backtest, vec![0_u8; 100]).expect("write unreadable database");
    fs::write(format!("{}-wal", backtest.display()), vec![0_u8; 40]).expect("write wal file");
    fs::write(format!("{}-shm", backtest.display()), vec![0_u8; 20]).expect("write shm file");

    let response = fixture
        .overview_service()
        .overview(OverviewRequest::default(), CHECKED_AT.to_owned())
        .expect("overview");
    let entry = response
        .databases
        .iter()
        .find(|database| database.descriptor.id == DATABASE_BACKTEST)
        .expect("backtest database entry");
    assert_eq!(
        entry.status, "unavailable",
        "a database error stays local to its entry"
    );
    assert_eq!(entry.storage.main_bytes, 100);
    assert_eq!(entry.storage.wal_bytes, 40);
    assert!(entry.storage.shm_bytes >= 20);
    assert_eq!(
        entry.storage.total_bytes,
        entry.storage.main_bytes + entry.storage.wal_bytes + entry.storage.shm_bytes
    );
    assert_eq!(response.totals.total_bytes, entry.storage.total_bytes);
    assert_eq!(response.databases.len(), 9);
}

// Parity: go:452dea11:internal/app/apiserver/datamigration/maintenance_test.go:49 TestOverviewSupportsSummaryOnlyAndSingleDatabase
#[test]
fn overview_summary_only_skips_storage_and_a_single_filter_keeps_its_totals() {
    let fixture = Fixture::new();
    let strategy = fixture.initialize(DATABASE_STRATEGY);
    seed_soft_deleted_definition(&strategy);
    let service = fixture.overview_service();

    let summary = service
        .overview(
            OverviewRequest {
                summary_only: true,
                database_id: String::new(),
            },
            CHECKED_AT.to_owned(),
        )
        .expect("summary overview");
    assert_eq!(summary.databases.len(), 9);
    assert_eq!(summary.totals.total_bytes, 0);
    for database in &summary.databases {
        assert_eq!(database.storage.total_bytes, 0);
        assert!(
            database.cleanable.is_none(),
            "a summary never collects cleanable categories"
        );
    }

    let single = service
        .overview(
            OverviewRequest {
                summary_only: false,
                database_id: format!(" {DATABASE_STRATEGY} "),
            },
            CHECKED_AT.to_owned(),
        )
        .expect("single database overview");
    assert_eq!(single.databases.len(), 1);
    assert_eq!(single.databases[0].descriptor.id, DATABASE_STRATEGY);
    assert_eq!(
        single.totals.total_bytes,
        single.databases[0].storage.total_bytes
    );
    assert_eq!(
        single.databases[0].cleanable.as_ref().map(Vec::len),
        Some(1),
        "the strategy database reports its soft-deleted definition"
    );

    let unknown = service
        .overview(
            OverviewRequest {
                summary_only: false,
                database_id: "missing".to_owned(),
            },
            CHECKED_AT.to_owned(),
        )
        .expect_err("an unknown database id is rejected");
    assert!(matches!(unknown, OverviewError::UnknownDatabase(id) if id == "missing"));
}

// Parity: go:452dea11:internal/app/apiserver/datamigration/maintenance_test.go:255 TestOverviewCleanableCategoriesADKPreviewAndCompact
#[test]
fn overview_lists_cleanable_categories_for_soft_deleted_and_history_rows() {
    let fixture = Fixture::new();
    let strategy = fixture.initialize(DATABASE_STRATEGY);
    seed_soft_deleted_definition(&strategy);
    let backtest_runs = fixture.initialize(DATABASE_BACKTEST_RUNS);
    seed_backtest_history(&backtest_runs, 25);
    let adk = fixture.initialize(DATABASE_ADK);
    seed_soft_deleted_adk(&adk);

    let response = fixture
        .overview_service()
        .overview(OverviewRequest::default(), CHECKED_AT.to_owned())
        .expect("overview");
    let cleanable = |id: &str| {
        response
            .databases
            .iter()
            .find(|database| database.descriptor.id == id)
            .expect("managed database entry")
            .cleanable
            .clone()
            .unwrap_or_default()
    };

    let strategy_items = cleanable(DATABASE_STRATEGY);
    assert_eq!(strategy_items.len(), 1);
    assert_eq!(strategy_items[0].kind, CLEANUP_SOFT_DELETED);
    assert_eq!(strategy_items[0].count, 1);
    assert!(strategy_items[0].estimated_bytes > 0);

    let backtest_items = cleanable(DATABASE_BACKTEST_RUNS);
    assert_eq!(backtest_items.len(), 1);
    assert_eq!(backtest_items[0].count, 25);

    let adk_items = cleanable(DATABASE_ADK);
    assert_eq!(adk_items.len(), 3, "agents, workflows and triggers");
    assert_eq!(
        adk_items
            .iter()
            .map(|item| (item.label.as_str(), item.count))
            .collect::<Vec<_>>(),
        vec![
            ("已删除智能体", 1),
            ("已删除工作流", 1),
            // The overview counts payload-level deletion only; a trigger that
            // cascades from a deleted workflow is reported by the cleanup
            // candidate query below, exactly like the Go owner.
            ("已删除触发器", 0),
        ]
    );

    let preview = fixture
        .preview_service()
        .preview(CleanupPreviewRequest {
            kind: CLEANUP_SOFT_DELETED.to_owned(),
            database_id: DATABASE_ADK.to_owned(),
            older_than_days: 0,
            keep_latest: 0,
        })
        .expect("ADK cleanup preview");
    assert_eq!(preview.candidate_count, 3);
    assert_eq!(preview.confirmation_text, "CLEANUP adk 3");
}

// Parity: go:452dea11:internal/app/apiserver/datamigration/manager_boundaries_test.go:187 TestManagerMarkerPersistenceNormalizesAndSurfacesFilesystemErrors
#[test]
fn overview_normalizes_rebuild_marker_ids_and_fails_closed_when_it_is_unreadable() {
    let fixture = Fixture::new();
    fs::write(
        &fixture.marker_path,
        br#"{"databaseIds":[" strategy ","","adk","strategy"]}"#,
    )
    .expect("write rebuild marker");
    let service = fixture.overview_service();

    let response = service
        .overview(
            OverviewRequest {
                summary_only: true,
                database_id: String::new(),
            },
            CHECKED_AT.to_owned(),
        )
        .expect("overview with a pending rebuild");
    let mut scheduled = response
        .databases
        .iter()
        .filter(|database| database.rebuild_scheduled)
        .map(|database| database.descriptor.id.clone())
        .collect::<Vec<_>>();
    scheduled.sort();
    assert_eq!(
        scheduled,
        vec![DATABASE_ADK.to_owned(), DATABASE_STRATEGY.to_owned()],
        "marker ids are trimmed, deduplicated and matched to descriptors"
    );
    assert!(
        response
            .databases
            .iter()
            .filter(|database| database.rebuild_scheduled)
            .all(|database| database.restart_required)
    );

    fs::remove_file(&fixture.marker_path).expect("remove marker");
    fs::create_dir(&fixture.marker_path).expect("replace the marker with a directory");
    let error = service
        .overview(OverviewRequest::default(), CHECKED_AT.to_owned())
        .expect_err("an unreadable marker fails the whole overview");
    assert!(matches!(error, OverviewError::RebuildMarker(_)));
}

// Parity: go:452dea11:internal/app/apiserver/datamigration/maintenance_failure_paths_test.go:142 TestMaintenanceCandidateQueriesFailSafelyWhenSchemaDoesNotMatch
#[test]
fn cleanup_candidates_fail_closed_when_the_maintenance_tables_are_missing() {
    let fixture = Fixture::new();
    let strategy = fixture.initialize(DATABASE_STRATEGY);
    execute(&strategy, "DROP TABLE strategy_design_definitions;");
    let store = ManagedDatabaseCleanupCandidateStore;
    let query = CleanupCandidateQuery {
        kind: CLEANUP_SOFT_DELETED.to_owned(),
        older_than_days: 0,
        keep_latest: 0,
        cutoff: None,
    };
    let error = store
        .candidates(fixture.descriptor(DATABASE_STRATEGY), &query)
        .expect_err("a missing cleanup table fails the candidate query");
    assert!(
        error.contains("no such table"),
        "unexpected candidate error: {error}"
    );

    let response = fixture
        .overview_service()
        .overview(
            OverviewRequest {
                summary_only: false,
                database_id: DATABASE_STRATEGY.to_owned(),
            },
            CHECKED_AT.to_owned(),
        )
        .expect("overview");
    assert!(
        response.databases[0].cleanable.is_none(),
        "an unreadable cleanable query leaves the overview without categories"
    );

    let watchlist = fixture.initialize(DATABASE_WATCHLIST);
    assert!(watchlist.is_file());
    let error = store
        .candidates(fixture.descriptor(DATABASE_WATCHLIST), &query)
        .expect_err("unsupported cleanup categories are rejected");
    assert!(
        error.contains("unsupported"),
        "unexpected category error: {error}"
    );

    let history_path = fixture.directory_path().join("history-candidates.db");
    execute(
        &history_path,
        "CREATE TABLE backtest_runs
             (id TEXT, status TEXT, request_json TEXT, result_json TEXT, updated_at TEXT);
         INSERT INTO backtest_runs
             VALUES (NULL, 'completed', '{}', '{}', '2026-01-01T00:00:00Z');",
    );
    let history_descriptor = DatabaseDescriptor {
        id: DATABASE_BACKTEST_RUNS.to_owned(),
        name: "backtest runs".to_owned(),
        path: history_path.to_string_lossy().into_owned(),
        description: String::new(),
        features: Vec::new(),
        expected_version: 1,
    };
    let history_query = CleanupCandidateQuery {
        kind: jftrade_datamanagement::CLEANUP_BACKTEST_HISTORY.to_owned(),
        older_than_days: 1,
        keep_latest: 1,
        cutoff: Some(jftrade_kernel::WireTimestamp::from_offset_datetime(
            time::OffsetDateTime::parse(
                "2026-02-01T00:00:00Z",
                &time::format_description::well_known::Rfc3339,
            )
            .expect("cutoff timestamp"),
        )),
    };
    let error = store
        .candidates(&history_descriptor, &history_query)
        .expect_err("a candidate row without an id is rejected");
    assert!(
        error.contains("decode backtest cleanup id"),
        "unexpected NULL-id error: {error}"
    );

    let adk = fixture.initialize(DATABASE_ADK);
    execute(&adk, "DROP TABLE adk_workflow_triggers;");
    let error = store
        .candidates(fixture.descriptor(DATABASE_ADK), &query)
        .expect_err("a missing ADK trigger table fails the candidate query");
    assert!(
        error.contains("no such table"),
        "unexpected ADK candidate error: {error}"
    );
}

// Parity: go:452dea11:internal/app/apiserver/datamigration/maintenance_test.go:185 TestBacktestCleanupPreviewUsesAgeAndLatestProtection
#[test]
fn backtest_history_preview_prefers_age_and_latest_protection() {
    let fixture = Fixture::new();
    let runs = fixture.initialize(DATABASE_BACKTEST_RUNS);
    seed_backtest_history(&runs, 25);
    execute(
        &runs,
        "INSERT INTO backtest_runs (id, status, request_json, result_json, created_at, updated_at)
         VALUES ('running', 'running', '{}', '{}', '2025-01-01T00:00:00Z', '2025-01-01T00:00:00Z');",
    );
    let previews = fixture.preview_service();
    let preview = previews
        .preview(CleanupPreviewRequest {
            kind: jftrade_datamanagement::CLEANUP_BACKTEST_HISTORY.to_owned(),
            database_id: DATABASE_BACKTEST_RUNS.to_owned(),
            older_than_days: 30,
            keep_latest: 20,
        })
        .expect("backtest history preview");
    assert_eq!(
        preview.candidate_count, 5,
        "only runs older than the retention window and beyond the latest twenty qualify"
    );
    assert_eq!(preview.confirmation_text, "CLEANUP backtest-runs 5");
    assert!(preview.will_compact);

    let approved = previews
        .approved_preview(&preview.preview_id, OffsetDateTime::now_utc())
        .expect("approved preview")
        .expect("stored preview");
    let result = fixture
        .maintenance_store()
        .execute_cleanup(&approved)
        .expect("execute the approved cleanup");
    assert_eq!(result.deleted_count, 5);
    assert!(result.compacted);
    assert_eq!(
        Connection::open(&runs)
            .expect("reopen backtest runs")
            .query_row("SELECT COUNT(*) FROM backtest_runs", [], |row| row
                .get::<_, i64>(0))
            .expect("count remaining runs"),
        21,
        "the latest twenty runs plus the running run stay"
    );
}

// Parity: go:452dea11:internal/app/apiserver/datamigration/manager_boundaries_test.go:108 TestInspectDatabaseClassifiesFilesystemAndSchemaStates
#[test]
fn database_inspection_classifies_filesystem_and_schema_states() {
    let fixture = Fixture::new();
    let directory_path = fixture.directory_path().join("strategy-runtime.db");
    fs::create_dir(&directory_path).expect("create a directory shaped like a database");
    let legacy = fixture.directory_path().join("backtest.db");
    execute(&legacy, "CREATE TABLE legacy (id TEXT PRIMARY KEY);");
    fixture.initialize(DATABASE_ADK);

    let response = fixture
        .overview_service()
        .overview(OverviewRequest::default(), CHECKED_AT.to_owned())
        .expect("overview");
    let status = |id: &str| {
        response
            .databases
            .iter()
            .find(|database| database.descriptor.id == id)
            .expect("managed database entry")
            .clone()
    };
    let missing = status(DATABASE_WATCHLIST);
    assert_eq!(missing.status, "missing");
    assert!(missing.current_version.is_none());
    let unavailable = status(DATABASE_STRATEGY);
    assert_eq!(unavailable.status, "unavailable");
    assert!(unavailable.error.contains("not a regular file"));
    let incompatible = status(DATABASE_BACKTEST);
    assert_eq!(incompatible.status, "incompatible");
    assert!(incompatible.error.contains("schema metadata is missing"));
    let ready = status(DATABASE_ADK);
    assert_eq!(ready.status, "ready");
    assert_eq!(
        ready.current_version,
        Some(fixture.descriptor(DATABASE_ADK).expected_version)
    );
}

// Parity: go:452dea11:internal/app/apiserver/datamigration/manager_boundaries_test.go:163 TestInspectDatabaseRejectsManifestDrift
#[test]
fn database_inspection_rejects_manifest_drift() {
    let fixture = Fixture::new();
    let adk = fixture.initialize(DATABASE_ADK);
    execute(
        &adk,
        "CREATE TABLE known_application_table (id TEXT PRIMARY KEY);",
    );

    let response = fixture
        .overview_service()
        .overview(
            OverviewRequest {
                summary_only: false,
                database_id: DATABASE_ADK.to_owned(),
            },
            CHECKED_AT.to_owned(),
        )
        .expect("overview");
    assert_eq!(response.databases[0].status, "incompatible");
    assert!(
        response.databases[0]
            .error
            .contains("unknown application table"),
        "manifest drift is reported: {}",
        response.databases[0].error
    );
    assert_eq!(
        response.databases[0].current_version,
        Some(fixture.descriptor(DATABASE_ADK).expected_version)
    );
}

// Parity: go:452dea11:internal/app/apiserver/datamigration/manager_test.go:75 TestManagerDescriptorsMatchSchemaCatalog
#[test]
fn managed_descriptors_match_the_pinned_schema_catalog() {
    let fixture = Fixture::new();
    assert_eq!(fixture.descriptors.len(), 9);
    for descriptor in &fixture.descriptors {
        assert!(
            !descriptor.path.is_empty(),
            "descriptor {} has no path",
            descriptor.id
        );
        let path = PathBuf::from(&descriptor.path);
        let connection = Connection::open(&path).expect("create database");
        initialize_current(&connection, &descriptor.id).expect("initialize schema");
        assert_eq!(
            jftrade_store_sqlite::current_version(&connection, &descriptor.id),
            Some(descriptor.expected_version),
            "descriptor {} must pin the schema catalog version",
            descriptor.id
        );
        drop(connection);
    }
    let artifact = fixture.descriptor(DATABASE_ADK_ARTIFACT);
    assert_eq!(
        Path::new(&artifact.path)
            .file_name()
            .and_then(|name| name.to_str()),
        Some("adk-artifact.db")
    );
    assert_eq!(
        Path::new(&artifact.path).parent(),
        Path::new(&fixture.descriptor(DATABASE_ADK_SESSION).path).parent(),
        "the artifact database lives next to the session database"
    );
}

// Parity: go:452dea11:internal/app/apiserver/datamigration/maintenance_failure_paths_test.go:552 TestMaintenanceCleanupAndCompactionReportActualReclaimedStorage
#[test]
fn cleanup_and_compaction_report_the_reclaimed_bytes_measured_on_disk() {
    let fixture = Fixture::new();
    let strategy = fixture.initialize(DATABASE_STRATEGY);
    execute(
        &strategy,
        "INSERT INTO strategy_design_definitions
             (id, script, visual_model_json, deleted_at)
         VALUES ('large-deleted', zeroblob(2097152), '{}', '2026-01-01T00:00:00Z');",
    );
    let previews = fixture.preview_service();
    let preview = previews
        .preview(CleanupPreviewRequest {
            kind: CLEANUP_SOFT_DELETED.to_owned(),
            database_id: DATABASE_STRATEGY.to_owned(),
            older_than_days: 0,
            keep_latest: 0,
        })
        .expect("cleanup preview");
    assert_eq!(preview.candidate_count, 1);

    let approved = previews
        .approved_preview(&preview.preview_id, OffsetDateTime::now_utc())
        .expect("approved preview")
        .expect("stored preview");
    let maintenance = fixture.maintenance_store();
    let result = maintenance
        .execute_cleanup(&approved)
        .expect("execute cleanup");
    assert_eq!(result.deleted_count, 1);
    assert!(result.compacted);
    assert!(result.reclaimed_bytes > 0);
    assert!(result.after_bytes < result.before_bytes);
    assert!(result.warning.is_empty());

    let watchlist = fixture.initialize(DATABASE_WATCHLIST);
    execute(
        &watchlist,
        "INSERT INTO watchlist_instruments
             (instrument_id, market, symbol, name, created_at, updated_at)
         VALUES ('US.TEST', 'US', 'TEST', zeroblob(2097152), '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');
         DELETE FROM watchlist_instruments WHERE instrument_id = 'US.TEST';",
    );
    let compact = maintenance
        .compact(DATABASE_WATCHLIST, "2026-09-21T00:00:00Z")
        .expect("compact the watchlist database");
    assert!(compact.compacted);
    assert!(compact.reclaimed_bytes > 0);
    assert!(compact.after_bytes < compact.before_bytes);
}

#[test]
fn cleanup_preview_requires_a_ready_database_with_the_purgeable_table() {
    let fixture = Fixture::new();
    let previews = fixture.preview_service();
    let request = CleanupPreviewRequest {
        kind: CLEANUP_SOFT_DELETED.to_owned(),
        database_id: DATABASE_STRATEGY.to_owned(),
        older_than_days: 0,
        keep_latest: 0,
    };
    let error = previews
        .preview(request.clone())
        .expect_err("a missing database cannot be previewed");
    assert!(
        error.to_string().contains("not ready for cleanup"),
        "unexpected missing-database error: {error}"
    );

    let strategy = fixture.initialize(DATABASE_STRATEGY);
    execute(&strategy, "DROP TABLE strategy_design_definitions;");
    let error = previews
        .preview(request)
        .expect_err("a ready database without the cleanup table is rejected");
    assert!(
        error.to_string().contains("not ready for cleanup"),
        "unexpected missing-table error: {error}"
    );
}
