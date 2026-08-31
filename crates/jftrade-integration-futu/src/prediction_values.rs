//! JSON conversion and combo-request validation for prediction responses.

use prost::Message;
use serde_json::{Value, json};

use super::{PredictionMarketReadError, code, invalid, security};

pub(super) fn feature_result(feature: &str, entries: Vec<Value>, next: Option<String>) -> Value {
    let has_more = next.is_some();
    json!({
        "asOf": now_rfc3339(),
        "entries": entries,
        "nextCursor": next,
        "hasMore": has_more,
        "total": entries.len(),
        "provider": {
            "brokerId": "futu",
            "securityFirm": "Futu/Moomoo via OpenD",
            "featureId": feature,
            "capability": "available",
            "selectionReason": "adapter_request",
            "resolvedAt": now_rfc3339(),
        },
    })
}

pub(super) fn now_rfc3339() -> String {
    time::OffsetDateTime::now_utc()
        .format(&time::format_description::well_known::Rfc3339)
        .unwrap_or_else(|_| "1970-01-01T00:00:00Z".to_owned())
}

pub(super) fn security_value(value: &crate::trade_proto::qot_common::Security) -> Value {
    json!({"market": value.market, "code": value.code, "instrumentId": format!("US.{}", value.code)})
}

pub(super) fn event_value(item: crate::trade_proto::qot_get_event_contract_event_list::EventItem) -> Value {
    json!({
        "eventSecurity": security_value(&item.event_security),
        "eventName": item.event_name,
        "eventSubName": item.event_sub_name,
        "status": item.status,
        "seriesSecurity": item.series_security.as_ref().map(security_value),
        "startDate": item.start_date,
        "endDate": item.end_date,
        "category": item.category,
        "tags": item.tags,
        "mutuallyExclusive": item.mutually_exclusive,
        "competition": item.competition,
        "competitionScope": item.competition_scope,
    })
}

pub(super) fn contract_value(item: crate::trade_proto::qot_get_event_contract::ContractItem) -> Value {
    json!({
        "contractSecurity": security_value(&item.contract_security),
        "eventSecurity": item.event_security.as_ref().map(security_value),
        "seriesSecurity": item.series_security.as_ref().map(security_value),
        "contractType": item.contract_type,
        "title": item.title,
        "yesSubTitle": item.yes_sub_title,
        "openTime": item.open_time,
        "closeTime": item.close_time,
        "determinationTime": item.determination_time,
        "settledTime": item.settled_time,
        "latestExpirationTime": item.latest_expiration_time,
        "status": item.status,
        "result": item.result,
        "settlementValue": item.settlement_value,
        "expirationValue": item.expiration_value,
        "volume": item.volume,
        "canCloseEarly": item.can_close_early,
        "tickSize": item.tick_size,
        "category": item.category,
        "tag": item.tag,
    })
}

pub(super) fn milestone_value(item: crate::trade_proto::qot_get_event_contract_milestone_list::MilestoneItem) -> Value {
    json!({
        "milestoneSecurity": security_value(&item.milestone_security),
        "title": item.title,
        "category": item.category,
        "type": item.r#type,
        "startDate": item.start_date,
        "endDate": item.end_date,
        "primaryEventSecurity": item.primary_event_security.as_ref().map(security_value),
        "relatedEventList": item.related_event_list.iter().map(security_value).collect::<Vec<_>>(),
        "notificationMessage": item.notification_message,
    })
}

pub(super) fn snapshot_value(item: crate::trade_proto::qot_get_event_contract_snapshot::SnapshotItem) -> Value {
    json!({
        "code": security_value(&item.code),
        "name": item.name,
        "eventCode": item.event_code.as_ref().map(security_value),
        "yesSubTitle": item.yes_sub_title,
        "noSubTitle": item.no_sub_title,
        "status": item.status,
        "price": item.price,
        "cumulativeVolume": item.cumulative_volume,
        "yesBid": item.yes_bid,
        "yesBidSize": item.yes_bid_size,
        "yesAsk": item.yes_ask,
        "yesAskSize": item.yes_ask_size,
        "noBid": item.no_bid,
        "noBidSize": item.no_bid_size,
        "noAsk": item.no_ask,
        "noAskSize": item.no_ask_size,
        "lastTradeTime": item.last_trade_time,
        "volume24h": item.volume_24h,
        "openInterest": item.open_interest,
    })
}

pub(super) fn order_book_value(item: crate::trade_proto::qot_get_event_contract_order_book::OrderBookItem) -> Value {
    let levels = |items: Vec<crate::trade_proto::qot_get_event_contract_order_book::OrderBookLevel>| {
        items.into_iter().map(|level| json!({"price": level.price, "size": level.size})).collect::<Vec<_>>()
    };
    json!({
        "code": security_value(&item.code),
        "yesBids": levels(item.yes_bids),
        "yesAsks": levels(item.yes_asks),
        "noBids": levels(item.no_bids),
        "noAsks": levels(item.no_asks),
    })
}

