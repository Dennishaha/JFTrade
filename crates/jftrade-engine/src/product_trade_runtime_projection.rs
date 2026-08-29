//! Shared production trade runtime state and market-data projections.

use std::sync::{Arc, Mutex, RwLock};

use jftrade_api::LiveHub;
use jftrade_integration_futu::{
    HistoricalKlineQuery, HistoricalKlineReadPort, HistoricalKlineResult, SecuritySnapshotReadPort,
    OptionExerciseProbabilityReadPort, OptionUnderlyingOverviewReadPort,
    OptionUnderlyingRankReadPort, OptionContractRankReadPort, OptionUnderlyingHisVolatilityReadPort,
    OptionStrategySpreadReadPort, OptionStrategyReadPort, OptionStrategyAnalysisReadPort,
    TradeReadPort, TradeSecurity,
};
use jftrade_marketdata::{CacheLookup, ProviderRouter};
use jftrade_settings::FutuIntegrationConfig;
use serde_json::{Map, Value, json};
use crate::product::product_query::{
    normalize_candle_period, normalize_optional_query_time, parse_candle_before_time, QueryMap,
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
    security_snapshots: Arc<RwLock<Option<Arc<dyn SecuritySnapshotReadPort>>>>,
    pub(crate) option_expirations:
        Arc<RwLock<Option<Arc<dyn jftrade_integration_futu::OptionExpirationReadPort>>>>,
    pub(crate) option_chains:
        Arc<RwLock<Option<Arc<dyn jftrade_integration_futu::OptionChainReadPort>>>>,
    pub(crate) option_screens:
        Arc<RwLock<Option<Arc<dyn jftrade_integration_futu::OptionScreenReadPort>>>>,
    pub(crate) option_quotes: Arc<RwLock<Option<Arc<dyn jftrade_integration_futu::OptionQuoteReadPort>>>>,
    pub(crate) option_volatility: Arc<RwLock<Option<Arc<dyn jftrade_integration_futu::OptionVolatilityReadPort>>>>,
    pub(crate) option_exercise_probability: Arc<RwLock<Option<Arc<dyn OptionExerciseProbabilityReadPort>>>>,
    pub(crate) option_underlying_overview: Arc<RwLock<Option<Arc<dyn OptionUnderlyingOverviewReadPort>>>>,
    pub(crate) option_underlying_his_volatility: Arc<RwLock<Option<OptionUnderlyingHisVolatilityPort>>>,
    pub(crate) option_strategy_spread: Arc<RwLock<Option<OptionStrategySpreadPort>>>,
    pub(crate) option_strategy: Arc<RwLock<Option<OptionStrategyPort>>>,
    pub(crate) option_strategy_analysis: Arc<RwLock<Option<OptionStrategyAnalysisPort>>>,
    pub(crate) option_underlying_rank: Arc<RwLock<Option<Arc<dyn OptionUnderlyingRankReadPort>>>>,
    pub(crate) option_contract_rank: Arc<RwLock<Option<Arc<dyn OptionContractRankReadPort>>>>,
    pub(crate) option_events: Arc<RwLock<Option<Arc<dyn jftrade_integration_futu::OptionEventReadPort>>>>,
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
type OptionStrategySpreadPort = Arc<dyn OptionStrategySpreadReadPort>;
type OptionStrategyPort = Arc<dyn OptionStrategyReadPort>;
type OptionStrategyAnalysisPort = Arc<dyn OptionStrategyAnalysisReadPort>;
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

    pub(crate) fn set_security_snapshots(
        &self,
        reader: Option<Arc<dyn SecuritySnapshotReadPort>>,
    ) {
        *self.security_snapshots.write().unwrap_or_else(|error| error.into_inner()) = reader;
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
                .filter_map(|security| qot_market_label(security.market).map(|market| format!("{market}.{}", security.code)))
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
                ("lastPrice".to_owned(), json!(tick.price.to_f64().map_err(|error| error.to_string())?)),
                ("volume".to_owned(), Value::Number(volume)),
                ("observedAt".to_owned(), Value::String(format_unix_millis_rfc3339(tick.observed_at_ms))),
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
                ("quoteAt".to_owned(), Value::String(format_unix_millis_rfc3339(tick.observed_at_ms))),
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
            for key in ["symbolName", "openPrice", "highPrice", "lowPrice", "lastClose", "turnover", "marketTime"] {
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

fn insert_rich_quote_fields(
    item: &mut Map<String, Value>,
    rich: &jftrade_marketdata::TradeQuoteSnapshot,
) -> Result<(), String> {
    if let Some(name) = rich.name.as_ref() {
        item.insert("symbolName".to_owned(), Value::String(name.clone()));
    }
    for (key, value) in [
        ("openPrice", rich.open_price),
        ("highPrice", rich.high_price),
        ("lowPrice", rich.low_price),
        ("lastClose", rich.previous_close),
    ] {
        if let Some(value) = value {
            item.insert(key.to_owned(), json!(value.to_f64().map_err(|error| error.to_string())?));
        }
    }
    if let Some(turnover) = rich.turnover.as_ref() {
        item.insert("turnover".to_owned(), decimal_number(turnover)?);
    }
    if let Some(update_time) = rich.update_time.as_ref() {
        item.insert("marketTime".to_owned(), Value::String(update_time.clone()));
    }
    for (key, value) in [
        ("preMarket", rich.pre_market.as_ref()),
        ("afterMarket", rich.after_market.as_ref()),
        ("overnight", rich.overnight.as_ref()),
    ] {
        if let Some(value) = value {
            item.insert(key.to_owned(), extended_value(value)?);
        }
    }
    Ok(())
}

fn extended_value(value: &jftrade_marketdata::ExtendedQuoteSnapshot) -> Result<Value, String> {
    let mut result = Map::new();
    for (key, number) in [("price", value.price), ("highPrice", value.high_price), ("lowPrice", value.low_price)] {
        if let Some(number) = number {
            result.insert(key.to_owned(), json!(number.to_f64().map_err(|error| error.to_string())?));
        }
    }
    for (key, number) in [("volume", value.volume.as_ref()), ("turnover", value.turnover.as_ref()), ("change", value.change.as_ref()), ("changeRate", value.change_rate.as_ref()), ("amplitude", value.amplitude.as_ref())] {
        if let Some(number) = number {
            result.insert(key.to_owned(), decimal_number(number)?);
        }
    }
    Ok(Value::Object(result))
}

fn insert_rich_security_fields(
    item: &mut Map<String, Value>,
    rich: &jftrade_marketdata::TradeQuoteSnapshot,
) -> Result<(), String> {
    insert_rich_quote_fields(item, rich)?;
    if let Some(name) = rich.name.as_ref() {
        item.remove("symbolName");
        item.insert("name".to_owned(), Value::String(name.clone()));
    }
    if let Some(previous_close) = item.remove("lastClose") {
        item.insert("previousClose".to_owned(), previous_close);
    }
    if let Some(value) = rich.is_suspended {
        item.insert("isSuspended".to_owned(), Value::Bool(value));
    }
    if let Some(status) = rich.status {
        item.insert("status".to_owned(), json!(status));
    }
    if let Some(update_time) = rich.update_time.as_ref() {
        item.insert("updateTime".to_owned(), Value::String(update_time.clone()));
    }
    item.remove("marketTime");
    Ok(())
}

fn security_snapshot_value(
    snapshot: jftrade_marketdata::BrokerSecuritySnapshot,
) -> Result<Value, String> {
    let mut item = Map::new();
    if let Some(symbol) = snapshot.symbol {
        item.insert("symbol".to_owned(), Value::String(symbol));
    }
    if let Some(market) = snapshot.market {
        item.insert("market".to_owned(), Value::String(market));
    }
    if let Some(name) = snapshot.name {
        item.insert("name".to_owned(), Value::String(name));
    }
    for (key, value) in [
        ("lastPrice", snapshot.last_price),
        ("bidPrice", snapshot.bid_price),
        ("askPrice", snapshot.ask_price),
        ("openPrice", snapshot.open_price),
        ("highPrice", snapshot.high_price),
        ("lowPrice", snapshot.low_price),
        ("previousClose", snapshot.previous_close),
    ] {
        if let Some(value) = value {
            item.insert(key.to_owned(), json!(value.to_f64().map_err(|error| error.to_string())?));
        }
    }
    if let Some(turnover) = snapshot.turnover.as_ref() {
        item.insert("turnover".to_owned(), decimal_number(turnover)?);
    }
    if let Some(volume) = snapshot.volume.as_ref() {
        item.insert("volume".to_owned(), decimal_number(volume)?);
    }
    if let Some(status) = snapshot.status {
        item.insert("status".to_owned(), json!(status));
    }
    if let Some(value) = snapshot.is_suspended {
        item.insert("isSuspended".to_owned(), Value::Bool(value));
    }
    if let Some(value) = snapshot.lot_size {
        item.insert("lotSize".to_owned(), json!(value));
    }
    if let Some(value) = snapshot.security_type {
        item.insert("securityType".to_owned(), Value::String(value));
    }
    if let Some(value) = snapshot.update_time {
        item.insert("updateTime".to_owned(), Value::String(value));
    }
    if let Some(value) = snapshot.pe_rate.as_ref() {
        item.insert("peRate".to_owned(), decimal_number(value)?);
    }
    if let Some(value) = snapshot.pb_rate.as_ref() {
        item.insert("pbRate".to_owned(), decimal_number(value)?);
    }
    Ok(Value::Object(item))
}

fn decimal_number(value: &jftrade_kernel::DecimalText) -> Result<Value, String> {
    value
        .as_str()
        .parse::<serde_json::Number>()
        .map(Value::Number)
        .map_err(|error| format!("invalid cached decimal {}: {error}", value.as_str()))
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
        let requested_limit = request
            .query
            .get_first("limit")
            .and_then(|raw| raw.trim().parse::<i32>().ok())
            .filter(|value| *value > 0)
            .map(|value| value.min(1000));
        let limit = requested_limit.map_or(500, |value| value.max(200));
        let extended_hours = market == "US" && crate::product::product_query::is_intraday_candle_period(period);
        let sessions = parse_requested_sessions(&request.query, extended_hours)
            .map_err(super::BrokerReadSnapshotError::Invalid)?;
        let before = request.query.get_first("before").unwrap_or("").trim();
        let begin = request.query.get_first("fromTime").unwrap_or("").trim();
        let end = request.query.get_first("toTime").unwrap_or("").trim();
        let (begin_time, end_time) = if !before.is_empty() {
            ("1970-01-01 00:00:00".to_owned(), super::normalize_history_time(before, &market).map_err(super::BrokerReadSnapshotError::Invalid)?)
        } else {
            (
                super::normalize_history_time(&begin.to_owned().if_empty_then("1970-01-01 00:00:00"), &market).map_err(super::BrokerReadSnapshotError::Invalid)?,
                super::normalize_history_time(&end.to_owned().if_empty_then("2999-12-31 23:59:59"), &market).map_err(super::BrokerReadSnapshotError::Invalid)?,
            )
        };
        let session_code = if !extended_hours { None } else if sessions.len() == 1 {
            Some(match sessions[0] { "regular" => 1, "extended" => 2, _ => 3 })
        } else { Some(3) };
        let adjustment = match request.query.get_first("adjustment").unwrap_or("forward") {
            "none" => 0,
            "backward" => 2,
            "forward" | "" => 1,
            other => return Err(super::BrokerReadSnapshotError::Invalid(format!("invalid candle adjustment {other:?}"))),
        };
        let mut historical = HistoricalKlineResult {
            security: jftrade_integration_futu::HistoricalSecurity { market: market_code, code: code.trim().to_ascii_uppercase() },
            name: None,
            klines: Vec::new(),
            next_req_key: Vec::new(),
        };
        let plans = if extended_hours && sessions.len() > 1 {
            sessions.iter().map(|session| Some(match *session { "regular" => 1, "extended" => 2, _ => 3 })).collect::<Vec<_>>()
        } else { vec![session_code] };
        for plan in plans {
            let mut cursor = Vec::new();
            let mut exhausted = false;
            for _page_number in 0..32 {
                let page = runtime.historical_klines(&HistoricalKlineQuery {
                    market: market_code,
                    symbol: code.trim().to_ascii_uppercase(),
                    period: period.to_owned(),
                    adjustment,
                    begin_time: begin_time.clone(),
                    end_time: end_time.clone(),
                    max_ack_kl_num: Some(limit),
                    next_req_key: cursor.clone(),
                    extended_time: extended_hours.then_some(true),
                    session: plan,
                }).map_err(super::unavailable)?;
                historical.name = historical.name.or(page.name);
                historical.klines.extend(page.klines);
                if page.next_req_key.is_empty() { exhausted = true; break; }
                cursor = page.next_req_key;
                if historical.klines.len() >= usize::try_from(limit).unwrap_or(usize::MAX) {
                    historical.next_req_key = cursor.clone();
                    exhausted = true;
                    break;
                }
                historical.next_req_key = cursor.clone();
            }
            if !exhausted && !cursor.is_empty() {
                return Err(super::unavailable("Futu historical klines pagination exceeded 32 pages"));
            }
        }
        historical.klines.sort_by(|left, right| left.time.cmp(&right.time));
        historical.klines.dedup_by(|left, right| left.time == right.time);
        Ok(json!({
            "checkedAt": super::checked_at(),
            "connectivity": "connected",
            "klines": historical_snapshot(request, &historical, period, extended_hours, &sessions, requested_limit),
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
    extended_hours: bool,
    sessions: &[&str],
    requested_limit: Option<i32>,
) -> Value {
    let mut rows = Vec::with_capacity(result.klines.len());
    for candle in &result.klines {
        if candle.is_blank { continue; }
        let mut row = serde_json::Map::new();
        let market = super::qot_market_label(result.security.market).unwrap_or("UTC");
        row.insert("time".to_owned(), json!(canonical_candle_time(&candle.time, market)));
        if let Some(value) = candle.open_price { row.insert("open".to_owned(), json!(value)); }
        if let Some(value) = candle.close_price { row.insert("close".to_owned(), json!(value)); }
        if let Some(value) = candle.high_price { row.insert("high".to_owned(), json!(value)); }
        if let Some(value) = candle.low_price { row.insert("low".to_owned(), json!(value)); }
        if let Some(value) = candle.volume { row.insert("volume".to_owned(), json!(value as f64)); }
        if let Some(value) = candle.turnover { row.insert("turnover".to_owned(), json!(value)); }
        if let Some(value) = candle.change_rate { row.insert("changeRate".to_owned(), json!(value)); }
        rows.push(Value::Object(row));
    }
    if let Some(limit) = requested_limit {
        let limit = usize::try_from(limit).unwrap_or(0);
        if rows.len() > limit {
            rows = rows.split_off(rows.len() - limit);
        }
    }
    let next_before = rows.first().and_then(|row| row["time"].as_str()).map(str::to_owned);
    let bounded = request.query.get_first("fromTime").is_some_and(|v| !v.trim().is_empty())
        || request.query.get_first("toTime").is_some_and(|v| !v.trim().is_empty());
    let has_more = !bounded && !result.next_req_key.is_empty();
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
        "extendedHours": extended_hours,
        "session": if sessions.len() == 1 { sessions[0] } else if extended_hours { "all" } else { "regular" },
        "sessions": sessions,
    })
}

fn canonical_candle_time(value: &str, market: &str) -> String {
    if value.contains('T') || value.ends_with('Z') {
        return value.to_owned();
    }
    let timezone = match market {
        "US" => "America/New_York",
        "HK" => "Asia/Hong_Kong",
        "SH" | "SZ" | "CN" => "Asia/Shanghai",
        "JP" => "Asia/Tokyo",
        _ => "UTC",
    };
    let Ok(local) = jiff::civil::DateTime::strptime("%Y-%m-%d %H:%M:%S", value) else {
        return value.to_owned();
    };
    let Ok(zoned) = local.in_tz(timezone) else { return value.to_owned(); };
    zoned.timestamp().to_string()
}

fn parse_requested_sessions(
    query: &QueryMap,
    extended_hours: bool,
) -> Result<Vec<&'static str>, String> {
    let mut values = Vec::new();
    for key in ["sessions", "session"] {
        if let Some(items) = query.get_all(key) {
            values.extend(items.iter().flat_map(|item| item.split(',')));
        }
    }
    if values.is_empty() {
        return Ok(if extended_hours { vec!["regular", "extended", "overnight"] } else { vec!["regular"] });
    }
    let mut result = Vec::new();
    for value in values {
        match value.trim().to_ascii_lowercase().as_str() {
            "regular" if !result.contains(&"regular") => result.push("regular"),
            "extended" if extended_hours && !result.contains(&"extended") => result.push("extended"),
            "overnight" if extended_hours && !result.contains(&"overnight") => result.push("overnight"),
            "regular" | "extended" | "overnight" => return Err("requested session is unsupported for this period or market".to_owned()),
            other => return Err(format!("invalid candle session {other:?}")),
        }
    }
    Ok(result)
}

#[cfg(test)]
#[path = "product_trade_runtime_projection_tests.rs"]
mod tests;
