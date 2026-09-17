//! Shared OpenD quote-rights owner for the product trade runtime.
//!
//! The Go adapter kept connect status, the entitlement snapshot, its
//! generation and the retryable failure cache in one place
//! (`pkg/futu/adapter_capabilities.go`). The runtime exposes that value here so
//! capability projection never has to re-derive entitlements from socket
//! state, and so generation fencing has exactly one writer.

use std::sync::{Arc, RwLock};
use std::time::SystemTime;

use jftrade_integration_futu::{QuoteRightSnapshot, QuoteRightsState};

/// Generation-fenced quote-rights state plus the active OpenD session id.
#[derive(Clone, Default)]
pub(crate) struct QuoteRightsOwner {
    state: Arc<RwLock<QuoteRightsState>>,
    generation: Arc<RwLock<u64>>,
}

impl std::fmt::Debug for QuoteRightsOwner {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("QuoteRightsOwner")
            .field("generation", &self.generation())
            .finish_non_exhaustive()
    }
}

impl QuoteRightsOwner {
    /// Active OpenD session generation. Zero means no authenticated session,
    /// so every entitlement lookup must stay unverified.
    #[allow(dead_code)]
    pub(crate) fn generation(&self) -> u64 {
        *self
            .generation
            .read()
            .unwrap_or_else(|error| error.into_inner())
    }

    #[allow(dead_code)]
    pub(crate) fn set_generation(&self, generation: u64) {
        *self
            .generation
            .write()
            .unwrap_or_else(|error| error.into_inner()) = generation;
    }

    /// Right value for one field, fenced to the active OpenD generation.
    #[allow(dead_code)]
    pub(crate) fn right_value(
        &self,
        field: jftrade_integration_futu::QuoteRightField,
    ) -> Option<i32> {
        self.state().right_value(self.generation(), field)
    }

    #[allow(dead_code)]
    pub(crate) fn state(&self) -> QuoteRightsState {
        self.state
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
    }

    #[allow(dead_code)]
    pub(crate) fn store_connect_status(
        &self,
        generation: u64,
        quote_logged_in: bool,
        trade_logged_in: bool,
        observed_at: SystemTime,
    ) {
        self.state
            .write()
            .unwrap_or_else(|error| error.into_inner())
            .store_connect_status(generation, quote_logged_in, trade_logged_in, observed_at);
    }

    #[allow(dead_code)]
    pub(crate) fn store_snapshot(
        &self,
        generation: u64,
        rights: QuoteRightSnapshot,
        observed_at: SystemTime,
    ) -> bool {
        self.state
            .write()
            .unwrap_or_else(|error| error.into_inner())
            .store_quote_right(generation, rights, observed_at)
    }
}
