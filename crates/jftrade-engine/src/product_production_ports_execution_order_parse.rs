//! Validation and neutral conversion for execution-order write requests.

use serde_json::{Map, Value};

use jftrade_integration_futu::{
    TradeComboLeg, TradeHeader, TradePlaceComboOrderRequest, TradePlaceOrderRequest,
};
use jftrade_store_sqlite::StoredExecutionOrder;

#[path = "product_production_ports_execution_order_markets.rs"]
mod markets;

pub(super) use markets::{
    market_label, quote_market, quote_market_label, sec_market, trade_market,
};
use markets::quote_market_from_trade_market;

#[derive(Clone, Debug)]
pub(super) struct ParsedOrder {
    pub(super) header: TradeHeader,
    pub(super) broker_id: String,
    pub(super) market: String,
    pub(super) symbol: String,
    pub(super) code: String,
    pub(super) side: i32,
    pub(super) order_type: i32,
    pub(super) quantity: f64,
    pub(super) price: Option<f64>,
    pub(super) remark: Option<String>,
    pub(super) client_order_id: Option<String>,
    pub(super) time_in_force: Option<i32>,
    pub(super) session: Option<i32>,
    pub(super) stop_price: Option<f64>,
    pub(super) fill_outside_rth: Option<bool>,
    pub(super) preview_id: Option<String>,
    pub(super) product_class: String,
    pub(super) order_kind: String,
    pub(super) quantity_mode: String,
    pub(super) amount: Option<f64>,
    pub(super) prediction_side: Option<i32>,
}

pub(super) fn requires_locked_preview(order: &ParsedOrder) -> bool {
    order.order_kind == "event_single"
        || matches!(
            order.product_class.as_str(),
            "option" | "future" | "event_contract"
        )
}

impl ParsedOrder {
    pub(super) fn to_trade_request(&self) -> TradePlaceOrderRequest {
        TradePlaceOrderRequest {
            header: self.header.clone(),
            trd_side: self.side,
            order_type: self.order_type,
            code: self.code.clone(),
            quantity: self.quantity,
            price: self.price,
            remark: self.remark.clone().or_else(|| self.client_order_id.clone()),
            time_in_force: self.time_in_force,
            fill_outside_rth: self.fill_outside_rth,
            aux_price: self.stop_price,
            trail_type: None,
            trail_value: None,
            trail_spread: None,
            session: self.session,
            position_id: None,
            expire_time: None,
            amount: self.amount,
            prediction_side: self.prediction_side,
            sec_market: Some(sec_market(self.header.trd_market)),
        }
    }
}

#[derive(Clone, Debug)]
pub(super) struct ParsedCombo {
    pub(super) order: ParsedOrder,
    pub(super) legs: Vec<TradeComboLeg>,
    /// Server-issued prediction RFQ identity. OpenD requires this value when
    /// placing event-parlay combos; dropping it would submit an unbound quote.
    pub(super) quote_id: Option<String>,
    /// Original normalized JSON legs retained for the combo preview/order
    /// projection. OpenD's neutral leg omits optional quantity/amount/price
    /// fields and must not force the public wire shape to drop them.
    pub(super) leg_payloads: Vec<Value>,
}

impl ParsedCombo {
    pub(super) fn combo_quantity(&self) -> f64 {
        if self.order.order_kind == "event_parlay"
            && self
                .order
                .amount
                .is_some_and(|value| value.is_finite() && value > 0.0)
        {
            return self.order.amount.unwrap_or(1.0);
        }
        for (leg, payload) in self.legs.iter().zip(&self.leg_payloads) {
            let Some(quantity) = payload
                .get("quantity")
                .and_then(Value::as_f64)
                .filter(|value| value.is_finite() && *value > 0.0)
            else {
                continue;
            };
            let ratio = leg
                .qty_ratio
                .filter(|value| value.is_finite() && *value > 0.0)
                .unwrap_or(1.0);
            return quantity / ratio;
        }
        self.order.quantity
    }

