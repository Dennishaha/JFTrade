//! OpenD session event listener bridging quotes, depth, and reconnects to LiveHub.

use std::sync::Arc;

pub(crate) struct LiveHubOpenDEventListener {
    live_hub: Arc<jftrade_api::LiveHub>,
}

impl std::fmt::Debug for LiveHubOpenDEventListener {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("LiveHubOpenDEventListener")
            .finish_non_exhaustive()
    }
}

impl LiveHubOpenDEventListener {
    pub(crate) fn new(live_hub: Arc<jftrade_api::LiveHub>) -> Self {
        Self { live_hub }
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
                        let Some(price) = quote.cur_price else {
                            continue;
                        };
                        let Some(at) = quote.update_time.as_deref() else {
                            continue;
                        };
                        let at = at.trim();
                        if at.is_empty() {
                            continue;
                        }
                        let instrument_id = format!("{market}.{code}");
                        let payload = serde_json::json!({
                            "type": "market-data.tick",
                            "brokerId": "futu",
                            "instrumentId": instrument_id,
                            "price": price,
                            "volume": quote.volume,
                            "turnover": quote.turnover,
                            "highPrice": quote.high_price,
                            "lowPrice": quote.low_price,
                            "openPrice": quote.open_price,
                            "lastClosePrice": quote.last_close_price,
                            "at": at,
                        });
                        let envelope = serde_json::json!({
                            "eventId": format!("market-data.tick|{instrument_id}|{at}"),
                            "type": "market-data.tick",
                            "source": "market-data",
                            "entityId": instrument_id,
                            "serverTime": at,
                            "payload": payload,
                        });
                        self.live_hub.publish(envelope);
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
                    let at = ob
                        .server_receive_time_bid
                        .as_deref()
                        .or(ob.server_receive_time_ask.as_deref());
                    let Some(at) = at.filter(|s| !s.trim().is_empty()) else {
                        return;
                    };
                    let bids = ob
                        .bids
                        .iter()
                        .filter_map(|b| {
                            b.price.map(|p| {
                                serde_json::json!({
                                    "price": p,
                                    "volume": b.volume,
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
                                })
                            })
                        })
                        .collect::<Vec<_>>();
                    let instrument_id = format!("{market}.{code}");
                    let payload = serde_json::json!({
                        "type": "market.depth",
                        "brokerId": "futu",
                        "instrumentId": instrument_id,
                        "depth": {
                            "bids": bids,
                            "asks": asks,
                        },
                        "at": at,
                    });
                    let envelope = serde_json::json!({
                        "eventId": format!("market.depth|{instrument_id}|{at}"),
                        "type": "market.depth",
                        "source": "market-data",
                        "entityId": instrument_id,
                        "serverTime": at,
                        "payload": payload,
                    });
                    self.live_hub.publish(envelope);
                }
                _ => {}
            },
            jftrade_integration_futu::OpenDSessionCoordinatorOutcome::Reconnected { .. } => {
                let at = current_utc_rfc3339();
                let envelope = serde_json::json!({
                    "eventId": format!("console.refresh|market-data|{at}"),
                    "type": "console.refresh",
                    "source": "market-data",
                    "entityId": "futu",
                    "serverTime": at,
                    "payload": {
                        "scope": "market-data",
                    },
                });
                self.live_hub.publish(envelope);
            }
            _ => {}
        }
    }

    fn on_error(&self, error: &str) {
        let at = current_utc_rfc3339();
        let envelope = serde_json::json!({
            "eventId": format!("system.notification|market-data-error|{at}"),
            "type": "system.notification",
            "source": "notification",
            "entityId": "futu",
            "serverTime": at,
            "payload": {
                "level": "error",
                "message": error,
                "source": "futu",
            },
        });
        self.live_hub.publish(envelope);
    }
}

fn current_utc_rfc3339() -> String {
    time::OffsetDateTime::now_utc()
        .format(&time::format_description::well_known::Rfc3339)
        .unwrap_or_else(|_| "1970-01-01T00:00:00Z".to_owned())
}
