use std::collections::BTreeMap;
use std::sync::OnceLock;

use rusqlite::{Connection, OptionalExtension};
use serde::Deserialize;
use thiserror::Error;

const SCHEMA_DEFINITIONS: &str =
    include_str!("../../../tests/fixtures/rust-migration/stage9/sqlite-schema-definitions.json");
const SCHEMA_VERSION: &str = "stage9.sqlite-schema-definitions.v1";
const METADATA_TABLE: &str = "jftrade_schema_meta";
const KLINE_PATTERN: &str = "^local_klines__[a-z0-9_]+__[a-z0-9_]+__[a-z0-9_]+__(forward|backward|none)__(r|x)__[0-9a-f]{8}$";

#[derive(Clone, Debug, Deserialize)]
struct Catalog {
    version: String,
    definitions: Vec<Definition>,
}

#[derive(Clone, Debug, Deserialize)]
struct Definition {
    #[serde(rename = "ID")]
    id: String,
    #[serde(rename = "Version")]
    version: i64,
    #[serde(rename = "Statements")]
    statements: Option<Vec<String>>,
    #[serde(rename = "DynamicTable")]
    dynamic_table: Option<DynamicTable>,
}

#[derive(Clone, Debug, Deserialize)]
struct DynamicTable {
    #[serde(rename = "Pattern")]
    pattern: String,
    #[serde(rename = "PrototypeName")]
    prototype_name: String,
    #[serde(rename = "Statement")]
    statement: String,
}

#[derive(Clone, Debug, Eq, PartialEq)]
struct SchemaSnapshot {
    tables: BTreeMap<String, TableSnapshot>,
    views: Vec<String>,
    triggers: Vec<String>,
}

#[derive(Clone, Debug, Eq, PartialEq)]
struct TableSnapshot {
    columns: Vec<ColumnSnapshot>,
    indexes: Vec<IndexSnapshot>,
    foreign_keys: Vec<ForeignKeySnapshot>,
    without_row_id: bool,
    strict: bool,
}

#[derive(Clone, Debug, Eq, PartialEq)]
struct ColumnSnapshot {
    name: String,
    type_name: String,
    not_null: i64,
    default_value: String,
    primary_key: i64,
    hidden: i64,
}

#[derive(Clone, Debug, Eq, PartialEq)]
struct IndexSnapshot {
    name: String,
    unique: i64,
    origin: String,
    partial: i64,
    columns: Vec<IndexColumnSnapshot>,
    sql: String,
}

#[derive(Clone, Debug, Eq, PartialEq)]
struct IndexColumnSnapshot {
    sequence: i64,
    column_id: i64,
    name: String,
    desc: i64,
    collate: String,
    key: i64,
}

#[derive(Clone, Debug, Eq, PartialEq)]
struct ForeignKeySnapshot {
    id: i64,
    sequence: i64,
    table: String,
    from: String,
    to: String,
    on_update: String,
    on_delete: String,
    match_name: String,
}