    pub(super) fn to_trade_request(&self) -> TradePlaceComboOrderRequest {
        TradePlaceComboOrderRequest {
            header: self.order.header.clone(),
            combo_legs: self.legs.clone(),
            quantity: self.combo_quantity(),
            price: self.order.price,
            order_type: self.order.order_type,
            time_in_force: self.order.time_in_force,
            expire_time: None,
            remark: self.order.remark.clone(),
            quote_id: self.quote_id.clone(),
        }
    }
}

pub(super) fn parse_order(payload: &Value) -> Result<ParsedOrder, String> {
    let object = payload
        .as_object()
        .ok_or_else(|| "order payload must be an object".to_owned())?;
    // The Futu production command DTO has no reduce-only field. Reject an
    // explicit true value instead of silently ignoring a public request.
    if object.get("reduceOnly").and_then(Value::as_bool) == Some(true) {
        return Err("reduceOnly is not supported by the Futu execution adapter".to_owned());
    }
    let account_id = string_field(object, "accountId")
        .ok_or_else(|| "accountId is required".to_owned())?
        .parse::<u64>()
        .map_err(|_| "accountId must be numeric for Futu".to_owned())?;
    let supplied_symbol = string_field(object, "symbol");
    let supplied_code = string_field(object, "code");
    let symbol = supplied_symbol
        .clone()
        .or_else(|| supplied_code.clone())
        .or_else(|| string_field(object, "underlyingInstrumentId"))
        .or_else(|| {
            object
                .get("legs")
                .and_then(Value::as_array)
                .and_then(|legs| legs.first())
                .and_then(Value::as_object)
                .and_then(|leg| {
                    string_field(leg, "instrumentId")
                        .or_else(|| string_field(leg, "symbol"))
                        .or_else(|| string_field(leg, "code"))
                })
        })
        .ok_or_else(|| "symbol is required".to_owned())?;
    let (market, symbol, code) = normalize_instrument(
        string_field(object, "market").as_deref(),
        &symbol,
        supplied_code.as_deref(),
    )?;
    let trade_market_code = trade_market(&market);
    if trade_market_code == 0 {
        return Err(format!("unsupported market {market:?}"));
    }
    let has_legs = object.get("legs").is_some();
    let mut order_kind = string_field(object, "orderKind")
        .unwrap_or_else(|| "single".to_owned())
        .to_ascii_lowercase();
    let product_class_supplied = string_field(object, "productClass").is_some();
    let mut product_class = string_field(object, "productClass")
        .unwrap_or_else(|| "equity".to_owned())
        .to_ascii_lowercase();
    let requested_quantity = number_field(object, "quantity");
    let amount = number_field(object, "amount");
    if !has_legs && (product_class == "event_contract" || order_kind == "event_single") {
        order_kind = "event_single".to_owned();
        product_class = "event_contract".to_owned();
    }
    if has_legs && !product_class_supplied {
        product_class = if order_kind == "event_parlay" {
            "event_contract".to_owned()
        } else {
            "option".to_owned()
        };
    }
    if has_legs {
        if !matches!(order_kind.as_str(), "option_combo" | "event_parlay") {
            return Err("orderKind must be option_combo or event_parlay".to_owned());
        }
    } else if !matches!(order_kind.as_str(), "single" | "event_single") {
        return Err(format!(
            "orderKind {order_kind:?} must use the combo execution endpoint"
        ));
    }
    if !matches!(
        product_class.as_str(),
        "equity"
            | "fund"
            | "option"
            | "warrant"
            | "cbbc"
            | "future"
            | "event_contract"
            | "index"
            | "bond"
            | "plate"
    ) {
        return Err(format!("unsupported productClass {product_class:?}"));
    }
    let quantity_mode = string_field(object, "quantityMode")
        .unwrap_or_else(|| {
            if matches!(product_class.as_str(), "option" | "future") {
                "contracts".to_owned()
            } else if product_class == "event_contract" {
                "amount".to_owned()
            } else {
                "units".to_owned()
            }
        })
        .to_ascii_lowercase();
    let expected_mode = if product_class == "event_contract" {
        "amount"
    } else if matches!(product_class.as_str(), "option" | "future") {
        "contracts"
    } else {
        "units"
    };
    if quantity_mode != expected_mode {
        return Err(format!(
            "quantityMode {quantity_mode:?} is invalid for productClass {product_class:?}"
        ));
    }
    let quantity = if product_class == "event_contract" {
        amount.ok_or_else(|| "event-contract amount is required".to_owned())?
    } else if has_legs {
        requested_quantity.unwrap_or(1.0)
    } else {
        requested_quantity.ok_or_else(|| "quantity is required".to_owned())?
    };
    if !quantity.is_finite() || quantity <= 0.0 {
        return Err("quantity must be positive".to_owned());
    }
    if quantity_mode == "contracts" && quantity.fract() != 0.0 {
        return Err("option and future quantity must be an integer number of contracts".to_owned());
    }
    if !has_legs && product_class != "event_contract" {
        if amount.is_some() {
            return Err("amount is supported for event contracts only".to_owned());
        }
        if string_field(object, "predictionSide").is_some() {
            return Err("predictionSide is supported for event contracts only".to_owned());
        }
    }
    let price = number_field(object, "price");
    let stop_price = number_field(object, "stopPrice");
    if price.is_some_and(|value| !value.is_finite() || value <= 0.0) {
        return Err("price must be greater than 0 when provided".to_owned());
    }
    if stop_price.is_some_and(|value| !value.is_finite() || value <= 0.0) {
        return Err("stopPrice must be greater than 0 when provided".to_owned());
    }
    let order_type = parse_order_type(
        string_field(object, "orderType")
            .as_deref()
            .unwrap_or("LIMIT"),
    )?;
    if !has_legs {
        if order_type == 1 && price.is_none() {
            return Err("order type LIMIT requires price".to_owned());
        }
        if matches!(order_type, 3) && stop_price.is_none() {
            return Err("order type STOP requires stopPrice".to_owned());
        }
        if matches!(order_type, 4) && (price.is_none() || stop_price.is_none()) {
            return Err("order type STOP_LIMIT requires price and stopPrice".to_owned());
        }
    }
    let prediction_side = if product_class == "event_contract" && !has_legs {
        if !market.eq_ignore_ascii_case("US") {
            return Err("prediction contracts must use market US".to_owned());
        }
        let side = string_field(object, "predictionSide")
            .ok_or_else(|| "predictionSide must be YES or NO".to_owned())?;
        let side = match side.to_ascii_uppercase().as_str() {
            "YES" => 1,
            "NO" => 2,
            _ => return Err("predictionSide must be YES or NO".to_owned()),
        };
        if price.is_none_or(|value| !(0.01..=0.99).contains(&value)) {
            return Err("event-contract price must be between 0.01 and 0.99".to_owned());
        }
        Some(side)
    } else {
        None
    };
    let raw_session = string_field(object, "session");
    if raw_session.is_some() && !market.eq_ignore_ascii_case("US") {
        return Err("session is supported for US market orders only".to_owned());
    }
    if raw_session.is_some() && product_class == "option" {
        return Err("US options do not support stock extended-hours sessions".to_owned());
    }
    let trading_environment = string_field(object, "tradingEnvironment")
        .or_else(|| string_field(object, "env"))
        .unwrap_or_else(|| "SIMULATE".to_owned());
    let client_order_id = string_field(object, "clientOrderId");
    let remark = string_field(object, "remark").or_else(|| client_order_id.clone());
    let time_in_force =
        parse_time_in_force(string_field(object, "timeInForce").as_deref())?.or(Some(0));
    let session = if raw_session.is_none()
        && market.eq_ignore_ascii_case("US")
        && product_class != "option"
        && product_class != "event_contract"
    {
        // Match Go's normalizeExecutionSession default (RTH) and OpenD's
        // Common.Session enum rather than sending Session_NONE (0).
        Some(1)
    } else {
        parse_session(raw_session.as_deref())?
    };
    let fill_outside_rth = object
        .get("fillOutsideRTH")
        .and_then(Value::as_bool)
        .or_else(|| {
            if matches!(order_type, 1 | 4) {
                session.map(|value| value != 1)
            } else {
                None
            }
        });
    Ok(ParsedOrder {
        header: TradeHeader {
            trd_env: i32::from(trading_environment.eq_ignore_ascii_case("REAL")),
            acc_id: account_id,
            trd_market: trade_market_code,
            jp_acc_type: None,
        },
        broker_id: string_field(object, "brokerId")
            .map(|value| value.to_ascii_lowercase())
            .unwrap_or_else(|| "futu".to_owned()),
        market,
        symbol,
        code,
        side: parse_side(string_field(object, "side").as_deref().unwrap_or("BUY"))?,
        order_type,
        quantity,
        price,
        remark,
        client_order_id,
        time_in_force,
        session,
        stop_price,
        fill_outside_rth,
        preview_id: string_field(object, "previewId"),
        product_class,
        order_kind,
        quantity_mode,
        amount,
        prediction_side,
    })
}

