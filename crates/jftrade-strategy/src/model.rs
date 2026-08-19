use jftrade_kernel::{Fixed8, WireTimestamp};
use serde::{Deserialize, Serialize};
use thiserror::Error;

#[derive(Clone, Copy, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum ExecutionMode {
    NotifyOnly,
    Paper,
    Live,
}

#[derive(Clone, Copy, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum RuntimeState {
    Stopped,
    Starting,
    Running,
    Paused,
    Recovering,
    Stopping,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct Signal {
    pub signal_id: String,
    pub trace_id: String,
    pub instance_id: String,
    pub broker_id: String,
    pub account_id: String,
    pub market: String,
    pub symbol: String,
    pub side: String,
    pub quantity: Fixed8,
    pub price: Option<Fixed8>,
    pub observed_at: WireTimestamp,
}

impl Signal {
    pub fn validate(&self) -> Result<(), StrategyError> {
        for (field, value) in [
            ("signalId", self.signal_id.as_str()),
            ("traceId", self.trace_id.as_str()),
            ("instanceId", self.instance_id.as_str()),
            ("symbol", self.symbol.as_str()),
        ] {
            if value.trim().is_empty() {
                return Err(StrategyError::MissingField(field));
            }
        }
        if self.quantity.signum() <= 0 {
            return Err(StrategyError::InvalidQuantity);
        }
        let side = self.side.trim().to_ascii_uppercase();
        if side != "BUY" && side != "SELL" {
            return Err(StrategyError::InvalidSide);
        }
        Ok(())
    }
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct TradeIntent {
    pub idempotency_key: String,
    pub trace_id: String,
    pub broker_id: String,
    pub account_id: String,
    pub live: bool,
    pub market: String,
    pub symbol: String,
    pub side: String,
    pub quantity: Fixed8,
    pub price: Option<Fixed8>,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct TradePlanReceipt {
    pub accepted: bool,
    pub dispatch: bool,
    pub reason_code: Option<String>,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct StrategyNotification {
    pub source_event_id: String,
    pub trace_id: String,
    pub category: String,
    pub message: String,
    pub dispatch: bool,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SignalOutcome {
    pub signal_id: String,
    pub duplicate: bool,
    pub mode: ExecutionMode,
    pub trade_plan: Option<TradePlanReceipt>,
    pub notification: Option<StrategyNotification>,
}

#[derive(Debug, Error, Eq, PartialEq)]
pub enum StrategyError {
    #[error("{0} is required")]
    MissingField(&'static str),
    #[error("signal quantity must be positive")]
    InvalidQuantity,
    #[error("signal side must be BUY or SELL")]
    InvalidSide,
    #[error("strategy runtime is not running")]
    RuntimeNotRunning,
    #[error("strategy trading port failed: {0}")]
    TradingPort(String),
}
