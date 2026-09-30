use super::*;
use std::collections::{BTreeMap, BTreeSet};
use serde_json::json;
use std::sync::Arc;

/// Parity: go:452dea11:pkg/futu/adapter_advanced_test.go:92
/// TestEveryAllowlistedAdvancedProtocolMapsToCatalogFeature
///
/// Go fails when an allowlisted OpenD protocol has no `CapabilityCatalog`
/// feature, or when the same protocol is claimed by two features. Rust
/// keeps one protocol table in the capability catalog, so the equivalent
/// invariant is that every feature exposes at least one operation, every
/// operation id is unique inside its feature, and no protocol key is
/// registered twice with different OpenD protocol ids.
///
/// Go's "one protocol belongs to exactly one feature" rule cannot be
/// copied verbatim: Rust deliberately shares a protocol across features
/// where the wire call is identical but the caller contract differs
/// (`Qot_GetSecuritySnapshot` for `market.snapshot(s)`; the option-strategy
/// family for `derivatives.option_analysis` and `execution.combo_preview`;
/// `Trd_UpdateOrder(Fill)` pushes for single and combo placement).
#[test]
fn every_catalog_protocol_maps_to_a_feature_with_one_stable_id() {
    let mut protocol_ids: BTreeMap<String, u32> = BTreeMap::new();
    let mut shared: BTreeSet<String> = BTreeSet::new();
    let mut features = 0_usize;
    let mut protocols = 0_usize;
    let mut verified_framed: BTreeSet<String> = BTreeSet::new();
    for spec in FEATURE_SPECS {
        features += 1;
        let operations = operations::catalog_operations(spec.id, spec.method, spec.api, spec.ui, spec.tool);
        assert!(
            !operations.is_empty(),
            "{} must expose at least one capability operation",
            spec.id
        );
        let mut operation_ids = BTreeSet::new();
        for operation in &operations {
            let id = operation["id"].as_str().expect("operation id");
            assert!(
                operation_ids.insert(id.to_owned()),
                "{} declares duplicate operation {id}",
                spec.id
            );
            let Some(list) = operation.get("protocols").and_then(Value::as_array) else {
                continue;
            };
            for entry in list {
                let key = entry["key"].as_str().expect("protocol key");
                let protocol_id = entry["id"].as_u64().expect("protocol id") as u32;
                assert_ne!(protocol_id, 0, "{key} has no OpenD protocol id");
                assert!(
                    key.starts_with("Qot_") || key.starts_with("Trd_"),
                    "{key} is not a namespaced OpenD protocol"
                );
                // A catalog protocol the Rust adapter can actually frame
                // must agree with the generated OpenD protocol id; a
                // typo here would otherwise silently describe a protocol
                // that the integration crate cannot send.
                if let Some(framed) =
                    jftrade_integration_futu::trade_proto::framed_protocol_id(key)
                {
                    assert_eq!(
                        protocol_id, framed,
                        "{key} catalog id disagrees with the generated protocol id"
                    );
                    verified_framed.insert(key.to_owned());
                }
                protocols += 1;
                match protocol_ids.get(key) {
                    Some(existing) => {
                        assert_eq!(
                            *existing, protocol_id,
                            "{key} is registered with two different OpenD ids"
                        );
                        shared.insert(key.to_owned());
                    }
                    None => {
                        protocol_ids.insert(key.to_owned(), protocol_id);
                    }
                }
            }
        }
    }
    assert_eq!(features, FEATURE_SPECS.len());
    // Guard the shape of the table: if a refactor drops protocol metadata
    // the "every protocol maps to a feature" claim silently becomes vacuous.
    assert_eq!(
        protocols, 150,
        "catalog protocol references shrank unexpectedly"
    );
    // The catalog and the framed integration layer must overlap: the ids
    // are only verified for protocols the adapter can actually encode.
    // The catalog and the framed integration layer must keep overlapping:
    // ids are only cross-checked for protocols the adapter can encode.
    // 63 of the 66 framed protocols are catalog entries; the three
    // leftovers (`Qot_GetKl`, `Qot_GetRt`,
    // `Qot_RequestHistoryEventContractKl`) are served by their own typed
    // readers instead of the capability catalog.
    assert_eq!(
        verified_framed.len(),
        63,
        "catalog/generated protocol overlap changed"
    );
    assert_eq!(
        protocol_ids.len(),
        139,
        "catalog distinct protocol keys changed"
    );
    // Ten protocol keys intentionally serve two features: the snapshot
    // read for `market.snapshot(s)`, `Qot_GetStaticInfo` for
    // `market.instrument_profile` and the `research.rankings/fund_catalog`
    // operation, the option-strategy family for `derivatives.option_analysis`
    // and `execution.combo_preview`, and the trade quantity/update protocols
    // for single vs combo execution. Any other sharing is a catalog mistake.
    assert_eq!(
        shared,
        BTreeSet::from([
            "Qot_GetOptionStrategy".to_owned(),
            "Qot_GetOptionStrategyAnalysis".to_owned(),
            "Qot_GetOptionStrategySpread".to_owned(),
            "Qot_GetSecuritySnapshot".to_owned(),
            "Qot_GetStaticInfo".to_owned(),
            "Trd_GetComboMaxTrdQtys".to_owned(),
            "Trd_GetMaxTrdQtys".to_owned(),
            "Trd_ModifyOrder".to_owned(),
            "Trd_UpdateOrder".to_owned(),
            "Trd_UpdateOrderFill".to_owned(),
        ]),
        "shared protocol set changed"
    );
}

