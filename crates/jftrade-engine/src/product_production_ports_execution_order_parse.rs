//! Validation and neutral conversion for execution-order write requests.

use serde_json::{Map, Value};

use jftrade_integration_futu::{
    TradeHeader, TradePlaceComboOrderRequest, TradePlaceOrderRequest, TradeComboLeg,
};
use jftrade_store_sqlite::StoredExecutionOrder;

#[derive(Clone, Debug)]
pub(super) struct ParsedOrder {
    pub(super) header: TradeHeader,
    pub(super) broker_id: String,
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
            amount: None,
            prediction_side: None,
            sec_market: Some(sec_market(self.header.trd_market)),
        }
    }
}

#[derive(Clone, Debug)]
pub(super) struct ParsedCombo {
    pub(super) order: ParsedOrder,
    pub(super) legs: Vec<TradeComboLeg>,
}

impl ParsedCombo {
    pub(super) fn to_trade_request(&self) -> TradePlaceComboOrderRequest {
        TradePlaceComboOrderRequest {
            header: self.order.header.clone(),
            combo_legs: self.legs.clone(),
            quantity: self.order.quantity,
            price: self.order.price,
            order_type: self.order.order_type,
            time_in_force: self.order.time_in_force,
            expire_time: None,
            remark: self.order.remark.clone(),
            quote_id: None,
        }
    }
}

pub(super) fn parse_order(payload: &Value) -> Result<ParsedOrder, String> {
    let object = payload
        .as_object()
        .ok_or_else(|| "order payload must be an object".to_owned())?;
    let account_id = string_field(object, "accountId")
        .ok_or_else(|| "accountId is required".to_owned())?
        .parse::<u64>()
        .map_err(|_| "accountId must be numeric for Futu".to_owned())?;
    let symbol = string_field(object, "symbol")
        .or_else(|| string_field(object, "code"))
        .ok_or_else(|| "symbol is required".to_owned())?;
    let code = symbol
        .rsplit_once('.')
        .map_or_else(|| symbol.clone(), |(_, code)| code.to_owned());
    let market = string_field(object, "market").unwrap_or_else(|| "US".to_owned());
    let quantity =
        number_field(object, "quantity").ok_or_else(|| "quantity is required".to_owned())?;
    if !quantity.is_finite() || quantity <= 0.0 {
        return Err("quantity must be positive".to_owned());
    }
    let trading_environment = string_field(object, "tradingEnvironment")
        .or_else(|| string_field(object, "env"))
        .unwrap_or_else(|| "SIMULATE".to_owned());
    Ok(ParsedOrder {
        header: TradeHeader {
            trd_env: i32::from(trading_environment.eq_ignore_ascii_case("REAL")),
            acc_id: account_id,
            trd_market: trade_market(&market),
            jp_acc_type: None,
        },
        broker_id: string_field(object, "brokerId").unwrap_or_else(|| "futu".to_owned()),
        symbol,
        code,
        side: parse_side(string_field(object, "side").as_deref().unwrap_or("BUY"))?,
        order_type: parse_order_type(
            string_field(object, "orderType")
                .as_deref()
                .unwrap_or("LIMIT"),
        )?,
        quantity,
        price: number_field(object, "price"),
        remark: string_field(object, "remark"),
        client_order_id: string_field(object, "clientOrderId"),
        time_in_force: parse_time_in_force(string_field(object, "timeInForce").as_deref())?,
        session: parse_session(string_field(object, "session").as_deref())?,
        stop_price: number_field(object, "stopPrice"),
        fill_outside_rth: object.get("fillOutsideRTH").and_then(Value::as_bool),
    })
}

