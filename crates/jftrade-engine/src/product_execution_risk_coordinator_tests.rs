use super::*;
use crate::product::product_production_ports::ProductionSystemWritePort;
use crate::product::product_system_write_port::{
    SystemWriteInput, SystemWriteOperation, SystemWritePort,
};
use jftrade_kernel::Decimal;
use jftrade_trading::TradingEnvironment;
use std::fs;
use std::str::FromStr;
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
        quantity: Decimal::from_str(&qty.to_string()).unwrap_or_default(),
        price: Some(Decimal::from_str(&price.to_string()).unwrap_or_default()),
        // Units mode never carries an amount: the domain rejects a smuggled
        // amount, matching Go's `commandRiskShapeError`.
        amount: None,
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
// Parity: internal/api/trading/execution_test.go:47 TestHandleExecutionPlaceReturnsRiskRejectionEnvelope
// Verifies REAL trading environment pre-trade risk rejection blocks order placement and returns 403/409 risk envelope
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
    match order_res.unwrap_err() {
        ExecutionWritePortError::Failed { status, code, .. } => {
            assert_eq!(status, 403);
            assert_eq!(code, "REAL_TRADE_KILL_SWITCH_ACTIVE");
        }
        other => panic!("expected kill-switch rejection, got {other:?}"),
    }

    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        let _ = fs::set_permissions(&readonly_dir, fs::Permissions::from_mode(0o755));
    }
}

fn kill_switch_entry(id: &str) -> jftrade_trading::RealTradeKillSwitchEntry {
    jftrade_trading::RealTradeKillSwitchEntry {
        id: id.to_owned(),
        trading_environment: "REAL".to_owned(),
        operator_id: "fixture-operator".to_owned(),
        reason: "incident".to_owned(),
        activated_at: "2026-09-22T00:00:00Z".to_owned(),
        updated_at: "2026-09-22T00:00:00Z".to_owned(),
    }
}

fn runtime_risk_entry(max_quantity: &str) -> jftrade_trading::RealTradeRuntimeRiskEntry {
    jftrade_trading::RealTradeRuntimeRiskEntry {
        id: "runtime-risk-fixture".to_owned(),
        trading_environment: "REAL".to_owned(),
        real_trading_enabled: true,
        max_order_quantity: Some(
            Decimal::from_str(max_quantity).expect("runtime risk max quantity"),
        ),
        max_order_notional: None,
        operator_id: "fixture-operator".to_owned(),
        reason: "enable real trading".to_owned(),
        activated_at: "2026-09-22T00:00:00Z".to_owned(),
        updated_at: "2026-09-22T00:00:00Z".to_owned(),
    }
}

fn hard_stop_entry(id: &str) -> jftrade_trading::RealTradeHardStopEntry {
    jftrade_trading::RealTradeHardStopEntry {
        id: id.to_owned(),
        broker_id: "futu".to_owned(),
        trading_environment: "REAL".to_owned(),
        account_id: "acc-1".to_owned(),
        market: Some("US".to_owned()),
        symbol: Some("US.AAPL".to_owned()),
        hard_stop_scope: "symbol".to_owned(),
        operator_id: "fixture-operator".to_owned(),
        reason: "symbol halt".to_owned(),
        activated_at: "2026-09-22T00:00:00Z".to_owned(),
        updated_at: "2026-09-22T00:00:00Z".to_owned(),
    }
}

