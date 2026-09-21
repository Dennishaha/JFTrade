//! Batch quote aggregation and snapshot normalization for the watchlist port.

use std::collections::{BTreeSet, HashMap};
use jftrade_settings::MarketDataProvider;
use jftrade_watchlist::normalize_instrument_id;
use serde_json::{json, Value};

/// Go `WithQuoteCacheTTL` default: a batch-quote read inside this window is
/// served from the watchlist cache without touching the provider.
const WATCHLIST_QUOTE_CACHE_TTL: std::time::Duration = std::time::Duration::from_secs(30);

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
    /// `ActiveProviderState` generation observed when the quote was fetched.
    /// Go clears the watchlist quote cache in `ChangeQuoteProvider` and drops
    /// in-flight results owned by the previous provider; tagging every entry
    /// with the producing generation gives Rust the same fence.
    pub(crate) provider_generation: u64,
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
    fn provider_generation(&self) -> u64 {
        self.active_provider_state
            .as_ref()
            .map(|state| state.snapshot().generation)
            .unwrap_or_default()
    }

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
        let provider_generation = self.provider_generation();

        let now = provider_now_rfc3339();
        let mut quotes_map: HashMap<String, Value> = HashMap::new();
        let mut error_messages: HashMap<String, String> = HashMap::new();

        // Go serves a cached quote for the whole TTL window without calling the
        // provider again (`WithQuoteCacheTTL`); only ids that are missing or
        // owned by another provider generation are fetched below.
        let mut cached_ids: BTreeSet<String> = BTreeSet::new();
        {
            let cache_guard = self.quote_cache.lock().unwrap_or_else(|e| e.into_inner());
            for id in &normalized {
                let fresh = cache_guard.get(id).filter(|entry| {
                    entry.provider_generation == provider_generation
                        && entry.cached_at.elapsed() <= WATCHLIST_QUOTE_CACHE_TTL
                });
                if let Some(entry) = fresh {
                    quotes_map.insert(id.clone(), entry.quote.clone());
                    cached_ids.insert(id.clone());
                }
            }
        }

        if (active_provider == MarketDataProvider::Akshare
            || active_provider == MarketDataProvider::Yfinance)
            && let Some(helper) = self.helper.clone()
        {
            let ids_to_fetch: Vec<String> = normalized
                .iter()
                .filter(|id| !cached_ids.contains(*id))
                .cloned()
                .collect();
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

        let current_generation = self.provider_generation();
        let mut cache_guard = self.quote_cache.lock().unwrap_or_else(|e| e.into_inner());
        let now_inst = std::time::Instant::now();
        // A request that started under the previous provider must not
        // repopulate the cache after a switch, matching Go's in-flight fence.
        // Only quotes fetched by this request refresh the cache window; served
        // entries keep their original age instead of renewing the TTL.
        if current_generation == provider_generation {
            for (id, quote) in &quotes_map {
                if cached_ids.contains(id) {
                    continue;
                }
                cache_guard.insert(
                    id.clone(),
                    WatchlistQuoteCacheEntry {
                        quote: quote.clone(),
                        cached_at: now_inst,
                        provider_generation,
                    },
                );
            }
        }

        let mut quotes = Vec::new();
        let mut errors = Vec::new();

        for id in &normalized {
            if let Some(quote) = quotes_map.remove(id) {
                quotes.push(quote);
            } else if let Some(entry) = cache_guard.get(id).filter(|entry| {
                entry.provider_generation == current_generation
                    && entry.cached_at.elapsed() <= WATCHLIST_QUOTE_CACHE_TTL
            }) {
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

#[cfg(test)]
mod tests {
    use super::*;
    use crate::product::product_active_provider_state::ActiveProviderState;
    use crate::product::product_production_ports::SharedTradeReadRuntime;
    use crate::product::product_production_ports::product_production_ports_watchlist::ProductionWatchlistPort;
    use jftrade_integration_futu::SecuritySnapshotReadPort;
    use jftrade_settings::MarketDataProviderRuntimePort;
    use jftrade_store_sqlite::WatchlistStore;
    use std::sync::atomic::{AtomicUsize, Ordering};
    use std::sync::{Arc, Mutex};

    fn snapshot(symbol: &str) -> jftrade_marketdata::BrokerSecuritySnapshot {
        jftrade_marketdata::BrokerSecuritySnapshot {
            symbol: Some(symbol.to_owned()),
            market: Some(
                symbol
                    .split('.')
                    .next()
                    .unwrap_or_default()
                    .to_ascii_uppercase(),
            ),
            last_price: Some("101".parse().expect("last price")),
            previous_close: Some("100".parse().expect("previous close")),
            security_type: Some("stock".to_owned()),
            ..Default::default()
        }
    }

    struct CountingSnapshotReader {
        calls: AtomicUsize,
        switch_to: Option<MarketDataProvider>,
        state: Option<Arc<ActiveProviderState>>,
    }

    impl CountingSnapshotReader {
        fn new() -> Self {
            Self {
                calls: AtomicUsize::new(0),
                switch_to: None,
                state: None,
            }
        }

        fn switching_to(state: Arc<ActiveProviderState>, provider: MarketDataProvider) -> Self {
            Self {
                calls: AtomicUsize::new(0),
                switch_to: Some(provider),
                state: Some(state),
            }
        }
    }

    impl SecuritySnapshotReadPort for CountingSnapshotReader {
        fn query(
            &self,
            symbols: &[String],
        ) -> Result<Vec<jftrade_marketdata::BrokerSecuritySnapshot>, String> {
            self.calls.fetch_add(1, Ordering::SeqCst);
            // Model a provider switch that commits while this snapshot read is
            // still in flight, so the cache fence is exercised deterministically.
            if let (Some(state), Some(provider)) = (&self.state, self.switch_to) {
                let _ = state.activate(provider);
            }
            Ok(symbols.iter().map(|symbol| snapshot(symbol)).collect())
        }
    }

    fn watchlist_store(directory: &tempfile::TempDir) -> Arc<WatchlistStore> {
        let path = directory.path().join("watchlist.db");
        let connection = rusqlite::Connection::open(&path).expect("create watchlist database");
        jftrade_store_sqlite::initialize_current(&connection, "watchlist")
            .expect("initialize watchlist schema");
        drop(connection);
        Arc::new(WatchlistStore::open(&path).expect("watchlist store"))
    }

    fn watchlist_port(
        reader: Arc<CountingSnapshotReader>,
        state: Arc<ActiveProviderState>,
    ) -> (ProductionWatchlistPort, tempfile::TempDir) {
        let directory = tempfile::tempdir().expect("temporary directory");
        let runtime = SharedTradeReadRuntime::default();
        runtime.set_security_snapshots(Some(reader));
        let port = ProductionWatchlistPort {
            store: watchlist_store(&directory),
            trade_runtime: Some(Arc::new(runtime)),
            active_provider_state: Some(state),
            helper: None,
            quote_cache: Arc::new(Mutex::new(std::collections::HashMap::new())),
        };
        (port, directory)
    }

    fn batch(port: &ProductionWatchlistPort) -> Value {
        port.handle_batch_quotes(&json!({ "instrumentIds": ["HK.00700"] }))
            .expect("batch quotes")
    }

    #[test]
    fn quote_cache_serves_repeat_reads_within_one_provider_generation() {
        let reader = Arc::new(CountingSnapshotReader::new());
        let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
        let (port, _directory) = watchlist_port(Arc::clone(&reader), state);

        let first = batch(&port);
        let cached = batch(&port);

        assert_eq!(reader.calls.load(Ordering::SeqCst), 1);
        assert_eq!(first["quotes"], cached["quotes"]);
        assert!(cached["errors"].as_array().expect("errors").is_empty());
    }

    #[test]
    fn provider_switch_drops_quotes_cached_under_the_previous_provider() {
        let reader = Arc::new(CountingSnapshotReader::new());
        let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
        let (port, _directory) = watchlist_port(Arc::clone(&reader), Arc::clone(&state));
        batch(&port);
        assert_eq!(reader.calls.load(Ordering::SeqCst), 1);

        state
            .activate(MarketDataProvider::Akshare)
            .expect("switch provider");
        let switched = batch(&port);

        assert_eq!(
            reader.calls.load(Ordering::SeqCst),
            2,
            "a quote cached under the previous provider generation must not be served"
        );
        assert_eq!(switched["quotes"].as_array().expect("quotes").len(), 1);
        let cache = port.quote_cache.lock().expect("quote cache");
        let entry = cache.get("HK.00700").expect("cache entry");
        assert_eq!(entry.provider_generation, state.snapshot().generation);
    }

    #[test]
    fn in_flight_snapshot_does_not_repopulate_the_cache_after_a_provider_switch() {
        // Parity: go:452dea11:internal/watchlist/service_quotes_test.go:174 TestChangeQuoteProviderRejectsPreviousProviderInflightResults
        let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
        let reader = Arc::new(CountingSnapshotReader::switching_to(
            Arc::clone(&state),
            MarketDataProvider::Akshare,
        ));
        let (port, _directory) = watchlist_port(reader, state);

        let result = batch(&port);

        assert_eq!(result["quotes"].as_array().expect("quotes").len(), 1);
        assert!(
            port.quote_cache.lock().expect("quote cache").is_empty(),
            "a snapshot that started under the previous provider must not repopulate the cache"
        );
    }

    #[test]
    fn rejected_provider_switch_preserves_the_current_quote_cache() {
        // Parity: go:452dea11:internal/watchlist/service_quotes_test.go:210 TestChangeQuoteProviderFailurePreservesCurrentCache
        let reader = Arc::new(CountingSnapshotReader::new());
        let activation: Arc<
            dyn Fn(MarketDataProvider, Option<MarketDataProvider>) -> Result<(), String>
                + Send
                + Sync,
        > = Arc::new(|_, _| Err("provider health check failed".to_owned()));
        let state = Arc::new(
            ActiveProviderState::new(Some(MarketDataProvider::Futu)).with_activation(activation),
        );
        let (port, _directory) = watchlist_port(Arc::clone(&reader), Arc::clone(&state));
        batch(&port);
        assert_eq!(reader.calls.load(Ordering::SeqCst), 1);

        assert!(
            state.activate(MarketDataProvider::Akshare).is_err(),
            "the activation callback rejects the switch"
        );
        let cached = batch(&port);

        assert_eq!(
            reader.calls.load(Ordering::SeqCst),
            1,
            "a rejected provider switch must keep the current cache"
        );
        assert_eq!(cached["quotes"].as_array().expect("quotes").len(), 1);
    }
}
