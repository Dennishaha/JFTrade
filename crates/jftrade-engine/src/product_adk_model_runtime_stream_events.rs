//! Durable event projection helpers for the ADK stream runtime.

use jftrade_api::ApiStreamSender;
use jftrade_store_sqlite::AdkRunEvent;
use serde_json::{Value, json};
use std::sync::Arc;
use std::sync::atomic::{AtomicBool, Ordering};
use std::thread;
use std::time::Duration;

use super::{
    AdkChatPortError, AdkChatStreamFrame, AdkChatStreamSnapshot, ChatExecution,
    ProductionAdkChatRuntime, RunLeaseGuard, storage_unavailable, unavailable,
};
use crate::product::product_adk_model_runtime::encode_sse_event;

impl ProductionAdkChatRuntime {
    pub(super) fn emit_post_terminal_event(
        &self,
        chat: &ChatExecution,
        mut event: Value,
        expected_status: &str,
        run_lease: &RunLeaseGuard,
    ) -> Result<Value, AdkChatPortError> {
        let run = self
            .store()
            .get_run(&chat.run_id)
            .map_err(storage_unavailable)?
            .ok_or_else(|| unavailable("persisted ADK run disappeared"))?;
        if !run.status.eq_ignore_ascii_case(expected_status) {
            return Err(self.run_state_changed(&chat.run_id));
        }
        let mut payload: Value =
            serde_json::from_str(&run.payload_json).map_err(storage_unavailable)?;
        let events = payload
            .get_mut("streamEvents")
            .and_then(Value::as_array_mut)
            .ok_or_else(|| unavailable("persisted ADK run has no stream event list"))?;
        let sequence = events.len() as u64 + 1;
        if let Some(object) = event.as_object_mut() {
            object.insert("streamId".to_owned(), Value::String(chat.run_id.clone()));
            object.insert("sequence".to_owned(), Value::from(sequence));
            object.insert("runId".to_owned(), Value::String(chat.run_id.clone()));
        }
        events.push(event.clone());
        let event_id = format!("{}:stream:{}", chat.run_id, sequence);
        let updated = self
            .store()
            .update_run_payload_if_status_and_revision_with_events_with_lease(
                &chat.run_id,
                expected_status,
                &run.updated_at,
                &payload.to_string(),
                self.session_store.as_ref(),
                &[adk_run_event(
                    &event_id,
                    &chat.session_id,
                    &chat.run_id,
                    "assistant.stream",
                    event
                        .get("message")
                        .and_then(Value::as_str)
                        .unwrap_or_default(),
                )],
                run_lease.owner_id(),
                run_lease.token(),
            )
            .map_err(storage_unavailable)?;
        if !updated {
            return Err(self.run_state_changed(&chat.run_id));
        }
        Ok(event)
    }

    pub(super) fn stream_text_prefix(&self, run_id: &str) -> Result<String, AdkChatPortError> {
        let Some(run) = self.store().get_run(run_id).map_err(storage_unavailable)? else {
            return Ok(String::new());
        };
        let payload: Value =
            serde_json::from_str(&run.payload_json).map_err(storage_unavailable)?;
        let mut text = String::new();
        if let Some(events) = payload.get("streamEvents").and_then(Value::as_array) {
            for event in events {
                if event.get("type").and_then(Value::as_str) != Some("timeline") {
                    continue;
                }
                if let Some(value) = event.pointer("/timeline/text").and_then(Value::as_str) {
                    text = value.to_owned();
                }
            }
        }
        Ok(text)
    }

