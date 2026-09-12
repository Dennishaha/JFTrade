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

fn tick_at(
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

#[test]
fn test_cache_deduplicates_and_retains_latest_tick() {
    let mut cache = TickCache::new(5);
    let generation = 1;

    let tick1 = tick_at("US.AAPL", "150.0", "100", 1_000, generation, None);
    assert_eq!(cache.insert(tick1.clone(), generation), Ok(()));
    assert_eq!(cache.instrument_count(), 1);

    // Identical tick at the exact same timestamp is accepted (monotonic non-decreasing)
    assert_eq!(cache.insert(tick1, generation), Ok(()));
    assert_eq!(cache.instrument_count(), 1);

    // Tick moving backwards in timestamp must be rejected
    let stale_tick = tick_at("US.AAPL", "149.9", "50", 999, generation, None);
    assert_eq!(
        cache.insert(stale_tick, generation),
        Err(MarketDataError::InvalidSubscription(
            "tick timestamp moved backwards".to_string()
        ))
    );

    // New tick with new price and newer timestamp
    let tick2 = tick_at("US.AAPL", "150.5", "200", 2_000, generation, None);
    assert_eq!(cache.insert(tick2.clone(), generation), Ok(()));
    assert_eq!(cache.instrument_count(), 1);

    match cache.lookup("US.AAPL", 2_000, 0) {
        CacheLookup::Fresh(t) => {
            assert_eq!(t.price, price_dec("150.5"));
            assert_eq!(t.volume, decimal("200"));
        }
        other => panic!("expected fresh tick, got {other:?}"),
    }
}

#[test]
fn test_cache_retains_extended_quote_when_price_is_unchanged() {
    // Parity with Go TestCacheRetainsNewExtendedQuoteWhenPriceIsUnchanged
    let mut cache = TickCache::new(5);
    let generation = 2;
    let base_time = 1_720_555_200_000i64; // arbitrary ms

    let first_snapshot = TradeQuoteSnapshot {
        symbol: Some("AAPL".to_string()),
        previous_close: Some(price_dec("99")),
        ..Default::default()
    };
    let tick1 = tick_at(
        "US.AAPL",
        "100.0",
        "100",
        base_time,
        generation,
        Some(first_snapshot),
    );
    assert_eq!(cache.insert(tick1, generation), Ok(()));

    // Refreshed tick with same price but new AfterMarket extended quote at a later timestamp
    let post_price = price_dec("100.25");
    let refreshed_snapshot = TradeQuoteSnapshot {
        symbol: Some("AAPL".to_string()),
        previous_close: Some(price_dec("99")),
        after_market: Some(ExtendedQuoteSnapshot {
            price: Some(post_price),
            volume: Some(decimal("50")),
            ..Default::default()
        }),
        ..Default::default()
    };
    let tick2 = tick_at(
        "US.AAPL",
        "100.0",
        "100",
        base_time + 500,
        generation,
        Some(refreshed_snapshot.clone()),
    );
    assert_eq!(cache.insert(tick2, generation), Ok(()));

    match cache.lookup("US.AAPL", base_time + 500, 0) {
        CacheLookup::Fresh(t) => {
            assert_eq!(t.price, price_dec("100.0"));
            let snap = t.snapshot.expect("snapshot exists");
            let after = snap.after_market.expect("after market exists");
            assert_eq!(after.price, Some(post_price));
        }
        other => panic!("expected fresh tick with extended quote, got {other:?}"),
    }
}

#[test]
fn test_cache_generation_fencing_prevents_stale_provider_leak() {
    let mut cache = TickCache::new(5);
    let gen1 = 10;
    let gen2 = 11;

    let tick1 = tick_at("US.BABA", "85.0", "100", 1_000, gen1, None);
    assert_eq!(cache.insert(tick1, gen1), Ok(()));

    // Querying with wrong generation should return Missing
    assert_eq!(
        cache.lookup_for_generation("US.BABA", 1_000, 0, gen2),
        CacheLookup::Missing
    );
    assert!(matches!(
        cache.lookup_for_generation("US.BABA", 1_000, 0, gen1),
        CacheLookup::Fresh(_)
    ));

    // Inserting with mismatched generation fails with ProviderChanged
    let tick2 = tick_at("US.BABA", "85.5", "100", 1_500, gen1, None);
    assert_eq!(
        cache.insert(tick2, gen2),
        Err(MarketDataError::ProviderChanged)
    );
}

#[test]
fn test_cache_promotes_regular_close_only_for_after_session() {
    let mut cache = TickCache::new(5);
    let generation = 3;
    let regular = TradeQuoteSnapshot {
        symbol: Some("AAPL".to_owned()),
        last_price: Some(price_dec("111.14")),
        previous_close: Some(price_dec("108.98")),
        last_close: Some(price_dec("108.98")),
        trading_date: Some("2026-07-09".to_owned()),
        session: Some("regular".to_owned()),
        ..Default::default()
    };
    cache
        .insert(
            tick_at("US.AAPL", "111.14", "100", 1_000, generation, Some(regular)),
            generation,
        )
        .expect("regular close");

    let after = TradeQuoteSnapshot {
        symbol: Some("AAPL".to_owned()),
        last_close: Some(price_dec("108.98")),
        trading_date: Some("2026-07-09".to_owned()),
        session: Some("after".to_owned()),
        ..Default::default()
    };
    cache
        .insert(
            tick_at("US.AAPL", "111.81", "101", 2_000, generation, Some(after)),
            generation,
        )
        .expect("after-hours quote");
    let after_tick = match cache.lookup("US.AAPL", 2_000, 0) {
        CacheLookup::Fresh(tick) => tick,
        other => panic!("expected fresh after-hours tick, got {other:?}"),
    };
    let after_snapshot = after_tick.snapshot.expect("after snapshot");
    assert_eq!(after_snapshot.previous_close, Some(price_dec("111.14")));
    assert_eq!(after_snapshot.last_close, Some(price_dec("108.98")));

    let mut pre_cache = TickCache::new(5);
    pre_cache
        .insert(
            tick_at(
                "US.AAPL",
                "111.14",
                "100",
                1_000,
                generation,
                Some(TradeQuoteSnapshot {
                    symbol: Some("AAPL".to_owned()),
                    last_price: Some(price_dec("111.14")),
                    previous_close: Some(price_dec("108.98")),
                    last_close: Some(price_dec("108.98")),
                    trading_date: Some("2026-07-09".to_owned()),
                    session: Some("regular".to_owned()),
                    ..Default::default()
                }),
            ),
            generation,
        )
        .expect("regular close for pre-market");
    let pre = TradeQuoteSnapshot {
        symbol: Some("AAPL".to_owned()),
        trading_date: Some("2026-07-09".to_owned()),
        session: Some("pre".to_owned()),
        ..Default::default()
    };
    pre_cache
        .insert(
            tick_at("US.AAPL", "111.80", "102", 3_000, generation, Some(pre)),
            generation,
        )
        .expect("pre-market quote");
    let pre_tick = match pre_cache.lookup("US.AAPL", 3_000, 0) {
        CacheLookup::Fresh(tick) => tick,
        other => panic!("expected fresh pre-market tick, got {other:?}"),
    };
    let pre_snapshot = pre_tick.snapshot.expect("pre snapshot");
    assert_eq!(pre_snapshot.previous_close, Some(price_dec("108.98")));
}

#[test]
fn test_cache_does_not_inherit_extended_sessions_across_trading_days() {
    // Parity: internal/marketdata/cache_test.go:103 TestCacheDoesNotInheritExtendedSessionsAcrossTradingDays
    let mut cache = TickCache::new(5);
    let generation = 1;

    let day1 = TradeQuoteSnapshot {
        symbol: Some("AAPL".to_owned()),
        last_price: Some(price_dec("100.0")),
        previous_close: Some(price_dec("99.0")),
        trading_date: Some("2026-06-18".to_owned()),
        session: Some("pre".to_owned()),
        pre_market: Some(ExtendedQuoteSnapshot {
            price: Some(price_dec("100.5")),
            ..Default::default()
        }),
        ..Default::default()
    };
    cache
        .insert(
            tick_at("US.AAPL", "100.0", "1000", 1_000, generation, Some(day1)),
            generation,
        )
        .expect("day1 insert");

    // Incoming tick on holiday / new trading day with different trading_date
    let day2 = TradeQuoteSnapshot {
        symbol: Some("AAPL".to_owned()),
        last_price: Some(price_dec("101.0")),
        trading_date: Some("2026-06-19".to_owned()),
        session: Some("unknown".to_owned()),
        ..Default::default()
    };
    cache
        .insert(
            tick_at("US.AAPL", "101.0", "0", 2_000, generation, Some(day2)),
            generation,
        )
        .expect("day2 insert");

    let cached = match cache.lookup("US.AAPL", 2_000, 0) {
        CacheLookup::Fresh(t) => t,
        other => panic!("expected fresh tick, got {other:?}"),
    };
    let snapshot = cached.snapshot.expect("snapshot");
    assert_eq!(snapshot.trading_date.as_deref(), Some("2026-06-19"));
    assert_eq!(snapshot.session.as_deref(), Some("unknown"));
    assert!(snapshot.pre_market.is_none());
    assert!(snapshot.after_market.is_none());
}

#[test]
fn test_cache_promotes_us_regular_close_when_after_hours_trade_arrives() {
    // Parity: internal/marketdata/cache_test.go:138 TestCachePromotesUSRegularCloseWhenAfterHoursTradeArrives
    let mut cache = TickCache::new(5);
    let generation = 1;

    let regular_close = TradeQuoteSnapshot {
        symbol: Some("BABA".to_owned()),
        last_price: Some(price_dec("111.14")),
        previous_close: Some(price_dec("108.98")),
        last_close: Some(price_dec("108.98")),
        trading_date: Some("2026-07-09".to_owned()),
        session: Some("regular".to_owned()),
        ..Default::default()
    };
    cache
        .insert(
            tick_at(
                "US.BABA",
                "111.14",
                "14106666",
                1_000,
                generation,
                Some(regular_close),
            ),
            generation,
        )
        .expect("regular close insert");

    let after_hours = TradeQuoteSnapshot {
        symbol: Some("BABA".to_owned()),
        last_price: Some(price_dec("111.81")),
        trading_date: Some("2026-07-09".to_owned()),
        session: Some("after".to_owned()),
        ..Default::default()
    };
    cache
        .insert(
            tick_at(
                "US.BABA",
                "111.81",
                "100",
                2_000,
                generation,
                Some(after_hours),
            ),
            generation,
        )
        .expect("after hours insert");

    let cached = match cache.lookup("US.BABA", 2_000, 0) {
        CacheLookup::Fresh(t) => t,
        other => panic!("expected fresh tick, got {other:?}"),
    };
    let snapshot = cached.snapshot.expect("snapshot");
    assert_eq!(snapshot.previous_close, Some(price_dec("111.14")));
    assert_eq!(snapshot.last_close, Some(price_dec("108.98")));
    assert_eq!(cached.price, price_dec("111.81"));
}