/// A provider snapshot that claims a ready Futu OpenD session. The
/// projection still derives every concrete reader from the runtime, so tests
/// compose exactly the readers under assertion.
fn ready_provider() -> ProviderRuntimeSnapshot {
    ProviderRuntimeSnapshot {
        provider: Some(jftrade_settings::MarketDataProvider::Futu),
        opend_ready: true,
        ..Default::default()
    }
}

/// The declared (static) features the capability endpoint publishes for the
/// given runtime snapshot.
fn descriptor_features(
    runtime: &Arc<SharedTradeReadRuntime>,
    provider: &ProviderRuntimeSnapshot,
) -> Vec<Value> {
    project(runtime, provider, "")
        .expect("projection")["brokers"][0]["capabilities"]
        .as_array()
        .expect("descriptor capabilities")
        .iter()
        .flat_map(|capability| {
            capability["features"]
                .as_array()
                .cloned()
                .unwrap_or_default()
        })
        .collect()
}

/// Parity: go:452dea11:internal/productfeatures/capabilities_evaluation_test.go:64
/// TestCapabilitiesContextFiltersProductsAndSegments
///
/// Go filters runtime statuses by `productClass` / `marketSegment`, and the
/// declared capability's own product/segment lists only matter when they are
/// non-empty. Rust filters per feature id through `product_classes` /
/// `market_segments`, so prediction features must be reachable only through
/// `event_contract` + `prediction`, while equity/security queries stay disjoint.
#[test]
fn capabilities_filters_products_and_segments_like_the_catalog() {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    let provider = ready_provider();

    let equity_only = project(&runtime, &provider, "productClass=equity&market=US")
        .expect("product filter projection");
    let equity_items = equity_only["runtime"].as_array().expect("runtime items");
    assert!(
        equity_items
            .iter()
            .all(|item| item["featureId"] != "prediction.depth"),
        "prediction features must not satisfy the equity product filter"
    );
    assert!(
        equity_items.iter().any(|item| item["featureId"] == "market.depth"),
        "equity filter must still expose the market-data reads"
    );

    let prediction_segment = project(
        &runtime,
        &provider,
        "productClass=event_contract&marketSegment=securities&market=US",
    )
    .expect("mismatched segment projection");
    assert!(
        prediction_segment["runtime"]
            .as_array()
            .expect("runtime items")
            .iter()
            .all(|item| item["featureId"] != "prediction.depth"),
        "the prediction product must not appear under the securities segment"
    );

    let matching = project(
        &runtime,
        &provider,
        "productClass=event_contract&marketSegment=prediction&market=US",
    )
    .expect("matching filter projection");
    let matching_items = matching["runtime"].as_array().expect("runtime items");
    assert!(
        matching_items
            .iter()
            .any(|item| item["featureId"] == "prediction.depth"),
        "event_contract + prediction must expose the prediction depth capability"
    );
    assert!(
        matching_items
            .iter()
            .all(|item| item["market"] == "US"),
        "prediction capabilities are US-only"
    );
}

/// Parity: go:452dea11:internal/productfeatures/capabilities_evaluation_test.go:11
/// TestCapabilitiesContextFiltersAndReportsRuntimeEvaluation
///
/// Go resolves one adapter per descriptor, filters by broker/market/feature id,
/// and surfaces the evaluator's own code. Rust has a single Futu catalog, so the
/// equivalent observable behavior is that an unknown broker id yields an empty
/// runtime list while a feature-id filter narrows the statuses to that feature
/// and keeps the per-market evaluation attached.
#[test]
// Parity: go:452dea11:internal/productfeatures/service_routing_and_validation_test.go:14 TestProductFeatureServiceRemainingRoutingAndDegradationBranches
fn capabilities_filters_by_broker_market_and_feature_id() {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    let provider = ready_provider();

    let other_broker = project(&runtime, &provider, "brokerId=other").expect("broker filter");
    assert_eq!(other_broker["brokers"], json!([]));
    assert_eq!(other_broker["runtime"], json!([]));
    assert!(
        !other_broker["catalog"]["features"]
            .as_array()
            .expect("catalog features")
            .is_empty(),
        "the catalog itself stays discoverable for an unmatched broker filter"
    );

    let hk_only = project(&runtime, &provider, "market=HK&featureId=prediction.depth")
        .expect("market filter");
    assert_eq!(
        hk_only["runtime"],
        json!([]),
        "prediction capabilities are not offered for HK"
    );

    let filtered = project(&runtime, &provider, "featureId=prediction.depth").expect("feature filter");
    let items = filtered["runtime"].as_array().expect("runtime items");
    assert_eq!(items.len(), 1, "prediction depth is a single US capability");
    assert_eq!(items[0]["featureId"], "prediction.depth");
    assert_eq!(items[0]["market"], "US");
    assert_eq!(items[0]["brokerId"], "futu");
    // The declared descriptor keeps the static eligibility declaration; the
    // runtime entry overwrites `reasonCode` with the live evaluation verdict, so
    // a missing PredictionMarketReader must never report the static code as if
    // the capability were merely waiting for an eligible account.
    assert_eq!(
        descriptor_features(&runtime, &provider)
            .into_iter()
            .find(|feature| feature["id"] == "prediction.depth")
            .expect("descriptor prediction.depth")["reasonCode"],
        "RUNTIME_ELIGIBILITY_REQUIRED",
        "the declared capability keeps the eligibility reason"
    );
    assert_eq!(
        items[0]["capability"]["state"], "unavailable",
        "without a prediction reader the capability is unavailable"
    );
    assert_eq!(
        items[0]["capability"]["reasonCode"], "CAPABILITY_UNAVAILABLE",
        "the runtime entry reports the live verdict, not the static declaration"
    );

    let market_snapshot = project(&runtime, &provider, "featureId=market.snapshot").expect("feature");
    assert!(
        market_snapshot["runtime"]
            .as_array()
            .expect("runtime items")
            .iter()
            .any(|item| item["featureId"] == "market.snapshot" && item["market"] == "HK"),
        "market snapshot stays available for HK"
    );
}