#[derive(Debug, Error)]
pub enum SchemaManifestError {
    #[error("managed SQLite schema catalog is invalid: {0}")]
    Catalog(String),
    #[error("{component} database schema is incompatible: {reason}; rebuild database {path}")]
    Incompatible {
        component: String,
        path: String,
        reason: String,
    },
    #[error("inspect managed SQLite schema: {0}")]
    Inspect(#[from] rusqlite::Error),
}

impl SchemaManifestError {
    pub const fn is_incompatible(&self) -> bool {
        matches!(self, Self::Incompatible { .. })
    }
}

pub fn current_version(connection: &Connection, component: &str) -> Option<i64> {
    connection
        .query_row(
            "SELECT version FROM jftrade_schema_meta WHERE component_id = ?1 LIMIT 1",
            [component],
            |row| row.get(0),
        )
        .ok()
}

/// Create a brand-new managed database from the pinned schema manifest.
/// Existing files are never replaced; callers should open them and run
/// `validate_current` instead.  Keeping creation here guarantees that every
/// production store uses the same DDL as compatibility validation.
pub fn initialize_current(
    connection: &Connection,
    component: &str,
) -> Result<(), SchemaManifestError> {
    let definition = definition(component)?;
    for statement in definition.statements.as_deref().unwrap_or_default() {
        if !statement.trim().is_empty() {
            connection.execute_batch(statement)?;
        }
    }
    if let Some(dynamic) = &definition.dynamic_table {
        connection.execute_batch(&dynamic.statement)?;
    }
    connection.execute_batch(
        "CREATE TABLE jftrade_schema_meta (component_id TEXT PRIMARY KEY, version INTEGER NOT NULL, created_at TEXT NOT NULL)",
    )?;
    connection.execute(
        "INSERT INTO jftrade_schema_meta (component_id, version, created_at) VALUES (?1, ?2, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        (&definition.id, definition.version),
    )?;
    Ok(())
}

pub fn validate_current(
    connection: &Connection,
    path: &str,
    component: &str,
    expected_version: i64,
) -> Result<(), SchemaManifestError> {
    let definition = definition(component)?;
    if definition.version != expected_version {
        return incompatible(
            component,
            path,
            format!(
                "pinned schema version {} does not match required version {expected_version}",
                definition.version
            ),
        );
    }
    validate_metadata(connection, path, definition)?;
    let expected_connection = expected_database(definition)?;
    let mut expected = inspect_schema(&expected_connection)?;
    let mut actual = inspect_schema(connection)?;
    let mut dynamic_prototype = None;
    if let Some(dynamic) = &definition.dynamic_table {
        if dynamic.pattern != KLINE_PATTERN {
            return Err(SchemaManifestError::Catalog(format!(
                "unsupported dynamic table pattern {}",
                dynamic.pattern
            )));
        }
        dynamic_prototype = expected.tables.remove(&dynamic.prototype_name);
        if dynamic_prototype.is_none() {
            return Err(SchemaManifestError::Catalog(format!(
                "dynamic table prototype {} is missing",
                dynamic.prototype_name
            )));
        }
    }
    for (table_name, expected_table) in expected.tables {
        let Some(actual_table) = actual.tables.remove(&table_name) else {
            return incompatible(
                component,
                path,
                format!("required table is missing: {table_name}"),
            );
        };
        if actual_table != expected_table {
            return incompatible(
                component,
                path,
                format!("{table_name} structure does not match current schema"),
            );
        }
    }
    for (table_name, mut actual_table) in actual.tables {
        let Some(mut prototype) = dynamic_prototype.clone() else {
            return incompatible(
                component,
                path,
                format!("unknown application table: {table_name}"),
            );
        };
        if !valid_kline_table_name(&table_name) {
            return incompatible(
                component,
                path,
                format!("unknown application table: {table_name}"),
            );
        }
        normalize_automatic_indexes(&mut actual_table.indexes);
        normalize_automatic_indexes(&mut prototype.indexes);
        if actual_table != prototype {
            return incompatible(
                component,
                path,
                format!("{table_name} structure does not match current schema"),
            );
        }
    }
    if actual.views != expected.views {
        return incompatible(
            component,
            path,
            "views do not match current schema".to_owned(),
        );
    }
    if actual.triggers != expected.triggers {
        return incompatible(
            component,
            path,
            "triggers do not match current schema".to_owned(),
        );
    }
    validate_integrity(connection).map_err(|error| SchemaManifestError::Incompatible {
        component: component.to_owned(),
        path: path.to_owned(),
        reason: error,
    })
}

fn catalog() -> Result<&'static Catalog, SchemaManifestError> {
    static CATALOG: OnceLock<Result<Catalog, String>> = OnceLock::new();
    let result = CATALOG.get_or_init(|| {
        let catalog: Catalog = serde_json::from_str(SCHEMA_DEFINITIONS)
            .map_err(|error| format!("decode definitions: {error}"))?;
        if catalog.version != SCHEMA_VERSION || catalog.definitions.len() != 9 {
            return Err(format!(
                "expected {SCHEMA_VERSION} with 9 definitions, got {} with {}",
                catalog.version,
                catalog.definitions.len()
            ));
        }
        Ok(catalog)
    });
    result
        .as_ref()
        .map_err(|error| SchemaManifestError::Catalog(error.clone()))
}

fn definition(component: &str) -> Result<&'static Definition, SchemaManifestError> {
    catalog()?
        .definitions
        .iter()
        .find(|definition| definition.id == component)
        .ok_or_else(|| SchemaManifestError::Catalog(format!("unknown database id {component:?}")))
}

