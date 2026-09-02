use rusqlite::{Connection, Transaction, params};

use crate::schema_manifest::SchemaManifestError;

const BACKTEST_COMPONENT: &str = "backtest";
const STRATEGY_COMPONENT: &str = "strategy";
const ADK_COMPONENT: &str = "adk";

/// Apply an explicitly supported legacy schema path inside the caller's
/// transaction.  Every DDL step is followed by a metadata update; the caller
/// must run `validate_current` before committing so a partial or corrupt
/// legacy file is rolled back as one unit.
pub fn migrate_legacy_schema(
    transaction: &Transaction<'_>,
    path: &str,
    component: &str,
    from_version: i64,
    expected_version: i64,
) -> Result<(), SchemaManifestError> {
    if from_version >= expected_version {
        return Err(incompatible(
            component,
            path,
            format!("unsupported migration range {from_version} -> {expected_version}"),
        ));
    }

    let mut version = from_version;
    while version < expected_version {
        match (component, version) {
            (BACKTEST_COMPONENT, 2) => migrate_backtest_v2_to_v3(transaction, path)?,
            (STRATEGY_COMPONENT, 1) => migrate_strategy_v1_to_v2(transaction, path)?,
            (ADK_COMPONENT, 2) => migrate_adk_v2_to_v3(transaction, path)?,
            // ADK v4 changed runtime semantics but did not change the SQLite
            // shape.  Recording this explicit hop keeps the version history
            // honest while the final manifest validation remains mandatory.
            (ADK_COMPONENT, 3) => {}
            (_, unsupported) => {
                return Err(incompatible(
                    component,
                    path,
                    format!("no production migration is defined for version {unsupported}"),
                ));
            }
        }
        version += 1;
        set_version(transaction, component, version, path)?;
    }
    Ok(())
}

fn set_version(
    transaction: &Transaction<'_>,
    component: &str,
    version: i64,
    path: &str,
) -> Result<(), SchemaManifestError> {
    let changed = transaction
        .execute(
            "UPDATE jftrade_schema_meta SET version = ?1 WHERE component_id = ?2",
            params![version, component],
        )
        .map_err(SchemaManifestError::Inspect)?;
    if changed != 1 {
        return Err(incompatible(
            component,
            path,
            format!("schema metadata update affected {changed} rows"),
        ));
    }
    Ok(())
}

fn migrate_strategy_v1_to_v2(
    transaction: &Transaction<'_>,
    path: &str,
) -> Result<(), SchemaManifestError> {
    transaction
        .execute_batch(
            "CREATE TABLE IF NOT EXISTS strategy_definition_versions (
                definition_id TEXT NOT NULL,
                version TEXT NOT NULL,
                name TEXT NOT NULL DEFAULT '',
                description TEXT NOT NULL DEFAULT '',
                runtime TEXT NOT NULL DEFAULT '',
                source_format TEXT NOT NULL DEFAULT '',
                symbol TEXT NOT NULL DEFAULT '',
                interval TEXT NOT NULL DEFAULT '',
                script TEXT NOT NULL DEFAULT '',
                visual_model_json TEXT NOT NULL DEFAULT '',
                created_at TEXT NOT NULL DEFAULT '',
                updated_at TEXT NOT NULL DEFAULT '',
                saved_at TEXT NOT NULL DEFAULT '',
                PRIMARY KEY (definition_id, version),
                FOREIGN KEY (definition_id) REFERENCES strategy_design_definitions(id) ON DELETE CASCADE
            );
            CREATE INDEX IF NOT EXISTS idx_strategy_definition_versions_saved_at
                ON strategy_definition_versions (definition_id, saved_at DESC, version DESC);
            CREATE TRIGGER IF NOT EXISTS trg_strategy_definition_versions_immutable
                BEFORE UPDATE ON strategy_definition_versions
                BEGIN
                    SELECT RAISE(ABORT, 'strategy definition versions are immutable');
                END;
            INSERT INTO strategy_definition_versions (
                definition_id, version, name, description, runtime, source_format,
                symbol, interval, script, visual_model_json, created_at, updated_at,
                saved_at
            )
            SELECT id, version, name, description, runtime, source_format,
                   symbol, interval, script, visual_model_json, created_at, updated_at,
                   CASE WHEN TRIM(updated_at) <> '' THEN updated_at ELSE created_at END
              FROM strategy_design_definitions;",
        )
        .map_err(|error| migration_inspect("strategy", path, error))
}

