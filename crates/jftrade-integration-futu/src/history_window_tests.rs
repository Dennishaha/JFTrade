use super::*;
use std::collections::VecDeque;

#[derive(Debug)]
struct Pages {
    responses: Mutex<VecDeque<Result<HistoricalKlineResult, HistoricalKlineError>>>,
    requests: Mutex<Vec<HistoricalKlineQuery>>,
}

impl HistoricalKlineReadPort for Pages {
    fn query(
        &self,
        query: &HistoricalKlineQuery,
    ) -> Result<HistoricalKlineResult, HistoricalKlineError> {
        self.requests.lock().unwrap().push(query.clone());
        self.responses.lock().unwrap().pop_front().expect("page")
    }
}

fn query() -> HistoricalKlineQuery {
    HistoricalKlineQuery {
        market: 1,
        symbol: "00700".into(),
        period: "1m".into(),
        adjustment: 1,
        begin_time: "2026-09-09 09:30:00".into(),
        end_time: "2026-09-09 14:37:00".into(),
        max_ack_kl_num: Some(200),
        next_req_key: vec![],
        extended_time: Some(false),
        session: None,
    }
}

fn candle(time: &str) -> HistoricalKline {
    HistoricalKline {
        time: format!("2026-09-09 {time}:00"),
        is_blank: false,
        high_price: Some(10.0),
        open_price: Some(10.0),
        low_price: Some(10.0),
        close_price: Some(10.0),
        volume: Some(100),
        turnover: None,
        change_rate: None,
    }
}

fn page(times: &[&str], key: &[u8]) -> HistoricalKlineResult {
    HistoricalKlineResult {
        security: HistoricalSecurity {
            market: 1,
            code: "00700".into(),
        },
        name: None,
        klines: times.iter().map(|time| candle(time)).collect(),
        next_req_key: key.to_vec(),
    }
}

fn reader(responses: Vec<Result<HistoricalKlineResult, HistoricalKlineError>>) -> Pages {
    Pages {
        responses: Mutex::new(responses.into()),
        requests: Mutex::new(vec![]),
    }
}

#[test]
fn history_window_reads_missing_minutes_before_merging_current_bars() {
    let missing: Vec<String> = (19..60)
        .map(|m| format!("13:{m:02}"))
        .chain((0..37).map(|m| format!("14:{m:02}")))
        .collect();
    let times: Vec<&str> = missing.iter().map(String::as_str).collect();
    let reader = reader(vec![Ok(page(&["13:18"], &[1])), Ok(page(&times, &[]))]);
    let result = reader.query_window(&query()).unwrap();
    let merged = crate::merge_klines_by_time(&result.klines, &[candle("14:37")]);
    assert_eq!(merged.len(), 80);
    for pair in merged.windows(2) {
        let format = time::format_description::parse_borrowed::<2>(
            "[year]-[month]-[day] [hour]:[minute]:[second]",
        )
        .unwrap();
        let parse = |s: &str| time::PrimitiveDateTime::parse(s, &format).unwrap();
        assert_eq!(
            parse(&pair[1].time) - parse(&pair[0].time),
            time::Duration::minutes(1)
        );
    }
    let requests = reader.requests.lock().unwrap();
    assert_eq!(requests.len(), 2);
    let mut expected = query();
    expected.max_ack_kl_num = Some(200);
    assert_eq!(requests[0], expected);
    expected.next_req_key = vec![1];
    assert_eq!(requests[1], expected);
    assert!(result.next_req_key.is_empty());
}

#[test]
fn history_window_rejects_cursor_cycles_without_returning_partial_history() {
    let reader = reader(vec![
        Ok(page(&["13:18"], &[1])),
        Ok(page(&["13:19"], &[2])),
        Ok(page(&["13:20"], &[1])),
    ]);
    assert!(matches!(
        reader.query_window(&query()),
        Err(HistoricalKlineError::InvalidPagination)
    ));
    assert_eq!(reader.requests.lock().unwrap().len(), 3);
}

