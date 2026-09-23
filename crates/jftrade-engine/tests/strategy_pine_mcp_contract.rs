#[path = "../src/strategy_pine_mcp.rs"]
mod strategy_pine_mcp;

use serde_json::json;
use strategy_pine_mcp::{PINE_SPEC_TOOL, VALIDATE_PINE_TOOL, dispatch_strategy_pine_mcp};

#[test]
fn spec_leaf_preserves_frozen_sections_and_rejects_unknown_section() {
    let payload = dispatch_strategy_pine_mcp(PINE_SPEC_TOOL, &json!({"section": "support-matrix"}))
        .expect("spec");
    assert_eq!(payload["selectedSection"], "support-matrix");
    let section_ids = payload["sections"]
        .as_array()
        .expect("sections")
        .iter()
        .map(|section| section["id"].as_str().expect("section id"))
        .collect::<Vec<_>>();
    assert_eq!(
        section_ids,
        [
            "overview",
            "syntax",
            "expressions",
            "indicators",
            "orders",
            "support-matrix",
            "unsupported",
            "examples"
        ]
    );
    assert_eq!(payload["compatibilityScore"], 98.30);
    assert_eq!(payload["scoreModelVersion"], "closed-bar-strategy-v4.0");
    assert!(
        payload["compatibilityDimensions"]
            .as_array()
            .is_some_and(
                |dimensions| dimensions.iter().any(|item| item["id"] == "mtf"
                    && item["unsupportedIds"].as_array().is_some_and(|ids| ids
                        .iter()
                        .any(|id| id == "request.security.dynamic_symbol_timeframe")))
            )
    );
    assert_eq!(payload["externalEngine"]["engine"], "pinets-shadow");
    assert_eq!(payload["externalEngine"]["enabled"], false);
    assert_eq!(payload["externalEngine"]["license"], "AGPL-3.0-only");
    assert_eq!(payload["externalEngine"]["package"], "pinets@0.9.31");
    assert!(payload["externalEngine"].get("engineVersion").is_none());
    assert!(payload["externalEngine"].get("diagnostics").is_none());
    assert!(
        payload["goldenScripts"]
            .as_array()
            .is_some_and(|items| !items.is_empty())
    );
    assert_eq!(payload["sectionContent"]["id"], "support-matrix");
    assert!(payload["supportMatrix"].as_array().is_some_and(|items| {
        items
            .iter()
            .any(|item| item["capability"] == "v4.0 broker emulator boundary decision")
    }));
    let failure = dispatch_strategy_pine_mcp(PINE_SPEC_TOOL, &json!({"section": "unknown"}))
        .expect_err("unknown section");
    assert_eq!(failure.status, 400);
    assert_eq!(failure.code, "BAD_REQUEST");
}

#[test]
fn validate_leaf_defaults_requirements_and_maps_bad_arguments() {
    let payload = dispatch_strategy_pine_mcp(
        VALIDATE_PINE_TOOL,
        &json!({
            "script": "//@version=6\nstrategy(\"MCP\")\nfast = ta.ema(close, 8)"
        }),
    )
    .expect("validate");
    assert_eq!(payload["ok"], true);
    assert!(payload["requirements"].is_object());
    assert_eq!(payload["externalEngine"]["engine"], "pinets-shadow");
    assert_eq!(payload["externalEngine"]["status"], "disabled");
    assert_eq!(payload["externalEngine"]["license"], "");
    assert!(payload["externalEngine"].get("package").is_none());
    assert!(payload.get("diagnostics").is_none());
    assert!(payload.get("features").is_none());
    assert!(payload.get("ast").is_none());
    assert!(payload["saveHint"].is_null());
    let empty = dispatch_strategy_pine_mcp(VALIDATE_PINE_TOOL, &json!({"script": "  "}))
        .expect("empty validation");
    assert_eq!(empty["ok"], false);
    assert_eq!(empty["errors"][0], "script 是必填项");
    assert_eq!(empty["saveHint"]["specTool"], "strategy.pine_spec");
    assert!(
        empty["saveHint"]["message"]
            .as_str()
            .is_some_and(|message| {
                message.contains(empty["saveHint"]["skeleton"].as_str().unwrap_or_default())
            })
    );
    let invalid = dispatch_strategy_pine_mcp(
        VALIDATE_PINE_TOOL,
        &json!({"script": "//@version=6\nstrategy(\"Bad\")\nimport TradingView/ta/7"}),
    )
    .expect("invalid validation");
    assert_eq!(invalid["ok"], false);
    assert!(invalid["requirements"].is_null());
    assert!(invalid["saveHint"].is_object());
    let failure = dispatch_strategy_pine_mcp(VALIDATE_PINE_TOOL, &json!({"script": 7}))
        .expect_err("wrong script type");
    assert_eq!(failure.status, 400);
}

#[test]
fn unknown_leaf_fails_closed_without_fixture_success() {
    let failure =
        dispatch_strategy_pine_mcp("strategy.pine_unknown", &json!({})).expect_err("unknown leaf");
    assert_eq!(failure.status, 503);
    assert_eq!(failure.code, "MCP_TOOL_UNAVAILABLE");
    assert_eq!(failure.envelope()["status"], 503);
}