fn migrate_adk_v2_to_v3(
    transaction: &Transaction<'_>,
    path: &str,
) -> Result<(), SchemaManifestError> {
    let columns = table_columns(transaction, "adk_runs")?;
    let legacy_columns = [
        "id",
        "session_id",
        "agent_id",
        "status",
        "payload_json",
        "created_at",
        "updated_at",
    ];
    let current_columns = [
        "id",
        "session_id",
        "agent_id",
        "status",
        "client_request_id",
        "request_fingerprint",
        "payload_json",
        "created_at",
        "updated_at",
    ];
    if columns == legacy_columns {
        transaction
            .execute_batch(
                "DROP INDEX IF EXISTS idx_adk_runs_session;
                 CREATE TABLE adk_runs__migration (
                    id TEXT PRIMARY KEY,
                    session_id TEXT NOT NULL,
                    agent_id TEXT NOT NULL,
                    status TEXT NOT NULL,
                    client_request_id TEXT NOT NULL DEFAULT '',
                    request_fingerprint TEXT NOT NULL DEFAULT '',
                    payload_json TEXT NOT NULL,
                    created_at TEXT NOT NULL,
                    updated_at TEXT NOT NULL
                 );
                 INSERT INTO adk_runs__migration
                    (id, session_id, agent_id, status, client_request_id,
                     request_fingerprint, payload_json, created_at, updated_at)
                 SELECT id, session_id, agent_id, status, '', '', payload_json,
                        created_at, updated_at
                   FROM adk_runs;
                 DROP TABLE adk_runs;
                 ALTER TABLE adk_runs__migration RENAME TO adk_runs;
                 CREATE INDEX idx_adk_runs_session
                    ON adk_runs (session_id, created_at DESC);
                 CREATE UNIQUE INDEX idx_adk_runs_client_request
                    ON adk_runs (client_request_id) WHERE client_request_id <> '';",
            )
            .map_err(|error| migration_inspect("adk", path, error))?;
    } else if columns == current_columns {
        transaction
            .execute_batch(
                "CREATE INDEX IF NOT EXISTS idx_adk_runs_session
                    ON adk_runs (session_id, created_at DESC);
                 CREATE UNIQUE INDEX IF NOT EXISTS idx_adk_runs_client_request
                    ON adk_runs (client_request_id) WHERE client_request_id <> '';",
            )
            .map_err(|error| migration_inspect("adk", path, error))?;
    } else {
        return Err(incompatible(
            ADK_COMPONENT,
            path,
            format!("adk_runs columns do not match a supported v2 schema: {columns:?}"),
        ));
    }
    Ok(())
}

fn migrate_backtest_v2_to_v3(
    transaction: &Transaction<'_>,
    path: &str,
) -> Result<(), SchemaManifestError> {
    let mut names = transaction
        .prepare("SELECT name FROM sqlite_master WHERE type = 'table' AND name LIKE 'local_klines__%' ORDER BY name")
        .map_err(SchemaManifestError::Inspect)?
        .query_map([], |row| row.get::<_, String>(0))
        .map_err(SchemaManifestError::Inspect)?
        .collect::<Result<Vec<_>, _>>()
        .map_err(SchemaManifestError::Inspect)?;
    names.sort();

    for old_name in names {
        let Some(target_name) = legacy_kline_target(&old_name, path)? else {
            // Current-format tables can be left untouched.  Unknown names are
            // rejected by the final manifest validation after this transaction.
            continue;
        };
        if target_name == old_name {
            continue;
        }
        let target_exists = transaction
            .query_row(
                "SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?1)",
                [target_name.as_str()],
                |row| row.get::<_, i64>(0),
            )
            .map_err(SchemaManifestError::Inspect)?
            != 0;
        if target_exists {
            return Err(incompatible(
                BACKTEST_COMPONENT,
                path,
                format!("cannot rename {old_name}: target table {target_name} already exists"),
            ));
        }
        transaction
            .execute(
                &format!(
                    "ALTER TABLE {} RENAME TO {}",
                    quote_identifier(&old_name),
                    quote_identifier(&target_name)
                ),
                [],
            )
            .map_err(|error| migration_inspect(BACKTEST_COMPONENT, path, error))?;
    }
    Ok(())
}

