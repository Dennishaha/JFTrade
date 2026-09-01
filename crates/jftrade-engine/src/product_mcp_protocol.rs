//! Pure Streamable HTTP and JSON-RPC compatibility checks for the local MCP
//! listener.  Keeping these checks independent from the listener makes the
//! wire contract reviewable without starting a Tokio runtime.

use super::product_production_ports::{ProductionPortBundle, ProductionToolCatalog};
#[path = "product_mcp_protocol_readiness.rs"]
mod readiness;
#[path = "product_mcp_schema_catalog.rs"]
mod schema_catalog;
use axum::http::{HeaderMap, header};
#[cfg(test)]
pub(crate) use readiness::mcp_tool_adapter;
pub(crate) use readiness::mcp_tool_availability;
use serde_json::{Value, json};

pub(crate) const DEFAULT_PROTOCOL_VERSION: &str = "2025-03-26";
pub(crate) const MODERN_PROTOCOL_VERSION: &str = "2026-07-28";
pub(crate) const META_PROTOCOL_VERSION_KEY: &str = "io.modelcontextprotocol/protocolVersion";
pub(crate) const CODE_HEADER_MISMATCH: i64 = -32020;
/// The reviewed MCP catalog. Rust exposes the same 69 names in `tools/list`
/// and dispatches every name through a production executor. Runtime readiness
/// still reports provider/store outages as `unavailable` instead of turning an
/// external dependency failure into a fabricated success response.
pub(crate) const REVIEWED_READ_ONLY_TOOLS: &[&str] = &[
    "account.orders",
    "alerts.option_event.list",
    "alerts.price.list",
    "backtest.kline_sync_status",
    "backtest.result_view",
    "backtest.runs",
    "broker.cash_flows",
    "broker.fees",
    "broker.fills",
    "broker.margin_ratios",
    "broker.orders",
    "derivatives.futures",
    "derivatives.option_analysis",
    "derivatives.option_chain",
    "derivatives.option_events",
    "derivatives.option_screen",
    "derivatives.warrants",
    "execution.buying_power",
    "execution.order_events",
    "market.broker_queue",
    "market.candles",
    "market.capabilities",
    "market.capital_flow",
    "market.depth",
    "market.instrument_profile",
    "market.intraday",
    "market.providers",
    "market.search",
    "market.snapshot",
    "market.snapshots",
    "market.subscriptions",
    "market.ticks",
    "plugins.catalog",
    "portfolio.summary",
    "prediction.combo_eligible",
    "prediction.combo_quote",
    "prediction.depth",
    "prediction.discover",
    "prediction.history",
    "prediction.snapshot",
    "research.analyst",
    "research.calendar",
    "research.corporate_actions",
    "research.financials",
    "research.industry",
    "research.institutions",
    "research.instrument",
    "research.macro",
    "research.news",
    "research.ownership",
    "research.rankings",
    "research.screen",
    "research.screen_catalog",
    "research.short_interest",
    "research.technical_indicators",
    "research.valuation",
    "risk.events",
    "risk.state",
    "strategy.definition_versions.get",
    "strategy.definition_versions.list",
    "strategy.definitions",
    "strategy.instance_activity",
    "strategy.pine_spec",
    "strategy.validate_pine",
    "system.futu_opend",
    "system.runtime_dependencies",
    "system.status",
    "watchlist.list",
    "watchlist.remote.list",
];

