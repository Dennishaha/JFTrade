//! Batch quote aggregation and snapshot normalization for the watchlist port.

use std::collections::{BTreeSet, HashMap};
use jftrade_settings::MarketDataProvider;
use jftrade_watchlist::normalize_instrument_id;
use serde_json::{json, Value};

/// Go's service fallback when a provider does not expose a quote cache TTL.
const DEFAULT_QUOTE_CACHE_TTL: std::time::Duration = std::time::Duration::from_millis(2500);
/// Helper providers expose their polling interval as the quote cache TTL.
const HELPER_QUOTE_CACHE_TTL: std::time::Duration = std::time::Duration::from_secs(15);

fn quote_cache_ttl(provider: MarketDataProvider) -> std::time::Duration {
    match provider {
        MarketDataProvider::Futu => DEFAULT_QUOTE_CACHE_TTL,
        MarketDataProvider::Akshare | MarketDataProvider::Yfinance => HELPER_QUOTE_CACHE_TTL,
    }
}

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

        // Serialize physical reads for overlapping requests. The cache is
        // checked only after acquiring this gate, so a follower observes the
        // leader's result and does not issue a duplicate provider call.
        let _quote_fetch_guard = self
            .quote_fetch_lock
            .lock()
            .unwrap_or_else(|error| error.into_inner());
        let active_provider = self
            .active_provider_state
            .as_ref()
            .and_then(|s| s.get())
            .unwrap_or(MarketDataProvider::Futu);
        let provider_generation = self.provider_generation();
        let cache_ttl = quote_cache_ttl(active_provider);

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
                        && entry.cached_at.elapsed() <= cache_ttl
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

                            let session = snap
                                .get("session")
                                .and_then(Value::as_str)
                                .map(str::to_ascii_lowercase)
                                .filter(|value| {
                                    matches!(
                                        value.as_str(),
                                        "pre" | "regular" | "after" | "overnight"
                                    )
                                })
                                .unwrap_or_else(|| "regular".to_owned());
                            let active = match session.as_str() {
                                "pre" => snap.get("preMarket"),
                                "after" => snap.get("afterMarket"),
                                "overnight" => snap.get("overnight"),
                                _ => None,
                            };
                            let active_price = active.and_then(|value| {
                                val_to_f64(value.get("price")).filter(|price| *price > 0.0)
                            });
                            let last_price = active_price
                                .or_else(|| val_to_f64(snap.get("lastPrice")));
                            let prev_close = val_to_f64(snap.get("previousClose"));
                            let volume = val_to_f64(snap.get("volume"));
                            let turnover = val_to_f64(snap.get("turnover"));
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
                            quote_obj.insert("session".to_owned(), json!(session));

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
                    && entry.cached_at.elapsed() <= quote_cache_ttl(active_provider)
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
    use std::sync::{Arc, Condvar, Mutex};
    use std::thread;
    use std::time::{Duration, Instant};

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

    fn extended_snapshot(
        symbol: &str,
        session: &str,
    ) -> jftrade_marketdata::BrokerSecuritySnapshot {
        let extended = |price: &str, volume: &str, turnover: &str| {
            jftrade_marketdata::ExtendedQuoteSnapshot {
                price: Some(price.parse().expect("extended price")),
                quote_time: Some("2026-07-18T20:00:00Z".to_owned()),
                volume: Some(volume.parse().expect("extended volume")),
                turnover: Some(turnover.parse().expect("extended turnover")),
                ..Default::default()
            }
        };
        let (pre_market, after_market, overnight) = match session {
            "pre" => (Some(extended("113.20", "10", "700")), None, None),
            "after" => (None, Some(extended("118.40", "12", "900")), None),
            "overnight" => (None, None, Some(extended("110.70", "8", "500"))),
            _ => (None, None, None),
        };
        jftrade_marketdata::BrokerSecuritySnapshot {
            symbol: Some(symbol.to_owned()),
            market: Some(
                symbol
                    .split('.')
                    .next()
                    .unwrap_or_default()
                    .to_ascii_uppercase(),
            ),
            last_price: Some("114.97".parse().expect("regular price")),
            previous_close: Some("112.50".parse().expect("previous close")),
            volume: Some("42".parse().expect("regular volume")),
            turnover: Some("1200".parse().expect("regular turnover")),
            security_type: Some("stock".to_owned()),
            session: Some(session.to_owned()),
            pre_market,
            after_market,
            overnight,
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

    struct ExtendedSnapshotReader {
        calls: AtomicUsize,
        session: &'static str,
    }

    impl SecuritySnapshotReadPort for ExtendedSnapshotReader {
        fn query(
            &self,
            symbols: &[String],
        ) -> Result<Vec<jftrade_marketdata::BrokerSecuritySnapshot>, String> {
            self.calls.fetch_add(1, Ordering::SeqCst);
            Ok(symbols
                .iter()
                .map(|symbol| extended_snapshot(symbol, self.session))
                .collect())
        }
    }

    struct BlockingSnapshotReader {
        calls: AtomicUsize,
        started: (Mutex<bool>, Condvar),
        release: (Mutex<bool>, Condvar),
    }

    impl BlockingSnapshotReader {
        fn new() -> Self {
            Self {
                calls: AtomicUsize::new(0),
                started: (Mutex::new(false), Condvar::new()),
                release: (Mutex::new(false), Condvar::new()),
            }
        }

        fn wait_started(&self) {
            let mut started = self.started.0.lock().expect("started lock");
            while !*started {
                started = self.started.1.wait(started).expect("started wait");
            }
        }

        fn release(&self) {
            *self.release.0.lock().expect("release lock") = true;
            self.release.1.notify_all();
        }
    }

    impl SecuritySnapshotReadPort for BlockingSnapshotReader {
        fn query(
            &self,
            symbols: &[String],
        ) -> Result<Vec<jftrade_marketdata::BrokerSecuritySnapshot>, String> {
            self.calls.fetch_add(1, Ordering::SeqCst);
            *self.started.0.lock().expect("started lock") = true;
            self.started.1.notify_all();
            let mut release = self.release.0.lock().expect("release lock");
            while !*release {
                release = self.release.1.wait(release).expect("release wait");
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

    fn watchlist_port<R>(
        reader: Arc<R>,
        state: Arc<ActiveProviderState>,
    ) -> (ProductionWatchlistPort, tempfile::TempDir)
    where
        R: SecuritySnapshotReadPort + 'static,
    {
        let directory = tempfile::tempdir().expect("temporary directory");
        let runtime = SharedTradeReadRuntime::default();
        runtime.set_security_snapshots(Some(reader));
        let port = ProductionWatchlistPort {
            store: watchlist_store(&directory),
            trade_runtime: Some(Arc::new(runtime)),
            active_provider_state: Some(state),
            helper: None,
            quote_cache: Arc::new(Mutex::new(std::collections::HashMap::new())),
            quote_fetch_lock: Arc::new(Mutex::new(())),
        };
        (port, directory)
    }

    fn batch(port: &ProductionWatchlistPort) -> Value {
        batch_for(port, &["HK.00700"])
    }

    fn batch_for(port: &ProductionWatchlistPort, instrument_ids: &[&str]) -> Value {
        port.handle_batch_quotes(&json!({ "instrumentIds": instrument_ids }))
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
    fn batch_quotes_selects_extended_session_price_and_change() {
        // Parity: go:452dea11:internal/watchlist/futu/source_test.go:418
        // TestWatchlistQuoteSelectsExtendedSessionPriceAndChange.
        for (session, expected_price, expected_change, expected_volume, expected_turnover) in [
            ("pre", 113.2, 0.7, 42.0, 1200.0),
            ("after", 118.4, 5.9, 42.0, 1200.0),
            ("overnight", 110.7, -1.8, 42.0, 1200.0),
        ] {
            let reader = Arc::new(ExtendedSnapshotReader {
                calls: AtomicUsize::new(0),
                session,
            });
            let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
            let (port, _directory) = watchlist_port(Arc::clone(&reader), state);

            let response = batch(&port);
            let quote = &response["quotes"][0];
            assert_eq!(quote["session"], session);
            assert_eq!(quote["price"], expected_price);
            let change = quote["change"].as_f64().expect("change");
            assert!((change - expected_change).abs() < 1e-9);
            assert!(quote["changePercent"].as_f64().is_some());
            assert_eq!(quote["volume"], expected_volume);
            assert_eq!(quote["turnover"], expected_turnover);
            assert_eq!(reader.calls.load(Ordering::SeqCst), 1);
        }
    }

    #[test]
    fn overlapping_batch_quotes_share_one_snapshot_read() {
        // Parity: go:452dea11:internal/watchlist/service_quotes_test.go:62
        // TestBatchQuotesCachesAndSingleflightsOverlappingRequests.
        let reader = Arc::new(BlockingSnapshotReader::new());
        let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
        let (port, _directory) = watchlist_port(Arc::clone(&reader), state);
        let port = Arc::new(port);
        let leader_port = Arc::clone(&port);
        let leader = thread::spawn(move || batch_for(&leader_port, &["HK.00700", "HK.09988"]));
        reader.wait_started();

        let follower_port = Arc::clone(&port);
        let follower = thread::spawn(move || batch_for(&follower_port, &["HK.00700"]));
        let deadline = Instant::now() + Duration::from_secs(1);
        while reader.calls.load(Ordering::SeqCst) < 2 && Instant::now() < deadline {
            thread::yield_now();
        }
        reader.release();
        let leader_response = leader.join().expect("leader join");
        let follower_response = follower.join().expect("follower join");
        assert_eq!(leader_response["quotes"].as_array().map(Vec::len), Some(2));
        assert_eq!(follower_response["quotes"].as_array().map(Vec::len), Some(1));
        let combined = batch_for(&port, &["HK.00700", "HK.09988"]);
        assert_eq!(combined["quotes"].as_array().map(Vec::len), Some(2));
        assert_eq!(
            reader.calls.load(Ordering::SeqCst),
            1,
            "overlapping requests must share one physical snapshot read"
        );
    }

    #[test]
    fn batch_quote_cache_uses_provider_ttl_before_refetching() {
        // Parity: go:452dea11:internal/watchlist/service_quotes_test.go:117
        // TestBatchQuotesHonorsProviderCachePolicy. Helper-backed providers
        // publish a 15-second polling interval; the cache is still fresh at
        // 10 seconds and must be refetched after 16 seconds.
        let reader = Arc::new(CountingSnapshotReader::new());
        let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Akshare)));
        let (port, _directory) = watchlist_port(Arc::clone(&reader), state);
        batch(&port);
        port.quote_cache
            .lock()
            .expect("quote cache")
            .get_mut("HK.00700")
            .expect("cached quote")
            .cached_at = Instant::now()
            .checked_sub(Duration::from_secs(10))
            .expect("instant subtraction");
        batch(&port);
        assert_eq!(
            reader.calls.load(Ordering::SeqCst),
            1,
            "provider TTL must keep this quote cached before expiry"
        );
        port.quote_cache
            .lock()
            .expect("quote cache")
            .get_mut("HK.00700")
            .expect("cached quote")
            .cached_at = Instant::now()
            .checked_sub(Duration::from_secs(16))
            .expect("instant subtraction");
        batch(&port);
        assert_eq!(
            reader.calls.load(Ordering::SeqCst),
            2,
            "provider TTL must expire this quote before refetching"
        );
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
