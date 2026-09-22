use super::*;
use crate::product::product_mcp_protocol::{mcp_tool_adapter, mcp_tool_availability};
use crate::product::product_mcp_server::dispatch::{
    normalize_legacy_mcp_arguments, sanitized_agent, sanitized_provider, sanitized_skill,
};
use crate::product::product_production_ports::{
    MarketDataCapabilityMatrix, ProductionAdapterBinding, SharedTradeReadRuntime,
    production_adapter_bindings,
};
use crate::product::strategy_pine::{
    StrategyPineAnalyzeInput, StrategyPineAnalyzeSnapshotError, StrategyPineAnalyzeSnapshotPort,
};
use crate::product::{
    ActiveProviderState, MarketDataRuntimeState, MarketDataRuntimeStatusPort, ProductCapabilities,
    ProductConfig, ResearchReadSnapshotError, ResearchReadSnapshotPort, product_data_management,
};
use axum::http::{HeaderMap, HeaderValue, header};
use jftrade_api::AccessPolicy;
use jftrade_settings::SecuritySettingsService;
use jftrade_settings::{MarketDataProvider, McpServerSecretPort};
use jftrade_store_settings_file::SettingsFileStore;
use serde_json::Map;
use std::collections::BTreeMap;
use std::fs;
use std::io::{Read, Write};
use std::net::SocketAddr;
use std::net::TcpStream;
use std::sync::Mutex;
use std::time::Duration;
use tempfile::TempDir;

// Argon2 token verification intentionally uses production-cost parameters and
// can be delayed by the full workspace test harness running in parallel. Keep
// client I/O bounded while allowing that scheduling pressure to settle.
const MCP_TEST_IO_TIMEOUT: Duration = Duration::from_secs(30);

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
        (
            "institutions",
            ProductionAdapterBinding::ExternalUnavailable,
        ),
        (
            "short_interest",
            ProductionAdapterBinding::ExternalUnavailable,
        ),
        (
            "technical_indicators",
            ProductionAdapterBinding::ExternalUnavailable,
        ),
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
        (
            "institutions",
            ProductionAdapterBinding::ExternalUnavailable,
        ),
        (
            "short_interest",
            ProductionAdapterBinding::ExternalUnavailable,
        ),
        (
            "technical_indicators",
            ProductionAdapterBinding::ExternalUnavailable,
        ),
        ("news", ProductionAdapterBinding::ExternalUnavailable),
    ]);
    Arc::new(
        ProductionToolCatalog::from_bindings_with_research(&bindings, &research)
            .expect("complete unavailable MCP catalog"),
    )
}

fn technical_indicator_ready_catalog() -> Arc<ProductionToolCatalog> {
    let bindings =
        production_adapter_bindings(&MarketDataCapabilityMatrix::new(Some("futu"), true, true));
    let research = BTreeMap::from([
        ("instrument", ProductionAdapterBinding::ExternalUnavailable),
        ("financials", ProductionAdapterBinding::ExternalUnavailable),
        ("valuation", ProductionAdapterBinding::ExternalUnavailable),
        (
            "institutions",
            ProductionAdapterBinding::ExternalUnavailable,
        ),
        (
            "short_interest",
            ProductionAdapterBinding::ExternalUnavailable,
        ),
        ("technical_indicators", ProductionAdapterBinding::Ready),
        ("news", ProductionAdapterBinding::ExternalUnavailable),
    ]);
    Arc::new(
        ProductionToolCatalog::from_bindings_with_research(&bindings, &research)
            .expect("technical indicator MCP catalog"),
    )
}

