use std::fs;
use std::io::Write;
use std::path::{Path, PathBuf};
use std::sync::{Arc, Mutex};
use std::time::{SystemTime, UNIX_EPOCH};

use jftrade_api::{Clock, SystemClock};
use jftrade_settings::FutuOpenDInstallSettingsStorePort;
use jftrade_settings::{InterfaceSettingsStorePort, PineWorkerSettingsStorePort};
use jftrade_settings::normalize_pine_worker_settings;
use jftrade_store_settings_file::SettingsFileStore;
use jftrade_store_sqlite::{
    AdkStore, BacktestRunStore, BacktestSyncTaskStore, ExecutionOrderStore,
    StrategyRuntimeStore,
};
use jftrade_trading::{
    RealTradeControlEvent, RealTradeControlState, RealTradeHardStopEntry, RealTradeKillSwitchEntry,
    RealTradeRiskSnapshot, RealTradeRuntimeRiskEntry,
};
use serde_json::{Value, json};

use super::provider_now_rfc3339;
use crate::product::product_system_write_port::{
    RealTradeHardStopCommand, RealTradeKillSwitchCommand, RealTradeRuntimeRiskCommand,
    SystemWriteInput, SystemWriteOperation, SystemWritePort, SystemWritePortError,
};
use super::product_production_ports_execution::ExecutionReconciliationWorker;
use crate::product::{
    MarketDataRuntimeStatusPort, ProductionRuntimeStatus, SystemReadSnapshotError,
    SystemReadSnapshotPort,
};
use crate::real_trade_control::RealTradeControlReader;
pub(crate) struct ProductionSystemPort {
    pub(crate) runtime_status: Option<Arc<dyn MarketDataRuntimeStatusPort>>,
    pub(crate) live_hub: Option<Arc<jftrade_api::LiveHub>>,
    pub(crate) settings: Arc<SettingsFileStore>,
    pub(crate) opend_status: ProductionRuntimeStatus,
    pub(crate) worker_status: ProductionRuntimeStatus,
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
    pub(crate) real_trade_control: RealTradeControlReader,
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
                Ok(json!({
                    "subscriptions": [],
                    "recentInvalidations": [],
                    "brokers": [],
                    "runtime": worker.status(),
                }))
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
            SystemReadSnapshotError::Unavailable(
                "runtime dependency worker panicked".to_owned(),
            )
        })??;
        serde_json::to_value(dependencies)
            .map_err(|error| SystemReadSnapshotError::Unavailable(error.to_string()))
    }

    fn futu_opend_snapshot(&self) -> Result<Value, SystemReadSnapshotError> {
        let Some(runtime_status) = self.runtime_status.as_ref() else {
            return Ok(json!({
                "status": "unavailable",
                "reason": "broker integration not enabled",
            }));
        };
        let settings = self
            .settings
            .load_futu_open_d_install_settings()
            .map_err(|error| SystemReadSnapshotError::Unavailable(error.to_string()))?
            .ok_or_else(|| {
                SystemReadSnapshotError::Unavailable(
                    "Futu OpenD settings are not configured".to_owned(),
                )
            })?;
        let state = runtime_status.snapshot();
        let has_error = state
            .quote_last_error
            .as_ref()
            .is_some_and(|error| !error.is_empty())
            || state
                .stream_last_error
                .as_ref()
                .is_some_and(|error| !error.is_empty());
        let connectivity = if state.connected {
            "connected"
        } else if has_error {
            "degraded"
        } else {
            "disconnected"
        };
        let status = if state.connected {
            "healthy"
        } else if has_error || self.opend_status == ProductionRuntimeStatus::Degraded {
            "degraded"
        } else {
            "offline"
        };
        let last_error = state
            .quote_last_error
            .clone()
            .filter(|error| !error.is_empty())
            .or(state
                .stream_last_error
                .clone()
                .filter(|error| !error.is_empty()));
        let live_snapshot = self.live_hub.as_ref().map(|hub| hub.snapshot());
        let live_connections = live_snapshot.as_ref().map_or(0, |snapshot| snapshot.connected);
        let live_limit = settings.max_websocket_connections.max(0) as usize;
        let live_at_limit = live_limit > 0 && live_connections >= live_limit;
        let process_inventory_available = false;
        Ok(json!({
            "checkedAt": provider_now_rfc3339(),
            "status": status,
            "runtime": {
                "connectivity": connectivity,
                "host": settings.host,
                "apiPort": settings.api_port,
                "websocketPort": settings.websocket_port,
                "useEncryption": settings.use_encryption,
                "websocketKeyConfigured": settings.websocket_key_required,
                "marketDataTransport": "bbgo-opend-tcp-api",
                "quoteLoggedIn": Value::Null,
                "tradeLoggedIn": Value::Null,
                "programStatus": Value::Null,
                "serverVersion": Value::Null,
                "minimumVersion": jftrade_integration_futu::MINIMUM_OPEND_VERSION,
                "lastError": last_error,
            },
            "diagnosis": {
                "code": if state.connected { "NONE" } else { "OPEND_UNAVAILABLE" },
                "summary": last_error,
                "manualRetryRequired": !state.connected,
                "restartOpenDRecommended": false,
            },
            "localSocketDiagnostics": {
                "transportMode": "bbgo-opend-tcp-api",
                "configuredOpenDWebSocketLimit": settings.max_websocket_connections,
                "configuredOpenDWebSocketLimitActive": false,
                "configuredOpenDWebSocketLimitScope": "stored for FTWebSocket compatibility; current market-data path uses the OpenD native API via bbgo",
                "websocketEstablishedConnections": live_connections,
                "jftradeLiveWebSocketLimit": settings.max_websocket_connections,
                "jftradeLiveWebSocketAtLimit": live_at_limit,
                "likelyConnectionSaturation": live_at_limit,
                "openDWebSocketPoolLikelySaturation": false,
                "liveQuoteBackoffActive": state.quote_retry_at.is_some(),
                "liveQuoteRetryAfter": state.quote_retry_at,
                "liveQuoteFailureCount": state.quote_failures,
                "liveQuoteLastError": state.quote_last_error,
                "liveStreamBackoffActive": state.stream_retry_at.is_some(),
                "liveStreamRetryAfter": state.stream_retry_at,
                "liveStreamFailureCount": state.stream_failures,
                "liveStreamLastError": state.stream_last_error,
                "topClientProcesses": [],
                "topClientProcessesStatus": if process_inventory_available { "available" } else { "unavailable" },
            },
            "localInstallation": {
                "platform": std::env::consts::OS,
                "installed": false,
                "version": Value::Null,
                "installPath": Value::Null,
                "guiDetected": false,
                "process": {"running": false, "pid": Value::Null, "executablePath": Value::Null},
            },
            "latestVersion": {
                "value": Value::Null,
                "sourceUrl": Value::Null,
                "checkedAt": Value::Null,
                "status": "unknown",
                "error": Value::Null,
            },
            "recommendations": [],
        }))
    }
}