#[test]
fn history_window_propagates_later_page_failures() {
    let reader = reader(vec![
        Ok(page(&["13:18"], &[1])),
        Err(HistoricalKlineError::Rejected {
            ret_type: -2,
            err_code: 429,
            message: "history rate limited".to_owned(),
        }),
    ]);
    assert!(matches!(
        reader.query_window(&query()),
        Err(HistoricalKlineError::Rejected {
            ret_type: -2,
            err_code: 429,
            ..
        })
    ));
}

#[test]
fn history_window_bounds_upstream_requests() {
    let reader = reader((1..=32).map(|n| Ok(page(&["13:18"], &[n]))).collect());
    assert!(matches!(
        reader.query_window(&query()),
        Err(HistoricalKlineError::InvalidPagination)
    ));
    assert_eq!(reader.requests.lock().unwrap().len(), 32);
}

#[test]
fn history_window_accepts_a_terminal_page_at_the_request_budget() {
    let mut pages: Vec<_> = (1..32).map(|n| Ok(page(&["13:18"], &[n]))).collect();
    pages.push(Ok(page(&["13:19"], &[])));
    let reader = reader(pages);
    let result = reader.query_window(&query()).unwrap();
    assert_eq!(result.klines.len(), 2);
    assert_eq!(reader.requests.lock().unwrap().len(), 32);
}

#[test]
fn history_window_sorts_single_pages_and_clamps_provider_page_sizes() {
    for (limit, expected) in [(1, 200), (300, 300), (2000, 1000)] {
        let reader = reader(vec![Ok(page(&["13:19", "13:18", "13:18"], &[]))]);
        let mut request = query();
        request.max_ack_kl_num = Some(limit);
        let result = reader.query_window(&request).unwrap();
        assert_eq!(result.klines.len(), 2);
        assert_eq!(result.klines[0].time, "2026-09-09 13:18:00");
        assert_eq!(
            reader.requests.lock().unwrap()[0].max_ack_kl_num,
            Some(expected)
        );
    }
}

/// Parity: pkg/futu/exchange_mapping_boundaries_test.go:85
/// TestFutuKLineQueryWindowAndPreflightValidation (page-size boundary).
///
/// Go's `resolveHistoricalKLinePageSize` returns 0 for a non-positive limit so
/// the caller leaves `MaxAckKLNum` unset, enlarges anything below 200, and
/// clamps the tail at 1000. The 0 case is a real wire difference, not a
/// rounding detail: 200 would force a page size OpenD did not ask for.
#[test]
fn historical_page_size_preserves_the_unset_non_positive_budget() {
    for (limit, expected) in [(0, 0), (-1, 0), (1, 200), (50, 200), (199, 200), (200, 200)] {
        assert_eq!(
            crate::resolve_historical_kline_page_size(limit),
            expected,
            "page size for limit {limit}"
        );
    }
    for (limit, expected) in [(500, 500), (1000, 1000), (1001, 1000), (5000, 1000)] {
        assert_eq!(
            crate::resolve_historical_kline_page_size(limit),
            expected,
            "page size for limit {limit}"
        );
    }
}

#[test]
fn history_window_preserves_real_market_breaks_and_updates_duplicate_bars() {
    let mut last = page(&["12:00", "13:00"], &[]);
    last.klines[0].volume = Some(200);
    let reader = reader(vec![Ok(page(&["12:00"], &[1])), Ok(last)]);
    let result = reader.query_window(&query()).unwrap();
    assert_eq!(result.klines.len(), 2);
    assert_eq!(result.klines[0].volume, Some(200));
    assert_eq!(result.klines[1].time, "2026-09-09 13:00:00");
}

#[test]
fn history_window_rejects_another_security_on_a_later_page() {
    let mut wrong = page(&["13:19"], &[]);
    wrong.security.code = "00005".into();
    let reader = reader(vec![Ok(page(&["13:18"], &[1])), Ok(wrong)]);
    assert!(matches!(
        reader.query_window(&query()),
        Err(HistoricalKlineError::InvalidPagination)
    ));
}

