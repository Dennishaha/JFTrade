//! OpenD research payload normalization (Go `normalizeResearchProtocolPayload`).
//!
//! The Go adapter added a stable broker-neutral projection to raw OpenD
//! research rows (instrument identity, common ranking/calendar aliases) while
//! retaining every original protocol field. The Rust engine consumes typed
//! readers, but the same canonical fields are part of the public research
//! envelope, so the projection must have exactly one owner here instead of
//! being re-derived per route.

use serde_json::{Map, Value};

use crate::research_params::{ResearchParamsError, research_number_parity};

const PAGINATION_KEYS: &[&str] = &[
    "nextPage",
    "nextKey",
    "hasMore",
    "total",
    "totalCount",
    "allCount",
    "currency",
];

/// Go `isResearchNormalizationProtocol`.
pub fn is_research_normalization_protocol(protocol: &str) -> bool {
    [
        "Qot_GetEarnings",
        "Qot_GetDividend",
        "Qot_GetUSPreMarket",
        "Qot_GetUSAfterHours",
        "Qot_GetUSOvernight",
        "Qot_GetTopMovers",
        "Qot_GetHotList",
        "Qot_GetShortSelling",
        "Qot_GetPeriodChange",
        "Qot_GetHighDividend",
        "Qot_GetHeatMap",
        "Qot_GetRiseFall",
        "Qot_GetInstitution",
        "Qot_GetArk",
        "Qot_GetIndustrial",
    ]
    .iter()
    .any(|prefix| protocol.starts_with(prefix))
        || matches!(
            protocol,
            "Qot_GetEconomicCalendar"
                | "Qot_GetIpoList"
                | "Qot_GetPlateSet"
                | "Qot_GetPlateSecurity"
                | "Qot_GetOwnerPlate"
                | "Qot_GetStaticInfo"
        )
}

/// Go `normalizeResearchProtocolPayload`. Scalar rows are preserved, objects
/// are normalized in place, and pagination-only payloads are returned as-is.
pub fn normalize_research_protocol_payload(protocol: &str, payload: &Value) -> Value {
    if !is_research_normalization_protocol(protocol) {
        return payload.clone();
    }
    let Some(object) = payload.as_object() else {
        return payload.clone();
    };
    if object.is_empty() || payload_contains_only_pagination_metadata(object) {
        return payload.clone();
    }
    let mut result = normalize_research_entry(protocol, object.clone());
    for (key, raw) in result.clone() {
        let Some(values) = raw.as_array() else {
            continue;
        };
        let normalized: Vec<Value> = values
            .iter()
            .map(|value| match value.as_object() {
                Some(entry) => normalize_research_entry(protocol, entry.clone()).into(),
                None => value.clone(),
            })
            .collect();
        result.insert(key, Value::Array(normalized));
    }
    Value::Object(result)
}

fn payload_contains_only_pagination_metadata(payload: &Map<String, Value>) -> bool {
    !payload.is_empty()
        && payload
            .keys()
            .all(|key| PAGINATION_KEYS.contains(&key.as_str()))
}

fn normalize_research_entry(protocol: &str, mut entry: Map<String, Value>) -> Map<String, Value> {
    if protocol == "Qot_GetIpoList" {
        flatten_research_ipo(&mut entry);
    }
    let (security, source, basic) = research_entry_security(&entry);
    if let Some(security) = security {
        let market = string_value(security.get("market")).to_ascii_uppercase();
        let code = string_value(security.get("code")).to_ascii_uppercase();
        entry.insert(
            "instrumentId".to_owned(),
            Value::String(format!("{market}.{code}")),
        );
        entry.insert("market".to_owned(), Value::String(market));
        entry.insert("symbol".to_owned(), Value::String(code));
        if let Some(name) = research_entry_name(&entry, &basic) {
            entry.insert("name".to_owned(), Value::String(name));
        }
        if let Some(product_class) = research_product_class(protocol, &entry, &source, &basic) {
            entry.insert("productClass".to_owned(), Value::String(product_class));
        }
    }
    alias_once(&mut entry, "changeRate", "changeRatio");
    alias_once(&mut entry, "price", "curPrice");
    alias_once(&mut entry, "marketValue", "marketVal");
    alias_once(&mut entry, "dividendYield", "dividendYieldTTM");
    normalize_research_calendar_fields(protocol, &mut entry);
    normalize_research_institution_fields(protocol, &mut entry);
    entry
}

fn alias_once(entry: &mut Map<String, Value>, target: &str, source: &str) {
    if entry.get(target).is_none_or(Value::is_null)
        && let Some(value) = entry.get(source).filter(|value| !value.is_null())
    {
        entry.insert(target.to_owned(), value.clone());
    }
}