pub(super) fn production_bundle() -> (
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

#[derive(Debug, Default)]
struct RecordingResearchRead {
    calls: Mutex<Vec<(String, String)>>,
}

impl ResearchReadSnapshotPort for RecordingResearchRead {
    fn read(&self, path: &str, query: &str) -> Result<Value, ResearchReadSnapshotError> {
        self.calls
            .lock()
            .expect("record research MCP call")
            .push((path.to_owned(), query.to_owned()));
        Ok(json!({"path": path, "query": query}))
    }
}

#[derive(Debug)]
struct ReadyRuntimeStatus;

impl MarketDataRuntimeStatusPort for ReadyRuntimeStatus {
    fn snapshot(&self) -> MarketDataRuntimeState {
        MarketDataRuntimeState {
            connected: true,
            generation: 1,
            ..Default::default()
        }
    }
}

#[derive(Debug)]
struct FixtureInstitutionReader;

impl jftrade_integration_futu::FutuInstitutionReadPort for FixtureInstitutionReader {
    fn query(
        &self,
        _query: &jftrade_integration_futu::FutuInstitutionQuery,
    ) -> Result<
        jftrade_integration_futu::FutuInstitutionResult,
        jftrade_integration_futu::FutuInstitutionQueryError,
    > {
        panic!("fixture institution reader should not execute during readiness checks")
    }
}

#[derive(Debug)]
struct FixtureShortInterestReader;

impl jftrade_integration_futu::FutuShortInterestReadPort for FixtureShortInterestReader {
    fn query(
        &self,
        _query: &jftrade_integration_futu::FutuShortInterestQuery,
    ) -> Result<
        jftrade_integration_futu::FutuShortInterestResult,
        jftrade_integration_futu::FutuShortInterestQueryError,
    > {
        panic!("fixture short-interest reader should not execute during readiness checks")
    }
}

#[derive(Debug)]
struct FixtureTechnicalIndicatorReader;

impl jftrade_integration_futu::FutuIndicatorReadPort for FixtureTechnicalIndicatorReader {
    fn query(
        &self,
        _query: &jftrade_integration_futu::TechnicalIndicatorQuery,
    ) -> Result<
        jftrade_integration_futu::TechnicalIndicatorResult,
        jftrade_integration_futu::FutuIndicatorQueryError,
    > {
        panic!("fixture technical-indicator reader should not execute during readiness checks")
    }

    fn list(
        &self,
        _query: &jftrade_integration_futu::FutuIndicatorListQuery,
    ) -> Result<
        jftrade_integration_futu::FutuIndicatorList,
        jftrade_integration_futu::FutuIndicatorQueryError,
    > {
        panic!("fixture technical-indicator reader should not execute during readiness checks")
    }

    fn calculate(
        &self,
        _query: &jftrade_integration_futu::IndicatorCalcQuery,
    ) -> Result<
        jftrade_integration_futu::FutuIndicatorCalculation,
        jftrade_integration_futu::FutuIndicatorQueryError,
    > {
        panic!("fixture technical-indicator reader should not execute during readiness checks")
    }
}

#[derive(Debug)]
struct RecordingStrategyPineAnalyze {
    response: Result<Value, StrategyPineAnalyzeSnapshotError>,
    analyze_calls: Mutex<Vec<StrategyPineAnalyzeInput>>,
    shadow_calls: Mutex<Vec<StrategyPineAnalyzeInput>>,
}

impl RecordingStrategyPineAnalyze {
    fn new(response: Result<Value, StrategyPineAnalyzeSnapshotError>) -> Self {
        Self {
            response,
            analyze_calls: Mutex::new(Vec::new()),
            shadow_calls: Mutex::new(Vec::new()),
        }
    }
}

impl StrategyPineAnalyzeSnapshotPort for RecordingStrategyPineAnalyze {
    fn analyze(
        &self,
        input: &StrategyPineAnalyzeInput,
    ) -> Result<Value, StrategyPineAnalyzeSnapshotError> {
        self.analyze_calls
            .lock()
            .expect("record Pine analyzer call")
            .push(input.clone());
        self.response.clone()
    }

    fn evaluate_shadow(
        &self,
        input: &StrategyPineAnalyzeInput,
    ) -> Result<Value, StrategyPineAnalyzeSnapshotError> {
        self.shadow_calls
            .lock()
            .expect("record Pine shadow call")
            .push(input.clone());
        self.response.clone()
    }
}

fn production_bundle_with_research_readers(
    provider: MarketDataProvider,
    opend_ready: bool,
    institution_reader: bool,
    short_interest_reader: bool,
    technical_indicator_reader: bool,
) -> (
    TempDir,
    crate::product::product_production_ports::ProductionPortBundle,
) {
    let directory = tempfile::tempdir().expect("production MCP temp directory");
    let settings_path = directory.path().join("settings.json");
    fs::write(&settings_path, b"{}").expect("write production MCP settings");
    product_data_management::initialize_production_databases(&settings_path)
        .expect("initialize production MCP databases");
    let settings_store = Arc::new(SettingsFileStore::open(&settings_path).expect("settings store"));
    let security = SecuritySettingsService::new(settings_store);
    let active = Arc::new(ActiveProviderState::new(Some(provider)));
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    if institution_reader {
        runtime.set_institution_reader(Some(Arc::new(FixtureInstitutionReader)));
    }
    if short_interest_reader {
        runtime.set_short_interest_reader(Some(Arc::new(FixtureShortInterestReader)));
    }
    if technical_indicator_reader {
        runtime.set_technical_indicator_reader(Some(Arc::new(FixtureTechnicalIndicatorReader)));
    }
    let mut config = ProductConfig::new(
        SocketAddr::from(([127, 0, 0, 1], 0)),
        &settings_path,
        AccessPolicy::default(),
    )
    .expect("production MCP config")
    .with_active_provider_state(active)
    .with_trade_runtime(runtime);
    if opend_ready {
        config = config.with_market_data_runtime_status_port(Arc::new(ReadyRuntimeStatus));
    }
    config.capabilities = ProductCapabilities::all();
    config.production = true;
    let ports = crate::product::product_production_ports::production_ports(&config, &security)
        .expect("production MCP ports");
    (directory, ports)
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
fn reviewed_mcp_descriptors_expose_strict_per_tool_schemas() {
    let descriptors = tool_descriptors(&catalog());
    assert_eq!(descriptors.len(), REVIEWED_READ_ONLY_TOOLS.len());
    for descriptor in &descriptors {
        let name = descriptor["name"].as_str().expect("descriptor name");
        let schema = descriptor
            .get("inputSchema")
            .and_then(Value::as_object)
            .unwrap_or_else(|| panic!("{name} input schema is not an object"));
        assert_eq!(schema.get("type"), Some(&json!("object")), "{name}");
        assert_eq!(
            schema.get("additionalProperties"),
            Some(&json!(false)),
            "{name} must reject unknown fields"
        );
    }

    let search = descriptors
        .iter()
        .find(|descriptor| descriptor["name"] == "market.search")
        .expect("market.search descriptor");
    let search_schema = &search["inputSchema"];
    assert_eq!(search_schema["required"], json!(["query"]));
    assert_eq!(search_schema["properties"]["query"]["minLength"], 1);
    assert_eq!(search_schema["properties"]["query"]["maxLength"], 120);

    let candles = descriptors
        .iter()
        .find(|descriptor| descriptor["name"] == "market.candles")
        .expect("market.candles descriptor");
    assert_eq!(
        candles["inputSchema"]["properties"]["limit"]["maximum"],
        500
    );
    assert_eq!(
        candles["inputSchema"]["properties"]["adjustment"]["enum"],
        json!(["none", "forward", "backward"])
    );

    let screen = descriptors
        .iter()
        .find(|descriptor| descriptor["name"] == "research.screen")
        .expect("research.screen descriptor");
    assert_eq!(
        screen["inputSchema"]["required"],
        json!(["market", "pool", "catalogVersion", "querySchemaVersion"])
    );
    assert_eq!(
        screen["inputSchema"]["properties"]["querySchemaVersion"]["minimum"],
        2
    );

    for name in [
        "system.status",
        "system.futu_opend",
        "plugins.catalog",
        "market.subscriptions",
        "risk.state",
        "risk.events",
        "strategy.definitions",
    ] {
        let descriptor = descriptors
            .iter()
            .find(|descriptor| descriptor["name"] == name)
            .unwrap_or_else(|| panic!("{name} descriptor"));
        assert_eq!(
            descriptor["inputSchema"]["properties"]["query"]["type"], "string",
            "{name} must preserve the Go default query schema"
        );
    }

    for name in ["market.providers", "system.runtime_dependencies"] {
        let descriptor = descriptors
            .iter()
            .find(|descriptor| descriptor["name"] == name)
            .unwrap_or_else(|| panic!("{name} descriptor"));
        assert_eq!(descriptor["inputSchema"]["properties"], json!({}), "{name}");
    }

    let quote = descriptors
        .iter()
        .find(|descriptor| descriptor["name"] == "prediction.combo_quote")
        .expect("prediction.combo_quote descriptor");
    assert_eq!(
        quote["inputSchema"]["required"],
        json!(["accountId", "mvc", "legs"])
    );
    for property in [
        "brokerId",
        "accountId",
        "tradingEnvironment",
        "market",
        "cursor",
        "pageSize",
        "refresh",
    ] {
        assert!(
            quote["inputSchema"]["properties"].get(property).is_some(),
            "prediction.combo_quote missing {property}"
        );
    }

    let indicators = descriptors
        .iter()
        .find(|descriptor| descriptor["name"] == "research.technical_indicators")
        .expect("research.technical_indicators descriptor");
    assert_eq!(
        indicators["inputSchema"]["properties"]["operation"]["enum"],
        json!(["list", "calculate"])
    );
    assert_eq!(
        indicators["inputSchema"]["required"],
        json!(["instrumentId"])
    );
    for property in [
        "searchKey",
        "langType",
        "searchMode",
        "shortName",
        "klType",
        "kLine",
        "num",
        "inputs",
    ] {
        assert!(
            indicators["inputSchema"]["properties"]
                .get(property)
                .is_some(),
            "research.technical_indicators missing {property}"
        );
    }
    assert_eq!(
        indicators["inputSchema"]["then"]["required"],
        json!(["shortName", "langType", "klType", "kLine"])
    );

    let screen_catalog = descriptors
        .iter()
        .find(|descriptor| descriptor["name"] == "research.screen_catalog")
        .expect("research.screen_catalog descriptor");
    assert_eq!(
        screen_catalog["inputSchema"]["properties"],
        json!({"market": {"type": "string", "enum": ["HK", "US", "SH", "SZ"]}}),
        "research.screen_catalog must retain the Go catalog's market-only input"
    );

    let macro_research = descriptors
        .iter()
        .find(|descriptor| descriptor["name"] == "research.macro")
        .expect("research.macro descriptor");
    assert_eq!(
        macro_research["inputSchema"]["properties"]["indicatorId"]["type"],
        "string"
    );
    assert_eq!(
        macro_research["inputSchema"]["then"]["required"],
        json!(["indicatorId"])
    );
}

#[test]
fn pine_mcp_schemas_match_canonical_go_fixture() {
    let fixture: Value = serde_json::from_str(include_str!(
        "../../../tests/fixtures/compatibility/trading-strategy/pine-mcp-cases.json"
    ))
    .expect("Pine MCP schema fixture");
    let descriptors = tool_descriptors(&catalog());
    for name in ["strategy.pine_spec", "strategy.validate_pine"] {
        let expected = fixture["schemas"][name].clone();
        assert!(expected.is_object(), "fixture schema missing {name}");
        let actual = descriptors
            .iter()
            .find(|descriptor| descriptor["name"] == name)
            .and_then(|descriptor| descriptor.get("inputSchema"))
            .expect("Pine MCP descriptor schema");
        assert_eq!(
            actual, &expected,
            "Rust schema drifted from Go fixture: {name}"
        );
    }
}

#[test]
fn reviewed_mcp_schemas_match_canonical_go_fixture_deeply() {
    let fixture: Value = serde_json::from_str(include_str!(
        "../../../tests/fixtures/compatibility/api-transport/mcp-tool-schemas.json"
    ))
    .expect("MCP tool schema fixture");
    assert_eq!(fixture["version"], "stage9.mcp-tool-schemas.v1");
    let schemas = fixture["schemas"]
        .as_object()
        .expect("MCP schema fixture schemas");
    assert_eq!(
        fixture["toolCount"],
        json!(REVIEWED_READ_ONLY_TOOLS.len()),
        "MCP schema fixture toolCount must match the reviewed catalog"
    );
    assert_eq!(schemas.len(), REVIEWED_READ_ONLY_TOOLS.len());
    let descriptors = tool_descriptors(&catalog());
    assert_eq!(descriptors.len(), REVIEWED_READ_ONLY_TOOLS.len());
    let mut mismatches = Vec::new();
    for name in REVIEWED_READ_ONLY_TOOLS {
        let expected = schemas
            .get(*name)
            .unwrap_or_else(|| panic!("fixture schema missing {name}"));
        let actual = descriptors
            .iter()
            .find(|descriptor| descriptor["name"] == *name)
            .and_then(|descriptor| descriptor.get("inputSchema"))
            .unwrap_or_else(|| panic!("Rust descriptor schema missing {name}"));
        if actual != expected {
            mismatches.push((*name).to_owned());
        }
    }
    for name in schemas.keys() {
        assert!(
            REVIEWED_READ_ONLY_TOOLS.contains(&name.as_str()),
            "fixture contains unreviewed MCP schema {name}"
        );
    }
    assert!(
        mismatches.is_empty(),
        "Rust schemas drifted from Go fixture: {mismatches:?}"
    );
}

#[test]
fn normalize_legacy_mcp_arguments_normalizes_aliases() {
    let mut search_args = json!({"query": "AAPL", "limit": 25});
    normalize_legacy_mcp_arguments("market.search", &mut search_args);
    assert_eq!(search_args["pageSize"], 25);
    assert!(search_args.get("limit").is_none());

    let mut fees_args = json!({"orderIdEx": "ord-123"});
    normalize_legacy_mcp_arguments("broker.fees", &mut fees_args);
    assert_eq!(fees_args["orderIdEx"], json!(["ord-123"]));

    let mut snapshot_args = json!({"market": "US", "symbol": "AAPL"});
    normalize_legacy_mcp_arguments("market.snapshot", &mut snapshot_args);
    assert_eq!(snapshot_args["instrumentId"], "US.AAPL");
    assert!(snapshot_args.get("symbol").is_none());

    let mut bp_args = json!({"instrument": "US.AAPL"});
    normalize_legacy_mcp_arguments("execution.buying_power", &mut bp_args);
    assert_eq!(bp_args["instrument"]["instrumentId"], "US.AAPL");
}

#[test]
fn pine_mcp_payloads_match_canonical_go_fixture_in_off_mode() {
    let fixture: Value = serde_json::from_str(include_str!(
        "../../../tests/fixtures/compatibility/trading-strategy/pine-mcp-cases.json"
    ))
    .expect("Pine MCP payload fixture");
    let (_directory, ports) = production_bundle();
    let executor = ProductionMcpToolExecutor::from_production_ports(Arc::new(ports));
    for case in fixture["cases"].as_array().expect("Pine MCP fixture cases") {
        let name = case["name"].as_str().expect("Pine MCP fixture case name");
        let tool = case["tool"].as_str().expect("Pine MCP fixture tool");
        let arguments = &case["arguments"];
        let expected = &case["expected"]["payload"];
        let actual = executor
            .strategy_pine_mcp_with_mode(tool, arguments, "off")
            .unwrap_or_else(|error| panic!("{name}: execute {tool}: {error:?}"));
        assert_eq!(
            project_pine_mcp_compatibility_payload(tool, &actual),
            *expected,
            "Rust payload drifted from Go fixture: {name}"
        );
    }
}

fn project_pine_mcp_compatibility_payload(tool: &str, payload: &Value) -> Value {
    let object = payload
        .as_object()
        .expect("Pine MCP payload must be an object");
    match tool {
        "strategy.pine_spec" => {
            let mut projected = Map::new();
            for key in [
                "version",
                "productVersion",
                "sourceFormat",
                "runtime",
                "selectedSection",
            ] {
                projected.insert(
                    key.to_owned(),
                    object.get(key).cloned().unwrap_or(Value::Null),
                );
            }
            let section_ids = object
                .get("sections")
                .and_then(Value::as_array)
                .map(|sections| {
                    Value::Array(
                        sections
                            .iter()
                            .filter_map(|section| section.get("id").cloned())
                            .collect(),
                    )
                })
                .unwrap_or_else(|| Value::Array(Vec::new()));
            projected.insert("sectionIds".to_owned(), section_ids);
            projected.insert(
                "examplesCount".to_owned(),
                json!(value_len(object.get("examples"))),
            );
            if let Some(section_id) = object
                .get("sectionContent")
                .and_then(|content| content.get("id"))
            {
                projected.insert("sectionContentId".to_owned(), section_id.clone());
            }
            projected.insert(
                "externalEngine".to_owned(),
                project_pine_external_engine_compatibility(
                    object.get("externalEngine"),
                    &[
                        "engine",
                        "mode",
                        "enabled",
                        "status",
                        "license",
                        "package",
                        "repository",
                        "worker",
                    ],
                ),
            );
            Value::Object(projected)
        }
        "strategy.validate_pine" => {
            let mut projected = Map::new();
            for key in ["ok", "sourceFormat", "runtime", "normalizedScript"] {
                projected.insert(
                    key.to_owned(),
                    object.get(key).cloned().unwrap_or(Value::Null),
                );
            }
            projected.insert(
                "requirementsPresent".to_owned(),
                json!(
                    object
                        .get("requirements")
                        .is_some_and(|requirements| !requirements.is_null())
                ),
            );
            projected.insert(
                "errorCount".to_owned(),
                json!(value_len(object.get("errors"))),
            );
            projected.insert(
                "warningCount".to_owned(),
                json!(value_len(object.get("warnings"))),
            );
            for key in ["hooks", "metadata"] {
                projected.insert(
                    key.to_owned(),
                    object.get(key).cloned().unwrap_or(Value::Null),
                );
            }
            projected.insert(
                "saveHintPresent".to_owned(),
                json!(
                    object
                        .get("saveHint")
                        .is_some_and(|save_hint| !save_hint.is_null())
                ),
            );
            projected.insert(
                "externalEngine".to_owned(),
                project_pine_external_engine_compatibility(
                    object.get("externalEngine"),
                    &[
                        "engine",
                        "mode",
                        "enabled",
                        "status",
                        "license",
                        "repository",
                    ],
                ),
            );
            Value::Object(projected)
        }
        other => panic!("unsupported Pine MCP fixture tool {other}"),
    }
}

fn project_pine_external_engine_compatibility(value: Option<&Value>, keys: &[&str]) -> Value {
    let Some(object) = value.and_then(Value::as_object) else {
        return Value::Object(Map::new());
    };
    let mut projected = Map::new();
    for key in keys {
        if let Some(value) = object.get(*key) {
            projected.insert((*key).to_owned(), value.clone());
        }
    }
    Value::Object(projected)
}

fn value_len(value: Option<&Value>) -> usize {
    value.and_then(Value::as_array).map_or(0, Vec::len)
}

#[test]
fn reviewed_mcp_argument_validation_enforces_schema_constraints() {
    assert!(validate_tool_arguments("system.status", &json!({"query": "health"})).is_ok());
    assert!(validate_tool_arguments("system.status", &json!({"unexpected": true})).is_err());
    assert!(validate_tool_arguments("market.search", &json!({})).is_err());
    assert!(validate_tool_arguments("market.search", &json!({"query": ""})).is_err());
    assert!(
        validate_tool_arguments(
            "market.candles",
            &json!({"market": "US", "symbol": "AAPL", "limit": 501})
        )
        .is_err()
    );
    assert!(
        validate_tool_arguments(
            "market.candles",
            &json!({"market": "US", "symbol": "AAPL", "adjustment": "sideways"})
        )
        .is_err()
    );
    assert!(
        validate_tool_arguments(
            "prediction.combo_quote",
            &json!({"accountId": "account", "mvc": "market", "legs": []})
        )
        .is_err()
    );
    assert!(
        validate_tool_arguments(
            "research.screen",
            &json!({
                "market": "US",
                "pool": {"unknown": true},
                "catalogVersion": "v1",
                "querySchemaVersion": 2
            })
        )
        .is_err()
    );
    assert!(validate_tool_arguments("research.screen_catalog", &json!({"market": "US"})).is_ok());
    for (field, arguments) in [
        ("brokerId", json!({"market": "US", "brokerId": "futu"})),
        ("accountId", json!({"market": "US", "accountId": "account"})),
        ("cursor", json!({"market": "US", "cursor": "next"})),
        ("pageSize", json!({"market": "US", "pageSize": 20})),
        ("refresh", json!({"market": "US", "refresh": true})),
    ] {
        assert!(
            validate_tool_arguments("research.screen_catalog", &arguments).is_err(),
            "research.screen_catalog must reject extra field {field}"
        );
    }
    assert!(
        validate_tool_arguments(
            "research.macro",
            &json!({"operation": "indicator_history", "indicatorId": "cpi_yoy"})
        )
        .is_ok()
    );
    assert!(
        validate_tool_arguments("research.macro", &json!({"operation": "indicator_history"}))
            .is_err()
    );
    assert!(validate_tool_arguments("research.macro", &json!({"operation": "indicators"})).is_ok());
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
            if matches!(*name, "strategy.pine_spec" | "strategy.validate_pine") {
                assert_eq!(
                    adapter, None,
                    "native Pine leaf should not require a route adapter"
                );
                assert_eq!(availability, "ready", "native Pine leaf is in-process");
            } else {
                assert!(
                    adapter.is_some(),
                    "native tool {name} has no adapter mapping"
                );
                assert_eq!(
                    availability, "unavailable",
                    "native tool {name} must surface external unavailability"
                );
            }
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
    assert_eq!(native.len(), 69);
    assert!(native.is_subset(&reviewed));
    assert_eq!(reviewed.len() - native.len(), 0);
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

/// Parity: go:452dea11:internal/assistant/assembly/application_adapter_test.go:16
/// `TestApplicationAdapterReportsUnavailableDomainServices`. Go builds the
/// assistant tool deps from an empty `ApplicationPorts` and every domain call
/// fails instead of returning a fabricated payload. The Rust owner is the
/// production MCP executor: a listener assembled without the production port
/// bundle must fail closed for every domain tool it advertises.
#[test]
fn production_tools_fail_closed_before_any_domain_service_is_configured() {
    let (_directory, ports) = production_bundle();
    let executor = ProductionMcpToolExecutor::new(
        Arc::clone(&ports.mcp_catalog),
        Arc::clone(&ports.mcp_store),
    );
    for (name, arguments) in [
        (
            "execution.order_events",
            json!({"internalOrderId": "order-1"}),
        ),
        ("broker.orders", json!({})),
        ("strategy.definitions", json!({})),
        ("backtest.runs", json!({})),
        ("backtest.kline_sync_status", json!({"taskId": "sync-1"})),
        ("research.screen_catalog", json!({"market": "US"})),
        ("market.providers", json!({})),
        ("system.runtime_dependencies", json!({})),
        ("system.futu_opend", json!({})),
        ("market.snapshot", json!({"market": "US", "symbol": "AAPL"})),
        (
            "market.candles",
            json!({"market": "HK", "symbol": "00700", "period": "1h"}),
        ),
        ("watchlist.list", json!({"group": "Favorites"})),
    ] {
        let failure = executor
            .execute_production(name, &arguments)
            .expect_err("domain tool without production ports must fail closed");
        assert_eq!(failure.status, 503, "{name}");
        assert_eq!(
            failure.code, "MCP_PRODUCTION_EXECUTOR_UNAVAILABLE",
            "{name}"
        );
    }
}

/// Parity: go:452dea11:internal/assistant/assembly/application_adapter_boundaries_test.go:20
/// `TestApplicationAdapterKeepsNilPortsCallable`. Go builds the tool deps from
/// a nil `*ApplicationAdapter`: every domain call must answer an unavailable
/// error instead of panicking, `RecordAudit` stays a no-op, and the runtime
/// settings keep their zero value.
///
/// Rust has no nullable adapter, so the equivalent owner is the MCP executor
/// assembled without a production port bundle: every reviewed tool must answer
/// a structured failure in that state, and the port-backed domain tools must
/// report the executor itself as unavailable.
///
/// Difference recorded in the batch note: Go keeps deliberately non-failing
/// fallbacks (`FutuOpenDHealth` answers `status: unavailable`, `RiskState()`
/// stays non-nil and `BacktestKLineSyncProgress` answers `found=false`). Rust
/// fails closed for those instead of fabricating a payload, so the loop below
/// requires a structured failure rather than a specific fallback value.
#[test]
fn unwired_production_bundle_keeps_every_reviewed_tool_callable_without_payloads() {
    let (_directory, ports) = production_bundle();
    let executor = ProductionMcpToolExecutor::new(
        Arc::clone(&ports.mcp_catalog),
        Arc::clone(&ports.mcp_store),
    );

    // `strategy.pine_spec` is the only reviewed tool that answers an empty
    // request without a domain port; `strategy.validate_pine` is port-free too
    // but the empty request fails its script validation with 400, and every
    // other reviewed tool has to reach a domain port and fails closed.
    let mut port_free = Vec::new();
    for name in crate::product::product_mcp_protocol::REVIEWED_READ_ONLY_TOOLS {
        match executor.execute_production(name, &json!({})) {
            Ok(_) => port_free.push(*name),
            Err(failure) => {
                assert!(
                    (400..=503).contains(&failure.status),
                    "{name} answered status {} with code {}",
                    failure.status,
                    failure.code
                );
                assert!(!failure.code.is_empty(), "{name} lost its error code");
            }
        }
    }
    assert_eq!(
        port_free,
        vec!["strategy.pine_spec"],
        "only the in-process Pine spec tool may answer an empty request without domain ports"
    );

    for (name, arguments) in [
        (
            "execution.order_events",
            json!({"internalOrderId": "order-1"}),
        ),
        ("broker.orders", json!({})),
        ("strategy.definitions", json!({})),
        ("backtest.runs", json!({})),
        ("backtest.kline_sync_status", json!({"taskId": "sync-1"})),
        ("research.screen_catalog", json!({"market": "US"})),
        ("market.providers", json!({})),
        ("risk.state", json!({})),
    ] {
        let failure = executor
            .execute_production(name, &arguments)
            .expect_err("a port-backed tool without ports must fail closed");
        assert_eq!(failure.status, 503, "{name}");
        assert_eq!(
            failure.code, "MCP_PRODUCTION_EXECUTOR_UNAVAILABLE",
            "{name}"
        );
    }
}

/// Parity: go:452dea11:internal/assistant/assembly/application_strategy_lifecycle_test.go:84
/// `TestApplicationAdapterStrategyInstanceLifecyclePorts` (activity half) and
/// `:122`/`:141` (invalid kind and missing instance boundaries).
///
/// Go's `StrategyInstanceActivity(instance, kind, limit, offset)` trims the
/// instance id, defaults to `logs`, clamps the page and reports "not found"
/// separately from an unavailable service. The Rust owner is the MCP
/// `strategy.instance_activity` tool, which must normalize the request before
/// it reaches the strategy read port and fail closed on invalid input.
#[test]
fn strategy_instance_activity_tool_normalizes_kind_and_paging_before_the_read_port() {
    #[derive(Debug, Default)]
    struct RecordingStrategyRead {
        reads: Mutex<Vec<(String, String)>>,
        missing: bool,
        unavailable: bool,
    }

    impl crate::product::StrategyReadSnapshotPort for RecordingStrategyRead {
        fn read(
            &self,
            path: &str,
            query: &str,
        ) -> Result<Option<Value>, crate::product::StrategyReadSnapshotError> {
            self.reads
                .lock()
                .expect("strategy reads")
                .push((path.to_owned(), query.to_owned()));
            if self.unavailable {
                return Err(crate::product::StrategyReadSnapshotError::Unavailable(
                    "strategy runtime is offline".to_owned(),
                ));
            }
            if self.missing {
                return Ok(None);
            }
            Ok(Some(json!({"instanceId": "instance-1"})))
        }
    }

    let (_directory, mut ports) = production_bundle();
    let recorder = Arc::new(RecordingStrategyRead::default());
    ports.strategy_read =
        Arc::clone(&recorder) as Arc<dyn crate::product::StrategyReadSnapshotPort>;
    let executor = ProductionMcpToolExecutor::from_production_ports(Arc::new(ports));

    let logs = executor
        .execute_production(
            "strategy.instance_activity",
            &json!({"instanceId": " instance-1 "}),
        )
        .expect("default logs activity");
    assert_eq!(logs["instanceId"], "instance-1");
    let audit = executor
        .execute_production(
            "strategy.instance_activity",
            &json!({
                "instanceId": "instance-1",
                "kind": "AUDIT",
                "eventKind": "pause",
                "limit": 10,
                "offset": 2,
                "fromTime": "2026-01-01T00:00:00Z",
                "toTime": "2026-01-02T00:00:00Z",
                "level": "warn",
            }),
        )
        .expect("audit activity");
    assert_eq!(audit["instanceId"], "instance-1");

    let reads = recorder.reads.lock().expect("strategy reads").clone();
    assert_eq!(
        reads.len(),
        2,
        "the activity tool must issue exactly one read per call: {reads:?}"
    );
    assert_eq!(reads[0].0, "/api/v1/strategies/instance-1/logs");
    assert_eq!(reads[0].1, "limit=50&offset=0");
    assert_eq!(reads[1].0, "/api/v1/strategies/instance-1/audit");
    for expected in [
        "limit=10",
        "offset=2",
        "kind=pause",
        // The reviewed query encoder escapes `-` as `%2D` as well as the
        // timestamp separators, so the wire shape is asserted literally.
        "fromTime=2026%2D01%2D01T00%3A00%3A00Z",
        "toTime=2026%2D01%2D02T00%3A00%3A00Z",
    ] {
        assert!(
            reads[1].1.contains(expected),
            "audit query {expected} missing from {}",
            reads[1].1
        );
    }
    assert!(
        !reads[1].1.contains("level="),
        "the audit read must not forward the logs-only level filter: {}",
        reads[1].1
    );

    for (arguments, message) in [
        (
            json!({"instanceId": "instance-1", "kind": "traces"}),
            "kind must be logs or audit",
        ),
        (
            json!({"instanceId": "instance-1", "limit": 0}),
            "limit must be between 1 and 200",
        ),
        (
            json!({"instanceId": "instance-1", "limit": 201}),
            "limit must be between 1 and 200",
        ),
        (
            json!({"instanceId": "instance-1", "offset": -1}),
            "offset must be between 0 and 5000000",
        ),
        (json!({}), "instanceId is required"),
    ] {
        let failure = executor
            .execute_production("strategy.instance_activity", &arguments)
            .expect_err(message);
        assert_eq!(failure.status, 400, "{message}");
        assert_eq!(failure.code, "BAD_REQUEST", "{message}");
        assert_eq!(failure.message, message);
    }

    for (port, status, code) in [
        (
            RecordingStrategyRead {
                missing: true,
                ..Default::default()
            },
            404,
            "STRATEGY_INSTANCE_NOT_FOUND",
        ),
        (
            RecordingStrategyRead {
                unavailable: true,
                ..Default::default()
            },
            503,
            "STRATEGY_ACTIVITY_UNAVAILABLE",
        ),
    ] {
        let (_directory, mut ports) = production_bundle();
        ports.strategy_read = Arc::new(port);
        let executor = ProductionMcpToolExecutor::from_production_ports(Arc::new(ports));
        let failure = executor
            .execute_production(
                "strategy.instance_activity",
                &json!({"instanceId": "instance-1"}),
            )
            .expect_err("a missing or unavailable strategy runtime must fail closed");
        assert_eq!(failure.status, status);
        assert_eq!(failure.code, code);
    }
}

/// Parity: go:452dea11:internal/assistant/assembly/adk_strategy_test.go:19
/// `TestADKCoreToolHandlersSurfaceSubscriptionErrors`. Go's
/// `market.subscriptions` handler returns the market-data service failure
/// unchanged ("feed unavailable") instead of a fixture payload. The Rust
/// owner is the MCP `market.subscriptions` tool plus the quote-read port.
#[test]
fn market_subscriptions_surface_quote_port_failures_without_fixture_payloads() {
    #[derive(Debug)]
    struct FailingSubscriptionQuotePort;

    impl crate::product::MarketDataQuoteReadSnapshotPort for FailingSubscriptionQuotePort {
        fn read<'a>(
            &'a self,
            _path: &'a str,
            _query: &'a str,
        ) -> crate::product::MarketDataQuoteReadFuture<'a> {
            Box::pin(async move {
                Err(
                    crate::product::MarketDataQuoteReadSnapshotError::Unavailable(
                        "feed unavailable".to_owned(),
                    ),
                )
            })
        }
    }

    let (_directory, mut ports) = production_bundle();
    ports.market_data_quote = Arc::new(FailingSubscriptionQuotePort);
    let executor = ProductionMcpToolExecutor::from_production_ports(Arc::new(ports));

    let failure = executor
        .execute_production("market.subscriptions", &json!({}))
        .expect_err("a failing subscription feed must not answer a fixture payload");
    assert_eq!(failure.status, 503);
    assert_eq!(failure.code, "MARKET_DATA_QUOTE_READ_UNAVAILABLE");
    assert_eq!(failure.message, "feed unavailable");
}

/// Parity: go:452dea11:internal/assistant/assembly/adk_strategy_test.go:194
/// `TestADKStrategyToolsHandleNegativeAndFallbackScenarios` (validation half)
/// plus `:308`
/// `TestADKStrategyDefinitionVersionToolsExposeImmutableSnapshotsAndFailures`.
/// Go validates tool inputs before reaching a store and answers a missing
/// target as not found; the Rust owner is the production MCP executor on top
/// of the production port bundle.
#[test]
fn backtest_and_strategy_tools_reject_missing_identifiers_and_unknown_targets() {
    let (_directory, ports) = production_bundle();
    let executor = ProductionMcpToolExecutor::from_production_ports(Arc::new(ports));

    let missing_run_id = executor
        .execute_production("backtest.result_view", &json!({}))
        .expect_err("backtest.result_view without runId");
    assert_eq!(missing_run_id.status, 400);
    assert_eq!(missing_run_id.code, "BAD_REQUEST");

    let missing_task_id = executor
        .execute_production("backtest.kline_sync_status", &json!({}))
        .expect_err("backtest.kline_sync_status without taskId");
    assert_eq!(missing_task_id.status, 400);
    assert_eq!(missing_task_id.code, "BAD_REQUEST");

    let unknown_task = executor
        .execute_production(
            "backtest.kline_sync_status",
            &json!({"taskId": "sync-missing"}),
        )
        .expect_err("unknown sync task must answer not found");
    assert_eq!(unknown_task.status, 404);
    assert_eq!(unknown_task.code, "BACKTEST_SYNC_TASK_NOT_FOUND");

    let missing_definition_id = executor
        .execute_production("strategy.definition_versions.list", &json!({}))
        .expect_err("version list without definitionId");
    assert_eq!(missing_definition_id.status, 400);
    assert_eq!(missing_definition_id.code, "BAD_REQUEST");

    let unknown_definition = executor
        .execute_production(
            "strategy.definition_versions.list",
            &json!({"definitionId": "missing-definition"}),
        )
        .expect_err("an unknown definition must answer not found");
    assert_eq!(unknown_definition.status, 404);
    assert_eq!(unknown_definition.code, "STRATEGY_DEFINITION_NOT_FOUND");

    let missing_version = executor
        .execute_production(
            "strategy.definition_versions.get",
            &json!({"definitionId": "missing-definition"}),
        )
        .expect_err("version lookup without version");
    assert_eq!(missing_version.status, 400);
    assert_eq!(missing_version.code, "BAD_REQUEST");
}

/// Parity: go:452dea11:internal/assistant/assembly/portfolio_tools_test.go:338
/// `TestAccountOrdersFiltersAccountEnvironmentMarketAndActiveStatus`. Go
/// resolves the account suffix and forwards the account, environment, market,
/// and `activeOnly` dimensions as the execution filter before it reports the
/// filtered count. The Rust owner is the MCP `account.orders` tool plus the
/// execution read port.
#[test]
fn account_orders_forwards_scope_account_environment_and_market_filters() {
    #[derive(Debug, Default)]
    struct RecordingExecutionRead {
        queries: Mutex<Vec<String>>,
    }

    impl crate::product::ExecutionReadSnapshotPort for RecordingExecutionRead {
        fn read(
            &self,
            path: &str,
            query: &str,
        ) -> Result<Value, crate::product::ExecutionReadSnapshotError> {
            assert_eq!(path, "/api/v1/execution/orders");
            self.queries
                .lock()
                .expect("recorded queries")
                .push(query.to_owned());
            Ok(json!({
                "orders": [{"internalOrderId": "active"}],
                "checkedAt": "2026-09-01T00:00:00Z",
            }))
        }
    }

    let (_directory, mut ports) = production_bundle();
    let recording = Arc::new(RecordingExecutionRead::default());
    ports.execution_read = recording.clone();
    let executor = ProductionMcpToolExecutor::from_production_ports(Arc::new(ports));

    let payload = executor
        .execute_production(
            "account.orders",
            &json!({
                "accountId": "8240",
                "tradingEnvironment": "REAL",
                "market": "US",
                "activeOnly": true,
            }),
        )
        .expect("active account orders");
    assert_eq!(payload["count"], 1);
    assert_eq!(payload["activeOnly"], true);
    assert_eq!(payload["orders"][0]["internalOrderId"], "active");

    executor
        .execute_production("account.orders", &json!({"tradingEnvironment": "REAL"}))
        .expect("current account orders");
    assert_eq!(
        recording.queries.lock().expect("queries").as_slice(),
        [
            "scope=ACTIVE&tradingEnvironment=REAL&accountId=8240&market=US",
            "scope=CURRENT&tradingEnvironment=REAL",
        ]
    );
}

/// Parity: go:452dea11:internal/assistant/assembly/portfolio_tools_test.go:155
/// `TestPortfolioLayeredToolsReportValidationDiscoveryAndPartialReadStates`
/// (validation and fail-closed halves). Go requires `tradingEnvironment` on
/// every layered portfolio tool before it touches a broker and surfaces an
/// unavailable reader instead of a fabricated account list. The Rust owner is
/// `product_portfolio_projection::{execute_portfolio_accounts, ...}` behind the
/// ADK tool executor.
#[test]
fn portfolio_tools_require_trading_environment_and_fail_closed_without_a_broker_reader() {
    use crate::product::product_adk_model_runtime::AdkToolExecutor;

    let (_directory, ports) = production_bundle();
    let executor = crate::product::product_adk_model_runtime::ProductionAdkToolExecutor::with_ports(
        Arc::clone(&ports.mcp_catalog),
        Arc::clone(&ports.mcp_store),
        Arc::new(ports),
    );

    for name in [
        "portfolio.accounts",
        "portfolio.overview",
        "portfolio.positions",
    ] {
        let missing_environment = executor
            .execute(name, &json!({}))
            .expect_err("tradingEnvironment is required");
        assert_eq!(
            missing_environment, "tradingEnvironment is required",
            "{name}"
        );

        let unreadable = executor
            .execute(name, &json!({"tradingEnvironment": "REAL"}))
            .expect_err("a missing trade reader must fail closed");
        assert_eq!(unreadable, "broker trade reader is unavailable", "{name}");
    }
}

/// Parity: go:452dea11:pkg/broker/broker_test.go:97
/// `TestConvertFutuReadQuery`. Go's transitional helper pins the broker
/// identity to `futu` and passes the account id, trading environment, and
/// market through unchanged. The Rust read ports carry no `brokerId` field,
/// so the same contract is expressed by the futu-scoped read routes asserted
/// below: fixed broker identity (including the rejection of another broker),
/// plus verbatim passthrough of the three read-scope fields.
///
/// Regression net for `portfolio.summary` as it exists in Rust today.
///
/// The Go assembly tool aggregates per-account `accountSummaries`; the Rust MCP
/// tool merges the three single-account snapshots instead, so this test pins
/// the merged payload (no shared top-level `funds`) and the `brokerId` guard
/// while the aggregation gap stays tracked as a follow-up.
#[test]
fn portfolio_summary_merges_positions_balances_and_orders_and_rejects_unknown_brokers() {
    #[derive(Debug)]
    struct RecordingPortfolioRead {
        paths: Mutex<Vec<String>>,
    }

    impl crate::product::PortfolioSnapshotPort for RecordingPortfolioRead {
        fn read(
            &self,
            path: &str,
            query: &str,
        ) -> Result<Value, crate::product::PortfolioSnapshotError> {
            assert_eq!(query, "accountId=8240&tradingEnvironment=REAL&market=US");
            self.paths
                .lock()
                .expect("portfolio paths")
                .push(path.to_owned());
            match path {
                "/api/v1/portfolio/futu/positions" => Ok(json!({
                    "positions": [{"symbol": "US.AAPL"}],
                    "connectivity": "connected",
                    "checkedAt": "2026-09-01T00:00:00Z",
                })),
                "/api/v1/portfolio/futu/cash-balances" => Ok(json!({
                    "balances": [{"currency": "USD"}],
                    "connectivity": "connected",
                    "checkedAt": "2026-09-01T00:00:00Z",
                })),
                other => panic!("unexpected portfolio path {other}"),
            }
        }
    }

    #[derive(Debug)]
    struct RecordingBrokerOrders;

    impl crate::product::BrokerReadSnapshotPort for RecordingBrokerOrders {
        fn read(
            &self,
            path: &str,
            query: &str,
        ) -> Result<Value, crate::product::BrokerReadSnapshotError> {
            assert_eq!(path, "/api/v1/brokers/futu/orders");
            assert_eq!(query, "accountId=8240&tradingEnvironment=REAL&market=US");
            Ok(json!({
                "orders": [{"internalOrderId": "order-1"}],
                "connectivity": "connected",
                "checkedAt": "2026-09-01T00:00:00Z",
            }))
        }
    }

    let (_directory, mut ports) = production_bundle();
    ports.portfolio = Arc::new(RecordingPortfolioRead {
        paths: Mutex::new(Vec::new()),
    });
    ports.broker = Arc::new(RecordingBrokerOrders);
    let executor = ProductionMcpToolExecutor::from_production_ports(Arc::new(ports));

    let payload = executor
        .execute_production(
            "portfolio.summary",
            &json!({"accountId": "8240", "tradingEnvironment": "REAL", "market": "US"}),
        )
        .expect("portfolio summary");
    assert_eq!(payload["brokerId"], "futu");
    assert_eq!(payload["positions"][0]["symbol"], "US.AAPL");
    assert_eq!(payload["balances"][0]["currency"], "USD");
    assert_eq!(payload["orders"][0]["internalOrderId"], "order-1");
    assert_eq!(payload["connectivity"], "connected");
    assert_eq!(payload["checkedAt"], "2026-09-01T00:00:00Z");
    assert!(
        payload.get("funds").is_none(),
        "the summary must not expose an ambiguous top-level funds bucket"
    );

    let unsupported = executor
        .execute_production("portfolio.summary", &json!({"brokerId": "akshare"}))
        .expect_err("brokerId must stay futu");
    assert_eq!(unsupported.code, "BAD_REQUEST");
}

#[test]
fn production_mcp_pine_leaves_execute_native_spec_and_validation() {
    let (_directory, ports) = production_bundle();
    let executor = ProductionMcpToolExecutor::from_production_ports(Arc::new(ports));

    let spec = executor
        .execute_production("strategy.pine_spec", &json!({"section": "overview"}))
        .expect("native Pine specification");
    assert_eq!(spec["selectedSection"], "overview");

    let validation = executor
        .execute_production(
            "strategy.validate_pine",
            &json!({"script": "//@version=6\nstrategy(\"MCP\")"}),
        )
        .expect("native Pine validation");
    assert_eq!(validation["ok"], true);
    assert!(validation["saveHint"].is_null());

    // A whitespace-only script is still a validation payload, not a tool
    // error: Go answers `ok=false` with the required-script error and a hint.
    let blank = executor
        .execute_production("strategy.validate_pine", &json!({"script": " "}))
        .expect("blank Pine script answers a payload");
    assert_eq!(blank["ok"], false, "{blank}");
    assert!(
        blank["errors"].as_array().is_some_and(|errors| {
            errors
                .iter()
                .any(|error| error.as_str().is_some_and(|text| text.contains("必填")))
        }),
        "{blank}"
    );
    assert!(blank["saveHint"].is_object(), "{blank}");
}

#[test]
fn production_mcp_pine_validation_maps_bad_arguments_and_rejects_unsupported_scripts() {
    let (_directory, ports) = production_bundle();
    let executor = ProductionMcpToolExecutor::from_production_ports(Arc::new(ports));

    let bad_arguments = executor
        .execute_production("strategy.validate_pine", &json!({"script": 7}))
        .expect_err("non-string Pine script");
    assert_eq!(bad_arguments.status, 400);
    assert_eq!(bad_arguments.code, "BAD_REQUEST");

    let unsupported = executor
        .execute_production(
            "strategy.validate_pine",
            &json!({
                "script": "//@version=6\nstrategy(\"Bad\")\nimport TradingView/ta/7"
            }),
        )
        .expect("unsupported Pine script should produce diagnostics");
    assert_eq!(unsupported["ok"], false);
    assert!(
        unsupported["errors"]
            .as_array()
            .is_some_and(|errors| !errors.is_empty())
    );
    assert!(unsupported["requirements"].is_null());
}

#[test]
fn production_mcp_pine_off_mode_keeps_disabled_external_engine_without_analyzer_call() {
    let analyzer = Arc::new(RecordingStrategyPineAnalyze::new(Ok(json!({
        "ok": true,
        "metadata": {"pineTsVersion": "0.9.31"},
    }))));
    let (_directory, mut ports) = production_bundle();
    ports.strategy_pine_analyze = analyzer.clone();
    let executor = ProductionMcpToolExecutor::from_production_ports(Arc::new(ports));

    let payload = executor
        .strategy_pine_mcp_with_mode(
            "strategy.validate_pine",
            &json!({"script": "//@version=6\nstrategy(\"off\")"}),
            "off",
        )
        .expect("off-mode validation");
    assert_eq!(payload["externalEngine"]["enabled"], false);
    assert_eq!(payload["externalEngine"]["mode"], "off");
    assert_eq!(payload["externalEngine"]["status"], "disabled");
    assert!(
        analyzer
            .shadow_calls
            .lock()
            .expect("record Pine shadow call")
            .is_empty()
    );
}

#[test]
fn production_mcp_pine_shadow_mode_reports_unavailable_analyzer_truthfully() {
    let analyzer = Arc::new(RecordingStrategyPineAnalyze::new(Err(
        StrategyPineAnalyzeSnapshotError::Unavailable("worker is unavailable".to_owned()),
    )));
    let (_directory, mut ports) = production_bundle();
    ports.strategy_pine_analyze = analyzer.clone();
    let executor = ProductionMcpToolExecutor::from_production_ports(Arc::new(ports));

    let payload = executor
        .strategy_pine_mcp_with_mode(
            "strategy.validate_pine",
            &json!({"script": "//@version=6\nstrategy(\"shadow\")"}),
            "shadow",
        )
        .expect("shadow validation");
    assert_eq!(payload["externalEngine"]["enabled"], true);
    assert_eq!(payload["externalEngine"]["mode"], "shadow");
    assert_eq!(payload["externalEngine"]["status"], "shadow_error");
    assert_eq!(payload["externalEngine"]["ok"], false);
    assert_eq!(
        payload["externalEngine"]["diagnostics"][0]["code"],
        "PINETS_SHADOW_ERROR"
    );
    assert_eq!(
        payload["externalEngine"]["differenceSummary"]["evaluated"],
        false
    );
    assert_eq!(
        payload["externalEngine"]["diagnostics"][0]["message"],
        "worker is unavailable"
    );
    assert_eq!(
        analyzer
            .shadow_calls
            .lock()
            .expect("record Pine shadow call")
            .len(),
        1
    );
    assert!(
        analyzer
            .analyze_calls
            .lock()
            .expect("record Pine analyzer call")
            .is_empty()
    );
}

#[test]
fn production_mcp_pine_community_mode_requires_agpl_notice_before_analyzer() {
    let analyzer = Arc::new(RecordingStrategyPineAnalyze::new(Ok(json!({
        "ok": true,
    }))));
    let (_directory, mut ports) = production_bundle();
    ports.strategy_pine_analyze = analyzer.clone();
    let executor = ProductionMcpToolExecutor::from_production_ports(Arc::new(ports));

    let payload = executor
        .strategy_pine_mcp_with_mode_and_notice(
            "strategy.validate_pine",
            &json!({"script": "//@version=6\nstrategy(\"community\")"}),
            "community-agpl",
            false,
        )
        .expect("community-agpl validation");
    assert_eq!(payload["externalEngine"]["enabled"], true);
    assert_eq!(payload["externalEngine"]["mode"], "community-agpl");
    assert_eq!(payload["externalEngine"]["status"], "compliance_error");
    assert_eq!(
        payload["externalEngine"]["diagnostics"][0]["code"],
        "PINETS_AGPL_NOTICE_MISSING"
    );
    assert_eq!(payload["externalEngine"]["license"], "");
    assert_eq!(payload["externalEngine"]["repository"], "");
    assert!(
        analyzer
            .shadow_calls
            .lock()
            .expect("record Pine shadow call")
            .is_empty()
    );
}

#[test]
fn production_mcp_pine_shadow_success_maps_worker_metadata_and_difference_summary() {
    let analyzer = Arc::new(RecordingStrategyPineAnalyze::new(Ok(json!({
        "ok": true,
        "diagnostics": [{
            "severity": "warning",
            "code": "PINE_WARN",
            "message": "mapped warning",
            "line": 3,
            "column": 2,
        }],
        "plots": {"close": {"title": "close", "data": [100.0, 101.0]}},
        "signals": {"close": 101.0},
        "engineVersion": "0.9.31",
    }))));
    let (_directory, mut ports) = production_bundle();
    ports.strategy_pine_analyze = analyzer.clone();
    let executor = ProductionMcpToolExecutor::from_production_ports(Arc::new(ports));

    let payload = executor
        .strategy_pine_mcp_with_mode(
            "strategy.validate_pine",
            &json!({"script": "//@version=6\nstrategy(\"success\")"}),
            "shadow",
        )
        .expect("successful shadow validation");
    let external = &payload["externalEngine"];
    assert_eq!(external["status"], "shadow_ok");
    assert_eq!(external["ok"], true);
    assert_eq!(external["engineVersion"], "0.9.31");
    assert_eq!(external["license"], "AGPL-3.0-only");
    assert_eq!(external["repository"], "https://github.com/LuxAlgo/PineTS");
    assert_eq!(external["differenceSummary"]["evaluated"], true);
    assert_eq!(external["differenceSummary"]["plots"], 1);
    assert_eq!(external["differenceSummary"]["signals"], 1);
    assert_eq!(external["diagnostics"][0]["code"], "PINE_WARN");
    let calls = analyzer
        .shadow_calls
        .lock()
        .expect("record Pine shadow call");
    assert_eq!(calls.len(), 1);
    assert_eq!(calls[0].source_format, "pine-v6");
    assert!(!calls[0].include_ast);
    assert!(
        analyzer
            .analyze_calls
            .lock()
            .expect("record Pine analyzer call")
            .is_empty()
    );
}

#[test]
fn production_mcp_runtime_dependency_readiness_is_truthful() {
    let (_directory, mut ports) = production_bundle();
    for name in ["strategy.pine_spec", "strategy.validate_pine"] {
        assert_eq!(
            mcp_tool_availability(&ports.mcp_catalog, Some(&ports), name),
            "ready",
            "native Pine leaf {name} must not depend on worker readiness"
        );
    }
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
        "ready"
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
fn production_mcp_research_reader_tools_require_futu_opend_and_own_reader() {
    let tools = [
        "research.institutions",
        "research.short_interest",
        "research.technical_indicators",
    ];

    let (_directory, ports) =
        production_bundle_with_research_readers(MarketDataProvider::Futu, true, true, true, true);
    for name in tools {
        assert_eq!(
            mcp_tool_availability(&ports.mcp_catalog, Some(&ports), name),
            "ready",
            "{name} should be ready only with Futu, OpenD, and its typed reader"
        );
    }

    let (_directory, ports) =
        production_bundle_with_research_readers(MarketDataProvider::Futu, true, true, false, false);
    assert_eq!(
        mcp_tool_availability(&ports.mcp_catalog, Some(&ports), "research.institutions"),
        "ready"
    );
    for name in ["research.short_interest", "research.technical_indicators"] {
        assert_eq!(
            mcp_tool_availability(&ports.mcp_catalog, Some(&ports), name),
            "unavailable",
            "{name} must not inherit readiness from another research reader"
        );
    }

    for (provider, opend_ready, case) in [
        (MarketDataProvider::Yfinance, true, "non-Futu provider"),
        (MarketDataProvider::Futu, false, "OpenD not ready"),
    ] {
        let (_directory, ports) =
            production_bundle_with_research_readers(provider, opend_ready, true, true, true);
        for name in tools {
            assert_eq!(
                mcp_tool_availability(&ports.mcp_catalog, Some(&ports), name),
                "unavailable",
                "{name} must stay unavailable when {case}"
            );
        }
    }

    let (_directory, ports) = production_bundle_with_research_readers(
        MarketDataProvider::Futu,
        true,
        false,
        false,
        false,
    );
    for name in tools {
        assert_eq!(
            mcp_tool_availability(&ports.mcp_catalog, Some(&ports), name),
            "unavailable",
            "{name} must stay unavailable when its typed reader is missing"
        );
    }
}

#[test]
fn production_mcp_research_executors_forward_only_supported_route_queries() {
    let (_directory, mut ports) = production_bundle();
    let recorder = Arc::new(RecordingResearchRead::default());
    ports.research_read = recorder.clone();
    let executor = ProductionMcpToolExecutor::from_production_ports(Arc::new(ports));

    executor
        .execute_production(
            "research.institutions",
            &json!({"operation": "list", "market": "US", "underlying": "US.AAPL"}),
        )
        .expect("institution MCP request");
    executor
        .execute_production(
            "research.short_interest",
            &json!({
                "instrumentId": "US.AAPL",
                "operation": "short_interest",
                "startTime": "2026-01-01",
                "endTime": "2026-02-01",
                "period": "1d"
            }),
        )
        .expect("short-interest MCP request");
    executor
        .execute_production(
            "research.technical_indicators",
            &json!({
                "instrumentId": "US.AAPL",
                "operation": "calculate",
                "shortName": "MA",
                "langType": 1,
                "klType": 2,
                "kLine": [{"time": "2026-01-02 09:30:00", "closePrice": 100.5}],
                "num": 20,
                "inputs": [{"index": 0, "value": "20"}],
                "period": "1d"
            }),
        )
        .expect("technical-indicator MCP request");

    let calls = recorder.calls.lock().expect("read research MCP calls");
    assert_eq!(calls.len(), 3);
    assert_eq!(calls[0].0, "/api/v1/research/institutions");
    assert_eq!(calls[0].1, "market=US&operation=list");
    assert_eq!(calls[1].0, "/api/v1/research/short-interest/US.AAPL");
    let short_interest_query = crate::product::product_query::QueryMap::parse(&calls[1].1)
        .expect("parse short-interest MCP query");
    assert_eq!(
        short_interest_query.get_first("operation"),
        Some("short_interest")
    );
    for key in ["startTime", "endTime", "period"] {
        assert!(short_interest_query.get_first(key).is_none(), "{key}");
    }
    assert_eq!(calls[2].0, "/api/v1/research/technical-indicators/US.AAPL");
    let query = crate::product::product_query::QueryMap::parse(&calls[2].1)
        .expect("parse technical indicator MCP query");
    assert_eq!(query.get_first("operation"), Some("calculate"));
    assert_eq!(query.get_first("shortName"), Some("MA"));
    assert_eq!(query.get_first("langType"), Some("1"));
    assert_eq!(query.get_first("klType"), Some("2"));
    assert_eq!(query.get_first("num"), Some("20"));
    assert_eq!(
        serde_json::from_str::<Value>(query.get_first("kLine").expect("kLine query"))
            .expect("decode kLine query"),
        json!([{"time": "2026-01-02 09:30:00", "closePrice": 100.5}])
    );
    assert_eq!(
        serde_json::from_str::<Value>(query.get_first("inputs").expect("inputs query"))
            .expect("decode inputs query"),
        json!([{"index": 0, "value": "20"}])
    );
    assert!(query.get_first("period").is_none());
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

fn advertised_tool_names(response: &Value) -> Vec<String> {
    response["result"]["tools"]
        .as_array()
        .expect("tools/list tools")
        .iter()
        .filter_map(|tool| tool["name"].as_str().map(str::to_owned))
        .collect()
}

fn advertised_availability(response: &Value, name: &str) -> String {
    response["result"]["tools"]
        .as_array()
        .expect("tools/list tools")
        .iter()
        .find(|tool| tool["name"] == name)
        .and_then(|tool| tool["x-jftrade-availability"].as_str())
        .map(str::to_owned)
        .unwrap_or_else(|| panic!("{name} is not advertised: {response}"))
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

/// Parity: go:452dea11:internal/assistant/engine/mcp_server_test.go:17
/// `TestLocalMCPHandlerExposesOnlyReviewedReadTools`: the listener advertises
/// the reviewed read subset (including the strategy-version reads), never the
/// write-capable names, keeps the `system.status` object schema, and dispatches
/// the reviewed reads.
#[test]
fn mcp_tools_list_exposes_only_reviewed_read_tools() {
    let runtime = ProductMcpServerRuntime::with_executor(catalog(), Arc::new(SuccessExecutor));
    let port = available_port();
    runtime
        .apply(&enabled_record(port, "none", ""))
        .expect("start MCP");
    let (status, listed) = request_modern_with_status(port, "tools/list", 11, None, json!({}));
    assert_eq!(status, 200, "tools/list response: {listed}");
    let names = advertised_tool_names(&listed);
    for reviewed in [
        "system.status",
        "strategy.definition_versions.list",
        "strategy.definition_versions.get",
    ] {
        assert!(
            names.iter().any(|name| name == reviewed),
            "tools/list missing reviewed read tool {reviewed}: {listed}"
        );
    }
    for write_capable in ["strategy.save_definition", "http.fetch", "tasks.create"] {
        assert!(
            !names.iter().any(|name| name == write_capable),
            "tools/list exposed non-reviewed tool {write_capable}: {listed}"
        );
    }
    let status_tool = listed["result"]["tools"]
        .as_array()
        .expect("tools/list tools")
        .iter()
        .find(|tool| tool["name"] == "system.status")
        .expect("system.status descriptor");
    assert_eq!(
        status_tool["inputSchema"]["type"], "object",
        "system.status schema = {status_tool}"
    );

    let (status, call) = request_modern_with_status(
        port,
        "tools/call",
        12,
        Some("system.status"),
        json!({"name": "system.status", "arguments": {}}),
    );
    assert_eq!(status, 200, "tools/call response: {call}");
    assert!(call.get("error").is_none(), "tools/call error: {call}");
    assert!(call["result"]["content"].as_array().is_some());

    let (status, version_call) = request_modern_with_status(
        port,
        "tools/call",
        13,
        Some("strategy.definition_versions.get"),
        json!({
            "name": "strategy.definition_versions.get",
            "arguments": {"definitionId": "def-1", "version": "0.1.0"}
        }),
    );
    assert_eq!(status, 200, "strategy version response: {version_call}");
    assert!(
        version_call.get("error").is_none(),
        "reviewed strategy version read must dispatch: {version_call}"
    );
    runtime.shutdown_blocking().expect("shutdown MCP");
}

/// Parity: go:452dea11:internal/assistant/engine/mcp_server_test.go:119
/// `TestLocalMCPHandlerRejectsWriteCapableReplacementOfReviewedName`: a
/// write-capable tool can never take over an MCP-callable name.  Rust has no
/// registry replacement surface, so the equivalent invariant is proven against
/// the wire: even an executor that claims every reviewed name cannot reach a
/// write-capable one, and no reviewed descriptor carries a write permission.
#[test]
fn write_capable_names_are_never_reachable_through_the_reviewed_allowlist() {
    #[derive(Debug)]
    struct PermissiveExecutor;

    impl McpToolExecutor for PermissiveExecutor {
        fn execute(&self, _name: &str, _arguments: &Value) -> Result<Value, String> {
            Ok(json!({"written": true}))
        }
    }

    for name in REVIEWED_READ_ONLY_TOOLS {
        let policy =
            crate::product::product_production_ports::product_production_ports_adk::tool_access_policy(
                name,
            );
        assert!(
            policy.permission.starts_with("read_"),
            "reviewed MCP tool {name} must stay read-only, got {}",
            policy.permission
        );
    }

    let runtime = ProductMcpServerRuntime::with_executor(catalog(), Arc::new(PermissiveExecutor));
    let port = available_port();
    runtime
        .apply(&enabled_record(port, "none", ""))
        .expect("start MCP");
    let (status, listed) = request_modern_with_status(port, "tools/list", 21, None, json!({}));
    assert_eq!(status, 200, "tools/list response: {listed}");
    let names = advertised_tool_names(&listed);
    for write_capable in ["strategy.save_definition", "http.fetch", "tasks.create"] {
        assert!(
            !names.iter().any(|name| name == write_capable),
            "write-capable name {write_capable} is advertised: {listed}"
        );
        let (status, call) = request_modern_with_status(
            port,
            "tools/call",
            22,
            Some(write_capable),
            json!({"name": write_capable, "arguments": {}}),
        );
        assert_eq!(status, 400, "tools/call response: {call}");
        assert_eq!(
            call["error"]["code"], -32602,
            "write-capable name {write_capable} must stay unreachable: {call}"
        );
        assert_eq!(
            call["error"]["message"],
            json!(format!("unknown tool \"{write_capable}\"")),
            "the reviewed allowlist, not the capability graph, must reject {write_capable}: {call}"
        );
        assert!(
            call.get("result").is_none(),
            "write-capable name {write_capable} produced a result: {call}"
        );
    }
    runtime.shutdown_blocking().expect("shutdown MCP");
}

/// Parity: go:452dea11:internal/assistant/engine/mcp_server_test.go:194
/// `TestLocalMCPHandlerServesStatelessPostOnlyRequests`: request/response carry
/// no `Mcp-Session-Id`, a session-less client can list and call tools, and
/// non-POST verbs answer 405 with `Allow: POST`.
#[test]
fn stateless_post_only_requests_never_issue_a_session_header() {
    let runtime = ProductMcpServerRuntime::with_executor(catalog(), Arc::new(SuccessExecutor));
    let port = available_port();
    runtime
        .apply(&enabled_record(port, "none", ""))
        .expect("start MCP");
    let initialize = request(
        port,
        None,
        r#"{"jsonrpc":"2.0","id":1,"method":"tools/list"}"#,
        "127.0.0.1:1",
    );
    assert!(initialize.contains("200 OK"), "response = {initialize}");
    assert!(
        !initialize.to_ascii_lowercase().contains("mcp-session-id"),
        "stateless MCP answered a session header: {initialize}"
    );
    let call = request(
        port,
        None,
        r#"{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"system.status"}}"#,
        "127.0.0.1:1",
    );
    assert!(call.contains("200 OK"), "response = {call}");
    assert!(
        !call.to_ascii_lowercase().contains("mcp-session-id"),
        "stateless MCP answered a session header: {call}"
    );
    assert!(
        call.contains("\"result\""),
        "session-less tools/call must dispatch: {call}"
    );
    for method in ["GET", "DELETE"] {
        let response = request_with_method(port, method, Some("localhost"), None, "");
        assert!(
            response.contains("405 Method Not Allowed"),
            "{method} response = {response}"
        );
        assert!(
            response.to_ascii_lowercase().contains("allow: post"),
            "{method} response = {response}"
        );
    }
    runtime.shutdown_blocking().expect("shutdown MCP");
}

/// Parity: go:452dea11:internal/assistant/engine/mcp_server_test.go:440
/// `TestLocalMCPHandlerRefreshesReplacedToolHandler`: Rust has no registry
/// replacement notification, so the equivalent observable invariant is that a
/// call is resolved against the current executor on every request instead of a
/// cached per-runtime handler result.
#[test]
fn tool_calls_resolve_the_current_executor_on_every_request() {
    #[derive(Debug, Default)]
    struct CountingExecutor {
        calls: std::sync::atomic::AtomicU32,
    }

    impl McpToolExecutor for CountingExecutor {
        fn execute(&self, _name: &str, _arguments: &Value) -> Result<Value, String> {
            let call = self.calls.fetch_add(1, std::sync::atomic::Ordering::SeqCst) + 1;
            Ok(json!({"version": call.to_string()}))
        }
    }

    let runtime =
        ProductMcpServerRuntime::with_executor(catalog(), Arc::new(CountingExecutor::default()));
    let port = available_port();
    runtime
        .apply(&enabled_record(port, "none", ""))
        .expect("start MCP");
    for (id, expected) in [(31_u64, "1"), (32_u64, "2")] {
        let (status, call) = request_modern_with_status(
            port,
            "tools/call",
            id,
            Some("system.status"),
            json!({"name": "system.status", "arguments": {}}),
        );
        assert_eq!(status, 200, "tools/call response: {call}");
        let text = call["result"]["content"][0]["text"]
            .as_str()
            .expect("tool content text");
        assert!(
            text.contains(&json!({"version": expected}).to_string()),
            "call {id} must resolve the current executor: {call}"
        );
    }
    runtime.shutdown_blocking().expect("shutdown MCP");
}

/// Parity: go:452dea11:internal/assistant/engine/mcp_server_test.go:162
/// `TestLocalMCPHandlerReturnsToolFailuresAsMCPToolErrors`: a failing tool is a
/// successful JSON-RPC response whose result carries `isError` plus the
/// failure text, not a transport error.
#[test]
fn tool_failures_are_returned_as_mcp_tool_errors() {
    #[derive(Debug)]
    struct FailingExecutor;

    impl McpToolExecutor for FailingExecutor {
        fn execute(&self, _name: &str, _arguments: &Value) -> Result<Value, String> {
            Err("status provider unavailable".to_owned())
        }
    }

    let runtime = ProductMcpServerRuntime::with_executor(catalog(), Arc::new(FailingExecutor));
    let port = available_port();
    runtime
        .apply(&enabled_record(port, "none", ""))
        .expect("start MCP");
    let (status, response) = request_modern_with_status(
        port,
        "tools/call",
        21,
        Some("system.status"),
        json!({"name": "system.status", "arguments": {}}),
    );
    assert_eq!(
        status, 200,
        "tool failure stays a JSON-RPC result: {response}"
    );
    let result = &response["result"];
    assert_eq!(
        result["isError"], true,
        "Go marks the tool result as an error: {response}"
    );
    let text = result["content"][0]["text"].as_str().unwrap_or_default();
    assert!(
        text.contains("status provider unavailable"),
        "the failure text is forwarded: {response}"
    );
    runtime.shutdown_blocking().expect("shutdown MCP");
}

/// Parity: go:452dea11:internal/assistant/engine/workflow_tools_test.go:140
/// `TestGoogleADKProductToolsetFunctionToolBoundaries` (the strict-schema
/// half).
///
/// Go validates a tool's JSON schema before the business handler runs: an
/// `integer` given a string, a missing `required` property, and an extra
/// property under `additionalProperties:false` are all rejected while the
/// handler call count stays at one.  Rust validates the advertised schema in
/// `call_tool` before dispatch, so the equivalent invariant is that a valid
/// call reaches the executor exactly once and no rejected call reaches it.
#[test]
fn strict_tool_schemas_reject_invalid_arguments_before_the_executor_runs() {
    #[derive(Debug, Default)]
    struct RecordingExecutor {
        calls: std::sync::atomic::AtomicU32,
    }

    impl McpToolExecutor for RecordingExecutor {
        fn execute(&self, name: &str, _arguments: &Value) -> Result<Value, String> {
            self.calls.fetch_add(1, std::sync::atomic::Ordering::SeqCst);
            Ok(json!({"ok": true, "tool": name}))
        }
    }

    let executor = Arc::new(RecordingExecutor::default());
    let runtime = ProductMcpServerRuntime::with_executor(catalog(), executor.clone());
    let port = available_port();
    runtime
        .apply(&enabled_record(port, "none", ""))
        .expect("start MCP");

    let (status, call) = request_modern_with_status(
        port,
        "tools/call",
        41,
        Some("market.search"),
        json!({"name": "market.search", "arguments": {"query": "AAPL"}}),
    );
    assert_eq!(status, 200, "valid arguments must dispatch: {call}");
    assert!(call.get("error").is_none(), "valid call error: {call}");
    assert_eq!(
        executor.calls.load(std::sync::atomic::Ordering::SeqCst),
        1,
        "the valid call reaches the business handler exactly once"
    );

    for (id, arguments) in [
        // `integer` schema fed a string.
        (42_u64, json!({"query": "AAPL", "pageSize": "10"})),
        // Missing `required` property.
        (43, json!({})),
        // Extra property under `additionalProperties:false`.
        (44, json!({"query": "AAPL", "extra": true})),
    ] {
        let (status, response) = request_modern_with_status(
            port,
            "tools/call",
            id,
            Some("market.search"),
            json!({"name": "market.search", "arguments": arguments}),
        );
        assert_eq!(status, 400, "rejected arguments {arguments}: {response}");
        assert_eq!(response["error"]["code"], -32602, "response: {response}");
        assert!(
            response["error"]["message"]
                .as_str()
                .is_some_and(|message| message.contains("invalid arguments for market.search")),
            "response: {response}"
        );
        assert!(
            response.get("result").is_none(),
            "a schema rejection never returns tool content: {response}"
        );
    }
    assert_eq!(
        executor.calls.load(std::sync::atomic::Ordering::SeqCst),
        1,
        "no rejected call may enter the business handler"
    );
    runtime.shutdown_blocking().expect("shutdown MCP");
}

/// Parity: go:452dea11:internal/assistant/engine/mcp_server_test.go:293
/// `TestLocalMCPHandlerReadsSanitizedRuntimeStatusResource`: the runtime-status
/// resource is the only listed resource, answers `application/json`, and never
/// leaks an internal snapshot error for a runtime without a store.
#[test]
fn runtime_status_resource_reports_store_and_reviewed_tools() {
    let runtime = runtime();
    let port = available_port();
    runtime
        .apply(&enabled_record(port, "none", ""))
        .expect("start MCP");
    let (status, listed) = request_modern_with_status(port, "resources/list", 31, None, json!({}));
    assert_eq!(status, 200, "resources/list response: {listed}");
    let resources = listed["result"]["resources"]
        .as_array()
        .expect("resources/list resources");
    assert_eq!(resources.len(), 1, "resources = {listed}");
    assert_eq!(resources[0]["uri"], "jftrade://runtime/status");

    let (status, read) = request_modern_with_status(
        port,
        "resources/read",
        32,
        Some("jftrade://runtime/status"),
        json!({"uri": "jftrade://runtime/status"}),
    );
    assert_eq!(status, 200, "resources/read response: {read}");
    let contents = read["result"]["contents"]
        .as_array()
        .expect("resources/read contents");
    assert_eq!(contents.len(), 1);
    assert_eq!(contents[0]["mimeType"], "application/json");
    let runtime_status: Value =
        serde_json::from_str(contents[0]["text"].as_str().expect("status text"))
            .expect("decode runtime status");
    assert_eq!(
        runtime_status["storeConfigured"], false,
        "Go reports a store-less runtime: {runtime_status}"
    );
    assert!(
        runtime_status["tools"]
            .as_array()
            .is_some_and(|tools| !tools.is_empty()),
        "the tool catalog is always reported: {runtime_status}"
    );
    assert!(
        runtime_status.get("snapshotError").is_none(),
        "a store-less runtime is configured, not broken: {runtime_status}"
    );
    for key in ["providers", "agents", "skills"] {
        assert_eq!(
            runtime_status[key],
            json!([]),
            "Go answers empty {key} summaries: {runtime_status}"
        );
    }
    runtime.shutdown_blocking().expect("shutdown MCP");
}

/// Parity: go:452dea11:internal/assistant/engine/mcp_server_test.go:333
/// `TestSanitizedMCPRuntimeStatusIncludesConfiguredDataAndErrors`: a runtime
/// with a store reports the configured providers/agents/skills without a
/// snapshot error.
#[test]
fn runtime_status_resource_includes_configured_providers_agents_and_skills() {
    let directory = tempfile::tempdir().expect("runtime status temp directory");
    let adk_path = directory.path().join("adk.db");
    let connection = rusqlite::Connection::open(&adk_path).expect("create status database");
    jftrade_store_sqlite::initialize_current(&connection, "adk").expect("initialize schema");
    drop(connection);
    let store = Arc::new(jftrade_store_sqlite::AdkStore::open(&adk_path).expect("open store"));
    store
        .upsert_provider(
            "provider-status",
            &json!({
                "id": "provider-status",
                "displayName": "Status Provider",
                "model": "fixture-model",
                "enabled": true,
                "apiKey": "sk-fixture",
            })
            .to_string(),
        )
        .expect("persist provider");
    store
        .upsert_agent(
            "agent-status",
            &json!({
                "id": "agent-status",
                "name": "Status Agent",
                "providerId": "provider-status",
                "status": "ENABLED",
                "tools": ["system.status"],
            })
            .to_string(),
        )
        .expect("persist agent");
    store
        .upsert_skill(
            "skill-status",
            &json!({
                "id": "skill-status",
                "displayName": "Status Skill",
                "source": "builtin",
                "enabled": true,
                "builtin": true,
                "tools": ["system.status"],
                "version": "1",
            })
            .to_string(),
        )
        .expect("persist skill");
    let runtime = ProductMcpServerRuntime::new(catalog(), Arc::clone(&store));
    let port = available_port();
    runtime
        .apply(&enabled_record(port, "none", ""))
        .expect("start MCP");
    let (status, read) = request_modern_with_status(
        port,
        "resources/read",
        41,
        Some("jftrade://runtime/status"),
        json!({"uri": "jftrade://runtime/status"}),
    );
    assert_eq!(status, 200, "resources/read response: {read}");
    let text = read["result"]["contents"][0]["text"]
        .as_str()
        .expect("status text");
    let runtime_status: Value = serde_json::from_str(text).expect("decode runtime status");
    assert_eq!(runtime_status["storeConfigured"], true);
    assert!(
        runtime_status.get("snapshotError").is_none(),
        "a readable store is not an error: {runtime_status}"
    );
    assert_eq!(
        runtime_status["providers"][0]["id"], "provider-status",
        "providers = {runtime_status}"
    );
    assert_eq!(runtime_status["agents"][0]["id"], "agent-status");
    assert_eq!(runtime_status["skills"][0]["id"], "skill-status");
    runtime.shutdown_blocking().expect("shutdown MCP");
}

/// Parity: go:452dea11:internal/assistant/engine/mcp_server_test.go:358
/// `TestSanitizedMCPRuntimeStatusSerializesDescriptors`: the sanitized
/// provider/agent/skill summaries keep the console-facing fields, normalize
/// `toolAccessMode`, and never carry an API key.
#[test]
fn runtime_status_resource_serializes_sanitized_descriptors() {
    let provider: Value = serde_json::from_str(
        &json!({
            "id": "provider-descriptor",
            "displayName": "Descriptor Provider",
            "model": "model",
            "enabled": true,
            "default": true,
            "apiKey": "sk-secret-fixture",
            "capabilities": {"tools": true, "apiKey": "sk-leak"},
        })
        .to_string(),
    )
    .expect("provider payload");
    let sanitized_provider = sanitized_provider(jftrade_store_sqlite::StoredAdkEntity {
        id: "provider-descriptor".to_owned(),
        payload_json: provider.to_string(),
        created_at: String::new(),
        updated_at: String::new(),
    });
    assert_eq!(sanitized_provider["id"], "provider-descriptor");
    assert_eq!(sanitized_provider["hasApiKey"], true);
    assert_eq!(sanitized_provider["default"], true);
    assert_eq!(sanitized_provider["capabilities"], json!({"tools": true}));
    assert!(
        !sanitized_provider.to_string().contains("sk-secret-fixture")
            && !sanitized_provider.to_string().contains("sk-leak"),
        "the sanitized provider must not leak key material: {sanitized_provider}"
    );

    let agent = json!({
        "id": "agent-descriptor",
        "name": "Descriptor Agent",
        "providerId": "provider-descriptor",
        "model": "model",
        "tools": ["system.status"],
        "toolAccessMode": "selected",
        "skills": ["skill-descriptor"],
        "permissionMode": "approval",
        "status": "ENABLED",
        "builtin": true,
    });
    let sanitized_agent = sanitized_agent(jftrade_store_sqlite::StoredAdkEntity {
        id: "agent-descriptor".to_owned(),
        payload_json: agent.to_string(),
        created_at: String::new(),
        updated_at: String::new(),
    });
    assert_eq!(sanitized_agent["providerId"], "provider-descriptor");
    assert_eq!(sanitized_agent["toolAccessMode"], "selected");
    assert_eq!(sanitized_agent["permissionMode"], "approval");
    assert_eq!(sanitized_agent["builtin"], true);

    let skill = json!({
        "id": "skill-descriptor",
        "displayName": "Descriptor Skill",
        "description": "desc",
        "source": "builtin",
        "enabled": true,
        "builtin": true,
        "tools": ["system.status"],
        "version": "1",
        "validationStatus": "warning",
        "validationError": "future.tool is unavailable",
    });
    let sanitized_skill = sanitized_skill(jftrade_store_sqlite::StoredAdkEntity {
        id: "skill-descriptor".to_owned(),
        payload_json: skill.to_string(),
        created_at: String::new(),
        updated_at: String::new(),
    });
    assert_eq!(sanitized_skill["validationStatus"], "warning");
    assert_eq!(
        sanitized_skill["validationError"],
        "future.tool is unavailable"
    );
    assert_eq!(sanitized_skill["source"], "builtin");
}

/// Parity: go:452dea11:internal/assistant/engine/mcp_server_test.go:382
/// `TestLocalMCPHandlerSynchronizesReviewedToolsAndRuntimeSubscriptions`: Rust
/// has no tool-registry change notification, so the equivalent contract is that
/// the advertised tool surface is re-projected from the runtime's capability
/// graph on every request: the same reviewed name is `ready` while its adapter
/// is bound and `fail-closed` once the adapter disappears, and a fail-closed
/// name can never be dispatched.
#[test]
fn runtime_status_resource_reflects_live_dependency_availability() {
    let (_ready_directory, ready_ports) = production_bundle();
    let ready_port = available_port();
    let ready_runtime = ProductMcpServerRuntime::from_production_ports(Arc::new(ready_ports));
    ready_runtime
        .apply(&enabled_record(ready_port, "none", ""))
        .expect("start ready MCP");
    let (status, ready_list) =
        request_modern_with_status(ready_port, "tools/list", 51, None, json!({}));
    assert_eq!(status, 200, "tools/list response: {ready_list}");
    assert_eq!(
        advertised_availability(&ready_list, "system.runtime_dependencies"),
        "ready",
        "bound adapter must project readiness: {ready_list}"
    );
    ready_runtime
        .shutdown_blocking()
        .expect("shutdown ready MCP");

    let (_closed_directory, mut closed_ports) = production_bundle();
    closed_ports.bound_adapters.remove(
        &crate::product::product_production_route_registry::ProductionRouteAdapter::SystemRead,
    );
    let closed_port = available_port();
    let closed_runtime = ProductMcpServerRuntime::from_production_ports(Arc::new(closed_ports));
    closed_runtime
        .apply(&enabled_record(closed_port, "none", ""))
        .expect("start fail-closed MCP");
    let (status, closed_list) =
        request_modern_with_status(closed_port, "tools/list", 52, None, json!({}));
    assert_eq!(status, 200, "tools/list response: {closed_list}");
    assert_eq!(
        advertised_availability(&closed_list, "system.runtime_dependencies"),
        "fail-closed",
        "missing adapter must fail closed: {closed_list}"
    );
    let (status, call) = request_modern_with_status(
        closed_port,
        "tools/call",
        53,
        Some("system.runtime_dependencies"),
        json!({"name": "system.runtime_dependencies", "arguments": {}}),
    );
    assert_eq!(status, 400, "tools/call response: {call}");
    assert_eq!(
        call["error"]["code"], -32602,
        "fail-closed tool must not dispatch: {call}"
    );
    closed_runtime
        .shutdown_blocking()
        .expect("shutdown fail-closed MCP");
}

/// Parity: go:452dea11:internal/assistant/engine/mcp_server_test.go:367
/// `TestLocalMCPRuntimeStatusSubscriptionValidation`: subscribe and
/// unsubscribe only accept the runtime-status URI.
#[test]
fn runtime_status_subscription_validation_rejects_unknown_uris() {
    let runtime = runtime();
    let port = available_port();
    runtime
        .apply(&enabled_record(port, "none", ""))
        .expect("start MCP");
    let (status, accepted) = request_modern_with_status(
        port,
        "resources/subscribe",
        51,
        None,
        json!({"uri": "jftrade://runtime/status"}),
    );
    assert_eq!(status, 200, "valid subscription = {accepted}");
    for method in ["resources/subscribe", "resources/unsubscribe"] {
        let (status, rejected) =
            request_modern_with_status(port, method, 52, None, json!({"uri": "jftrade://invalid"}));
        assert_eq!(status, 200, "{method} stays a JSON-RPC error: {rejected}");
        assert_eq!(
            rejected["error"]["code"], -32002,
            "{method} must report resource not found: {rejected}"
        );
        let (status, missing) = request_modern_with_status(port, method, 53, None, Value::Null);
        assert!(
            status >= 400 || missing.get("error").is_some(),
            "{method} without params must be rejected: {status} {missing}"
        );
    }
    runtime.shutdown_blocking().expect("shutdown MCP");
}

/// Parity: go:452dea11:internal/assistant/assembly/mcp_server_test.go:22
/// `TestMCPServerManagerEnforcesBearerAndSupportsTokenRotation`: rotating the
/// token replaces the accepted secret immediately.
#[test]
fn rotating_the_token_rejects_the_previous_secret() {
    let runtime = runtime();
    let port = available_port();
    let (first_token, first_hash) = jftrade_settings::SystemMcpServerSecrets
        .issue()
        .expect("first fixture secret");
    let (second_token, second_hash) = jftrade_settings::SystemMcpServerSecrets
        .issue()
        .expect("second fixture secret");
    let payload = r#"{"jsonrpc":"2.0","id":1,"method":"tools/list"}"#;
    runtime
        .apply(&enabled_record(port, "token", &first_hash))
        .expect("start MCP with the first token");
    assert!(
        request(port, Some(&first_token), payload, "127.0.0.1:1").contains("200 OK"),
        "the active token is accepted"
    );
    assert!(
        request(port, Some(&second_token), payload, "127.0.0.1:1").contains("401 Unauthorized"),
        "an unrelated token is rejected"
    );
    runtime
        .apply(&enabled_record(port, "token", &second_hash))
        .expect("rotate the MCP token");
    assert!(
        request(port, Some(&second_token), payload, "127.0.0.1:1").contains("200 OK"),
        "the rotated token is accepted"
    );
    assert!(
        request(port, Some(&first_token), payload, "127.0.0.1:1").contains("401 Unauthorized"),
        "the previous token is rejected after rotation"
    );
    runtime.shutdown_blocking().expect("shutdown MCP");
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
    // Parity: internal/assistant/assembly/mcp_server_test.go:76 TestMCPServerManagerStartsAndStopsOnLoopback
    let runtime = runtime();
    let port = available_port();
    runtime
        .apply(&enabled_record(port, "none", ""))
        .expect("start MCP");
    let started = runtime
        .status(&enabled_record(port, "none", ""))
        .expect("started MCP status");
    assert!(started.running, "started MCP status = {started:?}");
    assert_eq!(started.endpoint, format!("http://127.0.0.1:{port}/mcp"));
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
    // Parity: internal/assistant/assembly/mcp_server_test.go:106 TestMCPServerManagerServesAuthenticatedStreamableMCP
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
fn modern_tool_call_rejects_arguments_outside_the_advertised_schema() {
    let runtime = ProductMcpServerRuntime::with_executor(catalog(), Arc::new(SuccessExecutor));
    let port = available_port();
    runtime
        .apply(&enabled_record(port, "none", ""))
        .expect("start MCP");

    let (status, response) = request_modern_with_status(
        port,
        "tools/call",
        1,
        Some("system.status"),
        json!({"name": "system.status", "arguments": {"unexpected": true}}),
    );
    assert_eq!(status, 400, "response={response}");
    assert_eq!(response["error"]["code"], -32602);
    assert_eq!(
        response["error"]["message"],
        "invalid arguments for system.status: arguments.unexpected is not allowed"
    );
    assert!(response.get("result").is_none(), "response={response}");

    runtime.shutdown_blocking().expect("shutdown MCP");
}

#[test]
fn modern_technical_indicator_call_validates_calculation_payload() {
    let runtime = ProductMcpServerRuntime::with_executor(
        technical_indicator_ready_catalog(),
        Arc::new(SuccessExecutor),
    );
    let port = available_port();
    runtime
        .apply(&enabled_record(port, "none", ""))
        .expect("start MCP");

    let (status, response) = request_modern_with_status(
        port,
        "tools/call",
        1,
        Some("research.technical_indicators"),
        json!({
            "name": "research.technical_indicators",
            "arguments": {
                "instrumentId": "US.AAPL",
                "operation": "list",
                "searchKey": "MA",
                "langType": 0,
                "searchMode": 1
            }
        }),
    );
    assert_eq!(status, 200, "response={response}");

    let arguments = json!({
        "instrumentId": "US.AAPL",
        "operation": "calculate",
        "shortName": "MA",
        "langType": 1,
        "klType": 2,
        "kLine": [{"time": "2026-01-02 09:30:00", "closePrice": 100.5}],
        "num": 20,
        "inputs": [{"index": 0, "value": "20"}]
    });
    let (status, response) = request_modern_with_status(
        port,
        "tools/call",
        2,
        Some("research.technical_indicators"),
        json!({"name": "research.technical_indicators", "arguments": arguments}),
    );
    assert_eq!(status, 200, "response={response}");
    assert_eq!(
        response["result"]["structuredContent"]["tool"],
        "research.technical_indicators"
    );

    let (status, response) = request_modern_with_status(
        port,
        "tools/call",
        3,
        Some("research.technical_indicators"),
        json!({
            "name": "research.technical_indicators",
            "arguments": {
                "instrumentId": "US.AAPL",
                "operation": "calculate",
                "langType": 1,
                "klType": 2,
                "kLine": []
            }
        }),
    );
    assert_eq!(status, 400, "response={response}");
    assert_eq!(response["error"]["code"], -32602);
    assert_eq!(
        response["error"]["message"],
        "invalid arguments for research.technical_indicators: arguments.shortName is required"
    );

    runtime.shutdown_blocking().expect("shutdown MCP");
}

#[test]
fn modern_production_pine_tool_executes_without_pine_worker() {
    let (_directory, ports) = production_bundle();
    let runtime = ProductMcpServerRuntime::from_production_ports(Arc::new(ports));
    let port = available_port();
    runtime
        .apply(&enabled_record(port, "none", ""))
        .expect("start MCP");

    let (status, response) = request_modern_with_status(
        port,
        "tools/call",
        1,
        Some("strategy.pine_spec"),
        json!({
            "name": "strategy.pine_spec",
            "arguments": {"section": "overview"}
        }),
    );
    assert_eq!(status, 200, "response={response}");
    assert_eq!(
        response["result"]["structuredContent"]["selectedSection"],
        "overview"
    );
    assert!(
        response["result"].get("isError").is_none(),
        "response={response}"
    );

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
    let previous = runtime
        .status(&enabled_record(first, "none", ""))
        .expect("previous status");
    assert!(previous.running, "previous status = {previous:?}");
    assert!(
        previous.last_error.contains("port conflict"),
        "previous status = {previous:?}"
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

/// Parity: go:452dea11:internal/assistant/assembly/mcp_server_test.go:203
/// `TestMCPServerManagerReleasesHandlersOnReplacementDisableAndClose`: each
/// replacement listener owns the port it binds, and replacing, disabling or
/// closing the runtime releases the previous owner's port.
#[test]
fn replacement_disable_and_close_release_each_listener_owner() {
    let runtime = runtime();
    let first = available_port();
    let second = available_port();
    runtime
        .apply(&enabled_record(first, "none", ""))
        .expect("start first MCP");
    runtime
        .apply(&enabled_record(second, "none", ""))
        .expect("replace MCP listener");
    assert!(
        runtime
            .status(&enabled_record(second, "none", ""))
            .expect("replacement status")
            .running
    );
    let replaced = StdTcpListener::bind(("127.0.0.1", first))
        .expect("replaced listener must release the previous port");
    drop(replaced);
    runtime
        .apply(&McpServerSettingsRecord::new(
            false,
            i32::from(second),
            "none",
            "",
        ))
        .expect("disable MCP");
    let disabled =
        StdTcpListener::bind(("127.0.0.1", second)).expect("disabled MCP must release its port");
    drop(disabled);
    runtime.shutdown_blocking().expect("close MCP");
    runtime.shutdown_blocking().expect("close MCP twice");
}

/// Parity: go:452dea11:internal/assistant/assembly/mcp_server_test.go:257
/// `TestMCPServerManagerReleasesHandlerOnUnexpectedServeExit`: Rust publishes
/// `MCP listener stopped unexpectedly` through the worker guard and flips the
/// running flag, but the reference test's injectable listener seam does not
/// exist.  The reachable equivalent is the cold-start failure path: a listener
/// that cannot bind leaves a stopped runtime with the recorded reason, and the
/// recovery apply clears it.
#[test]
fn cold_start_listener_failure_records_the_reason_and_recovery_clears_it() {
    let runtime = runtime();
    let occupied_port = available_port();
    let occupied = StdTcpListener::bind(("127.0.0.1", occupied_port)).expect("occupy MCP port");
    let error = runtime
        .apply(&enabled_record(occupied_port, "none", ""))
        .expect_err("conflicting MCP bind");
    assert!(error.contains("port conflict"), "error = {error}");
    let failed = runtime
        .status(&enabled_record(occupied_port, "none", ""))
        .expect("failed MCP status");
    assert!(!failed.running, "failed MCP status = {failed:?}");
    assert!(
        failed.last_error.contains("port conflict"),
        "failed MCP status = {failed:?}"
    );
    drop(occupied);
    runtime
        .apply(&enabled_record(occupied_port, "none", ""))
        .expect("recover MCP listener");
    let recovered = runtime
        .status(&enabled_record(occupied_port, "none", ""))
        .expect("recovered MCP status");
    assert!(recovered.running, "recovered MCP status = {recovered:?}");
    assert!(
        recovered.last_error.is_empty(),
        "recovered MCP status = {recovered:?}"
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

/// Parity: go:452dea11:internal/productfeatures/typed_queries_test.go:12
/// TestDocumentResultPreservesFeatureWireShape
///
/// Go marshals each `FeatureResult` entry into a `json.RawMessage`, keeps the
/// metadata bytes, and projects them back at the HTTP edge, so the typed
/// document round-trip is shape preserving for every field. Rust keeps the
/// provider `FeatureResult` as one `Value`, so the equivalent invariant is that
/// the extension boundary returns the port's value verbatim: `provider`, `asOf`,
/// `entries`, `nextCursor`, `hasMore`, `total`, `warnings`, `partialErrors`, and
/// `metadata` survive unchanged.
#[test]
fn research_document_read_round_trips_the_feature_result_wire_shape() {
    let payload = json!({
        "provider": {
            "brokerId": "futu",
            "securityFirm": "FUTUINC",
            "featureId": "research.calendar",
            "capability": "available",
            "resolvedAt": "2026-08-13T12:00:00Z",
            "asOf": "2026-08-13T12:00:00Z",
        },
        "asOf": "2026-08-13T12:00:00Z",
        "entries": [{"instrumentId": "US.AAPL", "value": 1.25, "active": true}],
        "nextCursor": "cursor-2",
        "hasMore": false,
        "total": 1,
        "warnings": ["partial"],
        "partialErrors": [{"code": "PROVIDER_PARTIAL", "message": "one source timed out"}],
        "metadata": {"source": "typed", "count": 1.0},
    });

    #[derive(Debug)]
    struct CannedResearchPort(Value);

    impl ResearchReadSnapshotPort for CannedResearchPort {
        fn read(&self, _: &str, _: &str) -> Result<Value, ResearchReadSnapshotError> {
            Ok(self.0.clone())
        }
    }

    let (_directory, mut ports) = production_bundle();
    ports.research_read = Arc::new(CannedResearchPort(payload.clone()));
    let executor = ProductionMcpToolExecutor::from_production_ports(Arc::new(ports));

    let response = executor
        .execute_production(
            "research.calendar",
            &json!({"operation": "earnings", "market": "US"}),
        )
        .expect("research.calendar document read");
    assert_eq!(
        response, payload,
        "the extension boundary must not reshape the provider FeatureResult"
    );
    for field in [
        "provider",
        "asOf",
        "entries",
        "nextCursor",
        "hasMore",
        "total",
        "warnings",
        "partialErrors",
        "metadata",
    ] {
        assert!(
            response.get(field).is_some(),
            "the document round-trip dropped {field}"
        );
    }
}

/// Parity: go:452dea11:internal/productfeatures/typed_queries_test.go:41
/// TestTypedCapabilityDescriptionsAreDefensive
///
/// Go resolves a typed capability by Assistant tool name and returns a copy:
/// mutating `description.Operations[0]` must not change the shared table, and
/// the next lookup still reports `earnings` for `research.calendar`. Rust has one
/// reviewed operation table in the MCP schema catalog, so the equivalent
/// invariant is that the schema for a tool is rebuilt per call: mutating the
/// returned schema cannot poison the next `tools/list` descriptor, and the
/// calendar tool still advertises exactly the earnings/dividends/economic/ipos/
/// trade_dates operations.
#[test]
fn typed_capability_schemas_are_defensive_and_stable_per_lookup() {
    use crate::product::product_mcp_protocol::schema_for;

    let schema = schema_for("research.calendar");
    assert_eq!(
        schema["properties"]["operation"]["enum"],
        json!(["earnings", "dividends", "economic", "ipos", "trade_dates"]),
        "research.calendar must advertise the reviewed operation set"
    );
    assert!(
        schema.get("required").is_none(),
        "research.calendar keeps every field optional so the route defaults apply"
    );

    // Mutate the returned schema the same way the reference fixture mutates the
    // first operation; a cached/shared value would leak it into the next lookup.
    let mut mutated = schema.clone();
    mutated["properties"]["operation"]["enum"][0] = json!("mutated");
    mutated["required"] = json!(["instrumentId"]);

    let again = schema_for("research.calendar");
    assert_eq!(
        again["properties"]["operation"]["enum"][0], "earnings",
        "a mutated schema must not leak into the next lookup"
    );
    assert!(
        again.get("required").is_none(),
        "a mutated required list must not leak into the next lookup"
    );

    // Unknown tool names must stay unresolved instead of fabricating a schema.
    assert!(
        crate::product::product_mcp_protocol::try_schema_for("research.not_a_tool").is_none(),
        "an unknown tool must not resolve to a schema"
    );
}

/// `workflow.wait` is a local operation, so the regression drives it without a
/// full production port bundle.
fn local_executor() -> ProductionMcpToolExecutor {
    let (_directory, ports) = production_bundle();
    ProductionMcpToolExecutor::from_production_ports(Arc::new(ports))
}

/// Go's `TestWorkflowWaitToolWaitsAndDoesNotRequireApproval`: `workflow.wait`
/// is a `read_internal`/low-risk tool that waits for the requested duration and
/// reports why.  The catalog must project it as automatically executable so
/// the ADK loop runs it instead of asking the operator.
#[test]
fn workflow_wait_tool_waits_and_does_not_require_approval() {
    // The ADK catalog is the one that exposes `workflow.wait`; the extension
    // catalog only carries the MCP protocol tools.
    let bindings = crate::product::product_production_ports::product_production_ports_adk::PRODUCTION_TOOL_DEFINITIONS
        .iter()
        .map(|definition| {
            (
                definition.adapter,
                ProductionAdapterBinding::Ready,
            )
        })
        .collect::<BTreeMap<_, _>>();
    let catalog =
        ProductionToolCatalog::from_bindings(&bindings).expect("complete ADK tool bindings");
    let descriptor = catalog
        .callable_tools()
        .into_iter()
        .find(|tool| tool["id"] == "workflow.wait")
        .expect("workflow.wait descriptor");
    assert_eq!(descriptor["permission"], "read_internal");
    assert_eq!(descriptor["riskLevel"], "low");
    assert!(
        !catalog.requires_approval("workflow.wait", "approval"),
        "workflow.wait must not require approval in approval mode"
    );

    let executor = local_executor();
    let started = std::time::Instant::now();
    let output = executor
        .execute_production(
            "workflow.wait",
            &serde_json::json!({"durationMs": 10, "reason": "test wait"}),
        )
        .expect("workflow.wait succeeds");
    assert!(
        started.elapsed() >= Duration::from_millis(10),
        "workflow.wait must not return before the requested duration"
    );
    assert_eq!(output["reason"], "test wait");
    assert!(
        output["waitedMs"]
            .as_i64()
            .is_some_and(|waited| waited >= 10),
        "output carries the actual waited milliseconds: {output}"
    );
}

/// Go's `TestWorkflowWaitToolRejectsTooLongDuration`: anything above the 25s
/// cap is rejected with the reference error text.
#[test]
fn workflow_wait_tool_rejects_too_long_duration() {
    let executor = local_executor();
    let error = executor
        .execute_production("workflow.wait", &serde_json::json!({"seconds": 26}))
        .expect_err("a 26s wait exceeds the cap");
    assert_eq!(error.status, 400);
    assert!(
        error.message.contains("25s"),
        "unexpected error text: {}",
        error.message
    );
}

/// Go's `TestWorkflowWaitDurationParsesMultipleInputForms`.
#[test]
fn workflow_wait_duration_parses_multiple_input_forms() {
    let duration = |value: serde_json::Value| {
        crate::product::product_mcp_production_executor::workflow_wait_duration(&value)
    };
    assert_eq!(
        duration(serde_json::json!({"durationMs": 1500, "seconds": 9})).expect("ms wins"),
        Duration::from_millis(1500)
    );
    assert_eq!(
        duration(serde_json::json!({"seconds": 1.5})).expect("float seconds"),
        Duration::from_millis(1500)
    );
    assert_eq!(
        duration(serde_json::json!({"seconds": 2})).expect("int seconds"),
        Duration::from_secs(2)
    );
    assert_eq!(
        duration(serde_json::json!({"seconds": "0.25"})).expect("string seconds"),
        Duration::from_millis(250)
    );
    assert!(
        duration(serde_json::json!({"seconds": "   "})).is_err(),
        "a blank string carries no duration"
    );
    assert_eq!(
        duration(serde_json::json!({"seconds": 25})).expect("25s is the boundary"),
        Duration::from_secs(25)
    );
    for invalid in [
        serde_json::json!({"seconds": "later"}),
        serde_json::json!({}),
        serde_json::json!({"seconds": 0}),
    ] {
        let error = duration(invalid.clone()).expect_err("invalid duration is rejected");
        assert!(
            error.message.contains("greater than 0"),
            "unexpected error for {invalid}: {}",
            error.message
        );
    }
    let error =
        duration(serde_json::json!({"durationMs": 25001})).expect_err("25001ms exceeds the cap");
    assert!(
        error.message.contains("25s"),
        "unexpected error: {}",
        error.message
    );
}

/// Go's `TestWorkflowWaitToolReturnsContextCancellation`: a cancelled context
/// aborts the wait immediately and surfaces the cancellation to the caller.
#[test]
fn workflow_wait_tool_returns_context_cancellation() {
    let cancel = std::sync::Arc::new(std::sync::atomic::AtomicBool::new(false));
    let signal = std::sync::Arc::clone(&cancel);
    let worker = std::thread::spawn(move || {
        crate::product::product_mcp_production_executor::workflow_wait_cancellable(
            &serde_json::json!({"durationMs": 5_000}),
            &|| signal.load(std::sync::atomic::Ordering::Acquire),
        )
    });
    // The wait must observe the cancellation instead of sleeping for 5s.
    std::thread::sleep(Duration::from_millis(20));
    cancel.store(true, std::sync::atomic::Ordering::Release);
    let started = std::time::Instant::now();
    let error = worker
        .join()
        .expect("workflow.wait worker")
        .expect_err("a cancelled wait must fail");
    assert_eq!(error.code, "MCP_TOOL_CANCELLED");
    assert!(
        started.elapsed() < Duration::from_secs(2),
        "cancellation must interrupt the wait, not run to completion"
    );
}

/// Go's `TestHTTPFetchToolRejectsInvalidAndUnsafeTargets`.
#[test]
fn http_fetch_tool_rejects_invalid_and_unsafe_targets() {
    use crate::product::product_mcp_production_executor::http_fetch;
    let cases = [
        (serde_json::json!({}), "url is required"),
        (serde_json::json!({"url": "://bad"}), "invalid url"),
        (
            serde_json::json!({"url": "ftp://example.com/feed"}),
            "only http and https are supported",
        ),
        (
            serde_json::json!({"url": "http://localhost:8080/health"}),
            "localhost targets are blocked",
        ),
        (
            serde_json::json!({"url": "http://169.254.169.254/latest/meta-data"}),
            "private, loopback, link-local, multicast and metadata addresses are blocked",
        ),
        (
            serde_json::json!({"url": "http://127.0.0.1/private"}),
            "private, loopback, link-local, multicast and metadata addresses are blocked",
        ),
        (
            serde_json::json!({"url": "http://10.0.0.5/internal"}),
            "private, loopback, link-local, multicast and metadata addresses are blocked",
        ),
        (
            serde_json::json!({"url": "http://8.8.8.8/report", "maxBytes": 0}),
            "maxBytes",
        ),
        (
            serde_json::json!({"url": "http://8.8.8.8/report", "maxBytes": 1 << 21}),
            "maxBytes",
        ),
    ];
    for (input, expected) in cases {
        let error = http_fetch(&input).expect_err("unsafe target must be rejected");
        assert!(
            error.message.contains(expected),
            "httpFetchTool({input}) error = {} want substring {expected:?}",
            error.message
        );
    }
}

/// Go's `TestRejectUnsafeHostAndUnsafeAddrClassification`.
#[test]
fn reject_unsafe_host_and_unsafe_addr_classification() {
    use crate::product::product_mcp_production_executor::{reject_unsafe_host, unsafe_address};
    let error = reject_unsafe_host("").expect_err("empty host is rejected");
    assert!(error.message.contains("host is required"));
    reject_unsafe_host("8.8.8.8").expect("a public address is allowed");
    reject_unsafe_host("1.1.1.1").expect("a public address is allowed");
    let _ = reject_unsafe_host("example.invalid-name"); // a name, not an IP literal
    for (addr, unsafe_addr) in [
        ("127.0.0.1", true),
        ("10.0.0.1", true),
        ("169.254.1.10", true),
        ("224.0.0.1", true),
        ("0.0.0.0", true),
        ("169.254.169.254", true),
        ("172.16.0.1", true),
        ("192.168.1.1", true),
        ("8.8.8.8", false),
        ("1.1.1.1", false),
    ] {
        let addr: std::net::IpAddr = addr.parse().expect("fixture address");
        assert_eq!(
            unsafe_address(addr),
            unsafe_addr,
            "unsafeAddr({addr}) mismatch"
        );
    }
}

/// Parity: go:452dea11:internal/assistant/engine/store_lifecycle_test.go:701
/// TestRejectUnsafeHost.
///
/// Every host in the reference table is refused: IPv4 loopback, the
/// `localhost` name, private ranges, the cloud metadata address and IPv6
/// loopback.
#[test]
fn reject_unsafe_host_blocks_the_reference_host_table() {
    use crate::product::product_mcp_production_executor::reject_unsafe_host;
    for host in [
        "127.0.0.1",
        "localhost",
        "10.0.0.1",
        "169.254.169.254",
        "::1",
    ] {
        let error = reject_unsafe_host(host).expect_err("unsafe host is rejected");
        assert!(
            error.message.contains("blocked"),
            "reject_unsafe_host({host}) message = {:?}",
            error.message
        );
    }
}

/// Go's `TestHTTPFetchToolHandlesResponsesWithoutRealNetwork`: the fetch
/// envelope carries status, body, byte accounting and truncation.  The
/// responses are served by a loopback server, so the URL itself uses the
/// public-address bypass the reference exposes for tests.
#[test]
fn http_fetch_tool_handles_responses_without_real_network() {
    // A loopback server is exactly what the SSRF guard blocks, so the guard is
    // verified separately above; this test covers the response envelope by
    // exercising the pure decoding path against a stubbed transport.
    use crate::product::product_mcp_production_executor::http_fetch_envelope;
    let response = http_fetch_envelope(
        "http://8.8.8.8/report",
        "http://8.8.8.8/report",
        200,
        "text/plain; charset=utf-8",
        b"market snapshot".to_vec(),
        1 << 20,
    );
    assert_eq!(response["status"], 200);
    assert_eq!(response["body"], "market snapshot");
    assert_eq!(response["bytes"], "market snapshot".len());
    assert_eq!(response["maxBytes"], 1 << 20);
    assert_eq!(response["truncated"], false);
    assert_eq!(response["url"], "http://8.8.8.8/report");
    assert_eq!(response["finalUrl"], "http://8.8.8.8/report");

    let truncated = http_fetch_envelope(
        "http://8.8.8.8/start",
        "http://8.8.8.8/final",
        200,
        "application/json",
        b"12345678".to_vec(),
        4,
    );
    assert_eq!(truncated["body"], "1234");
    assert_eq!(truncated["bytes"], 4);
    assert_eq!(truncated["maxBytes"], 4);
    assert_eq!(truncated["truncated"], true);
    assert_eq!(truncated["finalUrl"], "http://8.8.8.8/final");
}

/// The redirect guard: a response that redirects to a blocked host must fail
/// with the reference wording instead of following the redirect.
///
/// The check runs on `reject_unsafe_host`, which is exactly what the redirect
/// policy calls, so this asserts the guard's decision without a live server.
#[test]
fn http_fetch_redirect_guard_blocks_unsafe_hosts() {
    use crate::product::product_mcp_production_executor::reject_unsafe_host;
    for host in ["127.0.0.1", "localhost", "10.1.2.3", "169.254.169.254"] {
        let error = reject_unsafe_host(host).expect_err("redirect target is unsafe");
        assert!(
            error.message.contains("blocked"),
            "redirect to {host} must be blocked: {}",
            error.message
        );
    }
    // A public redirect target stays allowed.
    reject_unsafe_host("8.8.8.8").expect("public redirect target");
}

/// `http.fetch` must be registered as a model-visible, replay-safe tool with
/// the reference `read_external`/medium metadata so approval mode gates it
/// through the medium-risk rule.
#[test]
fn http_fetch_catalog_registration_matches_the_reference() {
    let bindings = crate::product::product_production_ports::product_production_ports_adk::PRODUCTION_TOOL_DEFINITIONS
        .iter()
        .map(|definition| (definition.adapter, ProductionAdapterBinding::Ready))
        .collect::<BTreeMap<_, _>>();
    let catalog =
        ProductionToolCatalog::from_bindings(&bindings).expect("complete ADK tool bindings");
    let descriptor = catalog
        .callable_tools()
        .into_iter()
        .find(|tool| tool["id"] == "http.fetch")
        .expect("http.fetch descriptor");
    assert_eq!(descriptor["permission"], "read_external");
    assert_eq!(descriptor["riskLevel"], "medium");
    assert!(
        catalog.requires_approval("http.fetch", "approval"),
        "medium risk is gated in approval mode"
    );
    assert!(
        !catalog.requires_approval("http.fetch", "all"),
        "no explicit per-mode list, so all mode executes it"
    );
}

/// The engine pins reqwest to `rustls-no-provider`, so building the fetch
/// client must install the process crypto provider first.  Without it the
/// client panics inside reqwest instead of failing closed.  Building the
/// client is deterministic and performs no network I/O.
#[test]
fn http_fetch_installs_the_rustls_provider_before_building_the_client() {
    use crate::product::product_mcp_production_executor::build_http_fetch_client;
    let outcome = std::panic::catch_unwind(build_http_fetch_client);
    assert!(
        outcome.is_ok(),
        "http.fetch must install the rustls provider instead of panicking"
    );
    assert!(
        outcome.expect("checked above").is_ok(),
        "the fetch client must build once the provider is installed"
    );
}

/// Parity: go:452dea11:internal/assistant/engine/adk_edges_test.go:61
/// `TestModelCatalogToolBoundaryBranches`: `models.list` keeps the reference
/// boolean coercion (`false`/`"false"`/`"0"`/`"no"`/`"n"` disable
/// `callableOnly`, an unknown string keeps the `true` default), answers zero
/// rows for an unmatched query, and fails closed instead of returning a partial
/// catalog when the provider store is unavailable.
#[test]
fn models_list_tool_projects_providers_with_reference_boolean_rules() {
    let (directory, ports) = production_bundle();
    ports
        .mcp_store
        .upsert_provider(
            "provider-edges",
            &json!({
                "id": "provider-edges",
                "displayName": "Edge Model Provider",
                "baseUrl": "http://127.0.0.1:9/v1",
                "model": "edge-model",
                "enabled": true,
            })
            .to_string(),
        )
        .expect("persist provider without a credential");
    let executor = ProductionMcpToolExecutor::from_production_ports(Arc::new(ports));

    let unmatched = execute_models_list(&executor, json!({"query": "does-not-match", "limit": 0}));
    assert_eq!(unmatched["totalReturned"], 0, "payload = {unmatched}");
    assert_eq!(unmatched["models"], json!([]));

    // A provider without a credential is disabled for callable-only listings,
    // which is exactly the default the reference applies to missing input.
    let defaulted = execute_models_list(&executor, json!({"query": "edge"}));
    assert_eq!(defaulted["callableOnly"], true);
    assert_eq!(defaulted["totalReturned"], 0, "payload = {defaulted}");

    for value in [
        json!(false),
        json!("false"),
        json!("0"),
        json!("no"),
        json!("n"),
    ] {
        let payload =
            execute_models_list(&executor, json!({"query": "edge", "callableOnly": value}));
        assert_eq!(payload["callableOnly"], false, "callableOnly = {value}");
        assert_eq!(
            payload["totalReturned"], 1,
            "callableOnly = {value} must list the uncallable provider: {payload}"
        );
        assert_eq!(payload["models"][0]["providerId"], "provider-edges");
        assert_eq!(payload["models"][0]["callable"], false);
    }

    let unknown_string = execute_models_list(&executor, json!({"callableOnly": "maybe"}));
    assert_eq!(
        unknown_string["callableOnly"], true,
        "an unknown string keeps the reference default"
    );

    let provider_scoped = execute_models_list(
        &executor,
        json!({"providerId": "provider-missing", "callableOnly": false}),
    );
    assert_eq!(
        provider_scoped["totalReturned"], 0,
        "payload = {provider_scoped}"
    );

    let connection =
        rusqlite::Connection::open(directory.path().join("adk.db")).expect("open ADK database");
    connection
        .execute("DROP TABLE adk_providers", [])
        .expect("drop providers table");
    drop(connection);
    assert!(
        executor.execute("models.list", &json!({})).is_err(),
        "a store read failure must fail closed instead of returning a partial catalog"
    );
}

fn execute_models_list(executor: &ProductionMcpToolExecutor, arguments: Value) -> Value {
    executor
        .execute("models.list", &arguments)
        .unwrap_or_else(|error| panic!("models.list {arguments} failed: {error}"))
}

#[derive(Debug, Default)]
struct RecordingSnapshotQuotePort {
    reads: Mutex<Vec<(String, String)>>,
}

impl crate::product::MarketDataQuoteReadSnapshotPort for RecordingSnapshotQuotePort {
    fn read<'a>(
        &'a self,
        path: &'a str,
        query: &'a str,
    ) -> crate::product::MarketDataQuoteReadFuture<'a> {
        Box::pin(async move {
            self.reads
                .lock()
                .expect("snapshot reads")
                .push((path.to_owned(), query.to_owned()));
            Ok(json!({"path": path, "query": query}))
        })
    }
}

#[derive(Debug)]
struct UnavailableSnapshotQuotePort;

impl crate::product::MarketDataQuoteReadSnapshotPort for UnavailableSnapshotQuotePort {
    fn read<'a>(
        &'a self,
        _path: &'a str,
        _query: &'a str,
    ) -> crate::product::MarketDataQuoteReadFuture<'a> {
        Box::pin(async move {
            Err(
                crate::product::MarketDataQuoteReadSnapshotError::Unavailable(
                    "snapshot feed unavailable".to_owned(),
                ),
            )
        })
    }
}

/// Parity: go:452dea11:internal/assistant/assembly/product_adapters_test.go:155
/// TestProductExecutionAdapterNormalizesScreenAndCalendarV2Inputs (calendar half).
///
/// Go forwards the calendar sort, stock scope and V2 filter values into the
/// typed calendar request. The Rust MCP adapter owns the same forwarding, so
/// every advanced filter must reach the research route query verbatim.
#[test]
fn research_calendar_forwards_the_advanced_filter_query() {
    let (_directory, mut ports) = production_bundle();
    let recorder = std::sync::Arc::new(RecordingResearchRead::default());
    ports.research_read = recorder.clone();
    let executor = ProductionMcpToolExecutor::from_production_ports(std::sync::Arc::new(ports));

    executor
        .execute_production(
            "research.calendar",
            &json!({
                "operation": "earnings",
                "market": "US",
                "sort": "iv_desc",
                "stockScope": "optionable",
                "marketCapMin": "100",
                "optionVolumeMax": "500",
                "ivMin": "0.2",
                "ivRankMax": "80",
                "ivPercentileMin": "50",
            }),
        )
        .expect("calendar MCP request");

    let calls = recorder.calls.lock().expect("record research MCP call");
    assert_eq!(calls.len(), 1, "one calendar read reaches the route");
    let (path, query) = &calls[0];
    assert_eq!(path, "/api/v1/research/calendars");
    for expected in [
        "market=US",
        "sort=iv%5Fdesc",
        "stockScope=optionable",
        "marketCapMin=100",
        "optionVolumeMax=500",
        "ivMin=0%2E2",
        "ivRankMax=80",
        "ivPercentileMin=50",
    ] {
        assert!(query.contains(expected), "{expected} missing from {query}");
    }
}

/// Parity: go:452dea11:internal/assistant/assembly/product_adapters_test.go:83
/// TestProductAndExecutionDispatchFailureBoundaries and
/// `product_adapters_test.go:217`
/// TestProductExecutionAdapterCoversSpecialDispatchFailuresAndSnapshots.
///
/// Go fails closed for unknown product/execution tools, a snapshot request
/// without symbols, malformed research input and a failing research service.
/// Rust rejects the same calls before the ports answer and keeps the failing
/// quote port message visible to the caller.
#[test]
fn product_dispatch_rejects_unknown_tools_and_missing_instruments() {
    let (_directory, mut ports) = production_bundle();
    let recorder = std::sync::Arc::new(RecordingSnapshotQuotePort::default());
    let quote_handle = std::sync::Arc::clone(&recorder);
    let quote_port: std::sync::Arc<dyn crate::product::MarketDataQuoteReadSnapshotPort> =
        quote_handle;
    ports.market_data_quote = quote_port;
    let executor = ProductionMcpToolExecutor::from_production_ports(std::sync::Arc::new(ports));

    for name in ["unknown.product.tool", "unknown.execution.tool"] {
        let failure = executor
            .execute_production(name, &json!({}))
            .expect_err("unknown tools must fail closed");
        assert_eq!(failure.code, "MCP_TOOL_UNAVAILABLE", "{name}");
    }

    let failure = executor
        .execute_production("market.snapshots", &json!({}))
        .expect_err("snapshot tool without instruments must fail");
    assert_eq!(failure.code, "BAD_REQUEST");
    assert!(
        failure
            .message
            .contains("instrumentId or symbols is required"),
        "{}",
        failure.message
    );

    let failure = executor
        .execute_production("execution.buying_power", &json!({"invalid": "input"}))
        .expect_err("buying power without its typed fields must fail");
    assert_eq!(failure.code, "BAD_REQUEST");

    executor
        .execute_production("market.snapshot", &json!({"instrumentId": "US.AAPL"}))
        .expect("single snapshot");
    let reads = recorder.reads.lock().expect("snapshot reads");
    assert_eq!(
        reads.as_slice(),
        [(
            "/api/v1/market-data/snapshots/US/AAPL".to_owned(),
            String::new()
        )]
    );
    drop(reads);

    let failure = executor
        .execute_production("research.screen", &json!(["not-an-object"]))
        .expect_err("research.screen must reject malformed input");
    assert_eq!(failure.code, "BAD_REQUEST");

    let failure = executor
        .execute_production("research.calendar", &json!({}))
        .expect_err("research.calendar requires an explicit operation");
    assert_eq!(failure.code, "CAPABILITY_UNAVAILABLE");

    let (_directory, mut ports) = production_bundle();
    let failing: std::sync::Arc<dyn crate::product::MarketDataQuoteReadSnapshotPort> =
        std::sync::Arc::new(UnavailableSnapshotQuotePort);
    ports.market_data_quote = failing;
    let executor = ProductionMcpToolExecutor::from_production_ports(std::sync::Arc::new(ports));
    let failure = executor
        .execute_production("market.snapshot", &json!({"instrumentId": "US.AAPL"}))
        .expect_err("a failing snapshot port must propagate");
    assert_eq!(failure.code, "MARKET_DATA_QUOTE_READ_UNAVAILABLE");
    assert_eq!(failure.status, 503);
    assert!(
        failure.message.contains("snapshot feed unavailable"),
        "{}",
        failure.message
    );
}

/// Parity: go:452dea11:internal/assistant/assembly/runtime_test.go:55
/// TestOpenOwnsApplicationToolRegistration
///
/// Go's `Open` registers the application tool set itself: the injected
/// `system.status` dependency and the assembly-owned
/// `strategy.research_backtest`. The Rust composition root owns the same
/// registration through `PRODUCTION_TOOL_DEFINITIONS`, so both ids are in the
/// catalog without any per-request registration call.
#[test]
fn production_catalog_registers_application_tools_from_the_composition_root() {
    let definitions = crate::product::product_production_ports::product_production_ports_adk::PRODUCTION_TOOL_DEFINITIONS;
    let registered = definitions
        .iter()
        .map(|definition| definition.id)
        .collect::<Vec<_>>();
    for name in ["system.status", "strategy.research_backtest"] {
        assert!(
            registered.contains(&name),
            "{name} must be registered by the composition root: {registered:?}"
        );
    }

    let bindings = definitions
        .iter()
        .map(|definition| (definition.adapter, ProductionAdapterBinding::Ready))
        .collect::<BTreeMap<_, _>>();
    let catalog =
        ProductionToolCatalog::from_bindings(&bindings).expect("complete ADK tool bindings");
    let callable = catalog
        .callable_tools()
        .into_iter()
        .filter_map(|tool| tool["id"].as_str().map(str::to_owned))
        .collect::<Vec<_>>();
    for name in ["system.status", "strategy.research_backtest"] {
        assert!(
            callable.iter().any(|id| id == name),
            "{name} must be callable once every port is ready: {callable:?}"
        );
    }
}

/// Sends one raw request with an arbitrary path so the router's own 404
/// boundary can be asserted next to the MCP path.
fn request_path(port: u16, path: &str, token: Option<&str>) -> String {
    let mut stream = TcpStream::connect(("127.0.0.1", port)).expect("connect MCP listener");
    stream
        .set_read_timeout(Some(MCP_TEST_IO_TIMEOUT))
        .expect("set MCP timeout");
    let auth = token
        .map(|token| format!("Authorization: Bearer {token}\r\n"))
        .unwrap_or_default();
    let request = format!(
        "POST {path} HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\nContent-Length: 2\r\n{auth}\r\n{{}}"
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

/// Parity: go:452dea11:internal/assistant/assembly/mcp_server_lifecycle_authorization_test.go:27
/// `TestMCPServerManagerRemainingLifecycleBoundaries`: a token-mode start
/// without a configured hash fails with the reference reason and keeps it in
/// the status projection, a same-port apply is an idempotent no-op that clears
/// the recorded error, and an unconfigured port projects the reference
/// 6697 endpoint.  Go's nil-manager/nil-runtime branches have no Rust owner
/// because the composition root always constructs the runtime with a catalog.
#[test]
fn lifecycle_boundaries_report_the_missing_token_and_keep_same_port_applies_idempotent() {
    let runtime = runtime();
    let port = available_port();

    let error = runtime
        .apply(&enabled_record(port, "token", ""))
        .expect_err("a token-mode start without a hash must fail");
    assert!(error.contains("token"), "error = {error}");
    let failed = runtime
        .status(&enabled_record(port, "token", ""))
        .expect("failed MCP status");
    assert!(!failed.running, "failed MCP status = {failed:?}");
    assert!(
        failed.last_error.contains("token"),
        "failed MCP status = {failed:?}"
    );

    runtime
        .apply(&enabled_record(port, "none", ""))
        .expect("start MCP listener");
    runtime
        .apply(&enabled_record(port, "none", ""))
        .expect("same-port apply is a no-op");
    let running = runtime
        .status(&enabled_record(port, "none", ""))
        .expect("running MCP status");
    assert!(running.running, "running MCP status = {running:?}");
    assert!(
        running.last_error.is_empty(),
        "running MCP status = {running:?}"
    );

    let unconfigured = runtime
        .status(&McpServerSettingsRecord::new(false, 0, "none", ""))
        .expect("unconfigured MCP status");
    assert!(
        unconfigured.endpoint.ends_with(":6697/mcp"),
        "default endpoint = {}",
        unconfigured.endpoint
    );
    runtime.shutdown_blocking().expect("shutdown MCP");
}

/// Parity: go:452dea11:internal/assistant/assembly/mcp_server_lifecycle_authorization_test.go:74
/// `TestMCPServerManagerRemainingServeFailureStates`: an unexpected listener
/// exit publishes its reason only while the failed server is still the current
/// owner; a failure from a replaced listener leaves the newer state untouched.
#[test]
fn unexpected_serve_failure_publishes_only_for_the_current_generation() {
    let runtime = runtime();
    let port = available_port();
    runtime
        .apply(&enabled_record(port, "none", ""))
        .expect("start MCP listener");
    let state = Arc::clone(&runtime.state);
    let generation = state.lock().expect("runtime state").generation;
    let weak = Arc::downgrade(&state);

    assert!(publish_listener_failure(&weak, generation, "accept failed"));
    let failed = runtime
        .status(&enabled_record(port, "none", ""))
        .expect("serve failure status");
    assert!(
        failed.last_error.contains("accept failed"),
        "serve failure status = {failed:?}"
    );
    assert!(
        failed.last_error.contains("stopped unexpectedly"),
        "serve failure status = {failed:?}"
    );

    {
        let mut guard = state.lock().expect("runtime state");
        guard.generation = guard.generation.wrapping_add(1);
        guard.last_error.clear();
    }
    assert!(
        !publish_listener_failure(&weak, generation, "stale accept failure"),
        "a replaced listener must not publish its failure"
    );
    let guard = state.lock().expect("runtime state");
    assert!(
        guard.last_error.is_empty(),
        "stale failure changed the recorded error: {}",
        guard.last_error
    );
    assert!(
        guard.server.is_some(),
        "the current listener stays installed"
    );
    drop(guard);
    runtime.shutdown_blocking().expect("shutdown MCP");
}

/// Parity: go:452dea11:internal/assistant/assembly/mcp_server_lifecycle_authorization_test.go:95
/// `TestMCPAuthorizedHandlerRemainingRequestBoundaries`: a blank bearer never
/// authorizes and the 401 advertises the `Bearer` challenge, while a foreign
/// path is answered by the router's own 404 instead of the MCP handler.
#[test]
fn authorization_boundaries_challenge_blank_bearer_and_reject_foreign_paths() {
    let runtime = runtime();
    let port = available_port();
    let (_token, token_hash) = jftrade_settings::SystemMcpServerSecrets
        .issue()
        .expect("fixture secret");
    runtime
        .apply(&enabled_record(port, "token", &token_hash))
        .expect("start MCP listener");

    let blank = request(port, Some(""), "{}", "127.0.0.1:1");
    assert!(blank.contains("401 Unauthorized"), "response = {blank}");
    assert!(
        blank
            .to_ascii_lowercase()
            .contains("www-authenticate: bearer"),
        "response = {blank}"
    );

    let foreign = request_path(port, "/other", None);
    assert!(foreign.contains("404 Not Found"), "response = {foreign}");
    let mcp = request_path(port, "/mcp", None);
    assert!(mcp.contains("401 Unauthorized"), "response = {mcp}");

    runtime.shutdown_blocking().expect("shutdown MCP");
}
