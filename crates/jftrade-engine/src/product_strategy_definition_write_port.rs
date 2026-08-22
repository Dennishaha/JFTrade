//! Stage 9 test-cutover leaf for strategy-definition mutation routes.
//!
//! The Go strategy definition store and catalog remain the only production
//! owners. This module deliberately has no SQLite, PineTS, runtime, or
//! provider dependency. The integration branch may inject a consumer-owned
//! mutation port from an explicit test-cutover profile later.

use std::collections::BTreeMap;

use percent_encoding::percent_decode_str;
use serde_json::{Map, Value, json};

use jftrade_api::ApiRequest;

pub const STRATEGY_DEFINITION_CREATE_PATH: &str = "/api/v1/strategy-definitions";
pub const STRATEGY_DEFINITION_UPDATE_PATH: &str = "/api/v1/strategy-definitions/{definitionId}";
pub const STRATEGY_DEFINITION_DELETE_PATH: &str = "/api/v1/strategy-definitions/{definitionId}";
pub const STRATEGY_DEFINITION_APPLY_LINKED_PATH: &str =
    "/api/v1/strategy-definitions/{definitionId}/apply-linked-instances";
pub const STRATEGY_DEFINITION_INSTANTIATE_PATH: &str =
    "/api/v1/strategy-definitions/{definitionId}/instantiate";

pub const STRATEGY_DEFINITION_WRITE_ROUTES: [(&str, &str); 5] = [
    ("POST", STRATEGY_DEFINITION_CREATE_PATH),
    ("PUT", STRATEGY_DEFINITION_UPDATE_PATH),
    ("DELETE", STRATEGY_DEFINITION_DELETE_PATH),
    ("POST", STRATEGY_DEFINITION_APPLY_LINKED_PATH),
    ("POST", STRATEGY_DEFINITION_INSTANTIATE_PATH),
];

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum StrategyDefinitionWriteOperation {
    Create,
    Update,
    Delete,
    ApplyLinkedInstances,
    Instantiate,
}

impl StrategyDefinitionWriteOperation {
    #[allow(dead_code)]
    pub const fn name(self) -> &'static str {
        match self {
            Self::Create => "create",
            Self::Update => "update",
            Self::Delete => "delete",
            Self::ApplyLinkedInstances => "apply-linked-instances",
            Self::Instantiate => "instantiate",
        }
    }
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct StrategyDefinitionWriteInput {
    pub operation: StrategyDefinitionWriteOperation,
    pub definition_id: Option<String>,
    pub definition: Option<Value>,
    pub binding: Option<Value>,
    pub binding_error: Option<String>,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct StrategyDefinitionWriteResponse {
    pub status: u16,
    pub headers: BTreeMap<String, String>,
    pub body: Value,
}

#[allow(dead_code)]
#[derive(Clone, Debug, Eq, PartialEq)]
pub enum StrategyDefinitionWritePortError {
    Unavailable(String),
    Failed {
        status: u16,
        code: String,
        message: String,
    },
}

/// Consumer-owned mutation boundary for all five strategy-definition writes.
///
/// The implementation owns definition normalization, versioning, soft-delete
/// guards, linked-instance lifecycle, persistence and rollback. The leaf only
/// binds the HTTP-shaped input and maps the injected result back to the Go
/// envelope.
pub trait StrategyDefinitionWritePort: Send + Sync + std::fmt::Debug {
    fn mutate(
        &self,
        input: &StrategyDefinitionWriteInput,
    ) -> Result<Value, StrategyDefinitionWritePortError>;
}

pub fn strategy_definition_write_routes() -> &'static [(&'static str, &'static str); 5] {
    &STRATEGY_DEFINITION_WRITE_ROUTES
}

