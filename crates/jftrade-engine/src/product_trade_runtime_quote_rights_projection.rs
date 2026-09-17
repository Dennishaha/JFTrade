//! Composition seams for the OpenD entitlement owner.
//!
//! Provider activation installs one acquisition owner and asks for the first
//! `GetUserInfo` refresh; the capability projection then reads that fenced
//! state. Keeping the two calls here leaves `product_trade_runtime_projection`
//! inside the bounded production-file budget.

use std::sync::{Arc, Mutex};
use std::time::SystemTime;

use jftrade_integration_futu::{
    OpenDQuoteRightsOwner, OpenDSessionCoordinator, QuoteRightSnapshot, QuoteRightsError,
    query_quote_rights,
};

use super::SharedTradeReadRuntime;

impl SharedTradeReadRuntime {
    /// Installs the OpenD entitlement owner created at provider activation and
    /// publishes the generation the authenticated session started on. The
    /// owner is the only writer of connect status and quote rights.
    pub(crate) fn install_quote_rights_owner(
        &self,
        owner: Arc<OpenDQuoteRightsOwner>,
        generation: u64,
    ) {
        self.quote_rights.set_acquisition(owner, generation);
    }

    /// Refreshes quote entitlements through OpenD and stores them fenced to the
    /// active generation. Failures stay cached and retryable; they never panic
    /// the activation path.
    pub(crate) fn refresh_quote_rights(
        &self,
        coordinator: &Arc<Mutex<OpenDSessionCoordinator>>,
    ) -> Result<QuoteRightSnapshot, QuoteRightsError> {
        let owner = self
            .quote_rights
            .acquisition()
            .ok_or(QuoteRightsError::NotConfigured)?;
        let generation = owner.generation();
        self.quote_rights.refresh(SystemTime::now(), || {
            let mut guard = coordinator
                .lock()
                .unwrap_or_else(|error| error.into_inner());
            query_quote_rights(&mut guard, generation).map_err(|error| error.to_string())
        })
    }
}
