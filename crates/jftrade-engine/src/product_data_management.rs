use std::env;
use std::fs;
use std::path::Path;
use std::sync::Arc;

use jftrade_datamanagement::{
    CleanupPreviewIdPort, CleanupPreviewService, DATABASE_ADK, DATABASE_ADK_ARTIFACT,
    DATABASE_ADK_SESSION, DATABASE_BACKTEST, DATABASE_BACKTEST_RUNS, DATABASE_EXECUTION,
    DATABASE_RESEARCH, DATABASE_STRATEGY, DATABASE_WATCHLIST, MaintenanceService,
    ManagedDatabasePaths, OverviewService, managed_database_descriptors,
};
use jftrade_owner_lock::{OwnerDiagnostic, WriterLease};
use jftrade_store_sqlite::{
    ManagedDatabaseCleanupCandidateStore, ManagedDatabaseMaintenanceStore,
    ManagedDatabaseOverviewStore, initialize_current,
};
use rusqlite::{Connection, MAIN_DB, OpenFlags, Transaction, TransactionBehavior};

const REBUILD_MARKER_FILENAME: &str = "database-rebuild.json";

pub fn overview_service(settings_path: &Path) -> OverviewService {
    overview_service_with_lookup(settings_path, |name| env::var(name).ok())
}

pub(crate) fn managed_database_runtime_descriptors(
    settings_path: &Path,
) -> Vec<jftrade_datamanagement::DatabaseDescriptor> {
    database_descriptors(settings_path, |name| env::var(name).ok()).0
}

pub fn cleanup_preview_service(settings_path: &Path) -> Arc<CleanupPreviewService> {
    Arc::new(cleanup_preview_service_with_lookup(settings_path, |name| {
        env::var(name).ok()
    }))
}

#[allow(dead_code)]
pub fn maintenance_service(
    settings_path: &Path,
    previews: Arc<CleanupPreviewService>,
) -> MaintenanceService {
    maintenance_service_with_profile(
        settings_path,
        previews,
        crate::product::PRODUCT_PRODUCTION_ROUTE_PROFILE,
    )
}

pub fn maintenance_service_with_profile(
    settings_path: &Path,
    previews: Arc<CleanupPreviewService>,
    profile: &str,
) -> MaintenanceService {
    let (descriptors, marker_path) =
        database_descriptors(settings_path, |name| env::var(name).ok());
    MaintenanceService::new(
        previews,
        Arc::new(ManagedDatabaseMaintenanceStore::new(
            descriptors,
            marker_path,
            profile,
        )),
    )
}

/// Open or create all managed production databases without replacing an
/// existing file.  Existing schemas are validated against the pinned
/// manifest; incompatibilities fail startup and leave the original untouched.
pub fn initialize_production_databases(settings_path: &Path) -> Result<(), String> {
    let (descriptors, _) = database_descriptors(settings_path, |name| env::var(name).ok());
    for descriptor in descriptors {
        let path = Path::new(&descriptor.path);
        if let Some(parent) = path
            .parent()
            .filter(|parent| !parent.as_os_str().is_empty())
        {
            fs::create_dir_all(parent)
                .map_err(|error| format!("create {}: {error}", parent.display()))?;
        }
        let existed = match fs::symlink_metadata(path) {
            Ok(metadata) if metadata.file_type().is_symlink() || !metadata.is_file() => {
                return Err(format!(
                    "managed database {} is not a regular file",
                    path.display()
                ));
            }
            Ok(_) => true,
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => false,
            Err(error) => {
                return Err(format!(
                    "inspect managed database {}: {error}",
                    path.display()
                ));
            }
        };
        let _writer_lease = WriterLease::acquire(
            path,
            &OwnerDiagnostic::current("rust", "production-migration.v1"),
        )
        .map_err(|error| {
            format!(
                "acquire startup writer lease for {}: {error}",
                path.display()
            )
        })?;
        let connection = Connection::open_with_flags(
            path,
            OpenFlags::SQLITE_OPEN_READ_WRITE | OpenFlags::SQLITE_OPEN_CREATE,
        )
        .map_err(|error| format!("open {}: {error}", path.display()))?;
        if existed {
            match jftrade_store_sqlite::validate_current(
                &connection,
                &descriptor.path,
                &descriptor.id,
                descriptor.expected_version,
            ) {
                Ok(()) => {}
                Err(error)
                    if jftrade_store_sqlite::current_version(&connection, &descriptor.id)
                        .is_some_and(|version| version < descriptor.expected_version) =>
                {
                    let from_version =
                        jftrade_store_sqlite::current_version(&connection, &descriptor.id)
                            .ok_or_else(|| {
                                format!(
                                    "{}; schema metadata version disappeared before migration",
                                    error
                                )
                            })?;
                    migrate_legacy_schema(
                        &connection,
                        &descriptor.path,
                        &descriptor.id,
                        from_version,
                        descriptor.expected_version,
                    )
                    .map_err(|migration_error| {
                        format!("{}; migration failed: {migration_error}", error)
                    })?;
                }
                Err(error) => return Err(error.to_string()),
            }
        } else {
            initialize_current(&connection, &descriptor.id).map_err(|error| error.to_string())?;
        }
    }
    Ok(())
}

