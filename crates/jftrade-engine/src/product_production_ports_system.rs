use std::path::PathBuf;
use std::str::FromStr;
use std::sync::Arc;
use std::time::{SystemTime, UNIX_EPOCH};

use jftrade_api::{Clock, SystemClock};
use jftrade_kernel::Decimal;
use jftrade_settings::{
    InterfaceSettingsStorePort, MarketDataProvider, MarketDataProviderRuntimePort,
    PineWorkerSettingsStorePort, normalize_pine_worker_settings,
};
use jftrade_store_settings_file::SettingsFileStore;
use jftrade_store_sqlite::{
    AdkStore, BacktestRunStore, BacktestSyncTaskStore, ExecutionOrderStore, StrategyRuntimeStore,
};
use jftrade_trading::{
    RealTradeControlEvent, RealTradeControlState, RealTradeHardStopEntry, RealTradeKillSwitchEntry,
    RealTradeRiskSnapshot, RealTradeRuntimeRiskEntry,
};
use serde_json::{Value, json};

use super::product_production_ports_execution::ExecutionReconciliationWorker;
use super::provider_now_rfc3339;
use crate::product::ExecutionRiskCoordinator;
use crate::product::product_system_write_port::{
    RealTradeHardStopCommand, RealTradeKillSwitchCommand, RealTradeRuntimeRiskCommand,
    SystemWriteInput, SystemWriteOperation, SystemWritePort, SystemWritePortError,
};
use crate::product::{
    MarketDataRuntimeStatusPort, ProductionRuntimeStatus, SystemReadSnapshotError,
    SystemReadSnapshotPort,
};
pub(crate) struct ProductionSystemPort {
    pub(crate) active_provider_state: Arc<super::ActiveProviderState>,
    pub(crate) trade_runtime: Option<Arc<super::SharedTradeReadRuntime>>,
    pub(crate) runtime_status: Option<Arc<dyn MarketDataRuntimeStatusPort>>,
    pub(crate) live_hub: Option<Arc<jftrade_api::LiveHub>>,
    pub(crate) settings: Arc<SettingsFileStore>,
    pub(crate) opend_status: ProductionRuntimeStatus,
    pub(crate) worker_status: ProductionRuntimeStatus,
    pub(crate) pine_readiness: Option<Arc<jftrade_integration_pine::PineReadinessState>>,
    pub(crate) execution_reconciliation_worker: Option<Arc<ExecutionReconciliationWorker>>,
    pub(crate) database_leases:
        crate::product::product_production_ports::ProductionDatabaseLeaseSnapshot,
    /// Durable stores backing the storage overview projection.  These are
    /// the same leased instances used by the production route adapters; the
    /// system read path never opens a second connection or an ephemeral
    /// queue.
    pub(crate) backtest_store: Arc<BacktestRunStore>,
    pub(crate) backtest_sync_tasks: Arc<BacktestSyncTaskStore>,
    pub(crate) execution_store: Arc<ExecutionOrderStore>,
    pub(crate) adk_store: Arc<AdkStore>,
    pub(crate) strategy_runtime_store: Arc<StrategyRuntimeStore>,
    pub(crate) real_trade_control: Arc<ExecutionRiskCoordinator>,
}

impl std::fmt::Debug for ProductionSystemPort {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("ProductionSystemPort")
            .field("runtime_status", &self.runtime_status.is_some())
            .field("live_hub", &self.live_hub.is_some())
            .field("settings_path", &self.settings.path())
            .field("opend_status", &self.opend_status)
            .field("worker_status", &self.worker_status)
            .field("database_leases", &self.database_leases.status)
            .finish()
    }
}

impl ProductionSystemPort {
    /// Worker evidence comes from the runtime status the composition root
    /// observed while starting helper/Pine workers; "healthy" is only
    /// reported when every configured worker actually reached readiness.
    fn workers_evidence(&self) -> &'static str {
        if self.worker_status == ProductionRuntimeStatus::Ready
            && self
                .pine_readiness
                .as_ref()
                .is_some_and(|readiness| !readiness.is_ready())
        {
            return "degraded";
        }
        match self.worker_status {
            ProductionRuntimeStatus::Ready => "healthy",
            ProductionRuntimeStatus::Degraded | ProductionRuntimeStatus::Failed => "degraded",
            ProductionRuntimeStatus::Unavailable => "unavailable",
        }
    }

    /// Database integrity is derived from the actual WriterLease acquisition
    /// snapshot, never assumed.
    fn database_integrity_evidence(&self) -> &'static str {
        match self.database_leases.status {
            "acquired" => "ok",
            "partial" => "degraded",
            _ => "unavailable",
        }
    }

    /// Settings integrity is proven by actually reading the settings file at
    /// query time; a failed read downgrades the projection.
    fn settings_integrity_evidence(&self) -> Result<&'static str, SystemReadSnapshotError> {
        match self.settings.load_interface_settings() {
            Ok(_) => Ok("ok"),
            Err(_) => Ok("degraded"),
        }
    }
}

