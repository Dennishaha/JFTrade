fn provider_descriptor_wire(
    descriptor: jftrade_marketdata::ProviderDescriptor,
) -> serde_json::Value {
    let mut value = serde_json::to_value(descriptor)
        .expect("validated provider descriptor must be serializable");
    let Some(capabilities) = value
        .get_mut("capabilities")
        .and_then(serde_json::Value::as_object_mut)
    else {
        return value;
    };
    if capabilities
        .get("orderBookLevels")
        .and_then(serde_json::Value::as_array)
        .is_some_and(Vec::is_empty)
    {
        capabilities.insert("orderBookLevels".to_owned(), serde_json::Value::Null);
    }
    if capabilities
        .get("historicalLookbackDays")
        .and_then(serde_json::Value::as_object)
        .is_some_and(serde_json::Map::is_empty)
    {
        capabilities.remove("historicalLookbackDays");
    }
    value
}

fn broker_settings_wire(inputs: jftrade_settings::BrokerSettingsInputs) -> serde_json::Value {
    json!({
        "brokers": [{
            "descriptor": jftrade_integration_futu::broker_descriptor(),
            "integration": inputs.saved_integration,
            "defaults": inputs.effective_config,
        }],
        "accounts": inputs.accounts,
    })
}

const BUILTIN_AGENT_INSTRUCTION: &str = "你是 JFTrade 投资分析 agent。优先使用内部行情、账户、策略和回测工具；涉及安装 skill、保存策略、运行优化或改变自动化状态时遵守当前审批等级。输出必须说明使用了哪些数据来源，不提供保证收益承诺。\n\n对目标明确的任务，要在当前运行中连续完成诊断、结论以及直接相关的可执行方案。安全、只读且能从现有上下文合理推断的下一步，必须直接完成；不得用‘你想先做哪项’、‘你更想看哪部分’、‘是否继续’或‘如果需要我可以继续’把它留给用户。多个安全分支都直接服务原始意图时，采用推荐默认值或合并覆盖，不得仅为减少工作量要求用户选择。\n\n只有三类真正阻塞情况可以调用 interaction.request_user：缺少只有用户才能提供的必要信息、存在无法合并的重大取舍，或继续会越过权限/任务范围边界。提问时必须如实填写 decisionKind 和 blockingReason。实际写操作仍走审批流程，不得用提问工具替代授权。\n\n收到 interaction.request_user 的回答后，回答只是解除阻塞，必须继续完成原始请求，而不是总结或复述计划后结束运行。";

const BUILTIN_AGENT_TOOLS: &[&str] = &[
    "interaction.request_user",
    "workflow.wait",
    "tools.search",
    "models.list",
    "system.status",
    "system.futu_opend",
    "plugins.catalog",
    "market.capabilities",
    "market.search",
    "market.snapshot",
    "market.snapshots",
    "market.candles",
    "market.intraday",
    "market.subscriptions",
    "watchlist.list",
    "research.instrument",
    "research.financials",
    "research.valuation",
    "research.news",
    "research.screen",
    "portfolio.accounts",
    "portfolio.overview",
    "portfolio.positions",
    "account.orders",
    "risk.state",
    "strategy.definitions",
    "strategy.validate_pine",
    "strategy.research_backtest",
    "backtest.runs",
    "backtest.result_view",
    "backtest.kline_sync_status",
];

const BUILTIN_AGENT_SKILLS: &[&str] = &[
    "jftrade-workflow-management",
    "jftrade-operations",
    "jftrade-market",
    "jftrade-derivatives",
    "jftrade-research",
    "jftrade-prediction",
    "jftrade-trading",
    "jftrade-portfolio",
    "jftrade-strategy-research",
    "jftrade-strategy-publish",
    "external-http",
];

fn agent_templates_wire() -> serde_json::Value {
    json!({
        "templates": [{
            "id": "jftrade-default",
            "name": "默认助手",
            "instruction": BUILTIN_AGENT_INSTRUCTION,
            "providerId": "",
            "tools": BUILTIN_AGENT_TOOLS,
            "toolAccessMode": "selected",
            "skills": BUILTIN_AGENT_SKILLS,
            "permissionMode": "approval",
            "memoryEnabled": true,
            "workMode": "chat",
            "loopMaxIterations": 5,
            "status": "ENABLED"
        }]
    })
}

