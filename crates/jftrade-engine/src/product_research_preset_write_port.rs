use std::collections::BTreeMap;

use jftrade_research::normalize_definition_v2;
use jftrade_store_sqlite::{
    RESEARCH_PRESET_TEST_CUTOVER_PROFILE, ResearchPresetMutation, ResearchPresetStoreError,
    ResearchPresetTestCutoverStore,
};
use percent_encoding::percent_decode_str;
use serde_json::{Map, Value, json};
use thiserror::Error;
use time::{OffsetDateTime, format_description::well_known::Rfc3339};

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
/// Go continues to own revision fencing, SQLite, transactionality, and all
/// production registration. Test adapters return a complete Go-shaped data
/// projection and never receive a production database handle.
pub trait ResearchPresetWritePort: Send + Sync + std::fmt::Debug {
    fn mutate(
        &self,
        mutation: &ResearchPresetWriteMutation,
    ) -> Result<Value, ResearchPresetWritePortError>;
}

/// Durable SQLite adapter used only when an explicit test-cutover profile is
/// supplied. Production composition never constructs this port; Go remains
/// the sole research-preset owner until formal cutover.
#[allow(dead_code)]
pub struct ResearchPresetSqliteTestCutoverPort {
    store: ResearchPresetTestCutoverStore,
}

#[allow(dead_code)]
impl std::fmt::Debug for ResearchPresetSqliteTestCutoverPort {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("ResearchPresetSqliteTestCutoverPort")
            .field("path", &self.store.path())
            .finish()
    }
}

#[allow(dead_code)]
impl ResearchPresetSqliteTestCutoverPort {
    pub fn open(path: impl AsRef<std::path::Path>) -> Result<Self, ResearchPresetStoreError> {
        Ok(Self {
            store: ResearchPresetTestCutoverStore::open_existing(
                path,
                RESEARCH_PRESET_TEST_CUTOVER_PROFILE,
            )?,
        })
    }
}

#[allow(dead_code)]
impl ResearchPresetWritePort for ResearchPresetSqliteTestCutoverPort {
    fn mutate(
        &self,
        mutation: &ResearchPresetWriteMutation,
    ) -> Result<Value, ResearchPresetWritePortError> {
        match mutation {
            ResearchPresetWriteMutation::Create { payload } => self.create(payload),
            ResearchPresetWriteMutation::Update { preset_id, payload } => {
                self.update(preset_id, payload)
            }
            ResearchPresetWriteMutation::Delete { preset_id } => self.delete(preset_id),
        }
    }
}

#[allow(dead_code)]
impl ResearchPresetSqliteTestCutoverPort {
    fn create(&self, payload: &Value) -> Result<Value, ResearchPresetWritePortError> {
        let object = payload
            .as_object()
            .ok_or_else(|| invalid_preset("name is required"))?;
        let name = normalized_name(object.get("name"))?;
        let definition = normalized_definition(object.get("definition"))?;
        let timestamp = current_timestamp();
        for attempt in 0..4 {
            let preset = ResearchPresetMutation {
                preset_id: generated_preset_id(attempt),
                name: name.clone(),
                query_schema_version: 2,
                definition: definition.clone(),
                revision: 1,
            };
            match self.store.insert(&preset, &timestamp) {
                Ok(stored) => return serde_json::to_value(stored).map_err(failed_encoding),
                Err(ResearchPresetStoreError::Conflict) if attempt < 3 => continue,
                Err(error) => return Err(map_store_error(error)),
            }
        }
        Err(ResearchPresetWritePortError::Failed(
            "could not allocate research preset id".to_owned(),
        ))
    }

