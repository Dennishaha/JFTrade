use std::fs;
use std::path::{Path, PathBuf};

use jftrade_trading::{RealTradeControlState, RealTradeRiskSnapshot};

pub const REAL_TRADE_CONTROL_PATH_ENV: &str = "JFTRADE_REAL_TRADE_CONTROL_PATH";
const DEFAULT_REAL_TRADE_CONTROL_FILENAME: &str = "real-trade-control.json";

#[derive(Clone, Debug)]
pub struct RealTradeControlReader {
    path: PathBuf,
}

impl RealTradeControlReader {
    pub fn new(path: impl Into<PathBuf>) -> Self {
        Self { path: path.into() }
    }

    pub fn snapshot(&self) -> RealTradeRiskSnapshot {
        match self.read_state() {
            Ok(state) => RealTradeRiskSnapshot::from_control_state(state, None),
            Err(error) => RealTradeRiskSnapshot::from_control_state(
                RealTradeControlState::default(),
                Some(error),
            ),
        }
    }

    fn read_state(&self) -> Result<RealTradeControlState, String> {
        let contents = match fs::read(&self.path) {
            Ok(contents) => contents,
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => {
                return Ok(RealTradeControlState::default());
            }
            Err(error) => return Err(format!("read real-trade control state: {error}")),
        };
        if contents.iter().all(u8::is_ascii_whitespace) {
            return Ok(RealTradeControlState::default());
        }
        serde_json::from_slice(&contents)
            .map_err(|error| format!("decode real-trade control state: {error}"))
    }
}