fn research_entry_security(
    entry: &Map<String, Value>,
) -> (Option<Map<String, Value>>, String, Map<String, Value>) {
    for key in ["plate", "security"] {
        if let Some(security) = entry.get(key).and_then(Value::as_object)
            && !string_value(security.get("instrumentId")).is_empty()
        {
            return (Some(security.clone()), key.to_owned(), Map::new());
        }
    }
    let basic = entry
        .get("basic")
        .and_then(Value::as_object)
        .cloned()
        .unwrap_or_default();
    if let Some(security) = basic.get("security").and_then(Value::as_object)
        && !string_value(security.get("instrumentId")).is_empty()
    {
        return (Some(security.clone()), "basic.security".to_owned(), basic);
    }
    (None, String::new(), basic)
}

fn research_entry_name(entry: &Map<String, Value>, basic: &Map<String, Value>) -> Option<String> {
    for key in ["name", "plateName", "institutionName"] {
        let value = string_value(entry.get(key));
        if !value.is_empty() {
            return Some(value);
        }
    }
    let name = string_value(basic.get("name"));
    (!name.is_empty()).then_some(name)
}

/// Go `researchSecurityType`: text aliases first, then the numeric
/// `Qot_Common.SecurityType` table.
pub fn research_security_type(value: &Value) -> Option<&'static str> {
    // Go `stringValue` only accepts actual strings; numeric Qot_Common
    // security types must fall through to the numeric table.
    let text = match value {
        Value::String(text) => text.trim().to_ascii_lowercase(),
        _ => String::new(),
    };
    if !text.is_empty() {
        return match text.as_str() {
            "eqty" | "equity" => Some("equity"),
            "trust" | "fund" => Some("fund"),
            "drvt" | "option" => Some("option"),
            "bwrt" | "warrant" => Some("warrant"),
            "index" => Some("index"),
            "plate" => Some("plate"),
            "future" => Some("future"),
            "bond" => Some("bond"),
            "forex" => Some("forex"),
            "crypto" => Some("crypto"),
            _ => None,
        };
    }
    let number = research_number_parity(value)?;
    if number.fract() != 0.0 {
        return None;
    }
    match number as i64 {
        1 => Some("bond"),
        2 | 5 => Some("warrant"),
        3 => Some("equity"),
        4 => Some("fund"),
        6 => Some("index"),
        7 | 9 => Some("plate"),
        8 => Some("option"),
        10 => Some("future"),
        11 => Some("forex"),
        12 => Some("crypto"),
        _ => None,
    }
}

/// Go `researchProductClass` fallbacks.
pub fn research_product_class(
    protocol: &str,
    entry: &Map<String, Value>,
    security_source: &str,
    basic: &Map<String, Value>,
) -> Option<String> {
    let explicit = string_value(entry.get("productClass")).to_lowercase();
    if !explicit.is_empty() {
        return Some(explicit);
    }
    if security_source == "plate"
        || matches!(
            protocol,
            "Qot_GetPlateSet" | "Qot_GetOwnerPlate" | "Qot_GetHeatMapData"
        )
    {
        return Some("plate".to_owned());
    }
    if let Some(sec_type) = research_security_type(basic.get("secType").unwrap_or(&Value::Null)) {
        return Some(sec_type.to_owned());
    }
    if protocol == "Qot_GetStaticInfo" {
        return Some("fund".to_owned());
    }
    Some("equity".to_owned())
}

/// Go `flattenResearchIPO`.
pub fn flatten_research_ipo(entry: &mut Map<String, Value>) {
    let basic = entry
        .get("basic")
        .and_then(Value::as_object)
        .cloned()
        .unwrap_or_default();
    for key in ["name", "listTime", "listTimestamp"] {
        copy_field(&basic, entry, key);
    }
    for key in ["cnExData", "hkExData", "usExData"] {
        if let Some(extra) = entry.get(key).and_then(Value::as_object).cloned() {
            for (field, value) in extra {
                if entry.get(&field).is_none_or(Value::is_null) {
                    entry.insert(field, value);
                }
            }
        }
    }
    for (target, source) in [
        ("issuePrice", "ipoPrice"),
        ("issuePrice", "listPrice"),
        ("issuePriceMin", "ipoPriceMin"),
        ("issuePriceMax", "ipoPriceMax"),
        ("listingDate", "listTime"),
        ("issueVolume", "issueSize"),
    ] {
        if entry.get(target).is_none_or(Value::is_null)
            && let Some(value) = entry.get(source).filter(|value| !value.is_null())
        {
            entry.insert(target.to_owned(), value.clone());
        }
    }
}

fn copy_field(source: &Map<String, Value>, target: &mut Map<String, Value>, key: &str) {
    if target.get(key).is_none_or(Value::is_null)
        && let Some(value) = source.get(key).filter(|value| !value.is_null())
    {
        target.insert(key.to_owned(), value.clone());
    }
}

