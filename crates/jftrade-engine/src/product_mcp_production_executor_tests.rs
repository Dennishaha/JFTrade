//! Production MCP executor unit tests.
//!
//! Kept in a `#[path]` test module so the executor itself stays inside the
//! production file budget (`*_tests.rs` files are exempt).

use super::*;
use crate::product::MarketDataProviderReadSnapshotError;

#[test]
fn production_failure_projects_product_error_envelope() {
    let failure = McpToolFailure::failed(502, "MARKET_SNAPSHOT_FAILED", "upstream refused");
    assert_eq!(
        failure.envelope(),
        json!({
            "ok": false,
            "error": {"code": "MARKET_SNAPSHOT_FAILED", "message": "upstream refused"},
            "status": 502,
        })
    );
}

#[test]
fn production_argument_validation_rejects_missing_or_out_of_range_values() {
    assert_eq!(
        required_string(&json!({}), "query")
            .expect_err("missing query")
            .status,
        400
    );
    assert_eq!(
        bounded_integer(&json!({"limit": 0}), "limit", 20, 1, 100)
            .expect_err("zero limit")
            .code,
        "BAD_REQUEST"
    );
    assert!(instrument(&json!({"instrumentId": "US"})).is_err());
}

#[test]
fn production_snapshot_failures_and_malformed_payloads_fail_closed() {
    assert_eq!(
        provider_error(MarketDataProviderReadSnapshotError::Unavailable(
            "provider is offline".to_owned(),
        ))
        .status,
        503
    );
    assert_eq!(
        provider_error(MarketDataProviderReadSnapshotError::Failed {
            code: "UPSTREAM_REFUSED".to_owned(),
            message: "provider refused request".to_owned(),
        })
        .status,
        502
    );
    assert!(nullable_runs(&json!({"runs": null})).unwrap().is_none());
    let malformed = nullable_runs(&json!({"runs": {}})).expect_err("malformed runs");
    assert_eq!(malformed.status, 502);
    assert_eq!(malformed.code, "MCP_PRODUCTION_PAYLOAD_INVALID");
    let missing = nullable_runs(&json!({})).expect_err("missing runs");
    assert_eq!(missing.status, 502);
}

/// Parity: go:452dea11:internal/assistant/assembly/application_adapter_test.go:278
/// `TestApplicationAdapterProjectsBacktestState` (run-list half). Go's
/// assistant summary flattens the stored request, so `backtest.runs`
/// filters compare against `definitionId`/`definitionVersion` directly.
/// The Rust read port publishes the request nested under `request`, and
/// the tool must resolve both layouts instead of silently returning no
/// runs for a definition that exists.
#[test]
fn backtest_runs_filters_resolve_nested_request_fields_and_top_level_status() {
    let payload = json!({"runs": [
        {
            "id": "run-1",
            "status": "completed",
            "marketDataProvider": "yfinance",
            "request": {
                "definitionId": "strategy-1",
                "definitionVersion": "3",
                "symbol": "AAPL"
            }
        },
        {
            "id": "run-2",
            "status": "failed",
            "marketDataProvider": "futu",
            "request": {
                "definitionId": "strategy-2",
                "definitionVersion": "1",
                "symbol": "AAPL"
            }
        }
    ]});

    let by_definition =
        helpers::filter_backtest_runs(payload.clone(), &json!({"definitionId": "strategy-1"}))
            .expect("definition filter");
    assert_eq!(by_definition["runCount"], 1);
    assert_eq!(by_definition["runs"][0]["id"], "run-1");
    assert_eq!(by_definition["truncated"], false);

    let by_version =
        helpers::filter_backtest_runs(payload.clone(), &json!({"definitionVersion": "1"}))
            .expect("definition version filter");
    assert_eq!(by_version["runs"][0]["id"], "run-2");

    let by_status = helpers::filter_backtest_runs(payload.clone(), &json!({"status": "COMPLETED"}))
        .expect("status filter");
    assert_eq!(by_status["runs"][0]["id"], "run-1");

    let limited =
        helpers::filter_backtest_runs(payload.clone(), &json!({"limit": 1})).expect("limit");
    assert_eq!(limited["runCount"], 1);
    assert_eq!(limited["totalMatched"], 2);
    assert_eq!(limited["truncated"], true);

    let unfiltered =
        helpers::filter_backtest_runs(payload.clone(), &json!({})).expect("no filters");
    assert_eq!(unfiltered["runCount"], 2);
    assert_eq!(unfiltered["runs"], payload["runs"]);

    let empty = helpers::filter_backtest_runs(json!({"runs": null}), &json!({"limit": 5}))
        .expect("empty run list");
    assert_eq!(empty, json!({"runs": null, "runCount": 0}));
}

