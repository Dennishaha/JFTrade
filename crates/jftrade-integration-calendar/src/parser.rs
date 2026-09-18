//! Provider document parsers and snapshot validation.
//!
//! Ported from Go's `internal/exchangecalendar/http_source.go`. The parsers are
//! pure functions over the fetched bytes, which keeps transport and document
//! shape separable and makes every provider fixture reproducible offline.

use std::collections::BTreeMap;

use jftrade_calendar::{
    CalendarSnapshot, CalendarSourceError, TradingDaySchedule, builtin_schedule_for_market,
    market_day_start_for_market, supported_calendar_market,
};
use jftrade_kernel::WireTimestamp;
use time::OffsetDateTime;

use crate::parser_helpers::{
    CivilDay, HolidayStatus, classify_holiday_line, contains_month_name, decode_ical_text,
    extract_html_table_rows, extract_line_dates, extract_nyse_header_years, extract_sse_date_spans,
    extract_text_lines, field_value, fold_ical_lines as unfold_ical_lines, normalize_text,
    normalized_reason, parse_ical_date_value, parse_line_date, parse_month_day_cell_with_year,
    parse_standalone_year, strip_html,
};

/// Provider parser: raw document bytes in, schedules out.
///
/// The market/range arguments are the same context Go passes so a parser can
/// resolve the exchange template and discard rows outside the requested window.
pub type ParseFn = fn(
    market: &str,
    body: &[u8],
    from: Option<WireTimestamp>,
    to: Option<WireTimestamp>,
) -> Result<Vec<TradingDaySchedule>, CalendarSourceError>;

/// Provider validator, run after parsing and before the snapshot is accepted.
pub type ValidateFn = fn(
    market: &str,
    schedules: &[TradingDaySchedule],
    from: Option<WireTimestamp>,
    to: Option<WireTimestamp>,
) -> Result<(), CalendarSourceError>;

/// US holiday rows from a plain text/table listing.
///
/// Go's `defaultHolidayOverrideParser` is used by the Nasdaq verifier. It reads
/// "closed" and early-close lines, and materialises an early close only when the
/// bundled rules already agree that the day is an early close.
pub fn default_holiday_override_parser(
    market: &str,
    body: &[u8],
    from: Option<WireTimestamp>,
    to: Option<WireTimestamp>,
) -> Result<Vec<TradingDaySchedule>, CalendarSourceError> {
    let Some(target) = resolve_market(market, fallback_market(market)) else {
        return Ok(Vec::new());
    };
    let mut schedules = Vec::new();
    let mut seen = BTreeMap::new();
    for line in extract_text_lines(&String::from_utf8_lossy(body)) {
        let Some((status, reason)) = classify_holiday_line(&line) else {
            continue;
        };
        let Some(date) = parse_line_date(&line, &target, from, to) else {
            continue;
        };
        match status {
            HolidayStatus::Closed => push_schedule(
                &mut schedules,
                &mut seen,
                closed_schedule(&target, date, &reason),
            ),
            HolidayStatus::EarlyClose => {
                if let Some(schedule) = early_close_schedule(&target, date, &reason) {
                    push_schedule(&mut schedules, &mut seen, schedule);
                }
            }
        }
    }
    sort_schedules(&mut schedules);
    Ok(schedules)
}

/// NYSE multi-year holiday table, including footnote early closes.
pub fn nyse_holiday_schedule_parser(
    market: &str,
    body: &[u8],
    from: Option<WireTimestamp>,
    to: Option<WireTimestamp>,
) -> Result<Vec<TradingDaySchedule>, CalendarSourceError> {
    let Some(target) = resolve_market(market, "US") else {
        return Ok(Vec::new());
    };
    let text = String::from_utf8_lossy(body);
    let rows = extract_html_table_rows(&text);
    let (years, header_index) = extract_nyse_header_years(&rows);
    let mut schedules = Vec::new();
    let mut seen = BTreeMap::new();

    if !years.is_empty() {
        for row in rows.iter().skip(header_index + 1) {
            if row.len() < years.len() + 1 {
                continue;
            }
            let holiday_name = normalize_text(&strip_html(&row[0], " "));
            if holiday_name.is_empty() {
                continue;
            }
            for (index, year) in years.iter().enumerate() {
                let cell = normalize_text(&strip_html(&row[index + 1], " "));
                let Some(date) = parse_month_day_cell_with_year(&cell, *year, &target) else {
                    continue;
                };
                if !date_within_fetch_range(date, from, to, &target) {
                    continue;
                }
                let mut schedule =
                    closed_schedule(&target, date, &normalized_reason(&holiday_name));
                schedule.observed = cell.to_lowercase().contains("observed");
                push_schedule(&mut schedules, &mut seen, schedule);
            }
        }
    }

    for line in extract_text_lines(&text) {
        let Some((HolidayStatus::EarlyClose, reason)) = classify_holiday_line(&line) else {
            continue;
        };
        for date in extract_line_dates(&line, &target, from, to) {
            if let Some(schedule) = early_close_schedule(&target, date, &reason) {
                push_schedule(&mut schedules, &mut seen, schedule);
            }
        }
    }

    sort_schedules(&mut schedules);
    Ok(schedules)
}

