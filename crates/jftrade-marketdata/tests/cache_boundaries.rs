use jftrade_kernel::DecimalText;
use jftrade_marketdata::{
    CacheLookup, ExtendedQuoteSnapshot, MarketDataError, Tick, TickCache, TradeQuoteSnapshot,
};
use rust_decimal::Decimal;

fn price_dec(value: &str) -> Decimal {
    value.parse().expect("valid decimal fixture")
}

fn decimal(value: &str) -> DecimalText {
    value.parse().expect("valid decimal fixture")
}

fn tick(
    instrument_id: &str,
    price: &str,
    volume: &str,
    observed_at_ms: i64,
    provider_generation: u64,
    snapshot: Option<TradeQuoteSnapshot>,
) -> Tick {
    Tick {
        instrument_id: instrument_id.to_owned(),
        price: price_dec(price),
        volume: decimal(volume),
        snapshot,
        observed_at_ms,
        provider_generation,
    }
}

fn quote_snapshot(previous_close: &str, after_market_price: Option<&str>) -> TradeQuoteSnapshot {
    TradeQuoteSnapshot {
        symbol: Some("AAPL".to_owned()),
        previous_close: Some(price_dec(previous_close)),
        after_market: after_market_price.map(|price| ExtendedQuoteSnapshot {
            price: Some(price_dec(price)),
            ..ExtendedQuoteSnapshot::default()
        }),
        ..TradeQuoteSnapshot::default()
    }
}

#[test]
fn cache_retains_latest_sample_for_trimmed_case_insensitive_key() {
    let mut cache = TickCache::new(2);
    let generation = 7;

    for (instrument_id, price, observed_at_ms) in [
        (" us.aapl ", "100", 1_000),
        ("US.AAPL", "101", 1_001),
        ("us.aapl", "102", 1_002),
    ] {
        cache
            .insert(
                tick(instrument_id, price, "10", observed_at_ms, generation, None),
                generation,
            )
            .expect("current-generation tick");
    }

    assert_eq!(cache.instrument_count(), 1);
    assert_eq!(
        cache.lookup("  Us.Aapl ", 1_002, 0),
        CacheLookup::Fresh(tick("us.aapl", "102", "10", 1_002, generation, None))
    );

    // A zero capacity is clamped to one retained sample and still exposes the
    // most recent normalized key through the same lookup path.
    let mut single_sample_cache = TickCache::new(0);
    single_sample_cache
        .insert(
            tick(" hk.00700 ", "321.4", "100", 2_000, generation, None),
            generation,
        )
        .expect("current-generation tick");
    single_sample_cache
        .insert(
            tick("HK.00700", "322.5", "101", 2_001, generation, None),
            generation,
        )
        .expect("current-generation tick");
    assert!(matches!(
        single_sample_cache.lookup("hk.00700", 2_001, 0),
        CacheLookup::Fresh(Tick {
            price,
            observed_at_ms: 2_001,
            ..
        }) if price == price_dec("322.5")
    ));
}

#[test]
fn cache_rejects_empty_generation_and_backwards_timestamp_inputs() {
    let generation = 3;
    let mut cache = TickCache::new(2);

    assert_eq!(
        cache.insert(tick("  ", "100", "1", 1_000, generation, None), generation,),
        Err(MarketDataError::InvalidSubscription(
            "tick instrumentId is required".to_owned(),
        ))
    );
    assert_eq!(cache.instrument_count(), 0);

    let current = tick("US.AAPL", "100", "1", 1_000, generation, None);
    assert_eq!(
        cache.insert(current.clone(), generation + 1),
        Err(MarketDataError::ProviderChanged)
    );
    cache
        .insert(current.clone(), generation)
        .expect("current-generation tick");

    assert_eq!(
        cache.insert(
            tick("US.AAPL", "101", "2", 999, generation, None),
            generation,
        ),
        Err(MarketDataError::InvalidSubscription(
            "tick timestamp moved backwards".to_owned(),
        ))
    );
    assert_eq!(
        cache.require_fresh("US.AAPL", 1_000, 0),
        Ok(current.clone())
    );
    assert_eq!(
        cache.require_fresh("US.AAPL", 1_001, 0),
        Err(MarketDataError::CacheStale("US.AAPL".to_owned()))
    );

    cache.clear();
    assert_eq!(cache.instrument_count(), 0);
    assert_eq!(cache.lookup("US.AAPL", 1_000, 0), CacheLookup::Missing);
}

