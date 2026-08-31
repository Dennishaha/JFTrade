use jftrade_marketdata::{InstrumentRef, PhysicalSubscriptionSnapshotPort, ProviderRouter};
use jftrade_settings::MarketDataProvider;
use serde::Deserialize;
use serde_json::{Value, json};
use std::sync::{Arc, Mutex};
use percent_encoding::percent_decode_str;

use super::product_production_ports_market_data_projection::{
    broker_polling_subscription_response, current_unix_millis, render_subscriptions_data,
};
use crate::product::product_active_provider_state::ActiveProviderState;
use crate::product::product_market_data_subscription_mutation_port::{
    MARKET_DATA_SUBSCRIPTION_ACQUIRE_PATH, MARKET_DATA_SUBSCRIPTION_CLEAR_PATH,
    MARKET_DATA_SUBSCRIPTION_HEARTBEAT_PATH, MARKET_DATA_SUBSCRIPTION_RELEASE_PATH,
    MarketDataSubscriptionMutationPort, MarketDataSubscriptionMutationPortError,
    MarketDataSubscriptionMutationRequest,
};
use crate::product::product_query::QueryMap;

#[derive(Clone)]
pub(crate) struct ProductionMarketDataSubscriptionMutationPort {
    active_provider_state: Arc<ActiveProviderState>,
    router: Option<Arc<Mutex<ProviderRouter>>>,
    physical: Option<Arc<dyn PhysicalSubscriptionSnapshotPort>>,
    trade_runtime: Option<Arc<super::super::product_production_ports_trade::SharedTradeReadRuntime>>,
}

impl std::fmt::Debug for ProductionMarketDataSubscriptionMutationPort {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("ProductionMarketDataSubscriptionMutationPort")
            .field("has_router", &self.router.is_some())
            .field("has_physical", &self.physical.is_some())
            .field("has_trade_runtime", &self.trade_runtime.is_some())
            .finish()
    }
}

impl ProductionMarketDataSubscriptionMutationPort {
    pub(crate) fn new(
        active_provider_state: Arc<ActiveProviderState>,
        router: Option<Arc<Mutex<ProviderRouter>>>,
        physical: Option<Arc<dyn PhysicalSubscriptionSnapshotPort>>,
    ) -> Self {
        Self {
            active_provider_state,
            router,
            physical,
            trade_runtime: None,
        }
    }

    pub(crate) fn with_trade_runtime(
        mut self,
        trade_runtime: Option<Arc<super::super::product_production_ports_trade::SharedTradeReadRuntime>>,
    ) -> Self {
        self.trade_runtime = trade_runtime;
        self
    }

    #[allow(dead_code)]
    fn active_provider(
        &self,
    ) -> Result<MarketDataProvider, MarketDataSubscriptionMutationPortError> {
        self.active_provider_state.get().ok_or_else(|| {
            MarketDataSubscriptionMutationPortError::Unavailable(
                "active market-data provider is not configured".to_owned(),
            )
        })
    }

