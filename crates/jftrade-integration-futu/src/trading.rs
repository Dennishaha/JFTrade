use std::str::FromStr;

use jftrade_kernel::Fixed8;
use jftrade_trading::BrokerOrderEvent;
use serde::{Deserialize, Serialize};
use thiserror::Error;

#[derive(Clone, Copy, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum TradeProtocol {
    GetAccountList,
    UnlockTrade,
    SubscribeAccountPush,
    GetFunds,
    GetPositionList,
    GetOrderList,
    PlaceOrder,
    ModifyOrder,
    GetOrderFillList,
    GetHistoryOrderList,
    GetHistoryOrderFillList,
    GetOrderFee,
    PlaceComboOrder,
    UpdateOrder,
    UpdateOrderFill,
}

impl TradeProtocol {
    pub const fn id(self) -> u32 {
        match self {
            Self::GetAccountList => 2001,
            Self::UnlockTrade => 2005,
            Self::SubscribeAccountPush => 2008,
            Self::GetFunds => 2101,
            Self::GetPositionList => 2102,
            Self::GetOrderList => 2201,
            Self::PlaceOrder => 2202,
            Self::ModifyOrder => 2205,
            Self::UpdateOrder => 2208,
            Self::GetOrderFillList => 2211,
            Self::UpdateOrderFill => 2218,
            Self::GetHistoryOrderList => 2221,
            Self::GetHistoryOrderFillList => 2222,
            Self::GetOrderFee => 2225,
            Self::PlaceComboOrder => 2227,
        }
    }

    pub const fn is_write(self) -> bool {
        matches!(
            self,
            Self::UnlockTrade | Self::PlaceOrder | Self::ModifyOrder | Self::PlaceComboOrder
        )
    }
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct TradeProtocolPlan {
    pub protocol: TradeProtocol,
    pub protocol_id: u32,
    pub dispatch: bool,
    pub read_only: bool,
}

pub fn plan_shadow_protocol(
    protocol: TradeProtocol,
) -> Result<TradeProtocolPlan, TradeProtocolError> {
    if protocol.is_write() {
        return Err(TradeProtocolError::WriteForbidden(protocol.id()));
    }
    Ok(TradeProtocolPlan {
        protocol,
        protocol_id: protocol.id(),
        dispatch: false,
        read_only: true,
    })
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct RawOrderUpdate {
    pub event_id: String,
    pub trace_id: String,
    pub broker_order_id: String,
    pub sequence: u64,
    pub status: String,
    pub fill_id: Option<String>,
    pub fill_quantity: Option<String>,
    pub occurred_at: String,
}

pub fn map_order_update(raw: RawOrderUpdate) -> Result<BrokerOrderEvent, TradeProtocolError> {
    let fill_quantity = raw
        .fill_quantity
        .as_deref()
        .map(Fixed8::from_str)
        .transpose()
        .map_err(|_| TradeProtocolError::InvalidFillQuantity)?;
    if raw.fill_id.is_some() != fill_quantity.is_some() {
        return Err(TradeProtocolError::IncompleteFill);
    }
    if raw
        .fill_id
        .as_deref()
        .is_some_and(|fill_id| fill_id.trim().is_empty())
        || fill_quantity.is_some_and(|quantity| quantity.signum() <= 0)
    {
        return Err(TradeProtocolError::InvalidFillQuantity);
    }
    let occurred_at = raw
        .occurred_at
        .parse()
        .map_err(|_| TradeProtocolError::InvalidOccurredAt)?;
    Ok(BrokerOrderEvent {
        event_id: raw.event_id,
        trace_id: raw.trace_id,
        broker_order_id: raw.broker_order_id,
        sequence: raw.sequence,
        raw_status: raw.status,
        fill_id: raw.fill_id,
        fill_quantity,
        occurred_at,
    })
}

#[derive(Debug, Error, Eq, PartialEq)]
pub enum TradeProtocolError {
    #[error("OpenD trade write protocol {0} is forbidden in Stage 5 shadow mode")]
    WriteForbidden(u32),
    #[error("OpenD fill quantity is not valid Fixed8 data")]
    InvalidFillQuantity,
    #[error("OpenD fill id and quantity must be present together")]
    IncompleteFill,
    #[error("OpenD order update time is not valid RFC3339")]
    InvalidOccurredAt,
}

#[cfg(test)]
mod tests {
    use super::{
        RawOrderUpdate, TradeProtocol, TradeProtocolError, map_order_update, plan_shadow_protocol,
    };

    #[test]
    fn protocol_ids_match_go_opend_and_shadow_forbids_writes() {
        assert_eq!(TradeProtocol::GetAccountList.id(), 2001);
        assert_eq!(TradeProtocol::PlaceOrder.id(), 2202);
        assert_eq!(TradeProtocol::ModifyOrder.id(), 2205);
        assert_eq!(TradeProtocol::UpdateOrderFill.id(), 2218);
        assert_eq!(
            plan_shadow_protocol(TradeProtocol::PlaceOrder),
            Err(TradeProtocolError::WriteForbidden(2202))
        );
        let read = plan_shadow_protocol(TradeProtocol::GetPositionList).expect("read plan");
        assert!(read.read_only && !read.dispatch);
    }

    #[test]
    fn mapper_rejects_partial_or_malformed_fill_fields() {
        let raw = RawOrderUpdate {
            event_id: "event-1".to_owned(),
            trace_id: "trace-1".to_owned(),
            broker_order_id: "order-1".to_owned(),
            sequence: 1,
            status: "FILLED_PART".to_owned(),
            fill_id: Some("fill-1".to_owned()),
            fill_quantity: None,
            occurred_at: "now".to_owned(),
        };
        assert_eq!(
            map_order_update(raw),
            Err(TradeProtocolError::IncompleteFill)
        );
    }

    #[test]
    fn mapper_requires_rfc3339_event_time() {
        let raw = RawOrderUpdate {
            event_id: "event-1".to_owned(),
            trace_id: "trace-1".to_owned(),
            broker_order_id: "order-1".to_owned(),
            sequence: 1,
            status: "NEW".to_owned(),
            fill_id: None,
            fill_quantity: None,
            occurred_at: "not-a-time".to_owned(),
        };
        assert_eq!(
            map_order_update(raw),
            Err(TradeProtocolError::InvalidOccurredAt)
        );
    }
}
