//! Generated Futu trade protobuf modules and strict response validation.

pub mod common {
    include!(concat!(env!("OUT_DIR"), "/common.rs"));
}
pub mod qot_common {
    include!(concat!(env!("OUT_DIR"), "/qot_common.rs"));
}
pub mod trd_common {
    include!(concat!(env!("OUT_DIR"), "/trd_common.rs"));
}

use crate::trade_proto_fill_validation::validate_fill_s2c;
use crate::trade_proto_order_validation::validate_order_s2c;
use crate::trade_proto_validation::{
    validate_account_s2c, validate_cash_flow_s2c, validate_funds, validate_position_s2c,
};

#[derive(Debug, thiserror::Error, PartialEq, Eq)]
pub enum ResponseError {
    #[error("OpenD retType={ret_type} errCode={err_code}: {message}")]
    ReturnCode {
        ret_type: i32,
        err_code: i32,
        message: String,
    },
    #[error("OpenD response missing required s2c")]
    MissingS2c,
    #[error("failed to decode OpenD {operation} response: {message}")]
    Decode {
        operation: &'static str,
        message: String,
    },
    #[error("OpenD {operation} field {field} must be a YYYY-MM-DD HH:MM:SS[.MS] timestamp")]
    InvalidTime {
        operation: &'static str,
        field: &'static str,
    },
    #[error("{0}")]
    Validation(#[from] ValidationError),
}

#[derive(Debug, thiserror::Error, PartialEq, Eq)]
pub enum ValidationError {
    #[error("OpenD {operation} field {field} must be finite")]
    NonFinite {
        operation: &'static str,
        field: String,
    },
    #[error("OpenD {operation} field {field} must not be empty")]
    EmptyField {
        operation: &'static str,
        field: &'static str,
    },
    #[error("OpenD {operation} field {field} must be non-negative")]
    Negative {
        operation: &'static str,
        field: &'static str,
    },
    #[error("OpenD {operation} field {field} must be a YYYY-MM-DD HH:MM:SS[.MS] timestamp")]
    InvalidTime {
        operation: &'static str,
        field: &'static str,
    },
}

fn validate_response_for(
    ret_type: i32,
    err_code: Option<i32>,
    ret_msg: Option<&str>,
    has_s2c: bool,
) -> Result<(), ResponseError> {
    if ret_type != 0 {
        return Err(ResponseError::ReturnCode {
            ret_type,
            err_code: err_code.unwrap_or_default(),
            message: ret_msg.unwrap_or_default().to_owned(),
        });
    }
    if !has_s2c {
        return Err(ResponseError::MissingS2c);
    }
    Ok(())
}

macro_rules! trade_list_proto {
    (
        $module:ident,
        $file:literal,
        $operation:literal,
        $protocol_id:literal,
        $validator:ident
    ) => {
        pub mod $module {
            use prost::Message;

            include!(concat!(env!("OUT_DIR"), "/", $file));

            /// Futu OpenD protocol identifier for this operation.
            pub const PROTOCOL_ID: u32 = $protocol_id;

            /// Encodes a typed request body for the OpenD frame payload.
            pub fn encode_request(request: &Request) -> Vec<u8> {
                request.encode_to_vec()
            }

            /// Decodes and validates a typed OpenD response, returning its S2C payload.
            pub fn decode_response(body: &[u8]) -> Result<S2c, super::ResponseError> {
                let response =
                    Response::decode(body).map_err(|error| super::ResponseError::Decode {
                        operation: $operation,
                        message: error.to_string(),
                    })?;
                super::validate_response_for(
                    response.ret_type,
                    response.err_code,
                    response.ret_msg.as_deref(),
                    true,
                )?;
                let payload = response.s2c.unwrap_or_default();
                super::$validator($operation, &payload)?;
                Ok(payload)
            }
        }
    };
}

macro_rules! trade_funds_proto {
    ($module:ident, $file:literal, $operation:literal, $protocol_id:literal) => {
        pub mod $module {
            use prost::Message;

            include!(concat!(env!("OUT_DIR"), "/", $file));

            /// Futu OpenD protocol identifier for this operation.
            pub const PROTOCOL_ID: u32 = $protocol_id;

            /// Encodes a typed request body for the OpenD frame payload.
            pub fn encode_request(request: &Request) -> Vec<u8> {
                request.encode_to_vec()
            }

            /// Decodes the funds projection, normalizing absent S2C/funds to zero values.
            pub fn decode_response(
                body: &[u8],
            ) -> Result<super::trd_common::Funds, super::ResponseError> {
                let response =
                    Response::decode(body).map_err(|error| super::ResponseError::Decode {
                        operation: $operation,
                        message: error.to_string(),
                    })?;
                super::validate_response_for(
                    response.ret_type,
                    response.err_code,
                    response.ret_msg.as_deref(),
                    true,
                )?;
                let funds = response.s2c.and_then(|s2c| s2c.funds).unwrap_or_default();
                super::validate_funds($operation, &funds)?;
                Ok(funds)
            }
        }
    };
}

trade_list_proto!(
    trd_get_acc_list,
    "trd_get_acc_list.rs",
    "GetAccountList",
    2001,
    validate_account_s2c
);

trade_list_proto!(
    trd_flow_summary,
    "trd_flow_summary.rs",
    "FlowSummary",
    2226,
    validate_cash_flow_s2c
);
trade_funds_proto!(trd_get_funds, "trd_get_funds.rs", "GetFunds", 2101);
trade_list_proto!(
    trd_get_position_list,
    "trd_get_position_list.rs",
    "GetPositionList",
    2102,
    validate_position_s2c
);
trade_list_proto!(
    trd_get_order_list,
    "trd_get_order_list.rs",
    "GetOrderList",
    2201,
    validate_order_s2c
);
trade_list_proto!(
    trd_get_order_fill_list,
    "trd_get_order_fill_list.rs",
    "GetOrderFillList",
    2211,
    validate_fill_s2c
);

#[cfg(test)]
mod tests {
    use prost::Message;