fn validate_metadata(
    connection: &Connection,
    path: &str,
    definition: &Definition,
) -> Result<(), SchemaManifestError> {
    let table = connection
        .query_row(
            "SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?1 LIMIT 1",
            [METADATA_TABLE],
            |row| row.get::<_, String>(0),
        )
        .optional()?;
    if table.is_none() {
        return incompatible(
            &definition.id,
            path,
            "schema metadata is missing".to_owned(),
        );
    }
    let stored_version = connection
        .query_row(
            "SELECT version FROM jftrade_schema_meta WHERE component_id = ?1 LIMIT 1",
            [&definition.id],
            |row| row.get::<_, i64>(0),
        )
        .optional()
        .map_err(|error| SchemaManifestError::Incompatible {
            component: definition.id.clone(),
            path: path.to_owned(),
            reason: format!("schema metadata is unreadable: {error}"),
        })?;
    let Some(stored_version) = stored_version else {
        return incompatible(
            &definition.id,
            path,
            "component metadata is missing".to_owned(),
        );
    };
    if stored_version != definition.version {
        return incompatible(
            &definition.id,
            path,
            format!(
                "schema version {stored_version} does not match required version {}",
                definition.version
            ),
        );
    }
    let metadata_rows =
        connection.query_row("SELECT COUNT(*) FROM jftrade_schema_meta", [], |row| {
            row.get::<_, i64>(0)
        })?;
    if metadata_rows != 1 {
        return incompatible(
            &definition.id,
            path,
            format!(
                "schema metadata contains {metadata_rows} component rows; exactly one is required"
            ),
        );
    }
    Ok(())
}

fn expected_database(definition: &Definition) -> Result<Connection, SchemaManifestError> {
    let connection = Connection::open_in_memory()?;
    for statement in definition.statements.as_deref().unwrap_or_default() {
        if !statement.trim().is_empty() {
            connection.execute_batch(statement)?;
        }
    }
    if let Some(dynamic) = &definition.dynamic_table {
        connection.execute_batch(&dynamic.statement)?;
    }
    connection.execute_batch(
        "CREATE TABLE jftrade_schema_meta (component_id TEXT PRIMARY KEY, version INTEGER NOT NULL, created_at TEXT NOT NULL)",
    )?;
    connection.execute(
        "INSERT INTO jftrade_schema_meta (component_id, version, created_at) VALUES (?1, ?2, 'manifest')",
        (&definition.id, definition.version),
    )?;
    Ok(connection)
}

fn inspect_schema(connection: &Connection) -> Result<SchemaSnapshot, rusqlite::Error> {
    let mut snapshot = SchemaSnapshot {
        tables: BTreeMap::new(),
        views: Vec::new(),
        triggers: Vec::new(),
    };
    let mut statement = connection.prepare(
        "SELECT type, name, COALESCE(sql, '') FROM sqlite_master \
         WHERE type IN ('table', 'view', 'trigger') AND name NOT LIKE 'sqlite_%' \
         ORDER BY type, name",
    )?;
    let objects = statement
        .query_map([], |row| {
            Ok((
                row.get::<_, String>(0)?,
                row.get::<_, String>(1)?,
                row.get::<_, String>(2)?,
            ))
        })?
        .collect::<Result<Vec<_>, _>>()?;
    for (object_type, name, ddl) in objects {
        match object_type.as_str() {
            "table" => {
                snapshot
                    .tables
                    .insert(name.clone(), inspect_table(connection, &name, &ddl)?);
            }
            "view" => snapshot
                .views
                .push(format!("{name}:{}", normalize_sql(&ddl))),
            "trigger" => snapshot
                .triggers
                .push(format!("{name}:{}", normalize_sql(&ddl))),
            _ => {}
        }
    }
    snapshot.views.sort();
    snapshot.triggers.sort();
    Ok(snapshot)
}

fn inspect_table(
    connection: &Connection,
    table_name: &str,
    ddl: &str,
) -> Result<TableSnapshot, rusqlite::Error> {
    let normalized = normalize_sql(ddl);
    Ok(TableSnapshot {
        columns: inspect_columns(connection, table_name)?,
        indexes: inspect_indexes(connection, table_name)?,
        foreign_keys: inspect_foreign_keys(connection, table_name)?,
        without_row_id: normalized.contains(" without rowid"),
        strict: normalized.ends_with(" strict"),
    })
}

