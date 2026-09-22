use std::path::{Path, PathBuf};
use std::sync::Mutex;
use std::time::{SystemTime, UNIX_EPOCH};

use jftrade_trading::{
    HardStop, PreTradeRiskOrder, PreTradeRiskPolicy, RealTradeControlEvent, RealTradeControlState,
    RealTradeRiskSnapshot, evaluate_pre_trade_risk,
};

use crate::product::product_execution_write_port::ExecutionWritePortError;
use crate::product::product_system_write_port::SystemWritePortError;
use crate::real_trade_control::{ensure_default_state_file, load_state_strict, persist_state};

const REAL_TRADE_EVENT_LIMIT: usize = 200;

#[derive(Debug)]
pub(crate) struct ExecutionRiskCoordinator {
    path: PathBuf,
    submission_gate: Mutex<()>,
    state: Mutex<CoordinatorInner>,
}

#[derive(Debug)]
struct CoordinatorInner {
    generation: u64,
    state: RealTradeControlState,
    control_plane_error: Option<String>,
}

impl ExecutionRiskCoordinator {
    pub(crate) fn open(path: impl Into<PathBuf>) -> Result<Self, String> {
        let path = path.into();
        ensure_default_state_file(&path)?;
        let state = load_state_strict(&path)?;
        Ok(Self {
            path,
            submission_gate: Mutex::new(()),
            state: Mutex::new(CoordinatorInner {
                generation: 1,
                state,
                control_plane_error: None,
            }),
        })
    }

    pub(crate) fn new(path: impl Into<PathBuf>) -> Self {
        let path = path.into();
        let _ = ensure_default_state_file(&path);
        let (state, control_plane_error) = match load_state_strict(&path) {
            Ok(state) => (state, None),
            Err(error) => (RealTradeControlState::default(), Some(error)),
        };
        Self {
            path,
            submission_gate: Mutex::new(()),
            state: Mutex::new(CoordinatorInner {
                generation: 1,
                state,
                control_plane_error,
            }),
        }
    }

    #[allow(dead_code)]
    pub(crate) fn path(&self) -> &Path {
        &self.path
    }

    #[allow(dead_code)]
    pub(crate) fn generation(&self) -> u64 {
        self.state.lock().map(|guard| guard.generation).unwrap_or(0)
    }

    #[allow(dead_code)]
    pub(crate) fn bump_generation(&self) -> u64 {
        if let Ok(mut guard) = self.state.lock() {
            guard.generation = guard.generation.wrapping_add(1);
            guard.generation
        } else {
            0
        }
    }

    pub(crate) fn snapshot(&self) -> RealTradeRiskSnapshot {
        match load_state_strict(&self.path) {
            Ok(fresh_state) => {
                if let Ok(mut guard) = self.state.lock() {
                    guard.state = fresh_state;
                    guard.control_plane_error = None;
                }
            }
            Err(error) => {
                if let Ok(mut guard) = self.state.lock() {
                    guard.control_plane_error = Some(error);
                    guard.generation = guard.generation.wrapping_add(1);
                }
            }
        }
        if let Ok(guard) = self.state.lock() {
            RealTradeRiskSnapshot::from_control_state(
                guard.state.clone(),
                guard.control_plane_error.clone(),
            )
        } else {
            RealTradeRiskSnapshot::from_control_state(
                RealTradeControlState::default(),
                Some("risk coordinator lock poisoned".to_owned()),
            )
        }
    }

    #[allow(dead_code)]
    pub(crate) fn snapshot_policy(&self) -> PreTradeRiskPolicy {
        let snapshot = self.snapshot();
        policy_from_snapshot(&snapshot)
    }

