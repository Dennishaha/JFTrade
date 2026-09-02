fn product_schema_for(name: &str) -> Option<Value> {
    if let Some(schema) = typed_instrument_schema(name) {
        return Some(schema);
    }
    if let Some(schema) = typed_collection_schema(name) {
        return Some(schema);
    }
    match name {
        "research.screen" => Some(research_screen_schema()),
        "prediction.discover" => Some(prediction_discovery_schema()),
        "prediction.combo_quote" => Some(prediction_quote_schema()),
        "market.capabilities" => Some(strict_object(common_capability_properties(), &[])),
        "market.search" => Some(strict_object(
            read_properties(object([("query", string_schema(1, 120))])),
            &["query"],
        )),
        "market.snapshot" => Some(strict_object(
            read_properties(object([("instrumentId", string_schema(3, 80))])),
            &["instrumentId"],
        )),
        "market.snapshots" => Some(strict_object(
            read_properties(object([(
                "symbols",
                json!({
                    "type": "array",
                    "items": string_schema(1, 80),
                    "minItems": 1,
                    "maxItems": 200,
                }),
            )])),
            &["symbols"],
        )),
        "market.candles" => Some(market_series_schema(true)),
        "market.depth" => Some(market_series_schema(false)),
        "market.instrument_profile"
        | "market.intraday"
        | "market.ticks"
        | "market.broker_queue"
        | "market.capital_flow"
        | "derivatives.option_chain"
        | "derivatives.option_analysis"
        | "research.news" => Some(strict_object(
            read_properties(instrument_operation_properties(name)),
            &["instrumentId"],
        )),
        "research.technical_indicators" => Some(technical_indicator_schema()),
        "derivatives.option_screen"
        | "derivatives.option_events"
        | "derivatives.warrants"
        | "derivatives.futures"
        | "research.institutions"
        | "research.industry"
        | "alerts.price.list"
        | "alerts.option_event.list"
        | "watchlist.remote.list" => Some(strict_object(
            read_properties(operation_properties(name)),
            &[],
        )),
        "research.macro" => Some(research_macro_schema()),
        "execution.buying_power" => Some(strict_object(
            product_rule_properties(),
            &[
                "accountId",
                "tradingEnvironment",
                "market",
                "instrument",
                "orderKind",
            ],
        )),
        _ => None,
    }
}

fn core_schema_for(name: &str) -> Option<Value> {
    match name {
        "account.orders" => Some(portfolio_schema(true)),
        "broker.orders" | "broker.fills" => Some(broker_orders_or_fills_schema()),
        "broker.cash_flows" => Some(broker_cash_flows_schema()),
        "broker.fees" => Some(broker_fees_schema()),
        "broker.margin_ratios" => Some(broker_margin_ratios_schema()),
        "execution.order_events" => Some(execution_order_events_schema()),
        "market.providers" => Some(strict_object(Map::new(), &[])),
        "market.subscriptions"
        | "plugins.catalog"
        | "risk.events"
        | "risk.state"
        | "strategy.definitions"
        | "system.futu_opend"
        | "system.status" => Some(default_query_schema()),
        "system.runtime_dependencies" => Some(strict_object(Map::new(), &[])),
        "research.screen_catalog" => Some(strict_object(
            object([("market", enum_schema(&["HK", "US", "SH", "SZ"]))]),
            &[],
        )),
        "watchlist.list" => Some(watchlist_schema()),
        "portfolio.summary" => Some(portfolio_schema(false)),
        "backtest.runs" => Some(backtest_runs_schema()),
        "backtest.result_view" => Some(backtest_result_view_schema()),
        "backtest.kline_sync_status" => Some(backtest_kline_sync_status_schema()),
        "strategy.definition_versions.list" => Some(strategy_definition_versions_list_schema()),
        "strategy.definition_versions.get" => Some(strategy_definition_versions_get_schema()),
        "strategy.instance_activity" => Some(strategy_instance_activity_schema()),
        "strategy.pine_spec" => Some(strategy_pine_spec_schema()),
        "strategy.validate_pine" => Some(strategy_validate_pine_schema()),
        _ => None,
    }
}

fn typed_instrument_schema(name: &str) -> Option<Value> {
    let operations = match name {
        "research.instrument" => &[
            "profile",
            "executives",
            "executive_background",
            "operational_efficiency",
            "top_brokers",
        ][..],
        "research.financials" => &[
            "statements",
            "revenue_breakdown",
            "earnings_price_move",
            "earnings_price_history",
        ][..],
        "research.valuation" => &["detail", "constituents"][..],
        "research.analyst" => &["consensus", "ratings", "morningstar", "changes"][..],
        "research.ownership" => &[
            "overview",
            "changes",
            "holders",
            "institutional",
            "insider_holders",
            "insider_transactions",
            "management_changes",
        ][..],
        "research.corporate_actions" => &["dividends", "buybacks", "splits", "code_changes"][..],
        "research.short_interest" => &["daily_volume", "short_interest"][..],
        "prediction.snapshot" | "prediction.depth" => &[][..],
        "prediction.history" => &["candles", "historical", "ticks"][..],
        _ => return None,
    };
    Some(strict_object(
        read_properties(instrument_operation_properties_with(name, operations)),
        &["instrumentId"],
    ))
}

fn typed_collection_schema(name: &str) -> Option<Value> {
    let operations = match name {
        "research.screen" => return None,
        "research.calendar" => &["earnings", "dividends", "economic", "ipos", "trade_dates"][..],
        "research.rankings" => &[
            "earnings_beat",
            "dividend",
            "pre_market",
            "after_hours",
            "overnight",
            "top_movers",
            "hot",
            "short_selling",
            "period_change",
            "high_dividend_state",
            "heatmap",
            "rise_fall_distribution",
            "market_state",
            "fund_catalog",
        ][..],
        "research.institutions" => &[
            "list",
            "profile",
            "distribution",
            "holding_changes",
            "holdings",
            "ark_fund_holdings",
            "ark_stock_activity",
            "ark_transactions",
        ][..],
        "research.industry" => &[
            "chains",
            "chain_detail",
            "chains_by_plate",
            "plate",
            "plate_stocks",
            "owner_plates",
            "plate_list",
            "plate_members",
        ][..],
        "prediction.combo_eligible" => &[][..],
        _ => return None,
    };
    Some(strict_object(
        read_properties(operation_properties_with(name, operations)),
        &[],
    ))
}
