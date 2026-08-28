use std::fs;
use std::io::Write;
use std::path::{Path, PathBuf};
use std::sync::Mutex;
use std::time::{SystemTime, UNIX_EPOCH};

use jftrade_api::{Clock, SystemClock};
use jftrade_trading::{
    RealTradeControlEvent, RealTradeControlState, RealTradeHardStopEntry,
    RealTradeKillSwitchEntry, RealTradeRiskSnapshot, RealTradeRuntimeRiskEntry,
};
use serde_json::Value;

use crate::product::product_system_write_port::{
    RealTradeHardStopCommand, RealTradeKillSwitchCommand, RealTradeRuntimeRiskCommand,
    SystemWriteInput, SystemWriteOperation, SystemWritePort, SystemWritePortError,
};

const REAL_TRADE_EVENT_LIMIT: usize = 200;

#[derive(Debug)]
pub(super) struct ProductionSystemWritePort {
    path: PathBuf,
    state: Mutex<RealTradeControlState>,
}

impl ProductionSystemWritePort {
    pub(super) fn open(path: impl Into<PathBuf>) -> Result<Self, String> {
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
