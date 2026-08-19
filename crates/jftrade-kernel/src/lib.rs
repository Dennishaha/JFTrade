#![forbid(unsafe_code)]

//! Stable value codecs shared by migration capabilities.
//!
//! These types preserve existing Go wire and SQLite semantics. They do not own
//! transport, persistence, or business service behavior.

mod decimal;
mod fixed8;
mod timestamp;

pub use decimal::DecimalText;
pub use fixed8::Fixed8;
pub use timestamp::WireTimestamp;

use thiserror::Error;

/// Failure returned when a boundary value cannot be decoded without loss.
#[derive(Clone, Debug, Eq, Error, PartialEq)]
pub enum CodecError {
    /// The value is not a supported decimal representation.
    #[error("invalid decimal value: {0}")]
    InvalidDecimal(String),
    /// Expanding the value would exceed the defensive codec limit.
    #[error("decimal value exceeds the supported expansion limit")]
    DecimalExpansionLimit,
    /// The value cannot be represented by the signed eight-decimal fixed type.
    #[error("fixed8 value is out of range")]
    Fixed8OutOfRange,
    /// The timestamp is not valid RFC3339.
    #[error("invalid RFC3339 timestamp: {0}")]
    InvalidTimestamp(String),
    /// The timestamp cannot be represented as signed Unix milliseconds.
    #[error("timestamp is outside the Unix millisecond range")]
    TimestampOutOfRange,
}