/// Parity: go:452dea11:pkg/strategy/pine/parse_test.go:763
/// TestCompatibilityScoreAndSupportedFeatureIDsAreRegistryDriven
///
/// Go asserts the v4.0 score model (version, score inside 98..100, five
/// dimensions) and that `SupportedFeatureIDs()` is registry driven: every id
/// below is advertised and the two unsupported ids stay out. The spec leaf
/// carries the score model and the frozen compatibility payload carries the
/// registry, so both surfaces are pinned here.
#[test]
fn spec_leaf_and_frozen_payload_keep_the_go_compatibility_registry() {
    let payload = dispatch_strategy_pine_mcp(PINE_SPEC_TOOL, &json!({"section": "support-matrix"}))
        .expect("spec");
    assert_eq!(payload["scoreModelVersion"], "closed-bar-strategy-v4.0");
    let score = payload["compatibilityScore"].as_f64().expect("score");
    assert!(
        (98.0..=100.0).contains(&score),
        "score {score} must stay inside the v4.0 closed-bar range"
    );
    assert_eq!(
        payload["compatibilityDimensions"]
            .as_array()
            .expect("dimensions")
            .len(),
        5,
        "the v4.0 model must keep five dimensions"
    );

    let fixture: serde_json::Value = serde_json::from_str(include_str!(
        "../../../tests/fixtures/compatibility/api-transport/strategy-pine.json"
    ))
    .expect("strategy-pine fixture");
    let features = fixture["cases"][0]["data"]["features"]
        .as_array()
        .expect("fixture features")
        .iter()
        .map(|item| item.as_str().expect("feature id").to_owned())
        .collect::<Vec<_>>();
    for id in [
        "indicator.v13_migration_set",
        "indicator.v14_window_momentum_set",
        "indicator.v15_common_ta_set",
        "indicator.v16_mtf_tuple_bindings",
        "indicator.v17_source_aware_semantic_requirements",
        "indicator.v21_bbw_cog_anchored_vwap",
        "indicator.v24_mtf_stoch",
        "request.security.pure_expression",
        "request.security.v15_common_ta_expression",
        "request.security.v16_tuple_whitelist",
        "request.security.v17_semantic_tuple_corpus",
        "request.security.v21_ast_pure_expression",
        "request.security.v22_general_tuple",
        "request.security.v23_pure_collection_object_expression",
        "request.security.v24_mtf_stoch",
        "request.security.v27_pure_helper_expression",
        "request.security.v28_object_method_expression",
        "request.security.v29_object_history_expression",
        "request.security.v32_diagnostic_matrix",
        "request.security.v32_lower_timeframe_preflight",
        "syntax.v15_loop_control_subset",
        "syntax.v16_security_tuple_destructure",
        "syntax.v17_ast_semantic_transition",
        "syntax.v21_collection_runtime_core",
        "syntax.v22_structured_loop_runtime",
        "syntax.v22_pure_udt_method_runtime",
        "syntax.v23_collection_api_expansion",
        "syntax.v23_pure_method_body_named_args",
        "syntax.v24_collection_api_expansion",
        "syntax.v24_runtime_loop_fallback",
        "syntax.v24_persistent_object_field_set",
        "syntax.v25_array_stat_api",
        "syntax.v26_collection_iteration",
        "syntax.v26_collection_history_snapshot",
        "syntax.v26_object_collection_fields",
        "syntax.v26_library_export_metadata",
        "syntax.v27_collection_history_aggregates",
        "syntax.v27_map_matrix_iteration",
        "syntax.v28_object_history_read",
        "syntax.v28_method_chain",
        "syntax.v28_export_metadata",
        "syntax.v29_object_history_method_receiver",
        "syntax.v29_method_chain_named_defaults",
        "syntax.v29_request_security_diagnostics",
        "syntax.v30_stable_semantic_declarations",
        "syntax.v30_varip_closed_bar_policy",
        "syntax.v30_parser_whitespace_comments",
        "syntax.v31_public_surface_lock",
        "syntax.v33_advanced_language_boundary",
        "syntax.arrays_maps_matrices",
        "syntax.methods_types_libraries",
        "syntax.dynamic_loops_while",
        "expression.v22_general_tuple",
        "expression.v23_object_field_set",
        "expression.v25_string_helpers",
        "expression.v25_timeframe_change",
        "expression.v27_timeframe_helpers",
        "tooling.visual_metadata_output",
        "tooling.v20_language_foundation",
        "tooling.v31_structured_helper_diagnostics",
        "tooling.v33_structured_language_diagnostics",
        "tooling.v34_generated_support_snapshot",
        "tooling.v40_broker_boundary_snapshot",
        "tooling.migration_corpus_v21",
        "tooling.migration_corpus_v22",
        "tooling.migration_corpus_v23",
        "tooling.migration_corpus_v24",
        "tooling.migration_corpus_v25",
        "tooling.migration_corpus_v26",
        "tooling.migration_corpus_v27",
        "tooling.migration_corpus_v28",
        "tooling.migration_corpus_v29",
        "tooling.migration_corpus_v30",
        "order.entry_reversal",
        "order.allow_entry_in",
        "strategy.v40_broker_boundary_decision",
    ] {
        assert!(
            features.iter().any(|candidate| candidate == id),
            "missing {id}"
        );
    }
    for id in [
        "order.oca_partial_fill",
        "request.security.dynamic_symbol_timeframe",
    ] {
        assert!(
            !features.iter().any(|candidate| candidate == id),
            "unsupported feature {id} must stay out of the registry"
        );
    }
    let mut sorted = features.clone();
    sorted.sort();
    sorted.dedup();
    assert_eq!(sorted.len(), features.len(), "registry ids must be unique");
}
