//! Generated Futu trade protobuf modules and strict response validation.

pub mod common {
    include!(concat!(env!("OUT_DIR"), "/common.rs"));
}
pub mod qot_common {
    include!(concat!(env!("OUT_DIR"), "/qot_common.rs"));
}
pub mod qot_get_security_snapshot {
    include!(concat!(env!("OUT_DIR"), "/qot_get_security_snapshot.rs"));
}
pub mod qot_get_option_expiration_date {
    include!(concat!(
        env!("OUT_DIR"),
        "/qot_get_option_expiration_date.rs"
    ));
    pub const PROTOCOL_ID: u32 = 3224;
}
pub mod qot_get_option_chain {
    include!(concat!(env!("OUT_DIR"), "/qot_get_option_chain.rs"));
    pub const PROTOCOL_ID: u32 = 3209;
}
pub mod qot_get_option_quote {
    include!(concat!(env!("OUT_DIR"), "/qot_get_option_quote.rs"));
    pub const PROTOCOL_ID: u32 = 3255;
}
pub mod qot_option_screen {
    include!(concat!(env!("OUT_DIR"), "/qot_option_screen.rs"));
    pub const PROTOCOL_ID: u32 = 3253;
}
pub mod trd_common {
    include!(concat!(env!("OUT_DIR"), "/trd_common.rs"));
}

use crate::trade_proto_fee_validation::validate_order_fee_s2c;
use crate::trade_proto_fill_validation::validate_fill_s2c;
use crate::trade_proto_margin_ratio_validation::validate_margin_ratio_s2c;
use crate::trade_proto_max_qty_validation::validate_max_trade_quantity_s2c;
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
    #[error("OpenD response missing required maxTrdQtys")]
    MissingMaxTradeQuantity,
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
    #[error("OpenD {operation} field {field} has an unsupported value")]
    UnsupportedValue {
        operation: &'static str,
        field: String,
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
trade_list_proto!(
    trd_get_order_fee,
    "trd_get_order_fee.rs",
    "GetOrderFee",
    2225,
    validate_order_fee_s2c
);
trade_list_proto!(
    trd_get_margin_ratio,
    "trd_get_margin_ratio.rs",
    "GetMarginRatio",
    2223,
    validate_margin_ratio_s2c
);
pub mod trd_get_max_trd_qtys {
    use prost::Message;

    include!(concat!(env!("OUT_DIR"), "/trd_get_max_trd_qtys.rs"));

    pub const PROTOCOL_ID: u32 = 2111;

    pub fn encode_request(request: &Request) -> Vec<u8> {
        request.encode_to_vec()
    }

    pub fn decode_response(body: &[u8]) -> Result<S2c, super::ResponseError> {
        let response = Response::decode(body).map_err(|error| super::ResponseError::Decode {
            operation: "GetMaxTrdQtys",
            message: error.to_string(),
        })?;
        super::validate_response_for(
            response.ret_type,
            response.err_code,
            response.ret_msg.as_deref(),
            response.s2c.is_some(),
        )?;
        let payload = response.s2c.ok_or(super::ResponseError::MissingS2c)?;
        if payload.max_trd_qtys.is_none() {
            return Err(super::ResponseError::MissingMaxTradeQuantity);
        }
        super::validate_max_trade_quantity_s2c("GetMaxTrdQtys", &payload)?;
        Ok(payload)
    }
}

#[cfg(test)]
#[path = "trade_proto_tests.rs"]
mod tests;
