//! Watchlist production port implementation.

use std::sync::Arc;

use jftrade_store_sqlite::{WatchlistStore, WatchlistStoreError};
use jftrade_watchlist::{GroupRef, Memberships};
use percent_encoding::percent_decode_str;
use serde_json::{Value, json};

use crate::product::product_active_provider_state::ActiveProviderState;
use crate::product::product_watchlist_remote_write_port::{
    RemoteWatchlistWriteAction, RemoteWatchlistWritePort, RemoteWatchlistWritePortError,
    RemoteWatchlistWriteResolution,
};
use crate::product::product_watchlist_write_port::{
    WatchlistWriteMutation, WatchlistWritePort, WatchlistWritePortError,
};
use crate::product::{
    RemoteWatchlistSnapshotError, RemoteWatchlistSnapshotPort,
    WatchlistMembershipSnapshotError, WatchlistMembershipSnapshotPort,
    WatchlistReadSnapshotError, WatchlistReadSnapshotPort,
};

#[derive(Debug)]
pub(crate) struct ProductionWatchlistPort {
    pub(crate) store: Arc<WatchlistStore>,
}

const DEFAULT_PAGE_LIMIT: usize = 100;
const MAX_PAGE_LIMIT: usize = 500;

#[derive(Debug, Default)]
struct WatchlistReadQuery {
    group_id: String,
    cursor: String,
    query: String,
    market: String,
    source_id: String,
    limit: usize,
}

fn watchlist_store_error(error: WatchlistStoreError) -> WatchlistWritePortError {
    let message = error.to_string();
    let (status, code) = match error {
        WatchlistStoreError::NotFound => (404, "WATCHLIST_NOT_FOUND"),
        WatchlistStoreError::Conflict => (409, "WATCHLIST_CONFLICT"),
        WatchlistStoreError::ProtectedGroup => (409, "WATCHLIST_GROUP_PROTECTED"),
        WatchlistStoreError::InvalidInstrument(_) | WatchlistStoreError::Validation(_) => {
            (400, "WATCHLIST_INVALID")
        }
        WatchlistStoreError::EmptyPath
        | WatchlistStoreError::NotRegularFile(_)
        | WatchlistStoreError::WriterLease(_)
        | WatchlistStoreError::Open(_)
        | WatchlistStoreError::Configure(_)
        | WatchlistStoreError::Schema(_)
        | WatchlistStoreError::LockUnavailable => (503, "WATCHLIST_UNAVAILABLE"),
        WatchlistStoreError::UnsupportedProfile(_) | WatchlistStoreError::Query(_) => {
            (500, "WATCHLIST_FAILED")
        }
        WatchlistStoreError::Incompatible(_) => (503, "WATCHLIST_UNAVAILABLE"),
    };
    WatchlistWritePortError {
        status,
        code: code.to_owned(),
        message,
    }
}

fn string_array(
    value: Option<&Value>,
    field: &str,
) -> Result<Vec<String>, WatchlistWritePortError> {
    let Some(value) = value.filter(|value| !value.is_null()) else {
        return Ok(Vec::new());
    };
    let Some(values) = value.as_array() else {
        return Err(WatchlistWritePortError {
            status: 400,
            code: "WATCHLIST_INVALID".to_owned(),
            message: format!("{field} must be an array of strings"),
        });
    };
    values
        .iter()
        .map(|value| {
            value
                .as_str()
                .map(str::trim)
                .filter(|value| !value.is_empty())
                .map(str::to_owned)
                .ok_or_else(|| WatchlistWritePortError {
                    status: 400,
                    code: "WATCHLIST_INVALID".to_owned(),
                    message: format!("{field} must contain non-empty strings"),
                })
        })
        .collect()
}

fn invalid_query(message: impl Into<String>) -> WatchlistReadSnapshotError {
    WatchlistReadSnapshotError::Invalid(message.into())
}

fn decode_query_component(value: &str) -> Result<String, WatchlistReadSnapshotError> {
    percent_decode_str(&value.replace('+', " "))
        .decode_utf8()
        .map(|value| value.into_owned())
        .map_err(|_| invalid_query("invalid watchlist query encoding"))
}