const REAL_TRADE_EVENT_LIMIT: usize = 200;

#[derive(Debug)]
pub(crate) struct ProductionSystemWritePort {
    path: PathBuf,
    state: Mutex<RealTradeControlState>,
}

impl ProductionSystemWritePort {
    pub(crate) fn open(path: impl Into<PathBuf>) -> Result<Self, String> {
        let path = path.into();
        let state = load_state(&path)?;
        Ok(Self {
            path,
            state: Mutex::new(state),
        })
    }

    fn mutate_state(&self, input: &SystemWriteInput) -> Result<Value, SystemWritePortError> {
        let mut current = self
            .state
            .lock()
            .map_err(|_| control_failed("real-trade control lock is poisoned"))?;
        let mut next = current.clone();
        let now = SystemClock.now_rfc3339();
        match input.operation {
            SystemWriteOperation::ManualRetry => {
                return Err(SystemWritePortError::Unavailable(
                    "OpenD runtime is not configured".to_owned(),
                ));
            }
            SystemWriteOperation::ActivateKillSwitch => {
                activate_kill_switch(&mut next, required_kill_switch(input)?, &now);
            }
            SystemWriteOperation::ReleaseKillSwitch => {
                release_kill_switch(&mut next, required_kill_switch(input)?, &now);
            }
            SystemWriteOperation::UpdateRisk => {
                update_risk(&mut next, required_risk(input)?, &now);
            }
            SystemWriteOperation::DisableRisk => {
                disable_risk(&mut next, required_risk(input)?, &now);
            }
            SystemWriteOperation::ActivateHardStop => {
                activate_hard_stop(&mut next, required_hard_stop(input)?, &now);
            }
            SystemWriteOperation::ReleaseHardStop => {
                release_hard_stop(
                    &mut next,
                    input
                        .hard_stop_id
                        .as_deref()
                        .ok_or_else(|| control_failed("real-trade hard stop id is missing"))?,
                    required_hard_stop(input)?,
                    &now,
                )?;
            }
        }
        persist_state(&self.path, &next).map_err(|error| control_failed(&error))?;
        let response = snapshot_value(next.clone())?;
        *current = next;
        Ok(response)
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
        max_order_quantity: command.max_order_quantity,
        max_order_notional: command.max_order_notional,
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

fn load_state(path: &Path) -> Result<RealTradeControlState, String> {
    let bytes = match fs::read(path) {
        Ok(bytes) => bytes,
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => {
            return Ok(RealTradeControlState::default());
        }
        Err(error) => return Err(format!("read real-trade control state: {error}")),
    };
    if bytes.iter().all(u8::is_ascii_whitespace) {
        return Ok(RealTradeControlState::default());
    }
    serde_json::from_slice(&bytes)
        .map_err(|error| format!("decode real-trade control state: {error}"))
}

fn persist_state(path: &Path, state: &RealTradeControlState) -> Result<(), String> {
    let parent = path
        .parent()
        .filter(|parent| !parent.as_os_str().is_empty())
        .unwrap_or_else(|| Path::new("."));
    fs::create_dir_all(parent)
        .map_err(|error| format!("create real-trade control dir: {error}"))?;
    let bytes = serde_json::to_vec_pretty(state)
        .map_err(|error| format!("encode real-trade control state: {error}"))?;
    let mut file = tempfile::NamedTempFile::new_in(parent)
        .map_err(|error| format!("create real-trade control temporary file: {error}"))?;
    file.write_all(&bytes)
        .map_err(|error| format!("write real-trade control state: {error}"))?;
    file.write_all(b"\n")
        .map_err(|error| format!("write real-trade control newline: {error}"))?;
    file.as_file()
        .sync_all()
        .map_err(|error| format!("sync real-trade control state: {error}"))?;
    file.persist(path)
        .map_err(|error| format!("replace real-trade control state: {}", error.error))?;
    Ok(())
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
        return scope;
    }
    if !command.symbol.trim().is_empty() {
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

fn next_id(prefix: &str) -> String {
    let nanos = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map_or(0, |duration| duration.as_nanos());
    format!("{prefix}-{nanos}")
}

fn control_failed(message: &str) -> SystemWritePortError {
    SystemWritePortError::Failed {
        status: 409,
        code: "REAL_TRADE_CONTROL_FAILED".to_owned(),
        message: message.to_owned(),
    }
}