    pub(super) fn append_provider_event(
        &self,
        chat: &ChatExecution,
        event: &Value,
        run_lease: &RunLeaseGuard,
    ) -> Result<(), AdkChatPortError> {
        let run = self
            .store()
            .get_run(&chat.run_id)
            .map_err(storage_unavailable)?
            .ok_or_else(|| unavailable("persisted ADK run disappeared"))?;
        let mut payload: Value =
            serde_json::from_str(&run.payload_json).map_err(storage_unavailable)?;
        let provider_events = payload
            .get_mut("providerEvents")
            .and_then(Value::as_array_mut)
            .ok_or_else(|| unavailable("persisted ADK run has no provider event list"))?;
        let sequence = provider_events.len() as u64 + 1;
        provider_events.push(event.clone());
        let event_id = format!("{}:provider:{}", chat.run_id, sequence);
        let event_content = event.to_string();
        let updated = self
            .store()
            .update_run_payload_if_status_and_revision_with_events_with_lease(
                &chat.run_id,
                "RUNNING",
                &run.updated_at,
                &payload.to_string(),
                self.session_store.as_ref(),
                &[adk_run_event(
                    &event_id,
                    &chat.session_id,
                    &chat.run_id,
                    "assistant.provider",
                    &event_content,
                )],
                run_lease.owner_id(),
                run_lease.token(),
            )
            .map_err(storage_unavailable)?;
        if !updated {
            return Err(self.run_state_changed(&chat.run_id));
        }
        Ok(())
    }

    pub(super) fn tail_existing_stream(
        &self,
        snapshot: AdkChatStreamSnapshot,
        sender: ApiStreamSender,
        cancellation: Arc<AtomicBool>,
    ) {
        if sender.send(b"retry: 3000\n\n".to_vec()).is_err() {
            return;
        }
        let mut seen = 0usize;
        for frame in snapshot.frames {
            // The snapshot frames come from the durable projection, which
            // stamps Go's `replay:true` reconnect marker, so they are forwarded
            // verbatim instead of re-encoded.
            let bytes = match frame {
                AdkChatStreamFrame::Event { data, .. } => encode_sse_event(&data),
                AdkChatStreamFrame::Comment(comment) => format!("{comment}\n\n").into_bytes(),
            };
            if sender.send(bytes).is_err() {
                return;
            }
            seen = seen.saturating_add(1);
        }
        let Some(run_id) = snapshot.headers.get("X-ADK-Stream-ID").cloned() else {
            return;
        };
        let mut sent_current = false;
        loop {
            if cancellation.load(Ordering::Acquire) || sender.is_closed() {
                return;
            }
            let run = match self.store.get_run(&run_id) {
                Ok(Some(run)) => run,
                Ok(None) => return,
                Err(error) => {
                    let event = json!({
                        "type": "error",
                        "message": format!("assistant stream replay failed: {error}"),
                    });
                    let _ = sender.send(encode_sse_event(&event));
                    return;
                }
            };
            let payload: Value = match serde_json::from_str(&run.payload_json) {
                Ok(payload) => payload,
                Err(error) => {
                    let event = json!({
                        "type": "error",
                        "message": format!("assistant stream replay is corrupt: {error}"),
                    });
                    let _ = sender.send(encode_sse_event(&event));
                    return;
                }
            };
            if !sent_current {
                let current = json!({"type": "run", "run": payload.clone()});
                if sender.send(encode_sse_event(&current)).is_err() {
                    return;
                }
                sent_current = true;
            }
            let events = payload
                .get("streamEvents")
                .and_then(Value::as_array)
                .cloned()
                .unwrap_or_default();
            for event in events.iter().skip(seen) {
                if sender.send(encode_sse_event(event)).is_err() {
                    return;
                }
            }
            seen = events.len();
            if matches!(
                run.status.to_ascii_uppercase().as_str(),
                "COMPLETED" | "FAILED" | "TIMED_OUT" | "CANCELLED" | "DENIED" | "PENDING"
            ) {
                if !events.last().is_some_and(|event| {
                    event
                        .get("type")
                        .and_then(Value::as_str)
                        .is_some_and(|kind| matches!(kind, "final" | "error"))
                }) {
                    let current = json!({"type": "run", "run": payload});
                    let _ = sender.send(encode_sse_event(&current));
                }
                return;
            }
            thread::sleep(Duration::from_millis(100));
        }
    }

