use std::collections::BTreeSet;

use crate::{
    ExecutionMode, RuntimeState, Signal, SignalOutcome, StrategyError, StrategyNotification,
    TradeIntent, TradePlanReceipt,
};

pub trait TradePlannerPort {
    fn plan_trade(&mut self, intent: TradeIntent) -> Result<TradePlanReceipt, String>;
}

pub struct StrategyCoordinator<P> {
    mode: ExecutionMode,
    state: RuntimeState,
    planner: P,
    seen_signals: BTreeSet<String>,
    generation: u64,
}

impl<P: TradePlannerPort> StrategyCoordinator<P> {
    pub fn new(mode: ExecutionMode, planner: P) -> Self {
        Self {
            mode,
            state: RuntimeState::Stopped,
            planner,
            seen_signals: BTreeSet::new(),
            generation: 0,
        }
    }

    pub fn start(&mut self) -> bool {
        if self.state != RuntimeState::Stopped {
            return false;
        }
        self.state = RuntimeState::Starting;
        true
    }

    pub fn ready(&mut self) -> bool {
        if !matches!(
            self.state,
            RuntimeState::Starting | RuntimeState::Recovering
        ) {
            return false;
        }
        self.state = RuntimeState::Running;
        true
    }

    pub fn disconnected(&mut self) -> bool {
        if self.state != RuntimeState::Running {
            return false;
        }
        self.generation += 1;
        self.state = RuntimeState::Recovering;
        true
    }

    pub fn pause(&mut self) -> bool {
        if self.state != RuntimeState::Running {
            return false;
        }
        self.state = RuntimeState::Paused;
        true
    }

    pub fn resume(&mut self) -> bool {
        if self.state != RuntimeState::Paused {
            return false;
        }
        self.state = RuntimeState::Running;
        true
    }

    pub fn stop(&mut self) -> bool {
        if matches!(self.state, RuntimeState::Stopped | RuntimeState::Stopping) {
            return false;
        }
        self.state = RuntimeState::Stopping;
        true
    }

    pub fn stopped(&mut self) -> bool {
        if self.state == RuntimeState::Stopped {
            return false;
        }
        self.state = RuntimeState::Stopped;
        true
    }

    pub fn handle_signal(&mut self, signal: Signal) -> Result<SignalOutcome, StrategyError> {
        signal.validate()?;
        if self.state != RuntimeState::Running {
            return Err(StrategyError::RuntimeNotRunning);
        }
        if !self.seen_signals.insert(signal.signal_id.clone()) {
            return Ok(SignalOutcome {
                signal_id: signal.signal_id,
                duplicate: true,
                mode: self.mode,
                trade_plan: None,
                notification: None,
            });
        }
        if self.mode == ExecutionMode::NotifyOnly {
            return Ok(SignalOutcome {
                notification: Some(StrategyNotification {
                    source_event_id: signal.signal_id.clone(),
                    trace_id: signal.trace_id.clone(),
                    category: "strategy.order.signal".to_owned(),
                    message: format!(
                        "{} {} {} at {}",
                        signal.side.trim().to_ascii_uppercase(),
                        signal.quantity,
                        signal.symbol.trim().to_ascii_uppercase(),
                        signal
                            .price
                            .map_or_else(|| "MARKET".to_owned(), |price| price.to_string())
                    ),
                    dispatch: false,
                }),
                signal_id: signal.signal_id,
                duplicate: false,
                mode: self.mode,
                trade_plan: None,
            });
        }
        let signal_id = signal.signal_id.clone();
        let receipt = self.planner.plan_trade(TradeIntent {
            idempotency_key: format!("strategy:{}:{}", signal.instance_id, signal.signal_id),
            trace_id: signal.trace_id,
            broker_id: signal.broker_id,
            account_id: signal.account_id,
            live: self.mode == ExecutionMode::Live,
            market: signal.market,
            symbol: signal.symbol,
            side: signal.side,
            quantity: signal.quantity,
            price: signal.price,
        });
        let receipt = match receipt {
            Ok(receipt) => receipt,
            Err(error) => {
                self.seen_signals.remove(&signal_id);
                return Err(StrategyError::TradingPort(error));
            }
        };
        Ok(SignalOutcome {
            signal_id: signal.signal_id,
            duplicate: false,
            mode: self.mode,
            trade_plan: Some(receipt),
            notification: None,
        })
    }