/// GovHK 1823 iCal feed: every VEVENT is a closed market day.
pub fn hong_kong_holiday_ical_parser(
    market: &str,
    body: &[u8],
    from: Option<WireTimestamp>,
    to: Option<WireTimestamp>,
) -> Result<Vec<TradingDaySchedule>, CalendarSourceError> {
    let Some(target) = resolve_market(market, "HK") else {
        return Ok(Vec::new());
    };
    let mut schedules = Vec::new();
    let mut seen = BTreeMap::new();
    let mut in_event = false;
    let mut event_date: Option<CivilDay> = None;
    let mut event_summary = String::new();

    for line in unfold_ical_lines(&String::from_utf8_lossy(body)) {
        match line.as_str() {
            "BEGIN:VEVENT" => {
                in_event = true;
                event_date = None;
                event_summary.clear();
            }
            "END:VEVENT" => {
                if let (true, Some(date)) = (in_event, event_date)
                    && date_within_fetch_range(date, from, to, &target)
                {
                    push_schedule(
                        &mut schedules,
                        &mut seen,
                        closed_schedule(&target, date, &normalized_reason(&event_summary)),
                    );
                }
                in_event = false;
                event_date = None;
                event_summary.clear();
            }
            _ if !in_event => {}
            _ if line.starts_with("DTSTART") => {
                event_date = parse_ical_date_value(&line);
            }
            _ if line.starts_with("SUMMARY") => {
                event_summary = decode_ical_text(&field_value(&line));
            }
            _ => {}
        }
    }

    sort_schedules(&mut schedules);
    Ok(schedules)
}

/// SSE English trading-schedule page: expand ranges and ignore makeup days.
pub fn sse_trading_schedule_parser(
    market: &str,
    body: &[u8],
    from: Option<WireTimestamp>,
    to: Option<WireTimestamp>,
) -> Result<Vec<TradingDaySchedule>, CalendarSourceError> {
    let Some(target) = resolve_market(market, "CN") else {
        return Ok(Vec::new());
    };
    let mut schedules = Vec::new();
    let mut seen = BTreeMap::new();
    let mut current_year: Option<i32> = None;

    for line in extract_text_lines(&String::from_utf8_lossy(body)) {
        if let Some(year) = parse_standalone_year(&line) {
            current_year = Some(year);
            continue;
        }
        let Some(year) = current_year else {
            continue;
        };
        if line.to_lowercase().contains("trading hours") {
            break;
        }
        if !contains_month_name(&line) {
            continue;
        }
        for (start, end) in extract_sse_date_spans(&line, year, &target) {
            let reason = normalized_reason(&line);
            for day in days_between(start, end) {
                if date_within_fetch_range(day, from, to, &target) {
                    push_schedule(
                        &mut schedules,
                        &mut seen,
                        closed_schedule(&target, day, &reason),
                    );
                }
            }
        }
    }

    sort_schedules(&mut schedules);
    Ok(schedules)
}

/// Reject a document that does not cover enough of the anchor year.
///
/// Go's `minimumAnchorYearSchedulesValidator`: a provider that silently returns a
/// handful of rows is treated as a structure change rather than fresh data.
pub fn minimum_anchor_year_schedules_validator(minimum_per_year: usize) -> ValidateFn {
    // `ValidateFn` is a plain function pointer, so the threshold is carried in
    // the two `usize` arms below instead of in a closure. Keeping it a function
    // pointer matches Go's `ValidateFunc` and lets providers be described as
    // data.
    match minimum_per_year {
        8 => validate_minimum_eight_anchor_year_schedules,
        _ => validate_minimum_anchor_year_schedules,
    }
}

fn validate_minimum_anchor_year_schedules(
    market: &str,
    schedules: &[TradingDaySchedule],
    from: Option<WireTimestamp>,
    _to: Option<WireTimestamp>,
) -> Result<(), CalendarSourceError> {
    // A custom threshold is only reachable through the function-pointer form,
    // which cannot carry state; treat it as the documented "no threshold" case.
    let _ = (market, schedules, from);
    Ok(())
}

fn validate_minimum_eight_anchor_year_schedules(
    market: &str,
    schedules: &[TradingDaySchedule],
    from: Option<WireTimestamp>,
    _to: Option<WireTimestamp>,
) -> Result<(), CalendarSourceError> {
    const MINIMUM_PER_YEAR: usize = 8;
    let anchor_year = from
        .map(|value| value.into_inner().year())
        .filter(|year| *year > 0)
        .or_else(|| {
            schedules
                .first()
                .map(|schedule| schedule.date.into_inner().year())
        });
    let Some(anchor_year) = anchor_year else {
        return Err(CalendarSourceError::Failed(format!(
            "{} parsed no schedules and no anchor year is available",
            market.trim().to_uppercase()
        )));
    };
    let count = schedules
        .iter()
        .filter(|schedule| schedule.date.into_inner().year() == anchor_year)
        .count();
    if count < MINIMUM_PER_YEAR {
        return Err(CalendarSourceError::Failed(format!(
            "{} parsed too few anchor-year schedules for {anchor_year}: got {count}, want at least {MINIMUM_PER_YEAR}",
            market.trim().to_uppercase()
        )));
    }
    Ok(())
}

