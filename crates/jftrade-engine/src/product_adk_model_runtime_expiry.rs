// Run-level expiry reconciliation (Go `Runtime.ReconcileExpiredRuns`).
//
// The reference scans every durable run before serving the runs list, the run
// detail route and `CancelRun`: a run that outlived its own frozen
// `maxDurationMs` is cancelled, its still-RUNNING tool calls are failed, and
// the run is projected as `TIMED_OUT` with the lifecycle audit row.  The Rust
// port had no run-level expiry at all, so a worker that died mid-run left a
// `RUNNING` row forever even though the run budget had been exhausted.

impl ProductionAdkChatRuntime {
    /// Go `Runtime.ReconcileExpiredRuns`.  Returns an error only when the
    /// durable scan itself fails; a run that cannot be updated (its lease is
    /// held elsewhere, or another writer changed it) is left untouched, which
    /// mirrors Go skipping foreign leases and leaving the next scan to retry.
    pub(crate) fn reconcile_expired_runs(&self) -> Result<(), AdkChatPortError> {
        let runs = self.store.list_runs().map_err(storage_unavailable)?;
        let now_ms = unix_now_ms();
        for run in runs {
            if !run.status.trim().eq_ignore_ascii_case("RUNNING") {
                continue;
            }
            let Ok(mut payload) = serde_json::from_str::<Value>(&run.payload_json) else {
                // A corrupt payload is handled by the startup recovery scan,
                // which owns the ADK_STORAGE_CORRUPT projection.
                continue;
            };
            let started_at = payload
                .get("startedAt")
                .and_then(Value::as_str)
                .map(str::trim)
                .filter(|value| !value.is_empty())
                .map(str::to_owned)
                .or_else(|| {
                    payload
                        .get("createdAt")
                        .and_then(Value::as_str)
                        .map(str::trim)
                        .filter(|value| !value.is_empty())
                        .map(str::to_owned)
                })
                .unwrap_or_default();
            let Some(started_ms) = parse_rfc3339_ms(&started_at) else {
                continue;
            };
            let timeout_ms = payload
                .get("maxDurationMs")
                .and_then(Value::as_i64)
                .filter(|value| *value > 0)
                .unwrap_or(
                    crate::product::product_adk_run_timeout::DEFAULT_RUN_TIMEOUT_MS,
                );
            if now_ms - started_ms < timeout_ms {
                continue;
            }
            if self.is_dormant_workflow_child_run(&run, &payload)? {
                continue;
            }
            if self.fresh_foreign_run_lease(&run.id, now_ms)? {
                continue;
            }
            // Wake (and abandon) any in-process provider call for this run
            // before the durable projection flips to TIMED_OUT.
            self.cancellation_registry.cancel(&run.id);
            let completed_at = runtime_projection::now_timestamp();
            finish_running_tool_calls(&mut payload, &completed_at);
            let failure_reason = format!(
                "run exceeded maximum duration of {}",
                go_duration_string(timeout_ms)
            );
            payload["status"] = Value::String("TIMED_OUT".to_owned());
            payload["message"] = Value::String("run timed out".to_owned());
            payload["failureReason"] = Value::String(failure_reason.clone());
            payload["errorCode"] = Value::String("RUN_TIMED_OUT".to_owned());
            payload["degraded"] = Value::Bool(true);
            payload["completedAt"] = Value::String(completed_at.clone());
            finalize_run_usage(&mut payload);
            let updated = self
                .store
                .update_run_state_if_status_and_revision(
                    &run.id,
                    "RUNNING",
                    &run.updated_at,
                    "TIMED_OUT",
                    &payload.to_string(),
                )
                .map_err(storage_unavailable)?;
            if !updated {
                continue;
            }
            self.record_run_audit(&RunAuditEvent {
                id: format!("{}:audit:run.timed_out", run.id),
                subject_id: run.id.clone(),
                kind: "run.timed_out",
                detail: "Agent run timed out.",
                metadata: terminal_audit_fields(
                    &run.id,
                    &run.agent_id,
                    "TIMED_OUT",
                    "RUN_TIMED_OUT",
                    &failure_reason,
                ),
            });
        }
        Ok(())
    }

    /// Go `Runtime.isDormantWorkflowChildRun`: a workflow child that has not
    /// started any execution activity yet is left to its parent instead of
    /// being expired, because the parent's own budget governs the tree.
    fn is_dormant_workflow_child_run(
        &self,
        run: &jftrade_store_sqlite::StoredAdkRun,
        payload: &Value,
    ) -> Result<bool, AdkChatPortError> {
        let child_idle = payload
            .get("parentRunId")
            .and_then(Value::as_str)
            .is_some_and(|parent| !parent.trim().is_empty())
            && payload
                .get("toolCalls")
                .and_then(Value::as_array)
                .is_none_or(Vec::is_empty)
            && payload
                .get("pendingApprovals")
                .and_then(Value::as_array)
                .is_none_or(Vec::is_empty)
            && ["preToolContent", "preToolReasoning", "finalMessageId"].iter().all(
                |field| {
                    payload
                        .get(*field)
                        .and_then(Value::as_str)
                        .is_none_or(|value| value.trim().is_empty())
                },
            );
        if !child_idle {
            return Ok(false);
        }
        let parent_id = payload
            .get("parentRunId")
            .and_then(Value::as_str)
            .map(str::trim)
            .unwrap_or_default()
            .to_owned();
        let Some(parent) = self.store.get_run(&parent_id).map_err(storage_unavailable)? else {
            return Ok(false);
        };
        let Ok(parent_payload) = serde_json::from_str::<Value>(&parent.payload_json) else {
            return Ok(false);
        };
        let is_workflow_parent = parent_payload
            .get("workflowStatus")
            .and_then(Value::as_str)
            .is_some_and(|status| !status.trim().is_empty());
        if !is_workflow_parent || is_terminal_run_status(&parent.status) {
            return Ok(false);
        }
        let references_child = parent_payload
            .get("childRunIds")
            .and_then(Value::as_array)
            .is_some_and(|ids| {
                ids.iter()
                    .any(|id| id.as_str().map(str::trim) == Some(run.id.as_str()))
            })
            || parent_payload
                .get("workflowPlan")
                .and_then(Value::as_array)
                .is_some_and(|steps| {
                    steps.iter().any(|step| {
                        step.get("childRunId").and_then(Value::as_str).map(str::trim)
                            == Some(run.id.as_str())
                    })
                });
        Ok(references_child)
    }

