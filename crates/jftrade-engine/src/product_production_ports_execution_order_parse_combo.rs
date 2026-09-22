//! Option-combo and event-parlay intent validation.

use super::*;

pub(in super::super) fn parse_combo_with_defaults(
    payload: &Value,
    default_environment: Option<&str>,
) -> Result<ParsedCombo, String> {
    let object = payload
        .as_object()
        .ok_or_else(|| "combo payload must be an object".to_owned())?;
    let request_market = string_field(object, "market");
    let mut order = parse_order_for_combo(payload, default_environment)?;
    // ComboOrderIntent keeps the caller's market string (unlike a single
    // order's ParseInstrument projection, which resolves SH/SZ to CN).
    if let Some(ref request_market) = request_market {
        order.market = request_market.to_ascii_uppercase();
    }
    if !matches!(order.order_kind.as_str(), "option_combo" | "event_parlay") {
        return Err("orderKind must be option_combo or event_parlay".to_owned());
    }
    if string_field(object, "clientOrderId").is_none() {
        return Err(
            "clientOrderId is required for idempotent combo preview and submission".to_owned(),
        );
    }
    if order.order_kind == "option_combo" {
        // Go's `validateOptionComboRequest` rejects the event-contract size
        // unit before anything else: `amount` must never ride along with an
        // option combo, whose size is the contract quantity.
        if order.amount.is_some() {
            return Err("amount is supported for event parlay orders only".to_owned());
        }
        if order.product_class != "option" {
            return Err("option_combo requires productClass option".to_owned());
        }
        if string_field(object, "underlyingInstrumentId").is_none() {
            return Err("option combo requires underlyingInstrumentId".to_owned());
        }
        if string_field(object, "nearExpiry").is_none() {
            return Err("option combo requires nearExpiry".to_owned());
        }
        let strategy = string_field(object, "optionStrategy")
            .unwrap_or_default()
            .to_ascii_lowercase();
        match strategy.as_str() {
            "vertical" | "strangle" | "butterfly" => {
                let spread = number_field(object, "spread");
                if spread.is_none_or(|value| !value.is_finite() || value <= 0.0) {
                    return Err(format!(
                        "{strategy} option combo requires a positive spread"
                    ));
                }
            }
            "straddle" => {}
            "calendar" => {
                if string_field(object, "farExpiry").is_none() {
                    return Err("calendar option combo requires farExpiry".to_owned());
                }
            }
            _ => return Err(format!("unsupported optionStrategy {strategy:?}")),
        }
    } else {
        if order.product_class != "event_contract" {
            return Err("event_parlay requires productClass event_contract".to_owned());
        }
        if request_market
            .as_deref()
            .is_none_or(|market| !market.eq_ignore_ascii_case("US"))
        {
            return Err("event parlay must use market US".to_owned());
        }
        if string_field(object, "rfqId").is_none()
            || number_field(object, "amount").is_none_or(|value| !value.is_finite() || value <= 0.0)
        {
            return Err("event parlay requires rfqId and positive amount".to_owned());
        }
        if number_field(object, "price").is_some() {
            return Err(
                "event parlay price is bound to the server-side RFQ and must not be provided"
                    .to_owned(),
            );
        }
        validate_event_parlay_quote(object, order.header.trd_env)?;
    }
    let leg_payloads = payload
        .get("legs")
        .and_then(Value::as_array)
        .ok_or_else(|| "legs is required".to_owned())?
        .to_vec();
    let legs = leg_payloads
        .iter()
        .map(|item| {
            let object = item
                .as_object()
                .ok_or_else(|| "combo leg must be an object".to_owned())?;
            let instrument = string_field(object, "instrumentId")
                .or_else(|| string_field(object, "symbol"))
                .or_else(|| string_field(object, "code"))
                .ok_or_else(|| "combo leg instrumentId is required".to_owned())?;
            let instrument = instrument.to_ascii_uppercase().replace(':', ".");
            let (market, code) = if order.order_kind == "event_parlay" {
                // Event contracts are represented by Futu's dedicated US
                // event quote market regardless of the public US symbol
                // prefix.
                (
                    quote_market("US_EVENT"),
                    instrument.trim_start_matches("US.").to_owned(),
                )
            } else {
                // Go resolves every option leg through
                // `futuSecurityFromSymbol` -> `market.ParseInstrument`, which
                // rejects a symbol without a market prefix.  Falling back to
                // the trade market here would accept `BAD` as an unqualified
                // US option and hide a malformed caller intent.
                let (market, code) = instrument.rsplit_once('.').ok_or_else(|| {
                    "combo leg instrumentId must be in MARKET.CODE form".to_owned()
                })?;
                (quote_market(market), code.trim().to_owned())
            };
            if market == 0 || code.trim().is_empty() {
                return Err("combo leg instrumentId has an unsupported market".to_owned());
            }
            if let Some(product_class) = string_field(object, "productClass")
                && product_class.to_ascii_lowercase() != order.product_class
            {
                return Err("combo cannot mix product classes".to_owned());
            }
            let side_value = string_field(object, "side").ok_or_else(|| {
                "each combo leg requires instrumentId, BUY/SELL side, and positive ratio".to_owned()
            })?;
            if !matches!(
                side_value.to_ascii_uppercase().as_str(),
                "BUY" | "B" | "SELL" | "S"
            ) {
                return Err(
                    "each combo leg requires instrumentId, BUY/SELL side, and positive ratio"
                        .to_owned(),
                );
            }
            let side = parse_side(&side_value)?;
            let ratio = number_field(object, "ratio")
                .or_else(|| number_field(object, "qtyRatio"))
                .ok_or_else(|| {
                    "each combo leg requires instrumentId, BUY/SELL side, and positive ratio"
                        .to_owned()
                })?;
            if !ratio.is_finite() || ratio <= 0.0 || ratio.fract() != 0.0 {
                return Err(
                    "each combo leg requires instrumentId, BUY/SELL side, and positive ratio"
                        .to_owned(),
                );
            }
            let prediction_side = string_field(object, "predictionSide")
                .map(|value| match value.to_ascii_uppercase().as_str() {
                    "YES" => Ok(1),
                    "NO" => Ok(2),
                    _ => Err("predictionSide must be YES or NO".to_owned()),
                })
                .transpose()?;
            if order.order_kind == "event_parlay" && prediction_side.is_none() {
                return Err("event parlay legs require predictionSide YES or NO".to_owned());
            }
            if order.order_kind == "option_combo" && prediction_side.is_some() {
                return Err("predictionSide is only supported for event parlay legs".to_owned());
            }
            Ok(TradeComboLeg {
                market,
                code,
                side: Some(side),
                qty_ratio: Some(ratio),
                position_id: object.get("positionId").and_then(Value::as_u64),
                pred_side: prediction_side,
            })
        })
        .collect::<Result<Vec<_>, String>>()?;
    if legs.len() < 2 {
        return Err("combo requires at least two legs".to_owned());
    }
    Ok(ParsedCombo {
        order,
        legs,
        quote_id: string_field(object, "rfqId"),
        leg_payloads,
    })
}

fn validate_event_parlay_quote(
    object: &Map<String, Value>,
    trading_environment: i32,
) -> Result<(), String> {
    if trading_environment == 1 {
        return Err("prediction quote persistence is unavailable for REAL orders".to_owned());
    }
    let quote_expires_at = string_field(object, "quoteExpiresAt")
        .ok_or_else(|| "Parlay quote expired; request a new RFQ".to_owned())?;
    let parsed = time::OffsetDateTime::parse(
        &quote_expires_at,
        &time::format_description::well_known::Rfc3339,
    )
    .map_err(|error| format!("quoteExpiresAt is invalid: {error}"))?;
    if !time::OffsetDateTime::now_utc().lt(&parsed) {
        return Err("Parlay quote expired; request a new RFQ".to_owned());
    }
    Ok(())
}
