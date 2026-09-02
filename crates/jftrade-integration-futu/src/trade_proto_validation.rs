use crate::trade_proto::{
    ResponseError, ValidationError, trd_common, trd_flow_summary, trd_get_acc_list,
    trd_get_position_list,
};

pub(crate) fn validate_finite(
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

pub(crate) fn validate_optional_finite(
    operation: &'static str,
    field: &'static str,
    value: Option<f64>,
) -> Result<(), ResponseError> {
    if let Some(value) = value {
        validate_finite(operation, field, value)?;
    }
    Ok(())
}

pub(crate) fn validate_required_text(
    operation: &'static str,
    field: &'static str,
    value: &str,
) -> Result<(), ResponseError> {
    if value.trim().is_empty() {
        return Err(ValidationError::EmptyField { operation, field }.into());
    }
    Ok(())
}

pub(crate) fn validate_non_negative(
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

pub(crate) fn validate_optional_non_negative(
    operation: &'static str,
    field: &'static str,
    value: Option<f64>,
) -> Result<(), ResponseError> {
    if let Some(value) = value {
        validate_non_negative(operation, field, value)?;
    }
    Ok(())
}

pub(crate) fn validate_account_s2c(
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

pub(crate) fn validate_position_s2c(
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
            ("pl_ratio", position.pl_ratio),
            ("td_pl_val", position.td_pl_val),
            ("td_trd_val", position.td_trd_val),
            ("td_buy_val", position.td_buy_val),
            ("td_sell_val", position.td_sell_val),
            ("unrealized_pl", position.unrealized_pl),
            ("realized_pl", position.realized_pl),
            ("average_pl_ratio", position.average_pl_ratio),
        ] {
            validate_optional_finite(operation, field, value)?;
        }
        for (field, value) in [
            ("cost_price", position.cost_price),
            ("diluted_cost_price", position.diluted_cost_price),
            ("average_cost_price", position.average_cost_price),
        ] {
            validate_optional_non_negative(operation, field, value)?;
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

pub(crate) fn validate_funds(
    operation: &'static str,
    funds: &trd_common::Funds,
) -> Result<(), ResponseError> {
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
        avl_withdrawal_cash
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

pub(crate) fn validate_cash_flow_s2c(
    operation: &'static str,
    payload: &trd_flow_summary::S2c,
) -> Result<(), ResponseError> {
    for flow in &payload.flow_summary_info_list {
        if let Some(amount) = flow.cash_flow_amount {
            validate_finite(operation, "cash_flow_amount", amount)?;
        }
    }
    Ok(())
}
