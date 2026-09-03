use std::collections::BTreeMap;

use serde::Deserialize;
use serde::de::DeserializeOwned;
use serde_json::{Map, Value, json};

pub const WATCHLIST_WRITE_ROUTES: [(&str, &str); 8] = [
    ("DELETE", "/api/v1/watchlist/bindings"),
    ("DELETE", "/api/v1/watchlist/groups/{groupId}"),
    ("PATCH", "/api/v1/watchlist/groups/{groupId}"),
    ("POST", "/api/v1/watchlist/groups"),
    ("POST", "/api/v1/watchlist/imports/preview"),
    ("POST", "/api/v1/watchlist/imports/{previewId}/commit"),
    ("POST", "/api/v1/watchlist/quotes/batch"),
    (
        "PUT",
        "/api/v1/watchlist/instruments/{market}/{symbol}/memberships",
    ),
];

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct WatchlistWriteRequest {
    pub method: String,
    pub path: String,
    pub body: Option<Vec<u8>>,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct WatchlistWriteMutation {
    pub value: Value,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct WatchlistWritePortError {
    pub status: u16,
    pub code: String,
    pub message: String,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct WatchlistWriteResponse {
    pub status: u16,
    pub headers: BTreeMap<String, String>,
    pub body: Value,
}

/// The integration branch may inject a Go-owned rehearsal adapter here.
/// Nothing in this port opens SQLite, starts a quote worker, connects OpenD,
/// or changes the production owner. The only call site is the explicit test
/// leaf used by the frozen compatibility replay.
pub trait WatchlistWritePort: Send + Sync {
    fn mutate(&self, mutation: &WatchlistWriteMutation) -> Result<Value, WatchlistWritePortError>;
}

#[derive(Default, Deserialize)]
#[serde(rename_all = "camelCase")]
struct CreateGroupInput {
    name: Option<String>,
}

#[derive(Default, Deserialize)]
#[serde(rename_all = "camelCase")]
struct UpdateGroupInput {
    name: Option<String>,
    expected_revision: Option<i64>,
}

#[derive(Default, Deserialize)]
#[serde(rename_all = "camelCase")]
struct PreviewInput {
    source_id: Option<String>,
    remote_group_id: Option<String>,
    local_group_id: Option<String>,
    new_group_name: Option<String>,
}

#[derive(Default, Deserialize)]
#[serde(rename_all = "camelCase")]
struct CommitInput {
    delete_instrument_ids: Option<Vec<String>>,
}

#[derive(Default, Deserialize)]
#[serde(rename_all = "camelCase")]
struct QuoteBatchInput {
    instrument_ids: Option<Vec<String>>,
}

#[derive(Default, Deserialize)]
#[serde(rename_all = "camelCase")]
struct MembershipInput {
    group_ids: Option<Vec<String>>,
    new_group_names: Option<Vec<String>>,
    expected_revision: Option<i64>,
}

pub fn watchlist_write_routes() -> &'static [(&'static str, &'static str); 8] {
    &WATCHLIST_WRITE_ROUTES
}

pub fn dispatch_watchlist_write(
    request: &WatchlistWriteRequest,
    port: Option<&dyn WatchlistWritePort>,
    timestamp: &str,
) -> WatchlistWriteResponse {
    let (path, query) = split_path_query(&request.path);

    if request.method == "DELETE" && path == "/api/v1/watchlist/bindings" {
        let query_values = match parse_query(query) {
            Ok(values) => values,
            Err(()) => return bad_request("invalid watchlist binding query", timestamp),
        };
        let mutation = object_mutation(
            "delete-binding",
            json!({"bindingId": first_query_value(&query_values, "bindingId").unwrap_or_default()}),
        );
        return invoke_or_unavailable(mutation, port, timestamp);
    }

    if let Some(group_id) = path_suffix(path, "/api/v1/watchlist/groups/") {
        if group_id.contains('/') {
            return not_found(timestamp);
        }
        if request.method == "DELETE" {
            return invoke_or_unavailable(
                object_mutation("delete-group", json!({"groupId": group_id})),
                port,
                timestamp,
            );
        }
        if request.method == "PATCH" {
            let input: UpdateGroupInput = match decode_json(request.body.as_deref()) {
                Ok(input) => input,
                Err(()) => return bad_request("invalid watchlist group payload", timestamp),
            };
            let name = input.name.unwrap_or_default();
            let expected_revision = input.expected_revision.unwrap_or_default();
            if name.is_empty() || expected_revision < 1 {
                return bad_request("invalid watchlist group payload", timestamp);
            }
            return invoke_or_unavailable(
                object_mutation(
                    "update-group",
                    json!({
                        "groupId": group_id,
                        "name": name,
                        "expectedRevision": expected_revision,
                    }),
                ),
                port,
                timestamp,
            );
        }
    }

    if request.method == "POST" && path == "/api/v1/watchlist/groups" {
        let input: CreateGroupInput = match decode_json(request.body.as_deref()) {
            Ok(input) => input,
            Err(()) => return bad_request("invalid watchlist group payload", timestamp),
        };
        let name = input.name.unwrap_or_default();
        if name.is_empty() {
            return bad_request("invalid watchlist group payload", timestamp);
        }
        return invoke_or_unavailable(
            object_mutation("create-group", json!({"name": name})),
            port,
            timestamp,
        );
    }

    if request.method == "POST" && path == "/api/v1/watchlist/imports/preview" {
        let input: PreviewInput = match decode_json(request.body.as_deref()) {
            Ok(input) => input,
            Err(()) => {
                return bad_request("invalid watchlist import preview payload", timestamp);
            }
        };
        let source_id = input.source_id.unwrap_or_default();
        let remote_group_id = input.remote_group_id.unwrap_or_default();
        if source_id.is_empty() || remote_group_id.is_empty() {
            return bad_request("invalid watchlist import preview payload", timestamp);
        }
        return invoke_or_unavailable(
            object_mutation(
                "preview-import",
                json!({
                    "sourceId": source_id,
                    "remoteGroupId": remote_group_id,
                    "localGroupId": input.local_group_id.unwrap_or_default(),
                    "newGroupName": input.new_group_name.unwrap_or_default(),
                }),
            ),
            port,
            timestamp,
        );
    }

    if request.method == "POST"
        && let Some(preview_id) = path_suffix(path, "/api/v1/watchlist/imports/")
        && let Some(preview_id) = preview_id.strip_suffix("/commit")
        && !preview_id.is_empty()
        && !preview_id.contains('/')
    {
        let input = if request.body.as_deref().is_none_or(|body| body.is_empty()) {
            CommitInput::default()
        } else {
            match decode_json(request.body.as_deref()) {
                Ok(input) => input,
                Err(()) => {
                    return bad_request("invalid watchlist import commit payload", timestamp);
                }
            }
        };
        return invoke_or_unavailable(
            object_mutation(
                "commit-import",
                json!({
                    "previewId": preview_id,
                    "deleteInstrumentIds": input.delete_instrument_ids,
                }),
            ),
            port,
            timestamp,
        );
    }

    if request.method == "POST" && path == "/api/v1/watchlist/quotes/batch" {
        let input: QuoteBatchInput = match decode_json(request.body.as_deref()) {
            Ok(input) => input,
            Err(()) => return bad_request("invalid watchlist quote payload", timestamp),
        };
        let instrument_ids = input.instrument_ids.unwrap_or_default();
        if instrument_ids.is_empty() {
            return bad_request("invalid watchlist quote payload", timestamp);
        }
        return invoke_or_unavailable(
            object_mutation("batch-quotes", json!({"instrumentIds": instrument_ids})),
            port,
            timestamp,
        );
    }

    if request.method == "PUT"
        && let Some(value) = path_suffix(path, "/api/v1/watchlist/instruments/")
        && let Some(value) = value.strip_suffix("/memberships")
    {
        let mut parts = value.split('/');
        let market = parts.next().unwrap_or_default();
        let symbol = parts.next().unwrap_or_default();
        if market.is_empty() || symbol.is_empty() || parts.next().is_some() {
            return not_found(timestamp);
        }
        let input: MembershipInput = match decode_json(request.body.as_deref()) {
            Ok(input) => input,
            Err(()) => return bad_request("invalid watchlist memberships payload", timestamp),
        };
        return invoke_or_unavailable(
            object_mutation(
                "replace-memberships",
                json!({
                    "instrumentId": format!("{market}.{symbol}"),
                    "groupIds": input.group_ids,
                    "newGroupNames": input.new_group_names,
                    "expectedRevision": input.expected_revision.unwrap_or_default(),
                }),
            ),
            port,
            timestamp,
        );
    }

    not_found(timestamp)
}

fn invoke_or_unavailable(
    mutation: WatchlistWriteMutation,
    port: Option<&dyn WatchlistWritePort>,
    timestamp: &str,
) -> WatchlistWriteResponse {
    let Some(port) = port else {
        return error_response(
            503,
            "WATCHLIST_UNAVAILABLE",
            "watchlist service is unavailable",
            timestamp,
        );
    };
    match port.mutate(&mutation) {
        Ok(data) => success_response(data, timestamp),
        Err(error) => error_response(error.status, &error.code, &error.message, timestamp),
    }
}

fn object_mutation(route: &str, fields: Value) -> WatchlistWriteMutation {
    let mut object = match fields {
        Value::Object(object) => object,
        _ => Map::new(),
    };
    object.insert("route".to_owned(), Value::String(route.to_owned()));
    WatchlistWriteMutation {
        value: Value::Object(object),
    }
}

fn decode_json<T: DeserializeOwned + Default>(body: Option<&[u8]>) -> Result<T, ()> {
    let body = body.filter(|body| !body.is_empty()).ok_or(())?;
    let mut decoder = serde_json::Deserializer::from_slice(body);
    let value = Value::deserialize(&mut decoder).map_err(|_| ())?;
    if value.is_null() {
        return Ok(T::default());
    }
    serde_json::from_value(value).map_err(|_| ())
}

fn split_path_query(path: &str) -> (&str, &str) {
    path.split_once('?').unwrap_or((path, ""))
}

fn path_suffix<'a>(path: &'a str, prefix: &str) -> Option<&'a str> {
    let suffix = path.strip_prefix(prefix)?;
    if suffix.is_empty() {
        return None;
    }
    Some(suffix)
}

