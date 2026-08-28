//! Generated Futu trade protobuf modules and strict response validation.

pub mod common { include!(concat!(env!("OUT_DIR"), "/common.rs")); }
pub mod qot_common { include!(concat!(env!("OUT_DIR"), "/qot_common.rs")); }
pub mod trd_common { include!(concat!(env!("OUT_DIR"), "/trd_common.rs")); }
pub mod trd_get_acc_list { include!(concat!(env!("OUT_DIR"), "/trd_get_acc_list.rs")); }
pub mod trd_get_funds { include!(concat!(env!("OUT_DIR"), "/trd_get_funds.rs")); }
pub mod trd_get_position_list { include!(concat!(env!("OUT_DIR"), "/trd_get_position_list.rs")); }
pub mod trd_get_order_list { include!(concat!(env!("OUT_DIR"), "/trd_get_order_list.rs")); }
pub mod trd_get_order_fill_list { include!(concat!(env!("OUT_DIR"), "/trd_get_order_fill_list.rs")); }

#[derive(Debug, thiserror::Error, PartialEq, Eq)]
pub enum ResponseError {
    #[error("OpenD retType={ret_type} errCode={err_code}: {message}")]
    ReturnCode { ret_type: i32, err_code: i32, message: String },
    #[error("OpenD response missing required s2c")]
    MissingS2c,
}

pub fn validate_response(ret_type: i32, err_code: Option<i32>, ret_msg: Option<&str>, has_s2c: bool) -> Result<(), ResponseError> {
    if ret_type != 0 { return Err(ResponseError::ReturnCode { ret_type, err_code: err_code.unwrap_or_default(), message: ret_msg.unwrap_or_default().to_owned() }); }
    if !has_s2c { return Err(ResponseError::MissingS2c); }
    Ok(())
}
