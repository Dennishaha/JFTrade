use serde::{Deserialize, Serialize};

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct MarketState {
    pub market: String,
    pub state: i32,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct WireGlobalState {
    pub qot_logged_in: Option<bool>,
    pub trade_logged_in: Option<bool>,
    pub server_version: Option<String>,
    pub program_status: Option<String>,
    pub program_timestamp: Option<String>,
    #[serde(default)]
    pub markets: Vec<MarketState>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OpenDProbe {
    pub connectivity: String,
    pub status: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub issue_code: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub last_error: Option<String>,
    pub quote_logged_in: Option<bool>,
    pub trade_logged_in: Option<bool>,
    pub server_version: Option<String>,
    pub program_status: Option<String>,
    pub program_timestamp: Option<String>,
    pub markets: Vec<MarketState>,
}

impl OpenDProbe {
    pub fn from_global_state(state: Option<WireGlobalState>, version_supported: bool) -> Self {
        let Some(state) = state else {
            return Self::degraded("GetGlobalState returned no server state", None);
        };
        if !version_supported {
            let mut probe = Self::degraded(
                "OpenD version does not meet the minimum requirement",
                Some("OPEND_VERSION_UNSUPPORTED"),
            );
            probe.server_version = state.server_version;
            return probe;
        }
        Self {
            connectivity: "connected".to_owned(),
            status: "healthy".to_owned(),
            issue_code: None,
            last_error: None,
            quote_logged_in: state.qot_logged_in,
            trade_logged_in: state.trade_logged_in,
            server_version: state.server_version,
            program_status: state.program_status,
            program_timestamp: state.program_timestamp,
            markets: state.markets,
        }
    }

    pub fn market_data_ready(&self) -> bool {
        self.connectivity == "connected"
            && self.status == "healthy"
            && self.last_error.is_none()
            && self.quote_logged_in == Some(true)
    }

    pub fn disconnected(message: impl Into<String>) -> Self {
        let mut probe = Self::degraded(message, Some("OPEND_DISCONNECTED"));
        probe.connectivity = "disconnected".to_owned();
        probe.status = "offline".to_owned();
        probe
    }

    fn degraded(message: impl Into<String>, issue_code: Option<&str>) -> Self {
        Self {
            connectivity: "degraded".to_owned(),
            status: "degraded".to_owned(),
            issue_code: issue_code.map(str::to_owned),
            last_error: Some(message.into()),
            quote_logged_in: None,
            trade_logged_in: None,
            server_version: None,
            program_status: None,
            program_timestamp: None,
            markets: Vec::new(),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn probe_opend_reports_closed_port_as_disconnected() {
        // Parity: go:452dea11:internal/integration/futu/probe_test.go:114 TestProbeOpenDReportsClosedPortAsDisconnected
        let disconnected = OpenDProbe::disconnected("connection refused");
        assert_eq!(disconnected.status, "offline");
        assert_eq!(disconnected.connectivity, "disconnected");
        assert!(disconnected.last_error.is_some());
        assert_eq!(
            disconnected.issue_code.as_deref(),
            Some("OPEND_DISCONNECTED")
        );
        assert!(!disconnected.market_data_ready());
    }

    #[test]
    fn probe_from_global_state_enforces_minimum_version_and_maps_neutral_state() {
        // Parity: go:452dea11:internal/integration/futu/probe_test.go:137 TestProbeFromGlobalStateEnforcesMinimumVersionAndMapsNeutralState
        let nil_state = OpenDProbe::from_global_state(None, true);
        assert_eq!(nil_state.status, "degraded");
        assert_eq!(nil_state.connectivity, "degraded");
        assert!(nil_state.last_error.is_some());

        let unsupported = OpenDProbe::from_global_state(
            Some(WireGlobalState {
                qot_logged_in: Some(true),
                trade_logged_in: Some(false),
                server_version: Some("10.8.6708".to_owned()),
                program_status: Some("Ready".to_owned()),
                program_timestamp: None,
                markets: Vec::new(),
            }),
            false,
        );
        assert_eq!(unsupported.status, "degraded");
        assert_eq!(
            unsupported.issue_code.as_deref(),
            Some("OPEND_VERSION_UNSUPPORTED")
        );
        assert_eq!(unsupported.server_version.as_deref(), Some("10.8.6708"));
        assert!(!unsupported.market_data_ready());

        // The healthy neutral state keeps the probe fields Go asserts.
        let healthy = OpenDProbe::from_global_state(
            Some(WireGlobalState {
                qot_logged_in: Some(true),
                trade_logged_in: Some(false),
                server_version: Some("10.9.6908".to_owned()),
                program_status: Some("ProgramStatusType_Ready: all services ready".to_owned()),
                program_timestamp: None,
                markets: Vec::new(),
            }),
            true,
        );
        assert_eq!(healthy.status, "healthy");
        assert_eq!(healthy.connectivity, "connected");
        assert_eq!(healthy.quote_logged_in, Some(true));
        assert_eq!(healthy.trade_logged_in, Some(false));
        assert_eq!(
            healthy.program_status.as_deref(),
            Some("ProgramStatusType_Ready: all services ready")
        );
        assert!(healthy.market_data_ready());
    }

    #[test]
    fn unknown_quote_login_fails_closed() {
        let probe = OpenDProbe::from_global_state(
            Some(WireGlobalState {
                qot_logged_in: None,
                trade_logged_in: Some(true),
                server_version: Some("10.9.100".to_owned()),
                program_status: Some("Ready".to_owned()),
                program_timestamp: None,
                markets: Vec::new(),
            }),
            true,
        );
        assert_eq!(probe.status, "healthy");
        assert!(!probe.market_data_ready());
    }
}