pub(super) fn parse_combo(payload: &Value) -> Result<ParsedCombo, String> {
    let object = payload
        .as_object()
        .ok_or_else(|| "combo payload must be an object".to_owned())?;
    let request_market = string_field(object, "market");
    let mut order = parse_order(payload)?;
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
                instrument.rsplit_once('.').map_or_else(
                    || {
                        (
                            quote_market_from_trade_market(order.header.trd_market),
                            instrument.clone(),
                        )
                    },
                    |(market, code)| (quote_market(market), code.trim().to_owned()),
                )
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

pub(super) fn new_order(id: &str, parsed: &ParsedOrder, timestamp: &str) -> StoredExecutionOrder {
    StoredExecutionOrder {
        internal_order_id: id.to_owned(),
        broker_id: parsed.broker_id.clone(),
        broker_order_id: None,
        broker_order_id_ex: None,
        source: "api".to_owned(),
        source_detail: "rust-production".to_owned(),
        trading_environment: if parsed.header.trd_env == 1 {
            "REAL"
        } else {
            "SIMULATE"
        }
        .to_owned(),
        account_id: parsed.header.acc_id.to_string(),
        market: parsed.market.clone(),
        symbol: Some(parsed.symbol.clone()),
        side: Some(side_label(parsed.side).to_owned()),
        order_type: Some(order_type_label(parsed.order_type).to_owned()),
        status: "SUBMITTING".to_owned(),
        raw_broker_status: None,
        requested_quantity: Some(parsed.quantity),
        requested_price: parsed.price,
        filled_quantity: None,
        filled_average_price: None,
        remark: parsed.remark.clone(),
        last_error: None,
        last_error_code: None,
        last_error_source: None,
        submitted_at: None,
        updated_at: timestamp.to_owned(),
        created_at: timestamp.to_owned(),
        order_kind: parsed.order_kind.clone(),
        product_class: parsed.product_class.clone(),
        quantity_mode: parsed.quantity_mode.clone(),
        client_order_id: parsed.client_order_id.clone(),
        preview_id: parsed.preview_id.clone(),
        normalized_request: "{}".to_owned(),
        requested_amount: parsed.amount,
        payout: None,
        fees: None,
    }
}

fn string_field(object: &Map<String, Value>, key: &str) -> Option<String> {
    object
        .get(key)
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(str::to_owned)
}

fn number_field(object: &Map<String, Value>, key: &str) -> Option<f64> {
    object.get(key).and_then(Value::as_f64)
}

/// Normalize the public market/symbol pair the same way as Go's
/// `market.ParseInstrument`: an exchange-qualified symbol determines the
/// prefix, while the resolved market for SH/SZ is the aggregate CN market.
fn normalize_instrument(
    requested_market: Option<&str>,
    raw_symbol: &str,
    supplied_code: Option<&str>,
) -> Result<(String, String, String), String> {
    let symbol = raw_symbol.trim().to_ascii_uppercase().replace(':', ".");
    let code_field = supplied_code
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(|value| value.to_ascii_uppercase());
    if symbol.is_empty() && code_field.is_none() {
        return Err("symbol or code is required".to_owned());
    }

    if let Some((prefix, code)) = symbol.split_once('.') {
        let prefix = prefix.trim().to_ascii_uppercase();
        let code = code.trim().to_ascii_uppercase();
        if prefix.is_empty() || code.is_empty() {
            return Err(format!("symbol {raw_symbol:?} must be in MARKET.CODE form"));
        }
        if code_field.as_deref().is_some_and(|value| value != code) {
            return Err(format!(
                "code {:?} does not match symbol {:?}",
                supplied_code, raw_symbol
            ));
        }
        let (resolved_market, preferred_prefix) = normalize_market_input(&prefix)?;
        if preferred_prefix.is_empty() {
            return Err(format!(
                "market {prefix:?} requires an exchange-qualified symbol like SH.600519 or SZ.000001"
            ));
        }
        if let Some(requested) = requested_market {
            let (requested_resolved, requested_prefix) = normalize_market_input(requested)?;
            if requested_resolved != resolved_market
                || (!requested_prefix.is_empty() && requested_prefix != preferred_prefix)
            {
                return Err(format!(
                    "market {requested:?} does not match symbol {raw_symbol:?}"
                ));
            }
        }
        return Ok((resolved_market, format!("{preferred_prefix}.{code}"), code));
    }

    if code_field
        .as_deref()
        .is_some_and(|value| !symbol.is_empty() && value != symbol)
    {
        return Err(format!(
            "code {:?} does not match symbol {:?}",
            supplied_code, raw_symbol
        ));
    }
    let code = if symbol.is_empty() {
        code_field.unwrap_or_default()
    } else {
        symbol
    };
    if code.is_empty() {
        return Err("symbol or code is required".to_owned());
    }
    let requested = requested_market
        .ok_or_else(|| "market is required when symbol has no market prefix".to_owned())?;
    let (resolved_market, preferred_prefix) = normalize_market_input(requested)?;
    if preferred_prefix.is_empty() {
        return Err(format!(
            "market {requested:?} requires an exchange-qualified symbol like SH.600519 or SZ.000001"
        ));
    }
    Ok((resolved_market, format!("{preferred_prefix}.{code}"), code))
}

fn normalize_market_input(value: &str) -> Result<(String, String), String> {
    match value.trim().to_ascii_uppercase().as_str() {
        "US" => Ok(("US".to_owned(), "US".to_owned())),
        "HK" => Ok(("HK".to_owned(), "HK".to_owned())),
        "SH" | "CNSH" => Ok(("CN".to_owned(), "SH".to_owned())),
        "SZ" | "CNSZ" => Ok(("CN".to_owned(), "SZ".to_owned())),
        "CN" => Ok(("CN".to_owned(), String::new())),
        "SG" => Ok(("SG".to_owned(), "SG".to_owned())),
        _ => Err(format!("unsupported market {:?}", value.trim())),
    }
}

fn parse_side(value: &str) -> Result<i32, String> {
    match value.trim().to_ascii_uppercase().as_str() {
        "BUY" | "B" => Ok(1),
        "SELL" | "S" => Ok(2),
        _ => Err(format!("unsupported side {value:?}")),
    }
}

pub(super) fn parse_order_type(value: &str) -> Result<i32, String> {
    match value.trim().to_ascii_uppercase().as_str() {
        "LIMIT" | "NORMAL" => Ok(1),
        "MARKET" => Ok(2),
        "STOP" | "STOP_MARKET" => Ok(3),
        "STOP_LIMIT" => Ok(4),
        "ABSOLUTE_LIMIT" => Ok(5),
        "AUCTION" => Ok(6),
        "AUCTION_LIMIT" => Ok(7),
        _ => Err(format!("unsupported orderType {value:?}")),
    }
}

fn parse_time_in_force(value: Option<&str>) -> Result<Option<i32>, String> {
    value
        .map(|value| match value.trim().to_ascii_uppercase().as_str() {
            "DAY" => Ok(0),
            "GTC" => Ok(1),
            "IOC" => Ok(2),
            "GTD" => Ok(3),
            _ => Err(format!("unsupported timeInForce {value:?}")),
        })
        .transpose()
}

pub(super) fn parse_session(value: Option<&str>) -> Result<Option<i32>, String> {
    value
        .map(|value| match value.trim().to_ascii_lowercase().as_str() {
            "rth" | "regular" | "normal" => Ok(1),
            "eth" | "extended" | "pre" | "premarket" | "after" | "afterhours" => Ok(2),
            "all" => Ok(3),
            "overnight" => Ok(4),
            _ => Err(format!("unsupported session {value:?}")),
        })
        .transpose()
}

include!("product_production_ports_execution_order_labels.rs");
