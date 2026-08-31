//! OpenD customization adapters used by the production engine.
//!
//! This module intentionally exposes broker-neutral JSON projections.  The
//! generated protobuf messages remain private to the integration crate while
//! the authenticated coordinator remains the sole owner of the TCP session.

use std::sync::{Arc, Mutex};

use prost::Message;
use serde_json::{Value, json};
use thiserror::Error;

use crate::{OpenDSessionCoordinator, OpenDSessionCoordinatorError};

const PROTO_GET_USER_SECURITY: u32 = 3213;
const PROTO_MODIFY_USER_SECURITY: u32 = 3214;
const PROTO_GET_USER_SECURITY_GROUP: u32 = 3222;
const PROTO_GET_PRICE_REMINDER: u32 = 3221;
const PROTO_SET_PRICE_REMINDER: u32 = 3220;
const PROTO_GET_OPTION_EVENT_ALERT: u32 = 3308;
const PROTO_SET_OPTION_EVENT_ALERT: u32 = 3309;

#[derive(Debug, Error)]
pub enum CustomizationError {
    #[error("OpenD customization managed session: {0}")]
    ManagedSession(#[from] crate::OpenDManagedSessionError),
    #[error("OpenD customization session: {0}")]
    Session(#[from] OpenDSessionCoordinatorError),
    #[error("OpenD customization decode failed: {0}")]
    Decode(#[from] prost::DecodeError),
    #[error("OpenD customization request rejected: {0}")]
    Rejected(String),
    #[error("invalid customization request: {0}")]
    Invalid(String),
}

pub trait RemoteWatchlistReadPort: Send + Sync + std::fmt::Debug {
    fn groups(&self) -> Result<Vec<Value>, CustomizationError>;
    fn members(&self, group_name: &str) -> Result<Vec<Value>, CustomizationError>;
}

pub trait RemoteWatchlistWritePort: Send + Sync + std::fmt::Debug {
    fn modify(
        &self,
        group_name: &str,
        operation: &str,
        securities: &[Value],
    ) -> Result<Value, CustomizationError>;
}

#[derive(Clone, Debug)]
pub struct FutuRemoteWatchlistReader {
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
}

impl FutuRemoteWatchlistReader {
    pub fn new(coordinator: Arc<Mutex<OpenDSessionCoordinator>>) -> Self {
        Self { coordinator }
    }

    fn call(&self, protocol: u32, body: Vec<u8>) -> Result<Vec<u8>, CustomizationError> {
        let coordinator = self
            .coordinator
            .lock()
            .map_err(|_| OpenDSessionCoordinatorError::Closed)?;
        Ok(coordinator
            .session()?
            .managed_session()
            .call(protocol, &body)?)
    }
}

impl RemoteWatchlistReadPort for FutuRemoteWatchlistReader {
    fn groups(&self) -> Result<Vec<Value>, CustomizationError> {
        use crate::trade_proto::qot_get_user_security_group as wire;
        let body = wire::Request {
            c2s: wire::C2s {
                group_type: 3,
                header: None,
            },
        }
        .encode_to_vec();
        let response =
            wire::Response::decode(self.call(PROTO_GET_USER_SECURITY_GROUP, body)?.as_slice())?;
        ensure_success(response.ret_type, response.ret_msg)?;
        Ok(response
            .s2c
            .map(|s| {
                s.group_list
                    .into_iter()
                    .map(|g| {
                        json!({
                            "name": g.group_name,
                            "type": group_type_label(g.group_type),
                        })
                    })
                    .collect()
            })
            .unwrap_or_default())
    }

    fn members(&self, group_name: &str) -> Result<Vec<Value>, CustomizationError> {
        let group_name = group_name.trim();
        if group_name.is_empty() {
            return Err(CustomizationError::Invalid(
                "group name is required".to_owned(),
            ));
        }
        use crate::trade_proto::qot_get_user_security as wire;
        let body = wire::Request {
            c2s: wire::C2s {
                group_name: group_name.to_owned(),
                header: None,
            },
        }
        .encode_to_vec();
        let response =
            wire::Response::decode(self.call(PROTO_GET_USER_SECURITY, body)?.as_slice())?;
        ensure_success(response.ret_type, response.ret_msg)?;
        Ok(response
            .s2c
            .map(|s| {
                s.static_info_list
                    .into_iter()
                    .map(|entry| {
                        let basic = entry.basic;
                        let security = basic.security;
                        json!({
                            "instrumentId": instrument_id(security.market, &security.code),
                            "market": market_label(security.market),
                            "symbol": security.code,
                            "name": basic.name,
                            "lotSize": basic.lot_size,
                            "securityType": basic.sec_type,
                        })
                    })
                    .collect()
            })
            .unwrap_or_default())
    }
}

impl RemoteWatchlistWritePort for FutuRemoteWatchlistReader {
    fn modify(
        &self,
        group_name: &str,
        operation: &str,
        securities: &[Value],
    ) -> Result<Value, CustomizationError> {
        let group_name = group_name.trim();
        if group_name.is_empty() {
            return Err(CustomizationError::Invalid(
                "group name is required".to_owned(),
            ));
        }
        let op = match operation.trim().to_ascii_lowercase().as_str() {
            "add" | "append" => 1,
            "delete" | "remove" => 2,
            "move_out" | "moveout" => 3,
            _ => {
                return Err(CustomizationError::Invalid(
                    "operation must be add, delete, or move_out".to_owned(),
                ));
            }
        };
        if securities.is_empty() {
            return Err(CustomizationError::Invalid(
                "securityList must contain at least one security".to_owned(),
            ));
        }
        let security_list = securities
            .iter()
            .enumerate()
            .map(|(index, entry)| parse_remote_security(entry, index))
            .collect::<Result<Vec<_>, _>>()?;
        let changed = security_list.len();
        use crate::trade_proto::qot_modify_user_security as wire;
        let body = wire::Request {
            c2s: wire::C2s {
                group_name: group_name.to_owned(),
                op,
                security_list,
                header: None,
            },
        }
        .encode_to_vec();
        let response =
            wire::Response::decode(self.call(PROTO_MODIFY_USER_SECURITY, body)?.as_slice())?;
        ensure_success(response.ret_type, response.ret_msg)?;
        Ok(json!({"groupName": group_name, "operation": operation, "changed": changed}))
    }
}

#[derive(Clone, Debug)]
pub struct FutuAlertQuery {
    pub coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
}

#[derive(Clone, Debug)]
pub struct FutuAlertWrite {
    pub coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
}

pub trait AlertCustomizationReadPort: Send + Sync + std::fmt::Debug {
    fn price(&self, market: Option<&str>) -> Result<Vec<Value>, CustomizationError>;
    fn option_events(
        &self,
        count: i32,
        page: Option<&str>,
    ) -> Result<Vec<Value>, CustomizationError>;
}

pub trait AlertCustomizationWritePort: Send + Sync + std::fmt::Debug {
    fn set_price(&self, payload: &Value) -> Result<Value, CustomizationError>;
    fn set_option_event(&self, payload: &Value) -> Result<Value, CustomizationError>;
}

impl FutuAlertQuery {
    fn call(&self, protocol: u32, body: Vec<u8>) -> Result<Vec<u8>, CustomizationError> {
        let coordinator = self
            .coordinator
            .lock()
            .map_err(|_| OpenDSessionCoordinatorError::Closed)?;
        Ok(coordinator
            .session()?
            .managed_session()
            .call(protocol, &body)?)
    }
}

impl AlertCustomizationReadPort for FutuAlertQuery {
    fn price(&self, market: Option<&str>) -> Result<Vec<Value>, CustomizationError> {
        use crate::trade_proto::qot_get_price_reminder as wire;
        let body = wire::Request {
            c2s: wire::C2s {
                security: None,
                market: market.and_then(parse_market),
                header: None,
            },
        }
        .encode_to_vec();
        let response =
            wire::Response::decode(self.call(PROTO_GET_PRICE_REMINDER, body)?.as_slice())?;
        ensure_success(response.ret_type, response.ret_msg)?;
        Ok(response
            .s2c
            .map(|s| {
                s.price_reminder_list
                    .into_iter()
                    .flat_map(price_entries)
                    .collect()
            })
            .unwrap_or_default())
    }

    fn option_events(
        &self,
        count: i32,
        page: Option<&str>,
    ) -> Result<Vec<Value>, CustomizationError> {
        use crate::trade_proto::qot_get_option_event_alert as wire;
        let count = count.clamp(1, 500);
        let body = wire::Request {
            c2s: wire::C2s {
                count: Some(count),
                page: page.map(str::to_owned),
            },
        }
        .encode_to_vec();
        let response =
            wire::Response::decode(self.call(PROTO_GET_OPTION_EVENT_ALERT, body)?.as_slice())?;
        ensure_success(response.ret_type, response.ret_msg)?;
        Ok(response
            .s2c
            .map(|s| s.alert_list.into_iter().map(option_event_entry).collect())
            .unwrap_or_default())
    }
}

impl FutuAlertWrite {
    fn call(&self, protocol: u32, body: Vec<u8>) -> Result<Vec<u8>, CustomizationError> {
        let coordinator = self
            .coordinator
            .lock()
            .map_err(|_| OpenDSessionCoordinatorError::Closed)?;
        Ok(coordinator
            .session()?
            .managed_session()
            .call(protocol, &body)?)
    }
}

impl AlertCustomizationWritePort for FutuAlertWrite {
    fn set_price(&self, payload: &Value) -> Result<Value, CustomizationError> {
        let security = security_from_payload(payload)?;
        let price = payload
            .get("price")
            .and_then(Value::as_f64)
            .filter(|value| value.is_finite() && *value > 0.0)
            .ok_or_else(|| {
                CustomizationError::Invalid("price must be a positive finite number".to_owned())
            })?;
        let enabled = payload
            .get("enabled")
            .and_then(Value::as_bool)
            .ok_or_else(|| CustomizationError::Invalid("enabled must be a boolean".to_owned()))?;
        let key = parse_optional_i64(payload.get("key"), "key")?;
        let op = price_reminder_operation(payload, key, enabled)?;
        use crate::trade_proto::qot_set_price_reminder as wire;
        let body = wire::Request {
            c2s: wire::C2s {
                security,
                op,
                key,
                r#type: parse_optional_i32(payload.get("type"), "type")?,
                freq: parse_optional_i32(
                    payload.get("frequency").or_else(|| payload.get("freq")),
                    "frequency",
                )?,
                value: Some(price),
                note: payload
                    .get("note")
                    .and_then(Value::as_str)
                    .map(str::to_owned),
                reminder_session_list: Vec::new(),
                header: None,
            },
        }
        .encode_to_vec();
        let response =
            wire::Response::decode(self.call(PROTO_SET_PRICE_REMINDER, body)?.as_slice())?;
        ensure_success(response.ret_type, response.ret_msg)?;
        Ok(json!({"key": response.s2c.map(|s| s.key).unwrap_or_default()}))
    }

    fn set_option_event(&self, payload: &Value) -> Result<Value, CustomizationError> {
        let operation = parse_option_event_operation(payload.get("operation"))?;
        let alert_list = payload
            .get("alertList")
            .and_then(Value::as_array)
            .ok_or_else(|| CustomizationError::Invalid("alertList must be an array".to_owned()))?;
        if alert_list.is_empty() {
            return Err(CustomizationError::Invalid(
                "alertList must contain at least one alert".to_owned(),
            ));
        }
        let alert_list = alert_list
            .iter()
            .enumerate()
            .map(|(index, item)| {
                option_event_from_payload(item).map_err(|error| match error {
                    CustomizationError::Invalid(message) => {
                        CustomizationError::Invalid(format!("alertList[{index}]: {message}"))
                    }
                    other => other,
                })
            })
            .collect::<Result<Vec<_>, _>>()?;
        use crate::trade_proto::qot_set_option_event_alert as wire;
        let body = wire::Request {
            c2s: wire::C2s {
                oper_type: operation,
                alert_list,
            },
        }
        .encode_to_vec();
        let response =
            wire::Response::decode(self.call(PROTO_SET_OPTION_EVENT_ALERT, body)?.as_slice())?;
        ensure_success(response.ret_type, response.ret_msg)?;
        Ok(json!({"updated": true}))
    }
}

fn ensure_success(ret_type: i32, message: Option<String>) -> Result<(), CustomizationError> {
    if ret_type == 0 {
        return Ok(());
    }
    Err(CustomizationError::Rejected(
        message.unwrap_or_else(|| format!("retType={ret_type}")),
    ))
}

fn group_type_label(value: i32) -> &'static str {
    match value {
        1 => "custom",
        2 => "system",
        _ => "unknown",
    }
}
fn market_label(value: i32) -> &'static str {
    match value {
        1 => "HK",
        11 => "US",
        21 => "SH",
        22 => "SZ",
        31 => "SG",
        41 => "JP",
        51 => "AU",
        61 => "MY",
        71 => "CA",
        81 => "FX",
        91 => "CC",
        101 => "US",
        _ => "",
    }
}
fn parse_market(value: &str) -> Option<i32> {
    match value.trim().to_ascii_uppercase().as_str() {
        "HK" => Some(1),
        "US" => Some(11),
        "SH" => Some(21),
        "SZ" => Some(22),
        "CN" => Some(21),
        "SG" => Some(31),
        "JP" => Some(41),
        "AU" => Some(51),
        "MY" => Some(61),
        "CA" => Some(71),
        "FX" => Some(81),
        "CC" => Some(91),
        "EVENT" | "EVENT_CONTRACT" => Some(101),
        value => value
            .parse::<i32>()
            .ok()
            .filter(|market| !market_label(*market).is_empty()),
    }
}
fn instrument_id(market: i32, code: &str) -> String {
    format!("{}.{}", market_label(market), code)
}

fn parse_i32(value: &Value) -> Option<i32> {
    value
        .as_i64()
        .and_then(|value| i32::try_from(value).ok())
        .or_else(|| {
            value
                .as_str()
                .and_then(|value| value.trim().parse::<i32>().ok())
        })
}

fn parse_remote_security(
    value: &Value,
    index: usize,
) -> Result<crate::trade_proto::qot_common::Security, CustomizationError> {
    let object = value.as_object().ok_or_else(|| {
        CustomizationError::Invalid(format!("securityList[{index}] must be an object"))
    })?;
    let market = object.get("market").ok_or_else(|| {
        CustomizationError::Invalid(format!("securityList[{index}].market is required"))
    })?;
    let market = match market {
        Value::Number(value) => value
            .as_i64()
            .and_then(|value| i32::try_from(value).ok())
            .filter(|value| !market_label(*value).is_empty()),
        Value::String(value) => parse_market(value),
        _ => None,
    }
    .ok_or_else(|| {
        CustomizationError::Invalid(format!("securityList[{index}].market is invalid"))
    })?;
    let code = object
        .get("code")
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty() && !value.chars().any(char::is_control))
        .map(str::to_ascii_uppercase)
        .ok_or_else(|| {
            CustomizationError::Invalid(format!("securityList[{index}].code is required"))
        })?;
    Ok(crate::trade_proto::qot_common::Security { market, code })
}