fn runtime_message(runtime: &ProductRuntimeSnapshot) -> String {
    if let Some(error) = &runtime.last_error {
        return format!("Rust retained runtime failed: {error}");
    }
    let helper = match runtime.helper_state {
        Some(jftrade_integration_marketdata_helper::ProcessState::Ready) => "ready",
        Some(_) => "not-ready",
        None => "not-configured",
    };
    format!(
        "Rust read-only product shadow reports system status and settings projections; PineTS workers {}/{} ready; market-data helper {helper}",
        runtime.pine_ready, runtime.pine_total
    )
}

impl ApiPort for ProductApi {
    fn dispatch(&self, request: ApiRequest) -> PortFuture<'_> {
        Box::pin(async move {
            match (request.method.as_str(), request.path.as_str()) {
                ("GET", "/api/v1/system/status") => Ok(self.system_status()),
                ("GET", "/api/v1/system/runtime-dependencies") => {
                    Ok(self.runtime_dependencies().await)
                }
                ("GET", "/api/v1/system/futu-opend") => {
                    self.system_read("/api/v1/system/futu-opend")
                }
                ("GET", "/api/v1/system/futu-opend/install-guide") => {
                    self.futu_open_d_install_guide()
                }
                ("GET", "/api/v1/system/storage/overview") => Ok(self.storage_overview()),
                ("GET", "/api/v1/system/real-trade-approvals") => Ok(self.real_trade_approvals()),
                ("GET", "/api/v1/system/real-trade-hard-stops") => Ok(self.real_trade_hard_stops()),
                ("GET", "/api/v1/system/real-trade-hard-stop-events") => {
                    Ok(self.real_trade_hard_stop_events())
                }
                ("GET", "/api/v1/system/real-trade-kill-switch") => {
                    Ok(self.real_trade_kill_switch())
                }
                ("GET", "/api/v1/system/real-trade-kill-switch-events") => {
                    Ok(self.real_trade_kill_switch_events())
                }
                ("GET", "/api/v1/system/real-trade-risk-limits") => {
                    Ok(self.real_trade_risk_limits())
                }
                ("GET", "/api/v1/system/real-trade-risk-events") => {
                    Ok(self.real_trade_risk_events())
                }
                ("GET", "/api/v1/system/worker/broker-order-updates") => {
                    self.system_read("/api/v1/system/worker/broker-order-updates")
                }
                ("GET", "/api/v1/adk/agent-templates") => {
                    Ok(ApiOutput::Json(agent_templates_wire()))
                }
                ("GET", "/api/v1/alerts/option-events") => {
                    self.alerts(AlertKind::OptionEvents, &request.query)
                }
                ("GET", "/api/v1/alerts/price") => {
                    self.alerts(AlertKind::Price, &request.query)
                }
                ("GET", "/api/v1/settings/ui") => self.appearance(),
                ("GET", "/api/v1/settings/brokers") => self.broker_settings(),
                ("GET", "/api/v1/settings/onboarding") => self.onboarding().await,
                ("PUT", "/api/v1/settings/onboarding") => self.save_onboarding(&request.body).await,
                ("PUT", "/api/v1/settings/ui") => self.save_appearance(&request.body),
                ("GET", "/api/v1/settings/execution") => self.execution_settings(),
                ("PUT", "/api/v1/settings/execution") => {
                    self.save_execution_settings(&request.body)
                }
                ("GET", "/api/v1/settings/adk") => self.assistant_runtime_settings(),
                ("GET", "/api/v1/settings/adk/mcp") => self.mcp_server_settings(),
                ("PUT", "/api/v1/settings/adk/mcp") => self.save_mcp_server_settings(&request.body),
                ("POST", "/api/v1/settings/adk/mcp/token/reset") => self.reset_mcp_server_token(),
                ("PUT", "/api/v1/settings/adk") => {
                    self.save_assistant_runtime_settings(&request.body)
                }
                ("GET", "/api/v1/settings/system-notifications") => {
                    self.system_notification_settings()
                }
                ("GET", "/api/v1/settings/pine-worker") => self.pine_worker_settings(),
                ("GET", "/api/v1/settings/security") => self.security_settings(),
                ("PUT", "/api/v1/settings/security") => {
                    self.save_security_settings(&request.body, request.desktop_trusted)
                }
                ("GET", "/api/v1/settings/market-data-provider") => {
                    self.active_market_data_provider()
                }
                ("PUT", "/api/v1/settings/market-data-provider") => {
                    self.save_active_market_data_provider(&request.body)
                }
                ("GET", "/api/v1/settings/backtest-market-data-provider") => {
                    self.backtest_market_data_provider()
                }
                ("PUT", "/api/v1/settings/backtest-market-data-provider") => {
                    self.save_backtest_market_data_provider(&request.body)
                }
                ("GET", "/api/v1/settings/exchange-calendars") => self.exchange_calendar_settings(),
                ("GET", "/api/v1/settings/data-management/databases") => {
                    self.database_overview(&request.query)
                }
                ("GET", "/api/v1/backtests") => self.backtest_list(),
                ("GET", path) if is_backtest_sync_path(path) => self.backtest_sync_progress(path),
                ("GET", path) if is_backtest_status_path(path) => self.backtest_status(path),
                ("GET", path) if is_backtest_result_path(path) => self.backtest_result(path),
                ("GET", "/api/v1/strategies") => {
                    self.strategy_read("/api/v1/strategies", &request.query)
                }
                ("GET", path) if is_strategy_read_path(path) => {
                    self.strategy_read(path, &request.query)
                }
                ("GET", "/api/v1/research/screens/catalog") => {
                    self.research_screen_catalog(&request.query)
                }
                ("GET", "/api/v1/strategy-definitions") => self.strategy_definition_list(),
                ("GET", path) if is_strategy_definition_version_path(path) => {
                    self.strategy_definition_version(path)
                }
                ("GET", path) if is_strategy_definition_versions_path(path) => {
                    self.strategy_definition_versions(path)
                }
                ("GET", path) if is_strategy_definition_detail_path(path) => {
                    self.strategy_definition_detail(path, &request.query)
                }
                ("GET", "/api/v1/system/exchange-calendars/sources") => {
                    self.calendar_source_snapshot()
                }
                ("GET", "/api/v1/system/exchange-calendars/status") => {
                    self.calendar_status_snapshot()
                }
                ("POST", path) if is_calendar_control_path(path, "/refresh") => {
                    self.calendar_refresh(path)
                }
                ("POST", path) if is_calendar_control_path(path, "/probe") => {
                    self.calendar_probe(path)
                }
                ("GET", path) if is_watchlist_membership_path(path) => {
                    self.watchlist_memberships(path)
                }
                ("GET", path) if is_watchlist_read_path(path) => {
                    self.watchlist_read(path, &request.query)
                }
                ("GET", path) if is_portfolio_path(path) => {
                    self.portfolio_read(path, &request.query)
                }
                ("GET", path) if is_research_read_path(path) => {
                    self.research_read(path, &request.query)
                }
                ("GET", path) if is_research_preset_read_path(path) => {
                    self.research_preset_read(path, &request.query)
                }
                ("GET", path) if is_execution_read_path(path) => {
                    self.execution_read(path, &request.query)
                }
                ("GET", path) if is_market_data_provider_read_path(path) => {
                    self.market_data_provider_read(path, &request.query)
                }
                ("GET", path) if is_broker_read_path(path) => {
                    self.broker_read(path, &request.query)
                }
                ("GET", "/api/v1/watchlists/remote") => {
                    self.remote_watchlist_read(&request.query)
                }
                ("GET", "/api/v1/plugins") => self.plugin_catalog(),
                ("GET", path) if is_plugin_operation_path(path) => self.plugin_operation(path),
                ("GET", path) if is_plugin_uninstall_guidance_path(path) => {
                    self.plugin_uninstall_guidance(path)
                }
                ("POST", "/api/v1/settings/data-management/cleanup/preview") => {
                    self.cleanup_preview(&request.body)
                }
                ("POST", "/api/v1/settings/data-management/cleanup/execute") => {
                    self.cleanup_execute(&request.body)
                }
                ("POST", "/api/v1/settings/data-management/databases/rebuild") => {
                    self.database_rebuild(&request.body)
                }
                ("POST", path) if is_data_management_database_path(path, "/backup") => {
                    self.database_backup(path, &request.body)
                }
                ("POST", path) if is_data_management_database_path(path, "/compact") => {
                    self.database_compact(path, &request.body)
                }
                ("PUT", "/api/v1/settings/exchange-calendars") => {
                    self.save_exchange_calendar_settings(&request.body)
                }
                ("PUT", "/api/v1/settings/system-notifications") => {
                    self.save_system_notification_settings(&request.body)
                }
                ("POST", "/api/v1/settings/system-notifications/test") => {
                    self.test_system_notification()
                }
                ("PUT", "/api/v1/settings/pine-worker") => {
                    self.save_pine_worker_settings(&request.body)
                }
                ("PUT", path) if is_broker_integration_path(path) => {
                    self.save_broker_integration(&request.body)
                }
                ("POST", "/api/v1/settings/broker-accounts") => {
                    self.create_managed_broker_account(&request.body)
                }
                ("PUT", path) if is_managed_account_path(path) => {
                    let id = managed_account_id(path)?;
                    self.update_managed_broker_account(&id, &request.body)
                }
                ("DELETE", path) if is_managed_account_path(path) => {
                    let id = managed_account_id(path)?;
                    self.delete_managed_broker_account(&id)
                }
                _ => Err(ApiFailure::new(
                    501,
                    "RUST_OWNER_NOT_IMPLEMENTED",
                    format!("Rust product owner has not implemented {}", request.path),
                )),
            }
        })
    }
}

