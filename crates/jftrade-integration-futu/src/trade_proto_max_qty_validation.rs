use crate::trade_proto::{ResponseError, ValidationError, trd_get_max_trd_qtys};
use crate::trade_proto_validation::{validate_non_negative, validate_optional_non_negative};

pub(crate) fn validate_max_trade_quantity_s2c(
    operation: &'static str,
    payload: &trd_get_max_trd_qtys::S2c,
) -> Result<(), ResponseError> {
    let Some(max) = payload.max_trd_qtys.as_ref() else {
        return Ok(());
    };
    validate_non_negative(operation, "max_cash_buy", max.max_cash_buy)?;
    validate_non_negative(operation, "max_position_sell", max.max_position_sell)?;
    for (field, value) in [
        ("max_cash_and_margin_buy", max.max_cash_and_margin_buy),
        ("max_sell_short", max.max_sell_short),
        ("max_buy_back", max.max_buy_back),
        ("long_required_im", max.long_required_im),
        ("short_required_im", max.short_required_im),
    ] {
        validate_optional_non_negative(operation, field, value)?;
    }
    if let Some(session) = max.session
        && !(0..=4).contains(&session)
    {
        return Err(ValidationError::UnsupportedValue {
            operation,
            field: format!("session={session}"),
        }
        .into());
    }
    Ok(())
}
