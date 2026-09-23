//! Production MCP executor unit tests.
//!
//! Kept in a `#[path]` test module so the executor itself stays inside the
//! production file budget (`*_tests.rs` files are exempt).

use super::*;
use crate::product::{
    MarketDataProviderReadSnapshotError, MarketDataQuoteReadSnapshotError, SystemReadSnapshotError,
    WatchlistReadSnapshotError,
};

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

/// Go's closure contract points every owner port at a failing implementation
/// (`errors.New("owner port unavailable")`) and requires the tool to surface an
/// error instead of an empty payload.  Rust keeps the same rule in the port
/// error mappers, so an unavailable reader can never be mistaken for "no data".
///
/// Parity: go:452dea11:internal/assistant/assembly/adk_closure_contracts_test.go:73
#[test]
fn owner_port_failures_map_to_tool_failures_for_the_closure_readers() {
    let quote = quote_error(MarketDataQuoteReadSnapshotError::Unavailable(
        "owner port unavailable".to_owned(),
    ));
    assert_eq!(quote.status, 503);
    assert_eq!(quote.code, "MARKET_DATA_QUOTE_READ_UNAVAILABLE");
    assert_eq!(quote.message, "owner port unavailable");

    let watchlist = watchlist_error(WatchlistReadSnapshotError::Unavailable(
        "owner port unavailable".to_owned(),
    ));
    assert_eq!(watchlist.status, 503);
    assert_eq!(watchlist.code, "WATCHLIST_UNAVAILABLE");

    let system = system_error(SystemReadSnapshotError::Unavailable(
        "owner port unavailable".to_owned(),
    ));
    assert_eq!(system.status, 503);
    assert_eq!(system.code, "SYSTEM_READ_UNAVAILABLE");
    assert!(
        system.message.contains("owner port unavailable"),
        "the owner reason survives: {}",
        system.message
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

/// Parity: go:452dea11:internal/assistant/assembly/adk_summary_contracts_test.go:33
/// `TestADKBacktestSummariesRetainCountsWithoutEmbeddingRawSeries`. The model
/// summary derives `tradeCount`/`candlesCount`/`latestLog`/`totalReturn` from
/// the stored run, the raw `result` payload and its series never travel with
/// it, and the run list still narrows by definition, status and limit.
#[test]
fn adk_backtest_run_summaries_keep_counts_without_raw_series() {
    use crate::product::BacktestResultViewRequest;
    use crate::product::product_research_backtest_projection::project_authoritative_result_view;

    let run = json!({
        "id": "run-1",
        "status": "completed",
        "request": {
            "definitionId": "definition-1",
            "symbol": "US.AAPL",
            "interval": "1d",
            "initialBalance": 100_000.0,
            "useExtendedHours": true
        },
        "result": {
            "candles": [{"time": "2025-01-01", "close": 10.5}],
            "cases": [{
                "realizedPnl": 1_500.0,
                "finalEquity": 101_500.0,
                "totalTrades": 3,
                "winRate": 0.66,
                "processedBars": 1
            }]
        },
        "logs": ["started", "completed"],
        "runtimeErrors": ["warning"]
    });

    let view = project_authoritative_result_view(
        &run,
        None,
        &BacktestResultViewRequest {
            run_id: "run-1".to_owned(),
            view: Some("summary".to_owned()),
            ..Default::default()
        },
    )
    .expect("summary projection");

    let summary = &view["summary"];
    assert_eq!(summary["tradeCount"], 3, "{view}");
    assert_eq!(summary["candlesCount"], 1, "{view}");
    assert_eq!(summary["latestLog"], "completed", "{view}");
    let total_return = summary["totalReturn"].as_f64().expect("totalReturn");
    assert!((total_return - 0.015).abs() < 1e-12, "{view}");
    assert_eq!(view["run"]["useExtendedHours"], true, "{view}");
    // The counts replace the series, so neither the run nor the summary may
    // embed the stored `result` payload or a candles array.
    assert!(view["run"].get("result").is_none(), "{view}");
    assert!(view.get("result").is_none(), "{view}");
    assert!(summary.get("candles").is_none(), "{view}");
    assert_eq!(view["series"], json!({}), "{view}");

    let filtered = helpers::filter_backtest_runs(
        json!({"runs": [
            {"id": "a", "status": "queued", "request": {"definitionId": "definition-1"}},
            {"id": "b", "status": "completed", "request": {"definitionId": "definition-2"}}
        ]}),
        &json!({"definitionId": "definition-1", "status": "queued", "limit": 1}),
    )
    .expect("filtered run list");
    assert_eq!(filtered["runCount"], 1, "{filtered}");
    assert_eq!(filtered["totalMatched"], 1, "{filtered}");
    assert_eq!(filtered["truncated"], false, "{filtered}");
    assert_eq!(filtered["runs"][0]["id"], "a", "{filtered}");
}

/// Parity: go:452dea11:internal/assistant/assembly/adk_strategy_input_validation_test.go:10
/// `TestStrategyADKInputsAndSummariesEnforceBusinessBoundaries` — the queued
/// backtest-summary branch.
///
/// Go's `SummarizeADKBacktestRuns` copies an explicit `useExtendedHours` onto a
/// run that has no result yet and leaves `totalReturn` nil, because
/// `summarizeADKBacktestRun` returns the identity fields untouched while
/// `run.Result == nil`. The Rust result-view projection answers the same empty
/// summary instead of deriving a zero return from the request's initial
/// balance, and the run list keeps the explicit selection.
#[test]
fn queued_backtest_summaries_keep_explicit_extended_hours_without_return_metrics() {
    use crate::product::BacktestResultViewRequest;
    use crate::product::product_research_backtest_projection::project_authoritative_result_view;

    let run = json!({
        "id": "run-queued",
        "status": "QUEUED",
        "request": {
            "definitionId": "definition-1",
            "symbol": "US.AAPL",
            "interval": "1d",
            "initialBalance": 100_000.0,
            "useExtendedHours": true
        }
    });
    let view = project_authoritative_result_view(
        &run,
        None,
        &BacktestResultViewRequest {
            run_id: "run-queued".to_owned(),
            view: Some("summary".to_owned()),
            ..Default::default()
        },
    )
    .expect("queued summary projection");

    assert_eq!(view["run"]["useExtendedHours"], true, "{view}");
    assert_eq!(
        view["summary"],
        json!({}),
        "a run without a stored result has no summary metrics: {view}"
    );
    assert_eq!(view["series"], json!({}), "{view}");

    let filtered =
        helpers::filter_backtest_runs(json!({"runs": [run]}), &json!({"status": "queued"}))
            .expect("queued run list");
    assert_eq!(filtered["runCount"], 1, "{filtered}");
    assert_eq!(
        filtered["runs"][0]["request"]["useExtendedHours"], true,
        "the stored selection survives the list projection: {filtered}"
    );
    assert!(
        filtered["runs"][0].get("totalReturn").is_none(),
        "a queued run never carries a derived return: {filtered}"
    );
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

/// Parity: go:452dea11:pkg/strategy/pineengine/pine_ts_payload_test.go:39 TestShadowPayloadReportsWorkerStartupFailure
///
/// Parity: go:452dea11:pkg/strategy/pineengine/pine_ts_payload_test.go:190 TestWorkerErrorStringAndStderrSuffix
#[test]
fn pine_shadow_error_payload_keeps_the_worker_failure_message() {
    let payload = super::pine::pine_shadow_error_payload(
        PINE_MODE_SHADOW,
        "pinets worker script unavailable".to_owned(),
    );
    assert_eq!(payload["enabled"], true);
    assert_eq!(payload["ok"], false);
    assert_eq!(payload["mode"], PINE_MODE_SHADOW);
    assert_eq!(payload["status"], "shadow_error");
    assert_eq!(payload["engine"], "pinets-shadow");
    assert_eq!(payload["repository"], "https://github.com/LuxAlgo/PineTS");
    assert_eq!(payload["differenceSummary"]["evaluated"], false);
    let diagnostics = payload["diagnostics"].as_array().expect("diagnostics");
    assert_eq!(diagnostics.len(), 1);
    assert_eq!(diagnostics[0]["code"], "PINETS_SHADOW_ERROR");
    assert!(
        diagnostics[0]["message"]
            .as_str()
            .unwrap_or_default()
            .contains("pinets worker script unavailable")
    );
}

/// Parity: go:452dea11:pkg/strategy/pineengine/pine_ts_payload_test.go:97 TestExternalEnginePayloadFromResultMapsSuccessAndFailure
#[test]
fn pine_shadow_success_payload_projects_engine_metadata_and_counts() {
    let payload = super::pine::pine_shadow_success_payload(
        PINE_MODE_SHADOW,
        serde_json::json!({
            "ok": true,
            "metadata": {"pineTsVersion": "0.9.31"},
            "plots": {"SMA": {"title": "SMA"}},
            "signals": {"cross": true},
            "diagnostics": [{
                "severity": "warning",
                "code": "TEST_WARN",
                "message": "mapped through",
                "line": 3,
                "column": 2
            }]
        }),
    );
    assert_eq!(payload["enabled"], true);
    assert_eq!(payload["ok"], true);
    assert_eq!(payload["status"], "shadow_ok");
    assert_eq!(payload["mode"], PINE_MODE_SHADOW);
    assert_eq!(payload["engine"], "pinets-shadow");
    assert_eq!(payload["engineVersion"], "0.9.31");
    assert_eq!(payload["license"], "AGPL-3.0-only");
    assert_eq!(payload["differenceSummary"]["evaluated"], true);
    assert_eq!(payload["differenceSummary"]["plots"], 1);
    assert_eq!(payload["differenceSummary"]["signals"], 1);
    let diagnostics = payload["diagnostics"].as_array().expect("diagnostics");
    assert_eq!(diagnostics[0]["severity"], "warning");
    assert_eq!(diagnostics[0]["code"], "TEST_WARN");
    assert_eq!(diagnostics[0]["line"], 3);
    // Go keeps a single diagnostic default for values it cannot classify.
    let fallback = super::pine::pine_shadow_success_payload(
        PINE_MODE_SHADOW,
        serde_json::json!({"ok": true, "diagnostics": ["raw"]}),
    );
    assert_eq!(fallback["diagnostics"][0]["code"], "PINETS_SHADOW_ERROR");
    assert_eq!(fallback["engineVersion"], "");
    assert_eq!(fallback["differenceSummary"]["plots"], 0);
}

/// Parity: go:452dea11:pkg/strategy/pineengine/pine_ts_payload_test.go:72 TestCommunityAGPLModeBlocksExecutionWhenNoticeCannotBeFound
#[test]
fn pine_external_engine_payload_requires_the_agpl_notice_in_community_mode() {
    let payload = super::pine::pine_external_engine_payload(
        None,
        PINE_MODE_COMMUNITY_AGPL,
        "plot(close)",
        false,
    )
    .expect("compliance gate runs before the analyzer");
    assert_eq!(payload["enabled"], true);
    assert_eq!(payload["ok"], false);
    assert_eq!(payload["mode"], PINE_MODE_COMMUNITY_AGPL);
    assert_eq!(payload["status"], "compliance_error");
    assert_eq!(payload["license"], "");
    assert_eq!(payload["repository"], "");
    let diagnostics = payload["diagnostics"].as_array().expect("diagnostics");
    assert_eq!(diagnostics.len(), 1);
    assert_eq!(diagnostics[0]["code"], "PINETS_AGPL_NOTICE_MISSING");
    assert_eq!(payload["differenceSummary"]["evaluated"], false);
}

/// Parity: go:452dea11:pkg/strategy/pineengine/pine_ts_payload_test.go:55 TestShadowPayloadRunsConfiguredWorkerAndReturnsExternalResult
///
/// A shadow mode without a configured analyzer surfaces the startup failure the
/// caller projects through the error payload.
#[test]
fn pine_external_engine_payload_reports_a_missing_analyzer() {
    let error = super::pine::pine_external_engine_payload(
        None,
        PINE_MODE_SHADOW,
        "//@version=6\nindicator(\"SMA\")\nplot(close)",
        true,
    )
    .expect_err("shadow mode requires the analyzer port");
    assert!(
        error.contains("pine analyzer is not configured"),
        "error {error:?}"
    );
    let payload = super::pine::pine_shadow_error_payload(PINE_MODE_SHADOW, error);
    assert_eq!(payload["status"], "shadow_error");
    assert_eq!(payload["ok"], false);
}
