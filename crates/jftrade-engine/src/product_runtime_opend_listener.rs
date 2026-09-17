//! OpenD session event listener bridging quotes, depth, and reconnects to LiveHub.

use std::sync::{Arc, Mutex};
use tokio::sync::Notify;

use jftrade_marketdata::TradeVolumeTracker;

pub(crate) struct LiveHubOpenDEventListener {
    live_hub: Arc<jftrade_api::LiveHub>,
    reconciliation_wake: Option<Arc<Notify>>,
    /// Cumulative-volume baselines keyed per instrument. Go keeps the same
    /// state on `Stream.tradeVolumes`; the listener is the only writer for the
    /// live push path, so it owns the map rather than the LiveHub.
    trade_volumes: Mutex<TradeVolumeTracker>,
}

impl std::fmt::Debug for LiveHubOpenDEventListener {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("LiveHubOpenDEventListener")
            .finish_non_exhaustive()
    }
}

impl LiveHubOpenDEventListener {
    pub(crate) fn with_reconciliation_wake(
        live_hub: Arc<jftrade_api::LiveHub>,
        reconciliation_wake: Arc<Notify>,
    ) -> Self {
        Self {
            live_hub,
            reconciliation_wake: Some(reconciliation_wake),
            trade_volumes: Mutex::new(TradeVolumeTracker::new()),
        }
    }
}

impl jftrade_integration_futu::OpenDSessionEventListener for LiveHubOpenDEventListener {
    fn on_event(&self, outcome: &jftrade_integration_futu::OpenDSessionCoordinatorOutcome) {
        match outcome {
            jftrade_integration_futu::OpenDSessionCoordinatorOutcome::Push(push) => match push {
                jftrade_integration_futu::QuotePush::Basic(basic) => {
                    for quote in &basic.quotes {
                        let Some(sec) = quote.security.as_ref() else {
                            continue;
                        };
                        let Some(raw_market) = sec.market else {
                            continue;
                        };
                        let market = match raw_market {
                            1 => "HK",
                            11 => "US",
                            21 => "SH",
                            22 => "SZ",
                            31 => "SG",
                            41 => "JP",
                            51 => "AU",
                            61 => "MY",
                            71 => "CA",
                            _ => continue,
                        };
                        let Some(code) = sec.code.as_deref() else {
                            continue;
                        };
                        let code = code.trim().to_ascii_uppercase();
                        if code.is_empty() {
                            continue;
                        };
                        // Go's `emitBasicQotSnapshot` returns before emitting a
                        // trade when `ticker.Last` is zero, and
                        // `tickFromSnapshot` rejects a zero snapshot price, so a
                        // quote without a traded price never reaches the live
                        // contract.
                        if quote.cur_price.is_none_or(|price| price == 0.0) {
                            continue;
                        }
                        let Some(at) = quote.update_time.as_deref() else {
                            continue;
                        };
                        let at = at.trim();
                        if at.is_empty() {
                            continue;
                        }
                        let instrument_id = format!("{market}.{code}");
                        self.publish_basic_quote_tick(market, &code, &instrument_id, at, quote);
                    }
                }
                jftrade_integration_futu::QuotePush::OrderBook(ob) => {
                    let Some(sec) = ob.security.as_ref() else {
                        return;
                    };
                    let Some(raw_market) = sec.market else {
                        return;
                    };
                    let market = match raw_market {
                        1 => "HK",
                        11 => "US",
                        21 => "SH",
                        22 => "SZ",
                        31 => "SG",
                        41 => "JP",
                        51 => "AU",
                        61 => "MY",
                        71 => "CA",
                        _ => return,
                    };
                    let Some(code) = sec.code.as_deref() else {
                        return;
                    };
                    let code = code.trim().to_ascii_uppercase();
                    if code.is_empty() {
                        return;
                    };
                    // Parity: `go:452dea11:pkg/futu/stream_orderbook.go:48`
                    // `handleOrderBookPush`. Go never reads the server receive
                    // timestamps; it only requires a resolvable security and at
                    // least one non-zero best price. Fall back to the local
                    // clock so a push without server times is not dropped.
                    let at = ob
                        .server_receive_time_bid
                        .as_deref()
                        .filter(|value| !value.trim().is_empty())
                        .or_else(|| {
                            ob.server_receive_time_ask
                                .as_deref()
                                .filter(|value| !value.trim().is_empty())
                        })
                        .map(str::to_owned)
                        .unwrap_or_else(current_utc_rfc3339);
                    let bids = ob
                        .bids
                        .iter()
                        .filter_map(|b| {
                            b.price.map(|p| {
                                serde_json::json!({
                                    "price": p,
                                    "volume": b.volume,
                                    "orderCount": b.order_count.unwrap_or(0),
                                })
                            })
                        })
                        .collect::<Vec<_>>();
                    let asks = ob
                        .asks
                        .iter()
                        .filter_map(|a| {
                            a.price.map(|p| {
                                serde_json::json!({
                                    "price": p,
                                    "volume": a.volume,
                                    "orderCount": a.order_count.unwrap_or(0),
                                })
                            })
                        })
                        .collect::<Vec<_>>();
                    // Go builds the best bid/ask BookTicker and returns before
                    // emitting when both sides are zero, so an empty or
                    // zero-priced push never reaches consumers.
                    if bids.is_empty() && asks.is_empty() {
                        return;
                    }
                    let envelope = order_book_depth_envelope(market, &code, &at, bids, asks);
                    self.live_hub.publish(envelope);
                }
                _ => {}
            },
            jftrade_integration_futu::OpenDSessionCoordinatorOutcome::Reconnected { .. } => {
                if let Some(wake) = self.reconciliation_wake.as_ref() {
                    wake.notify_one();
                }
                let at = current_utc_rfc3339();
                self.publish_runtime_event(
                    "market-data.resync",
                    "market-data",
                    "futu",
                    &at,
                    serde_json::json!({
                        "type": "market-data.resync",
                        "at": at,
                        "reason": "provider_reconnected",
                        "action": "refresh",
                    }),
                );
                let envelope = serde_json::json!({
                    "eventId": format!("console.refresh|market-data|{at}"),
                    "type": "console.refresh",
                    "source": "market-data",
                    "entityId": "futu",
                    "serverTime": at,
                    "payload": {
                        "type": "console.refresh",
                        "at": at,
                        "scope": "market-data",
                        "checkedAt": at,
                    },
                });
                self.live_hub.publish(envelope);
            }
            jftrade_integration_futu::OpenDSessionCoordinatorOutcome::Dropped => {
                self.publish_stale("provider_disconnected", None);
            }
            _ => {}
        }
    }

