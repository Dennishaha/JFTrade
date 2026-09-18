//! Document-shape helpers shared by the provider parsers.
//!
//! These mirror Go's `extractTextLines` / `extractHTMLTableRows` /
//! `unfoldICalLines` family and the civil-date normalisation the parsers rely
//! on. They are pure functions over provider bytes, so they are kept separate
//! from the provider-specific parsers in `parser.rs`.

use std::collections::BTreeMap;

use jftrade_calendar::market_local_midnight;
use jftrade_kernel::WireTimestamp;

use crate::parser::date_within_fetch_range;
use regex::Regex;
use time::{Date, Month, PrimitiveDateTime, Time};

/// Provider date before it is anchored in the exchange timezone.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub(crate) struct CivilDay {
    pub(crate) year: i32,
    pub(crate) month: u8,
    pub(crate) day: u8,
}

impl CivilDay {
    /// Build a civil day only when it names a real calendar date.
    ///
    /// Go reaches the same guarantee through `time.ParseInLocation`: an
    /// authority document that says `February 30` simply fails to parse, so the
    /// row is discarded instead of being silently normalised into March.
    pub(crate) fn parse(year: i32, month: u8, day: u8) -> Option<Self> {
        Date::from_calendar_date(year, Month::try_from(month).ok()?, day).ok()?;
        Some(Self { year, month, day })
    }
}

