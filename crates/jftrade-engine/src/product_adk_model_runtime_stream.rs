//! Streaming lifecycle and terminal persistence for the production ADK model runtime.
//!
//! Keeping this implementation separate from provider selection and the
//! transport adapter keeps the runtime module below the repository's file-size
//! limit.  Run projections are committed with status compare-and-swap (CAS),
//! so a concurrent user cancellation cannot be overwritten by a late model
//! completion.

use std::collections::BTreeMap;
use std::sync::Arc;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::mpsc::SyncSender;

use jftrade_api::{ApiStream, ApiStreamSender};
use jftrade_store_sqlite::AdkRunEvent;
use serde_json::{Value, json};

use super::{
    RunAuditEvent, lifecycle_audit_kind, record_resumed_run_audit, terminal_audit_fields,
    terminal_audit_message,
};
use crate::product::product_adk_chat_stream_port::{
    AdkChatInput, AdkChatLiveStream, AdkChatPortError, AdkChatPortOutput, AdkChatRoute,
    AdkChatStreamFrame, AdkChatStreamSnapshot,
};

use super::runtime_projection;
use super::{
    ChatExecution, ModelResponse, ProductionAdkChatRuntime, RunLeaseGuard, ToolCallStaging,
    run_cancelled, storage_unavailable, stream_from_payload, unavailable,
};

#[path = "product_adk_model_runtime_stream_events.rs"]
mod stream_events;

impl ProductionAdkChatRuntime {
    pub(super) fn start_live_stream(
        &self,
        input: AdkChatInput,
        stream: ApiStream,
        sender: ApiStreamSender,
        started: SyncSender<Result<AdkChatPortOutput, AdkChatPortError>>,
        supervisor_cancel: Arc<AtomicBool>,
    ) {
        let prepared = self.prepare_chat(AdkChatRoute::Stream, &input);
        let chat = match prepared {
            Ok(super::PreparedChat::Existing(AdkChatPortOutput::Stream(snapshot)))
                if !snapshot.terminal =>
            {
                let output = AdkChatPortOutput::LiveStream(AdkChatLiveStream {
                    headers: snapshot.headers.clone(),
                    stream,
                });
                if started.send(Ok(output)).is_ok() {
                    self.tail_existing_stream(snapshot, sender, supervisor_cancel);
                }
                return;
            }
            Ok(super::PreparedChat::Existing(output)) => {
                let _ = started.send(Ok(output));
                return;
            }
            Ok(super::PreparedChat::New(chat, run_lease, run_slot)) => (chat, run_lease, run_slot),
            Err(error) => {
                let _ = started.send(Err(error));
                return;
            }
        };
        // Hold Go's `runSem` slot until the live stream finishes; the binding
        // is intentionally kept for the whole function scope.
        let (chat, run_lease, _run_slot) = chat;
        if sender.send(b"retry: 3000\n\n".to_vec()).is_err() {
            let disconnect = client_disconnected();
            let _ = self.persist_cancelled(&chat, &disconnect, &run_lease);
            let _ = started.send(Err(AdkChatPortError::Failed {
                status: 499,
                code: "CLIENT_DISCONNECTED".to_owned(),
                message: "assistant chat client disconnected".to_owned(),
            }));
            return;
        }
        // Go's `executeADKChatStream` calls `previewSession()` before running
        // the chat, so the first frame of a live stream is the `session`
        // event and only then the `run` snapshot.  The console binds the
        // transcript to the session id from that frame.
        // The auto-compaction deltas Go publishes through `onDelta` arrive
        // before the run exists, so they lead the session and run frames.
        for delta in &chat.context_deltas {
            if sender
                .send(super::encode_sse_event(&delta.sse_frame()))
                .is_err()
            {
                let disconnect = client_disconnected();
                let _ = self.persist_cancelled(&chat, &disconnect, &run_lease);
                let _ = started.send(Err(AdkChatPortError::Failed {
                    status: 499,
                    code: "CLIENT_DISCONNECTED".to_owned(),
                    message: "assistant chat client disconnected".to_owned(),
                }));
                return;
            }
        }
        if let Err(error) = self.emit_preview_session(&chat, &sender, &run_lease) {
            let _ = started.send(Err(error));
            return;
        }
        let initial = json!({
            "type": "run",
            "run": {"id": chat.run_id, "sessionId": chat.session_id, "agentId": chat.agent_id, "status": "RUNNING"},
        });
        let initial_event = match self.emit_stream_event(&chat, initial, Some(&sender), &run_lease)
        {
            Ok(event) => event,
            Err(error) => {
                if super::is_client_disconnect(&error) {
                    let _ = self.persist_cancelled(&chat, &error, &run_lease);
                }
                let _ = started.send(Err(error));
                return;
            }
        };
        if sender
            .send(super::encode_sse_event(&initial_event))
            .is_err()
        {
            let disconnect = client_disconnected();
            let _ = self.persist_cancelled(&chat, &disconnect, &run_lease);
            let _ = started.send(Err(disconnect));
            return;
        }
        let cancellation = self.cancellation_registry.register(&chat.run_id);
        let output = AdkChatPortOutput::LiveStream(AdkChatLiveStream {
            headers: BTreeMap::from([(String::from("X-ADK-Stream-ID"), chat.run_id.clone())]),
            stream,
        });
        if started.send(Ok(output)).is_err() {
            self.cancellation_registry
                .unregister(&chat.run_id, &cancellation);
            let _ = self.persist_cancelled(&chat, &client_disconnected(), &run_lease);
            return;
        }
        self.run_live_stream(chat, sender, cancellation, run_lease);
    }

