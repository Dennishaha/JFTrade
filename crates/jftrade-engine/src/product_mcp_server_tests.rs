use super::*;
use crate::product::product_mcp_protocol::{mcp_tool_adapter, mcp_tool_availability};
use crate::product::product_production_ports::{
    MarketDataCapabilityMatrix, ProductionAdapterBinding, production_adapter_bindings,
};
use crate::product::{ProductCapabilities, ProductConfig, product_data_management};
use axum::http::{HeaderMap, HeaderValue, header};
use jftrade_api::AccessPolicy;
use jftrade_settings::McpServerSecretPort;
use jftrade_settings::SecuritySettingsService;
use jftrade_store_settings_file::SettingsFileStore;
use std::collections::BTreeMap;
use std::fs;
use std::io::{Read, Write};
use std::net::SocketAddr;
use std::net::TcpStream;
use std::time::Duration;
use tempfile::TempDir;

const MCP_TEST_IO_TIMEOUT: Duration = Duration::from_secs(10);

fn available_port() -> u16 {
    StdTcpListener::bind("127.0.0.1:0")
        .expect("reserve MCP test port")
        .local_addr()
        .expect("MCP test address")
        .port()
}

fn catalog() -> Arc<ProductionToolCatalog> {
    let bindings = production_adapter_bindings(&MarketDataCapabilityMatrix::new(
        Some("yfinance"),
        true,
        true,
    ));
    let research = BTreeMap::from([
        ("instrument", ProductionAdapterBinding::Ready),
        ("financials", ProductionAdapterBinding::Ready),
        ("valuation", ProductionAdapterBinding::Ready),
        ("news", ProductionAdapterBinding::Ready),
    ]);
    Arc::new(
        ProductionToolCatalog::from_bindings_with_research(&bindings, &research)
            .expect("fixture MCP catalog"),
    )
}

fn unavailable_catalog() -> Arc<ProductionToolCatalog> {
    let mut bindings = production_adapter_bindings(&MarketDataCapabilityMatrix::new(
        Some("yfinance"),
        true,
        true,
    ));
    for binding in bindings.values_mut() {
        *binding = ProductionAdapterBinding::ExternalUnavailable;
    }
    let research = BTreeMap::from([
        ("instrument", ProductionAdapterBinding::ExternalUnavailable),
        ("financials", ProductionAdapterBinding::ExternalUnavailable),
        ("valuation", ProductionAdapterBinding::ExternalUnavailable),
        ("news", ProductionAdapterBinding::ExternalUnavailable),
    ]);
    Arc::new(
        ProductionToolCatalog::from_bindings_with_research(&bindings, &research)
            .expect("complete unavailable MCP catalog"),
    )
}

fn production_bundle() -> (
    TempDir,
    crate::product::product_production_ports::ProductionPortBundle,
) {
    let directory = tempfile::tempdir().expect("production MCP temp directory");
    let settings_path = directory.path().join("settings.json");
    fs::write(
        &settings_path,
        br#"{"pineWorker":{"backtestWorkerLimit":2,"instanceWorkerLimit":10,"nodeBinaryPath":"/definitely/missing/node"}}"#,
    )
    .expect("write production MCP settings");
    product_data_management::initialize_production_databases(&settings_path)
        .expect("initialize production MCP databases");
    let settings_store = Arc::new(SettingsFileStore::open(&settings_path).expect("settings store"));
    let security = SecuritySettingsService::new(settings_store);
    let mut config = ProductConfig::new(
        SocketAddr::from(([127, 0, 0, 1], 0)),
        &settings_path,
        AccessPolicy::default(),
    )
    .expect("production MCP config");
    config.capabilities = ProductCapabilities::all();
    config.production = true;
    let ports = crate::product::product_production_ports::production_ports(&config, &security)
        .expect("production MCP ports");
    (directory, ports)
}

#[derive(Debug)]
struct NoopExecutor;