/// Parity: go:452dea11:internal/productfeatures/capabilities_evaluation_test.go:99
/// TestCapabilitiesContextMarksDeclaredButMissingInterfaceUnavailable
///
/// Go replaces the whole evaluation with `ADAPTER_INTERFACE_UNAVAILABLE` when the
/// catalog declares an adapter interface the selected broker does not compose.
/// Rust derives the same condition from its runtime readers: a Futu session that
/// has no concrete option-chain/valuation/news reader must publish the read
/// feature as `unavailable` instead of pretending the catalog entry is usable.
#[test]
// Parity: go:452dea11:internal/productfeatures/service_test.go:210 TestProductFeatureServiceFailureBoundaries
fn capabilities_mark_declared_but_missing_readers_unavailable() {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    let provider = ready_provider();

    let value = project(&runtime, &provider, "market=US").expect("projection");
    let items = value["runtime"].as_array().expect("runtime items");

    for feature_id in [
        "derivatives.option_chain",
        "research.valuation",
        "research.news",
        "market.depth",
    ] {
        let item = items
            .iter()
            .find(|item| item["featureId"] == feature_id)
            .unwrap_or_else(|| panic!("missing {feature_id}"));
        assert_eq!(
            item["evaluation"]["state"], "unavailable",
            "{feature_id} must be unavailable without its production reader"
        );
        assert_eq!(
            item["evaluation"]["quoteRight"]["code"], "CAPABILITY_UNAVAILABLE",
            "{feature_id} must name the missing concrete reader"
        );
        assert_eq!(
            item["capability"]["state"], "unavailable",
            "{feature_id} evaluated capability state"
        );
        assert_eq!(
            item["capability"]["reasonCode"], "CAPABILITY_UNAVAILABLE",
            "{feature_id} evaluated capability reason code"
        );
    }

    // The verdict is owned by the runtime composition, not by a hard-coded
    // catalog answer: composing the concrete reader moves the read dimension
    // from "missing adapter" to "entitlement not verified yet".
    let depth_before = project(&runtime, &provider, "featureId=market.depth").expect("projection");
    assert_eq!(
        depth_before["runtime"][0]["evaluation"]["quoteRight"]["code"],
        "CAPABILITY_UNAVAILABLE",
        "market.depth has no concrete microstructure reader yet"
    );

    runtime.set_market_microstructure(Some(Arc::new(NoopMicrostructureReader)));
    let depth_after = project(&runtime, &provider, "featureId=market.depth").expect("projection");
    assert_eq!(
        depth_after["runtime"][0]["evaluation"]["quoteRight"]["code"],
        "QUOTE_RIGHT_UNVERIFIED",
        "a composed reader stays degraded until the entitlement generation verifies"
    );
    assert!(
        depth_after["runtime"][0]["capability"]["state"] != "available",
        "a composed reader alone never proves the entitlement"
    );
}

#[derive(Debug)]
struct NoopMicrostructureReader;

impl jftrade_integration_futu::MarketMicrostructureReadPort for NoopMicrostructureReader {
    fn query(
        &self,
        _: jftrade_integration_futu::MarketMicrostructureOperation,
        _: &str,
        _: &Value,
    ) -> Result<Value, jftrade_integration_futu::MarketMicrostructureError> {
        Ok(json!({}))
    }
}

