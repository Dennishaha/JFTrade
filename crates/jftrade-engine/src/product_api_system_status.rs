impl ProductApi {
    fn system_status(&self) -> ApiOutput {
        let uptime = duration_millis(self.started.elapsed());
        let requests = self.metrics.request_observability_snapshot();
        let live = live_observability(&self.live_connections);
        let market_data =
            market_data_runtime_projection(self.market_data_runtime_status_port.as_deref());
        let strategy_runtime =
            strategy_runtime_projection(self.strategy_runtime_status_port.as_deref());
        let runtime = self.runtime.snapshot();
        let real_trade = self.real_trade_control.snapshot();
        let checked_at = SystemClock.now_rfc3339();
        let settings_path = runtime
            .resources
            .iter()
            .find(|resource| resource.id == "settings-file")
            .map(|resource| resource.path.as_str())
            .unwrap_or_default();
        let broker = json!(jftrade_integration_futu::broker_descriptor());
        let exchange_calendars = self
            .calendar_manager
            .as_ref()
            .and_then(|manager| manager.status_snapshot().ok())
            .map(|status| json!(status));
        let persistence = if runtime.production {
            match self.production_persistence_projection(settings_path, &checked_at) {
                Ok(persistence) => persistence,
                Err(failure) => return production_failure_output(failure),
            }
        } else {
            json!({
                "engine": "json",
                "databasePath": settings_path,
                "status": "ok",
                "migrated": true,
                "pendingMigrations": [],
                "tables": ["broker_integrations", "broker_accounts"],
                "checkedAt": checked_at,
            })
        };
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
                "engine": persistence["engine"].clone(),
                "databasePath": persistence["databasePath"].clone(),
                "status": persistence["status"].clone(),
                "migrated": persistence["migrated"].clone(),
                "pendingMigrations": persistence["pendingMigrations"].clone(),
                "tables": persistence["tables"].clone(),
                "checkedAt": persistence["checkedAt"].clone()
            },
            "observability": {
                "api": { "startedAt": self.started_at, "uptimeMs": uptime },
                "live": live,
                "marketdata": market_data,
                "exchangeCalendars": exchange_calendars,
                "broker": broker,
                "strategyRuntime": strategy_runtime,
                "requests": requests
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

    fn production_persistence_projection(
        &self,
        settings_path: &str,
        checked_at: &str,
    ) -> Result<Value, ApiFailure> {
        let ports = self.production_ports.as_ref().ok_or_else(|| {
            ApiFailure::new(
                503,
                "PERSISTENCE_UNAVAILABLE",
                "production persistence bundle is not configured",
            )
        })?;
        let overview = self
            .settings
            .data_management
            .overview(
                OverviewRequest {
                    summary_only: true,
                    database_id: String::new(),
                },
                checked_at.to_owned(),
            )
            .map_err(|error| {
                ApiFailure::new(
                    500,
                    "PERSISTENCE_OVERVIEW_FAILED",
                    error.to_string(),
                )
            })?;
        let pending_migrations = overview
            .databases
            .iter()
            .filter(|database| database.status != "ready")
            .map(|database| database.descriptor.id.clone())
            .collect::<Vec<_>>();
        let tables = overview
            .databases
            .iter()
            .map(|database| database.descriptor.id.clone())
            .collect::<Vec<_>>();
        let all_ready = ports.database_leases.status == "acquired"
            && overview
                .databases
                .iter()
                .all(|database| database.status == "ready");
        let status = if all_ready {
            "ok"
        } else if ports.database_leases.status == "partial"
            || overview
                .databases
                .iter()
                .any(|database| database.status == "ready")
        {
            "degraded"
        } else {
            "unavailable"
        };
        Ok(json!({
            "engine": "sqlite",
            "databasePath": settings_path,
            "status": status,
            "migrated": all_ready,
            "pendingMigrations": pending_migrations,
            "tables": tables,
            "checkedAt": checked_at,
        }))
    }
}

fn production_failure_output(failure: ApiFailure) -> ApiOutput {
    let body = serde_json::to_vec(&json!({
        "ok": false,
        "error": {
            "code": failure.code,
            "message": failure.message,
        },
        "timestamp": SystemClock.now_rfc3339(),
    }))
    .unwrap_or_else(|_| b"{\"ok\":false}".to_vec());
    ApiOutput::Raw {
        status: failure.status,
        content_type: "application/json".to_owned(),
        body,
        headers: std::collections::BTreeMap::new(),
    }
}

fn live_observability(metrics: &LiveConnectionMetrics) -> Value {
    let snapshot = metrics.snapshot();
    json!({
        "connected": snapshot.connected,
        "limit": snapshot.limit,
        "atLimit": snapshot.at_limit,
        "activeInstruments": snapshot.active_instruments,
    })
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
