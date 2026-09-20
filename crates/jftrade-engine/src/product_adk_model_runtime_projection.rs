//! Go's `ProjectedChatResponse` run projection for the production ADK runtime.
//!
//! Go answers `POST /api/v1/adk/chat` from a *projection*: the persisted run is
//! merged with the tool activity of the turn and the session transcript before
//! it reaches the wire.  `internal/assistant/model` owns that merge
//! (`ToolSummariesForRun`, `OptimizationTaskID`, `MergeRunActivitySnapshot` and
//! the assistant-text merge used by `ProjectedAssistantMessageForRun`), and
//! the Rust runtime had none of it: a completed tool run exposed only the
//! final model text plus the raw `toolCalls` array, so `toolSummaries`,
//! `optimizationTaskId`, `preToolContent` and `usage.toolCallsTotal` never
//! reached the console even though the durable payload had every ingredient.
//!
//! Reference: go:452dea11:internal/assistant/model/runner_lifecycle.go and
//! go:452dea11:internal/assistant/engine/event_projection.go.

use jftrade_store_sqlite::StoredAdkEvent;
use serde_json::{Map, Value, json};

/// Byte budget Go's `SummarizeToolOutput` uses before it appends the
/// `...(truncated)` marker.
const TOOL_SUMMARY_LIMIT: usize = 1800;

/// Go's `nowString`: the RFC3339 UTC stamp `startRun` freezes on `startedAt`.
pub(super) fn now_timestamp() -> String {
    super::runtime_recovery::retry_timestamp(super::runtime_recovery::unix_now_ms())
}

/// Go's `SummarizeToolOutput`: render the tool output as one compact line.
///
/// Go slices the JSON text at a *byte* offset, which can split a multi-byte
/// rune.  Rust strings are UTF-8 by construction, so the equivalent truncation
/// happens at the closest character boundary at or below that offset.
pub(super) fn summarize_tool_output(tool_name: &str, output: &Value) -> String {
    let text = serde_json::to_string(output).unwrap_or_else(|_| output.to_string());
    let text = if text.len() > TOOL_SUMMARY_LIMIT {
        let mut end = TOOL_SUMMARY_LIMIT;
        while end > 0 && !text.is_char_boundary(end) {
            end -= 1;
        }
        format!("{}...(truncated)", &text[..end])
    } else {
        text
    };
    format!("{tool_name} => {text}")
}

/// Go's `ToolSummariesForRun`: one summary line per terminal tool call.
pub(super) fn tool_summaries_for_run(tool_calls: &[Value]) -> Vec<String> {
    let mut summaries = Vec::with_capacity(tool_calls.len());
    for call in tool_calls {
        let tool_name = call
            .get("toolName")
            .or_else(|| call.get("name"))
            .and_then(Value::as_str)
            .unwrap_or_default();
        match call
            .get("status")
            .and_then(Value::as_str)
            .unwrap_or_default()
        {
            "SUCCEEDED" => summaries.push(summarize_tool_output(
                tool_name,
                call.get("output").unwrap_or(&Value::Null),
            )),
            "FAILED" => {
                // Go only summarizes a failure that carries an error message;
                // a failed call without `Error` stays invisible there too.
                if let Some(error) = call.get("error").and_then(Value::as_str) {
                    summaries.push(format!("{tool_name} failed: {error}"));
                }
            }
            "DENIED" => summaries.push(format!("{tool_name} denied by user")),
            _ => {}
        }
    }
    summaries
}

/// Go's `OptimizationTaskID`: the `taskId` of the first successful
/// `strategy.optimize` call, so the console can deep-link the optimizer.
pub(super) fn optimization_task_id(tool_calls: &[Value]) -> String {
    for call in tool_calls {
        if call.get("toolName").and_then(Value::as_str) != Some("strategy.optimize") {
            continue;
        }
        if call.get("status").and_then(Value::as_str) != Some("SUCCEEDED") {
            continue;
        }
        if let Some(task_id) = call.pointer("/output/taskId").and_then(Value::as_str) {
            return task_id.trim().to_owned();
        }
    }
    String::new()
}

