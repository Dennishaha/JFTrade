#![forbid(unsafe_code)]

//! Broker-neutral taxonomies and errors used at capability-defined port boundaries.

use std::collections::BTreeMap;
use std::error::Error as StdError;
use std::fmt;
use std::time::Duration;

use jftrade_kernel::Fixed8;
use serde::{Deserialize, Serialize};
use thiserror::Error;

macro_rules! string_taxonomy {
    ($name:ident, [$($known:literal),+ $(,)?]) => {
        #[derive(Clone, Debug, Deserialize, Eq, Hash, Ord, PartialEq, PartialOrd, Serialize)]
        #[serde(transparent)]
        pub struct $name(String);

        impl $name {
            pub fn new(value: impl Into<String>) -> Self {
                Self(value.into())
            }

            pub fn as_str(&self) -> &str {
                &self.0
            }

            pub fn is_known(&self) -> bool {
                matches!(self.0.as_str(), $($known)|+)
            }
        }

        impl fmt::Display for $name {
            fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
                formatter.write_str(&self.0)
            }
        }
    };
}

string_taxonomy!(
    ProductClass,
    [
        "equity",
        "fund",
        "option",
        "warrant",
        "cbbc",
        "future",
        "event_contract",
        "index",
        "bond",
        "plate",
        "unknown",
    ]
);
string_taxonomy!(MarketSegment, ["securities", "derivatives", "prediction"]);
string_taxonomy!(QuantityMode, ["units", "contracts", "amount"]);
string_taxonomy!(
    OrderKind,
    ["single", "option_combo", "event_single", "event_parlay"]
);

#[derive(Clone, Debug, Eq, Error, PartialEq)]
#[error("broker {broker_id}: [{code}] {message}")]
pub struct BrokerError {
    pub broker_id: String,
    pub code: String,
    pub message: String,
}

impl BrokerError {
    pub fn new(
        broker_id: impl Into<String>,
        code: impl Into<String>,
        message: impl Into<String>,
    ) -> Self {
        Self {
            broker_id: broker_id.into(),
            code: code.into(),
            message: message.into(),
        }
    }
}

/// The quantity fields a broker may overlay on an existing market
/// description.  The full market model is owned by each consumer; this
/// broker-neutral value deliberately contains only the fields touched by a
/// [`MarketRuleItem`].
#[derive(Clone, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Market {
    pub symbol: String,
    pub min_quantity: Fixed8,
    pub step_size: Fixed8,
}

/// Alias that makes the narrow purpose of [`Market`] explicit at call sites.
pub type MarketQuantityConstraints = Market;

/// Broker-provided quantity constraints for one instrument.
#[derive(Clone, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct MarketRuleItem {
    pub symbol: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub lot_size: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub min_quantity: Option<Fixed8>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub step_size: Option<Fixed8>,
}

const FIXED8_SCALE: i64 = 100_000_000;

/// Applies one broker rule to a market copy.
///
/// A positive lot size initializes both minimum and step quantity. Explicit
/// positive constraints then override those defaults, matching the Go
/// broker-neutral rule order. Non-positive and non-finite values are ignored.
pub fn apply_market_rule(mut market: Market, rule: &MarketRuleItem) -> Market {
    if let Some(lot_size) = rule.lot_size.filter(|value| *value > 0) {
        let lot = Fixed8::from_scaled(i64::from(lot_size) * FIXED8_SCALE);
        market.min_quantity = lot;
        market.step_size = lot;
    }
    if let Some(min_quantity) = rule.min_quantity.filter(|value| is_positive_finite(*value)) {
        market.min_quantity = min_quantity;
    }
    if let Some(step_size) = rule.step_size.filter(|value| is_positive_finite(*value)) {
        market.step_size = step_size;
    }
    market
}

/// Applies the first case-insensitive, trimmed symbol match to a market copy.
pub fn apply_market_rules(market: Market, rules: &[MarketRuleItem]) -> Market {
    let symbol = normalize_symbol(&market.symbol);
    if let Some(rule) = rules
        .iter()
        .find(|rule| normalize_symbol(&rule.symbol) == symbol)
    {
        apply_market_rule(market, rule)
    } else {
        market
    }
}

fn normalize_symbol(symbol: &str) -> String {
    symbol.trim().to_ascii_uppercase()
}

fn is_positive_finite(value: Fixed8) -> bool {
    value.signum() > 0 && value != Fixed8::POS_INFINITY
}

string_taxonomy!(
    SnapshotAvailabilityKind,
    ["entitlement", "unsupported", "subscription_quota"]
);

impl SnapshotAvailabilityKind {
    pub fn is_fallback_eligible(&self) -> bool {
        self.is_known()
    }
}

#[derive(Clone, Debug, Eq, Error, PartialEq)]
#[error("{message}")]
pub struct SnapshotAvailabilityError {
    pub kind: SnapshotAvailabilityKind,
    pub message: String,
}