fn parse_read_query(query: &str) -> Result<WatchlistReadQuery, WatchlistReadSnapshotError> {
    let mut parsed = WatchlistReadQuery {
        limit: DEFAULT_PAGE_LIMIT,
        ..WatchlistReadQuery::default()
    };
    for pair in query.split('&').filter(|pair| !pair.is_empty()) {
        let (raw_key, raw_value) = pair.split_once('=').unwrap_or((pair, ""));
        let key = decode_query_component(raw_key)?;
        let value = decode_query_component(raw_value)?;
        match key.as_str() {
            "groupId" => parsed.group_id = value,
            "cursor" => parsed.cursor = value,
            "query" => parsed.query = value,
            "sourceId" => parsed.source_id = value,
            "market" => parsed.market = normalize_market(&value)?,
            "limit" => {
                let limit = value
                    .parse::<usize>()
                    .map_err(|_| invalid_query("limit must be a positive integer"))?;
                if limit == 0 {
                    return Err(invalid_query("limit must be a positive integer"));
                }
                parsed.limit = limit.min(MAX_PAGE_LIMIT);
            }
            _ => {}
        }
    }
    Ok(parsed)
}

fn normalize_market(value: &str) -> Result<String, WatchlistReadSnapshotError> {
    match value.trim().to_uppercase().as_str() {
        "" => Ok(String::new()),
        "CN" => Ok("CN".to_owned()),
        "CNSH" | "SH" => Ok("SH".to_owned()),
        "CNSZ" | "SZ" => Ok("SZ".to_owned()),
        "HK" | "US" => Ok(value.trim().to_uppercase()),
        _ => Err(invalid_query(format!(
            "unsupported market {:?}",
            value.trim()
        ))),
    }
}

fn source_id_from_path(path: &str) -> Option<String> {
    let source_id = path
        .strip_prefix("/api/v1/watchlist/sources/")?
        .strip_suffix("/groups")?;
    if source_id.is_empty() || source_id.contains('/') {
        return None;
    }
    percent_decode_str(source_id)
        .decode_utf8()
        .ok()
        .map(|value| value.into_owned())
}

impl WatchlistMembershipSnapshotPort for ProductionWatchlistPort {
    fn memberships(
        &self,
        instrument_id: &str,
    ) -> Result<Memberships, WatchlistMembershipSnapshotError> {
        let memberships = self
            .store
            .get_memberships(instrument_id)
            .map_err(|e| WatchlistMembershipSnapshotError::Unavailable(e.to_string()))?;
        Ok(Memberships {
            instrument_id: memberships.instrument_id,
            revision: memberships.revision,
            groups: memberships
                .groups
                .into_iter()
                .map(|g| GroupRef {
                    group_id: g.group_id,
                    name: g.name,
                })
                .collect(),
        })
    }
}

impl WatchlistReadSnapshotPort for ProductionWatchlistPort {
    fn read(&self, path: &str, query: &str) -> Result<Value, WatchlistReadSnapshotError> {
        let parsed_query = parse_read_query(query)?;
        match path {
            "/api/v1/watchlist/groups" => {
                let groups = self
                    .store
                    .list_groups()
                    .map_err(watchlist_read_store_error)?;
                Ok(json!({ "groups": groups }))
            }
            "/api/v1/watchlist/items" => {
                let (items, next_cursor) = self
                    .store
                    .list_items_page(
                        option_value(&parsed_query.group_id),
                        option_value(&parsed_query.cursor),
                        parsed_query.limit,
                        option_value(&parsed_query.query),
                        option_value(&parsed_query.market),
                    )
                    .map_err(watchlist_read_store_error)?;
                let mut response = json!({ "items": items });
                if let Some(next_cursor) = next_cursor {
                    response["nextCursor"] = json!(next_cursor);
                }
                Ok(response)
            }
            "/api/v1/watchlist/sources" => {
                let sources = self
                    .store
                    .list_sources()
                    .map_err(watchlist_read_store_error)?;
                Ok(json!({ "sources": sources }))
            }
            "/api/v1/watchlist/bindings" => {
                let bindings = self
                    .store
                    .list_bindings()
                    .map_err(watchlist_read_store_error)?;
                let bindings = if parsed_query.source_id.is_empty() {
                    bindings
                } else {
                    bindings
                        .into_iter()
                        .filter(|binding| binding.source_id == parsed_query.source_id)
                        .collect()
                };
                Ok(json!({ "bindings": bindings }))
            }
            "/api/v1/watchlist/import-runs" => {
                let (runs, next_cursor) = self
                    .store
                    .list_import_runs_page(
                        option_value(&parsed_query.source_id),
                        option_value(&parsed_query.cursor),
                        parsed_query.limit,
                    )
                    .map_err(watchlist_read_store_error)?;
                let mut response = json!({ "items": runs });
                if let Some(next_cursor) = next_cursor {
                    response["nextCursor"] = json!(next_cursor);
                }
                Ok(response)
            }
            _ if source_id_from_path(path).is_some() => {
                let source_id = source_id_from_path(path).expect("source id checked above");
                if !self
                    .store
                    .source_exists(&source_id)
                    .map_err(watchlist_read_store_error)?
                {
                    return Err(WatchlistReadSnapshotError::NotFound);
                }
                let groups = self
                    .store
                    .list_remote_groups(&source_id)
                    .map_err(watchlist_read_store_error)?;
                Ok(json!({ "groups": groups }))
            }
            _ => Err(WatchlistReadSnapshotError::NotFound),
        }
    }
}

