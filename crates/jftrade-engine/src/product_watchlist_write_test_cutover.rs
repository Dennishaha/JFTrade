//! Durable watchlist test-cutover adapter backed by `jftrade-store-sqlite`.
//!
//! This module is compiled only for Rust tests. It connects to the real
//! watchlist SQLite schema with schema validation and single-writer lease,
//! and is never constructed by the default product profile.

use std::path::{Path, PathBuf};
use std::sync::Arc;

use jftrade_store_sqlite::{
    WATCHLIST_TEST_CUTOVER_PROFILE, WatchlistStoreError, WatchlistTestCutoverStore,
};
use serde_json::{Value, json};
use time::{OffsetDateTime, format_description::well_known::Rfc3339};

use super::product_watchlist_write_port::{
    WatchlistWriteMutation, WatchlistWritePort, WatchlistWritePortError,
};

pub struct WatchlistSqliteTestCutoverPort {
    path: PathBuf,
    store: Arc<WatchlistTestCutoverStore>,
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
    pub fn open(path: impl AsRef<Path>) -> Result<Self, WatchlistStoreError> {
        let path = path.as_ref().to_owned();
        let store =
            WatchlistTestCutoverStore::open_existing(&path, WATCHLIST_TEST_CUTOVER_PROFILE)?;
        Ok(Self {
            path,
            store: Arc::new(store),
        })
    }

    pub fn path(&self) -> &Path {
        &self.path
    }

    pub fn store(&self) -> &WatchlistTestCutoverStore {
        &self.store
    }
}