/// Project the broker-order-updates diagnostics in Go's `SnapshotResponse`
/// shape.  `subscriptions` stays empty because the engine does not own a
/// broker push subscription; connectivity and the bounded invalidation
/// history now come from the reconciliation worker itself.
pub(crate) fn broker_order_updates_snapshot(
    worker: &ExecutionReconciliationWorker,
) -> serde_json::Value {
    let status = worker.status();
    let invalidations = worker
        .invalidations()
        .into_iter()
        .map(|entry| {
            json!({
                "subscriptionKey": null,
                "brokerId": entry.broker_id,
                "tradingEnvironment": null,
                "accountId": null,
                "market": null,
                "kind": entry.kind,
                "message": entry.message,
                "errorContext": null,
                "consecutiveFailures": null,
                "retryDelayMs": null,
                "backoffUntil": null,
                "createdAt": entry.created_at,
            })
        })
        .collect::<Vec<_>>();
    let brokers = match worker.connectivity() {
        Some(connectivity) => vec![json!({
            "brokerId": super::product_production_ports_trade::ACTIVE_TRADE_BROKER_ID,
            "lastAction": "reconcile-orders",
            "lastActionAt": status.last_scan_at,
            "connectivity": connectivity,
            "lastError": status.last_error,
        })],
        None => Vec::new(),
    };
    json!({
        "subscriptions": [],
        "recentInvalidations": invalidations,
        "brokers": brokers,
        "runtime": status,
    })
}

impl SystemReadSnapshotPort for ProductionSystemPort {
    fn read(&self, path: &str) -> Result<Value, SystemReadSnapshotError> {
        match path {
            "/api/v1/system/futu-opend" => self.futu_opend_snapshot(),
            "/api/v1/system/runtime-dependencies" => self.runtime_dependencies_snapshot(),
            "/api/v1/system/storage/overview" => self.storage_overview_snapshot(),
            // Keep the route fail-closed when the runtime was assembled
            // outside the async production owner (for example, a synchronous
            // composition test).  A real runtime injects the worker below so
            // its status remains observable without broker side effects in a
            // GET request.
            "/api/v1/system/worker/broker-order-updates" => {
                let Some(worker) = self.execution_reconciliation_worker.as_ref() else {
                    return Err(SystemReadSnapshotError::Unavailable(
                        "broker order updates worker is not configured".to_owned(),
                    ));
                };
                Ok(broker_order_updates_snapshot(worker))
            }
            "/api/v1/system/info" => Ok(json!({
                "version": env!("CARGO_PKG_VERSION"),
                "architecture": std::env::consts::ARCH,
                "os": std::env::consts::OS,
                "engine": "rust",
                "productionOwner": "rust",
            })),
            "/api/v1/system/real-trade-kill-switch" => {
                Ok(json!(self.real_trade_control.snapshot().kill_switch()))
            }
            "/api/v1/system/real-trade-risk-limits" => {
                Ok(json!(self.real_trade_control.snapshot().risk_limits()))
            }
            "/api/v1/system/real-trade-risk-events" => {
                Ok(json!(self.real_trade_control.snapshot().risk_events()))
            }
            "/api/v1/system/status" => {
                let market_data = self.runtime_status.as_ref().map(|port| port.snapshot());
                let status = if market_data.as_ref().is_some_and(|state| state.connected) {
                    "operational"
                } else {
                    "degraded"
                };
                Ok(json!({
                    "status": status,
                    "workers": self.workers_evidence(),
                    "marketData": {
                        "connected": market_data.as_ref().is_some_and(|state| state.connected),
                        "activeCount": market_data.as_ref().map_or(0, |state| state.active_count),
                    },
                }))
            }
            "/api/v1/system/diagnostics" => {
                let settings_integrity = self.settings_integrity_evidence()?;
                Ok(json!({
                    "databaseIntegrity": self.database_integrity_evidence(),
                    "settingsIntegrity": settings_integrity,
                    "marketDataRuntime": self.runtime_status.as_ref().map(|port| {
                        if port.snapshot().connected { "ready" } else { "degraded" }
                    }),
                }))
            }
            _ => Err(SystemReadSnapshotError::Unavailable(format!(
                "system path not found: {path}"
            ))),
        }
    }
}

