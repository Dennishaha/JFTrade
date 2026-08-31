//! Shared production trade runtime state and market-data projections.
use std::sync::{Arc, Mutex, RwLock};
use jftrade_api::LiveHub;
use jftrade_integration_futu::{
    HistoricalKlineQuery, HistoricalKlineReadPort, HistoricalKlineResult, SecuritySnapshotReadPort,
    FutureInfoReadPort,
    OptionExerciseProbabilityReadPort, OptionUnderlyingOverviewReadPort,
    OptionUnderlyingRankReadPort, OptionContractRankReadPort, OptionUnderlyingHisVolatilityReadPort,
    OptionStrategySpreadReadPort, OptionStrategyReadPort, OptionStrategyAnalysisReadPort,
    OptionMarketStatisticReadPort, OptionUnderlyingHisStatisticReadPort,
    OptionZeroDteScreenerReadPort, OptionEarningsScreenerReadPort,
    OptionZeroDteContractReadPort, OptionSellerScreenerReadPort,
    ValuationDetailReadPort,
    FutuCorporateActionsReadPort, FutuCorporateActionsQuery, FutuCorporateActionsResult,
    FutuNewsQuery, FutuNewsReadPort, FutuNewsResult,
    PredictionComboQuotePort, PredictionMarketReadPort, PredictionMarketSubscriptionPort,
    TradeReadPort, TradeSecurity, TradeWritePort,
};
use jftrade_marketdata::{CacheLookup, ProviderRouter};
use jftrade_settings::FutuIntegrationConfig;
use serde_json::{Map, Value, json};
use tokio::sync::Notify;
use crate::product::product_query::{
    normalize_candle_period, normalize_optional_query_time, parse_candle_before_time,
};
use super::super::product_production_ports_market_data::product_production_ports_market_data_projection::{
    current_unix_millis, format_unix_millis_rfc3339,
};
use super::product_trade_margin_cache::MarginRatioCache;
use super::qot_market_label;
use super::{
    BrokerReadSnapshotError, ProductionBrokerPort, TradeRequest, checked_at,
    normalize_history_time, quote_market_code, unavailable,
};
#[path = "product_trade_runtime_candles.rs"]
mod product_trade_runtime_candles;
#[path = "product_trade_runtime_futures.rs"]
mod product_trade_runtime_futures;
#[path = "product_trade_runtime_projection_values.rs"]
mod product_trade_runtime_projection_values;
#[path = "product_trade_runtime_valuation.rs"]
mod product_trade_runtime_valuation;

use product_trade_runtime_candles::{historical_snapshot, parse_requested_sessions};

#[cfg(test)]
pub(super) fn canonical_candle_time(value: &str, market: &str) -> String { product_trade_runtime_candles::canonical_candle_time(value, market) }
use product_trade_runtime_projection_values::{
    insert_rich_quote_fields, insert_rich_security_fields, security_snapshot_value,
};

