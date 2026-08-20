use serde::{Deserialize, Serialize};

const BLOCKED_OPERATIONS: [&str; 2] = ["PLACE", "MODIFY"];

#[derive(Clone, Debug, Default, Deserialize, PartialEq, Serialize)]
#[serde(default, rename_all = "camelCase")]
pub struct RealTradeControlState {
    pub risk_config: Option<RealTradeRuntimeRiskEntry>,
    pub kill_switch: Option<RealTradeKillSwitchEntry>,
    pub hard_stops: Vec<RealTradeHardStopEntry>,
    pub events: Vec<RealTradeControlEvent>,
}

#[derive(Clone, Debug, Default, Deserialize, PartialEq, Serialize)]
#[serde(default, rename_all = "camelCase")]
pub struct RealTradeRuntimeRiskEntry {
    pub id: String,
    pub trading_environment: String,
    pub real_trading_enabled: bool,
    pub max_order_quantity: Option<f64>,
    pub max_order_notional: Option<f64>,
    pub operator_id: String,
    pub reason: String,
    pub activated_at: String,
    pub updated_at: String,
}

#[derive(Clone, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[serde(default, rename_all = "camelCase")]
pub struct RealTradeKillSwitchEntry {
    pub id: String,
    pub trading_environment: String,
    pub operator_id: String,
    pub reason: String,
    pub activated_at: String,
    pub updated_at: String,
}

#[derive(Clone, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[serde(default, rename_all = "camelCase")]
pub struct RealTradeHardStopEntry {
    pub id: String,
    pub broker_id: String,
    pub trading_environment: String,
    pub account_id: String,
    pub market: Option<String>,
    pub symbol: Option<String>,
    pub hard_stop_scope: String,
    pub operator_id: String,
    pub reason: String,
    pub activated_at: String,
    pub updated_at: String,
}

#[derive(Clone, Debug, Default, Deserialize, PartialEq, Serialize)]
#[serde(default, rename_all = "camelCase")]
pub struct RealTradeControlEvent {
    pub id: String,
    pub event_type: String,
    pub action: String,
    pub broker_id: String,
    pub operation: Option<String>,
    pub trading_environment: Option<String>,
    pub account_id: Option<String>,
    pub market: Option<String>,
    pub symbol: Option<String>,
    pub order_id: Option<String>,
    pub quantity: Option<f64>,
    pub price: Option<f64>,
    pub kill_switch_source: Option<String>,
    pub hard_stop_scope: Option<String>,
    pub operator_id: Option<String>,
    pub reason: Option<String>,
    pub error_code: Option<String>,
    pub hard_stop_id: Option<String>,
    pub real_trading_enabled: Option<bool>,
    pub configured_max_order_quantity: Option<f64>,
    pub configured_max_order_notional: Option<f64>,
    pub activated_at: Option<String>,
    pub created_at: String,
}

#[derive(Clone, Debug, Default, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct RealTradeRiskSnapshot {
    pub real_trading_enabled: bool,
    pub kill_switch_active: bool,
    pub kill_switch_source: Option<String>,
    pub runtime_kill_switch_active: bool,
    pub control_plane_available: bool,
    pub control_plane_error: Option<String>,
    pub kill_switch_entry: Option<RealTradeKillSwitchEntry>,
    pub kill_switch_events: Vec<RealTradeControlEvent>,
    pub blocked_operations: Vec<String>,
    pub allows_cancel: bool,
    pub hard_stops_active: bool,
    pub hard_stop_entries: Vec<RealTradeHardStopEntry>,
    pub hard_stop_events: Vec<RealTradeControlEvent>,
    pub risk_enabled: bool,
    pub runtime_risk_configured: bool,
    pub runtime_configured_max_order_quantity: Option<f64>,
    pub runtime_configured_max_order_notional: Option<f64>,
    pub effective_max_order_quantity: Option<f64>,
    pub effective_max_order_notional: Option<f64>,
    pub risk_entry: Option<RealTradeRuntimeRiskEntry>,
    pub risk_events: Vec<RealTradeControlEvent>,
}