#[derive(Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase")]
struct AppearanceWriteRequest {
    #[serde(default)]
    appearance: UiAppearanceSettings,
}

#[derive(Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase")]
struct ExchangeCalendarWriteRequest {
    #[serde(default)]
    exchange_calendars: ExchangeCalendarWriteInput,
}

#[derive(Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase", default)]
struct MarketDataProviderWriteRequest {
    active_provider: String,
}

#[derive(Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase", default)]
struct ExchangeCalendarWriteInput {
    auto_refresh_enabled: bool,
    error_notifications_enabled: Option<bool>,
    refresh_interval_hours: i32,
    warmup_markets: Vec<String>,
    source_policies: Vec<jftrade_settings::ExchangeCalendarSourcePolicy>,
    manual_overrides: Vec<jftrade_settings::ExchangeCalendarManualOverride>,
}

impl From<ExchangeCalendarWriteInput> for ExchangeCalendarSettings {
    fn from(input: ExchangeCalendarWriteInput) -> Self {
        Self {
            auto_refresh_enabled: input.auto_refresh_enabled,
            error_notifications_enabled: input.error_notifications_enabled.unwrap_or(true),
            refresh_interval_hours: input.refresh_interval_hours,
            warmup_markets: input.warmup_markets,
            source_policies: input.source_policies,
            manual_overrides: input.manual_overrides,
        }
    }
}

