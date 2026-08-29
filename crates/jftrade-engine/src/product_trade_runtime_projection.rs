//! Shared production trade runtime state and market-data projections.

use std::sync::{Arc, Mutex, RwLock};

use jftrade_api::LiveHub;
use jftrade_integration_futu::{TradeReadPort, TradeSecurity};
use jftrade_marketdata::{CacheLookup, ProviderRouter};
use jftrade_settings::FutuIntegrationConfig;
use serde_json::{Value, json};

use super::super::product_production_ports_market_data::product_production_ports_market_data_projection::{
    current_unix_millis, format_unix_millis_rfc3339,
};
use super::product_trade_margin_cache::MarginRatioCache;
use super::qot_market_label;

#[derive(Clone, Default)]
pub(crate) struct SharedTradeReadRuntime {
    state: Arc<RwLock<TradeRuntimeState>>,
    pub(crate) margin_ratio_cache: MarginRatioCache,
    connection: Arc<RwLock<Option<TradeRuntimeConnection>>>,
    live_hub: Arc<RwLock<Option<Arc<LiveHub>>>>,
    live_connection_limit: Arc<RwLock<Option<usize>>>,
    market_data_router: Arc<RwLock<Option<Arc<Mutex<ProviderRouter>>>>>,
}

#[derive(Clone, Debug)]
pub(crate) struct TradeRuntimeConnection {
    pub(crate) host: String,
    pub(crate) api_port: i32,
    pub(crate) websocket_port: i32,
    pub(crate) use_encryption: bool,
}

type TradeRuntimeState = Option<(Arc<dyn TradeReadPort>, bool)>;

impl std::fmt::Debug for SharedTradeReadRuntime {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("SharedTradeReadRuntime")
            .field("ready", &self.snapshot().is_ready())
            .finish()
    }
}

#[derive(Clone)]
pub(crate) struct TradeReadRuntimeSnapshot {
    pub(crate) client: Option<Arc<dyn TradeReadPort>>,
    pub(crate) trade_logged_in: Option<bool>,
}

impl TradeReadRuntimeSnapshot {
    pub(crate) fn is_ready(&self) -> bool {
        self.client.is_some() && self.trade_logged_in == Some(true)
    }
}

impl SharedTradeReadRuntime {
    pub(crate) fn set_runtime_projection(
        &self,
        config: &FutuIntegrationConfig,
        live_hub: Option<Arc<LiveHub>>,
        live_connection_limit: usize,
    ) {
        *self.connection.write().unwrap_or_else(|e| e.into_inner()) =
            Some(TradeRuntimeConnection {
                host: config.host.clone(),
                api_port: config.api_port,
                websocket_port: config.websocket_port,
                use_encryption: config.use_encryption,
            });
        *self.live_hub.write().unwrap_or_else(|e| e.into_inner()) = live_hub;
        *self
            .live_connection_limit
            .write()
            .unwrap_or_else(|e| e.into_inner()) = Some(live_connection_limit.max(1));
    }

    pub(crate) fn set_market_data_router(&self, router: Option<Arc<Mutex<ProviderRouter>>>) {
        *self
            .market_data_router
            .write()
            .unwrap_or_else(|error| error.into_inner()) = router;
    }

    pub(crate) fn security_snapshots(
        &self,
        securities: &[TradeSecurity],
    ) -> Result<Vec<Value>, String> {
        let router = self
            .market_data_router
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
            .ok_or_else(|| "Futu market-data router is unavailable".to_owned())?;
        let now_ms = current_unix_millis();
        let router_guard = router
            .lock()
            .map_err(|error| format!("failed to lock market-data router: {error}"))?;
        let cache = router_guard.cache_handle();
        let cache_guard = cache
            .lock()
            .map_err(|error| format!("failed to lock market-data cache: {error}"))?;
        let mut snapshots = Vec::new();
        for security in securities {
            let Some(market) = qot_market_label(security.market) else {
                continue;
            };
            let instrument_id = format!("{market}.{}", security.code);
            let tick = match cache_guard.lookup(&instrument_id, now_ms, 30_000) {
                CacheLookup::Fresh(tick) | CacheLookup::Stale(tick) => tick,
                CacheLookup::Missing => continue,
            };
            let volume = tick
                .volume
                .as_str()
                .parse::<serde_json::Number>()
                .map_err(|error| format!("invalid cached volume for {instrument_id}: {error}"))?;
            snapshots.push(json!({
                "symbol": tick.instrument_id,
                "lastPrice": tick.price,
                "volume": Value::Number(volume),
                "observedAt": format_unix_millis_rfc3339(tick.observed_at_ms),
            }));
        }
        Ok(snapshots)
    }

    pub(crate) fn connection_snapshot(&self) -> Option<TradeRuntimeConnection> {
        self.connection
            .read()
            .unwrap_or_else(|e| e.into_inner())
            .clone()
    }

    pub(crate) fn live_clients_snapshot(&self) -> Option<(usize, usize)> {
        let hub = self
            .live_hub
            .read()
            .unwrap_or_else(|e| e.into_inner())
            .clone()?;
        let limit = (*self
            .live_connection_limit
            .read()
            .unwrap_or_else(|e| e.into_inner()))?;
        Some((hub.snapshot().connected, limit))
    }

    pub(crate) fn set(&self, client: Option<Arc<dyn TradeReadPort>>, logged_in: Option<bool>) {
        *self.state.write().unwrap_or_else(|e| e.into_inner()) =
            client.map(|c| (c, logged_in == Some(true)));
    }

    pub(crate) fn clear(&self) {
        *self.state.write().unwrap_or_else(|e| e.into_inner()) = None;
    }

    pub(crate) fn snapshot(&self) -> TradeReadRuntimeSnapshot {
        self.state
            .read()
            .unwrap_or_else(|e| e.into_inner())
            .as_ref()
            .map_or(
                TradeReadRuntimeSnapshot {
                    client: None,
                    trade_logged_in: None,
                },
                |(c, logged)| TradeReadRuntimeSnapshot {
                    client: Some(Arc::clone(c)),
                    trade_logged_in: Some(*logged),
                },
            )
    }
}
