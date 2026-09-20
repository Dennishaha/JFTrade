//! Research backtest execution orchestration for ADK tools.
//!
//! Handles temporary strategy execution, polling completion, data readiness
//! checks with sync-task reuse, and delegating ResultView projection to
//! `BacktestReadSnapshotPort::result_view`.

use std::time::Duration;

use serde_json::{Value, json};
use sha2::{Digest, Sha256};

use super::product_backtests_write_port::{BacktestsWriteInput, BacktestsWritePortResult};
#[allow(unused_imports)]
pub(crate) use super::product_research_backtest_readiness::derive_effective_since_time;
use super::product_research_backtest_readiness::{
    EnsureDataOutcome, ensure_research_data_readiness,
};
use crate::product::BacktestResultViewRequest;
use crate::product::product_production_ports::ProductionPortBundle;

pub(crate) fn research_script_hash(script: &str) -> String {
    let mut hasher = Sha256::new();
    hasher.update(script.trim().as_bytes());
    let result = hasher.finalize();
    result[..8]
        .iter()
        .map(|b| format!("{b:02x}"))
        .collect::<String>()
}

pub(crate) fn validate_research_script(
    script: &str,
) -> Result<jftrade_strategy::pinespec::ValidationPayload, String> {
    if script.trim().is_empty() {
        return Err("script is required".to_owned());
    }
    let validation = jftrade_strategy::pinespec::validate_script(script, true, false);
    if !validation.ok {
        return Err(format!(
            "strategy script validation failed: {}",
            validation.errors.join("; ")
        ));
    }
    Ok(validation)
}

/// Go's `backtestStartInputFromMap` decodes `tradingCosts` into a typed cost
/// object and fails the whole call when the field is present but not an
/// object.  `null` is indistinguishable from an omitted field for
/// `json.Unmarshal` into a struct value, so it stays accepted.
pub(crate) fn validate_trading_costs(arguments: &Value) -> Result<(), String> {
    match arguments.get("tradingCosts") {
        None | Some(Value::Null) | Some(Value::Object(_)) => Ok(()),
        Some(_) => Err("tradingCosts must be a valid object".to_owned()),
    }
}

pub(crate) fn prepare_start_payload(arguments: &Value, script: &str) -> Value {
    let mut start_payload = arguments.clone();
    if let Some(obj) = start_payload.as_object_mut() {
        obj.insert(
            "strategyScript".to_owned(),
            Value::String(script.to_owned()),
        );
        if !obj.contains_key("symbol")
            && let Some(code_val) = obj.get("code").and_then(Value::as_str)
        {
            let sym = if code_val.contains('.') {
                code_val.to_owned()
            } else {
                let market = obj.get("market").and_then(Value::as_str).unwrap_or("HK");
                format!("{market}.{code_val}")
            };
            obj.insert("symbol".to_owned(), Value::String(sym));
        }
        if !obj.contains_key("market") {
            obj.insert("market".to_owned(), Value::String("HK".to_owned()));
        }
    }
    start_payload
}

pub(crate) fn poll_backtest_completion(
    ports: &ProductionPortBundle,
    run_id: &str,
    wait_ms: u64,
    initial_status: String,
) -> String {
    let mut current_status = initial_status;
    if wait_ms == 0
        || matches!(
            current_status.as_str(),
            "completed" | "failed" | "cancelled"
        )
    {
        return current_status;
    }
    let start_instant = std::time::Instant::now();
    let timeout = Duration::from_millis(wait_ms);
    let poll_interval = Duration::from_millis(50);

    while start_instant.elapsed() < timeout {
        std::thread::sleep(poll_interval);
        if let Ok(Some(status_val)) = ports.backtest_read.status(run_id)
            && let Some(st) = status_val.get("status").and_then(Value::as_str)
        {
            current_status = st.to_owned();
            if matches!(
                current_status.as_str(),
                "completed" | "failed" | "cancelled"
            ) {
                break;
            }
        }
    }
    current_status
}

