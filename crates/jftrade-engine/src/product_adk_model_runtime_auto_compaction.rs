// Session-context auto compaction (Go `Runtime.maybeAutoCompactSession` and
// `SessionContextManager.AutoCompactForModelContext`).
//
// The threshold lives on the projected next turn: the current projection plus
// the message the user is about to send.  0.85 auto-compacts, 0.93 switches to
// the aggressive failsafe, and a session without a resolved provider window
// never compacts.  The chat entry point skips a session with a live run while
// the workflow entry point is allowed to advance underneath one, which is why
// the caller passes `allow_active_run` explicitly.

use crate::product::product_production_ports::product_production_ports_adk::mutation::context;
use crate::product::product_production_ports::product_production_ports_adk::read::context_projection;
use crate::product::product_production_ports::product_production_ports_adk::read::context_window;
use crate::product::product_production_ports::product_production_ports_adk::read::notices;
use crate::product::product_production_ports::product_production_ports_adk::read::read_helpers;

/// Go `contextAutoCompactThresh` / `contextAggressiveThreshold`.
const CONTEXT_AUTO_COMPACT_THRESHOLD: f64 = 0.85;
const CONTEXT_AGGRESSIVE_THRESHOLD: f64 = 0.93;

/// One delta published by the auto compactor, mirroring Go's `ChatDelta`
/// (`Timeline` for the compaction notice, `Context` for the new snapshot).
#[derive(Debug)]
#[allow(dead_code)]
pub(crate) enum SessionContextDelta {
    Timeline(Value),
    Context(Value),
}