impl ProductionSystemPort {
    fn runtime_dependencies_snapshot(&self) -> Result<Value, SystemReadSnapshotError> {
        let configured_path = self
            .settings
            .load_pine_worker()
            .map_err(|error| SystemReadSnapshotError::Unavailable(error.to_string()))?
            .map(|settings| normalize_pine_worker_settings(&settings).node_binary_path)
            .unwrap_or_default();
        let dependencies = std::thread::spawn(move || {
            let runtime = tokio::runtime::Builder::new_current_thread()
                .enable_all()
                .build()
                .map_err(|error| SystemReadSnapshotError::Unavailable(error.to_string()))?;
            Ok::<_, SystemReadSnapshotError>(runtime.block_on(
                crate::product::runtime_dependencies::inspect(
                    provider_now_rfc3339(),
                    &configured_path,
                ),
            ))
        })
        .join()
        .map_err(|_| {
            SystemReadSnapshotError::Unavailable("runtime dependency worker panicked".to_owned())
        })??;
        serde_json::to_value(dependencies)
            .map_err(|error| SystemReadSnapshotError::Unavailable(error.to_string()))
    }

    fn futu_opend_snapshot(&self) -> Result<Value, SystemReadSnapshotError> {
        super::product_production_ports_open_d_snapshot::project(self)
    }
}

const REAL_TRADE_EVENT_LIMIT: usize = 200;

#[derive(Debug)]
pub(crate) struct ProductionSystemWritePort {
    coordinator: Arc<ExecutionRiskCoordinator>,
    active_provider_state: Option<Arc<super::ActiveProviderState>>,
    trade_runtime: Option<Arc<super::SharedTradeReadRuntime>>,
}

impl ProductionSystemWritePort {
    #[allow(dead_code)]
    pub(crate) fn open(path: impl Into<PathBuf>) -> Result<Self, String> {
        let coordinator = Arc::new(ExecutionRiskCoordinator::new(path));
        Ok(Self {
            coordinator,
            active_provider_state: None,
            trade_runtime: None,
        })
    }

    pub(crate) fn with_coordinator(coordinator: Arc<ExecutionRiskCoordinator>) -> Self {
        Self {
            coordinator,
            active_provider_state: None,
            trade_runtime: None,
        }
    }

    pub(crate) fn with_active_provider_state(
        mut self,
        active_provider_state: Option<Arc<super::ActiveProviderState>>,
    ) -> Self {
        self.active_provider_state = active_provider_state;
        self
    }

    pub(crate) fn with_trade_runtime(
        mut self,
        trade_runtime: Option<Arc<super::SharedTradeReadRuntime>>,
    ) -> Self {
        self.trade_runtime = trade_runtime;
        self
    }

    fn mutate_state(&self, input: &SystemWriteInput) -> Result<Value, SystemWritePortError> {
        let now = SystemClock.now_rfc3339();
        // Parity: go:452dea11:internal/trading/control_plane.go:269
        // RealTradeControlPlane.UpdateRuntimeRiskConfig validates limits before the availability gate.
        if input.operation == SystemWriteOperation::UpdateRisk {
            validate_runtime_risk(required_risk(input)?)?;
        }
        self.coordinator.mutate_with(|state| {
            match input.operation {
                SystemWriteOperation::ManualRetry => {
                    let Some(_) = self.trade_runtime.as_ref() else {
                        return Err(SystemWritePortError::Unavailable(
                            "OpenD runtime is not configured".to_owned(),
                        ));
                    };
                    if let Some(state) = self.active_provider_state.as_ref() {
                        let _ = state.activate(MarketDataProvider::Futu);
                    }
                    return Ok(json!({ "accepted": true }));
                }
                SystemWriteOperation::ActivateKillSwitch => {
                    activate_kill_switch(state, required_kill_switch(input)?, &now)
                }
                SystemWriteOperation::ReleaseKillSwitch => {
                    release_kill_switch(state, required_kill_switch(input)?, &now)
                }
                SystemWriteOperation::UpdateRisk => update_risk(state, required_risk(input)?, &now),
                SystemWriteOperation::DisableRisk => {
                    disable_risk(state, required_risk(input)?, &now)
                }
                SystemWriteOperation::ActivateHardStop => {
                    activate_hard_stop(state, required_hard_stop(input)?, &now)
                }
                SystemWriteOperation::ReleaseHardStop => {
                    let hard_stop_id = input
                        .hard_stop_id
                        .as_deref()
                        .ok_or_else(|| control_failed("real-trade hard stop id is missing"))?;
                    release_hard_stop(state, hard_stop_id, required_hard_stop(input)?, &now)?;
                }
            }
            snapshot_value(state.clone())
        })
    }
}

