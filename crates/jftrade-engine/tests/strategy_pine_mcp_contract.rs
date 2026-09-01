#[path = "../src/strategy_pine_mcp.rs"]
mod strategy_pine_mcp;

use serde_json::json;
use strategy_pine_mcp::{PINE_SPEC_TOOL, VALIDATE_PINE_TOOL, dispatch_strategy_pine_mcp};

#[test]
fn spec_leaf_preserves_frozen_sections_and_rejects_unknown_section() {
    let payload =
        dispatch_strategy_pine_mcp(PINE_SPEC_TOOL, &json!({"section": "examples"})).expect("spec");
    assert_eq!(payload["selectedSection"], "examples");
    assert_eq!(payload["sections"].as_array().expect("sections").len(), 7);
    assert_eq!(payload["externalEngine"]["engine"], "pinets-shadow");
    assert_eq!(payload["externalEngine"]["enabled"], false);
    assert_eq!(payload["externalEngine"]["license"], "AGPL-3.0-only");
    assert!(
        payload["goldenScripts"]
            .as_array()
            .is_some_and(|items| !items.is_empty())
    );
    assert_eq!(payload["sectionContent"]["id"], "examples");
    let failure = dispatch_strategy_pine_mcp(PINE_SPEC_TOOL, &json!({"section": "support-matrix"}))
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