fn price_reminder_operation(
    payload: &Value,
    key: Option<i64>,
    enabled: bool,
) -> Result<i32, CustomizationError> {
    let Some(operation) = payload.get("operation") else {
        return if key.is_some() {
            Ok(if enabled { 3 } else { 4 })
        } else if enabled {
            Ok(1)
        } else {
            Err(CustomizationError::Invalid(
                "key is required when enabled is false".to_owned(),
            ))
        };
    };
    let operation = parse_i32(operation).or_else(|| {
        operation
            .as_str()
            .and_then(|value| match value.trim().to_ascii_lowercase().as_str() {
                "add" => Some(1),
                "delete" | "del" | "remove" => Some(2),
                "enable" => Some(3),
                "disable" => Some(4),
                "modify" | "update" => Some(5),
                "delete_all" | "del_all" => Some(6),
                _ => None,
            })
    });
    let operation = operation.ok_or_else(|| {
        CustomizationError::Invalid(
            "operation must be add, delete, enable, disable, modify, or delete_all".to_owned(),
        )
    })?;
    if !(1..=6).contains(&operation) {
        return Err(CustomizationError::Invalid(
            "operation is invalid".to_owned(),
        ));
    }
    if matches!(operation, 2..=6) && key.is_none() && operation != 6 {
        return Err(CustomizationError::Invalid(
            "key is required for this operation".to_owned(),
        ));
    }
    Ok(operation)
}

