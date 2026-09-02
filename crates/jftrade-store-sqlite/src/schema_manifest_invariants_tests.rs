use std::fs;
use std::path::{Path, PathBuf};

use rusqlite::{Connection, OpenFlags};
use tempfile::{TempDir, tempdir};

use super::*;

fn database_path(root: &TempDir, component: &str) -> PathBuf {
    root.path().join(format!("{component}.db"))
}

fn create_database(
    root: &TempDir,
    definition: &Definition,
    replace: Option<(&str, &str)>,
) -> PathBuf {
    let path = database_path(root, &definition.id);
    let connection = Connection::open(&path).expect("create fixture database");
    for statement in definition.statements.as_deref().unwrap_or_default() {
        let statement = match replace {
            Some((old, new)) if statement.contains(old) => statement.replacen(old, new, 1),
            _ => statement.clone(),
        };
        connection
            .execute_batch(&statement)
            .expect("apply fixture schema statement");
    }
    if let Some(dynamic) = &definition.dynamic_table {
        let statement = match replace {
            Some((old, new)) if dynamic.statement.contains(old) => {
                dynamic.statement.replacen(old, new, 1)
            }
            _ => dynamic.statement.clone(),
        };
        connection
            .execute_batch(&statement)
            .expect("apply dynamic fixture statement");
    }
    connection
        .execute_batch(
            "CREATE TABLE jftrade_schema_meta (component_id TEXT PRIMARY KEY, version INTEGER NOT NULL, created_at TEXT NOT NULL)",
        )
        .expect("create schema metadata");
    connection
        .execute(
            "INSERT INTO jftrade_schema_meta (component_id, version, created_at) VALUES (?1, ?2, 'fixture')",
            (&definition.id, definition.version),
        )
        .expect("insert schema metadata");
    connection.close().expect("close fixture writer");
    path
}

fn mutate(path: &Path, action: impl FnOnce(&Connection)) {
    let connection = Connection::open(path).expect("open fixture writer");
    action(&connection);
    connection.close().expect("close fixture writer");
}

fn open_read_only(path: &Path) -> Connection {
    let connection = Connection::open_with_flags(
        path,
        OpenFlags::SQLITE_OPEN_READ_ONLY | OpenFlags::SQLITE_OPEN_NO_MUTEX,
    )
    .expect("open fixture read-only");
    connection
        .execute_batch("PRAGMA query_only = ON; PRAGMA foreign_keys = ON;")
        .expect("configure fixture read-only");
    connection
}

fn sidecar(path: &Path, suffix: &str) -> PathBuf {
    let mut value = path.as_os_str().to_os_string();
    value.push(suffix);
    value.into()
}

fn snapshot_files(path: &Path) -> Vec<(PathBuf, Option<Vec<u8>>)> {
    [
        path.to_path_buf(),
        sidecar(path, "-wal"),
        sidecar(path, "-shm"),
    ]
    .into_iter()
    .map(|file| {
        let contents = fs::read(&file).ok();
        (file, contents)
    })
    .collect()
}

fn validate(path: &Path, definition: &Definition) -> Result<(), SchemaManifestError> {
    let connection = open_read_only(path);
    validate_current(
        &connection,
        &path.to_string_lossy(),
        &definition.id,
        definition.version,
    )
}

fn assert_incompatible(result: Result<(), SchemaManifestError>, reason: &str) {
    match result {
        Err(SchemaManifestError::Incompatible { reason: actual, .. }) => assert!(
            actual.contains(reason),
            "expected reason containing {reason:?}, got {actual:?}"
        ),
        other => panic!("expected incompatible schema containing {reason:?}, got {other:?}"),
    }
}

#[test]
fn all_managed_schemas_validate_without_mutating_database_or_sidecar_bytes() {
    let root = tempdir().expect("tempdir");
    for definition in &catalog().expect("catalog").definitions {
        let path = create_database(&root, definition, None);
        let before = snapshot_files(&path);
        validate(&path, definition).expect("validate exact managed schema");
        assert_eq!(
            snapshot_files(&path),
            before,
            "{} was mutated",
            definition.id
        );
    }
}

#[test]
fn metadata_errors_take_precedence_for_every_managed_schema() {
    for definition in &catalog().expect("catalog").definitions {
        let root = tempdir().expect("tempdir");
        let path = create_database(&root, definition, None);
        mutate(&path, |connection| {
            connection
                .execute_batch("DROP TABLE jftrade_schema_meta")
                .expect("drop metadata");
        });
        assert_incompatible(validate(&path, definition), "schema metadata is missing");

        let root = tempdir().expect("tempdir");
        let path = create_database(&root, definition, None);
        mutate(&path, |connection| {
            connection
                .execute(
                    "UPDATE jftrade_schema_meta SET component_id = 'wrong-component'",
                    [],
                )
                .expect("replace component metadata");
        });
        assert_incompatible(validate(&path, definition), "component metadata is missing");

        let root = tempdir().expect("tempdir");
        let path = create_database(&root, definition, None);
        mutate(&path, |connection| {
            connection
                .execute("UPDATE jftrade_schema_meta SET version = version + 1", [])
                .expect("change metadata version");
            connection
                .execute(
                    "INSERT INTO jftrade_schema_meta VALUES ('extra-component', 1, 'fixture')",
                    [],
                )
                .expect("insert extra metadata");
        });
        assert_incompatible(validate(&path, definition), "schema version");

        let root = tempdir().expect("tempdir");
        let path = create_database(&root, definition, None);
        mutate(&path, |connection| {
            connection
                .execute(
                    "INSERT INTO jftrade_schema_meta VALUES ('extra-component', 1, 'fixture')",
                    [],
                )
                .expect("insert extra metadata");
        });
        assert_incompatible(validate(&path, definition), "exactly one is required");
    }
}

