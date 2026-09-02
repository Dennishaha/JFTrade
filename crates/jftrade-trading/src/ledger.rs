use std::collections::{BTreeMap, BTreeSet};

use jftrade_kernel::{Fixed8, WireTimestamp};
use serde::{Deserialize, Serialize};

use crate::{
    AuditEntry, BrokerOrderEvent, EventOutcome, OrderCommand, OrderProjection, OrderStatus,
    RiskEngine, ShadowCommandPlan, TradingError, canonical_broker_status, reconcile_status,
};

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ShadowCheckpoint {
    commands: BTreeMap<String, CommandState>,
    orders: BTreeMap<String, OrderState>,
    audit: Vec<AuditEntry>,
    next_audit_sequence: u64,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
struct OrderState {
    projection: OrderProjection,
    event_ids: BTreeSet<String>,
    fill_ids: BTreeSet<String>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
struct CommandState {
    fingerprint: String,
    accepted: bool,
    reason_code: Option<String>,
}

#[derive(Clone, Debug)]
pub struct ShadowTrading {
    risk: RiskEngine,
    commands: BTreeMap<String, CommandState>,
    orders: BTreeMap<String, OrderState>,
    audit: Vec<AuditEntry>,
    next_audit_sequence: u64,
}

impl ShadowTrading {
    pub fn new(risk: RiskEngine) -> Self {
        Self {
            risk,
            commands: BTreeMap::new(),
            orders: BTreeMap::new(),
            audit: Vec::new(),
            next_audit_sequence: 1,
        }
    }

    pub fn restore(risk: RiskEngine, checkpoint: ShadowCheckpoint) -> Result<Self, TradingError> {
        if checkpoint.next_audit_sequence == 0 {
            return Err(TradingError::InvalidCheckpoint(
                "audit sequence must be positive",
            ));
        }
        if checkpoint
            .audit
            .windows(2)
            .any(|pair| pair[0].sequence >= pair[1].sequence)
        {
            return Err(TradingError::InvalidCheckpoint(
                "audit sequence must be monotonic",
            ));
        }
        Ok(Self {
            risk,
            commands: checkpoint.commands,
            orders: checkpoint.orders,
            audit: checkpoint.audit,
            next_audit_sequence: checkpoint.next_audit_sequence,
        })
    }

    pub fn plan_order(
        &mut self,
        command: &OrderCommand,
        now: WireTimestamp,
    ) -> Result<ShadowCommandPlan, TradingError> {
        command.validate()?;
        let fingerprint = command.request_fingerprint();
        if let Some(existing) = self.commands.get(&command.idempotency_key).cloned() {
            if existing.fingerprint != fingerprint {
                self.append_audit(
                    &command.trace_id,
                    "ORDER_PLAN",
                    "IDEMPOTENCY_CONFLICT",
                    &command.idempotency_key,
                    now,
                );
                return Err(TradingError::IdempotencyConflict);
            }
            self.append_audit(
                &command.trace_id,
                "ORDER_PLAN",
                "IDEMPOTENT_REPLAY",
                &command.idempotency_key,
                now,
            );
            return Ok(ShadowCommandPlan {
                accepted: existing.accepted,
                replayed: true,
                dispatch: false,
                idempotency_key: command.idempotency_key.clone(),
                trace_id: command.trace_id.clone(),
                normalized_request: existing.fingerprint.clone(),
                reason_code: existing.reason_code.clone(),
            });
        }

        let decision = self.risk.evaluate(command);
        let outcome = if decision.allowed {
            "SHADOW_ACCEPTED"
        } else {
            "RISK_REJECTED"
        };
        self.append_audit(
            &command.trace_id,
            "ORDER_PLAN",
            outcome,
            decision.reason_code.as_deref().unwrap_or("no-dispatch"),
            now,
        );
        self.commands.insert(
            command.idempotency_key.clone(),
            CommandState {
                fingerprint: fingerprint.clone(),
                accepted: decision.allowed,
                reason_code: decision.reason_code.clone(),
            },
        );
        Ok(ShadowCommandPlan {
            accepted: decision.allowed,
            replayed: false,
            dispatch: false,
            idempotency_key: command.idempotency_key.clone(),
            trace_id: command.trace_id.clone(),
            normalized_request: fingerprint,
            reason_code: decision.reason_code,
        })
    }

    pub fn apply_event(&mut self, event: &BrokerOrderEvent) -> Result<EventOutcome, TradingError> {
        if event.event_id.trim().is_empty() {
            return Err(TradingError::InvalidEvent("eventId is required"));
        }
        if event.broker_order_id.trim().is_empty() {
            return Err(TradingError::InvalidEvent("brokerOrderId is required"));
        }
        let state = self
            .orders
            .entry(event.broker_order_id.clone())
            .or_insert_with(|| OrderState {
                projection: OrderProjection {
                    broker_order_id: event.broker_order_id.clone(),
                    status: OrderStatus::Unknown,
                    filled_quantity: Fixed8::ZERO,
                    last_sequence: 0,
                    accepted_events: 0,
                    duplicate_events: 0,
                    stale_events: 0,
                },
                event_ids: BTreeSet::new(),
                fill_ids: BTreeSet::new(),
            });
        let outcome = if !state.event_ids.insert(event.event_id.clone()) {
            state.projection.duplicate_events += 1;
            EventOutcome::Duplicate
        } else {
            let incoming = canonical_broker_status(&event.raw_status);
            let (status, accepted) = reconcile_status(state.projection.status, incoming);
            let stale_sequence = event.sequence < state.projection.last_sequence;
            if !accepted && incoming != state.projection.status || stale_sequence && !accepted {
                state.projection.stale_events += 1;
                EventOutcome::Stale
            } else {
                state.projection.status = status;
                state.projection.last_sequence = state.projection.last_sequence.max(event.sequence);
                if let (Some(fill_id), Some(quantity)) = (&event.fill_id, event.fill_quantity)
                    && state.fill_ids.insert(fill_id.clone())
                {
                    state.projection.filled_quantity = state
                        .projection
                        .filled_quantity
                        .checked_add(quantity)
                        .map_err(|_| TradingError::Arithmetic)?;
                }
                state.projection.accepted_events += 1;
                EventOutcome::Applied
            }
        };
        self.append_audit(
            &event.trace_id,
            "ORDER_EVENT",
            match outcome {
                EventOutcome::Applied => "APPLIED",
                EventOutcome::Duplicate => "DUPLICATE",
                EventOutcome::Stale => "STALE",
            },
            &event.event_id,
            event.occurred_at,
        );
        Ok(outcome)
    }

    pub fn order(&self, broker_order_id: &str) -> Option<&OrderProjection> {
        self.orders
            .get(broker_order_id)
            .map(|state| &state.projection)
    }

    pub fn orders(&self) -> Vec<&OrderProjection> {
        self.orders
            .values()
            .map(|state| &state.projection)
            .collect()
    }

    pub fn audit(&self) -> &[AuditEntry] {
        &self.audit
    }

    pub fn checkpoint(&self) -> ShadowCheckpoint {
        ShadowCheckpoint {
            commands: self.commands.clone(),
            orders: self.orders.clone(),
            audit: self.audit.clone(),
            next_audit_sequence: self.next_audit_sequence,
        }
    }

    fn append_audit(
        &mut self,
        trace_id: &str,
        action: &str,
        outcome: &str,
        detail: &str,
        at: WireTimestamp,
    ) {
        self.audit.push(AuditEntry {
            sequence: self.next_audit_sequence,
            trace_id: trace_id.to_owned(),
            action: action.to_owned(),
            outcome: outcome.to_owned(),
            detail: detail.to_owned(),
            at,
        });
        self.next_audit_sequence += 1;
    }
}

#[cfg(test)]
mod tests {
    use std::str::FromStr;

    use jftrade_kernel::{Fixed8, WireTimestamp};

    use super::ShadowTrading;
    use crate::{
        BrokerOrderEvent, EventOutcome, OrderCommand, OrderSide, OrderStatus, RiskConfig,
        RiskEngine, TradingEnvironment, TradingError,
    };

    fn risk() -> RiskEngine {
        RiskEngine::new(RiskConfig {
            real_trading_enabled: true,
            kill_switch_active: false,
            max_order_quantity: None,
            max_order_notional: None,
            hard_stops: Vec::new(),
        })
    }

    fn command() -> OrderCommand {
        OrderCommand {
            idempotency_key: "order-key".to_owned(),
            trace_id: "trace-order".to_owned(),
            broker_id: "futu".to_owned(),
            account_id: "acc-1".to_owned(),
            environment: TradingEnvironment::Simulate,
            market: "US".to_owned(),
            symbol: "AAPL".to_owned(),
            side: OrderSide::Buy,
            quantity: Fixed8::from_str("2").expect("quantity"),
            price: Some(Fixed8::from_str("100").expect("price")),
            client_order_id: "client-order".to_owned(),
        }
    }

    fn timestamp(second: u64) -> WireTimestamp {
        format!("2026-08-19T00:00:{second:02}Z")
            .parse()
            .expect("timestamp")
    }

    fn event(
        id: &str,
        sequence: u64,
        status: &str,
        fill: Option<(&str, &str)>,
    ) -> BrokerOrderEvent {
        BrokerOrderEvent {
            event_id: id.to_owned(),
            trace_id: "trace-event".to_owned(),
            broker_order_id: "broker-1".to_owned(),
            sequence,
            raw_status: status.to_owned(),
            fill_id: fill.map(|(fill_id, _)| fill_id.to_owned()),
            fill_quantity: fill.map(|(_, quantity)| Fixed8::from_str(quantity).expect("fill")),
            occurred_at: timestamp(sequence),
        }
    }

    #[test]
    fn idempotency_never_creates_a_dispatch_plan() {
        let mut shadow = ShadowTrading::new(risk());
        let first = shadow
            .plan_order(&command(), timestamp(0))
            .expect("first plan");
        let second = shadow.plan_order(&command(), timestamp(1)).expect("replay");
        assert!(first.accepted && !first.dispatch && !first.replayed);
        assert!(second.accepted && !second.dispatch && second.replayed);

        let mut conflicting = command();
        conflicting.symbol = "MSFT".to_owned();
        assert_eq!(
            shadow.plan_order(&conflicting, timestamp(2)),
            Err(TradingError::IdempotencyConflict)
        );
    }

    #[test]
    fn rejected_idempotent_replay_preserves_the_original_decision() {
        let mut shadow = ShadowTrading::new(RiskEngine::new(RiskConfig {
            real_trading_enabled: false,
            kill_switch_active: false,
            max_order_quantity: None,
            max_order_notional: None,
            hard_stops: Vec::new(),
        }));
        let mut input = command();
        input.environment = TradingEnvironment::Real;
        let first = shadow
            .plan_order(&input, timestamp(0))
            .expect("first rejection");
        let replay = shadow
            .plan_order(&input, timestamp(1))
            .expect("replayed rejection");
        assert!(!first.accepted && !first.replayed);
        assert!(!replay.accepted && replay.replayed);
        assert_eq!(replay.reason_code, first.reason_code);
    }

    #[test]
    fn duplicate_and_out_of_order_events_cannot_regress_or_double_fill() {
        let mut shadow = ShadowTrading::new(risk());
        assert_eq!(
            shadow.apply_event(&event("e1", 1, "NEW", None)),
            Ok(EventOutcome::Applied)
        );
        assert_eq!(
            shadow.apply_event(&event("e2", 3, "FILLED_PART", Some(("fill-1", "1")))),
            Ok(EventOutcome::Applied)
        );
        assert_eq!(
            shadow.apply_event(&event("e2", 3, "FILLED_PART", Some(("fill-1", "1")))),
            Ok(EventOutcome::Duplicate)
        );
        assert_eq!(
            shadow.apply_event(&event("e3", 2, "NEW", None)),
            Ok(EventOutcome::Stale)
        );
        assert_eq!(
            shadow.apply_event(&event("e4", 4, "FILLED_ALL", Some(("fill-2", "1")))),
            Ok(EventOutcome::Applied)
        );
        let order = shadow.order("broker-1").expect("order");
        assert_eq!(order.status, OrderStatus::Filled);
        assert_eq!(order.filled_quantity.to_string(), "2");
        assert_eq!((order.duplicate_events, order.stale_events), (1, 1));
    }

    #[test]
    fn checkpoint_restores_idempotency_and_event_deduplication() {
        let mut shadow = ShadowTrading::new(risk());
        shadow.plan_order(&command(), timestamp(0)).expect("plan");
        shadow
            .apply_event(&event("e1", 1, "NEW", None))
            .expect("event");
        let encoded = serde_json::to_vec(&shadow.checkpoint()).expect("encode checkpoint");
        let checkpoint = serde_json::from_slice(&encoded).expect("decode checkpoint");
        let mut restored = ShadowTrading::restore(risk(), checkpoint).expect("restore");
        assert!(
            restored
                .plan_order(&command(), timestamp(1))
                .expect("replay")
                .replayed
        );
        assert_eq!(
            restored.apply_event(&event("e1", 1, "NEW", None)),
            Ok(EventOutcome::Duplicate)
        );
    }
}