impl McpToolExecutor for NoopExecutor {
    fn execute(&self, _name: &str, _arguments: &Value) -> Result<Value, String> {
        Err("fixture executor unavailable".to_owned())
    }
}

#[derive(Debug)]
struct SuccessExecutor;

impl McpToolExecutor for SuccessExecutor {
    fn execute(&self, name: &str, _arguments: &Value) -> Result<Value, String> {
        Ok(json!({"ok": true, "tool": name}))
    }
}

fn runtime() -> Arc<ProductMcpServerRuntime> {
    ProductMcpServerRuntime::with_executor(catalog(), Arc::new(NoopExecutor))
}

fn request(port: u16, token: Option<&str>, payload: &str, remote: &str) -> String {
    let mut stream = TcpStream::connect(("127.0.0.1", port)).expect("connect MCP listener");
    stream
        .set_read_timeout(Some(MCP_TEST_IO_TIMEOUT))
        .expect("set MCP timeout");
    let auth = token
        .map(|token| format!("Authorization: Bearer {token}\r\n"))
        .unwrap_or_default();
    let request = format!(
        "POST /mcp HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\nX-Test-Remote: {remote}\r\nContent-Type: application/json\r\nAccept: application/json, text/event-stream\r\nContent-Length: {}\r\n{}\r\n{}",
        payload.len(),
        auth,
        payload
    );
    stream
        .write_all(request.as_bytes())
        .expect("write MCP request");
    let mut response = String::new();
    stream
        .read_to_string(&mut response)
        .expect("read MCP response");
    response
}

fn request_with_method(
    port: u16,
    method: &str,
    host: Option<&str>,
    token: Option<&str>,
    payload: &str,
) -> String {
    let mut stream = TcpStream::connect(("127.0.0.1", port)).expect("connect MCP listener");
    stream
        .set_read_timeout(Some(MCP_TEST_IO_TIMEOUT))
        .expect("set MCP timeout");
    let host = host.map_or(String::new(), |host| format!("Host: {host}\r\n"));
    let auth = token
        .map(|token| format!("Authorization: Bearer {token}\r\n"))
        .unwrap_or_default();
    let request = format!(
        "{method} /mcp HTTP/1.1\r\n{host}Connection: close\r\nContent-Type: application/json\r\nAccept: application/json, text/event-stream\r\nContent-Length: {}\r\n{auth}\r\n{payload}",
        payload.len()
    );
    stream
        .write_all(request.as_bytes())
        .expect("write MCP request");
    let mut response = String::new();
    stream
        .read_to_string(&mut response)
        .expect("read MCP response");
    response
}

fn request_modern(port: u16, method: &str, id: u64, name: Option<&str>, params: Value) -> Value {
    request_modern_with_status(port, method, id, name, params).1
}

fn request_modern_with_status(
    port: u16,
    method: &str,
    id: u64,
    name: Option<&str>,
    params: Value,
) -> (u16, Value) {
    let mut stream = TcpStream::connect(("127.0.0.1", port)).expect("connect MCP listener");
    stream
        .set_read_timeout(Some(MCP_TEST_IO_TIMEOUT))
        .expect("set MCP timeout");
    let mut params = params;
    if let Some(object) = params.as_object_mut() {
        object.insert(
            "_meta".to_owned(),
            json!({"io.modelcontextprotocol/protocolVersion": "2026-07-28"}),
        );
    }
    let payload = json!({"jsonrpc":"2.0", "id": id, "method": method, "params": params});
    let body = serde_json::to_vec(&payload).expect("encode MCP payload");
    let name_header = name.map_or(String::new(), |name| format!("Mcp-Name: {name}\r\n"));
    let request = format!(
        "POST /mcp HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\nContent-Type: application/json\r\nAccept: application/json, text/event-stream\r\nMcp-Protocol-Version: 2026-07-28\r\nMcp-Method: {method}\r\n{name_header}Content-Length: {}\r\n\r\n",
        body.len()
    );
    stream
        .write_all(request.as_bytes())
        .expect("write MCP headers");
    stream.write_all(&body).expect("write MCP body");
    let mut response = String::new();
    stream
        .read_to_string(&mut response)
        .expect("read MCP response");
    let status = response
        .lines()
        .next()
        .and_then(|line| line.split_whitespace().nth(1))
        .and_then(|value| value.parse::<u16>().ok())
        .expect("MCP HTTP status");
    let body = response
        .split_once("\r\n\r\n")
        .map(|(_, body)| body)
        .unwrap_or_default();
    (
        status,
        serde_json::from_str(body).unwrap_or_else(|_| panic!("invalid MCP response: {response}")),
    )
}