impl RealTradeRiskSnapshot {
    pub fn from_control_state(
        state: RealTradeControlState,
        unavailable_error: Option<String>,
    ) -> Self {
        let runtime_max_order_quantity = state
            .risk_config
            .as_ref()
            .and_then(|entry| positive_finite(entry.max_order_quantity));
        let runtime_max_order_notional = state
            .risk_config
            .as_ref()
            .and_then(|entry| positive_finite(entry.max_order_notional));
        let unavailable = unavailable_error.is_some();
        let runtime_kill_switch_active = state.kill_switch.is_some() || unavailable;
        let real_trading_enabled = if unavailable {
            true
        } else {
            state
                .risk_config
                .as_ref()
                .is_some_and(|entry| entry.real_trading_enabled)
        };
        let kill_switch_events = events_with_prefix(&state.events, "KILL_SWITCH_");
        let hard_stop_events = events_with_prefix(&state.events, "HARD_STOP_");
        let risk_events = risk_events(&state.events);
        let hard_stops_active = !state.hard_stops.is_empty();
        let runtime_risk_configured = state.risk_config.is_some();
        let risk_enabled =
            runtime_max_order_quantity.is_some() || runtime_max_order_notional.is_some();

        Self {
            real_trading_enabled,
            kill_switch_active: runtime_kill_switch_active,
            kill_switch_source: runtime_kill_switch_active.then(|| "RUNTIME".to_owned()),
            runtime_kill_switch_active,
            control_plane_available: !unavailable,
            control_plane_error: unavailable_error,
            kill_switch_entry: state.kill_switch,
            kill_switch_events,
            blocked_operations: blocked_operations(),
            allows_cancel: true,
            hard_stops_active,
            hard_stop_entries: state.hard_stops,
            hard_stop_events,
            risk_enabled,
            runtime_risk_configured,
            runtime_configured_max_order_quantity: runtime_max_order_quantity,
            runtime_configured_max_order_notional: runtime_max_order_notional,
            effective_max_order_quantity: runtime_max_order_quantity,
            effective_max_order_notional: runtime_max_order_notional,
            risk_entry: state.risk_config,
            risk_events,
        }
    }

    pub fn approvals(&self) -> RealTradeApprovalsResponse {
        RealTradeApprovalsResponse {
            real_trading_enabled: self.real_trading_enabled,
            required_confirmation_text: "ENABLE_REAL_TRADING",
            max_approval_age_ms: 5 * 60 * 1_000,
            approval_workflow_available: false,
            approval_workflow_status: "not_configured",
            approval_workflow_message: "real-trade approval workflow is not configured; runtime risk limits are enforced before broker submission.",
            approval_policy: RealTradeApprovalPolicy::default(),
            entries: Vec::new(),
        }
    }

    pub fn hard_stops(&self) -> RealTradeHardStopsResponse {
        RealTradeHardStopsResponse {
            blocked_operations: blocked_operations(),
            allows_cancel: true,
            entries: self.hard_stop_entries.clone(),
        }
    }

    pub fn hard_stop_events(&self) -> RealTradeHardStopEventsResponse {
        RealTradeHardStopEventsResponse {
            real_trading_enabled: self.real_trading_enabled,
            blocked_operations: blocked_operations(),
            allows_cancel: true,
            entries: self.hard_stop_events.clone(),
        }
    }

    pub fn kill_switch(&self) -> RealTradeKillSwitchStateResponse {
        RealTradeKillSwitchStateResponse {
            real_trading_enabled: self.real_trading_enabled,
            kill_switch_active: self.kill_switch_active,
            kill_switch_source: self.kill_switch_source.clone(),
            runtime_active: self.runtime_kill_switch_active,
            blocked_operations: blocked_operations(),
            allows_cancel: true,
            entry: self.kill_switch_entry.clone(),
        }
    }

    pub fn kill_switch_events(&self) -> RealTradeKillSwitchEventsResponse {
        RealTradeKillSwitchEventsResponse {
            real_trading_enabled: self.real_trading_enabled,
            kill_switch_active: self.kill_switch_active,
            runtime_active: self.runtime_kill_switch_active,
            blocked_operations: blocked_operations(),
            allows_cancel: true,
            entries: self.kill_switch_events.clone(),
        }
    }

    pub fn risk_limits(&self) -> RealTradeRiskLimitsResponse {
        RealTradeRiskLimitsResponse {
            real_trading_enabled: self.real_trading_enabled,
            risk_enabled: self.risk_enabled,
            runtime_risk_configured: self.runtime_risk_configured,
            runtime_configured_max_order_quantity: self.runtime_configured_max_order_quantity,
            runtime_configured_max_order_notional: self.runtime_configured_max_order_notional,
            effective_max_order_quantity: self.effective_max_order_quantity,
            effective_max_order_notional: self.effective_max_order_notional,
            entry: self.risk_entry.clone(),
        }
    }