/// Parity: go:452dea11:internal/trading/execution_test.go:496
/// `TestKillSwitchActivationWaitsForInFlightRealPlacement`. Go starts a REAL
/// placement that blocks inside the broker gateway, then requires
/// `ActivateKillSwitch` to stay pending until that placement finishes; only
/// afterwards may the next REAL order be rejected with
/// `REAL_TRADE_KILL_SWITCH_ACTIVE`. Rust expresses the same serialization
/// with the coordinator's `submission_gate`.
#[test]
fn kill_switch_activation_waits_for_the_in_flight_real_placement() {
    use std::sync::Arc;
    use std::sync::mpsc;
    use std::thread;
    use std::time::Duration;

    let dir = TempDir::new().unwrap();
    let path = write_control_file(
        dir.path(),
        r#"{
            "riskConfig": {
                "realTradingEnabled": true,
                "maxOrderQuantity": 10
            }
        }"#,
    );
    let coordinator = Arc::new(ExecutionRiskCoordinator::new(path));

    let (entered_tx, entered_rx) = mpsc::channel();
    let (release_tx, release_rx) = mpsc::channel::<()>();
    let placement = {
        let coordinator = Arc::clone(&coordinator);
        let order = test_order(TradingEnvironment::Real, 1.0, 10.0);
        thread::spawn(move || {
            coordinator.execute_with_risk_guard(&order, || {
                entered_tx.send(()).expect("signal placement entry");
                release_rx.recv().expect("wait for placement release");
                Ok::<_, ExecutionWritePortError>("placed-before-kill")
            })
        })
    };
    entered_rx
        .recv_timeout(Duration::from_secs(2))
        .expect("REAL placement reached the broker gateway");

    let (activation_tx, activation_rx) = mpsc::channel();
    let activation = {
        let coordinator = Arc::clone(&coordinator);
        thread::spawn(move || {
            let result = coordinator.mutate_with(|state| {
                state.kill_switch = Some(kill_switch_entry("ks-inflight"));
                Ok(())
            });
            activation_tx
                .send(result)
                .expect("signal activation result");
        })
    };
    assert!(
        activation_rx
            .recv_timeout(Duration::from_millis(50))
            .is_err(),
        "kill switch activation must wait for the in-flight REAL placement"
    );

    release_tx
        .send(())
        .expect("release the in-flight placement");
    assert_eq!(
        placement
            .join()
            .expect("placement thread")
            .expect("in-flight placement"),
        "placed-before-kill"
    );
    activation_rx
        .recv_timeout(Duration::from_secs(2))
        .expect("activation completes after the placement")
        .expect("kill switch persisted");
    activation.join().expect("activation thread");

    let rejected = coordinator
        .execute_with_risk_guard(&test_order(TradingEnvironment::Real, 1.0, 10.0), || {
            Ok("must-not-run")
        })
        .expect_err("post-activation REAL order");
    assert!(
        matches!(
            rejected,
            ExecutionWritePortError::Failed {
                status: 403,
                ref code,
                ..
            } if code == "REAL_TRADE_KILL_SWITCH_ACTIVE"
        ),
        "post-activation error = {rejected:?}"
    );
}

/// Parity: go:452dea11:internal/trading/execution_test.go:738
/// `TestRealTradeControlPlanePersistsKillSwitchAndHardStop`. Go activates the
/// kill switch, reloads the plane from disk, releases it, enables runtime
/// risk, activates a hard stop and observes the hard-stop rejection event.
/// Rust keeps the same durable round trip through the control-state file.
#[test]
fn kill_switch_and_hard_stop_survive_a_restart_with_rejection_audit() {
    let dir = TempDir::new().unwrap();
    let path = dir.path().join("real-trade-control.json");
    let coordinator = ExecutionRiskCoordinator::new(&path);
    coordinator
        .mutate_with(|state| {
            state.kill_switch = Some(kill_switch_entry("ks-restart"));
            Ok(())
        })
        .expect("activate kill switch");
    let snapshot = coordinator.snapshot();
    assert!(snapshot.kill_switch_active);
    assert_eq!(snapshot.kill_switch_source.as_deref(), Some("RUNTIME"));

    let reloaded = ExecutionRiskCoordinator::new(&path);
    assert!(
        reloaded.snapshot().kill_switch_active,
        "kill switch survives restart"
    );
    let rejected = reloaded
        .execute_with_risk_guard(&test_order(TradingEnvironment::Real, 1.0, 10.0), || {
            Ok("must-not-run")
        })
        .expect_err("kill-switch rejection after restart");
    assert!(
        matches!(
            rejected,
            ExecutionWritePortError::Failed {
                status: 403,
                ref code,
                ..
            } if code == "REAL_TRADE_KILL_SWITCH_ACTIVE" || code == "REAL_TRADING_DISABLED"
        ),
        "restart rejection = {rejected:?}"
    );

    reloaded
        .mutate_with(|state| {
            state.kill_switch = None;
            state.risk_config = Some(runtime_risk_entry("10"));
            Ok(())
        })
        .expect("release kill switch and enable runtime risk");
    assert!(!reloaded.snapshot().kill_switch_active);
    reloaded
        .mutate_with(|state| {
            state.hard_stops.push(hard_stop_entry("hs-restart"));
            Ok(())
        })
        .expect("activate hard stop");

    let after_restart = ExecutionRiskCoordinator::new(&path);
    assert!(after_restart.snapshot().hard_stops_active);
    let stopped = after_restart
        .execute_with_risk_guard(&test_order(TradingEnvironment::Real, 1.0, 10.0), || {
            Ok("must-not-run")
        })
        .expect_err("hard-stop rejection after restart");
    assert!(
        matches!(
            stopped,
            ExecutionWritePortError::Failed {
                status: 403,
                ref code,
                ..
            } if code == "REAL_TRADE_HARD_STOP_ACTIVE"
        ),
        "hard-stop rejection = {stopped:?}"
    );

    let persisted = fs::read_to_string(&path).expect("read persisted control state");
    assert!(
        persisted.contains("HARD_STOP_REJECT"),
        "hard-stop rejection audit must be persisted: {persisted}"
    );
    let persisted_state: RealTradeControlState =
        serde_json::from_str(&persisted).expect("parse persisted control state");
    assert!(
        persisted_state
            .events
            .iter()
            .any(|event| event.event_type == "HARD_STOP_REJECT"),
        "persisted hard-stop rejection audit = {:?}",
        persisted_state.events
    );
}