fn enabled_record(port: u16, auth_mode: &str, token_hash: &str) -> McpServerSettingsRecord {
    McpServerSettingsRecord::new(true, i32::from(port), auth_mode, token_hash)
}

fn assert_corpus_result(actual: &Value, expected_entry: &Value, index: usize) {
    let actual_result = actual.get("result").expect("JSON-RPC result");
    let expected_result = expected_entry["response"]
        .as_object()
        .expect("corpus result object");
    for (key, expected) in expected_result {
        if key == "toolNames" {
            let actual_names = actual_result["tools"]
                .as_array()
                .expect("tools/list tools")
                .iter()
                .filter_map(|tool| tool.get("name").and_then(Value::as_str))
                .map(str::to_owned)
                .collect::<Vec<_>>();
            let expected_names = expected
                .as_array()
                .expect("corpus tool names")
                .iter()
                .filter_map(Value::as_str)
                .map(str::to_owned)
                .collect::<Vec<_>>();
            assert_eq!(
                actual_names, expected_names,
                "corpus exchange {index} tool names"
            );
        } else if key == "isError" && expected == &Value::Bool(false) {
            assert!(
                actual_result.get(key).is_none(),
                "corpus exchange {index} success unexpectedly includes isError"
            );
        } else {
            assert_eq!(
                actual_result.get(key),
                Some(expected),
                "corpus exchange {index} result.{key}"
            );
        }
    }
}

#[test]
fn reviewed_catalog_preserves_all_go_names_and_marks_unimplemented_rust_calls() {
    let descriptors = tool_descriptors(&catalog());
    assert_eq!(descriptors.len(), 69);
    let native = PRODUCTION_MCP_EXECUTABLE_TOOLS
        .iter()
        .copied()
        .collect::<std::collections::BTreeSet<_>>();
    for tool in descriptors {
        let name = tool["name"].as_str().expect("descriptor name");
        let availability = tool["x-jftrade-availability"]
            .as_str()
            .expect("availability marker");
        if native.contains(name) {
            assert!(
                matches!(availability, "ready" | "unavailable"),
                "native tool {name} must project runtime readiness, got {availability}"
            );
        } else {
            assert_eq!(availability, "fail-closed", "unimplemented tool {name}");
        }
    }
}

#[test]
fn native_mcp_names_have_explicit_mapping_and_fail_closed_matrix() {
    let native = PRODUCTION_MCP_EXECUTABLE_TOOLS
        .iter()
        .copied()
        .collect::<std::collections::BTreeSet<_>>();
    let catalog = unavailable_catalog();
    for name in REVIEWED_READ_ONLY_TOOLS {
        let adapter = mcp_tool_adapter(name);
        let availability = mcp_tool_availability(&catalog, None, name);
        if native.contains(name) {
            assert!(
                adapter.is_some(),
                "native tool {name} has no adapter mapping"
            );
            assert_eq!(
                availability, "unavailable",
                "native tool {name} must surface external unavailability"
            );
        } else {
            assert!(
                adapter.is_none(),
                "non-native tool {name} has an adapter mapping"
            );
            assert_eq!(availability, "fail-closed", "non-native tool {name}");
        }
    }
}

