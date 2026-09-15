use serde_json::{Value, json};
use std::collections::BTreeMap;

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
                        phys.subscribed_at
                            .as_deref()
                            .map(|s| json!(s))
                            .unwrap_or(Value::Null),
                        phys.unsubscribe_eligible_at
                            .as_deref()
                            .map(|s| json!(s))
                            .unwrap_or(Value::Null),
                        phys.last_error
                            .as_deref()
                            .map(|s| json!(s))
                            .unwrap_or(Value::Null),
                    )
                } else {
                    (default_entry_state, Value::Null, Value::Null, Value::Null)
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
            phys.total_used_quota
                .map(|q| json!(q))
                .unwrap_or(Value::Null),
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
        .and_then(|dt| {
            dt.format(&time::format_description::well_known::Rfc3339)
                .ok()
        })
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

/// Go's `ErrProviderChanged` projection: a read that raced a provider switch
/// must fail with 409 `MARKET_DATA_PROVIDER_CHANGED` instead of returning data
/// produced by the retired provider generation.
pub(crate) fn provider_changed_error() -> MarketDataQuoteReadSnapshotError {
    MarketDataQuoteReadSnapshotError::Failed {
        status: 409,
        code: "MARKET_DATA_PROVIDER_CHANGED".to_owned(),
        message: "market-data provider changed".to_owned(),
        retry_after_seconds: None,
    }
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
            // Go's yfinance/akshare adapters classify helper runtime warming and
            // akshare pool/upstream pressure into ErrProviderWarming/ErrProviderBusy,
            // and the transport layer renders them as 503 with a fixed
            // Retry-After contract.  Keep that mapping at the read owner so the
            // wire envelope matches Go instead of leaking the helper's
            // provider-specific codes.
            if let Some((status, mapped_code, mapped_message, retry_after)) =
                classify_helper_runtime_code(&code)
            {
                return MarketDataQuoteReadSnapshotError::Failed {
                    status,
                    code: mapped_code.to_owned(),
                    message: mapped_message.to_owned(),
                    retry_after_seconds: Some(retry_after),
                };
            }
            let error_code = if !code.is_empty() {
                code
            } else {
                default_code.to_owned()
            };
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

fn classify_helper_runtime_code(code: &str) -> Option<(u16, &'static str, &'static str, u64)> {
    match code.trim().to_ascii_uppercase().as_str() {
        "YFINANCE_RUNTIME_WARMING" | "AKSHARE_RUNTIME_WARMING" | "PROVIDER_RUNTIME_WARMING" => {
            Some((
                503,
                "MARKET_DATA_PROVIDER_WARMING",
                "行情服务正在预热，请稍后重试",
                1,
            ))
        }
        "AKSHARE_POOL_BUSY" | "AKSHARE_UPSTREAM_TIMEOUT" => Some((
            503,
            "MARKET_DATA_PROVIDER_BUSY",
            "行情服务当前繁忙，请稍后重试",
            2,
        )),
        _ => None,
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

#[cfg(test)]
mod tests {
    use super::*;
    use jftrade_integration_marketdata_helper::HttpAdapterError;

    fn remote_error(status: u16, code: &str, message: &str) -> HttpAdapterError {
        HttpAdapterError::Remote {
            status,
            code: code.to_owned(),
            message: message.to_owned(),
            retry_after_seconds: None,
        }
    }

    #[test]
    fn market_data_read_errors_expose_provider_warmup_retry_signal() {
        // Parity: go:452dea11:internal/api/marketdata/routes_boundaries_test.go:286 TestMarketDataReadErrorsExposeProviderWarmupRetrySignal
        for code in [
            "YFINANCE_RUNTIME_WARMING",
            "AKSHARE_RUNTIME_WARMING",
            "PROVIDER_RUNTIME_WARMING",
        ] {
            let error = map_helper_quote_error(
                remote_error(503, code, "runtime is warming up"),
                "MARKET_CANDLES_FAILED",
            );
            assert!(matches!(
                error,
                MarketDataQuoteReadSnapshotError::Failed {
                    status: 503,
                    ref code,
                    ref message,
                    retry_after_seconds: Some(1),
                } if code == "MARKET_DATA_PROVIDER_WARMING"
                    && message == "行情服务正在预热，请稍后重试"
            ));
        }
    }

    #[test]
    fn market_data_read_errors_expose_provider_busy_retry_signal() {
        // Parity: go:452dea11:internal/api/marketdata/routes_boundaries_test.go:300 TestMarketDataReadErrorsExposeProviderBusyRetrySignal
        for code in ["AKSHARE_POOL_BUSY", "AKSHARE_UPSTREAM_TIMEOUT"] {
            let error = map_helper_quote_error(
                remote_error(503, code, "worker pool is busy"),
                "MARKET_CANDLES_FAILED",
            );
            assert!(matches!(
                error,
                MarketDataQuoteReadSnapshotError::Failed {
                    status: 503,
                    ref code,
                    ref message,
                    retry_after_seconds: Some(2),
                } if code == "MARKET_DATA_PROVIDER_BUSY"
                    && message == "行情服务当前繁忙，请稍后重试"
            ));
        }
    }

    #[test]
    fn market_data_read_errors_preserve_unclassified_helper_failures() {
        let error = map_helper_quote_error(
            remote_error(429, "RATE_LIMITED", "slow down"),
            "MARKET_CANDLES_FAILED",
        );
        assert!(matches!(
            error,
            MarketDataQuoteReadSnapshotError::Failed {
                status: 429,
                ref code,
                retry_after_seconds: None,
                ..
            } if code == "RATE_LIMITED"
        ));
    }
}
