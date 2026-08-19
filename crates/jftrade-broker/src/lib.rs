#![forbid(unsafe_code)]

//! Broker-neutral taxonomies and errors used at capability-defined port boundaries.

use std::fmt;

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
