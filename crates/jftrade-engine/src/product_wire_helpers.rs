fn is_managed_account_path(path: &str) -> bool {
    path.strip_prefix("/api/v1/settings/broker-accounts/")
        .is_some_and(|id| !id.is_empty() && !id.contains('/'))
}

fn managed_account_id(path: &str) -> Result<String, ApiFailure> {
    let encoded = path
        .strip_prefix("/api/v1/settings/broker-accounts/")
        .filter(|id| !id.is_empty() && !id.contains('/'))
        .ok_or_else(|| ApiFailure::new(400, "BAD_REQUEST", "invalid account id"))?;
    percent_decode_str(encoded)
        .decode_utf8()
        .map(|id| id.into_owned())
        .map_err(|_| ApiFailure::new(400, "BAD_REQUEST", "invalid account id"))
}

/// Maps the API `sessions` query values onto the Futu route labels used by
/// [`jftrade_integration_futu::build_request_plans`]. A missing `sessions`
/// parameter means "every session" so US intraday reads include extended and
/// overnight bars, matching Go's `buildHistoricalKLineRequestPlansForSessions`.
pub(crate) fn kline_route_sessions(
    market: &str,
    period: &str,
    sessions: &[&str],
) -> Option<Vec<jftrade_integration_futu::MarketSession>> {
    if !market.eq_ignore_ascii_case("US") || !is_routeable_period(period) {
        return None;
    }
    if sessions.is_empty() {
        return None;
    }
    let mut mapped = Vec::new();
    for session in sessions {
        match *session {
            "regular" => mapped.push(jftrade_integration_futu::MarketSession::Regular),
            "extended" => {
                mapped.push(jftrade_integration_futu::MarketSession::Pre);
                mapped.push(jftrade_integration_futu::MarketSession::After);
            }
            "pre" => mapped.push(jftrade_integration_futu::MarketSession::Pre),
            "after" => mapped.push(jftrade_integration_futu::MarketSession::After),
            "overnight" => mapped.push(jftrade_integration_futu::MarketSession::Overnight),
            _ => {}
        }
    }
    mapped.sort();
    mapped.dedup();
    Some(mapped)
}

fn is_routeable_period(period: &str) -> bool {
    jftrade_integration_futu::period_duration_seconds(period) > 0
        && jftrade_integration_futu::period_duration_seconds(period) <= 3600
}