#[test]
fn reviewed_mcp_catalog_reports_native_and_fail_closed_counts() {
    let native = PRODUCTION_MCP_EXECUTABLE_TOOLS
        .iter()
        .copied()
        .collect::<std::collections::BTreeSet<_>>();
    let reviewed = REVIEWED_READ_ONLY_TOOLS
        .iter()
        .copied()
        .collect::<std::collections::BTreeSet<_>>();
    assert_eq!(reviewed.len(), 69);
    assert_eq!(native.len(), 31);
    assert!(native.is_subset(&reviewed));
    assert_eq!(reviewed.len() - native.len(), 38);
}

#[test]
fn production_mcp_local_tools_use_the_real_bundle_ports() {
    let (_directory, ports) = production_bundle();
    let executor = ProductionMcpToolExecutor::from_production_ports(Arc::new(ports));

    let plugins = executor
        .execute_production("plugins.catalog", &json!({}))
        .expect("plugins catalog production read");
    assert!(plugins.is_object(), "plugins={plugins}");

    let definitions = executor
        .execute_production("strategy.definitions", &json!({}))
        .expect("strategy definitions production read");
    assert_eq!(definitions["definitions"], json!([]));
    assert_eq!(definitions["definitionCount"], 0);

    let backtests = executor
        .execute_production("backtest.runs", &json!({}))
        .expect("backtest runs production read");
    assert!(backtests["runs"].is_null(), "backtests={backtests}");

    let dependencies = executor
        .execute_production("system.runtime_dependencies", &json!({}))
        .expect("runtime dependencies production read");
    assert!(
        dependencies["dependencies"].is_array(),
        "dependencies={dependencies}"
    );
}

#[test]
fn production_mcp_runtime_dependency_readiness_is_truthful() {
    let (_directory, mut ports) = production_bundle();
    assert_eq!(
        mcp_tool_availability(
            &ports.mcp_catalog,
            Some(&ports),
            "system.runtime_dependencies"
        ),
        "ready"
    );
    assert_eq!(
        mcp_tool_availability(&ports.mcp_catalog, Some(&ports), "research.screen_catalog"),
        "fail-closed"
    );

    ports.bound_adapters.remove(
        &crate::product::product_production_route_registry::ProductionRouteAdapter::SystemRead,
    );
    assert_eq!(
        mcp_tool_availability(
            &ports.mcp_catalog,
            Some(&ports),
            "system.runtime_dependencies"
        ),
        "fail-closed"
    );
}

#[test]
fn production_mcp_runtime_dependency_store_failures_return_503() {
    let (directory, ports) = production_bundle();
    fs::write(directory.path().join("settings.json"), b"{ malformed")
        .expect("corrupt production MCP settings");
    let executor = ProductionMcpToolExecutor::from_production_ports(Arc::new(ports));
    let failure = executor
        .execute_production("system.runtime_dependencies", &json!({}))
        .expect_err("malformed settings must fail closed");
    assert_eq!(failure.status, 503);
    assert_eq!(failure.code, "SYSTEM_READ_UNAVAILABLE");
}

#[test]
fn rust_listener_contract_is_backed_by_frozen_go_sdk_corpus() {
    let corpus: Value = serde_json::from_str(include_str!(
        "../../../testdata/mcp_go_sdk_v1_7_corpus.json"
    ))
    .expect("decode Go MCP corpus");
    let entries = corpus["entries"].as_array().expect("modern corpus entries");
    assert_eq!(entries.len(), 3);
    assert_eq!(entries[0]["requestMethod"], "server/discover");
    assert_eq!(entries[1]["requestMethod"], "tools/list");
    assert_eq!(entries[2]["requestMethod"], "tools/call");
    assert!(entries.iter().all(|entry| entry["method"] == "POST"
        && entry["path"] == "/mcp"
        && entry["status"] == 200));
    let expected_names = entries[1]["response"]["toolNames"]
        .as_array()
        .expect("frozen Go tool names");
    let names = tool_descriptors(&catalog())
        .into_iter()
        .filter_map(|tool| tool.get("name").and_then(Value::as_str).map(str::to_owned))
        .map(Value::String)
        .collect::<Vec<_>>();
    assert_eq!(&names, expected_names);
}

