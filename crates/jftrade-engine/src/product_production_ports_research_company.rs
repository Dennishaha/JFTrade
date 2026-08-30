//! Company-research helper adapter.
//!
//! The market-data sidecar deliberately keeps its private HTTP wire model in
//! snake_case.  This module is the single boundary where those responses are
//! validated and projected to the public broker FeatureResult/camelCase wire
//! contract.

use serde_json::{Map, Value, json};

use crate::product::ResearchReadSnapshotError;
use crate::product::product_query::QueryMap;

pub(super) type ResearchHelperRequest = (&'static str, String, String, Vec<(&'static str, String)>);

pub(super) fn research_helper_request(
    path: &str,
    query: &str,
) -> Result<ResearchHelperRequest, ResearchReadSnapshotError> {
    let (operation, suffix) = if let Some(value) = path.strip_prefix("/api/v1/research/financials/")
    {
        ("financials", value)
    } else if let Some(value) = path.strip_prefix("/api/v1/research/analyst/") {
        ("analyst", value)
    } else if let Some(value) = path.strip_prefix("/api/v1/research/ownership/") {
        ("ownership", value)
    } else if let Some(value) = path.strip_prefix("/api/v1/research/corporate-actions/") {
        ("corporate-actions", value)
    } else if let Some(value) = path.strip_prefix("/api/v1/research/instruments/") {
        ("profile", value)
    } else {
        return Err(capability("research", path));
    };
    let instrument = suffix.trim();
    let (raw_market, raw_symbol) = instrument
        .split_once('.')
        .ok_or_else(|| invalid("instrument must use MARKET.SYMBOL form"))?;
    let market = raw_market.trim();
    let symbol = raw_symbol.trim();
    if market.is_empty()
        || symbol.is_empty()
        || market.contains('/')
        || symbol.contains('/')
        || market.chars().any(char::is_whitespace)
        || symbol.chars().any(char::is_whitespace)
    {
        return Err(invalid("research instrument path is invalid"));
    }
    let market = market.to_ascii_uppercase();
    let symbol = canonical_symbol(&market, symbol);
    let query_map = QueryMap::parse(query).map_err(|_| invalid("invalid URL escape"))?;
    if let Some(requested) = query_map
        .get_first("operation")
        .map(str::trim)
        .filter(|value| !value.is_empty())
    {
        let accepted = match operation {
            "profile" => ["profile"].as_slice(),
            "financials" => ["statements"].as_slice(),
            "analyst" => ["consensus"].as_slice(),
            "ownership" => ["overview"].as_slice(),
            "corporate-actions" => ["dividends"].as_slice(),
            _ => [].as_slice(),
        };
        if !accepted
            .iter()
            .any(|value| value.eq_ignore_ascii_case(requested))
        {
            return Err(capability(operation, requested));
        }
    }
    let mut extra_query = Vec::new();
    if operation == "financials" {
        if let Some(statement) = query_map
            .get_first("statement")
            .map(str::trim)
            .filter(|value| !value.is_empty())
        {
            extra_query.push(("statement", statement.to_owned()));
        }
    } else if operation == "corporate-actions" {
        for key in ["from", "to"] {
            if let Some(value) = query_map
                .get_first(key)
                .map(str::trim)
                .filter(|value| !value.is_empty())
            {
                extra_query.push((key, value.to_owned()));
            }
        }
    }
    Ok((operation, market, symbol, extra_query))
}