fn migrate_legacy_schema(
    connection: &Connection,
    path: &str,
    component: &str,
    from_version: i64,
    expected_version: i64,
) -> Result<(), String> {
    create_verified_migration_backup(
        connection,
        path,
        &format!("{path}.pre-migration.bak"),
        component,
        from_version,
    )?;
    let transaction = Transaction::new_unchecked(connection, TransactionBehavior::Immediate)
        .map_err(|error| format!("begin immediate schema migration: {error}"))?;
    jftrade_store_sqlite::migrate_legacy_schema(
        &transaction,
        path,
        component,
        from_version,
        expected_version,
    )
    .map_err(|error| format!("apply schema migration: {error}"))?;
    if let Err(error) =
        jftrade_store_sqlite::validate_current(&transaction, path, component, expected_version)
    {
        drop(transaction);
        return Err(error.to_string());
    }
    transaction
        .commit()
        .map_err(|error| format!("commit metadata migration: {error}"))?;
    Ok(())
}

fn create_verified_migration_backup(
    source: &Connection,
    path: &str,
    backup_path: &str,
    component: &str,
    from_version: i64,
) -> Result<(), String> {
    verify_sqlite_integrity(source, path)?;
    let parent = Path::new(backup_path)
        .parent()
        .filter(|parent| !parent.as_os_str().is_empty())
        .unwrap_or_else(|| Path::new("."));
    let temporary = tempfile::NamedTempFile::new_in(parent).map_err(|error| {
        format!(
            "create temporary migration backup in {}: {error}",
            parent.display()
        )
    })?;
    source
        .backup(MAIN_DB, temporary.path(), None)
        .map_err(|error| {
            format!(
                "backup migration source {path} to {}: {error}",
                temporary.path().display()
            )
        })?;
    temporary
        .as_file()
        .sync_all()
        .map_err(|error| format!("sync temporary migration backup: {error}"))?;
    let backup = Connection::open_with_flags(temporary.path(), OpenFlags::SQLITE_OPEN_READ_ONLY)
        .map_err(|error| format!("open temporary migration backup: {error}"))?;
    verify_sqlite_integrity(&backup, &temporary.path().display().to_string())?;
    if jftrade_store_sqlite::current_version(&backup, component) != Some(from_version) {
        return Err(format!(
            "temporary migration backup metadata does not match {component} version {from_version}"
        ));
    }
    drop(backup);
    temporary
        .persist(backup_path)
        .map_err(|error| format!("atomically persist migration backup {backup_path}: {error}"))?;
    Ok(())
}

