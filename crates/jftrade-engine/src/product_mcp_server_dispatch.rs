use super::*;

pub(super) async fn handle_mcp_request(
    State(context): State<Arc<McpRequestContext>>,
    ConnectInfo(remote): ConnectInfo<SocketAddr>,
    request: Request<Body>,
) -> Response {
    let Some(state) = context.state.upgrade() else {
        return plain_error(StatusCode::SERVICE_UNAVAILABLE, "MCP server unavailable");
    };
    let settings = {
        let state = state.lock().unwrap_or_else(|error| error.into_inner());
        state.settings.clone()
    };
    if !is_loopback_remote(remote) {
        return plain_error(StatusCode::FORBIDDEN, "Forbidden");
    }
    if !settings.enabled {
        return plain_error(StatusCode::SERVICE_UNAVAILABLE, "Service Unavailable");
    }
    if settings.auth_mode != "none" {
        let authorized = request
            .headers()
            .get(header::AUTHORIZATION)
            .and_then(|value| value.to_str().ok())
            .and_then(bearer_token)
            .is_some_and(|token| verify_mcp_server_token(&settings.token_hash, token));
        if !authorized {
            let mut response = plain_error(StatusCode::UNAUTHORIZED, "Unauthorized");
            response.headers_mut().insert(
                header::WWW_AUTHENTICATE,
                header::HeaderValue::from_static("Bearer"),
            );
            return response;
        }
    }

    let host = request
        .headers()
        .get(header::HOST)
        .and_then(|value| value.to_str().ok())
        .unwrap_or_default();
    if !is_loopback_host(host) {
        return plain_error(StatusCode::FORBIDDEN, "Forbidden: invalid Host header");
    }

    if !mcp_origin_allowed(request.headers(), host) {
        return plain_error(StatusCode::FORBIDDEN, "Forbidden: invalid Origin header");
    }

    if request.method() != Method::POST {
        let mut response = plain_error(StatusCode::METHOD_NOT_ALLOWED, "Method Not Allowed");
        response
            .headers_mut()
            .insert(header::ALLOW, header::HeaderValue::from_static("POST"));
        return response;
    }

    let protocol_version = match validate_headers(request.headers()) {
        Ok(version) => version,
        Err(error) => {
            return plain_error(
                StatusCode::from_u16(error.status).unwrap_or(StatusCode::BAD_REQUEST),
                error.message,
            );
        }
    };

    let request_headers = request.headers().clone();
    let body = match to_bytes(request.into_body(), MCP_MAX_REQUEST_BYTES + 1).await {
        Ok(body) => body,
        Err(_) => return plain_error(StatusCode::PAYLOAD_TOO_LARGE, "Request Entity Too Large"),
    };
    if body.len() > MCP_MAX_REQUEST_BYTES {
        return plain_error(StatusCode::PAYLOAD_TOO_LARGE, "Request Entity Too Large");
    }
    let requests = match decode_messages(&body, &protocol_version) {
        Ok(requests) => requests,
        Err(message) => return plain_error(StatusCode::BAD_REQUEST, message),
    };
    if requests.len() == 1 {
        if let Err(message) = validate_standard_headers(
            &request_headers,
            requests.first().expect("one message"),
            &protocol_version,
        ) {
            let id = rpc_request_id(requests.first().expect("one message"));
            return json_response(
                StatusCode::BAD_REQUEST,
                rpc_error_value(id, message.code, &message.message),
            );
        }
        return dispatch_rpc(
            &context,
            requests.into_iter().next().expect("one message"),
            &protocol_version,
        )
        .await;
    }
    dispatch_rpc_batch(&context, requests, &protocol_version).await
}
pub(super) fn is_loopback_remote(remote: SocketAddr) -> bool {
    remote.ip().is_loopback()
}
pub(super) fn is_loopback_host(value: &str) -> bool {
    let value = value.trim();
    if value.is_empty() {
        return false;
    }
    if value == "localhost" {
        return true;
    }
    if let Some((host, port)) = value.rsplit_once(':')
        && host == "localhost"
        && port.parse::<u16>().is_ok()
    {
        return true;
    }
    value
        .parse::<SocketAddr>()
        .map(|address| address.ip().is_loopback())
        .or_else(|_| {
            value
                .trim_matches(['[', ']'])
                .parse::<std::net::IpAddr>()
                .map(|ip| ip.is_loopback())
        })
        .unwrap_or(false)
}