/// Parity: go:452dea11:internal/trading/execution_test.go:874
/// `TestRealTradeControlPlaneRollsBackFailedPersistence`. Go breaks the state
/// path and requires every failed mutation to leave the in-memory control
/// state untouched (kill switch, hard-stop entries, runtime-risk limits),
/// then succeeds again once persistence is restored.
#[test]
fn failed_persistence_rolls_back_hard_stop_and_runtime_risk_changes() {
    let dir = TempDir::new().unwrap();
    let path = write_control_file(
        dir.path(),
        r#"{
            "riskConfig": {
                "realTradingEnabled": true,
                "maxOrderQuantity": 10
            },
            "hardStops": [
                {
                    "id": "hs-rollback",
                    "brokerId": "futu",
                    "tradingEnvironment": "REAL",
                    "accountId": "acc-1",
                    "market": "US",
                    "symbol": "US.AAPL",
                    "hardStopScope": "symbol"
                }
            ]
        }"#,
    );
    let mut coordinator = ExecutionRiskCoordinator::new(path.clone());
    let before = coordinator.snapshot();
    assert!(before.hard_stops_active);

    let blocked_parent = dir.path().join("blocked");
    fs::write(&blocked_parent, "blocked").expect("write blocked parent file");
    let blocked_path = blocked_parent.join("real-trade-control.json");
    assert!(
        persist_state(&blocked_path, &RealTradeControlState::default()).is_err(),
        "persisting under a file path must fail"
    );

    coordinator.path = blocked_path.clone();
    let failed_release = coordinator
        .mutate_with(|state| {
            state.hard_stops.clear();
            Ok(())
        })
        .expect_err("hard-stop release must fail when state cannot be persisted");
    assert!(
        matches!(
            failed_release,
            SystemWritePortError::Failed { ref code, .. }
                if code == "CONTROL_PLANE_READ_FAILED" || code == "CONTROL_PLANE_PERSIST_FAILED"
        ),
        "failed release error = {failed_release:?}"
    );
    let after_failed_release = coordinator.snapshot();
    assert_eq!(
        after_failed_release.hard_stop_entries.len(),
        before.hard_stop_entries.len(),
        "failed hard-stop release must keep the active entries"
    );

    let failed_disable = coordinator
        .mutate_with(|state| {
            state.risk_config = None;
            Ok(())
        })
        .expect_err("runtime-risk disable must fail when state cannot be persisted");
    assert!(
        matches!(
            failed_disable,
            SystemWritePortError::Failed { ref code, .. }
                if code == "CONTROL_PLANE_READ_FAILED" || code == "CONTROL_PLANE_PERSIST_FAILED"
        ),
        "failed disable error = {failed_disable:?}"
    );
    let after_failed_disable = coordinator.snapshot();
    assert!(
        after_failed_disable.runtime_risk_configured,
        "failed runtime-risk disable must keep the active config"
    );
    assert_eq!(
        after_failed_disable
            .effective_max_order_quantity
            .map(|quantity| quantity.to_string()),
        before
            .effective_max_order_quantity
            .map(|quantity| quantity.to_string()),
        "failed runtime-risk disable must keep the active limit"
    );

    coordinator.path = path;
    coordinator
        .mutate_with(|state| {
            state.hard_stops.clear();
            Ok(())
        })
        .expect("hard-stop release after restoring persistence");
    let recovered = coordinator.snapshot();
    assert!(!recovered.hard_stops_active);
    assert!(recovered.runtime_risk_configured);
}