pub(super) fn kline_value(item: crate::trade_proto::qot_get_event_contract_kline::KlineItem) -> Value {
    json!({
        "code": security_value(&item.code),
        "preSide": item.pre_side,
        "name": item.name,
        "klineList": item.kline_list.into_iter().map(|point| json!({
            "timeKey": point.time_key,
            "open": point.open,
            "high": point.high,
            "low": point.low,
            "close": point.close,
            "volume": point.volume,
        })).collect::<Vec<_>>(),
    })
}

pub(super) fn ticker_value(item: crate::trade_proto::qot_get_event_contract_ticker::TickerItem) -> Value {
    json!({
        "code": security_value(&item.code),
        "tickerList": item.ticker_list.into_iter().map(|point| json!({
            "time": point.time,
            "yesPrice": point.yes_price,
            "noPrice": point.no_price,
            "volume": point.volume,
            "side": point.side,
            "sequence": point.sequence,
        })).collect::<Vec<_>>(),
    })
}

pub(super) fn combo_event_value(item: crate::trade_proto::qot_get_event_contract_combo_list::ComboEvent) -> Value {
    json!({
        "eventSecurity": security_value(&item.event_security),
        "eventName": item.event_name,
        "comboContracts": item.combo_contracts.iter().map(security_value).collect::<Vec<_>>(),
        "seriesSecurity": item.series_security.as_ref().map(security_value),
        "category": item.category,
        "competition": item.competition,
        "competitionScope": item.competition_scope,
    })
}

pub(super) fn combo_leg_value(leg: &crate::trade_proto::qot_common::ComboLeg) -> Value {
    json!({
        "security": security_value(&leg.security),
        "side": leg.side,
        "ratio": leg.qty_ratio,
        "predSide": leg.pred_side,
    })
}

#[derive(Debug)]
pub(super) struct ComboQuoteRequest {
    pub(super) mvc: String,
    pub(super) legs: Vec<crate::trade_proto::qot_common::ComboLeg>,
}

impl ComboQuoteRequest {
    pub(super) fn parse(value: &Value) -> Result<Self, PredictionMarketReadError> {
        let object = value.as_object().ok_or_else(|| invalid("prediction combo quote payload must be an object"))?;
        let mvc = object.get("mvc").and_then(Value::as_str).map(str::trim).filter(|v| !v.is_empty()).ok_or_else(|| invalid("mvc is required"))?;
        if mvc.len() > 256 || mvc.chars().any(char::is_control) {
            return Err(invalid("mvc is invalid"));
        }
        let legs = object.get("legs").or_else(|| object.get("comboLegList")).and_then(Value::as_array).ok_or_else(|| invalid("legs are required"))?;
        if legs.is_empty() || legs.len() > 20 {
            return Err(invalid("legs must contain between 1 and 20 items"));
        }
        let mut encoded = Vec::with_capacity(legs.len());
        for leg in legs {
            let item = leg.as_object().ok_or_else(|| invalid("prediction combo leg is invalid"))?;
            let instrument = item.get("instrumentId").and_then(Value::as_str).ok_or_else(|| invalid("prediction combo leg instrumentId is required"))?;
            let security_code = code(Some(instrument), "instrumentId")?;
            let side = item.get("side").and_then(Value::as_str).map(|v| match v.to_ascii_uppercase().as_str() { "BUY" => Ok(1), "SELL" => Ok(2), _ => Err(()) }).transpose().map_err(|_| invalid("prediction combo leg side must be BUY or SELL"))?.unwrap_or(1);
            let pred_side = item.get("predictionSide").and_then(Value::as_str).map(|v| match v.to_ascii_uppercase().as_str() { "YES" => Ok(1), "NO" => Ok(2), _ => Err(()) }).transpose().map_err(|_| invalid("predictionSide must be YES or NO"))?.unwrap_or(1);
            let ratio = item.get("ratio").and_then(Value::as_i64).unwrap_or(1);
            if !(1..=100).contains(&ratio) {
                return Err(invalid("prediction combo leg ratio must be between 1 and 100"));
            }
            encoded.push(crate::trade_proto::qot_common::ComboLeg {
                security: security(&security_code),
                side: Some(side),
                qty_ratio: Some(ratio as f64),
                position_id: None,
                pred_side: Some(pred_side),
            });
        }
        Ok(Self { mvc: mvc.to_owned(), legs: encoded })
    }

    pub(super) fn encode(&self) -> Vec<u8> {
        crate::trade_proto::qot_get_event_contract_combo_rfq::Request {
            c2s: crate::trade_proto::qot_get_event_contract_combo_rfq::C2s {
                combo_leg_list: self.legs.clone(),
                mvc: self.mvc.clone(),
            },
        }
        .encode_to_vec()
    }
}

pub(super) fn bytes_to_hex(value: &[u8]) -> String {
    const HEX: &[u8; 16] = b"0123456789abcdef";
    let mut output = String::with_capacity(value.len() * 2);
    for byte in value {
        output.push(HEX[(byte >> 4) as usize] as char);
        output.push(HEX[(byte & 0x0f) as usize] as char);
    }
    output
}
