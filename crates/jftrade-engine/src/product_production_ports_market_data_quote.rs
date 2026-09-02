//! Production market-data quote adapter for subscriptions, securities, snapshots, candles, and depth.

use jftrade_calendar::CalendarManager;
use jftrade_integration_marketdata_helper::{
    HelperCandlesResponse, HelperClient, HelperPriceValue, HelperSecurityResponse,
    HelperSnapshotResponse,
};
use jftrade_marketdata::{CacheLookup, ProviderRouter};
use jftrade_integration_futu::{
    MarketMicrostructureError, MarketMicrostructureOperation, MarketMicrostructureReadPort,
};
use jftrade_settings::MarketDataProvider;
use serde_json::{Value, json};

#[path = "product_production_ports_market_data_quote_reads.rs"]
mod quote_reads;
use std::sync::{Arc, Mutex};

use super::product_production_ports_market_data_projection::{
    current_unix_millis, format_unix_millis_rfc3339, map_helper_quote_error,
    parse_market_symbol_path, render_subscriptions_data,
};
use crate::product::product_active_provider_state::ActiveProviderState;
use crate::product::product_query::{
    CandleSessionError, QueryMap, is_intraday_candle_period, normalize_candle_period,
    normalize_optional_query_time, parse_candle_before_time, parse_candle_sessions,
};
use crate::product::{
    MarketDataQuoteReadFuture, MarketDataQuoteReadSnapshotError, MarketDataQuoteReadSnapshotPort,
};

#[derive(Clone)]
pub(crate) struct ProductionMarketDataQuotePort {
    active_provider_state: Arc<ActiveProviderState>,
    router: Option<Arc<Mutex<ProviderRouter>>>,
    helper: Option<HelperClient>,
    physical: Option<Arc<dyn jftrade_marketdata::PhysicalSubscriptionSnapshotPort>>,
    calendar: Option<Arc<CalendarManager>>,
    microstructure: Option<Arc<dyn MarketMicrostructureReadPort>>,
    trade_runtime:
        Option<Arc<super::super::product_production_ports_trade::SharedTradeReadRuntime>>,
}

impl std::fmt::Debug for ProductionMarketDataQuotePort {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("ProductionMarketDataQuotePort")
            .field("router", &self.router.is_some())
            .field("has_helper", &self.helper.is_some())
            .field("has_physical", &self.physical.is_some())
            .field("has_microstructure", &self.microstructure.is_some())
            .field("has_trade_runtime", &self.trade_runtime.is_some())
            .finish()
    }
}

impl ProductionMarketDataQuotePort {
    const MICROSTRUCTURE_PAGE_SIZE_MAX: i64 = 100;
    const TICKS_PAGE_SIZE_MAX: i64 = 1000;
    const DEPTH_NUM_MAX: i64 = 50;

    pub(crate) fn new(
        active_provider_state: Arc<ActiveProviderState>,
        router: Option<Arc<Mutex<ProviderRouter>>>,
        helper: Option<HelperClient>,
        physical: Option<Arc<dyn jftrade_marketdata::PhysicalSubscriptionSnapshotPort>>,
    ) -> Self {
        Self {
            active_provider_state,
            router,
            helper,
            physical,
            calendar: None,
            microstructure: None,
            trade_runtime: None,
        }
    }

    pub(crate) fn with_calendar(mut self, calendar: Arc<CalendarManager>) -> Self {
        self.calendar = Some(calendar);
        self
    }

    pub(crate) fn with_microstructure(
        mut self,
        reader: Option<Arc<dyn MarketMicrostructureReadPort>>,
    ) -> Self {
        self.microstructure = reader;
        self
    }

    pub(crate) fn with_trade_runtime(
        mut self,
        trade_runtime:
            Option<Arc<super::super::product_production_ports_trade::SharedTradeReadRuntime>>,
    ) -> Self {
        self.trade_runtime = trade_runtime;
        self
    }

    fn active_provider(&self) -> Result<MarketDataProvider, MarketDataQuoteReadSnapshotError> {
        self.active_provider_state.get().ok_or_else(|| {
            MarketDataQuoteReadSnapshotError::Unavailable(
                "active market-data provider is not configured".to_owned(),
            )
        })
    }

    fn microstructure_reader(&self) -> Option<Arc<dyn MarketMicrostructureReadPort>> {
        self.trade_runtime
            .as_ref()
            .and_then(|runtime| runtime.market_microstructure_reader())
            .or_else(|| self.microstructure.clone())
    }
}