/// Parity: go:452dea11:internal/productfeatures/capabilities_evaluation_test.go:118
/// TestStaticRuntimeEvaluationDistinguishesRequiredDimensions
///
/// Go's `staticRuntimeEvaluation` starts from `NOT_REQUIRED` on all three runtime
/// dimensions, replaces the ones the capability declares with the degraded
/// `RUNTIME_STATUS_UNKNOWN` sentinel, and rewrites an `available` capability to
/// `degraded` + `RUNTIME_STATUS_PARTIAL` when any dimension ends up unknown. The
/// Rust projection owns the equivalent matrix: read features require a quote
/// right but not an account, trade features require an account but not a quote
/// right, and prediction features additionally declare the eligibility reason.
#[test]
fn capability_dimension_requirements_follow_access_and_product_family() {
    let value = catalog();
    let features = value["features"].as_array().expect("catalog features");
    let feature = |id: &str| {
        features
            .iter()
            .find(|feature| feature["id"] == id)
            .unwrap_or_else(|| panic!("missing catalog feature {id}"))
    };

    for id in ["market.snapshot", "research.news", "prediction.depth"] {
        assert_eq!(feature(id)["access"], "read", "{id} access");
    }
    for id in ["execution.order_place", "alerts.price.set"] {
        let access = feature(id)["access"].as_str().expect("access");
        assert!(access == "trade" || access == "write", "{id} access = {access}");
    }

    let runtime = Arc::new(SharedTradeReadRuntime::default());
    let provider = ready_provider();
    let items = project(&runtime, &provider, "")
        .expect("projection")["runtime"]
        .as_array()
        .expect("runtime items")
        .clone();

    let read_item = items
        .iter()
        .find(|item| item["featureId"] == "market.snapshot")
        .expect("market.snapshot");
    assert_eq!(read_item["capability"]["requiresConnection"], true);
    assert_eq!(
        read_item["capability"]["requiresQuoteRight"], true,
        "read features require a quote right"
    );
    assert_eq!(
        read_item["capability"]["requiresAccount"], false,
        "read features do not require an account"
    );
    assert_eq!(
        read_item["evaluation"]["account"]["code"], "NOT_REQUIRED",
        "an unrequired dimension is available/NOT_REQUIRED"
    );

    let trade_item = items
        .iter()
        .find(|item| item["featureId"] == "execution.order_place")
        .expect("execution.order_place");
    assert_eq!(
        trade_item["capability"]["requiresAccount"], true,
        "trade features require an account"
    );
    assert_eq!(
        trade_item["capability"]["requiresQuoteRight"], false,
        "trade features do not require a quote right"
    );
    assert_eq!(
        trade_item["evaluation"]["quoteRight"]["code"], "NOT_REQUIRED",
        "an unrequired dimension is available/NOT_REQUIRED"
    );

    let prediction = items
        .iter()
        .find(|item| item["featureId"] == "prediction.depth")
        .expect("prediction.depth");
    assert_eq!(
        prediction["capability"]["requiresAccount"], true,
        "prediction features declare the account/eligibility requirement"
    );
    // Go's `staticRuntimeEvaluation` keeps the declared `degraded` + reason for a
    // static projection; the Rust descriptor is that static projection, while the
    // runtime entry carries the live evaluation.
    let declared_prediction = descriptor_features(&runtime, &provider)
        .into_iter()
        .find(|feature| feature["id"] == "prediction.depth")
        .expect("declared prediction.depth");
    assert_eq!(declared_prediction["state"], "degraded");
    assert_eq!(
        declared_prediction["reasonCode"], "RUNTIME_ELIGIBILITY_REQUIRED"
    );
    assert_eq!(
        declared_prediction["requiresConnection"], true,
        "every capability declares the connection dimension"
    );
    assert_eq!(
        declared_prediction["requiresQuoteRight"], true,
        "prediction reads declare the quote-right dimension"
    );
}

/// Parity: go:452dea11:internal/assistant/assembly/product_adapters_test.go:15
/// TestCustomizationToolsMapToOpenDOperations
///
/// Go freezes the customization adapter to exactly one OpenD action per tool:
/// both alert writers map to `set` and the remote watchlist writer maps to
/// `modify`. The Rust capability catalog owns that mapping, so each feature
/// must expose one operation carrying the same action id and OpenD request.
#[test]
fn customization_tools_map_to_their_single_opend_action() {
    for (feature, action, protocol) in [
        ("alerts.price.set", "set", "Qot_SetPriceReminder"),
        ("alerts.option_event.set", "set", "Qot_SetOptionEventAlert"),
        (
            "watchlist.remote.modify",
            "modify",
            "Qot_ModifyUserSecurity",
        ),
    ] {
        let spec = FEATURE_SPECS
            .iter()
            .find(|spec| spec.id == feature)
            .unwrap_or_else(|| panic!("{feature} must exist in the capability catalog"));
        let operations =
            operations::catalog_operations(spec.id, spec.method, spec.api, spec.ui, spec.tool);
        assert_eq!(
            operations.len(),
            1,
            "{feature} must expose exactly one customization action"
        );
        assert_eq!(operations[0]["id"], action, "{feature} action id");
        assert_eq!(operations[0]["tool"], feature, "{feature} keeps its tool id");
        let protocols = operations[0]["protocols"]
            .as_array()
            .unwrap_or_else(|| panic!("{feature} protocols"));
        assert_eq!(
            protocols.len(),
            1,
            "{feature} must own exactly one OpenD request"
        );
        assert_eq!(
            protocols[0]["key"], protocol,
            "{feature} OpenD protocol key"
        );
        assert_eq!(protocols[0]["kind"], "request", "{feature} protocol kind");
    }
}

