//! Durable recovery supervision for model-provider failures.
//!
//! A model call is allowed to fail without losing the durable run.  The run
//! remains `RUNNING`, records a bounded retry deadline and is retried by one
//! process-local supervisor.  The SQLite run lease remains the cross-process
//! fence; this worker only decides when to ask the normal continuation path to
//! try again.

use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::mpsc::{self, Sender};
use std::sync::{Arc, Mutex, Weak};
use std::thread::{self, JoinHandle};
use std::time::Duration;

use serde_json::Value;
use time::{OffsetDateTime, format_description::well_known::Rfc3339};

use super::{AdkChatPortError, ProductionAdkChatRuntime, RunLeaseGuard, lease_owner_id};

/// Keep polling responsive enough for a provider becoming available while
/// avoiding a hot loop when the runtime has a large ADK database.
const RECOVERY_POLL_INTERVAL: Duration = Duration::from_secs(1);
const RECOVERY_INITIAL_DELAY: Duration = Duration::from_millis(100);
const PROVIDER_RETRY_BASE_MS: i64 = 1_000;
const PROVIDER_RETRY_MAX_MS: i64 = 60_000;

/// Owns the one background scanner attached to a production ADK runtime.
/// The weak runtime reference prevents a cycle and lets the scanner stop
/// naturally if the runtime is dropped without an explicit shutdown call.
#[derive(Debug)]
pub(super) struct DurableRunRecoverySupervisor {
    stop: Arc<AtomicBool>,
    wake: Sender<()>,
    join: Mutex<Option<JoinHandle<()>>>,
    /// Keep an OS-thread creation failure observable.  The production
    /// runtime treats a missing scanner as not ready instead of advertising
    /// durable recovery as enabled.
    startup_error: Option<String>,
}

impl DurableRunRecoverySupervisor {
    pub(super) fn start(runtime: Weak<ProductionAdkChatRuntime>) -> Arc<Self> {
        let stop = Arc::new(AtomicBool::new(false));
        let (wake, receiver) = mpsc::channel();
        let thread_stop = Arc::clone(&stop);
        let spawn_result = thread::Builder::new()
            .name("jftrade-adk-recovery".to_owned())
            .spawn(move || {
                // `Arc::new_cyclic` publishes its strong reference just after
                // this thread is spawned.  Wait briefly rather than treating
                // the initial failed weak upgrade as a terminal condition.
                while !thread_stop.load(Ordering::Acquire) {
                    let Some(runtime) = runtime.upgrade() else {
                        if receiver.recv_timeout(RECOVERY_INITIAL_DELAY).is_ok() {
                            break;
                        }
                        continue;
                    };
                    runtime.recover_durable_provider_runs();
                    if receiver.recv_timeout(RECOVERY_POLL_INTERVAL).is_ok() {
                        break;
                    }
                }
            });
        let (join, startup_error) = match spawn_result {
            Ok(handle) => (Some(handle), None),
            Err(error) => {
                let message = format!("failed to start ADK durable recovery supervisor: {error}");
                eprintln!("{message}");
                (None, Some(message))
            }
        };
        Arc::new(Self {
            stop,
            wake,
            join: Mutex::new(join),
            startup_error,
        })
    }

    pub(super) fn is_ready(&self) -> bool {
        self.startup_error.is_none() && self.join.lock().is_ok_and(|join| join.is_some())
    }

    pub(super) fn startup_error(&self) -> Option<&str> {
        self.startup_error.as_deref()
    }

    pub(super) fn shutdown(&self) {
        self.stop.store(true, Ordering::Release);
        let _ = self.wake.send(());
        if let Ok(mut join) = self.join.lock()
            && let Some(handle) = join.take()
            && handle.thread().id() != thread::current().id()
        {
            let _ = handle.join();
        }
    }
}

impl Drop for DurableRunRecoverySupervisor {
    fn drop(&mut self) {
        self.stop.store(true, Ordering::Release);
        let _ = self.wake.send(());
        if let Ok(join) = self.join.get_mut()
            && let Some(handle) = join.take()
            && handle.thread().id() != thread::current().id()
        {
            let _ = handle.join();
        }
    }
}