#[allow(dead_code)]
pub fn load_state(path: &Path) -> Result<RealTradeControlState, String> {
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

pub fn load_state_strict(path: &Path) -> Result<RealTradeControlState, String> {
    let bytes =
        fs::read(path).map_err(|error| format!("read real-trade control state: {error}"))?;
    if bytes.iter().all(u8::is_ascii_whitespace) {
        return Err("real-trade control state file is empty".to_owned());
    }
    serde_json::from_slice(&bytes)
        .map_err(|error| format!("decode real-trade control state: {error}"))
}

pub fn ensure_default_state_file(path: &Path) -> Result<(), String> {
    if !path.exists() {
        persist_state(path, &RealTradeControlState::default())?;
    }
    Ok(())
}

pub fn persist_state(path: &Path, state: &RealTradeControlState) -> Result<(), String> {
    use std::io::Write;
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
        .map_err(|error| format!("flush real-trade control file: {error}"))?;
    file.persist(path)
        .map_err(|error| format!("persist real-trade control state: {error}"))?;
    Ok(())
}

pub fn derive_real_trade_control_path(settings_path: &Path) -> PathBuf {
    settings_path
        .parent()
        .filter(|parent| !parent.as_os_str().is_empty())
        .map_or_else(
            || PathBuf::from(DEFAULT_REAL_TRADE_CONTROL_FILENAME),
            |parent| parent.join(DEFAULT_REAL_TRADE_CONTROL_FILENAME),
        )
}

#[cfg(test)]
mod tests {
    use std::fs;

    use serde_json::json;
    use tempfile::tempdir;

    use super::{RealTradeControlReader, derive_real_trade_control_path};

    fn wire_keys(value: &serde_json::Value) -> Vec<String> {
        let mut keys = value
            .as_object()
            .expect("wire object")
            .keys()
            .cloned()
            .collect::<Vec<_>>();
        keys.sort();
        keys
    }

    fn expected_keys(keys: &[&str]) -> Vec<String> {
        let mut keys = keys.iter().map(|key| (*key).to_owned()).collect::<Vec<_>>();
        keys.sort();
        keys
    }

    // Parity: go:452dea11:internal/app/apiserver/servercore/notification_market_workflow_contracts_test.go:140 TestRealTradeControlPathPrefersExplicitOverride
    #[test]
    fn path_is_sibling_of_settings_file() {
        assert_eq!(
            derive_real_trade_control_path(std::path::Path::new("/var/lib/jftrade/settings.json")),
            std::path::Path::new("/var/lib/jftrade/real-trade-control.json")
        );
        assert_eq!(
            derive_real_trade_control_path(std::path::Path::new("settings.json")),
            std::path::Path::new("real-trade-control.json")
        );
    }

    #[test]
    fn reader_refreshes_state_and_fails_closed_on_decode_error() {
        let directory = tempdir().expect("temporary directory");
        let path = directory.path().join("real-trade-control.json");
        let reader = RealTradeControlReader::new(&path);
        assert!(!reader.snapshot().real_trading_enabled);

        fs::write(
            &path,
            br#"{"riskConfig":{"realTradingEnabled":true,"maxOrderQuantity":12.5}}"#,
        )
        .expect("write control state");
        let active = reader.snapshot();
        assert!(active.real_trading_enabled);
        assert_eq!(
            active.effective_max_order_quantity,
            Some(jftrade_kernel::Decimal::new(125, 1))
        );

        fs::write(&path, b"{").expect("write malformed state");
        let unavailable = reader.snapshot();
        assert!(unavailable.real_trading_enabled);
        assert!(unavailable.kill_switch_active);
        assert!(!unavailable.control_plane_available);
    }

    #[derive(serde::Deserialize)]
    struct RealTradeCorpus {
        version: String,
        cases: Vec<RealTradeCase>,
    }

    #[derive(serde::Deserialize)]
    #[serde(rename_all = "camelCase")]
    struct RealTradeCase {
        name: String,
        document: Option<String>,
        expected_unavailable: bool,
    }

    // Parity: go:452dea11:internal/system/service_test.go:83 TestRealTradeDefaultsMatchFrontendContract
    #[test]
    fn default_control_reads_match_the_frontend_contract() {
        let directory = tempdir().expect("temporary directory");
        let snapshot =
            RealTradeControlReader::new(directory.path().join("real-trade-control.json"))
                .snapshot();

        let approvals = serde_json::to_value(snapshot.approvals()).expect("approvals wire value");
        assert_eq!(
            wire_keys(&approvals),
            expected_keys(&[
                "approvalWorkflowAvailable",
                "approvalWorkflowMessage",
                "approvalWorkflowStatus",
                "approvalPolicy",
                "entries",
                "maxApprovalAgeMs",
                "realTradingEnabled",
                "requiredConfirmationText",
            ])
        );
        assert_eq!(approvals["realTradingEnabled"], false);
        assert_eq!(approvals["requiredConfirmationText"], "ENABLE_REAL_TRADING");
        assert_eq!(approvals["maxApprovalAgeMs"], 5 * 60 * 1_000);
        assert_eq!(approvals["approvalWorkflowAvailable"], false);
        assert_eq!(approvals["approvalWorkflowStatus"], "not_configured");
        assert!(
            approvals["approvalWorkflowMessage"]
                .as_str()
                .is_some_and(|message| !message.is_empty()),
            "approval workflow message: {approvals}"
        );
        assert_eq!(approvals["entries"], json!([]));
        assert_eq!(
            approvals["approvalPolicy"]["approverAllowlistEnabled"],
            false
        );
        assert_eq!(approvals["approvalPolicy"]["approverCount"], 0);
        assert_eq!(
            approvals["approvalPolicy"]["approvalWorkflowAvailable"],
            false
        );
        assert_eq!(approvals["approvalPolicy"]["approvalMode"], "none");
        assert!(approvals["approvalPolicy"]["largeOrderNotional"].is_null());

        let kill_switch =
            serde_json::to_value(snapshot.kill_switch()).expect("kill switch wire value");
        assert_eq!(
            wire_keys(&kill_switch),
            expected_keys(&[
                "allowsCancel",
                "blockedOperations",
                "entry",
                "killSwitchActive",
                "killSwitchSource",
                "realTradingEnabled",
                "runtimeActive",
            ])
        );
        assert_eq!(kill_switch["killSwitchActive"], false);
        assert_eq!(kill_switch["allowsCancel"], true);
        assert!(kill_switch["killSwitchSource"].is_null());
        assert!(kill_switch["entry"].is_null());

        let risk_limits =
            serde_json::to_value(snapshot.risk_limits()).expect("risk limits wire value");
        assert_eq!(
            wire_keys(&risk_limits),
            expected_keys(&[
                "effectiveMaxOrderQuantity",
                "effectiveMaxOrderNotional",
                "entry",
                "realTradingEnabled",
                "riskEnabled",
                "runtimeConfiguredMaxOrderNotional",
                "runtimeConfiguredMaxOrderQuantity",
                "runtimeRiskConfigured",
            ])
        );
        assert_eq!(risk_limits["riskEnabled"], false);
        assert!(risk_limits["entry"].is_null());

        let risk_events =
            serde_json::to_value(snapshot.risk_events()).expect("risk events wire value");
        assert_eq!(
            wire_keys(&risk_events),
            expected_keys(&[
                "effectiveMaxOrderQuantity",
                "effectiveMaxOrderNotional",
                "entries",
                "maxOrderNotional",
                "maxOrderQuantity",
                "realTradingEnabled",
                "riskEnabled",
                "runtimeConfiguredMaxOrderNotional",
                "runtimeConfiguredMaxOrderQuantity",
                "runtimeRiskConfigured",
            ])
        );
        assert_eq!(risk_events["riskEnabled"], false);
        assert_eq!(risk_events["entries"], json!([]));
    }

    // Parity: go:452dea11:internal/system/service_test.go:201 TestRealTradeStateNormalizesTypedNilSlices
    #[test]
    fn empty_control_state_serializes_empty_entry_slices() {
        let directory = tempdir().expect("temporary directory");
        let path = directory.path().join("real-trade-control.json");
        // The Go snapshot may carry typed-nil slices; every projection must
        // still answer with a JSON array instead of `null`.
        fs::write(&path, br#"{"riskConfig":null,"killSwitch":null}"#)
            .expect("seed empty control state");
        let snapshot = RealTradeControlReader::new(&path).snapshot();
        assert!(snapshot.control_plane_available);

        let hard_stops =
            serde_json::to_value(snapshot.hard_stops()).expect("hard stops wire value");
        assert_eq!(hard_stops["entries"], json!([]));
        assert_eq!(hard_stops["allowsCancel"], true);
        let hard_stop_events =
            serde_json::to_value(snapshot.hard_stop_events()).expect("hard stop events wire value");
        assert_eq!(hard_stop_events["entries"], json!([]));
        let kill_switch_events = serde_json::to_value(snapshot.kill_switch_events())
            .expect("kill switch events wire value");
        assert_eq!(kill_switch_events["entries"], json!([]));
        let risk_events =
            serde_json::to_value(snapshot.risk_events()).expect("risk events wire value");
        assert_eq!(risk_events["entries"], json!([]));

        let missing_reader = RealTradeControlReader::new(directory.path().join("absent.json"));
        let missing = missing_reader.snapshot();
        assert_eq!(missing.hard_stop_entries, Vec::new());
        assert_eq!(missing.risk_events, Vec::new());
    }

    #[test]
    fn real_trade_reads_replay_frozen_compatibility_cases() {
        let corpus: RealTradeCorpus = serde_json::from_str(include_str!(
            "../../../tests/fixtures/compatibility/api-transport/real-trade-control-corpus.json"
        ))
        .expect("real-trade corpus");
        assert_eq!(corpus.version, "stage9.real-trade-read.v1");
        assert!(corpus.cases.len() >= 5);

        let directory = tempdir().expect("temporary directory");
        let mut results = Vec::with_capacity(corpus.cases.len());
        for (index, test_case) in corpus.cases.iter().enumerate() {
            let path = directory.path().join(format!("case-{index}.json"));
            if let Some(document) = &test_case.document {
                fs::write(&path, document).expect("seed real-trade corpus case");
            }
            let snapshot = RealTradeControlReader::new(path).snapshot();
            assert_eq!(
                !snapshot.control_plane_available, test_case.expected_unavailable,
                "{} availability",
                test_case.name
            );
            results.push(json!({
                "name": test_case.name,
                "controlPlaneAvailable": snapshot.control_plane_available,
                "status": {
                    "realTradingEnabled": snapshot.real_trading_enabled,
                    "realTradingKillSwitch": {
                        "active": snapshot.kill_switch_active,
                        "runtimeActive": snapshot.runtime_kill_switch_active,
                        "blockedOperations": snapshot.blocked_operations,
                        "allowsCancel": snapshot.allows_cancel,
                    },
                    "realTradingRisk": {
                        "enabled": snapshot.risk_enabled,
                        "maxOrderQuantity": snapshot.effective_max_order_quantity,
                        "maxOrderNotional": snapshot.effective_max_order_notional,
                        "runtimeConfiguredMaxOrderQuantity": snapshot.runtime_configured_max_order_quantity,
                        "runtimeConfiguredMaxOrderNotional": snapshot.runtime_configured_max_order_notional,
                        "runtimeRiskConfigured": snapshot.runtime_risk_configured,
                    }
                },
                "approvals": snapshot.approvals(),
                "hardStops": snapshot.hard_stops(),
                "hardStopEvents": snapshot.hard_stop_events(),
                "killSwitch": snapshot.kill_switch(),
                "killSwitchEvents": snapshot.kill_switch_events(),
                "riskLimits": snapshot.risk_limits(),
                "riskEvents": snapshot.risk_events(),
            }));
        }

        assert_eq!(results.len(), corpus.cases.len());
    }
}
