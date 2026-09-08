//! Batch quote aggregation and snapshot normalization for the watchlist port.

use std::collections::{BTreeSet, HashMap};
use jftrade_settings::MarketDataProvider;
use jftrade_watchlist::normalize_instrument_id;
use serde_json::{json, Value};

use super::{
    string_array, ProductionWatchlistPort, WatchlistWritePortError, MAX_PAGE_LIMIT,
};
use crate::product::product_production_ports::provider_now_rfc3339;
use crate::product::product_production_ports::product_production_ports_trade::{
    qot_market_label, quote_market_code,
};

#[derive(Clone, Debug)]
pub(crate) struct WatchlistQuoteCacheEntry {
    pub(crate) quote: Value,
    pub(crate) cached_at: std::time::Instant,
}

fn val_to_f64(v: Option<&Value>) -> Option<f64> {
    v.and_then(|val| {
        val.as_f64()
            .or_else(|| val.as_str().and_then(|s| s.parse::<f64>().ok()))
            .or_else(|| val.as_i64().map(|i| i as f64))
    })
}

pub(crate) fn parse_helper_snapshot(
    snap: &Value,
    now: &str,
    source: &str,
) -> Option<(String, Value)> {
    let instrument_id = snap
        .get("instrument_id")
        .or_else(|| snap.get("instrumentId"))
        .and_then(Value::as_str)
        .map(str::to_owned)
        .or_else(|| {
            let market = snap.get("market").and_then(Value::as_str)?;
            let symbol = snap.get("symbol").and_then(Value::as_str)?;
            Some(format!("{market}.{symbol}"))
        })?;

    let canonical_id = normalize_instrument_id(&instrument_id).unwrap_or(instrument_id);

    let last_price = val_to_f64(snap.get("price"));
    let prev_close = val_to_f64(snap.get("previous_close_price"))
        .or_else(|| val_to_f64(snap.get("previousClose")))
        .or_else(|| val_to_f64(snap.get("last_close_price")));
    let volume = val_to_f64(snap.get("volume"));
    let turnover = val_to_f64(snap.get("turnover"));
    let name = snap
        .get("name")
        .and_then(Value::as_str)
        .filter(|s| !s.trim().is_empty())
        .map(str::to_owned);
    let observed_at = snap
        .get("observed_at")
        .or_else(|| snap.get("observedAt"))
        .and_then(Value::as_str)
        .filter(|s| !s.trim().is_empty())
        .map(str::to_owned)
        .unwrap_or_else(|| now.to_owned());
    let update_time = snap
        .get("quote_at")
        .or_else(|| snap.get("updateTime"))
        .and_then(Value::as_str)
        .filter(|s| !s.trim().is_empty())
        .map(str::to_owned);

    let mut quote_obj = serde_json::Map::new();
    quote_obj.insert("instrumentId".to_owned(), json!(canonical_id));
    quote_obj.insert("source".to_owned(), json!(source));
    quote_obj.insert("type".to_owned(), json!("stock"));
    quote_obj.insert("observedAt".to_owned(), json!(observed_at));
    quote_obj.insert("session".to_owned(), json!("regular"));

    if let Some(p) = last_price {
        quote_obj.insert("price".to_owned(), json!(p));
    }
    if let Some(prev) = prev_close {
        quote_obj.insert("previousClose".to_owned(), json!(prev));
    }
    if let (Some(p), Some(prev)) = (last_price, prev_close) {
        let change = p - prev;
        quote_obj.insert("change".to_owned(), json!(change));
        if prev != 0.0 {
            quote_obj.insert("changePercent".to_owned(), json!(change / prev * 100.0));
        }
    }
    if let Some(v) = volume {
        quote_obj.insert("volume".to_owned(), json!(v));
    }
    if let Some(t) = turnover {
        quote_obj.insert("turnover".to_owned(), json!(t));
    }
    if let Some(n) = name {
        quote_obj.insert("name".to_owned(), json!(n));
    }
    if let Some(u) = update_time {
        quote_obj.insert("updateTime".to_owned(), json!(u));
    }

    Some((canonical_id, Value::Object(quote_obj)))
}

