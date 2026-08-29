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
    TradeReadPort, TradeSecurity,
};
use jftrade_marketdata::{CacheLookup, ProviderRouter};
use jftrade_settings::FutuIntegrationConfig;
use serde_json::{Map, Value, json};
use crate::product::product_query::{
    normalize_candle_period, normalize_optional_query_time, parse_candle_before_time,
};
use super::super::product_production_ports_market_data::product_production_ports_market_data_projection::{
    current_unix_millis, format_unix_millis_rfc3339,
};
use super::product_trade_margin_cache::MarginRatioCache;
use super::qot_market_label;

#[path = "product_trade_runtime_candles.rs"]
mod product_trade_runtime_candles;
#[path = "product_trade_runtime_futures.rs"]
mod product_trade_runtime_futures;
#[path = "product_trade_runtime_projection_values.rs"]
mod product_trade_runtime_projection_values;

use product_trade_runtime_projection_values::{
    insert_rich_quote_fields, insert_rich_security_fields, security_snapshot_value,
};
use product_trade_runtime_candles::{
    canonical_candle_time, historical_snapshot, parse_requested_sessions,
};

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

    pub(crate) fn set_security_snapshots(&self, reader: Option<Arc<dyn SecuritySnapshotReadPort>>) {
        *self
            .security_snapshots
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
        let runtime = self
            .trade_runtime
            .as_ref()
            .ok_or_else(|| super::unavailable("Futu market-data runtime is unavailable"))?;
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
        let runtime = self
            .trade_runtime
            .as_ref()
            .ok_or_else(|| super::unavailable("Futu market-data runtime is unavailable"))?;
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
            .ok_or_else(|| {
                super::BrokerReadSnapshotError::Invalid(
                    "query parameter symbol is required".to_owned(),
                )
            })?;
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
        let runtime = self
            .trade_runtime
            .as_ref()
            .ok_or_else(|| super::unavailable("Futu historical klines runtime is unavailable"))?;
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
        let extended_hours =
            market == "US" && crate::product::product_query::is_intraday_candle_period(period);
        let sessions = parse_requested_sessions(&request.query, extended_hours)
            .map_err(super::BrokerReadSnapshotError::Invalid)?;
        let before = request.query.get_first("before").unwrap_or("").trim();
        let begin = request.query.get_first("fromTime").unwrap_or("").trim();
        let end = request.query.get_first("toTime").unwrap_or("").trim();
        let (begin_time, end_time) = if !before.is_empty() {
            (
                "1970-01-01 00:00:00".to_owned(),
                super::normalize_history_time(before, &market)
                    .map_err(super::BrokerReadSnapshotError::Invalid)?,
            )
        } else {
            (
                super::normalize_history_time(
                    &begin.to_owned().if_empty_then("1970-01-01 00:00:00"),
                    &market,
                )
                .map_err(super::BrokerReadSnapshotError::Invalid)?,
                super::normalize_history_time(
                    &end.to_owned().if_empty_then("2999-12-31 23:59:59"),
                    &market,
                )
                .map_err(super::BrokerReadSnapshotError::Invalid)?,
            )
        };
        let session_code = if !extended_hours {
            None
        } else if sessions.len() == 1 {
            Some(match sessions[0] {
                "regular" => 1,
                "extended" => 2,
                _ => 3,
            })
        } else {
            Some(3)
        };
        let adjustment = match request.query.get_first("adjustment").unwrap_or("forward") {
            "none" => 0,
            "backward" => 2,
            "forward" | "" => 1,
            other => {
                return Err(super::BrokerReadSnapshotError::Invalid(format!(
                    "invalid candle adjustment {other:?}"
                )));
            }
        };
        let mut historical = HistoricalKlineResult {
            security: jftrade_integration_futu::HistoricalSecurity {
                market: market_code,
                code: code.trim().to_ascii_uppercase(),
            },
            name: None,
            klines: Vec::new(),
            next_req_key: Vec::new(),
        };
        let plans = if extended_hours && sessions.len() > 1 {
            sessions
                .iter()
                .map(|session| {
                    Some(match *session {
                        "regular" => 1,
                        "extended" => 2,
                        _ => 3,
                    })
                })
                .collect::<Vec<_>>()
        } else {
            vec![session_code]
        };
        for plan in plans {
            let mut cursor = Vec::new();
            let mut exhausted = false;
            for _page_number in 0..32 {
                let page = runtime
                    .historical_klines(&HistoricalKlineQuery {
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
                    })
                    .map_err(super::unavailable)?;
                historical.name = historical.name.or(page.name);
                historical.klines.extend(page.klines);
                if page.next_req_key.is_empty() {
                    exhausted = true;
                    break;
                }
                cursor = page.next_req_key;
                if historical.klines.len() >= usize::try_from(limit).unwrap_or(usize::MAX) {
                    historical.next_req_key = cursor.clone();
                    exhausted = true;
                    break;
                }
                historical.next_req_key = cursor.clone();
            }
            if !exhausted && !cursor.is_empty() {
                return Err(super::unavailable(
                    "Futu historical klines pagination exceeded 32 pages",
                ));
            }
        }
        historical
            .klines
            .sort_by(|left, right| left.time.cmp(&right.time));
        historical
            .klines
            .dedup_by(|left, right| left.time == right.time);
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
        if self.is_empty() {
            fallback.to_owned()
        } else {
            self
        }
    }
}

#[cfg(test)]
#[path = "product_trade_runtime_projection_tests.rs"]
mod tests;
