// Exchange-calendar session resolution for routed Futu history reads.
// Kept out of the port module so it stays inside the workspace
// production-file line budget while the routed-history keep-filter still
// resolves against the authoritative calendar.

/// Resolves the exchange-calendar session used by the routed-history
/// keep-filter.
pub(super) fn calendar_session_for_route(
    calendar: Option<&jftrade_calendar::CalendarManager>,
    market: &str,
    at: &str,
) -> Option<jftrade_integration_futu::MarketSession> {
    let calendar = calendar?;
    let parsed =
        time::OffsetDateTime::parse(at, &time::format_description::well_known::Rfc3339).ok()?;
    let session = calendar
        .classify_session(
            market,
            jftrade_kernel::WireTimestamp::from_offset_datetime(parsed),
        )
        .ok()??;
    match session.as_str() {
        "regular" => Some(jftrade_integration_futu::MarketSession::Regular),
        "pre" => Some(jftrade_integration_futu::MarketSession::Pre),
        "after" => Some(jftrade_integration_futu::MarketSession::After),
        "overnight" => Some(jftrade_integration_futu::MarketSession::Overnight),
        _ => None,
    }
}

/// Applies the routed-history keep-filter for one returned page.
///
/// Go's plan filter only drops a bar when `resolveMarketSession` produces a
/// market session the route does not keep. For RTH and OVERNIGHT routes the
/// plan *forces* the label, so an unclassifiable bar is retained here and the
/// per-candle annotation step is what reports Go's
/// `unable to classify K-line session at <ts>` data error. Dropping the bar
/// here would silently answer an empty page instead of that error.
pub(super) fn filter_routed_page(
    plan: &jftrade_integration_futu::HistoricalKlineRequestPlan,
    result: &mut jftrade_integration_futu::HistoricalKlineResult,
    calendar: Option<&jftrade_calendar::CalendarManager>,
    market: &str,
) {
    if plan.keep_sessions.is_empty() {
        return;
    }
    // Without an authoritative calendar the route cannot be refined further;
    // keep the page as returned.
    if calendar.is_none() {
        return;
    }
    let mut kept = Vec::with_capacity(result.klines.len());
    for kline in result.klines.drain(..) {
        let at = canonical_candle_time(&kline.time, market);
        let Some(clock) = calendar_session_for_route(calendar, market, &at) else {
            kept.push(kline);
            continue;
        };
        let resolved = plan.resolve_market_session(clock);
        if plan.should_keep_market_session(resolved) {
            kept.push(kline);
        }
    }
    result.klines = kept;
}
