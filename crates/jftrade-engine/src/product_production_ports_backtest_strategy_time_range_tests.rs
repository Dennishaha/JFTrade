//! Parity regression tests for the backtest start time range and chart type.

    use super::super::product_production_ports_backtest_parse::{
        with_normalized_chart_type, with_normalized_time_range,
    };
    use super::*;
    use serde_json::json;

    fn start_payload(start_date: &str, end_date: &str, start_time: &str, end_time: &str) -> Value {
        let mut payload = json!({
            "definitionId": "strategy-1",
            "market": "US",
            "code": "AAPL",
            "interval": "1m",
            "rehabType": "forward",
        });
        let object = payload.as_object_mut().expect("payload object");
        for (key, value) in [
            ("startDate", start_date),
            ("endDate", end_date),
            ("startTime", start_time),
            ("endTime", end_time),
        ] {
            if !value.is_empty() {
                object.insert(key.to_owned(), Value::String(value.to_owned()));
            }
        }
        payload
    }

    #[test]
    fn backtest_start_normalizes_chart_type() {
        // Parity: go:452dea11:internal/backtest/service_test.go:196 TestPrepareResolvedBacktestNormalizesChartType
        let with_chart_type = |value: Option<&str>| {
            let mut payload = start_payload("2026-03-08", "2026-03-08", "", "");
            if let Some(value) = value {
                payload["chartType"] = json!(value);
            }
            with_normalized_chart_type(&payload).expect("normalized chart type")
        };
        assert_eq!(with_chart_type(None)["chartType"], "standard");
        assert_eq!(with_chart_type(Some("renko"))["chartType"], "standard");
        assert_eq!(with_chart_type(Some("HeikinAshi"))["chartType"], "heikinashi");
    }

    #[test]
    fn chart_type_normalization_matches_the_pkg_chart_table() {
        // Parity: go:452dea11:pkg/chart/chart_type_test.go:5 TestNormalizeChartType
        let normalize = |value: Option<&str>| {
            let mut payload = start_payload("2026-03-08", "2026-03-08", "", "");
            payload["chartType"] = match value {
                Some(value) => json!(value),
                None => Value::Null,
            };
            with_normalized_chart_type(&payload).expect("normalized chart type")["chartType"].clone()
        };
        for (value, want) in [
            ("", "standard"),
            ("standard", "standard"),
            ("  HEIKINASHI ", "heikinashi"),
            ("renko", "standard"),
        ] {
            assert_eq!(
                normalize(Some(value)),
                json!(want),
                "NormalizeChartType({value:?})"
            );
        }
        assert_eq!(normalize(None), json!("standard"), "missing chartType");

        let mut payload = start_payload("2026-03-08", "2026-03-08", "", "");
        payload["chartType"] = json!(3);
        assert!(
            matches!(
                with_normalized_chart_type(&payload),
                Err(BacktestsWritePortError::BadRequest(message))
                    if message == "chartType must be a string"
            ),
            "non-string chartType must keep the parse boundary"
        );
    }

    #[test]
    fn backtest_start_resolves_market_dates_with_dst() {
        // Parity: go:452dea11:internal/backtest/time_test.go:8 TestResolveBacktestTimeRangeUsesMarketDateAndDST
        let payload = start_payload("2026-03-08", "2026-03-08", "", "");
        let parsed = parse_start_request(&payload).expect("us market date range");
        assert_eq!(parsed.start_time_ms, 1_772_946_000_000);
        assert_eq!(parsed.end_time_ms, 1_773_028_799_999);
        assert_eq!(
            parsed.end_time_ms - parsed.start_time_ms + 1,
            23 * 60 * 60 * 1_000
        );

        let normalized =
            with_normalized_time_range(&payload, "US.AAPL").expect("normalized us range");
        assert_eq!(normalized["startTime"], "2026-03-08T05:00:00Z");
        assert_eq!(normalized["endTime"], "2026-03-09T03:59:59.999999999Z");
        assert_eq!(normalized["startDate"], "2026-03-08");
        assert_eq!(normalized["endDate"], "2026-03-08");
        assert_eq!(normalized["marketTimezone"], "America/New_York");
    }

    #[test]
    fn backtest_start_resolves_hong_kong_calendar_day() {
        // Parity: go:452dea11:internal/backtest/time_test.go:33 TestResolveBacktestTimeRangeUsesHongKongCalendarDay
        let mut payload = start_payload("2026-01-01", "2026-01-01", "", "");
        payload["market"] = json!("HK");
        payload["code"] = json!("00700");
        let parsed = parse_start_request(&payload).expect("hong kong market date range");
        assert_eq!(parsed.symbol, "HK.00700");
        assert_eq!(parsed.start_time_ms, 1_767_196_800_000);
        assert_eq!(parsed.end_time_ms, 1_767_283_199_999);

        let normalized =
            with_normalized_time_range(&payload, "HK.00700").expect("normalized hk range");
        assert_eq!(normalized["startTime"], "2025-12-31T16:00:00Z");
        assert_eq!(normalized["endTime"], "2026-01-01T15:59:59.999999999Z");
        assert_eq!(normalized["marketTimezone"], "Asia/Hong_Kong");
    }

    #[test]
    fn backtest_start_normalizes_legacy_offset_timestamps() {
        // Parity: go:452dea11:internal/backtest/time_test.go:55 TestResolveBacktestTimeRangeNormalizesLegacyTimestamps
        let parsed = parse_start_request(&start_payload(
            "",
            "",
            "2026-06-20T09:30:00+08:00",
            "2026-06-20T10:30:00+08:00",
        ))
        .expect("legacy offset timestamps");
        assert_eq!(parsed.start_time_ms, 1_781_919_000_000);
        assert_eq!(parsed.end_time_ms, 1_781_922_600_000);

        let normalized = with_normalized_time_range(
            &start_payload(
                "",
                "",
                "2026-06-20T09:30:00+08:00",
                "2026-06-20T10:30:00+08:00",
            ),
            "US.AAPL",
        )
        .expect("normalized legacy range");
        assert_eq!(normalized["startTime"], "2026-06-20T01:30:00Z");
        assert_eq!(normalized["endTime"], "2026-06-20T02:30:00Z");
        assert_eq!(normalized["marketTimezone"], "America/New_York");
        assert!(normalized.get("startDate").is_none());
        assert!(normalized.get("endDate").is_none());
    }