/// Validate a helper payload and convert it to the public FeatureResult shape.
pub(super) fn project_research_payload(
    operation: &str,
    payload: Value,
    requested_market: &str,
    requested_symbol: &str,
    provider: &str,
    expected_statement: Option<&str>,
) -> Result<Value, ResearchReadSnapshotError> {
    let payload = normalize_aliases(payload);
    let object = payload
        .as_object()
        .ok_or_else(|| bad_gateway("market-data helper returned a non-object research response"))?;
    let instrument_id = required_text(object, "instrument_id")?;
    let requested_symbol = canonical_symbol(requested_market, requested_symbol);
    let expected_id = format!("{}.{}", requested_market, requested_symbol).to_ascii_uppercase();
    if !instrument_id.eq_ignore_ascii_case(&expected_id) {
        return Err(bad_gateway(
            "research response instrument_id does not match request",
        ));
    }
    if operation == "profile" {
        let market = required_text(object, "market")?;
        let symbol = required_text(object, "symbol")?;
        if !market.eq_ignore_ascii_case(requested_market)
            || !symbol.eq_ignore_ascii_case(&requested_symbol)
        {
            return Err(bad_gateway("profile identity does not match request"));
        }
    }
    let (entries, metadata, source) = match operation {
        "profile" => project_profile(object)?,
        "financials" => project_financials(object, expected_statement)?,
        "analyst" => project_analyst(object)?,
        "ownership" => project_ownership(object)?,
        "corporate-actions" => {
            project_corporate_actions(object, requested_market, &requested_symbol)?
        }
        _ => return Err(unavailable("unsupported helper research operation")),
    };
    let feature_id = match operation {
        "profile" => "research.instrument",
        "financials" => "research.financials",
        "analyst" => "research.analyst",
        "ownership" => "research.ownership",
        "corporate-actions" => "research.corporate_actions",
        _ => unreachable!(),
    };
    let source = if source.starts_with("market-data-") {
        let suffix = match operation {
            "profile" => "profile",
            "financials" => "financials",
            "analyst" => "analyst",
            "ownership" => "ownership",
            "corporate-actions" => "actions",
            _ => unreachable!(),
        };
        format!("{provider}-{suffix}")
    } else {
        source
    };
    let as_of = crate::product::product_production_ports::provider_now_rfc3339();
    let total = entries.len();
    let mut result = json!({
        "provider": {
            "brokerId": provider,
            "featureId": feature_id,
            "capability": "available",
            "selectionReason": "embedded-market-data-provider",
            "resolvedAt": as_of,
            "asOf": as_of,
        },
        "resolvedInstrument": {
            "instrumentId": instrument_id.to_ascii_uppercase(),
            "code": requested_symbol,
            "productClass": "unknown",
            "marketSegment": "securities",
            "quoteMarket": requested_market.to_ascii_uppercase(),
            "tradeMarket": requested_market.to_ascii_uppercase(),
            "quantityMode": "units",
        },
        "asOf": as_of,
        "entries": entries,
        "hasMore": false,
        "total": total,
        "metadata": metadata,
    });
    if let Some(metadata) = result.get_mut("metadata").and_then(Value::as_object_mut) {
        metadata.insert("source".to_owned(), json!(source));
    }
    // Older Rust adapter tests and a few clients read the selected statement
    // from the top-level document.  Keep this harmless compatibility field in
    // addition to the canonical FeatureResult metadata projection.
    if operation == "financials" {
        // Preserve the helper's selected statement at the top level for
        // existing Rust callers; the canonical Go projection still consumes
        // metadata.structureList and the period entries.
        if let Some(statement) = object.get("statement").cloned() {
            result["statement"] = statement;
        }
    }
    Ok(result)
}

fn normalize_aliases(value: Value) -> Value {
    match value {
        Value::Array(values) => Value::Array(values.into_iter().map(normalize_aliases).collect()),
        Value::Object(object) => {
            let mut normalized = Map::new();
            for (key, value) in object {
                let key = match key.as_str() {
                    "instrumentId" => "instrument_id",
                    "fieldId" => "field_id",
                    "displayName" => "display_name",
                    "periodText" => "period_text",
                    "analystCount" => "analyst_count",
                    "targetPrice" => "target_price",
                    "strongBuy" => "strong_buy",
                    "staticDate" => "static_date",
                    "holderPct" => "holder_pct",
                    "exDate" => "ex_date",
                    "updateTime" => "update_time",
                    _ => key.as_str(),
                };
                normalized.insert(key.to_owned(), normalize_aliases(value));
            }
            Value::Object(normalized)
        }
        other => other,
    }
}

fn canonical_symbol(market: &str, symbol: &str) -> String {
    let symbol = symbol.to_ascii_uppercase();
    if market == "HK" && symbol.chars().all(|character| character.is_ascii_digit()) {
        return format!("{symbol:0>5}");
    }
    symbol
}

