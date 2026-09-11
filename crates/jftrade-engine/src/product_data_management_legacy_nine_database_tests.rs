// Isolated upgrade/recovery coverage for a published Go-compatible install.
//
// The fixture is built only under `tempdir`; it never reads or writes the
// workspace `var/` databases.  The current manifest is the Go `452dea11`
// schema baseline, while the supported legacy versions are shaped exactly as
// the explicit Rust migration table expects.  The test exercises the real
// nine-database startup owner instead of calling a store migration in
// isolation.

use std::collections::BTreeMap;
use std::fs;
use std::path::PathBuf;

use jftrade_datamanagement::{
    DatabaseDescriptor, DATABASE_ADK, DATABASE_ADK_ARTIFACT, DATABASE_ADK_SESSION,
    DATABASE_BACKTEST, DATABASE_BACKTEST_RUNS, DATABASE_EXECUTION, DATABASE_RESEARCH,
    DATABASE_STRATEGY, DATABASE_WATCHLIST,
};
use jftrade_store_sqlite::{current_version, validate_current};
use rusqlite::{Connection, OpenFlags};
use tempfile::TempDir;

use super::super::{database_descriptors, initialize_production_databases};

const COMPONENTS: [&str; 9] = [
    DATABASE_BACKTEST,
    DATABASE_BACKTEST_RUNS,
    DATABASE_STRATEGY,
    DATABASE_EXECUTION,
    DATABASE_ADK,
    DATABASE_ADK_SESSION,
    DATABASE_ADK_ARTIFACT,
    DATABASE_WATCHLIST,
    DATABASE_RESEARCH,
];

#[test]
fn legacy_nine_database_install_upgrades_and_preserves_data() {
    let source = setup_current_install();
    seed_install(&source.descriptors);

    let upgrade_root = tempfile::tempdir().expect("upgrade root");
    let upgrade_settings = copy_install(&source.descriptors, &upgrade_root);
    let upgrade_descriptors = database_descriptors(&upgrade_settings, |_| None).0;
    shape_supported_legacy_versions(&upgrade_descriptors);

    initialize_production_databases(&upgrade_settings).expect("upgrade isolated nine-db install");

    let mut observed = BTreeMap::new();
    for descriptor in &upgrade_descriptors {
        let connection = open_read_only(&descriptor.path);
        assert_eq!(
            current_version(&connection, &descriptor.id),
            Some(descriptor.expected_version),
            "{} must reach the pinned manifest version",
            descriptor.id
        );
        validate_current(
            &connection,
            &descriptor.path,
            &descriptor.id,
            descriptor.expected_version,
        )
        .unwrap_or_else(|error| panic!("{} must validate after upgrade: {error}", descriptor.id));
        observed.insert(descriptor.id.clone(), row_count(&connection, &descriptor.id));
    }

    assert_eq!(observed.len(), COMPONENTS.len());
    assert_eq!(observed[DATABASE_BACKTEST], 1);
    assert_eq!(observed[DATABASE_BACKTEST_RUNS], 1);
    assert_eq!(observed[DATABASE_STRATEGY], 1);
    assert_eq!(observed[DATABASE_EXECUTION], 1);
    assert_eq!(observed[DATABASE_ADK], 1);
    assert_eq!(observed[DATABASE_ADK_SESSION], 1);
    assert_eq!(observed[DATABASE_ADK_ARTIFACT], 1);
    assert_eq!(observed[DATABASE_WATCHLIST], 1);
    assert_eq!(observed[DATABASE_RESEARCH], 1);

    // Only components that actually cross a supported legacy boundary claim a
    // migration rollback artifact.  The other six remain byte-compatible
    // current files and are not silently rewritten.
    for id in [DATABASE_BACKTEST, DATABASE_STRATEGY, DATABASE_ADK] {
        let descriptor = descriptor(&upgrade_descriptors, id);
        assert!(
            PathBuf::from(format!("{}.pre-migration.bak", descriptor.path)).is_file(),
            "{id} must retain a verified pre-migration backup"
        );
    }
    for id in [
        DATABASE_BACKTEST_RUNS,
        DATABASE_EXECUTION,
        DATABASE_ADK_SESSION,
        DATABASE_ADK_ARTIFACT,
        DATABASE_WATCHLIST,
        DATABASE_RESEARCH,
    ] {
        let descriptor = descriptor(&upgrade_descriptors, id);
        assert!(
            !PathBuf::from(format!("{}.pre-migration.bak", descriptor.path)).exists(),
            "{id} must not claim a migration backup when no migration ran"
        );
    }
}