fn table_columns(
    connection: &Connection,
    table_name: &str,
) -> Result<Vec<String>, SchemaManifestError> {
    let mut statement = connection
        .prepare(&format!(
            "PRAGMA table_xinfo({})",
            quote_identifier(table_name)
        ))
        .map_err(SchemaManifestError::Inspect)?;
    statement
        .query_map([], |row| row.get::<_, String>(1))
        .map_err(SchemaManifestError::Inspect)?
        .collect::<Result<Vec<_>, _>>()
        .map_err(SchemaManifestError::Inspect)
}

fn legacy_kline_target(name: &str, path: &str) -> Result<Option<String>, SchemaManifestError> {
    const PREFIX: &str = "local_klines__";
    let Some(suffix) = name.strip_prefix(PREFIX) else {
        return Ok(None);
    };
    let parts = suffix.split("__").collect::<Vec<_>>();
    if parts.len() != 5
        || parts[..2].iter().any(|part| part.is_empty())
        || !matches!(parts[2], "forward" | "backward" | "none")
        || !matches!(parts[3], "r" | "x")
        || parts[4].len() != 8
        || !parts[4]
            .bytes()
            .all(|byte| byte.is_ascii_hexdigit() && !byte.is_ascii_uppercase())
    {
        return Ok(None);
    }
    if name == "local_klines__manifest__1m__forward__r__00000000" {
        return Ok(Some(
            "local_klines__manifest__symbol__1m__forward__r__00000000".to_owned(),
        ));
    }
    let provider = "futu";
    let symbol_component = parts[0];
    let legacy_hash = u32::from_str_radix(parts[4], 16).map_err(|error| {
        incompatible(
            BACKTEST_COMPONENT,
            path,
            format!("invalid legacy K-line hash in {name}: {error}"),
        )
    })?;
    let raw_symbol = legacy_raw_symbol(symbol_component, legacy_hash).ok_or_else(|| {
        incompatible(
            BACKTEST_COMPONENT,
            path,
            format!("cannot safely recover the raw symbol encoded by legacy table {name}"),
        )
    })?;
    let interval = parts[1];
    let hash = fnv1a(format!("{provider}|{raw_symbol}").as_bytes());
    Ok(Some(format!(
        "{PREFIX}{provider}__{symbol_component}__{interval}__{}__{}__{hash:08x}",
        parts[2], parts[3]
    )))
}

fn legacy_raw_symbol(symbol_component: &str, expected_hash: u32) -> Option<String> {
    let mut candidates = vec![symbol_component.to_owned()];
    if let Some((market, code)) = symbol_component.split_once('_')
        && matches!(market, "hk" | "us" | "sh" | "sz" | "cn")
        && !code.is_empty()
    {
        candidates.push(format!("{market}.{code}"));
    }
    candidates
        .into_iter()
        .find(|candidate| fnv1a(candidate.as_bytes()) == expected_hash)
}

fn fnv1a(bytes: &[u8]) -> u32 {
    let mut hash = 0x811c9dc5u32;
    for byte in bytes {
        hash ^= u32::from(*byte);
        hash = hash.wrapping_mul(0x01000193);
    }
    hash
}

fn quote_identifier(value: &str) -> String {
    format!("\"{}\"", value.replace('"', "\"\""))
}

fn migration_inspect(component: &str, path: &str, error: rusqlite::Error) -> SchemaManifestError {
    SchemaManifestError::Incompatible {
        component: component.to_owned(),
        path: path.to_owned(),
        reason: format!("migration DDL failed: {error}"),
    }
}

fn incompatible(component: &str, path: &str, reason: String) -> SchemaManifestError {
    SchemaManifestError::Incompatible {
        component: component.to_owned(),
        path: path.to_owned(),
        reason,
    }
}
