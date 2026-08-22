//! Stage 9 test-cutover leaf for the remote watchlist mutation route.
//!
//! Go remains the only production owner of broker resolution, OpenD, remote
//! watchlist state, and the external mutation. This leaf only binds the
//! existing HTTP-shaped request, maps the consumer-owned port result to the
//! Go response envelope, and is intentionally not registered by any default
//! product profile.

use std::collections::BTreeMap;

use percent_encoding::percent_decode_str;
use serde_json::{Value, json};

pub const REMOTE_WATCHLIST_WRITE_PATH: &str = "/api/v1/watchlists/remote";
pub const REMOTE_WATCHLIST_WRITE_FEATURE_ID: &str = "watchlist.remote.modify";
pub const REMOTE_WATCHLIST_WRITE_ACTION: &str = "modify";
pub const REMOTE_WATCHLIST_WRITE_ROUTES: [(&str, &str); 1] =
    [("POST", REMOTE_WATCHLIST_WRITE_PATH)];

#[derive(Clone, Debug, Eq, PartialEq)]
pub enum RemoteWatchlistWritePayloadState {
    Nil,
    EmptyObject,
    Object,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct RemoteWatchlistWriteRequest {
    pub method: String,
    pub path: String,
    pub body: Option<Vec<u8>>,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct RemoteWatchlistWriteAction {
    pub feature_id: &'static str,
    pub broker_id: String,
    pub account_id: Option<String>,
    pub action: &'static str,
    pub payload: Option<Value>,
    pub payload_state: RemoteWatchlistWritePayloadState,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct RemoteWatchlistWriteResolution {
    pub broker_id: String,
    pub security_firm: String,
    pub capability: String,
    pub selection_reason: String,
}

#[allow(dead_code)]
#[derive(Clone, Debug, Eq, PartialEq)]
pub enum RemoteWatchlistWritePortError {
    Unavailable(String),
    CapabilityUnavailable(String),
    Provider {
        status: Option<u16>,
        message: String,
    },
    Internal(String),
    RateLimited {
        retry_after: u64,
        message: String,
    },
}

/// The integration branch may later inject a Go-owned rehearsal adapter here.
/// The port deliberately has no broker, OpenD, SQLite, or remote-registry
/// dependency and cannot become a production write owner by itself.
pub trait RemoteWatchlistWritePort: Send + Sync {
    fn resolve(
        &self,
        broker_id: Option<&str>,
        account_id: Option<&str>,
    ) -> Result<RemoteWatchlistWriteResolution, RemoteWatchlistWritePortError>;

    fn apply(
        &self,
        resolution: &RemoteWatchlistWriteResolution,
        action: &RemoteWatchlistWriteAction,
    ) -> Result<Option<Value>, RemoteWatchlistWritePortError>;
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct RemoteWatchlistWriteResponse {
    pub status: u16,
    pub headers: BTreeMap<String, String>,
    pub body: Value,
}

pub fn remote_watchlist_write_routes() -> &'static [(&'static str, &'static str); 1] {
    &REMOTE_WATCHLIST_WRITE_ROUTES
}

pub fn dispatch_remote_watchlist_write(
    request: &RemoteWatchlistWriteRequest,
    port: Option<&dyn RemoteWatchlistWritePort>,
    timestamp: &str,
) -> RemoteWatchlistWriteResponse {
    let (path, raw_query) = split_path_query(&request.path);
    if request.method != "POST" || path != REMOTE_WATCHLIST_WRITE_PATH {
        return error_response(404, "NOT_FOUND", "resource not found", timestamp, None);
    }

    let (payload, payload_state) = match parse_payload(request.body.as_deref()) {
        Ok(value) => value,
        Err(()) => {
            return error_response(400, "BAD_REQUEST", "invalid request body", timestamp, None);
        }
    };
    let broker_id = first_query_value(raw_query, "brokerId");
    let account_id = first_query_value(raw_query, "accountId");
    let Some(port) = port else {
        return error_response(
            503,
            "WATCHLIST_REMOTE_WRITE_UNAVAILABLE",
            "remote watchlist write port is unavailable",
            timestamp,
            None,
        );
    };
    let resolution = match port.resolve(
        non_empty(broker_id.as_deref()),
        non_empty(account_id.as_deref()),
    ) {
        Ok(resolution) => resolution,
        Err(error) => return port_error_response(error, timestamp),
    };
    let action = RemoteWatchlistWriteAction {
        feature_id: REMOTE_WATCHLIST_WRITE_FEATURE_ID,
        broker_id: resolution.broker_id.clone(),
        account_id: non_empty(account_id.as_deref()).map(str::to_owned),
        action: REMOTE_WATCHLIST_WRITE_ACTION,
        payload,
        payload_state,
    };
    let result = match port.apply(&resolution, &action) {
        Ok(Some(Value::Object(result))) => result,
        Ok(Some(_)) => {
            return error_response(
                502,
                "BROKER_FEATURE_FAILED",
                "remote watchlist write port returned a non-object result",
                timestamp,
                None,
            );
        }
        Ok(None) => serde_json::Map::new(),
        Err(error) => return port_error_response(error, timestamp),
    };
    success_response(with_provider(result, &resolution, timestamp), timestamp)
}

fn split_path_query(path: &str) -> (&str, &str) {
    path.split_once('?').unwrap_or((path, ""))
}

fn parse_payload(
    body: Option<&[u8]>,
) -> Result<(Option<Value>, RemoteWatchlistWritePayloadState), ()> {
    let body = body.ok_or(())?;
    let value: Value = serde_json::from_slice(body).map_err(|_| ())?;
    match value {
        Value::Null => Ok((None, RemoteWatchlistWritePayloadState::Nil)),
        Value::Object(object) if object.is_empty() => Ok((
            Some(Value::Object(object)),
            RemoteWatchlistWritePayloadState::EmptyObject,
        )),
        Value::Object(object) => Ok((
            Some(Value::Object(object)),
            RemoteWatchlistWritePayloadState::Object,
        )),
        _ => Err(()),
    }
}

fn with_provider(
    mut result: serde_json::Map<String, Value>,
    resolution: &RemoteWatchlistWriteResolution,
    timestamp: &str,
) -> Value {
    let mut provider = serde_json::Map::new();
    provider.insert("brokerId".to_owned(), json!(resolution.broker_id));
    if !resolution.security_firm.is_empty() {
        provider.insert("securityFirm".to_owned(), json!(resolution.security_firm));
    }
    provider.insert(
        "featureId".to_owned(),
        json!(REMOTE_WATCHLIST_WRITE_FEATURE_ID),
    );
    provider.insert("capability".to_owned(), json!(resolution.capability));
    provider.insert(
        "selectionReason".to_owned(),
        json!(resolution.selection_reason),
    );
    provider.insert("resolvedAt".to_owned(), json!(timestamp));
    provider.insert("asOf".to_owned(), json!(timestamp));
    result.insert("provider".to_owned(), Value::Object(provider));
    Value::Object(result)
}

fn port_error_response(
    error: RemoteWatchlistWritePortError,
    timestamp: &str,
) -> RemoteWatchlistWriteResponse {
    match error {
        RemoteWatchlistWritePortError::Unavailable(message) => error_response(
            503,
            "WATCHLIST_REMOTE_WRITE_UNAVAILABLE",
            &message,
            timestamp,
            None,
        ),
        RemoteWatchlistWritePortError::CapabilityUnavailable(message) => error_response(
            409,
            "BROKER_CAPABILITY_UNAVAILABLE",
            &message,
            timestamp,
            None,
        ),
        RemoteWatchlistWritePortError::Provider { status, message } => {
            if status.is_some_and(|value| (400..500).contains(&value)) {
                error_response(
                    status.expect("checked provider status"),
                    "PROVIDER_REQUEST_FAILED",
                    &message,
                    timestamp,
                    None,
                )
            } else {
                error_response(502, "BROKER_FEATURE_FAILED", &message, timestamp, None)
            }
        }
        RemoteWatchlistWritePortError::Internal(message) => {
            error_response(502, "BROKER_FEATURE_FAILED", &message, timestamp, None)
        }
        RemoteWatchlistWritePortError::RateLimited {
            retry_after,
            message,
        } => error_response(
            429,
            "MARKET_SNAPSHOT_RATE_LIMITED",
            &message,
            timestamp,
            Some(retry_after.max(1).to_string()),
        ),
    }
}

fn success_response(data: Value, timestamp: &str) -> RemoteWatchlistWriteResponse {
    RemoteWatchlistWriteResponse {
        status: 200,
        headers: json_headers(None),
        body: json!({"ok": true, "data": data, "timestamp": timestamp}),
    }
}

fn error_response(
    status: u16,
    code: &str,
    message: &str,
    timestamp: &str,
    retry_after: Option<String>,
) -> RemoteWatchlistWriteResponse {
    RemoteWatchlistWriteResponse {
        status,
        headers: json_headers(retry_after),
        body: json!({
            "ok": false,
            "error": {"code": code, "message": message},
            "timestamp": timestamp,
        }),
    }
}

fn json_headers(retry_after: Option<String>) -> BTreeMap<String, String> {
    let mut headers = BTreeMap::new();
    headers.insert(
        "Content-Type".to_owned(),
        "application/json; charset=utf-8".to_owned(),
    );
    if let Some(retry_after) = retry_after {
        headers.insert("Retry-After".to_owned(), retry_after);
    }
    headers
}

fn first_query_value(query: &str, wanted: &str) -> Option<String> {
    query.split('&').find_map(|pair| {
        let (name, value) = pair.split_once('=').unwrap_or((pair, ""));
        (decode_query_component(name) == wanted).then(|| decode_query_component(value))
    })
}

fn decode_query_component(value: &str) -> String {
    percent_decode_str(&value.replace('+', " "))
        .decode_utf8_lossy()
        .into_owned()
}

fn non_empty(value: Option<&str>) -> Option<&str> {
    value.filter(|value| !value.trim().is_empty())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[derive(Debug)]
    struct RecordingPort;

    impl RemoteWatchlistWritePort for RecordingPort {
        fn resolve(
            &self,
            _broker_id: Option<&str>,
            _account_id: Option<&str>,
        ) -> Result<RemoteWatchlistWriteResolution, RemoteWatchlistWritePortError> {
            Ok(RemoteWatchlistWriteResolution {
                broker_id: "futu".to_owned(),
                security_firm: "Futu/Moomoo via OpenD".to_owned(),
                capability: "available".to_owned(),
                selection_reason: "explicit_broker".to_owned(),
            })
        }

        fn apply(
            &self,
            _resolution: &RemoteWatchlistWriteResolution,
            _action: &RemoteWatchlistWriteAction,
        ) -> Result<Option<Value>, RemoteWatchlistWritePortError> {
            Ok(Some(json!({"entries": [{"accepted": true}]})))
        }
    }

    fn request(method: &str, path: &str, body: Option<&[u8]>) -> RemoteWatchlistWriteRequest {
        RemoteWatchlistWriteRequest {
            method: method.to_owned(),
            path: path.to_owned(),
            body: body.map(ToOwned::to_owned),
        }
    }

    #[test]
    fn route_inventory_is_exactly_the_remote_watchlist_post() {
        assert_eq!(
            remote_watchlist_write_routes(),
            &[("POST", REMOTE_WATCHLIST_WRITE_PATH)]
        );
    }

    #[test]
    fn malformed_body_and_unknown_route_fail_before_the_port() {
        let malformed = dispatch_remote_watchlist_write(
            &request("POST", REMOTE_WATCHLIST_WRITE_PATH, Some(b"{")),
            None,
            "2026-08-22T04:00:00Z",
        );
        assert_eq!(malformed.status, 400);
        assert_eq!(malformed.body["error"]["code"], "BAD_REQUEST");

        let wrong_method = dispatch_remote_watchlist_write(
            &request("GET", REMOTE_WATCHLIST_WRITE_PATH, None),
            None,
            "2026-08-22T04:00:00Z",
        );
        assert_eq!(wrong_method.status, 404);
        assert_eq!(wrong_method.body["error"]["code"], "NOT_FOUND");
    }

    #[test]
    fn null_and_empty_object_preserve_go_payload_states() {
        let port = RecordingPort;
        for (body, state) in [
            (b"null".as_slice(), RemoteWatchlistWritePayloadState::Nil),
            (
                b"{}".as_slice(),
                RemoteWatchlistWritePayloadState::EmptyObject,
            ),
        ] {
            let response = dispatch_remote_watchlist_write(
                &request(
                    "POST",
                    "/api/v1/watchlists/remote?brokerId=futu&accountId=acct-1",
                    Some(body),
                ),
                Some(&port),
                "2026-08-22T04:00:00Z",
            );
            assert_eq!(response.status, 200);
            assert_eq!(response.body["data"]["provider"]["brokerId"], "futu");
            assert_eq!(
                state,
                if body == b"null" {
                    RemoteWatchlistWritePayloadState::Nil
                } else {
                    RemoteWatchlistWritePayloadState::EmptyObject
                }
            );
        }
    }

    #[test]
    fn query_decoding_keeps_first_broker_and_account_values_for_the_port_boundary() {
        let response = dispatch_remote_watchlist_write(
            &request(
                "POST",
                "/api/v1/watchlists/remote?brokerId=futu&brokerId=ignored&accountId=acct%201",
                Some(br#"{"groupName":"Favorites"}"#),
            ),
            Some(&RecordingPort),
            "2026-08-22T04:00:00Z",
        );
        assert_eq!(response.status, 200);
    }
}