/// Parity: go:452dea11:internal/trading/execution_test.go:939
/// `TestRealTradeControlPlaneFailsClosedWhenPersistedStateCannotLoad`. Go
/// receives a usable-but-failing plane from a corrupt state file: the
/// snapshot reports the control plane as unavailable, REAL orders are
/// rejected, and mutations are refused.
#[test]
fn unreadable_persisted_state_fails_closed_and_blocks_mutations() {
    let dir = TempDir::new().unwrap();
    let path = write_control_file(dir.path(), "{");
    assert!(
        ExecutionRiskCoordinator::open(&path).is_err(),
        "strict open must report the unreadable state"
    );

    let coordinator = ExecutionRiskCoordinator::new(&path);
    let rejected = coordinator
        .execute_with_risk_guard(&test_order(TradingEnvironment::Real, 1.0, 10.0), || {
            Ok("must-not-run")
        })
        .expect_err("REAL order with unreadable control plane");
    assert!(
        matches!(
            rejected,
            ExecutionWritePortError::Failed {
                status: 500,
                ref code,
                ..
            } if code == "CONTROL_PLANE_UNAVAILABLE"
        ),
        "fail-closed error = {rejected:?}"
    );

    let snapshot = coordinator.snapshot();
    assert!(!snapshot.control_plane_available);
    assert!(snapshot.control_plane_error.is_some());
    assert!(snapshot.kill_switch_active);

    let blocked = coordinator
        .mutate_with(|state| {
            state.kill_switch = None;
            Ok(())
        })
        .expect_err("mutations must be rejected while the state is unreadable");
    assert!(
        matches!(
            blocked,
            SystemWritePortError::Failed { ref code, .. }
                if code == "CONTROL_PLANE_READ_FAILED"
        ),
        "mutation error = {blocked:?}"
    );
}

#[test]
fn real_trade_gates_come_only_from_the_control_plane_file() {
    // Parity: go:452dea11:internal/trading/execution_test.go:718 TestRealTradeEnvVariablesDoNotConfigurePreTradeRisk
    let dir = TempDir::new().unwrap();
    let path = dir.path().join("real-trade-control.json");
    let coordinator = ExecutionRiskCoordinator::new(path);
    let snapshot = coordinator.snapshot();
    assert!(!snapshot.real_trading_enabled);
    assert!(!snapshot.kill_switch_active);
    assert!(!snapshot.runtime_risk_configured);
    assert!(!snapshot.risk_enabled);
    assert!(snapshot.effective_max_order_quantity.is_none());
    assert!(snapshot.effective_max_order_notional.is_none());
    assert!(snapshot.risk_entry.is_none());

    let rejected = coordinator
        .execute_with_risk_guard(&test_order(TradingEnvironment::Real, 10.0, 150.0), || {
            Ok("must-not-run")
        })
        .expect_err("REAL order with a default control plane");
    assert!(
        matches!(
            rejected,
            ExecutionWritePortError::Failed {
                status: 403,
                ref code,
                ..
            } if code == "REAL_TRADING_DISABLED"
        ),
        "REAL order error = {rejected:?}"
    );
}

fn runtime_risk_command(
    environment: &str,
    real_trading_enabled: bool,
    max_order_quantity: Option<f64>,
    max_order_notional: Option<f64>,
) -> crate::product::product_system_write_port::RealTradeRuntimeRiskCommand {
    crate::product::product_system_write_port::RealTradeRuntimeRiskCommand {
        trading_environment: environment.to_owned(),
        real_trading_enabled,
        max_order_quantity,
        max_order_notional,
        operator_id: "tester".to_owned(),
        reason: "session open".to_owned(),
    }
}

