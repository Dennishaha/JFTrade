use jftrade_kernel::WireTimestamp;
use time::Weekday;

use crate::{
    BUILTIN_SOURCE_ID, CalendarManagerSettings, CalendarManualOverride, CalendarSessionWindow,
    TradingDaySchedule,
};

pub(crate) fn manual_schedule(
    settings: &CalendarManagerSettings,
    market: &str,
    at: WireTimestamp,
) -> Option<TradingDaySchedule> {
    let date = at.into_inner().date().to_string();
    settings
        .manual_overrides
        .iter()
        .find(|manual| {
            let manual_market = normalize_market(&manual.market);
            (manual_market == market || (manual_market == "CN" && matches!(market, "SH" | "SZ")))
                && manual.date.trim() == date
        })
        .map(|manual| schedule_from_manual(manual, market, at))
}

fn schedule_from_manual(
    manual: &CalendarManualOverride,
    market: &str,
    at: WireTimestamp,
) -> TradingDaySchedule {
    let mut sessions = manual
        .sessions
        .iter()
        .filter(|session| session.end_minute > session.start_minute)
        .map(|session| CalendarSessionWindow {
            kind: normalize_session_kind(&session.kind),
            start_minute: session.start_minute,
            end_minute: session.end_minute,
        })
        .collect::<Vec<_>>();
    sessions.sort_by_key(|session| (session.start_minute, session.end_minute));
    TradingDaySchedule {
        market_code: market.to_owned(),
        date: at,
        status: normalize_status(&manual.status),
        sessions,
        reason: manual.reason.trim().to_owned(),
        source_id: crate::MANUAL_OVERRIDE_SOURCE_ID.to_owned(),
        observed: manual.observed,
        updated_at: None,
    }
}

pub(crate) fn builtin_schedule(market: &str, at: WireTimestamp) -> TradingDaySchedule {
    let closed = matches!(
        at.into_inner().weekday(),
        Weekday::Saturday | Weekday::Sunday
    );
    let sessions = if closed {
        Vec::new()
    } else {
        builtin_sessions(market)
    };
    TradingDaySchedule {
        market_code: market.to_owned(),
        date: at,
        status: if closed { "closed" } else { "open" }.to_owned(),
        sessions,
        reason: if closed { "weekend" } else { "" }.to_owned(),
        source_id: BUILTIN_SOURCE_ID.to_owned(),
        observed: false,
        updated_at: None,
    }
}

fn builtin_sessions(market: &str) -> Vec<CalendarSessionWindow> {
    let regular = |start_minute, end_minute| CalendarSessionWindow {
        kind: "regular".to_owned(),
        start_minute,
        end_minute,
    };
    match market {
        "HK" => vec![regular(570, 720), regular(780, 960)],
        "CN" | "SH" | "SZ" => vec![regular(570, 690), regular(780, 900)],
        _ => vec![regular(570, 960)],
    }
}

fn normalize_market(market: &str) -> String {
    market.trim().to_uppercase()
}

fn normalize_status(status: &str) -> String {
    match status.trim().to_lowercase().as_str() {
        "open" | "closed" | "early_close" | "special" => status.trim().to_lowercase(),
        _ => "unknown".to_owned(),
    }
}

fn normalize_session_kind(kind: &str) -> String {
    match kind.trim().to_lowercase().as_str() {
        "closed" | "pre" | "regular" | "after" | "overnight" => kind.trim().to_lowercase(),
        _ => "unknown".to_owned(),
    }
}