/// MCP tools with a native Rust production executor. The reviewed Go catalog
/// remains wire-compatible at 69 names, but only this explicit set may ever
/// be projected as `ready` or dispatched by the Rust listener. Research
/// capabilities without a concrete Rust port remain fail-closed until their
/// production adapters are installed. Pine specification and validation are
/// native Rust leaves and do not require a production port bundle.
pub(crate) const PRODUCTION_MCP_EXECUTABLE_TOOLS: &[&str] = &[
    "system.status",
    "system.futu_opend",
    "system.runtime_dependencies",
    "plugins.catalog",
    "market.providers",
    "market.capabilities",
    "market.search",
    "market.instrument_profile",
    "market.snapshot",
    "market.candles",
    "market.intraday",
    "market.ticks",
    "market.depth",
    "market.broker_queue",
    "market.capital_flow",
    "market.snapshots",
    "market.subscriptions",
    "derivatives.option_chain",
    "derivatives.option_analysis",
    "derivatives.option_events",
    "derivatives.option_screen",
    "derivatives.warrants",
    "derivatives.futures",
    "broker.cash_flows",
    "broker.fees",
    "broker.margin_ratios",
    "execution.order_events",
    "execution.buying_power",
    "watchlist.list",
    "watchlist.remote.list",
    "portfolio.summary",
    "account.orders",
    "broker.orders",
    "broker.fills",
    "strategy.definitions",
    "strategy.definition_versions.list",
    "strategy.definition_versions.get",
    "strategy.instance_activity",
    "strategy.pine_spec",
    "strategy.validate_pine",
    "backtest.runs",
    "backtest.kline_sync_status",
    "backtest.result_view",
    "risk.state",
    "risk.events",
    "prediction.discover",
    "prediction.snapshot",
    "prediction.depth",
    "prediction.history",
    "prediction.combo_eligible",
    "prediction.combo_quote",
    "alerts.price.list",
    "alerts.option_event.list",
    "research.instrument",
    "research.institutions",
    "research.financials",
    "research.analyst",
    "research.ownership",
    "research.corporate_actions",
    "research.valuation",
    "research.news",
    "research.short_interest",
    "research.technical_indicators",
    "research.screen",
    "research.screen_catalog",
    "research.calendar",
    "research.macro",
    "research.rankings",
    "research.industry",
];
pub(crate) const SUPPORTED_PROTOCOL_VERSIONS: &[&str] = &[
    "2026-07-28",
    "2025-11-25",
    "2025-06-18",
    "2025-03-26",
    "2024-11-05",
];

#[derive(Clone, Debug, Eq, PartialEq)]
pub(crate) struct ProtocolError {
    pub(crate) status: u16,
    pub(crate) message: &'static str,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub(crate) struct StandardHeaderError {
    pub(crate) code: i64,
    pub(crate) message: String,
}

impl StandardHeaderError {
    fn mismatch(message: impl Into<String>) -> Self {
        Self {
            code: CODE_HEADER_MISMATCH,
            message: message.into(),
        }
    }

    fn invalid_params(message: impl Into<String>) -> Self {
        Self {
            code: -32602,
            message: message.into(),
        }
    }
}

impl ProtocolError {
    const fn new(status: u16, message: &'static str) -> Self {
        Self { status, message }
    }
}

pub(crate) fn optional_string(arguments: &Value, key: &str) -> Option<String> {
    arguments
        .get(key)
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(str::to_owned)
}

pub(crate) fn optional_bool(arguments: &Value, key: &str, default: bool) -> bool {
    match arguments.get(key) {
        Some(Value::Bool(value)) => *value,
        Some(Value::String(value)) => match value.trim().to_ascii_lowercase().as_str() {
            "true" | "1" | "yes" | "y" => true,
            "false" | "0" | "no" | "n" => false,
            _ => default,
        },
        _ => default,
    }
}

pub(crate) fn optional_integer(arguments: &Value, key: &str, default: i64) -> i64 {
    match arguments.get(key) {
        Some(Value::Number(value)) => value.as_i64().unwrap_or(default),
        Some(Value::String(value)) => value.trim().parse().unwrap_or(default),
        _ => default,
    }
}

pub(crate) fn provider_model(row: jftrade_store_sqlite::StoredAdkEntity) -> Result<Value, String> {
    let payload: Value = serde_json::from_str(&row.payload_json)
        .map_err(|error| format!("invalid model provider payload: {error}"))?;
    let object = payload
        .as_object()
        .ok_or_else(|| "invalid model provider payload: expected object".to_owned())?;
    let text = |key: &str| {
        object
            .get(key)
            .and_then(Value::as_str)
            .unwrap_or_default()
            .trim()
            .to_owned()
    };
    let enabled = object
        .get("enabled")
        .and_then(Value::as_bool)
        .unwrap_or(false);
    let has_api_key = object
        .get("hasApiKey")
        .and_then(Value::as_bool)
        .unwrap_or_else(|| {
            object
                .get("apiKey")
                .and_then(Value::as_str)
                .is_some_and(|key| !key.trim().is_empty())
        });
    Ok(json!({
        "providerId": row.id,
        "providerName": text("displayName"),
        "model": text("model"),
        "baseUrl": text("baseUrl"),
        "contextWindowTokens": object.get("contextWindowTokens").and_then(Value::as_i64).unwrap_or(0),
        "enabled": enabled,
        "default": object.get("default").and_then(Value::as_bool).unwrap_or(false),
        "hasApiKey": has_api_key,
        "callable": enabled && has_api_key,
        "capabilities": safe_capabilities(object.get("capabilities")),
    }))
}