    fn run_live_stream(
        &self,
        chat: super::ChatExecution,
        sender: ApiStreamSender,
        cancellation: Arc<AtomicBool>,
        run_lease: RunLeaseGuard,
    ) {
        let _guard = super::CancellationGuard {
            registry: Arc::clone(&self.cancellation_registry),
            run_id: chat.run_id.clone(),
            token: Arc::clone(&cancellation),
        };
        let result = super::stream_adapter::execute_model_stream(
            chat.request.clone(),
            |event| self.forward_provider_event(&chat, event, &sender, &run_lease),
            || {
                sender.is_closed()
                    || cancellation.load(Ordering::Acquire)
                    || self.run_is_cancelled(&chat.run_id)
            },
        );
        // A takeover may happen after the provider returns but before the
        // terminal projection is written.  Never let a stale stream worker
        // publish a late success/failure over the new owner's run state.
        if run_lease.is_lost() {
            return;
        }
        match result {
            Ok(model_response) if !model_response.tool_calls.is_empty() => {
                match self.persist_tool_calls(&chat, &model_response, &run_lease) {
                    Ok(ToolCallStaging::Pending(AdkChatPortOutput::Json(response))) => {
                        let event = self
                            .emit_post_terminal_event(
                                &chat,
                                // The browser stream contract has no
                                // `pending` event.  Approval waits are
                                // terminal from the transport perspective;
                                // the embedded run status and
                                // pendingApprovals carry the resumable state.
                                json!({"type": "final", "response": response}),
                                "PENDING",
                                &run_lease,
                            )
                            .unwrap_or_else(|_| {
                                json!({"type": "error", "message": "assistant tool call staging failed"})
                            });
                        let _ = sender.send(super::encode_sse_event(&event));
                    }
                    Ok(ToolCallStaging::Pending(_)) => {}
                    Ok(ToolCallStaging::Released) => {
                        // The released calls keep the run RUNNING; the loop
                        // below executes them and publishes the final frame.
                        self.run_tool_loop_stream(&chat, &sender, &cancellation, &run_lease);
                    }
                    Err(error) => {
                        let event = self
                            .emit_post_terminal_event(
                                &chat,
                                json!({
                                    "type": "error",
                                    "message": super::format_adk_error(&error),
                                }),
                                "FAILED",
                                &run_lease,
                            )
                            .unwrap_or_else(|_| {
                                json!({
                                    "type": "error",
                                    "message": super::format_adk_error(&error),
                                })
                            });
                        let _ = sender.send(super::encode_sse_event(&event));
                    }
                }
            }
            Ok(model_response) => match self.persist_success(&chat, model_response, &run_lease) {
                Ok(response) => {
                    if let Ok(Some(event)) = self.latest_stream_event(&chat.run_id) {
                        let _ = sender.send(super::encode_sse_event(&event));
                    } else {
                        let _ = sender.send(super::encode_sse_event(&json!({
                            "type": "final",
                            "response": response,
                        })));
                    }
                }
                Err(error) => {
                    if super::is_client_disconnect(&error)
                        || self.run_is_cancelled(&chat.run_id)
                        || super::is_run_cancelled(&error)
                    {
                        let _ = self.persist_cancelled(&chat, &error, &run_lease);
                        return;
                    }
                    let persisted = match self.persist_failure(&chat, &error, &run_lease) {
                        Ok(()) => true,
                        Err(persist_error)
                            if super::is_run_cancelled(&persist_error)
                                || self.run_is_cancelled(&chat.run_id) =>
                        {
                            return;
                        }
                        Err(_) => false,
                    };
                    let event = if persisted {
                        self.latest_stream_event(&chat.run_id)
                            .ok()
                            .flatten()
                            .unwrap_or_else(|| {
                                json!({"type":"error","message":super::format_adk_error(&error)})
                            })
                    } else {
                        json!({"type":"error","message":super::format_adk_error(&error)})
                    };
                    let _ = sender.send(super::encode_sse_event(&event));
                }
            },
            Err(error) => {
                if super::is_client_disconnect(&error)
                    || self.run_is_cancelled(&chat.run_id)
                    || super::is_run_cancelled(&error)
                {
                    let _ = self.persist_cancelled(&chat, &error, &run_lease);
                    return;
                }
                // Go publishes the terminal projection for a provider failure
                // (`publishTerminalError` -> `RecoverTerminalChatResponse`
                // emits `final` with the failed run and its synthetic reply).
                // The persisted terminal row already carries that projection,
                // so the retained history supplies the frame; the bare `error`
                // event is only the degenerate case where nothing was stored.
                let persisted = match self.persist_failure(&chat, &error, &run_lease) {
                    Ok(()) => true,
                    Err(persist_error)
                        if super::is_run_cancelled(&persist_error)
                            || self.run_is_cancelled(&chat.run_id) =>
                    {
                        return;
                    }
                    Err(_) => false,
                };
                let event = if persisted {
                    self.latest_stream_event(&chat.run_id)
                        .ok()
                        .flatten()
                        .unwrap_or_else(
                            || json!({"type":"error","message":super::format_adk_error(&error)}),
                        )
                } else {
                    json!({"type":"error","message":super::format_adk_error(&error)})
                };
                let _ = sender.send(super::encode_sse_event(&event));
            }
        }
    }