pub(super) fn mcp_origin_allowed(headers: &axum::http::HeaderMap, host: &str) -> bool {
    let Some(value) = headers.get(header::ORIGIN) else {
        return true;
    };
    let Ok(origin) = value.to_str() else {
        return false;
    };
    let origin = origin.trim();
    if origin.is_empty() || origin.eq_ignore_ascii_case("null") {
        return false;
    }
    let Ok(origin_url) = reqwest::Url::parse(origin) else {
        return false;
    };
    if !matches!(origin_url.scheme(), "http" | "https")
        || origin_url.host_str().is_none()
        || origin_url.path() != "/"
        || origin_url.query().is_some()
        || origin_url.fragment().is_some()
        || !origin_url.username().is_empty()
        || origin_url.password().is_some()
    {
        return false;
    }
    let Ok(host_url) = reqwest::Url::parse(&format!("http://{host}")) else {
        return false;
    };
    origin_url
        .host_str()
        .zip(host_url.host_str())
        .is_some_and(|(origin_host, request_host)| {
            origin_host.eq_ignore_ascii_case(request_host)
                && origin_url.port_or_known_default() == host_url.port_or_known_default()
        })
}
fn bearer_token(header_value: &str) -> Option<&str> {
    let mut parts = header_value.split_whitespace();
    let scheme = parts.next()?;
    let token = parts.next()?;
    if parts.next().is_some() || !scheme.eq_ignore_ascii_case("Bearer") {
        return None;
    }
    (!token.is_empty()).then_some(token)
}
async fn dispatch_rpc(
    context: &McpRequestContext,
    request: Value,
    protocol_version: &str,
) -> Response {
    if let Err(message) = validate_message(&request) {
        return plain_error(StatusCode::BAD_REQUEST, message);
    }
    if let Err(message) = validate_call_shape(&request) {
        return plain_error(StatusCode::BAD_REQUEST, message);
    }
    if !known_method(
        request
            .get("method")
            .and_then(Value::as_str)
            .unwrap_or_default(),
    ) {
        let id = request.get("id").cloned().unwrap_or(Value::Null);
        if is_modern_protocol(protocol_version) && !id.is_null() {
            return json_response(
                StatusCode::NOT_FOUND,
                rpc_error_value(id, -32601, "Method not found"),
            );
        }
        return plain_error(StatusCode::BAD_REQUEST, "Method not found");
    }
    let Some(result) = dispatch_rpc_value(context, request, protocol_version).await else {
        return StatusCode::ACCEPTED.into_response();
    };
    let status = if is_method_not_found(&result) && is_modern_protocol(protocol_version) {
        StatusCode::NOT_FOUND
    } else if (is_invalid_params(&result) || is_invalid_request(&result))
        && is_modern_protocol(protocol_version)
    {
        StatusCode::BAD_REQUEST
    } else {
        StatusCode::OK
    };
    json_response(status, result)
}
async fn dispatch_rpc_batch(
    context: &McpRequestContext,
    requests: Vec<Value>,
    protocol_version: &str,
) -> Response {
    let mut responses = Vec::with_capacity(requests.len());
    for request in requests {
        if let Err(message) = validate_message(&request) {
            return plain_error(StatusCode::BAD_REQUEST, message);
        }
        if let Err(message) = validate_call_shape(&request) {
            return plain_error(StatusCode::BAD_REQUEST, message);
        }
        if !known_method(
            request
                .get("method")
                .and_then(Value::as_str)
                .unwrap_or_default(),
        ) {
            return plain_error(StatusCode::BAD_REQUEST, "Method not found");
        }
        if let Some(response) = dispatch_rpc_value(context, request, protocol_version).await {
            responses.push(response);
        }
    }
    if responses.is_empty() {
        StatusCode::ACCEPTED.into_response()
    } else {
        json_response(StatusCode::OK, Value::Array(responses))
    }
}
async fn dispatch_rpc_value(
    context: &McpRequestContext,
    request: Value,
    protocol_version: &str,
) -> Option<Value> {
    let object = request.as_object().expect("RPC request object validated");
    // Go treats an explicit JSON null ID as an absent ID/notification.
    let id = object.get("id").filter(|id| !id.is_null()).cloned();
    let method = object.get("method").and_then(Value::as_str);
    let Some(method) = method else {
        return Some(rpc_error_value(
            id.unwrap_or(Value::Null),
            -32600,
            "Invalid Request",
        ));
    };
    let params = object.get("params").cloned().unwrap_or_else(|| json!({}));
    if requires_object_params(method) {
        match object.get("params") {
            None | Some(Value::Null) => {
                return id.map(|id| rpc_error_value(id, -32600, "Invalid Request"));
            }
            Some(params) if !params.is_object() => {
                return id.map(|id| rpc_error_value(id, -32602, "Invalid params"));
            }
            Some(_) => {}
        }
    }
    let Some(id) = id else {
        // Notifications are acknowledged without a response body.
        return None;
    };
    let result = match method {
        "server/discover" => Ok(json!({
            "_meta": {"io.modelcontextprotocol/serverInfo": {"name": MCP_SERVER_NAME, "version": MCP_SERVER_VERSION}},
            "cacheScope": "public",
            "capabilities": {
                "tools": {"listChanged": true},
                "resources": {"listChanged": true, "subscribe": true}
            },
            "instructions": "JFTrade local read-only market, portfolio, risk, strategy, and backtest tools.",
            "resultType": "complete",
            "supportedVersions": ["2026-07-28", "2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"],
            "ttlMs": 0
        })),
        "initialize" => Ok(json!({
            "protocolVersion": negotiate_initialize_version(&params, protocol_version),
            "capabilities": {
                "tools": {"listChanged": true},
                "resources": {"listChanged": true, "subscribe": true}
            },
            "serverInfo": {"name": MCP_SERVER_NAME, "version": MCP_SERVER_VERSION},
            "instructions": "JFTrade local read-only market, portfolio, risk, strategy, and backtest tools."
        })),
        "ping" => Ok(json!({})),
        "tools/list" => Ok(json!({
            "tools": tool_descriptors_with_ports(
                &context.catalog,
                context.production_ports.as_deref(),
            )
        })),
        "resources/list" => Ok(json!({"resources": [runtime_resource()]})),
        "resources/read" => read_resource(context, &params),
        "resources/subscribe" | "resources/unsubscribe" => subscribe_resource(&params),
        "tools/call" => call_tool(context, &params),
        "notifications/initialized" | "notifications/cancelled" => Ok(json!({})),
        _ => Err((-32601, "Method not found".to_owned())),
    };
    match result {
        Ok(mut result) => {
            if is_modern_protocol(protocol_version)
                && method != "server/discover"
                && method != "initialize"
                && let Some(object) = result.as_object_mut()
            {
                object
                    .entry("_meta")
                    .or_insert_with(|| json!({"io.modelcontextprotocol/serverInfo": {"name": MCP_SERVER_NAME, "version": MCP_SERVER_VERSION}}));
                object
                    .entry("resultType")
                    .or_insert_with(|| Value::String("complete".to_owned()));
                object
                    .entry("ttlMs")
                    .or_insert_with(|| Value::Number(0.into()));
                object
                    .entry("cacheScope")
                    .or_insert_with(|| Value::String("public".to_owned()));
            }
            Some(rpc_result_value(id, result))
        }
        Err((code, message)) => Some(rpc_error_value(id, code, &message)),
    }
}

