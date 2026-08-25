//! Durable watchlist test-cutover adapter.
//!
//! This module is compiled only for Rust tests. Its SQLite schema is an
//! isolated fixture store and is never opened by the default product profile.

use rusqlite::OptionalExtension;
use serde_json::{Value, json};

use super::product_watchlist_write_port::{
    WatchlistWriteMutation, WatchlistWritePort, WatchlistWritePortError,
};

pub struct WatchlistSqliteTestCutoverPort {
    path: std::path::PathBuf,
    connection: std::sync::Mutex<rusqlite::Connection>,
    next_id: std::sync::atomic::AtomicU64,
}

impl std::fmt::Debug for WatchlistSqliteTestCutoverPort {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("WatchlistSqliteTestCutoverPort")
            .field("path", &self.path)
            .finish()
    }
}

#[allow(dead_code)]
impl WatchlistSqliteTestCutoverPort {
    pub fn open(path: impl AsRef<std::path::Path>) -> Result<Self, String> {
        let path = path.as_ref().to_owned();
        let connection = rusqlite::Connection::open(&path).map_err(|error| error.to_string())?;
        connection
            .execute_batch(
                "PRAGMA foreign_keys = ON;
                 CREATE TABLE IF NOT EXISTS watchlist_test_groups (
                    id TEXT PRIMARY KEY,
                    name TEXT NOT NULL UNIQUE,
                    revision INTEGER NOT NULL,
                    protected INTEGER NOT NULL DEFAULT 0,
                    deleted INTEGER NOT NULL DEFAULT 0
                 );
                 CREATE TABLE IF NOT EXISTS watchlist_test_memberships (
                    instrument_id TEXT PRIMARY KEY,
                    group_ids TEXT NOT NULL,
                    revision INTEGER NOT NULL
                 );
                 CREATE TABLE IF NOT EXISTS watchlist_test_previews (
                    id TEXT PRIMARY KEY,
                    status TEXT NOT NULL,
                    delete_ids TEXT NOT NULL
                 );
                 CREATE TABLE IF NOT EXISTS watchlist_test_events (
                    sequence INTEGER PRIMARY KEY AUTOINCREMENT,
                    route TEXT NOT NULL,
                    payload TEXT NOT NULL
                 );",
            )
            .map_err(|error| error.to_string())?;
        let next_id = connection
            .query_row(
                "SELECT COALESCE(MAX(sequence), 0) + 1 FROM watchlist_test_events",
                [],
                |row| row.get::<_, i64>(0),
            )
            .map_err(|error| error.to_string())?;
        let next_id = u64::try_from(next_id).map_err(|error| error.to_string())?;
        Ok(Self {
            path,
            connection: std::sync::Mutex::new(connection),
            next_id: std::sync::atomic::AtomicU64::new(next_id),
        })
    }

    pub fn seed_group(&self, id: &str, name: &str, revision: i64) -> Result<(), String> {
        let connection = self.lock()?;
        connection
            .execute(
                "INSERT OR REPLACE INTO watchlist_test_groups
                    (id, name, revision, protected, deleted) VALUES (?1, ?2, ?3, 0, 0)",
                rusqlite::params![id, name, revision],
            )
            .map_err(|error| error.to_string())?;
        Ok(())
    }

    pub fn group(&self, id: &str) -> Result<Option<Value>, String> {
        let connection = self.lock()?;
        load_group(&connection, id).map_err(|error| error.to_string())
    }

    pub fn event_count(&self, route: &str) -> Result<u64, String> {
        let connection = self.lock()?;
        let count = connection
            .query_row(
                "SELECT COUNT(*) FROM watchlist_test_events WHERE route = ?1",
                rusqlite::params![route],
                |row| row.get::<_, i64>(0),
            )
            .map_err(|error| error.to_string())?;
        u64::try_from(count).map_err(|_| "negative watchlist event count".to_owned())
    }

    pub fn reject_revision(&self, revision: i64) -> Result<(), String> {
        let connection = self.lock()?;
        connection
            .execute_batch("DROP TRIGGER IF EXISTS watchlist_test_reject_revision")
            .map_err(|error| error.to_string())?;
        let statement = format!(
            "CREATE TRIGGER watchlist_test_reject_revision
             BEFORE UPDATE OF revision ON watchlist_test_groups
             WHEN NEW.revision = {revision} BEGIN
                 SELECT RAISE(ABORT, 'test-cutover revision rejection');
             END"
        );
        connection
            .execute_batch(&statement)
            .map_err(|error| error.to_string())?;
        Ok(())
    }

    pub fn clear_rejection(&self) -> Result<(), String> {
        let connection = self.lock()?;
        connection
            .execute_batch("DROP TRIGGER IF EXISTS watchlist_test_reject_revision")
            .map_err(|error| error.to_string())?;
        Ok(())
    }