fn safe_capabilities(value: Option<&Value>) -> Value {
    let Some(object) = value.and_then(Value::as_object) else {
        return json!({});
    };
    let mut capabilities = std::collections::BTreeMap::new();
    for (key, value) in object {
        if is_sensitive_key(key) {
            continue;
        }
        if let Some(value) = value.as_bool() {
            capabilities.insert(key.clone(), Value::Bool(value));
        }
    }
    Value::Object(capabilities.into_iter().collect())
}

fn is_sensitive_key(key: &str) -> bool {
    let compact = key
        .to_ascii_lowercase()
        .chars()
        .filter(|character| character.is_ascii_alphanumeric())
        .collect::<String>();
    [
        "apikey",
        "token",
        "secret",
        "password",
        "credential",
        "authorization",
        "privatekey",
    ]
    .iter()
    .any(|needle| compact.contains(needle))
}

pub(crate) fn model_search_text(model: &Value) -> String {
    let mut values = Vec::new();
    for key in ["providerId", "providerName", "model", "baseUrl"] {
        if let Some(value) = model.get(key).and_then(Value::as_str) {
            values.push(value.to_ascii_lowercase());
        }
    }
    if let Some(capabilities) = model.get("capabilities").and_then(Value::as_object) {
        values.extend(capabilities.iter().filter_map(|(key, value)| {
            value
                .as_bool()
                .filter(|enabled| *enabled)
                .map(|_| key.to_ascii_lowercase())
        }));
    }
    values.join(" ")
}

pub(crate) fn reviewed_tool_name(tool: &Value) -> Option<&str> {
    let name = tool.get("id").and_then(Value::as_str)?;
    REVIEWED_READ_ONLY_TOOLS.contains(&name).then_some(name)
}

pub(crate) fn validate_tool_arguments(name: &str, arguments: &Value) -> Result<(), String> {
    schema_catalog::validate_arguments(name, arguments)
        .map_err(|message| format!("invalid arguments for {name}: {message}"))
}

pub(crate) fn is_modern_protocol(version: &str) -> bool {
    version >= MODERN_PROTOCOL_VERSION
}

pub(crate) fn is_method_not_found(response: &Value) -> bool {
    response
        .get("error")
        .and_then(|error| error.get("code"))
        .and_then(Value::as_i64)
        == Some(-32601)
}

pub(crate) fn is_invalid_params(response: &Value) -> bool {
    response
        .get("error")
        .and_then(|error| error.get("code"))
        .and_then(Value::as_i64)
        == Some(-32602)
}

pub(crate) fn is_invalid_request(response: &Value) -> bool {
    response
        .get("error")
        .and_then(|error| error.get("code"))
        .and_then(Value::as_i64)
        == Some(-32600)
}

pub(crate) fn rpc_request_id(request: &Value) -> Value {
    request
        .get("id")
        .filter(|id| !id.is_null())
        .cloned()
        .unwrap_or(Value::Null)
}

pub(crate) fn known_method(method: &str) -> bool {
    matches!(
        method,
        "server/discover"
            | "initialize"
            | "ping"
            | "tools/list"
            | "resources/list"
            | "resources/read"
            | "resources/subscribe"
            | "resources/unsubscribe"
            | "tools/call"
            | "notifications/initialized"
            | "notifications/cancelled"
    )
}

pub(crate) fn validate_call_shape(request: &Value) -> Result<(), &'static str> {
    let object = request.as_object().ok_or("Invalid Request")?;
    let method = object
        .get("method")
        .and_then(Value::as_str)
        .ok_or("Invalid Request")?;
    let has_id = object.get("id").is_some_and(|id| !id.is_null());
    let is_notification =
        method.starts_with("notifications/") || object.get("id").is_some_and(Value::is_null);
    if is_notification == has_id || (!is_notification && object.get("id").is_none()) {
        return Err("Invalid Request");
    }
    Ok(())
}

pub(crate) fn requires_object_params(method: &str) -> bool {
    matches!(
        method,
        "server/discover"
            | "initialize"
            | "resources/read"
            | "resources/subscribe"
            | "resources/unsubscribe"
            | "tools/call"
    )
}

#[allow(dead_code)]
pub(crate) fn tool_descriptors(catalog: &ProductionToolCatalog) -> Vec<Value> {
    tool_descriptors_with_ports(catalog, None)
}

pub(crate) fn tool_descriptors_with_ports(
    catalog: &ProductionToolCatalog,
    ports: Option<&ProductionPortBundle>,
) -> Vec<Value> {
    let callable = catalog.callable_tools();
    REVIEWED_READ_ONLY_TOOLS
        .iter()
        .map(|name| {
            let registered = callable
                .iter()
                .find(|tool| tool.get("id").and_then(Value::as_str) == Some(*name));
            let title = registered
                .and_then(|tool| tool.get("displayName").and_then(Value::as_str))
                .unwrap_or(name);
            let availability = mcp_tool_availability(catalog, ports, name);
            serde_json::json!({
                "name": name,
                "title": title,
                "description": title,
                "inputSchema": schema_catalog::schema_for(name),
                "x-jftrade-availability": availability
            })
        })
        .collect()
}

