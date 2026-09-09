//! 5-field cron parser and schedule calculation with timezone support.
//!
//! Evaluates `minute hour dom month dow` with standard cron syntax:
//! `*`, `*/step`, `start-end`, `start-end/step`, `val1,val2,...`.
//! Timezones default to "Asia/Shanghai" matching the Go reference implementation.

use std::collections::BTreeSet;

use serde_json::Value;
use thiserror::Error;
use time::OffsetDateTime;
use time::format_description::well_known::Rfc3339;

#[derive(Debug, Error, PartialEq, Eq)]
pub enum CronError {
    #[error("schedule cron is required")]
    MissingCron,
    #[error("schedule cron must contain 5 fields")]
    InvalidFieldCount,
    #[error("invalid cron field '{field}': {detail}")]
    InvalidField { field: &'static str, detail: String },
    #[error("invalid schedule timezone: {0}")]
    InvalidTimezone(String),
    #[error("failed to find next schedule run within search limit")]
    SearchLimitExceeded,
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct CronField {
    pub is_wildcard: bool,
    pub values: BTreeSet<u32>,
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct ParsedCron {
    pub minute: CronField,
    pub hour: CronField,
    pub dom: CronField,
    pub month: CronField,
    pub dow: CronField,
}

fn parse_single_or_range(
    range_str: &str,
    step: u32,
    min: u32,
    max: u32,
    field_name: &'static str,
    values: &mut BTreeSet<u32>,
) -> Result<(), CronError> {
    let (start, end) = if range_str == "*" {
        (min, max)
    } else if let Some((start_s, end_s)) = range_str.split_once('-') {
        let start: u32 = start_s
            .trim()
            .parse()
            .map_err(|_| CronError::InvalidField {
                field: field_name,
                detail: format!("invalid range start '{start_s}'"),
            })?;
        let end: u32 = end_s.trim().parse().map_err(|_| CronError::InvalidField {
            field: field_name,
            detail: format!("invalid range end '{end_s}'"),
        })?;
        (start, end)
    } else {
        let single: u32 = range_str.parse().map_err(|_| CronError::InvalidField {
            field: field_name,
            detail: format!("invalid number '{range_str}'"),
        })?;
        (single, single)
    };

    if start > end {
        return Err(CronError::InvalidField {
            field: field_name,
            detail: format!("range start {start} > end {end}"),
        });
    }

    if field_name == "dow" {
        if start > 7 || end > 7 {
            return Err(CronError::InvalidField {
                field: field_name,
                detail: format!("dow out of range (0-7): {start}-{end}"),
            });
        }
    } else if start < min || end > max {
        return Err(CronError::InvalidField {
            field: field_name,
            detail: format!("value out of range ({min}-{max}): {start}-{end}"),
        });
    }

    let mut current = start;
    while current <= end {
        let normalized = if field_name == "dow" && current == 7 {
            0
        } else {
            current
        };
        values.insert(normalized);
        let Some(next) = current.checked_add(step) else {
            break;
        };
        current = next;
    }
    Ok(())
}

fn parse_field(
    field_str: &str,
    min: u32,
    max: u32,
    field_name: &'static str,
) -> Result<CronField, CronError> {
    let normalized = normalize_named_field(field_str, field_name);
    let trimmed = normalized.trim();
    if trimmed.is_empty() {
        return Err(CronError::InvalidField {
            field: field_name,
            detail: "empty field".to_string(),
        });
    }

    let mut is_wildcard = false;
    let mut values = BTreeSet::new();

    for part in trimmed.split(',') {
        let part = part.trim();
        if part.is_empty() {
            return Err(CronError::InvalidField {
                field: field_name,
                detail: "empty item in list".to_string(),
            });
        }

        let (range_str, step) = if let Some((r, s)) = part.split_once('/') {
            let step: u32 = s.trim().parse().map_err(|_| CronError::InvalidField {
                field: field_name,
                detail: format!("invalid step '{s}'"),
            })?;
            if step == 0 {
                return Err(CronError::InvalidField {
                    field: field_name,
                    detail: "step cannot be 0".to_string(),
                });
            }
            let range = r.trim();
            // robfig/cron semantics: N/step means N-max/step, while
            // */step remains a wildcard field for DOM/DOW union rules.
            let range = if range != "*" && !range.contains('-') {
                format!("{range}-{max}")
            } else {
                range.to_owned()
            };
            (range, step)
        } else {
            (part.to_owned(), 1)
        };

        is_wildcard |= range_str == "*" && step == 1;

        parse_single_or_range(&range_str, step, min, max, field_name, &mut values)?;
    }

    if values.is_empty() {
        return Err(CronError::InvalidField {
            field: field_name,
            detail: "no valid values resolved".to_string(),
        });
    }

    Ok(CronField {
        is_wildcard,
        values,
    })
}

pub fn parse_cron_expr(expr: &str) -> Result<ParsedCron, CronError> {
    let parts: Vec<&str> = expr.split_whitespace().collect();
    if parts.len() != 5 {
        return Err(CronError::InvalidFieldCount);
    }
    let minute = parse_field(parts[0], 0, 59, "minute")?;
    let hour = parse_field(parts[1], 0, 23, "hour")?;
    let dom = parse_field(parts[2], 1, 31, "dom")?;
    let month = parse_field(parts[3], 1, 12, "month")?;
    let dow = parse_field(parts[4], 0, 7, "dow")?;

    Ok(ParsedCron {
        minute,
        hour,
        dom,
        month,
        dow,
    })
}

fn weekday_to_cron_dow(weekday: jiff::civil::Weekday) -> u32 {
    match weekday {
        jiff::civil::Weekday::Sunday => 0,
        jiff::civil::Weekday::Monday => 1,
        jiff::civil::Weekday::Tuesday => 2,
        jiff::civil::Weekday::Wednesday => 3,
        jiff::civil::Weekday::Thursday => 4,
        jiff::civil::Weekday::Friday => 5,
        jiff::civil::Weekday::Saturday => 6,
    }
}

pub fn next_schedule_run(
    config: &Value,
    from: OffsetDateTime,
) -> Result<OffsetDateTime, CronError> {
    let cron_expr = config
        .get("cron")
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|s| !s.is_empty())
        .ok_or(CronError::MissingCron)?;

    let parsed = parse_cron_expr(cron_expr)?;

    let tz_str = config
        .get("timezone")
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|s| !s.is_empty())
        .unwrap_or("Asia/Shanghai");

    let tz = jiff::tz::TimeZone::get(tz_str)
        .map_err(|err| CronError::InvalidTimezone(format!("{tz_str}: {err}")))?;

    let mut seconds = from.unix_timestamp().div_euclid(60) * 60 + 60;
    let limit = seconds.saturating_add(366 * 24 * 3600 * 5);
    // Iterate actual instants, not ambiguous civil timestamps. This includes
    // both occurrences of a fall-back hour and skips nonexistent spring times.
    while seconds < limit {
        let ts = jiff::Timestamp::from_second(seconds)
            .map_err(|e| CronError::InvalidTimezone(e.to_string()))?;
        let local = ts.to_zoned(tz.clone());
        let dom = parsed.dom.values.contains(&(local.day() as u32));
        let dow = parsed
            .dow
            .values
            .contains(&weekday_to_cron_dow(local.weekday()));
        let day = if parsed.dom.is_wildcard || parsed.dow.is_wildcard {
            dom && dow
        } else {
            dom || dow
        };
        if parsed.month.values.contains(&(local.month() as u32))
            && day
            && parsed.hour.values.contains(&(local.hour() as u32))
            && parsed.minute.values.contains(&(local.minute() as u32))
        {
            return OffsetDateTime::from_unix_timestamp(seconds)
                .map_err(|e| CronError::InvalidTimezone(e.to_string()));
        }
        seconds += 60;
    }

    Err(CronError::SearchLimitExceeded)
}

pub fn next_run_at_string(config: &Value, from: OffsetDateTime) -> Result<String, CronError> {
    let next = next_schedule_run(config, from)?;
    next.format(&Rfc3339)
        .map_err(|err| CronError::InvalidTimezone(err.to_string()))
}

pub fn validate_cron_config(config: &Value) -> Result<(), CronError> {
    next_schedule_run(config, OffsetDateTime::now_utc()).map(|_| ())
}

fn normalize_named_field(input: &str, name: &str) -> String {
    let mut value = input.trim().to_ascii_uppercase().replace('?', "*");
    let names: &[&str] = match name {
        "month" => &[
            "JAN", "FEB", "MAR", "APR", "MAY", "JUN", "JUL", "AUG", "SEP", "OCT", "NOV", "DEC",
        ],
        "dow" => &["SUN", "MON", "TUE", "WED", "THU", "FRI", "SAT"],
        _ => &[],
    };
    for (i, token) in names.iter().enumerate() {
        value = value.replace(token, &(i + usize::from(name == "month")).to_string());
    }
    value
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    fn parse_time(s: &str) -> OffsetDateTime {
        OffsetDateTime::parse(s, &Rfc3339).expect("parse time")
    }

    #[test]
    fn parse_valid_cron_expressions() {
        let parsed = parse_cron_expr("* * * * *").expect("parse wildcard");
        assert!(parsed.minute.is_wildcard);
        assert_eq!(parsed.minute.values.len(), 60);

        let parsed = parse_cron_expr("*/15 9-17 1,15 * 1-5").expect("parse complex");
        assert_eq!(parsed.minute.values, BTreeSet::from([0, 15, 30, 45]));
        assert_eq!(
            parsed.hour.values,
            BTreeSet::from([9, 10, 11, 12, 13, 14, 15, 16, 17])
        );
        assert_eq!(parsed.dom.values, BTreeSet::from([1, 15]));
        assert_eq!(parsed.dow.values, BTreeSet::from([1, 2, 3, 4, 5]));
    }

    #[test]
    fn parse_invalid_cron_expressions() {
        assert_eq!(
            parse_cron_expr("* * * *").unwrap_err(),
            CronError::InvalidFieldCount
        );
        assert_eq!(
            parse_cron_expr("* * * * * *").unwrap_err(),
            CronError::InvalidFieldCount
        );
        assert!(matches!(
            parse_cron_expr("60 * * * *"),
            Err(CronError::InvalidField {
                field: "minute",
                ..
            })
        ));
        assert!(matches!(
            parse_cron_expr("* 24 * * *"),
            Err(CronError::InvalidField { field: "hour", .. })
        ));
        assert!(matches!(
            parse_cron_expr("* * 32 * *"),
            Err(CronError::InvalidField { field: "dom", .. })
        ));
        assert!(matches!(
            parse_cron_expr("* * * 13 *"),
            Err(CronError::InvalidField { field: "month", .. })
        ));
        assert!(matches!(
            parse_cron_expr("* * * * 8"),
            Err(CronError::InvalidField { field: "dow", .. })
        ));
    }

    #[test]
    fn next_schedule_run_calculation_with_timezone() {
        // Daily at 09:30 Asia/Shanghai (= 01:30 UTC)
        let config = json!({
            "cron": "30 9 * * *",
            "timezone": "Asia/Shanghai",
        });
        let base = parse_time("2026-09-08T00:00:00Z");
        let next = next_schedule_run(&config, base).expect("next run");
        assert_eq!(next, parse_time("2026-09-08T01:30:00Z"));

        // If base is after 01:30 UTC on 2026-09-08, next should be 2026-09-09 01:30:00 UTC
        let base_after = parse_time("2026-09-08T02:00:00Z");
        let next_after = next_schedule_run(&config, base_after).expect("next run after");
        assert_eq!(next_after, parse_time("2026-09-09T01:30:00Z"));
    }

    #[test]
    fn next_schedule_run_rfc3339_string() {
        let config = json!({
            "cron": "0 0 1 1 *",
            "timezone": "UTC",
        });
        let base = parse_time("2026-05-01T12:00:00Z");
        let s = next_run_at_string(&config, base).expect("next run string");
        assert_eq!(s, "2027-01-01T00:00:00Z");
    }
}