    /// Execute the calls a live stream released without confirmation and
    /// publish whatever terminal frame the run loop persisted.
    ///
    /// A stream that only needed automatically executable tools must finish in
    /// the same connection: Go's ADK loop runs those tools inline and emits the
    /// `final` event afterwards, so the client never sees a false approval
    /// wait.
    pub(super) fn run_tool_loop_stream(
        &self,
        chat: &super::ChatExecution,
        sender: &ApiStreamSender,
        cancellation: &Arc<AtomicBool>,
        run_lease: &RunLeaseGuard,
    ) {
        self.run_tool_loop(chat.clone(), Arc::clone(cancellation), run_lease);
        let event = match self.latest_stream_event(&chat.run_id) {
            Ok(Some(event)) => event,
            Ok(None) => json!({
                "type": "error",
                "message": "assistant tool execution finished without a projection",
            }),
            Err(error) => json!({
                "type": "error",
                "message": super::format_adk_error(&error),
            }),
        };
        let _ = sender.send(super::encode_sse_event(&event));
    }

    pub(super) fn forward_provider_event(
        &self,
        chat: &super::ChatExecution,
        provider_event: &Value,
        sender: &ApiStreamSender,
        run_lease: &RunLeaseGuard,
    ) -> Result<(), AdkChatPortError> {
        if run_lease.is_lost() {
            return Err(unavailable("assistant run execution lease was lost"));
        }
        let prior_text = self.stream_text_prefix(&chat.run_id)?;
        self.append_provider_event(chat, provider_event, run_lease)?;
        if provider_event.get("type").and_then(Value::as_str) != Some("response.output_text.delta")
        {
            return Ok(());
        }
        let Some(delta) = provider_event.get("delta").and_then(Value::as_str) else {
            return Ok(());
        };
        if delta.is_empty() {
            return Ok(());
        }
        let value = json!({
            "type": "timeline",
            "timeline": {
                "id": format!("{}:assistant", chat.run_id),
                "kind": "assistant_message",
                "status": "streaming",
                "text": format!("{prior_text}{delta}"),
            },
        });
        if run_lease.is_lost() {
            return Err(unavailable("assistant run execution lease was lost"));
        }
        let event = self.emit_stream_event(chat, value, Some(sender), run_lease)?;
        let _ = sender.send(super::encode_sse_event(&event));
        Ok(())
    }

