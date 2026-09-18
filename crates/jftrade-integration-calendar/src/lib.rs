#![forbid(unsafe_code)]

//! HTTP calendar providers for the exchange-calendar capability.
//!
//! Go's `internal/exchangecalendar` ships four official providers
//! (`nyse_official`, `nasdaq_verifier`, `hk_gov_1823_ical`,
//! `mainland_official_notice`). The domain crate owns the ports and the
//! builtin fallback rules; this adapter owns the transport and the
//! provider-specific document parsing, and it is injected by the engine's
//! composition root.

mod http_source;
mod parser;
mod parser_helpers;

pub use http_source::{
    CalendarHttpClient, DEFAULT_HTTP_TIMEOUT, HttpCalendarSource, ReqwestCalendarClient,
    default_registry, default_sources,
};
pub use parser::{
    ParseFn, ValidateFn, default_holiday_override_parser, hong_kong_holiday_ical_parser,
    minimum_anchor_year_schedules_validator, nyse_holiday_schedule_parser,
    sse_trading_schedule_parser,
};