fn parse_query(query: &str) -> Result<Vec<(String, String)>, ()> {
    if query.is_empty() {
        return Ok(Vec::new());
    }
    query
        .split('&')
        .map(|pair| {
            let (name, value) = pair.split_once('=').unwrap_or((pair, ""));
            Ok((
                decode_query_component(name)?,
                decode_query_component(value)?,
            ))
        })
        .collect()
}

fn first_query_value(values: &[(String, String)], wanted: &str) -> Option<String> {
    values
        .iter()
        .find(|(name, _)| name == wanted)
        .map(|(_, value)| value.clone())
}

fn decode_query_component(value: &str) -> Result<String, ()> {
    let mut bytes = Vec::with_capacity(value.len());
    let raw = value.as_bytes();
    let mut index = 0;
    while index < raw.len() {
        match raw[index] {
            b'+' => bytes.push(b' '),
            b'%' => {
                if index + 2 >= raw.len() {
                    return Err(());
                }
                let high = hex_value(raw[index + 1]).ok_or(())?;
                let low = hex_value(raw[index + 2]).ok_or(())?;
                bytes.push((high << 4) | low);
                index += 2;
            }
            byte => bytes.push(byte),
        }
        index += 1;
    }
    String::from_utf8(bytes).map_err(|_| ())
}