fn verify_sqlite_integrity(connection: &Connection, path: &str) -> Result<(), String> {
    let mut statement = connection
        .prepare("PRAGMA quick_check")
        .map_err(|error| format!("prepare quick_check for {path}: {error}"))?;
    let results = statement
        .query_map([], |row| row.get::<_, String>(0))
        .map_err(|error| format!("run quick_check for {path}: {error}"))?
        .collect::<Result<Vec<_>, _>>()
        .map_err(|error| format!("read quick_check for {path}: {error}"))?;
    if results.is_empty()
        || !results
            .iter()
            .all(|result| result.trim().eq_ignore_ascii_case("ok"))
    {
        return Err(format!(
            "quick_check failed for {path}: {}",
            results.join(", ")
        ));
    }
    let foreign_key_violation = connection
        .query_row(
            "SELECT EXISTS(SELECT 1 FROM pragma_foreign_key_check)",
            [],
            |row| row.get::<_, i64>(0),
        )
        .map_err(|error| format!("run foreign_key_check for {path}: {error}"))?;
    if foreign_key_violation != 0 {
        return Err(format!("foreign_key_check failed for {path}"));
    }
    Ok(())
}

fn overview_service_with_lookup(
    settings_path: &Path,
    lookup: impl Fn(&str) -> Option<String>,
) -> OverviewService {
    let (descriptors, marker_path) = database_descriptors(settings_path, lookup);
    OverviewService::new(
        descriptors,
        Arc::new(ManagedDatabaseOverviewStore::new(marker_path)),
    )
}

fn cleanup_preview_service_with_lookup(
    settings_path: &Path,
    lookup: impl Fn(&str) -> Option<String>,
) -> CleanupPreviewService {
    let (descriptors, marker_path) = database_descriptors(settings_path, lookup);
    let overview = Arc::new(ManagedDatabaseOverviewStore::new(marker_path));
    CleanupPreviewService::new(
        descriptors,
        overview,
        Arc::new(ManagedDatabaseCleanupCandidateStore),
        Arc::new(SystemCleanupPreviewIds),
    )
}

fn database_descriptors(
    settings_path: &Path,
    lookup: impl Fn(&str) -> Option<String>,
) -> (
    Vec<jftrade_datamanagement::DatabaseDescriptor>,
    std::path::PathBuf,
) {
    let root = settings_path
        .parent()
        .filter(|path| !path.as_os_str().is_empty())
        .unwrap_or_else(|| Path::new(""));
    let path = |environment: &str, filename: &str| {
        lookup(environment)
            .filter(|value| !value.trim().is_empty())
            .map(|value| value.trim().to_owned())
            .unwrap_or_else(|| root.join(filename).to_string_lossy().into_owned())
    };
    let adk_session = path("JFTRADE_ADK_SESSION_DB", "adk-session.db");
    let adk_artifact = Path::new(&adk_session)
        .parent()
        .filter(|parent| !parent.as_os_str().is_empty())
        .unwrap_or_else(|| Path::new(""))
        .join("adk-artifact.db")
        .to_string_lossy()
        .into_owned();
    let paths = ManagedDatabasePaths::new([
        (
            DATABASE_BACKTEST,
            path("JFTRADE_BACKTEST_DB", "backtest.db"),
        ),
        (
            DATABASE_BACKTEST_RUNS,
            path("JFTRADE_BACKTEST_RUN_DB", "backtest-runs.db"),
        ),
        (
            DATABASE_STRATEGY,
            path("JFTRADE_STRATEGY_RUNTIME_DB", "strategy-runtime.db"),
        ),
        (
            DATABASE_EXECUTION,
            path("JFTRADE_EXECUTION_ORDER_DB", "execution-orders.db"),
        ),
        (DATABASE_ADK, path("JFTRADE_ADK_DB", "adk.db")),
        (DATABASE_ADK_SESSION, adk_session),
        (DATABASE_ADK_ARTIFACT, adk_artifact),
        (
            DATABASE_WATCHLIST,
            path("JFTRADE_WATCHLIST_DB", "watchlists.db"),
        ),
        (
            DATABASE_RESEARCH,
            path("JFTRADE_RESEARCH_DB", "research.db"),
        ),
    ]);
    (
        managed_database_descriptors(&paths),
        root.join(REBUILD_MARKER_FILENAME),
    )
}