fn settings_failure(error: jftrade_settings::SettingsStoreError) -> ApiFailure {
    ApiFailure::new(500, "SETTINGS_SAVE_FAILED", error.to_string())
}

fn settings_read_failure(error: jftrade_settings::SettingsStoreError) -> ApiFailure {
    ApiFailure::new(500, "SETTINGS_READ_FAILED", error.to_string())
}

fn mcp_server_read_failure(error: McpServerSettingsError) -> ApiFailure {
    ApiFailure::new(500, "SETTINGS_READ_FAILED", error.to_string())
}

fn mcp_server_save_failure(error: McpServerSettingsError) -> ApiFailure {
    let message = error.to_string();
    match error {
        McpServerSettingsError::InvalidPort
        | McpServerSettingsError::InvalidAuthMode
        | McpServerSettingsError::TokenRequired => {
            ApiFailure::new(400, "MCP_SERVER_SETTINGS_REJECTED", message)
        }
        _ => ApiFailure::new(500, "MCP_SERVER_SETTINGS_FAILED", message),
    }
}

fn mcp_server_token_reset_failure(error: McpServerSettingsError) -> ApiFailure {
    ApiFailure::new(500, "MCP_SERVER_TOKEN_RESET_FAILED", error.to_string())
}

fn security_settings_read_failure(error: SecuritySettingsError) -> ApiFailure {
    ApiFailure::new(500, "SETTINGS_READ_FAILED", error.to_string())
}

