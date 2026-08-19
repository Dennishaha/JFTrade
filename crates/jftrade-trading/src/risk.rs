use jftrade_kernel::Fixed8;
use serde::{Deserialize, Serialize};

use crate::{OrderCommand, TradingEnvironment};

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct HardStop {
    pub broker_id: Option<String>,
    pub account_id: Option<String>,
    pub market: Option<String>,
    pub symbol: Option<String>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct RiskConfig {
    pub real_trading_enabled: bool,
    pub kill_switch_active: bool,
    pub max_order_quantity: Option<Fixed8>,
    pub max_order_notional: Option<Fixed8>,
    #[serde(default)]
    pub hard_stops: Vec<HardStop>,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct RiskDecision {
    pub allowed: bool,
    pub reason_code: Option<String>,
}

impl RiskDecision {
    fn allow() -> Self {
        Self {
            allowed: true,
            reason_code: None,
        }
    }

    fn reject(reason: &str) -> Self {
        Self {
            allowed: false,
            reason_code: Some(reason.to_owned()),
        }
    }
}

#[derive(Clone, Debug)]
pub struct RiskEngine {
    config: RiskConfig,
}

impl RiskEngine {
    pub fn new(config: RiskConfig) -> Self {
        Self { config }
    }

    pub fn evaluate(&self, command: &OrderCommand) -> RiskDecision {
        if command.environment == TradingEnvironment::Simulate {
            return RiskDecision::allow();
        }
        if !self.config.real_trading_enabled {
            return RiskDecision::reject("REAL_TRADING_DISABLED");
        }
        if self.config.kill_switch_active {
            return RiskDecision::reject("REAL_TRADE_KILL_SWITCH_ACTIVE");
        }
        if self
            .config
            .hard_stops
            .iter()
            .any(|hard_stop| hard_stop.matches(command))
        {
            return RiskDecision::reject("REAL_TRADE_HARD_STOP_ACTIVE");
        }
        if self
            .config
            .max_order_quantity
            .is_some_and(|maximum| command.quantity > maximum)
        {
            return RiskDecision::reject("MAX_ORDER_QUANTITY_EXCEEDED");
        }
        if let Some(maximum) = self.config.max_order_notional {
            let Some(price) = command.price else {
                return RiskDecision::reject("RISK_PRICE_UNAVAILABLE");
            };
            let Ok(notional) = command.quantity.checked_mul(price) else {
                return RiskDecision::reject("INVALID_ORDER_RISK_SHAPE");
            };
            if notional > maximum {
                return RiskDecision::reject("MAX_ORDER_NOTIONAL_EXCEEDED");
            }
        }
        RiskDecision::allow()
    }
}

impl HardStop {
    fn matches(&self, command: &OrderCommand) -> bool {
        owner_matches(&self.broker_id, &command.broker_id)
            && owner_matches(&self.account_id, &command.account_id)
            && exact_matches(&self.market, &command.market)
            && symbol_matches(&self.symbol, &command.market, &command.symbol)
    }
}

fn owner_matches(expected: &Option<String>, actual: &str) -> bool {
    expected.as_ref().is_none_or(|value| {
        let value = value.trim();
        value.is_empty() || value == "*" || value.eq_ignore_ascii_case(actual.trim())
    })
}

fn exact_matches(expected: &Option<String>, actual: &str) -> bool {
    expected
        .as_ref()
        .is_none_or(|value| value.trim().is_empty() || value.trim().eq_ignore_ascii_case(actual))
}

fn symbol_matches(expected: &Option<String>, market: &str, symbol: &str) -> bool {
    let Some(expected) = expected
        .as_ref()
        .map(|value| value.trim())
        .filter(|value| !value.is_empty())
    else {
        return true;
    };
    let expected = expected.to_ascii_uppercase();
    let market = market.trim().to_ascii_uppercase();
    let symbol = symbol.trim().to_ascii_uppercase();
    expected == symbol
        || !market.is_empty() && expected == format!("{market}.{symbol}")
        || !market.is_empty() && symbol == format!("{market}.{expected}")
}

#[cfg(test)]
mod tests {
    use std::str::FromStr;

    use jftrade_kernel::Fixed8;

    use super::{RiskConfig, RiskEngine};
    use crate::{OrderCommand, OrderSide, TradingEnvironment};

    fn command(environment: TradingEnvironment) -> OrderCommand {
        OrderCommand {
            idempotency_key: "key-1".to_owned(),
            trace_id: "trace-1".to_owned(),
            broker_id: "futu".to_owned(),
            account_id: "acc-1".to_owned(),
            environment,
            market: "US".to_owned(),
            symbol: "AAPL".to_owned(),
            side: OrderSide::Buy,
            quantity: Fixed8::from_str("11").expect("quantity"),
            price: Some(Fixed8::from_str("10").expect("price")),
            client_order_id: "client-1".to_owned(),
        }
    }

    #[test]
    fn simulate_bypasses_real_trade_controls_but_real_fails_closed() {
        let engine = RiskEngine::new(RiskConfig {
            real_trading_enabled: false,
            kill_switch_active: true,
            max_order_quantity: None,
            max_order_notional: None,
            hard_stops: Vec::new(),
        });
        assert!(
            engine
                .evaluate(&command(TradingEnvironment::Simulate))
                .allowed
        );
        assert_eq!(
            engine
                .evaluate(&command(TradingEnvironment::Real))
                .reason_code
                .as_deref(),
            Some("REAL_TRADING_DISABLED")
        );
    }

    #[test]
    fn real_notional_uses_fixed_point_arithmetic() {
        let engine = RiskEngine::new(RiskConfig {
            real_trading_enabled: true,
            kill_switch_active: false,
            max_order_quantity: Some(Fixed8::from_str("20").expect("maximum quantity")),
            max_order_notional: Some(Fixed8::from_str("100").expect("maximum notional")),
            hard_stops: Vec::new(),
        });
        assert_eq!(
            engine
                .evaluate(&command(TradingEnvironment::Real))
                .reason_code
                .as_deref(),
            Some("MAX_ORDER_NOTIONAL_EXCEEDED")
        );
    }
}
