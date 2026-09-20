//! Multi-candidate strategy optimization for the ADK `strategy.optimize` tool.
//!
//! The reference handler (`internal/assistant/assembly/tool_catalog.go:574`)
//! turns one tool call into several real asynchronous backtest runs — one per
//! candidate strategy definition — persists an `OptimizationTask` row that
//! references them and answers with the task id plus the candidate run
//! references.  Every candidate that was enqueued before a failure is rolled
//! back so a partially queued optimization never outlives its task row.

use serde_json::{Value, json};

use crate::product::product_backtests_write_port::{
    BacktestsWriteInput, BacktestsWritePortError, BacktestsWritePortResult,
};
use crate::product::product_production_ports::ProductionPortBundle;
use crate::product::product_research_backtest_execution::validate_trading_costs;

/// The reference caps one optimization call at twelve candidates.
pub(crate) const MAX_OPTIMIZATION_CANDIDATES: usize = 12;

/// Go's `stringOrDefault(stringValue(input, "objective"), "return")`.
const DEFAULT_OBJECTIVE: &str = "return";

pub(crate) const OPTIMIZE_RESULT_MESSAGE: &str =
    "候选策略已进入真实回测队列；使用 backtest.runs 查询进度和结果。";

pub(crate) fn execute_strategy_optimize(
    ports: &ProductionPortBundle,
    arguments: &Value,
) -> Result<Value, String> {
    let definition_ids = candidate_definition_ids(arguments)?;
    if definition_ids.len() > MAX_OPTIMIZATION_CANDIDATES {
        return Err(format!(
            "at most {MAX_OPTIMIZATION_CANDIDATES} optimization candidates are allowed"
        ));
    }
    validate_trading_costs(arguments)?;
    let objective = arguments
        .get("objective")
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .unwrap_or(DEFAULT_OBJECTIVE)
        .to_owned();
    let task_id = optimization_task_id();
    // Go's `freezeBacktestProviderID(startInput.MarketDataProvider,
    // deps.BacktestProviderID)` resolves the provider once, before the first
    // candidate is queued, and every candidate inherits that frozen value so a
    // concurrent default-provider change cannot split one optimization across
    // two data sources.
    let frozen_provider = frozen_market_data_provider(ports, arguments);
    let mut runs = Vec::with_capacity(definition_ids.len());
    let mut run_refs = Vec::with_capacity(definition_ids.len());
    for definition_id in &definition_ids {
        let payload = candidate_payload(arguments, definition_id, frozen_provider.as_deref());
        match ports
            .backtests_write
            .mutate(&BacktestsWriteInput::Start { payload })
        {
            Ok(BacktestsWritePortResult::Data(value)) => {
                let run_id = value
                    .get("id")
                    .and_then(Value::as_str)
                    .map(str::to_owned)
                    .ok_or_else(|| {
                        format!("queue candidate {definition_id:?}: backtest start returned no id")
                    })?;
                let status = value
                    .get("status")
                    .and_then(Value::as_str)
                    .unwrap_or("queued")
                    .to_owned();
                runs.push(candidate_run_value(
                    definition_id,
                    &run_id,
                    &status,
                    arguments,
                ));
                run_refs.push(json!({
                    "definitionId": definition_id,
                    "runId": run_id,
                }));
            }
            Ok(other) => {
                cancel_candidates(ports, &run_refs);
                return Err(format!(
                    "queue candidate {definition_id:?}: unexpected backtest start result {other:?}"
                ));
            }
            Err(error) => {
                cancel_candidates(ports, &run_refs);
                return Err(format!(
                    "queue candidate {definition_id:?}: {}",
                    port_error_text(&error)
                ));
            }
        }
    }
    let task = json!({
        "id": task_id,
        "status": "queued",
        "objective": objective,
        "runs": run_refs,
    });
    ports
        .mcp_store
        .upsert_optimization_task(&task_id, &task.to_string())
        .map_err(|error| format!("persist optimization task: {error}"))?;
    Ok(json!({
        "taskId": task_id,
        "status": "queued",
        "objective": objective,
        "runs": runs,
        "message": OPTIMIZE_RESULT_MESSAGE,
    }))
}

/// Go prefers `definitionIds` and falls back to a single `definitionId`.
fn candidate_definition_ids(arguments: &Value) -> Result<Vec<String>, String> {
    let mut ids = arguments
        .get("definitionIds")
        .and_then(Value::as_array)
        .map(|values| {
            values
                .iter()
                .filter_map(Value::as_str)
                .map(str::trim)
                .filter(|value| !value.is_empty())
                .map(str::to_owned)
                .collect::<Vec<_>>()
        })
        .unwrap_or_default();
    if ids.is_empty()
        && let Some(single) = arguments
            .get("definitionId")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty())
    {
        ids.push(single.to_owned());
    }
    if ids.is_empty() {
        return Err("definitionIds is required".to_owned());
    }
    Ok(ids)
}