impl MarketDataQuoteReadSnapshotPort for ProductionMarketDataQuotePort {
    fn read<'a>(&'a self, path: &'a str, query: &'a str) -> MarketDataQuoteReadFuture<'a> {
        Box::pin(async move {
            if path == "/api/v1/market-data/subscriptions" {
                return self.read_subscriptions();
            }
            if let Some(suffix) = path.strip_prefix("/api/v1/market-data/securities/") {
                return self.read_securities(suffix, query).await;
            }
            if let Some(suffix) = path.strip_prefix("/api/v1/market-data/snapshots/") {
                return self.read_snapshots(suffix, query).await;
            }
            if let Some(suffix) = path.strip_prefix("/api/v1/market-data/candles/") {
                return self.read_candles(suffix, query).await;
            }
            if let Some(suffix) = path.strip_prefix("/api/v1/market-data/depth/") {
                return self.read_depth(suffix, query);
            }
            if let Some(suffix) = path.strip_prefix("/api/v1/market-data/broker-queue/") {
                return self.read_broker_feature("market.broker_queue", suffix, query);
            }
            if let Some(suffix) = path.strip_prefix("/api/v1/market-data/capital-flow/") {
                return self.read_broker_feature("market.capital_flow", suffix, query);
            }
            if let Some(suffix) = path.strip_prefix("/api/v1/market-data/intraday/") {
                return self.read_broker_feature("market.intraday", suffix, query);
            }
            if let Some(suffix) = path.strip_prefix("/api/v1/market-data/ticks/") {
                return self.read_ticks(suffix, query);
            }
            if let Some(suffix) = path.strip_prefix("/api/v1/market-data/instruments/")
                && suffix.ends_with("/profile")
            {
                let instrument_id = suffix.trim_end_matches("/profile");
                return self.read_broker_feature("market.instrument_profile", instrument_id, query);
            }

            Err(MarketDataQuoteReadSnapshotError::Unavailable(format!(
                "unsupported market-data quote path: {path}"
            )))
        })
    }
}

impl ProductionMarketDataQuotePort {
    fn read_subscriptions(&self) -> Result<Value, MarketDataQuoteReadSnapshotError> {
        let router = self.router.as_ref();
        if router.is_none() {
            let snapshot = self.active_provider_state.snapshot();
            if !snapshot.helper_ready
                || !matches!(
                    snapshot.provider,
                    Some(MarketDataProvider::Yfinance) | Some(MarketDataProvider::Akshare)
                )
            {
                return Err(MarketDataQuoteReadSnapshotError::Unavailable(
                    "market-data subscription provider is not configured".to_owned(),
                ));
            }
            let mut response = render_subscriptions_data(
                &jftrade_marketdata::DemandSnapshot::default(),
                None,
            );
            response["transport"] = json!({"mode": "snapshot-poll-fallback"});
            return Ok(response);
        }
        let physical_snapshot = self
            .physical
            .as_ref()
            .map(|p| {
                p.physical_subscription_snapshot().map_err(|error| {
                    MarketDataQuoteReadSnapshotError::Failed {
                        status: 500,
                        code: "SUBSCRIPTION_FAILED".to_owned(),
                        message: error,
                        retry_after_seconds: None,
                    }
                })
            })
            .transpose()?
            .flatten();
        let demand = if let Some(router) = router {
            router
                .lock()
                .map_err(|error| MarketDataQuoteReadSnapshotError::Failed {
                    status: 500,
                    code: "SUBSCRIPTION_FAILED".to_owned(),
                    message: format!("failed to lock router: {error}"),
                    retry_after_seconds: None,
                })?
                .demand()
        } else {
            unreachable!("router absence handled by polling fallback above");
        };
        Ok(render_subscriptions_data(
            &demand,
            physical_snapshot.as_ref(),
        ))
    }