impl SystemWritePort for ProductionSystemWritePort {
    fn mutate(&self, input: &SystemWriteInput) -> Result<Value, SystemWritePortError> {
        self.mutate_state(input)
    }
}

fn activate_kill_switch(
    state: &mut RealTradeControlState,
    command: &RealTradeKillSwitchCommand,
    now: &str,
) {
    let environment = normalize_environment(&command.trading_environment);
    let activated_at = state
        .kill_switch
        .as_ref()
        .map(|entry| entry.activated_at.clone())
        .filter(|value| !value.is_empty())
        .unwrap_or_else(|| now.to_owned());
    let entry = RealTradeKillSwitchEntry {
        id: "kill-switch-control-plane".to_owned(),
        trading_environment: environment.clone(),
        operator_id: normalize_operator(&command.operator_id),
        reason: command.reason.trim().to_owned(),
        activated_at: activated_at.clone(),
        updated_at: now.to_owned(),
    };
    state.kill_switch = Some(entry.clone());
    prepend_event(
        state,
        RealTradeControlEvent {
            id: next_id("rtks-event"),
            event_type: "activated".to_owned(),
            action: "KILL_SWITCH_ACTIVATE".to_owned(),
            broker_id: "*".to_owned(),
            trading_environment: Some(environment),
            kill_switch_source: Some("RUNTIME".to_owned()),
            operator_id: Some(entry.operator_id),
            reason: optional_trimmed(&entry.reason),
            activated_at: Some(activated_at),
            created_at: now.to_owned(),
            ..RealTradeControlEvent::default()
        },
    );
}

fn release_kill_switch(
    state: &mut RealTradeControlState,
    command: &RealTradeKillSwitchCommand,
    now: &str,
) {
    let previous = state.kill_switch.take();
    let environment = previous.as_ref().map_or_else(
        || normalize_environment(&command.trading_environment),
        |entry| entry.trading_environment.clone(),
    );
    prepend_event(
        state,
        RealTradeControlEvent {
            id: next_id("rtks-event"),
            event_type: "released".to_owned(),
            action: "KILL_SWITCH_RELEASE".to_owned(),
            broker_id: "*".to_owned(),
            trading_environment: Some(environment),
            kill_switch_source: Some("RUNTIME".to_owned()),
            operator_id: Some(normalize_operator(&command.operator_id)),
            reason: optional_trimmed(&command.reason),
            activated_at: previous.map(|entry| entry.activated_at),
            created_at: now.to_owned(),
            ..RealTradeControlEvent::default()
        },
    );
}

fn update_risk(
    state: &mut RealTradeControlState,
    command: &RealTradeRuntimeRiskCommand,
    now: &str,
) {
    let environment = normalize_environment(&command.trading_environment);
    let activated_at = state
        .risk_config
        .as_ref()
        .map(|entry| entry.activated_at.clone())
        .filter(|value| !value.is_empty())
        .unwrap_or_else(|| now.to_owned());
    let entry = RealTradeRuntimeRiskEntry {
        id: "runtime-risk-config".to_owned(),
        trading_environment: environment.clone(),
        real_trading_enabled: command.real_trading_enabled,
        max_order_quantity: opt_f64_dec(command.max_order_quantity),
        max_order_notional: opt_f64_dec(command.max_order_notional),
        operator_id: normalize_operator(&command.operator_id),
        reason: command.reason.trim().to_owned(),
        activated_at: activated_at.clone(),
        updated_at: now.to_owned(),
    };
    state.risk_config = Some(entry.clone());
    prepend_event(
        state,
        RealTradeControlEvent {
            id: next_id("rtrc-event"),
            event_type: "updated".to_owned(),
            action: "RISK_CONFIG_UPDATED".to_owned(),
            broker_id: "*".to_owned(),
            trading_environment: Some(environment),
            operator_id: Some(entry.operator_id),
            reason: optional_trimmed(&entry.reason),
            real_trading_enabled: Some(entry.real_trading_enabled),
            configured_max_order_quantity: entry.max_order_quantity,
            configured_max_order_notional: entry.max_order_notional,
            activated_at: Some(activated_at),
            created_at: now.to_owned(),
            ..RealTradeControlEvent::default()
        },
    );
}