#[test]
fn cache_treats_clock_rollback_as_stale() {
    let generation = 4;
    let mut cache = TickCache::new(1);
    let current = tick("US.AAPL", "100", "1", 2_000, generation, None);
    cache
        .insert(current, generation)
        .expect("current-generation tick");

    // A wall clock that moves behind the observation must not be accepted as
    // a zero-age sample merely because the age subtraction saturates.
    assert_eq!(
        cache.lookup("US.AAPL", 1_999, 10),
        CacheLookup::Stale(tick("US.AAPL", "100", "1", 2_000, generation, None))
    );
}

#[test]
fn cache_preserves_same_price_quote_context_and_generation_fencing() {
    let generation = 11;
    let mut cache = TickCache::new(2);
    let baseline_snapshot = quote_snapshot("99", None);
    let refreshed_snapshot = quote_snapshot("99", Some("101"));

    cache
        .insert(
            tick(
                "US.AAPL",
                "100",
                "10",
                4_000,
                generation,
                Some(baseline_snapshot),
            ),
            generation,
        )
        .expect("baseline snapshot");
    cache
        .insert(
            tick(
                "US.AAPL",
                "100",
                "10",
                4_001,
                generation,
                Some(refreshed_snapshot.clone()),
            ),
            generation,
        )
        .expect("same-price context refresh");

    let cached = match cache.lookup_for_generation("US.AAPL", 4_001, 0, generation) {
        CacheLookup::Fresh(value) => value,
        other => panic!("expected fresh rich quote, got {other:?}"),
    };
    assert_eq!(cached.price, price_dec("100"));
    assert_eq!(cached.snapshot, Some(refreshed_snapshot));
    assert_eq!(
        cache.lookup_for_generation("US.AAPL", 4_001, 0, generation + 1),
        CacheLookup::Missing
    );
}

#[test]
fn test_cache_deduplicates_promotes_and_inherits() {
    // Parity: internal/marketdata/cache_test.go:12 TestCacheDeduplicatesPromotesAndInherits
    let mut cache = TickCache::new(2);
    let generation = 1;
    let snapshot = quote_snapshot("99", Some("100.5"));
    let first = tick(
        "US.AAPL",
        "100",
        "1000",
        1_000,
        generation,
        Some(snapshot.clone()),
    );

    cache
        .insert(first.clone(), generation)
        .expect("insert first");
    assert_eq!(cache.instrument_count(), 1);

    // Same price context update
    let promoted_snapshot = quote_snapshot("99", Some("101.0"));
    let promoted = tick(
        "US.AAPL",
        "100",
        "1000",
        1_001,
        generation,
        Some(promoted_snapshot.clone()),
    );
    cache.insert(promoted, generation).expect("insert promoted");
    assert_eq!(cache.instrument_count(), 1);

    let cached = match cache.lookup("US.AAPL", 1_001, 0) {
        CacheLookup::Fresh(t) => t,
        other => panic!("expected fresh tick, got {other:?}"),
    };
    assert_eq!(cached.snapshot, Some(promoted_snapshot));
}

#[test]
fn test_cache_freshness_retention_and_maximum() {
    // Parity: internal/marketdata/cache_test.go:76 TestCacheFreshnessRetentionAndMaximum
    let mut cache = TickCache::new(10);
    let generation = 2;
    let t = tick("US.AAPL", "150", "50", 10_000, generation, None);
    cache.insert(t, generation).expect("insert");

    // Fresh within max_age
    assert!(matches!(
        cache.lookup("US.AAPL", 10_500, 1_000),
        CacheLookup::Fresh(_)
    ));
    // Stale past max_age
    assert!(matches!(
        cache.lookup("US.AAPL", 12_000, 1_000),
        CacheLookup::Stale(_)
    ));
}

#[test]
fn test_tick_from_trade_inherits_latest_quote_fields_through_cache() {
    // Parity: internal/integration/futu/marketdata_runtime_test.go:364 TestTickFromTradeInheritsLatestQuoteFieldsThroughCache
    let mut cache = TickCache::new(5);
    let generation = 1;

    let initial_snapshot = quote_snapshot("697.0", Some("705.0"));
    let quote_tick = tick(
        "HK.00700",
        "700.1",
        "43210",
        1_000,
        generation,
        Some(initial_snapshot.clone()),
    );
    cache
        .insert(quote_tick, generation)
        .expect("insert quote tick");

    // Incoming trade tick inherits snapshot
    let trade_tick = tick(
        "HK.00700",
        "702.3",
        "0",
        1_001,
        generation,
        Some(initial_snapshot.clone()),
    );
    cache
        .insert(trade_tick, generation)
        .expect("insert trade tick");

    let cached = match cache.lookup("HK.00700", 1_001, 0) {
        CacheLookup::Fresh(t) => t,
        other => panic!("expected fresh tick, got {other:?}"),
    };
    assert_eq!(cached.price, Decimal::from_str_exact("702.3").unwrap());
    assert_eq!(cached.snapshot, Some(initial_snapshot));
}