impl WatchlistWritePort for WatchlistSqliteTestCutoverPort {
    fn mutate(&self, mutation: &WatchlistWriteMutation) -> Result<Value, WatchlistWritePortError> {
        let route = mutation.value["route"].as_str().unwrap_or_default();
        let timestamp = now_rfc3339();

        let mut data = match route {
            "create-group" => {
                let name = mutation.value["name"].as_str().unwrap_or_default();
                let group = self
                    .store
                    .create_group(name, &timestamp)
                    .map_err(map_store_error)?;
                json!({
                    "id": group.group_id,
                    "groupId": group.group_id,
                    "name": group.name,
                    "revision": group.revision,
                })
            }
            "update-group" => {
                let group_id = mutation.value["groupId"].as_str().unwrap_or_default();
                let name = mutation.value["name"].as_str().unwrap_or_default();
                let expected_revision = mutation.value["expectedRevision"]
                    .as_i64()
                    .unwrap_or_default();
                let group = self
                    .store
                    .update_group(group_id, name, expected_revision, &timestamp)
                    .map_err(map_store_error)?;
                json!({
                    "id": group.group_id,
                    "groupId": group.group_id,
                    "name": group.name,
                    "revision": group.revision,
                })
            }
            "delete-group" => {
                let group_id = mutation.value["groupId"].as_str().unwrap_or_default();
                self.store.delete_group(group_id).map_err(map_store_error)?;
                json!({"deleted": true})
            }
            "delete-binding" => {
                let binding_id = mutation.value["bindingId"].as_str().unwrap_or_default();
                if binding_id.is_empty() {
                    return Err(WatchlistWritePortError {
                        status: 400,
                        code: "WATCHLIST_INVALID".to_owned(),
                        message: "bindingId is required".to_owned(),
                    });
                }
                match self.store.delete_binding(binding_id) {
                    Ok(()) => json!({"deleted": true}),
                    Err(WatchlistStoreError::NotFound) => json!({"deleted": true}),
                    Err(err) => return Err(map_store_error(err)),
                }
            }
            "preview-import" => {
                let source_id = mutation.value["sourceId"].as_str().unwrap_or_default();
                let remote_group_id = mutation.value["remoteGroupId"].as_str().unwrap_or_default();
                let local_group_id = mutation.value["localGroupId"].as_str();
                let new_group_name = mutation.value["newGroupName"].as_str();
                let preview = self
                    .store
                    .create_import_preview(
                        source_id,
                        remote_group_id,
                        local_group_id,
                        new_group_name,
                        &timestamp,
                    )
                    .map_err(map_store_error)?;
                json!({
                    "id": preview.preview_id.clone(),
                    "previewId": preview.preview_id,
                    "status": preview.status,
                    "sourceId": preview.source_id,
                    "remoteGroupId": preview.remote_group_id,
                })
            }
            "commit-import" => {
                let preview_id = mutation.value["previewId"].as_str().unwrap_or_default();
                let delete_instrument_ids: Vec<String> = mutation.value["deleteInstrumentIds"]
                    .as_array()
                    .map(|arr| {
                        arr.iter()
                            .filter_map(|v| v.as_str().map(ToOwned::to_owned))
                            .collect()
                    })
                    .unwrap_or_default();
                let run = self
                    .store
                    .commit_import_preview(preview_id, &delete_instrument_ids, &timestamp)
                    .map_err(map_store_error)?;
                json!({
                    "id": run.run_id.clone(),
                    "runId": run.run_id,
                    "status": run.status,
                    "previewId": run.preview_id,
                })
            }
            "batch-quotes" => {
                json!({"quotes": [], "errors": []})
            }
            "replace-memberships" => {
                let instrument_id = mutation.value["instrumentId"].as_str().unwrap_or_default();
                let group_ids: Vec<String> = mutation.value["groupIds"]
                    .as_array()
                    .map(|arr| {
                        arr.iter()
                            .filter_map(|v| v.as_str().map(ToOwned::to_owned))
                            .collect()
                    })
                    .unwrap_or_default();
                let new_group_names: Vec<String> = mutation.value["newGroupNames"]
                    .as_array()
                    .map(|arr| {
                        arr.iter()
                            .filter_map(|v| v.as_str().map(ToOwned::to_owned))
                            .collect()
                    })
                    .unwrap_or_default();
                let expected_revision = mutation.value["expectedRevision"]
                    .as_i64()
                    .unwrap_or_default();
                let memberships = self
                    .store
                    .replace_memberships(
                        instrument_id,
                        &group_ids,
                        &new_group_names,
                        expected_revision,
                        &timestamp,
                    )
                    .map_err(map_store_error)?;
                json!({
                    "instrumentId": memberships.instrument_id,
                    "revision": memberships.revision,
                    "groups": memberships.groups,
                })
            }
            _ => {
                return Err(WatchlistWritePortError {
                    status: 500,
                    code: "INTERNAL_ERROR".to_owned(),
                    message: format!("unknown watchlist mutation route: {route}"),
                });
            }
        };

        if let Value::Object(object) = &mut data {
            object.insert("route".to_owned(), Value::String(route.to_owned()));
        }
        Ok(data)
    }
}

fn map_store_error(error: WatchlistStoreError) -> WatchlistWritePortError {
    match error {
        WatchlistStoreError::NotFound => WatchlistWritePortError {
            status: 404,
            code: "WATCHLIST_NOT_FOUND".to_owned(),
            message: "watchlist resource not found".to_owned(),
        },
        WatchlistStoreError::Conflict => WatchlistWritePortError {
            status: 409,
            code: "WATCHLIST_BUSY".to_owned(),
            message: "watchlist state conflict".to_owned(),
        },
        WatchlistStoreError::ProtectedGroup => WatchlistWritePortError {
            status: 409,
            code: "PROTECTED_GROUP".to_owned(),
            message: "protected watchlist group cannot be deleted or renamed".to_owned(),
        },
        WatchlistStoreError::Validation(message)
        | WatchlistStoreError::InvalidInstrument(message) => WatchlistWritePortError {
            status: 400,
            code: "WATCHLIST_INVALID".to_owned(),
            message,
        },
        _ => WatchlistWritePortError {
            status: 500,
            code: "INTERNAL_ERROR".to_owned(),
            message: error.to_string(),
        },
    }
}

fn now_rfc3339() -> String {
    OffsetDateTime::now_utc()
        .format(&Rfc3339)
        .unwrap_or_else(|_| "2026-08-26T00:00:00Z".to_owned())
}