/// Parity: go:452dea11:internal/assistant/assembly/adk_strategy_test.go:397
/// `TestADKBacktestRunsFiltersByDefinitionVersionStatusAndLimit`. Go keeps
/// the newest matching run first, reports `runCount`/`totalMatched`/
/// `truncated` for the limit, accepts a status spelling in any case,
/// resolves the provider case-insensitively, and returns the unfiltered
/// legacy payload when no filter argument is present.
#[test]
fn backtest_runs_filters_match_the_go_definition_version_status_and_limit_matrix() {
    let payload = json!({"runs": [
        {"id": "run-newest", "status": "COMPLETED", "marketDataProvider": "yfinance",
         "request": {"definitionId": "def-1", "definitionVersion": "0.1.1"}},
        {"id": "run-older", "status": "completed", "marketDataProvider": "futu",
         "request": {"definitionId": "def-1", "definitionVersion": "0.1.1"}},
        {"id": "run-baseline", "status": "COMPLETED",
         "request": {"definitionId": "def-1", "definitionVersion": "0.1.0"}},
        {"id": "run-other", "status": "FAILED", "marketDataProvider": "futu",
         "request": {"definitionId": "def-2", "definitionVersion": "0.1.1"}}
    ]});

    let unfiltered =
        helpers::filter_backtest_runs(payload.clone(), &json!({})).expect("no filters");
    assert_eq!(unfiltered["runCount"], 4);
    assert_eq!(unfiltered["runs"].as_array().map(Vec::len), Some(4));

    let limited = helpers::filter_backtest_runs(
        payload.clone(),
        &json!({
            "definitionId": "def-1",
            "definitionVersion": "0.1.1",
            "status": "completed",
            "limit": 1
        }),
    )
    .expect("filtered run list");
    assert_eq!(limited["runCount"], 1);
    assert_eq!(limited["totalMatched"], 2);
    assert_eq!(limited["truncated"], true);
    assert_eq!(limited["runs"][0]["id"], "run-newest");

    let version_only =
        helpers::filter_backtest_runs(payload.clone(), &json!({"definitionVersion": "0.1.0"}))
            .expect("version-only run list");
    assert_eq!(version_only["runCount"], 1);
    assert_eq!(version_only["totalMatched"], 1);
    assert_eq!(version_only["truncated"], false);
    assert_eq!(version_only["runs"][0]["id"], "run-baseline");

    let provider_only =
        helpers::filter_backtest_runs(payload.clone(), &json!({"marketDataProvider": "YFINANCE"}))
            .expect("provider-only run list");
    assert_eq!(provider_only["runCount"], 1);
    assert_eq!(provider_only["runs"][0]["id"], "run-newest");
}

#[test]
fn pine_external_mode_parser_accepts_only_supported_values() {
    assert_eq!(pine_external_mode_value(None), PINE_MODE_OFF);
    assert_eq!(pine_external_mode_value(Some(" off ")), PINE_MODE_OFF);
    assert_eq!(pine_external_mode_value(Some(" SHADOW ")), PINE_MODE_SHADOW);
    assert_eq!(
        pine_external_mode_value(Some("community-agpl")),
        PINE_MODE_COMMUNITY_AGPL
    );
    assert_eq!(pine_external_mode_value(Some("unknown")), PINE_MODE_OFF);
}
