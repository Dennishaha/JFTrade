//! OpenD quote-rights acquisition and state owner.
//!
//! Go `pkg/futu/adapter_capabilities.go` keeps connect status, the entitlement
//! snapshot, its connection generation and the retryable failure cache in one
//! place, and refreshes them through one `GetUserInfo` query per connection.
//! Rust previously fenced that state (`quote_rights.rs`) but had no production
//! writer: entitlement pushes and the `GetUserInfo` query never reached it, so
//! every capability read stayed `QUOTE_RIGHT_UNVERIFIED`.
//!
//! This type is the single owner and writer. `quote_rights.rs` holds the pure
//! state machine; this module owns the wire read, the per-session gate that
//! makes the read happen once per connection, and the generation fencing that
//! keeps a late response from being attributed to a replacement session.

use std::sync::{Arc, Mutex, RwLock};
use std::time::SystemTime;

use prost::Message;
use thiserror::Error;

use crate::trade_proto::get_user_info::{C2s, Request, Response};
use crate::trade_proto::notify::{NotifyType, Response as NotifyResponse};
use crate::{
    ConnectStatusSnapshot, OpenDSessionCoordinator, OpenDSessionCoordinatorError,
    PROTO_GET_USER_INFO, QuoteRightField, QuoteRightSnapshot, QuoteRightsError,
    QuoteRightsFetchOutcome, QuoteRightsState,
};

#[derive(Debug, Error)]
pub enum OpenDQuoteRightsError {
    #[error("OpenD session coordinator failed: {0}")]
    Coordinator(#[from] OpenDSessionCoordinatorError),
    #[error("OpenD GetUserInfo request failed: {0}")]
    Session(#[from] crate::OpenDManagedSessionError),
    #[error("decode OpenD GetUserInfo response: {0}")]
    Decode(#[from] prost::DecodeError),
    #[error("OpenD GetUserInfo failed: {0}")]
    Rejected(String),
}

/// Reads one `GetUserInfo` entitlement snapshot fenced to `generation`.
///
/// The generation is re-checked after the RPC so a response that arrives once
/// the session has already been replaced is rejected instead of being stored
/// against the new connection.
pub fn query_quote_rights(
    coordinator: &mut OpenDSessionCoordinator,
    generation: u64,
) -> Result<QuoteRightSnapshot, OpenDQuoteRightsError> {
    if generation == 0 || coordinator.generation() != generation {
        return Err(OpenDQuoteRightsError::Coordinator(
            OpenDSessionCoordinatorError::Closed,
        ));
    }
    let body = Request {
        c2s: C2s { flag: None },
    }
    .encode_to_vec();
    let response = coordinator
        .session()?
        .managed_session()
        .call(PROTO_GET_USER_INFO, &body)?;
    if coordinator.generation() != generation {
        return Err(OpenDQuoteRightsError::Coordinator(
            OpenDSessionCoordinatorError::Closed,
        ));
    }
    let response = Response::decode(response.as_slice())?;
    if response.ret_type != 0 {
        return Err(OpenDQuoteRightsError::Rejected(
            response
                .ret_msg
                .unwrap_or_else(|| "OpenD GetUserInfo request failed".to_owned()),
        ));
    }
    let Some(s2c) = response.s2c else {
        return Err(OpenDQuoteRightsError::Rejected(
            "OpenD GetUserInfo returned no quote entitlement fields".to_owned(),
        ));
    };
    QuoteRightSnapshot::from_user_info(&s2c).ok_or_else(|| {
        OpenDQuoteRightsError::Rejected(
            "OpenD GetUserInfo returned no quote entitlement fields".to_owned(),
        )
    })
}

/// Decodes one OpenD system notification (`Notify.proto`) for the capability
/// owner. Go registers the same handler on protocol 1003 and ignores anything
/// that is not a connect-status or quote-right push.
pub fn decode_capability_notification(frame: &crate::Frame) -> Option<NotifyResponse> {
    if frame.header.proto_id != crate::PROTO_NOTIFY {
        return None;
    }
    let response = NotifyResponse::decode(frame.body.as_slice()).ok()?;
    if response.ret_type != 0 {
        return None;
    }
    response.s2c.as_ref()?;
    Some(response)
}

/// Single owner of OpenD connect status, entitlement snapshot, generation and
/// retryable failure cache.
#[derive(Clone, Default)]
pub struct OpenDQuoteRightsOwner {
    state: Arc<RwLock<QuoteRightsState>>,
    generation: Arc<RwLock<u64>>,
    /// Serialises refreshes so concurrent capability reads share one
    /// `GetUserInfo` call per connection. Notifications do not take this gate,
    /// so a push can still resolve the entitlement while a query is in flight.
    refresh_gate: Arc<Mutex<()>>,
}

impl std::fmt::Debug for OpenDQuoteRightsOwner {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("OpenDQuoteRightsOwner")
            .field("generation", &self.generation())
            .field(
                "revision",
                &self.state.read().map(|s| s.revision()).unwrap_or(0),
            )
            .finish_non_exhaustive()
    }
}

impl OpenDQuoteRightsOwner {
    pub fn new() -> Self {
        Self::default()
    }

    /// Active OpenD session generation; zero means no authenticated session and
    /// therefore no entitlement may be reported as verified.
    pub fn generation(&self) -> u64 {
        *self
            .generation
            .read()
            .unwrap_or_else(|error| error.into_inner())
    }

    pub fn set_generation(&self, generation: u64) {
        *self
            .generation
            .write()
            .unwrap_or_else(|error| error.into_inner()) = generation;
    }

    fn with_state<T>(&self, action: impl FnOnce(&mut QuoteRightsState) -> T) -> T {
        let mut state = self
            .state
            .write()
            .unwrap_or_else(|error| error.into_inner());
        action(&mut state)
    }

    /// Go `captureCapabilityNotificationAt` for a `ConnStatus` push.
    pub fn store_connect_status(
        &self,
        generation: u64,
        quote_logged_in: bool,
        trade_logged_in: bool,
        observed_at: SystemTime,
    ) {
        self.with_state(|state| {
            state.store_connect_status(generation, quote_logged_in, trade_logged_in, observed_at);
        });
    }

    /// Go `captureCapabilityNotificationAt` for a `QotRight` push. Returns
    /// `false` when a newer generation already owns the entitlement.
    pub fn store_quote_right(
        &self,
        generation: u64,
        rights: QuoteRightSnapshot,
        observed_at: SystemTime,
    ) -> bool {
        self.with_state(|state| state.store_quote_right(generation, rights, observed_at))
    }

    /// Ingests one decoded `Notify.proto` push, fenced to `generation`.
    /// Unknown notification types leave the state untouched, and a `QotRight`
    /// push clones the wire value so later mutation cannot change the owner.
    pub fn ingest_notification(
        &self,
        generation: u64,
        response: &NotifyResponse,
        observed_at: SystemTime,
    ) {
        let Some(s2c) = response.s2c.as_ref() else {
            return;
        };
        if s2c.r#type == NotifyType::ConnStatus as i32 {
            let Some(status) = s2c.connect_status.as_ref() else {
                return;
            };
            self.store_connect_status(
                generation,
                status.qot_logined,
                status.trd_logined,
                observed_at,
            );
        } else if s2c.r#type == NotifyType::QotRight as i32 {
            let Some(rights) = s2c.qot_right.as_ref() else {
                return;
            };
            self.store_quote_right(
                generation,
                QuoteRightSnapshot::from_notification(rights),
                observed_at,
            );
        }
    }