/// Go's `mergeProjectedText`.
///
/// The ADK projection folds every assistant text part of one invocation into a
/// single entry.  A partial delta always appends, an already-accumulated
/// prefix is replaced by the longer text, and a re-delivered suffix is dropped,
/// so a retried or replayed provider answer never doubles the reply.
pub(super) fn merge_projected_text(current: &str, text: &str, partial: bool) -> String {
    if text.is_empty() {
        return current.to_owned();
    }
    if partial || current.is_empty() {
        return format!("{current}{text}");
    }
    if text.starts_with(current) {
        return text.to_owned();
    }
    if current.ends_with(text) {
        return current.to_owned();
    }
    format!("{current}{text}")
}

/// Project the durable run payload over Go's wire field set.
///
/// The durable row also carries runtime bookkeeping (`streamEvents`,
/// `providerEvents`, `route`, `toolResults`, ...) that Go's `Run` JSON contract
/// never exposes, so the wire projection keeps only the frozen field set from
/// the `api-transport` fixture.
pub(super) fn run_wire_fields(payload: &Value, fields: &[&str]) -> Map<String, Value> {
    match payload.as_object() {
        Some(object) => object
            .iter()
            .filter(|(key, _)| fields.contains(&key.as_str()))
            .map(|(key, value)| (key.clone(), value.clone()))
            .collect(),
        None => Map::new(),
    }
}

/// Go's `encoding/json` tags: these `assistantmodel.Run` fields disappear from
/// the wire when they hold their zero value.
///
/// The frozen `api-transport` fixture pins the consequence: a completed run
/// carries neither `degraded: false` nor the empty `errorCode`/`failureReason`
/// pair that Rust used to publish, while a failed run keeps every one of them
/// because its values are non-zero.
const GO_OMIT_WHEN_ZERO: &[&str] = &[
    "cancelledAt",
    "childRunIds",
    "completedAt",
    "degraded",
    "errorCode",
    "failureReason",
    "finalMessageId",
    "inputRequest",
    "inputRequests",
    "iteration",
    "model",
    "objective",
    "optimizationTaskId",
    "parentRunId",
    "pauseRequestedAt",
    "pausedAt",
    "pausedReason",
    "permissionMode",
    "preToolContent",
    "preToolReasoning",
    "providerId",
    "providerName",
    "reasoningEffort",
    "resumeState",
    "startedAt",
    "toolSummaries",
    "userMessage",
    "workMode",
    "workflowCursor",
    "workflowEngine",
    "workflowPlan",
    "workflowStatus",
];

/// Apply Go's zero-value omission to a projected run object.
pub(super) fn drop_go_zero_fields(run: &mut Map<String, Value>) {
    run.retain(|key, value| !(GO_OMIT_WHEN_ZERO.contains(&key.as_str()) && go_zero_value(value)));
}

fn go_zero_value(value: &Value) -> bool {
    match value {
        Value::Null => true,
        Value::Bool(flag) => !flag,
        Value::String(text) => text.is_empty(),
        Value::Number(number) => number.as_f64().is_some_and(|number| number == 0.0),
        Value::Array(items) => items.is_empty(),
        Value::Object(_) => false,
    }
}

/// Build the Go `Run` wire object: the durable payload's allowed fields plus
/// the projection overrides, with Go's zero-value omission applied last.
pub(super) fn go_run_wire(
    payload: &Value,
    fields: &[&str],
    overrides: Vec<(&str, Value)>,
) -> Value {
    let mut object = run_wire_fields(payload, fields);
    for (key, value) in overrides {
        object.insert(key.to_owned(), value);
    }
    drop_go_zero_fields(&mut object);
    Value::Object(object)
}

/// Go's `applySessionProjectionToRun` tool fields.
pub(super) fn tool_projection_fields(tool_calls: &[Value]) -> Vec<(&'static str, Value)> {
    let mut fields = vec![("toolCalls", Value::Array(tool_calls.to_vec()))];
    let summaries = tool_summaries_for_run(tool_calls);
    if !summaries.is_empty() {
        fields.push(("toolSummaries", json!(summaries)));
    }
    let task_id = optimization_task_id(tool_calls);
    if !task_id.is_empty() {
        fields.push(("optimizationTaskId", Value::String(task_id)));
    }
    fields
}

