use std::collections::BTreeMap;
use serde_json::{json, Value};

use jftrade_marketdata::{DemandSnapshot, PhysicalSubscriptionSnapshot};

use crate::product::MarketDataQuoteReadSnapshotError;
use crate::product::product_query::has_invalid_percent_escape;

pub(crate) fn render_subscriptions_data(
    snapshot: &DemandSnapshot,
    physical: Option<&PhysicalSubscriptionSnapshot>,
) -> Value {
    let mut by_market_map: BTreeMap<String, usize> = BTreeMap::new();

    let physical_entries_map = physical.map(|p| {
        p.entries
            .iter()
            .map(|e| (e.key.as_str(), e))
            .collect::<BTreeMap<_, _>>()
    });

    let default_entry_state = if physical.is_some() {
        "pending_subscribe"
    } else {
        "unmanaged"
    };

    let entries = snapshot
        .entries
        .iter()
        .map(|entry| {
            *by_market_map.entry(entry.market.clone()).or_default() += 1;
            let created_at = format_unix_millis_rfc3339(entry.created_at_ms);
            let updated_at = format_unix_millis_rfc3339(entry.updated_at_ms);

            let physical_key = match entry.channel.as_str() {
                "ORDER_BOOK" => format!("ORDER_BOOK:{}", entry.instrument_id),
                "KLINE" => {
                    if let Some(interval) = &entry.interval {
                        format!("KLINE:{}:{}", entry.instrument_id, interval)
                    } else {
                        format!("BASIC:{}", entry.instrument_id)
                    }
                }
                _ => format!("BASIC:{}", entry.instrument_id),
            };

            let (broker_state, subscribed_at, unsubscribe_eligible_at, last_error) =
                if let Some(map) = &physical_entries_map
                    && let Some(phys) = map.get(physical_key.as_str())
                {
                    (
                        phys.broker_state.as_str(),
                        phys.subscribed_at.as_deref().map(|s| json!(s)).unwrap_or(Value::Null),
                        phys.unsubscribe_eligible_at.as_deref().map(|s| json!(s)).unwrap_or(Value::Null),
                        phys.last_error.as_deref().map(|s| json!(s)).unwrap_or(Value::Null),
                    )
                } else {
                    (
                        default_entry_state,
                        Value::Null,
                        Value::Null,
                        Value::Null,
                    )
                };

            json!({
                "brokerState": broker_state,
                "channel": entry.channel,
                "consumers": entry.consumers,
                "createdAt": created_at,
                "depthLevel": entry.depth_level,
                "instrumentId": entry.instrument_id,
                "interval": entry.interval,
                "key": entry.key,
                "lastError": last_error,
                "market": entry.market,
                "refCount": entry.ref_count,
                "subscribedAt": subscribed_at,
                "symbol": entry.symbol,
                "unsubscribeEligibleAt": unsubscribe_eligible_at,
                "updatedAt": updated_at,
            })
        })
        .collect::<Vec<_>>();

    let by_market = by_market_map
        .into_iter()
        .map(|(market, used)| {
            json!({
                "limit": Value::Null,
                "market": market,
                "remaining": Value::Null,
                "used": used,
            })
        })
        .collect::<Vec<_>>();

    let (
        desired_count,
        own_active_count,
        pending_release_count,
        total_used_quota,
        remain_quota,
        broker_state,
    ) = if let Some(phys) = physical {
        (
            phys.desired_count,
            phys.own_active_count,
            phys.pending_release_count,
            phys.total_used_quota.map(|q| json!(q)).unwrap_or(Value::Null),
            phys.remain_quota.map(|q| json!(q)).unwrap_or(Value::Null),
            json!({
                "checkedAt": phys.checked_at.as_deref().map(|s| json!(s)).unwrap_or(Value::Null),
                "connectionGeneration": phys.connection_generation.map(|g| json!(g)).unwrap_or(Value::Null),
                "desiredCount": phys.desired_count,
                "entries": phys.entries,
                "fallbackCount": phys.fallback_count,
                "lastError": phys.last_error.as_deref().map(|s| json!(s)).unwrap_or(Value::Null),
                "observedConnectionGeneration": phys.observed_connection_generation.map(|g| json!(g)).unwrap_or(Value::Null),
                "ownActiveCount": phys.own_active_count,
                "ownUsedQuota": phys.own_used_quota.map(|q| json!(q)).unwrap_or(Value::Null),
                "pendingReleaseCount": phys.pending_release_count,
                "reconciledAt": phys.reconciled_at.as_deref().map(|s| json!(s)).unwrap_or(Value::Null),
                "remainQuota": phys.remain_quota.map(|q| json!(q)).unwrap_or(Value::Null),
                "totalUsedQuota": phys.total_used_quota.map(|q| json!(q)).unwrap_or(Value::Null),
            }),
        )
    } else {
        (
            snapshot.logical_count,
            0,
            0,
            Value::Null,
            Value::Null,
            json!({
                "desiredCount": snapshot.logical_count,
                "entries": [],
                "ownActiveCount": 0,
                "pendingReleaseCount": 0,
                "remainQuota": Value::Null,
                "totalUsedQuota": Value::Null,
            }),
        )
    };

    json!({
        "brokerState": broker_state,
        "desiredCount": desired_count,
        "entries": entries,
        "ownActiveCount": own_active_count,
        "pendingReleaseCount": pending_release_count,
        "quota": {
            "byMarket": by_market,
            "totalLimit": Value::Null,
            "totalRemaining": Value::Null,
            "totalUsed": snapshot.logical_count,
        },
        "remainQuota": remain_quota,
        "totalActiveSubscriptions": snapshot.logical_count,
        "totalUsedQuota": total_used_quota,
    })
}