fn price_entries(
    reminder: crate::trade_proto::qot_get_price_reminder::PriceReminder,
) -> Vec<Value> {
    let security = reminder.security;
    let instrument = instrument_id(security.market, &security.code);
    reminder.item_list.into_iter().map(|item| json!({
        "key": item.key, "enabled": item.is_enable, "target": item.value,
        "type": format_price_type(item.r#type), "frequency": format_frequency(item.freq),
        "instrumentId": instrument, "market": market_label(security.market), "symbol": security.code,
        "sessions": ["regular"],
    })).collect()
}

fn option_event_entry(
    item: crate::trade_proto::qot_get_option_event_alert::EventAlertItem,
) -> Value {
    let underlying = item
        .underlying
        .and_then(|s| parse_security_value(s.market, s.code));
    json!({
        "key": item.key.unwrap_or_default(), "enabled": item.enable.unwrap_or(false),
        "note": item.note.unwrap_or_default(), "underlying": underlying,
        "optionType": item.option_type.map(|v| if v == 1 { "call" } else { "put" }),
        "sideTypeList": item.side_type_list, "orderTypeList": item.order_type_list,
        "optionMarket": item.option_market.map(option_market_label),
        "earningsDateBegin": item.earnings_date_begin,
    })
}

fn parse_security_value(market: i32, code: String) -> Option<Value> {
    Some(
        json!({"market": market_label(market), "code": code, "instrumentId": instrument_id(market, &code)}),
    )
}
fn format_price_type(value: i32) -> &'static str {
    match value {
        1 => "price_up",
        2 => "price_down",
        _ => "unknown",
    }
}
fn format_frequency(value: i32) -> &'static str {
    match value {
        1 => "once",
        2 => "once_a_day",
        _ => "always",
    }
}

