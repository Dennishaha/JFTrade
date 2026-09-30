
    use super::*;

    fn decimal(value: &str) -> Decimal {
        value.parse().expect("decimal fixture")
    }

    fn decimal_text(value: &str) -> DecimalText {
        value.parse().expect("decimal text fixture")
    }

    fn tick(snapshot: TradeQuoteSnapshot) -> Tick {
        Tick {
            instrument_id: "US.AAPL".to_owned(),
            price: decimal("114.97"),
            volume: decimal_text("1179135"),
            volume_delta: None,
            snapshot: Some(snapshot),
            observed_at_ms: 1_750_000_000_000,
            provider_generation: 1,
        }
    }

    // Parity: go:452dea11:internal/marketdata/quote_availability_test.go:9
    // TestSnapshotSerializationPreservesAuthoritativeMissingQuoteFields.
    #[test]
    fn authoritative_snapshot_keeps_missing_quote_fields_null() {
        let value = project_cached_snapshot(
            &tick(TradeQuoteSnapshot {
                authoritative: true,
                ..Default::default()
            }),
            "US",
            "2026-09-25T02:00:00Z",
        );

        for field in ["bid", "ask", "volume", "turnover"] {
            assert!(value[field].is_null(), "authoritative {field} = {value}");
        }
    }

    // Parity: go:452dea11:internal/marketdata/quote_availability_test.go:35
    // TestSnapshotSerializationKeepsLegacyZeroValuesAvailable.
    #[test]
    fn legacy_tick_without_snapshot_keeps_zero_quote_fields_available() {
        let value = project_cached_snapshot(
            &Tick {
                instrument_id: "US.AAPL".to_owned(),
                price: decimal("100"),
                volume: decimal_text("0"),
                volume_delta: None,
                snapshot: None,
                observed_at_ms: 1_750_000_000_000,
                provider_generation: 1,
            },
            "US",
            "2026-09-25T02:00:00Z",
        );

        for field in ["bid", "ask", "volume", "turnover"] {
            assert_eq!(value[field], "0", "legacy {field} = {value}");
        }
    }

    // Parity: go:452dea11:internal/marketdata/cache_test.go:261 TestSerializationPreservesNullExtendedAndStringPrices
    #[test]
    fn cached_projection_uses_active_after_quote_and_separates_closes() {
        let snapshot = TradeQuoteSnapshot {
            symbol: Some("US.AAPL".to_owned()),
            last_price: Some(decimal("114.97")),
            previous_close: Some(decimal("114.97")),
            last_close: Some(decimal("112.50")),
            high_price: Some(decimal("115.70")),
            low_price: Some(decimal("114.13")),
            volume: Some(decimal_text("1179135")),
            turnover: Some(decimal_text("135465382.564")),
            session: Some("after".to_owned()),
            after_market: Some(ExtendedQuoteSnapshot {
                price: Some(decimal("118.40")),
                high_price: Some(decimal("121.20")),
                low_price: Some(decimal("115.50")),
                volume: Some(decimal_text("0")),
                turnover: Some(decimal_text("67722995.69")),
                quote_time: Some("2026-07-18T20:15:00Z".to_owned()),
                trading_date: Some("2026-07-18".to_owned()),
                exchange_timezone: Some("America/New_York".to_owned()),
                session_start_at: Some("2026-07-18T16:00:00Z".to_owned()),
                session_end_at: Some("2026-07-19T00:00:00Z".to_owned()),
                ..Default::default()
            }),
            ..Default::default()
        };
        let value = project_cached_snapshot(
            &tick(snapshot),
            "US",
            "2026-07-18T20:15:00Z",
        );

        assert_eq!(value["price"], "118.40");
        assert_eq!(value["highPrice"], "121.20");
        assert_eq!(value["lowPrice"], "115.50");
        assert_eq!(value["volume"], "0");
        assert_eq!(value["turnover"], "67722995.69");
        assert_eq!(value["previousClosePrice"], "114.97");
        assert_eq!(value["lastClosePrice"], "112.50");
        assert_eq!(value["session"], "after");
        assert_eq!(value["extendedHours"], true);
        assert!(value["openPrice"].is_null());
        assert!(value["extended"]["preMarket"].is_null());
        assert!(value["extended"]["overnight"].is_null());
        assert_eq!(value["extended"]["afterMarket"]["quoteTime"], "2026-07-18T20:15:00Z");
        assert_eq!(
            value["extended"]["afterMarket"]["sessionStartAt"],
            "2026-07-18T16:00:00Z"
        );
    }

    /// Parity: go:452dea11:pkg/futu/quote_snapshot_test.go:217
    /// TestPreviousClosePriceConditionBySessionType
    ///
    /// Go's `ShouldUseRegularCloseAsPreviousClose` is `IsUSSymbol(symbol) &&
    /// session != SessionRegular && regularClose > 0`. Every non-regular US
    /// session (pre/after/overnight/closed/**unknown**) reports the latest
    /// regular-session close; a non-positive close never rewrites the value.
    #[test]
    fn previous_close_condition_switches_on_session_type() {
        let regular_close = decimal("9.22");
        for session in ["pre", "after", "overnight", "closed", "unknown", "UNKNOWN"] {
            assert!(
                uses_regular_close_as_previous_close("US", session, regular_close),
                "US {session} must prefer the latest regular-session close"
            );
        }
        assert!(
            !uses_regular_close_as_previous_close("US", "regular", regular_close),
            "the US regular session must keep the provider LastClosePrice"
        );
        assert!(
            !uses_regular_close_as_previous_close("US", "closed", Decimal::ZERO),
            "a zero regular close must never replace the provider LastClosePrice"
        );
        assert_ne!(
            decimal("9.22"),
            decimal("9.09"),
            "test data sanity"
        );
    }

    /// Parity: go:452dea11:pkg/futu/quote_snapshot_test.go:258
    /// TestPreviousClosePriceConditionDoesNotRewriteNonUSUnknownSession
    #[test]
    fn previous_close_condition_does_not_rewrite_non_us_unknown_sessions() {
        let regular_close = decimal("321.40");
        for instrument_market in ["HK", "SH", "SZ", "CN"] {
            assert!(
                !uses_regular_close_as_previous_close(
                    instrument_market,
                    "unknown",
                    regular_close
                ),
                "{instrument_market} unknown session must keep LastClosePrice"
            );
            assert!(
                !uses_regular_close_as_previous_close(
                    instrument_market,
                    "closed",
                    regular_close
                ),
                "{instrument_market} closed session must keep LastClosePrice"
            );
        }
    }

    // Parity: go:452dea11:internal/app/apiserver/marketdataapp/watchlist_source_test.go:175 TestWatchlistQuoteUsesPriorCloseForClosedRegularYahooQuote
    /// Parity: go:452dea11:pkg/futu/quote_snapshot_test.go:49
    /// TestQuoteSnapshotPreviousClosePriceInClosedSession and
    /// go:452dea11:pkg/futu/quote_snapshot_test.go:164
    /// TestQuoteSnapshotPreviousClosePriceZeroCurPrice
    ///
    /// Closed US session: `previousClosePrice` is the latest regular-session
    /// close (Friday's CurPrice) while `lastClosePrice` stays the raw provider
    /// value. With a zero current price the condition fails and the provider
    /// LastClosePrice is preserved instead of emitting an empty close.
    #[test]
    fn closed_us_session_reports_regular_close_as_previous_close() {
        let closed = TradeQuoteSnapshot {
            last_price: Some(decimal("9.22")),
            previous_close: Some(decimal("9.09")),
            last_close: Some(decimal("9.09")),
            session: Some("closed".to_owned()),
            ..Default::default()
        };
        let value = project_cached_snapshot(&tick(closed), "US", "2026-05-31T16:00:00Z");
        assert_eq!(
            value["previousClosePrice"], "9.22",
            "a closed US session must report the latest regular-session close"
        );
        assert_eq!(value["lastClosePrice"], "9.09");
        assert_eq!(value["session"], "closed");
    }

    #[test]
    // Parity: go:452dea11:pkg/futu/quote_snapshot_test.go:164 TestQuoteSnapshotPreviousClosePriceZeroCurPrice
    fn zero_current_price_falls_back_to_provider_last_close() {
        let zero_price = TradeQuoteSnapshot {
            last_price: Some(Decimal::ZERO),
            previous_close: Some(decimal("9.09")),
            last_close: Some(decimal("9.09")),
            session: Some("closed".to_owned()),
            ..Default::default()
        };
        let mut zero_tick = tick(zero_price);
        zero_tick.price = Decimal::ZERO;
        let value = project_cached_snapshot(&zero_tick, "US", "2026-05-31T16:00:00Z");
        assert_eq!(
            value["previousClosePrice"], "9.09",
            "a zero regular close must fall back to the provider LastClosePrice"
        );
    }

    /// Parity: go:452dea11:pkg/futu/quote_snapshot_test.go:93
    /// TestQuoteSnapshotHolidayRemainsClosedWithStaleExtendedBlocks
    ///
    /// On a market holiday OpenD keeps returning the previous session's
    /// pre/after/overnight blocks. Go keeps `session=closed`, reports the
    /// regular close as both `price` and `previousClosePrice`, and still
    /// exposes the three blocks (without independent quote timestamps) so the
    /// UI can decide whether to display them.
    #[test]
    fn holiday_snapshot_stays_closed_with_stale_extended_blocks() {
        let extended = |price: &str| ExtendedQuoteSnapshot {
            price: Some(decimal(price)),
            quote_time: None,
            ..Default::default()
        };
        let snapshot = TradeQuoteSnapshot {
            last_price: Some(decimal("195.50")),
            previous_close: Some(decimal("193.20")),
            last_close: Some(decimal("193.20")),
            session: Some("closed".to_owned()),
            pre_market: Some(extended("196.10")),
            after_market: Some(extended("195.30")),
            overnight: Some(extended("194.90")),
            ..Default::default()
        };
        let mut holiday_tick = tick(snapshot);
        holiday_tick.price = decimal("195.50");
        let value = project_cached_snapshot(&holiday_tick, "US", "2026-06-19T16:00:00Z");
        assert_eq!(value["session"], "closed");
        assert_eq!(value["extendedHours"], false);
        assert_eq!(value["price"], "195.50");
        assert_eq!(value["previousClosePrice"], "195.50");
        for block in ["preMarket", "afterMarket", "overnight"] {
            assert!(
                !value["extended"][block].is_null(),
                "stale {block} block must stay available for display decisions"
            );
            assert_eq!(
                value["extended"][block]["quoteTime"], "",
                "{block} must not carry an independent OpenD quote timestamp"
            );
        }
    }

    /// Parity: go:452dea11:pkg/futu/quote_snapshot_test.go:133
    /// TestQuoteSnapshotPreviousClosePriceInAfterHours
    ///
    /// After hours the primary price is the after-market price, but
    /// `previousClosePrice` must be today's regular-session close while
    /// `lastClosePrice` keeps the raw provider value. The after-market block
    /// must not reuse the regular quote timestamp.
    #[test]
    fn after_hours_projection_uses_todays_regular_close_and_keeps_block_time_empty() {
        let snapshot = TradeQuoteSnapshot {
            last_price: Some(decimal("195.50")),
            previous_close: Some(decimal("193.20")),
            last_close: Some(decimal("193.20")),
            session: Some("after".to_owned()),
            after_market: Some(ExtendedQuoteSnapshot {
                price: Some(decimal("195.30")),
                change_rate: Some(decimal_text("-0.10")),
                quote_time: None,
                ..Default::default()
            }),
            ..Default::default()
        };
        let mut after_tick = tick(snapshot);
        after_tick.price = decimal("195.50");
        let value = project_cached_snapshot(&after_tick, "US", "2026-06-02T21:00:00Z");
        assert_eq!(value["price"], "195.30");
        assert_eq!(value["previousClosePrice"], "195.50");
        assert_eq!(value["lastClosePrice"], "193.20");
        assert_eq!(value["session"], "after");
        assert_eq!(value["extendedHours"], true);
        assert_eq!(value["extended"]["afterMarket"]["price"], "195.30");
        assert_eq!(
            value["extended"]["afterMarket"]["quoteTime"], "",
            "the after block must not reuse the BasicQot regular quote time"
        );
    }

    #[test]
    fn cached_projection_does_not_promote_stale_overnight_during_regular_hours() {
        let snapshot = TradeQuoteSnapshot {
            last_price: Some(decimal("114.97")),
            previous_close: Some(decimal("112.50")),
            session: Some("regular".to_owned()),
            overnight: Some(ExtendedQuoteSnapshot {
                price: Some(decimal("119.10")),
                ..Default::default()
            }),
            ..Default::default()
        };
        let value = project_cached_snapshot(
            &tick(snapshot),
            "US",
            "2026-07-18T14:00:00Z",
        );

        assert_eq!(value["price"], "114.97");
        assert_eq!(value["extendedHours"], false);
        assert_eq!(value["extended"]["overnight"]["price"], "119.10");
    }

    #[test]
    fn fallback_projection_normalizes_numbers_and_projects_active_extended_fields() {
        let value = project_fallback_snapshot(
            &json!({
                "lastPrice": 114.97,
                "previousClose": 114.97,
                "lastClosePrice": 112.5,
                "session": "after",
                "highPrice": 115.7,
                "lowPrice": 114.13,
                "volume": 1179135,
                "afterMarket": {
                    "price": 118.4,
                    "highPrice": 121.2,
                    "lowPrice": 115.5,
                    "volume": 0,
                    "turnover": 67722995.69,
                    "quoteTime": "2026-07-18T20:15:00Z",
                    "tradingDate": "2026-07-18",
                    "exchangeTimezone": "America/New_York"
                }
            }),
            "US",
            "2026-07-18T20:15:00Z",
        )
        .expect("fallback projection");

        assert_eq!(value["price"], "118.4");
        assert_eq!(value["previousClosePrice"], "114.97");
        assert_eq!(value["lastClosePrice"], "112.5");
        assert_eq!(value["volume"], "0");
        assert_eq!(value["extended"]["afterMarket"]["volume"], "0");
        assert_eq!(value["extended"]["afterMarket"]["tradingDate"], "2026-07-18");
    }

    /// Parity: go:452dea11:internal/productfeatures/market_data_reads_test.go:371
    /// TestWorkspaceSnapshotRestoresRegularCloseComparisonSemantics
    ///
    /// Outside the regular session a US listing compares the latest regular
    /// close with the prior trading-day close: `previousClosePrice` becomes the
    /// latest regular close (`lastPrice`) while `lastClosePrice` keeps the
    /// provider value. A non-US unknown session keeps both raw fields.
    #[test]
    fn workspace_snapshot_restores_regular_close_comparison_semantics() {
        let cases = [
            ("US", "regular", "195.50", "193.20", "193.20", "193.20"),
            ("US", "after", "195.50", "193.20", "195.50", "193.20"),
            ("SZ", "unknown", "74.10", "72.76", "72.76", "72.76"),
        ];
        for (market, session, last_price, previous_close, want_previous, want_last) in cases {
            let snapshot = TradeQuoteSnapshot {
                last_price: Some(decimal(last_price)),
                previous_close: Some(decimal(previous_close)),
                session: Some(session.to_owned()),
                ..Default::default()
            };
            let value =
                project_cached_snapshot(&tick(snapshot), market, "2026-07-18T14:00:00Z");
            assert_eq!(
                value["previousClosePrice"], want_previous,
                "{market}/{session} previousClosePrice"
            );
            assert_eq!(
                value["lastClosePrice"], want_last,
                "{market}/{session} lastClosePrice"
            );
        }
    }

    /// Parity: go:452dea11:internal/productfeatures/market_data_reads_test.go:418
    /// TestWorkspaceSnapshotUsesActiveExtendedSessionFields
    ///
    /// The active pre/after/overnight block replaces the primary quote fields,
    /// `previousClosePrice` reports the latest regular close and
    /// `lastClosePrice` keeps the prior close while `extendedHours` turns on.
    #[test]
    fn workspace_snapshot_uses_active_extended_session_fields() {
        let cases = [
            ("pre", "116.25"),
            ("after", "118.40"),
            ("overnight", "119.10"),
        ];
        for (session, block_price) in cases {
            let block = ExtendedQuoteSnapshot {
                price: Some(decimal(block_price)),
                high_price: Some(decimal("121.20")),
                low_price: Some(decimal("115.50")),
                volume: Some(decimal_text("567216")),
                turnover: Some(decimal_text("67722995.69")),
                ..Default::default()
            };
            let mut snapshot = TradeQuoteSnapshot {
                last_price: Some(decimal("114.97")),
                previous_close: Some(decimal("117.49")),
                high_price: Some(decimal("115.70")),
                low_price: Some(decimal("114.13")),
                volume: Some(decimal_text("1179135")),
                turnover: Some(decimal_text("135465382.564")),
                session: Some(session.to_owned()),
                ..Default::default()
            };
            match session {
                "pre" => snapshot.pre_market = Some(block),
                "after" => snapshot.after_market = Some(block),
                "overnight" => snapshot.overnight = Some(block),
                other => panic!("unexpected session {other}"),
            }
            let value =
                project_cached_snapshot(&tick(snapshot), "US", "2026-07-18T14:00:00Z");
            assert_eq!(value["price"], block_price, "{session} price");
            assert_eq!(value["highPrice"], "121.20", "{session} highPrice");
            assert_eq!(value["lowPrice"], "115.50", "{session} lowPrice");
            assert_eq!(value["volume"], "567216", "{session} volume");
            assert_eq!(
                value["turnover"], "67722995.69",
                "{session} turnover"
            );
            assert_eq!(
                value["previousClosePrice"], "114.97",
                "{session} previousClosePrice is the latest regular close"
            );
            assert_eq!(
                value["lastClosePrice"], "117.49",
                "{session} lastClosePrice keeps the prior close"
            );
            assert_eq!(value["extendedHours"], true, "{session} extendedHours");
        }
    }

    /// Parity: go:452dea11:internal/productfeatures/market_data_reads_test.go:470
    /// TestWorkspaceSnapshotExtendedSessionFallbacksRemainStable
    ///
    /// A regular/closed session ignores a stale extended block; an active
    /// overnight session falls back to the regular fields when the block is
    /// missing or has no positive price, keeps an explicitly zero volume from a
    /// partial block and still reports `extendedHours`.
    #[test]
    fn workspace_snapshot_extended_session_fallbacks_remain_stable() {
        let stale = ExtendedQuoteSnapshot {
            price: Some(decimal("119.10")),
            high_price: Some(decimal("121.20")),
            low_price: Some(decimal("115.50")),
            volume: Some(decimal_text("567216")),
            turnover: Some(decimal_text("67722995.69")),
            ..Default::default()
        };
        let zero_price = ExtendedQuoteSnapshot {
            price: Some(Decimal::ZERO),
            high_price: Some(decimal("121.20")),
            volume: Some(decimal_text("0")),
            ..Default::default()
        };
        let partial = ExtendedQuoteSnapshot {
            price: Some(decimal("119.10")),
            volume: Some(decimal_text("0")),
            ..Default::default()
        };
        let cases = [
            ("regular", Some(stale.clone()), false),
            ("closed", Some(stale.clone()), false),
            ("overnight", None, true),
            ("overnight", Some(zero_price), true),
            ("overnight", Some(partial), true),
        ];
        for (session, block, want_extended) in cases {
            let snapshot = TradeQuoteSnapshot {
                last_price: Some(decimal("114.97")),
                previous_close: Some(decimal("117.49")),
                high_price: Some(decimal("115.70")),
                low_price: Some(decimal("114.13")),
                volume: Some(decimal_text("1179135")),
                turnover: Some(decimal_text("135465382.564")),
                session: Some(session.to_owned()),
                overnight: block,
                ..Default::default()
            };
            let value =
                project_cached_snapshot(&tick(snapshot), "US", "2026-07-18T14:00:00Z");
            let active_overnight = session == "overnight"
                && value["extended"]["overnight"]["price"] == "119.10";
            let want_price = if active_overnight { "119.10" } else { "114.97" };
            let want_volume = if active_overnight { "0" } else { "1179135" };
            assert_eq!(value["price"], want_price, "{session} price");
            assert_eq!(value["volume"], want_volume, "{session} volume");
            assert_eq!(
                value["extendedHours"], want_extended,
                "{session} extendedHours"
            );
            if active_overnight {
                assert_eq!(value["highPrice"], "115.70", "{session} keeps high");
                assert_eq!(value["lowPrice"], "114.13", "{session} keeps low");
                assert_eq!(
                    value["turnover"], "135465382.564",
                    "{session} keeps turnover"
                );
            }
        }
    }

    /// Parity: go:452dea11:internal/productfeatures/market_data_reads_test.go:307
    /// TestWorkspaceMarketDataReadsSurfaceProviderFailuresAndNormalizeFallbacks
    ///
    /// `workspaceSnapshot(nil, …)` returns nil and a snapshot without a usable
    /// price stays nil instead of fabricating a quote; the observation time is
    /// taken from `observedAt`/`quoteAt`/`updateTime` when present and falls
    /// back to the caller's resolved time otherwise.
    #[test]
    fn fallback_projection_preserves_observation_fallbacks() {
        assert!(
            project_fallback_snapshot(&json!({}), "US", "2026-07-18T12:01:00Z").is_none(),
            "an empty entry must stay nil like Go's workspaceSnapshot"
        );
        assert!(
            project_fallback_snapshot(&json!({"lastPrice": 0}), "US", "2026-07-18T12:01:00Z")
                .is_none(),
            "a non-positive price must stay nil"
        );

        let updated = project_fallback_snapshot(
            &json!({"lastPrice": 114.97, "updateTime": "2026-07-18T12:00:00Z"}),
            "US",
            "2026-07-18T12:01:00Z",
        )
        .expect("update-time snapshot");
        assert_eq!(updated["observedAt"], "2026-07-18T12:00:00Z");
        assert_eq!(updated["at"], "2026-07-18T12:00:00Z");

        let stamped = project_fallback_snapshot(
            &json!({"lastPrice": 114.97}),
            "US",
            "2026-07-18T12:01:00Z",
        )
        .expect("fallback-time snapshot");
        assert_eq!(stamped["observedAt"], "2026-07-18T12:01:00Z");
    }