    fn on_error(&self, error: &str) {
        self.publish_stale("provider_error", Some(error));
        let at = current_utc_rfc3339();
        let envelope = serde_json::json!({
            "eventId": format!("system.notification|market-data-error|{at}"),
            "type": "system.notification",
            "source": "notification",
            "entityId": "futu",
            "serverTime": at,
            "payload": {
                "type": "system.notification",
                "at": at,
                "level": "error",
                "message": error,
                "source": "futu",
            },
        });
        self.live_hub.publish(envelope);
    }
}

impl LiveHubOpenDEventListener {
    /// Publishes one BasicQot push as the Go `market-data.tick` trade payload.
    ///
    /// Go derives the per-event `volumeDelta` from the cumulative counter in
    /// `Stream.nextTradeQuantity` and publishes both values explicitly
    /// (`internal/marketdata/responses.go::TickEventDTO.JSON`). Keeping the
    /// conversion here means the wire contract the frontend already consumes
    /// (`cumulativeVolume`/`volumeDelta` plus the `instrument`/`snapshot`
    /// blocks) is produced by the same owner that sees every push.
    fn publish_basic_quote_tick(
        &self,
        market: &str,
        code: &str,
        instrument_id: &str,
        at: &str,
        quote: &jftrade_integration_futu::BasicQuote,
    ) {
        let Some(price) = quote.cur_price else {
            return;
        };
        let observed_at_ms = quote
            .update_timestamp
            .filter(|value| value.is_finite() && *value > 0.0)
            .map(|value| (value * 1_000.0).round() as i64)
            .unwrap_or_else(|| time::OffsetDateTime::now_utc().unix_timestamp() * 1_000);
        let session =
            jftrade_integration_futu::quote_session_label(instrument_id, observed_at_ms, None);
        let cumulative_volume = quote
            .volume
            .map(|value| value.to_string())
            .and_then(|value| value.parse::<jftrade_kernel::DecimalText>().ok());
        // Go's `emitBasicQotSnapshot` returns before `EmitMarketTrade` when the
        // cumulative counter is negative, so no trade event is published at
        // all. Reproducing that requires checking the counter before the
        // tracker consumes it: a rejected sample must also leave the retained
        // baseline untouched.
        if cumulative_volume
            .as_ref()
            .is_some_and(jftrade_marketdata::TradeVolumeTracker::is_negative_cumulative)
        {
            return;
        }
        let volume_delta = match cumulative_volume.as_ref() {
            Some(cumulative) => self
                .trade_volumes
                .lock()
                .unwrap_or_else(|error| error.into_inner())
                .next_quantity(instrument_id, observed_at_ms, &session, cumulative),
            None => "0"
                .parse::<jftrade_kernel::DecimalText>()
                .expect("zero is valid decimal text"),
        };
        let decimal_text = |value: f64| value.to_string();
        let payload = serde_json::json!({
            "type": "market-data.tick",
            "at": at,
            "brokerId": "futu",
            "source": "futu",
            "cumulativeVolume": cumulative_volume.as_ref().map(|value| value.as_str()),
            "volumeDelta": volume_delta.as_str(),
            "instrument": {
                "market": market,
                "symbol": code,
                "instrumentId": instrument_id,
            },
            "snapshot": {
                "price": decimal_text(price),
                "volume": cumulative_volume.as_ref().map(|value| value.as_str()),
                "turnover": quote.turnover.map(decimal_text),
                "highPrice": quote.high_price.map(decimal_text),
                "lowPrice": quote.low_price.map(decimal_text),
                "openPrice": quote.open_price.map(decimal_text),
                "lastClosePrice": quote.last_close_price.map(decimal_text),
                "at": at,
                "session": session,
            },
        });
        self.live_hub.publish(serde_json::json!({
            "eventId": format!("market-data.tick|{instrument_id}|{at}"),
            "type": "market-data.tick",
            "source": "market-data",
            "entityId": instrument_id,
            "serverTime": at,
            "payload": payload,
        }));
    }

