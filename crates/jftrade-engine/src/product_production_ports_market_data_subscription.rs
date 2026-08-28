use std::sync::{Arc, Mutex};
use jftrade_marketdata::{InstrumentRef, PhysicalSubscriptionSnapshotPort, ProviderRouter};
use jftrade_settings::MarketDataProvider;
use serde::Deserialize;
use serde_json::{Value, json};

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
}

impl std::fmt::Debug for ProductionMarketDataSubscriptionMutationPort {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("ProductionMarketDataSubscriptionMutationPort")
            .field("has_router", &self.router.is_some())
            .field("has_physical", &self.physical.is_some())
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
        }
    }

    #[allow(dead_code)]
    fn active_provider(
        &self,
    ) -> Result<MarketDataProvider, MarketDataSubscriptionMutationPortError> {
        self.active_provider_state
            .get()
            .ok_or_else(|| {
                MarketDataSubscriptionMutationPortError::Unavailable(
                    "active market-data provider is not configured".to_owned(),
                )
            })
    }

    fn uses_broker_polling(explicit_broker: Option<&str>) -> bool {
        if let Some(b) = explicit_broker.map(str::trim).filter(|s| !s.is_empty()) {
            return !b.eq_ignore_ascii_case("futu");
        }
        false
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

impl MarketDataSubscriptionMutationPort for ProductionMarketDataSubscriptionMutationPort {
    fn dispatch(
        &self,
        request: &MarketDataSubscriptionMutationRequest,
    ) -> Result<Value, MarketDataSubscriptionMutationPortError> {
        let method = request.method.as_str();
        let path = request.path.as_str();

        if path.starts_with("/api/v1/market-data/prediction/contracts/") {
            return Err(MarketDataSubscriptionMutationPortError::Unavailable(
                "prediction market-data subscriptions are not configured".to_owned(),
            ));
        }

        match (method, path) {
            ("POST", MARKET_DATA_SUBSCRIPTION_ACQUIRE_PATH) => self.acquire(request),
            ("POST", MARKET_DATA_SUBSCRIPTION_RELEASE_PATH) => self.release(request),
            ("DELETE", MARKET_DATA_SUBSCRIPTION_CLEAR_PATH) => self.clear(request),
            ("POST", MARKET_DATA_SUBSCRIPTION_HEARTBEAT_PATH) => self.heartbeat(request),
            _ => Err(MarketDataSubscriptionMutationPortError::Unavailable(format!(
                "unsupported subscription mutation route: {method} {path}"
            ))),
        }
    }
}

impl ProductionMarketDataSubscriptionMutationPort {
    fn acquire(
        &self,
        request: &MarketDataSubscriptionMutationRequest,
    ) -> Result<Value, MarketDataSubscriptionMutationPortError> {
        let body: SubscriptionRequestBody = serde_json::from_slice(&request.body).map_err(|_| {
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

        let raw_instruments = body
            .instruments
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

            if channel != "KLINE" && item.interval.as_deref().is_some_and(|i| !i.trim().is_empty()) {
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
                let valid_intervals = [
                    "1m", "3m", "5m", "15m", "30m", "1h", "1d", "1w", "1mo",
                ];
                if interval.is_empty() || !valid_intervals.contains(&interval) {
                    return Err(MarketDataSubscriptionMutationPortError::Failed {
                        status: 400,
                        code: "BAD_REQUEST".to_owned(),
                        message: format!("unsupported KLINE subscription interval {:?}", raw_interval),
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

        let provider_broker_id = body
            .provider_broker_id
            .as_deref()
            .map(str::trim)
            .unwrap_or_default()
            .to_ascii_lowercase();

        let uses_polling = Self::uses_broker_polling(body.provider_broker_id.as_deref());

        if uses_polling {
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
        let res = render_subscriptions_data(
            &snapshot,
            physical_snapshot.as_ref(),
        );
        Ok(res)
    }

    fn release(
        &self,
        request: &MarketDataSubscriptionMutationRequest,
    ) -> Result<Value, MarketDataSubscriptionMutationPortError> {
        let body: SubscriptionRequestBody = serde_json::from_slice(&request.body).map_err(|_| {
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
            let raw_channel = target.channel.as_deref().map(str::trim).unwrap_or("SNAPSHOT");
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

            if channel != "KLINE" && target.interval.as_deref().is_some_and(|i| !i.trim().is_empty()) {
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
                let valid_intervals = [
                    "1m", "3m", "5m", "15m", "30m", "1h", "1d", "1w", "1mo",
                ];
                if !valid_intervals.contains(&interval) {
                    return Err(MarketDataSubscriptionMutationPortError::Failed {
                        status: 400,
                        code: "BAD_REQUEST".to_owned(),
                        message: format!("unsupported KLINE subscription interval {:?}", raw_interval),
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

        let provider_broker_id = body
            .provider_broker_id
            .as_deref()
            .map(str::trim)
            .unwrap_or_default()
            .to_ascii_lowercase();

        let uses_polling = Self::uses_broker_polling(body.provider_broker_id.as_deref());

        if uses_polling {
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
        let mut res = render_subscriptions_data(
            &snapshot,
            physical_snapshot.as_ref(),
        );
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
        let consumer_id = query_map
            .get_first("consumerId")
            .unwrap_or_default();

        let now_ms = current_unix_millis();
        let snapshot = if let Some(router) = &self.router {
            let mut router_guard = router.lock().unwrap_or_else(|e| e.into_inner());
            let target = if consumer_id.trim().is_empty() {
                None
            } else {
                Some(consumer_id.trim())
            };
            router_guard.clear_demand(target, now_ms)
        } else {
            jftrade_marketdata::DemandSnapshot::default()
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
        let mut res = render_subscriptions_data(
            &snapshot,
            physical_snapshot.as_ref(),
        );
        res["cleared"] = json!(true);
        Ok(res)
    }

    fn heartbeat(
        &self,
        request: &MarketDataSubscriptionMutationRequest,
    ) -> Result<Value, MarketDataSubscriptionMutationPortError> {
        let body: SubscriptionRequestBody = serde_json::from_slice(&request.body).map_err(|_| {
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

        let provider_broker_id = body
            .provider_broker_id
            .as_deref()
            .map(str::trim)
            .unwrap_or_default()
            .to_ascii_lowercase();

        let uses_polling = Self::uses_broker_polling(body.provider_broker_id.as_deref());

        if uses_polling {
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
        let res = render_subscriptions_data(
            &snapshot,
            physical_snapshot.as_ref(),
        );
        Ok(res)
    }
}