#[test]
fn modern_go_corpus_replay_matches_http_status_headers_and_result_shape() {
    let corpus: Value = serde_json::from_str(include_str!(
        "../../../testdata/mcp_go_sdk_v1_7_corpus.json"
    ))
    .expect("decode Go MCP corpus");
    let entries = corpus["entries"].as_array().expect("modern corpus entries");
    assert_eq!(entries.len(), 3);

    let runtime = ProductMcpServerRuntime::with_executor(catalog(), Arc::new(SuccessExecutor));
    let port = available_port();
    runtime
        .apply(&enabled_record(port, "none", ""))
        .expect("start MCP");
    let requests = [
        ("server/discover", None, json!({})),
        ("tools/list", None, json!({})),
        (
            "tools/call",
            Some("system.status"),
            json!({"name": "system.status", "arguments": {}}),
        ),
    ];
    for (index, (method, name, params)) in requests.into_iter().enumerate() {
        let (status, response) =
            request_modern_with_status(port, method, index as u64 + 1, name, params);
        let entry = &entries[index];
        assert_eq!(entry["method"], "POST", "corpus exchange {index} method");
        assert_eq!(entry["path"], "/mcp", "corpus exchange {index} path");
        assert_eq!(
            entry["requestMethod"], method,
            "corpus exchange {index} RPC method"
        );
        assert_eq!(
            status,
            entry["status"].as_u64().expect("corpus status") as u16
        );
        let headers = entry["headers"].as_object().expect("corpus headers");
        assert_eq!(headers["Content-Type"][0], "application/json");
        assert_eq!(headers["Accept"][0], "application/json, text/event-stream");
        assert_eq!(headers["Mcp-Protocol-Version"][0], "2026-07-28");
        assert_eq!(headers["Mcp-Method"][0], method);
        match name {
            Some(name) => assert_eq!(headers["Mcp-Name"][0], name),
            None => assert!(headers.get("Mcp-Name").is_none()),
        }
        assert_corpus_result(&response, entry, index);
    }
    runtime.shutdown_blocking().expect("shutdown MCP");
}

#[test]
fn origin_policy_matches_go_for_absent_same_origin_and_rejections() {
    let cases = [
        (None, true),
        (Some("http://localhost"), true),
        (Some("http://localhost/"), true),
        (Some("://bad"), false),
        (Some("null"), false),
        (Some("http://evil.example"), false),
    ];
    for (origin, expected) in cases {
        let mut headers = HeaderMap::new();
        if let Some(origin) = origin {
            headers.insert(
                header::ORIGIN,
                HeaderValue::from_str(origin).expect("origin"),
            );
        }
        assert_eq!(
            mcp_origin_allowed(&headers, "localhost"),
            expected,
            "Origin {origin:?}"
        );
    }
}

#[test]
fn disabled_runtime_has_stopped_status_and_releases_listener() {
    let runtime = runtime();
    let port = available_port();
    runtime
        .apply(&McpServerSettingsRecord::new(
            false,
            i32::from(port),
            "none",
            "",
        ))
        .expect("disable MCP");
    assert!(
        !runtime
            .status(&McpServerSettingsRecord::new(
                false,
                i32::from(port),
                "none",
                ""
            ))
            .expect("MCP status")
            .running
    );
    runtime.shutdown_blocking().expect("shutdown MCP");
    StdTcpListener::bind(("127.0.0.1", port)).expect("MCP port released");
}

#[test]
fn token_auth_and_tools_list_use_reviewed_catalog() {
    let runtime = runtime();
    let port = available_port();
    let (token, token_hash) = jftrade_settings::SystemMcpServerSecrets
        .issue()
        .expect("fixture secret");
    runtime
        .apply(&enabled_record(port, "token", &token_hash))
        .expect("start MCP");
    let denied = request(
        port,
        None,
        r#"{"jsonrpc":"2.0","id":1,"method":"tools/list"}"#,
        "127.0.0.1:1",
    );
    assert!(denied.contains("401 Unauthorized"), "response = {denied}");
    let response = request(
        port,
        Some(&token),
        r#"{"jsonrpc":"2.0","id":1,"method":"tools/list"}"#,
        "127.0.0.1:1",
    );
    assert!(response.contains("200 OK"), "response = {response}");
    assert!(response.contains("system.status"), "response = {response}");
    runtime.shutdown_blocking().expect("shutdown MCP");
}

