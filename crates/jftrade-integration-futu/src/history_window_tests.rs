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
        Err(HistoricalKlineError::MissingS2c),
    ]);
    assert!(matches!(
        reader.query_window(&query()),
        Err(HistoricalKlineError::MissingS2c)
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
