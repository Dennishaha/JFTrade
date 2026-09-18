// Current-bucket merge for routed Futu history reads.
// Kept out of the port module so it stays inside the workspace
// production-file line budget while the `Qot_GetKL` merge keeps enforcing the
// caller's window and session filters.

/// Inputs shared by the current-bucket merge path.
struct CurrentBucketMerge<'a> {
    market: &'a str,
    symbol: &'a str,
    period: &'a str,
    market_code: i32,
    window_begin: &'a str,
    window_end: &'a str,
    sessions: &'a [&'a str],
    route_sessions: bool,
    calendar: Option<&'a jftrade_calendar::CalendarManager>,
}

/// Merges the unresolved `Qot_GetKL` bucket into the routed history result.
///
/// Go calls `filterKLinesByWindow(currentKLines, beginAt, endAt)` before the
/// merge because `Qot_GetKL` can only ask for a bucket *count*, so the caller's
/// explicit window has to be enforced on the response. It then re-runs
/// `filterKLinesBySessions` over the merged list: the routed history pages were
/// already filtered per route, but the current-bucket call is not routed, so a
/// `regular`-only request could otherwise answer with a pre/after-hours bar.
fn merge_current_bucket(
    result: &mut jftrade_integration_futu::HistoricalKlineResult,
    runtime: &crate::product::product_production_ports::SharedTradeReadRuntime,
    request: CurrentBucketMerge<'_>,
) {
    let query = jftrade_integration_futu::CurrentKlineQuery::new(
        request.market_code,
        request.symbol,
        request.period,
    );
    let Ok(current_res) = runtime.current_kline(&query) else {
        return;
    };
    if !current_res.klines.is_empty() {
        let current_klines = quote_reads_futu::filter_current_klines_by_window(
            &current_res.klines,
            request.market,
            request.period,
            request.window_begin,
            request.window_end,
        );
        result.klines = jftrade_integration_futu::kline_query::merge_klines_by_time(
            &result.klines,
            &current_klines,
        );
    }
    if result.name.is_none() {
        result.name = current_res.name;
    }
    if request.route_sessions {
        result.klines = quote_reads_futu::filter_klines_by_sessions(
            &result.klines,
            request.calendar,
            request.market,
            request.period,
            request.sessions,
        );
    }
}