pub(crate) fn extract_run_metadata(
    result_val: Option<&Value>,
    arguments: &Value,
) -> (String, String, String, bool, String, Value) {
    let request = result_val.and_then(|r| r.get("request"));
    let provider = result_val
        .and_then(|r| r.get("marketDataProvider"))
        .or_else(|| request.and_then(|req| req.get("marketDataProvider")))
        .or_else(|| arguments.get("marketDataProvider"))
        .and_then(Value::as_str)
        .unwrap_or("futu")
        .to_owned();
    let chart_type = result_val
        .and_then(|r| r.get("chartType"))
        .or_else(|| request.and_then(|req| req.get("chartType")))
        .or_else(|| arguments.get("chartType"))
        .and_then(Value::as_str)
        .unwrap_or("standard")
        .to_owned();
    let instrument_type = result_val
        .and_then(|r| r.get("instrumentType"))
        .or_else(|| request.and_then(|req| req.get("instrumentType")))
        .or_else(|| arguments.get("instrumentType"))
        .and_then(Value::as_str)
        .unwrap_or("stock")
        .to_owned();
    let use_extended_hours = result_val
        .and_then(|r| r.get("useExtendedHours"))
        .or_else(|| request.and_then(|req| req.get("useExtendedHours")))
        .or_else(|| arguments.get("useExtendedHours"))
        .and_then(Value::as_bool)
        .unwrap_or(false);
    let execution_model = result_val
        .and_then(|r| r.get("executionModel"))
        .or_else(|| request.and_then(|req| req.get("executionModel")))
        .or_else(|| arguments.get("executionModel"))
        .and_then(Value::as_str)
        .unwrap_or("conservative-bar-v1")
        .to_owned();
    let trading_costs = result_val
        .and_then(|r| r.get("tradingCosts"))
        .or_else(|| request.and_then(|req| req.get("tradingCosts")))
        .or_else(|| arguments.get("tradingCosts"))
        .cloned()
        .unwrap_or(Value::Null);

    (
        provider,
        chart_type,
        instrument_type,
        use_extended_hours,
        execution_model,
        trading_costs,
    )
}

pub(crate) fn build_result_view_request_from_options(
    run_id: &str,
    options: Option<&Value>,
) -> BacktestResultViewRequest {
    let view = options
        .and_then(|o| o.get("view"))
        .and_then(Value::as_str)
        .map(str::to_owned);
    let include = options
        .and_then(|o| o.get("include"))
        .and_then(Value::as_array)
        .map(|arr| {
            arr.iter()
                .filter_map(Value::as_str)
                .map(str::to_owned)
                .collect()
        });
    let start_time = options
        .and_then(|o| o.get("startTime"))
        .and_then(Value::as_str)
        .map(str::to_owned);
    let end_time = options
        .and_then(|o| o.get("endTime"))
        .and_then(Value::as_str)
        .map(str::to_owned);
    let cursor = options
        .and_then(|o| o.get("cursor"))
        .and_then(Value::as_str)
        .map(str::to_owned);
    let limit = options
        .and_then(|o| o.get("limit"))
        .and_then(Value::as_u64)
        .map(|v| v as usize);
    let resolution = options
        .and_then(|o| o.get("resolution"))
        .and_then(Value::as_str)
        .map(str::to_owned);

    BacktestResultViewRequest {
        run_id: run_id.to_owned(),
        view,
        include,
        start_time,
        end_time,
        cursor,
        limit,
        resolution,
    }
}

fn start_research_backtest_run(
    ports: &ProductionPortBundle,
    start_payload: &Value,
) -> Result<(String, String), String> {
    let start_result = ports.backtests_write.mutate(&BacktestsWriteInput::Start {
        payload: start_payload.clone(),
    });
    match start_result {
        Ok(BacktestsWritePortResult::Data(data)) => {
            let id = data
                .get("runId")
                .or_else(|| data.get("id"))
                .and_then(Value::as_str)
                .ok_or_else(|| "backtest start response missing runId".to_owned())?
                .to_owned();
            let status = data
                .get("status")
                .and_then(Value::as_str)
                .unwrap_or("queued")
                .to_owned();
            Ok((id, status))
        }
        Err(err) => Err(format!("failed to start research backtest: {err:?}")),
        Ok(other) => Err(format!("unexpected backtest start result: {other:?}")),
    }
}

fn attach_result_view(
    ports: &ProductionPortBundle,
    response: &mut Value,
    run_id: &str,
    view_opts: Option<&Value>,
) {
    let view_req = build_result_view_request_from_options(run_id, view_opts);
    match ports.backtest_read.result_view(&view_req) {
        Ok(Some(snapshot)) => {
            response["resultView"] = snapshot.data;
        }
        Ok(None) => {
            response["resultViewError"] = Value::String("backtest run was not found".to_owned());
        }
        Err(err) => {
            response["resultViewError"] = Value::String(err.to_string());
        }
    }
}