fn hex_value(value: u8) -> Option<u8> {
    match value {
        b'0'..=b'9' => Some(value - b'0'),
        b'a'..=b'f' => Some(value - b'a' + 10),
        b'A'..=b'F' => Some(value - b'A' + 10),
        _ => None,
    }
}

fn success_response(data: Value, timestamp: &str) -> WatchlistWriteResponse {
    WatchlistWriteResponse {
        status: 200,
        headers: json_headers(),
        body: json!({"ok": true, "data": data, "timestamp": timestamp}),
    }
}

fn bad_request(message: &str, timestamp: &str) -> WatchlistWriteResponse {
    error_response(400, "BAD_REQUEST", message, timestamp)
}

fn not_found(timestamp: &str) -> WatchlistWriteResponse {
    error_response(404, "NOT_FOUND", "resource not found", timestamp)
}

fn error_response(
    status: u16,
    code: &str,
    message: &str,
    timestamp: &str,
) -> WatchlistWriteResponse {
    WatchlistWriteResponse {
        status,
        headers: json_headers(),
        body: json!({
            "ok": false,
            "error": {"code": code, "message": message},
            "timestamp": timestamp,
        }),
    }
}

fn json_headers() -> BTreeMap<String, String> {
    BTreeMap::from([(
        "Content-Type".to_owned(),
        "application/json; charset=utf-8".to_owned(),
    )])
}

