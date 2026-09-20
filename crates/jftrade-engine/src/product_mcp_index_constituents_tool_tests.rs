//! Executor coverage for the tool-only `market.index_constituents` capability.
//!
//! Owner: `internal/assistant/assembly/market_index_constituents_tools_test.go`
//! on the `go` branch.  The reference registers the tool in the ADK registry
//! without a public HTTP route, so the Rust owner is the production MCP
//! executor plus the AKShare-owned read port.

use super::tests::production_bundle;
use crate::product::product_mcp_production_executor::ProductionMcpToolExecutor;
use crate::product::product_production_ports::{
    ProductionAdapterBinding, ProductionPortBundle, ProductionToolCatalog,
};
use crate::product::{MarketIndexConstituentsReadError, MarketIndexConstituentsReadPort};
use jftrade_settings::MarketDataProvider;
use jftrade_settings::MarketDataProviderRuntimePort;
use serde_json::{Value, json};
use std::sync::{Arc, Mutex};

/// Records the normalized request and answers either the reference payload or
/// a scripted failure, so the tool boundary is asserted without a live helper.
#[derive(Debug)]
struct RecordingIndexConstituents {
    reads: Mutex<Vec<(String, String, usize)>>,
    failure: Option<MarketIndexConstituentsReadError>,
}

impl RecordingIndexConstituents {
    fn answering() -> Self {
        Self {
            reads: Mutex::new(Vec::new()),
            failure: None,
        }
    }

    fn failing(error: MarketIndexConstituentsReadError) -> Self {
        Self {
            reads: Mutex::new(Vec::new()),
            failure: Some(error),
        }
    }

    fn recorded(&self) -> Vec<(String, String, usize)> {
        self.reads.lock().expect("index constituent reads").clone()
    }
}

impl MarketIndexConstituentsReadPort for RecordingIndexConstituents {
    fn read(
        &self,
        market: &str,
        symbol: &str,
        limit: usize,
    ) -> Result<Value, MarketIndexConstituentsReadError> {
        self.reads.lock().expect("index constituent reads").push((
            market.to_owned(),
            symbol.to_owned(),
            limit,
        ));
        match self.failure.clone() {
            Some(error) => Err(error),
            None => Ok(json!({
                "market": market,
                "symbol": symbol,
                "instrumentId": format!("{market}.{symbol}"),
                "constituents": [
                    {"code": "600519", "name": "贵州茅台", "weight": null},
                ],
                "source": "akshare-index-constituents",
            })),
        }
    }
}

fn bundle_with_index_constituents(
    port: Arc<dyn MarketIndexConstituentsReadPort>,
) -> (tempfile::TempDir, Arc<ProductionPortBundle>) {
    let (directory, mut ports) = production_bundle();
    ports.market_index_constituents = port;
    (directory, Arc::new(ports))
}

/// Parity: go:452dea11:internal/assistant/assembly/market_index_constituents_tools_test.go:14
/// `TestADKMarketIndexConstituentsToolForwardsNormalizedInputs`: the tool
/// requires an instrument, bounds `limit` to 1..1000 with a 200 default,
/// upper-cases the market, and returns the projected index member list.
#[test]
fn market_index_constituents_tool_normalizes_its_inputs_before_the_read_port() {
    let recorder = Arc::new(RecordingIndexConstituents::answering());
    let (_directory, ports) = bundle_with_index_constituents(recorder.clone());
    let executor = ProductionMcpToolExecutor::from_production_ports(Arc::clone(&ports));

    let missing = executor
        .execute_production("market.index_constituents", &json!({}))
        .expect_err("a call without market and symbol must fail");
    assert_eq!(missing.status, 400);
    assert_eq!(missing.code, "BAD_REQUEST");

    let above_range = executor
        .execute_production(
            "market.index_constituents",
            &json!({"market": "SH", "symbol": "000300", "limit": 1001}),
        )
        .expect_err("limit above the reviewed maximum must fail");
    assert_eq!(above_range.status, 400);
    assert!(
        above_range.message.contains("between 1 and 1000"),
        "message = {}",
        above_range.message
    );
    let below_range = executor
        .execute_production(
            "market.index_constituents",
            &json!({"market": "SH", "symbol": "000300", "limit": 0}),
        )
        .expect_err("limit below the reviewed minimum must fail");
    assert_eq!(below_range.status, 400);

    let value = executor
        .execute_production(
            "market.index_constituents",
            &json!({"market": "sh", "symbol": "000300", "limit": "300"}),
        )
        .expect("normalized index constituents read");
    assert_eq!(value["instrumentId"], "SH.000300");
    assert!(value["constituents"][0]["weight"].is_null());

    executor
        .execute_production(
            "market.index_constituents",
            &json!({"market": "SH", "symbol": "000300"}),
        )
        .expect("default limit read");

    assert_eq!(
        recorder.recorded(),
        [
            ("SH".to_owned(), "000300".to_owned(), 300),
            ("SH".to_owned(), "000300".to_owned(), 200),
        ]
    );
}

