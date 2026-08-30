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
    RemoteWatchlistSnapshotError, RemoteWatchlistSnapshotPort, WatchlistMembershipSnapshotError,
    WatchlistMembershipSnapshotPort, WatchlistReadSnapshotError, WatchlistReadSnapshotPort,
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

#[derive(Debug, Default, PartialEq, Eq)]
struct RemoteWatchlistReadQuery {
    operation: String,
    remote_group_id: String,
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

fn parse_remote_read_query(
    query: &str,
) -> Result<RemoteWatchlistReadQuery, RemoteWatchlistSnapshotError> {
    let mut parsed = RemoteWatchlistReadQuery {
        operation: "groups".to_owned(),
        ..RemoteWatchlistReadQuery::default()
    };
    for pair in query.split('&').filter(|pair| !pair.is_empty()) {
        let (raw_key, raw_value) = pair.split_once('=').unwrap_or((pair, ""));
        let key_input = raw_key.replace('+', " ");
        let value_input = raw_value.replace('+', " ");
        let key = percent_decode_str(&key_input).decode_utf8().map_err(|_| {
            RemoteWatchlistSnapshotError::Invalid(
                "invalid remote watchlist query encoding".to_owned(),
            )
        })?;
        let value = percent_decode_str(&value_input)
            .decode_utf8()
            .map_err(|_| {
                RemoteWatchlistSnapshotError::Invalid(
                    "invalid remote watchlist query encoding".to_owned(),
                )
            })?;
        match key.as_ref() {
            "operation" => {
                let operation = value.trim().to_ascii_lowercase();
                if !matches!(operation.as_str(), "groups" | "members") {
                    return Err(RemoteWatchlistSnapshotError::Invalid(
                        "operation must be groups or members".to_owned(),
                    ));
                }
                parsed.operation = operation;
            }
            "remoteGroupId" => parsed.remote_group_id = value.trim().to_owned(),
            _ => {}
        }
    }
    if parsed.operation == "members" && parsed.remote_group_id.is_empty() {
        return Err(RemoteWatchlistSnapshotError::Invalid(
            "remoteGroupId is required for members operation".to_owned(),
        ));
    }
    if parsed.remote_group_id.len() > 512 || parsed.remote_group_id.chars().any(char::is_control) {
        return Err(RemoteWatchlistSnapshotError::Invalid(
            "remoteGroupId is invalid".to_owned(),
        ));
    }
    Ok(parsed)
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
        | WatchlistStoreError::Validation(message) => WatchlistReadSnapshotError::Invalid(message),
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
                let name = mutation
                    .value
                    .get("name")
                    .and_then(Value::as_str)
                    .map(str::trim)
                    .filter(|s| !s.is_empty())
                    .ok_or_else(|| WatchlistWritePortError {
                        status: 400,
                        code: "WATCHLIST_INVALID".to_owned(),
                        message: "name is required".to_owned(),
                    })?;
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
                let group_id = mutation
                    .value
                    .get("groupId")
                    .and_then(Value::as_str)
                    .map(str::trim)
                    .filter(|s| !s.is_empty())
                    .ok_or_else(|| WatchlistWritePortError {
                        status: 400,
                        code: "WATCHLIST_INVALID".to_owned(),
                        message: "groupId is required".to_owned(),
                    })?;
                let name = mutation
                    .value
                    .get("name")
                    .and_then(Value::as_str)
                    .map(str::trim)
                    .filter(|s| !s.is_empty())
                    .ok_or_else(|| WatchlistWritePortError {
                        status: 400,
                        code: "WATCHLIST_INVALID".to_owned(),
                        message: "name is required".to_owned(),
                    })?;
                let expected_revision = mutation
                    .value
                    .get("expectedRevision")
                    .and_then(Value::as_i64)
                    .ok_or_else(|| WatchlistWritePortError {
                        status: 400,
                        code: "WATCHLIST_INVALID".to_owned(),
                        message: "expectedRevision is required".to_owned(),
                    })?;
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
                let group_id = mutation
                    .value
                    .get("groupId")
                    .and_then(Value::as_str)
                    .map(str::trim)
                    .filter(|s| !s.is_empty())
                    .ok_or_else(|| WatchlistWritePortError {
                        status: 400,
                        code: "WATCHLIST_INVALID".to_owned(),
                        message: "groupId is required".to_owned(),
                    })?;
                self.store
                    .delete_group(group_id)
                    .map_err(watchlist_store_error)?;
                Ok(json!({"deleted": true}))
            }
            "delete-binding" => {
                let binding_id = mutation
                    .value
                    .get("bindingId")
                    .and_then(Value::as_str)
                    .map(str::trim)
                    .filter(|s| !s.is_empty())
                    .ok_or_else(|| WatchlistWritePortError {
                        status: 400,
                        code: "WATCHLIST_INVALID".to_owned(),
                        message: "bindingId is required".to_owned(),
                    })?;
                self.store
                    .delete_binding(binding_id)
                    .map_err(watchlist_store_error)?;
                Ok(json!({"deleted": true}))
            }
            "preview-import" => {
                let source_id = mutation
                    .value
                    .get("sourceId")
                    .and_then(Value::as_str)
                    .map(str::trim)
                    .filter(|s| !s.is_empty())
                    .ok_or_else(|| WatchlistWritePortError {
                        status: 400,
                        code: "WATCHLIST_INVALID".to_owned(),
                        message: "sourceId is required".to_owned(),
                    })?;
                let remote_group_id = mutation
                    .value
                    .get("remoteGroupId")
                    .and_then(Value::as_str)
                    .map(str::trim)
                    .filter(|s| !s.is_empty())
                    .ok_or_else(|| WatchlistWritePortError {
                        status: 400,
                        code: "WATCHLIST_INVALID".to_owned(),
                        message: "remoteGroupId is required".to_owned(),
                    })?;
                let local_group_id = mutation
                    .value
                    .get("localGroupId")
                    .and_then(Value::as_str)
                    .map(str::trim)
                    .filter(|s| !s.is_empty());
                let new_group_name = mutation
                    .value
                    .get("newGroupName")
                    .and_then(Value::as_str)
                    .map(str::trim)
                    .filter(|s| !s.is_empty());
                let preview = self
                    .store
                    .create_import_preview(
                        source_id,
                        remote_group_id,
                        local_group_id,
                        new_group_name,
                        &timestamp,
                    )
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
                let preview_id = mutation
                    .value
                    .get("previewId")
                    .and_then(Value::as_str)
                    .map(str::trim)
                    .filter(|s| !s.is_empty())
                    .ok_or_else(|| WatchlistWritePortError {
                        status: 400,
                        code: "WATCHLIST_INVALID".to_owned(),
                        message: "previewId is required".to_owned(),
                    })?;
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
                let instrument_id = mutation
                    .value
                    .get("instrumentId")
                    .and_then(Value::as_str)
                    .map(str::trim)
                    .filter(|s| !s.is_empty())
                    .ok_or_else(|| WatchlistWritePortError {
                        status: 400,
                        code: "WATCHLIST_INVALID".to_owned(),
                        message: "instrumentId is required".to_owned(),
                    })?;
                let group_ids = string_array(mutation.value.get("groupIds"), "groupIds")?;
                let new_group_names =
                    string_array(mutation.value.get("newGroupNames"), "newGroupNames")?;
                let expected_revision = mutation
                    .value
                    .get("expectedRevision")
                    .and_then(Value::as_i64)
                    .filter(|r| *r >= 0)
                    .ok_or_else(|| WatchlistWritePortError {
                        status: 400,
                        code: "WATCHLIST_INVALID".to_owned(),
                        message: "expectedRevision must be non-negative".to_owned(),
                    })?;
                let memberships = self
                    .store
                    .replace_memberships(
                        instrument_id,
                        &group_ids,
                        &new_group_names,
                        expected_revision,
                        &timestamp,
                    )
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
    pub(crate) trade_runtime: Option<Arc<super::SharedTradeReadRuntime>>,
}

impl RemoteWatchlistSnapshotPort for ProductionRemoteWatchlistPort {
    fn read(&self, query: &str) -> Result<Value, RemoteWatchlistSnapshotError> {
        let parsed = parse_remote_read_query(query)?;
        let snapshot = self.active_provider_state.snapshot();
        if snapshot.provider.is_none() || !snapshot.opend_ready {
            return Err(RemoteWatchlistSnapshotError::Unavailable(
                "remote watchlist provider is not configured".to_owned(),
            ));
        }
        let reader = self
            .trade_runtime
            .as_ref()
            .and_then(|runtime| runtime.remote_watchlist_reader())
            .ok_or_else(|| {
                RemoteWatchlistSnapshotError::Unavailable(
                    "remote watchlist reader is unavailable".to_owned(),
                )
            })?;
        let now = crate::product::product_production_ports::provider_now_rfc3339();
        let entries = if parsed.operation == "groups" {
            reader
                .groups()
                .map_err(|error| RemoteWatchlistSnapshotError::Unavailable(error.to_string()))?
                .into_iter()
                .map(|mut value| {
                    if let Some(name) = value.get("name").and_then(Value::as_str).map(str::to_owned)
                    {
                        value["sourceId"] = json!("futu:default");
                        value["remoteGroupId"] = json!(format!("futu-group:{name}"));
                    }
                    value
                })
                .collect::<Vec<_>>()
        } else {
            let group_id = parsed
                .remote_group_id
                .strip_prefix("futu-group:")
                .unwrap_or(&parsed.remote_group_id);
            reader
                .members(group_id)
                .map_err(|error| RemoteWatchlistSnapshotError::Unavailable(error.to_string()))?
        };
        Ok(json!({
            "asOf": now,
            "entries": entries,
            "hasMore": false,
            "total": entries.len(),
            "metadata": {"source": "futu-opend"},
            "provider": {"brokerId": "futu", "featureId": "watchlist.remote.list", "capability": "available", "selectionReason": "active_provider", "resolvedAt": now, "asOf": now}
        }))
    }
}

impl RemoteWatchlistWritePort for ProductionRemoteWatchlistPort {
    fn resolve(
        &self,
        broker_id: Option<&str>,
        _account_id: Option<&str>,
    ) -> Result<RemoteWatchlistWriteResolution, RemoteWatchlistWritePortError> {
        let snapshot = self.active_provider_state.snapshot();
        if snapshot.provider.is_none() || !snapshot.opend_ready {
            return Err(RemoteWatchlistWritePortError::Unavailable(
                "remote watchlist provider is not configured".to_owned(),
            ));
        }
        if broker_id.is_some_and(|id| !id.eq_ignore_ascii_case("futu")) {
            return Err(RemoteWatchlistWritePortError::CapabilityUnavailable(
                "only futu remote watchlists are supported".to_owned(),
            ));
        }
        Ok(RemoteWatchlistWriteResolution {
            broker_id: "futu".to_owned(),
            security_firm: "Futu/Moomoo via OpenD".to_owned(),
            capability: "available".to_owned(),
            selection_reason: if broker_id.is_some() {
                "explicit_broker".to_owned()
            } else {
                "active_provider".to_owned()
            },
        })
    }

