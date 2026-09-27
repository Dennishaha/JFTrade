//! Regression tests for watchlist batch quote projection.
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