#[derive(Clone, Default)]
pub(crate) struct SharedTradeReadRuntime {
    state: Arc<RwLock<TradeRuntimeState>>,
    /// The command-side client is tracked separately from the read snapshot.
    /// Provider activation can replace the OpenD session after the product
    /// ports have been constructed, so execution writes must resolve this
    /// handle at call time rather than retaining a startup-only `Option`.
    trade_writer: Arc<RwLock<Option<Arc<dyn TradeWritePort>>>>,
    pub(crate) margin_ratio_cache: MarginRatioCache,
    connection: Arc<RwLock<Option<TradeRuntimeConnection>>>,
    live_hub: Arc<RwLock<Option<Arc<LiveHub>>>>,
    live_connection_limit: Arc<RwLock<Option<usize>>>,
    market_data_router: Arc<RwLock<Option<Arc<Mutex<ProviderRouter>>>>>,
    historical_klines: Arc<RwLock<Option<Arc<dyn HistoricalKlineReadPort>>>>,
    security_snapshots: Arc<RwLock<Option<Arc<dyn SecuritySnapshotReadPort>>>>,
    pub(crate) future_info: Arc<RwLock<Option<Arc<dyn FutureInfoReadPort>>>>,
    pub(crate) option_expirations:
        Arc<RwLock<Option<Arc<dyn jftrade_integration_futu::OptionExpirationReadPort>>>>,
    pub(crate) option_chains:
        Arc<RwLock<Option<Arc<dyn jftrade_integration_futu::OptionChainReadPort>>>>,
    pub(crate) option_screens:
        Arc<RwLock<Option<Arc<dyn jftrade_integration_futu::OptionScreenReadPort>>>>,
    pub(crate) option_quotes:
        Arc<RwLock<Option<Arc<dyn jftrade_integration_futu::OptionQuoteReadPort>>>>,
    pub(crate) option_volatility:
        Arc<RwLock<Option<Arc<dyn jftrade_integration_futu::OptionVolatilityReadPort>>>>,
    pub(crate) option_exercise_probability:
        Arc<RwLock<Option<Arc<dyn OptionExerciseProbabilityReadPort>>>>,
    pub(crate) option_underlying_overview:
        Arc<RwLock<Option<Arc<dyn OptionUnderlyingOverviewReadPort>>>>,
    pub(crate) option_underlying_his_volatility:
        Arc<RwLock<Option<OptionUnderlyingHisVolatilityPort>>>,
    pub(crate) option_market_statistic: Arc<RwLock<Option<OptionMarketStatisticPort>>>,
    pub(crate) option_underlying_his_statistic:
        Arc<RwLock<Option<OptionUnderlyingHisStatisticPort>>>,
    pub(crate) option_strategy_spread: Arc<RwLock<Option<OptionStrategySpreadPort>>>,
    pub(crate) option_strategy: Arc<RwLock<Option<OptionStrategyPort>>>,
    pub(crate) option_strategy_analysis: Arc<RwLock<Option<OptionStrategyAnalysisPort>>>,
    pub(crate) option_underlying_rank: Arc<RwLock<Option<Arc<dyn OptionUnderlyingRankReadPort>>>>,
    pub(crate) option_contract_rank: Arc<RwLock<Option<Arc<dyn OptionContractRankReadPort>>>>,
    pub(crate) option_events:
        Arc<RwLock<Option<Arc<dyn jftrade_integration_futu::OptionEventReadPort>>>>,
    pub(crate) option_zero_dte_screener:
        Arc<RwLock<Option<Arc<dyn OptionZeroDteScreenerReadPort>>>>,
    pub(crate) option_earnings_screener:
        Arc<RwLock<Option<Arc<dyn OptionEarningsScreenerReadPort>>>>,
    pub(crate) option_zero_dte_contract:
        Arc<RwLock<Option<Arc<dyn OptionZeroDteContractReadPort>>>>,
    pub(crate) option_seller_screener: Arc<RwLock<Option<Arc<dyn OptionSellerScreenerReadPort>>>>,
    pub(crate) prediction_reader: Arc<RwLock<Option<Arc<dyn PredictionMarketReadPort>>>>,
    pub(crate) prediction_subscription:
        Arc<RwLock<Option<Arc<dyn PredictionMarketSubscriptionPort>>>>,
    pub(crate) prediction_combo_quote:
        Arc<RwLock<Option<Arc<dyn PredictionComboQuotePort>>>>,
    prediction_subscription_state: Arc<Mutex<PredictionSubscriptionState>>,
    pub(crate) valuation_detail: Arc<RwLock<Option<Arc<dyn ValuationDetailReadPort>>>>,
    pub(crate) news_reader: Arc<RwLock<Option<Arc<dyn FutuNewsReadPort>>>>,
    pub(crate) corporate_actions_reader: Arc<RwLock<Option<Arc<dyn FutuCorporateActionsReadPort>>>>,
    pub(crate) remote_watchlist_reader:
        Arc<RwLock<Option<Arc<dyn jftrade_integration_futu::RemoteWatchlistReadPort>>>>,
    pub(crate) remote_watchlist_writer:
        Arc<RwLock<Option<Arc<dyn jftrade_integration_futu::RemoteWatchlistWritePort>>>>,
    pub(crate) alert_reader:
        Arc<RwLock<Option<Arc<dyn jftrade_integration_futu::AlertCustomizationReadPort>>>>,
    pub(crate) alert_writer:
        Arc<RwLock<Option<Arc<dyn jftrade_integration_futu::AlertCustomizationWritePort>>>>,
    /// OpenD lifecycle listeners use this channel to wake the execution
    /// reconciliation worker after a ready/reconnect transition.
    reconciliation_wake: Arc<Notify>,
}
#[derive(Clone, Debug)]
pub(crate) struct TradeRuntimeConnection {
    pub(crate) host: String,
    pub(crate) api_port: i32,
    pub(crate) websocket_port: i32,
    pub(crate) use_encryption: bool,
}
type TradeRuntimeState = Option<(Arc<dyn TradeReadPort>, bool)>;
type OptionUnderlyingHisVolatilityPort = Arc<dyn OptionUnderlyingHisVolatilityReadPort>;
type OptionMarketStatisticPort = Arc<dyn OptionMarketStatisticReadPort>;
type OptionUnderlyingHisStatisticPort = Arc<dyn OptionUnderlyingHisStatisticReadPort>;
type OptionStrategySpreadPort = Arc<dyn OptionStrategySpreadReadPort>;
type OptionStrategyPort = Arc<dyn OptionStrategyReadPort>;
type OptionStrategyAnalysisPort = Arc<dyn OptionStrategyAnalysisReadPort>;

