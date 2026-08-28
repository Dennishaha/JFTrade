use std::collections::BTreeMap;

use jftrade_store_sqlite::{StoredAdkApproval, StoredAdkRun};
use serde_json::{Value, json};
use time::{Duration, OffsetDateTime, format_description::well_known::Rfc3339};

use super::{AdkReadSnapshot, AdkReadSnapshotError, ProductionAdkPort, invalid_payload};

const WINDOW_DAYS: i64 = 7;

pub(super) fn read(
    port: &ProductionAdkPort,
) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
    let now = OffsetDateTime::now_utc();
    let since = now - Duration::days(WINDOW_DAYS);
    let runs = port.store.list_runs()?;
    let approvals = port.store.list_approvals()?;
    // Public ADK sessions are persisted in the main ADK database. The
    // auxiliary session database stores event/timeline data only and must not
    // be used as the source for session totals.
    let sessions = port.store.list_sessions()?;
    let workflows = port.store.list_workflows()?;
    let triggers = workflows
        .iter()
        .map(|workflow| port.store.list_workflow_triggers(&workflow.id))
        .collect::<Result<Vec<_>, _>>()?
        .into_iter()
        .flatten()
        .collect::<Vec<_>>();
    let logs = port.store.list_workflow_trigger_logs()?;
    let agents = agent_provider_map(port)?;
    let (run_metrics, tool_metrics, usage_metrics) = run_metrics(&runs, &agents)?;
    let approval_metrics = approval_metrics(&approvals, now)?;

    Ok(AdkReadSnapshot::Json(json!({
        "runs": {
            "total": runs.len(),
            "last7Days": runs.iter().filter(|run| in_window(&run.created_at, since)).count(),
            "byStatus": run_metrics.by_status,
            "byAgent": run_metrics.by_agent,
            "byProvider": run_metrics.by_provider,
            "lifecycle": run_metrics.lifecycle,
        },
        "tools": tool_metrics.to_value(),
        "approvals": approval_metrics.to_value(
            approvals.len(),
            approvals.iter().filter(|approval| in_window(&approval.created_at, since)).count(),
        ),
        "usage": usage_metrics.to_value(),
        "sessions": {
            "total": sessions.len(),
            "last7Days": sessions.iter().filter(|session| in_window(&session.created_at, since)).count(),
        },
        "workflows": {
            "definitions": workflows.len(),
            "enabledDefinitions": workflows.iter().filter(|workflow| enabled(&workflow.status)).count(),
            "triggers": triggers.len(),
            "enabledTriggers": triggers.iter().filter(|trigger| enabled(&trigger.status)).count(),
            "invocations": logs.len(),
            "invocationsLast7Days": logs.iter().filter(|log| in_window(&log.created_at, since)).count(),
            "byStatus": counts(logs.iter().map(|log| log.status.as_str())),
            "byTriggerType": counts(logs.iter().map(|log| log.trigger_type.as_str())),
        },
        "measurementWindow": {
            "days": WINDOW_DAYS,
            "since": format_timestamp(since)?,
        },
        "checkedAt": format_timestamp(now)?,
    })))
}

#[derive(Default)]
struct RunMetrics {
    by_status: BTreeMap<String, usize>,
    by_agent: BTreeMap<String, usize>,
    by_provider: BTreeMap<String, usize>,
    lifecycle: BTreeMap<String, usize>,
}

#[derive(Default)]
struct ToolMetrics {
    total: usize,
    successful: usize,
    duration_total: i64,
    duration_count: i64,
    output_bytes_total: usize,
    output_bytes_max: usize,
    truncated: usize,
    error_count: usize,
    retryable_errors: usize,
    by_name: BTreeMap<String, usize>,
    by_status: BTreeMap<String, usize>,
    by_error_code: BTreeMap<String, usize>,
}

impl ToolMetrics {
    fn to_value(&self) -> Value {
        json!({
            "total": self.total,
            "successful": self.successful,
            "averageDurationMs": if self.duration_count == 0 { 0 } else { self.duration_total / self.duration_count },
            "outputBytesTotal": self.output_bytes_total,
            "outputBytesMax": self.output_bytes_max,
            "truncated": self.truncated,
            "errorCount": self.error_count,
            "retryableErrors": self.retryable_errors,
            "byName": self.by_name,
            "byStatus": self.by_status,
            "byErrorCode": self.by_error_code,
        })
    }
}

#[derive(Default)]
struct UsageMetrics {
    samples: usize,
    tokens_in: i64,
    tokens_out: i64,
}

impl UsageMetrics {
    fn to_value(&self) -> Value {
        if self.samples == 0 {
            return json!({
                "samples": 0,
                "tokensInTotal": null,
                "tokensOutTotal": null,
                "tokensInAverage": null,
                "tokensOutAverage": null,
            });
        }
        let samples = i64::try_from(self.samples).unwrap_or(i64::MAX);
        json!({
            "samples": self.samples,
            "tokensInTotal": self.tokens_in,
            "tokensOutTotal": self.tokens_out,
            "tokensInAverage": self.tokens_in / samples,
            "tokensOutAverage": self.tokens_out / samples,
        })
    }
}