#[cfg(test)]
mod tests {
    use super::*;

    #[derive(Debug)]
    struct RecordingPort;

    impl WatchlistWritePort for RecordingPort {
        fn mutate(
            &self,
            _mutation: &WatchlistWriteMutation,
        ) -> Result<Value, WatchlistWritePortError> {
            Ok(json!({"deleted": true}))
        }
    }

    #[test]
    fn route_inventory_is_exactly_the_eight_watchlist_mutations() {
        assert_eq!(watchlist_write_routes(), &WATCHLIST_WRITE_ROUTES,);
    }

    #[test]
    fn malformed_body_and_query_fail_before_the_port() {
        let malformed = dispatch_watchlist_write(
            &WatchlistWriteRequest {
                method: "POST".to_owned(),
                path: "/api/v1/watchlist/groups".to_owned(),
                body: Some(b"{".to_vec()),
            },
            Some(&RecordingPort),
            "2026-08-23T00:00:00Z",
        );
        assert_eq!(malformed.status, 400);
        assert_eq!(malformed.body["error"]["code"], "BAD_REQUEST");

        let malformed_query = dispatch_watchlist_write(
            &WatchlistWriteRequest {
                method: "DELETE".to_owned(),
                path: "/api/v1/watchlist/bindings?%zz".to_owned(),
                body: None,
            },
            Some(&RecordingPort),
            "2026-08-23T00:00:00Z",
        );
        assert_eq!(malformed_query.status, 400);
        assert_eq!(
            malformed_query.body["error"]["message"],
            "invalid watchlist binding query"
        );
    }

    #[test]
    fn valid_route_fails_closed_without_a_test_port() {
        let response = dispatch_watchlist_write(
            &WatchlistWriteRequest {
                method: "POST".to_owned(),
                path: "/api/v1/watchlist/groups".to_owned(),
                body: Some(br#"{"name":"Growth"}"#.to_vec()),
            },
            None,
            "2026-08-23T00:00:00Z",
        );
        assert_eq!(response.status, 503);
        assert_eq!(response.body["error"]["code"], "WATCHLIST_UNAVAILABLE");
    }

    #[test]
    fn trailing_json_is_ignored_like_the_go_json_binding() {
        let response = dispatch_watchlist_write(
            &WatchlistWriteRequest {
                method: "POST".to_owned(),
                path: "/api/v1/watchlist/groups".to_owned(),
                body: Some(br#"{"name":"Growth"}{"ignored":true}"#.to_vec()),
            },
            Some(&RecordingPort),
            "2026-08-23T00:00:00Z",
        );
        assert_eq!(response.status, 200);
        assert_eq!(response.body["data"]["deleted"], true);
    }
}