fn option_value(value: &str) -> Option<&str> {
    (!value.is_empty()).then_some(value)
}

fn watchlist_read_store_error(error: WatchlistStoreError) -> WatchlistReadSnapshotError {
    match error {
        WatchlistStoreError::NotFound => WatchlistReadSnapshotError::NotFound,
        WatchlistStoreError::InvalidInstrument(message)
        | WatchlistStoreError::Validation(message) => {
            WatchlistReadSnapshotError::Invalid(message)
        }
        other => WatchlistReadSnapshotError::Unavailable(other.to_string()),
    }
}

impl WatchlistWritePort for ProductionWatchlistPort {
    fn mutate(&self, mutation: &WatchlistWriteMutation) -> Result<Value, WatchlistWritePortError> {
        let route = mutation.value["route"].as_str().unwrap_or_default();
        let timestamp = time::OffsetDateTime::now_utc()
            .format(&time::format_description::well_known::Rfc3339)
            .unwrap_or_default();

        match route {
            "create-group" => {
                let name = mutation.value["name"].as_str().unwrap_or_default();
                let group = self
                    .store
                    .create_group(name, &timestamp)
                    .map_err(watchlist_store_error)?;
                Ok(json!({
                    "id": group.group_id,
                    "groupId": group.group_id,
                    "name": group.name,
                    "revision": group.revision,
                }))
            }
            "update-group" => {
                let group_id = mutation.value["groupId"].as_str().unwrap_or_default();
                let name = mutation.value["name"].as_str().unwrap_or_default();
                let expected_revision = mutation.value["expectedRevision"].as_i64().unwrap_or_default();
                let group = self
                    .store
                    .update_group(group_id, name, expected_revision, &timestamp)
                    .map_err(watchlist_store_error)?;
                Ok(json!({
                    "id": group.group_id,
                    "groupId": group.group_id,
                    "name": group.name,
                    "revision": group.revision,
                }))
            }
            "delete-group" => {
                let group_id = mutation.value["groupId"].as_str().unwrap_or_default();
                self.store
                    .delete_group(group_id)
                    .map_err(watchlist_store_error)?;
                Ok(json!({"deleted": true}))
            }
            "delete-binding" => {
                let binding_id = mutation.value["bindingId"].as_str().unwrap_or_default();
                self.store
                    .delete_binding(binding_id)
                    .map_err(watchlist_store_error)?;
                Ok(json!({"deleted": true}))
            }
            "preview-import" => {
                let source_id = mutation.value["sourceId"].as_str().unwrap_or_default();
                let remote_group_id = mutation.value["remoteGroupId"].as_str().unwrap_or_default();
                let local_group_id = mutation.value["localGroupId"].as_str();
                let new_group_name = mutation.value["newGroupName"].as_str();
                let preview = self
                    .store
                    .create_import_preview(source_id, remote_group_id, local_group_id, new_group_name, &timestamp)
                    .map_err(watchlist_store_error)?;
                Ok(json!({
                    "id": preview.preview_id.clone(),
                    "previewId": preview.preview_id,
                    "status": preview.status,
                    "sourceId": preview.source_id,
                    "remoteGroupId": preview.remote_group_id,
                }))
            }
            "commit-import" => {
                let preview_id = mutation.value["previewId"].as_str().unwrap_or_default();
                let delete_instrument_ids = string_array(
                    mutation.value.get("deleteInstrumentIds"),
                    "deleteInstrumentIds",
                )?;
                let run = self
                    .store
                    .commit_import_preview(preview_id, &delete_instrument_ids, &timestamp)
                    .map_err(watchlist_store_error)?;
                Ok(json!({
                    "id": run.run_id.clone(),
                    "runId": run.run_id,
                    "status": run.status,
                    "previewId": run.preview_id,
                }))
            }
            "batch-quotes" => Err(WatchlistWritePortError {
                status: 503,
                code: "MARKET_DATA_UNAVAILABLE".to_owned(),
                message: "market-data provider runtime is not configured".to_owned(),
            }),
            "replace-memberships" => {
                let instrument_id = mutation.value["instrumentId"].as_str().unwrap_or_default();
                let group_ids = string_array(mutation.value.get("groupIds"), "groupIds")?;
                let new_group_names = string_array(
                    mutation.value.get("newGroupNames"),
                    "newGroupNames",
                )?;
                let expected_revision = mutation.value["expectedRevision"].as_i64().unwrap_or_default();
                let memberships = self
                    .store
                    .replace_memberships(instrument_id, &group_ids, &new_group_names, expected_revision, &timestamp)
                    .map_err(watchlist_store_error)?;
                Ok(json!({
                    "instrumentId": memberships.instrument_id,
                    "revision": memberships.revision,
                    "groups": memberships.groups,
                }))
            }
            _ => Err(WatchlistWritePortError {
                status: 500,
                code: "INTERNAL_ERROR".to_owned(),
                message: format!("unknown watchlist mutation route: {route}"),
            }),
        }
    }
}