pub(super) fn parse_combo(payload: &Value) -> Result<ParsedCombo, String> {
    let order = parse_order(payload)?;
    let legs = payload
        .get("legs")
        .and_then(Value::as_array)
        .ok_or_else(|| "legs is required".to_owned())?
        .iter()
        .map(|item| {
            let object = item
                .as_object()
                .ok_or_else(|| "combo leg must be an object".to_owned())?;
            let instrument = string_field(object, "instrumentId")
                .or_else(|| string_field(object, "symbol"))
                .or_else(|| string_field(object, "code"))
                .ok_or_else(|| "combo leg instrumentId is required".to_owned())?;
            let (market, code) = instrument.rsplit_once('.').map_or_else(
                || (order.header.trd_market, instrument.clone()),
                |(market, code)| (trade_market(market), code.to_owned()),
            );
            Ok(TradeComboLeg {
                market,
                code,
                side: string_field(object, "side")
                    .as_deref()
                    .map(parse_side)
                    .transpose()?,
                qty_ratio: number_field(object, "ratio")
                    .or_else(|| number_field(object, "qtyRatio")),
                position_id: object.get("positionId").and_then(Value::as_u64),
                pred_side: None,
            })
        })
        .collect::<Result<Vec<_>, String>>()?;
    if legs.is_empty() {
        return Err("legs must not be empty".to_owned());
    }
    Ok(ParsedCombo { order, legs })
}

pub(super) fn new_order(id: &str, parsed: &ParsedOrder, timestamp: &str) -> StoredExecutionOrder {
    StoredExecutionOrder {
        internal_order_id: id.to_owned(),
        broker_id: parsed.broker_id.clone(),
        broker_order_id: None,
        broker_order_id_ex: None,
        source: "api".to_owned(),
        source_detail: "rust-production".to_owned(),
        trading_environment: if parsed.header.trd_env == 1 { "REAL" } else { "SIMULATE" }
            .to_owned(),
        account_id: parsed.header.acc_id.to_string(),
        market: market_label(parsed.header.trd_market),
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
        order_kind: "single".to_owned(),
        product_class: "equity".to_owned(),
        quantity_mode: "units".to_owned(),
        client_order_id: parsed.client_order_id.clone(),
        preview_id: None,
        normalized_request: "{}".to_owned(),
        requested_amount: None,
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

fn parse_side(value: &str) -> Result<i32, String> {
    match value.trim().to_ascii_uppercase().as_str() {
        "BUY" | "B" => Ok(1),
        "SELL" | "S" => Ok(2),
        "SELL_SHORT" | "SELLSHORT" => Ok(3),
        "BUY_BACK" | "BUYBACK" => Ok(4),
        _ => Err(format!("unsupported side {value:?}")),
    }
}

fn parse_order_type(value: &str) -> Result<i32, String> {
    match value.trim().to_ascii_uppercase().as_str() {
        "LIMIT" | "NORMAL" => Ok(1),
        "MARKET" => Ok(2),
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

fn parse_session(value: Option<&str>) -> Result<Option<i32>, String> {
    value
        .map(|value| match value.trim().to_ascii_lowercase().as_str() {
            "regular" | "normal" => Ok(0),
            "pre" | "premarket" => Ok(1),
            "after" | "afterhours" => Ok(2),
            _ => Err(format!("unsupported session {value:?}")),
        })
        .transpose()
}

pub(super) fn trade_market(value: &str) -> i32 {
    match value.trim().to_ascii_uppercase().as_str() {
        "HK" => 1,
        "US" => 2,
        "CN" | "SH" | "SZ" => 3,
        "SG" => 6,
        _ => 0,
    }
}

fn sec_market(trd_market: i32) -> i32 {
    match trd_market {
        1 => 1,
        2 => 2,
        3 => 31,
        _ => 0,
    }
}

fn market_label(value: i32) -> String {
    match value {
        1 => "HK",
        2 => "US",
        3 => "CN",
        6 => "SG",
        _ => "UNKNOWN",
    }
    .to_owned()
}

const fn side_label(value: i32) -> &'static str {
    match value {
        1 => "BUY",
        2 => "SELL",
        3 => "SELL_SHORT",
        4 => "BUY_BACK",
        _ => "UNKNOWN",
    }
}

const fn order_type_label(value: i32) -> &'static str {
    match value {
        1 => "LIMIT",
        2 => "MARKET",
        _ => "UNKNOWN",
    }
}
