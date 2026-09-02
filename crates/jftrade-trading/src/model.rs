use jftrade_kernel::{Fixed8, WireTimestamp};
use serde::{Deserialize, Serialize};
use thiserror::Error;

#[derive(Clone, Copy, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum TradingEnvironment {
    Simulate,
    Real,
}

impl TradingEnvironment {
    pub const fn as_str(self) -> &'static str {
        match self {
            Self::Simulate => "SIMULATE",
            Self::Real => "REAL",
        }
    }
}

#[derive(Clone, Copy, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum OrderSide {
    Buy,
    Sell,
}

impl OrderSide {
    pub const fn as_str(self) -> &'static str {
        match self {
            Self::Buy => "BUY",
            Self::Sell => "SELL",
        }
    }
}

#[derive(Clone, Copy, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum OrderStatus {
    Created,
    PrecheckRejected,
    Submitting,
    SubmissionUnknown,
    Submitted,
    BrokerAccepted,
    PartiallyFilled,
    Filled,
    CancelRequested,
    Cancelled,
    Rejected,
    Expired,
    Unknown,
}

impl OrderStatus {
    pub const fn is_terminal(self) -> bool {
        matches!(
            self,
            Self::PrecheckRejected
                | Self::Filled
                | Self::Cancelled
                | Self::Rejected
                | Self::Expired
        )
    }
}

pub fn canonical_broker_status(raw: &str) -> OrderStatus {
    let normalized = raw.trim().to_ascii_uppercase().replace(['-', ' '], "_");
    let normalized = normalized
        .strip_prefix("ORDER_STATUS_")
        .unwrap_or(&normalized);
    match normalized {
        "CREATED" => OrderStatus::Created,
        "PRECHECK_REJECTED" => OrderStatus::PrecheckRejected,
        "UNSUBMITTED" | "WAITING_SUBMIT" | "SUBMITTING" => OrderStatus::Submitting,
        "SUBMITTED" | "NEW" | "ACCEPTED" | "BROKER_ACCEPTED" => OrderStatus::BrokerAccepted,
        "FILLED_PART" | "PARTIAL_FILLED" | "PARTIALLY_FILLED" => OrderStatus::PartiallyFilled,
        "FILLED_ALL" | "FILLED" => OrderStatus::Filled,
        "CANCELLING_PART" | "CANCELLING_ALL" | "CANCELING" | "CANCEL_REQUESTED"
        | "PENDING_CANCEL" => OrderStatus::CancelRequested,
        "CANCELLED_PART" | "CANCELLED_ALL" | "CANCELLED" | "CANCELED_PART" | "CANCELED_ALL"
        | "CANCELED" | "DELETED" => OrderStatus::Cancelled,
        "SUBMIT_FAILED" | "SUBMITFAILED" | "FAILED" | "REJECTED" | "DISABLED"
        | "FILL_CANCELLED" | "FILLCANCELLED" => OrderStatus::Rejected,
        "EXPIRED" => OrderStatus::Expired,
        _ => OrderStatus::Unknown,
    }
}

pub fn canonical_stored_status(raw: &str) -> OrderStatus {
    let normalized = raw.trim().to_ascii_uppercase().replace(['-', ' '], "_");
    match normalized.as_str() {
        "CREATED" => OrderStatus::Created,
        "PRECHECK_REJECTED" => OrderStatus::PrecheckRejected,
        "SUBMITTING" => OrderStatus::Submitting,
        "SUBMISSION_UNKNOWN" => OrderStatus::SubmissionUnknown,
        "SUBMITTED" => OrderStatus::Submitted,
        "BROKER_ACCEPTED" => OrderStatus::BrokerAccepted,
        "PARTIALLY_FILLED" => OrderStatus::PartiallyFilled,
        "FILLED" => OrderStatus::Filled,
        "CANCEL_REQUESTED" => OrderStatus::CancelRequested,
        "CANCELLED" => OrderStatus::Cancelled,
        "REJECTED" => OrderStatus::Rejected,
        "EXPIRED" => OrderStatus::Expired,
        "UNKNOWN" => OrderStatus::Unknown,
        _ => canonical_broker_status(raw),
    }
}

