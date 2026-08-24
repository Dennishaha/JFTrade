impl ProductConfig {
    pub fn settings_path(&self) -> &std::path::Path {
        &self.settings_path
    }

    pub fn real_trade_control_path(&self) -> &std::path::Path {
        &self.real_trade_control_path
    }

    pub fn with_notification_port(mut self, port: Arc<dyn ProductNotificationPort>) -> Self {
        self.notification_port = Some(port);
        self
    }

    #[cfg(test)]
    fn with_calendar_manager(mut self, manager: Arc<CalendarManager>) -> Self {
        self.calendar_manager = Some(manager);
        self
    }

    #[cfg(test)]
    fn with_watchlist_membership_snapshot_port(
        mut self,
        port: Arc<dyn WatchlistMembershipSnapshotPort>,
    ) -> Self {
        self.watchlist_membership_snapshot_port = Some(port);
        self
    }

    #[cfg(test)]
    fn with_watchlist_read_snapshot_port(
        mut self,
        port: Arc<dyn WatchlistReadSnapshotPort>,
    ) -> Self {
        self.watchlist_read_snapshot_port = Some(port);
        self
    }

    #[cfg(test)]
    fn with_portfolio_snapshot_port(mut self, port: Arc<dyn PortfolioSnapshotPort>) -> Self {
        self.portfolio_snapshot_port = Some(port);
        self
    }

    #[cfg(test)]
    fn with_research_read_snapshot_port(mut self, port: Arc<dyn ResearchReadSnapshotPort>) -> Self {
        self.research_read_snapshot_port = Some(port);
        self
    }

    #[cfg(test)]
    fn with_research_preset_read_snapshot_port(
        mut self,
        port: Arc<dyn ResearchPresetReadSnapshotPort>,
    ) -> Self {
        self.research_preset_read_snapshot_port = Some(port);
        self
    }

    #[cfg(test)]
    fn with_execution_read_snapshot_port(
        mut self,
        port: Arc<dyn ExecutionReadSnapshotPort>,
    ) -> Self {
        self.execution_read_snapshot_port = Some(port);
        self
    }

    #[cfg(test)]
    fn with_execution_write_port(mut self, port: Arc<dyn ExecutionWritePort>) -> Self {
        self.stage9_write_ports.execution = Some(port);
        self
    }

    #[cfg(test)]
    fn with_market_data_subscription_mutation_port(
        mut self,
        port: Arc<dyn MarketDataSubscriptionMutationPort>,
    ) -> Self {
        self.stage9_write_ports.market_data_subscription_mutation = Some(port);
        self
    }

    #[cfg(test)]
    fn with_brokers_write_port(mut self, port: Arc<dyn BrokersWritePort>) -> Self {
        self.stage9_write_ports.brokers = Some(port);
        self
    }

    #[cfg(test)]
    fn with_research_screen_write_port(
        mut self,
        port: Arc<dyn ResearchScreenWritePort>,
    ) -> Self {
        self.stage9_write_ports.research_screen = Some(port);
        self
    }

    #[cfg(test)]
    fn with_market_data_provider_read_snapshot_port(
        mut self,
        port: Arc<dyn MarketDataProviderReadSnapshotPort>,
    ) -> Self {
        self.market_data_provider_read_snapshot_port = Some(port);
        self
    }

    pub fn with_market_data_runtime_status_port(
        mut self,
        port: Arc<dyn MarketDataRuntimeStatusPort>,
    ) -> Self {
        self.market_data_runtime_status_port = Some(port);
        self
    }

    #[cfg(test)]
    fn with_market_data_catalog_read_snapshot_port(
        mut self,
        port: Arc<dyn MarketDataCatalogReadSnapshotPort>,
    ) -> Self {
        self.market_data_catalog_read_snapshot_port = Some(port);
        self
    }

    #[cfg(test)]
    fn with_market_data_derivative_read_snapshot_port(
        mut self,
        port: Arc<dyn MarketDataDerivativeReadSnapshotPort>,
    ) -> Self {
        self.market_data_derivative_read_snapshot_port = Some(port);
        self
    }

    #[cfg(test)]
    fn with_market_data_options_read_snapshot_port(
        mut self,
        port: Arc<dyn MarketDataOptionsReadSnapshotPort>,
    ) -> Self {
        self.market_data_options_read_snapshot_port = Some(port);
        self
    }

    #[cfg(test)]
    fn with_market_data_news_actions_read_snapshot_port(
        mut self,
        port: Arc<dyn MarketDataNewsActionsReadSnapshotPort>,
    ) -> Self {
        self.market_data_news_actions_read_snapshot_port = Some(port);
        self
    }

    #[cfg(test)]
    fn with_market_data_news_search_read_snapshot_port(
        mut self,
        port: Arc<dyn MarketDataNewsSearchReadSnapshotPort>,
    ) -> Self {
        self.market_data_news_search_read_snapshot_port = Some(port);
        self
    }

    #[cfg(test)]
    fn with_adk_read_snapshot_port(mut self, port: Arc<dyn AdkReadSnapshotPort>) -> Self {
        self.adk_read_snapshot_port = Some(port);
        self
    }

    #[cfg(test)]
    fn with_market_data_quote_read_snapshot_port(
        mut self,
        port: Arc<dyn MarketDataQuoteReadSnapshotPort>,
    ) -> Self {
        self.market_data_quote_read_snapshot_port = Some(port);
        self
    }

    #[cfg(test)]
    fn with_market_data_prediction_read_snapshot_port(
        mut self,
        port: Arc<dyn MarketDataPredictionReadSnapshotPort>,
    ) -> Self {
        self.market_data_prediction_read_snapshot_port = Some(port);
        self
    }

    #[cfg(test)]
    fn with_broker_read_snapshot_port(mut self, port: Arc<dyn BrokerReadSnapshotPort>) -> Self {
        self.broker_read_snapshot_port = Some(port);
        self
    }

    #[cfg(test)]
    fn with_remote_watchlist_snapshot_port(
        mut self,
        port: Arc<dyn RemoteWatchlistSnapshotPort>,
    ) -> Self {
        self.remote_watchlist_snapshot_port = Some(port);
        self
    }

    #[cfg(test)]
    fn with_remote_watchlist_write_port(mut self, port: Arc<dyn RemoteWatchlistWritePort>) -> Self {
        self.remote_watchlist_write_port = Some(port);
        self
    }

    #[cfg(test)]
    fn with_watchlist_write_port(mut self, port: Arc<dyn WatchlistWritePort>) -> Self {
        self.watchlist_write_port = Some(port);
        self
    }

    #[cfg(test)]
    fn with_backtests_write_port(mut self, port: Arc<dyn BacktestsWritePort>) -> Self {
        self.backtests_write_port = Some(port);
        self
    }

    #[cfg(test)]
    fn with_plugin_uninstall_guidance_snapshot_port(
        mut self,
        port: Arc<dyn PluginUninstallGuidanceSnapshotPort>,
    ) -> Self {
        self.plugin_uninstall_guidance_snapshot_port = Some(port);
        self
    }

    #[cfg(test)]
    fn with_plugin_snapshot_port(mut self, port: Arc<dyn PluginSnapshotPort>) -> Self {
        self.plugin_snapshot_port = Some(port);
        self
    }

    #[cfg(test)]
    fn with_plugin_write_port(mut self, port: Arc<dyn PluginWritePort>) -> Self {
        self.plugin_write_port = Some(port);
        self
    }

    #[cfg(test)]
    fn with_research_preset_write_port(mut self, port: Arc<dyn ResearchPresetWritePort>) -> Self {
        self.research_preset_write_port = Some(port);
        self
    }

    #[cfg(test)]
    fn with_research_preset_sqlite_test_cutover(
        mut self,
        path: impl AsRef<std::path::Path>,
    ) -> Result<Self, ResearchPresetStoreError> {
        self.research_preset_write_port = Some(Arc::new(
            ResearchPresetSqliteTestCutoverPort::open(path)?,
        ));
        Ok(self)
    }

    #[cfg(test)]
    fn with_strategy_definition_write_port(
        mut self,
        port: Arc<dyn StrategyDefinitionWritePort>,
    ) -> Self {
        self.strategy_definition_write_port = Some(port);
        self
    }

    #[cfg(test)]
    fn with_market_data_provider_actions_port(
        mut self,
        port: Arc<dyn MarketDataProviderActionsPort>,
    ) -> Self {
        self.market_data_provider_actions_port = Some(port);
        self
    }

    #[cfg(test)]
    fn with_adk_chat_stream_port(mut self, port: Arc<dyn AdkChatStreamPort>) -> Self {
        self.adk_chat_stream_port = Some(port);
        self
    }

    #[cfg(test)]
    fn with_adk_mutation_port(mut self, port: Arc<dyn AdkMutationPort>) -> Self {
        self.adk_mutation_port = Some(port);
        self
    }

    #[cfg(test)]
    fn with_alert_snapshot_port(mut self, port: Arc<dyn AlertSnapshotPort>) -> Self {
        self.alert_snapshot_port = Some(port);
        self
    }

    #[cfg(test)]
    fn with_alert_write_port(mut self, port: Arc<dyn AlertWritePort>) -> Self {
        self.alert_write_port = Some(port);
        self
    }

    #[cfg(test)]
    fn with_strategy_definition_snapshot_port(
        mut self,
        port: Arc<dyn StrategyDefinitionSnapshotPort>,
    ) -> Self {
        self.strategy_definition_snapshot_port = Some(port);
        self
    }

    #[cfg(test)]
    fn with_strategy_pine_analyze_snapshot_port(
        mut self,
        port: Arc<dyn StrategyPineAnalyzeSnapshotPort>,
    ) -> Self {
        self.strategy_pine_analyze_snapshot_port = Some(port);
        self
    }

    #[cfg(test)]
    fn with_ws_live_snapshot_port(mut self, port: Arc<dyn WsLiveSnapshotPort>) -> Self {
        self.ws_live_snapshot_port = Some(port);
        self
    }

    #[cfg(test)]
    fn with_auth_session_snapshot_port(mut self, port: Arc<dyn AuthSessionSnapshotPort>) -> Self {
        self.auth_session_snapshot_port = Some(port);
        self
    }

    #[cfg(test)]
    fn with_auth_session_write_port(mut self, port: Arc<dyn AuthSessionWritePort>) -> Self {
        self.auth_session_write_port = Some(port);
        self
    }

    #[cfg(test)]
    fn with_system_write_port(mut self, port: Arc<dyn SystemWritePort>) -> Self {
        self.stage9_write_ports.system = Some(port);
        self
    }
}
