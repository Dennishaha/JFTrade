//! Shared production trade runtime state and market-data projections.

use std::sync::{Arc, Mutex, RwLock};

use jftrade_api::LiveHub;
use jftrade_integration_futu::{
    HistoricalKlineQuery, HistoricalKlineReadPort, HistoricalKlineResult, TradeReadPort,
    TradeSecurity,
};
use jftrade_marketdata::{CacheLookup, ProviderRouter};
use jftrade_settings::FutuIntegrationConfig;
use serde_json::{Value, json};

use crate::product::product_query::{
    normalize_candle_period, normalize_optional_query_time, parse_candle_before_time,
};

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
    historical_klines: Arc<RwLock<Option<Arc<dyn HistoricalKlineReadPort>>>>,
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

    pub(crate) fn set_historical_klines(&self, reader: Option<Arc<dyn HistoricalKlineReadPort>>) {
        *self
            .historical_klines
            .write()
            .unwrap_or_else(|error| error.into_inner()) = reader;
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
            quotes.push(json!({
                "symbol": tick.instrument_id,
                "lastPrice": last_price,
                "volume": Value::Number(volume),
                "quoteAt": format_unix_millis_rfc3339(tick.observed_at_ms),
            }));
        }
        let first = quotes
            .first()
            .cloned()
            .ok_or_else(|| "query parameter symbol is required".to_owned())?;
        Ok(json!({
            "accountId": account_id,
            "symbol": first["symbol"],
            "lastPrice": first["lastPrice"],
            "volume": first["volume"],
            "quoteAt": first["quoteAt"],
            "quotes": quotes,
        }))
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

impl super::ProductionBrokerPort {
    pub(super) fn read_securities_route(
        &self,
        request: &super::TradeRequest,
    ) -> Result<Value, super::BrokerReadSnapshotError> {
        let securities = request
            .securities()
            .map_err(super::BrokerReadSnapshotError::Invalid)?;
        let runtime = self.trade_runtime.as_ref().ok_or_else(|| {
            super::unavailable("Futu market-data runtime is unavailable")
        })?;
        let snapshots = runtime
            .security_snapshots(&securities)
            .map_err(super::unavailable)?;
        Ok(json!({
            "checkedAt": super::checked_at(),
            "connectivity": "connected",
            "securities": {
                "accountId": request.account_id().unwrap_or_default(),
                "snapshots": snapshots,
            },
        }))
    }

    pub(super) fn read_quote_route(
        &self,
        request: &super::TradeRequest,
    ) -> Result<Value, super::BrokerReadSnapshotError> {
        let securities = request
            .securities()
            .map_err(super::BrokerReadSnapshotError::Invalid)?;
        let runtime = self.trade_runtime.as_ref().ok_or_else(|| {
            super::unavailable("Futu market-data runtime is unavailable")
        })?;
        let quote = runtime
            .quote_snapshot(&securities, request.account_id().unwrap_or_default())
            .map_err(super::unavailable)?;
        Ok(json!({
            "checkedAt": super::checked_at(),
            "connectivity": "connected",
            "quote": quote,
        }))
    }