    async fn read_securities(
        &self,
        suffix: &str,
        _query: &str,
    ) -> Result<Value, MarketDataQuoteReadSnapshotError> {
        let (market, symbol) = parse_market_symbol_path(suffix)?;
        let provider = self.active_provider()?;

        if let Some(helper) = &self.helper
            && (provider == MarketDataProvider::Yfinance || provider == MarketDataProvider::Akshare)
        {
            let provider_str = match provider {
                MarketDataProvider::Yfinance => "yfinance",
                MarketDataProvider::Akshare => "akshare",
                MarketDataProvider::Futu => "futu",
            };
            let resp = helper
                .get_provider_json::<HelperSecurityResponse>(
                    provider_str,
                    &["security", &market, &symbol],
                )
                .await
                .map_err(|error| map_helper_quote_error(error, "MARKET_SECURITY_DETAILS_FAILED"))?;

            if resp.name.trim().is_empty() {
                return Err(MarketDataQuoteReadSnapshotError::Failed {
                    status: 502,
                    code: "MARKET_SECURITY_DETAILS_FAILED".to_owned(),
                    message: format!("missing name for {market}.{symbol}"),
                    retry_after_seconds: None,
                });
            }

            return Ok(json!({
                "meta": {
                    "source": resp.source,
                },
                "request": {
                    "instrumentId": format!("{market}.{symbol}"),
                    "market": market,
                    "symbol": symbol,
                },
                "security": {
                    "currency": resp.currency,
                    "exchange": resp.exchange,
                    "instrumentId": format!("{market}.{symbol}"),
                    "market": market,
                    "name": resp.name,
                    "securityType": resp.security_type,
                    "supportedPeriods": resp.supported_periods,
                    "symbol": symbol,
                    "timezone": resp.timezone,
                },
            }));
        }

        Err(MarketDataQuoteReadSnapshotError::Unavailable(format!(
            "security details provider is unavailable for {market}.{symbol}"
        )))
    }


    fn read_depth(
        &self,
        suffix: &str,
        query: &str,
    ) -> Result<Value, MarketDataQuoteReadSnapshotError> {
        let (market, symbol) = parse_market_symbol_path(suffix)?;
        let query_map =
            QueryMap::parse(query).map_err(|_| MarketDataQuoteReadSnapshotError::Failed {
                status: 400,
                code: "BAD_REQUEST".to_owned(),
                message: "invalid URL escape".to_owned(),
                retry_after_seconds: None,
            })?;
        let num = parse_bounded_query_i64(
            &query_map,
            "num",
            Self::DEPTH_NUM_MAX,
        )?
        .unwrap_or(10);

        let provider = self.active_provider()?;
        if provider == MarketDataProvider::Futu {
            let instrument_id = format!(
                "{}.{}",
                market.to_ascii_uppercase(),
                symbol.to_ascii_uppercase()
            );
            self.require_order_book_subscription(&instrument_id)?;
            let params = json!({"num": num});
            let reader = self
                .microstructure_reader()
                .ok_or_else(|| {
                    MarketDataQuoteReadSnapshotError::Unavailable(
                        "Futu market depth reader is unavailable".to_owned(),
                    )
                })?;
            return reader
                .query(MarketMicrostructureOperation::Depth, &instrument_id, &params)
                .map_err(|error| map_microstructure_error(error, "OPEND_DEPTH_FAILED"));
        }

        Err(MarketDataQuoteReadSnapshotError::Unavailable(format!(
            "depth provider is not supported for {market}.{symbol}"
        )))
    }

