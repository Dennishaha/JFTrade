//! Unit coverage for the `strategy.optimize` candidate helpers.

use serde_json::json;

use super::{candidate_definition_ids, candidate_payload, optimization_task_id};

/// Go prefers `definitionIds` and falls back to a single `definitionId`, and
/// both spellings are trimmed and de-blanked.
#[test]
fn candidate_ids_prefer_the_list_and_fall_back_to_one_definition() {
    let listed = candidate_definition_ids(&json!({
        "definitionIds": [" def-a ", "", "def-b"],
        "definitionId": "def-ignored",
    }))
    .expect("listed candidates");
    assert_eq!(listed, vec!["def-a".to_owned(), "def-b".to_owned()]);

    let single =
        candidate_definition_ids(&json!({"definitionId": " def-solo "})).expect("single candidate");
    assert_eq!(single, vec!["def-solo".to_owned()]);

    let error =
        candidate_definition_ids(&json!({"market": "US"})).expect_err("candidates are required");
    assert_eq!(error, "definitionIds is required");
}

/// The candidate payload keeps the start fields and replaces the tool-level
/// selectors with the one definition this candidate runs.
#[test]
fn candidate_payload_replaces_the_tool_selectors_with_one_definition() {
    let payload = candidate_payload(
        &json!({
            "definitionIds": ["def-a", "def-b"],
            "objective": "sharpe",
            "market": "US",
            "symbol": "US.AAPL",
        }),
        "def-b",
        Some("futu"),
    );
    assert_eq!(payload["definitionId"], "def-b");
    assert_eq!(
        payload["marketDataProvider"], "futu",
        "the frozen provider is carried onto every candidate"
    );
    assert_eq!(payload["market"], "US");
    assert_eq!(payload["symbol"], "US.AAPL");
    assert!(payload.get("definitionIds").is_none());
    assert!(payload.get("objective").is_none());
}

/// Go's `freezeBacktestProviderID`: the requested provider is trimmed and
/// lowercased before it reaches a candidate.
#[test]
fn candidate_payload_keeps_the_requested_provider_override() {
    let payload = candidate_payload(
        &json!({"definitionId": "def-a", "marketDataProvider": " YFINANCE "}),
        "def-a",
        Some("yfinance"),
    );
    assert_eq!(payload["marketDataProvider"], "yfinance");
}

/// Go's `"opt-" + time.Now().UTC().Format("20060102T150405.000000000")`.
#[test]
fn optimization_task_ids_use_go_timestamp_shape() {
    let task_id = optimization_task_id();
    let body = task_id.strip_prefix("opt-").expect("opt- prefix");
    assert_eq!(body.len(), 25, "unexpected task id: {task_id}");
    let (date, time) = body.split_once('T').expect("date/time separator");
    assert_eq!(date.len(), 8);
    assert!(date.chars().all(|value| value.is_ascii_digit()));
    let (clock, fraction) = time.split_once('.').expect("fraction separator");
    assert_eq!(clock.len(), 6);
    assert!(clock.chars().all(|value| value.is_ascii_digit()));
    assert_eq!(fraction.len(), 9);
    assert!(fraction.chars().all(|value| value.is_ascii_digit()));
    assert!(
        date.starts_with("20"),
        "the UTC date must be a real calendar day: {task_id}"
    );
}

/// Go's `skillsruntime.DefaultToolInputSchema("strategy.optimize")` keeps the
/// object closed and requires the candidate list plus the execution window, so
/// the model cannot queue a run without them.
#[test]
fn the_tool_schema_requires_the_candidate_list_and_execution_window() {
    let schema = crate::product::product_mcp_protocol::try_schema_for("strategy.optimize")
        .expect("reviewed strategy.optimize schema");
    assert_eq!(schema["type"], "object");
    assert_eq!(schema["additionalProperties"], false);
    let required = schema["required"].as_array().expect("required list");
    for field in ["definitionIds", "market", "symbol", "startTime", "endTime"] {
        assert!(
            required.iter().any(|value| value.as_str() == Some(field)),
            "{field} must stay required: {schema}"
        );
    }
    assert_eq!(
        schema["properties"]["definitionIds"]["maxItems"], 12,
        "the schema mirrors the handler's candidate limit"
    );
    assert!(
        schema["properties"]
            .as_object()
            .is_some_and(|properties| !properties.is_empty()),
        "the schema must declare properties: {schema}"
    );
}