pub fn dispatch_strategy_definition_write(
    request: &ApiRequest,
    port: Option<&dyn StrategyDefinitionWritePort>,
    timestamp: &str,
) -> StrategyDefinitionWriteResponse {
    let (operation, definition_id) = match parse_route(&request.method, &request.path) {
        Ok(route) => route,
        Err(spec) => return error_response(spec, timestamp),
    };
    let input = match parse_input(operation, definition_id, &request.body) {
        Ok(input) => input,
        Err(response) => return error_response(response, timestamp),
    };
    let Some(port) = port else {
        return error_response(
            ErrorSpec {
                status: 503,
                code: "STRATEGY_DEFINITIONS_UNAVAILABLE".to_owned(),
                message: "strategy definition write port is unavailable".to_owned(),
            },
            timestamp,
        );
    };
    match port.mutate(&input) {
        Ok(data) => success_response(data, timestamp),
        Err(StrategyDefinitionWritePortError::Unavailable(message)) => error_response(
            ErrorSpec {
                status: 503,
                code: "STRATEGY_DEFINITIONS_UNAVAILABLE".to_owned(),
                message,
            },
            timestamp,
        ),
        Err(StrategyDefinitionWritePortError::Failed {
            status,
            code,
            message,
        }) => error_response(
            ErrorSpec {
                status,
                code,
                message,
            },
            timestamp,
        ),
    }
}

struct ErrorSpec {
    status: u16,
    code: String,
    message: String,
}

fn parse_route(
    method: &str,
    path: &str,
) -> Result<(StrategyDefinitionWriteOperation, Option<String>), ErrorSpec> {
    if method == "POST" && path == STRATEGY_DEFINITION_CREATE_PATH {
        return Ok((StrategyDefinitionWriteOperation::Create, None));
    }
    for (suffix, operation) in [
        (
            "/apply-linked-instances",
            StrategyDefinitionWriteOperation::ApplyLinkedInstances,
        ),
        (
            "/instantiate",
            StrategyDefinitionWriteOperation::Instantiate,
        ),
    ] {
        if let Some(raw_id) = path
            .strip_prefix("/api/v1/strategy-definitions/")
            .and_then(|value| value.strip_suffix(suffix))
        {
            if method != "POST" {
                return Err(not_found_spec(path));
            }
            if raw_id.is_empty() || raw_id.contains('/') {
                return Err(not_found_spec(path));
            }
            return parse_definition_id(raw_id)
                .map(|id| (operation, Some(id)))
                .map_err(|_| bad_request_spec("invalid definition id"));
        }
    }
    if let Some(raw_id) = path.strip_prefix("/api/v1/strategy-definitions/") {
        if raw_id.contains('/') {
            return Err(not_found_spec(path));
        }
        if raw_id.is_empty() {
            return Err(bad_request_spec("invalid definition id"));
        }
        let operation = match method {
            "PUT" => StrategyDefinitionWriteOperation::Update,
            "DELETE" => StrategyDefinitionWriteOperation::Delete,
            _ => return Err(not_found_spec(path)),
        };
        return parse_definition_id(raw_id)
            .map(|id| (operation, Some(id)))
            .map_err(|_| bad_request_spec("invalid definition id"));
    }
    Err(not_found_spec(path))
}

fn parse_definition_id(raw_id: &str) -> Result<String, ()> {
    if raw_id.is_empty() || raw_id.contains('/') {
        return Err(());
    }
    let decoded = percent_decode_str(raw_id).decode_utf8().map_err(|_| ())?;
    if decoded.is_empty() || decoded.contains('/') {
        return Err(());
    }
    Ok(decoded.into_owned())
}