#[test]
fn legacy_nine_database_failure_restores_every_file_and_version() {
    let source = setup_current_install();
    seed_install(&source.descriptors);

    let failure_root = tempfile::tempdir().expect("failure root");
    let failure_settings = copy_install(&source.descriptors, &failure_root);
    let failure_descriptors = database_descriptors(&failure_settings, |_| None).0;
    shape_backtest_and_adk_legacy_versions(&failure_descriptors);
    shape_broken_strategy(&failure_descriptors);

    let before = failure_descriptors
        .iter()
        .map(|descriptor| (descriptor.id.clone(), snapshot_files(&descriptor.path)))
        .collect::<BTreeMap<_, _>>();
    let error = initialize_production_databases(&failure_settings)
        .expect_err("broken legacy strategy must abort the whole nine-db startup");
    assert!(error.contains("migration failed"), "migration context: {error}");

    for descriptor in &failure_descriptors {
        assert_eq!(
            snapshot_files(&descriptor.path),
            before[&descriptor.id],
            "startup failure must restore {} byte-for-byte",
            descriptor.id
        );
    }
    assert_eq!(
        current_version_at(&failure_descriptors, DATABASE_BACKTEST),
        Some(2),
        "the earlier backtest migration must be rolled back"
    );
    assert_eq!(
        current_version_at(&failure_descriptors, DATABASE_STRATEGY),
        Some(1),
        "the failing strategy migration must remain legacy"
    );
}

struct IsolatedInstall {
    _root: TempDir,
    descriptors: Vec<DatabaseDescriptor>,
}

fn setup_current_install() -> IsolatedInstall {
    let root = tempfile::tempdir().expect("source root");
    let settings = root.path().join("settings.json");
    fs::write(&settings, b"{}\n").expect("settings fixture");
    initialize_production_databases(&settings).expect("initialize source nine-db install");
    let descriptors = database_descriptors(&settings, |_| None).0;
    assert_eq!(descriptors.len(), COMPONENTS.len());
    IsolatedInstall {
        _root: root,
        descriptors,
    }
}

fn copy_install(source: &[DatabaseDescriptor], destination: &TempDir) -> PathBuf {
    let settings = destination.path().join("settings.json");
    fs::write(&settings, b"{}\n").expect("destination settings fixture");
    let targets = database_descriptors(&settings, |_| None).0;
    for source_descriptor in source {
        let target = descriptor(&targets, &source_descriptor.id);
        fs::copy(&source_descriptor.path, &target.path).unwrap_or_else(|error| {
            panic!(
                "copy {} to {}: {error}",
                source_descriptor.path, target.path
            )
        });
    }
    settings
}