    pub(super) fn read_klines_route(
        &self,
        request: &super::TradeRequest,
    ) -> Result<Value, super::BrokerReadSnapshotError> {
        let symbol = request
            .query
            .get_first("symbol")
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .ok_or_else(|| super::BrokerReadSnapshotError::Invalid(
                "query parameter symbol is required".to_owned(),
            ))?;
        let period = request.query.get_first("period").unwrap_or("1d");
        normalize_candle_period(period).map_err(|error| {
            super::BrokerReadSnapshotError::Invalid(format!("invalid candle period: {error:?}"))
        })?;
        if let Some(raw_limit) = request.query.get_first("limit") {
            raw_limit.trim().parse::<i32>().map_err(|_| {
                super::BrokerReadSnapshotError::Invalid(
                    "query parameter limit is invalid".to_owned(),
                )
            })?;
        }
        let before = request.query.get_first("before").unwrap_or("");
        let from = request.query.get_first("fromTime").unwrap_or("");
        let to = request.query.get_first("toTime").unwrap_or("");
        if !before.trim().is_empty() && (!from.trim().is_empty() || !to.trim().is_empty()) {
            return Err(super::BrokerReadSnapshotError::Invalid(
                "beforeTime cannot be combined with fromTime or toTime".to_owned(),
            ));
        }
        if !before.trim().is_empty() {
            parse_candle_before_time(before).map_err(|_| {
                super::BrokerReadSnapshotError::Invalid(
                    "before must be an RFC3339 timestamp".to_owned(),
                )
            })?;
        }
        for value in [from, to] {
            if !value.trim().is_empty() {
                normalize_optional_query_time(value).map_err(|_| {
                    super::BrokerReadSnapshotError::Invalid(
                        "fromTime and toTime must be valid timestamps".to_owned(),
                    )
                })?;
            }
        }
        let runtime = self.trade_runtime.as_ref().ok_or_else(|| {
            super::unavailable("Futu historical klines runtime is unavailable")
        })?;
        let (market, code) = symbol.split_once('.').ok_or_else(|| {
            super::BrokerReadSnapshotError::Invalid("symbol must be MARKET.CODE".to_owned())
        })?;
        let market = market.trim().to_ascii_uppercase();
        let market_code = super::quote_market_code(&market).ok_or_else(|| {
            super::BrokerReadSnapshotError::Invalid("symbol market is unsupported".to_owned())
        })?;
        let period = normalize_candle_period(period).map_err(|error| {
            super::BrokerReadSnapshotError::Invalid(format!("invalid candle period: {error:?}"))
        })?;
        let limit = request
            .query
            .get_first("limit")
            .and_then(|raw| raw.trim().parse::<i32>().ok())
            .filter(|value| *value > 0)
            .map(|value| value.min(1000));
        let before = request.query.get_first("before").unwrap_or("").trim();
        let begin = request.query.get_first("fromTime").unwrap_or("").trim();
        let end = request.query.get_first("toTime").unwrap_or("").trim();
        let (begin_time, end_time) = if !before.is_empty() {
            ("1970-01-01 00:00:00".to_owned(), before.to_owned())
        } else {
            (
                begin.to_owned().if_empty_then("1970-01-01 00:00:00"),
                end.to_owned().if_empty_then("2999-12-31 23:59:59"),
            )
        };
        let historical = runtime.historical_klines(&HistoricalKlineQuery {
            market: market_code,
            symbol: code.trim().to_ascii_uppercase(),
            period: period.to_owned(),
            adjustment: match request.query.get_first("adjustment").unwrap_or("forward") {
                "none" => 0,
                "backward" => 2,
                _ => 1,
            },
            begin_time,
            end_time,
            max_ack_kl_num: limit,
            next_req_key: Vec::new(),
            extended_time: None,
            session: None,
        }).map_err(super::unavailable)?;
        Ok(json!({
            "checkedAt": super::checked_at(),
            "connectivity": "connected",
            "klines": historical_snapshot(request, &historical, period),
        }))
    }
}

trait EmptyTime {
    fn if_empty_then(self, fallback: &str) -> String;
}

impl EmptyTime for String {
    fn if_empty_then(self, fallback: &str) -> String {
        if self.is_empty() { fallback.to_owned() } else { self }
    }
}

fn historical_snapshot(
    request: &super::TradeRequest,
    result: &HistoricalKlineResult,
    period: &str,
) -> Value {
    let mut rows = Vec::with_capacity(result.klines.len());
    for candle in &result.klines {
        if candle.is_blank { continue; }
        let mut row = serde_json::Map::new();
        row.insert("time".to_owned(), json!(candle.time));
        if let Some(value) = candle.open_price { row.insert("open".to_owned(), json!(value)); }
        if let Some(value) = candle.close_price { row.insert("close".to_owned(), json!(value)); }
        if let Some(value) = candle.high_price { row.insert("high".to_owned(), json!(value)); }
        if let Some(value) = candle.low_price { row.insert("low".to_owned(), json!(value)); }
        if let Some(value) = candle.volume { row.insert("volume".to_owned(), json!(value as f64)); }
        if let Some(value) = candle.turnover { row.insert("turnover".to_owned(), json!(value)); }
        if let Some(value) = candle.change_rate { row.insert("changeRate".to_owned(), json!(value)); }
        rows.push(Value::Object(row));
    }
    let next_before = rows.first().and_then(|row| row["time"].as_str()).map(str::to_owned);
    let has_more = !result.next_req_key.is_empty();
    let pagination = if has_more {
        json!({"hasMore": true, "nextBefore": next_before})
    } else {
        json!({"hasMore": false})
    };
    json!({
        "accountId": request.account_id().unwrap_or_default(),
        "symbol": format!("{}.{}", super::qot_market_label(result.security.market).unwrap_or("UNKNOWN"), result.security.code),
        "period": period,
        "klines": rows,
        "pagination": pagination,
        "extendedHours": false,
        "session": "regular",
    })
}