fn parse_input(
    operation: StrategyDefinitionWriteOperation,
    definition_id: Option<String>,
    body: &[u8],
) -> Result<StrategyDefinitionWriteInput, ErrorSpec> {
    match operation {
        StrategyDefinitionWriteOperation::Create => {
            let mut definition = parse_definition_body(body, true)?;
            if let Value::Object(object) = &mut definition {
                object.insert("id".to_owned(), Value::String(String::new()));
            }
            Ok(StrategyDefinitionWriteInput {
                operation,
                definition_id: None,
                definition: Some(definition),
                binding: None,
                binding_error: None,
            })
        }
        StrategyDefinitionWriteOperation::Update => {
            let definition_id = definition_id.ok_or_else(|| ErrorSpec {
                status: 400,
                code: "BAD_REQUEST".to_owned(),
                message: "invalid definition id".to_owned(),
            })?;
            let mut definition = parse_definition_body(body, true)?;
            if let Value::Object(object) = &mut definition {
                object.insert("id".to_owned(), Value::String(definition_id.clone()));
            }
            Ok(StrategyDefinitionWriteInput {
                operation,
                definition_id: Some(definition_id),
                definition: Some(definition),
                binding: None,
                binding_error: None,
            })
        }
        StrategyDefinitionWriteOperation::Delete
        | StrategyDefinitionWriteOperation::ApplyLinkedInstances => {
            Ok(StrategyDefinitionWriteInput {
                operation,
                definition_id,
                definition: None,
                binding: None,
                binding_error: None,
            })
        }
        StrategyDefinitionWriteOperation::Instantiate => {
            let (binding, binding_error) = parse_binding_body(body);
            Ok(StrategyDefinitionWriteInput {
                operation,
                definition_id,
                definition: None,
                binding,
                binding_error,
            })
        }
    }
}

fn parse_definition_body(body: &[u8], required: bool) -> Result<Value, ErrorSpec> {
    if body.is_empty() {
        return if required {
            Err(bad_request_spec("invalid definition payload"))
        } else {
            Ok(Value::Object(Map::new()))
        };
    }
    parse_object(body, "invalid definition payload")
}

fn parse_binding_body(body: &[u8]) -> (Option<Value>, Option<String>) {
    if body.is_empty() {
        return (Some(Value::Object(Map::new())), None);
    }
    match parse_object(body, "invalid strategy instance payload") {
        Ok(value) => (Some(value), None),
        Err(error) => (None, Some(error.message)),
    }
}

fn parse_object(body: &[u8], message: &'static str) -> Result<Value, ErrorSpec> {
    let value: Value = serde_json::from_slice(body).map_err(|_| bad_request_spec(message))?;
    match value {
        Value::Null => Ok(Value::Object(Map::new())),
        Value::Object(object) => Ok(Value::Object(object)),
        _ => Err(bad_request_spec(message)),
    }
}

fn success_response(data: Value, timestamp: &str) -> StrategyDefinitionWriteResponse {
    StrategyDefinitionWriteResponse {
        status: 200,
        headers: json_headers(),
        body: json!({"ok": true, "data": data, "timestamp": timestamp}),
    }
}