fn project_profile(
    object: &Map<String, Value>,
) -> Result<(Vec<Value>, Map<String, Value>, String), ResearchReadSnapshotError> {
    let market = required_text(object, "market")?;
    let symbol = required_text(object, "symbol")?;
    let groups = required_array(object, "groups")?;
    let mut entries = Vec::new();
    for group in groups {
        let group = object_entry(group, "profile group")?;
        let title = required_text(group, "title")?;
        entries.push(json!({"fieldType": "title", "name": title}));
        for field in required_array(group, "fields")? {
            let field = object_entry(field, "profile field")?;
            let name = required_text(field, "name")?;
            let value = required_text(field, "value")?;
            entries.push(json!({"fieldType": "text", "name": name, "value": value}));
        }
    }
    // Keep identity fields validated even though the public envelope carries
    // them in resolvedInstrument.
    if market.trim().is_empty() || symbol.trim().is_empty() {
        return Err(bad_gateway("profile response identity is empty"));
    }
    // Validate the optional currency field but keep it out of the public
    // FeatureResult metadata; Go's company-profile projection only exposes
    // grouped fields and the common provider envelope.
    let _ = optional_text(object, "currency")?;
    Ok((entries, Map::new(), "market-data-profile".to_owned()))
}

fn project_financials(
    object: &Map<String, Value>,
    expected_statement: Option<&str>,
) -> Result<(Vec<Value>, Map<String, Value>, String), ResearchReadSnapshotError> {
    let statement = required_text(object, "statement")?;
    if !matches!(
        statement.to_ascii_lowercase().as_str(),
        "income" | "balance" | "cashflow"
    ) {
        return Err(bad_gateway("financial statement kind is invalid"));
    }
    if expected_statement.is_some_and(|expected| !statement.eq_ignore_ascii_case(expected)) {
        return Err(bad_gateway("financial statement does not match request"));
    }
    let fields = required_array(object, "fields")?;
    let periods = required_array(object, "periods")?;
    let mut field_ids = Vec::with_capacity(fields.len());
    let mut structure = Vec::with_capacity(fields.len());
    for field in fields {
        let field = object_entry(field, "financial field")?;
        let field_id = required_text(field, "field_id")?.to_owned();
        let display_name = required_text(field, "display_name")?;
        field_ids.push(field_id.clone());
        structure.push(json!({"fieldId": field_id, "displayName": display_name}));
    }
    let mut entries = Vec::with_capacity(periods.len());
    for period in periods {
        let period = object_entry(period, "financial period")?;
        let period_text = required_text(period, "period_text")?;
        let values = period
            .get("values")
            .and_then(Value::as_object)
            .ok_or_else(|| bad_gateway("financial period values must be an object"))?;
        let mut item_list = Vec::new();
        for field_id in &field_ids {
            let Some(value) = values.get(field_id) else {
                continue;
            };
            let value = object_entry(value, "financial value")?;
            let mut item = Map::new();
            item.insert("fieldId".to_owned(), json!(field_id));
            copy_optional_number(value, &mut item, "data", "data")?;
            copy_optional_number(value, &mut item, "yoy", "yoy")?;
            copy_optional_number(value, &mut item, "qoq", "qoq")?;
            item_list.push(Value::Object(item));
        }
        let mut entry = Map::new();
        entry.insert("periodText".to_owned(), json!(period_text));
        entry.insert("itemList".to_owned(), Value::Array(item_list));
        if let Some(currency) = optional_text(object, "currency")? {
            entry.insert("currencyCode".to_owned(), json!(currency));
        }
        entries.push(Value::Object(entry));
    }
    let mut metadata = Map::new();
    metadata.insert("structureList".to_owned(), Value::Array(structure));
    Ok((entries, metadata, "market-data-financials".to_owned()))
}

fn project_analyst(
    object: &Map<String, Value>,
) -> Result<(Vec<Value>, Map<String, Value>, String), ResearchReadSnapshotError> {
    let mut entry = Map::new();
    copy_optional_number(object, &mut entry, "rating", "rating")?;
    copy_optional_integer(object, &mut entry, "analyst_count", "analystCount")?;
    if let Some(target) = optional_object(object, "target_price")? {
        copy_optional_number(target, &mut entry, "lowest", "lowest")?;
        copy_optional_number(target, &mut entry, "average", "average")?;
        copy_optional_number(target, &mut entry, "highest", "highest")?;
    }
    if let Some(distribution) = optional_object(object, "distribution")? {
        for (from, to) in [
            ("strong_buy", "strongBuy"),
            ("buy", "buy"),
            ("hold", "hold"),
            ("underperform", "underperform"),
            ("sell", "sell"),
        ] {
            copy_required_number(distribution, &mut entry, from, to)?;
        }
    }
    if let Some(update_time) = optional_text(object, "update_time")? {
        entry.insert("updateTimeStr".to_owned(), json!(update_time));
    }
    Ok((
        vec![Value::Object(entry)],
        Map::new(),
        "market-data-analyst".to_owned(),
    ))
}

