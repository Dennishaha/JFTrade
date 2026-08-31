use super::*;

pub(super) fn underlying_security(
    payload: &Value,
    parsed: &ParsedCombo,
) -> Result<(i32, String), ExecutionWritePortError> {
    let instrument = payload
        .get("underlyingInstrumentId")
        .and_then(Value::as_str)
        .unwrap_or(&parsed.order.symbol);
    let (market, code) = instrument.trim().rsplit_once('.').map_or_else(
        || {
            (
                market_label(parsed.order.header.trd_market),
                instrument.trim().to_owned(),
            )
        },
        |(market, code)| (market.to_owned(), code.to_owned()),
    );
    let market = match market.trim().to_ascii_uppercase().as_str() {
        "US" => 11,
        "HK" => 1,
        _ => {
            return Err(failed(
                400,
                "BAD_REQUEST",
                "option combo underlying market must be US or HK",
            ));
        }
    };
    let code = code.trim().to_ascii_uppercase();
    if code.is_empty() {
        return Err(failed(
            400,
            "BAD_REQUEST",
            "option combo underlying code is required",
        ));
    }
    Ok((market, code))
}

pub(super) fn option_strategy_legs(parsed: &ParsedCombo) -> Vec<jftrade_integration_futu::OptionStrategyLeg> {
    parsed
        .legs
        .iter()
        .map(|leg| {
            let market = quote_market_label(leg.market);
            let code = leg.code.trim().to_ascii_uppercase();
            jftrade_integration_futu::OptionStrategyLeg {
                security: jftrade_integration_futu::OptionStrategySecurity {
                    market: market.to_owned(),
                    code: code.clone(),
                    quote_market: market.to_owned(),
                    trade_market: market.to_owned(),
                    instrument_id: format!("{market}.{code}"),
                },
                side: leg.side,
                qty_ratio: leg.qty_ratio,
                position_id: leg.position_id,
                pred_side: leg.pred_side,
            }
        })
        .collect()
}

pub(super) fn same_option_strategy_legs(
    left: &[jftrade_integration_futu::OptionStrategyLeg],
    right: &[jftrade_integration_futu::OptionStrategyLeg],
) -> bool {
    if left.len() != right.len() {
        return false;
    }
    let mut left = left.iter().map(option_strategy_leg_key).collect::<Vec<_>>();
    let mut right = right
        .iter()
        .map(option_strategy_leg_key)
        .collect::<Vec<_>>();
    left.sort();
    right.sort();
    left == right
}

pub(super) fn option_strategy_leg_key(leg: &jftrade_integration_futu::OptionStrategyLeg) -> String {
    format!(
        "{}|{}|{}",
        leg.security.code.trim().to_ascii_uppercase(),
        leg.side.unwrap_or_default(),
        leg.qty_ratio.unwrap_or_default(),
    )
}

pub(super) fn option_strategy_name(strategy: i32) -> &'static str {
    match strategy {
        4 => "vertical",
        6 => "straddle",
        7 => "strangle",
        9 => "butterfly",
        15 => "calendar",
        _ => "",
    }
}

pub(super) fn normalize_event_contract_code(value: &str) -> Option<String> {
    let value = value.trim();
    let value = value
        .strip_prefix("US.")
        .or_else(|| value.strip_prefix("us."))
        .unwrap_or(value)
        .trim();
    if value.is_empty()
        || value.len() > 512
        || value
            .chars()
            .any(|ch| ch.is_whitespace() || ch.is_control() || matches!(ch, '/' | '\\' | '?' | '#'))
    {
        return None;
    }
    Some(value.to_ascii_uppercase())
}

pub(crate) fn canonical_combo_legs(parsed: &ParsedCombo) -> Value {
    Value::Array(
        parsed
            .legs
            .iter()
            .enumerate()
            .map(|(index, leg)| {
                // Combo normalization in Go upper-cases the supplied
                // instrument id but does not rewrite it to a market prefix.
                // Preserve that exact public value (including SH/SZ and
                // unqualified symbols) instead of the lossy OpenD market enum.
                let raw = parsed.leg_payloads.get(index).and_then(Value::as_object);
                let instrument = raw
                    .and_then(|object| {
                        ["instrumentId", "symbol", "code"]
                            .into_iter()
                            .find_map(|key| {
                                object
                                    .get(key)
                                    .and_then(Value::as_str)
                                    .map(str::trim)
                                    .filter(|value| !value.is_empty())
                            })
                    })
                    .map(|value| value.to_ascii_uppercase())
                    .unwrap_or_else(|| {
                        format!("{}.{}", quote_market_label(leg.market), leg.code.trim())
                            .to_ascii_uppercase()
                    });
                let ratio = leg
                    .qty_ratio
                    .filter(|value| value.is_finite() && value.fract() == 0.0)
                    .map(|value| value as i64);
                let mut value = json!({
                    "instrumentId": instrument,
                    "productClass": parsed.order.product_class.clone(),
                    "side": leg.side.map(side_label).unwrap_or("UNKNOWN"),
                    "ratio": ratio,
                });
                if let Some(object) = raw {
                    for key in ["quantity", "amount", "price"] {
                        if let Some(number) = object.get(key).filter(|value| !value.is_null()) {
                            value[key] = number.clone();
                        }
                    }
                    if let Some(prediction_side) = object
                        .get("predictionSide")
                        .and_then(Value::as_str)
                        .map(str::trim)
                        .filter(|value| !value.is_empty())
                    {
                        value["predictionSide"] =
                            Value::String(prediction_side.to_ascii_uppercase());
                    }
                } else if let Some(prediction_side) = leg.pred_side {
                    value["predictionSide"] =
                        Value::String(prediction_side_label(prediction_side).to_owned());
                }
                value
            })
            .collect(),
    )
}

pub(super) fn preview_symbol(parsed: &ParsedOrder) -> String {
    parsed.symbol.trim().to_ascii_uppercase()
}

pub(super) fn prediction_side_label(value: i32) -> &'static str {
    match value {
        1 => "YES",
        2 => "NO",
        _ => "UNKNOWN",
    }
}

pub(super) fn add_five_minutes(timestamp: &str) -> Result<String, ExecutionWritePortError> {
    let parsed =
        time::OffsetDateTime::parse(timestamp, &time::format_description::well_known::Rfc3339)
            .map_err(|error| failed(500, "EXECUTION_TIME_ERROR", error.to_string()))?;
    (parsed + time::Duration::minutes(5))
        .format(&time::format_description::well_known::Rfc3339)
        .map_err(|error| failed(500, "EXECUTION_TIME_ERROR", error.to_string()))
}

pub(crate) fn jftrade_broker_capability_version() -> String {
    "2026-07-17.opend-10.9.6908".to_owned()
}

pub(super) fn map_trade_error(error: TradeSessionError) -> ExecutionWritePortError {
    super::execution_order_helpers::map_trade_error(error)
}
