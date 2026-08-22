use serde::Deserialize;
use serde_json::{Map, Value, json};

pub const WS_LIVE_ROUTE: (&str, &str) = ("GET", "/api/v1/ws/live");
const FIXTURE_TIME: &str = "fixture-time";

#[derive(Clone, Debug, Deserialize)]
pub struct WsLiveFixture {
    pub version: String,
    pub route: WsLiveRoute,
    pub cases: Vec<WsLiveCase>,
}

#[derive(Clone, Debug, Deserialize)]
pub struct WsLiveRoute {
    pub method: String,
    pub path: String,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct WsLiveCase {
    pub name: String,
    pub method: String,
    pub request_path: String,
    pub scenario: String,
    pub input: WsLiveInput,
    pub expected: Value,
}

#[derive(Clone, Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct WsLiveInput {
    pub backend_available: bool,
    pub connection_limit: i64,
    #[serde(default)]
    pub origin_policy: String,
    #[serde(default)]
    pub offer_protocol: bool,
    #[serde(default)]
    #[serde(default)]
    pub heartbeat_interval_ms: i64,
    #[serde(default)]
    pub subscribe: Option<WsLiveSubscriptions>,
    #[serde(default)]
    pub ticks: Vec<WsLiveTick>,
    #[serde(default)]
    pub ticks_error: bool,
    #[serde(default)]
    pub notifications: Vec<WsLiveNotification>,
    #[serde(default)]
    pub security_error: bool,
    #[serde(default)]
    pub depth_error: bool,
    #[serde(default)]
    #[serde(default)]
    pub depth_resolved_at: String,
    #[serde(default)]
    pub depth_payload: Option<Value>,
}

#[derive(Clone, Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct WsLiveSubscriptions {
    #[serde(default)]
    pub provider_broker_id: String,
    #[serde(default)]
    pub active_instruments: Vec<String>,
    #[serde(default)]
    pub security_details: Vec<WsLiveSecuritySubscription>,
    #[serde(default)]
    pub depth: Vec<WsLiveDepthSubscription>,
    #[serde(default)]
    pub console_refresh: bool,
}

#[derive(Clone, Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct WsLiveSecuritySubscription {
    pub market: String,
    pub symbol: String,
    pub instrument_id: String,
}

#[derive(Clone, Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct WsLiveDepthSubscription {
    pub market: String,
    pub symbol: String,
    pub instrument_id: String,
    pub num: i64,
}

#[derive(Clone, Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct WsLiveTick {
    pub instrument_id: String,
    pub observed_at: String,
    pub payload: Value,
}

#[derive(Clone, Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct WsLiveNotification {
    pub sequence: u64,
    pub at: String,
    pub level: String,
    pub title: String,
    #[serde(default)]
    pub message: String,
    pub source: String,
    pub broker_id: String,
    pub category: String,
}

#[derive(serde::Serialize)]
struct EventEnvelope {
    #[serde(rename = "eventId")]
    event_id: String,
    #[serde(rename = "type")]
    event_type: String,
    source: String,
    #[serde(rename = "entityId")]
    entity_id: String,
    #[serde(rename = "serverTime")]
    server_time: String,
    payload: Value,
}