    fn update(
        &self,
        preset_id: &str,
        payload: &Value,
    ) -> Result<Value, ResearchPresetWritePortError> {
        let object = payload
            .as_object()
            .ok_or_else(|| invalid_preset("expectedRevision must be positive"))?;
        let expected_revision = object
            .get("expectedRevision")
            .and_then(Value::as_u64)
            .filter(|revision| *revision > 0)
            .ok_or_else(|| invalid_preset("expectedRevision must be positive"))?;
        let has_name = object.get("name").is_some_and(|value| !value.is_null());
        let has_definition = object
            .get("definition")
            .is_some_and(|value| !value.is_null());
        if !has_name && !has_definition {
            return Err(invalid_preset("name or definition is required"));
        }
        let current = self.store.get(preset_id).map_err(map_store_error)?;
        let name = if has_name {
            normalized_name(object.get("name"))?
        } else {
            current.preset.name.clone()
        };
        let definition = if has_definition {
            normalized_definition(object.get("definition"))?
        } else {
            current.preset.definition.clone()
        };
        let next_revision = expected_revision
            .checked_add(1)
            .ok_or_else(|| invalid_preset("expectedRevision exceeds supported range"))?;
        let preset = ResearchPresetMutation {
            preset_id: current.preset.preset_id,
            name,
            query_schema_version: 2,
            definition,
            revision: next_revision,
        };
        let stored = self
            .store
            .replace_revision(&preset, expected_revision, &current_timestamp())
            .map_err(map_store_error)?;
        serde_json::to_value(stored).map_err(failed_encoding)
    }

    fn delete(&self, preset_id: &str) -> Result<Value, ResearchPresetWritePortError> {
        self.store.delete(preset_id).map_err(map_store_error)?;
        Ok(json!({"deleted": true}))
    }
}

#[allow(dead_code)]
fn normalized_name(value: Option<&Value>) -> Result<String, ResearchPresetWritePortError> {
    let name = value
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|name| !name.is_empty())
        .ok_or_else(|| invalid_preset("name is required"))?;
    if name.chars().count() > 80 {
        return Err(invalid_preset("name must not exceed 80 characters"));
    }
    Ok(name.to_owned())
}

#[allow(dead_code)]
fn normalized_definition(value: Option<&Value>) -> Result<Value, ResearchPresetWritePortError> {
    let value = value
        .cloned()
        .ok_or_else(|| invalid_preset("definition is required"))?;
    normalize_definition_v2(value).map_err(|error| invalid_preset(error.to_string()))
}

#[allow(dead_code)]
fn invalid_preset(message: impl Into<String>) -> ResearchPresetWritePortError {
    ResearchPresetWritePortError::Invalid(format!(
        "invalid research screen preset: {}",
        message.into()
    ))
}

#[allow(dead_code)]
fn failed_encoding(error: serde_json::Error) -> ResearchPresetWritePortError {
    ResearchPresetWritePortError::Failed(format!("encode research screen preset: {error}"))
}

#[allow(dead_code)]
fn map_store_error(error: ResearchPresetStoreError) -> ResearchPresetWritePortError {
    match error {
        ResearchPresetStoreError::NotFound => {
            ResearchPresetWritePortError::NotFound("research screen preset not found".to_owned())
        }
        ResearchPresetStoreError::Conflict => {
            ResearchPresetWritePortError::Conflict("research screen preset conflict".to_owned())
        }
        ResearchPresetStoreError::Incompatible(message) => invalid_preset(message),
        ResearchPresetStoreError::UnsupportedProfile(_message) => {
            ResearchPresetWritePortError::Unavailable
        }
        ResearchPresetStoreError::NotRegularFile(_)
        | ResearchPresetStoreError::EmptyPath
        | ResearchPresetStoreError::WriterLease(_)
        | ResearchPresetStoreError::Open(_)
        | ResearchPresetStoreError::Configure(_)
        | ResearchPresetStoreError::Schema(_)
        | ResearchPresetStoreError::LockUnavailable
        | ResearchPresetStoreError::Query(_) => ResearchPresetWritePortError::Unavailable,
    }
}

#[allow(dead_code)]
fn current_timestamp() -> String {
    OffsetDateTime::now_utc()
        .format(&Rfc3339)
        .unwrap_or_else(|_| "1970-01-01T00:00:00Z".to_owned())
}

#[allow(dead_code)]
fn generated_preset_id(attempt: u8) -> String {
    let mut bytes = [0_u8; 12];
    if getrandom::fill(&mut bytes).is_err() {
        bytes[0] = attempt;
    }
    let suffix = bytes
        .iter()
        .map(|byte| format!("{byte:02x}"))
        .collect::<String>();
    format!("rsp_{suffix}")
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
