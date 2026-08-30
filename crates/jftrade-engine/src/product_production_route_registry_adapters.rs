fn system_adapter(method: &str, path: &str) -> Option<ProductionRouteAdapter> {
    if path.starts_with("/api/v1/system/exchange-calendars/") {
        return Some(ProductionRouteAdapter::Calendar);
    }
    if method != "GET" {
        return Some(if path == "/api/v1/system/futu-opend/manual-retry" {
            ProductionRouteAdapter::SystemOpenDWrite
        } else {
            ProductionRouteAdapter::RealTradeControlWrite
        });
    }
    if matches!(
        path,
        "/api/v1/system/futu-opend" | "/api/v1/system/worker/broker-order-updates"
    ) {
        return Some(ProductionRouteAdapter::SystemRead);
    }
    Some(ProductionRouteAdapter::SystemCore)
}

fn watchlist_adapter(method: &str, path: &str) -> ProductionRouteAdapter {
    if method != "GET" {
        return ProductionRouteAdapter::WatchlistWrite;
    }
    if path.ends_with("/memberships") {
        ProductionRouteAdapter::WatchlistMemberships
    } else {
        ProductionRouteAdapter::WatchlistRead
    }
}

fn research_adapter(method: &str, path: &str) -> Option<ProductionRouteAdapter> {
    if path == "/api/v1/research/screens/catalog" {
        return Some(ProductionRouteAdapter::ResearchCatalog);
    }
    if path.starts_with("/api/v1/research/screens/presets") {
        return Some(if method == "GET" {
            ProductionRouteAdapter::ResearchPresetRead
        } else {
            ProductionRouteAdapter::ResearchPresetWrite
        });
    }
    if method == "POST" && path == "/api/v1/research/screens" {
        return Some(ProductionRouteAdapter::ResearchScreenWrite);
    }
    if method == "GET" && path == "/api/v1/research/rankings" {
        return Some(ProductionRouteAdapter::ResearchRankingsRead);
    }
    if method == "GET" && path == "/api/v1/research/industries" {
        return Some(ProductionRouteAdapter::ResearchIndustriesRead);
    }
    if method == "GET" && path == "/api/v1/research/calendars" {
        return Some(ProductionRouteAdapter::ResearchCalendarRead);
    }
    if method == "GET" && path == "/api/v1/research/macro" {
        return Some(ProductionRouteAdapter::ResearchMacroRead);
    }
    (method == "GET").then_some(ProductionRouteAdapter::ResearchRead)
}

fn backtest_adapter(method: &str, path: &str) -> ProductionRouteAdapter {
    match (method, path) {
        ("GET", "/api/v1/backtests/sync/{taskId}") => {
            ProductionRouteAdapter::BacktestSyncRead
        }
        ("GET", _) => ProductionRouteAdapter::BacktestRead,
        ("POST", "/api/v1/backtests/sync") => ProductionRouteAdapter::BacktestSyncStart,
        ("DELETE", "/api/v1/backtests/sync/{taskId}") => {
            ProductionRouteAdapter::BacktestSyncCancel
        }
        ("DELETE", _) => ProductionRouteAdapter::BacktestDelete,
        _ => ProductionRouteAdapter::BacktestStart,
    }
}

