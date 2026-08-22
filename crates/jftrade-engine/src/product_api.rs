struct ProductApi {
    api_port: u16,
    settings: ProductSettingsServices,
    metrics: Arc<TransportMetrics>,
    started_at: String,
    started: Instant,
    runtime: Arc<ProductRuntimeState>,
    real_trade_control: RealTradeControlReader,
    notification_port: Option<Arc<dyn ProductNotificationPort>>,
    calendar_manager: Option<Arc<CalendarManager>>,
    watchlist_membership_snapshot_port: Option<Arc<dyn WatchlistMembershipSnapshotPort>>,
    watchlist_read_snapshot_port: Option<Arc<dyn WatchlistReadSnapshotPort>>,
    portfolio_snapshot_port: Option<Arc<dyn PortfolioSnapshotPort>>,
    research_read_snapshot_port: Option<Arc<dyn ResearchReadSnapshotPort>>,
    research_preset_read_snapshot_port: Option<Arc<dyn ResearchPresetReadSnapshotPort>>,
    execution_read_snapshot_port: Option<Arc<dyn ExecutionReadSnapshotPort>>,
    market_data_provider_read_snapshot_port: Option<Arc<dyn MarketDataProviderReadSnapshotPort>>,
    market_data_catalog_read_snapshot_port: Option<Arc<dyn MarketDataCatalogReadSnapshotPort>>,
    market_data_derivative_read_snapshot_port:
        Option<Arc<dyn MarketDataDerivativeReadSnapshotPort>>,
    market_data_options_read_snapshot_port: Option<Arc<dyn MarketDataOptionsReadSnapshotPort>>,
    market_data_news_actions_read_snapshot_port:
        Option<Arc<dyn MarketDataNewsActionsReadSnapshotPort>>,
    broker_read_snapshot_port: Option<Arc<dyn BrokerReadSnapshotPort>>,
    system_read_snapshot_port: Option<Arc<dyn SystemReadSnapshotPort>>,
    remote_watchlist_snapshot_port: Option<Arc<dyn RemoteWatchlistSnapshotPort>>,
    plugin_uninstall_guidance_snapshot_port: Option<Arc<dyn PluginUninstallGuidanceSnapshotPort>>,
    plugin_snapshot_port: Option<Arc<dyn PluginSnapshotPort>>,
    alert_snapshot_port: Option<Arc<dyn AlertSnapshotPort>>,
    strategy_definition_snapshot_port: Option<Arc<dyn StrategyDefinitionSnapshotPort>>,
    backtest_read_snapshot_port: Option<Arc<dyn BacktestReadSnapshotPort>>,
    backtest_sync_read_snapshot_port: Option<Arc<dyn BacktestSyncReadSnapshotPort>>,
    strategy_read_snapshot_port: Option<Arc<dyn StrategyReadSnapshotPort>>,
    auth_session_snapshot_port: Option<Arc<dyn AuthSessionSnapshotPort>>,
    notification_sequence: AtomicU64,
    capabilities: ProductCapabilities,
}
struct ProductOptionalPorts {
    notification: Option<Arc<dyn ProductNotificationPort>>,
    calendar_manager: Option<Arc<CalendarManager>>,
    watchlist_membership_snapshot: Option<Arc<dyn WatchlistMembershipSnapshotPort>>,
    watchlist_read_snapshot: Option<Arc<dyn WatchlistReadSnapshotPort>>,
    portfolio_snapshot: Option<Arc<dyn PortfolioSnapshotPort>>,
    research_read_snapshot: Option<Arc<dyn ResearchReadSnapshotPort>>,
    research_preset_read_snapshot: Option<Arc<dyn ResearchPresetReadSnapshotPort>>,
    execution_read_snapshot: Option<Arc<dyn ExecutionReadSnapshotPort>>,
    market_data_provider_read_snapshot: Option<Arc<dyn MarketDataProviderReadSnapshotPort>>,
    market_data_catalog_read_snapshot: Option<Arc<dyn MarketDataCatalogReadSnapshotPort>>,
    market_data_derivative_read_snapshot: Option<Arc<dyn MarketDataDerivativeReadSnapshotPort>>,
    market_data_options_read_snapshot: Option<Arc<dyn MarketDataOptionsReadSnapshotPort>>,
    market_data_news_actions_read_snapshot: Option<Arc<dyn MarketDataNewsActionsReadSnapshotPort>>,
    broker_read_snapshot: Option<Arc<dyn BrokerReadSnapshotPort>>,
    system_read_snapshot: Option<Arc<dyn SystemReadSnapshotPort>>,
    remote_watchlist_snapshot: Option<Arc<dyn RemoteWatchlistSnapshotPort>>,
    plugin_uninstall_guidance_snapshot: Option<Arc<dyn PluginUninstallGuidanceSnapshotPort>>,
    plugin_snapshot: Option<Arc<dyn PluginSnapshotPort>>,
    alert_snapshot: Option<Arc<dyn AlertSnapshotPort>>,
    strategy_definition_snapshot: Option<Arc<dyn StrategyDefinitionSnapshotPort>>,
    backtest_read_snapshot: Option<Arc<dyn BacktestReadSnapshotPort>>,
    backtest_sync_read_snapshot: Option<Arc<dyn BacktestSyncReadSnapshotPort>>,
    strategy_read_snapshot: Option<Arc<dyn StrategyReadSnapshotPort>>,
    auth_session_snapshot: Option<Arc<dyn AuthSessionSnapshotPort>>,
}
struct ProductSettingsServices {
    appearance: AppearanceService,
    brokers: BrokerSettingsService,
    onboarding: OnboardingSettingsService,
    futu_install: FutuOpenDInstallSettingsService,
    execution: ExecutionService,
    assistant_runtime: AssistantRuntimeService,
    system_notifications: SystemNotificationService,
    pine_worker: PineWorkerSettingsService,
    security: SecuritySettingsService,
    market_data_provider: MarketDataProviderSettingsService,
    backtest_market_data_provider: BacktestMarketDataProviderSettingsService,
    mcp_server: McpServerSettingsService,
    exchange_calendars: ExchangeCalendarSettingsService,
    data_management: OverviewService,
    cleanup_preview: Arc<CleanupPreviewService>,
    maintenance: MaintenanceService,
}
impl ProductApi {
    fn new(
        api_port: u16,
        settings: ProductSettingsServices,
        metrics: Arc<TransportMetrics>,
        runtime: Arc<ProductRuntimeState>,
        real_trade_control: RealTradeControlReader,
        optional_ports: ProductOptionalPorts,
        capabilities: ProductCapabilities,
    ) -> Self {
        Self {
            api_port,
            settings,
            metrics,
            started_at: SystemClock.now_rfc3339(),
            started: Instant::now(),
            runtime,
            real_trade_control,
            notification_port: optional_ports.notification,
            calendar_manager: optional_ports.calendar_manager,
            watchlist_membership_snapshot_port: optional_ports.watchlist_membership_snapshot,
            watchlist_read_snapshot_port: optional_ports.watchlist_read_snapshot,
            portfolio_snapshot_port: optional_ports.portfolio_snapshot,
            research_read_snapshot_port: optional_ports.research_read_snapshot,
            research_preset_read_snapshot_port: optional_ports.research_preset_read_snapshot,
            execution_read_snapshot_port: optional_ports.execution_read_snapshot,
            market_data_provider_read_snapshot_port: optional_ports.market_data_provider_read_snapshot,
            market_data_catalog_read_snapshot_port: optional_ports.market_data_catalog_read_snapshot,
            market_data_derivative_read_snapshot_port: optional_ports.market_data_derivative_read_snapshot,
            market_data_options_read_snapshot_port: optional_ports.market_data_options_read_snapshot,
            market_data_news_actions_read_snapshot_port: optional_ports
                .market_data_news_actions_read_snapshot,
            broker_read_snapshot_port: optional_ports.broker_read_snapshot,
            system_read_snapshot_port: optional_ports.system_read_snapshot,
            remote_watchlist_snapshot_port: optional_ports.remote_watchlist_snapshot,
            plugin_uninstall_guidance_snapshot_port: optional_ports
                .plugin_uninstall_guidance_snapshot,
            plugin_snapshot_port: optional_ports.plugin_snapshot,
            alert_snapshot_port: optional_ports.alert_snapshot,
            strategy_definition_snapshot_port: optional_ports.strategy_definition_snapshot,
            backtest_read_snapshot_port: optional_ports.backtest_read_snapshot,
            backtest_sync_read_snapshot_port: optional_ports.backtest_sync_read_snapshot,
            strategy_read_snapshot_port: optional_ports.strategy_read_snapshot,
            auth_session_snapshot_port: optional_ports.auth_session_snapshot,
            notification_sequence: AtomicU64::new(0),
            capabilities,
        }
    }
    fn system_status(&self) -> ApiOutput {
        let requests = self.metrics.snapshot();
        let uptime = duration_millis(self.started.elapsed());
        let runtime = self.runtime.snapshot();
        let real_trade = self.real_trade_control.snapshot();
        let helper_ready = runtime.helper_state
            == Some(jftrade_integration_marketdata_helper::ProcessState::Ready);
        let message = runtime_message(&runtime);
        let checked_at = SystemClock.now_rfc3339();
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
                "version": env!("CARGO_PKG_VERSION"),
                "commit": option_env!("JFTRADE_BUILD_COMMIT").unwrap_or("rust-development"),
                "buildTime": option_env!("JFTRADE_BUILD_TIME").unwrap_or("development"),
                "goos": std::env::consts::OS,
                "goarch": std::env::consts::ARCH
            },
            "persistence": {
                "engine": "rust-settings-file",
                "databasePath": "",
                "status": "partial",
                "migrated": false,
                "pendingMigrations": ["remaining capability stores"],
                "tables": [],
                "checkedAt": self.started_at
            },
            "observability": {
                "api": { "startedAt": self.started_at, "uptimeMs": uptime },
                "live": { "connected": 0, "limit": 100, "atLimit": false, "activeInstruments": [] },
                "marketdata": {
                    "status": if helper_ready { "helper-ready" } else { "not-owned" },
                    "connected": helper_ready, "closed": !helper_ready,
                    "generation": 0, "activeCount": 0, "lastRefreshAt": null,
                    "quoteRetryAt": null, "quoteFailures": 0, "quoteLastError": null,
                    "streamRetryAt": null, "streamFailures": 0, "streamLastError": null
                },
                "exchangeCalendars": null,
                "broker": null,
                "strategyRuntime": null,
                "requests": {
                    "started": requests.started,
                    "completed": requests.completed,
                    "failures": requests.failures,
                    "inFlight": requests.in_flight
                }
            },
            "runtimeResources": {
                "checkedAt": checked_at,
                "count": runtime.resources.len(),
                "items": runtime.resources
            },
            "broker": null,
            "strategyRuntime": { "activeStrategies": 0, "activeInstances": [] },
            "message": message,
            "migrationOwner": if self.capabilities.is_empty() { "read-only-shadow" } else { "cutover" }
        }))
    }
    fn appearance(&self) -> Result<ApiOutput, ApiFailure> {
        self.settings
            .appearance
            .appearance()
            .map(|appearance| ApiOutput::Json(json!({ "appearance": appearance })))
            .map_err(settings_failure)
    }
    fn broker_settings(&self) -> Result<ApiOutput, ApiFailure> {
        let inputs = self
            .settings
            .brokers
            .inputs()
            .map_err(settings_read_failure)?;
        Ok(ApiOutput::Json(broker_settings_wire(inputs)))
    }
    fn save_broker_integration(&self, body: &[u8]) -> Result<ApiOutput, ApiFailure> {
        let input: BrokerIntegration = serde_json::from_slice(body)
            .map_err(|_| ApiFailure::new(400, "BAD_REQUEST", "invalid integration payload"))?;
        self.settings
            .brokers
            .save_integration(&input, &SystemClock.now_rfc3339())
            .map(|integration| ApiOutput::Json(json!(integration)))
            .map_err(broker_settings_failure)
    }
    fn create_managed_broker_account(&self, body: &[u8]) -> Result<ApiOutput, ApiFailure> {
        let input: ManagedBrokerAccount = serde_json::from_slice(body)
            .map_err(|_| ApiFailure::new(400, "BAD_REQUEST", "invalid account payload"))?;
        self.settings
            .brokers
            .create_account(&input, &SystemClock.now_rfc3339())
            .map(|account| ApiOutput::Json(json!(account)))
            .map_err(broker_settings_failure)
    }
    fn update_managed_broker_account(
        &self,
        id: &str,
        body: &[u8],
    ) -> Result<ApiOutput, ApiFailure> {
        let input: ManagedBrokerAccount = serde_json::from_slice(body)
            .map_err(|_| ApiFailure::new(400, "BAD_REQUEST", "invalid account payload"))?;
        self.settings
            .brokers
            .update_account(id, &input, &SystemClock.now_rfc3339())
            .map(|account| ApiOutput::Json(json!(account)))
            .map_err(broker_settings_failure)
    }
    fn delete_managed_broker_account(&self, id: &str) -> Result<ApiOutput, ApiFailure> {
        self.settings
            .brokers
            .delete_account(id)
            .map(|()| ApiOutput::Json(json!({"deleted": true, "id": id})))
            .map_err(broker_settings_failure)
    }
    async fn onboarding(&self) -> Result<ApiOutput, ApiFailure> {
        let dependencies =
            runtime_dependencies::inspect(SystemClock.now_rfc3339(), self.runtime.node_runtime())
                .await;
        let readiness = self
            .settings
            .onboarding
            .readiness(dependencies.all_required_satisfied)
            .map_err(settings_read_failure)?;
        Ok(ApiOutput::Json(json!({
            "state": readiness.state,
            "shouldShowOobe": readiness.should_show_oobe,
            "reasons": readiness.reasons,
            "recommendedBrokerId": "futu",
            "brokers": [{
                "descriptor": jftrade_integration_futu::broker_descriptor(),
                "enabled": readiness.broker_enabled,
                "available": true,
                "configured": readiness.broker_configured,
            }]
        })))
    }
    async fn save_onboarding(&self, body: &[u8]) -> Result<ApiOutput, ApiFailure> {
        let request: OnboardingWriteRequest = serde_json::from_slice(body)
            .map_err(|_| ApiFailure::new(400, "BAD_REQUEST", "invalid onboarding payload"))?;
        self.settings
            .onboarding
            .save(&request, &SystemClock.now_rfc3339())
            .map_err(settings_failure)?;
        self.onboarding().await
    }
    fn save_appearance(&self, body: &[u8]) -> Result<ApiOutput, ApiFailure> {
        let payload: AppearanceWriteRequest = serde_json::from_slice(body)
            .map_err(|_| ApiFailure::new(400, "BAD_REQUEST", "invalid appearance payload"))?;
        self.settings
            .appearance
            .save_appearance(&payload.appearance)
            .map(|appearance| ApiOutput::Json(json!({ "appearance": appearance })))
            .map_err(settings_failure)
    }
    async fn runtime_dependencies(&self) -> ApiOutput {
        let dependencies =
            runtime_dependencies::inspect(SystemClock.now_rfc3339(), self.runtime.node_runtime())
                .await;
        ApiOutput::Json(
            serde_json::to_value(dependencies)
                .expect("runtime dependency projection must be serializable"),
        )
    }
    fn futu_open_d_install_guide(&self) -> Result<ApiOutput, ApiFailure> {
        let settings = self
            .settings
            .futu_install
            .settings()
            .map_err(settings_read_failure)?;
        Ok(ApiOutput::Json(json!({
            "brokerId": "futu",
            "title": "Futu OpenD",
            "description": "Configure Futu OpenD. Current market data reaches OpenD through the bbgo exchange adapter and the native API port; WebSocket settings remain available for compatibility and future push-stream support.",
            "options": [],
            "nextSteps": [
                format!("安装或升级至 Futu OpenD {} 或更高版本。", jftrade_integration_futu::MINIMUM_OPEND_VERSION),
                "确认 OpenD 已登录，并先保证 API Port 可从本机访问。",
                "保存 Host 和 API Port；WebSocket Port / Key 目前主要用于兼容配置与诊断。",
                "保存后刷新 OpenD 健康状态，确认 API 侧连接正常。"
            ],
            "settings": {
                "host": settings.host,
                "apiPort": settings.api_port,
                "websocketPort": settings.websocket_port,
                "maxWebSocketConnections": settings.max_websocket_connections,
                "useEncryption": settings.use_encryption,
                "websocketKeyRequired": settings.websocket_key_required,
                "marketDataTransport": "bbgo-opend-tcp-api",
                "minimumVersion": jftrade_integration_futu::MINIMUM_OPEND_VERSION,
            }
        })))
    }
    fn storage_overview(&self) -> ApiOutput {
        ApiOutput::Json(json!({
            "pendingOutbox": [],
            "recentJobs": [],
            "recentAuditLogs": [],
            "recentExecutionCommands": [],
        }))
    }
    fn database_overview(&self, query: &str) -> Result<ApiOutput, ApiFailure> {
        let request = parse_database_overview_query(query);
        self.settings
            .data_management
            .overview(request, SystemClock.now_rfc3339())
            .map(|response| ApiOutput::Json(json!(response)))
            .map_err(database_overview_failure)
    }
    fn research_screen_catalog(&self, query: &str) -> Result<ApiOutput, ApiFailure> {
        let (broker_id, market) = parse_research_screen_catalog_query(query);
        jftrade_research::screen_catalog(&broker_id, &market)
            .map(ApiOutput::Json)
            .map_err(research_screen_catalog_failure)
    }
    fn calendar_source_snapshot(&self) -> Result<ApiOutput, ApiFailure> {
        let manager = self.calendar_manager.as_ref().ok_or_else(|| {
            ApiFailure::new(
                503,
                "EXCHANGE_CALENDAR_SOURCES_UNAVAILABLE",
                "exchange calendar manager is not configured",
            )
        })?;
        let snapshot = manager.sources_snapshot().map_err(|error| {
            ApiFailure::new(
                503,
                "EXCHANGE_CALENDAR_SOURCES_UNAVAILABLE",
                error.to_string(),
            )
        })?;
        Ok(ApiOutput::Json(json!({ "sources": snapshot.sources })))
    }
    fn calendar_status_snapshot(&self) -> Result<ApiOutput, ApiFailure> {
        let manager = self.calendar_manager.as_ref().ok_or_else(|| {
            ApiFailure::new(
                503,
                "EXCHANGE_CALENDAR_STATUS_UNAVAILABLE",
                "exchange calendar manager is not configured",
            )
        })?;
        let snapshot = manager.status_snapshot().map_err(|error| {
            ApiFailure::new(
                503,
                "EXCHANGE_CALENDAR_STATUS_UNAVAILABLE",
                error.to_string(),
            )
        })?;
        Ok(ApiOutput::Json(json!(snapshot)))
    }
    fn calendar_refresh(&self, path: &str) -> Result<ApiOutput, ApiFailure> {
        let manager = self.calendar_manager.as_ref().ok_or_else(|| {
            ApiFailure::new(
                503,
                "EXCHANGE_CALENDAR_REFRESH_UNAVAILABLE",
                "exchange calendar manager is not configured",
            )
        })?;
        let result = calendar_market_from_path(path, "/refresh/")
            .map_or_else(|| manager.refresh_all(), |market| manager.refresh_market(market))
            .map_err(|error| {
                ApiFailure::new(
                    503,
                    "EXCHANGE_CALENDAR_REFRESH_UNAVAILABLE",
                    error.to_string(),
                )
            })?;
        Ok(ApiOutput::Json(json!(result)))
    }
    fn calendar_probe(&self, path: &str) -> Result<ApiOutput, ApiFailure> {
        let manager = self.calendar_manager.as_ref().ok_or_else(|| {
            ApiFailure::new(
                503,
                "EXCHANGE_CALENDAR_PROBE_UNAVAILABLE",
                "exchange calendar manager is not configured",
            )
        })?;
        let result = calendar_market_from_path(path, "/probe/")
            .map_or_else(|| manager.probe_all(), |market| manager.probe_market(market))
            .map_err(|error| {
                ApiFailure::new(
                    503,
                    "EXCHANGE_CALENDAR_PROBE_UNAVAILABLE",
                    error.to_string(),
                )
            })?;
        Ok(ApiOutput::Json(json!(result)))
    }
    fn alerts(&self, kind: AlertKind, query: &str) -> Result<ApiOutput, ApiFailure> {
        let port = self.alert_snapshot_port.as_ref().ok_or_else(|| {
            ApiFailure::new(
                503,
                "ALERTS_UNAVAILABLE",
                "alert snapshot port is not configured",
            )
        })?;
        port.snapshot(kind, query)
            .map(ApiOutput::Json)
            .map_err(alert_snapshot_failure)
    }
    fn strategy_definition_detail(
        &self,
        path: &str,
        query: &str,
    ) -> Result<ApiOutput, ApiFailure> {
        let definition_id = strategy_definition_id(path)?;
        let preview = parse_strategy_definition_preview(query)?;
        let port = self.strategy_definition_port()?;
        let definition = port
            .get(&definition_id, &preview)
            .map_err(strategy_definition_snapshot_failure)?
            .ok_or_else(|| ApiFailure::new(404, "NOT_FOUND", "resource not found"))?;
        Ok(ApiOutput::Json(definition))
    }
    fn strategy_definition_versions(&self, path: &str) -> Result<ApiOutput, ApiFailure> {
        let definition_id = strategy_definition_versions_id(path)?;
        let port = self.strategy_definition_port()?;
        let versions = port
            .versions(&definition_id)
            .map_err(strategy_definition_snapshot_failure)?
            .ok_or_else(|| ApiFailure::new(404, "NOT_FOUND", "resource not found"))?;
        Ok(ApiOutput::Json(json!(versions)))
    }

    fn strategy_definition_version(&self, path: &str) -> Result<ApiOutput, ApiFailure> {
        let (definition_id, version) = strategy_definition_version_path(path)?;
        let port = self.strategy_definition_port()?;
        let snapshot = port
            .version(&definition_id, &version)
            .map_err(strategy_definition_snapshot_failure)?
            .ok_or_else(|| ApiFailure::new(404, "NOT_FOUND", "resource not found"))?;
        Ok(ApiOutput::Json(snapshot))
    }

    fn cleanup_preview(&self, body: &[u8]) -> Result<ApiOutput, ApiFailure> {
        let request: CleanupPreviewRequest = serde_json::from_slice(body)
            .map_err(|_| ApiFailure::new(400, "BAD_REQUEST", "invalid cleanup preview payload"))?;
        self.settings
            .cleanup_preview
            .preview(request)
            .map(|response| ApiOutput::Json(json!(response)))
            .map_err(cleanup_preview_failure)
    }

    fn cleanup_execute(&self, body: &[u8]) -> Result<ApiOutput, ApiFailure> {
        let request: CleanupExecuteRequest = serde_json::from_slice(body)
            .map_err(|_| ApiFailure::new(400, "BAD_REQUEST", "invalid cleanup payload"))?;
        self.settings
            .maintenance
            .execute_cleanup(request)
            .map(|response| ApiOutput::Json(json!(response)))
            .map_err(|error| maintenance_failure(error, "DATABASE_CLEANUP_FAILED"))
    }

    fn database_compact(&self, path: &str, body: &[u8]) -> Result<ApiOutput, ApiFailure> {
        let database_id = data_management_database_id(path, "/compact")?;
        let request: CompactRequest = serde_json::from_slice(body)
            .map_err(|_| ApiFailure::new(400, "BAD_REQUEST", "invalid compaction payload"))?;
        self.settings
            .maintenance
            .compact(&database_id, request, &SystemClock.now_rfc3339())
            .map(|response| ApiOutput::Json(json!(response)))
            .map_err(|error| maintenance_failure(error, "DATABASE_COMPACT_FAILED"))
    }

    fn database_backup(&self, path: &str, body: &[u8]) -> Result<ApiOutput, ApiFailure> {
        let database_id = data_management_database_id(path, "/backup")?;
        let request: BackupRequest = serde_json::from_slice(body)
            .map_err(|_| ApiFailure::new(400, "BAD_REQUEST", "invalid backup payload"))?;
        self.settings
            .maintenance
            .backup(&database_id, request, &SystemClock.now_rfc3339())
            .map(|response| ApiOutput::Json(json!(response)))
            .map_err(|error| maintenance_failure(error, "DATABASE_BACKUP_FAILED"))
    }

    fn database_rebuild(&self, body: &[u8]) -> Result<ApiOutput, ApiFailure> {
        let request: RebuildRequest = serde_json::from_slice(body)
            .map_err(|_| ApiFailure::new(400, "BAD_REQUEST", "invalid database rebuild payload"))?;
        self.settings
            .maintenance
            .rebuild(request, &SystemClock.now_rfc3339())
            .map(|response| ApiOutput::Json(json!(response)))
            .map_err(|error| maintenance_failure(error, "DATABASE_REBUILD_REJECTED"))
    }

    fn real_trade_approvals(&self) -> ApiOutput {
        ApiOutput::Json(json!(self.real_trade_control.snapshot().approvals()))
    }

    fn real_trade_hard_stops(&self) -> ApiOutput {
        ApiOutput::Json(json!(self.real_trade_control.snapshot().hard_stops()))
    }

    fn real_trade_hard_stop_events(&self) -> ApiOutput {
        ApiOutput::Json(json!(self.real_trade_control.snapshot().hard_stop_events()))
    }

    fn real_trade_kill_switch(&self) -> ApiOutput {
        ApiOutput::Json(json!(self.real_trade_control.snapshot().kill_switch()))
    }

    fn real_trade_kill_switch_events(&self) -> ApiOutput {
        ApiOutput::Json(json!(
            self.real_trade_control.snapshot().kill_switch_events()
        ))
    }

    fn real_trade_risk_limits(&self) -> ApiOutput {
        ApiOutput::Json(json!(self.real_trade_control.snapshot().risk_limits()))
    }

    fn real_trade_risk_events(&self) -> ApiOutput {
        ApiOutput::Json(json!(self.real_trade_control.snapshot().risk_events()))
    }

    fn execution_settings(&self) -> Result<ApiOutput, ApiFailure> {
        self.settings
            .execution
            .settings()
            .map(|settings| ApiOutput::Json(json!(settings)))
            .map_err(settings_read_failure)
    }

    fn save_execution_settings(&self, body: &[u8]) -> Result<ApiOutput, ApiFailure> {
        let input: ExecutionSettings = serde_json::from_slice(body)
            .map_err(|_| ApiFailure::new(400, "BAD_REQUEST", "invalid execution payload"))?;
        self.settings
            .execution
            .save(&input)
            .map(|settings| ApiOutput::Json(json!(settings)))
            .map_err(settings_failure)
    }

    fn assistant_runtime_settings(&self) -> Result<ApiOutput, ApiFailure> {
        self.settings
            .assistant_runtime
            .settings()
            .map(|settings| ApiOutput::Json(json!(settings)))
            .map_err(settings_read_failure)
    }

    fn mcp_server_settings(&self) -> Result<ApiOutput, ApiFailure> {
        self.settings
            .mcp_server
            .stopped_snapshot()
            .map(|snapshot| ApiOutput::Json(json!(snapshot)))
            .map_err(mcp_server_read_failure)
    }

    fn save_mcp_server_settings(&self, body: &[u8]) -> Result<ApiOutput, ApiFailure> {
        let input: McpServerSettingsUpdate = serde_json::from_slice(body)
            .map_err(|_| ApiFailure::new(400, "BAD_REQUEST", "invalid MCP server payload"))?;
        self.settings
            .mcp_server
            .save(&input)
            .map_err(mcp_server_save_failure)?;
        self.mcp_server_settings()
    }

    fn reset_mcp_server_token(&self) -> Result<ApiOutput, ApiFailure> {
        self.settings
            .mcp_server
            .reset_token()
            .map(|result| ApiOutput::Json(json!(result)))
            .map_err(mcp_server_token_reset_failure)
    }

    fn save_assistant_runtime_settings(&self, body: &[u8]) -> Result<ApiOutput, ApiFailure> {
        let input: AssistantRuntimeSettings = serde_json::from_slice(body)
            .map_err(|_| ApiFailure::new(400, "BAD_REQUEST", "invalid adk payload"))?;
        self.settings
            .assistant_runtime
            .save(&input)
            .map(|settings| ApiOutput::Json(json!(settings)))
            .map_err(settings_failure)
    }

    fn system_notification_settings(&self) -> Result<ApiOutput, ApiFailure> {
        self.settings
            .system_notifications
            .settings()
            .map(|settings| ApiOutput::Json(json!(settings)))
            .map_err(settings_read_failure)
    }

    fn save_system_notification_settings(&self, body: &[u8]) -> Result<ApiOutput, ApiFailure> {
        let input: SystemNotificationSettings = serde_json::from_slice(body).map_err(|_| {
            ApiFailure::new(400, "BAD_REQUEST", "invalid system notification payload")
        })?;
        self.settings
            .system_notifications
            .save(&input)
            .map(|settings| ApiOutput::Json(json!(settings)))
            .map_err(settings_failure)
    }

    fn test_system_notification(&self) -> Result<ApiOutput, ApiFailure> {
        let port = self.notification_port.as_ref().ok_or_else(|| {
            ApiFailure::new(
                500,
                "SYSTEM_NOTIFICATION_TEST_FAILED",
                "desktop system notifications are not available",
            )
        })?;
        let settings = self
            .settings
            .system_notifications
            .settings()
            .map_err(settings_read_failure)?;
        let sequence = self.notification_sequence.fetch_add(1, Ordering::SeqCst) + 1;
        let level = "warn";
        let category = "system.notification.test";
        let delivery = if should_forward_system_notification(&settings, level, category) {
            port.deliver(ProductNotificationRequest {
                title: "JFTrade 系统通知测试".to_owned(),
                body: "系统通知通道已连接。".to_owned(),
                sound_enabled: settings.sound_enabled,
            })
        } else {
            ProductNotificationDelivery {
                delivered: false,
                status: "filtered".to_owned(),
                message: "notification filtered by desktop settings".to_owned(),
            }
        };
        Ok(ApiOutput::Json(json!({
            "event": {
                "type": "system.notification",
                "id": format!("system-notification-{sequence}"),
                "at": SystemClock.now_rfc3339(),
                "level": level,
                "title": "JFTrade 系统通知测试",
                "message": "系统通知通道已连接。",
                "source": "desktop",
                "brokerId": "",
                "category": category
            },
            "delivery": delivery
        })))
    }

    fn pine_worker_settings(&self) -> Result<ApiOutput, ApiFailure> {
        self.settings
            .pine_worker
            .settings()
            .map(|settings| ApiOutput::Json(json!(settings)))
            .map_err(settings_read_failure)
    }

    fn security_settings(&self) -> Result<ApiOutput, ApiFailure> {
        self.settings
            .security
            .settings()
            .map(|settings| ApiOutput::Json(json!(settings)))
            .map_err(security_settings_read_failure)
    }

    fn save_security_settings(
        &self,
        body: &[u8],
        desktop_trusted: bool,
    ) -> Result<ApiOutput, ApiFailure> {
        if !desktop_trusted {
            return Err(ApiFailure::new(
                403,
                "WEB_ACCESS_SETTINGS_DESKTOP_ONLY",
                "Web access settings can only be changed from the JFTrade desktop app",
            ));
        }
        let input: SecuritySettingsUpdate = serde_json::from_slice(body)
            .map_err(|_| ApiFailure::new(400, "BAD_REQUEST", "invalid security payload"))?;
        self.settings
            .security
            .save(&input)
            .map(|settings| ApiOutput::Json(json!(settings)))
            .map_err(security_settings_save_failure)
    }

    fn active_market_data_provider(&self) -> Result<ApiOutput, ApiFailure> {
        self.settings
            .market_data_provider
            .active_provider()
            .map(|active_provider| ApiOutput::Json(json!({ "activeProvider": active_provider })))
            .map_err(settings_read_failure)
    }

    fn backtest_market_data_provider(&self) -> Result<ApiOutput, ApiFailure> {
        let active_provider = self
            .settings
            .backtest_market_data_provider
            .active_provider()
            .map_err(settings_read_failure)?;
        let mut descriptors = vec![jftrade_integration_futu::provider_descriptor()];
        descriptors.extend(jftrade_integration_marketdata_helper::provider_descriptors());
        let available_providers = descriptors
            .into_iter()
            .map(provider_descriptor_wire)
            .collect::<Vec<_>>();
        Ok(ApiOutput::Json(json!({
            "activeProvider": active_provider,
            "availableProviders": available_providers,
        })))
    }

    fn save_active_market_data_provider(&self, body: &[u8]) -> Result<ApiOutput, ApiFailure> {
        let payload: MarketDataProviderWriteRequest =
            serde_json::from_slice(body).map_err(|_| {
                ApiFailure::new(400, "BAD_REQUEST", "invalid market-data provider payload")
            })?;
        self.settings
            .market_data_provider
            .save(&payload.active_provider)
            .map(|active_provider| ApiOutput::Json(json!({"activeProvider": active_provider})))
            .map_err(|error| market_data_provider_failure(error, "MARKET_DATA_PROVIDER_INVALID"))
    }

    fn save_backtest_market_data_provider(&self, body: &[u8]) -> Result<ApiOutput, ApiFailure> {
        let payload: MarketDataProviderWriteRequest =
            serde_json::from_slice(body).map_err(|_| {
                ApiFailure::new(400, "BAD_REQUEST", "invalid market-data provider payload")
            })?;
        self.settings
            .backtest_market_data_provider
            .save(&payload.active_provider)
            .map_err(|error| {
                market_data_provider_failure(error, "BACKTEST_MARKET_DATA_PROVIDER_INVALID")
            })?;
        self.backtest_market_data_provider()
    }

    fn exchange_calendar_settings(&self) -> Result<ApiOutput, ApiFailure> {
        self.settings
            .exchange_calendars
            .settings()
            .map(|settings| ApiOutput::Json(json!({ "exchangeCalendars": settings })))
            .map_err(settings_read_failure)
    }

    fn save_exchange_calendar_settings(&self, body: &[u8]) -> Result<ApiOutput, ApiFailure> {
        let payload: ExchangeCalendarWriteRequest = serde_json::from_slice(body).map_err(|_| {
            ApiFailure::new(400, "BAD_REQUEST", "invalid exchange calendar payload")
        })?;
        self.settings
            .exchange_calendars
            .save(payload.exchange_calendars.into())
            .map(|settings| ApiOutput::Json(json!({"exchangeCalendars": settings})))
            .map_err(settings_failure)
    }

}
