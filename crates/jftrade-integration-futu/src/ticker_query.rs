//! Fenced OpenD BasicQot ticker read used by live tick-candle reads.
//!
//! Parity: `internal/integration/futu/marketdata_runtime.go::QueryTicker` and
//! the `period=tick` branch of `internal/marketdata/service.go::GetCandles`.
//!
//! Go's tick candles are a cache-first read: when no fresh sample is cached the
//! service queries the provider ticker once, ingests the result, and still
//! answers from cache when that query fails. Rust keeps the provable parts of
//! that contract here: the query is fenced to the lifecycle generation, the
//! instrument must already hold a BASIC subscription, and the returned tick
//! carries the caller-supplied observation clock plus the active generation.

use std::sync::{Arc, Mutex};

use jftrade_marketdata::Tick;
use thiserror::Error;

use crate::{
    BasicQuoteQueryError, OpenDBasicQuoteExecutor, OpenDSessionCoordinator,
    OpenDSessionCoordinatorError, QuoteSessionResolver,
};

/// Reads one provider ticker sample for an instrument that already owns a
/// logical BASIC subscription.
///
/// The composition root owns the OpenD session; this port only borrows it for
/// the duration of one fenced BasicQot read and never mutates cache, demand or
/// provider activation.
pub trait TickerQuoteReadPort: Send + Sync + std::fmt::Debug {
    /// Returns the provider ticker sample for one instrument.
    ///
    /// Go's `QueryTicker` returns `(*Tick, error)`, so a provider that answers
    /// the call without a usable sample yields `(nil, nil)`. That state is
    /// `Ok(None)` here and must stay distinguishable from a provider error:
    /// the candle reader treats it as "nothing was ingested" and still answers
    /// from the retained window.
    fn query_ticker(
        &self,
        instrument_id: &str,
        observed_at_ms: i64,
    ) -> Result<Option<Tick>, TickerQuoteError>;
}

#[derive(Clone)]
pub struct OpenDTickerQuoteReader {
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
    session_resolver: Option<Arc<dyn QuoteSessionResolver>>,
}

impl std::fmt::Debug for OpenDTickerQuoteReader {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("OpenDTickerQuoteReader")
            .field("has_session_resolver", &self.session_resolver.is_some())
            .finish_non_exhaustive()
    }
}

impl OpenDTickerQuoteReader {
    pub fn new(coordinator: Arc<Mutex<OpenDSessionCoordinator>>) -> Self {
        Self {
            coordinator,
            session_resolver: None,
        }
    }

    pub fn with_session_resolver(
        mut self,
        resolver: Option<Arc<dyn QuoteSessionResolver>>,
    ) -> Self {
        self.session_resolver = resolver;
        self
    }
}

impl TickerQuoteReadPort for OpenDTickerQuoteReader {
    fn query_ticker(
        &self,
        instrument_id: &str,
        observed_at_ms: i64,
    ) -> Result<Option<Tick>, TickerQuoteError> {
        let coordinator = self
            .coordinator
            .lock()
            .map_err(|_| TickerQuoteError::Lock)?;
        let session = coordinator.session_clone()?;
        let lifecycle = coordinator.lifecycle();
        let mut ticks = OpenDBasicQuoteExecutor::new(session)
            .with_session_resolver(self.session_resolver.clone())
            .query_ticks(lifecycle, &[instrument_id.to_owned()], observed_at_ms)?;
        Ok(ticks.pop())
    }
}

#[derive(Debug, Error)]
pub enum TickerQuoteError {
    #[error(transparent)]
    Query(#[from] BasicQuoteQueryError),
    #[error(transparent)]
    Session(#[from] OpenDSessionCoordinatorError),
    #[error("OpenD ticker read failed: coordinator lock is poisoned")]
    Lock,
}