/// Validate the headers required by the Go SDK's stateless streamable
/// transport and return the effective protocol version.
pub(crate) fn validate_headers(headers: &HeaderMap) -> Result<String, ProtocolError> {
    if !is_json_media_type(headers.get(header::CONTENT_TYPE)) {
        return Err(ProtocolError::new(
            415,
            "Content-Type must be 'application/json'",
        ));
    }
    let (json_ok, stream_ok) = accepts_json_and_stream(headers);
    if !json_ok || !stream_ok {
        return Err(ProtocolError::new(
            400,
            "Accept must contain both 'application/json' and 'text/event-stream'",
        ));
    }
    let version = headers
        .get("Mcp-Protocol-Version")
        .and_then(|value| value.to_str().ok())
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .unwrap_or(DEFAULT_PROTOCOL_VERSION);
    if !SUPPORTED_PROTOCOL_VERSIONS.contains(&version) && version < MODERN_PROTOCOL_VERSION {
        return Err(ProtocolError::new(
            400,
            "Bad Request: Unsupported protocol version",
        ));
    }
    Ok(version.to_owned())
}

/// Standard request headers introduced by the 2026-07-28 protocol. The Go
/// SDK requires the method header for every request and a matching name for
/// operations whose target is encoded in params.
pub(crate) fn validate_standard_headers(
    headers: &HeaderMap,
    message: &Value,
    protocol_version: &str,
) -> Result<(), StandardHeaderError> {
    let Some(object) = message.as_object() else {
        return Err(StandardHeaderError::invalid_params("Invalid Request"));
    };
    let method = object
        .get("method")
        .and_then(Value::as_str)
        .ok_or_else(|| StandardHeaderError::invalid_params("Invalid Request"))?;
    let meta_version = object
        .get("params")
        .and_then(Value::as_object)
        .and_then(|params| params.get("_meta"))
        .and_then(Value::as_object)
        .and_then(|meta| meta.get(META_PROTOCOL_VERSION_KEY));
    let meta_version = match meta_version {
        Some(Value::String(value)) if !value.trim().is_empty() => Some(value.trim()),
        Some(_) => {
            return Err(StandardHeaderError::invalid_params(format!(
                "missing or invalid _meta field {META_PROTOCOL_VERSION_KEY:?}"
            )));
        }
        None => None,
    };
    let header_version = headers
        .get("Mcp-Protocol-Version")
        .and_then(|value| value.to_str().ok())
        .map(str::trim)
        .filter(|value| !value.is_empty());
    if protocol_version >= MODERN_PROTOCOL_VERSION || meta_version.is_some() {
        let Some(header_version) = header_version else {
            return Err(StandardHeaderError::mismatch(format!(
                "Mcp-Protocol-Version header is required for requests carrying {META_PROTOCOL_VERSION_KEY:?}"
            )));
        };
        let Some(meta_version) = meta_version else {
            return Err(StandardHeaderError::invalid_params(format!(
                "missing or invalid _meta field {META_PROTOCOL_VERSION_KEY:?}"
            )));
        };
        if header_version != meta_version {
            return Err(StandardHeaderError::mismatch(format!(
                "Mcp-Protocol-Version header {header_version:?} does not match request {META_PROTOCOL_VERSION_KEY} {meta_version:?}"
            )));
        }
    } else {
        return Ok(());
    }
    let method_header = headers
        .get("Mcp-Method")
        .and_then(|value| value.to_str().ok())
        .filter(|value| !value.is_empty())
        .ok_or_else(|| StandardHeaderError::mismatch("missing required Mcp-Method header"))?;
    if method_header != method {
        return Err(StandardHeaderError::mismatch(
            "Mcp-Method header does not match request method",
        ));
    }
    let target_name = match method {
        "tools/call" => object
            .get("params")
            .and_then(Value::as_object)
            .and_then(|params| params.get("name"))
            .and_then(Value::as_str),
        "resources/read" => object
            .get("params")
            .and_then(Value::as_object)
            .and_then(|params| params.get("uri"))
            .and_then(Value::as_str),
        _ => None,
    };
    if matches!(method, "tools/call" | "resources/read") && target_name.is_none() {
        return Err(StandardHeaderError::mismatch(
            "failed to extract name from parameters",
        ));
    }
    if let Some(target_name) = target_name {
        let name_header = headers
            .get("Mcp-Name")
            .and_then(|value| value.to_str().ok())
            .filter(|value| !value.is_empty())
            .ok_or_else(|| StandardHeaderError::mismatch("missing required Mcp-Name header"))?;
        if name_header != target_name {
            return Err(StandardHeaderError::mismatch(
                "Mcp-Name header does not match request target",
            ));
        }
    }
    Ok(())
}

