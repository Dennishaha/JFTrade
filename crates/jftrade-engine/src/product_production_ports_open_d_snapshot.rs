//! Production OpenD system-health projection.
//!
//! Split out of `product_production_ports_system.rs` so the Go-parity
//! diagnosis logic (typed issue code, manual retry, restart guidance and the
//! version-upgrade summary) stays reviewable and the port file stays inside
//! the bounded production-file budget.

use jftrade_settings::{BrokerSettingsStorePort, FutuOpenDInstallSettingsStorePort};
use serde_json::{Value, json};

use super::ProductionSystemPort;
use crate::product::{ProductionRuntimeStatus, SystemReadSnapshotError};

use super::provider_now_rfc3339;

/// Project `/api/v1/system/futu-opend` for an enabled Futu integration.
///
/// The projection mirrors the Go `Coordinator.OpenDHealth` branch selection:
/// the probe's typed issue code wins over the generic connectivity code, any
/// last error requires a manual retry, and only an untyped dial/refused error
/// recommends restarting OpenD.  A version rejection additionally names the
/// detected build and the minimum supported version in the diagnosis summary.
pub(crate) fn project(
    port: &ProductionSystemPort,
) -> Result<Value, SystemReadSnapshotError> {
    let quote_ready = port.active_provider_state.snapshot().opend_ready;
    let broker_inputs = port.settings.load_broker_settings_inputs().ok();
    let futu_enabled = broker_inputs
        .as_ref()
        .and_then(|inputs| inputs.saved_integration.as_ref())
        .is_some_and(|int| int.enabled);
    if !futu_enabled && !quote_ready && port.runtime_status.is_none() {
        return Ok(json!({
            "status": "unavailable",
            "reason": "broker integration not enabled",
        }));
    }
    let settings = port
        .settings
        .load_futu_open_d_install_settings()
        .map_err(|error| SystemReadSnapshotError::Unavailable(error.to_string()))?
        .ok_or_else(|| {
            SystemReadSnapshotError::Unavailable(
                "Futu OpenD settings are not configured".to_owned(),
            )
        })?;
    let state = port.runtime_status.as_ref().map(|port| port.snapshot()).unwrap_or_default();
    let has_error = state
        .quote_last_error
        .as_ref()
        .is_some_and(|error| !error.is_empty())
        || state
            .stream_last_error
            .as_ref()
            .is_some_and(|error| !error.is_empty());
    let last_error = state
        .quote_last_error
        .clone()
        .filter(|error| !error.is_empty())
        .or(state
            .stream_last_error
            .clone()
            .filter(|error| !error.is_empty()));

    let host = settings.host.trim();
    let api_port = settings.api_port;
    let socket_addr = format!("{host}:{api_port}")
        .parse::<std::net::SocketAddr>()
        .ok()
        .or_else(|| {
            use std::net::ToSocketAddrs;
            (host, api_port as u16)
                .to_socket_addrs()
                .ok()
                .and_then(|mut it| it.next())
        });
    let probe_result = socket_addr.map(|addr| {
        let config = jftrade_integration_futu::OpenDTcpProbeConfig::new(
            addr,
            std::time::Duration::from_millis(1500),
        );
        jftrade_integration_futu::OpenDTcpProbe::probe(config)
    });

    let (
        connectivity,
        status,
        server_version,
        program_status,
        quote_logged_in,
        diag_code,
        manual_retry,
        restart_recommended,
        effective_last_error,
    ) = if let Some(Ok(probe)) = probe_result {
        if probe.status == "healthy" {
            let current = port.active_provider_state.snapshot();
            if !current.opend_ready {
                port.active_provider_state.set_readiness(
                    current.helper_ready,
                    true,
                    current.router_ready,
                );
            }
        }
        // Go keeps the probe's own diagnosis: a typed issue code wins over
        // the generic connectivity code, any last error demands a manual
        // retry, and only an untyped dial/refused error asks for an OpenD
        // restart (a version rejection must not).
        let probe_error = probe
            .last_error
            .as_deref()
            .filter(|error| !error.trim().is_empty());
        let probe_code = match (probe.issue_code.as_deref(), probe_error) {
            (Some(code), _) => code.to_owned(),
            (None, Some(_)) => "OPEND_API_CONNECTIVITY".to_owned(),
            (None, None) => "NONE".to_owned(),
        };
        let probe_restart = probe.issue_code.is_none()
            && probe_error.is_some_and(|error| {
                let lower = error.to_lowercase();
                lower.contains("dial") || lower.contains("connection refused")
            });
        (
            probe.connectivity,
            probe.status,
            probe.server_version.map(Value::String).unwrap_or(Value::Null),
            probe.program_status.map(Value::String).unwrap_or(Value::Null),
            probe.quote_logged_in,
            probe_code,
            probe_error.is_some(),
            probe_restart,
            probe.last_error,
        )
    } else if quote_ready || state.connected {
        (
            "connected".to_owned(), "healthy".to_owned(),
            Value::Null, Value::Null, Some(true), "NONE".to_owned(), false, false, last_error,
        )
    } else {
        // Go distinguishes "could not dial" (offline/disconnected) from
        // "dialed but the protocol exchange failed" (degraded/degraded).
        // The managed-session error keeps that distinction for rejection,
        // decode and missing-state failures; a bare I/O error stays on the
        // dial-failure path.
        let probe_failure = match probe_result.as_ref() {
            Some(Err(error)) => Some(error),
            _ => None,
        };
        let protocol_degraded = probe_failure.is_some_and(|error| {
            matches!(
                error,
                jftrade_integration_futu::OpenDTcpProbeError::Rejected { .. }
                    | jftrade_integration_futu::OpenDTcpProbeError::Decode { .. }
                    | jftrade_integration_futu::OpenDTcpProbeError::MissingInitState
                    | jftrade_integration_futu::OpenDTcpProbeError::MissingGlobalState
                    | jftrade_integration_futu::OpenDTcpProbeError::UnsupportedVersion { .. }
                    | jftrade_integration_futu::OpenDTcpProbeError::Session(
                        jftrade_integration_futu::OpenDManagedSessionError::Closed(_)
                    )
            )
        });
        let err = last_error
            .or_else(|| probe_failure.map(|error| error.to_string()));
        let restart_candidate = err.as_ref().is_some_and(|e| {
            let l = e.to_lowercase();
            l.contains("dial")
                || l.contains("connection refused")
                || l.contains("os { code: 61")
        });
        let degraded = has_error
            || protocol_degraded
            || port.opend_status == ProductionRuntimeStatus::Degraded;
        let status = if degraded { "degraded" } else { "offline" };
        let code = if err.is_some() {
            "OPEND_API_CONNECTIVITY".to_owned()
        } else {
            "OPEND_UNAVAILABLE".to_owned()
        };
        (
            if has_error || protocol_degraded {
                "degraded".to_owned()
            } else {
                "disconnected".to_owned()
            },
            status.to_owned(),
            Value::Null,
            Value::Null,
            None,
            code,
            true,
            restart_candidate,
            err,
        )
    };
    // Go reports the minimum-version rejection as a summary naming both the
    // detected build and the requirement, so the console can point at the
    // upgrade target.  Typed issue codes keep the probe's own text.
    let diag_summary = match (&effective_last_error, diag_code.as_str()) {
        (Some(_), "OPEND_VERSION_UNSUPPORTED") => Some(Value::String(format!(
            "OpenD {} is below the minimum supported version {}; upgrade OpenD and retry",
            server_version.as_str().unwrap_or("unknown"),
            jftrade_integration_futu::MINIMUM_OPEND_VERSION
        ))),
        (Some(error), _) => Some(Value::String(error.clone())),
        (None, _) => None,
    };
    let trade_logged_in = port.trade_runtime.as_ref().and_then(|r| r.snapshot().trade_logged_in);

    let live_snapshot = port.live_hub.as_ref().map(|hub| hub.snapshot());
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
            "quoteLoggedIn": quote_logged_in,
            "tradeLoggedIn": trade_logged_in,
            "programStatus": program_status,
            "serverVersion": server_version,
            "minimumVersion": jftrade_integration_futu::MINIMUM_OPEND_VERSION,
            "lastError": effective_last_error,
        },
        "diagnosis": {
            "code": diag_code,
            "summary": diag_summary,
            "manualRetryRequired": manual_retry,
            "restartOpenDRecommended": restart_recommended,
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
