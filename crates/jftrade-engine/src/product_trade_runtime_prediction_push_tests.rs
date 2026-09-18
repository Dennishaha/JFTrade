use super::*;

/// Parity: go:452dea11:internal/productfeatures/service_routing_and_validation_test.go:346
/// TestQueryUsesFreshPredictionPushBeforePolling
/// go:452dea11:internal/productfeatures/prediction_quote_candle_bridge_test.go:137
/// TestPredictionPushSourceCachesFreshUniqueUpdates
///
/// Go stores the newest `PredictionMarketUpdate` per
/// `brokerID|INSTRUMENT|DATATYPE` and answers a matching depth/history read
/// from that cache while the sample is at most five seconds old, without ever
/// calling the broker. A duplicate sequence keeps the previous sample; a
/// sequence-free update always replaces it; a stale sample falls back to
/// polling; and the listener is attached exactly once per broker id.
#[test]
fn fresh_prediction_push_serves_reads_and_duplicate_sequences_keep_the_first_sample() {
    let cache = PredictionPushCache::default();

    // Go's `ensurePredictionPushSource` guard: only the first registration per
    // broker id attaches the OpenD listener.
    assert!(cache.register("futu"));
    assert!(!cache.register("futu"));
    assert!(!cache.register(" FUTU "));
    assert!(cache.register("other-broker"));

    cache.store(
        "futu",
        "US.EVENT.ONE",
        "ORDER_BOOK",
        "1",
        "2026-07-18T12:00:00Z",
        vec![json!({"price": 0.4})],
    );
    // Duplicate sequence: keep the first sample, exactly like Go's
    // `current.Sequence == update.Sequence` guard.
    cache.store(
        "futu",
        "US.EVENT.ONE",
        "ORDER_BOOK",
        "1",
        "2026-07-18T12:00:01Z",
        vec![json!({"price": 0.9})],
    );
    let result = cache
        .result("FUTU", " us.event.one ", "order_book")
        .expect("a fresh push must serve the read");
    assert_eq!(result["entries"][0]["price"], 0.4);
    assert_eq!(result["metadata"]["source"], "push");
    assert_eq!(result["metadata"]["dataType"], "ORDER_BOOK");
    assert_eq!(result["metadata"]["sequence"], "1");
    assert_eq!(result["asOf"], "2026-07-18T12:00:00Z");

    // A sequence-free update always replaces the retained sample.
    cache.store(
        "futu",
        "US.EVENT.ONE",
        "ORDER_BOOK",
        "",
        "2026-07-18T12:00:02Z",
        vec![json!({"price": 0.5})],
    );
    let replaced = cache
        .result("futu", "US.EVENT.ONE", "ORDER_BOOK")
        .expect("sequence-free push");
    assert_eq!(replaced["entries"][0]["price"], 0.5);

    // A different sequence also replaces the sample.
    cache.store(
        "futu",
        "US.EVENT.ONE",
        "ORDER_BOOK",
        "2",
        "2026-07-18T12:00:03Z",
        vec![json!({"price": 0.6})],
    );
    assert_eq!(
        cache.result("futu", "US.EVENT.ONE", "ORDER_BOOK").expect("new sequence")
            ["metadata"]["sequence"],
        "2"
    );

    // Unknown key and other broker ids never answer from the cache.
    assert!(cache.result("futu", "US.EVENT.TWO", "ORDER_BOOK").is_none());
    assert!(cache.result("other-broker", "US.EVENT.ONE", "ORDER_BOOK").is_none());
    assert!(cache.result("futu", "US.EVENT.ONE", "KLINE").is_none());
}

/// Go's freshness window is exactly five seconds: a sample received inside the
/// window answers the read, and anything strictly older falls back to polling.
#[test]
fn prediction_push_samples_expire_after_the_five_second_window() {
    let cache = PredictionPushCache::default();
    let now = Instant::now();
    cache.store_at(
        "futu",
        "US.EVENT.ONE",
        "TICKER",
        "7",
        "2026-07-18T12:00:00Z",
        vec![json!({"price": 0.31})],
        now,
    );
    assert!(
        cache
            .result_at("futu", "US.EVENT.ONE", "TICKER", now)
            .is_some(),
        "a sample inside the window must serve the read"
    );
    assert!(
        cache
            .result_at(
                "futu",
                "US.EVENT.ONE",
                "TICKER",
                now + PREDICTION_PUSH_TTL - Duration::from_millis(1),
            )
            .is_some(),
        "a sample inside the window must still be fresh"
    );
    // Go compares `now.Sub(update.AsOf) > 5s`, so exactly five seconds is not
    // stale yet and the first instant past it falls back to polling.
    assert!(
        cache
            .result_at("futu", "US.EVENT.ONE", "TICKER", now + PREDICTION_PUSH_TTL)
            .is_some(),
        "the exact five second boundary is still fresh in Go"
    );
    assert!(
        cache
            .result_at(
                "futu",
                "US.EVENT.ONE",
                "TICKER",
                now + PREDICTION_PUSH_TTL + Duration::from_millis(1),
            )
            .is_none(),
        "a sample past the window must fall back to polling"
    );
}

/// The push key folds the broker id to lower case and the instrument/data type
/// to upper case, matching Go's `predictionPushKey`.
#[test]
fn prediction_push_key_folds_broker_instrument_and_data_type() {
    assert_eq!(
        prediction_push_key(" FUTU ", " us.event.one ", " order_book "),
        "futu|US.EVENT.ONE|ORDER_BOOK"
    );
}