/// The same reference case also pins the descriptor: `read_internal`/low with
/// no per-mode confirmation list, the `market` category, and membership in the
/// `jftrade-market` skill (Go's `RequiredSkills`).
#[test]
fn market_index_constituents_descriptor_keeps_the_reviewed_policy_and_skill() {
    let definitions =
        crate::product::product_production_ports::product_production_ports_adk::PRODUCTION_TOOL_DEFINITIONS;
    let bindings = definitions
        .iter()
        .map(|definition| (definition.adapter, ProductionAdapterBinding::Ready))
        .collect::<std::collections::BTreeMap<_, _>>();
    let catalog = ProductionToolCatalog::from_bindings(&bindings).expect("catalog bindings");
    let descriptor = catalog
        .callable_tools()
        .into_iter()
        .find(|tool| tool["id"] == "market.index_constituents")
        .expect("market.index_constituents descriptor");
    assert_eq!(descriptor["category"], "market");
    assert_eq!(descriptor["permission"], "read_internal");
    assert_eq!(descriptor["riskLevel"], "low");
    assert_eq!(descriptor["requiresApprovalIn"], json!([]));
    let tool = catalog
        .openai_tools()
        .into_iter()
        .find(|tool| tool["name"] == "market.index_constituents")
        .expect("market.index_constituents function tool");
    assert_eq!(tool["parameters"]["type"], "object");
    assert_eq!(tool["parameters"]["additionalProperties"], false);
    assert_eq!(tool["parameters"]["properties"]["limit"]["default"], 200);
    assert_eq!(
        tool["parameters"]["required"],
        json!(["market", "symbol"]),
        "the reviewed schema requires market and symbol"
    );

    let (_directory, ports) = production_bundle();
    let skills = match ports
        .adk_read
        .read("/api/v1/adk/skills", "")
        .expect("read builtin skills")
    {
        crate::product::AdkReadSnapshot::Json(value) => {
            value["skills"].as_array().expect("skills array").clone()
        }
        crate::product::AdkReadSnapshot::Stream(_) => panic!("expected skills JSON"),
    };
    let market_skill = skills
        .iter()
        .find(|skill| skill["id"] == "jftrade-market")
        .expect("jftrade-market builtin skill");
    assert!(
        market_skill["tools"]
            .as_array()
            .expect("market skill tools")
            .iter()
            .any(|tool| tool == "market.index_constituents"),
        "the market skill owns the index constituent tool: {}",
        market_skill["tools"]
    );
}

/// Parity: go:452dea11:internal/assistant/assembly/market_index_constituents_tools_test.go:60
/// `TestADKMarketIndexConstituentsToolFailsClosedWithoutPort`: without the
/// owner port the tool answers a structured failure instead of an empty list.
#[test]
fn market_index_constituents_tool_fails_closed_without_an_owner_port() {
    let (_directory, ports) = production_bundle();
    let unwired = ProductionMcpToolExecutor::new(
        Arc::clone(&ports.mcp_catalog),
        Arc::clone(&ports.mcp_store),
    );
    let failure = unwired
        .execute_production(
            "market.index_constituents",
            &json!({"market": "SH", "symbol": "000300"}),
        )
        .expect_err("a listener without the production bundle must fail closed");
    assert_eq!(failure.status, 503);
    assert_eq!(failure.code, "MCP_PRODUCTION_EXECUTOR_UNAVAILABLE");

    // A wired bundle that has not selected a market-data provider still fails
    // closed: the empty result is never presented as a valid member list.
    let unconfigured = ProductionMcpToolExecutor::from_production_ports(Arc::new(ports));
    let failure = unconfigured
        .execute_production(
            "market.index_constituents",
            &json!({"market": "SH", "symbol": "000300"}),
        )
        .expect_err("an unconfigured provider must fail closed");
    assert_eq!(failure.status, 503);
    assert_eq!(failure.code, "MARKET_INDEX_CONSTITUENTS_UNAVAILABLE");
}

/// Parity: go:452dea11:internal/assistant/assembly/market_index_constituents_tools_test.go:69
/// `TestADKMarketIndexConstituentsToolSurfacesProviderCapabilityAsClearMessage`:
/// a provider without the feed reports the capability class and names the
/// active provider instead of a generic failure.
#[test]
fn market_index_constituents_tool_surfaces_the_provider_capability() {
    let scripted = Arc::new(RecordingIndexConstituents::failing(
        MarketIndexConstituentsReadError::Failed {
            status: 409,
            code: "MARKET_DATA_CAPABILITY_UNSUPPORTED".to_owned(),
            message: "market-data capability is unsupported: active provider \"futu-opend\" does not support index constituents".to_owned(),
            retry_after_seconds: None,
        },
    ));
    let (_directory, ports) = bundle_with_index_constituents(scripted);
    let executor = ProductionMcpToolExecutor::from_production_ports(Arc::clone(&ports));
    let failure = executor
        .execute_production(
            "market.index_constituents",
            &json!({"market": "SH", "symbol": "000300"}),
        )
        .expect_err("a provider without the feed must not answer a payload");
    assert_eq!(failure.status, 409);
    assert_eq!(failure.code, "MARKET_DATA_CAPABILITY_UNSUPPORTED");
    assert!(
        failure.message.contains("futu-opend"),
        "message = {}",
        failure.message
    );
    assert_eq!(failure.retry_after_seconds, None);
}

/// The same capability answer has to survive the real composition adapter:
/// activating Futu makes the AKShare-owned port reject the call with the
/// reference provider label, so the console never sees an empty member list.
#[test]
fn market_index_constituents_tool_reports_the_active_futu_owner() {
    let (_directory, ports) = production_bundle();
    let ports = Arc::new(ports);
    ports
        .active_provider_state
        .activate(MarketDataProvider::Futu)
        .expect("activate Futu");
    let executor = ProductionMcpToolExecutor::from_production_ports(Arc::clone(&ports));

    let failure = executor
        .execute_production(
            "market.index_constituents",
            &json!({"market": "SH", "symbol": "000300"}),
        )
        .expect_err("Futu has no index constituent feed");
    assert_eq!(failure.status, 409);
    assert_eq!(failure.code, "MARKET_DATA_CAPABILITY_UNSUPPORTED");
    assert!(
        failure.message.contains("futu-opend"),
        "message = {}",
        failure.message
    );
    assert!(
        !executor.supports("market.index_constituents") || failure.status == 409,
        "the executor keeps the capability answer even while Futu is active"
    );
}
