use std::collections::BTreeMap;

use percent_encoding::percent_decode_str;
use serde_json::{Map, Value, json};
use thiserror::Error;

pub const CREATE_PRESET_PATH: &str = "/api/v1/research/screens/presets";
pub const PRESET_BY_ID_PATH: &str = "/api/v1/research/screens/presets/{presetId}";

pub const RESEARCH_PRESET_WRITE_ROUTES: [(&str, &str); 3] = [
    ("POST", CREATE_PRESET_PATH),
    ("PATCH", PRESET_BY_ID_PATH),
    ("DELETE", PRESET_BY_ID_PATH),
];

#[allow(dead_code)]
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum ResearchPresetWriteOperation {
    Create,
    Update,
    Delete,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub enum ResearchPresetWriteMutation {
    Create { payload: Value },
    Update { preset_id: String, payload: Value },
    Delete { preset_id: String },
}

#[allow(dead_code)]
impl ResearchPresetWriteMutation {
    pub fn operation(&self) -> ResearchPresetWriteOperation {
        match self {
            Self::Create { .. } => ResearchPresetWriteOperation::Create,
            Self::Update { .. } => ResearchPresetWriteOperation::Update,
            Self::Delete { .. } => ResearchPresetWriteOperation::Delete,
        }
    }
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ResearchPresetWriteRequest {
    pub method: String,
    pub path: String,
    pub body: Option<Vec<u8>>,
}

#[allow(dead_code)]
#[derive(Clone, Debug, Eq, PartialEq, Error)]
pub enum ResearchPresetWritePortError {
    #[error("research preset store is unavailable")]
    Unavailable,
    #[error("{0}")]
    NotFound(String),
    #[error("{0}")]
    Conflict(String),
    #[error("{0}")]
    Invalid(String),
    #[error("{0}")]
    Failed(String),
}

/// Consumer-owned mutation boundary for the three research preset routes.
///
/// Go continues to own definition normalization, revision fencing, SQLite,
/// transactionality, and all production registration. Test adapters return a
/// complete Go-shaped data projection and never receive a database handle.
pub trait ResearchPresetWritePort: Send + Sync + std::fmt::Debug {
    fn mutate(
        &self,
        mutation: &ResearchPresetWriteMutation,
    ) -> Result<Value, ResearchPresetWritePortError>;
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ResearchPresetWriteResponse {
    pub status: u16,
    pub headers: BTreeMap<String, String>,
    pub body: Value,
}

pub fn research_preset_write_routes() -> &'static [(&'static str, &'static str); 3] {
    &RESEARCH_PRESET_WRITE_ROUTES
}

pub fn dispatch_research_preset_write(
    request: &ResearchPresetWriteRequest,
    port: Option<&dyn ResearchPresetWritePort>,
    timestamp: &str,
) -> ResearchPresetWriteResponse {
    let (path, _) = split_path_query(&request.path);
    let mutation = match parse_mutation(&request.method, path, request.body.as_deref()) {
        Ok(mutation) => mutation,
        Err(ParseMutationError::NotFound) => {
            return error_response(404, "NOT_FOUND", "resource not found", timestamp);
        }
        Err(ParseMutationError::Invalid(message)) => {
            return error_response(400, "RESEARCH_PRESET_INVALID", &message, timestamp);
        }
    };
    let Some(port) = port else {
        return error_response(
            503,
            "RESEARCH_PRESET_UNAVAILABLE",
            "research preset store is unavailable",
            timestamp,
        );
    };
    match port.mutate(&mutation) {
        Ok(data) => success_response(data, timestamp),
        Err(error) => port_error_response(error, timestamp),
    }
}

#[derive(Clone, Debug, Eq, PartialEq)]
enum ParseMutationError {
    NotFound,
    Invalid(String),
}

fn parse_mutation(
    method: &str,
    path: &str,
    body: Option<&[u8]>,
) -> Result<ResearchPresetWriteMutation, ParseMutationError> {
    if method == "POST" && path == CREATE_PRESET_PATH {
        return parse_payload(body, ResearchPresetWriteOperation::Create)
            .map(|payload| ResearchPresetWriteMutation::Create { payload });
    }
    if method != "PATCH" && method != "DELETE" {
        return Err(ParseMutationError::NotFound);
    }
    let Some(raw_id) = path.strip_prefix("/api/v1/research/screens/presets/") else {
        return Err(ParseMutationError::NotFound);
    };
    if raw_id.is_empty() || raw_id.contains('/') {
        return Err(ParseMutationError::NotFound);
    }
    let preset_id = percent_decode_str(raw_id)
        .decode_utf8()
        .map_err(|_| ParseMutationError::Invalid("invalid research screen preset id".to_owned()))?;
    let preset_id = preset_id.trim();
    if method == "DELETE" {
        return Ok(ResearchPresetWriteMutation::Delete {
            preset_id: preset_id.to_owned(),
        });
    }
    parse_payload(body, ResearchPresetWriteOperation::Update).map(|payload| {
        ResearchPresetWriteMutation::Update {
            preset_id: preset_id.to_owned(),
            payload,
        }
    })
}

fn parse_payload(
    body: Option<&[u8]>,
    operation: ResearchPresetWriteOperation,
) -> Result<Value, ParseMutationError> {
    let Some(body) = body else {
        return Err(invalid_payload());
    };
    let payload: Value = serde_json::from_slice(body).map_err(|_| invalid_payload())?;
    if let Value::Object(object) = &payload {
        let allowed = match operation {
            ResearchPresetWriteOperation::Create => ["name", "definition"].as_slice(),
            ResearchPresetWriteOperation::Update => {
                ["name", "definition", "expectedRevision"].as_slice()
            }
            ResearchPresetWriteOperation::Delete => &[] as &[&str],
        };
        if object.keys().any(|key| !allowed.contains(&key.as_str())) {
            return Err(invalid_payload());
        }
        if !payload_fields_have_wire_types(object, operation) {
            return Err(invalid_payload());
        }
    } else if !payload.is_null() {
        return Err(invalid_payload());
    }
    Ok(payload)
}

fn payload_fields_have_wire_types(
    object: &Map<String, Value>,
    operation: ResearchPresetWriteOperation,
) -> bool {
    let is_string_or_null = |key: &str| {
        object
            .get(key)
            .is_none_or(|value| value.is_string() || value.is_null())
    };
    let is_object_or_null = |key: &str| {
        object
            .get(key)
            .is_none_or(|value| value.is_object() || value.is_null())
    };
    match operation {
        ResearchPresetWriteOperation::Create => {
            is_string_or_null("name") && is_object_or_null("definition")
        }
        ResearchPresetWriteOperation::Update => {
            is_string_or_null("name")
                && is_object_or_null("definition")
                && object
                    .get("expectedRevision")
                    .is_none_or(|value| value.is_null() || value.as_i64().is_some())
        }
        ResearchPresetWriteOperation::Delete => true,
    }
}

fn invalid_payload() -> ParseMutationError {
    ParseMutationError::Invalid("invalid research screen preset payload".to_owned())
}

fn split_path_query(path: &str) -> (&str, &str) {
    path.split_once('?').unwrap_or((path, ""))
}

fn port_error_response(
    error: ResearchPresetWritePortError,
    timestamp: &str,
) -> ResearchPresetWriteResponse {
    match error {
        ResearchPresetWritePortError::Unavailable => error_response(
            503,
            "RESEARCH_PRESET_UNAVAILABLE",
            "research preset store is unavailable",
            timestamp,
        ),
        ResearchPresetWritePortError::NotFound(message) => {
            error_response(404, "RESEARCH_PRESET_NOT_FOUND", &message, timestamp)
        }
        ResearchPresetWritePortError::Conflict(message) => {
            error_response(409, "RESEARCH_PRESET_CONFLICT", &message, timestamp)
        }
        ResearchPresetWritePortError::Invalid(message) => {
            error_response(400, "RESEARCH_PRESET_INVALID", &message, timestamp)
        }
        ResearchPresetWritePortError::Failed(message) => {
            error_response(500, "RESEARCH_PRESET_FAILED", &message, timestamp)
        }
    }
}

fn success_response(data: Value, timestamp: &str) -> ResearchPresetWriteResponse {
    ResearchPresetWriteResponse {
        status: 200,
        headers: json_headers(),
        body: json!({"ok": true, "data": data, "timestamp": timestamp}),
    }
}

fn error_response(
    status: u16,
    code: &str,
    message: &str,
    timestamp: &str,
) -> ResearchPresetWriteResponse {
    ResearchPresetWriteResponse {
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

    #[test]
    fn route_inventory_is_exactly_the_three_preset_mutations() {
        assert_eq!(research_preset_write_routes().len(), 3);
        assert_eq!(
            research_preset_write_routes()
                .iter()
                .filter(|(method, _)| *method == "POST")
                .count(),
            1
        );
        assert_eq!(
            research_preset_write_routes()
                .iter()
                .filter(|(method, _)| *method == "PATCH")
                .count(),
            1
        );
        assert_eq!(
            research_preset_write_routes()
                .iter()
                .filter(|(method, _)| *method == "DELETE")
                .count(),
            1
        );
    }

    #[test]
    fn exact_route_and_invalid_payload_precedence_match_go_boundary() {
        let request = ResearchPresetWriteRequest {
            method: "POST".to_owned(),
            path: CREATE_PRESET_PATH.to_owned(),
            body: Some(br#"{"name":"Value","unknown":true}"#.to_vec()),
        };
        let response = dispatch_research_preset_write(&request, None, "2026-08-22T04:00:00Z");
        assert_eq!(response.status, 400);
        assert_eq!(response.body["error"]["code"], "RESEARCH_PRESET_INVALID");

        let wrong_method = ResearchPresetWriteRequest {
            method: "GET".to_owned(),
            path: CREATE_PRESET_PATH.to_owned(),
            body: None,
        };
        assert_eq!(
            dispatch_research_preset_write(&wrong_method, None, "2026-08-22T04:00:00Z").status,
            404
        );
        let extra_segment = ResearchPresetWriteRequest {
            method: "DELETE".to_owned(),
            path: "/api/v1/research/screens/presets/value/extra".to_owned(),
            body: None,
        };
        assert_eq!(
            dispatch_research_preset_write(&extra_segment, None, "2026-08-22T04:00:00Z").status,
            404
        );
    }
}
