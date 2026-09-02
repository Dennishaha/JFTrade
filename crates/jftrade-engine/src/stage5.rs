use std::sync::{Arc, Mutex};

use jftrade_kernel::WireTimestamp;
use jftrade_strategy::{
    ExecutionMode, StrategyCoordinator, TradeIntent, TradePlanReceipt, TradePlannerPort,
};
use jftrade_trading::{
    OrderCommand, OrderSide, RiskConfig, RiskEngine, ShadowCheckpoint, ShadowTrading,
    TradingEnvironment,
};
use thiserror::Error;

#[derive(Clone)]
pub struct Stage5TradingPort {
    shadow: Arc<Mutex<ShadowTrading>>,
    planned_at: WireTimestamp,
}

pub struct Stage5Assembly {
    shadow: Arc<Mutex<ShadowTrading>>,
    planned_at: WireTimestamp,
}

impl Stage5Assembly {
    pub fn new(risk_config: RiskConfig, planned_at: WireTimestamp) -> Self {
        Self {
            shadow: Arc::new(Mutex::new(ShadowTrading::new(RiskEngine::new(risk_config)))),
            planned_at,
        }
    }

    pub fn strategy(&self, mode: ExecutionMode) -> StrategyCoordinator<Stage5TradingPort> {
        StrategyCoordinator::new(
            mode,
            Stage5TradingPort {
                shadow: Arc::clone(&self.shadow),
                planned_at: self.planned_at,
            },
        )
    }

    pub fn with_shadow<T>(
        &self,
        operation: impl FnOnce(&ShadowTrading) -> T,
    ) -> Result<T, Stage5AssemblyError> {
        let shadow = self
            .shadow
            .lock()
            .map_err(|_| Stage5AssemblyError::Poisoned)?;
        Ok(operation(&shadow))
    }

    pub fn with_shadow_mut<T>(
        &self,
        operation: impl FnOnce(&mut ShadowTrading) -> T,
    ) -> Result<T, Stage5AssemblyError> {
        let mut shadow = self
            .shadow
            .lock()
            .map_err(|_| Stage5AssemblyError::Poisoned)?;
        Ok(operation(&mut shadow))
    }

    pub fn checkpoint(&self) -> Result<ShadowCheckpoint, Stage5AssemblyError> {
        self.with_shadow(ShadowTrading::checkpoint)
    }
}

impl TradePlannerPort for Stage5TradingPort {
    fn plan_trade(&mut self, intent: TradeIntent) -> Result<TradePlanReceipt, String> {
        let side = if intent.side.trim().eq_ignore_ascii_case("SELL") {
            OrderSide::Sell
        } else if intent.side.trim().eq_ignore_ascii_case("BUY") {
            OrderSide::Buy
        } else {
            return Err("signal side must be BUY or SELL".to_owned());
        };
        let command = OrderCommand {
            client_order_id: intent.idempotency_key.clone(),
            idempotency_key: intent.idempotency_key,
            trace_id: intent.trace_id,
            broker_id: intent.broker_id,
            account_id: intent.account_id,
            environment: if intent.live {
                TradingEnvironment::Real
            } else {
                TradingEnvironment::Simulate
            },
            market: intent.market,
            symbol: intent.symbol,
            side,
            quantity: intent.quantity,
            price: intent.price,
        };
        let mut shadow = self
            .shadow
            .lock()
            .map_err(|_| "stage 5 shadow lock is poisoned".to_owned())?;
        let plan = shadow
            .plan_order(&command, self.planned_at)
            .map_err(|error| error.to_string())?;
        Ok(TradePlanReceipt {
            accepted: plan.accepted,
            dispatch: plan.dispatch,
            reason_code: plan.reason_code,
        })
    }
}

#[derive(Debug, Error)]
pub enum Stage5AssemblyError {
    #[error("stage 5 shadow state lock is poisoned")]
    Poisoned,
}

#[cfg(test)]
mod tests {
    use std::str::FromStr;

    use jftrade_kernel::Fixed8;
    use jftrade_strategy::{ExecutionMode, Signal};
    use jftrade_trading::RiskConfig;

    use super::Stage5Assembly;

    #[test]
    fn composition_maps_strategy_through_a_non_dispatching_trading_port() {
        let assembly = Stage5Assembly::new(
            RiskConfig {
                real_trading_enabled: true,
                kill_switch_active: false,
                max_order_quantity: None,
                max_order_notional: None,
                hard_stops: Vec::new(),
            },
            "2026-08-19T00:00:00Z".parse().expect("timestamp"),
        );
        let mut strategy = assembly.strategy(ExecutionMode::Live);
        assert!(strategy.start() && strategy.ready());
        let outcome = strategy
            .handle_signal(Signal {
                signal_id: "signal-1".to_owned(),
                trace_id: "trace-1".to_owned(),
                instance_id: "instance-1".to_owned(),
                broker_id: "futu".to_owned(),
                account_id: "acc-1".to_owned(),
                market: "US".to_owned(),
                symbol: "AAPL".to_owned(),
                side: "BUY".to_owned(),
                quantity: Fixed8::from_str("1").expect("quantity"),
                price: Some(Fixed8::from_str("100").expect("price")),
                observed_at: "2026-08-19T00:00:00Z".parse().expect("timestamp"),
            })
            .expect("signal");
        let receipt = outcome.trade_plan.expect("trade plan");
        assert!(receipt.accepted);
        assert!(!receipt.dispatch);
        assert_eq!(
            assembly
                .with_shadow(|shadow| shadow.audit().len())
                .expect("audit"),
            1
        );
    }
}