    fn publish_stale(&self, reason: &str, error: Option<&str>) {
        let at = current_utc_rfc3339();
        self.publish_runtime_event(
            "market-data.stale",
            "market-data",
            "futu",
            &at,
            serde_json::json!({
                "type": "market-data.stale",
                "at": at,
                "stale": true,
                "reason": reason,
                "error": error,
            }),
        );
    }

    fn publish_runtime_event(
        &self,
        event_type: &str,
        source: &str,
        entity_id: &str,
        at: &str,
        payload: serde_json::Value,
    ) {
        self.live_hub.publish(serde_json::json!({
            "eventId": format!("{event_type}|{entity_id}|{at}"),
            "type": event_type,
            "source": source,
            "entityId": entity_id,
            "serverTime": at,
            "payload": payload,
        }));
    }
}

fn current_utc_rfc3339() -> String {
    time::OffsetDateTime::now_utc()
        .format(&time::format_description::well_known::Rfc3339)
        .unwrap_or_else(|_| "1970-01-01T00:00:00Z".to_owned())
}

fn order_book_depth_envelope(
    market: &str,
    code: &str,
    at: &str,
    bids: Vec<serde_json::Value>,
    asks: Vec<serde_json::Value>,
) -> serde_json::Value {
    let instrument_id = format!("{market}.{code}");
    let num = bids.len().max(asks.len()).max(1);
    let payload = serde_json::json!({
        "type": "market.depth",
        "brokerId": "futu",
        "instrumentId": instrument_id,
        "at": at,
        "provenance": "stream",
        "depth": {
            "symbol": instrument_id,
            "bids": bids,
            "asks": asks,
        },
        "meta": {
            "instrumentId": instrument_id,
            "source": "futu",
            "resolvedAt": at,
        },
        "request": {
            "market": market,
            "symbol": code,
            "instrumentId": instrument_id,
            "num": num,
        },
    });
    serde_json::json!({
        "eventId": format!("market.depth|{instrument_id}|{at}"),
        "type": "market.depth",
        "source": "market-data",
        "entityId": instrument_id,
        "serverTime": at,
        "payload": payload,
    })
}
