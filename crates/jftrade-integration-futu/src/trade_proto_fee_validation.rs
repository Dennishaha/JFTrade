use crate::trade_proto::{ResponseError, trd_get_order_fee};
use crate::trade_proto_validation::{
    validate_finite, validate_optional_finite, validate_required_text,
};

pub(crate) fn validate_order_fee_s2c(
    operation: &'static str,
    payload: &trd_get_order_fee::S2c,
) -> Result<(), ResponseError> {
    for fee in &payload.order_fee_list {
        validate_required_text(operation, "order_id_ex", &fee.order_id_ex)?;
        validate_optional_finite(operation, "fee_amount", fee.fee_amount)?;
        for item in &fee.fee_list {
            validate_finite(operation, "value", item.value.unwrap_or_default())?;
        }
    }
    Ok(())
}