fn has_positive_limit(value: Option<f64>) -> bool {
    value.is_some_and(|limit| limit > 0.0 && limit.is_finite())
}

fn validate_runtime_risk_limit(value: Option<f64>, name: &str) -> Result<(), SystemWritePortError> {
    if has_positive_limit(value) || value.is_none() {
        return Ok(());
    }
    Err(control_failed(&format!(
        "{name} must be positive when provided"
    )))
}

fn validate_runtime_risk(
    command: &RealTradeRuntimeRiskCommand,
) -> Result<(), SystemWritePortError> {
    if command.real_trading_enabled
        && !has_positive_limit(command.max_order_quantity)
        && !has_positive_limit(command.max_order_notional)
    {
        return Err(control_failed(
            "at least one positive runtime risk limit is required before enabling real trading",
        ));
    }
    validate_runtime_risk_limit(command.max_order_quantity, "maxOrderQuantity")?;
    validate_runtime_risk_limit(command.max_order_notional, "maxOrderNotional")?;
    Ok(())
}

fn disable_risk(
    state: &mut RealTradeControlState,
    command: &RealTradeRuntimeRiskCommand,
    now: &str,
) {
    let previous = state.risk_config.take();
    let environment = previous.as_ref().map_or_else(
        || normalize_environment(&command.trading_environment),
        |entry| entry.trading_environment.clone(),
    );
    prepend_event(
        state,
        RealTradeControlEvent {
            id: next_id("rtrc-event"),
            event_type: "disabled".to_owned(),
            action: "RISK_CONFIG_DISABLED".to_owned(),
            broker_id: "*".to_owned(),
            trading_environment: Some(environment),
            operator_id: Some(normalize_operator(&command.operator_id)),
            reason: optional_trimmed(&command.reason),
            real_trading_enabled: Some(false),
            activated_at: previous.map(|entry| entry.activated_at),
            created_at: now.to_owned(),
            ..RealTradeControlEvent::default()
        },
    );
}

fn activate_hard_stop(
    state: &mut RealTradeControlState,
    command: &RealTradeHardStopCommand,
    now: &str,
) {
    let entry = RealTradeHardStopEntry {
        id: next_id("rths"),
        broker_id: normalize_broker(&command.broker_id),
        trading_environment: normalize_environment(&command.trading_environment),
        account_id: normalize_account(&command.account_id),
        market: optional_upper(&command.market),
        symbol: optional_upper(&command.symbol),
        hard_stop_scope: normalize_hard_stop_scope(command),
        operator_id: normalize_operator(&command.operator_id),
        reason: command.reason.trim().to_owned(),
        activated_at: now.to_owned(),
        updated_at: now.to_owned(),
    };
    state.hard_stops.push(entry.clone());
    prepend_event(
        state,
        RealTradeControlEvent {
            id: next_id("rths-event"),
            event_type: "activated".to_owned(),
            action: "HARD_STOP_ACTIVATE".to_owned(),
            broker_id: entry.broker_id.clone(),
            trading_environment: Some(entry.trading_environment.clone()),
            account_id: Some(entry.account_id.clone()),
            market: entry.market.clone(),
            symbol: entry.symbol.clone(),
            hard_stop_scope: Some(entry.hard_stop_scope.clone()),
            operator_id: Some(entry.operator_id.clone()),
            reason: optional_trimmed(&entry.reason),
            hard_stop_id: Some(entry.id.clone()),
            activated_at: Some(entry.activated_at.clone()),
            created_at: now.to_owned(),
            ..RealTradeControlEvent::default()
        },
    );
}