impl CivilDay {
    /// Anchor this civil date at the market's local midnight.
    pub(crate) fn anchor(self, market: &str) -> WireTimestamp {
        market_local_midnight(market, self.year, self.month, self.day).unwrap_or_else(|_| {
            WireTimestamp::from_offset_datetime(
                PrimitiveDateTime::new(
                    Date::from_calendar_date(
                        self.year,
                        Month::try_from(self.month).unwrap_or(Month::January),
                        self.day,
                    )
                    .unwrap_or_else(|_| {
                        Date::from_calendar_date(1, Month::January, 1).expect("valid year 1")
                    }),
                    Time::MIDNIGHT,
                )
                .assume_utc(),
            )
        })
    }
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub(crate) enum HolidayStatus {
    Closed,
    EarlyClose,
}

pub(crate) fn classify_holiday_line(line: &str) -> Option<(HolidayStatus, String)> {
    let normalized = line.trim().to_lowercase();
    let reason = normalized_reason(&normalized);
    if normalized.contains("closed") || normalized.contains("休市") {
        return Some((HolidayStatus::Closed, reason));
    }
    if normalized.contains("early close")
        || normalized.contains("half day")
        || normalized.contains("half-day")
        || normalized.contains("提前收市")
        || normalized.contains("1:00 p.m.")
        || normalized.contains("1:00 pm")
    {
        return Some((HolidayStatus::EarlyClose, reason));
    }
    None
}

pub(crate) fn normalized_reason(line: &str) -> String {
    line.trim()
        .to_lowercase()
        .chars()
        .filter_map(|character| match character {
            ' ' => Some('_'),
            ',' | '.' | '(' | ')' => None,
            '/' => Some('_'),
            other => Some(other),
        })
        .collect()
}

pub(crate) fn extract_text_lines(body: &str) -> Vec<String> {
    let mut lines = Vec::new();
    let mut seen = BTreeMap::new();
    let mut append = |line: &str| {
        let trimmed = normalize_text(line);
        if trimmed.is_empty() {
            return;
        }
        if seen.insert(trimmed.clone(), ()).is_none() {
            lines.push(trimmed);
        }
    };

    let row_pattern = Regex::new(r"(?is)<tr[^>]*>(.*?)</tr>").expect("static row regex");
    for captures in row_pattern.captures_iter(body) {
        append(&strip_html(captures.get(1).map_or("", |m| m.as_str()), " "));
    }
    let block_pattern =
        Regex::new(r"(?is)<(li|p|div|section|article)[^>]*>(.*?)</(li|p|div|section|article)>")
            .expect("static block regex");
    for captures in block_pattern.captures_iter(body) {
        append(&strip_html(captures.get(2).map_or("", |m| m.as_str()), " "));
    }
    let tag_pattern = Regex::new(r"(?s)<[^>]+>").expect("static tag regex");
    for line in tag_pattern.replace_all(body, "\n").split('\n') {
        append(line);
    }
    lines
}

pub(crate) fn extract_html_table_rows(body: &str) -> Vec<Vec<String>> {
    let row_pattern = Regex::new(r"(?is)<tr[^>]*>(.*?)</tr>").expect("static row regex");
    let cell_pattern = Regex::new(r"(?is)<t[hd][^>]*>(.*?)</t[hd]>").expect("static cell regex");
    let mut rows = Vec::new();
    for row in row_pattern.captures_iter(body) {
        let inner = row.get(1).map_or("", |m| m.as_str());
        let cells = cell_pattern
            .captures_iter(inner)
            .map(|cell| cell.get(1).map_or(String::new(), |m| m.as_str().to_owned()))
            .collect::<Vec<_>>();
        if !cells.is_empty() {
            rows.push(cells);
        }
    }
    rows
}

pub(crate) fn fold_ical_lines(body: &str) -> Vec<String> {
    let normalized = body.replace("\r\n", "\n");
    let mut lines: Vec<String> = Vec::new();
    for raw in normalized.split('\n') {
        let line = raw.trim_end_matches('\r');
        if let Some(last) = lines.last_mut()
            && (line.starts_with(' ') || line.starts_with('\t'))
        {
            last.push_str(line.trim_start_matches([' ', '\t']));
            continue;
        }
        lines.push(line.trim().to_owned());
    }
    lines
}

pub(crate) fn field_value(line: &str) -> String {
    line.split_once(':')
        .map_or(String::new(), |(_, value)| value.trim().to_owned())
}

pub(crate) fn parse_ical_date_value(line: &str) -> Option<CivilDay> {
    // `DTSTART;VALUE=DATE:20260101` and `DTSTART:20260101T000000Z` both carry
    // the date as the first eight digits of the value; an empty value yields
    // `None` exactly like Go's `parseICalDateValue`.
    let value = line.split_once(':').map_or("", |(_, tail)| tail).trim();
    let digits = value.get(0..8)?;
    if !digits.bytes().all(|byte| byte.is_ascii_digit()) {
        return None;
    }
    CivilDay::parse(
        digits.get(0..4)?.parse::<i32>().ok()?,
        digits.get(4..6)?.parse::<u8>().ok()?,
        digits.get(6..8)?.parse::<u8>().ok()?,
    )
}

pub(crate) fn decode_ical_text(value: &str) -> String {
    let replaced = value
        .replace("\\n", " ")
        .replace("\\N", " ")
        .replace("\\,", ",")
        .replace("\\;", ";")
        .replace("\\\\", "\\");
    normalize_text(&replaced)
}

pub(crate) fn parse_line_date(
    line: &str,
    market: &str,
    from: Option<WireTimestamp>,
    to: Option<WireTimestamp>,
) -> Option<CivilDay> {
    extract_line_dates(line, market, from, to)
        .into_iter()
        .next()
}

pub(crate) fn extract_line_dates(
    line: &str,
    market: &str,
    from: Option<WireTimestamp>,
    to: Option<WireTimestamp>,
) -> Vec<CivilDay> {
    let pattern = Regex::new(
        r"(?i)([A-Z][a-z]+ \d{1,2}, \d{4}|[A-Z][a-z]{2} \d{1,2}, \d{4}|\d{1,2} [A-Z][a-z]+ \d{4}|\d{4}-\d{2}-\d{2}|\d{4}/\d{2}/\d{2}|\d{4}\.\d{2}\.\d{2}|\d{4}年\d{1,2}月\d{1,2}日)",
    )
    .expect("static date regex");
    pattern
        .find_iter(line)
        .filter_map(|hit| parse_any_date(hit.as_str()))
        .filter(|date| date_within_fetch_range(*date, from, to, market))
        .collect()
}

pub(crate) fn parse_any_date(value: &str) -> Option<CivilDay> {
    // Numeric "2006年1月2日" and "2026-10-01" forms first, then the
    // month-name forms in both orders ("June 19, 2026", "1 October 2026").
    if let Some(day) = parse_numeric_date(value) {
        return Some(day);
    }
    let pattern = Regex::new(
        r"(?i)([A-Z][a-z]+)\s+(\d{1,2}),?\s+(\d{4})|(\d{1,2})\s+([A-Z][a-z]+)\s+(\d{4})",
    )
    .expect("static month-name regex");
    let captures = pattern.captures(value)?;
    if let (Some(month), Some(day), Some(year)) =
        (captures.get(1), captures.get(2), captures.get(3))
    {
        return CivilDay::parse(
            year.as_str().parse::<i32>().ok()?,
            month_number(&month.as_str().to_lowercase())?,
            day.as_str().parse::<u8>().ok()?,
        );
    }
    CivilDay::parse(
        captures.get(6)?.as_str().parse::<i32>().ok()?,
        month_number(&captures.get(5)?.as_str().to_lowercase())?,
        captures.get(4)?.as_str().parse::<u8>().ok()?,
    )
}

pub(crate) fn parse_numeric_date(value: &str) -> Option<CivilDay> {
    let digits = Regex::new(r"(\d{4})\D+(\d{1,2})\D+(\d{1,2})").expect("static numeric regex");
    let captures = digits.captures(value)?;
    CivilDay::parse(
        captures.get(1)?.as_str().parse::<i32>().ok()?,
        captures.get(2)?.as_str().parse::<u8>().ok()?,
        captures.get(3)?.as_str().parse::<u8>().ok()?,
    )
}

pub(crate) fn month_number(value: &str) -> Option<u8> {
    Some(match value {
        "january" | "jan" => 1,
        "february" | "feb" => 2,
        "march" | "mar" => 3,
        "april" | "apr" => 4,
        "may" => 5,
        "june" | "jun" => 6,
        "july" | "jul" => 7,
        "august" | "aug" => 8,
        "september" | "sep" | "sept" => 9,
        "october" | "oct" => 10,
        "november" | "nov" => 11,
        "december" | "dec" => 12,
        _ => return None,
    })
}

pub(crate) fn strip_html(body: &str, separator: &str) -> String {
    let tag_pattern = Regex::new(r"(?s)<[^>]+>").expect("static tag regex");
    tag_pattern.replace_all(body, separator).into_owned()
}

pub(crate) fn normalize_text(value: &str) -> String {
    value.split_whitespace().collect::<Vec<_>>().join(" ")
}

pub(crate) fn extract_nyse_header_years(rows: &[Vec<String>]) -> (Vec<i32>, usize) {
    for (index, row) in rows.iter().enumerate() {
        if row.len() < 2 {
            continue;
        }
        if normalize_text(&strip_html(&row[0], " ")) != "Holiday" {
            continue;
        }
        let mut years = Vec::new();
        for cell in &row[1..] {
            let Ok(year) = normalize_text(&strip_html(cell, " ")).parse::<i32>() else {
                years.clear();
                break;
            };
            years.push(year);
        }
        if !years.is_empty() {
            return (years, index);
        }
    }
    (Vec::new(), 0)
}

pub(crate) fn parse_month_day_cell_with_year(
    cell: &str,
    year: i32,
    _market: &str,
) -> Option<CivilDay> {
    let pattern = Regex::new(
        r"(?i)(January|February|March|April|May|June|July|August|September|October|November|December)\s+(\d{1,2})",
    )
    .expect("static month/day regex");
    let captures = pattern.captures(cell)?;
    CivilDay::parse(
        year,
        month_number(&captures.get(1)?.as_str().to_lowercase())?,
        captures.get(2)?.as_str().parse::<u8>().ok()?,
    )
}

pub(crate) fn parse_standalone_year(line: &str) -> Option<i32> {
    let trimmed = line.trim().trim_start_matches("##").trim();
    let pattern = Regex::new(r"^\d{4}$").expect("static year regex");
    pattern
        .is_match(trimmed)
        .then(|| trimmed.parse().ok())
        .flatten()
}

pub(crate) fn contains_month_name(line: &str) -> bool {
    let lower = line.to_lowercase();
    [
        "january",
        "february",
        "march",
        "april",
        "may",
        "june",
        "july",
        "august",
        "september",
        "october",
        "november",
        "december",
    ]
    .iter()
    .any(|month| lower.contains(month))
}

pub(crate) fn extract_sse_date_spans(
    line: &str,
    year: i32,
    market: &str,
) -> Vec<(CivilDay, CivilDay)> {
    let lower = line.to_lowercase();
    let truncated = match lower.find(", plus ") {
        Some(index) => &line[..index],
        None => line,
    };
    let pattern = Regex::new(
        r"(?i)(January|February|March|April|May|June|July|August|September|October|November|December)\s+(\d{1,2})(?:,\s*(\d{4}))?(?:\s*\([^)]*\))?(?:\s*-\s*(January|February|March|April|May|June|July|August|September|October|November|December)\s+(\d{1,2})(?:,\s*(\d{4}))?(?:\s*\([^)]*\))?)?",
    )
    .expect("static SSE range regex");
    let mut spans = Vec::new();
    for captures in pattern.captures_iter(truncated) {
        let Some(start_month) = captures
            .get(1)
            .and_then(|m| month_number(&m.as_str().to_lowercase()))
        else {
            continue;
        };
        let Some(start_day) = captures.get(2).and_then(|m| m.as_str().parse::<u8>().ok()) else {
            continue;
        };
        let start_year = captures
            .get(3)
            .and_then(|m| m.as_str().trim().parse::<i32>().ok())
            .unwrap_or(year);
        let Some(start) = CivilDay::parse(start_year, start_month, start_day) else {
            continue;
        };
        let mut end = start;
        if let (Some(end_month), Some(end_day)) = (
            captures
                .get(4)
                .and_then(|m| month_number(&m.as_str().to_lowercase())),
            captures.get(5).and_then(|m| m.as_str().parse::<u8>().ok()),
        ) {
            let explicit_end_year = captures
                .get(6)
                .and_then(|m| m.as_str().trim().parse::<i32>().ok());
            let end_year = explicit_end_year.unwrap_or(start_year);
            let Some(parsed_end) = CivilDay::parse(end_year, end_month, end_day) else {
                continue;
            };
            // "December 31 - January 2" with no explicit end year rolls into
            // the following year, exactly like Go's `extractSSEDateSpans`.
            if explicit_end_year.is_none()
                && (parsed_end.month, parsed_end.day) < (start.month, start.day)
            {
                let Some(rolled) = CivilDay::parse(end_year + 1, end_month, end_day) else {
                    continue;
                };
                end = rolled;
            } else {
                end = parsed_end;
            }
        }
        let _ = market;
        spans.push((start, end));
    }
    spans
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::str::FromStr;

    /// Parity: go:452dea11:internal/exchangecalendar/http_source_boundaries_test.go:89
    /// TestCalendarParserHelpersHandleMalformedAndPartialAuthorityDocuments
    /// (helper-level half).
    ///
    /// The integration tests reach these helpers through the provider parsers;
    /// these cases assert the helper contracts directly, because the observable
    /// behaviour at the parser level cannot distinguish "discarded by the
    /// helper" from "discarded by the caller".
    #[test]
    fn helper_contracts_match_the_go_authority_boundaries() {
        let unfolded =
            fold_ical_lines("BEGIN:VEVENT\r\nSUMMARY:National\r\n Day\r\nEND:VEVENT\r\n");
        assert_eq!(unfolded.len(), 4, "lines = {unfolded:?}");
        assert_eq!(
            unfolded[1], "SUMMARY:NationalDay",
            "a folded continuation is appended without re-inserting the break"
        );

        assert_eq!(
            field_value("SUMMARY without separator"),
            "",
            "a line without a separator has no value"
        );
        assert_eq!(field_value("SUMMARY: National Day "), "National Day");

        assert!(
            parse_ical_date_value("DTSTART:20260102T153000Z").is_some(),
            "a timestamped DTSTART carries its civil date"
        );
        assert!(
            parse_ical_date_value("DTSTART;VALUE=DATE:20260102").is_some(),
            "a date-only DTSTART is parsed too"
        );
        assert_eq!(
            parse_ical_date_value("DTSTART:"),
            None,
            "an empty DTSTART value is rejected"
        );
        assert_eq!(
            parse_ical_date_value("DTSTART:invalid"),
            None,
            "a non-numeric DTSTART value is rejected"
        );

        assert!(
            parse_month_day_cell_with_year("February 30", 2026, "US").is_none(),
            "an impossible civil day is rejected instead of normalised into March"
        );
        assert!(
            parse_month_day_cell_with_year("TBD", 2026, "US").is_none(),
            "a non-date cell has no month/day pair"
        );
        assert!(parse_month_day_cell_with_year("February 29", 2024, "US").is_some());
        assert!(
            parse_month_day_cell_with_year("February 29", 2026, "US").is_none(),
            "a leap day outside a leap year is rejected"
        );
        assert_eq!(parse_standalone_year("## 2026"), Some(2026));
        assert_eq!(
            parse_standalone_year("calendar 2026"),
            None,
            "an embedded year is not a standalone year header"
        );
        assert!(!contains_month_name("exchange holiday notice"));
        assert!(contains_month_name("National Day October 1"));

        let spans = extract_sse_date_spans(
            "National Day December 31, 2026 - January 2, 2027, plus makeup workdays",
            2026,
            "US",
        );
        assert_eq!(
            spans.len(),
            1,
            "the makeup-day tail is truncated: {spans:?}"
        );
        assert_eq!(spans[0].1.year, 2027, "the explicit end year is honoured");

        assert!(
            extract_sse_date_spans("Holiday February 30", 2026, "US").is_empty(),
            "an impossible single date produces no span"
        );
        assert!(
            extract_sse_date_spans("Holiday January 1 - February 30", 2026, "US").is_empty(),
            "an impossible span end produces no span"
        );
        assert!(
            parse_any_date("February 30, 2026").is_none(),
            "a month-name date with an impossible day is not a date"
        );
        assert!(
            parse_numeric_date("2026-02-30").is_none(),
            "a numeric date with an impossible day is not a date"
        );
    }

    #[test]
    fn fetch_range_uses_the_market_local_boundaries() {
        // Every fixture instant is market-anchored (US Eastern in July), which
        // is what Go's `DayStart(template, ...)` compares: a provider civil day
        // is anchored at local midnight, and the window edges are folded onto
        // the same local day before the comparison.
        let from = Some(WireTimestamp::from_str("2026-07-02T00:00:00-04:00").expect("window"));
        let to = Some(WireTimestamp::from_str("2026-07-04T00:00:00-04:00").expect("window"));
        let day = |day| CivilDay::parse(2026, 7, day).expect("valid civil day");

        assert!(
            !date_within_fetch_range(day(1), from, to, "US"),
            "a day before the window is rejected"
        );
        assert!(date_within_fetch_range(day(2), from, to, "US"));
        assert!(date_within_fetch_range(day(4), from, to, "US"));
        assert!(
            !date_within_fetch_range(day(5), from, to, "US"),
            "a day after the window is rejected"
        );
        assert!(
            date_within_fetch_range(day(1), None, None, "US"),
            "an unbounded window accepts every date"
        );
    }
}
