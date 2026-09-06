use super::*;
use crate::product::product_mcp_protocol::{mcp_tool_adapter, mcp_tool_availability};
use crate::product::product_mcp_server::dispatch::normalize_legacy_mcp_arguments;
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