    pub fn connect_status_for_generation(&self, generation: u64) -> Option<ConnectStatusSnapshot> {
        self.state
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .connect_status_for_generation(generation)
            .cloned()
    }

    /// Right value for one product field, fenced to the active generation.
    /// `None` means unverified and must never be treated as allowed.
    pub fn right_value(&self, field: QuoteRightField) -> Option<i32> {
        self.state
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .right_value(self.generation(), field)
    }

    /// Snapshot of the fenced state; used by projections that need more than
    /// one field for the same generation.
    pub fn state(&self) -> QuoteRightsState {
        self.state
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
    }

    /// Verified entitlement for the active generation, if the wire has
    /// reported one. Used by the capability projection.
    pub fn verified_rights(&self) -> Option<(QuoteRightSnapshot, SystemTime)> {
        self.state
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .verified_rights(self.generation())
            .map(|(value, observed_at)| (value.clone(), observed_at))
    }

    /// Go `ensureQuoteRights`. A verified snapshot for the active generation is
    /// returned without touching the wire; a cached failure inside the retry
    /// interval short-circuits with the same reason; otherwise exactly one
    /// caller runs `query` and the result is stored generation-fenced.
    pub fn ensure(
        &self,
        now: SystemTime,
        query: impl FnOnce() -> Result<QuoteRightSnapshot, String>,
    ) -> Result<QuoteRightSnapshot, QuoteRightsError> {
        let generation = self.generation();
        if generation == 0 {
            return Err(QuoteRightsError::NotConfigured);
        }
        let _gate = self
            .refresh_gate
            .lock()
            .unwrap_or_else(|error| error.into_inner());
        // Re-check inside the gate: the caller that won the race has already
        // stored its snapshot, so later callers must not query again.
        if let Some((snapshot, _)) = self.verified_rights() {
            return Ok(snapshot);
        }
        if let Some(reason) =
            self.with_state(|state| state.cached_failure_due(generation, now).map(str::to_owned))
        {
            return Err(QuoteRightsError::Failed(reason));
        }
        let start_revision = self.with_state(|state| state.revision());
        match query() {
            Ok(snapshot) => {
                let accepted = self.with_state(|state| {
                    state.store_query_result(generation, start_revision, snapshot.clone(), now)
                });
                if accepted {
                    return Ok(snapshot);
                }
                // A same-generation notification already resolved the
                // entitlement; Go keeps the notification and drops the query.
                self.verified_rights()
                    .map(|(value, _)| value)
                    .ok_or(QuoteRightsError::RefreshRequired)
            }
            Err(reason) => {
                let outcome = self.with_state(|state| {
                    state.handle_fetch_failure(generation, generation, reason, now)
                });
                match outcome {
                    QuoteRightsFetchOutcome::ResolvedByNotification => self
                        .verified_rights()
                        .map(|(value, _)| value)
                        .ok_or(QuoteRightsError::RefreshRequired),
                    QuoteRightsFetchOutcome::RefreshRequired => {
                        Err(QuoteRightsError::RefreshRequired)
                    }
                    QuoteRightsFetchOutcome::Failed(reason) => {
                        Err(QuoteRightsError::Failed(reason))
                    }
                }
            }
        }
    }

    /// Refreshes directly through OpenD, fenced to the active generation.
    pub fn refresh(
        &self,
        coordinator: &mut OpenDSessionCoordinator,
        now: SystemTime,
    ) -> Result<QuoteRightSnapshot, QuoteRightsError> {
        let generation = self.generation();
        self.ensure(now, || {
            query_quote_rights(coordinator, generation).map_err(|error| error.to_string())
        })
    }
}

#[cfg(test)]
#[path = "open_d_quote_rights_tests.rs"]
mod tests;