    pub(super) fn run_is_cancelled(&self, run_id: &str) -> bool {
        self.store()
            .get_run(run_id)
            .ok()
            .flatten()
            .is_some_and(|run| run.status.eq_ignore_ascii_case("CANCELLED"))
    }

    pub(super) fn run_state_changed(&self, run_id: &str) -> AdkChatPortError {
        if self.run_is_cancelled(run_id) {
            AdkChatPortError::Failed {
                status: 499,
                code: "RUN_CANCELLED".to_owned(),
                message: "assistant chat run was cancelled".to_owned(),
            }
        } else {
            super::unavailable("assistant chat run state changed while streaming")
        }
    }

    pub(super) fn latest_stream_event(
        &self,
        run_id: &str,
    ) -> Result<Option<Value>, AdkChatPortError> {
        let Some(run) = self
            .store()
            .get_run(run_id)
            .map_err(super::storage_unavailable)?
        else {
            return Ok(None);
        };
        let payload: Value =
            serde_json::from_str(&run.payload_json).map_err(super::storage_unavailable)?;
        Ok(payload
            .get("streamEvents")
            .and_then(Value::as_array)
            .and_then(|events| events.last().cloned()))
    }

    pub(super) fn finish_chat(
        &self,
        chat: &ChatExecution,
        result: Result<ModelResponse, AdkChatPortError>,
        run_lease: &RunLeaseGuard,
    ) -> Result<AdkChatPortOutput, AdkChatPortError> {
        match result {
            Ok(model_response) => {
                let response = self.persist_success(chat, model_response, run_lease)?;
                if chat.route == AdkChatRoute::Chat {
                    Ok(AdkChatPortOutput::Json(response))
                } else {
                    let run = self
                        .store()
                        .get_run(&chat.run_id)
                        .map_err(storage_unavailable)?
                        .ok_or_else(|| unavailable("persisted ADK run disappeared"))?;
                    stream_from_payload(&run.payload_json)
                }
            }
            Err(error) => {
                // Go's `CompleteChatRun` treats *every* provider failure as a
                // terminal run: `markFailedChatRun` + `PersistRunTerminalState`
                // record the failure, `AttachFinalAssistantMessage` links the
                // synthetic `userFacingADKError` reply, and the caller returns
                // `ProjectedChatResponse`.  The chat handler then answers
                // `200 ok=true` with a FAILED run plus `reply`; the stream
                // route publishes the same projection as its `final` frame.
                // Rust previously kept a retryable outage durable
                // (`providerRetry` + `502`), which Go never does, so the
                // failure is persisted as terminal here and the projection is
                // returned instead of the raw error.
                let persisted = self.persist_failure(chat, &error, run_lease);
                let run = self
                    .store()
                    .get_run(&chat.run_id)
                    .map_err(storage_unavailable)?
                    .ok_or_else(|| unavailable("persisted ADK run disappeared"))?;
                if let Some(response) = super::persisted_response(&run.payload_json)? {
                    return Ok(match chat.route {
                        AdkChatRoute::Chat => AdkChatPortOutput::Json(response),
                        AdkChatRoute::Stream => stream_from_payload(&run.payload_json)?,
                    });
                }
                // The terminal row could not carry a projection (storage
                // corruption or a concurrent cancellation), so keep the
                // original provider error visible.
                persisted?;
                Err(error)
            }
        }
    }

    /// Go's `ProjectedChatResponse` timeline: `Store.SessionTimeline` for the
    /// session, i.e. the stored transcript rather than a single synthetic
    /// entry.  The frozen `api-transport` chat fixtures carry a `user_message`
    /// entry followed by the assistant reply, which is what the console renders
    /// when it reloads a finished turn.
    pub(super) fn session_timeline_value(
        &self,
        session_id: &str,
        pending: Option<runtime_projection::PendingTimelineEntry<'_>>,
    ) -> Result<Vec<Value>, AdkChatPortError> {
        let events = self
            .session_store
            .list_events(session_id)
            .map_err(storage_unavailable)?;
        Ok(runtime_projection::session_timeline(&events, pending))
    }

    fn store(&self) -> &std::sync::Arc<jftrade_store_sqlite::AdkStore> {
        &self.store
    }
}

pub(super) fn client_disconnected() -> AdkChatPortError {
    AdkChatPortError::Failed {
        status: 499,
        code: "CLIENT_DISCONNECTED".to_owned(),
        message: "assistant chat client disconnected".to_owned(),
    }
}

include!("product_adk_model_runtime_stream_success.rs");
include!("product_adk_model_runtime_failure.rs");