#[derive(Debug, Default)]
struct SystemCleanupPreviewIds;

impl CleanupPreviewIdPort for SystemCleanupPreviewIds {
    fn new_preview_id(&self) -> Result<String, String> {
        let mut bytes = [0_u8; 16];
        getrandom::fill(&mut bytes).map_err(|error| error.to_string())?;
        let mut id = String::with_capacity(bytes.len() * 2);
        for byte in bytes {
            use std::fmt::Write as _;
            write!(&mut id, "{byte:02x}").map_err(|error| error.to_string())?;
        }
        Ok(id)
    }
}

#[cfg(test)]
mod tests {
    use std::collections::BTreeMap;
    use std::path::PathBuf;
    use std::str::FromStr;
    use std::sync::Arc;

    use jftrade_datamanagement::{CleanupPreviewRequest, OverviewRequest};
    use jftrade_kernel::WireTimestamp;
    use jftrade_store_sqlite::ManagedDatabaseCleanupCandidateStore;
    use serde::Deserialize;
    use serde_json::{Value, json};

    use super::*;

    #[test]
    fn paths_follow_go_environment_overrides_and_adk_artifact_lifecycle() {
        let overrides = BTreeMap::from([
            ("JFTRADE_BACKTEST_DB", "/override/backtest.db".to_owned()),
            (
                "JFTRADE_ADK_SESSION_DB",
                "/override/adk/session.db".to_owned(),
            ),
        ]);
        let service = overview_service_with_lookup(Path::new("/runtime/settings.json"), |name| {
            overrides.get(name).cloned()
        });
        let response = service
            .overview(
                OverviewRequest {
                    summary_only: true,
                    ..OverviewRequest::default()
                },
                "2026-08-20T00:00:00Z".to_owned(),
            )
            .expect("overview");
        let by_id = response
            .databases
            .into_iter()
            .map(|database| (database.descriptor.id.clone(), database.descriptor.path))
            .collect::<BTreeMap<_, _>>();
        assert_eq!(by_id[DATABASE_BACKTEST], "/override/backtest.db");
        assert_eq!(by_id[DATABASE_STRATEGY], "/runtime/strategy-runtime.db");
        assert_eq!(by_id[DATABASE_ADK_SESSION], "/override/adk/session.db");
        assert_eq!(
            by_id[DATABASE_ADK_ARTIFACT],
            "/override/adk/adk-artifact.db"
        );
    }

    #[test]
    fn stage9_data_management_overview_matches_current_go_owner() {
        let Some(root) = std::env::var_os("JFTRADE_STAGE9_DATA_MANAGEMENT_ROOT") else {
            return;
        };
        let Some(reference_path) = std::env::var_os("JFTRADE_STAGE9_DATA_MANAGEMENT_REFERENCE")
        else {
            return;
        };
        let root = Path::new(&root);
        let service = overview_service_with_lookup(&root.join("settings.json"), |_| None);
        let checked_at = "2026-08-20T00:00:00Z".to_owned();
        let all = service
            .overview(OverviewRequest::default(), checked_at.clone())
            .expect("complete overview");
        let summary = service
            .overview(
                OverviewRequest {
                    summary_only: true,
                    ..OverviewRequest::default()
                },
                checked_at.clone(),
            )
            .expect("summary overview");
        let filtered = service
            .overview(
                OverviewRequest {
                    database_id: DATABASE_STRATEGY.to_owned(),
                    ..OverviewRequest::default()
                },
                checked_at.clone(),
            )
            .expect("filtered overview");
        let unknown_error = service
            .overview(
                OverviewRequest {
                    database_id: "unknown".to_owned(),
                    ..OverviewRequest::default()
                },
                checked_at,
            )
            .expect_err("unknown database must fail")
            .to_string();
        let actual = json!({
            "version": "stage9.data-management-overview.v1",
            "all": all,
            "summary": summary,
            "filtered": filtered,
            "unknownError": unknown_error,
        });
        let expected: Value = serde_json::from_slice(
            &std::fs::read(reference_path).expect("read Go data-management reference"),
        )
        .expect("decode Go data-management reference");
        assert_eq!(actual, expected);
    }