impl ProductionAdkChatRuntime {
    /// Scan durable runs and hand due recoverable work to the normal
    /// continuation path.  `ContinuationSupervisor::spawn` is the process
    /// local single-worker guard, while `RunLeaseGuard` fences every SQLite
    /// mutation and provider call across processes.
    pub(super) fn recover_durable_provider_runs(&self) {
        let runs = match self.store.list_runs() {
            Ok(runs) => runs,
            Err(error) => {
                eprintln!("ADK durable recovery scan failed: {error}");
                return;
            }
        };
        for run in runs {
            if !run.status.eq_ignore_ascii_case("RUNNING") {
                continue;
            }
            let payload = match serde_json::from_str::<Value>(&run.payload_json) {
                Ok(value) => value,
                Err(error) => {
                    eprintln!(
                        "ADK durable recovery skipped run {} with invalid payload: {error}",
                        run.id
                    );
                    continue;
                }
            };
            if !recoverable_state(&payload) || !retry_is_due(&payload) {
                continue;
            }
            // A live lease means the original executor is still active.  Do
            // not create a second continuation until the lease expires.
            if self
                .store
                .get_run_lease(&run.id)
                .ok()
                .flatten()
                .is_some_and(|lease| lease.expires_at_unix_ms > unix_now_ms())
            {
                continue;
            }
            match self.resume_approval(&run.id) {
                Ok(()) | Err(AdkChatPortError::Conflict(_)) => {}
                Err(error) if is_provider_configuration_error(&error) => {
                    // Invalid provider endpoint/auth configuration is not a
                    // transient upstream failure, but it must not spin the
                    // recovery scanner or turn a durable RUNNING request
                    // into a fabricated terminal success. Probe again only
                    // after the bounded maximum interval, so a settings
                    // correction can still resume the request.
                    if let Err(persist_error) =
                        self.persist_provider_retry_for_run(&run.id, &run.session_id, &error, false)
                    {
                        eprintln!(
                            "ADK durable recovery could not persist blocked provider run {}: {persist_error:?}",
                            run.id
                        );
                    }
                }
                Err(error) if is_provider_configuration_unavailable(&error) => {
                    // Provider resolution/configuration failures are not
                    // transient retry errors, but leaving the run in a
                    // recoverable state without a marker would make the
                    // scanner hot-loop every second.  Persist a bounded,
                    // explicitly non-retryable probe marker instead.  It
                    // keeps the run RUNNING and lets a later settings fix be
                    // observed without fabricating a terminal success.
                    if let Err(persist_error) =
                        self.persist_provider_retry_for_run(&run.id, &run.session_id, &error, false)
                    {
                        eprintln!(
                            "ADK durable recovery could not persist blocked provider run {}: {persist_error:?}",
                            run.id
                        );
                    }
                }
                Err(error) if super::is_provider_retryable_error(&error) => {
                    if let Err(persist_error) =
                        self.persist_provider_retry_for_run(&run.id, &run.session_id, &error, true)
                    {
                        eprintln!(
                            "ADK durable recovery could not persist retry for run {}: {persist_error:?}",
                            run.id
                        );
                    }
                }
                Err(error @ AdkChatPortError::Unavailable(_)) => {
                    // Storage, lease and cancellation failures are not
                    // provider outages. Leave the durable run untouched and
                    // let the next scan retry after the underlying condition
                    // has recovered.
                    eprintln!("ADK durable recovery skipped run {}: {error:?}", run.id);
                }
                Err(error) => eprintln!(
                    "ADK durable recovery could not resume run {}: {error:?}",
                    run.id
                ),
            }
        }
    }

    /// Persist a retry marker under a newly acquired run lease.  This path is
    /// used when provider resolution itself fails before a continuation can be
    /// spawned, including after process restart.
    fn persist_provider_retry_for_run(
        &self,
        run_id: &str,
        session_id: &str,
        error: &AdkChatPortError,
        retryable: bool,
    ) -> Result<(), AdkChatPortError> {
        let owner_id = format!("{}:recovery", lease_owner_id(run_id));
        let run_lease = match RunLeaseGuard::acquire(Arc::clone(&self.store), run_id, &owner_id) {
            Ok(lease) => lease,
            Err(AdkChatPortError::Conflict(_)) => return Ok(()),
            Err(error) => return Err(error),
        };
        let Some(run) = self
            .store
            .get_run(run_id)
            .map_err(super::storage_unavailable)?
        else {
            return Err(super::unavailable("persisted ADK run disappeared"));
        };
        if !run.status.eq_ignore_ascii_case("RUNNING") {
            return Ok(());
        }
        // The scan payload may be stale after a concurrent request.  Always
        // build the retry marker from the current durable revision.
        let current_payload: Value =
            serde_json::from_str(&run.payload_json).map_err(super::storage_unavailable)?;
        self.persist_provider_retry_with_lease(
            run_id,
            session_id,
            &run,
            &current_payload,
            error,
            &run_lease,
            retryable,
        )
    }
}