    fn read_broker_feature(
        &self,
        feature_name: &str,
        _instrument_id: &str,
        query: &str,
    ) -> Result<Value, MarketDataQuoteReadSnapshotError> {
        let query_map =
            QueryMap::parse(query).map_err(|_| MarketDataQuoteReadSnapshotError::Failed {
                status: 400,
                code: "BAD_REQUEST".to_owned(),
                message: "invalid URL escape".to_owned(),
                retry_after_seconds: None,
            })?;
        let page_size = parse_bounded_query_i64(
            &query_map,
            "pageSize",
            Self::MICROSTRUCTURE_PAGE_SIZE_MAX,
        )?;
        let period_type = parse_optional_query_i32(&query_map, "periodType")?;
        let begin_time = validate_optional_query_time(&query_map, "beginTime")?;
        let end_time = validate_optional_query_time(&query_map, "endTime")?;
        let provider = self.active_provider()?;
        if provider != MarketDataProvider::Futu {
            let broker_id = query_map.get_first("brokerId").unwrap_or("api-test");
            return Err(MarketDataQuoteReadSnapshotError::Failed {
                status: 409,
                code: "BROKER_CAPABILITY_UNAVAILABLE".to_owned(),
                message: format!("broker feature capability is unavailable: feature \"{feature_name}\" with broker \"{broker_id}\" is not registered"),
                retry_after_seconds: None,
            });
        }
        let operation = match feature_name {
            "market.broker_queue" => MarketMicrostructureOperation::BrokerQueue,
            "market.capital_flow" => match query_map.get_first("operation") {
                None | Some("") | Some("flow") => MarketMicrostructureOperation::CapitalFlow,
                Some("distribution") => MarketMicrostructureOperation::CapitalDistribution,
                Some(operation) => {
                    return Err(MarketDataQuoteReadSnapshotError::Failed {
                        status: 400,
                        code: "BAD_REQUEST".to_owned(),
                        message: format!("unsupported capital flow operation {operation:?}"),
                        retry_after_seconds: None,
                    });
                }
            },
            "market.intraday" => MarketMicrostructureOperation::Intraday,
            "market.instrument_profile" => MarketMicrostructureOperation::Profile,
            _ => return Err(MarketDataQuoteReadSnapshotError::Unavailable("unsupported market microstructure feature".to_owned())),
        };
        let instrument = _instrument_id.trim();
        let instrument_id = if instrument.contains('.') { instrument.to_ascii_uppercase() } else { format!("US.{}", instrument.to_ascii_uppercase()) };
        let reader = self
            .microstructure_reader()
            .ok_or_else(|| {
                MarketDataQuoteReadSnapshotError::Unavailable(
                    "Futu market microstructure reader is unavailable".to_owned(),
                )
            })?;
        let mut params = serde_json::Map::new();
        if let Some(value) = page_size {
            params.insert("pageSize".to_owned(), json!(value));
        }
        if let Some(value) = period_type {
            params.insert("periodType".to_owned(), json!(value));
        }
        if let Some(value) = begin_time {
            params.insert("beginTime".to_owned(), json!(value));
        }
        if let Some(value) = end_time {
            params.insert("endTime".to_owned(), json!(value));
        }
        reader
            .query(operation, &instrument_id, &Value::Object(params))
            .map_err(|error| map_microstructure_error(error, "OPEND_MARKET_FEATURE_FAILED"))
    }

    fn read_ticks(
        &self,
        suffix: &str,
        query: &str,
    ) -> Result<Value, MarketDataQuoteReadSnapshotError> {
        let query_map = QueryMap::parse(query).map_err(|_| MarketDataQuoteReadSnapshotError::Failed { status: 400, code: "BAD_REQUEST".to_owned(), message: "invalid URL escape".to_owned(), retry_after_seconds: None })?;
        let page_size = parse_bounded_query_i64(
            &query_map,
            "pageSize",
            Self::TICKS_PAGE_SIZE_MAX,
        )?
        .unwrap_or(100);
        let provider = self.active_provider()?;
        if provider != MarketDataProvider::Futu {
            let broker_id = query_map.get_first("brokerId").unwrap_or("api-test");
            return Err(MarketDataQuoteReadSnapshotError::Failed { status: 409, code: "BROKER_CAPABILITY_UNAVAILABLE".to_owned(), message: format!("broker feature capability is unavailable: feature \"market.ticks\" with broker \"{broker_id}\" is not registered"), retry_after_seconds: None });
        }
        let instrument_id = if suffix.contains('.') { suffix.to_ascii_uppercase() } else { format!("US.{}", suffix.to_ascii_uppercase()) };
        let reader = self
            .microstructure_reader()
            .ok_or_else(|| {
                MarketDataQuoteReadSnapshotError::Unavailable(
                    "Futu market ticks reader is unavailable".to_owned(),
                )
            })?;
        reader
            .query(
                MarketMicrostructureOperation::Ticks,
                &instrument_id,
                &json!({"pageSize": page_size}),
            )
            .map_err(|error| map_microstructure_error(error, "OPEND_TICKS_FAILED"))
    }

    fn require_order_book_subscription(
        &self,
        instrument_id: &str,
    ) -> Result<(), MarketDataQuoteReadSnapshotError> {
        let Some(router) = self.router.as_ref() else {
            return Err(MarketDataQuoteReadSnapshotError::Unavailable(
                "market-data provider router is not configured".to_owned(),
            ));
        };
        let demand = router
            .lock()
            .map_err(|error| MarketDataQuoteReadSnapshotError::Failed {
                status: 500,
                code: "MARKET_DATA_SUBSCRIPTION_FAILED".to_owned(),
                message: format!("failed to lock market-data provider router: {error}"),
                retry_after_seconds: None,
            })?
            .demand();
        if demand.entries.iter().any(|entry| {
            entry.channel.eq_ignore_ascii_case("ORDER_BOOK")
                && entry.instrument_id.eq_ignore_ascii_case(instrument_id)
        }) {
            return Ok(());
        }
        Err(MarketDataQuoteReadSnapshotError::Failed {
            status: 409,
            code: "MARKET_DATA_SUBSCRIPTION_REQUIRED".to_owned(),
            message: format!("ORDER_BOOK subscription required for {instrument_id}"),
            retry_after_seconds: None,
        })
    }
}