/// Parity: go:452dea11:pkg/broker/catalog_test.go:75
/// `TestCapabilityCatalogRejectsUnsafeWriteMCP`. Go rejects a catalog entry
/// that declares external-write access while asking to join the read-only MCP
/// surface. Rust derives `surface.readOnlyMcp` from the access class, so that
/// combination cannot be constructed; this test asserts the same invariant for
/// every published feature (write/trade tools never enter the reviewed
/// read-only surface, reviewed reads always do).
///
/// Parity: go:452dea11:internal/assistant/assembly/adk_product_catalog_test.go:29
/// TestCapabilityCatalogSurfacesAreRegisteredAndMCPBounded
///
/// Go walks the builtin capability catalog and requires every feature tool to
/// carry the class of its access level: reviewed reads are `read_internal` and
/// inside the local read-only MCP surface, external writes are
/// `write_external`/high, and trading tools are `live_trading`/critical with
/// confirmation in every mode and never in the read-only surface.
///
/// Rust keeps the class metadata on the capability catalog projection
/// (`permission`/`approval` plus `surface.readOnlyMcp`) and the tool-level
/// permission class on the ADK descriptor policy, so the reachable half
/// asserts both layers for reads. The registered-descriptor half of the write
/// and trade classes is unreachable: `PRODUCTION_TOOL_DEFINITIONS` deliberately
/// holds no execution/alert/watchlist mutation tool, so those features are
/// asserted at the catalog layer and recorded as a boundary in the batch note.
#[test]
fn capability_access_classes_bound_the_reviewed_read_only_surface() {
    use crate::product::product_mcp_protocol::REVIEWED_READ_ONLY_TOOLS;
    use crate::product::product_production_ports::product_production_ports_adk::{
        PRODUCTION_TOOL_DEFINITIONS, tool_access_policy,
    };
    use jftrade_assistant::{
        ALL_PERMISSION_MODES, ToolDescriptor, ToolIdempotencyMode, tool_requires_approval,
    };

    let catalog = catalog();
    let features = catalog["features"].as_array().expect("catalog features");
    assert!(!features.is_empty(), "the catalog publishes its features");
    let registered = PRODUCTION_TOOL_DEFINITIONS
        .iter()
        .map(|definition| definition.id)
        .collect::<BTreeSet<_>>();

    let mut classes = BTreeSet::new();
    let mut approval_gated = 0_usize;
    for feature in features {
        let id = feature["id"].as_str().expect("feature id");
        let tool = feature["surface"]["tool"].as_str().expect("feature tool");
        assert!(!tool.is_empty(), "{id} has no tool mapping");
        let access = feature["access"].as_str().expect("feature access");
        classes.insert(access.to_owned());
        let reviewed = REVIEWED_READ_ONLY_TOOLS.contains(&tool);
        match access {
            "read" => {
                assert_eq!(
                    feature["permission"], "read_only",
                    "{id} read permission class"
                );
                assert_eq!(feature["approval"], "none", "{id} read approval class");
                assert_eq!(
                    feature["surface"]["readOnlyMcp"], true,
                    "{id} reviewed read must opt into the read-only MCP surface"
                );
                assert!(
                    reviewed,
                    "{id} is a reviewed read capability but {tool} is absent from the read-only MCP surface"
                );
                assert!(
                    registered.contains(tool),
                    "{id} maps to unregistered tool {tool}"
                );
                assert_eq!(
                    tool_access_policy(tool).permission,
                    "read_internal",
                    "{tool} registered permission class"
                );
            }
            "write" => {
                assert_eq!(
                    feature["permission"], "write_external",
                    "{id} write permission class"
                );
                assert_eq!(feature["approval"], "high", "{id} write approval class");
                assert!(
                    !reviewed,
                    "{id} external write {tool} leaked into the read-only MCP surface"
                );
            }
            "trade" => {
                assert_eq!(
                    feature["permission"], "live_trading",
                    "{id} trading permission class"
                );
                assert_eq!(
                    feature["approval"], "critical",
                    "{id} trading approval class"
                );
                assert!(
                    !reviewed,
                    "{id} trading tool {tool} leaked into the read-only MCP surface"
                );
                // Go's `len(RequiresApprovalIn) == 3`: every trading tool is
                // confirmed in every permission mode. Rust derives that from
                // the `live_trading` permission class, so the catalog class
                // has to keep gating all three modes once a descriptor exists.
                let descriptor = ToolDescriptor {
                    name: tool.to_owned(),
                    display_name: tool.to_owned(),
                    description: tool.to_owned(),
                    category: "execution".to_owned(),
                    permission: feature["permission"]
                        .as_str()
                        .expect("feature permission")
                        .to_owned(),
                    risk_level: feature["approval"]
                        .as_str()
                        .expect("feature approval")
                        .to_owned(),
                    idempotency_mode: ToolIdempotencyMode::ReplaySafe,
                    allowed_modes: ALL_PERMISSION_MODES
                        .iter()
                        .map(|mode| (*mode).to_owned())
                        .collect(),
                    requires_approval_in: Vec::new(),
                    input_schema: serde_json::json!({"type": "object"}),
                };
                for mode in ALL_PERMISSION_MODES {
                    assert!(
                        tool_requires_approval(&descriptor, mode),
                        "{id} must stay confirmation-gated in {mode}"
                    );
                }
                approval_gated += 1;
            }
            other => panic!("{id} declares unknown access class {other}"),
        }
    }
    assert_eq!(
        classes,
        BTreeSet::from(["read".to_owned(), "trade".to_owned(), "write".to_owned()]),
        "the catalog keeps exactly the three reviewed access classes"
    );
    assert_eq!(
        approval_gated, 6,
        "the six trading features keep their every-mode confirmation"
    );
}