    /// Publishes Go's `previewSession()` frame: the durable session payload
    /// (with the persisted id/createdAt/updatedAt pinned) is emitted and
    /// persisted as the first event of a live `/chat/stream` response, before
    /// the `run` snapshot and the model call.  Returning `Err` asks the caller
    /// to publish the failure and stop the stream.
    pub(super) fn emit_preview_session(
        &self,
        chat: &ChatExecution,
        sender: &ApiStreamSender,
        run_lease: &RunLeaseGuard,
    ) -> Result<(), AdkChatPortError> {
        let session = match self.store().get_session(&chat.session_id) {
            Ok(Some(session)) => session,
            // A stream that created its session in the same transaction
            // always has one; a missing row keeps Go's "no preview" behavior.
            Ok(None) => return Ok(()),
            Err(error) => return Err(storage_unavailable(error)),
        };
        let session_value = match serde_json::from_str::<Value>(&session.payload_json) {
            Ok(mut payload) => {
                if let Some(object) = payload.as_object_mut() {
                    object.insert("id".to_owned(), Value::String(session.id.clone()));
                    object.insert(
                        "createdAt".to_owned(),
                        Value::String(session.created_at.clone()),
                    );
                    object.insert(
                        "updatedAt".to_owned(),
                        Value::String(session.updated_at.clone()),
                    );
                }
                payload
            }
            Err(error) => return Err(storage_unavailable(error)),
        };
        let session_event = serde_json::json!({"type": "session", "session": session_value});
        let session_frame = self.emit_stream_event(chat, session_event, Some(sender), run_lease)?;
        match sender.send(encode_sse_event(&session_frame)) {
            Ok(()) => Ok(()),
            Err(_) => {
                let disconnect = super::client_disconnected();
                let _ = self.persist_cancelled(chat, &disconnect, run_lease);
                Err(disconnect)
            }
        }
    }

    pub(super) fn emit_stream_event(
        &self,
        chat: &ChatExecution,
        mut event: Value,
        sender: Option<&ApiStreamSender>,
        run_lease: &RunLeaseGuard,
    ) -> Result<Value, AdkChatPortError> {
        let run = self
            .store()
            .get_run(&chat.run_id)
            .map_err(storage_unavailable)?
            .ok_or_else(|| unavailable("persisted ADK run disappeared"))?;
        let mut payload: Value =
            serde_json::from_str(&run.payload_json).map_err(storage_unavailable)?;
        let events = payload
            .get_mut("streamEvents")
            .and_then(Value::as_array_mut)
            .ok_or_else(|| unavailable("persisted ADK run has no stream event list"))?;
        let sequence = events.len() as u64 + 1;
        if let Some(object) = event.as_object_mut() {
            object.insert("streamId".to_owned(), Value::String(chat.run_id.clone()));
            object.insert("sequence".to_owned(), Value::from(sequence));
            object.insert("runId".to_owned(), Value::String(chat.run_id.clone()));
        }
        events.push(event.clone());
        let event_id = format!("{}:stream:{}", chat.run_id, sequence);
        let content = event
            .pointer("/timeline/text")
            .and_then(Value::as_str)
            .or_else(|| event.pointer("/response/reply").and_then(Value::as_str))
            .unwrap_or_default();
        let updated = self
            .store()
            .update_run_payload_if_status_and_revision_with_events_with_lease(
                &chat.run_id,
                "RUNNING",
                &run.updated_at,
                &payload.to_string(),
                self.session_store.as_ref(),
                &[adk_run_event(
                    &event_id,
                    &chat.session_id,
                    &chat.run_id,
                    "assistant.stream",
                    content,
                )],
                run_lease.owner_id(),
                run_lease.token(),
            )
            .map_err(storage_unavailable)?;
        if !updated {
            return Err(self.run_state_changed(&chat.run_id));
        }
        if sender.is_some_and(ApiStreamSender::is_closed) {
            return Err(AdkChatPortError::Failed {
                status: 499,
                code: "CLIENT_DISCONNECTED".to_owned(),
                message: "assistant chat client disconnected".to_owned(),
            });
        }
        Ok(event)
    }
}

fn adk_run_event<'a>(
    id: &'a str,
    session_id: &'a str,
    invocation_id: &'a str,
    author: &'a str,
    content: &'a str,
) -> AdkRunEvent<'a> {
    AdkRunEvent {
        id,
        session_id,
        invocation_id,
        author,
        content,
    }
}