fn inspect_columns(
    connection: &Connection,
    table_name: &str,
) -> Result<Vec<ColumnSnapshot>, rusqlite::Error> {
    let mut statement = connection.prepare(&format!(
        "PRAGMA table_xinfo({})",
        quote_identifier(table_name)
    ))?;
    statement
        .query_map([], |row| {
            Ok(ColumnSnapshot {
                name: row.get(1)?,
                type_name: row.get::<_, String>(2)?.trim().to_ascii_uppercase(),
                not_null: row.get(3)?,
                default_value: row
                    .get::<_, Option<String>>(4)?
                    .map(|value| normalize_sql(&value))
                    .unwrap_or_default(),
                primary_key: row.get(5)?,
                hidden: row.get(6)?,
            })
        })?
        .collect()
}

fn inspect_indexes(
    connection: &Connection,
    table_name: &str,
) -> Result<Vec<IndexSnapshot>, rusqlite::Error> {
    let mut statement = connection.prepare(&format!(
        "PRAGMA index_list({})",
        quote_identifier(table_name)
    ))?;
    let mut indexes = statement
        .query_map([], |row| {
            Ok(IndexSnapshot {
                name: row.get(1)?,
                unique: row.get(2)?,
                origin: row.get(3)?,
                partial: row.get(4)?,
                columns: Vec::new(),
                sql: String::new(),
            })
        })?
        .collect::<Result<Vec<_>, _>>()?;
    for index in &mut indexes {
        let mut columns_statement = connection.prepare(&format!(
            "PRAGMA index_xinfo({})",
            quote_identifier(&index.name)
        ))?;
        index.columns = columns_statement
            .query_map([], |row| {
                Ok(IndexColumnSnapshot {
                    sequence: row.get(0)?,
                    column_id: row.get(1)?,
                    name: row.get::<_, Option<String>>(2)?.unwrap_or_default(),
                    desc: row.get(3)?,
                    collate: row.get(4)?,
                    key: row.get(5)?,
                })
            })?
            .collect::<Result<Vec<_>, _>>()?;
        index.sql = connection
            .query_row(
                "SELECT sql FROM sqlite_master WHERE type = 'index' AND name = ?1",
                [&index.name],
                |row| row.get::<_, Option<String>>(0),
            )
            .optional()?
            .flatten()
            .map(|value| normalize_index_sql(&value))
            .unwrap_or_default();
    }
    indexes.sort_by(|left, right| left.name.cmp(&right.name));
    Ok(indexes)
}

fn inspect_foreign_keys(
    connection: &Connection,
    table_name: &str,
) -> Result<Vec<ForeignKeySnapshot>, rusqlite::Error> {
    let mut statement = connection.prepare(&format!(
        "PRAGMA foreign_key_list({})",
        quote_identifier(table_name)
    ))?;
    let mut foreign_keys = statement
        .query_map([], |row| {
            Ok(ForeignKeySnapshot {
                id: row.get(0)?,
                sequence: row.get(1)?,
                table: row.get(2)?,
                from: row.get(3)?,
                to: row.get::<_, Option<String>>(4)?.unwrap_or_default(),
                on_update: row.get(5)?,
                on_delete: row.get(6)?,
                match_name: row.get(7)?,
            })
        })?
        .collect::<Result<Vec<_>, _>>()?;
    foreign_keys.sort_by_key(|value| (value.id, value.sequence));
    Ok(foreign_keys)
}

fn validate_integrity(connection: &Connection) -> Result<(), String> {
    let mut quick_check = connection
        .prepare("PRAGMA quick_check")
        .map_err(|error| error.to_string())?;
    let results = quick_check
        .query_map([], |row| row.get::<_, String>(0))
        .map_err(|error| error.to_string())?
        .collect::<Result<Vec<_>, _>>()
        .map_err(|error| error.to_string())?;
    if results.is_empty()
        || !results
            .iter()
            .all(|value| value.trim().eq_ignore_ascii_case("ok"))
    {
        return Err(format!("quick_check failed: {}", results.join(", ")));
    }
    let mut foreign_keys = connection
        .prepare("PRAGMA foreign_key_check")
        .map_err(|error| error.to_string())?;
    if foreign_keys
        .query([])
        .map_err(|error| error.to_string())?
        .next()
        .map_err(|error| error.to_string())?
        .is_some()
    {
        return Err("foreign_key_check failed".to_owned());
    }
    Ok(())
}