pub fn replay_fixture_case(case: &WsLiveCase) -> Result<Value, String> {
    if case.method != WS_LIVE_ROUTE.0 || case.request_path != WS_LIVE_ROUTE.1 {
        return Err(format!(
            "unsupported ws-live request {} {}",
            case.method, case.request_path
        ));
    }
    if !case.input.backend_available {
        return Ok(rejection(
            404,
            "text/plain; charset=utf-8",
            "404 page not found\n",
        ));
    }
    if case.input.origin_policy == "forbidden" {
        return Ok(rejection(
            403,
            "text/plain; charset=utf-8",
            "websocket: request origin not allowed\n",
        ));
    }
    match case.scenario.as_str() {
        "heartbeat" => success(
            case,
            vec![session(case, vec![heartbeat(&case.input, "")], None)],
            1,
            false,
            0,
            0,
        ),
        "subscription-order" => {
            let subscription = normalized_subscriptions(case.input.subscribe.as_ref());
            let mut frames = vec![
                heartbeat(&case.input, ""),
                heartbeat(&case.input, &subscription.provider_broker_id),
            ];
            frames.extend(subscription_auxiliary_frames(&case.input, &subscription));
            success(
                case,
                vec![session(case, frames, None)],
                1,
                false,
                subscription.security_details.len(),
                subscription.depth.len(),
            )
        }
        "tick-notification" => {
            let subscription = normalized_subscriptions(case.input.subscribe.as_ref());
            let mut frames = vec![heartbeat(&case.input, "")];
            frames.extend(case.input.notifications.iter().map(notification));
            frames.push(heartbeat(&case.input, &subscription.provider_broker_id));
            frames.extend(tick_frames(&case.input, &subscription));
            success(case, vec![session(case, frames, None)], 1, true, 0, 0)
        }
        "depth-update" => {
            let subscription = normalized_subscriptions(case.input.subscribe.as_ref());
            let mut frames = vec![
                heartbeat(&case.input, ""),
                heartbeat(&case.input, &subscription.provider_broker_id),
            ];
            frames.extend(depth_frames(&case.input, &subscription));
            frames.extend(depth_frames(&case.input, &subscription));
            success(case, vec![session(case, frames, None)], 1, false, 0, 2)
        }
        "invalid-subscription" => success(
            case,
            vec![session(
                case,
                vec![heartbeat(&case.input, "")],
                Some(close_code()),
            )],
            1,
            false,
            0,
            0,
        ),
        "provider-error" => {
            let subscription = normalized_subscriptions(case.input.subscribe.as_ref());
            success(
                case,
                vec![session(
                    case,
                    vec![
                        heartbeat(&case.input, ""),
                        heartbeat(&case.input, &subscription.provider_broker_id),
                    ],
                    Some(close_code()),
                )],
                1,
                true,
                0,
                0,
            )
        }
        "server-close" => success(
            case,
            vec![session(
                case,
                vec![heartbeat(&case.input, "")],
                Some(close_code()),
            )],
            1,
            false,
            0,
            0,
        ),
        "client-reconnect" => success(
            case,
            vec![
                session(case, vec![heartbeat(&case.input, "")], None),
                session(case, vec![heartbeat(&case.input, "")], None),
            ],
            2,
            false,
            0,
            0,
        ),
        "connection-limit" => {
            let first = session(case, vec![heartbeat(&case.input, "")], None);
            let mut result = success(case, vec![first], 1, false, 0, 0)?;
            if let Value::Object(object) = &mut result {
                object.insert(
                    "rejected".to_owned(),
                    rejection_body(
                        503,
                        "application/json; charset=utf-8",
                        &format!(
                            "{{\"error\":{{\"code\":\"LIVE_WS_LIMIT_REACHED\",\"message\":\"live websocket connection limit reached ({})\"}},\"ok\":false,\"timestamp\":\"{}\"}}\n",
                            effective_limit(&case.input), FIXTURE_TIME
                        ),
                    ),
                );
            }
            Ok(result)
        }
        other => Err(format!("unsupported ws-live scenario {other}")),
    }
}

fn success(
    case: &WsLiveCase,
    sessions: Vec<Value>,
    session_count: usize,
    market_ticks_called: bool,
    security_calls: usize,
    depth_calls: usize,
) -> Result<Value, String> {
    let mut object = Map::new();
    object.insert("sessions".to_owned(), Value::Array(sessions));
    object.insert(
        "calls".to_owned(),
        json!({
            "ensureNotificationBridge": session_count,
            "depthSubscribe": session_count,
            "depthUnsubscribe": session_count,
            "marketTicksCalled": market_ticks_called,
            "securityCalls": security_calls,
            "depthCalls": depth_calls,
        }),
    );
    if case.input.security_error || case.input.depth_error {
        let _ = case;
    }
    Ok(Value::Object(object))
}