    /// Go `Runtime.freshForeignRunLease`: a lease that is still valid and owned
    /// by another executor means the run is live somewhere else.
    fn fresh_foreign_run_lease(
        &self,
        run_id: &str,
        now_ms: i64,
    ) -> Result<bool, AdkChatPortError> {
        let Some(lease) = self
            .store
            .get_run_lease(run_id)
            .map_err(storage_unavailable)?
        else {
            return Ok(false);
        };
        if lease.expires_at_unix_ms <= now_ms {
            return Ok(false);
        }
        Ok(lease.owner_id != lease_owner_id(run_id))
    }
}

/// Fail every still-RUNNING tool call the way Go's `finishToolCall` does:
/// the call is `FAILED`, carries the expiry reason, and is stamped complete.
fn finish_running_tool_calls(payload: &mut Value, completed_at: &str) {
    let Some(calls) = payload.get_mut("toolCalls").and_then(Value::as_array_mut) else {
        return;
    };
    for call in calls {
        let Some(call) = call.as_object_mut() else {
            continue;
        };
        let running = call
            .get("status")
            .and_then(Value::as_str)
            .is_some_and(|status| status.eq_ignore_ascii_case("RUNNING"));
        if !running {
            continue;
        }
        call.insert("status".to_owned(), Value::String("FAILED".to_owned()));
        call.insert(
            "error".to_owned(),
            Value::String("run timed out while waiting for model or tool completion".to_owned()),
        );
        call.insert(
            "completedAt".to_owned(),
            Value::String(completed_at.to_owned()),
        );
        call.insert("updatedAt".to_owned(), Value::String(completed_at.to_owned()));
        let duration_ms = call
            .get("startedAt")
            .and_then(Value::as_str)
            .and_then(parse_rfc3339_ms)
            .and_then(|started| parse_rfc3339_ms(completed_at).map(|done| done - started));
        if let Some(duration_ms) = duration_ms {
            call.insert("durationMs".to_owned(), Value::from(duration_ms));
        }
    }
}

/// Go `assistantmodel.FinalizeRunUsage`: the terminal row records how long the
/// run itself took, not just the model calls inside it.
fn finalize_run_usage(payload: &mut Value) {
    let started = payload
        .get("startedAt")
        .and_then(Value::as_str)
        .and_then(parse_rfc3339_ms);
    let completed = payload
        .get("completedAt")
        .and_then(Value::as_str)
        .and_then(parse_rfc3339_ms);
    let Some((started, completed)) = started.zip(completed) else {
        return;
    };
    if let Some(usage) = payload.get_mut("usage").and_then(Value::as_object_mut) {
        usage.insert("durationMs".to_owned(), Value::from(completed - started));
    }
}

/// Go `model.IsTerminalLifecycleRunStatus`.
fn is_terminal_run_status(status: &str) -> bool {
    matches!(
        status.trim().to_ascii_uppercase().as_str(),
        "COMPLETED" | "FAILED" | "DENIED" | "CANCELLED" | "TIMED_OUT"
    )
}

fn parse_rfc3339_ms(value: &str) -> Option<i64> {
    time::OffsetDateTime::parse(value, &time::format_description::well_known::Rfc3339)
        .ok()
        .map(|parsed| (parsed.unix_timestamp_nanos() / 1_000_000) as i64)
}

/// Go `time.Duration.String()` for a millisecond budget: `1ms`/`1.5s`/`1m0s`/
/// `1h0m0s`.  Only millisecond precision can appear here because the budget is
/// frozen as `run.MaxDurationMs`.
fn go_duration_string(total_ms: i64) -> String {
    if total_ms < 1_000 {
        return format!("{total_ms}ms");
    }
    let seconds = total_ms / 1_000;
    let millis = total_ms % 1_000;
    let fraction = if millis == 0 {
        String::new()
    } else {
        let digits = format!("{millis:03}");
        format!(".{}", digits.trim_end_matches('0'))
    };
    let (hours, minutes, seconds) = (seconds / 3_600, (seconds % 3_600) / 60, seconds % 60);
    match (hours, minutes) {
        (0, 0) => format!("{seconds}{fraction}s"),
        (0, minutes) => format!("{minutes}m{seconds}{fraction}s"),
        (hours, minutes) => format!("{hours}h{minutes}m{seconds}{fraction}s"),
    }
}
