use serde_json::{Value, json};

use super::ProductionAdapterBinding;
use crate::product::product_production_route_registry::ProductionRouteAdapter;

pub(super) fn allowed_modes(binding: ProductionAdapterBinding) -> Value {
    if binding == ProductionAdapterBinding::Ready {
        json!(["approval", "less_approval", "all"])
    } else {
        json!([])
    }
}

pub(super) fn helper_provider(provider: Option<jftrade_settings::MarketDataProvider>) -> bool {
    matches!(
        provider,
        Some(jftrade_settings::MarketDataProvider::Yfinance)
            | Some(jftrade_settings::MarketDataProvider::Akshare)
    )
}

pub(super) fn is_provider_dynamic_adapter(adapter: ProductionRouteAdapter) -> bool {
    matches!(
        adapter,
        ProductionRouteAdapter::MarketDataSearchRead
            | ProductionRouteAdapter::MarketDataCandlesRead
            | ProductionRouteAdapter::MarketDataSecuritiesRead
            | ProductionRouteAdapter::MarketDataMarketsRead
            | ProductionRouteAdapter::MarketDataSnapshotsRead
            | ProductionRouteAdapter::MarketDataBatchSnapshotsWrite
            | ProductionRouteAdapter::MarketDataSubscriptionRead
            | ProductionRouteAdapter::MarketDataSubscriptionAcquireWrite
            | ProductionRouteAdapter::MarketDataSubscriptionReleaseWrite
            | ProductionRouteAdapter::MarketDataSubscriptionClearWrite
            | ProductionRouteAdapter::MarketDataSubscriptionHeartbeatWrite
            | ProductionRouteAdapter::MarketDataNewsActionsRead
            | ProductionRouteAdapter::MarketDataNewsSearchRead
            | ProductionRouteAdapter::MarketIndexConstituentsRead
            | ProductionRouteAdapter::ResearchScreenWrite
            | ProductionRouteAdapter::MarketDataFuturesRead
            | ProductionRouteAdapter::MarketDataOptionsChainRead
            | ProductionRouteAdapter::MarketDataOptionsScreenRead
            | ProductionRouteAdapter::MarketDataOptionsAnalysisRead
            | ProductionRouteAdapter::MarketDataOptionsEventsRead
            | ProductionRouteAdapter::RemoteWatchlistRead
            | ProductionRouteAdapter::AlertsRead
    )
}