fn bad_query(message: impl Into<String>) -> MarketDataQuoteReadSnapshotError {
    MarketDataQuoteReadSnapshotError::Failed {
        status: 400,
        code: "BAD_REQUEST".to_owned(),
        message: message.into(),
        retry_after_seconds: None,
    }
}

fn parse_bounded_query_i64(
    query: &QueryMap,
    key: &str,
    max: i64,
) -> Result<Option<i64>, MarketDataQuoteReadSnapshotError> {
    let Some(raw) = query.get_first(key) else {
        return Ok(None);
    };
    let value = raw
        .trim()
        .parse::<i64>()
        .map_err(|_| bad_query(format!("{key} must be an integer")))?;
    if !(1..=max).contains(&value) {
        return Err(bad_query(format!("{key} must be between 1 and {max}")));
    }
    Ok(Some(value))
}

fn parse_optional_query_i32(
    query: &QueryMap,
    key: &str,
) -> Result<Option<i32>, MarketDataQuoteReadSnapshotError> {
    let Some(raw) = query.get_first(key) else {
        return Ok(None);
    };
    if raw.trim().is_empty() {
        return Err(bad_query(format!("{key} must be an integer")));
    }
    raw.trim()
        .parse::<i32>()
        .map(Some)
        .map_err(|_| bad_query(format!("{key} must be an integer")))
}

fn validate_optional_query_time(
    query: &QueryMap,
    key: &str,
) -> Result<Option<String>, MarketDataQuoteReadSnapshotError> {
    let Some(raw) = query.get_first(key) else {
        return Ok(None);
    };
    normalize_optional_query_time(raw)
        .map_err(|_| bad_query(format!("{key} must be a valid timestamp")))?;
    let trimmed = raw.trim();
    if trimmed.is_empty() {
        Ok(None)
    } else {
        // Validate against the shared query-time grammar but preserve the
        // caller's wire spelling. OpenD's capital-flow protocol expects the
        // original date/time string rather than an RFC3339 conversion.
        Ok(Some(trimmed.to_owned()))
    }
}

fn map_microstructure_error(
    error: MarketMicrostructureError,
    default_code: &str,
) -> MarketDataQuoteReadSnapshotError {
    match error {
        MarketMicrostructureError::Invalid(message) => MarketDataQuoteReadSnapshotError::Failed {
            status: 400,
            code: "BAD_REQUEST".to_owned(),
            message,
            retry_after_seconds: None,
        },
        MarketMicrostructureError::Session(message) => {
            MarketDataQuoteReadSnapshotError::Failed {
                status: 503,
                code: "MARKET_DATA_PROVIDER_UNAVAILABLE".to_owned(),
                message,
                retry_after_seconds: None,
            }
        }
        MarketMicrostructureError::Decode {
            operation: _,
            message,
        } => MarketDataQuoteReadSnapshotError::Failed {
            status: 502,
            code: default_code.to_owned(),
            message,
            retry_after_seconds: None,
        },
        MarketMicrostructureError::Rejected {
            ret_type,
            err_code,
            message,
            ..
        } => {
            let (status, code) = if err_code == 429 {
                (429, "MARKET_DATA_RATE_LIMITED")
            } else if err_code == 400 || ret_type == -500 {
                (400, "BAD_REQUEST")
            } else if ret_type == -100 || ret_type == -200 {
                (503, "MARKET_DATA_PROVIDER_UNAVAILABLE")
            } else {
                (502, default_code)
            };
            let retry_after_seconds = (status == 429)
                .then(|| retry_after_seconds(&message).unwrap_or(1))
                .or_else(|| retry_after_seconds(&message));
            MarketDataQuoteReadSnapshotError::Failed {
                status,
                code: code.to_owned(),
                message,
                retry_after_seconds,
            }
        }
    }
}

fn retry_after_seconds(message: &str) -> Option<u64> {
    let lower = message.to_ascii_lowercase();
    let marker = "retry after ";
    let start = lower.find(marker)? + marker.len();
    let digits = lower[start..]
        .chars()
        .take_while(char::is_ascii_digit)
        .collect::<String>();
    if digits.is_empty() {
        return None;
    }
    digits.parse::<u64>().ok().map(|seconds| seconds.max(1))
}