fn project_ownership(
    object: &Map<String, Value>,
) -> Result<(Vec<Value>, Map<String, Value>, String), ResearchReadSnapshotError> {
    let groups = required_array(object, "groups")?;
    let mut main = Vec::new();
    let mut holder_types = Vec::new();
    for group in groups {
        let group = object_entry(group, "ownership group")?;
        let kind = required_text(group, "kind")?;
        if !matches!(
            kind.to_ascii_lowercase().as_str(),
            "major_holders" | "holder_types" | "institutional_holders" | "mutualfund_holders"
        ) {
            return Err(bad_gateway("ownership group kind is invalid"));
        }
        let items = required_array(group, "items")?;
        let mut item_list = Vec::with_capacity(items.len());
        for item in items {
            let item = object_entry(item, "ownership item")?;
            let mut projected = Map::new();
            projected.insert("name".to_owned(), json!(required_text(item, "name")?));
            copy_optional_number(item, &mut projected, "holder_pct", "holderPct")?;
            item_list.push(Value::Object(projected));
        }
        let mut projected = Map::new();
        projected.insert("itemList".to_owned(), Value::Array(item_list));
        if let Some(date) = optional_text(group, "static_date")? {
            projected.insert("staticDateStr".to_owned(), json!(date));
        }
        if kind.eq_ignore_ascii_case("major_holders") {
            main.push(Value::Object(projected));
        } else {
            holder_types.push(Value::Object(projected));
        }
    }
    let mut metadata = Map::new();
    metadata.insert("mainHolderInfoList".to_owned(), Value::Array(main));
    metadata.insert("holderTypeInfoList".to_owned(), Value::Array(holder_types));
    Ok((Vec::new(), metadata, "market-data-ownership".to_owned()))
}

fn project_corporate_actions(
    object: &Map<String, Value>,
    requested_market: &str,
    requested_symbol: &str,
) -> Result<(Vec<Value>, Map<String, Value>, String), ResearchReadSnapshotError> {
    let market = required_text(object, "market")?;
    let symbol = required_text(object, "symbol")?;
    let requested_symbol = canonical_symbol(requested_market, requested_symbol);
    if !market.eq_ignore_ascii_case(requested_market)
        || !symbol.eq_ignore_ascii_case(&requested_symbol)
    {
        return Err(bad_gateway(
            "corporate actions identity does not match request",
        ));
    }
    let source = required_text(object, "source")?.to_owned();
    let mut entries = Vec::new();
    for event in required_array(object, "events")? {
        let event = object_entry(event, "corporate action event")?;
        let kind = required_text(event, "kind")?;
        if !matches!(kind.to_ascii_lowercase().as_str(), "dividend" | "split") {
            return Err(bad_gateway("corporate action event kind is invalid"));
        }
        let ex_date = required_text(event, "ex_date")?;
        let mut projected = Map::new();
        projected.insert("kind".to_owned(), json!(kind));
        projected.insert("exDate".to_owned(), json!(ex_date));
        let amount = optional_number(event, "amount")?;
        let ratio = optional_number(event, "ratio")?;
        if kind.eq_ignore_ascii_case("dividend") && amount.is_none() {
            return Err(bad_gateway("dividend event is missing amount"));
        }
        if kind.eq_ignore_ascii_case("split") && ratio.is_none() {
            return Err(bad_gateway("split event is missing ratio"));
        }
        if kind.eq_ignore_ascii_case("dividend") && ratio.is_some() {
            return Err(bad_gateway("dividend event must not contain ratio"));
        }
        if kind.eq_ignore_ascii_case("split") && amount.is_some() {
            return Err(bad_gateway("split event must not contain amount"));
        }
        if kind.eq_ignore_ascii_case("dividend") {
            if let Some(value) = amount.as_ref().and_then(Value::as_f64) {
                projected.insert("statement".to_owned(), json!(format!("每股派息 {value}")));
            }
        } else if let Some(value) = ratio.as_ref().and_then(Value::as_f64) {
            projected.insert("statement".to_owned(), json!(format!("1 拆 {value}")));
        }
        entries.push(Value::Object(projected));
    }
    Ok((entries, Map::new(), source))
}

fn required_text<'a>(
    object: &'a Map<String, Value>,
    key: &str,
) -> Result<&'a str, ResearchReadSnapshotError> {
    object
        .get(key)
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .ok_or_else(|| bad_gateway(format!("research response is missing {key}")))
}

