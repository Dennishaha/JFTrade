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
    /// A finite fixed-point calculation overflowed the signed representation.
    #[error("fixed8 arithmetic overflow")]
    Fixed8ArithmeticOverflow,
    /// Division by zero is never a valid fixed-point operation.
    #[error("fixed8 division by zero")]
    Fixed8DivisionByZero,
    /// Infinite sentinel values are wire-compatible but cannot enter accounting.
    #[error("non-finite fixed8 value cannot be used in arithmetic")]
    Fixed8NonFiniteArithmetic,
    /// Quantity and price normalization require a positive increment.
    #[error("fixed8 increment must be positive")]
    InvalidFixed8Increment,
    /// The timestamp is not valid RFC3339.
    #[error("invalid RFC3339 timestamp: {0}")]
    InvalidTimestamp(String),
    /// The timestamp cannot be represented as signed Unix milliseconds.
    #[error("timestamp is outside the Unix millisecond range")]
    TimestampOutOfRange,
}