    #[derive(Debug, Deserialize)]
    #[serde(rename_all = "camelCase")]
    struct CleanupPreviewReference {
        version: String,
        cases: Vec<CleanupPreviewReferenceCase>,
    }

    #[derive(Debug, Deserialize)]
    #[serde(rename_all = "camelCase")]
    struct CleanupPreviewReferenceCase {
        name: String,
        request: CleanupPreviewRequest,
        evaluated_at: Option<String>,
        response: Option<Value>,
        error: Option<String>,
    }

    #[test]
    fn stage9_data_management_cleanup_preview_matches_current_go_owner() {
        let Some(root) = std::env::var_os("JFTRADE_STAGE9_DATA_MANAGEMENT_CLEANUP_ROOT") else {
            return;
        };
        let Some(reference_path) =
            std::env::var_os("JFTRADE_STAGE9_DATA_MANAGEMENT_CLEANUP_REFERENCE")
        else {
            return;
        };
        let root = std::path::Path::new(&root);
        let settings_path = root.join("settings.json");
        let (descriptors, marker_path) = database_descriptors(&settings_path, |_| None);
        let overview = Arc::new(ManagedDatabaseOverviewStore::new(marker_path));
        let service = jftrade_datamanagement::CleanupPreviewService::new(
            descriptors,
            overview,
            Arc::new(ManagedDatabaseCleanupCandidateStore),
            Arc::new(TestCleanupPreviewIds),
        );
        let reference: CleanupPreviewReference = serde_json::from_slice(
            &std::fs::read(reference_path).expect("read Go cleanup preview reference"),
        )
        .expect("decode Go cleanup preview reference");
        assert_eq!(
            reference.version,
            "stage9.data-management-cleanup-preview.v1"
        );
        assert!(reference.cases.len() >= 7);

        for case in reference.cases {
            let result = match case.response {
                Some(expected) => {
                    let evaluated_at = case
                        .evaluated_at
                        .as_deref()
                        .expect("valid cleanup case has evaluatedAt");
                    let now = WireTimestamp::from_str(evaluated_at)
                        .expect("parse cleanup evaluation instant")
                        .into_inner();
                    let response = service
                        .preview_at(case.request, now)
                        .unwrap_or_else(|error| panic!("preview {}: {error}", case.name));
                    assert_eq!(response.preview_id.len(), 32, "preview {} id", case.name);
                    assert!(
                        response
                            .preview_id
                            .bytes()
                            .all(|byte| byte.is_ascii_hexdigit()),
                        "preview {} id is not hexadecimal",
                        case.name
                    );
                    let expires_at = WireTimestamp::from_str(&response.expires_at)
                        .expect("parse cleanup expiry")
                        .into_inner();
                    assert_eq!(
                        expires_at.unix_timestamp() - now.unix_timestamp(),
                        10 * 60,
                        "preview {} expiry lifetime",
                        case.name
                    );
                    let mut actual =
                        serde_json::to_value(response).expect("encode cleanup response");
                    actual["previewId"] = expected["previewId"].clone();
                    assert_eq!(actual, expected, "cleanup preview case {}", case.name);
                    None
                }
                None => Some(
                    service
                        .preview(case.request)
                        .expect_err("invalid cleanup preview should fail")
                        .to_string(),
                ),
            };
            if let Some(actual_error) = result {
                assert_eq!(
                    Some(actual_error),
                    case.error,
                    "cleanup preview error case {}",
                    case.name
                );
            }
        }
    }