/// Parity: go:452dea11:internal/assistant/assembly/adk_product_catalog_test.go:77
/// TestProductToolRegistryAndOperationSchemasAreCatalogBacked
///
/// Go builds one `catalogOperations` map from the builtin capability catalog,
/// requires every registered product tool except `market.capabilities` to own
/// at least one operation, requires `ProductToolOperations()` to equal the
/// catalog operations per tool, and requires every
/// `LocalMCPReadOnlyToolNames` entry to be registered with `read_internal`.
///
/// Rust keeps the tool-to-operation relation on the capability catalog and the
/// accepted operation values in the reviewed MCP schema, so the reachable
/// assertions are: every feature's tool owns at least one operation, the
/// reviewed read-only surface is registered with the read class,
/// `market.capabilities` stays the catalog entry without broker operations, and
/// every schema that declares an operation enum matches the catalog exactly.
/// The execution/alert/watchlist mutation half of Go's product tool set has no
/// Rust registration to check and is recorded as a boundary in the batch note.
#[test]
fn reviewed_tool_operation_schemas_are_catalog_backed() {
    use crate::product::product_mcp_protocol::{REVIEWED_READ_ONLY_TOOLS, try_schema_for};
    use crate::product::product_production_ports::product_production_ports_adk::{
        PRODUCTION_TOOL_DEFINITIONS, tool_access_policy,
    };

    let mut catalog_operations: BTreeMap<String, BTreeSet<String>> = BTreeMap::new();
    for feature in catalog()["features"].as_array().expect("catalog features") {
        let id = feature["id"].as_str().expect("feature id");
        let tool = feature["surface"]["tool"].as_str().expect("feature tool");
        let mut feature_tools = BTreeSet::new();
        for operation in feature["operations"].as_array().expect("feature operations") {
            let operation_id = operation["id"].as_str().expect("operation id");
            let Some(operation_tool) = operation["tool"].as_str() else {
                continue;
            };
            feature_tools.insert(operation_tool.to_owned());
            catalog_operations
                .entry(operation_tool.to_owned())
                .or_default()
                .insert(operation_id.to_owned());
        }
        assert!(
            feature_tools.contains(tool),
            "{id} maps to {tool} but no operation declares that tool: {feature_tools:?}"
        );
    }

    let registered = PRODUCTION_TOOL_DEFINITIONS
        .iter()
        .map(|definition| definition.id)
        .collect::<BTreeSet<_>>();
    // Go skips exactly one product tool: the capability directory itself is not
    // a broker operation, so it must stay without catalog operations.
    assert!(
        registered.contains("market.capabilities"),
        "the capability directory must stay registered"
    );
    assert!(
        !catalog_operations.contains_key("market.capabilities"),
        "the capability directory must not claim a broker operation"
    );

    let mut operation_enums = BTreeSet::new();
    for name in REVIEWED_READ_ONLY_TOOLS {
        assert!(
            registered.contains(*name),
            "local MCP tool {name} is not registered in the production catalog"
        );
        assert_eq!(
            tool_access_policy(name).permission,
            "read_internal",
            "local MCP tool {name} permission class"
        );
        let schema = try_schema_for(name)
            .unwrap_or_else(|| panic!("local MCP tool {name} has no reviewed schema"));
        let Some(values) = schema["properties"]["operation"]["enum"].as_array() else {
            continue;
        };
        let declared = values
            .iter()
            .map(|value| {
                value
                    .as_str()
                    .expect("operation enum value")
                    .to_owned()
            })
            .collect::<BTreeSet<_>>();
        let catalog_values = catalog_operations.get(*name).unwrap_or_else(|| {
            panic!("{name} declares operation values but no catalog operation maps to it")
        });
        assert_eq!(
            &declared, catalog_values,
            "{name} operation schema must equal the capability catalog operations"
        );
        operation_enums.insert((*name).to_owned());
    }

    // The equality above must not pass vacuously if every schema loses its
    // operation enum.
    for name in [
        "market.candles",
        "research.rankings",
        "research.screen",
        "prediction.history",
        "derivatives.option_analysis",
    ] {
        assert!(
            operation_enums.contains(name),
            "{name} must declare the operation enum compared against the catalog"
        );
    }
}

#[derive(Clone, Copy, PartialEq, Eq, Debug)]
enum TypedSchemaKind {
    Instrument,
    Collection,
    PredictionDiscovery,
    PredictionQuote,
}