fn security_settings_save_failure(error: SecuritySettingsError) -> ApiFailure {
    let message = error.to_string();
    match error {
        SecuritySettingsError::InvalidPort => {
            ApiFailure::new(400, "INVALID_WEB_ACCESS_PORT", message)
        }
        SecuritySettingsError::PasswordRequired
        | SecuritySettingsError::PasswordTooShort
        | SecuritySettingsError::PasswordTooLong => {
            ApiFailure::new(400, "INVALID_WEB_ACCESS_PASSWORD", message)
        }
        SecuritySettingsError::Runtime { .. } | SecuritySettingsError::RuntimeRollback { .. } => {
            ApiFailure::new(409, "WEB_ACCESS_LISTENER_UPDATE_FAILED", message)
        }
        SecuritySettingsError::PasswordHash(_) | SecuritySettingsError::Store(_) => {
            ApiFailure::new(500, "SETTINGS_SAVE_FAILED", message)
        }
    }
}

fn broker_settings_failure(error: BrokerSettingsError) -> ApiFailure {
    match error {
        BrokerSettingsError::MissingAccountId => {
            ApiFailure::new(400, "BAD_REQUEST", error.to_string())
        }
        BrokerSettingsError::AccountNotFound => {
            ApiFailure::new(404, "NOT_FOUND", error.to_string())
        }
        BrokerSettingsError::Store(_) => {
            ApiFailure::new(500, "SETTINGS_SAVE_FAILED", error.to_string())
        }
    }
}

fn market_data_provider_failure(
    error: MarketDataProviderSettingsError,
    invalid_code: &'static str,
) -> ApiFailure {
    let message = error.to_string();
    match error {
        MarketDataProviderSettingsError::Invalid => ApiFailure::new(400, invalid_code, message),
        MarketDataProviderSettingsError::Runtime(_) => {
            ApiFailure::new(409, "MARKET_DATA_PROVIDER_UPDATE_FAILED", message)
        }
        MarketDataProviderSettingsError::Store(_) => {
            ApiFailure::new(500, "SETTINGS_SAVE_FAILED", message)
        }
    }
}

fn database_overview_failure(error: OverviewError) -> ApiFailure {
    let message = error.to_string();
    match error {
        OverviewError::UnknownDatabase(_) => {
            ApiFailure::new(400, "DATABASE_STATUS_REJECTED", message)
        }
        OverviewError::RebuildMarker(_) => ApiFailure::new(500, "DATABASE_STATUS_FAILED", message),
    }
}

fn cleanup_preview_failure(error: CleanupPreviewError) -> ApiFailure {
    ApiFailure::new(400, "DATABASE_CLEANUP_PREVIEW_REJECTED", error.to_string())
}

fn maintenance_failure(error: MaintenanceOperationError, fallback_code: &'static str) -> ApiFailure {
    let message = error.to_string();
    match error {
        MaintenanceOperationError::PreviewNotFound => {
            ApiFailure::new(404, "CLEANUP_PREVIEW_NOT_FOUND", message)
        }
        MaintenanceOperationError::Conflict(_) => {
            ApiFailure::new(409, "DATABASE_MAINTENANCE_CONFLICT", message)
        }
        MaintenanceOperationError::Stale => {
            ApiFailure::new(409, "CLEANUP_PREVIEW_STALE", message)
        }
        MaintenanceOperationError::Rejected(_) | MaintenanceOperationError::Failed(_) => {
            ApiFailure::new(400, fallback_code, message)
        }
    }
}

fn is_data_management_database_path(path: &str, suffix: &str) -> bool {
    data_management_database_id(path, suffix).is_ok()
}

fn is_calendar_control_path(path: &str, operation: &str) -> bool {
    let base = format!("/api/v1/system/exchange-calendars{operation}");
    path == base
        || path
            .strip_prefix(&(base + "/"))
            .is_some_and(|market| !market.is_empty() && !market.contains('/'))
}

fn calendar_market_from_path<'a>(path: &'a str, marker: &str) -> Option<&'a str> {
    path.strip_prefix("/api/v1/system/exchange-calendars")?
        .strip_prefix(marker)
        .filter(|market| !market.is_empty() && !market.contains('/'))
}