    pub const fn state(&self) -> RuntimeState {
        self.state
    }

    pub const fn generation(&self) -> u64 {
        self.generation
    }

    pub fn into_planner(self) -> P {
        self.planner
    }
}

#[cfg(test)]
mod tests {
    use std::str::FromStr;

    use jftrade_kernel::Fixed8;

    use super::{StrategyCoordinator, TradePlannerPort};
    use crate::{
        ExecutionMode, RuntimeState, Signal, StrategyError, TradeIntent, TradePlanReceipt,
    };

    #[derive(Default)]
    struct RecordingPlanner {
        intents: Vec<TradeIntent>,
        failures_remaining: usize,
    }

    impl TradePlannerPort for RecordingPlanner {
        fn plan_trade(&mut self, intent: TradeIntent) -> Result<TradePlanReceipt, String> {
            if self.failures_remaining > 0 {
                self.failures_remaining -= 1;
                return Err("unavailable".to_owned());
            }
            self.intents.push(intent);
            Ok(TradePlanReceipt {
                accepted: true,
                dispatch: false,
                reason_code: None,
            })
        }
    }

    fn signal(id: &str) -> Signal {
        Signal {
            signal_id: id.to_owned(),
            trace_id: format!("trace-{id}"),
            instance_id: "instance-1".to_owned(),
            broker_id: "futu".to_owned(),
            account_id: "acc-1".to_owned(),
            market: "US".to_owned(),
            symbol: "AAPL".to_owned(),
            side: "BUY".to_owned(),
            quantity: Fixed8::from_str("1").expect("quantity"),
            price: Some(Fixed8::from_str("100").expect("price")),
            observed_at: "2026-08-19T00:00:00Z".parse().expect("timestamp"),
        }
    }

    #[test]
    fn notify_only_never_calls_trading_port_or_dispatches_notification() {
        let mut coordinator =
            StrategyCoordinator::new(ExecutionMode::NotifyOnly, RecordingPlanner::default());
        assert!(coordinator.start());
        assert!(coordinator.ready());
        let first = coordinator.handle_signal(signal("s1")).expect("signal");
        assert!(first.trade_plan.is_none());
        assert!(!first.notification.expect("notification").dispatch);
        assert!(
            coordinator
                .handle_signal(signal("s1"))
                .expect("duplicate")
                .duplicate
        );
        assert!(coordinator.into_planner().intents.is_empty());
    }

    #[test]
    fn live_uses_narrow_port_but_shadow_receipt_never_dispatches() {
        let mut coordinator =
            StrategyCoordinator::new(ExecutionMode::Live, RecordingPlanner::default());
        assert!(coordinator.start());
        assert!(coordinator.ready());
        let outcome = coordinator.handle_signal(signal("s2")).expect("signal");
        assert!(!outcome.trade_plan.expect("trade plan").dispatch);
        assert!(coordinator.disconnected());
        assert_eq!(coordinator.state(), RuntimeState::Recovering);
        assert_eq!(
            coordinator.handle_signal(signal("s3")),
            Err(StrategyError::RuntimeNotRunning)
        );
        assert!(coordinator.ready());
        assert_eq!(coordinator.generation(), 1);
    }

    #[test]
    fn port_failure_can_retry_through_the_idempotent_narrow_port() {
        let mut coordinator = StrategyCoordinator::new(
            ExecutionMode::Paper,
            RecordingPlanner {
                intents: Vec::new(),
                failures_remaining: 1,
            },
        );
        assert!(coordinator.start());
        assert!(coordinator.ready());
        assert_eq!(
            coordinator.handle_signal(signal("s4")),
            Err(StrategyError::TradingPort("unavailable".to_owned()))
        );
        let recovered = coordinator.handle_signal(signal("s4")).expect("recovered");
        assert!(!recovered.duplicate);
        assert!(recovered.trade_plan.expect("trade plan").accepted);
        assert_eq!(coordinator.into_planner().intents.len(), 1);
    }
}