/// Parity: go:452dea11:internal/assistant/assembly/typed_product_capabilities_test.go:11
/// `TestTypedProductCapabilitiesDriveFeatureAndAssistantSchemas`. Go walks
/// `productfeatures.TypedCapabilityDescriptions()` and requires every typed
/// capability to resolve to the same feature id, to keep a closed schema, to
/// advertise exactly the reviewed operations, and to require the routing
/// field of its schema kind. Rust keeps the same sixteen capabilities in
/// `FEATURE_SPECS` plus the reviewed MCP schema catalog, so the assertions run
/// over that single table instead of a second capability list.
#[test]
fn typed_product_capabilities_drive_feature_ids_and_reviewed_schemas() {
    use crate::product::product_mcp_protocol::schema_for;

    let capabilities: [(&str, TypedSchemaKind, &[&str]); 16] = [
        (
            "research.instrument",
            TypedSchemaKind::Instrument,
            &[
                "profile",
                "executives",
                "executive_background",
                "operational_efficiency",
                "top_brokers",
            ],
        ),
        (
            "research.financials",
            TypedSchemaKind::Instrument,
            &[
                "statements",
                "revenue_breakdown",
                "earnings_price_move",
                "earnings_price_history",
            ],
        ),
        (
            "research.valuation",
            TypedSchemaKind::Instrument,
            &["detail", "constituents"],
        ),
        (
            "research.analyst",
            TypedSchemaKind::Instrument,
            &["consensus", "ratings", "morningstar", "changes"],
        ),
        (
            "research.ownership",
            TypedSchemaKind::Instrument,
            &[
                "overview",
                "changes",
                "holders",
                "institutional",
                "insider_holders",
                "insider_transactions",
                "management_changes",
            ],
        ),
        (
            "research.corporate_actions",
            TypedSchemaKind::Instrument,
            &["dividends", "buybacks", "splits", "code_changes"],
        ),
        (
            "research.short_interest",
            TypedSchemaKind::Instrument,
            &["daily_volume", "short_interest"],
        ),
        ("research.screen", TypedSchemaKind::Collection, &["stock_v2"]),
        (
            "research.calendar",
            TypedSchemaKind::Collection,
            &["earnings", "dividends", "economic", "ipos", "trade_dates"],
        ),
        (
            "research.rankings",
            TypedSchemaKind::Collection,
            &[
                "earnings_beat",
                "dividend",
                "pre_market",
                "after_hours",
                "overnight",
                "top_movers",
                "hot",
                "short_selling",
                "period_change",
                "high_dividend_state",
                "heatmap",
                "rise_fall_distribution",
                "market_state",
                "fund_catalog",
            ],
        ),
        (
            "prediction.discover",
            TypedSchemaKind::PredictionDiscovery,
            &[
                "categories",
                "competitions",
                "series",
                "events",
                "contracts",
                "milestones",
            ],
        ),
        ("prediction.snapshot", TypedSchemaKind::Instrument, &[]),
        ("prediction.depth", TypedSchemaKind::Instrument, &[]),
        (
            "prediction.history",
            TypedSchemaKind::Instrument,
            &["candles", "historical", "ticks"],
        ),
        (
            "prediction.combo_eligible",
            TypedSchemaKind::Collection,
            &[],
        ),
        ("prediction.combo_quote", TypedSchemaKind::PredictionQuote, &[]),
    ];

    let required_contains = |schema: &Value, field: &str| {
        schema["required"]
            .as_array()
            .is_some_and(|required| required.iter().any(|value| value == field))
    };

    for (tool, kind, operations) in capabilities {
        let specs = FEATURE_SPECS
            .iter()
            .filter(|spec| spec.tool == tool)
            .collect::<Vec<_>>();
        assert_eq!(
            specs.len(),
            1,
            "{tool} must map to exactly one capability feature"
        );
        assert_eq!(
            specs[0].id, tool,
            "{tool} keeps its feature id identical to the assistant tool name"
        );

        let schema = schema_for(tool);
        assert_eq!(
            schema["additionalProperties"],
            json!(false),
            "{tool} must reject unknown properties: {schema}"
        );

        match kind {
            TypedSchemaKind::Instrument => assert!(
                required_contains(&schema, "instrumentId"),
                "{tool} must require instrumentId: {schema}"
            ),
            TypedSchemaKind::PredictionDiscovery => assert!(
                required_contains(&schema, "operation"),
                "{tool} must require operation: {schema}"
            ),
            TypedSchemaKind::PredictionQuote => {
                for field in ["accountId", "mvc", "legs"] {
                    assert!(
                        required_contains(&schema, field),
                        "{tool} must require {field}: {schema}"
                    );
                }
            }
            TypedSchemaKind::Collection => {}
        }

        if !operations.is_empty() {
            assert_eq!(
                schema["properties"]["operation"]["enum"],
                json!(operations),
                "{tool} must advertise the reviewed operations in order"
            );
        }
    }
}