/// Go `normalizeResearchCalendarFields` (the protocols the parity checklist
/// asserts on).
pub fn normalize_research_calendar_fields(protocol: &str, entry: &mut Map<String, Value>) {
    match protocol {
        "Qot_GetEarningsCalendar" => {
            let date = string_value(entry.get("earningsDate"));
            let timestamp = entry.get("earningsTimestamp").cloned();
            set_research_event_fields(entry, "earnings", &date, timestamp.as_ref());
        }
        "Qot_GetEconomicCalendar" => {
            let timestamp = entry.get("timestamp").cloned();
            set_research_event_fields(entry, "economic", "", timestamp.as_ref());
            alias_once(entry, "region", "country");
            alias_once(entry, "importance", "star");
            alias_once(entry, "previousValue", "previous");
            alias_once(entry, "forecastValue", "consensus");
            alias_once(entry, "actualValue", "actual");
        }
        "Qot_GetDividendCalendar" => {
            let date = string_value(entry.get("exDate"));
            set_research_event_fields(entry, "dividend", &date, None);
        }
        "Qot_GetIpoList" => {
            let date = string_value(entry.get("listingDate"));
            let timestamp = entry.get("listTimestamp").cloned();
            set_research_event_fields(entry, "ipo", &date, timestamp.as_ref());
        }
        _ => {}
    }
}

fn set_research_event_fields(
    entry: &mut Map<String, Value>,
    calendar_type: &str,
    explicit_date: &str,
    timestamp_value: Option<&Value>,
) {
    entry.insert(
        "calendarType".to_owned(),
        Value::String(calendar_type.to_owned()),
    );
    if !explicit_date.is_empty() {
        entry.insert(
            "eventDate".to_owned(),
            Value::String(explicit_date.to_owned()),
        );
        if entry.get("date").is_none_or(Value::is_null) {
            entry.insert("date".to_owned(), Value::String(explicit_date.to_owned()));
        }
    }
    let Some(timestamp) = timestamp_value.and_then(research_number_parity) else {
        return;
    };
    let seconds = if timestamp.abs() >= 1e12 {
        timestamp / 1000.0
    } else {
        timestamp
    };
    entry.insert("eventTimestamp".to_owned(), Value::from(seconds));
    if let Ok(moment) = time::OffsetDateTime::from_unix_timestamp(seconds as i64) {
        let formatted = moment
            .format(&time::format_description::well_known::Rfc3339)
            .unwrap_or_default();
        entry.insert("eventTime".to_owned(), Value::String(formatted));
        if string_value(entry.get("eventDate")).is_empty() {
            entry.insert(
                "eventDate".to_owned(),
                Value::String(moment.date().to_string()),
            );
        }
    }
}

/// Go `normalizeResearchInstitutionFields` (holding-change aliases).
pub fn normalize_research_institution_fields(protocol: &str, entry: &mut Map<String, Value>) {
    if !protocol.starts_with("Qot_GetInstitution") && !protocol.starts_with("Qot_GetArk") {
        return;
    }
    alias_once(entry, "marketValueChange", "positionValueChange");
    alias_once(entry, "holdingCountChange", "positionCountChange");
    alias_once(entry, "marketValue", "positionValue");
    alias_once(entry, "holdingCount", "positionCount");
}

/// Local pagination plan for PlateSet/PlateSecurity/StaticInfo, mirroring Go
/// `applyResearchLocalPagination` (clamped offset, `local:N` cursor).
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct LocalResearchPage {
    pub entries: Vec<Value>,
    pub total: usize,
    pub has_more: bool,
    pub next_cursor: String,
}

pub fn apply_research_local_pagination(
    entries: &[Value],
    page_size: i64,
    cursor: &str,
    protocol: &str,
) -> Result<LocalResearchPage, ResearchParamsError> {
    let uses_local_pagination = matches!(
        protocol,
        "Qot_GetPlateSet" | "Qot_GetPlateSecurity" | "Qot_GetStaticInfo"
    );
    let total = entries.len();
    if !uses_local_pagination || page_size <= 0 {
        return Ok(LocalResearchPage {
            entries: entries.to_vec(),
            total,
            has_more: false,
            next_cursor: String::new(),
        });
    }
    let offset = if cursor.trim().is_empty() {
        0
    } else {
        let value = cursor.trim();
        let Some(raw) = value.strip_prefix("local:") else {
            return Err(ResearchParamsError::Invalid(format!(
                "futu: invalid local research cursor {:?}",
                value
            )));
        };
        raw.parse::<usize>().map_err(|_| {
            ResearchParamsError::Invalid(format!("futu: invalid local research cursor {value:?}"))
        })?
    };
    let offset = offset.min(total);
    let end = (offset + page_size as usize).min(total);
    let has_more = end < total;
    Ok(LocalResearchPage {
        entries: entries[offset..end].to_vec(),
        total,
        has_more,
        next_cursor: if has_more {
            format!("local:{end}")
        } else {
            String::new()
        },
    })
}

fn string_value(value: Option<&Value>) -> String {
    match value {
        Some(Value::String(value)) => value.trim().to_owned(),
        Some(Value::Number(value)) => value.to_string(),
        Some(Value::Bool(value)) => value.to_string(),
        _ => String::new(),
    }
}

#[cfg(test)]
#[path = "research_normalization_tests.rs"]
mod tests;
