//! Market threshold trigger evaluator.
//!
//! Supports `above`, `below`, `cross_up`, `cross_down` edge transitions,
//! cooldown enforcement, and state tracking for market quotes.

use std::collections::BTreeSet;

use serde_json::{Map, Value, json};
use time::OffsetDateTime;
use time::format_description::well_known::Rfc3339;

pub const DEFAULT_MARKET_THRESHOLD_COOLDOWN_SEC: i64 = 900;

#[derive(Clone, Debug, PartialEq)]
pub struct ThresholdMatch {
    pub instrument_id: String,
    pub event: Value,
    pub threshold: Value,
}

pub fn evaluate_market_threshold_trigger(
    trigger_config: &mut Value,
    events: &[Value],
    now: OffsetDateTime,
) -> (Vec<ThresholdMatch>, bool) {
    let instruments = config_instrument_ids(trigger_config);
    if instruments.is_empty() {
        return (Vec::new(), false);
    }

    let path = trigger_config
        .get("path")
        .or_else(|| trigger_config.get("snapshotPath"))
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|s| !s.is_empty())
        .unwrap_or("snapshot.price")
        .to_owned();

    let Some(threshold) = config_float(trigger_config, "value") else {
        return (Vec::new(), false);
    };

    let edge = trigger_config
        .get("edge")
        .and_then(Value::as_str)
        .map(str::trim)
        .map(str::to_ascii_lowercase)
        .filter(|s| !s.is_empty())
        .unwrap_or_else(|| "cross_up".to_owned());

    let operator = trigger_config
        .get("operator")
        .and_then(Value::as_str)
        .map(str::trim)
        .unwrap_or_default()
        .to_owned();

    let cooldown_sec = trigger_config
        .get("cooldownSec")
        .and_then(any_f64)
        .map(|v| v.round() as i64)
        .unwrap_or(DEFAULT_MARKET_THRESHOLD_COOLDOWN_SEC);

    let state = ensure_config_state(trigger_config);
    let mut changed = false;
    let mut matches = Vec::new();

    for event in events {
        let Some(instrument_id) = event_instrument_id(event) else {
            continue;
        };
        if !instruments.contains(&instrument_id) {
            continue;
        }

        let Some(current) = extract_numeric(event, &path) else {
            continue;
        };

        let last_values = ensure_state_map(state, "lastValues");
        let previous = last_values.get(&instrument_id).and_then(any_f64);
        let fired = threshold_fired(&edge, &operator, previous, current, threshold);

        last_values.insert(instrument_id.clone(), json!(current));
        changed = true;

        let last_triggered_at = ensure_state_map(state, "lastTriggeredAt");
        let last_triggered_str = last_triggered_at
            .get(&instrument_id)
            .and_then(Value::as_str);

        if !fired || !cooldown_allows(last_triggered_str, now, cooldown_sec) {
            continue;
        }

        let now_str = now.format(&Rfc3339).unwrap_or_default();
        last_triggered_at.insert(instrument_id.clone(), json!(now_str));

        let threshold_info = json!({
            "instrumentId": instrument_id,
            "path": path,
            "edge": edge,
            "operator": operator,
            "value": threshold,
            "previous": previous,
            "current": current,
        });

        let mut matched_event = event.clone();
        if let Some(obj) = matched_event.as_object_mut() {
            obj.insert("threshold".to_owned(), threshold_info.clone());
        }

        matches.push(ThresholdMatch {
            instrument_id,
            event: matched_event,
            threshold: threshold_info,
        });
    }

    (matches, changed)
}

fn threshold_fired(
    edge: &str,
    operator: &str,
    previous: Option<f64>,
    current: f64,
    threshold: f64,
) -> bool {
    match edge {
        "cross_down" => match previous {
            Some(prev) => prev >= threshold && current < threshold,
            None => false,
        },
        "above" => compare_threshold(
            if operator.is_empty() { ">" } else { operator },
            current,
            threshold,
        ),
        "below" => compare_threshold(
            if operator.is_empty() { "<" } else { operator },
            current,
            threshold,
        ),
        _ => match previous {
            Some(prev) => prev <= threshold && current > threshold,
            None => false,
        },
    }
}

fn compare_threshold(operator: &str, current: f64, threshold: f64) -> bool {
    match operator.trim() {
        ">=" => current >= threshold,
        "<" => current < threshold,
        "<=" => current <= threshold,
        _ => current > threshold,
    }
}