fn is_provider_configuration_error(error: &AdkChatPortError) -> bool {
    matches!(
        error,
        AdkChatPortError::Failed { code, .. }
            if matches!(
                code.as_str(),
                "MODEL_PROVIDER_UNAVAILABLE"
                    | "MODEL_PROVIDER_UNAUTHORIZED"
                    | "MODEL_PROVIDER_FORBIDDEN"
            )
    )
}

fn is_provider_configuration_unavailable(error: &AdkChatPortError) -> bool {
    let AdkChatPortError::Unavailable(message) = error else {
        return false;
    };
    let message = message.to_ascii_lowercase();
    (message.contains("provider") || message.contains("model"))
        && [
            "not configured",
            "disabled",
            "unavailable",
            "baseurl",
            "api key",
        ]
        .iter()
        .any(|marker| message.contains(marker))
}

fn recoverable_state(payload: &Value) -> bool {
    let state = payload
        .get("resumeState")
        .and_then(Value::as_str)
        .map(str::trim)
        .unwrap_or_default()
        .to_ascii_lowercase();
    matches!(
        state.as_str(),
        "provider_waiting"
            | "provider_executing"
            | "approval_resuming"
            | "input_resuming"
            | "tool_executing"
            | "tool_result_persisted"
    ) || payload
        .get("toolCalls")
        .and_then(Value::as_array)
        .is_some_and(|calls| {
            calls.iter().any(|call| {
                call.get("status")
                    .and_then(Value::as_str)
                    .is_some_and(|status| status.eq_ignore_ascii_case("RUNNING"))
            })
        })
}

pub(super) fn retry_is_due(payload: &Value) -> bool {
    let Some(retry) = payload.get("providerRetry") else {
        return true;
    };
    let Some(retry) = retry.as_object() else {
        // A malformed marker must not turn into a hot retry loop.  Leave the
        // run durable and observable until an operator repairs its payload.
        return false;
    };
    retry
        .get("nextRetryAtUnixMs")
        .and_then(Value::as_i64)
        .is_some_and(|next| next <= unix_now_ms())
}

pub(super) fn unix_now_ms() -> i64 {
    OffsetDateTime::now_utc()
        .unix_timestamp_nanos()
        .checked_div(1_000_000)
        .and_then(|value| i64::try_from(value).ok())
        .unwrap_or(i64::MAX)
}

pub(super) fn retry_delay_ms(attempt: u64) -> i64 {
    let exponent = attempt.saturating_sub(1).min(16);
    let multiplier = 1_i64.checked_shl(exponent as u32).unwrap_or(i64::MAX);
    PROVIDER_RETRY_BASE_MS
        .saturating_mul(multiplier)
        .min(PROVIDER_RETRY_MAX_MS)
}

pub(super) const fn max_retry_delay_ms() -> i64 {
    PROVIDER_RETRY_MAX_MS
}

pub(super) fn retry_timestamp(unix_ms: i64) -> String {
    OffsetDateTime::from_unix_timestamp_nanos(i128::from(unix_ms) * 1_000_000)
        .ok()
        .and_then(|value| value.format(&Rfc3339).ok())
        .unwrap_or_else(|| "9999-12-31T23:59:59Z".to_owned())
}

pub(super) fn retry_details(error: &AdkChatPortError) -> (u16, String, String) {
    match error {
        AdkChatPortError::Unavailable(message) => {
            (503, "ADK_UNAVAILABLE".to_owned(), message.clone())
        }
        AdkChatPortError::Conflict(message) => (
            409,
            "ADK_CHAT_IDEMPOTENCY_CONFLICT".to_owned(),
            message.clone(),
        ),
        AdkChatPortError::Failed {
            status,
            code,
            message,
        } => (*status, code.clone(), message.clone()),
    }
}

