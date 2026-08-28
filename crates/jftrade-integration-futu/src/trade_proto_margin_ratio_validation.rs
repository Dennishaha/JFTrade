use crate::trade_proto::{ResponseError, trd_get_margin_ratio};
use crate::trade_proto_validation::{validate_optional_finite, validate_required_text};

pub(crate) fn validate_margin_ratio_s2c(
    operation: &'static str,
    payload: &trd_get_margin_ratio::S2c,
) -> Result<(), ResponseError> {
    for info in &payload.margin_ratio_info_list {
        validate_required_text(operation, "security.code", &info.security.code)?;
        for (field, value) in [
            ("short_pool_remain", info.short_pool_remain),
            ("short_fee_rate", info.short_fee_rate),
            ("alert_long_ratio", info.alert_long_ratio),
            ("alert_short_ratio", info.alert_short_ratio),
            ("im_long_ratio", info.im_long_ratio),
            ("im_short_ratio", info.im_short_ratio),
            ("mcm_long_ratio", info.mcm_long_ratio),
            ("mcm_short_ratio", info.mcm_short_ratio),
            ("mm_long_ratio", info.mm_long_ratio),
            ("mm_short_ratio", info.mm_short_ratio),
        ] {
            validate_optional_finite(operation, field, value)?;
        }
    }
    Ok(())
}
