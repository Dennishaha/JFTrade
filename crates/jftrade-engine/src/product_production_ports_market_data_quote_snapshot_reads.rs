//! Snapshot reads for the production market-data quote port.
//!
//! Go's `marketSnapshot` handler owns the refresh query, the basic-subscription
//! demand gate, the provider/helper fallback chain and the cached-snapshot
//! answer for a failed provider call. Keeping that longer flow in its own
//! module leaves the candle path in `quote_reads.rs` under the production
//! line budget.

use super::quote_snapshot::{project_cached_snapshot, project_fallback_snapshot};
use super::quote_tick_candles::TICK_FRESHNESS_MS;
use super::*;

impl ProductionMarketDataQuotePort {
    pub(super) async fn read_snapshots(
        &self,
        suffix: &str,
        query: &str,
    ) -> Result<Value, MarketDataQuoteReadSnapshotError> {
        let (market, symbol) = parse_market_symbol_path(suffix)?;
        // Snapshot cache keys and provider queries share the canonical identity
        // regardless of the route segments' casing.
        let market = market.to_ascii_uppercase();
        let symbol = symbol.to_ascii_uppercase();
        let query_map =
            QueryMap::parse(query).map_err(|_| MarketDataQuoteReadSnapshotError::Failed {
                status: 400,
                code: "BAD_REQUEST".to_owned(),
                message: "invalid URL escape".to_owned(),
                retry_after_seconds: None,
            })?;
        // Go binds `refresh` through OptionalBoolValue, so the documented
        // truthy/falsy aliases (including a blank value) select the cache or
        // force path and anything else is a 400 before provider access.
        let refresh = match query_map.get_first("refresh") {
            None => false,
            Some(value) => parse_optional_query_bool(value).map_err(|_| {
                MarketDataQuoteReadSnapshotError::Failed {
                    status: 400,
                    code: "BAD_REQUEST".to_owned(),
                    message: "invalid refresh query".to_owned(),
                    retry_after_seconds: None,
                }
            })?,
        };

        let provider = self.active_provider()?;
        super::quote_broker::validate_explicit_broker(&query_map, provider)?;

        // Go gates live reads behind `requireBasicSubscriptionDemand` once a
        // subscription reconciler exists; the router is the Rust owner of that
        // logical demand and poll-only providers stay authorized by selection.
        if provider == MarketDataProvider::Futu && self.router.is_some() {
            self.require_basic_subscription_lease(&format!("{market}.{symbol}"), "SNAPSHOT", None)?;
        }

        if let Some(helper) = &self.helper
            && (provider == MarketDataProvider::Yfinance || provider == MarketDataProvider::Akshare)
        {
            let provider_str = match provider {
                MarketDataProvider::Yfinance => "yfinance",
                MarketDataProvider::Akshare => "akshare",
                MarketDataProvider::Futu => "futu",
            };
            // Go fences the provider query with the active provider
            // generation and reports 409 MARKET_DATA_PROVIDER_CHANGED when a
            // switch lands mid-read, so a retired provider's snapshot never
            // reaches the caller or the cache.
            let generation = self.active_provider_state.snapshot().generation;
            let resp = helper
                .get_provider_json::<HelperSnapshotResponse>(
                    provider_str,
                    &["snapshot", &market, &symbol],
                )
                .await
                .map_err(|error| map_helper_quote_error(error, "MARKET_SNAPSHOT_FAILED"))?;
            if self.active_provider_state.snapshot().generation != generation {
                return Err(provider_changed_error());
            }

            let instrument_id = format!("{market}.{symbol}");
            let price_str = resp.price.as_str();
            if price_str.trim().is_empty() {
                return Err(MarketDataQuoteReadSnapshotError::Failed {
                    status: 502,
                    code: "MARKET_DATA_PROVIDER_FAILED".to_owned(),
                    message: "empty price string in snapshot response".to_owned(),
                    retry_after_seconds: None,
                });
            }

            if resp.observed_at.trim().is_empty() {
                return Err(MarketDataQuoteReadSnapshotError::Failed {
                    status: 502,
                    code: "MARKET_DATA_PROVIDER_FAILED".to_owned(),
                    message: "missing observedAt timestamp in snapshot response".to_owned(),
                    retry_after_seconds: None,
                });
            }

            let to_json_val = |opt: &Option<HelperPriceValue>| -> Value {
                opt.as_ref()
                    .map(|v| json!(v.as_str()))
                    .unwrap_or(Value::Null)
            };

            return Ok(json!({
                "meta": {
                    "fromCache": !refresh,
                    "instrumentId": instrument_id,
                    "resolvedAt": resp.observed_at,
                    "source": resp.source,
                    "brokerId": provider_str,
                },
                "request": {
                    "instrumentId": instrument_id,
                    "market": market,
                    "symbol": symbol
                },
                "snapshot": {
                    "ask": to_json_val(&resp.ask),
                    "at": resp.observed_at,
                    "bid": to_json_val(&resp.bid),
                    "extended": {
                        "afterMarket": Value::Null,
                        "overnight": Value::Null,
                        "preMarket": Value::Null
                    },
                    "extendedHours": false,
                    "highPrice": to_json_val(&resp.high_price),
                    "lastClosePrice": to_json_val(&resp.last_close_price),
                    "lowPrice": to_json_val(&resp.low_price),
                    "observedAt": resp.observed_at,
                    "openPrice": to_json_val(&resp.open_price),
                    "previousClosePrice": to_json_val(&resp.previous_close_price),
                    "price": price_str,
                    "session": "regular",
                    "turnover": to_json_val(&resp.turnover),
                    "volume": to_json_val(&resp.volume),
                }
            }));
        }

        if provider == MarketDataProvider::Futu {
            let instrument_id = format!("{market}.{symbol}");
            let now_ms = current_unix_millis();
            // Go's `GetSnapshot` skips `cache.Latest` entirely when the caller
            // forces a refresh, so `refresh=true` must never answer from a
            // retained sample even when it is still fresh.
            let cached_tick = if refresh {
                None
            } else {
                self.router.as_ref().and_then(|router| {
                    let router_guard = router.lock().unwrap_or_else(|e| e.into_inner());
                    let cache_handle = router_guard.cache_handle();
                    let cache_guard = cache_handle.lock().unwrap_or_else(|e| e.into_inner());
                    // Go reads `cache.Latest(instrumentID, TickFreshness)`;
                    // only a sample inside the 1.5s freshness window may
                    // answer the read. A stale sample must fall through to
                    // the provider instead of masquerading as a cache hit.
                    match cache_guard.lookup(&instrument_id, now_ms, TICK_FRESHNESS_MS) {
                        CacheLookup::Fresh(t) => Some(t),
                        CacheLookup::Stale(_) | CacheLookup::Missing => None,
                    }
                })
            };

            if let Some(tick) = cached_tick {
                let observed_at = format_unix_millis_rfc3339(tick.observed_at_ms);
                let snapshot = project_cached_snapshot(&tick, &market, &observed_at);

                return Ok(json!({
                    "meta": {
                        "fromCache": true,
                        "instrumentId": instrument_id,
                        "resolvedAt": observed_at,
                        "source": "futu",
                        "brokerId": "futu",
                    },
                    "request": {
                        "instrumentId": instrument_id,
                        "market": market,
                        "symbol": symbol
                    },
                    "snapshot": snapshot
                }));
            }

            let generation = self.active_provider_state.snapshot().generation;
            let fallback_snapshot = if let (Some(runtime), Some(market_code)) =
                (&self.trade_runtime, quote_market_code(&market))
            {
                let security = jftrade_integration_futu::TradeSecurity {
                    market: market_code,
                    code: symbol.clone(),
                };
                runtime
                    .security_snapshots(&[security])
                    .ok()
                    .and_then(|mut list| {
                        if list.is_empty() {
                            None
                        } else {
                            Some(list.remove(0))
                        }
                    })
            } else {
                None
            };
            if self.active_provider_state.snapshot().generation != generation {
                return Err(provider_changed_error());
            }

            if let Some(snap) = fallback_snapshot {
                let fallback_observed_at = format_unix_millis_rfc3339(now_ms);
                if let Some(snapshot) =
                    project_fallback_snapshot(&snap, &market, &fallback_observed_at)
                {
                    let observed_at = snapshot
                        .get("observedAt")
                        .and_then(Value::as_str)
                        .unwrap_or(&fallback_observed_at);
                    return Ok(json!({
                        "meta": {
                            "fromCache": false,
                            "instrumentId": instrument_id,
                            "resolvedAt": observed_at,
                            "source": "futu",
                            "brokerId": "futu",
                        },
                        "request": {
                            "instrumentId": instrument_id,
                            "market": market,
                            "symbol": symbol,
                        },
                        "snapshot": snapshot
                    }));
                }
            }

            return Err(MarketDataQuoteReadSnapshotError::Unavailable(format!(
                "no cached snapshot available for {instrument_id}"
            )));
        }

        Err(MarketDataQuoteReadSnapshotError::Unavailable(format!(
            "snapshot provider is not supported for {market}.{symbol}"
        )))
    }
}