#[test]
fn static_tables_columns_indexes_and_options_reject_drift() {
    let runs = definition("backtest-runs").expect("backtest-runs definition");

    let root = tempdir().expect("tempdir");
    let path = create_database(&root, runs, None);
    mutate(&path, |connection| {
        connection
            .execute_batch("DROP TABLE backtest_runs")
            .expect("drop required table");
    });
    assert_incompatible(validate(&path, runs), "required table is missing");

    let root = tempdir().expect("tempdir");
    let path = create_database(&root, runs, None);
    mutate(&path, |connection| {
        connection
            .execute_batch("CREATE TABLE rogue_static_table (id INTEGER)")
            .expect("create rogue table");
    });
    assert_incompatible(validate(&path, runs), "unknown application table");

    for (old, new) in [
        (
            "status TEXT NOT NULL DEFAULT ''",
            "status BLOB NOT NULL DEFAULT ''",
        ),
        ("updated_at DESC, id ASC", "updated_at ASC, id ASC"),
        ("\n\t\t)", "\n\t\t) STRICT"),
    ] {
        let root = tempdir().expect("tempdir");
        let path = create_database(&root, runs, Some((old, new)));
        assert_incompatible(validate(&path, runs), "structure does not match");
    }

    let execution = definition("execution-orders").expect("execution-orders definition");
    let root = tempdir().expect("tempdir");
    let path = create_database(
        &root,
        execution,
        Some((
            " WHERE client_order_id IS NOT NULL AND TRIM(client_order_id) <> ''",
            "",
        )),
    );
    assert_incompatible(validate(&path, execution), "structure does not match");
}

#[test]
fn dynamic_table_names_shapes_and_options_are_enforced() {
    let definition = definition("backtest").expect("backtest definition");
    let dynamic = definition.dynamic_table.as_ref().expect("dynamic table");
    let valid_name = "local_klines__us__aapl__1m__forward__r__deadbeef";

    let root = tempdir().expect("tempdir");
    let path = create_database(&root, definition, None);
    mutate(&path, |connection| {
        connection
            .execute_batch(
                &dynamic
                    .statement
                    .replacen(&dynamic.prototype_name, valid_name, 1),
            )
            .expect("create valid dynamic table");
    });
    validate(&path, definition).expect("accept valid dynamic table");

    for (name, statement) in [
        (
            "local_klines__US__aapl__1m__forward__r__deadbeef",
            dynamic.statement.clone(),
        ),
        (
            valid_name,
            dynamic
                .statement
                .replacen("volume TEXT NOT NULL", "volume INTEGER NOT NULL", 1),
        ),
        (
            valid_name,
            dynamic.statement.replacen(" WITHOUT ROWID", "", 1),
        ),
    ] {
        let root = tempdir().expect("tempdir");
        let path = create_database(&root, definition, None);
        mutate(&path, |connection| {
            connection
                .execute_batch(&statement.replacen(&dynamic.prototype_name, name, 1))
                .expect("create drifted dynamic table");
        });
        let expected = if valid_kline_table_name(name) {
            "structure does not match"
        } else {
            "unknown application table"
        };
        assert_incompatible(validate(&path, definition), expected);
    }
}

#[test]
fn foreign_keys_triggers_views_and_orphaned_rows_fail_closed() {
    let strategy = definition("strategy").expect("strategy definition");
    let root = tempdir().expect("tempdir");
    let path = create_database(
        &root,
        strategy,
        Some(("ON DELETE CASCADE", "ON DELETE SET NULL")),
    );
    assert_incompatible(validate(&path, strategy), "structure does not match");

    let root = tempdir().expect("tempdir");
    let path = create_database(
        &root,
        strategy,
        Some(("versions are immutable", "versions are mutable")),
    );
    assert_incompatible(validate(&path, strategy), "triggers do not match");

    let root = tempdir().expect("tempdir");
    let path = create_database(&root, strategy, None);
    mutate(&path, |connection| {
        connection
            .execute_batch(
                "CREATE VIEW rogue_strategy_view AS SELECT id FROM strategy_design_definitions",
            )
            .expect("create rogue view");
    });
    assert_incompatible(validate(&path, strategy), "views do not match");

    let sessions = definition("adk-session").expect("adk-session definition");
    let root = tempdir().expect("tempdir");
    let path = create_database(&root, sessions, None);
    mutate(&path, |connection| {
        connection
            .execute_batch("PRAGMA foreign_keys = OFF")
            .expect("disable writer foreign keys");
        connection
            .execute(
                "INSERT INTO events (id, app_name, user_id, session_id) VALUES ('orphan', 'app', 'user', 'missing')",
                [],
            )
            .expect("insert orphan event");
    });
    assert_incompatible(validate(&path, sessions), "foreign_key_check failed");
}

#[test]
fn truncated_database_is_rejected_without_repair_or_rewrite() {
    let definition = definition("research").expect("research definition");
    let root = tempdir().expect("tempdir");
    let path = create_database(&root, definition, None);
    let length = fs::metadata(&path).expect("database metadata").len();
    let file = fs::OpenOptions::new()
        .write(true)
        .open(&path)
        .expect("open database for corruption fixture");
    file.set_len(length.saturating_sub(100))
        .expect("truncate database page");
    drop(file);
    let before = fs::read(&path).expect("read corrupted bytes");
    assert!(validate(&path, definition).is_err());
    assert_eq!(fs::read(&path).expect("reread corrupted bytes"), before);
}