fn normalize_automatic_indexes(indexes: &mut [IndexSnapshot]) {
    for index in indexes {
        if index.origin != "c" {
            index.name.clear();
            index.sql.clear();
        }
    }
}

fn normalize_sql(value: &str) -> String {
    value
        .split_whitespace()
        .collect::<Vec<_>>()
        .join(" ")
        .to_ascii_lowercase()
}

fn normalize_index_sql(value: &str) -> String {
    normalize_sql(value)
        .replacen(
            "create unique index if not exists ",
            "create unique index ",
            1,
        )
        .replacen("create index if not exists ", "create index ", 1)
}

fn quote_identifier(value: &str) -> String {
    format!("\"{}\"", value.replace('"', "\"\""))
}

fn valid_kline_table_name(name: &str) -> bool {
    let Some(suffix) = name.strip_prefix("local_klines__") else {
        return false;
    };
    let parts = suffix.split("__").collect::<Vec<_>>();
    parts.len() == 6
        && parts[..3].iter().all(|part| {
            !part.is_empty()
                && part
                    .bytes()
                    .all(|byte| byte.is_ascii_lowercase() || byte.is_ascii_digit() || byte == b'_')
        })
        && matches!(parts[3], "forward" | "backward" | "none")
        && matches!(parts[4], "r" | "x")
        && parts[5].len() == 8
        && parts[5]
            .bytes()
            .all(|byte| byte.is_ascii_hexdigit() && !byte.is_ascii_uppercase())
}

fn incompatible<T>(component: &str, path: &str, reason: String) -> Result<T, SchemaManifestError> {
    Err(SchemaManifestError::Incompatible {
        component: component.to_owned(),
        path: path.to_owned(),
        reason,
    })
}

#[cfg(test)]
#[path = "schema_manifest_invariants_tests.rs"]
mod invariants_tests;

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::tempdir;

    #[test]
    fn pinned_catalog_contains_all_current_go_definitions() {
        let catalog = catalog().expect("catalog");
        assert_eq!(catalog.definitions.len(), 9);
        assert_eq!(
            definition("execution-orders").expect("execution").version,
            5
        );
        assert!(matches!(
            definition("unknown"),
            Err(SchemaManifestError::Catalog(_))
        ));
    }

    #[test]
    fn all_nine_manifests_accept_their_exact_schema_and_reject_a_rogue_table() {
        let root = tempdir().expect("tempdir");
        for definition in &catalog().expect("catalog").definitions {
            let path = root.path().join(format!("{}.db", definition.id));
            let connection = Connection::open(&path).expect("open database");
            for statement in definition.statements.as_deref().unwrap_or_default() {
                connection.execute_batch(statement).expect("apply schema");
            }
            if let Some(dynamic) = &definition.dynamic_table {
                connection
                    .execute_batch(&dynamic.statement)
                    .expect("apply dynamic schema");
            }
            connection
                .execute_batch(
                    "CREATE TABLE jftrade_schema_meta (component_id TEXT PRIMARY KEY, version INTEGER NOT NULL, created_at TEXT NOT NULL)",
                )
                .expect("create metadata");
            connection
                .execute(
                    "INSERT INTO jftrade_schema_meta (component_id, version, created_at) VALUES (?1, ?2, 'test')",
                    (&definition.id, definition.version),
                )
                .expect("insert metadata");
            validate_current(
                &connection,
                &path.to_string_lossy(),
                &definition.id,
                definition.version,
            )
            .expect("validate exact schema");
            connection
                .execute_batch("CREATE TABLE rogue_stage9_table (id TEXT PRIMARY KEY)")
                .expect("create rogue table");
            assert!(matches!(
                validate_current(
                    &connection,
                    &path.to_string_lossy(),
                    &definition.id,
                    definition.version,
                ),
                Err(SchemaManifestError::Incompatible { .. })
            ));
        }
    }
}