/// One candidate keeps every start field of the tool call except the
/// tool-level candidate selectors, which the reference maps into its typed
/// start input instead of forwarding.
fn candidate_payload(
    arguments: &Value,
    definition_id: &str,
    frozen_provider: Option<&str>,
) -> Value {
    let mut payload = arguments.clone();
    if let Some(object) = payload.as_object_mut() {
        object.remove("definitionIds");
        object.remove("objective");
        object.insert(
            "definitionId".to_owned(),
            Value::String(definition_id.to_owned()),
        );
        if let Some(provider) = frozen_provider {
            object.insert(
                "marketDataProvider".to_owned(),
                Value::String(provider.to_owned()),
            );
        }
    }
    payload
}

/// Go's `freezeBacktestProviderID` lowercases and trims the requested
/// provider.  When the caller names no provider the candidate payload stays
/// provider-free, so the backtest write port resolves its own frozen default
/// at queue time exactly like the reference service does.
fn frozen_market_data_provider(_ports: &ProductionPortBundle, arguments: &Value) -> Option<String> {
    arguments
        .get("marketDataProvider")
        .and_then(Value::as_str)
        .map(str::trim)
        .map(str::to_ascii_lowercase)
        .filter(|value| !value.is_empty())
}

/// Go's `runs[]` entry: the candidate identity plus the frozen execution
/// request fields the console shows next to the queued run.
fn candidate_run_value(
    definition_id: &str,
    run_id: &str,
    status: &str,
    arguments: &Value,
) -> Value {
    let field = |name: &str, fallback: &str| {
        arguments
            .get(name)
            .cloned()
            .unwrap_or_else(|| Value::String(fallback.to_owned()))
    };
    json!({
        "definitionId": definition_id,
        "runId": run_id,
        "status": status,
        "marketDataProvider": arguments.get("marketDataProvider").cloned().unwrap_or(Value::Null),
        "chartType": field("chartType", "standard"),
        "instrumentType": field("instrumentType", "stock"),
        "useExtendedHours": arguments.get("useExtendedHours").cloned().unwrap_or(Value::Bool(false)),
        "tradingCosts": arguments.get("tradingCosts").cloned().unwrap_or(Value::Null),
        "executionModel": arguments.get("executionModel").cloned().unwrap_or(Value::Null),
    })
}

fn cancel_candidates(ports: &ProductionPortBundle, run_refs: &[Value]) {
    for reference in run_refs {
        let Some(run_id) = reference.get("runId").and_then(Value::as_str) else {
            continue;
        };
        let _ = ports.backtests_write.mutate(&BacktestsWriteInput::Cancel {
            run_id: run_id.to_owned(),
        });
    }
}

fn port_error_text(error: &BacktestsWritePortError) -> String {
    match error {
        BacktestsWritePortError::Unavailable(message)
        | BacktestsWritePortError::BadRequest(message)
        | BacktestsWritePortError::StrategyNotFound(message)
        | BacktestsWritePortError::Conflict(message)
        | BacktestsWritePortError::Failed(message) => message.clone(),
    }
}

/// Go's `"opt-" + time.Now().UTC().Format("20060102T150405.000000000")`.
fn optimization_task_id() -> String {
    let nanos = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|elapsed| elapsed.as_nanos())
        .unwrap_or_default();
    let seconds = (nanos / 1_000_000_000) as i64;
    let subsecond = (nanos % 1_000_000_000) as u64;
    let days = seconds.div_euclid(86_400);
    let second_of_day = seconds.rem_euclid(86_400);
    let (year, month, day) = civil_from_days(days);
    format!(
        "opt-{year:04}{month:02}{day:02}T{:02}{:02}{:02}.{subsecond:09}",
        second_of_day / 3_600,
        (second_of_day % 3_600) / 60,
        second_of_day % 60,
    )
}

/// Howard Hinnant's civil-from-days conversion, matching `time.Time`'s
/// proleptic Gregorian calendar for the UTC instants involved.
fn civil_from_days(days: i64) -> (i64, i64, i64) {
    let shifted = days + 719_468;
    let era = if shifted >= 0 {
        shifted
    } else {
        shifted - 146_096
    } / 146_097;
    let day_of_era = shifted - era * 146_097;
    let year_of_era =
        (day_of_era - day_of_era / 1_460 + day_of_era / 36_524 - day_of_era / 146_096) / 365;
    let year = year_of_era + era * 400;
    let day_of_year = day_of_era - (365 * year_of_era + year_of_era / 4 - year_of_era / 100);
    let month_prime = (5 * day_of_year + 2) / 153;
    let day = day_of_year - (153 * month_prime + 2) / 5 + 1;
    let month = if month_prime < 10 {
        month_prime + 3
    } else {
        month_prime - 9
    };
    (if month <= 2 { year + 1 } else { year }, month, day)
}

#[cfg(test)]
#[path = "product_strategy_optimize_execution_tests.rs"]
mod tests;
