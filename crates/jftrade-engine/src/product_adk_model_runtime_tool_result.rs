// Structured tool-result normalization for the ADK tool loop.
//
// Extracted from `product_adk_model_runtime_tool_loop.rs` so the production
// fragment stays under the workspace 800-line architecture limit. It is a
// module-scope `include!`, so every import and helper of the parent module is
// still in scope.
//
// Go's `googleADKTool.executeAndMap` owns this projection: a handler result
// that is not an object is wrapped as `{"result": value}`, and an object that
// signals failure through the `{"success":false}` or legacy `{"error":"..."}`
// contract is re-projected as `structuredToolErrorEnvelope`.
// `consumeFunctionResponse` then records the call `FAILED`/`TIMED_OUT`/
// `CANCELLED` while the run keeps its turn and only becomes `degraded`.

/// One executor result projected onto the durable tool call.
struct MappedToolOutput {
    output: Value,
    status: String,
    /// `ToolCall.Error` when the reference records a text that differs from
    /// the persisted envelope's nested `error.message`: Go classifies the
    /// envelope's model-facing `message` (the "工具 X 返回错误: …" form).
    error_text: Option<String>,
}

fn durable_tool_completion_status(call_status: &str) -> &str {
    match call_status {
        "SUCCEEDED" | "FAILED" => "COMPLETED",
        other => other,
    }
}

fn fail_closed_tool_error(name: &str, error: AdkChatPortError, fail_closed: bool) -> AdkChatPortError {
    if fail_closed
        && matches!(&error, AdkChatPortError::Failed { code, .. } if code == "TOOL_EXECUTION_FAILED")
    {
        return AdkChatPortError::Failed {
            status: 500,
            code: "ADK_TOOL_OUTCOME_UNKNOWN".to_owned(),
            message: format!(
                "tool {name} returned an uncertain write failure: {}",
                tool_error_text(&error),
            ),
        };
    }
    error
}

fn replayed_tool_call_status(invocation: &StoredAdkToolInvocation, output: &Value) -> String {
    if !invocation.status.eq_ignore_ascii_case("COMPLETED") {
        return invocation.status.clone();
    }
    if output["success"] == false {
        return classify_tool_error_text(&tool_result_error_message(output)).0;
    }
    "SUCCEEDED".to_owned()
}

fn map_tool_output(name: &str, output: Value) -> MappedToolOutput {
    let Value::Object(object) = &output else {
        return MappedToolOutput {
            output: json!({"result": output}),
            status: "SUCCEEDED".to_owned(),
            error_text: None,
        };
    };
    let Some(message) = structured_tool_failure_message(object) else {
        return MappedToolOutput {
            output,
            status: "SUCCEEDED".to_owned(),
            error_text: None,
        };
    };
    let (code, retryable) = structured_tool_failure_metadata(object);
    let envelope_message = format!("工具 {name} 返回错误: {message}");
    let (status, error_text) = classify_tool_error_text(&envelope_message);
    MappedToolOutput {
        output: json!({
            "success": false,
            "message": envelope_message,
            "error": {"code": code, "message": message, "retryable": retryable},
            "errorCode": code,
            "retryable": retryable,
        }),
        status,
        error_text: Some(error_text),
    }
}

/// Go's `toolErrorEnvelope`: the envelope persisted when the tool handler
/// itself failed.  The nested `error.message` keeps the raw error text so the
/// durable `ToolCall.Error` is the text the reference records.
fn tool_error_envelope(name: &str, error: &AdkChatPortError) -> Value {
    let failure_text = tool_error_text(error);
    let (code, retryable) = classify_tool_failure(error);
    json!({
        "success": false,
        "message": format!("工具 {name} 执行失败: {failure_text}"),
        "error": {
            "code": code,
            "message": failure_text,
            "retryable": retryable,
        },
        "errorCode": code,
        "retryable": retryable,
    })
}

fn tool_claim_lease_lost(message: impl Into<String>) -> AdkChatPortError {
    AdkChatPortError::Failed {
        status: 409,
        code: "ADK_RUN_LEASE_LOST".to_owned(),
        message: message.into(),
    }
}

/// Go's `structuredToolError`: `{"success":false}` uses the trimmed `message`
/// (defaulting to `tool execution failed`), and the legacy `{"error": ...}`
/// shape is a failure unless the rendered text is blank or `<nil>`.
fn structured_tool_failure_message(object: &serde_json::Map<String, Value>) -> Option<String> {
    if object.is_empty() {
        return None;
    }
    if let Some(success) = object.get("success") {
        // Go reads the flag through a lenient bool assertion, so a missing or
        // non-boolean `success` counts as `false`, i.e. a failure.
        if success.as_bool().unwrap_or(false) {
            return None;
        }
        return Some(
            rendered_message(object.get("message")).unwrap_or_else(|| "tool execution failed".to_owned()),
        );
    }
    let error = rendered_message(object.get("error"))?;
    if error.eq_ignore_ascii_case("<nil>") {
        return None;
    }
    Some(error)
}

/// Go renders structured fields with `fmt.Sprint`, so the persisted message is
/// the trimmed text for strings and the JSON rendering for other values.
fn rendered_message(value: Option<&Value>) -> Option<String> {
    let text = match value? {
        Value::Null => return None,
        Value::String(text) => text.clone(),
        other => other.to_string(),
    };
    let text = text.trim().to_owned();
    if text.is_empty() { None } else { Some(text) }
}

/// Go's `structuredToolErrorMetadata`: only a nested `error` object carries
/// the code/retryability, and the code is upper-cased.  The reference leaves a
/// missing code to its `<nil>` `fmt.Sprint` artifact; Rust keeps the documented
/// `TOOL_EXECUTION_FAILED` default instead of persisting that sentinel.
fn structured_tool_failure_metadata(object: &serde_json::Map<String, Value>) -> (String, bool) {
    let Some(nested) = object.get("error").and_then(Value::as_object) else {
        return ("TOOL_EXECUTION_FAILED".to_owned(), false);
    };
    let code = nested
        .get("code")
        .and_then(Value::as_str)
        .map(|code| code.trim().to_ascii_uppercase())
        .filter(|code| !code.is_empty())
        .unwrap_or_else(|| "TOOL_EXECUTION_FAILED".to_owned());
    let retryable = nested
        .get("retryable")
        .and_then(Value::as_bool)
        .unwrap_or(false);
    (code, retryable)
}

/// Go's `ClassifyToolErrorText`: the durable status of a tool call that failed
/// through its response payload rather than through an executor error.
fn classify_tool_error_text(text: &str) -> (String, String) {
    let trimmed = text.trim();
    let lower = trimmed.to_ascii_lowercase();
    if lower.contains("context deadline exceeded") {
        (
            "TIMED_OUT".to_owned(),
            prefixed_tool_error(trimmed, "tool execution timed out"),
        )
    } else if lower.contains("context canceled") {
        (
            "CANCELLED".to_owned(),
            prefixed_tool_error(trimmed, "tool execution cancelled"),
        )
    } else {
        ("FAILED".to_owned(), trimmed.to_owned())
    }
}

/// Go's `PrefixedToolError`: prefix only when the text does not carry it yet.
fn prefixed_tool_error(text: &str, prefix: &str) -> String {
    let trimmed = text.trim();
    if trimmed.is_empty() {
        return prefix.to_owned();
    }
    if trimmed.to_ascii_lowercase().contains(prefix) {
        return trimmed.to_owned();
    }
    format!("{prefix}: {trimmed}")
}
