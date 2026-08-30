//! Production market-data provider status projection.

use std::sync::{Arc, Mutex};

use jftrade_settings::MarketDataProvider;
use serde_json::{Value, json};

use super::product_production_ports_market_data::product_production_ports_market_data_projection::render_subscriptions_data;
use crate::product::product_active_provider_state::ActiveProviderState;
use crate::product::{
    MarketDataProviderReadSnapshotError, MarketDataProviderReadSnapshotPort,
    MarketDataRuntimeState, MarketDataRuntimeStatusPort,
};
use jftrade_marketdata::{PhysicalSubscriptionSnapshotPort, ProviderRouter};

#[derive(Clone)]
pub(crate) struct ProductionMarketDataProviderPort {
    pub(crate) active_provider_state: Arc<ActiveProviderState>,
    pub(crate) runtime_status: Option<Arc<dyn MarketDataRuntimeStatusPort>>,
    pub(crate) router: Option<Arc<Mutex<ProviderRouter>>>,
    pub(crate) physical: Option<Arc<dyn PhysicalSubscriptionSnapshotPort>>,
}

impl std::fmt::Debug for ProductionMarketDataProviderPort {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("ProductionMarketDataProviderPort")
            .field("runtime_status", &self.runtime_status.is_some())
            .field("router", &self.router.is_some())
            .field("physical", &self.physical.is_some())
            .finish()
    }
}

impl MarketDataProviderReadSnapshotPort for ProductionMarketDataProviderPort {
    fn read(&self, path: &str, _query: &str) -> Result<Value, MarketDataProviderReadSnapshotError> {
        if path != "/api/v1/market-data/provider" {
            return Err(MarketDataProviderReadSnapshotError::Unavailable(
                "market-data provider runtime is unavailable".to_owned(),
            ));
        }
        let snapshot = self.active_provider_state.snapshot();
        let active_provider = snapshot.provider.ok_or_else(|| {
            MarketDataProviderReadSnapshotError::Unavailable(
                "active market-data provider is not configured".to_owned(),
            )
        })?;
        let runtime = self.runtime_status.as_ref().map(|port| port.snapshot());
        let (connected, readiness, stream_mode, last_error) = match active_provider {
            MarketDataProvider::Futu => {
                let connected =
                    runtime.as_ref().is_some_and(|state| state.connected) || snapshot.opend_ready;
                let readiness = if connected {
                    "ready"
                } else if snapshot.router_ready || runtime.is_some() {
                    "degraded"
                } else {
                    "unavailable"
                };
                let last_error = runtime
                    .as_ref()
                    .and_then(|state| {
                        state
                            .quote_last_error
                            .as_deref()
                            .or(state.stream_last_error.as_deref())
                    })
                    .map(str::to_owned)
                    .or_else(|| {
                        (!connected)
                            .then(|| "market-data provider runtime is not connected".to_owned())
                    });
                (
                    connected,
                    readiness,
                    if connected { "push-stream" } else { "idle" },
                    last_error,
                )
            }
            MarketDataProvider::Yfinance | MarketDataProvider::Akshare => {
                let connected = snapshot.helper_ready;
                let readiness = if connected { "ready" } else { "unavailable" };
                let last_error = (!connected).then(|| "market-data helper is not ready".to_owned());
                (connected, readiness, "idle", last_error)
            }
        };
        let demand = self
            .router
            .as_ref()
            .map(|router| {
                router
                    .lock()
                    .unwrap_or_else(|error| error.into_inner())
                    .demand()
            })
            .unwrap_or_default();
        let physical = self
            .physical
            .as_ref()
            .map(|port| {
                port.physical_subscription_snapshot().map_err(|message| {
                    MarketDataProviderReadSnapshotError::Failed {
                        code: "MARKET_DATA_SUBSCRIPTIONS_FAILED".to_owned(),
                        message,
                    }
                })
            })
            .transpose()?
            .flatten();
        let subscriptions = render_subscriptions_data(&demand, physical.as_ref());
        Ok(json!({
            "checkedAt": provider_now_rfc3339(),
            "descriptor": provider_descriptor_for(active_provider),
            "health": {
                "connected": connected,
                "readiness": readiness,
                "lastError": last_error,
                "streamMode": stream_mode,
                "activeCount": runtime.as_ref().map_or(0, |state| state.active_count),
            },
            "runtime": runtime.as_ref().map(runtime_wire).unwrap_or_else(|| runtime_wire(&MarketDataRuntimeState::default())),
            "subscriptions": subscriptions,
        }))
    }
}

fn provider_descriptor_for(provider: MarketDataProvider) -> Value {
    let descriptor = match provider {
        MarketDataProvider::Futu => jftrade_integration_futu::provider_descriptor(),
        MarketDataProvider::Yfinance => {
            jftrade_integration_marketdata_helper::yfinance_descriptor()
        }
        MarketDataProvider::Akshare => jftrade_integration_marketdata_helper::akshare_descriptor(),
    };
    crate::product::provider_descriptor_wire(descriptor)
}

fn runtime_wire(state: &MarketDataRuntimeState) -> Value {
    const ZERO_TIME: &str = "0001-01-01T00:00:00Z";
    let timestamp = |value: Option<jftrade_kernel::WireTimestamp>| {
        value
            .map(|value| json!(value))
            .unwrap_or_else(|| json!(ZERO_TIME))
    };
    json!({
        "Connected": state.connected, "Closed": state.closed, "Generation": state.generation,
        "ActiveCount": state.active_count, "LastRefreshAt": timestamp(state.last_refresh_at),
        "QuoteRetryAt": timestamp(state.quote_retry_at), "QuoteFailures": state.quote_failures,
        "QuoteLastError": state.quote_last_error.as_deref().unwrap_or_default(),
        "StreamRetryAt": timestamp(state.stream_retry_at), "StreamFailures": state.stream_failures,
        "StreamLastError": state.stream_last_error.as_deref().unwrap_or_default(),
    })
}

pub(crate) fn provider_now_rfc3339() -> String {
    time::OffsetDateTime::now_utc()
        .format(&time::format_description::well_known::Rfc3339)
        .unwrap_or_else(|_| "1970-01-01T00:00:00Z".to_owned())
}