fn release_hard_stop(
    state: &mut RealTradeControlState,
    id: &str,
    command: &RealTradeHardStopCommand,
    now: &str,
) -> Result<(), SystemWritePortError> {
    let position = state
        .hard_stops
        .iter()
        .position(|entry| entry.id == id)
        .ok_or_else(|| control_failed("real-trade hard stop not found"))?;
    let entry = state.hard_stops.remove(position);
    prepend_event(
        state,
        RealTradeControlEvent {
            id: next_id("rths-event"),
            event_type: "released".to_owned(),
            action: "HARD_STOP_RELEASE".to_owned(),
            broker_id: entry.broker_id,
            trading_environment: Some(entry.trading_environment),
            account_id: Some(entry.account_id),
            market: entry.market,
            symbol: entry.symbol,
            hard_stop_scope: Some(entry.hard_stop_scope),
            operator_id: Some(normalize_operator(&command.operator_id)),
            reason: optional_trimmed(&command.reason),
            hard_stop_id: Some(entry.id),
            activated_at: Some(entry.activated_at),
            created_at: now.to_owned(),
            ..RealTradeControlEvent::default()
        },
    );
    Ok(())
}

fn prepend_event(state: &mut RealTradeControlState, event: RealTradeControlEvent) {
    state.events.insert(0, event);
    state.events.truncate(REAL_TRADE_EVENT_LIMIT);
}

fn snapshot_value(state: RealTradeControlState) -> Result<Value, SystemWritePortError> {
    serde_json::to_value(RealTradeRiskSnapshot::from_control_state(state, None))
        .map_err(|error| control_failed(&format!("encode real-trade control response: {error}")))
}

fn required_kill_switch(
    input: &SystemWriteInput,
) -> Result<&RealTradeKillSwitchCommand, SystemWritePortError> {
    input
        .kill_switch
        .as_ref()
        .ok_or_else(|| control_failed("real-trade kill switch command is missing"))
}

fn required_hard_stop(
    input: &SystemWriteInput,
) -> Result<&RealTradeHardStopCommand, SystemWritePortError> {
    input
        .hard_stop
        .as_ref()
        .ok_or_else(|| control_failed("real-trade hard stop command is missing"))
}

fn required_risk(
    input: &SystemWriteInput,
) -> Result<&RealTradeRuntimeRiskCommand, SystemWritePortError> {
    input
        .risk
        .as_ref()
        .ok_or_else(|| control_failed("real-trade risk command is missing"))
}

fn normalize_environment(value: &str) -> String {
    normalized_or(value, "REAL", true)
}

fn normalize_broker(value: &str) -> String {
    let value = value.trim().to_ascii_lowercase();
    if value.is_empty() {
        "*".to_owned()
    } else {
        value
    }
}

fn normalize_account(value: &str) -> String {
    normalized_or(value, "*", false)
}

fn normalize_operator(value: &str) -> String {
    normalized_or(value, "local", false)
}

fn normalized_or(value: &str, fallback: &str, uppercase: bool) -> String {
    let value = value.trim();
    if value.is_empty() {
        fallback.to_owned()
    } else if uppercase {
        value.to_ascii_uppercase()
    } else {
        value.to_owned()
    }
}

fn normalize_hard_stop_scope(command: &RealTradeHardStopCommand) -> String {
    let scope = command.hard_stop_scope.trim().to_ascii_uppercase();
    if matches!(scope.as_str(), "ACCOUNT" | "MARKET" | "SYMBOL") {
        scope
    } else if !command.symbol.trim().is_empty() {
        "SYMBOL".to_owned()
    } else if !command.market.trim().is_empty() {
        "MARKET".to_owned()
    } else {
        "ACCOUNT".to_owned()
    }
}

fn optional_upper(value: &str) -> Option<String> {
    optional_trimmed(value).map(|value| value.to_ascii_uppercase())
}

fn optional_trimmed(value: &str) -> Option<String> {
    let value = value.trim();
    (!value.is_empty()).then(|| value.to_owned())
}

fn opt_f64_dec(value: Option<f64>) -> Option<Decimal> {
    value
        .filter(|v| v.is_finite())
        .and_then(|v| Decimal::from_str(&v.to_string()).ok())
}

fn next_id(prefix: &str) -> String {
    let nanos = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map_or(0, |d| d.as_nanos());
    format!("{prefix}-{nanos}")
}

fn control_failed(message: &str) -> SystemWritePortError {
    SystemWritePortError::Failed {
        status: 409,
        code: "REAL_TRADE_CONTROL_FAILED".to_owned(),
        message: message.to_owned(),
    }
}