#[derive(Default)]
struct PredictionSubscriptionState {
    counts: std::collections::BTreeMap<String, usize>,
    leases: std::collections::BTreeMap<String, PredictionSubscriptionLease>,
    sequence: u64,
}

struct PredictionSubscriptionLease {
    key: String,
    code: String,
    _data_types: Vec<String>,
}

impl PredictionSubscriptionState {
    fn clear(&mut self) {
        self.counts.clear();
        self.leases.clear();
        self.sequence = 0;
    }
}
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
    pub(crate) fn reconciliation_wake(&self) -> Arc<Notify> {
        Arc::clone(&self.reconciliation_wake)
    }

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

    pub(crate) fn market_data_reader_available(&self) -> bool {
        self.market_data_router
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .is_some()
    }

    pub(crate) fn set_historical_klines(&self, reader: Option<Arc<dyn HistoricalKlineReadPort>>) {
        *self
            .historical_klines
            .write()
            .unwrap_or_else(|error| error.into_inner()) = reader;
    }

    pub(crate) fn historical_klines_available(&self) -> bool {
        self.historical_klines
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .is_some()
    }

    /// Return the current OpenD historical-candle reader for consumers that
    /// own a long-lived task but must follow provider activation/replacement.
    /// The reader is cloned from the lock so callers never hold runtime state
    /// while issuing the blocking OpenD request.
    pub(crate) fn historical_klines_reader(&self) -> Option<Arc<dyn HistoricalKlineReadPort>> {
        self.historical_klines
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
    }

    pub(crate) fn set_security_snapshots(&self, reader: Option<Arc<dyn SecuritySnapshotReadPort>>) {
        *self
            .security_snapshots
            .write()
            .unwrap_or_else(|error| error.into_inner()) = reader;
    }

    pub(crate) fn set_news_reader(&self, reader: Option<Arc<dyn FutuNewsReadPort>>) {
        *self
            .news_reader
            .write()
            .unwrap_or_else(|error| error.into_inner()) = reader;
    }

    pub(crate) fn set_prediction_adapters(
        &self,
        reader: Option<Arc<dyn PredictionMarketReadPort>>,
        subscription: Option<Arc<dyn PredictionMarketSubscriptionPort>>,
        combo_quote: Option<Arc<dyn PredictionComboQuotePort>>,
    ) {
        *self.prediction_reader.write().unwrap_or_else(|e| e.into_inner()) = reader;
        *self
            .prediction_subscription
            .write()
            .unwrap_or_else(|e| e.into_inner()) = subscription;
        *self
            .prediction_combo_quote
            .write()
            .unwrap_or_else(|e| e.into_inner()) = combo_quote;
    }

    pub(crate) fn prediction_reader_available(&self) -> bool {
        self.prediction_reader
            .read()
            .unwrap_or_else(|e| e.into_inner())
            .is_some()
    }

    pub(crate) fn prediction_read(
        &self,
        path: &str,
        query: &str,
    ) -> Result<Value, String> {
        self.prediction_reader
            .read()
            .unwrap_or_else(|e| e.into_inner())
            .clone()
            .ok_or_else(|| "Futu prediction market-data reader is unavailable".to_owned())?
            .read(path, query)
            .map_err(|error| error.to_string())
    }

    pub(crate) fn prediction_subscribe(
        &self,
        code: &str,
        data_types: &[String],
    ) -> Result<Value, String> {
        self.prediction_subscription
            .read()
            .unwrap_or_else(|e| e.into_inner())
            .clone()
            .ok_or_else(|| "Futu prediction subscription adapter is unavailable".to_owned())?
            .subscribe(code, data_types)
            .map_err(|error| error.to_string())
    }

    pub(crate) fn prediction_subscription_available(&self) -> bool {
        self.prediction_subscription
            .read()
            .unwrap_or_else(|e| e.into_inner())
            .is_some()
    }

    /// Acquire a prediction subscription with Go-compatible reference-counted
    /// lease semantics. The OpenD subscribe call is issued only for the first
    /// lease for a normalized `(code, dataTypes)` key.
    pub(crate) fn prediction_acquire(
        &self,
        code: &str,
        data_types: &[String],
    ) -> Result<Value, String> {
        let code = normalize_prediction_code(code)?;
        let data_types = normalize_prediction_data_types(data_types)?;
        let key = format!("{}|{}", code, data_types.join(","));
        let mut state = self
            .prediction_subscription_state
            .lock()
            .unwrap_or_else(|error| error.into_inner());
        if !state.counts.contains_key(&key) {
            self.prediction_subscribe(&code, &data_types)?;
            state.counts.insert(key.clone(), 0);
        }
        let count = state.counts.entry(key.clone()).or_insert(0);
        *count = count.saturating_add(1);
        state.sequence = state.sequence.saturating_add(1);
        let lease_id = format!(
            "prediction-lease-{}-{}",
            current_unix_millis(),
            state.sequence
        );
        state.leases.insert(
            lease_id.clone(),
            PredictionSubscriptionLease {
                key,
                code: code.clone(),
                _data_types: data_types.clone(),
            },
        );
        Ok(json!({
            "leaseId": lease_id,
            "instrumentId": format!("US.{code}"),
            "dataTypes": data_types,
            "provider": prediction_provider_value(&data_types),
        }))
    }

    /// Release a prediction lease. Unknown lease ids are intentionally
    /// idempotent; only the final lease invokes OpenD unsubscribe.
    pub(crate) fn prediction_release(&self, lease_id: &str) -> Result<Value, String> {
        let lease_id = lease_id.trim();
        if lease_id.is_empty() {
            return Err("invalid prediction subscription leaseId".to_owned());
        }
        let mut state = self
            .prediction_subscription_state
            .lock()
            .unwrap_or_else(|error| error.into_inner());
        let Some(lease) = state.leases.remove(lease_id) else {
            return Ok(json!({"released": true}));
        };
        let count = state.counts.get(&lease.key).copied().unwrap_or_default();
        if count > 1 {
            state.counts.insert(lease.key, count - 1);
            return Ok(json!({"released": true}));
        }
        state.counts.remove(&lease.key);
        self.prediction_unsubscribe(&lease.code)?;
        Ok(json!({"released": true}))
    }

    pub(crate) fn prediction_unsubscribe(&self, code: &str) -> Result<Value, String> {
        self.prediction_subscription
            .read()
            .unwrap_or_else(|e| e.into_inner())
            .clone()
            .ok_or_else(|| "Futu prediction subscription adapter is unavailable".to_owned())?
            .unsubscribe(code)
            .map_err(|error| error.to_string())
    }

    pub(crate) fn prediction_combo_quote(&self, payload: &Value) -> Result<Value, String> {
        self.prediction_combo_quote
            .read()
            .unwrap_or_else(|e| e.into_inner())
            .clone()
            .ok_or_else(|| "Futu prediction combo quote adapter is unavailable".to_owned())?
            .quote(payload)
            .map_err(|error| error.to_string())
    }

    pub(crate) fn prediction_combo_quote_available(&self) -> bool {
        self.prediction_combo_quote
            .read()
            .unwrap_or_else(|e| e.into_inner())
            .is_some()
    }

    pub(crate) fn news_reader_available(&self) -> bool {
        self.news_reader
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .is_some()
    }

    pub(crate) fn news(&self, query: &FutuNewsQuery) -> Result<FutuNewsResult, String> {
        self.news_reader
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
            .ok_or_else(|| "Futu news runtime is unavailable".to_owned())?
            .query(query)
            .map_err(|error| error.to_string())
    }

    pub(crate) fn set_corporate_actions_reader(
        &self,
        reader: Option<Arc<dyn FutuCorporateActionsReadPort>>,
    ) {
        *self
            .corporate_actions_reader
            .write()
            .unwrap_or_else(|error| error.into_inner()) = reader;
    }

    pub(crate) fn set_customization_readers(
        &self,
        remote: Option<Arc<dyn jftrade_integration_futu::RemoteWatchlistReadPort>>,
        alert: Option<Arc<dyn jftrade_integration_futu::AlertCustomizationReadPort>>,
    ) {
        *self
            .remote_watchlist_reader
            .write()
            .unwrap_or_else(|e| e.into_inner()) = remote;
        *self.alert_reader.write().unwrap_or_else(|e| e.into_inner()) = alert;
    }

    pub(crate) fn set_customization_writers(
        &self,
        remote: Option<Arc<dyn jftrade_integration_futu::RemoteWatchlistWritePort>>,
        alert: Option<Arc<dyn jftrade_integration_futu::AlertCustomizationWritePort>>,
    ) {
        *self
            .remote_watchlist_writer
            .write()
            .unwrap_or_else(|e| e.into_inner()) = remote;
        *self.alert_writer.write().unwrap_or_else(|e| e.into_inner()) = alert;
    }

    pub(crate) fn remote_watchlist_reader(
        &self,
    ) -> Option<Arc<dyn jftrade_integration_futu::RemoteWatchlistReadPort>> {
        self.remote_watchlist_reader
            .read()
            .unwrap_or_else(|e| e.into_inner())
            .clone()
    }
    pub(crate) fn remote_watchlist_writer(
        &self,
    ) -> Option<Arc<dyn jftrade_integration_futu::RemoteWatchlistWritePort>> {
        self.remote_watchlist_writer
            .read()
            .unwrap_or_else(|e| e.into_inner())
            .clone()
    }
    pub(crate) fn alert_reader(
        &self,
    ) -> Option<Arc<dyn jftrade_integration_futu::AlertCustomizationReadPort>> {
        self.alert_reader
            .read()
            .unwrap_or_else(|e| e.into_inner())
            .clone()
    }
    pub(crate) fn alert_writer(
        &self,
    ) -> Option<Arc<dyn jftrade_integration_futu::AlertCustomizationWritePort>> {
        self.alert_writer
            .read()
            .unwrap_or_else(|e| e.into_inner())
            .clone()
    }

    pub(crate) fn corporate_actions_reader_available(&self) -> bool {
        self.corporate_actions_reader
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .is_some()
    }

    pub(crate) fn corporate_actions(
        &self,
        query: &FutuCorporateActionsQuery,
    ) -> Result<FutuCorporateActionsResult, String> {
        self.corporate_actions_reader
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
            .ok_or_else(|| "Futu corporate actions runtime is unavailable".to_owned())?
            .query(query)
            .map_err(|error| error.to_string())
    }

    pub(crate) fn historical_klines(
        &self,
        query: &HistoricalKlineQuery,
    ) -> Result<HistoricalKlineResult, String> {
        let reader = self
            .historical_klines
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
            .ok_or_else(|| "Futu historical klines runtime is unavailable".to_owned())?;
        reader.query(query).map_err(|error| error.to_string())
    }

    pub(crate) fn security_snapshots(
        &self,
        securities: &[TradeSecurity],
    ) -> Result<Vec<Value>, String> {
        if let Some(reader) = self
            .security_snapshots
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
        {
            let instruments = securities
                .iter()
                .filter_map(|security| {
                    qot_market_label(security.market)
                        .map(|market| format!("{market}.{}", security.code))
                })
                .collect::<Vec<_>>();
            let snapshots = reader.query(&instruments)?;
            return snapshots.into_iter().map(security_snapshot_value).collect();
        }
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
            let mut snapshot = Map::from_iter([
                ("symbol".to_owned(), Value::String(tick.instrument_id)),
                (
                    "lastPrice".to_owned(),
                    json!(tick.price.to_f64().map_err(|error| error.to_string())?),
                ),
                ("volume".to_owned(), Value::Number(volume)),
                (
                    "observedAt".to_owned(),
                    Value::String(format_unix_millis_rfc3339(tick.observed_at_ms)),
                ),
            ]);
            if let Some(rich) = tick.snapshot {
                insert_rich_security_fields(&mut snapshot, &rich)?;
            }
            snapshots.push(Value::Object(snapshot));
        }
        Ok(snapshots)
    }

    pub(crate) fn quote_snapshot(
        &self,
        securities: &[TradeSecurity],
        account_id: &str,
    ) -> Result<Value, String> {
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
        let mut quotes = Vec::with_capacity(securities.len());
        for security in securities {
            let market = qot_market_label(security.market)
                .ok_or_else(|| format!("invalid market code {}", security.market))?;
            let instrument_id = format!("{market}.{}", security.code);
            let tick = match cache_guard.lookup(&instrument_id, now_ms, 30_000) {
                CacheLookup::Fresh(tick) | CacheLookup::Stale(tick) => tick,
                CacheLookup::Missing => {
                    return Err(format!("no cached quote available for {instrument_id}"));
                }
            };
            let last_price = tick
                .price
                .to_f64()
                .map_err(|error| format!("invalid cached price for {instrument_id}: {error}"))?;
            let volume = tick
                .volume
                .as_str()
                .parse::<serde_json::Number>()
                .map_err(|error| format!("invalid cached volume for {instrument_id}: {error}"))?;
            let mut item = Map::from_iter([
                ("symbol".to_owned(), Value::String(tick.instrument_id)),
                ("lastPrice".to_owned(), json!(last_price)),
                ("volume".to_owned(), Value::Number(volume)),
                (
                    "quoteAt".to_owned(),
                    Value::String(format_unix_millis_rfc3339(tick.observed_at_ms)),
                ),
            ]);
            if let Some(rich) = tick.snapshot {
                insert_rich_quote_fields(&mut item, &rich)?;
            }
            quotes.push(Value::Object(item));
        }
        let first = quotes
            .first()
            .cloned()
            .ok_or_else(|| "query parameter symbol is required".to_owned())?;
        let mut result = json!({
            "accountId": account_id,
            "symbol": first["symbol"],
            "lastPrice": first["lastPrice"],
            "volume": first["volume"],
            "quoteAt": first["quoteAt"],
            "quotes": quotes,
        });
        if let Some(object) = result.as_object_mut() {
            for key in [
                "symbolName",
                "openPrice",
                "highPrice",
                "lowPrice",
                "lastClose",
                "turnover",
                "marketTime",
            ] {
                if let Some(value) = first.get(key).filter(|value| !value.is_null()) {
                    object.insert(key.to_owned(), value.clone());
                }
            }
        }
        Ok(result)
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

    pub(crate) fn set_writer(&self, writer: Option<Arc<dyn TradeWritePort>>) {
        *self
            .trade_writer
            .write()
            .unwrap_or_else(|error| error.into_inner()) = writer;
    }

    pub(crate) fn writer_snapshot(&self) -> Option<Arc<dyn TradeWritePort>> {
        self.trade_writer
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
    }

    pub(crate) fn clear(&self) {
        *self.state.write().unwrap_or_else(|e| e.into_inner()) = None;
        self.set_writer(None);
        self.set_news_reader(None);
        self.set_prediction_adapters(None, None, None);
        self.prediction_subscription_state
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .clear();
        self.set_corporate_actions_reader(None);
        self.set_customization_readers(None, None);
        self.set_customization_writers(None, None);
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

fn normalize_prediction_code(value: &str) -> Result<String, String> {
    let value = value.trim();
    let value = value.strip_prefix("US.").unwrap_or(value);
    if value.is_empty()
        || value.len() > 512
        || value
            .chars()
            .any(|ch| ch.is_whitespace() || ch.is_control() || matches!(ch, '/' | '\\' | '?' | '#'))
    {
        return Err("invalid prediction subscription code".to_owned());
    }
    Ok(value.to_ascii_uppercase())
}

fn normalize_prediction_data_types(values: &[String]) -> Result<Vec<String>, String> {
    let mut result = Vec::new();
    for value in values {
        let value = value.trim().to_ascii_uppercase();
        if !matches!(value.as_str(), "ORDER_BOOK" | "KLINE" | "TICKER") {
            return Err(format!("unsupported prediction data type {value:?}"));
        }
        if !result.contains(&value) {
            result.push(value);
        }
    }
    if result.is_empty() {
        return Err("at least one prediction data type is required".to_owned());
    }
    result.sort();
    Ok(result)
}

fn prediction_provider_value(data_types: &[String]) -> Value {
    let feature_id = if data_types.len() == 1 && data_types[0] == "ORDER_BOOK" {
        "prediction.depth"
    } else {
        "prediction.history"
    };
    let resolved_at = format_unix_millis_rfc3339(current_unix_millis());
    json!({
        "brokerId": "futu",
        "securityFirm": "Futu/Moomoo via OpenD",
        "featureId": feature_id,
        "capability": "available",
        "selectionReason": "adapter_request",
        "resolvedAt": resolved_at,
        "asOf": resolved_at,
    })
}

#[path = "product_trade_runtime_broker_routes.rs"]
mod product_trade_runtime_broker_routes;

#[cfg(test)]
#[path = "product_trade_runtime_projection_tests.rs"]
mod tests;