    fn apply(
        &self,
        _resolution: &RemoteWatchlistWriteResolution,
        action: &RemoteWatchlistWriteAction,
    ) -> Result<Option<Value>, RemoteWatchlistWritePortError> {
        let snapshot = self.active_provider_state.snapshot();
        if snapshot.provider.is_none() || !snapshot.opend_ready {
            return Err(RemoteWatchlistWritePortError::Unavailable(
                "remote watchlist provider is not configured".to_owned(),
            ));
        }
        let runtime = self
            .trade_runtime
            .as_ref()
            .and_then(|runtime| runtime.remote_watchlist_writer())
            .ok_or_else(|| {
                RemoteWatchlistWritePortError::Unavailable(
                    "remote watchlist writer is unavailable".to_owned(),
                )
            })?;
        let payload = action.payload.as_ref().ok_or_else(|| {
            RemoteWatchlistWritePortError::Internal(
                "remote watchlist payload is required".to_owned(),
            )
        })?;
        let group_name = payload
            .get("groupName")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .ok_or_else(|| RemoteWatchlistWritePortError::Provider {
                status: Some(400),
                message: "groupName is required".to_owned(),
            })?;
        let operation = payload
            .get("op")
            .and_then(Value::as_i64)
            .and_then(|value| match value {
                1 => Some("add"),
                2 => Some("delete"),
                3 => Some("move_out"),
                _ => None,
            })
            .ok_or_else(|| RemoteWatchlistWritePortError::Provider {
                status: Some(400),
                message: "op must be 1 (add), 2 (delete), or 3 (move_out)".to_owned(),
            })?;
        let securities = payload
            .get("securityList")
            .and_then(Value::as_array)
            .filter(|values| !values.is_empty())
            .ok_or_else(|| RemoteWatchlistWritePortError::Provider {
                status: Some(400),
                message: "securityList must contain at least one security".to_owned(),
            })?;
        validate_remote_security_list(securities)?;
        runtime
            .modify(group_name, operation, securities)
            .map(Some)
            .map_err(|error| RemoteWatchlistWritePortError::Provider {
                status: None,
                message: error.to_string(),
            })
    }
}

fn validate_remote_security_list(
    securities: &[Value],
) -> Result<(), RemoteWatchlistWritePortError> {
    for (index, security) in securities.iter().enumerate() {
        let object =
            security
                .as_object()
                .ok_or_else(|| RemoteWatchlistWritePortError::Provider {
                    status: Some(400),
                    message: format!("securityList[{index}] must be an object"),
                })?;
        let market = object
            .get("market")
            .and_then(Value::as_i64)
            .and_then(|value| i32::try_from(value).ok());
        if !market.is_some_and(|value| {
            matches!(
                value,
                1 | 11 | 21 | 22 | 31 | 41 | 51 | 61 | 71 | 81 | 91 | 101
            )
        }) {
            return Err(RemoteWatchlistWritePortError::Provider {
                status: Some(400),
                message: format!("securityList[{index}].market is invalid"),
            });
        }
        let valid_code = object
            .get("code")
            .and_then(Value::as_str)
            .map(str::trim)
            .is_some_and(|value| !value.is_empty() && !value.chars().any(char::is_control));
        if !valid_code {
            return Err(RemoteWatchlistWritePortError::Provider {
                status: Some(400),
                message: format!("securityList[{index}].code is required"),
            });
        }
    }
    Ok(())
}

#[cfg(test)]
mod remote_watchlist_tests;