#[cfg(test)]
mod tests {
    use super::super::is_provider_retryable_error;
    use super::*;
    use crate::product::product_adk_chat_stream_port::{AdkChatPortOutput, AdkChatStreamFrame};
    use jftrade_store_sqlite::{AdkStore, CreateAdkRunParams, initialize_current};
    use rusqlite::Connection;
    use serde_json::json;
    use std::fs::File;
    use tempfile::tempdir;

    #[test]
    fn provider_retry_delay_is_bounded_and_monotonic() {
        assert_eq!(retry_delay_ms(0), 1_000);
        assert_eq!(retry_delay_ms(1), 1_000);
        assert_eq!(retry_delay_ms(2), 2_000);
        assert_eq!(retry_delay_ms(100), PROVIDER_RETRY_MAX_MS);
        assert_eq!(max_retry_delay_ms(), 60_000);
    }

    #[test]
    fn provider_recovery_requires_running_state_marker() {
        assert!(recoverable_state(
            &json!({"resumeState": "provider_waiting"})
        ));
        assert!(recoverable_state(
            &json!({"resumeState": "provider_executing"})
        ));
        assert!(!recoverable_state(&json!({"resumeState": "completed"})));
    }

    #[test]
    fn running_provider_marker_survives_store_restart_for_recovery_scan() {
        let directory = tempdir().expect("temporary directory");
        let adk_path = directory.path().join("adk.db");
        File::create(&adk_path).expect("create ADK database");
        initialize_current(
            &Connection::open(&adk_path).expect("initialize ADK database"),
            "adk",
        )
        .expect("initialize ADK schema");
        {
            let store = AdkStore::open(&adk_path).expect("open ADK store");
            store
                .create_run(CreateAdkRunParams {
                    id: "run-restart-recovery",
                    session_id: "session-restart-recovery",
                    agent_id: "agent-restart-recovery",
                    status: "RUNNING",
                    client_request_id: "request-restart-recovery",
                    request_fingerprint: "fingerprint-restart-recovery",
                    payload_json: &json!({
                        "status": "RUNNING",
                        "resumeState": "provider_waiting",
                        "providerRetry": {
                            "attempt": 2,
                            "retryable": true,
                            "nextRetryAtUnixMs": unix_now_ms() - 1,
                        },
                    })
                    .to_string(),
                })
                .expect("persist running provider marker");
        }
        let reopened = AdkStore::open(&adk_path).expect("reopen ADK store");
        let run = reopened
            .get_run("run-restart-recovery")
            .expect("load recovered run")
            .expect("recovered run exists");
        let payload: Value = serde_json::from_str(&run.payload_json).expect("decode payload");
        assert_eq!(run.status, "RUNNING");
        assert!(recoverable_state(&payload));
        assert!(retry_is_due(&payload));
        assert_eq!(payload["providerRetry"]["attempt"], 2);
    }

    #[test]
    fn non_provider_unavailable_errors_are_not_treated_as_configuration_recovery() {
        assert!(is_provider_configuration_unavailable(
            &AdkChatPortError::Unavailable("assistant model provider is disabled".to_owned())
        ));
        assert!(!is_provider_configuration_unavailable(
            &AdkChatPortError::Unavailable("ADK storage unavailable".to_owned())
        ));
        assert!(!is_provider_configuration_unavailable(
            &AdkChatPortError::Unavailable("assistant run lease was lost".to_owned())
        ));
    }

    #[test]
    fn non_stream_provider_auth_rejections_are_external_and_not_retryable() {
        for (status, code, mapped_status) in [
            (
                reqwest::StatusCode::UNAUTHORIZED,
                "MODEL_PROVIDER_UNAUTHORIZED",
                502,
            ),
            (
                reqwest::StatusCode::FORBIDDEN,
                "MODEL_PROVIDER_FORBIDDEN",
                503,
            ),
        ] {
            let error = super::super::provider_rejection(
                status,
                None,
                &json!({"error": {"message": "provider rejected credentials"}}),
            );
            assert!(matches!(
                &error,
                AdkChatPortError::Failed { status, code: actual, .. }
                    if *status == mapped_status && actual == code
            ));
            assert!(!is_provider_retryable_error(&error));
        }
    }