    use super::{
        ResponseError, ValidationError, trd_common, trd_flow_summary, trd_get_acc_list,
        trd_get_funds, trd_get_order_fill_list, trd_get_order_list, trd_get_position_list,
    };

    fn header() -> trd_common::TrdHeader {
        trd_common::TrdHeader {
            trd_env: 0,
            acc_id: 42,
            trd_market: 1,
            jp_acc_type: None,
        }
    }

    #[test]
    fn account_list_request_round_trips_through_typed_encoder() {
        let request = trd_get_acc_list::Request {
            c2s: trd_get_acc_list::C2s {
                user_id: 0,
                trd_category: Some(1),
                need_general_sec_account: Some(true),
            },
        };
        let encoded = trd_get_acc_list::encode_request(&request);
        let decoded = trd_get_acc_list::Request::decode(encoded.as_slice()).expect("request");
        assert_eq!(decoded, request);
        assert_eq!(trd_get_acc_list::PROTOCOL_ID, 2001);
    }

    #[test]
    fn funds_response_decodes_typed_s2c() {
        let response = trd_get_funds::Response {
            ret_type: 0,
            ret_msg: Some("ok".to_owned()),
            err_code: Some(0),
            s2c: Some(trd_get_funds::S2c {
                header: header(),
                funds: Some(trd_common::Funds {
                    power: 123.0,
                    ..Default::default()
                }),
            }),
        };
        let decoded = trd_get_funds::decode_response(&response.encode_to_vec()).expect("response");
        assert_eq!(decoded.power, 123.0);
    }

    #[test]
    fn return_code_is_mapped_before_payload_validation() {
        let response = trd_get_acc_list::Response {
            ret_type: -1,
            ret_msg: Some("account unavailable".to_owned()),
            err_code: Some(1101),
            s2c: None,
        };
        assert_eq!(
            trd_get_acc_list::decode_response(&response.encode_to_vec()),
            Err(ResponseError::ReturnCode {
                ret_type: -1,
                err_code: 1101,
                message: "account unavailable".to_owned(),
            })
        );
    }

    #[test]
    fn list_missing_s2c_normalizes_to_an_empty_result() {
        let response = trd_get_acc_list::Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: None,
        };
        let decoded =
            trd_get_acc_list::decode_response(&response.encode_to_vec()).expect("response");
        assert!(decoded.acc_list.is_empty());
    }