fn security_from_payload(
    payload: &Value,
) -> Result<crate::trade_proto::qot_common::Security, CustomizationError> {
    let instrument = payload
        .get("symbol")
        .or_else(|| payload.get("instrumentId"))
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .ok_or_else(|| CustomizationError::Invalid("symbol is required".to_owned()))?;
    let (market, code) = instrument
        .split_once('.')
        .ok_or_else(|| CustomizationError::Invalid("symbol must be MARKET.CODE".to_owned()))?;
    let market = parse_market(market)
        .ok_or_else(|| CustomizationError::Invalid("unsupported market".to_owned()))?;
    let code = code.trim();
    if code.is_empty() || code.chars().any(char::is_control) {
        return Err(CustomizationError::Invalid(
            "symbol code is invalid".to_owned(),
        ));
    }
    Ok(crate::trade_proto::qot_common::Security {
        market,
        code: code.to_ascii_uppercase(),
    })
}

fn option_event_from_payload(
    payload: &Value,
) -> Result<crate::trade_proto::qot_get_option_event_alert::EventAlertItem, CustomizationError> {
    let underlying = match payload.get("underlying") {
        None | Some(Value::Null) => None,
        Some(value) => Some(parse_option_underlying(value)?),
    };
    let option_market = match payload.get("optionMarket") {
        None | Some(Value::Null) => None,
        Some(value) => Some(
            parse_option_market_value(value)
                .ok_or_else(|| CustomizationError::Invalid("optionMarket is invalid".to_owned()))?,
        ),
    };
    let key = parse_optional_i64(payload.get("key"), "key")?;
    let enabled =
        match payload.get("enabled") {
            None | Some(Value::Null) => None,
            Some(value) => Some(value.as_bool().ok_or_else(|| {
                CustomizationError::Invalid("enabled must be a boolean".to_owned())
            })?),
        };
    Ok(
        crate::trade_proto::qot_get_option_event_alert::EventAlertItem {
            key,
            enable: enabled,
            option_market,
            watchlist_group_name: payload
                .get("watchlistGroupName")
                .and_then(Value::as_str)
                .map(str::to_owned),
            underlying,
            option_type: payload.get("optionType").and_then(parse_i32),
            side_type_list: parse_i32_array(payload.get("sideTypeList"), "sideTypeList")?,
            order_type_list: parse_i32_array(payload.get("orderTypeList"), "orderTypeList")?,
            market_cap_range: None,
            expiry_days_range: None,
            price_range: None,
            size_range: None,
            premium_range: None,
            iv_range: None,
            earnings_date_begin: payload
                .get("earningsDateBegin")
                .and_then(Value::as_str)
                .map(str::to_owned),
            earnings_date_end: payload
                .get("earningsDateEnd")
                .and_then(Value::as_str)
                .map(str::to_owned),
            note: payload
                .get("note")
                .and_then(Value::as_str)
                .map(str::to_owned),
        },
    )
}

