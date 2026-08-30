include!("product_api_types.rs");
struct ProductApi {
    api_port: u16,
    production_routes:
        Option<Arc<crate::product::product_production_route_registry::ProductionRouteRegistry>>,
    production_ports:
        Option<Arc<crate::product::product_production_ports::ProductionPortBundle>>,
    settings: ProductSettingsServices,
    metrics: Arc<TransportMetrics>,
    live_connections: Arc<LiveConnectionMetrics>,
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
    market_data_news_search_read_snapshot_port:
        Option<Arc<dyn MarketDataNewsSearchReadSnapshotPort>>,
    adk_read_snapshot_port: Option<Arc<dyn AdkReadSnapshotPort>>,
    market_data_quote_read_snapshot_port: Option<Arc<dyn MarketDataQuoteReadSnapshotPort>>,
    market_data_prediction_read_api: MarketDataPredictionReadApi,
    market_data_runtime_status_port: Option<Arc<dyn MarketDataRuntimeStatusPort>>,
    broker_read_snapshot_port: Option<Arc<dyn BrokerReadSnapshotPort>>,
    system_read_snapshot_port: Option<Arc<dyn SystemReadSnapshotPort>>,
    remote_watchlist_snapshot_port: Option<Arc<dyn RemoteWatchlistSnapshotPort>>,
    remote_watchlist_write_port: Option<Arc<dyn RemoteWatchlistWritePort>>,
    watchlist_write_port: Option<Arc<dyn WatchlistWritePort>>,
    plugin_uninstall_guidance_snapshot_port: Option<Arc<dyn PluginUninstallGuidanceSnapshotPort>>,
    plugin_snapshot_port: Option<Arc<dyn PluginSnapshotPort>>,
    plugin_write_port: Option<Arc<dyn PluginWritePort>>,
    research_screen_write_port: Option<Arc<dyn ResearchScreenWritePort>>,
    research_preset_write_port: Option<Arc<dyn ResearchPresetWritePort>>,
    strategy_definition_write_port: Option<Arc<dyn StrategyDefinitionWritePort>>,
    market_data_provider_actions: MarketDataProviderActionsApi,
    market_data_subscription_mutation: MarketDataSubscriptionMutationApi,
    adk_chat_stream_port: Option<Arc<dyn AdkChatStreamPort>>,
    adk_mutation_port: Option<Arc<dyn AdkMutationPort>>,
    alert_snapshot_port: Option<Arc<dyn AlertSnapshotPort>>,
    alert_write_port: Option<Arc<dyn AlertWritePort>>,
    strategy_definition_snapshot_port: Option<Arc<dyn StrategyDefinitionSnapshotPort>>,
    strategy_pine_analyze_snapshot_port: Option<Arc<dyn StrategyPineAnalyzeSnapshotPort>>,
    backtest_read_snapshot_port: Option<Arc<dyn BacktestReadSnapshotPort>>,
    backtest_sync_read_snapshot_port: Option<Arc<dyn BacktestSyncReadSnapshotPort>>,
    backtests_write_port: Option<Arc<dyn BacktestsWritePort>>,
    strategy_read_snapshot_port: Option<Arc<dyn StrategyReadSnapshotPort>>,
    strategy_runtime_status_port: Option<Arc<dyn StrategyRuntimeStatusPort>>,
    strategy_runtime_write_port: Option<Arc<dyn StrategyRuntimeWritePort>>,
    auth_session_snapshot_port: Option<Arc<dyn AuthSessionSnapshotPort>>,
    auth_session_write_port: Option<Arc<dyn AuthSessionWritePort>>,
    auth_session_invalidation_port:
        Option<Arc<dyn product_auth_session_manager::AuthSessionInvalidationPort>>,
    stage9_write_ports: ProductStage9WritePorts,
    notification_sequence: AtomicU64,
}
impl ProductApi {
    fn new(
        api_port: u16,
        settings: ProductSettingsServices,
        metrics: Arc<TransportMetrics>,
        live_connections: Arc<LiveConnectionMetrics>,
        runtime: Arc<ProductRuntimeState>,
        real_trade_control: RealTradeControlReader,
        optional_ports: ProductOptionalPorts,
    ) -> Self {
        let research_screen_write_port = optional_ports.stage9_write_ports.research_screen.clone();
        let market_data_subscription_mutation =
            new_market_data_subscription_mutation_api(&optional_ports);
        Self {
            api_port,
            production_routes: optional_ports.production_routes,
            production_ports: optional_ports.production_ports,
            settings,
            metrics,
            live_connections,
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
            market_data_news_search_read_snapshot_port: optional_ports
                .market_data_news_search_read_snapshot,
            adk_read_snapshot_port: optional_ports.adk_read_snapshot,
            market_data_quote_read_snapshot_port: optional_ports.market_data_quote_read_snapshot,
            market_data_prediction_read_api: MarketDataPredictionReadApi::new(
                optional_ports.market_data_prediction_read_snapshot,
            ),
            market_data_runtime_status_port: optional_ports.market_data_runtime_status,
            broker_read_snapshot_port: optional_ports.broker_read_snapshot,
            system_read_snapshot_port: optional_ports.system_read_snapshot,
            remote_watchlist_snapshot_port: optional_ports.remote_watchlist_snapshot,
            remote_watchlist_write_port: optional_ports.remote_watchlist_write,
            watchlist_write_port: optional_ports.watchlist_write,
            plugin_uninstall_guidance_snapshot_port: optional_ports
                .plugin_uninstall_guidance_snapshot,
            plugin_snapshot_port: optional_ports.plugin_snapshot,
            plugin_write_port: optional_ports.plugin_write,
            research_screen_write_port,
            research_preset_write_port: optional_ports.research_preset_write,
            strategy_definition_write_port: optional_ports.strategy_definition_write,
            market_data_subscription_mutation,
            market_data_provider_actions: MarketDataProviderActionsApi::new(
                optional_ports.market_data_provider_actions,
            ),
            adk_chat_stream_port: optional_ports.adk_chat_stream,
            adk_mutation_port: optional_ports.adk_mutation,
            alert_snapshot_port: optional_ports.alert_snapshot,
            alert_write_port: optional_ports.alert_write,
            strategy_definition_snapshot_port: optional_ports.strategy_definition_snapshot,
            strategy_pine_analyze_snapshot_port: optional_ports.strategy_pine_analyze_snapshot,
            backtest_read_snapshot_port: optional_ports.backtest_read_snapshot,
            backtest_sync_read_snapshot_port: optional_ports.backtest_sync_read_snapshot,
            backtests_write_port: optional_ports.backtests_write,
            strategy_read_snapshot_port: optional_ports.strategy_read_snapshot,
            strategy_runtime_status_port: optional_ports.strategy_runtime_status,
            strategy_runtime_write_port: optional_ports.strategy_runtime_write,
            auth_session_snapshot_port: optional_ports.auth_session_snapshot,
            auth_session_write_port: optional_ports.auth_session_write,
            auth_session_invalidation_port: optional_ports.auth_session_invalidation,
            stage9_write_ports: optional_ports.stage9_write_ports,
            notification_sequence: AtomicU64::new(0),
        }
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
        let dependencies = self.runtime_dependency_snapshot().await?;
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
                503,
                "SYSTEM_NOTIFICATION_UNAVAILABLE",
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
        let settings = self
            .settings
            .security
            .save(&input)
            .map_err(security_settings_save_failure)?;
        if let Some(port) = self.auth_session_invalidation_port.as_ref() {
            port.invalidate_all_sessions().map_err(|message| {
                ApiFailure::new(500, "AUTH_SESSION_INVALIDATION_FAILED", message)
            })?;
        }
        Ok(ApiOutput::Json(json!(settings)))
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
        let settings = self
            .settings
            .exchange_calendars
            .save(payload.exchange_calendars.into())
            .map_err(settings_failure)?;
        if let Some(manager) = &self.calendar_manager {
            manager
                .reload_settings(
                    crate::product::product_production_ports::calendar_manager_settings(
                        settings.clone(),
                    ),
                )
                .map_err(|error| {
                    ApiFailure::new(
                        503,
                        "EXCHANGE_CALENDAR_SETTINGS_UNAVAILABLE",
                        error.to_string(),
                    )
                })?;
        }
        Ok(ApiOutput::Json(json!({"exchangeCalendars": settings})))
    }
}