pub(crate) fn execute_research_backtest(
    ports: &ProductionPortBundle,
    arguments: &Value,
) -> Result<Value, String> {
    let script = arguments
        .get("script")
        .or_else(|| arguments.get("strategyScript"))
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|s| !s.is_empty())
        .ok_or_else(|| "script is required".to_owned())?;

    let validation = validate_research_script(script)?;
    validate_trading_costs(arguments)?;
    let start_payload = prepare_start_payload(arguments, script);

    let (run_id, initial_status) = match ensure_research_data_readiness(
        ports,
        arguments,
        &validation,
        &start_payload,
        script,
    )? {
        EnsureDataOutcome::Ready => start_research_backtest_run(ports, &start_payload)?,
        EnsureDataOutcome::Syncing(syncing_resp) => return Ok(syncing_resp),
    };

    let wait_ms = arguments
        .get("waitForCompletionMs")
        .and_then(Value::as_i64)
        .unwrap_or(0)
        .clamp(0, 25_000) as u64;

    let mut current_status = poll_backtest_completion(ports, &run_id, wait_ms, initial_status);

    let result_snapshot = ports.backtest_read.result(&run_id).ok().flatten();
    if let Some(ref res) = result_snapshot
        && let Some(st) = res.get("status").and_then(Value::as_str)
    {
        current_status = st.to_owned();
    }

    let (provider, chart_type, inst_type, extended, exec_model, fees) =
        extract_run_metadata(result_snapshot.as_ref(), arguments);

    let mut response = json!({
        "ok": true,
        "status": current_status,
        "runId": run_id,
        "marketDataProvider": provider,
        "chartType": chart_type,
        "instrumentType": inst_type,
        "useExtendedHours": extended,
        "executionModel": exec_model,
        "tradingCosts": fees,
        "scriptHash": research_script_hash(script),
        "validation": {
            "metadata": validation.metadata,
            "hooks": validation.hooks,
            "warnings": validation.warnings,
        },
        "saveRecommendation": "仅当用户明确要求保存/发布/更新策略定义时，再调用 strategy.save_definition。",
    });

    attach_result_view(ports, &mut response, &run_id, arguments.get("resultView"));
    Ok(response)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_start_script_rejects_blank_research_script() {
        // Parity: internal/backtest/recovery_test.go:45 TestStartScriptRejectsBlankResearchScript
        assert_eq!(
            validate_research_script(" \n\t "),
            Err("script is required".to_owned())
        );
        assert_eq!(
            validate_research_script(""),
            Err("script is required".to_owned())
        );
    }

    /// Free text without any Pine structure is rejected before the tool reads
    /// any port, so `strategy.research_backtest` cannot start a run from a
    /// non-strategy prompt.
    ///
    /// Parity: go:452dea11:internal/assistant/assembly/adk_capability_contracts_test.go:94
    #[test]
    fn research_script_validation_rejects_free_text() {
        let error = validate_research_script("not pine").expect_err("free text is not Pine");
        assert!(
            error.starts_with("strategy script validation failed"),
            "unexpected error: {error}"
        );
    }

    /// Go's `backtestStartInputFromMap` decodes `tradingCosts` only when the
    /// field is present and `json.Unmarshal` folds `null` into an omitted
    /// field.
    ///
    /// Parity: go:452dea11:internal/assistant/assembly/adk_capability_contracts_test.go:94
    #[test]
    fn research_trading_costs_stay_optional_but_must_be_an_object() {
        validate_trading_costs(&json!({})).expect("an omitted field stays accepted");
        for accepted in [json!({"commissionRate": 0.0003}), json!({}), Value::Null] {
            validate_trading_costs(&json!({"tradingCosts": accepted}))
                .unwrap_or_else(|error| panic!("accepted costs {accepted}: {error}"));
        }
        for rejected in [json!("cheap"), json!([0.0003]), json!(7), json!(true)] {
            assert_eq!(
                validate_trading_costs(&json!({"tradingCosts": rejected})),
                Err("tradingCosts must be a valid object".to_owned()),
                "rejected costs {rejected}"
            );
        }
    }
}