impl ProductionAdkChatRuntime {
    /// Go `SessionContextManager.ShouldAutoCompact`.
    pub(crate) fn auto_compaction_mode(ratio: f64) -> Option<&'static str> {
        if ratio >= CONTEXT_AGGRESSIVE_THRESHOLD {
            Some("aggressive")
        } else if ratio >= CONTEXT_AUTO_COMPACT_THRESHOLD {
            Some("normal")
        } else {
            None
        }
    }

    /// Go `SessionContextManager.ProjectedSnapshot`: the durable projection
    /// with the pending user message folded in.
    pub(crate) fn projected_context_projection(
        &self,
        session_id: &str,
        pending_text: &str,
    ) -> Option<(Value, f64, usize)> {
        let session = self.store.get_session(session_id).ok().flatten()?;
        let events = self.session_store.list_events(session_id).ok()?;
        let segments = self.store.list_handoff_segments(session_id, true).ok()?;
        let window = context_window::resolve_session_context_window_tokens(
            &self.store,
            session_id,
            &session.payload_json,
        );
        let recent_window = context_window::resolve_session_context_recent_window(
            &self.store,
            session_id,
            &session.payload_json,
        );
        let mut snapshot = context_projection::rebuild_context_snapshot(
            session_id,
            &session.payload_json,
            "",
            &events,
            &segments,
            window,
            recent_window,
        )
        .ok()?;
        if window == 0 {
            return Some((snapshot, 0.0, recent_window));
        }
        let current = snapshot
            .get("currentInputTokens")
            .and_then(Value::as_u64)
            .unwrap_or(0) as usize;
        let pending = read_helpers::estimate_context_tokens(pending_text);
        let projected = current.saturating_add(pending);
        let ratio = projected as f64 / window as f64;
        if let Some(object) = snapshot.as_object_mut() {
            object.insert("projectedNextTurnTokens".to_owned(), Value::from(projected));
            object.insert("pendingUserTokens".to_owned(), Value::from(pending));
            object.insert("usageRatio".to_owned(), Value::from(ratio));
        }
        Some((snapshot, ratio, recent_window))
    }

    /// Go `SessionContextManager.HasActiveRun`: a run waiting for approval is
    /// quiescent, so only `RUNNING` blocks an automatic compaction.
    pub(crate) fn session_has_running_run(
        &self,
        session_id: &str,
    ) -> Result<bool, AdkChatPortError> {
        Ok(self
            .store
            .list_runs()
            .map_err(storage_unavailable)?
            .into_iter()
            .any(|run| {
                run.session_id == session_id && run.status.trim().eq_ignore_ascii_case("RUNNING")
            }))
    }

    /// Go `Runtime.maybeAutoCompactSession` / `MaybeAutoCompactSessionDuringWorkflow`.
    pub(crate) fn maybe_auto_compact_session(
        &self,
        session_id: &str,
        pending_text: &str,
        allow_active_run: bool,
        mut on_delta: impl FnMut(SessionContextDelta) -> Result<(), AdkChatPortError>,
    ) -> Result<(), AdkChatPortError> {
        if session_id.trim().is_empty() {
            return Ok(());
        }
        let Some((_, ratio, recent_window)) =
            self.projected_context_projection(session_id, pending_text)
        else {
            return Ok(());
        };
        let Some(mode) = Self::auto_compaction_mode(ratio) else {
            return Ok(());
        };
        if !allow_active_run && self.session_has_running_run(session_id)? {
            return Ok(());
        }
        let (guard, acquired) =
            crate::product::product_adk_session_compaction_gate::begin_session_compaction(session_id);
        let _guard = guard;
        if !acquired {
            // Go returns quietly here: a second compactor waits for the next
            // turn instead of publishing a duplicate notice.
            return Ok(());
        }
        let Some(notice) = notices::create_context_compaction_notice(&self.store, session_id) else {
            return Ok(());
        };
        on_delta(SessionContextDelta::Timeline(notices::notice_delta_value(
            &notice,
        )))?;
        let reason = if mode == "aggressive" {
            "context usage exceeded aggressive failsafe threshold"
        } else {
            "context usage exceeded automatic compaction threshold"
        };
        match self.compact_session_context_for_runtime(
            session_id,
            mode,
            reason,
            recent_window,
        ) {
            Ok(snapshot) => {
                if let Some(notice) = notices::update_context_compaction_notice(
                    &self.store,
                    &notice,
                    notices::TIMELINE_STATUS_FINAL,
                    notices::CONTEXT_COMPACTION_DONE_TEXT,
                ) {
                    on_delta(SessionContextDelta::Timeline(notices::notice_delta_value(
                        &notice,
                    )))?;
                }
                on_delta(SessionContextDelta::Context(snapshot))?;
                Ok(())
            }
            Err(error) => {
                if let Some(notice) = notices::update_context_compaction_notice(
                    &self.store,
                    &notice,
                    notices::TIMELINE_STATUS_ERROR,
                    notices::CONTEXT_COMPACTION_FAILED_TEXT,
                ) {
                    on_delta(SessionContextDelta::Timeline(notices::notice_delta_value(
                        &notice,
                    )))?;
                }
                Err(error)
            }
        }
    }

    /// Go `SessionContextManager.AutoCompactForModelContext`: the model payload
    /// is built after this ran, so the provider sees the compacted projection.
    /// No notice is emitted on this path.
    pub(crate) fn auto_compact_for_model_context(
        &self,
        session_id: &str,
        pending_text: &str,
    ) -> Result<(), AdkChatPortError> {
        if session_id.trim().is_empty() {
            return Ok(());
        }
        let Some((_, ratio, recent_window)) =
            self.projected_context_projection(session_id, pending_text)
        else {
            return Ok(());
        };
        let Some(mode) = Self::auto_compaction_mode(ratio) else {
            return Ok(());
        };
        let (guard, acquired) =
            crate::product::product_adk_session_compaction_gate::begin_session_compaction(session_id);
        let _guard = guard;
        if !acquired {
            return Ok(());
        }
        let reason = if mode == "aggressive" {
            "context usage exceeded aggressive failsafe threshold before model call"
        } else {
            "context usage exceeded automatic compaction threshold before model call"
        };
        let _ = self.compact_session_context_for_runtime(session_id, mode, reason, recent_window);
        Ok(())
    }

    fn compact_session_context_for_runtime(
        &self,
        session_id: &str,
        mode: &str,
        reason: &str,
        recent_window: usize,
    ) -> Result<Value, AdkChatPortError> {
        let session = self
            .store
            .get_session(session_id)
            .map_err(storage_unavailable)?
            .ok_or_else(|| unavailable("session not found"))?;
        context::compact_session_projection(
            &self.store,
            &self.session_store,
            session_id,
            &session,
            mode,
            "auto",
            reason,
            recent_window,
            false,
        )
        .map_err(|error| unavailable(format!("session context compaction failed: {error}")))
    }
}