    pub fn risk_events(&self) -> RealTradeRiskEventsResponse {
        RealTradeRiskEventsResponse {
            real_trading_enabled: self.real_trading_enabled,
            risk_enabled: self.risk_enabled,
            runtime_risk_configured: self.runtime_risk_configured,
            runtime_configured_max_order_quantity: self.runtime_configured_max_order_quantity,
            runtime_configured_max_order_notional: self.runtime_configured_max_order_notional,
            effective_max_order_quantity: self.effective_max_order_quantity,
            effective_max_order_notional: self.effective_max_order_notional,
            max_order_quantity: self.effective_max_order_quantity,
            max_order_notional: self.effective_max_order_notional,
            entries: self.risk_events.clone(),
        }
    }
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct RealTradeApprovalsResponse {
    pub real_trading_enabled: bool,
    pub required_confirmation_text: &'static str,
    pub max_approval_age_ms: i64,
    pub approval_workflow_available: bool,
    pub approval_workflow_status: &'static str,
    pub approval_workflow_message: &'static str,
    pub approval_policy: RealTradeApprovalPolicy,
    pub entries: Vec<()>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct RealTradeApprovalPolicy {
    pub approver_allowlist_enabled: bool,
    pub approver_count: usize,
    pub large_order_notional: Option<f64>,
    pub approval_workflow_available: bool,
    pub approval_mode: &'static str,
}

impl Default for RealTradeApprovalPolicy {
    fn default() -> Self {
        Self {
            approver_allowlist_enabled: false,
            approver_count: 0,
            large_order_notional: None,
            approval_workflow_available: false,
            approval_mode: "none",
        }
    }
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct RealTradeHardStopsResponse {
    pub blocked_operations: Vec<String>,
    pub allows_cancel: bool,
    pub entries: Vec<RealTradeHardStopEntry>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct RealTradeHardStopEventsResponse {
    pub real_trading_enabled: bool,
    pub blocked_operations: Vec<String>,
    pub allows_cancel: bool,
    pub entries: Vec<RealTradeControlEvent>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct RealTradeKillSwitchStateResponse {
    pub real_trading_enabled: bool,
    pub kill_switch_active: bool,
    pub kill_switch_source: Option<String>,
    pub runtime_active: bool,
    pub blocked_operations: Vec<String>,
    pub allows_cancel: bool,
    pub entry: Option<RealTradeKillSwitchEntry>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct RealTradeKillSwitchEventsResponse {
    pub real_trading_enabled: bool,
    pub kill_switch_active: bool,
    pub runtime_active: bool,
    pub blocked_operations: Vec<String>,
    pub allows_cancel: bool,
    pub entries: Vec<RealTradeControlEvent>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct RealTradeRiskLimitsResponse {
    pub real_trading_enabled: bool,
    pub risk_enabled: bool,
    pub runtime_risk_configured: bool,
    pub runtime_configured_max_order_quantity: Option<f64>,
    pub runtime_configured_max_order_notional: Option<f64>,
    pub effective_max_order_quantity: Option<f64>,
    pub effective_max_order_notional: Option<f64>,
    pub entry: Option<RealTradeRuntimeRiskEntry>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct RealTradeRiskEventsResponse {
    pub real_trading_enabled: bool,
    pub risk_enabled: bool,
    pub runtime_risk_configured: bool,
    pub runtime_configured_max_order_quantity: Option<f64>,
    pub runtime_configured_max_order_notional: Option<f64>,
    pub effective_max_order_quantity: Option<f64>,
    pub effective_max_order_notional: Option<f64>,
    pub max_order_quantity: Option<f64>,
    pub max_order_notional: Option<f64>,
    pub entries: Vec<RealTradeControlEvent>,
}

fn blocked_operations() -> Vec<String> {
    BLOCKED_OPERATIONS.map(str::to_owned).to_vec()
}

fn positive_finite(value: Option<f64>) -> Option<f64> {
    value.filter(|value| *value > 0.0 && value.is_finite())
}

fn events_with_prefix(
    events: &[RealTradeControlEvent],
    prefix: &str,
) -> Vec<RealTradeControlEvent> {
    events
        .iter()
        .filter(|event| event.action.to_ascii_uppercase().starts_with(prefix))
        .cloned()
        .collect()
}

fn risk_events(events: &[RealTradeControlEvent]) -> Vec<RealTradeControlEvent> {
    events
        .iter()
        .filter(|event| {
            let action = event.action.to_ascii_uppercase();
            action.starts_with("RISK_CONFIG_") || action.starts_with("RISK_LIMIT_")
        })
        .cloned()
        .collect()
}

#[cfg(test)]
mod tests {
    use super::{RealTradeControlEvent, RealTradeControlState, RealTradeRiskSnapshot};

    #[test]
    fn unavailable_control_plane_fails_closed() {
        let snapshot = RealTradeRiskSnapshot::from_control_state(
            RealTradeControlState::default(),
            Some("decode real-trade control state".to_owned()),
        );
        assert!(snapshot.real_trading_enabled);
        assert!(snapshot.kill_switch_active);
        assert!(snapshot.runtime_kill_switch_active);
        assert_eq!(snapshot.kill_switch_source.as_deref(), Some("RUNTIME"));
        assert!(!snapshot.control_plane_available);
        assert_eq!(snapshot.blocked_operations, ["PLACE", "MODIFY"]);
    }

    #[test]
    fn event_projection_preserves_order_and_filters_case_insensitively() {
        let state = RealTradeControlState {
            events: vec![
                RealTradeControlEvent {
                    id: "risk".to_owned(),
                    action: "risk_limit_reject".to_owned(),
                    ..RealTradeControlEvent::default()
                },
                RealTradeControlEvent {
                    id: "kill".to_owned(),
                    action: "kill_switch_activate".to_owned(),
                    ..RealTradeControlEvent::default()
                },
                RealTradeControlEvent {
                    id: "other".to_owned(),
                    action: "ORDER_PLACE".to_owned(),
                    ..RealTradeControlEvent::default()
                },
            ],
            ..RealTradeControlState::default()
        };
        let snapshot = RealTradeRiskSnapshot::from_control_state(state, None);
        assert_eq!(snapshot.risk_events[0].id, "risk");
        assert_eq!(snapshot.kill_switch_events[0].id, "kill");
        assert!(snapshot.hard_stop_events.is_empty());
    }
}