fn seed_install(descriptors: &[DatabaseDescriptor]) {
    for descriptor in descriptors {
        let connection = Connection::open(&descriptor.path).expect("open seed database");
        match descriptor.id.as_str() {
            DATABASE_BACKTEST => connection
                .execute(
                    "INSERT INTO local_klines__manifest__symbol__1m__forward__r__00000000
                     (end_time, start_time, open, high, low, close, volume)
                     VALUES (2000, 1000, '100000000', '120000000', '90000000', '110000000', '42')",
                    [],
                )
                .expect("seed backtest candle"),
            DATABASE_BACKTEST_RUNS => connection
                .execute(
                    "INSERT INTO backtest_runs
                     (id, status, request_json, result_json, created_at, updated_at)
                     VALUES ('legacy-run', 'completed', '{}', '{\"equity\":1}', 't0', 't1')",
                    [],
                )
                .expect("seed backtest run"),
            DATABASE_STRATEGY => connection
                .execute(
                    "INSERT INTO strategy_design_definitions
                     (id, name, version, description, runtime, source_format, symbol,
                      interval, script, visual_model_json, created_at, updated_at)
                     VALUES ('legacy-definition', 'Legacy strategy', 'v1', 'fixture',
                             'pine', 'pine', 'HK.00700', '1m', 'close > open',
                             '{\"nodes\":[]}', 't0', 't1')",
                    [],
                )
                .expect("seed strategy definition"),
            DATABASE_EXECUTION => connection
                .execute(
                    "INSERT INTO execution_orders
                     (internal_order_id, broker_id, status, updated_at, created_at)
                     VALUES ('legacy-order', 'fixture', 'submitted', 't1', 't0')",
                    [],
                )
                .expect("seed execution order"),
            DATABASE_ADK => connection
                .execute(
                    "INSERT INTO adk_runs
                     (id, session_id, agent_id, status, client_request_id,
                      request_fingerprint, payload_json, created_at, updated_at)
                     VALUES ('legacy-run', 'legacy-session', 'legacy-agent', 'queued',
                             'legacy-client', 'legacy-fingerprint', '{}', 't0', 't1')",
                    [],
                )
                .expect("seed adk run"),
            DATABASE_ADK_SESSION => connection
                .execute(
                    "INSERT INTO sessions
                     (app_name, user_id, id, state, create_time, update_time)
                     VALUES ('app', 'user', 'legacy-session', '{}', 't0', 't1')",
                    [],
                )
                .expect("seed adk session"),
            DATABASE_ADK_ARTIFACT => connection
                .execute(
                    "INSERT INTO artifacts
                     (app_name, user_id, session_id, file_name, version, part_json,
                      mime_type, custom_metadata_json, created_at, updated_at)
                     VALUES ('app', 'user', 'legacy-session', 'notes', 1, '{}',
                             'application/json', NULL, 't0', 't1')",
                    [],
                )
                .expect("seed adk artifact"),
            DATABASE_WATCHLIST => connection
                .execute(
                    "INSERT INTO watchlist_groups
                     (group_id, name, name_key, is_default, protected, revision,
                      created_at, updated_at)
                     VALUES ('legacy-group', 'Legacy', 'legacy', 0, 0, 1, 't0', 't1')",
                    [],
                )
                .expect("seed watchlist group"),
            DATABASE_RESEARCH => connection
                .execute(
                    "INSERT INTO research_screen_presets
                     (preset_id, name, name_key, query_schema_version, query_json,
                      revision, created_at, updated_at)
                     VALUES ('legacy-preset', 'Legacy', 'legacy', 2, '{}', 1, 't0', 't1')",
                    [],
                )
                .expect("seed research preset"),
            other => panic!("unexpected managed database {other}"),
        };
    }
}

fn shape_supported_legacy_versions(descriptors: &[DatabaseDescriptor]) {
    shape_backtest_legacy(descriptors);
    shape_strategy_legacy(descriptors);
    shape_adk_legacy(descriptors);
}

fn shape_backtest_and_adk_legacy_versions(descriptors: &[DatabaseDescriptor]) {
    shape_backtest_legacy(descriptors);
    shape_adk_legacy(descriptors);
}

fn shape_backtest_legacy(descriptors: &[DatabaseDescriptor]) {
    let backtest = descriptor(descriptors, DATABASE_BACKTEST);
    Connection::open(&backtest.path)
        .expect("open backtest legacy fixture")
        .execute_batch(
            "ALTER TABLE local_klines__manifest__symbol__1m__forward__r__00000000
                 RENAME TO local_klines__manifest__1m__forward__r__00000000;
             CREATE TABLE local_klines__hk_00700__1m__forward__r__0c59bfa3 (
                 end_time INTEGER NOT NULL,
                 start_time INTEGER NOT NULL,
                 open TEXT NOT NULL,
                 high TEXT NOT NULL,
                 low TEXT NOT NULL,
                 close TEXT NOT NULL,
                 volume TEXT NOT NULL,
                 PRIMARY KEY (end_time)
             ) WITHOUT ROWID;
             INSERT INTO local_klines__hk_00700__1m__forward__r__0c59bfa3
                 (end_time, start_time, open, high, low, close, volume)
                 VALUES (3000, 2000, '200000000', '220000000', '190000000',
                         '210000000', '7');
             UPDATE jftrade_schema_meta SET version = 2 WHERE component_id = 'backtest';",
        )
        .expect("shape backtest v2");
}

fn shape_strategy_legacy(descriptors: &[DatabaseDescriptor]) {
    let strategy = descriptor(descriptors, DATABASE_STRATEGY);
    Connection::open(&strategy.path)
        .expect("open strategy legacy fixture")
        .execute_batch(
            "DROP TRIGGER trg_strategy_definition_versions_immutable;
             DROP INDEX idx_strategy_definition_versions_saved_at;
             DROP TABLE strategy_definition_versions;
             UPDATE jftrade_schema_meta SET version = 1 WHERE component_id = 'strategy';",
        )
        .expect("shape strategy v1");
}

