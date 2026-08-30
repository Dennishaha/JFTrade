// Dispatch for the canonical production route registry.
//
// The rehearsal profile intentionally keeps the legacy path matcher in
// `product_wire.rs`.  Production requests take the registry branch first and
// therefore can only reach a handler selected by the validated 278-operation
// binding table.  This module contains the small amount of path decoding
// needed inside a broad capability target; it never decides whether a route
// is registered.

use crate::product::product_production_route_registry::ProductionRouteAdapter as Target;

impl ProductApi {
    async fn dispatch_production_target(
        &self,
        target: Target,
        request: &ApiRequest,
    ) -> Result<ApiOutput, ApiFailure> {
        match target {
            Target::AuthSessionRead => self.auth_session(request),
            Target::AuthSessionWrite => self.auth_session_write(request),
            Target::Settings => self.dispatch_production_settings(request).await,
            Target::DataManagement => self.dispatch_production_data_management(request),
            Target::SystemCore => self.dispatch_production_system_core(request).await,
            Target::SystemRead => self.system_read(&request.path),
            Target::SystemOpenDWrite | Target::RealTradeControlWrite => {
                self.system_write(request)
            }
            Target::Calendar => self.dispatch_production_calendar(request),
            Target::WatchlistMemberships => self.watchlist_memberships(&request.path),
            Target::WatchlistRead => self.watchlist_read(&request.path, &request.query),
            Target::WatchlistWrite => self.watchlist_write(request),
            Target::RemoteWatchlistRead => self.remote_watchlist_read(&request.query),
            Target::RemoteWatchlistWrite => self.remote_watchlist_write(request),
            Target::StrategyDefinitionRead => {
                self.dispatch_production_strategy_definition_read(request)
            }
            Target::StrategyDefinitionWrite => self.product_write_mutation(request),
            Target::StrategyRuntimeRead => self.strategy_read(&request.path, &request.query),
            Target::StrategyRuntimeWrite => self.strategy_runtime_write(request),
            Target::StrategyPine => self.strategy_pine_analyze(&request.body),
            Target::ResearchCatalog => self.research_screen_catalog(&request.query),
            Target::ResearchRead
            | Target::ResearchRankingsRead
            | Target::ResearchIndustriesRead
            | Target::ResearchCalendarRead
            | Target::ResearchMacroRead => self.research_read(&request.path, &request.query),
            Target::ResearchPresetRead => {
                self.research_preset_read(&request.path, &request.query)
            }
            Target::ResearchPresetWrite | Target::ResearchScreenWrite => {
                self.product_write_mutation(request)
            }
            Target::BacktestRead => self.dispatch_production_backtest_read(request),
            Target::BacktestSyncRead => self.backtest_sync_progress(&request.path),
            Target::BacktestStart
            | Target::BacktestDelete
            | Target::BacktestSyncStart
            | Target::BacktestSyncCancel => self.backtests_write(request),
            Target::ExecutionRead => self.execution_read(&request.path, &request.query),
            Target::ExecutionWrite => self.execution_write(request),
            Target::BrokerRead => self.broker_read(&request.path, &request.query),
            Target::BrokerWrite => self.brokers_write(request),
            Target::PortfolioRead => self.portfolio_read(&request.path, &request.query),
            Target::MarketDataProviderRead => {
                self.market_data_provider_read(&request.path, &request.query)
            }
            Target::MarketDataMarketsRead | Target::MarketDataSearchRead => {
                self.market_data_catalog_read(&request.path, &request.query).await
            }
            Target::MarketDataSubscriptionRead
            | Target::MarketDataSecuritiesRead
            | Target::MarketDataSnapshotsRead
            | Target::MarketDataCandlesRead
            | Target::MarketDataDepthRead
            | Target::MarketDataTicksRead
            | Target::MarketDataBrokerQueueRead
            | Target::MarketDataCapitalFlowRead
            | Target::MarketDataIntradayRead
            | Target::MarketDataProfileRead => {
                self.market_data_quote_read(&request.path, &request.query)
                    .await
            }
            Target::MarketDataDerivativeRead | Target::MarketDataFuturesRead => {
                self.market_data_derivative_read(&request.path, &request.query)
            }
            Target::MarketDataOptionsRead
            | Target::MarketDataOptionsChainRead
            | Target::MarketDataOptionsExpirationsRead
            | Target::MarketDataOptionsScreenRead
            | Target::MarketDataOptionsAnalysisRead
            | Target::MarketDataOptionsEventsRead
            | Target::MarketDataOptionsUnusualRead
            | Target::MarketDataOptionsZeroDteRead
            | Target::MarketDataOptionsZeroDteContractRead
            | Target::MarketDataOptionsEarningsRead
            | Target::MarketDataOptionsSellerRead => {
                self.market_data_options_read(&request.path, &request.query)
            }
            Target::MarketDataNewsActionsRead => {
                self.market_data_news_actions_read(&request.path, &request.query)
            }
            Target::MarketDataNewsSearchRead => {
                self.market_data_news_search_read(&request.path, &request.query)
            }
            Target::MarketDataPredictionRead => {
                self.market_data_prediction_read_api.dispatch(request)
            }
            Target::MarketDataSubscriptionAcquireWrite
            | Target::MarketDataSubscriptionReleaseWrite
            | Target::MarketDataSubscriptionClearWrite
            | Target::MarketDataSubscriptionHeartbeatWrite
            | Target::MarketDataPredictionSubscriptionAcquireWrite
            | Target::MarketDataPredictionSubscriptionReleaseWrite => {
                self.market_data_subscription_mutation.dispatch(request)
            }
            Target::MarketDataInstrumentsNormalizeWrite
            | Target::MarketDataBatchSnapshotsWrite
            | Target::MarketDataOptionsAnalysisWrite
            | Target::MarketDataZeroDteWrite
            | Target::MarketDataPredictionCombosWrite => {
                self.market_data_provider_actions.dispatch(request).await
            }
            Target::PluginsRead => self.plugin_catalog(),
            Target::PluginsWrite => self.plugin_write(request),
            Target::PluginGuidanceRead => self.plugin_uninstall_guidance(&request.path),
            Target::AlertsRead => {
                let kind = if request.path.ends_with("/option-events") {
                    AlertKind::OptionEvents
                } else {
                    AlertKind::Price
                };
                self.alerts(kind, &request.query)
            }
            Target::AlertsWrite => self.alert_write(request),
            Target::AdkTemplatesRead => Ok(ApiOutput::Json(agent_templates_wire())),
            Target::AdkRead => self.adk_read(request),
            Target::AdkMutation => self.adk_mutation(request),
            Target::AdkChat => self.adk_chat_stream(request),
            Target::WebSocketLive => Err(ApiFailure::new(
                426,
                "WEBSOCKET_UPGRADE_REQUIRED",
                "live endpoint requires a websocket upgrade",
            )),
        }
    }