pub(crate) fn format_unix_millis_rfc3339(ms: i64) -> String {
    time::OffsetDateTime::from_unix_timestamp_nanos(i128::from(ms) * 1_000_000)
        .ok()
        .and_then(|dt| dt.format(&time::format_description::well_known::Rfc3339).ok())
        .unwrap_or_else(|| "1970-01-01T00:00:00Z".to_owned())
}


pub(crate) fn current_unix_millis() -> i64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .ok()
        .and_then(|d| i64::try_from(d.as_millis()).ok())
        .unwrap_or_default()
}

pub(crate) fn parse_market_symbol_path(
    suffix: &str,
) -> Result<(String, String), MarketDataQuoteReadSnapshotError> {
    if has_invalid_percent_escape(suffix) {
        return Err(MarketDataQuoteReadSnapshotError::Failed {
            status: 400,
            code: "BAD_REQUEST".to_owned(),
            message: "invalid URL escape".to_owned(),
            retry_after_seconds: None,
        });
    }
    let mut parts = suffix.splitn(2, '/');
    let market = parts.next().unwrap_or_default().trim();
    let symbol = parts.next().unwrap_or_default().trim();
    if market.is_empty() || symbol.is_empty() {
        return Err(MarketDataQuoteReadSnapshotError::Failed {
            status: 400,
            code: "BAD_REQUEST".to_owned(),
            message: "invalid instrument".to_owned(),
            retry_after_seconds: None,
        });
    }
    Ok((market.to_owned(), symbol.to_owned()))
}

pub(crate) fn map_helper_quote_error(
    error: jftrade_integration_marketdata_helper::HttpAdapterError,
    default_code: &str,
) -> MarketDataQuoteReadSnapshotError {
    match error {
        jftrade_integration_marketdata_helper::HttpAdapterError::Remote {
            status,
            code,
            message,
            retry_after_seconds,
        } => {
            let error_code = if !code.is_empty() { code } else { default_code.to_owned() };
            MarketDataQuoteReadSnapshotError::Failed {
                status,
                code: error_code,
                message,
                retry_after_seconds,
            }
        }
        jftrade_integration_marketdata_helper::HttpAdapterError::Timeout => {
            MarketDataQuoteReadSnapshotError::Failed {
                status: 504,
                code: "GATEWAY_TIMEOUT".to_owned(),
                message: "market-data helper request timed out".to_owned(),
                retry_after_seconds: None,
            }
        }
        jftrade_integration_marketdata_helper::HttpAdapterError::Unavailable(msg) => {
            MarketDataQuoteReadSnapshotError::Unavailable(msg)
        }
        jftrade_integration_marketdata_helper::HttpAdapterError::InvalidResponse(msg) => {
            MarketDataQuoteReadSnapshotError::Failed {
                status: 502,
                code: "BAD_GATEWAY".to_owned(),
                message: msg,
                retry_after_seconds: None,
            }
        }
        other => MarketDataQuoteReadSnapshotError::Failed {
            status: 500,
            code: default_code.to_owned(),
            message: other.to_string(),
            retry_after_seconds: None,
        },
    }
}

pub(crate) fn broker_polling_subscription_response(
    consumer_id: &str,
    broker_id: &str,
    instruments: Vec<Value>,
    action: &str,
) -> Value {
    let instruments_val = if action == "heartbeat" {
        Value::Null
    } else {
        json!(instruments)
    };
    json!({
        "action": action,
        "consumerId": consumer_id,
        "desiredCount": 0,
        "entries": [],
        "instruments": instruments_val,
        "ownActiveCount": 0,
        "pendingReleaseCount": 0,
        "providerBrokerId": broker_id.trim().to_ascii_lowercase(),
        "quota": {
            "byMarket": [],
            "totalLimit": Value::Null,
            "totalRemaining": Value::Null,
            "totalUsed": 0,
        },
        "totalActiveSubscriptions": 0,
        "transport": {
            "mode": "snapshot-poll-fallback",
        },
    })
}