    fn lock(&self) -> Result<std::sync::MutexGuard<'_, rusqlite::Connection>, String> {
        self.connection
            .lock()
            .map_err(|_| "watchlist fixture lock poisoned".to_owned())
    }

    fn mutate_transaction(
        &self,
        mutation: &WatchlistWriteMutation,
    ) -> Result<Value, WatchlistWritePortError> {
        let route = mutation.value["route"].as_str().unwrap_or_default();
        let connection = self
            .connection
            .lock()
            .map_err(|_| watchlist_failure("watchlist fixture lock poisoned"))?;
        let transaction = connection
            .unchecked_transaction()
            .map_err(|_| watchlist_failure("watchlist transaction failed"))?;
        let mut data = match route {
            "create-group" => create_group(&transaction, self, mutation)?,
            "update-group" => update_group(&transaction, mutation)?,
            "delete-group" => delete_group(&transaction, mutation)?,
            "delete-binding" => json!({"deleted": true}),
            "preview-import" => preview_import(&transaction, self, mutation)?,
            "commit-import" => commit_import(&transaction, mutation)?,
            "batch-quotes" => json!({"quotes": [], "errors": []}),
            "replace-memberships" => replace_memberships(&transaction, mutation)?,
            _ => return Err(watchlist_failure("unknown watchlist mutation")),
        };
        if let Value::Object(object) = &mut data {
            object.insert("route".to_owned(), Value::String(route.to_owned()));
        }
        transaction
            .execute(
                "INSERT INTO watchlist_test_events (route, payload) VALUES (?1, ?2)",
                rusqlite::params![
                    route,
                    serde_json::to_string(&mutation.value).unwrap_or_default()
                ],
            )
            .map_err(|_| watchlist_failure("watchlist event write failed"))?;
        transaction
            .commit()
            .map_err(|_| watchlist_failure("watchlist commit failed"))?;
        Ok(data)
    }
}

impl WatchlistWritePort for WatchlistSqliteTestCutoverPort {
    fn mutate(&self, mutation: &WatchlistWriteMutation) -> Result<Value, WatchlistWritePortError> {
        self.mutate_transaction(mutation)
    }
}

fn create_group(
    transaction: &rusqlite::Transaction<'_>,
    port: &WatchlistSqliteTestCutoverPort,
    mutation: &WatchlistWriteMutation,
) -> Result<Value, WatchlistWritePortError> {
    let name = mutation.value["name"].as_str().unwrap_or_default();
    let id = format!(
        "group-test-{}",
        port.next_id
            .fetch_add(1, std::sync::atomic::Ordering::Relaxed)
    );
    transaction
        .execute(
            "INSERT INTO watchlist_test_groups (id, name, revision, protected, deleted)
             VALUES (?1, ?2, 1, 0, 0)",
            rusqlite::params![id, name],
        )
        .map_err(|_| watchlist_failure("watchlist group create failed"))?;
    Ok(json!({"id": id, "name": name, "revision": 1}))
}

fn update_group(
    transaction: &rusqlite::Transaction<'_>,
    mutation: &WatchlistWriteMutation,
) -> Result<Value, WatchlistWritePortError> {
    let id = mutation.value["groupId"].as_str().unwrap_or_default();
    let expected = mutation.value["expectedRevision"]
        .as_i64()
        .unwrap_or_default();
    let current = transaction
        .query_row(
            "SELECT name, revision FROM watchlist_test_groups
             WHERE id = ?1 AND deleted = 0",
            rusqlite::params![id],
            |row| Ok((row.get::<_, String>(0)?, row.get::<_, i64>(1)?)),
        )
        .optional()
        .map_err(|_| watchlist_failure("watchlist group load failed"))?
        .ok_or_else(watchlist_not_found)?;
    if current.1 != expected {
        return Err(watchlist_busy());
    }
    let name = mutation.value["name"].as_str().unwrap_or_default();
    let revision = expected + 1;
    transaction
        .execute(
            "UPDATE watchlist_test_groups SET name = ?2, revision = ?3 WHERE id = ?1",
            rusqlite::params![id, name, revision],
        )
        .map_err(|_| watchlist_failure("watchlist group update failed"))?;
    Ok(json!({"id": id, "name": name, "revision": revision, "previousName": current.0}))
}

fn delete_group(
    transaction: &rusqlite::Transaction<'_>,
    mutation: &WatchlistWriteMutation,
) -> Result<Value, WatchlistWritePortError> {
    let id = mutation.value["groupId"].as_str().unwrap_or_default();
    let protected: i64 = transaction
        .query_row(
            "SELECT protected FROM watchlist_test_groups WHERE id = ?1 AND deleted = 0",
            rusqlite::params![id],
            |row| row.get(0),
        )
        .optional()
        .map_err(|_| watchlist_failure("watchlist group load failed"))?
        .ok_or_else(watchlist_not_found)?;
    if protected != 0 {
        return Err(WatchlistWritePortError {
            status: 409,
            code: "WATCHLIST_BUSY".to_owned(),
            message: "watchlist group is protected".to_owned(),
        });
    }
    transaction
        .execute(
            "UPDATE watchlist_test_groups SET deleted = 1 WHERE id = ?1",
            rusqlite::params![id],
        )
        .map_err(|_| watchlist_failure("watchlist group delete failed"))?;
    Ok(json!({"deleted": true, "groupId": id}))
}