    async fn dispatch_production_settings(
        &self,
        request: &ApiRequest,
    ) -> Result<ApiOutput, ApiFailure> {
        match (request.method.as_str(), request.path.as_str()) {
            ("GET", "/api/v1/settings/ui") => self.appearance(),
            ("PUT", "/api/v1/settings/ui") => self.save_appearance(&request.body),
            ("GET", "/api/v1/settings/brokers") => self.broker_settings(),
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
            ("GET", "/api/v1/settings/onboarding") => self.onboarding().await,
            ("PUT", "/api/v1/settings/onboarding") => self.save_onboarding(&request.body).await,
            ("GET", "/api/v1/settings/execution") => self.execution_settings(),
            ("PUT", "/api/v1/settings/execution") => {
                self.save_execution_settings(&request.body)
            }
            ("GET", "/api/v1/settings/adk") => self.assistant_runtime_settings(),
            ("PUT", "/api/v1/settings/adk") => {
                self.save_assistant_runtime_settings(&request.body)
            }
            ("GET", "/api/v1/settings/adk/mcp") => self.mcp_server_settings(),
            ("PUT", "/api/v1/settings/adk/mcp") => {
                self.save_mcp_server_settings(&request.body)
            }
            ("POST", "/api/v1/settings/adk/mcp/token/reset") => self.reset_mcp_server_token(),
            ("GET", "/api/v1/settings/system-notifications") => {
                self.system_notification_settings()
            }
            ("PUT", "/api/v1/settings/system-notifications") => {
                self.save_system_notification_settings(&request.body)
            }
            ("POST", "/api/v1/settings/system-notifications/test") => {
                self.test_system_notification()
            }
            ("GET", "/api/v1/settings/pine-worker") => self.pine_worker_settings(),
            ("PUT", "/api/v1/settings/pine-worker") => {
                self.save_pine_worker_settings(&request.body)
            }
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
            _ => Err(registered_target_mismatch(Target::Settings, request)),
        }
    }

