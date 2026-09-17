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
    // Go runs `normalizeOpenDMap` before the protocol-specific projection, so
    // a raw OpenD row that carries `{market: "QotMarket_US_Security", code}`
    // already has `instrumentId`/`market`/`quoteMarket`/`tradeMarket` when
    // `researchEntrySecurity` inspects it. Without this step the projection is
    // a no-op on every real wire payload.
    let normalized = normalize_open_d_value(payload);
    let Some(object) = normalized.as_object() else {
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

/// Go `normalizeOpenDValue`: recursively rewrites OpenD enum text and turns
/// every `{market, code}` security object into the broker-neutral identity the
/// projection expects.
fn normalize_open_d_value(value: &Value) -> Value {
    match value {
        Value::Object(object) => {
            let mut result = Map::with_capacity(object.len());
            for (key, item) in object {
                result.insert(key.clone(), normalize_open_d_value(item));
            }
            match normalize_open_d_security(&result) {
                Some(security) => Value::Object(security),
                None => Value::Object(result),
            }
        }
        Value::Array(values) => Value::Array(values.iter().map(normalize_open_d_value).collect()),
        Value::String(text) => Value::String(normalize_open_d_enum(text)),
        other => other.clone(),
    }
}

/// Prefixes Go strips from OpenD enum text (`QotMarket_`, `SecurityType_`, ...).
const OPEN_D_ENUM_PREFIXES: &[&str] = &[
    "QotMarket_",
    "SecurityType_",
    "OptionType_",
    "IndexOptionType_",
    "ExpirationCycle_",
    "EC_",
    "PredSide_",
    "TrdSide_",
    "OrderStatus_",
    "TrdEnv_",
    "TrdMarket_",
    "KLType_",
    "RehabType_",
];

fn normalize_open_d_enum(value: &str) -> String {
    for prefix in OPEN_D_ENUM_PREFIXES {
        let Some(remainder) = value.strip_prefix(prefix) else {
            continue;
        };
        // `EC_<group>_<value>` drops the group segment as well.
        let remainder = if *prefix == "EC_" {
            remainder
                .split_once('_')
                .map(|(_, rest)| rest)
                .unwrap_or(remainder)
        } else {
            remainder
        };
        return remainder.to_ascii_lowercase();
    }
    value.to_owned()
}

/// Go `normalizeOpenDSecurity`. A `code` plus a resolvable market (enum text or
/// the numeric `Qot_Common.QotMarket` code) yields the public market label and
/// the canonical `<MARKET>.<CODE>` instrument id. Anything else is left alone.
fn normalize_open_d_security(value: &Map<String, Value>) -> Option<Map<String, Value>> {
    let code = value.get("code").and_then(Value::as_str)?.trim();
    if code.is_empty() {
        return None;
    }
    let raw_market = match value.get("market") {
        Some(Value::String(text)) => text.trim().to_ascii_lowercase(),
        Some(Value::Number(number)) => number
            .as_i64()
            .and_then(qot_market_label)
            .map(|label| label.to_ascii_lowercase())
            .unwrap_or_default(),
        _ => String::new(),
    };
    if raw_market.is_empty() {
        return None;
    }
    let (public_market, product_class) = if raw_market.contains("future") {
        ("HK", Some("future"))
    } else if raw_market.contains("event") || raw_market.contains("prediction") {
        ("US", Some("event_contract"))
    } else if raw_market.contains("us") {
        ("US", None)
    } else if raw_market.contains("hk") {
        ("HK", None)
    } else if raw_market.contains("sh") {
        ("SH", None)
    } else if raw_market.contains("sz") {
        ("SZ", None)
    } else {
        return None;
    };
    let mut result = value.clone();
    result.insert("market".to_owned(), Value::String(public_market.to_owned()));
    result.insert(
        "quoteMarket".to_owned(),
        Value::String(public_market.to_owned()),
    );
    result.insert(
        "tradeMarket".to_owned(),
        Value::String(public_market.to_owned()),
    );
    result.insert(
        "instrumentId".to_owned(),
        Value::String(format!(
            "{public_market}.{}",
            code.trim().to_ascii_uppercase()
        )),
    );
    if let Some(product_class) = product_class {
        result.insert(
            "productClass".to_owned(),
            Value::String(product_class.to_owned()),
        );
    }
    Some(result)
}

/// Public market label for a numeric `Qot_Common.QotMarket` code.
fn qot_market_label(value: i64) -> Option<&'static str> {
    match value {
        1 => Some("HK"),
        11 => Some("US"),
        21 => Some("SH"),
        22 => Some("SZ"),
        31 => Some("SG"),
        41 => Some("JP"),
        51 => Some("AU"),
        61 => Some("MY"),
        71 => Some("CA"),
        _ => None,
    }
}

