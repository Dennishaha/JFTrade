// Audit-event projection for the ADK chat runtime.
//
// Extracted from `product_adk_model_runtime.rs` as a module-scope `include!`
// so the production fragment stays under the workspace 800-line limit. Go's
// `Runtime.audit` writes one `AuditEvent` per lifecycle transition
// (`run.completed`, `run.failed`, `run.timed_out`, `run.cancelled`,
// `run.denied`, `run.awaiting_approval`, `run.awaiting_input`), and
// `GET /api/v1/adk/audit` serves those rows back. The Rust runtime used to
// write none of them, so the console's audit view was permanently empty even
// though the store and read route were fully wired.

/// Deferred audit rows for one terminal/wait transition.
///
/// Go performs the audit insert as a best-effort side effect after the run
/// save. Rust keeps it explicit so the caller can attach it to the same store
/// transaction as the run projection, which is stricter than the reference and
/// removes the window where a run is terminal but its audit row is missing.
pub(super) struct RunAuditEvent<'a> {
    /// Deterministic `<run>:audit:<kind>` id. Go generates a random UUID; a
    /// deterministic id keeps the best-effort insert idempotent under the
    /// fencing retries the Rust loop performs.
    pub(super) id: String,
    pub(super) subject_id: String,
    pub(super) kind: &'a str,
    pub(super) detail: &'a str,
    pub(super) metadata: Value,
}

/// Go's `model.RunLifecycleAuditKind`.
pub(super) fn lifecycle_audit_kind(status: &str) -> &'static str {
    match status.to_ascii_uppercase().as_str() {
        "TIMED_OUT" => "run.timed_out",
        "CANCELLED" => "run.cancelled",
        "DENIED" => "run.denied",
        "FAILED" => "run.failed",
        _ => "run.completed",
    }
}

/// Go's `model.TerminalAuditMessage`.
pub(super) fn terminal_audit_message(status: &str) -> &'static str {
    if status.eq_ignore_ascii_case("COMPLETED") {
        "Agent run completed."
    } else {
        "Agent run finished with a terminal status."
    }
}

/// Go's `model.TerminalAuditFields`: always `runId`/`agentId`/`status`, plus
/// `errorCode`/`failureReason` only when non-empty.
pub(super) fn terminal_audit_fields(
    run_id: &str,
    agent_id: &str,
    status: &str,
    error_code: &str,
    failure_reason: &str,
) -> Value {
    let mut fields = serde_json::Map::new();
    fields.insert("runId".to_owned(), Value::String(run_id.to_owned()));
    fields.insert("agentId".to_owned(), Value::String(agent_id.to_owned()));
    fields.insert("status".to_owned(), Value::String(status.to_owned()));
    for (key, value) in [("errorCode", error_code), ("failureReason", failure_reason)] {
        let value = value.trim();
        if !value.is_empty() {
            fields.insert(key.to_owned(), Value::String(value.to_owned()));
        }
    }
    Value::Object(fields)
}

/// Go's `Runtime.auditResumedRun` writes two rows for an approval
/// continuation: `run.resumed` (the wait is over) immediately followed by the
/// lifecycle kind of the status the continuation reached.
///
/// `resumeState` is carried on both rows so the console can tell an approval
/// resolution apart from a plain terminal run, and the terminal row repeats
/// `failureReason` for the failure/denial cases.
pub(super) fn record_resumed_run_audit(
    store: &jftrade_store_sqlite::AdkStore,
    run_id: &str,
    agent_id: &str,
    status: &str,
    resume_state: &str,
    failure_reason: &str,
) {
    let mut resumed_metadata = serde_json::Map::new();
    resumed_metadata.insert("runId".to_owned(), Value::String(run_id.to_owned()));
    resumed_metadata.insert("agentId".to_owned(), Value::String(agent_id.to_owned()));
    resumed_metadata.insert("status".to_owned(), Value::String(status.to_owned()));
    resumed_metadata.insert(
        "resumeState".to_owned(),
        Value::String(resume_state.to_owned()),
    );
    let resumed = RunAuditEvent {
        id: format!("{run_id}:audit:run.resumed"),
        subject_id: run_id.to_owned(),
        kind: "run.resumed",
        detail: "Agent run resumed after approval resolution.",
        metadata: Value::Object(resumed_metadata),
    };
    let mut terminal_metadata = resumed.metadata.clone();
    if let Some(object) = terminal_metadata.as_object_mut() {
        object.insert(
            "failureReason".to_owned(),
            Value::String(failure_reason.to_owned()),
        );
    }
    let terminal = RunAuditEvent {
        id: format!("{run_id}:audit:{}", lifecycle_audit_kind(status)),
        subject_id: run_id.to_owned(),
        kind: lifecycle_audit_kind(status),
        detail: "Agent run reached a terminal state after approval resolution.",
        metadata: terminal_metadata,
    };
    record_audit_event(store, &resumed);
    record_audit_event(store, &terminal);
}

/// Go's `Runtime.audit`: insert the row with `{"id","kind","subjectId",
/// "detail","metadata","createdAt"}` as the payload and the same id/kind/
/// subject/createdAt columns. A store fault is logged, never propagated, so an
/// audit write can never turn a successful run into a failure.
impl ProductionAdkChatRuntime {
    pub(super) fn record_run_audit(&self, event: &RunAuditEvent<'_>) {
        record_audit_event(self.store.as_ref(), event);
    }
}

/// Shared best-effort insert so the runtime and the resume helper keep one
/// classification of "already audited" versus a real store fault.
fn record_audit_event(store: &jftrade_store_sqlite::AdkStore, event: &RunAuditEvent<'_>) {
    let payload = json!({
        "id": event.id,
        "kind": event.kind,
        "subjectId": event.subject_id,
        "detail": event.detail,
        "metadata": event.metadata,
    });
    match store.record_audit_event(
        &event.id,
        event.kind,
        &event.subject_id,
        &payload.to_string(),
    ) {
        Ok(()) => {}
        // A deterministic id already present means this transition was
        // audited by an earlier fenced attempt; keep the first row.
        Err(error) if error.to_string().to_ascii_lowercase().contains("unique") => {}
        Err(error) => eprintln!("failed to record ADK audit event {}: {error}", event.id),
    }
}