fn error_response(spec: ErrorSpec, timestamp: &str) -> StrategyDefinitionWriteResponse {
    StrategyDefinitionWriteResponse {
        status: spec.status,
        headers: json_headers(),
        body: json!({
            "ok": false,
            "error": {"code": spec.code, "message": spec.message},
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

fn bad_request_spec(message: &'static str) -> ErrorSpec {
    ErrorSpec {
        status: 400,
        code: "BAD_REQUEST".to_owned(),
        message: message.to_owned(),
    }
}

fn not_found_spec(path: &str) -> ErrorSpec {
    ErrorSpec {
        status: 404,
        code: "NOT_FOUND".to_owned(),
        message: format!("unknown endpoint {path}"),
    }
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use super::*;

    #[derive(Debug)]
    struct RecordingPort;

    impl StrategyDefinitionWritePort for RecordingPort {
        fn mutate(
            &self,
            input: &StrategyDefinitionWriteInput,
        ) -> Result<Value, StrategyDefinitionWritePortError> {
            if let Some(message) = input.binding_error.as_deref() {
                return Err(StrategyDefinitionWritePortError::Failed {
                    status: 400,
                    code: "BAD_REQUEST".to_owned(),
                    message: message.to_owned(),
                });
            }
            Ok(json!({
                "operation": input.operation.name(),
                "definitionId": input.definition_id,
                "definition": input.definition,
                "binding": input.binding,
                "bindingError": input.binding_error,
            }))
        }
    }

    fn request(method: &str, path: &str, body: &[u8]) -> ApiRequest {
        ApiRequest {
            method: method.to_owned(),
            path: path.to_owned(),
            query: String::new(),
            body: body.to_vec(),
            request_id: "strategy-definition-write-test".to_owned(),
            desktop_trusted: true,
            origin_provided: false,
            origin_allowed: true,
            browser_authenticated: true,
        }
    }

    #[test]
    fn route_contract_has_exactly_five_mutations() {
        assert_eq!(strategy_definition_write_routes().len(), 5);
        assert_eq!(
            strategy_definition_write_routes()
                .iter()
                .filter(|(method, _)| *method == "POST")
                .count(),
            3
        );
        assert_eq!(
            strategy_definition_write_routes()
                .iter()
                .filter(|(method, _)| *method == "PUT")
                .count(),
            1
        );
        assert_eq!(
            strategy_definition_write_routes()
                .iter()
                .filter(|(method, _)| *method == "DELETE")
                .count(),
            1
        );
    }

    #[test]
    fn create_clears_client_id_and_update_overrides_body_id() {
        let port = Arc::new(RecordingPort);
        let create = dispatch_strategy_definition_write(
            &request(
                "POST",
                STRATEGY_DEFINITION_CREATE_PATH,
                br#"{"id":"client-id","name":"Draft"}"#,
            ),
            Some(port.as_ref()),
            "2026-08-22T00:00:00Z",
        );
        assert_eq!(create.status, 200);
        assert_eq!(create.body["data"]["definition"]["id"], "");

        let update = dispatch_strategy_definition_write(
            &request(
                "PUT",
                "/api/v1/strategy-definitions/definition-1",
                br#"{"id":"body-id","name":"Updated"}"#,
            ),
            Some(port.as_ref()),
            "2026-08-22T00:00:00Z",
        );
        assert_eq!(update.status, 200);
        assert_eq!(update.body["data"]["definition"]["id"], "definition-1");
    }

    #[test]
    fn instantiate_accepts_empty_body_but_rejects_malformed_json() {
        let port = Arc::new(RecordingPort);
        let empty = dispatch_strategy_definition_write(
            &request(
                "POST",
                "/api/v1/strategy-definitions/definition-1/instantiate",
                &[],
            ),
            Some(port.as_ref()),
            "2026-08-22T00:00:00Z",
        );
        assert_eq!(empty.status, 200);
        assert_eq!(empty.body["data"]["binding"], json!({}));

        let malformed = dispatch_strategy_definition_write(
            &request(
                "POST",
                "/api/v1/strategy-definitions/definition-1/instantiate",
                b"{",
            ),
            Some(port.as_ref()),
            "2026-08-22T00:00:00Z",
        );
        assert_eq!(malformed.status, 400);
        assert_eq!(malformed.body["error"]["code"], "BAD_REQUEST");
        assert_eq!(
            malformed.body["error"]["message"],
            "invalid strategy instance payload"
        );
    }

    #[test]
    fn malformed_input_precedes_missing_port_and_unknown_routes_fail_closed() {
        let malformed = dispatch_strategy_definition_write(
            &request("POST", STRATEGY_DEFINITION_CREATE_PATH, b"{"),
            None,
            "2026-08-22T00:00:00Z",
        );
        assert_eq!(malformed.status, 400);
        assert_eq!(malformed.body["error"]["code"], "BAD_REQUEST");

        let unknown = dispatch_strategy_definition_write(
            &request(
                "POST",
                "/api/v1/strategy-definitions/definition-1/unknown",
                b"{}",
            ),
            None,
            "2026-08-22T00:00:00Z",
        );
        assert_eq!(unknown.status, 404);
        assert_eq!(unknown.body["error"]["code"], "NOT_FOUND");
    }
}