/// OpenD response envelope derived from a normalized payload, mirroring Go
/// `payloadEntries` + `setPagination` (`adapter_advanced_helpers.go` /
/// `adapter_prediction_normalization.go`).
///
/// The entry list is the first list-of-objects key in ascending key order; the
/// remaining keys become metadata. Pagination then follows Go's explicit rule:
/// the cursor defaults to `nextPage`/`nextKey`, an explicit boolean `hasMore`
/// overrides the derived value, and a `hasMore == false` clears the cursor.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ResearchPayloadEnvelope {
    pub entries: Vec<Value>,
    pub metadata: Map<String, Value>,
    pub total: usize,
    pub has_more: bool,
    pub next_cursor: String,
}

pub fn research_payload_envelope(payload: &Value) -> ResearchPayloadEnvelope {
    let normalized = normalize_open_d_value(payload);
    let (entries, mut metadata) = research_payload_entries(normalized.as_object());
    let object = normalized.as_object();

    let explicit_has_more = object
        .and_then(|object| object.get("hasMore"))
        .and_then(Value::as_bool);
    let mut next_cursor = object
        .and_then(|object| {
            ["nextPage", "nextKey"]
                .into_iter()
                .map(|key| string_value(object.get(key)))
                .find(|value| !value.is_empty())
        })
        .unwrap_or_default();
    let mut has_more = !next_cursor.is_empty();
    if let Some(explicit) = explicit_has_more {
        has_more = explicit;
    }
    if !has_more {
        next_cursor.clear();
    }

    let total = object
        .and_then(|object| {
            ["total", "totalCount", "allCount"]
                .into_iter()
                .find_map(|key| integer_value(object.get(key)))
        })
        .unwrap_or(entries.len() as i64)
        .max(0) as usize;

    metadata.remove("hasMore");
    metadata.remove("nextPage");
    metadata.remove("nextKey");
    metadata.remove("total");
    metadata.remove("totalCount");
    metadata.remove("allCount");
    ResearchPayloadEnvelope {
        entries,
        metadata,
        total,
        has_more,
        next_cursor,
    }
}

/// Go `payloadEntries`: the first list-of-objects key in ascending key order
/// becomes the entry list and is removed from the metadata; a payload with only
/// pagination keys (or none at all) yields no entries.
fn research_payload_entries(
    payload: Option<&Map<String, Value>>,
) -> (Vec<Value>, Map<String, Value>) {
    let Some(payload) = payload else {
        return (Vec::new(), Map::new());
    };
    let mut metadata = payload.clone();
    let mut keys: Vec<&String> = payload.keys().collect();
    keys.sort();
    let mut empty_list_key: Option<&String> = None;
    for key in keys {
        let Some(values) = payload[key].as_array() else {
            continue;
        };
        let entries: Vec<Value> = values
            .iter()
            .filter(|value| value.is_object())
            .cloned()
            .collect();
        if entries.is_empty() {
            if empty_list_key.is_none() {
                empty_list_key = Some(key);
            }
            continue;
        }
        metadata.remove(key);
        return (entries, metadata);
    }
    if let Some(key) = empty_list_key {
        metadata.remove(key);
        return (Vec::new(), metadata);
    }
    if payload_contains_only_pagination_metadata(payload) || payload.is_empty() {
        return (Vec::new(), metadata);
    }
    // A row-shaped payload is its own single entry.
    (vec![Value::Object(payload.clone())], Map::new())
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

/// Go `integerValue`: JSON numbers and integer-looking text are accepted.
fn integer_value(value: Option<&Value>) -> Option<i64> {
    match value? {
        Value::Number(number) => number.as_i64().or_else(|| {
            let float = number.as_f64()?;
            float.is_finite().then_some(float as i64)
        }),
        Value::String(text) => text.trim().parse::<i64>().ok(),
        _ => None,
    }
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