pub fn reconcile_status(current: OrderStatus, incoming: OrderStatus) -> (OrderStatus, bool) {
    if current == incoming {
        return (current, true);
    }
    if incoming == OrderStatus::Unknown {
        return (current, false);
    }
    if current == OrderStatus::Unknown {
        return (incoming, true);
    }
    if current.is_terminal() {
        return (current, false);
    }
    let accepted = match current {
        OrderStatus::Created => matches!(
            incoming,
            OrderStatus::Submitting
                | OrderStatus::Submitted
                | OrderStatus::BrokerAccepted
                | OrderStatus::PartiallyFilled
                | OrderStatus::Filled
                | OrderStatus::CancelRequested
                | OrderStatus::Cancelled
                | OrderStatus::Rejected
                | OrderStatus::Expired
        ),
        OrderStatus::Submitting | OrderStatus::SubmissionUnknown => matches!(
            incoming,
            OrderStatus::Submitted
                | OrderStatus::BrokerAccepted
                | OrderStatus::PartiallyFilled
                | OrderStatus::Filled
                | OrderStatus::CancelRequested
                | OrderStatus::Cancelled
                | OrderStatus::Rejected
                | OrderStatus::Expired
        ),
        OrderStatus::Submitted => matches!(
            incoming,
            OrderStatus::BrokerAccepted
                | OrderStatus::PartiallyFilled
                | OrderStatus::Filled
                | OrderStatus::CancelRequested
                | OrderStatus::Cancelled
                | OrderStatus::Rejected
                | OrderStatus::Expired
        ),
        OrderStatus::BrokerAccepted => matches!(
            incoming,
            OrderStatus::PartiallyFilled
                | OrderStatus::Filled
                | OrderStatus::CancelRequested
                | OrderStatus::Cancelled
                | OrderStatus::Rejected
                | OrderStatus::Expired
        ),
        OrderStatus::PartiallyFilled => matches!(
            incoming,
            OrderStatus::Filled
                | OrderStatus::CancelRequested
                | OrderStatus::Cancelled
                | OrderStatus::Rejected
                | OrderStatus::Expired
        ),
        OrderStatus::CancelRequested => matches!(
            incoming,
            OrderStatus::Filled
                | OrderStatus::Cancelled
                | OrderStatus::Rejected
                | OrderStatus::Expired
        ),
        OrderStatus::PrecheckRejected
        | OrderStatus::Filled
        | OrderStatus::Cancelled
        | OrderStatus::Rejected
        | OrderStatus::Expired
        | OrderStatus::Unknown => false,
    };
    if accepted {
        (incoming, true)
    } else {
        (current, false)
    }
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct OrderCommand {
    pub idempotency_key: String,
    pub trace_id: String,
    pub broker_id: String,
    pub account_id: String,
    pub environment: TradingEnvironment,
    pub market: String,
    pub symbol: String,
    pub side: OrderSide,
    pub quantity: Fixed8,
    pub price: Option<Fixed8>,
    pub client_order_id: String,
}

impl OrderCommand {
    pub fn validate(&self) -> Result<(), TradingError> {
        for (field, value) in [
            ("idempotencyKey", self.idempotency_key.as_str()),
            ("traceId", self.trace_id.as_str()),
            ("brokerId", self.broker_id.as_str()),
            ("accountId", self.account_id.as_str()),
            ("market", self.market.as_str()),
            ("symbol", self.symbol.as_str()),
            ("clientOrderId", self.client_order_id.as_str()),
        ] {
            if value.trim().is_empty() {
                return Err(TradingError::MissingField(field));
            }
        }
        if self.quantity.signum() <= 0 {
            return Err(TradingError::InvalidQuantity);
        }
        if self.price.is_some_and(|price| price.signum() <= 0) {
            return Err(TradingError::InvalidPrice);
        }
        Ok(())
    }

    pub fn request_fingerprint(&self) -> String {
        format!(
            "{}|{}|{}|{}|{}|{}|{}|{}|{}",
            self.broker_id.trim().to_ascii_lowercase(),
            self.account_id.trim(),
            self.environment.as_str(),
            self.market.trim().to_ascii_uppercase(),
            self.symbol.trim().to_ascii_uppercase(),
            self.side.as_str(),
            self.quantity,
            self.price
                .map_or_else(|| "market".to_owned(), |value| value.to_string()),
            self.client_order_id.trim()
        )
    }
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct BrokerOrderEvent {
    pub event_id: String,
    pub trace_id: String,
    pub broker_order_id: String,
    pub sequence: u64,
    pub raw_status: String,
    pub fill_id: Option<String>,
    pub fill_quantity: Option<Fixed8>,
    pub occurred_at: WireTimestamp,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct AuditEntry {
    pub sequence: u64,
    pub trace_id: String,
    pub action: String,
    pub outcome: String,
    pub detail: String,
    pub at: WireTimestamp,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ShadowCommandPlan {
    pub accepted: bool,
    pub replayed: bool,
    pub dispatch: bool,
    pub idempotency_key: String,
    pub trace_id: String,
    pub normalized_request: String,
    pub reason_code: Option<String>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OrderProjection {
    pub broker_order_id: String,
    pub status: OrderStatus,
    pub filled_quantity: Fixed8,
    pub last_sequence: u64,
    pub accepted_events: usize,
    pub duplicate_events: usize,
    pub stale_events: usize,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum EventOutcome {
    Applied,
    Duplicate,
    Stale,
}

#[derive(Debug, Error, Eq, PartialEq)]
pub enum TradingError {
    #[error("{0} is required")]
    MissingField(&'static str),
    #[error("order quantity must be positive")]
    InvalidQuantity,
    #[error("order price must be positive when provided")]
    InvalidPrice,
    #[error("idempotency key was reused with a different request")]
    IdempotencyConflict,
    #[error("order event is invalid: {0}")]
    InvalidEvent(&'static str),
    #[error("fixed-point arithmetic failed")]
    Arithmetic,
    #[error("checkpoint is inconsistent: {0}")]
    InvalidCheckpoint(&'static str),
    #[error("account portfolio is invalid: {0}")]
    InvalidPortfolio(&'static str),
}

#[cfg(test)]
mod tests {
    use super::{OrderStatus, canonical_broker_status, reconcile_status};

    #[test]
    fn canonical_status_and_transition_match_go_rules() {
        assert_eq!(
            canonical_broker_status("Filled_Part"),
            OrderStatus::PartiallyFilled
        );
        assert_eq!(canonical_broker_status("Deleted"), OrderStatus::Cancelled);
        assert_eq!(canonical_broker_status("timeout"), OrderStatus::Unknown);
        assert_eq!(
            reconcile_status(OrderStatus::CancelRequested, OrderStatus::Filled),
            (OrderStatus::Filled, true)
        );
        assert_eq!(
            reconcile_status(OrderStatus::PartiallyFilled, OrderStatus::BrokerAccepted),
            (OrderStatus::PartiallyFilled, false)
        );
    }
}