#[derive(Clone, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct BrokerReadFeatureCapability {
    pub supported_environments: Vec<String>,
    #[serde(skip_serializing_if = "is_false")]
    pub supports_history: bool,
    #[serde(skip_serializing_if = "is_false")]
    pub requires_symbols: bool,
    #[serde(skip_serializing_if = "is_false")]
    pub requires_clearing_date: bool,
    #[serde(skip_serializing_if = "is_false")]
    pub requires_price: bool,
    #[serde(skip_serializing_if = "is_false")]
    pub requires_order_id_ex: bool,
    #[serde(skip_serializing_if = "is_zero_u16")]
    pub default_num: u16,
    #[serde(skip_serializing_if = "is_zero_u16")]
    pub min_num: u16,
    #[serde(skip_serializing_if = "is_zero_u16")]
    pub max_num: u16,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub num_presets: Vec<u16>,
    #[serde(skip_serializing_if = "is_false")]
    pub supports_real_time_push: bool,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct BrokerMarketCapability {
    pub market: String,
    pub supports_quote: bool,
    pub supports_trade: bool,
    pub read_features: BTreeMap<String, BrokerReadFeatureCapability>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct BrokerRuntimeDescriptor {
    pub id: String,
    pub display_name: String,
    pub environments: Vec<String>,
    pub capabilities: Vec<BrokerMarketCapability>,
    pub notes: Vec<String>,
}

const fn is_zero_u16(value: &u16) -> bool {
    *value == 0
}

const fn is_false(value: &bool) -> bool {
    !*value
}

impl SnapshotAvailabilityError {
    pub fn new(kind: SnapshotAvailabilityKind, message: impl Into<String>) -> Self {
        Self {
            kind,
            message: message.into(),
        }
    }

    pub fn is_fallback_eligible(&self) -> bool {
        self.kind.is_fallback_eligible()
    }
}

/// A marker for snapshot failures that can be isolated by retrying a smaller
/// symbol batch.  Transport, cancellation, timeout and request-budget errors
/// should not use this marker.
#[derive(Clone, Debug, Eq, Error, PartialEq)]
#[error("{message}")]
pub struct SymbolScopedSnapshotError {
    message: String,
}

impl SymbolScopedSnapshotError {
    pub fn new(message: impl Into<String>) -> Self {
        Self {
            message: message.into(),
        }
    }

    pub fn message(&self) -> &str {
        &self.message
    }
}

/// Returns whether a snapshot error or one of its typed sources is marked as
/// symbol-scoped.
pub fn is_symbol_scoped_snapshot_error(error: &(dyn StdError + 'static)) -> bool {
    find_error::<SymbolScopedSnapshotError>(error).is_some()
}

/// Snapshot request rate-limit metadata retained at the broker-neutral port.
#[derive(Clone, Debug, Eq, Error, PartialEq)]
#[error("{message}")]
pub struct SnapshotRateLimitError {
    retry_after: Duration,
    message: String,
}

impl SnapshotRateLimitError {
    /// Constructs a rate-limit error, normalizing an empty delay to one second.
    pub fn new(retry_after: Duration) -> Self {
        let retry_after = normalize_retry_after(retry_after);
        Self {
            retry_after,
            message: format!(
                "broker snapshot rate limited; retry after {}",
                format_retry_after(retry_after)
            ),
        }
    }

    /// Constructs a rate-limit error that preserves an upstream message.
    pub fn with_message(retry_after: Duration, message: impl Into<String>) -> Self {
        Self {
            retry_after: normalize_retry_after(retry_after),
            message: message.into(),
        }
    }

    pub fn retry_after(&self) -> Duration {
        self.retry_after
    }
}

/// Extracts retry metadata through a typed error source chain.
pub fn snapshot_retry_after(error: &(dyn StdError + 'static)) -> Option<Duration> {
    find_error::<SnapshotRateLimitError>(error).map(SnapshotRateLimitError::retry_after)
}

/// Returns whether a snapshot error or one of its typed sources is rate-limited.
pub fn is_snapshot_rate_limited(error: &(dyn StdError + 'static)) -> bool {
    snapshot_retry_after(error).is_some()
}

/// Extracts the broker-neutral availability kind through a typed source chain.
pub fn snapshot_availability(error: &(dyn StdError + 'static)) -> Option<SnapshotAvailabilityKind> {
    find_error::<SnapshotAvailabilityError>(error).map(|value| value.kind.clone())
}

/// Returns whether a typed snapshot availability error can use a delayed
/// broker snapshot fallback.
pub fn is_snapshot_fallback_eligible(error: &(dyn StdError + 'static)) -> bool {
    snapshot_availability(error).is_some_and(|kind| kind.is_fallback_eligible())
}

fn normalize_retry_after(retry_after: Duration) -> Duration {
    if retry_after.is_zero() {
        Duration::from_secs(1)
    } else {
        retry_after
    }
}

fn format_retry_after(retry_after: Duration) -> String {
    let millis = retry_after.as_millis();
    let seconds = millis / 1_000;
    let remainder = millis % 1_000;
    if remainder == 0 {
        return format!("{seconds}s");
    }
    if seconds == 0 {
        return format!("{remainder}ms");
    }
    let fraction = format!("{remainder:03}").trim_end_matches('0').to_owned();
    format!("{seconds}.{fraction}s")
}

fn find_error<'a, T>(error: &'a (dyn StdError + 'static)) -> Option<&'a T>
where
    T: StdError + 'static,
{
    let mut current = Some(error);
    while let Some(candidate) = current {
        if let Some(found) = candidate.downcast_ref::<T>() {
            return Some(found);
        }
        current = candidate.source();
    }
    None
}