fn cooldown_allows(
    last_triggered_at: Option<&str>,
    now: OffsetDateTime,
    cooldown_sec: i64,
) -> bool {
    if cooldown_sec <= 0 {
        return true;
    }
    let Some(ts_str) = last_triggered_at.filter(|s| !s.trim().is_empty()) else {
        return true;
    };
    if let Ok(prev_time) = OffsetDateTime::parse(ts_str, &Rfc3339) {
        (now - prev_time).whole_seconds() >= cooldown_sec
    } else {
        true
    }
}

pub fn event_instrument_id(event: &Value) -> Option<String> {
    for key in ["entityId", "instrumentId"] {
        if let Some(s) = event.get(key).and_then(Value::as_str) {
            let trimmed = s.trim();
            if !trimmed.is_empty() && trimmed != "<nil>" {
                return Some(trimmed.to_ascii_uppercase());
            }
        }
    }
    if let Some(inst) = event.get("instrument").and_then(Value::as_object)
        && let Some(s) = inst.get("instrumentId").and_then(Value::as_str)
    {
        let trimmed = s.trim();
        if !trimmed.is_empty() && trimmed != "<nil>" {
            return Some(trimmed.to_ascii_uppercase());
        }
    }
    if let Some(payload) = event.get("payload") {
        return event_instrument_id(payload);
    }
    None
}

fn extract_numeric(event: &Value, path: &str) -> Option<f64> {
    numeric_at_path(event, path)
        .or_else(|| event.get("payload").and_then(|p| numeric_at_path(p, path)))
        .or_else(|| {
            if path == "snapshot.price" || path == "lastPrice" || path == "price" {
                numeric_at_path(event, "price")
                    .or_else(|| {
                        event
                            .get("payload")
                            .and_then(|p| numeric_at_path(p, "price"))
                    })
                    .or_else(|| numeric_at_path(event, "lastPrice"))
                    .or_else(|| {
                        event
                            .get("payload")
                            .and_then(|p| numeric_at_path(p, "lastPrice"))
                    })
                    .or_else(|| numeric_at_path(event, "currentPrice"))
                    .or_else(|| {
                        event
                            .get("payload")
                            .and_then(|p| numeric_at_path(p, "currentPrice"))
                    })
            } else {
                None
            }
        })
}

pub fn numeric_at_path(root: &Value, path: &str) -> Option<f64> {
    let mut current = root;
    for part in path.split('.') {
        let trimmed = part.trim();
        if trimmed.is_empty() {
            continue;
        }
        current = current.get(trimmed)?;
    }
    any_f64(current)
}

pub fn any_f64(value: &Value) -> Option<f64> {
    match value {
        Value::Number(num) => num.as_f64(),
        Value::String(s) => s.trim().parse::<f64>().ok(),
        _ => None,
    }
}

fn config_float(config: &Value, key: &str) -> Option<f64> {
    config.get(key).and_then(any_f64)
}

fn config_instrument_ids(config: &Value) -> BTreeSet<String> {
    let mut set = BTreeSet::new();
    let Some(raw) = config.get("instrumentIds") else {
        return set;
    };
    match raw {
        Value::Array(items) => {
            for item in items {
                if let Some(s) = item.as_str() {
                    let trimmed = s.trim();
                    if !trimmed.is_empty() {
                        set.insert(trimmed.to_ascii_uppercase());
                    }
                }
            }
        }
        Value::String(s) => {
            for part in s.split(',') {
                let trimmed = part.trim();
                if !trimmed.is_empty() {
                    set.insert(trimmed.to_ascii_uppercase());
                }
            }
        }
        _ => {}
    }
    set
}

fn ensure_config_state(config: &mut Value) -> &mut Map<String, Value> {
    if !config.is_object() {
        *config = Value::Object(Map::new());
    }
    let obj = config.as_object_mut().expect("object");
    if !obj.contains_key("state") || !obj["state"].is_object() {
        obj.insert("state".to_owned(), Value::Object(Map::new()));
    }
    obj.get_mut("state")
        .and_then(Value::as_object_mut)
        .expect("state object")
}