/// Go's `FinalizeRunUsage` combined with the projected tool-call total.
///
/// `RunUsage` keeps `modelCalls`/`toolCallsTotal` unconditionally and the
/// duration only when the run actually measured one.
pub(super) fn usage_wire_value(payload: &Value, tool_calls: &[Value], completed_at: &str) -> Value {
    let rounds = tool_calls
        .iter()
        .filter_map(|call| call.get("round").and_then(Value::as_i64))
        .max()
        .unwrap_or(0);
    let model_calls = if tool_calls.is_empty() { 1 } else { 1 + rounds };
    let duration = duration_ms(
        payload
            .get("startedAt")
            .and_then(Value::as_str)
            .unwrap_or_default(),
        completed_at,
    );
    let mut usage = Map::new();
    usage.insert("modelCalls".to_owned(), json!(model_calls));
    usage.insert("toolCallsTotal".to_owned(), json!(tool_calls.len()));
    if duration > 0 {
        usage.insert("durationMs".to_owned(), json!(duration));
    }
    for key in ["tokensIn", "tokensOut"] {
        let existing = payload
            .pointer(&format!("/usage/{key}"))
            .filter(|value| value.as_i64().is_some_and(|number| number > 0));
        if let Some(existing) = existing {
            usage.insert(key.to_owned(), existing.clone());
        }
    }
    Value::Object(usage)
}

/// Go's `FinalizeRunUsage`: milliseconds between the run's start and terminal
/// stamps, or `0` when either stamp is unusable.
fn duration_ms(started_at: &str, completed_at: &str) -> i64 {
    let parse = |value: &str| {
        time::OffsetDateTime::parse(value, &time::format_description::well_known::Rfc3339).ok()
    };
    match (parse(started_at), parse(completed_at)) {
        (Some(started), Some(completed)) => (completed - started).whole_milliseconds() as i64,
        _ => 0,
    }
}

/// One transcript entry the caller is about to persist together with the
/// projection it is building.
///
/// Go appends the assistant message before it reads `SessionTimeline`, so the
/// response already contains the entry it just linked `finalMessageId` to.
/// Rust writes the run row and the event in one compare-and-swap, so the
/// pending entry is projected in memory first.
pub(super) struct PendingTimelineEntry<'a> {
    pub id: &'a str,
    pub session_id: &'a str,
    pub run_id: &'a str,
    pub text: &'a str,
    pub created_at: &'a str,
}

fn timeline_value(
    id: &str,
    session_id: &str,
    run_id: &str,
    author: &str,
    text: &str,
    created_at: &str,
    sequence: usize,
) -> Value {
    let is_user = author.trim().eq_ignore_ascii_case("user");
    json!({
        "id": id,
        "sessionId": session_id,
        "runId": run_id,
        "kind": if is_user { "user_message" } else { "assistant_message" },
        "createdAt": created_at,
        "sequence": sequence,
        "status": "final",
        "text": text,
    })
}

/// Build the session timeline Go's `ProjectedChatResponse` returns.
///
/// Go serves `Store.SessionTimeline` here, so the response repeats the whole
/// session transcript rather than a single synthetic entry.  Rust's session
/// store also holds runtime bookkeeping events (`assistant.stream` deltas and
/// `assistant.tool` result envelopes) that are not renderable transcript rows;
/// those authors stay in the durable event log and out of the projection, so a
/// streamed turn does not publish its partial deltas twice.
pub(super) fn session_timeline(
    events: &[StoredAdkEvent],
    pending: Option<PendingTimelineEntry<'_>>,
) -> Vec<Value> {
    let mut entries: Vec<Value> = events
        .iter()
        .filter(|event| !is_internal_author(&event.author))
        .enumerate()
        .map(|(sequence, event)| {
            timeline_value(
                &event.id,
                &event.session_id,
                &event.invocation_id,
                &event.author,
                &event.content,
                &event.timestamp,
                sequence + 1,
            )
        })
        .collect();
    if let Some(pending) = pending {
        let already_stored = entries
            .iter()
            .any(|entry| entry.get("id").and_then(Value::as_str) == Some(pending.id));
        if !already_stored {
            let sequence = entries.len() + 1;
            entries.push(timeline_value(
                pending.id,
                pending.session_id,
                pending.run_id,
                "assistant",
                pending.text,
                pending.created_at,
                sequence,
            ));
        }
    }
    entries
}

fn is_internal_author(author: &str) -> bool {
    matches!(
        author.trim().to_ascii_lowercase().as_str(),
        "assistant.stream" | "assistant.tool"
    )
}