// ---------------------------------------------------------------------------
// Remote Watchlist
// ---------------------------------------------------------------------------

#[derive(Debug)]
pub(crate) struct ProductionRemoteWatchlistPort {
    pub(crate) _store: Arc<WatchlistStore>,
    pub(crate) active_provider_state: Arc<ActiveProviderState>,
}

impl RemoteWatchlistSnapshotPort for ProductionRemoteWatchlistPort {
    fn read(&self, _query: &str) -> Result<Value, RemoteWatchlistSnapshotError> {
        let snapshot = self.active_provider_state.snapshot();
        if snapshot.provider.is_none() || !snapshot.opend_ready {
            return Err(RemoteWatchlistSnapshotError::Unavailable(
                "remote watchlist provider is not configured".to_owned(),
            ));
        }
        Err(RemoteWatchlistSnapshotError::Unavailable(
            "remote watchlist provider is not configured".to_owned(),
        ))
    }
}

impl RemoteWatchlistWritePort for ProductionRemoteWatchlistPort {
    fn resolve(
        &self,
        _broker_id: Option<&str>,
        _account_id: Option<&str>,
    ) -> Result<RemoteWatchlistWriteResolution, RemoteWatchlistWritePortError> {
        let snapshot = self.active_provider_state.snapshot();
        if snapshot.provider.is_none() || !snapshot.opend_ready {
            return Err(RemoteWatchlistWritePortError::Unavailable(
                "remote watchlist provider is not configured".to_owned(),
            ));
        }
        Err(RemoteWatchlistWritePortError::Unavailable(
            "remote watchlist provider is not configured".to_owned(),
        ))
    }

    fn apply(
        &self,
        _resolution: &RemoteWatchlistWriteResolution,
        _action: &RemoteWatchlistWriteAction,
    ) -> Result<Option<Value>, RemoteWatchlistWritePortError> {
        let snapshot = self.active_provider_state.snapshot();
        if snapshot.provider.is_none() || !snapshot.opend_ready {
            return Err(RemoteWatchlistWritePortError::Unavailable(
                "remote watchlist provider is not configured".to_owned(),
            ));
        }
        Err(RemoteWatchlistWritePortError::Unavailable(
            "remote watchlist provider is not configured".to_owned(),
        ))
    }
}