fn market_data_adapter(method: &str, path: &str) -> Option<ProductionRouteAdapter> {
    match (method, path) {
        ("POST", "/api/v1/market-data/subscriptions") => {
            Some(ProductionRouteAdapter::MarketDataSubscriptionAcquireWrite)
        }
        ("POST", "/api/v1/market-data/subscriptions/release") => {
            Some(ProductionRouteAdapter::MarketDataSubscriptionReleaseWrite)
        }
        ("DELETE", "/api/v1/market-data/subscriptions") => {
            Some(ProductionRouteAdapter::MarketDataSubscriptionClearWrite)
        }
        ("POST", "/api/v1/market-data/subscriptions/heartbeat") => {
            Some(ProductionRouteAdapter::MarketDataSubscriptionHeartbeatWrite)
        }
        ("POST", p)
            if p.starts_with("/api/v1/market-data/prediction/contracts/")
                && p.ends_with("/subscriptions") =>
        {
            Some(ProductionRouteAdapter::MarketDataPredictionSubscriptionAcquireWrite)
        }
        ("DELETE", p)
            if p.starts_with("/api/v1/market-data/prediction/contracts/")
                && (p.ends_with("/subscriptions") || p.contains("/subscriptions/")) =>
        {
            Some(ProductionRouteAdapter::MarketDataPredictionSubscriptionReleaseWrite)
        }
        ("POST", "/api/v1/market-data/instruments/normalize") => {
            Some(ProductionRouteAdapter::MarketDataInstrumentsNormalizeWrite)
        }
        ("POST", "/api/v1/market-data/snapshots") => {
            Some(ProductionRouteAdapter::MarketDataBatchSnapshotsWrite)
        }
        ("POST", p) if p.starts_with("/api/v1/market-data/options/analysis") => {
            Some(ProductionRouteAdapter::MarketDataOptionsAnalysisWrite)
        }
        ("POST", p)
            if p.starts_with("/api/v1/market-data/options/zero-dte")
                || p.starts_with("/api/v1/market-data/options/events/zero-dte") =>
        {
            Some(ProductionRouteAdapter::MarketDataZeroDteWrite)
        }
        ("POST", p) if p.starts_with("/api/v1/market-data/prediction/combos") => {
            Some(ProductionRouteAdapter::MarketDataPredictionCombosWrite)
        }
        ("GET", "/api/v1/market-data/provider") => {
            Some(ProductionRouteAdapter::MarketDataProviderRead)
        }
        ("GET", "/api/v1/market-data/markets") => {
            Some(ProductionRouteAdapter::MarketDataMarketsRead)
        }
        ("GET", "/api/v1/market-data/instruments") => {
            Some(ProductionRouteAdapter::MarketDataSearchRead)
        }
        ("GET", "/api/v1/market-data/subscriptions") => {
            Some(ProductionRouteAdapter::MarketDataSubscriptionRead)
        }
        ("GET", p) if p.starts_with("/api/v1/market-data/securities/") => {
            Some(ProductionRouteAdapter::MarketDataSecuritiesRead)
        }
        ("GET", p) if p.starts_with("/api/v1/market-data/snapshots/") => {
            Some(ProductionRouteAdapter::MarketDataSnapshotsRead)
        }
        ("GET", p) if p.starts_with("/api/v1/market-data/candles/") => {
            Some(ProductionRouteAdapter::MarketDataCandlesRead)
        }
        ("GET", p) if p.starts_with("/api/v1/market-data/depth/") => {
            Some(ProductionRouteAdapter::MarketDataDepthRead)
        }
        ("GET", p) if p.starts_with("/api/v1/market-data/ticks/") => {
            Some(ProductionRouteAdapter::MarketDataTicksRead)
        }
        ("GET", p) if p.starts_with("/api/v1/market-data/broker-queue/") => {
            Some(ProductionRouteAdapter::MarketDataBrokerQueueRead)
        }
        ("GET", p) if p.starts_with("/api/v1/market-data/capital-flow/") => {
            Some(ProductionRouteAdapter::MarketDataCapitalFlowRead)
        }
        ("GET", p) if p.starts_with("/api/v1/market-data/intraday/") => {
            Some(ProductionRouteAdapter::MarketDataIntradayRead)
        }
        ("GET", p)
            if p.starts_with("/api/v1/market-data/instruments/") && p.ends_with("/profile") =>
        {
            Some(ProductionRouteAdapter::MarketDataProfileRead)
        }
        ("GET", "/api/v1/market-data/warrants") => {
            Some(ProductionRouteAdapter::MarketDataDerivativeRead)
        }
        ("GET", "/api/v1/market-data/futures") => {
            Some(ProductionRouteAdapter::MarketDataFuturesRead)
        }
        ("GET", p) if p.starts_with("/api/v1/market-data/options/") => {
            Some(if p.starts_with("/api/v1/market-data/options/chains/") {
                ProductionRouteAdapter::MarketDataOptionsChainRead
            } else if p.starts_with("/api/v1/market-data/options/expirations/") {
                ProductionRouteAdapter::MarketDataOptionsExpirationsRead
            } else if p == "/api/v1/market-data/options/screens" {
                ProductionRouteAdapter::MarketDataOptionsScreenRead
            } else if p.starts_with("/api/v1/market-data/options/analysis/") {
                ProductionRouteAdapter::MarketDataOptionsAnalysisRead
            } else if p == "/api/v1/market-data/options/events" {
                ProductionRouteAdapter::MarketDataOptionsEventsRead
            } else {
                ProductionRouteAdapter::MarketDataOptionsRead
            })
        }
        ("GET", "/api/v1/market-data/news") => {
            Some(ProductionRouteAdapter::MarketDataNewsSearchRead)
        }
        ("GET", p)
            if p.starts_with("/api/v1/market-data/news/")
                || p.starts_with("/api/v1/market-data/corporate-actions/") =>
        {
            Some(ProductionRouteAdapter::MarketDataNewsActionsRead)
        }
        ("GET", p) if p.starts_with("/api/v1/market-data/prediction/") => {
            Some(ProductionRouteAdapter::MarketDataPredictionRead)
        }
        _ => None,
    }
}

fn plugin_adapter(method: &str, path: &str) -> ProductionRouteAdapter {
    if method != "GET" {
        ProductionRouteAdapter::PluginsWrite
    } else if path.ends_with("/uninstall-guidance") {
        ProductionRouteAdapter::PluginGuidanceRead
    } else {
        ProductionRouteAdapter::PluginsRead
    }
}

fn adk_adapter(method: &str, path: &str) -> ProductionRouteAdapter {
    if path == "/api/v1/adk/agent-templates" {
        return ProductionRouteAdapter::AdkTemplatesRead;
    }
    if method == "GET" {
        return ProductionRouteAdapter::AdkRead;
    }
    if matches!(path, ADK_CHAT_PATH | ADK_CHAT_STREAM_PATH) {
        ProductionRouteAdapter::AdkChat
    } else {
        ProductionRouteAdapter::AdkMutation
    }
}