fn shape_adk_legacy(descriptors: &[DatabaseDescriptor]) {
    let adk = descriptor(descriptors, DATABASE_ADK);
    Connection::open(&adk.path)
        .expect("open adk legacy fixture")
        .execute_batch(
            "DROP INDEX idx_adk_runs_client_request;
             DROP INDEX idx_adk_runs_session;
             DROP TABLE adk_runs;
             CREATE TABLE adk_runs (
                 id TEXT PRIMARY KEY,
                 session_id TEXT NOT NULL,
                 agent_id TEXT NOT NULL,
                 status TEXT NOT NULL,
                 payload_json TEXT NOT NULL,
                 created_at TEXT NOT NULL,
                 updated_at TEXT NOT NULL
             );
             INSERT INTO adk_runs
                 (id, session_id, agent_id, status, payload_json, created_at, updated_at)
                 VALUES ('legacy-run', 'legacy-session', 'legacy-agent', 'queued', '{}', 't0', 't1');
             UPDATE jftrade_schema_meta SET version = 2 WHERE component_id = 'adk';",
        )
        .expect("shape adk v2");
}

fn shape_broken_strategy(descriptors: &[DatabaseDescriptor]) {
    let strategy = descriptor(descriptors, DATABASE_STRATEGY);
    Connection::open(&strategy.path)
        .expect("open broken strategy fixture")
        .execute_batch(
            "DROP TRIGGER trg_strategy_definition_versions_immutable;
             DROP INDEX idx_strategy_definition_versions_saved_at;
             DROP TABLE strategy_definition_versions;
             CREATE TABLE strategy_definition_versions (broken TEXT);
             UPDATE jftrade_schema_meta SET version = 1 WHERE component_id = 'strategy';",
        )
        .expect("shape broken strategy v1");
}

fn descriptor<'a>(descriptors: &'a [DatabaseDescriptor], id: &str) -> &'a DatabaseDescriptor {
    descriptors
        .iter()
        .find(|descriptor| descriptor.id == id)
        .unwrap_or_else(|| panic!("missing descriptor {id}"))
}

fn open_read_only(path: &str) -> Connection {
    Connection::open_with_flags(path, OpenFlags::SQLITE_OPEN_READ_ONLY)
        .expect("open database read-only")
}

fn row_count(connection: &Connection, id: &str) -> i64 {
    let table = if id == DATABASE_BACKTEST {
        connection
            .query_row(
                "SELECT name FROM sqlite_master
                 WHERE type = 'table' AND name LIKE 'local_klines__futu__%'
                 ORDER BY name LIMIT 1",
                [],
                |row| row.get::<_, String>(0),
            )
            .expect("find migrated K-line table")
    } else {
        match id {
            DATABASE_BACKTEST_RUNS => "backtest_runs".to_owned(),
            DATABASE_STRATEGY => "strategy_design_definitions".to_owned(),
            DATABASE_EXECUTION => "execution_orders".to_owned(),
            DATABASE_ADK => "adk_runs".to_owned(),
            DATABASE_ADK_SESSION => "sessions".to_owned(),
            DATABASE_ADK_ARTIFACT => "artifacts".to_owned(),
            DATABASE_WATCHLIST => "watchlist_groups".to_owned(),
            DATABASE_RESEARCH => "research_screen_presets".to_owned(),
            other => panic!("unexpected database {other}"),
        }
    };
    let table = table.replace('"', "\"\"");
    let sql = format!("SELECT COUNT(*) FROM \"{table}\"");
    connection
        .query_row(&sql, [], |row| row.get(0))
        .unwrap_or_else(|error| panic!("count {id}: {error}"))
}

fn current_version_at(descriptors: &[DatabaseDescriptor], id: &str) -> Option<i64> {
    let descriptor = descriptor(descriptors, id);
    let connection = open_read_only(&descriptor.path);
    current_version(&connection, id)
}

fn snapshot_files(path: &str) -> Vec<(PathBuf, Vec<u8>)> {
    ["", "-wal", "-shm", "-journal"]
        .into_iter()
        .map(|suffix| PathBuf::from(format!("{path}{suffix}")))
        .filter_map(|path| fs::read(&path).ok().map(|bytes| (path, bytes)))
        .collect()
}