fn session(case: &WsLiveCase, frames: Vec<String>, close: Option<Value>) -> Value {
    let mut object = Map::new();
    object.insert(
        "handshake".to_owned(),
        json!({
            "status": 101,
            "selectedProtocol": if case.input.offer_protocol { "jftrade.desktop.v1" } else { "" },
        }),
    );
    object.insert(
        "frames".to_owned(),
        Value::Array(frames.into_iter().map(Value::String).collect()),
    );
    if let Some(close) = close {
        object.insert("close".to_owned(), close);
    }
    Value::Object(object)
}

fn close_code() -> Value {
    json!({"kind": "close-code", "code": 1006})
}

fn rejection(status: u16, content_type: &str, body: &str) -> Value {
    let mut object = Map::new();
    object.insert("sessions".to_owned(), Value::Array(Vec::new()));
    object.insert(
        "rejected".to_owned(),
        rejection_body(status, content_type, body),
    );
    object.insert("calls".to_owned(), empty_calls());
    Value::Object(object)
}

fn rejection_body(status: u16, content_type: &str, body: &str) -> Value {
    json!({"status": status, "contentType": content_type, "body": body})
}

fn empty_calls() -> Value {
    json!({
        "ensureNotificationBridge": 0,
        "depthSubscribe": 0,
        "depthUnsubscribe": 0,
        "marketTicksCalled": false,
        "securityCalls": 0,
        "depthCalls": 0,
    })
}

fn effective_limit(input: &WsLiveInput) -> i64 {
    if input.connection_limit <= 0 {
        20
    } else {
        input.connection_limit
    }
}

fn heartbeat(input: &WsLiveInput, provider_broker_id: &str) -> String {
    let server_time = FIXTURE_TIME.to_owned();
    let payload = json!({
        "type": "heartbeat",
        "at": FIXTURE_TIME,
        "intervalMs": input.heartbeat_interval_ms,
        "providerBrokerId": provider_broker_id,
        "liveClients": {
            "connected": 1,
            "limit": effective_limit(input),
            "atLimit": effective_limit(input) <= 1,
        },
    });
    wire(EventEnvelope {
        event_id: format!("heartbeat|live-websocket|{FIXTURE_TIME}"),
        event_type: "heartbeat".to_owned(),
        source: "system".to_owned(),
        entity_id: "live-websocket".to_owned(),
        server_time,
        payload,
    })
}

fn subscription_auxiliary_frames(
    input: &WsLiveInput,
    subscription: &WsLiveSubscriptions,
) -> Vec<String> {
    let mut frames = Vec::new();
    if subscription.console_refresh {
        frames.push(console_refresh());
    }
    if !input.security_error {
        for item in &subscription.security_details {
            frames.push(security_frame(input, subscription, item));
        }
    }
    if !input.depth_error {
        frames.extend(depth_frames(input, subscription, &input.depth_resolved_at));
    }
    frames
}

fn console_refresh() -> String {
    let payload = json!({"type": "console.refresh", "at": FIXTURE_TIME, "checkedAt": FIXTURE_TIME});
    wire(EventEnvelope {
        event_id: format!("console.refresh|console|{FIXTURE_TIME}"),
        event_type: "console.refresh".to_owned(),
        source: "system".to_owned(),
        entity_id: "console".to_owned(),
        server_time: FIXTURE_TIME.to_owned(),
        payload,
    })
}