fn data_management_database_id(path: &str, suffix: &str) -> Result<String, ApiFailure> {
    let prefix = "/api/v1/settings/data-management/databases/";
    let Some(value) = path
        .strip_prefix(prefix)
        .and_then(|value| value.strip_suffix(suffix))
    else {
        return Err(ApiFailure::new(400, "BAD_REQUEST", "invalid database path"));
    };
    let value = value.trim_matches('/');
    if value.is_empty() || value.contains('/') {
        return Err(ApiFailure::new(400, "BAD_REQUEST", "invalid database id"));
    }
    Ok(percent_decode_str(value)
        .decode_utf8()
        .map_err(|_| ApiFailure::new(400, "BAD_REQUEST", "invalid database id"))?
        .into_owned())
}

fn research_screen_catalog_failure(error: ScreenCatalogError) -> ApiFailure {
    match error {
        ScreenCatalogError::UnsupportedFutuMarket
        | ScreenCatalogError::UnsupportedEmbeddedMarket(_) => {
            ApiFailure::new(400, "BAD_REQUEST", error.to_string())
        }
        ScreenCatalogError::BrokerUnavailable(_) => {
            ApiFailure::new(409, "BROKER_CAPABILITY_UNAVAILABLE", error.to_string())
        }
        ScreenCatalogError::FixtureInvalid(_) => {
            ApiFailure::new(500, "RESEARCH_SCREEN_CATALOG_FAILED", error.to_string())
        }
    }
}

fn alert_snapshot_failure(error: AlertSnapshotError) -> ApiFailure {
    ApiFailure::new(503, "ALERTS_UNAVAILABLE", error.to_string())
}

fn parse_database_overview_query(query: &str) -> OverviewRequest {
    let mut request = OverviewRequest::default();
    for pair in query.split('&').filter(|value| !value.is_empty()) {
        let (name, value) = pair.split_once('=').unwrap_or((pair, ""));
        let name = decode_query_component(name);
        let value = decode_query_component(value);
        match name.as_str() {
            "summaryOnly" => request.summary_only = value.eq_ignore_ascii_case("true"),
            "databaseId" => request.database_id = value.trim().to_owned(),
            _ => {}
        }
    }
    request
}

fn parse_research_screen_catalog_query(query: &str) -> (String, String) {
    let mut broker_id = None;
    let mut market = None;
    for pair in query.split('&').filter(|value| !value.is_empty()) {
        let (name, value) = pair.split_once('=').unwrap_or((pair, ""));
        let name = decode_query_component(name);
        let value = decode_query_component(value);
        match name.as_str() {
            "brokerId" if broker_id.is_none() => broker_id = Some(value),
            "market" if market.is_none() => market = Some(value),
            _ => {}
        }
    }
    (broker_id.unwrap_or_default(), market.unwrap_or_default())
}

fn is_plugin_uninstall_guidance_path(path: &str) -> bool {
    path.strip_prefix("/api/v1/plugins/")
        .and_then(|suffix| suffix.strip_suffix("/uninstall-guidance"))
        .is_some_and(|plugin_id| !plugin_id.contains('/'))
}

fn is_backtest_status_path(path: &str) -> bool {
    let Some(suffix) = path.strip_prefix("/api/v1/backtests/") else {
        return false;
    };
    let mut parts = suffix.split('/');
    parts.next().is_some_and(|run_id| !run_id.is_empty())
        && parts.next() == Some("status")
        && parts.next().is_none()
}

fn is_backtest_sync_path(path: &str) -> bool {
    path.strip_prefix("/api/v1/backtests/sync/")
        .is_some_and(|task_id| !task_id.is_empty() && !task_id.contains('/'))
}

fn is_backtest_result_path(path: &str) -> bool {
    path.strip_prefix("/api/v1/backtests/")
        .is_some_and(|run_id| !run_id.is_empty() && !run_id.contains('/'))
}

fn is_plugin_operation_path(path: &str) -> bool {
    path.strip_prefix("/api/v1/plugins/operations/")
        .is_some_and(|operation_id| !operation_id.contains('/'))
}