#[test]
fn modern_go_corpus_replays_discover_list_and_successful_call() {
    let runtime = ProductMcpServerRuntime::with_executor(catalog(), Arc::new(SuccessExecutor));
    let port = available_port();
    runtime
        .apply(&enabled_record(port, "none", ""))
        .expect("start MCP");

    let discover = request_modern(port, "server/discover", 1, None, json!({}));
    assert_eq!(
        discover["result"]["resultType"], "complete",
        "discover={discover}"
    );
    assert_eq!(discover["result"]["cacheScope"], "public");
    assert_eq!(
        discover["result"]["supportedVersions"]
            .as_array()
            .map(Vec::len),
        Some(5)
    );

    let list = request_modern(port, "tools/list", 2, None, json!({}));
    let tools = list["result"]["tools"]
        .as_array()
        .expect("tools/list array");
    assert_eq!(tools.len(), 69);
    assert_eq!(list["result"]["resultType"], "complete");
    assert_eq!(
        list["result"]["_meta"]["io.modelcontextprotocol/serverInfo"]["name"],
        "jftrade"
    );

    let call = request_modern(
        port,
        "tools/call",
        3,
        Some("system.status"),
        json!({"name": "system.status", "arguments": {}}),
    );
    assert_eq!(call["result"]["resultType"], "complete");
    assert_eq!(call["result"]["structuredContent"]["ok"], true);
    assert_eq!(call["result"]["structuredContent"]["tool"], "system.status");
    assert_eq!(call["result"]["content"][0]["type"], "text");
    assert!(call["result"].get("isError").is_none());

    runtime.shutdown_blocking().expect("shutdown MCP");
}

#[test]
fn modern_unknown_tool_is_json_rpc_invalid_params_not_http_success() {
    let runtime = runtime();
    let port = available_port();
    runtime
        .apply(&enabled_record(port, "none", ""))
        .expect("start MCP");

    let (status, response) = request_modern_with_status(
        port,
        "tools/call",
        1,
        Some("future.tool"),
        json!({"name": "future.tool", "arguments": {}}),
    );
    assert_eq!(status, 400, "response={response}");
    assert_eq!(response["error"]["code"], -32602);
    assert_eq!(response["error"]["message"], "unknown tool \"future.tool\"");
    assert!(response.get("result").is_none(), "response={response}");

    runtime.shutdown_blocking().expect("shutdown MCP");
}

#[test]
fn modern_catalog_unavailable_tool_is_json_rpc_invalid_params_not_tool_success() {
    let bindings = production_adapter_bindings(&MarketDataCapabilityMatrix::new(
        Some("yfinance"),
        true,
        true,
    ));
    let research = BTreeMap::from([
        ("instrument", ProductionAdapterBinding::ExternalUnavailable),
        ("financials", ProductionAdapterBinding::Ready),
        ("valuation", ProductionAdapterBinding::Ready),
        ("news", ProductionAdapterBinding::Ready),
    ]);
    let catalog = Arc::new(
        ProductionToolCatalog::from_bindings_with_research(&bindings, &research)
            .expect("fixture MCP catalog"),
    );
    let runtime = ProductMcpServerRuntime::with_executor(catalog, Arc::new(NoopExecutor));
    let port = available_port();
    runtime
        .apply(&enabled_record(port, "none", ""))
        .expect("start MCP");

    let (status, response) = request_modern_with_status(
        port,
        "tools/call",
        1,
        Some("research.instrument"),
        json!({"name": "research.instrument", "arguments": {}}),
    );
    assert_eq!(status, 400, "response={response}");
    assert_eq!(response["error"]["code"], -32602);
    assert_eq!(
        response["error"]["message"],
        "tool \"research.instrument\" is unavailable in the Rust MCP runtime"
    );
    assert!(response.get("result").is_none(), "response={response}");

    runtime.shutdown_blocking().expect("shutdown MCP");
}