    fn dispatch_production_data_management(
        &self,
        request: &ApiRequest,
    ) -> Result<ApiOutput, ApiFailure> {
        match (request.method.as_str(), request.path.as_str()) {
            ("GET", "/api/v1/settings/data-management/databases") => {
                self.database_overview(&request.query)
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
            _ => Err(registered_target_mismatch(Target::DataManagement, request)),
        }
    }

    async fn dispatch_production_system_core(
        &self,
        request: &ApiRequest,
    ) -> Result<ApiOutput, ApiFailure> {
        match request.path.as_str() {
            "/api/v1/system/status" => Ok(self.system_status()),
            "/api/v1/system/runtime-dependencies" => self.runtime_dependencies().await,
            "/api/v1/system/futu-opend/install-guide" => self.futu_open_d_install_guide(),
            "/api/v1/system/storage/overview" => Ok(self.storage_overview()),
            "/api/v1/system/real-trade-approvals" => Ok(self.real_trade_approvals()),
            "/api/v1/system/real-trade-hard-stops" => Ok(self.real_trade_hard_stops()),
            "/api/v1/system/real-trade-hard-stop-events" => {
                Ok(self.real_trade_hard_stop_events())
            }
            "/api/v1/system/real-trade-kill-switch" => Ok(self.real_trade_kill_switch()),
            "/api/v1/system/real-trade-kill-switch-events" => {
                Ok(self.real_trade_kill_switch_events())
            }
            "/api/v1/system/real-trade-risk-limits" => Ok(self.real_trade_risk_limits()),
            "/api/v1/system/real-trade-risk-events" => Ok(self.real_trade_risk_events()),
            _ => Err(registered_target_mismatch(Target::SystemCore, request)),
        }
    }

    fn dispatch_production_calendar(
        &self,
        request: &ApiRequest,
    ) -> Result<ApiOutput, ApiFailure> {
        match (request.method.as_str(), request.path.as_str()) {
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
            _ => Err(registered_target_mismatch(Target::Calendar, request)),
        }
    }

    fn dispatch_production_strategy_definition_read(
        &self,
        request: &ApiRequest,
    ) -> Result<ApiOutput, ApiFailure> {
        match request.path.as_str() {
            "/api/v1/strategy-definitions" => self.strategy_definition_list(),
            path if is_strategy_definition_version_path(path) => {
                self.strategy_definition_version(path)
            }
            path if is_strategy_definition_versions_path(path) => {
                self.strategy_definition_versions(path)
            }
            path if is_strategy_definition_detail_path(path) => {
                self.strategy_definition_detail(path, &request.query)
            }
            _ => Err(registered_target_mismatch(Target::StrategyDefinitionRead, request)),
        }
    }

    fn dispatch_production_backtest_read(
        &self,
        request: &ApiRequest,
    ) -> Result<ApiOutput, ApiFailure> {
        match request.path.as_str() {
            "/api/v1/backtests" => self.backtest_list(),
            path if is_backtest_status_path(path) => self.backtest_status(path),
            path if is_backtest_result_path(path) => self.backtest_result(path),
            _ => Err(registered_target_mismatch(Target::BacktestRead, request)),
        }
    }
}

fn registered_target_mismatch(target: Target, request: &ApiRequest) -> ApiFailure {
    ApiFailure::new(
        500,
        "ROUTE_REGISTRY_INVARIANT",
        format!(
            "registry target {} does not accept {} {}",
            target.name(), request.method, request.path
        ),
    )
}