/// Parity: go:452dea11:pkg/broker/catalog_test.go:11
/// TestCapabilityCatalog
///
/// Go validates the builtin catalog, requires at least 45 features, checks a
/// set of required product feature ids, and pins the console surfaces of the
/// warrant and future products. Rust keeps the catalog as the const
/// `FEATURE_SPECS` table, so the equivalent assertions run against the
/// projected catalog payload the HTTP API publishes.
#[test]
fn capability_catalog_publishes_required_product_surfaces() {
    let catalog = catalog();
    let features = catalog["features"].as_array().expect("catalog features");
    assert!(
        features.len() >= 45,
        "catalog feature count {} must cover the product surface",
        features.len()
    );
    for id in [
        "derivatives.option_chain",
        "derivatives.option_analysis",
        "research.financials",
        "prediction.discover",
        "prediction.combo_quote",
        "execution.combo_place",
        "watchlist.remote.modify",
    ] {
        assert!(
            features.iter().any(|feature| feature["id"] == id),
            "catalog must define {id}"
        );
    }
    let ui = |id: &str| {
        features
            .iter()
            .find(|feature| feature["id"] == id)
            .unwrap_or_else(|| panic!("{id} must exist in the capability catalog"))["surface"]["ui"]
            .as_str()
            .unwrap_or_default()
            .to_owned()
    };
    assert_eq!(ui("derivatives.warrants"), "/workspace?tab=warrants");
    assert_eq!(ui("derivatives.futures"), "/workspace");
}

/// Parity: go:452dea11:pkg/broker/product_capability_contracts_test.go:159
/// TestBrokerFeatureRouterRejectsDeclaredFeatureWithoutAdapterInterface
///
/// Go resolves a declared feature whose adapter interface is not composed and
/// fails with the concrete interface name (`OptionAnalyticsReader`). Rust has
/// no router; the composition root evaluates the same declaration, publishes
/// the required interface in the catalog and fails the feature closed with
/// `CAPABILITY_UNAVAILABLE` while that reader is absent.
#[test]
fn declared_features_publish_the_reader_interface_that_gates_them() {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    let provider = ready_provider();
    let value = project(&runtime, &provider, "market=US").expect("projection");
    let catalog_features = value["catalog"]["features"]
        .as_array()
        .expect("catalog features")
        .clone();
    let runtime_items = value["runtime"].as_array().expect("runtime items").clone();

    for (feature_id, adapter) in [
        ("derivatives.option_analysis", "OptionAnalyticsReader"),
        ("derivatives.option_chain", "DerivativeCatalogReader"),
        ("market.depth", "MarketMicrostructureReader"),
    ] {
        let declared = catalog_features
            .iter()
            .find(|feature| feature["id"] == feature_id)
            .unwrap_or_else(|| panic!("{feature_id} must be declared"));
        assert_eq!(
            declared["adapterInterface"], adapter,
            "{feature_id} must publish the interface that gates it"
        );

        let evaluated = runtime_items
            .iter()
            .find(|item| item["featureId"] == feature_id)
            .unwrap_or_else(|| panic!("{feature_id} must be evaluated"));
        assert_eq!(
            evaluated["evaluation"]["state"], "unavailable",
            "{feature_id} must fail closed while {adapter} is not composed"
        );
        assert_eq!(
            evaluated["evaluation"]["quoteRight"]["code"], "CAPABILITY_UNAVAILABLE",
            "{feature_id} must report the missing adapter interface"
        );
    }
}

/// Parity: go:452dea11:pkg/broker/product_capability_contracts_test.go:26
/// TestCapabilityOperationSurfaceFallbacksAndOverrides
///
/// Go derives UI surface ids from catalog routes (bare `/workspace`,
/// `/research`, `/watchlist` roots, `tab`/`surface`/`section` overrides and a
/// normalized fallback that still yields `app.root` for `/`), then applies
/// per-operation overrides such as the prediction-history subscription, which
/// keeps `workspace.chart` as its surface and exposes no tool.
#[test]
fn ui_surface_ids_and_operation_overrides_follow_the_catalog_route_table() {
    for (route, expected) in [
        ("", ""),
        ("/workspace", "workspace.root"),
        ("/workspace?tab=options", "workspace.options"),
        ("/workspace?surface=order", "workspace.order"),
        ("/workspace?unknown=1", "workspace.root"),
        ("/research", "research.market"),
        ("/research?section=macro", "research.macro"),
        ("/watchlist", "watchlist.root"),
        ("/", "app.root"),
        ("/settings/integrations?x=true", "settings.integrations.x.true"),
        ("%zz", "%zz"),
    ] {
        assert_eq!(ui_surface_id(route), expected, "surface id for {route:?}");
    }

    let history = FEATURE_SPECS
        .iter()
        .find(|spec| spec.id == "prediction.history")
        .expect("prediction.history");
    let operations =
        operations::catalog_operations(history.id, history.method, history.api, history.ui, history.tool);
    let subscribe = operations
        .iter()
        .find(|operation| operation["id"] == "subscribe")
        .expect("prediction history subscribe operation");
    assert_eq!(subscribe["httpMethod"], "POST", "subscription method");
    assert_eq!(
        subscribe["uiSurfaceId"], "workspace.chart",
        "subscription override keeps the chart surface"
    );
    assert!(
        subscribe.get("tool").is_none(),
        "the subscription override exposes no tool: {subscribe}"
    );

    let alerts = FEATURE_SPECS
        .iter()
        .find(|spec| spec.id == "alerts.price.list")
        .expect("alerts.price.list");
    let operations =
        operations::catalog_operations(alerts.id, alerts.method, alerts.api, alerts.ui, alerts.tool);
    assert_eq!(
        operations[0]["uiSurfaceId"], "",
        "a route without UI keeps an empty surface id"
    );
    assert_eq!(operations[0]["tool"], "alerts.price.list");
}