    #[test]
    fn funds_missing_s2c_normalizes_to_an_empty_snapshot() {
        let response = trd_get_funds::Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: None,
        };
        let decoded = trd_get_funds::decode_response(&response.encode_to_vec()).expect("response");
        assert_eq!(decoded, trd_common::Funds::default());
    }

    fn funds_response(funds: trd_common::Funds) -> trd_get_funds::Response {
        trd_get_funds::Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(trd_get_funds::S2c {
                header: header(),
                funds: Some(funds),
            }),
        }
    }

    #[test]
    fn funds_validation_accepts_zero_values_and_present_finite_optionals() {
        let funds = trd_common::Funds {
            power: 0.0,
            available_funds: Some(12.5),
            cash_info_list: vec![trd_common::AccCashInfo {
                cash: Some(0.0),
                available_balance: Some(1.0),
                net_cash_power: Some(2.0),
                ..Default::default()
            }],
            ..Default::default()
        };
        let decoded = trd_get_funds::decode_response(&funds_response(funds).encode_to_vec())
            .expect("finite funds");
        assert_eq!(decoded.power, 0.0);
        assert_eq!(decoded.available_funds, Some(12.5));
    }

    #[test]
    fn funds_validation_rejects_non_finite_required_and_optional_values() {
        for (field, value) in [("power", f64::NAN), ("cash", f64::INFINITY)] {
            let mut funds = trd_common::Funds::default();
            match field {
                "power" => funds.power = value,
                "cash" => funds.cash = value,
                _ => unreachable!(),
            }
            assert_eq!(
                trd_get_funds::decode_response(&funds_response(funds).encode_to_vec()),
                Err(ResponseError::Validation(ValidationError::NonFinite {
                    operation: "GetFunds",
                    field: field.to_owned(),
                }))
            );
        }

        let funds = trd_common::Funds {
            available_funds: Some(f64::NEG_INFINITY),
            ..Default::default()
        };
        assert!(matches!(
            trd_get_funds::decode_response(&funds_response(funds).encode_to_vec()),
            Err(ResponseError::Validation(ValidationError::NonFinite {
                field,
                operation: "GetFunds"
            })) if field == "available_funds"
        ));
    }

    #[test]
    fn account_validation_allows_unknown_enums_with_card_identity() {
        let response = trd_get_acc_list::Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(trd_get_acc_list::S2c {
                acc_list: vec![trd_common::TrdAcc {
                    trd_env: 999,
                    acc_id: 0,
                    card_num: Some("card-1".to_owned()),
                    acc_type: Some(999),
                    ..Default::default()
                }],
            }),
        };
        let decoded = trd_get_acc_list::decode_response(&response.encode_to_vec())
            .expect("unknown enums are compatible");
        assert_eq!(decoded.acc_list.len(), 1);
    }

    #[test]
    fn account_validation_rejects_missing_identity() {
        let response = trd_get_acc_list::Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(trd_get_acc_list::S2c {
                acc_list: vec![trd_common::TrdAcc::default()],
            }),
        };
        assert_eq!(
            trd_get_acc_list::decode_response(&response.encode_to_vec()),
            Err(ResponseError::Validation(ValidationError::EmptyField {
                operation: "GetAccountList",
                field: "account_identity",
            }))
        );
    }

    fn position_response(position: trd_common::Position) -> trd_get_position_list::Response {
        trd_get_position_list::Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(trd_get_position_list::S2c {
                header: header(),
                position_list: vec![position],
            }),
        }
    }

    fn valid_position() -> trd_common::Position {
        trd_common::Position {
            position_id: 1,
            position_side: 999,
            code: "US.AAPL".to_owned(),
            name: "Apple".to_owned(),
            qty: 0.0,
            can_sell_qty: 0.0,
            price: 0.0,
            val: 0.0,
            pl_val: 0.0,
            ..Default::default()
        }
    }

    #[test]
    fn position_validation_accepts_zero_values_and_unknown_enum() {
        let decoded = trd_get_position_list::decode_response(
            &position_response(valid_position()).encode_to_vec(),
        )
        .expect("valid zero position");
        assert_eq!(decoded.position_list[0].qty, 0.0);
        assert_eq!(decoded.position_list[0].position_side, 999);
    }

    #[test]
    fn position_validation_rejects_empty_identity_and_negative_quantity() {
        let mut position = valid_position();
        position.code.clear();
        assert!(matches!(
            trd_get_position_list::decode_response(&position_response(position).encode_to_vec()),
            Err(ResponseError::Validation(ValidationError::EmptyField {
                field: "code",
                operation: "GetPositionList"
            }))
        ));

        let mut position = valid_position();
        position.qty = -1.0;
        assert!(matches!(
            trd_get_position_list::decode_response(&position_response(position).encode_to_vec()),
            Err(ResponseError::Validation(ValidationError::Negative {
                field: "qty",
                operation: "GetPositionList"
            }))
        ));

        let mut position = valid_position();
        position.cost_price = Some(-0.01);
        assert!(matches!(
            trd_get_position_list::decode_response(&position_response(position).encode_to_vec()),
            Err(ResponseError::Validation(ValidationError::Negative {
                field: "cost_price",
                operation: "GetPositionList"
            }))
        ));
    }

    #[test]
    fn position_validation_rejects_non_finite_optional_values() {
        let mut position = valid_position();
        position.average_cost_price = Some(f64::NAN);
        assert!(matches!(
            trd_get_position_list::decode_response(&position_response(position).encode_to_vec()),
            Err(ResponseError::Validation(ValidationError::NonFinite {
                field,
                operation: "GetPositionList"
            })) if field == "average_cost_price"
        ));
    }

    fn order_response(order: trd_common::Order) -> trd_get_order_list::Response {
        trd_get_order_list::Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(trd_get_order_list::S2c {
                header: header(),
                order_list: vec![order],
            }),
        }
    }

    fn valid_order() -> trd_common::Order {
        trd_common::Order {
            trd_side: 999,
            order_type: 999,
            order_status: 999,
            order_id: 1,
            order_id_ex: "EXT-1".to_owned(),
            code: "US.AAPL".to_owned(),
            name: "Apple".to_owned(),
            qty: 0.0,
            create_time: "2026-08-29 09:30:00".to_owned(),
            update_time: "2026-08-29 09:30:00.123".to_owned(),
            ..Default::default()
        }
    }

    #[test]
    fn order_validation_accepts_zero_values_unknown_enums_and_millisecond_time() {
        let decoded =
            trd_get_order_list::decode_response(&order_response(valid_order()).encode_to_vec())
                .expect("valid order");
        assert_eq!(decoded.order_list[0].qty, 0.0);
        assert_eq!(decoded.order_list[0].order_status, 999);
    }

    #[test]
    fn order_validation_rejects_missing_identity_and_bad_time() {
        let mut order = valid_order();
        order.order_id_ex.clear();
        assert!(matches!(
            trd_get_order_list::decode_response(&order_response(order).encode_to_vec()),
            Err(ResponseError::Validation(ValidationError::EmptyField {
                field: "order_id_ex",
                operation: "GetOrderList"
            }))
        ));

        let mut order = valid_order();
        order.create_time = "2026-02-29 09:30:00".to_owned();
        assert!(matches!(
            trd_get_order_list::decode_response(&order_response(order).encode_to_vec()),
            Err(ResponseError::Validation(ValidationError::InvalidTime {
                field: "create_time",
                operation: "GetOrderList"
            }))
        ));
    }

    #[test]
    fn order_validation_rejects_negative_and_non_finite_numbers() {
        let mut order = valid_order();
        order.qty = -1.0;
        assert!(matches!(
            trd_get_order_list::decode_response(&order_response(order).encode_to_vec()),
            Err(ResponseError::Validation(ValidationError::Negative {
                field: "qty",
                operation: "GetOrderList"
            }))
        ));

        let mut order = valid_order();
        order.price = Some(f64::NAN);
        assert!(matches!(
            trd_get_order_list::decode_response(&order_response(order).encode_to_vec()),
            Err(ResponseError::Validation(ValidationError::NonFinite {
                field,
                operation: "GetOrderList"
            })) if field == "price"
        ));
    }

    fn fill_response(fill: trd_common::OrderFill) -> trd_get_order_fill_list::Response {
        trd_get_order_fill_list::Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(trd_get_order_fill_list::S2c {
                header: header(),
                order_fill_list: vec![fill],
            }),
        }
    }

    fn valid_fill() -> trd_common::OrderFill {
        trd_common::OrderFill {
            trd_side: 999,
            fill_id: 1,
            fill_id_ex: "FILL-1".to_owned(),
            code: "US.AAPL".to_owned(),
            name: "Apple".to_owned(),
            qty: 0.0,
            price: 0.0,
            create_time: "2026-08-29 09:30:00.123".to_owned(),
            ..Default::default()
        }
    }

    #[test]
    fn fill_validation_accepts_zero_values_unknown_enum_and_optional_ids() {
        let fill = trd_common::OrderFill {
            order_id: Some(7),
            order_id_ex: Some("EXT-7".to_owned()),
            ..valid_fill()
        };
        let decoded =
            trd_get_order_fill_list::decode_response(&fill_response(fill).encode_to_vec())
                .expect("valid fill");
        assert_eq!(decoded.order_fill_list[0].qty, 0.0);
        assert_eq!(decoded.order_fill_list[0].trd_side, 999);
    }

    #[test]
    fn fill_validation_rejects_missing_identity_and_bad_time() {
        let mut fill = valid_fill();
        fill.fill_id_ex.clear();
        assert!(matches!(
            trd_get_order_fill_list::decode_response(&fill_response(fill).encode_to_vec()),
            Err(ResponseError::Validation(ValidationError::EmptyField {
                field: "fill_id_ex",
                operation: "GetOrderFillList"
            }))
        ));

        let mut fill = valid_fill();
        fill.create_time = "2026-02-29 09:30:00".to_owned();
        assert!(matches!(
            trd_get_order_fill_list::decode_response(&fill_response(fill).encode_to_vec()),
            Err(ResponseError::Validation(ValidationError::InvalidTime {
                field: "create_time",
                operation: "GetOrderFillList"
            }))
        ));
    }

    #[test]
    fn fill_validation_rejects_negative_and_non_finite_values() {
        let mut fill = valid_fill();
        fill.qty = -1.0;
        assert!(matches!(
            trd_get_order_fill_list::decode_response(&fill_response(fill).encode_to_vec()),
            Err(ResponseError::Validation(ValidationError::Negative {
                field: "qty",
                operation: "GetOrderFillList"
            }))
        ));

        let mut fill = valid_fill();
        fill.update_timestamp = Some(f64::INFINITY);
        assert!(matches!(
            trd_get_order_fill_list::decode_response(&fill_response(fill).encode_to_vec()),
            Err(ResponseError::Validation(ValidationError::NonFinite {
                field,
                operation: "GetOrderFillList"
            })) if field == "update_timestamp"
        ));
    }

    #[test]
    fn cash_flow_response_decodes_and_rejects_non_finite_amount() {
        let response = trd_flow_summary::Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(trd_flow_summary::S2c {
                header: header(),
                flow_summary_info_list: vec![trd_flow_summary::FlowSummaryInfo {
                    clearing_date: Some("2026-08-21".to_owned()),
                    cash_flow_amount: Some(42.5),
                    ..Default::default()
                }],
            }),
        };
        let decoded = trd_flow_summary::decode_response(&response.encode_to_vec()).expect("flow");
        assert_eq!(
            decoded.flow_summary_info_list[0].cash_flow_amount,
            Some(42.5)
        );

        let invalid = trd_flow_summary::Response {
            s2c: Some(trd_flow_summary::S2c {
                header: header(),
                flow_summary_info_list: vec![trd_flow_summary::FlowSummaryInfo {
                    cash_flow_amount: Some(f64::NAN),
                    ..Default::default()
                }],
            }),
            ..response
        };
        assert!(matches!(
            trd_flow_summary::decode_response(&invalid.encode_to_vec()),
            Err(ResponseError::Validation(ValidationError::NonFinite { field, operation: "FlowSummary" }))
                if field == "cash_flow_amount"
        ));
    }
}