fn is_json_media_type(value: Option<&axum::http::HeaderValue>) -> bool {
    let Some(value) = value.and_then(|value| value.to_str().ok()) else {
        return false;
    };
    let mut pieces = value.split(';');
    if !pieces
        .next()
        .is_some_and(|media| media.trim().eq_ignore_ascii_case("application/json"))
    {
        return false;
    }
    pieces.all(valid_media_parameter)
}

fn valid_media_parameter(parameter: &str) -> bool {
    let parameter = parameter.trim();
    if parameter.is_empty() {
        return false;
    }
    let Some((name, value)) = parameter.split_once('=') else {
        return false;
    };
    if name.trim().is_empty() || value.trim().is_empty() {
        return false;
    }
    let value = value.trim();
    (!value.starts_with('"') && !value.ends_with('"'))
        || (value.starts_with('"') && value.ends_with('"') && value.len() >= 2)
}

fn accepts_json_and_stream(headers: &HeaderMap) -> (bool, bool) {
    let mut json = false;
    let mut stream = false;
    for value in headers.get_all(header::ACCEPT) {
        let Ok(value) = value.to_str() else {
            continue;
        };
        for token in value.split(',') {
            let token = token.trim();
            let base = token
                .split_once(';')
                .map_or(token, |(base, _)| base)
                .trim()
                .to_ascii_lowercase();
            match base.as_str() {
                "application/json" | "application/*" => json = true,
                "text/event-stream" | "text/*" => stream = true,
                "*/*" => {
                    json = true;
                    stream = true;
                }
                _ => {}
            }
        }
    }
    (json, stream)
}

/// Decode one JSON-RPC object or a legacy batch.  Batching was removed from
/// protocol 2025-06-18 onward by the Go SDK; older clients remain compatible.
pub(crate) fn decode_messages(
    body: &[u8],
    protocol_version: &str,
) -> Result<Vec<Value>, &'static str> {
    let value: Value = serde_json::from_slice(body).map_err(|_| "Parse error")?;
    match value {
        Value::Array(items) => {
            if items.is_empty() {
                return Err("empty batch");
            }
            if protocol_version >= "2025-06-18" {
                return Err("JSON-RPC batching is not supported in 2025-06-18 and later");
            }
            Ok(items)
        }
        Value::Object(_) => Ok(vec![value]),
        _ => Err("Invalid Request"),
    }
}

/// The SDK negotiates legacy initialize requests to the requested supported
/// version, while 2026-07-28 (and unknown future versions) use the legacy
/// 2025-11-25 response until a newer implementation is available.
pub(crate) fn negotiate_initialize_version(params: &Value, header_version: &str) -> String {
    let requested = params
        .get("protocolVersion")
        .and_then(Value::as_str)
        .filter(|version| !version.is_empty())
        .unwrap_or(header_version);
    if requested >= MODERN_PROTOCOL_VERSION {
        return "2025-11-25".to_owned();
    }
    if SUPPORTED_PROTOCOL_VERSIONS.contains(&requested) {
        requested.to_owned()
    } else {
        "2025-11-25".to_owned()
    }
}

/// Validate the JSON-RPC envelope before dispatch.  Params are structured
/// values only; method-specific object validation is performed by the server.
pub(crate) fn validate_message(value: &Value) -> Result<(), &'static str> {
    let Some(object) = value.as_object() else {
        return Err("Invalid Request");
    };
    if object.get("jsonrpc") != Some(&Value::String("2.0".to_owned())) {
        return Err("Invalid Request");
    }
    if !object.get("method").is_some_and(Value::is_string)
        || object
            .get("method")
            .and_then(Value::as_str)
            .is_some_and(str::is_empty)
    {
        return Err("Invalid Request");
    }
    if let Some(params) = object.get("params")
        && !params.is_null()
        && !params.is_object()
        && !params.is_array()
    {
        return Err("Invalid Request");
    }
    if let Some(id) = object.get("id")
        && !(id.is_null() || id.is_string() || id.is_number())
    {
        return Err("Invalid Request");
    }
    Ok(())
}

#[cfg(test)]
#[path = "product_mcp_protocol_tests.rs"]
mod tests;
