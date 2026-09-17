//! Shared OpenD quote-rights owner for the product trade runtime.
//!
//! The Go adapter kept connect status, the entitlement snapshot, its
//! generation and the retryable failure cache in one place
//! (`pkg/futu/adapter_capabilities.go`). Acquisition and fencing live in
//! `jftrade-integration-futu::OpenDQuoteRightsOwner`; the runtime holds the
//! handle created at provider activation, so entitlements are never re-derived
//! from socket state and there is exactly one writer.

use std::sync::{Arc, RwLock};
use std::time::SystemTime;

use jftrade_integration_futu::{
    OpenDQuoteRightsOwner, QuoteRightField, QuoteRightSnapshot, QuoteRightsError, QuoteRightsState,
};

/// Runtime-facing handle around the acquisition owner created at activation.
///
/// Before provider activation no owner exists, and every entitlement lookup
/// reports unverified rather than inventing a right.
#[derive(Clone, Default)]
pub(crate) struct QuoteRightsOwner {
    generation_when_uninstalled: Arc<RwLock<u64>>,
    owner: Arc<RwLock<Option<Arc<OpenDQuoteRightsOwner>>>>,
}

impl std::fmt::Debug for QuoteRightsOwner {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("QuoteRightsOwner")
            .field("generation", &self.generation())
            .field("installed", &self.installed().is_some())
            .finish_non_exhaustive()
    }
}

impl QuoteRightsOwner {
    fn installed(&self) -> Option<Arc<OpenDQuoteRightsOwner>> {
        self.owner
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
    }

    /// Active OpenD session generation. Zero means no authenticated session,
    /// so every entitlement lookup must stay unverified.
    pub(crate) fn generation(&self) -> u64 {
        match self.installed() {
            Some(owner) => owner.generation(),
            None => *self
                .generation_when_uninstalled
                .read()
                .unwrap_or_else(|error| error.into_inner()),
        }
    }

    /// Installs the acquisition owner created at activation and publishes the
    /// generation the authenticated session started on.
    pub(crate) fn set_acquisition(&self, owner: Arc<OpenDQuoteRightsOwner>, generation: u64) {
        owner.set_generation(generation);
        *self
            .generation_when_uninstalled
            .write()
            .unwrap_or_else(|error| error.into_inner()) = generation;
        *self
            .owner
            .write()
            .unwrap_or_else(|error| error.into_inner()) = Some(owner);
    }

    pub(crate) fn right_value(&self, field: QuoteRightField) -> Option<i32> {
        self.installed()?.right_value(field)
    }

    /// Stores one entitlement snapshot fenced to `generation`. Notification
    /// ingestion and tests use this path; production acquisition goes through
    /// [`Self::refresh`].
    #[cfg_attr(not(test), allow(dead_code))]
    pub(crate) fn store_snapshot(
        &self,
        generation: u64,
        rights: QuoteRightSnapshot,
        observed_at: SystemTime,
    ) -> bool {
        self.installed()
            .is_some_and(|owner| owner.store_quote_right(generation, rights, observed_at))
    }

    /// Snapshot of the fenced entitlement state.
    #[allow(dead_code)]
    pub(crate) fn state(&self) -> QuoteRightsState {
        self.installed()
            .map(|owner| owner.state())
            .unwrap_or_default()
    }

    pub(crate) fn acquisition(&self) -> Option<Arc<OpenDQuoteRightsOwner>> {
        self.installed()
    }

    /// Go `EvaluateCapability` login gate: returns the fenced connect-status
    /// snapshot when the required session (`quote` for reads, `trade` for
    /// everything else) is not logged in.
    pub(crate) fn connect_status(&self, read_access: bool) -> Option<SystemTime> {
        let owner = self.installed()?;
        let status = owner.connect_status_for_generation(owner.generation())?;
        let logged_in = if read_access {
            status.quote_logged_in
        } else {
            status.trade_logged_in
        };
        (!logged_in).then_some(status.observed_at)
    }

    /// Runs one fenced entitlement refresh through the installed owner. The
    /// caller supplies the OpenD query so this module stays free of the session
    /// coordinator; without an installed owner the refresh fails closed.
    pub(crate) fn refresh(
        &self,
        now: SystemTime,
        query: impl FnOnce() -> Result<QuoteRightSnapshot, String>,
    ) -> Result<QuoteRightSnapshot, QuoteRightsError> {
        let owner = self.installed().ok_or(QuoteRightsError::NotConfigured)?;
        owner.ensure(now, query)
    }
}