fn parse_market_value(value: &Value) -> Option<i32> {
    match value {
        Value::Number(value) => value
            .as_i64()
            .and_then(|value| i32::try_from(value).ok())
            .filter(|value| !market_label(*value).is_empty()),
        Value::String(value) => parse_market(value),
        _ => None,
    }
}

fn parse_option_market_value(value: &Value) -> Option<i32> {
    match value {
        Value::Number(value) => value
            .as_i64()
            .and_then(|value| i32::try_from(value).ok())
            .filter(|value| (1..=4).contains(value)),
        Value::String(value) => match value.trim().to_ascii_uppercase().as_str() {
            "US" | "US_SECURITY" | "US_STOCK" => Some(1),
            "US_INDEX" => Some(2),
            "HK" | "HK_SECURITY" | "HK_STOCK" => Some(3),
            "HK_INDEX" => Some(4),
            value => value
                .parse::<i32>()
                .ok()
                .filter(|value| (1..=4).contains(value)),
        },
        _ => None,
    }
}

fn option_market_label(value: i32) -> &'static str {
    match value {
        1 => "US",
        2 => "US_INDEX",
        3 => "HK",
        4 => "HK_INDEX",
        _ => "",
    }
}

fn parse_option_underlying(
    value: &Value,
) -> Result<crate::trade_proto::qot_common::Security, CustomizationError> {
    let object = value
        .as_object()
        .ok_or_else(|| CustomizationError::Invalid("underlying must be an object".to_owned()))?;
    let market = object
        .get("market")
        .and_then(parse_market_value)
        .ok_or_else(|| CustomizationError::Invalid("underlying.market is invalid".to_owned()))?;
    let code = object
        .get("code")
        .or_else(|| object.get("symbol"))
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty() && !value.chars().any(char::is_control))
        .map(str::to_ascii_uppercase)
        .ok_or_else(|| CustomizationError::Invalid("underlying.code is required".to_owned()))?;
    Ok(crate::trade_proto::qot_common::Security { market, code })
}

