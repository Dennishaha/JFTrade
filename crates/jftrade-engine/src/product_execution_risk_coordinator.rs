use std::path::{Path, PathBuf};
use std::sync::Mutex;
use std::time::{SystemTime, UNIX_EPOCH};

use jftrade_kernel::Fixed8;
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

            let snapshot = RealTradeRiskSnapshot::from_control_state(fresh_state.clone(), None);
            let policy = policy_from_snapshot(&snapshot);
            let decision = evaluate_pre_trade_risk(&policy, order);

            if !decision.allowed {
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
                    let mut candidate = fresh_state.clone();
                    candidate.events.insert(
                        0,
                        RealTradeControlEvent {
                            id: event_id,
                            event_type: "HARD_STOP_REJECT".to_owned(),
                            action: "REJECT".to_owned(),
                            broker_id: order.broker_id.clone(),
                            operation: Some(order.order_kind.clone()),
                            trading_environment: Some("real".to_owned()),
                            account_id: Some(order.account_id.clone()),
                            market: Some(order.market.clone()),
                            symbol: Some(order.symbol.clone()),
                            quantity: order.quantity.to_f64().ok(),
                            price: order.price.and_then(|p| p.to_f64().ok()),
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
                return Err(ExecutionWritePortError::Failed {
                    status,
                    code,
                    message,
                });
            }
        }

        submit_fn()
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
        effective_max_order_quantity: snapshot
            .effective_max_order_quantity
            .and_then(|v| Fixed8::from_f64(v).ok()),
        effective_max_order_notional: snapshot
            .effective_max_order_notional
            .and_then(|v| Fixed8::from_f64(v).ok()),
        hard_stops,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use jftrade_trading::TradingEnvironment;
    use std::fs;
    use tempfile::TempDir;

    fn write_control_file(dir: &Path, content: &str) -> PathBuf {
        let path = dir.join("real-trade-control.json");
        fs::write(&path, content).expect("write control file");
        path
    }

    fn test_order(env: TradingEnvironment, qty: f64, price: f64) -> PreTradeRiskOrder {
        PreTradeRiskOrder {
            broker_id: "futu".to_owned(),
            trading_environment: env,
            account_id: "acc-1".to_owned(),
            market: "US".to_owned(),
            symbol: "US.AAPL".to_owned(),
            side: "BUY".to_owned(),
            order_type: "LIMIT".to_owned(),
            order_kind: "single".to_owned(),
            product_class: "equity".to_owned(),
            quantity_mode: "units".to_owned(),
            quantity: Fixed8::from_f64(qty).unwrap(),
            price: Some(Fixed8::from_f64(price).unwrap()),
            amount: Some(Fixed8::from_f64(qty * price).unwrap()),
            legs: Vec::new(),
        }
    }

    #[test]
    fn simulate_order_bypasses_real_trade_control() {
        let dir = TempDir::new().unwrap();
        let path = dir.path().join("non-existent.json");
        let coordinator = ExecutionRiskCoordinator::new(path);
        let order = test_order(TradingEnvironment::Simulate, 100.0, 150.0);

        let result = coordinator.execute_with_risk_guard(&order, || Ok("done"));
        assert!(result.is_ok());
        assert_eq!(result.unwrap(), "done");
    }

    #[test]
    fn real_order_fails_closed_when_control_plane_unavailable() {
        let dir = TempDir::new().unwrap();
        let path = write_control_file(dir.path(), "{invalid-json-corrupt");
        let coordinator = ExecutionRiskCoordinator::new(path);
        let order = test_order(TradingEnvironment::Real, 10.0, 150.0);

        let result = coordinator.execute_with_risk_guard(&order, || Ok("done"));
        assert!(result.is_err());
        match result.unwrap_err() {
            ExecutionWritePortError::Failed { status, code, .. } => {
                assert_eq!(status, 500);
                assert_eq!(code, "CONTROL_PLANE_UNAVAILABLE");
            }
            other => panic!("expected 500 CONTROL_PLANE_UNAVAILABLE, got {other:?}"),
        }
    }

    #[test]
    fn real_order_rejects_when_kill_switch_active() {
        let dir = TempDir::new().unwrap();
        let path = write_control_file(
            dir.path(),
            r#"{
                "riskConfig": {
                    "realTradingEnabled": true
                },
                "killSwitch": {
                    "id": "ks-1"
                }
            }"#,
        );
        let coordinator = ExecutionRiskCoordinator::new(path);
        let order = test_order(TradingEnvironment::Real, 10.0, 150.0);

        let result = coordinator.execute_with_risk_guard(&order, || Ok("done"));
        assert!(result.is_err());
        match result.unwrap_err() {
            ExecutionWritePortError::Failed { status, code, .. } => {
                assert_eq!(status, 403);
                assert_eq!(code, "REAL_TRADE_KILL_SWITCH_ACTIVE");
            }
            other => panic!("expected 403 REAL_TRADE_KILL_SWITCH_ACTIVE, got {other:?}"),
        }
    }

    #[test]
    fn real_order_rejects_when_hard_stop_matches() {
        let dir = TempDir::new().unwrap();
        let path = write_control_file(
            dir.path(),
            r#"{
                "riskConfig": {
                    "realTradingEnabled": true
                },
                "hardStops": [
                    {
                        "id": "hs-aapl-1",
                        "brokerId": "futu",
                        "tradingEnvironment": "REAL",
                        "accountId": "acc-1",
                        "market": "US",
                        "symbol": "US.AAPL"
                    }
                ]
            }"#,
        );
        let coordinator = ExecutionRiskCoordinator::new(path);
        let order = test_order(TradingEnvironment::Real, 10.0, 150.0);

        let result = coordinator.execute_with_risk_guard(&order, || Ok("done"));
        assert!(result.is_err());
        match result.unwrap_err() {
            ExecutionWritePortError::Failed { status, code, .. } => {
                assert_eq!(status, 403);
                assert_eq!(code, "REAL_TRADE_HARD_STOP_ACTIVE");
            }
            other => panic!("expected 403 REAL_TRADE_HARD_STOP_ACTIVE, got {other:?}"),
        }

        // Verify HARD_STOP_REJECT audit event was recorded and persisted
        let fresh = load_state_strict(coordinator.path()).expect("load persisted state");
        assert_eq!(fresh.events.len(), 1);
        let event = &fresh.events[0];
        assert_eq!(event.event_type, "HARD_STOP_REJECT");
        assert_eq!(event.action, "REJECT");
        assert_eq!(event.hard_stop_id.as_deref(), Some("hs-aapl-1"));
        assert_eq!(event.symbol.as_deref(), Some("US.AAPL"));
    }

    #[test]
    fn real_order_succeeds_when_policy_permits() {
        let dir = TempDir::new().unwrap();
        let path = write_control_file(
            dir.path(),
            r#"{
                "riskConfig": {
                    "realTradingEnabled": true,
                    "maxOrderQuantity": 500.0,
                    "maxOrderNotional": 50000.0
                }
            }"#,
        );
        let coordinator = ExecutionRiskCoordinator::new(path);
        let order = test_order(TradingEnvironment::Real, 10.0, 150.0);

        let res = coordinator.execute_with_risk_guard(&order, || Ok("order-123"));
        assert_eq!(res.unwrap(), "order-123");
    }

    #[test]
    fn mutate_with_activates_kill_switch_atomically_blocking_orders() {
        let dir = TempDir::new().unwrap();
        let path = write_control_file(
            dir.path(),
            r#"{
                "riskConfig": {
                    "realTradingEnabled": true
                }
            }"#,
        );
        let coordinator = ExecutionRiskCoordinator::new(path);
        let order = test_order(TradingEnvironment::Real, 10.0, 150.0);

        // Before kill switch: order succeeds
        let res = coordinator.execute_with_risk_guard(&order, || Ok("submitted"));
        assert_eq!(res.unwrap(), "submitted");

        // Mutate control state to activate kill switch
        let gen_before = coordinator.generation();
        let mutate_res = coordinator.mutate_with(|state| {
            state.kill_switch = Some(jftrade_trading::RealTradeKillSwitchEntry {
                id: "ks-dynamic".to_owned(),
                trading_environment: "REAL".to_owned(),
                operator_id: "admin".to_owned(),
                reason: "emergency halt".to_owned(),
                activated_at: "2026-09-04T12:00:00Z".to_owned(),
                updated_at: "2026-09-04T12:00:00Z".to_owned(),
            });
            Ok(())
        });
        assert!(mutate_res.is_ok());
        assert!(coordinator.generation() > gen_before);

        // After kill switch: next order is blocked atomically
        let res = coordinator.execute_with_risk_guard(&order, || Ok("submitted"));
        assert!(res.is_err());
        match res.unwrap_err() {
            ExecutionWritePortError::Failed { code, .. } => {
                assert_eq!(code, "REAL_TRADE_KILL_SWITCH_ACTIVE");
            }
            other => panic!("expected REAL_TRADE_KILL_SWITCH_ACTIVE, got {other:?}"),
        }
    }

    #[test]
    fn external_file_modification_reflected_immediately() {
        let dir = TempDir::new().unwrap();
        let path = write_control_file(
            dir.path(),
            r#"{
                "riskConfig": {
                    "realTradingEnabled": true
                }
            }"#,
        );
        let coordinator = ExecutionRiskCoordinator::new(&path);
        let order = test_order(TradingEnvironment::Real, 10.0, 150.0);

        // Order succeeds initially
        let res = coordinator.execute_with_risk_guard(&order, || Ok("first"));
        assert_eq!(res.unwrap(), "first");

        // External modification directly to disk file: delete file
        fs::remove_file(&path).unwrap();

        // Next order fails closed while the file is absent.
        let res2 = coordinator.execute_with_risk_guard(&order, || Ok("second"));
        assert!(res2.is_err());
        match res2.unwrap_err() {
            ExecutionWritePortError::Failed { status, code, .. } => {
                assert_eq!(status, 500);
                assert_eq!(code, "CONTROL_PLANE_UNAVAILABLE");
            }
            other => panic!("expected 500 CONTROL_PLANE_UNAVAILABLE, got {other:?}"),
        }
    }

    #[test]
    fn mutate_with_persist_failure_fails_closed_and_preserves_memory_state() {
        let dir = TempDir::new().unwrap();
        let initial_json = r#"{
            "riskConfig": {
                "realTradingEnabled": true
            },
            "killSwitch": {
                "id": "ks-initial"
            }
        }"#;
        let path = write_control_file(dir.path(), initial_json);
        let mut coordinator = ExecutionRiskCoordinator::new(path);
        let readonly_dir = dir.path().join("ro");
        fs::create_dir(&readonly_dir).unwrap();
        let readonly_path = readonly_dir.join("control.json");
        fs::write(&readonly_path, initial_json).unwrap();
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            fs::set_permissions(&readonly_dir, fs::Permissions::from_mode(0o555)).unwrap();
        }
        coordinator.path = readonly_path.clone();

        let res = coordinator.mutate_with(|state| {
            state.kill_switch = None;
            Ok(())
        });
        assert!(res.is_err());
        match res.unwrap_err() {
            SystemWritePortError::Failed { status, code, .. } => {
                assert_eq!(status, 500);
                assert_eq!(code, "CONTROL_PLANE_PERSIST_FAILED");
            }
            other => panic!("expected CONTROL_PLANE_PERSIST_FAILED, got {other:?}"),
        }

        let snapshot = coordinator.snapshot();
        assert!(snapshot.kill_switch_active);
        assert!(snapshot.control_plane_available);

        let order = test_order(TradingEnvironment::Real, 10.0, 150.0);
        let order_res = coordinator.execute_with_risk_guard(&order, || Ok("submitted"));
        assert_eq!(order_res.unwrap(), "submitted");

        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            let _ = fs::set_permissions(&readonly_dir, fs::Permissions::from_mode(0o755));
        }
    }
}
