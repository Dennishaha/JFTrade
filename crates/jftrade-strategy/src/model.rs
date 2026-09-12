use jftrade_kernel::{Decimal, WireTimestamp};
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

impl RuntimeState {
    pub const fn as_str(self) -> &'static str {
        match self {
            Self::Stopped => "stopped",
            Self::Starting => "starting",
            Self::Running => "running",
            Self::Paused => "paused",
            Self::Recovering => "recovering",
            Self::Stopping => "stopping",
        }
    }
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
    pub quantity: Decimal,
    pub price: Option<Decimal>,
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
        if self.quantity <= Decimal::ZERO {
            return Err(StrategyError::InvalidQuantity);
        }
        let side = self.side.trim().to_ascii_uppercase();
        if side != "BUY" && side != "SELL" {
            return Err(StrategyError::InvalidSide);
        }
        Ok(())
    }
}

#[derive(Clone, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct BrokerAccountBinding {
    pub broker_id: String,
    pub account_id: String,
    pub trading_environment: String,
    pub market: String,
}

pub fn normalize_broker_account(
    input: Option<&BrokerAccountBinding>,
) -> Option<BrokerAccountBinding> {
    let input = input?;
    let broker_id = input.broker_id.trim().to_lowercase();
    let account_id = input.account_id.trim().to_string();
    let trading_environment = input.trading_environment.trim().to_uppercase();
    let market = input.market.trim().to_uppercase();

    if broker_id.is_empty()
        && account_id.is_empty()
        && trading_environment.is_empty()
        && market.is_empty()
    {
        return None;
    }

    Some(BrokerAccountBinding {
        broker_id,
        account_id,
        trading_environment,
        market,
    })
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
    pub quantity: Decimal,
    pub price: Option<Decimal>,
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

/// Read-only uninstall guidance projected from the Go plugin catalog.
///
/// The product layer deliberately receives the complete wire projection from
/// its consumer-owned port instead of deriving paths or shell commands. This
/// keeps platform-specific quoting and catalog normalization in the current
/// Go owner until the plugin lifecycle has a Rust adapter.
#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct PluginUninstallCommands {
    pub posix: String,
    pub powershell: String,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct PluginUninstallGuidance {
    pub plugin_id: String,
    pub path: String,
    pub exists: bool,
    pub commands: PluginUninstallCommands,
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
    #[error("bad request: {0}")]
    BadRequest(String),
    #[error("busy: {0}")]
    Busy(String),
    #[error("not found: {0}")]
    NotFound(String),
    #[error("upstream failure: {0}")]
    Upstream(String),
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_classified_strategy_errors_match_sentinel_kinds() {
        // Parity: internal/strategy/errors_test.go:8 TestClassifiedStrategyErrorsMatchSentinelKinds
        let bad_req = StrategyError::BadRequest("invalid strategy".to_string());
        assert!(matches!(bad_req, StrategyError::BadRequest(_)));
        assert_eq!(bad_req.to_string(), "bad request: invalid strategy");

        let busy = StrategyError::Busy("optimizer busy".to_string());
        assert!(matches!(busy, StrategyError::Busy(_)));
        assert_eq!(busy.to_string(), "busy: optimizer busy");

        let not_found = StrategyError::NotFound("strategy missing".to_string());
        assert!(matches!(not_found, StrategyError::NotFound(_)));
        assert_eq!(not_found.to_string(), "not found: strategy missing");

        let upstream = StrategyError::Upstream("pine worker failed".to_string());
        assert!(matches!(upstream, StrategyError::Upstream(_)));
        assert_eq!(upstream.to_string(), "upstream failure: pine worker failed");
    }

    #[test]
    fn test_normalize_broker_account_drops_empty_input() {
        // Parity: internal/strategy/instancebinding/binding_test.go:137 TestNormalizeBrokerAccountDropsEmptyInput
        let empty_input = BrokerAccountBinding {
            broker_id: " ".to_string(),
            account_id: " ".to_string(),
            trading_environment: " ".to_string(),
            market: " ".to_string(),
        };
        assert!(normalize_broker_account(Some(&empty_input)).is_none());
        assert!(normalize_broker_account(None).is_none());

        let valid = BrokerAccountBinding {
            broker_id: " FUTU ".to_string(),
            account_id: " 12345 ".to_string(),
            trading_environment: " sim ".to_string(),
            market: " us ".to_string(),
        };
        let norm = normalize_broker_account(Some(&valid)).expect("normalized");
        assert_eq!(norm.broker_id, "futu");
        assert_eq!(norm.account_id, "12345");
        assert_eq!(norm.trading_environment, "SIM");
        assert_eq!(norm.market, "US");
    }
}