fn plugin_operation_id(path: &str) -> Result<String, ApiFailure> {
    let encoded = path
        .strip_prefix("/api/v1/plugins/operations/")
        .filter(|operation_id| !operation_id.is_empty() && !operation_id.contains('/'))
        .ok_or_else(|| ApiFailure::new(400, "BAD_REQUEST", "operationId is required"))?;
    let decoded = percent_decode_str(encoded)
        .decode_utf8()
        .map_err(|_| ApiFailure::new(400, "BAD_REQUEST", "operationId is required"))?;
    let operation_id = decoded.trim();
    if operation_id.is_empty() {
        return Err(ApiFailure::new(
            400,
            "BAD_REQUEST",
            "operationId is required",
        ));
    }
    Ok(operation_id.to_owned())
}

fn is_strategy_definition_detail_path(path: &str) -> bool {
    path.strip_prefix("/api/v1/strategy-definitions/")
        .is_some_and(|suffix| !suffix.is_empty() && !suffix.contains('/'))
}

fn is_strategy_definition_versions_path(path: &str) -> bool {
    let Some(suffix) = path.strip_prefix("/api/v1/strategy-definitions/") else {
        return false;
    };
    let mut parts = suffix.split('/');
    parts.next().is_some_and(|id| !id.is_empty())
        && parts.next() == Some("versions")
        && parts.next().is_none()
}

fn is_strategy_definition_version_path(path: &str) -> bool {
    let Some(suffix) = path.strip_prefix("/api/v1/strategy-definitions/") else {
        return false;
    };
    let mut parts = suffix.split('/');
    parts.next().is_some_and(|id| !id.is_empty())
        && parts.next() == Some("versions")
        && parts.next().is_some_and(|version| !version.is_empty())
        && parts.next().is_none()
}

fn strategy_definition_id(path: &str) -> Result<String, ApiFailure> {
    let encoded = path
        .strip_prefix("/api/v1/strategy-definitions/")
        .filter(|id| !id.is_empty() && !id.contains('/'))
        .ok_or_else(|| ApiFailure::new(400, "BAD_REQUEST", "invalid definition id"))?;
    let decoded = percent_decode_str(encoded)
        .decode_utf8()
        .map_err(|_| ApiFailure::new(400, "BAD_REQUEST", "invalid definition id"))?;
    let id = decoded.trim();
    if id.is_empty() {
        return Err(ApiFailure::new(400, "BAD_REQUEST", "invalid definition id"));
    }
    Ok(id.to_owned())
}

fn strategy_definition_versions_id(path: &str) -> Result<String, ApiFailure> {
    let suffix = path
        .strip_prefix("/api/v1/strategy-definitions/")
        .ok_or_else(|| ApiFailure::new(400, "BAD_REQUEST", "invalid definition id"))?;
    let mut parts = suffix.split('/');
    let encoded_id = parts
        .next()
        .filter(|id| !id.is_empty())
        .ok_or_else(|| ApiFailure::new(400, "BAD_REQUEST", "invalid definition id"))?;
    if parts.next() != Some("versions") || parts.next().is_some() {
        return Err(ApiFailure::new(400, "BAD_REQUEST", "invalid definition id"));
    }
    let id = percent_decode_str(encoded_id)
        .decode_utf8()
        .map_err(|_| ApiFailure::new(400, "BAD_REQUEST", "invalid definition id"))?;
    let id = id.trim();
    if id.is_empty() {
        return Err(ApiFailure::new(400, "BAD_REQUEST", "invalid definition id"));
    }
    Ok(id.to_owned())
}

fn strategy_definition_version_path(path: &str) -> Result<(String, String), ApiFailure> {
    let suffix = path
        .strip_prefix("/api/v1/strategy-definitions/")
        .ok_or_else(|| ApiFailure::new(400, "BAD_REQUEST", "invalid definition version"))?;
    let mut parts = suffix.split('/');
    let encoded_id = parts
        .next()
        .filter(|id| !id.is_empty())
        .ok_or_else(|| ApiFailure::new(400, "BAD_REQUEST", "invalid definition version"))?;
    if parts.next() != Some("versions") {
        return Err(ApiFailure::new(400, "BAD_REQUEST", "invalid definition version"));
    }
    let encoded_version = parts
        .next()
        .filter(|version| !version.is_empty())
        .filter(|_| parts.next().is_none())
        .ok_or_else(|| ApiFailure::new(400, "BAD_REQUEST", "invalid definition version"))?;
    let decode = |encoded: &str| {
        percent_decode_str(encoded)
            .decode_utf8()
            .map(|value| value.trim().to_owned())
            .map_err(|_| ApiFailure::new(400, "BAD_REQUEST", "invalid definition version"))
    };
    let id = decode(encoded_id)?;
    let version = decode(encoded_version)?;
    if id.is_empty() || version.is_empty() {
        return Err(ApiFailure::new(400, "BAD_REQUEST", "invalid definition version"));
    }
    Ok((id, version))
}