    pub(crate) fn mutate_with<F, R>(&self, mutator: F) -> Result<R, SystemWritePortError>
    where
        F: FnOnce(&mut RealTradeControlState) -> Result<R, SystemWritePortError>,
    {
        let _gate = self.submission_gate.lock().map_err(|_| {
            SystemWritePortError::Unavailable("submission gate poisoned".to_owned())
        })?;

        let fresh_state = match load_state_strict(&self.path) {
            Ok(s) => s,
            Err(error) => {
                let mut guard = self.state.lock().map_err(|_| {
                    SystemWritePortError::Unavailable("risk coordinator lock poisoned".to_owned())
                })?;
                guard.control_plane_error = Some(error.clone());
                guard.generation = guard.generation.wrapping_add(1);
                return Err(SystemWritePortError::Failed {
                    status: 500,
                    code: "CONTROL_PLANE_READ_FAILED".to_owned(),
                    message: error,
                });
            }
        };

        let mut candidate = fresh_state;
        let result = mutator(&mut candidate)?;
        if let Err(error) = persist_state(&self.path, &candidate) {
            let mut guard = self.state.lock().map_err(|_| {
                SystemWritePortError::Unavailable("risk coordinator lock poisoned".to_owned())
            })?;
            guard.control_plane_error = Some(error.clone());
            guard.generation = guard.generation.wrapping_add(1);
            return Err(SystemWritePortError::Failed {
                status: 500,
                code: "CONTROL_PLANE_PERSIST_FAILED".to_owned(),
                message: error,
            });
        }

        let mut guard = self.state.lock().map_err(|_| {
            SystemWritePortError::Unavailable("risk coordinator lock poisoned".to_owned())
        })?;
        guard.state = candidate;
        guard.generation = guard.generation.wrapping_add(1);
        guard.control_plane_error = None;
        Ok(result)
    }

    pub(crate) fn execute_with_risk_guard<T, F>(
        &self,
        order: &PreTradeRiskOrder,
        submit_fn: F,
    ) -> Result<T, ExecutionWritePortError>
    where
        F: FnOnce() -> Result<T, ExecutionWritePortError>,
    {
        let _gate = self.submission_gate.lock().map_err(|_| {
            ExecutionWritePortError::Unavailable("submission gate poisoned".to_owned())
        })?;

        if order.trading_environment == jftrade_trading::TradingEnvironment::Real {
            let current_state = {
                let guard = self.state.lock().map_err(|_| {
                    ExecutionWritePortError::Unavailable(
                        "risk coordinator lock poisoned".to_owned(),
                    )
                })?;
                guard.state.clone()
            };

            let (fresh_state, control_error) = match load_state_strict(&self.path) {
                Ok(s) => {
                    if let Ok(mut guard) = self.state.lock() {
                        guard.state = s.clone();
                        guard.control_plane_error = None;
                    }
                    (s, None)
                }
                Err(error) => {
                    if let Ok(mut guard) = self.state.lock() {
                        guard.control_plane_error = Some(error.clone());
                        guard.generation = guard.generation.wrapping_add(1);
                    }
                    (current_state, Some(error))
                }
            };

            if let Some(error) = control_error {
                return Err(ExecutionWritePortError::Failed {
                    status: 500,
                    code: "CONTROL_PLANE_UNAVAILABLE".to_owned(),
                    message: format!("pre-trade risk control plane unavailable: {error}"),
                });
            }

            self.reject_real_order(order, fresh_state)?;
        }

        submit_fn()
    }

    /// Go evaluates the pre-trade gateway once before spending the preview
    /// credential (`evaluatePlaceExecutionOrderRisk`) and once more inside the
    /// submission window (`executePlaceOrderWithRisk`). The earlier call reads
    /// the control-plane state the coordinator already owns, so a REAL order
    /// the current state already rejects is refused without consuming the
    /// preview; the submission gate stays authoritative because it re-reads the
    /// control file.
    pub(crate) fn precheck(
        &self,
        order: &PreTradeRiskOrder,
    ) -> Result<(), ExecutionWritePortError> {
        if order.trading_environment != jftrade_trading::TradingEnvironment::Real {
            return Ok(());
        }
        let (state, control_error) = {
            let guard = self.state.lock().map_err(|_| {
                ExecutionWritePortError::Unavailable("risk coordinator lock poisoned".to_owned())
            })?;
            (guard.state.clone(), guard.control_plane_error.clone())
        };
        if let Some(error) = control_error {
            return Err(ExecutionWritePortError::Failed {
                status: 500,
                code: "CONTROL_PLANE_UNAVAILABLE".to_owned(),
                message: format!("pre-trade risk control plane unavailable: {error}"),
            });
        }
        self.reject_real_order(order, state)
    }