fn optional_text<'a>(
    object: &'a Map<String, Value>,
    key: &str,
) -> Result<Option<&'a str>, ResearchReadSnapshotError> {
    let Some(value) = object.get(key) else {
        return Ok(None);
    };
    if value.is_null() {
        return Ok(None);
    }
    value
        .as_str()
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(Some)
        .ok_or_else(|| bad_gateway(format!("research response field {key} must be a string")))
}

fn required_array<'a>(
    object: &'a Map<String, Value>,
    key: &str,
) -> Result<&'a [Value], ResearchReadSnapshotError> {
    match object.get(key) {
        None | Some(Value::Null) => Ok(&[]),
        Some(value) => value
            .as_array()
            .map(Vec::as_slice)
            .ok_or_else(|| bad_gateway(format!("research response field {key} must be an array"))),
    }
}

fn object_entry<'a>(
    value: &'a Value,
    label: &str,
) -> Result<&'a Map<String, Value>, ResearchReadSnapshotError> {
    value
        .as_object()
        .ok_or_else(|| bad_gateway(format!("{label} must be an object")))
}

fn optional_object<'a>(
    object: &'a Map<String, Value>,
    key: &str,
) -> Result<Option<&'a Map<String, Value>>, ResearchReadSnapshotError> {
    let Some(value) = object.get(key) else {
        return Ok(None);
    };
    if value.is_null() {
        return Ok(None);
    }
    value
        .as_object()
        .map(Some)
        .ok_or_else(|| bad_gateway(format!("research response field {key} must be an object")))
}

fn optional_number(
    object: &Map<String, Value>,
    key: &str,
) -> Result<Option<Value>, ResearchReadSnapshotError> {
    let Some(value) = object.get(key) else {
        return Ok(None);
    };
    if value.is_null() {
        return Ok(None);
    }
    if !value.is_number() || value.as_f64().is_none_or(|number| !number.is_finite()) {
        return Err(bad_gateway(format!(
            "research response field {key} must be numeric"
        )));
    }
    Ok(Some(value.clone()))
}

fn copy_optional_number(
    source: &Map<String, Value>,
    target: &mut Map<String, Value>,
    from: &str,
    to: &str,
) -> Result<(), ResearchReadSnapshotError> {
    if let Some(value) = optional_number(source, from)? {
        target.insert(to.to_owned(), value);
    }
    Ok(())
}

fn copy_required_number(
    source: &Map<String, Value>,
    target: &mut Map<String, Value>,
    from: &str,
    to: &str,
) -> Result<(), ResearchReadSnapshotError> {
    let value = optional_number(source, from)?
        .ok_or_else(|| bad_gateway(format!("research response is missing {from}")))?;
    target.insert(to.to_owned(), value);
    Ok(())
}

fn copy_optional_integer(
    source: &Map<String, Value>,
    target: &mut Map<String, Value>,
    from: &str,
    to: &str,
) -> Result<(), ResearchReadSnapshotError> {
    let Some(value) = source.get(from) else {
        return Ok(());
    };
    if value.is_null() {
        return Ok(());
    }
    let number = value
        .as_i64()
        .ok_or_else(|| bad_gateway(format!("research response field {from} must be an integer")))?;
    if number < 0 {
        return Err(bad_gateway(format!(
            "research response field {from} must be non-negative"
        )));
    }
    target.insert(to.to_owned(), json!(number));
    Ok(())
}

fn invalid(message: impl Into<String>) -> ResearchReadSnapshotError {
    ResearchReadSnapshotError::Invalid(message.into())
}

fn unavailable(message: impl Into<String>) -> ResearchReadSnapshotError {
    ResearchReadSnapshotError::Unavailable(message.into())
}

fn capability(feature: &str, operation: &str) -> ResearchReadSnapshotError {
    ResearchReadSnapshotError::Failed {
        status: 409,
        code: "CAPABILITY_UNAVAILABLE".to_owned(),
        message: format!(
            "embedded market-data provider does not serve {feature} operation {operation:?}"
        ),
        retry_after_seconds: None,
    }
}

fn bad_gateway(message: impl Into<String>) -> ResearchReadSnapshotError {
    ResearchReadSnapshotError::Failed {
        status: 502,
        code: "BAD_GATEWAY".to_owned(),
        message: message.into(),
        retry_after_seconds: None,
    }
}