fn security_frame(
    input: &WsLiveInput,
    subscription: &WsLiveSubscriptions,
    item: &WsLiveSecuritySubscription,
) -> String {
    let resolved_at = FIXTURE_TIME;
    let mut payload = json!({
        "request": {"market": item.market.trim().to_ascii_uppercase(), "symbol": item.symbol.trim().to_ascii_uppercase(), "instrumentId": item.instrument_id.trim().to_ascii_uppercase()},
        "security": {"name": "Tencent Holdings"},
        "meta": {"resolvedAt": resolved_at},
    });
    let object = payload.as_object_mut().expect("security payload object");
    object.insert(
        "type".to_owned(),
        Value::String("market.security-details".to_owned()),
    );
    object.insert("at".to_owned(), Value::String(resolved_at.to_owned()));
    object.insert(
        "brokerId".to_owned(),
        Value::String(subscription.provider_broker_id.clone()),
    );
    wire(EventEnvelope {
        event_id: format!(
            "market.security-details|{}|{resolved_at}",
            item.instrument_id.trim().to_ascii_uppercase()
        ),
        event_type: "market.security-details".to_owned(),
        source: "market-data".to_owned(),
        entity_id: item.instrument_id.trim().to_ascii_uppercase(),
        server_time: resolved_at.to_owned(),
        payload,
    })
}

fn depth_frames(
    input: &WsLiveInput,
    subscription: &WsLiveSubscriptions,
    resolved_at: &str,
) -> Vec<String> {
    subscription.depth.iter().filter_map(|item| {
        if input.depth_error { return None; }
        let instrument_id = item.instrument_id.trim().to_ascii_uppercase();
        let num = item.num.clamp(1, 50);
        let depth = input.depth_payload.clone().unwrap_or_else(|| json!({"bids": [{"price": "100"}]}));
        let mut payload = json!({
            "request": {"market": item.market.trim().to_ascii_uppercase(), "symbol": item.symbol.trim().to_ascii_uppercase(), "instrumentId": instrument_id, "num": num},
            "depth": depth,
            "meta": {"resolvedAt": FIXTURE_TIME},
        });
        let object = payload.as_object_mut().expect("depth payload object");
        object.insert("type".to_owned(), Value::String("market.depth".to_owned()));
        object.insert("at".to_owned(), Value::String(FIXTURE_TIME.to_owned()));
        object.insert("brokerId".to_owned(), Value::String(subscription.provider_broker_id.clone()));
        Some(wire(EventEnvelope {
            event_id: format!("market.depth|{instrument_id}|{num}|{FIXTURE_TIME}"),
            event_type: "market.depth".to_owned(),
            source: "market-data".to_owned(),
            entity_id: format!("{instrument_id}|{num}"),
            server_time: FIXTURE_TIME.to_owned(),
            payload,
        }))
    }).collect()
}

fn tick_frames(input: &WsLiveInput, subscription: &WsLiveSubscriptions) -> Vec<String> {
    if input.ticks_error {
        return Vec::new();
    }
    let mut seen = std::collections::BTreeSet::new();
    input
        .ticks
        .iter()
        .filter_map(|tick| {
            let instrument_id = tick.instrument_id.trim().to_ascii_uppercase();
            if instrument_id.is_empty()
                || !seen.insert(format!(
                    "{}|{}|{}",
                    subscription.provider_broker_id, instrument_id, tick.observed_at
                ))
            {
                return None;
            }
            let mut payload = tick.payload.clone();
            canonicalize_times(&mut payload);
            let object = payload.as_object_mut()?;
            object.insert(
                "brokerId".to_owned(),
                Value::String(subscription.provider_broker_id.clone()),
            );
            let event_type = object
                .get("type")
                .and_then(Value::as_str)
                .unwrap_or("market-data.tick")
                .to_owned();
            let server_time = object
                .get("at")
                .and_then(Value::as_str)
                .unwrap_or(FIXTURE_TIME)
                .to_owned();
            Some(wire(EventEnvelope {
                event_id: format!("{event_type}|{instrument_id}|{server_time}"),
                event_type,
                source: "market-data".to_owned(),
                entity_id: instrument_id,
                server_time,
                payload,
            }))
        })
        .collect()
}