/// Parity: pkg/futu/exchange_kline_test.go:291
/// TestQueryKLinesFollowsHistoryPaginationAndKeepsLatestLimit
///
/// Go follows `nextReqKey` across pages and keeps only the newest `limit`
/// bars. The bounded chart-history budget must not stop after a single page.
#[test]
fn history_window_follows_forward_pages_and_keeps_the_latest_limit() {
    let reader = reader(vec![
        Ok(page(&["10:00"], &[1])),
        Ok(page(&["10:05"], &[2])),
        Ok(page(&["10:10"], &[])),
    ]);
    let result = reader.query_window(&query()).expect("window");
    assert_eq!(reader.requests.lock().unwrap().len(), 3);
    let times = result
        .klines
        .iter()
        .map(|kline| kline.time.clone())
        .collect::<Vec<_>>();
    assert_eq!(
        times,
        vec![
            "2026-09-09 10:00:00",
            "2026-09-09 10:05:00",
            "2026-09-09 10:10:00"
        ]
    );
}

/// Parity: pkg/futu/exchange_kline_test.go:326
/// TestQueryKLinesAllowsMoreThanEightHistoryPages
///
/// OpenD can paginate a valid intraday window into more than the legacy eight
/// pages; the reader must keep following `nextReqKey` up to its budget.
#[test]
fn history_window_allows_more_than_eight_pages() {
    let mut responses: Vec<_> = (1..=9).map(|n| Ok(page(&["13:18"], &[n]))).collect();
    responses.push(Ok(page(&["13:19"], &[])));
    let reader = reader(responses);
    let result = reader.query_window(&query()).expect("nine pages");
    assert_eq!(reader.requests.lock().unwrap().len(), 10);
    assert_eq!(result.klines.len(), 2);
}

/// Parity: pkg/futu/exchange_kline_test.go:356
/// TestQueryKLinesUsesLargerHistoryPageSizeThanRequestedLimit
///
/// A small requested limit must not shrink the upstream page size: Go enlarges
/// the page to at least 200 bars and clamps at 1000.
#[test]
fn history_window_uses_a_larger_upstream_page_size_than_the_limit() {
    for (requested, expected) in [(2, 200), (500, 500), (5000, 1000)] {
        let reader = reader(vec![Ok(page(&["13:19"], &[]))]);
        let mut request = query();
        request.max_ack_kl_num = Some(requested);
        let _ = reader.query_window(&request).expect("window");
        assert_eq!(
            reader.requests.lock().unwrap()[0].max_ack_kl_num,
            Some(expected),
            "requested limit {requested}"
        );
    }
}

/// Parity: pkg/futu/exchange_kline_test.go:241 / :269
/// TestQueryKLinesNormalizesIntradayHistoryLabelToBucketStart /
/// TestQueryKLinesKeepsDailyHistoryLabelAsBucketStart
///
/// OpenD reports the bucket *label*; intraday bars must be shifted back by one
/// interval while daily bars keep the label as their open time. The adjustment
/// happens in the Futu decode path, so the same rule is asserted here through
/// `adjust_kline_time`.
#[test]
fn history_window_normalizes_intraday_history_label_to_bucket_start() {
    assert_eq!(
        crate::adjust_kline_time("2026-05-20 10:55:00", "1m"),
        "2026-05-20 10:54:00"
    );
    assert_eq!(
        crate::adjust_kline_time("2026-05-20 10:55:00", "5m"),
        "2026-05-20 10:50:00"
    );
    assert_eq!(
        crate::adjust_kline_time("2026-05-20 11:00:00", "60m"),
        "2026-05-20 10:00:00"
    );
}

#[test]
// Parity: go:452dea11:pkg/futu/exchange_kline_test.go:269 TestQueryKLinesKeepsDailyHistoryLabelAsBucketStart
fn history_window_keeps_daily_history_label_as_bucket_start() {
    assert_eq!(
        crate::adjust_kline_time("2026-05-20 00:00:00", "1d"),
        "2026-05-20 00:00:00"
    );
    assert_eq!(crate::adjust_kline_time("2026-05-20", "1d"), "2026-05-20");
}