fn runtime_resource() -> Value {
    json!({
        "name": "runtime-status",
        "title": "JFTrade Runtime Status",
        "description": "Sanitized JFTrade assistant runtime status.",
        "mimeType": "application/json",
        "uri": MCP_RUNTIME_STATUS_URI
    })
}

fn read_resource(context: &McpRequestContext, params: &Value) -> Result<Value, (i64, String)> {
    let uri = params
        .get("uri")
        .and_then(Value::as_str)
        .unwrap_or_default();
    if uri != MCP_RUNTIME_STATUS_URI {
        return Err((-32002, format!("resource not found: {uri}")));
    }
    let status = context
        .state
        .upgrade()
        .and_then(|state| {
            state.lock().ok().map(|state| {
            json!({
                "running": state.server.as_ref().is_some_and(McpServerOwner::is_running),
                "endpoint": state.bind.as_deref().map(|bind| format!("http://{bind}{MCP_PATH}")),
                "tools": tool_descriptors_with_ports(
                    &context.catalog,
                    context.production_ports.as_deref(),
                )
            })
        })
        })
        .unwrap_or_else(|| json!({"running": false, "tools": []}));
    Ok(
        json!({"contents": [{"uri": uri, "mimeType": "application/json", "text": serde_json::to_string(&status).unwrap_or_else(|_| "{}".to_owned())}]}),
    )
}