    /// Return the broker id for the explicit polling fallback or for an
    /// active helper provider when no router was composed.  The latter keeps
    /// helper-backed subscription mutations usable without pretending that a
    /// local demand book exists.
    fn polling_provider_id(&self, explicit_broker: Option<&str>) -> Option<String> {
        if let Some(broker) = explicit_broker.map(str::trim).filter(|s| !s.is_empty()) {
            return (!broker.eq_ignore_ascii_case("futu")).then(|| broker.to_ascii_lowercase());
        }
        if self.router.is_some() {
            return None;
        }
        let snapshot = self.active_provider_state.snapshot();
        if !snapshot.helper_ready {
            return None;
        }
        match snapshot.provider {
            Some(MarketDataProvider::Yfinance) => Some("yfinance".to_owned()),
            Some(MarketDataProvider::Akshare) => Some("akshare".to_owned()),
            Some(MarketDataProvider::Futu) | None => None,
        }
    }
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct SubscriptionRequestBody {
    consumer_id: Option<String>,
    provider_broker_id: Option<String>,
    instruments: Option<Vec<RawInstrumentRef>>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct RawInstrumentRef {
    channel: Option<String>,
    market: Option<String>,
    symbol: Option<String>,
    interval: Option<String>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct PredictionSubscriptionRequestBody {
    data_types: Option<Vec<String>>,
}

impl MarketDataSubscriptionMutationPort for ProductionMarketDataSubscriptionMutationPort {
    fn dispatch(
        &self,
        request: &MarketDataSubscriptionMutationRequest,
    ) -> Result<Value, MarketDataSubscriptionMutationPortError> {
        let method = request.method.as_str();
        let path = request.path.as_str();

        if let Some((code, lease_id)) = prediction_subscription_path(path)? {
            return match (method, lease_id) {
                ("POST", None) => self.prediction_acquire(&code, request),
                ("DELETE", Some(lease_id)) => self.prediction_release(&lease_id),
                _ => Err(MarketDataSubscriptionMutationPortError::Unavailable(
                    format!("unsupported prediction subscription route: {method} {path}"),
                )),
            };
        }

        match (method, path) {
            ("POST", MARKET_DATA_SUBSCRIPTION_ACQUIRE_PATH) => self.acquire(request),
            ("POST", MARKET_DATA_SUBSCRIPTION_RELEASE_PATH) => self.release(request),
            ("DELETE", MARKET_DATA_SUBSCRIPTION_CLEAR_PATH) => self.clear(request),
            ("POST", MARKET_DATA_SUBSCRIPTION_HEARTBEAT_PATH) => self.heartbeat(request),
            _ => Err(MarketDataSubscriptionMutationPortError::Unavailable(
                format!("unsupported subscription mutation route: {method} {path}"),
            )),
        }
    }
}

#[cfg(test)]
#[path = "product_production_ports_market_data_subscription_tests.rs"]
mod tests;

impl ProductionMarketDataSubscriptionMutationPort {
    fn prediction_runtime(
        &self,
    ) -> Result<&Arc<super::super::product_production_ports_trade::SharedTradeReadRuntime>, MarketDataSubscriptionMutationPortError> {
        let snapshot = self.active_provider_state.snapshot();
        if snapshot.provider != Some(MarketDataProvider::Futu) || !snapshot.opend_ready {
            return Err(MarketDataSubscriptionMutationPortError::Unavailable(
                "Futu prediction subscription provider is not ready".to_owned(),
            ));
        }
        let runtime = self.trade_runtime.as_ref().ok_or_else(|| {
            MarketDataSubscriptionMutationPortError::Unavailable(
                "Futu prediction subscription runtime is not configured".to_owned(),
            )
        })?;
        if !runtime.prediction_subscription_available() {
            return Err(MarketDataSubscriptionMutationPortError::Unavailable(
                "Futu prediction subscription adapter is not ready".to_owned(),
            ));
        }
        Ok(runtime)
    }

    fn prediction_acquire(
        &self,
        code: &str,
        request: &MarketDataSubscriptionMutationRequest,
    ) -> Result<Value, MarketDataSubscriptionMutationPortError> {
        let runtime = self.prediction_runtime()?;
        let body: PredictionSubscriptionRequestBody =
            serde_json::from_slice(&request.body).map_err(|_| {
                MarketDataSubscriptionMutationPortError::Failed {
                    status: 400,
                    code: "BAD_REQUEST".to_owned(),
                    message: "invalid prediction subscription payload".to_owned(),
                    retry_after_seconds: None,
                }
            })?;
        let data_types = body.data_types.ok_or_else(|| {
            MarketDataSubscriptionMutationPortError::Failed {
                status: 400,
                code: "BAD_REQUEST".to_owned(),
                message: "dataTypes must contain at least one supported type".to_owned(),
                retry_after_seconds: None,
            }
        })?;
        runtime
            .prediction_acquire(code, &data_types)
            .map_err(map_prediction_subscription_error)
    }

    fn prediction_release(
        &self,
        lease_id: &str,
    ) -> Result<Value, MarketDataSubscriptionMutationPortError> {
        let runtime = self.prediction_runtime()?;
        runtime
            .prediction_release(lease_id)
            .map_err(map_prediction_subscription_error)
    }
}

fn map_prediction_subscription_error(
    message: String,
) -> MarketDataSubscriptionMutationPortError {
    let lowercase = message.to_ascii_lowercase();
    if lowercase.contains("invalid")
        || lowercase.contains("required")
        || lowercase.contains("unsupported")
        || lowercase.contains("must be")
    {
        return MarketDataSubscriptionMutationPortError::Failed {
            status: 400,
            code: "BAD_REQUEST".to_owned(),
            message,
            retry_after_seconds: None,
        };
    }
    if lowercase.contains("session unavailable")
        || lowercase.contains("adapter is unavailable")
        || lowercase.contains("runtime is unavailable")
        || lowercase.contains("not ready")
    {
        return MarketDataSubscriptionMutationPortError::Unavailable(message);
    }
    MarketDataSubscriptionMutationPortError::Failed {
        status: 502,
        code: "BROKER_FEATURE_FAILED".to_owned(),
        message,
        retry_after_seconds: None,
    }
}

fn prediction_subscription_path(
    path: &str,
) -> Result<Option<(String, Option<String>)>, MarketDataSubscriptionMutationPortError> {
    const PREFIX: &str = "/api/v1/market-data/prediction/contracts/";
    let Some(suffix) = path.strip_prefix(PREFIX) else {
        return Ok(None);
    };
    let mut segments = suffix.split('/');
    let raw_code = segments.next().unwrap_or_default();
    if raw_code.is_empty() {
        return Err(prediction_bad_request("prediction contract code is invalid"));
    }
    let code = percent_decode_str(raw_code)
        .decode_utf8()
        .map_err(|_| prediction_bad_request("prediction contract code is invalid"))?;
    if code.is_empty()
        || code.len() > 512
        || code.chars().any(|value| {
            value.is_control() || value.is_whitespace() || matches!(value, '/' | '\\' | '?' | '#')
        })
    {
        return Err(prediction_bad_request("prediction contract code is invalid"));
    }
    if segments.next() != Some("subscriptions") {
        return Err(prediction_bad_request("unsupported prediction subscription route"));
    }
    let lease_id = match segments.next() {
        None => None,
        Some(value) if !value.is_empty() && segments.next().is_none() => {
            let decoded = percent_decode_str(value)
                .decode_utf8()
                .map_err(|_| prediction_bad_request("prediction subscription leaseId is invalid"))?;
            if decoded.chars().any(|ch| ch.is_control() || ch.is_whitespace()) {
                return Err(prediction_bad_request("prediction subscription leaseId is invalid"));
            }
            Some(decoded.into_owned())
        }
        _ => return Err(prediction_bad_request("unsupported prediction subscription route")),
    };
    Ok(Some((code.into_owned(), lease_id)))
}

fn prediction_bad_request(message: &str) -> MarketDataSubscriptionMutationPortError {
    MarketDataSubscriptionMutationPortError::Failed {
        status: 400,
        code: "BAD_REQUEST".to_owned(),
        message: message.to_owned(),
        retry_after_seconds: None,
    }
}

impl ProductionMarketDataSubscriptionMutationPort {
    fn acquire(
        &self,
        request: &MarketDataSubscriptionMutationRequest,
    ) -> Result<Value, MarketDataSubscriptionMutationPortError> {
        let body: SubscriptionRequestBody =
            serde_json::from_slice(&request.body).map_err(|_| {
                MarketDataSubscriptionMutationPortError::Failed {
                    status: 400,
                    code: "BAD_REQUEST".to_owned(),
                    message: "invalid subscription request".to_owned(),
                    retry_after_seconds: None,
                }
            })?;

        let consumer_id = body
            .consumer_id
            .as_deref()
            .map(str::trim)
            .filter(|c| !c.is_empty())
            .ok_or_else(|| MarketDataSubscriptionMutationPortError::Failed {
                status: 400,
                code: "BAD_REQUEST".to_owned(),
                message: "consumerId and instruments are required".to_owned(),
                retry_after_seconds: None,
            })?;

        let raw_instruments =
            body.instruments
                .ok_or_else(|| MarketDataSubscriptionMutationPortError::Failed {
                    status: 400,
                    code: "BAD_REQUEST".to_owned(),
                    message: "consumerId and instruments are required".to_owned(),
                    retry_after_seconds: None,
                })?;

        let mut refs = Vec::new();
        for item in &raw_instruments {
            let market = item.market.as_deref().map(str::trim).unwrap_or_default();
            let symbol = item.symbol.as_deref().map(str::trim).unwrap_or_default();
            if market.is_empty() || symbol.is_empty() {
                continue;
            }
            let raw_channel = item.channel.as_deref().map(str::trim).unwrap_or("SNAPSHOT");
            let channel = if raw_channel.is_empty() {
                "SNAPSHOT".to_owned()
            } else {
                raw_channel.to_ascii_uppercase()
            };

            let valid_channels = ["SNAPSHOT", "KLINE", "ORDER_BOOK", "TICK"];
            if !valid_channels.contains(&channel.as_str()) {
                return Err(MarketDataSubscriptionMutationPortError::Failed {
                    status: 400,
                    code: "BAD_REQUEST".to_owned(),
                    message: format!("unsupported subscription channel {:?}", raw_channel),
                    retry_after_seconds: None,
                });
            }

            if channel != "KLINE"
                && item
                    .interval
                    .as_deref()
                    .is_some_and(|i| !i.trim().is_empty())
            {
                return Err(MarketDataSubscriptionMutationPortError::Failed {
                    status: 400,
                    code: "BAD_REQUEST".to_owned(),
                    message: "subscription interval is only valid for KLINE".to_owned(),
                    retry_after_seconds: None,
                });
            }

            if channel == "KLINE" {
                let raw_interval = item.interval.as_deref().unwrap_or_default();
                let interval = raw_interval.trim();
                let valid_intervals = ["1m", "3m", "5m", "15m", "30m", "1h", "1d", "1w", "1mo"];
                if interval.is_empty() || !valid_intervals.contains(&interval) {
                    return Err(MarketDataSubscriptionMutationPortError::Failed {
                        status: 400,
                        code: "BAD_REQUEST".to_owned(),
                        message: format!(
                            "unsupported KLINE subscription interval {:?}",
                            raw_interval
                        ),
                        retry_after_seconds: None,
                    });
                }
            }

            refs.push(InstrumentRef {
                channel,
                market: market.to_owned(),
                symbol: symbol.to_owned(),
                interval: item.interval.clone(),
            });
        }

        if refs.is_empty() {
            return Err(MarketDataSubscriptionMutationPortError::Failed {
                status: 400,
                code: "BAD_REQUEST".to_owned(),
                message: "consumerId and instruments are required".to_owned(),
                retry_after_seconds: None,
            });
        }

        let polling_provider_id = self.polling_provider_id(body.provider_broker_id.as_deref());

        if let Some(provider_broker_id) = polling_provider_id {
            let raw_insts = raw_instruments
                .into_iter()
                .map(|r| {
                    let mut obj = json!({
                        "channel": r.channel.unwrap_or_else(|| "SNAPSHOT".to_owned()),
                        "market": r.market.unwrap_or_default(),
                        "symbol": r.symbol.unwrap_or_default(),
                    });
                    if let Some(interval) = r.interval {
                        obj["interval"] = json!(interval);
                    }
                    obj
                })
                .collect::<Vec<_>>();

            return Ok(broker_polling_subscription_response(
                consumer_id,
                &provider_broker_id,
                raw_insts,
                "acquired",
            ));
        }

        let router = self.router.as_ref().ok_or_else(|| {
            MarketDataSubscriptionMutationPortError::Unavailable(
                "market-data subscription provider is not configured".to_owned(),
            )
        })?;

        let now_ms = current_unix_millis();
        let snapshot = {
            let mut router_guard = router.lock().unwrap_or_else(|e| e.into_inner());
            router_guard
                .acquire_demand(consumer_id, refs.clone(), false, now_ms)
                .map_err(|e| MarketDataSubscriptionMutationPortError::Failed {
                    status: 500,
                    code: "SUBSCRIPTION_FAILED".to_owned(),
                    message: e.to_string(),
                    retry_after_seconds: None,
                })?
        };

        let physical_snapshot = self
            .physical
            .as_ref()
            .map(|p| {
                p.physical_subscription_snapshot().map_err(|error| {
                    MarketDataSubscriptionMutationPortError::Failed {
                        status: 500,
                        code: "SUBSCRIPTION_FAILED".to_owned(),
                        message: error,
                        retry_after_seconds: None,
                    }
                })
            })
            .transpose()?
            .flatten();
        let res = render_subscriptions_data(&snapshot, physical_snapshot.as_ref());
        Ok(res)
    }

    fn release(
        &self,
        request: &MarketDataSubscriptionMutationRequest,
    ) -> Result<Value, MarketDataSubscriptionMutationPortError> {
        let body: SubscriptionRequestBody =
            serde_json::from_slice(&request.body).map_err(|_| {
                MarketDataSubscriptionMutationPortError::Failed {
                    status: 400,
                    code: "BAD_REQUEST".to_owned(),
                    message: "invalid release request".to_owned(),
                    retry_after_seconds: None,
                }
            })?;

        let consumer_id = body
            .consumer_id
            .as_deref()
            .map(str::trim)
            .filter(|c| !c.is_empty())
            .ok_or_else(|| MarketDataSubscriptionMutationPortError::Failed {
                status: 400,
                code: "BAD_REQUEST".to_owned(),
                message: "consumerId is required".to_owned(),
                retry_after_seconds: None,
            })?;

        let mut target_ref: Option<InstrumentRef> = None;
        if let Some(instruments) = &body.instruments
            && !instruments.is_empty()
        {
            let target = &instruments[0];
            let market = target.market.as_deref().map(str::trim).unwrap_or_default();
            let symbol = target.symbol.as_deref().map(str::trim).unwrap_or_default();
            if market.is_empty() || symbol.is_empty() {
                return Err(MarketDataSubscriptionMutationPortError::Failed {
                    status: 400,
                    code: "BAD_REQUEST".to_owned(),
                    message: "release target market and symbol are required".to_owned(),
                    retry_after_seconds: None,
                });
            }
            let raw_channel = target
                .channel
                .as_deref()
                .map(str::trim)
                .unwrap_or("SNAPSHOT");
            let channel = if raw_channel.is_empty() {
                "SNAPSHOT".to_owned()
            } else {
                raw_channel.to_ascii_uppercase()
            };

            let valid_channels = ["SNAPSHOT", "KLINE", "ORDER_BOOK", "TICK"];
            if !valid_channels.contains(&channel.as_str()) {
                return Err(MarketDataSubscriptionMutationPortError::Failed {
                    status: 400,
                    code: "BAD_REQUEST".to_owned(),
                    message: format!("unsupported subscription channel {:?}", raw_channel),
                    retry_after_seconds: None,
                });
            }

            if channel != "KLINE"
                && target
                    .interval
                    .as_deref()
                    .is_some_and(|i| !i.trim().is_empty())
            {
                return Err(MarketDataSubscriptionMutationPortError::Failed {
                    status: 400,
                    code: "BAD_REQUEST".to_owned(),
                    message: "subscription interval is only valid for KLINE".to_owned(),
                    retry_after_seconds: None,
                });
            }

            if channel == "KLINE" {
                let raw_interval = target.interval.as_deref().unwrap_or_default();
                let interval = raw_interval.trim();
                let valid_intervals = ["1m", "3m", "5m", "15m", "30m", "1h", "1d", "1w", "1mo"];
                if !valid_intervals.contains(&interval) {
                    return Err(MarketDataSubscriptionMutationPortError::Failed {
                        status: 400,
                        code: "BAD_REQUEST".to_owned(),
                        message: format!(
                            "unsupported KLINE subscription interval {:?}",
                            raw_interval
                        ),
                        retry_after_seconds: None,
                    });
                }
            }

            target_ref = Some(InstrumentRef {
                channel,
                market: market.to_owned(),
                symbol: symbol.to_owned(),
                interval: target.interval.clone(),
            });
        }

        let polling_provider_id = self.polling_provider_id(body.provider_broker_id.as_deref());

        if let Some(provider_broker_id) = polling_provider_id {
            let raw_insts = body.instruments.map(|list| {
                list.into_iter()
                    .map(|r| {
                        let mut obj = json!({
                            "channel": r.channel.unwrap_or_else(|| "SNAPSHOT".to_owned()),
                            "market": r.market.unwrap_or_default(),
                            "symbol": r.symbol.unwrap_or_default(),
                        });
                        if let Some(interval) = r.interval {
                            obj["interval"] = json!(interval);
                        }
                        obj
                    })
                    .collect::<Vec<_>>()
            });

            return Ok(broker_polling_subscription_response(
                consumer_id,
                &provider_broker_id,
                raw_insts.unwrap_or_default(),
                "released",
            ));
        }

        let router = self.router.as_ref().ok_or_else(|| {
            MarketDataSubscriptionMutationPortError::Unavailable(
                "market-data subscription provider is not configured".to_owned(),
            )
        })?;

        let now_ms = current_unix_millis();
        let (_released, snapshot) = {
            let mut router_guard = router.lock().unwrap_or_else(|e| e.into_inner());
            if let Some(target) = &target_ref {
                router_guard.release_demand_instrument(consumer_id, target, now_ms)
            } else {
                router_guard.release_demand_consumer_with_time(consumer_id, now_ms)
            }
        };

        let physical_snapshot = self
            .physical
            .as_ref()
            .map(|p| {
                p.physical_subscription_snapshot().map_err(|error| {
                    MarketDataSubscriptionMutationPortError::Failed {
                        status: 500,
                        code: "SUBSCRIPTION_FAILED".to_owned(),
                        message: error,
                        retry_after_seconds: None,
                    }
                })
            })
            .transpose()?
            .flatten();
        let mut res = render_subscriptions_data(&snapshot, physical_snapshot.as_ref());
        res["released"] = json!(true);
        Ok(res)
    }

    fn clear(
        &self,
        request: &MarketDataSubscriptionMutationRequest,
    ) -> Result<Value, MarketDataSubscriptionMutationPortError> {
        let query_map = QueryMap::parse(&request.query).map_err(|_| {
            MarketDataSubscriptionMutationPortError::Failed {
                status: 400,
                code: "BAD_REQUEST".to_owned(),
                message: "invalid URL escape".to_owned(),
                retry_after_seconds: None,
            }
        })?;
        let consumer_id = query_map.get_first("consumerId").unwrap_or_default();

        let explicit_provider_broker_id = query_map.get_first("providerBrokerId");
        if let Some(provider_broker_id) = self.polling_provider_id(explicit_provider_broker_id) {
            return Ok(broker_polling_subscription_response(
                consumer_id,
                &provider_broker_id,
                Vec::new(),
                "cleared",
            ));
        }

        // A clear without a router is only valid for the explicit helper
        // polling fallback above.  Do not manufacture an empty demand
        // snapshot for an unconfigured provider.
        let Some(router) = &self.router else {
            return Err(MarketDataSubscriptionMutationPortError::Unavailable(
                "market-data subscription provider is not configured".to_owned(),
            ));
        };
        let now_ms = current_unix_millis();
        let snapshot = {
            let mut router_guard = router.lock().unwrap_or_else(|e| e.into_inner());
            let target = if consumer_id.trim().is_empty() {
                None
            } else {
                Some(consumer_id.trim())
            };
            router_guard.clear_demand(target, now_ms)
        };

        let physical_snapshot = self
            .physical
            .as_ref()
            .map(|p| {
                p.physical_subscription_snapshot().map_err(|error| {
                    MarketDataSubscriptionMutationPortError::Failed {
                        status: 500,
                        code: "SUBSCRIPTION_FAILED".to_owned(),
                        message: error,
                        retry_after_seconds: None,
                    }
                })
            })
            .transpose()?
            .flatten();
        let mut res = render_subscriptions_data(&snapshot, physical_snapshot.as_ref());
        res["cleared"] = json!(true);
        Ok(res)
    }

    fn heartbeat(
        &self,
        request: &MarketDataSubscriptionMutationRequest,
    ) -> Result<Value, MarketDataSubscriptionMutationPortError> {
        let body: SubscriptionRequestBody =
            serde_json::from_slice(&request.body).map_err(|_| {
                MarketDataSubscriptionMutationPortError::Failed {
                    status: 400,
                    code: "BAD_REQUEST".to_owned(),
                    message: "invalid heartbeat request".to_owned(),
                    retry_after_seconds: None,
                }
            })?;

        let consumer_id = body
            .consumer_id
            .as_deref()
            .map(str::trim)
            .filter(|c| !c.is_empty())
            .ok_or_else(|| MarketDataSubscriptionMutationPortError::Failed {
                status: 400,
                code: "BAD_REQUEST".to_owned(),
                message: "consumerId is required".to_owned(),
                retry_after_seconds: None,
            })?;

        let polling_provider_id = self.polling_provider_id(body.provider_broker_id.as_deref());

        if let Some(provider_broker_id) = polling_provider_id {
            return Ok(broker_polling_subscription_response(
                consumer_id,
                &provider_broker_id,
                vec![],
                "heartbeat",
            ));
        }

        let router = self.router.as_ref().ok_or_else(|| {
            MarketDataSubscriptionMutationPortError::Unavailable(
                "market-data subscription provider is not configured".to_owned(),
            )
        })?;

        let now_ms = current_unix_millis();
        let snapshot = {
            let mut router_guard = router.lock().unwrap_or_else(|e| e.into_inner());
            let (_updated, snapshot) = router_guard.heartbeat_demand(consumer_id, now_ms);
            snapshot
        };

        let physical_snapshot = self
            .physical
            .as_ref()
            .map(|p| {
                p.physical_subscription_snapshot().map_err(|error| {
                    MarketDataSubscriptionMutationPortError::Failed {
                        status: 500,
                        code: "SUBSCRIPTION_FAILED".to_owned(),
                        message: error,
                        retry_after_seconds: None,
                    }
                })
            })
            .transpose()?
            .flatten();
        let res = render_subscriptions_data(&snapshot, physical_snapshot.as_ref());
        Ok(res)
    }
}
