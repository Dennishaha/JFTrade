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
}

pub fn validate_response(
    ret_type: i32,
    err_code: Option<i32>,
    ret_msg: Option<&str>,
    has_s2c: bool,
) -> Result<(), ResponseError> {
    validate_response_for(ret_type, err_code, ret_msg, has_s2c)
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

fn validate_finite(
    operation: &'static str,
    field: impl Into<String>,
    value: f64,
) -> Result<(), ResponseError> {
    if value.is_finite() {
        Ok(())
    } else {
        Err(ValidationError::NonFinite {
            operation,
            field: field.into(),
        }
        .into())
    }
}

fn validate_optional_finite(
    operation: &'static str,
    field: &'static str,
    value: Option<f64>,
) -> Result<(), ResponseError> {
    if let Some(value) = value {
        validate_finite(operation, field, value)?;
    }
    Ok(())
}

fn validate_required_text(
    operation: &'static str,
    field: &'static str,
    value: &str,
) -> Result<(), ResponseError> {
    if value.trim().is_empty() {
        return Err(ValidationError::EmptyField { operation, field }.into());
    }
    Ok(())
}

fn validate_non_negative(
    operation: &'static str,
    field: &'static str,
    value: f64,
) -> Result<(), ResponseError> {
    validate_finite(operation, field, value)?;
    if value < 0.0 {
        return Err(ValidationError::Negative { operation, field }.into());
    }
    Ok(())
}

fn validate_account_s2c(
    operation: &'static str,
    payload: &trd_get_acc_list::S2c,
) -> Result<(), ResponseError> {
    for account in &payload.acc_list {
        if account.acc_id == 0 {
            let card = account.card_num.as_deref().unwrap_or_default();
            let universal = account.uni_card_num.as_deref().unwrap_or_default();
            if card.trim().is_empty() && universal.trim().is_empty() {
                return Err(ValidationError::EmptyField {
                    operation,
                    field: "account_identity",
                }
                .into());
            }
        }
    }
    Ok(())
}

fn validate_position_s2c(
    operation: &'static str,
    payload: &trd_get_position_list::S2c,
) -> Result<(), ResponseError> {
    for position in &payload.position_list {
        validate_required_text(operation, "code", &position.code)?;
        validate_required_text(operation, "name", &position.name)?;
        validate_non_negative(operation, "qty", position.qty)?;
        validate_non_negative(operation, "can_sell_qty", position.can_sell_qty)?;
        validate_non_negative(operation, "price", position.price)?;
        validate_finite(operation, "val", position.val)?;
        validate_finite(operation, "pl_val", position.pl_val)?;
        for (field, value) in [
            ("cost_price", position.cost_price),
            ("pl_ratio", position.pl_ratio),
            ("td_pl_val", position.td_pl_val),
            ("td_trd_val", position.td_trd_val),
            ("td_buy_val", position.td_buy_val),
            ("td_sell_val", position.td_sell_val),
            ("unrealized_pl", position.unrealized_pl),
            ("realized_pl", position.realized_pl),
            ("diluted_cost_price", position.diluted_cost_price),
            ("average_cost_price", position.average_cost_price),
            ("average_pl_ratio", position.average_pl_ratio),
        ] {
            validate_optional_finite(operation, field, value)?;
        }
        for (field, value) in [
            ("td_buy_qty", position.td_buy_qty),
            ("td_sell_qty", position.td_sell_qty),
            ("payout_if_win", position.payout_if_win),
        ] {
            if let Some(value) = value {
                validate_non_negative(operation, field, value)?;
            }
        }
    }
    Ok(())
}

fn validate_funds(operation: &'static str, funds: &trd_common::Funds) -> Result<(), ResponseError> {
    macro_rules! required {
        ($($field:ident),+ $(,)?) => {
            $(validate_finite(operation, stringify!($field), funds.$field)?;)+
        };
    }
    macro_rules! optional {
        ($($field:ident),+ $(,)?) => {
            $(validate_optional_finite(operation, stringify!($field), funds.$field)?;)+
        };
    }

    required!(
        power,
        total_assets,
        cash,
        market_val,
        frozen_cash,
        debt_cash,
        avl_withdrawal_cash,
    );
    optional!(
        available_funds,
        unrealized_pl,
        realized_pl,
        initial_margin,
        maintenance_margin,
        max_power_short,
        net_cash_power,
        long_mv,
        short_mv,
        pending_asset,
        max_withdrawal,
        margin_call_margin,
        beginning_dtbp,
        remaining_dtbp,
        dt_call_amount,
        securities_assets,
        fund_assets,
        bond_assets,
        crypto_mv,
        exposure_limit,
        used_limit,
        remaining_limit,
    );
    for cash in &funds.cash_info_list {
        validate_optional_finite(operation, "cash_info_list.cash", cash.cash)?;
        validate_optional_finite(
            operation,
            "cash_info_list.available_balance",
            cash.available_balance,
        )?;
        validate_optional_finite(
            operation,
            "cash_info_list.net_cash_power",
            cash.net_cash_power,
        )?;
    }
    for market in &funds.market_info_list {
        validate_optional_finite(operation, "market_info_list.assets", market.assets)?;
    }
    Ok(())
}

fn validate_noop_s2c<T>(_: &'static str, _: &T) -> Result<(), ResponseError> {
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
    validate_noop_s2c
);
trade_list_proto!(
    trd_get_order_fill_list,
    "trd_get_order_fill_list.rs",
    "GetOrderFillList",
    2211,
    validate_noop_s2c
);

/// Descriptive aliases for callers that use the Go operation names.
pub use trd_get_acc_list as get_account_list;
pub use trd_get_funds as get_funds;
pub use trd_get_order_fill_list as get_order_fill_list;
pub use trd_get_order_list as get_order_list;
pub use trd_get_position_list as get_position_list;

#[cfg(test)]
mod tests {
    use prost::Message;

    use super::{
        ResponseError, ValidationError, trd_common, trd_get_acc_list, trd_get_funds,
        trd_get_position_list,
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
}