fn notification(notification: &WsLiveNotification) -> String {
    let mut payload = Map::new();
    payload.insert(
        "type".to_owned(),
        Value::String("system.notification".to_owned()),
    );
    payload.insert(
        "id".to_owned(),
        Value::String(format!("system-notification-{}", notification.sequence)),
    );
    payload.insert("at".to_owned(), Value::String(FIXTURE_TIME.to_owned()));
    payload.insert(
        "level".to_owned(),
        Value::String(notification.level.clone()),
    );
    payload.insert(
        "title".to_owned(),
        Value::String(notification.title.clone()),
    );
    payload.insert(
        "source".to_owned(),
        Value::String(notification.source.clone()),
    );
    payload.insert(
        "brokerId".to_owned(),
        Value::String(notification.broker_id.clone()),
    );
    payload.insert(
        "category".to_owned(),
        Value::String(notification.category.clone()),
    );
    if !notification.message.is_empty() {
        payload.insert(
            "message".to_owned(),
            Value::String(notification.message.clone()),
        );
    }
    wire(EventEnvelope {
        event_id: format!("system-notification-{}", notification.sequence),
        event_type: "system.notification".to_owned(),
        source: "notification".to_owned(),
        entity_id: format!("system-notification-{}", notification.sequence),
        server_time: FIXTURE_TIME.to_owned(),
        payload: Value::Object(payload),
    })
}

fn normalized_subscriptions(input: Option<&WsLiveSubscriptions>) -> WsLiveSubscriptions {
    let Some(input) = input else {
        return WsLiveSubscriptions::default();
    };
    let mut active = input
        .active_instruments
        .iter()
        .map(|value| value.trim().to_ascii_uppercase())
        .filter(|value| !value.is_empty())
        .collect::<Vec<_>>();
    active.sort();
    active.dedup();
    let mut security = input
        .security_details
        .iter()
        .filter_map(|item| {
            let market = item.market.trim().to_ascii_uppercase();
            let symbol = item.symbol.trim().to_ascii_uppercase();
            let instrument_id = item.instrument_id.trim().to_ascii_uppercase();
            (!market.is_empty() && !symbol.is_empty() && !instrument_id.is_empty()).then_some(
                WsLiveSecuritySubscription {
                    market,
                    symbol,
                    instrument_id,
                },
            )
        })
        .collect::<Vec<_>>();
    security.sort_by(|left, right| left.instrument_id.cmp(&right.instrument_id));
    security.dedup_by(|left, right| left.instrument_id == right.instrument_id);
    let mut depth = input
        .depth
        .iter()
        .filter_map(|item| {
            let market = item.market.trim().to_ascii_uppercase();
            let symbol = item.symbol.trim().to_ascii_uppercase();
            let instrument_id = item.instrument_id.trim().to_ascii_uppercase();
            let num = item.num.clamp(1, 50);
            (!market.is_empty() && !symbol.is_empty() && !instrument_id.is_empty()).then_some(
                WsLiveDepthSubscription {
                    market,
                    symbol,
                    instrument_id,
                    num,
                },
            )
        })
        .collect::<Vec<_>>();
    depth.sort_by(|left, right| {
        left.instrument_id
            .cmp(&right.instrument_id)
            .then(left.num.cmp(&right.num))
    });
    depth
        .dedup_by(|left, right| left.instrument_id == right.instrument_id && left.num == right.num);
    WsLiveSubscriptions {
        provider_broker_id: input.provider_broker_id.trim().to_ascii_lowercase(),
        active_instruments: active,
        security_details: security,
        depth,
        console_refresh: input.console_refresh,
    }
}

fn canonicalize_times(value: &mut Value) {
    match value {
        Value::Array(values) => values.iter_mut().for_each(canonicalize_times),
        Value::Object(values) => values.values_mut().for_each(canonicalize_times),
        Value::String(text) if looks_like_timestamp(text) => *text = FIXTURE_TIME.to_owned(),
        _ => {}
    }
}

fn looks_like_timestamp(text: &str) -> bool {
    text.len() >= 20
        && text.as_bytes().get(4) == Some(&b'-')
        && text.as_bytes().get(10) == Some(&b'T')
        && text.ends_with('Z')
}

fn wire(envelope: EventEnvelope) -> String {
    serde_json::to_string(&envelope).expect("ws-live envelope serializes")
}
