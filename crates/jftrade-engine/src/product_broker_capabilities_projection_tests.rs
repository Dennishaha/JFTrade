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