fn run_metrics(
    runs: &[StoredAdkRun],
    agent_providers: &BTreeMap<String, String>,
) -> Result<(RunMetrics, ToolMetrics, UsageMetrics), AdkReadSnapshotError> {
    let mut run_metrics = RunMetrics::default();
    let mut tool_metrics = ToolMetrics::default();
    let mut usage_metrics = UsageMetrics::default();
    for run in runs {
        let payload = decode(&run.payload_json, "run")?;
        increment(&mut run_metrics.by_status, &run.status);
        increment(&mut run_metrics.by_agent, &run.agent_id);
        increment(
            &mut run_metrics.by_provider,
            provider_id(&payload, &run.agent_id, agent_providers),
        );
        accumulate_lifecycle(&mut run_metrics.lifecycle, &run.status, &payload);
        accumulate_usage(&mut usage_metrics, &payload);
        if let Some(tool_calls) = payload.get("toolCalls").and_then(Value::as_array) {
            for tool_call in tool_calls {
                accumulate_tool(&mut tool_metrics, tool_call);
            }
        }
    }
    for key in ["failed", "timedOut", "cancelled", "resumed", "orphaned"] {
        run_metrics.lifecycle.entry(key.to_owned()).or_default();
    }
    Ok((run_metrics, tool_metrics, usage_metrics))
}

fn accumulate_lifecycle(counts: &mut BTreeMap<String, usize>, status: &str, payload: &Value) {
    match status.trim().to_ascii_uppercase().as_str() {
        "FAILED" => increment(counts, "failed"),
        "TIMED_OUT" => increment(counts, "timedOut"),
        "CANCELLED" => increment(counts, "cancelled"),
        _ => {}
    }
    if string_field(payload, "resumeState") == "adk_confirmation_resolved" {
        increment(counts, "resumed");
    }
    if string_field(payload, "errorCode") == "RUN_ORPHANED" {
        increment(counts, "orphaned");
    }
}

fn accumulate_usage(metrics: &mut UsageMetrics, run: &Value) {
    let Some(usage) = run.get("usage") else {
        return;
    };
    let tokens_in = integer_field(usage, "tokensIn");
    let tokens_out = integer_field(usage, "tokensOut");
    if tokens_in > 0 || tokens_out > 0 {
        metrics.samples += 1;
        metrics.tokens_in += tokens_in;
        metrics.tokens_out += tokens_out;
    }
}

fn accumulate_tool(metrics: &mut ToolMetrics, tool: &Value) {
    metrics.total += 1;
    let name = string_field(tool, "toolName");
    let status = string_field(tool, "status");
    increment(&mut metrics.by_name, name);
    increment(&mut metrics.by_status, status);
    if status == "SUCCEEDED" {
        metrics.successful += 1;
    }
    let duration = integer_field(tool, "durationMs");
    if duration > 0 {
        metrics.duration_total += duration;
        metrics.duration_count += 1;
    }
    let (bytes, truncated, retryable, mut error_code) = tool_output(tool.get("output"));
    metrics.output_bytes_total += bytes;
    metrics.output_bytes_max = metrics.output_bytes_max.max(bytes);
    metrics.truncated += usize::from(truncated);
    metrics.retryable_errors += usize::from(retryable);
    if tool_is_error(tool, status, &error_code) {
        metrics.error_count += 1;
        if error_code.is_empty() {
            error_code = if tool.get("error").is_some_and(|error| !error.is_null()) {
                "TOOL_EXECUTION_FAILED".to_owned()
            } else if status.is_empty() {
                "TOOL_UNKNOWN".to_owned()
            } else {
                status.trim().to_ascii_uppercase()
            };
        }
        increment(&mut metrics.by_error_code, &error_code);
    }
}

fn tool_output(output: Option<&Value>) -> (usize, bool, bool, String) {
    let Some(output) = output.filter(|output| !output.is_null()) else {
        return (0, false, false, String::new());
    };
    let bytes = serde_json::to_vec(output).map_or(0, |bytes| bytes.len());
    let truncated = output
        .get("truncated")
        .and_then(Value::as_bool)
        .unwrap_or(false);
    let error = output.get("error").unwrap_or(&Value::Null);
    let retryable = error
        .get("retryable")
        .and_then(Value::as_bool)
        .unwrap_or(false);
    let code = string_field(error, "code").trim().to_ascii_uppercase();
    (bytes, truncated, retryable, code)
}

fn tool_is_error(tool: &Value, status: &str, error_code: &str) -> bool {
    if tool.get("error").is_some_and(|error| !error.is_null()) || !error_code.is_empty() {
        return true;
    }
    matches!(
        status.trim().to_ascii_uppercase().as_str(),
        "FAILED" | "TIMED_OUT" | "DENIED" | "CANCELLED" | "ERROR"
    )
}

#[derive(Default)]
struct ApprovalMetrics {
    pending: usize,
    approved: usize,
    denied: usize,
    recoverable: usize,
    pending_wait_total: i64,
    pending_wait_max: i64,
    resolution_wait_total: i64,
    resolution_wait_max: i64,
    resolution_count: i64,
}

