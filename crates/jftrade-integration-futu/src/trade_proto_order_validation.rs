use time::{Date, Month, Time};

use crate::trade_proto::{ResponseError, ValidationError, trd_get_order_list};
use crate::trade_proto_validation::{
    validate_non_negative, validate_optional_finite, validate_optional_non_negative,
    validate_required_text,
};

fn is_opend_time(value: &str) -> bool {
    let bytes = value.as_bytes();
    let has_milliseconds = bytes.len() == 23;
    if bytes.len() != 19 && !has_milliseconds {
        return false;
    }
    for (index, separator) in [(4, b'-'), (7, b'-'), (10, b' '), (13, b':'), (16, b':')] {
        if bytes[index] != separator {
            return false;
        }
    }
    if has_milliseconds && bytes[19] != b'.' {
        return false;
    }
    let digits = |start: usize, end: usize| bytes[start..end].iter().all(u8::is_ascii_digit);
    if !digits(0, 4)
        || !digits(5, 7)
        || !digits(8, 10)
        || !digits(11, 13)
        || !digits(14, 16)
        || !digits(17, 19)
    {
        return false;
    }
    if has_milliseconds && !digits(20, 23) {
        return false;
    }
    let parse = |start: usize, end: usize| value[start..end].parse::<u8>().ok();
    let year = match value[0..4].parse::<i32>() {
        Ok(year) => year,
        Err(_) => return false,
    };
    let month = match parse(5, 7).and_then(|month| Month::try_from(month).ok()) {
        Some(month) => month,
        None => return false,
    };
    let day = match parse(8, 10) {
        Some(day) => day,
        None => return false,
    };
    let hour = match parse(11, 13) {
        Some(hour) => hour,
        None => return false,
    };
    let minute = match parse(14, 16) {
        Some(minute) => minute,
        None => return false,
    };
    let second = match parse(17, 19) {
        Some(second) => second,
        None => return false,
    };
    Date::from_calendar_date(year, month, day).is_ok()
        && Time::from_hms(hour, minute, second).is_ok()
}

pub(crate) fn validate_opend_time(
    operation: &'static str,
    field: &'static str,
    value: &str,
) -> Result<(), ResponseError> {
    if is_opend_time(value) {
        Ok(())
    } else {
        Err(ValidationError::InvalidTime { operation, field }.into())
    }
}

pub(crate) fn validate_order_s2c(
    operation: &'static str,
    payload: &trd_get_order_list::S2c,
) -> Result<(), ResponseError> {
    for order in &payload.order_list {
        if order.order_id == 0 {
            return Err(ValidationError::EmptyField {
                operation,
                field: "order_id",
            }
            .into());
        }
        validate_required_text(operation, "order_id_ex", &order.order_id_ex)?;
        validate_required_text(operation, "code", &order.code)?;
        validate_required_text(operation, "name", &order.name)?;
        validate_opend_time(operation, "create_time", &order.create_time)?;
        validate_opend_time(operation, "update_time", &order.update_time)?;
        if let Some(expire_time) = order.expire_time.as_deref() {
            validate_opend_time(operation, "expire_time", expire_time)?;
        }
        validate_non_negative(operation, "qty", order.qty)?;
        for (field, value) in [
            ("price", order.price),
            ("fill_qty", order.fill_qty),
            ("fill_avg_price", order.fill_avg_price),
            ("aux_price", order.aux_price),
            ("trail_value", order.trail_value),
            ("trail_spread", order.trail_spread),
            ("order_amount", order.order_amount),
        ] {
            validate_optional_non_negative(operation, field, value)?;
        }
        for (field, value) in [
            ("create_timestamp", order.create_timestamp),
            ("update_timestamp", order.update_timestamp),
        ] {
            validate_optional_finite(operation, field, value)?;
        }
    }
    Ok(())
}
