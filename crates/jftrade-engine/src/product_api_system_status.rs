impl ProductApi {
    fn system_status(&self) -> ApiOutput {
        let uptime = duration_millis(self.started.elapsed());
        let runtime = self.runtime.snapshot();
        let real_trade = self.real_trade_control.snapshot();
        let checked_at = SystemClock.now_rfc3339();
        let settings_path = runtime
            .resources
            .iter()
            .find(|resource| resource.id == "settings-file")
            .map(|resource| resource.path.as_str())
            .unwrap_or_default();
        let helper_ready = runtime.helper_state
            == Some(jftrade_integration_marketdata_helper::ProcessState::Ready);
        let helper_failed = runtime.helper_state
            == Some(jftrade_integration_marketdata_helper::ProcessState::Failed)
            || runtime.last_error.is_some();
        let runtime_error = runtime.last_error.as_deref();
        let broker = json!(jftrade_integration_futu::broker_descriptor());
        let strategy_runtime = json!({
            "status": "idle",
            "activeStrategies": 0,
            "supportsBacktestParity": true,
            "activeInstances": [],
        });
        let exchange_calendars = self
            .calendar_manager
            .as_ref()
            .and_then(|manager| manager.status_snapshot().ok())
            .map(|status| json!(status));
        ApiOutput::Json(json!({
            "name": "JFTrade",
            "apiPort": self.api_port,
            "defaultBroker": "futu",
            "defaultTradingEnvironment": "SIMULATE",
            "realTradingEnabled": real_trade.real_trading_enabled,
            "realTradingKillSwitch": {
                "active": real_trade.kill_switch_active,
                "runtimeActive": real_trade.runtime_kill_switch_active,
                "blockedOperations": real_trade.blocked_operations,
                "allowsCancel": real_trade.allows_cancel
            },
            "realTradingRisk": {
                "enabled": real_trade.risk_enabled,
                "maxOrderQuantity": real_trade.effective_max_order_quantity,
                "maxOrderNotional": real_trade.effective_max_order_notional,
                "runtimeConfiguredMaxOrderQuantity": real_trade.runtime_configured_max_order_quantity,
                "runtimeConfiguredMaxOrderNotional": real_trade.runtime_configured_max_order_notional,
                "runtimeRiskConfigured": real_trade.runtime_risk_configured
            },
            "realTradeAccess": {
                "approverAllowlistEnabled": false,
                "approverCount": 0,
                "adminAllowlistEnabled": false,
                "adminCount": 0
            },
            "build": {
                "version": option_env!("JFTRADE_BUILD_VERSION").unwrap_or("dev"),
                "commit": option_env!("JFTRADE_BUILD_COMMIT").unwrap_or("unknown"),
                "buildTime": option_env!("JFTRADE_BUILD_TIME").unwrap_or("dev"),
                "goos": go_compatible_os(),
                "goarch": go_compatible_arch()
            },
            "persistence": {
                "engine": "json",
                "databasePath": settings_path,
                "status": "ok",
                "migrated": true,
                "pendingMigrations": [],
                "tables": ["broker_integrations", "broker_accounts"],
                "checkedAt": checked_at
            },
            "observability": {
                "api": { "startedAt": self.started_at, "uptimeMs": uptime },
                "live": { "connected": 0, "limit": 100, "atLimit": false, "activeInstruments": [] },
                "marketdata": {
                    "status": if helper_failed { "degraded" } else if helper_ready { "connected" } else { "idle" },
                    "connected": helper_ready, "closed": false,
                    "generation": 0, "activeCount": 0, "lastRefreshAt": null,
                    "quoteRetryAt": null, "quoteFailures": 0, "quoteLastError": runtime_error,
                    "streamRetryAt": null, "streamFailures": 0, "streamLastError": null
                },
                "exchangeCalendars": exchange_calendars,
                "broker": broker,
                "strategyRuntime": strategy_runtime,
                "requests": {
                    "recentErrors": [],
                    "recentSlowRequests": [],
                    "openD": { "totalCalls": 0, "failedCalls": 0 },
                    "slowThresholdMs": 750,
                    "minimumImportance": "low"
                }
            },
            "runtimeResources": {
                "checkedAt": checked_at,
                "count": runtime.resources.len(),
                "items": runtime.resources
            },
            "broker": broker,
            "strategyRuntime": strategy_runtime,
            "message": "JFTrade API adapter is running."
        }))
    }
}

fn go_compatible_os() -> &'static str {
    match std::env::consts::OS {
        "macos" => "darwin",
        other => other,
    }
}

fn go_compatible_arch() -> &'static str {
    match std::env::consts::ARCH {
        "aarch64" => "arm64",
        "x86_64" => "amd64",
        other => other,
    }
}