#[test]
fn loopback_policy_rejects_non_loopback_peer_addresses() {
    assert!(is_loopback_remote(
        "127.0.0.1:1".parse().expect("IPv4 peer")
    ));
    assert!(is_loopback_remote("[::1]:1".parse().expect("IPv6 peer")));
    assert!(!is_loopback_remote(
        "192.0.2.1:1".parse().expect("remote peer")
    ));
    for host in [
        "localhost",
        "localhost:6697",
        "127.0.0.1",
        "127.0.0.1:6697",
        "[::1]",
        "[::1]:6697",
    ] {
        assert!(is_loopback_host(host), "host should be accepted: {host}");
    }
    for host in ["", "evil.example", "localhost:bad", "127.0.0.1.evil"] {
        assert!(!is_loopback_host(host), "host should be rejected: {host}");
    }
}

#[test]
fn non_post_requests_still_cross_security_boundary_before_method_rejection() {
    let runtime = runtime();
    let port = available_port();
    let (token, token_hash) = jftrade_settings::SystemMcpServerSecrets
        .issue()
        .expect("fixture secret");
    runtime
        .apply(&enabled_record(port, "token", &token_hash))
        .expect("start MCP");
    let unauthorized = request_with_method(port, "GET", Some("localhost"), None, "");
    assert!(
        unauthorized.contains("401 Unauthorized"),
        "response = {unauthorized}"
    );
    let method_error = request_with_method(port, "GET", Some("localhost"), Some(&token), "");
    assert!(
        method_error.contains("405 Method Not Allowed"),
        "response = {method_error}"
    );
    assert!(
        method_error.contains("allow: POST"),
        "response = {method_error}"
    );
    runtime.shutdown_blocking().expect("shutdown MCP");
}

#[test]
fn host_rebinding_and_missing_host_are_rejected() {
    let runtime = runtime();
    let port = available_port();
    runtime
        .apply(&enabled_record(port, "none", ""))
        .expect("start MCP");
    let external = request_with_method(port, "POST", Some("evil.example"), None, "{}");
    assert!(external.contains("403 Forbidden"), "response = {external}");
    let missing = request_with_method(port, "POST", None, None, "{}");
    assert!(missing.contains("403 Forbidden"), "response = {missing}");
    runtime.shutdown_blocking().expect("shutdown MCP");
}

#[test]
fn port_conflict_keeps_previous_listener_and_reset_rebinds() {
    let runtime = runtime();
    let first = available_port();
    let second = available_port();
    runtime
        .apply(&enabled_record(first, "none", ""))
        .expect("start first MCP");
    let occupied = StdTcpListener::bind(("127.0.0.1", second)).expect("occupy MCP port");
    let error = runtime
        .apply(&enabled_record(second, "none", ""))
        .expect_err("conflicting MCP bind");
    assert!(error.contains("port conflict"), "error = {error}");
    assert!(
        runtime
            .status(&enabled_record(first, "none", ""))
            .expect("previous status")
            .running
    );
    drop(occupied);
    runtime
        .apply(&enabled_record(second, "none", ""))
        .expect("rebind MCP");
    assert!(
        runtime
            .status(&enabled_record(second, "none", ""))
            .expect("rebound status")
            .running
    );
    runtime.shutdown_blocking().expect("shutdown MCP");
}

#[test]
fn shutdown_is_idempotent_and_closed_runtime_rejects_rebind() {
    let runtime = runtime();
    runtime.shutdown_blocking().expect("first shutdown");
    runtime.shutdown_blocking().expect("second shutdown");
    let error = runtime
        .apply(&enabled_record(available_port(), "none", ""))
        .expect_err("closed MCP runtime");
    assert!(error.contains("closed"), "error = {error}");
}