fn parse_strategy_definition_preview(
    query: &str,
) -> Result<StrategyDefinitionPreview, ApiFailure> {
    let mut preview = StrategyDefinitionPreview::default();
    for pair in query.split('&').filter(|value| !value.is_empty()) {
        let (name, value) = pair.split_once('=').unwrap_or((pair, ""));
        let name = decode_query_component(name);
        let value = decode_query_component(value);
        match name.as_str() {
            "interval" => preview.interval = Some(value),
            "symbol" => preview.symbol = Some(value),
            "useExtendedHours" => {
                preview.use_extended_hours = match value.to_ascii_lowercase().as_str() {
                    "true" | "1" => true,
                    "false" | "0" | "" => false,
                    _ => {
                        return Err(ApiFailure::new(
                            400,
                            "BAD_REQUEST",
                            "invalid strategy definition query",
                        ));
                    }
                };
            }
            _ => {}
        }
    }
    Ok(preview)
}

fn plugin_uninstall_guidance_plugin_id(path: &str) -> Result<String, ApiFailure> {
    let encoded = path
        .strip_prefix("/api/v1/plugins/")
        .and_then(|suffix| suffix.strip_suffix("/uninstall-guidance"))
        .filter(|plugin_id| !plugin_id.contains('/'))
        .ok_or_else(|| ApiFailure::new(400, "BAD_REQUEST", "pluginId is invalid"))?;
    let decoded = percent_decode_str(encoded)
        .decode_utf8()
        .map_err(|_| ApiFailure::new(400, "BAD_REQUEST", "pluginId is invalid"))?;
    let plugin_id = decoded.trim();
    if plugin_id.is_empty() || plugin_id.contains('/') {
        return Err(ApiFailure::new(400, "BAD_REQUEST", "pluginId is invalid"));
    }
    Ok(plugin_id.to_owned())
}

fn strategy_definition_snapshot_failure(error: StrategyDefinitionSnapshotError) -> ApiFailure {
    ApiFailure::new(500, "STRATEGY_FAILED", error.to_string())
}

fn plugin_snapshot_failure(error: PluginSnapshotError) -> ApiFailure {
    ApiFailure::new(503, "PLUGINS_UNAVAILABLE", error.to_string())
}

fn decode_query_component(value: &str) -> String {
    percent_decode_str(&value.replace('+', " "))
        .decode_utf8_lossy()
        .into_owned()
}

fn is_broker_integration_path(path: &str) -> bool {
    path.strip_prefix("/api/v1/settings/brokers/")
        .and_then(|value| value.strip_suffix("/integration"))
        .is_some_and(|id| !id.is_empty() && !id.contains('/'))
}

fn is_managed_account_path(path: &str) -> bool {
    path.strip_prefix("/api/v1/settings/broker-accounts/")
        .is_some_and(|id| !id.is_empty() && !id.contains('/'))
}

fn managed_account_id(path: &str) -> Result<String, ApiFailure> {
    let encoded = path
        .strip_prefix("/api/v1/settings/broker-accounts/")
        .filter(|id| !id.is_empty() && !id.contains('/'))
        .ok_or_else(|| ApiFailure::new(400, "BAD_REQUEST", "invalid account id"))?;
    percent_decode_str(encoded)
        .decode_utf8()
        .map(|id| id.into_owned())
        .map_err(|_| ApiFailure::new(400, "BAD_REQUEST", "invalid account id"))
}

fn duration_millis(duration: Duration) -> u64 {
    u64::try_from(duration.as_millis()).unwrap_or(u64::MAX)
}