impl ProductionWatchlistPort {
    pub(crate) fn handle_batch_quotes(
        &self,
        value: &Value,
    ) -> Result<Value, WatchlistWritePortError> {
        let instrument_ids = string_array(value.get("instrumentIds"), "instrumentIds")?;
        if instrument_ids.is_empty() || instrument_ids.len() > MAX_PAGE_LIMIT {
            return Err(WatchlistWritePortError {
                status: 400,
                code: "WATCHLIST_INVALID".to_owned(),
                message: format!("instrumentIds must contain 1-{MAX_PAGE_LIMIT} items"),
            });
        }

        let mut normalized = Vec::with_capacity(instrument_ids.len());
        let mut seen = BTreeSet::new();
        for raw in &instrument_ids {
            let id = normalize_instrument_id(raw).map_err(|err| WatchlistWritePortError {
                status: 400,
                code: "WATCHLIST_INVALID".to_owned(),
                message: err.to_string(),
            })?;
            if seen.insert(id.clone()) {
                normalized.push(id);
            }
        }

        let active_provider = self
            .active_provider_state
            .as_ref()
            .and_then(|s| s.get())
            .unwrap_or(MarketDataProvider::Futu);

        let now = provider_now_rfc3339();
        let mut quotes_map: HashMap<String, Value> = HashMap::new();
        let mut error_messages: HashMap<String, String> = HashMap::new();

        if (active_provider == MarketDataProvider::Akshare
            || active_provider == MarketDataProvider::Yfinance)
            && let Some(helper) = self.helper.clone()
        {
            let ids_to_fetch = normalized.clone();
            let provider_name = if active_provider == MarketDataProvider::Akshare {
                "akshare"
            } else {
                "yfinance"
            };
            let helper_entries = std::thread::spawn(move || {
                let rt = tokio::runtime::Builder::new_current_thread()
                    .enable_all()
                    .build()
                    .map_err(|e| e.to_string())?;
                rt.block_on(async {
                    if provider_name == "akshare" {
                        helper
                            .post_provider_json::<_, Value>(
                                "akshare",
                                &["snapshots"],
                                &json!({ "instrument_ids": ids_to_fetch }),
                            )
                            .await
                            .map(|val| {
                                val.get("entries")
                                    .and_then(Value::as_array)
                                    .cloned()
                                    .unwrap_or_default()
                            })
                            .map_err(|e| e.to_string())
                    } else {
                        let mut entries = Vec::new();
                        for id in &ids_to_fetch {
                            if let Some((market, symbol)) = id.split_once('.')
                                && let Ok(snap) = helper
                                    .get_provider_json::<Value>(
                                        "yfinance",
                                        &["snapshot", market, symbol],
                                    )
                                    .await
                            {
                                entries.push(snap);
                            }
                        }
                        Ok(entries)
                    }
                })
            })
            .join()
            .unwrap_or_else(|_| Err("helper thread panicked".to_owned()));

            if let Ok(entries) = helper_entries {
                for snap in &entries {
                    if let Some((id, quote)) = parse_helper_snapshot(snap, &now, provider_name) {
                        quotes_map.insert(id, quote);
                    }
                }
            }
        }

        if let Some(runtime) = self.trade_runtime.as_ref() {
            let mut securities = Vec::new();
            for id in &normalized {
                if quotes_map.contains_key(id) {
                    continue;
                }
                if let Some((market, symbol)) = id.split_once('.')
                    && let Some(market_code) = quote_market_code(market)
                {
                    securities.push(jftrade_integration_futu::TradeSecurity {
                        market: market_code,
                        code: symbol.to_owned(),
                    });
                }
            }

            if !securities.is_empty() {
                match runtime.security_snapshots(&securities) {
                    Ok(snapshots) => {
                        for snap in snapshots {
                            let symbol = snap
                                .get("symbol")
                                .and_then(Value::as_str)
                                .unwrap_or_default();
                            let market = snap
                                .get("market")
                                .and_then(Value::as_str)
                                .unwrap_or_default();
                            let id = if !market.is_empty() && !symbol.is_empty() {
                                if symbol.contains('.') {
                                    symbol.to_owned()
                                } else {
                                    format!("{market}.{symbol}")
                                }
                            } else {
                                symbol.to_owned()
                            };
                            let canonical_id = normalize_instrument_id(&id).unwrap_or(id);

                            let last_price = snap.get("lastPrice").and_then(Value::as_f64);
                            let prev_close = snap.get("previousClose").and_then(Value::as_f64);
                            let volume = snap.get("volume").and_then(|v| {
                                v.as_f64().or_else(|| v.as_i64().map(|i| i as f64))
                            });
                            let turnover = snap.get("turnover").and_then(|v| {
                                v.as_f64().or_else(|| v.as_i64().map(|i| i as f64))
                            });
                            let name = snap
                                .get("name")
                                .and_then(Value::as_str)
                                .filter(|s| !s.trim().is_empty())
                                .map(str::to_owned);
                            let sec_type = snap
                                .get("securityType")
                                .and_then(Value::as_str)
                                .filter(|s| !s.trim().is_empty())
                                .map(str::to_owned)
                                .unwrap_or_else(|| "stock".to_owned());
                            let observed_at = snap
                                .get("observedAt")
                                .and_then(Value::as_str)
                                .filter(|s| !s.trim().is_empty())
                                .map(str::to_owned)
                                .unwrap_or_else(|| now.clone());
                            let update_time = snap
                                .get("updateTime")
                                .and_then(Value::as_str)
                                .filter(|s| !s.trim().is_empty())
                                .map(str::to_owned);

                            let mut quote_obj = serde_json::Map::new();
                            quote_obj.insert("instrumentId".to_owned(), json!(canonical_id));
                            quote_obj.insert(
                                "source".to_owned(),
                                json!("futu:security-snapshot"),
                            );
                            quote_obj.insert("type".to_owned(), json!(sec_type));
                            quote_obj.insert("observedAt".to_owned(), json!(observed_at));
                            quote_obj.insert("session".to_owned(), json!("regular"));

                            if let Some(p) = last_price {
                                quote_obj.insert("price".to_owned(), json!(p));
                            }
                            if let Some(prev) = prev_close {
                                quote_obj.insert("previousClose".to_owned(), json!(prev));
                            }
                            if let (Some(p), Some(prev)) = (last_price, prev_close) {
                                let change = p - prev;
                                quote_obj.insert("change".to_owned(), json!(change));
                                if prev != 0.0 {
                                    quote_obj.insert(
                                        "changePercent".to_owned(),
                                        json!(change / prev * 100.0),
                                    );
                                }
                            }
                            if let Some(v) = volume {
                                quote_obj.insert("volume".to_owned(), json!(v));
                            }
                            if let Some(t) = turnover {
                                quote_obj.insert("turnover".to_owned(), json!(t));
                            }
                            if let Some(n) = name {
                                quote_obj.insert("name".to_owned(), json!(n));
                            }
                            if let Some(u) = update_time {
                                quote_obj.insert("updateTime".to_owned(), json!(u));
                            }

                            quotes_map.insert(canonical_id, Value::Object(quote_obj));
                        }
                    }
                    Err(err) => {
                        for sec in &securities {
                            let id = qot_market_label(sec.market)
                                .map(|m| format!("{m}.{}", sec.code))
                                .unwrap_or_else(|| format!("{}.{}", sec.market, sec.code));
                            error_messages.insert(id, err.to_string());
                        }
                    }
                }
            }
        }

        let missing_ids: Vec<String> = normalized
            .iter()
            .filter(|id| !quotes_map.contains_key(*id))
            .cloned()
            .collect();

        if !missing_ids.is_empty() && let Some(helper) = self.helper.clone() {
            let helper_missing = std::thread::spawn(move || {
                let rt = tokio::runtime::Builder::new_current_thread()
                    .enable_all()
                    .build()
                    .map_err(|e| e.to_string())?;
                rt.block_on(async {
                    let mut items = Vec::new();
                    if let Ok(ak_resp) = helper
                        .post_provider_json::<_, Value>(
                            "akshare",
                            &["snapshots"],
                            &json!({ "instrument_ids": missing_ids }),
                        )
                        .await
                        && let Some(entries) = ak_resp.get("entries").and_then(Value::as_array)
                    {
                        items.extend(entries.iter().cloned());
                    }
                    let found: BTreeSet<String> = items
                        .iter()
                        .filter_map(|s| {
                            s.get("instrument_id")
                                .or_else(|| s.get("instrumentId"))
                                .and_then(Value::as_str)
                        })
                        .map(str::to_owned)
                        .collect();
                    for id in &missing_ids {
                        if found.contains(id) {
                            continue;
                        }
                        if let Some((market, symbol)) = id.split_once('.')
                            && let Ok(snap) = helper
                                .get_provider_json::<Value>(
                                    "yfinance",
                                    &["snapshot", market, symbol],
                                )
                                .await
                        {
                            items.push(snap);
                        }
                    }
                    Ok(items)
                })
            })
            .join()
            .unwrap_or_else(|_| Err("helper thread panicked".to_owned()));

            if let Ok(entries) = helper_missing {
                for snap in &entries {
                    if let Some((id, quote)) = parse_helper_snapshot(snap, &now, "helper:snapshot") {
                        quotes_map.insert(id, quote);
                    }
                }
            }
        }

        let mut cache_guard = self.quote_cache.lock().unwrap_or_else(|e| e.into_inner());
        let now_inst = std::time::Instant::now();
        for (id, quote) in &quotes_map {
            cache_guard.insert(
                id.clone(),
                WatchlistQuoteCacheEntry {
                    quote: quote.clone(),
                    cached_at: now_inst,
                },
            );
        }

        let mut quotes = Vec::new();
        let mut errors = Vec::new();
        let ttl = std::time::Duration::from_secs(30);

        for id in &normalized {
            if let Some(quote) = quotes_map.remove(id) {
                quotes.push(quote);
            } else if let Some(entry) = cache_guard.get(id).filter(|e| e.cached_at.elapsed() <= ttl) {
                quotes.push(entry.quote.clone());
            } else if let Some(msg) = error_messages.get(id) {
                errors.push(json!({
                    "code": "SNAPSHOT_FAILED",
                    "instrumentId": id,
                    "message": msg,
                }));
            } else {
                errors.push(json!({
                    "code": "NO_SNAPSHOT",
                    "instrumentId": id,
                    "message": "snapshot source returned no result",
                }));
            }
        }

        Ok(json!({
            "quotes": quotes,
            "errors": errors,
            "observedAt": now,
        }))
    }
}