fn preview_import(
    transaction: &rusqlite::Transaction<'_>,
    port: &WatchlistSqliteTestCutoverPort,
    mutation: &WatchlistWriteMutation,
) -> Result<Value, WatchlistWritePortError> {
    let id = format!(
        "preview-test-{}",
        port.next_id
            .fetch_add(1, std::sync::atomic::Ordering::Relaxed)
    );
    transaction
        .execute(
            "INSERT INTO watchlist_test_previews (id, status, delete_ids)
             VALUES (?1, 'READY', '[]')",
            rusqlite::params![id],
        )
        .map_err(|_| watchlist_failure("watchlist preview create failed"))?;
    Ok(
        json!({"id": id, "status": "READY", "sourceId": mutation.value["sourceId"], "remoteGroupId": mutation.value["remoteGroupId"]}),
    )
}

fn commit_import(
    transaction: &rusqlite::Transaction<'_>,
    mutation: &WatchlistWriteMutation,
) -> Result<Value, WatchlistWritePortError> {
    let id = mutation.value["previewId"].as_str().unwrap_or_default();
    let status = transaction
        .query_row(
            "SELECT status FROM watchlist_test_previews WHERE id = ?1",
            rusqlite::params![id],
            |row| row.get::<_, String>(0),
        )
        .optional()
        .map_err(|_| watchlist_failure("watchlist preview load failed"))?
        .ok_or_else(watchlist_not_found)?;
    if status != "READY" {
        return Err(watchlist_busy());
    }
    transaction
        .execute(
            "UPDATE watchlist_test_previews SET status = 'COMMITTED' WHERE id = ?1",
            rusqlite::params![id],
        )
        .map_err(|_| watchlist_failure("watchlist import commit failed"))?;
    Ok(json!({"previewId": id, "status": "COMMITTED", "completed": true}))
}

fn replace_memberships(
    transaction: &rusqlite::Transaction<'_>,
    mutation: &WatchlistWriteMutation,
) -> Result<Value, WatchlistWritePortError> {
    let instrument_id = mutation.value["instrumentId"].as_str().unwrap_or_default();
    let expected = mutation.value["expectedRevision"]
        .as_i64()
        .unwrap_or_default();
    let group_ids = mutation.value["groupIds"].clone();
    let current = transaction
        .query_row(
            "SELECT revision FROM watchlist_test_memberships WHERE instrument_id = ?1",
            rusqlite::params![instrument_id],
            |row| row.get::<_, i64>(0),
        )
        .optional()
        .map_err(|_| watchlist_failure("watchlist memberships load failed"))?
        .unwrap_or(0);
    if current != expected {
        return Err(watchlist_busy());
    }
    let revision = expected + 1;
    let group_text = serde_json::to_string(&group_ids).unwrap_or_else(|_| "null".to_owned());
    transaction
        .execute(
            "INSERT INTO watchlist_test_memberships (instrument_id, group_ids, revision)
             VALUES (?1, ?2, ?3)
             ON CONFLICT(instrument_id) DO UPDATE SET group_ids = excluded.group_ids, revision = excluded.revision",
            rusqlite::params![instrument_id, group_text, revision],
        )
        .map_err(|_| watchlist_failure("watchlist memberships update failed"))?;
    Ok(json!({"instrumentId": instrument_id, "groupIds": group_ids, "revision": revision}))
}

fn load_group(connection: &rusqlite::Connection, id: &str) -> rusqlite::Result<Option<Value>> {
    connection
        .query_row(
            "SELECT name, revision, deleted FROM watchlist_test_groups WHERE id = ?1",
            rusqlite::params![id],
            |row| Ok((row.get::<_, String>(0)?, row.get::<_, i64>(1)?, row.get::<_, i64>(2)?)),
        )
        .optional()
        .map(|row| row.map(|(name, revision, deleted)| json!({"id": id, "name": name, "revision": revision, "deleted": deleted != 0})))
}

fn watchlist_failure(message: &str) -> WatchlistWritePortError {
    WatchlistWritePortError {
        status: 500,
        code: "WATCHLIST_FAILED".to_owned(),
        message: message.to_owned(),
    }
}

fn watchlist_not_found() -> WatchlistWritePortError {
    WatchlistWritePortError {
        status: 404,
        code: "WATCHLIST_NOT_FOUND".to_owned(),
        message: "watchlist resource not found".to_owned(),
    }
}

fn watchlist_busy() -> WatchlistWritePortError {
    WatchlistWritePortError {
        status: 409,
        code: "WATCHLIST_BUSY".to_owned(),
        message: "watchlist revision conflict".to_owned(),
    }
}
