//! Protocol-neutral seam for exchange-calendar-aware quote projection.

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct QuoteSessionWindow {
    pub kind: String,
    pub start_minute: i32,
    pub end_minute: i32,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct QuoteSessionContext {
    pub session: String,
    pub trading_date: String,
    pub timezone: String,
    pub sessions: Vec<QuoteSessionWindow>,
}

/// Composition-owned calendar resolver. `None` means no resolver is
/// installed; production binds one before exposing the API and explicit
/// embedders may retain the historical fallback.
pub trait QuoteSessionResolver: Send + Sync + std::fmt::Debug {
    fn resolve_quote_session(
        &self,
        instrument_id: &str,
        observed_at_ms: i64,
    ) -> Option<QuoteSessionContext>;
}
