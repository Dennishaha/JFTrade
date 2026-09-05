impl ProductionMcpToolExecutor {
    pub(crate) fn supports(&self, name: &str) -> bool {
        matches!(
            name,
            "system.status"
                | "system.futu_opend"
                | "system.runtime_dependencies"
                | "market.providers"
                | "market.capabilities"
                | "market.search"
                | "market.instrument_profile"
                | "market.intraday"
                | "market.ticks"
                | "market.depth"
                | "market.broker_queue"
                | "market.capital_flow"
                | "market.snapshot"
                | "market.candles"
                | "market.snapshots"
                | "market.subscriptions"
                | "derivatives.futures"
                | "derivatives.warrants"
                | "derivatives.option_chain"
                | "derivatives.option_analysis"
                | "derivatives.option_events"
                | "derivatives.option_screen"
                | "prediction.discover"
                | "prediction.snapshot"
                | "prediction.depth"
                | "prediction.history"
                | "prediction.combo_eligible"
                | "prediction.combo_quote"
                | "alerts.price.list"
                | "alerts.option_event.list"
                | "research.instrument"
                | "research.financials"
                | "research.analyst"
                | "research.ownership"
                | "research.corporate_actions"
                | "research.valuation"
                | "research.institutions"
                | "research.short_interest"
                | "research.technical_indicators"
                | "research.news"
                | "research.screen"
                | "research.screen_catalog"
                | "research.calendar"
                | "research.macro"
                | "research.rankings"
                | "research.industry"
                | "broker.cash_flows"
                | "broker.fees"
                | "broker.margin_ratios"
                | "execution.order_events"
                | "execution.buying_power"
                | "plugins.catalog"
                | "watchlist.list"
                | "watchlist.remote.list"
                | "portfolio.summary"
                | "account.orders"
                | "broker.orders"
                | "broker.fills"
                | "strategy.definitions"
                | "strategy.definition_versions.list"
                | "strategy.definition_versions.get"
                | "strategy.instance_activity"
                | PINE_SPEC_TOOL
                | VALIDATE_PINE_TOOL
                | "backtest.runs"
                | "backtest.kline_sync_status"
                | "backtest.result_view"
                | "risk.state"
                | "risk.events"
        )
    }

    pub(crate) fn execute_production(
        &self,
        name: &str,
        arguments: &Value,
    ) -> Result<Value, McpToolFailure> {
        match name {
            "system.status" => self.system_read("/api/v1/system/status"),
            "system.futu_opend" => self.system_read("/api/v1/system/futu-opend"),
            "system.runtime_dependencies" => {
                self.system_read("/api/v1/system/runtime-dependencies")
            }
            "market.providers" => self.provider_read("/api/v1/market-data/provider", ""),
            "market.capabilities" => self.market_capabilities(arguments),
            "market.search" => self.market_search(arguments),
            "market.instrument_profile"
            | "market.intraday"
            | "market.ticks"
            | "market.depth"
            | "market.broker_queue"
            | "market.capital_flow" => self.market_microstructure(name, arguments),
            "market.snapshot" => self.market_snapshot(arguments),
            "market.candles" => self.market_candles(arguments),
            "market.snapshots" => self.market_snapshots(arguments),
            "market.subscriptions" => self.market_subscriptions(arguments),
            "derivatives.futures"
            | "derivatives.warrants"
            | "derivatives.option_chain"
            | "derivatives.option_analysis"
            | "derivatives.option_events"
            | "derivatives.option_screen" => self.derivative_read(name, arguments),
            "prediction.discover"
            | "prediction.snapshot"
            | "prediction.depth"
            | "prediction.history"
            | "prediction.combo_eligible"
            | "prediction.combo_quote" => self.prediction(name, arguments),
            "alerts.price.list" | "alerts.option_event.list" => self.alerts_read(name, arguments),
            "research.instrument"
            | "research.financials"
            | "research.analyst"
            | "research.ownership"
            | "research.corporate_actions"
            | "research.valuation"
            | "research.institutions"
            | "research.short_interest"
            | "research.technical_indicators"
            | "research.news"
            | "research.screen"
            | "research.screen_catalog"
            | "research.calendar"
            | "research.macro"
            | "research.rankings"
            | "research.industry" => self.research_read(name, arguments),
            "broker.cash_flows" => self.broker_cash_flows(arguments),
            "broker.fees" => self.broker_fees(arguments),
            "broker.margin_ratios" => self.broker_margin_ratios(arguments),
            "execution.order_events" => self.execution_order_events(arguments),
            "execution.buying_power" => self.execution_buying_power(arguments),
            "plugins.catalog" => self.plugins_catalog(),
            "watchlist.list" => self.watchlist_list(arguments),
            "watchlist.remote.list" => self.remote_watchlist_list(arguments),
            "portfolio.summary" => self.portfolio_summary(arguments),
            "account.orders" => self.account_orders(arguments),
            "broker.orders" => self.broker_read("orders", arguments),
            "broker.fills" => self.broker_read("fills", arguments),
            "strategy.definitions" => self.strategy_definitions(),
            "strategy.definition_versions.list" => self.strategy_definition_versions(arguments),
            "strategy.definition_versions.get" => self.strategy_definition_version(arguments),
            "strategy.instance_activity" => self.strategy_instance_activity(arguments),
            PINE_SPEC_TOOL | VALIDATE_PINE_TOOL => self.strategy_pine_mcp(name, arguments),
            "backtest.runs" => self.backtest_runs(arguments),
            "backtest.kline_sync_status" => self.backtest_kline_sync_status(arguments),
            "backtest.result_view" => self.backtest_result_view(arguments),
            "risk.state" => self.risk_state(),
            "risk.events" => self.risk_events(),
            _ => Err(McpToolFailure::unavailable(
                "MCP_TOOL_UNAVAILABLE",
                format!("production executor is not implemented for {name}"),
            )),
        }
    }

}