    #[test]
    fn production_database_metadata_migration_is_atomic_and_backed_up() {
        let directory = tempfile::tempdir().expect("temporary directory");
        let settings_path = directory.path().join("settings.json");
        fs::write(&settings_path, b"{}").expect("settings");
        initialize_production_databases(&settings_path).expect("initialize databases");
        let descriptor = database_descriptors(&settings_path, |_| None)
            .0
            .into_iter()
            .find(|descriptor| descriptor.id == DATABASE_STRATEGY)
            .expect("strategy descriptor");
        let connection = Connection::open(&descriptor.path).expect("open watchlist");
        connection
            .execute(
                "UPDATE jftrade_schema_meta SET version = 1 WHERE component_id = ?1",
                [&descriptor.id],
            )
            .expect("downgrade metadata for migration test");
        drop(connection);

        initialize_production_databases(&settings_path).expect("migrate metadata");
        let migrated = Connection::open(&descriptor.path).expect("reopen watchlist");
        assert_eq!(
            jftrade_store_sqlite::current_version(&migrated, &descriptor.id),
            Some(descriptor.expected_version)
        );
        assert!(std::path::Path::new(&(descriptor.path.clone() + ".pre-migration.bak")).is_file());
    }

    #[test]
    fn production_database_startup_leases_before_creating_database() {
        let directory = tempfile::tempdir().expect("temporary directory");
        let settings_path = directory.path().join("settings.json");
        fs::write(&settings_path, b"{}").expect("settings");
        let descriptor = database_descriptors(&settings_path, |_| None)
            .0
            .into_iter()
            .find(|descriptor| descriptor.id == DATABASE_BACKTEST)
            .expect("backtest descriptor");
        let path = Path::new(&descriptor.path);
        let _lease = WriterLease::acquire(path, &OwnerDiagnostic::current("test", "startup-lease"))
            .expect("hold startup lease");

        let error = initialize_production_databases(&settings_path)
            .expect_err("startup must reject a held writer lease");
        assert!(error.contains("acquire startup writer lease"));
        assert!(
            !path.exists(),
            "database must not be created before leasing"
        );
    }

    #[test]
    fn failed_production_migration_preserves_wal_and_shm_bytes() {
        let directory = tempfile::tempdir().expect("temporary directory");
        let settings_path = directory.path().join("settings.json");
        fs::write(&settings_path, b"{}").expect("settings");
        initialize_production_databases(&settings_path).expect("initialize databases");
        let descriptor = database_descriptors(&settings_path, |_| None)
            .0
            .into_iter()
            .find(|descriptor| descriptor.id == DATABASE_STRATEGY)
            .expect("strategy descriptor");
        let path = Path::new(&descriptor.path);
        let anchor = Connection::open(path).expect("open strategy database");
        anchor
            .execute_batch(
                "PRAGMA journal_mode = WAL;
                 PRAGMA wal_autocheckpoint = 0;
                 DROP TRIGGER trg_strategy_definition_versions_immutable;
                 DROP INDEX idx_strategy_definition_versions_saved_at;
                 DROP TABLE strategy_definition_versions;
                 CREATE TABLE strategy_definition_versions (broken TEXT);
                 UPDATE jftrade_schema_meta SET version = 1 WHERE component_id = 'strategy';",
            )
            .expect("shape malformed WAL schema");
        let sidecars = ["", "-wal", "-shm"]
            .into_iter()
            .map(|suffix| {
                let sidecar = PathBuf::from(format!("{}{suffix}", path.display()));
                (
                    sidecar.clone(),
                    fs::read(&sidecar).expect("read source bytes"),
                )
            })
            .collect::<Vec<_>>();

        let error = initialize_production_databases(&settings_path)
            .expect_err("malformed migration must fail");
        assert!(error.contains("migration failed"));
        for (sidecar, before) in sidecars {
            assert_eq!(
                fs::read(&sidecar).expect("read source bytes after failure"),
                before,
                "migration failure changed {}",
                sidecar.display()
            );
        }
        drop(anchor);
    }

    #[derive(Debug, Default)]
    struct TestCleanupPreviewIds;

    impl jftrade_datamanagement::CleanupPreviewIdPort for TestCleanupPreviewIds {
        fn new_preview_id(&self) -> Result<String, String> {
            Ok("0123456789abcdef0123456789abcdef".to_owned())
        }
    }
}