impl ApprovalMetrics {
    fn to_value(&self, total: usize, recent: usize) -> Value {
        json!({
            "pending": self.pending,
            "total": total,
            "last7Days": recent,
            "approved": self.approved,
            "denied": self.denied,
            "recoverablePending": self.recoverable,
            "pendingWaitMs": {
                "average": if self.pending == 0 { 0 } else { self.pending_wait_total / i64::try_from(self.pending).unwrap_or(i64::MAX) },
                "max": self.pending_wait_max,
            },
            "resolutionWaitMs": {
                "average": if self.resolution_count == 0 { 0 } else { self.resolution_wait_total / self.resolution_count },
                "max": self.resolution_wait_max,
                "count": self.resolution_count,
            },
        })
    }
}

fn approval_metrics(
    approvals: &[StoredAdkApproval],
    now: OffsetDateTime,
) -> Result<ApprovalMetrics, AdkReadSnapshotError> {
    let mut metrics = ApprovalMetrics::default();
    for approval in approvals {
        let payload = decode(&approval.payload_json, "approval")?;
        let status = approval.status.trim().to_ascii_uppercase();
        let end = if status == "PENDING" {
            now
        } else {
            parse_timestamp(&approval.updated_at).unwrap_or(now)
        };
        let wait = wait_milliseconds(&approval.created_at, end);
        match status.as_str() {
            "PENDING" => {
                metrics.pending += 1;
                metrics.pending_wait_total += wait;
                metrics.pending_wait_max = metrics.pending_wait_max.max(wait);
                if !string_field(&payload, "functionCallId").trim().is_empty()
                    && !string_field(&payload, "confirmationCallId")
                        .trim()
                        .is_empty()
                {
                    metrics.recoverable += 1;
                }
            }
            "APPROVED" => accumulate_resolution(&mut metrics, wait, true),
            "DENIED" => accumulate_resolution(&mut metrics, wait, false),
            _ => {}
        }
    }
    Ok(metrics)
}

fn accumulate_resolution(metrics: &mut ApprovalMetrics, wait: i64, approved: bool) {
    if approved {
        metrics.approved += 1;
    } else {
        metrics.denied += 1;
    }
    metrics.resolution_count += 1;
    metrics.resolution_wait_total += wait;
    metrics.resolution_wait_max = metrics.resolution_wait_max.max(wait);
}

fn agent_provider_map(
    port: &ProductionAdkPort,
) -> Result<BTreeMap<String, String>, AdkReadSnapshotError> {
    port.store
        .list_agents()?
        .into_iter()
        .map(|agent| {
            let payload = decode(&agent.payload_json, "agent")?;
            Ok((agent.id, string_field(&payload, "providerId").to_owned()))
        })
        .collect()
}

fn provider_id<'a>(
    run: &'a Value,
    agent_id: &str,
    agent_providers: &'a BTreeMap<String, String>,
) -> &'a str {
    let run_provider = string_field(run, "providerId").trim();
    if !run_provider.is_empty() {
        return run_provider;
    }
    let agent_provider = agent_providers.get(agent_id).map_or("", String::as_str).trim();
    if agent_provider.is_empty() {
        "unbound"
    } else {
        agent_provider
    }
}

fn counts<'a>(items: impl Iterator<Item = &'a str>) -> BTreeMap<String, usize> {
    let mut counts = BTreeMap::new();
    for item in items {
        increment(&mut counts, item);
    }
    counts
}

fn increment(counts: &mut BTreeMap<String, usize>, key: &str) {
    *counts.entry(key.to_owned()).or_default() += 1;
}

fn decode(raw: &str, resource: &str) -> Result<Value, AdkReadSnapshotError> {
    serde_json::from_str(raw).map_err(|error| invalid_payload(resource, error))
}

fn string_field<'a>(value: &'a Value, key: &str) -> &'a str {
    value.get(key).and_then(Value::as_str).unwrap_or_default()
}

fn integer_field(value: &Value, key: &str) -> i64 {
    value.get(key).and_then(Value::as_i64).unwrap_or_default()
}

fn enabled(status: &str) -> bool {
    status.trim().eq_ignore_ascii_case("ENABLED")
}

fn in_window(value: &str, since: OffsetDateTime) -> bool {
    parse_timestamp(value).is_some_and(|timestamp| timestamp >= since)
}

fn wait_milliseconds(created: &str, end: OffsetDateTime) -> i64 {
    let Some(created) = parse_timestamp(created) else {
        return 0;
    };
    i64::try_from((end - created).whole_milliseconds().max(0)).unwrap_or(i64::MAX)
}

fn parse_timestamp(value: &str) -> Option<OffsetDateTime> {
    OffsetDateTime::parse(value.trim(), &Rfc3339).ok()
}

fn format_timestamp(value: OffsetDateTime) -> Result<String, AdkReadSnapshotError> {
    value
        .format(&Rfc3339)
        .map_err(|error| AdkReadSnapshotError::Unavailable(format!("format ADK metrics timestamp: {error}")))
}