    /// Shared REAL rejection projection: maps the evaluated decision onto the
    /// wire error and records Go's hard-stop audit event.
    fn reject_real_order(
        &self,
        order: &PreTradeRiskOrder,
        state: RealTradeControlState,
    ) -> Result<(), ExecutionWritePortError> {
        let snapshot = RealTradeRiskSnapshot::from_control_state(state.clone(), None);
        let policy = policy_from_snapshot(&snapshot);
        let decision = evaluate_pre_trade_risk(&policy, order);
        if decision.allowed {
            return Ok(());
        }
        let code = decision
            .reason_code
            .unwrap_or_else(|| "PRE_TRADE_RISK_REJECTED".to_owned());
        let message = decision
            .reason_message
            .unwrap_or_else(|| "pre-trade risk rejected order submission".to_owned());

        if code == "REAL_TRADE_HARD_STOP_ACTIVE" {
            let now = time::OffsetDateTime::now_utc()
                .format(&time::format_description::well_known::Rfc3339)
                .unwrap_or_else(|_| "1970-01-01T00:00:00Z".to_owned());
            let event_id = {
                let nanos = SystemTime::now()
                    .duration_since(UNIX_EPOCH)
                    .map_or(0, |d| d.as_nanos());
                format!("rths-reject-{nanos}")
            };
            let mut candidate = state.clone();
            candidate.events.insert(
                0,
                RealTradeControlEvent {
                    id: event_id,
                    // Go's `recordRejectedHardStop` writes EventType
                    // "rejected" with Action "HARD_STOP_REJECT"; the action
                    // prefix is what the hard-stop event projection filters
                    // on, so the rejection must appear in that trail.
                    event_type: "rejected".to_owned(),
                    action: "HARD_STOP_REJECT".to_owned(),
                    broker_id: order.broker_id.clone(),
                    operation: Some(order.order_kind.clone()),
                    trading_environment: Some("real".to_owned()),
                    account_id: Some(order.account_id.clone()),
                    market: Some(order.market.clone()),
                    symbol: Some(order.symbol.clone()),
                    quantity: Some(order.quantity),
                    price: order.price,
                    operator_id: Some("system".to_owned()),
                    reason: Some(message.clone()),
                    error_code: Some(code.clone()),
                    hard_stop_id: decision.matched_hard_stop_id.clone(),
                    created_at: now,
                    ..RealTradeControlEvent::default()
                },
            );
            candidate.events.truncate(REAL_TRADE_EVENT_LIMIT);
            if let Err(error) = persist_state(&self.path, &candidate) {
                if let Ok(mut guard) = self.state.lock() {
                    guard.control_plane_error = Some(error.clone());
                    guard.generation = guard.generation.wrapping_add(1);
                }
                return Err(ExecutionWritePortError::Failed {
                    status: 500,
                    code: "CONTROL_PLANE_PERSIST_FAILED".to_owned(),
                    message: format!("persist hard-stop rejection audit: {error}"),
                });
            }
            if let Ok(mut guard) = self.state.lock() {
                guard.state = candidate;
                guard.control_plane_error = None;
                guard.generation = guard.generation.wrapping_add(1);
            }
        }

        let status = if code == "INVALID_ORDER_RISK_SHAPE" {
            400
        } else {
            403
        };
        Err(ExecutionWritePortError::Failed {
            status,
            code,
            message,
        })
    }
}

fn policy_from_snapshot(snapshot: &RealTradeRiskSnapshot) -> PreTradeRiskPolicy {
    let hard_stops = snapshot
        .hard_stop_entries
        .iter()
        .map(|entry| HardStop {
            id: Some(entry.id.clone()),
            broker_id: Some(entry.broker_id.clone()),
            trading_environment: Some(entry.trading_environment.clone()),
            account_id: Some(entry.account_id.clone()),
            market: entry.market.clone(),
            symbol: entry.symbol.clone(),
        })
        .collect();

    PreTradeRiskPolicy {
        control_plane_available: snapshot.control_plane_available,
        real_trading_enabled: snapshot.real_trading_enabled,
        kill_switch_active: snapshot.kill_switch_active,
        effective_max_order_quantity: snapshot.effective_max_order_quantity,
        effective_max_order_notional: snapshot.effective_max_order_notional,
        hard_stops,
    }
}

#[cfg(test)]
#[path = "product_execution_risk_coordinator_tests.rs"]
mod tests;