fn ensure_state_map<'a>(
    state: &'a mut Map<String, Value>,
    key: &str,
) -> &'a mut Map<String, Value> {
    if !state.contains_key(key) || !state[key].is_object() {
        state.insert(key.to_owned(), Value::Object(Map::new()));
    }
    state
        .get_mut(key)
        .and_then(Value::as_object_mut)
        .expect("state sub-map")
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    fn parse_time(s: &str) -> OffsetDateTime {
        OffsetDateTime::parse(s, &Rfc3339).expect("parse time")
    }

    #[test]
    fn evaluate_cross_up_transitions() {
        let mut config = json!({
            "instrumentIds": ["US.AAPL"],
            "snapshotPath": "snapshot.price",
            "value": 150.0,
            "edge": "cross_up",
            "cooldownSec": 60,
        });

        let now = parse_time("2026-09-08T12:00:00Z");

        // Tick 1: Price 148.0 -> establishes previous value, should not fire yet
        let event1 = json!({
            "entityId": "US.AAPL",
            "snapshot": { "price": 148.0 }
        });
        let (matches, changed) = evaluate_market_threshold_trigger(&mut config, &[event1], now);
        assert!(matches.is_empty());
        assert!(changed);

        // Tick 2: Price crosses up to 152.0 -> should fire
        let now2 = parse_time("2026-09-08T12:00:30Z");
        let event2 = json!({
            "entityId": "US.AAPL",
            "snapshot": { "price": 152.0 }
        });
        let (matches2, changed2) = evaluate_market_threshold_trigger(&mut config, &[event2], now2);
        assert_eq!(matches2.len(), 1);
        assert!(changed2);
        assert_eq!(matches2[0].instrument_id, "US.AAPL");
        assert_eq!(matches2[0].threshold["current"], 152.0);
        assert_eq!(matches2[0].threshold["previous"], 148.0);

        // Tick 3: Price still above (153.0) but within cooldown (now2 + 10s) -> should NOT fire
        let now3 = parse_time("2026-09-08T12:00:40Z");
        let event3 = json!({
            "entityId": "US.AAPL",
            "snapshot": { "price": 153.0 }
        });
        let (matches3, _) = evaluate_market_threshold_trigger(&mut config, &[event3], now3);
        assert!(matches3.is_empty());
    }

    #[test]
    fn evaluate_cross_down_transitions() {
        let mut config = json!({
            "instrumentIds": ["US.TSLA"],
            "value": 200.0,
            "edge": "cross_down",
            "cooldownSec": 60,
        });
        let now = parse_time("2026-09-08T12:00:00Z");

        // Tick 1: 205.0 -> baseline
        let event1 = json!({
            "instrument": { "instrumentId": "US.TSLA" },
            "snapshot": { "price": 205.0 }
        });
        let (matches, _) = evaluate_market_threshold_trigger(&mut config, &[event1], now);
        assert!(matches.is_empty());

        // Tick 2: 198.0 -> crosses down
        let now2 = parse_time("2026-09-08T12:01:00Z");
        let event2 = json!({
            "instrument": { "instrumentId": "US.TSLA" },
            "snapshot": { "price": 198.0 }
        });
        let (matches2, _) = evaluate_market_threshold_trigger(&mut config, &[event2], now2);
        assert_eq!(matches2.len(), 1);
        assert_eq!(matches2[0].threshold["current"], 198.0);
        assert_eq!(matches2[0].threshold["previous"], 205.0);
    }

    #[test]
    fn evaluate_above_and_below_levels() {
        let mut config = json!({
            "instrumentIds": ["HK.00700"],
            "value": 300.0,
            "edge": "above",
            "operator": ">=",
            "cooldownSec": 60,
        });
        let now = parse_time("2026-09-08T12:00:00Z");

        // 300.0 >= 300.0 -> fires immediately
        let event1 = json!({
            "entityId": "HK.00700",
            "price": 300.0
        });
        let (matches, _) = evaluate_market_threshold_trigger(&mut config, &[event1], now);
        assert_eq!(matches.len(), 1);

        // Second tick within cooldown -> suppressed
        let now2 = parse_time("2026-09-08T12:00:10Z");
        let event2 = json!({
            "entityId": "HK.00700",
            "price": 310.0
        });
        let (matches2, _) = evaluate_market_threshold_trigger(&mut config, &[event2], now2);
        assert!(matches2.is_empty());

        // Third tick after cooldown (now + 70s) -> fires again
        let now3 = parse_time("2026-09-08T12:01:15Z");
        let event3 = json!({
            "entityId": "HK.00700",
            "price": 315.0
        });
        let (matches3, _) = evaluate_market_threshold_trigger(&mut config, &[event3], now3);
        assert_eq!(matches3.len(), 1);
    }
}