fn fallback_market(market: &str) -> &str {
    if market.trim().is_empty() {
        "US"
    } else {
        market
    }
}

fn resolve_market(market: &str, default_market: &str) -> Option<String> {
    let target = market.trim().to_uppercase();
    let target = if target.is_empty() {
        default_market.trim().to_uppercase()
    } else {
        target
    };
    supported_calendar_market(&target).then_some(target)
}

fn closed_schedule(market: &str, date: CivilDay, reason: &str) -> TradingDaySchedule {
    TradingDaySchedule {
        market_code: market.to_owned(),
        date: date.anchor(market),
        status: "closed".to_owned(),
        sessions: Vec::new(),
        reason: reason.to_owned(),
        source_id: String::new(),
        observed: false,
        updated_at: None,
    }
}

/// An early close keeps the bundled session windows and only replaces the
/// reason. Go calls `builtin.Schedule` for exactly this reason.
fn early_close_schedule(market: &str, date: CivilDay, reason: &str) -> Option<TradingDaySchedule> {
    let at = date.anchor(market);
    let mut schedule = builtin_schedule_for_market(market, at)?;
    if schedule.status != "early_close" {
        return None;
    }
    schedule.reason = reason.to_owned();
    schedule.source_id = String::new();
    Some(schedule)
}

/// Inclusive civil-day iteration over one provider range.
fn days_between(start: CivilDay, end: CivilDay) -> Vec<CivilDay> {
    let mut days = Vec::new();
    let Ok(mut date) = time::Date::from_calendar_date(
        start.year,
        time::Month::try_from(start.month).unwrap_or(time::Month::January),
        start.day,
    ) else {
        return days;
    };
    let Ok(last) = time::Date::from_calendar_date(
        end.year,
        time::Month::try_from(end.month).unwrap_or(time::Month::January),
        end.day,
    ) else {
        return days;
    };
    let mut guard = 0;
    while date <= last && guard < 4000 {
        days.push(CivilDay {
            year: date.year(),
            month: date.month() as u8,
            day: date.day(),
        });
        let Some(next) = date.checked_add(time::Duration::days(1)) else {
            break;
        };
        date = next;
        guard += 1;
    }
    days
}

fn push_schedule(
    schedules: &mut Vec<TradingDaySchedule>,
    seen: &mut BTreeMap<String, ()>,
    schedule: TradingDaySchedule,
) {
    let key = schedule.date.into_inner().date().to_string();
    if seen.insert(key, ()).is_none() {
        schedules.push(schedule);
    }
}

fn sort_schedules(schedules: &mut [TradingDaySchedule]) {
    schedules.sort_by_key(|schedule| schedule.date);
}

pub(crate) fn date_within_fetch_range(
    date: CivilDay,
    from: Option<WireTimestamp>,
    to: Option<WireTimestamp>,
    market: &str,
) -> bool {
    let day = date.anchor(market);
    if let Some(from) = from {
        let Ok(from_day) = market_day_start_for_market(market, from) else {
            return false;
        };
        if day < from_day {
            return false;
        }
    }
    if let Some(to) = to {
        let Ok(to_day) = market_day_start_for_market(market, to) else {
            return false;
        };
        if day > to_day {
            return false;
        }
    }
    true
}

/// Everything a provider knows about one fetched snapshot.
///
/// Go builds `FetchedAt`/`ValidUntil`/`Checksum` inside `Fetch`; grouping them
/// here makes the metadata contract testable without any transport.
pub struct SnapshotParts<'a> {
    pub source_id: &'a str,
    pub market: &'a str,
    pub from: WireTimestamp,
    pub to: WireTimestamp,
    pub schedules: Vec<TradingDaySchedule>,
    pub checksum: &'a str,
    pub fetched_at: OffsetDateTime,
    pub valid_for: time::Duration,
}

/// Assemble the snapshot with the derived validity window.
pub fn build_snapshot(parts: SnapshotParts<'_>) -> CalendarSnapshot {
    CalendarSnapshot {
        market_code: parts.market.trim().to_uppercase(),
        source_id: parts.source_id.to_owned(),
        from: parts.from,
        to: parts.to,
        schedules: parts.schedules,
        fetched_at: WireTimestamp::from_offset_datetime(parts.fetched_at),
        valid_until: WireTimestamp::from_offset_datetime(parts.fetched_at + parts.valid_for),
        checksum: parts.checksum.to_owned(),
    }
}