fn dispatch_risk_update(
    port: &crate::product::product_production_ports::ProductionSystemWritePort,
    method: &str,
    body: &[u8],
) -> crate::product::product_system_write_port::SystemWriteResponse {
    crate::product::product_system_write_port::dispatch_system_write(
        &crate::product::product_system_write_port::SystemWriteRequest {
            method: method.to_owned(),
            path: "/api/v1/system/real-trade-risk-limits".to_owned(),
            body: body.to_vec(),
        },
        Some(port),
        "2026-07-04T01:02:03Z",
    )
}

#[test]
fn runtime_risk_config_records_audit_events_and_survives_disable_reload() {
    // Parity: go:452dea11:internal/trading/execution_test.go:813 TestRealTradeControlPlaneRuntimeRiskConfigValidationAndDisableEvents
    let dir = TempDir::new().unwrap();
    let path = dir.path().join("real-trade-control.json");
    let coordinator = std::sync::Arc::new(ExecutionRiskCoordinator::new(&path));
    let port = ProductionSystemWritePort::with_coordinator(std::sync::Arc::clone(&coordinator));

    // Enabling without limits and non-positive limits are rejected before the
    // port mutates the control plane.
    let without_limits = dispatch_risk_update(
        &port,
        "PUT",
        br#"{"tradingEnvironment":"real","realTradingEnabled":true,"operatorId":"tester"}"#,
    );
    assert_eq!(without_limits.status, 400);
    assert_eq!(without_limits.body["error"]["code"], "BAD_REQUEST");
    let negative_limit = dispatch_risk_update(
        &port,
        "PUT",
        br#"{"maxOrderQuantity":-1,"operatorId":"tester"}"#,
    );
    assert_eq!(negative_limit.status, 400);
    assert_eq!(negative_limit.body["error"]["code"], "BAD_REQUEST");
    assert!(
        coordinator.snapshot().risk_events.is_empty(),
        "rejected payloads must not append audit events"
    );

    let command = runtime_risk_command("real", true, Some(25.0), Some(5000.0));
    port.mutate(&SystemWriteInput {
        operation: SystemWriteOperation::UpdateRisk,
        hard_stop_id: None,
        kill_switch: None,
        hard_stop: None,
        risk: Some(command),
    })
    .expect("update runtime risk config");
    let snapshot = coordinator.snapshot();
    assert!(snapshot.real_trading_enabled);
    assert!(snapshot.runtime_risk_configured);
    assert_eq!(
        snapshot.risk_entry.as_ref().map(|entry| (
            entry.operator_id.as_str(),
            entry.trading_environment.as_str()
        )),
        Some(("tester", "REAL"))
    );
    assert_eq!(snapshot.risk_events.len(), 1);
    assert_eq!(snapshot.risk_events[0].action, "RISK_CONFIG_UPDATED");
    assert_eq!(snapshot.risk_events[0].real_trading_enabled, Some(true));
    assert_eq!(
        snapshot.effective_max_order_quantity,
        Some(Decimal::from_str("25").unwrap())
    );
    assert_eq!(
        snapshot.effective_max_order_notional,
        Some(Decimal::from_str("5000").unwrap())
    );

    port.mutate(&SystemWriteInput {
        operation: SystemWriteOperation::DisableRisk,
        hard_stop_id: None,
        kill_switch: None,
        hard_stop: None,
        risk: Some(runtime_risk_command("", false, None, None)),
    })
    .expect("disable runtime risk config");
    let snapshot = coordinator.snapshot();
    assert!(!snapshot.real_trading_enabled);
    assert!(!snapshot.runtime_risk_configured);
    assert!(snapshot.risk_entry.is_none());
    assert_eq!(snapshot.risk_events.len(), 2);
    assert_eq!(snapshot.risk_events[0].action, "RISK_CONFIG_DISABLED");
    assert_eq!(snapshot.risk_events[0].real_trading_enabled, Some(false));

    let reloaded = ExecutionRiskCoordinator::new(&path);
    let reloaded_snapshot = reloaded.snapshot();
    assert!(!reloaded_snapshot.runtime_risk_configured);
    assert!(reloaded_snapshot.risk_entry.is_none());
    assert_eq!(
        reloaded_snapshot.risk_events.len(),
        snapshot.risk_events.len(),
        "reload must preserve the audit event count"
    );
}