fn subscribe_resource(params: &Value) -> Result<Value, (i64, String)> {
    let uri = params
        .get("uri")
        .and_then(Value::as_str)
        .unwrap_or_default();
    if uri != MCP_RUNTIME_STATUS_URI {
        return Err((-32002, format!("resource not found: {uri}")));
    }
    Ok(json!({}))
}

fn call_tool(context: &McpRequestContext, params: &Value) -> Result<Value, (i64, String)> {
    let name = params
        .get("name")
        .and_then(Value::as_str)
        .unwrap_or_default();
    if !REVIEWED_READ_ONLY_TOOLS.contains(&name) {
        return Err((-32602, format!("unknown tool \"{name}\"")));
    }
    let availability =
        mcp_tool_availability(&context.catalog, context.production_ports.as_deref(), name);
    if !context.executor.supports(name) || availability == "fail-closed" {
        return Err((
            -32602,
            format!("tool \"{name}\" is unavailable in the Rust MCP runtime"),
        ));
    }
    let mut arguments = params
        .get("arguments")
        .cloned()
        .unwrap_or_else(|| json!({}));
    if !arguments.is_object() {
        return Err((-32602, "Invalid params".to_owned()));
    }
    normalize_legacy_mcp_arguments(name, &mut arguments);
    validate_tool_arguments(name, &arguments).map_err(|message| (-32602, message))?;
    match context.executor.execute_enveloped(name, &arguments) {
        Ok(value) => Ok(json!({
            "content": [{"type": "text", "text": serde_json::to_string(&value).unwrap_or_else(|_| "{}".to_owned())}],
            "structuredContent": value,
        })),
        Err(error) => Ok(json!({
            "content": [{"type": "text", "text": serde_json::to_string(&error).unwrap_or_else(|_| "{}".to_owned())}],
            "structuredContent": error,
            "isError": true,
        })),
    }
}

pub(crate) fn normalize_legacy_mcp_arguments(name: &str, arguments: &mut Value) {
    let Some(obj) = arguments.as_object_mut() else {
        return;
    };
    match name {
        "market.search" => {
            if let Some(limit) = obj.remove("limit") {
                obj.entry("pageSize".to_owned()).or_insert(limit);
            }
        }
        "broker.fees" => {
            if let Some(val) = obj.get("orderIdEx").cloned()
                && val.is_string()
            {
                obj.insert("orderIdEx".to_owned(), json!([val]));
            }
            if let Some(val) = obj.get("orderIdExList").cloned()
                && val.is_string()
            {
                obj.insert("orderIdExList".to_owned(), json!([val]));
            }
        }
        "market.snapshot" => {
            if !obj.contains_key("instrumentId")
                && let (Some(market), Some(symbol)) = (
                    obj.get("market").and_then(Value::as_str),
                    obj.get("symbol").and_then(Value::as_str),
                )
            {
                let instrument_id = format!("{market}.{symbol}");
                obj.insert("instrumentId".to_owned(), Value::String(instrument_id));
                obj.remove("symbol");
            }
        }
        "execution.buying_power" => {
            if let Some(instrument) = obj.get("instrument").and_then(Value::as_str) {
                let instrument_val = instrument.to_owned();
                obj.insert(
                    "instrument".to_owned(),
                    json!({"instrumentId": instrument_val}),
                );
            }
        }
        _ => {}
    }
}

fn rpc_result_value(id: Value, result: Value) -> Value {
    json!({"jsonrpc": "2.0", "id": id, "result": result})
}

fn rpc_error_value(id: Value, code: i64, message: &str) -> Value {
    json!({"jsonrpc": "2.0", "id": id, "error": {"code": code, "message": message}})
}

fn json_response(status: StatusCode, body: Value) -> Response {
    (status, Json(body)).into_response()
}

fn plain_error(status: StatusCode, message: &str) -> Response {
    (status, message.to_owned()).into_response()
}