fn parse_i32_array(value: Option<&Value>, field: &str) -> Result<Vec<i32>, CustomizationError> {
    let Some(value) = value else {
        return Ok(Vec::new());
    };
    let values = value
        .as_array()
        .ok_or_else(|| CustomizationError::Invalid(format!("{field} must be an array")))?;
    values
        .iter()
        .enumerate()
        .map(|(index, value)| {
            parse_i32(value).ok_or_else(|| {
                CustomizationError::Invalid(format!("{field}[{index}] must be an integer"))
            })
        })
        .collect()
}

fn parse_optional_i64(
    value: Option<&Value>,
    field: &str,
) -> Result<Option<i64>, CustomizationError> {
    let Some(value) = value else { return Ok(None) };
    if value.is_null() {
        return Ok(None);
    }
    value
        .as_i64()
        .map(Some)
        .ok_or_else(|| CustomizationError::Invalid(format!("{field} must be an integer")))
}

fn parse_optional_i32(
    value: Option<&Value>,
    field: &str,
) -> Result<Option<i32>, CustomizationError> {
    let Some(value) = value else { return Ok(None) };
    if value.is_null() {
        return Ok(None);
    }
    parse_i32(value)
        .map(Some)
        .ok_or_else(|| CustomizationError::Invalid(format!("{field} must be an integer")))
}

fn parse_option_event_operation(value: Option<&Value>) -> Result<i32, CustomizationError> {
    let value =
        value.ok_or_else(|| CustomizationError::Invalid("operation is required".to_owned()))?;
    let operation = parse_i32(value).or_else(|| {
        value
            .as_str()
            .and_then(|value| match value.trim().to_ascii_lowercase().as_str() {
                "add" => Some(1),
                "delete" | "del" | "remove" => Some(2),
                "modify" | "update" => Some(3),
                "enable" => Some(4),
                "disable" => Some(5),
                "delete_all" | "del_all" => Some(6),
                _ => None,
            })
    });
    let operation = operation.ok_or_else(|| {
        CustomizationError::Invalid(
            "operation must be add, delete, modify, enable, disable, or delete_all".to_owned(),
        )
    })?;
    if !(1..=6).contains(&operation) {
        return Err(CustomizationError::Invalid(
            "operation is invalid".to_owned(),
        ));
    }
    Ok(operation)
}