    #[test]
    fn running_stream_payload_replays_all_nonterminal_events() {
        let output = super::super::stream_from_payload(
            &json!({
                "status": "RUNNING",
                "streamId": "run-stream-recovery",
                "streamEvents": [
                    {"type": "run", "sequence": 1},
                    {"type": "error", "sequence": 2, "retryable": true, "terminal": false}
                ]
            })
            .to_string(),
        )
        .expect("stream snapshot");
        let AdkChatPortOutput::Stream(snapshot) = output else {
            panic!("running stream payload did not produce a snapshot");
        };
        assert!(!snapshot.terminal);
        assert_eq!(snapshot.frames.len(), 2);
        assert!(matches!(
            &snapshot.frames[1],
            AdkChatStreamFrame::Event { data, .. }
                if data["terminal"] == false && data["retryable"] == true
        ));
    }

    #[test]
    fn malformed_or_future_retry_markers_never_bypass_backoff() {
        assert!(retry_is_due(&json!({"status": "RUNNING"})));
        assert!(!retry_is_due(&json!({
            "providerRetry": {"nextRetryAtUnixMs": "not-a-number"}
        })));
        assert!(!retry_is_due(&json!({
            "providerRetry": {"nextRetryAtUnixMs": unix_now_ms() + 60_000}
        })));
        assert!(retry_is_due(&json!({
            "providerRetry": {"nextRetryAtUnixMs": unix_now_ms() - 1}
        })));
    }

    #[test]
    fn provider_configuration_outages_back_off_but_auth_and_storage_errors_do_not_retry() {
        assert!(super::super::is_provider_retryable_error(
            &AdkChatPortError::Unavailable(
                "assistant model provider API key is not configured".to_owned(),
            )
        ));
        assert!(!super::super::is_provider_retryable_error(
            &AdkChatPortError::Unavailable("ADK storage unavailable".to_owned())
        ));
        assert!(!super::super::is_provider_retryable_error(
            &AdkChatPortError::Failed {
                status: 502,
                code: "MODEL_PROVIDER_UNAUTHORIZED".to_owned(),
                message: "bad credentials".to_owned(),
            }
        ));
        assert!(!super::super::is_provider_retryable_error(
            &AdkChatPortError::Failed {
                status: 503,
                code: "MODEL_PROVIDER_FORBIDDEN".to_owned(),
                message: "forbidden".to_owned(),
            }
        ));
    }

    #[test]
    fn successful_supervisor_start_is_reported_as_ready() {
        let supervisor = DurableRunRecoverySupervisor::start(Weak::new());
        assert!(supervisor.startup_error().is_none());
        assert!(supervisor.is_ready());
        supervisor.shutdown();
        assert!(!supervisor.is_ready());
    }

    #[test]
    fn provider_retry_classifier_accepts_only_transient_failures() {
        let transient = |status: u16, code: &str| AdkChatPortError::Failed {
            status,
            code: code.to_owned(),
            message: "provider error".to_owned(),
        };
        assert!(is_provider_retryable_error(&transient(
            502,
            "MODEL_CALL_FAILED"
        )));
        assert!(is_provider_retryable_error(&transient(
            504,
            "MODEL_CALL_FAILED"
        )));
        assert!(is_provider_retryable_error(&transient(
            504,
            "MODEL_CALL_TIMEOUT"
        )));
        assert!(is_provider_retryable_error(&transient(
            429,
            "MODEL_PROVIDER_RATE_LIMITED"
        )));
        assert!(!is_provider_retryable_error(&transient(
            502,
            "MODEL_PROVIDER_UNAUTHORIZED"
        )));
        assert!(!is_provider_retryable_error(&transient(
            503,
            "MODEL_PROVIDER_FORBIDDEN"
        )));
        assert!(!is_provider_retryable_error(&transient(
            502,
            "MODEL_PROVIDER_UNAVAILABLE"
        )));
        assert!(!is_provider_retryable_error(&transient(
            400,
            "MODEL_CALL_FAILED"
        )));
        assert!(is_provider_retryable_error(&AdkChatPortError::Unavailable(
            "assistant model provider API key is not configured".to_owned()
        )));
    }
}
