//! PineTS shadow projection helpers for the production MCP executor.
//!
//! The native Pine specification/validation leaves remain deterministic and
//! in-process.  When the explicitly configured external mode is enabled,
//! this module projects the verified PineTS worker result without changing the
//! MCP or worker wire contracts.

use std::path::{Path, PathBuf};
use std::sync::Arc;

use serde_json::{Value, json};

use super::super::product_production_ports::ProductionPortBundle;
use super::super::strategy_pine::{StrategyPineAnalyzeInput, StrategyPineAnalyzeSnapshotError};

pub(super) const PINE_MODE_OFF: &str = "off";
pub(super) const PINE_MODE_SHADOW: &str = "shadow";
pub(super) const PINE_MODE_COMMUNITY_AGPL: &str = "community-agpl";
const PINE_SHADOW_ENGINE: &str = "pinets-shadow";
const PINE_SHADOW_LICENSE: &str = "AGPL-3.0-only";
const PINE_SHADOW_REPOSITORY: &str = "https://github.com/LuxAlgo/PineTS";

pub(super) fn pine_external_mode_value(value: Option<&str>) -> &'static str {
    match value {
        Some(value) if value.trim().eq_ignore_ascii_case(PINE_MODE_SHADOW) => PINE_MODE_SHADOW,
        Some(value) if value.trim().eq_ignore_ascii_case(PINE_MODE_COMMUNITY_AGPL) => {
            PINE_MODE_COMMUNITY_AGPL
        }
        _ => PINE_MODE_OFF,
    }
}

pub(super) fn pine_external_mode() -> &'static str {
    pine_external_mode_value(std::env::var("JFTRADE_PINETS_MODE").ok().as_deref())
}

pub(super) fn third_party_notice_available() -> bool {
    let repository_notice = Path::new("docs/legal/third-party-notices.md");
    let crate_notice =
        PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../../docs/legal/third-party-notices.md");
    repository_notice.is_file() || crate_notice.is_file()
}

fn pine_compliance_payload() -> Value {
    json!({
        "license": PINE_SHADOW_LICENSE,
        "commercialLicense": false,
        "sourceOffer": "docs/legal/third-party-notices.md",
        "networkUseNotice": "If PineTS functionality is exposed over a network, provide corresponding source and license notices for the AGPL-covered integration."
    })
}

pub(super) fn pine_shadow_base_payload(mode: &str, status: &str) -> Value {
    json!({
        "enabled": true,
        "mode": mode,
        "engine": PINE_SHADOW_ENGINE,
        "engineVersion": "",
        "license": "",
        "repository": PINE_SHADOW_REPOSITORY,
        "ok": false,
        "status": status,
        "diagnostics": [],
        "differenceSummary": {"evaluated": false},
        "compliance": pine_compliance_payload()
    })
}

pub(super) fn pine_shadow_error_payload(mode: &str, message: String) -> Value {
    let mut payload = pine_shadow_base_payload(mode, "shadow_error");
    payload["diagnostics"] = json!([{
        "severity": "error",
        "code": "PINETS_SHADOW_ERROR",
        "message": message,
        "line": 1,
        "column": 1,
        "endLine": 1,
        "endColumn": 1
    }]);
    payload
}

pub(super) fn pine_compliance_error_payload(mode: &str) -> Value {
    let mut payload = pine_shadow_base_payload(mode, "compliance_error");
    payload["license"] = json!("");
    payload["repository"] = json!("");
    payload["diagnostics"] = json!([{
        "severity": "error",
        "code": "PINETS_AGPL_NOTICE_MISSING",
        "message": "community-agpl mode requires docs/legal/third-party-notices.md to expose source and license obligations",
        "line": 1,
        "column": 1,
        "endLine": 1,
        "endColumn": 1
    }]);
    payload["differenceSummary"] = json!({
        "evaluated": false,
        "reason": "AGPL notice/source-offer file is missing"
    });
    payload
}

pub(super) fn pine_shadow_success_payload(mode: &str, result: Value) -> Value {
    let mut payload = pine_shadow_base_payload(mode, "shadow_ok");
    let metadata = result.get("metadata").and_then(Value::as_object);
    let engine_version = metadata
        .and_then(|metadata| metadata.get("pineTsVersion"))
        .and_then(Value::as_str)
        .or_else(|| result.get("engineVersion").and_then(Value::as_str))
        .unwrap_or_default();
    payload["engineVersion"] = json!(engine_version);
    payload["license"] = json!(PINE_SHADOW_LICENSE);
    payload["ok"] = json!(result.get("ok").and_then(Value::as_bool).unwrap_or(false));
    payload["diagnostics"] = pine_diagnostics(result.get("diagnostics"));
    payload["differenceSummary"] = json!({
        "evaluated": true,
        "plots": value_count(result.get("plots")),
        "signals": value_count(result.get("signals")),
        "authority": "pine-pinets production runtime remains authoritative"
    });
    payload
}

pub(super) fn pine_external_engine_payload(
    ports: Option<&Arc<ProductionPortBundle>>,
    mode: &str,
    script: &str,
    notice_available: bool,
) -> Result<Value, String> {
    if mode == PINE_MODE_COMMUNITY_AGPL && !notice_available {
        return Ok(pine_compliance_error_payload(mode));
    }
    let input = StrategyPineAnalyzeInput {
        script: script.to_owned(),
        source_format: "pine-v6".to_owned(),
        include_ast: false,
    };
    let result = ports
        .map(|ports| ports.strategy_pine_analyze.evaluate_shadow(&input))
        .unwrap_or_else(|| {
            Err(StrategyPineAnalyzeSnapshotError::Unavailable(
                "pine analyzer is not configured".to_owned(),
            ))
        });
    match result {
        Ok(result) => Ok(pine_shadow_success_payload(mode, result)),
        Err(error) => Err(error.message().to_owned()),
    }
}

fn pine_diagnostics(value: Option<&Value>) -> Value {
    let Some(items) = value.and_then(Value::as_array) else {
        return json!([]);
    };
    Value::Array(
        items
            .iter()
            .map(|item| {
                let Some(object) = item.as_object() else {
                    return json!({
                        "severity": "error",
                        "code": "PINETS_SHADOW_ERROR",
                        "message": item.to_string(),
                        "line": 0,
                        "column": 0,
                        "endLine": 0,
                        "endColumn": 0
                    });
                };
                let line = object.get("line").cloned().unwrap_or_else(|| json!(0));
                let column = object
                    .get("column")
                    .cloned()
                    .unwrap_or_else(|| json!(0));
                json!({
                    "severity": object.get("severity").cloned().unwrap_or_else(|| json!("error")),
                    "code": object.get("code").cloned().unwrap_or_else(|| json!("PINETS_SHADOW_ERROR")),
                    "message": object.get("message").cloned().unwrap_or_else(|| json!("pine shadow engine diagnostic")),
                    "line": line,
                    "column": column,
                    "endLine": object.get("endLine").cloned().unwrap_or_else(|| line.clone()),
                    "endColumn": object.get("endColumn").cloned().unwrap_or_else(|| column.clone())
                })
            })
            .collect(),
    )
}

fn value_count(value: Option<&Value>) -> usize {
    match value {
        Some(Value::Array(items)) => items.len(),
        Some(Value::Object(items)) => items.len(),
        _ => 0,
    }
}
