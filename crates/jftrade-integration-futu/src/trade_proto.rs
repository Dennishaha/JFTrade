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

macro_rules! trade_list_proto {
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
                Ok(response.s2c.unwrap_or_default())
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
                Ok(response.s2c.and_then(|s2c| s2c.funds).unwrap_or_default())
            }
        }
    };
}

trade_list_proto!(
    trd_get_acc_list,
    "trd_get_acc_list.rs",
    "GetAccountList",
    2001
);
trade_funds_proto!(trd_get_funds, "trd_get_funds.rs", "GetFunds", 2101);
trade_list_proto!(
    trd_get_position_list,
    "trd_get_position_list.rs",
    "GetPositionList",
    2102
);
trade_list_proto!(
    trd_get_order_list,
    "trd_get_order_list.rs",
    "GetOrderList",
    2201
);
trade_list_proto!(
    trd_get_order_fill_list,
    "trd_get_order_fill_list.rs",
    "GetOrderFillList",
    2211
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

    use super::{ResponseError, trd_common, trd_get_acc_list, trd_get_funds};

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
}
