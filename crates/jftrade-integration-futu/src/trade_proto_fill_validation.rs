use crate::trade_proto::{ResponseError, ValidationError, trd_get_order_fill_list};
use crate::trade_proto_order_validation::validate_opend_time;
use crate::trade_proto_validation::{
    validate_non_negative, validate_optional_finite, validate_required_text,
};

pub(crate) fn validate_fill_s2c(
    operation: &'static str,
    payload: &trd_get_order_fill_list::S2c,
) -> Result<(), ResponseError> {
    for fill in &payload.order_fill_list {
        if fill.fill_id == 0 {
            return Err(ValidationError::EmptyField {
                operation,
                field: "fill_id",
            }
            .into());
        }
        validate_required_text(operation, "fill_id_ex", &fill.fill_id_ex)?;
        validate_required_text(operation, "code", &fill.code)?;
        validate_required_text(operation, "name", &fill.name)?;
        validate_opend_time(operation, "create_time", &fill.create_time)?;
        if let Some(order_id) = fill.order_id {
            if order_id == 0 {
                return Err(ValidationError::EmptyField {
                    operation,
                    field: "order_id",
                }
                .into());
            }
        }
        if let Some(order_id_ex) = fill.order_id_ex.as_deref() {
            validate_required_text(operation, "order_id_ex", order_id_ex)?;
        }
        validate_non_negative(operation, "qty", fill.qty)?;
        validate_non_negative(operation, "price", fill.price)?;
        for (field, value) in [
            ("create_timestamp", fill.create_timestamp),
            ("update_timestamp", fill.update_timestamp),
        ] {
            validate_optional_finite(operation, field, value)?;
        }
    }
    Ok(())
}
