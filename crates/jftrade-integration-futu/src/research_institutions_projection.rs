//! Provider-neutral projections for OpenD institution and ARK responses.
//!
//! Keeping response mapping in a leaf module keeps the protocol reader focused
//! on session ownership, request encoding, and operation dispatch.  Generated
//! protobuf types remain private to the integration crate.

use super::{
    InstitutionDistribution, InstitutionDynamic, InstitutionEntry, InstitutionOperation,
    InstitutionQuery, InstitutionQueryError, InstitutionResult, InstitutionSecurity,
    InstitutionSummary, market_label,
};

pub(super) fn base_result(
    query: &InstitutionQuery,
    entries: Vec<InstitutionEntry>,
    next_page: Option<String>,
    all_count: Option<i32>,
    currency: Option<String>,
) -> InstitutionResult {
    InstitutionResult {
        operation: query.operation.as_str().to_owned(),
        entries,
        summary: None,
        distribution: Vec::new(),
        dynamic: None,
        all_count,
        next_page,
        currency: optional_text(currency),
    }
}

fn empty_entry(security: Option<InstitutionSecurity>, name: Option<String>) -> InstitutionEntry {
    InstitutionEntry {
        security,
        institution_id: None,
        name,
        industry_name: None,
        market_value: None,
        market_value_change: None,
        holding_count: None,
        holding_count_change: None,
        holding_value: None,
        holding_pct: None,
        last_holding_pct: None,
        portfolio_pct: None,
        change_shares: None,
        change_pct: None,
        shares: None,
        shares_change: None,
        weight: None,
        weight_change: None,
        change_amount: None,
        holding_date: None,
        disclosure_date: None,
        source: None,
    }
}

pub(super) fn map_institution_list(
    value: crate::trade_proto::qot_get_institution_list::InstitutionListItem,
) -> Result<InstitutionEntry, InstitutionQueryError> {
    validate_finite_fields([
        ("positionValue", value.position_value),
        ("positionValueChange", value.position_value_change),
    ])?;
    validate_non_negative_i32(value.position_count, "positionCount")?;
    validate_non_negative_i32(value.position_count_change, "positionCountChange")?;
    let mut entry = empty_entry(None, optional_text(value.institution_name));
    entry.institution_id = Some(value.institution_id);
    entry.market_value = value.position_value;
    entry.market_value_change = value.position_value_change;
    entry.holding_count = value.position_count;
    entry.holding_count_change = value.position_count_change;
    entry.disclosure_date = optional_text(value.disclosure_date);
    Ok(entry)
}

pub(super) fn map_holding_change(
    value: crate::trade_proto::qot_get_institution_holding_change::HoldingChangeItem,
) -> Result<InstitutionEntry, InstitutionQueryError> {
    let security = map_security(value.security)?;
    validate_finite_fields([
        ("portfolioPct", value.portfolio_pct),
        ("changePct", value.change_pct),
    ])?;
    let mut entry = empty_entry(Some(security), optional_text(value.name));
    entry.portfolio_pct = value.portfolio_pct;
    entry.change_shares = value.change_shares;
    entry.change_pct = value.change_pct;
    entry.holding_date = optional_text(value.holding_date);
    entry.source = optional_text(value.source);
    Ok(entry)
}

pub(super) fn map_holding(
    value: crate::trade_proto::qot_get_institution_holding_list::HoldingListItem,
) -> Result<InstitutionEntry, InstitutionQueryError> {
    let security = map_security(value.security)?;
    validate_finite_fields([
        ("holdingValue", value.holding_value),
        ("holdingPct", value.holding_pct),
        ("lastHoldingPct", value.last_holding_pct),
        ("portfolioPct", value.portfolio_pct),
        ("changePct", value.change_pct),
    ])?;
    let mut entry = empty_entry(Some(security), optional_text(value.name));
    entry.industry_name = optional_text(value.industry_name);
    entry.holding_value = value.holding_value;
    entry.holding_pct = value.holding_pct;
    entry.last_holding_pct = value.last_holding_pct;
    entry.portfolio_pct = value.portfolio_pct;
    entry.change_shares = value.change_shares;
    entry.change_pct = value.change_pct;
    entry.holding_date = optional_text(value.holding_date);
    entry.source = optional_text(value.source);
    Ok(entry)
}

pub(super) fn map_ark_holding(
    value: crate::trade_proto::qot_get_ark_fund_holding::ArkFundHoldingItem,
) -> Result<InstitutionEntry, InstitutionQueryError> {
    let security = value.security.map(map_security).transpose()?;
    validate_finite_fields([
        ("marketValue", value.market_value),
        ("weight", value.weight),
        ("weightChange", value.weight_change),
    ])?;
    let mut entry = empty_entry(security, optional_text(value.name));
    entry.market_value = value.market_value;
    entry.shares = value.shares;
    entry.shares_change = value.shares_change;
    entry.weight = value.weight;
    entry.weight_change = value.weight_change;
    Ok(entry)
}

pub(super) fn map_ark_transaction(
    value: crate::trade_proto::qot_get_ark_active_transaction::ArkActiveTransactionItem,
) -> Result<InstitutionEntry, InstitutionQueryError> {
    let security = value.security.map(map_security).transpose()?;
    validate_finite_fields([("changeAmount", value.change_amount)])?;
    let mut entry = empty_entry(security, optional_text(value.name));
    entry.change_shares = value.change_shares;
    entry.change_amount = value.change_amount;
    Ok(entry)
}

pub(super) fn map_profile(
    value: crate::trade_proto::qot_get_institution_profile::S2c,
) -> Result<InstitutionSummary, InstitutionQueryError> {
    validate_finite_fields([
        ("positionValue", value.position_value),
        ("lastPositionValue", value.last_position_value),
        ("positionValueChangePct", value.position_value_change_pct),
        ("top10Pct", value.top10_pct),
        ("top10PctChange", value.top10_pct_change),
    ])?;
    for (field, value) in [
        ("totalHoldingCount", value.total_holding_count),
        ("holdingChangeCount", value.holding_change_count),
    ] {
        if value.is_some_and(|value| value < 0) {
            return Err(InstitutionQueryError::InvalidResponse(format!(
                "OpenD institution {field} must be non-negative"
            )));
        }
    }
    for (field, value) in [
        ("newCount", value.new_count),
        ("soldOutCount", value.sold_out_count),
        ("increaseCount", value.increase_count),
        ("decreaseCount", value.decrease_count),
    ] {
        validate_non_negative_i32(value, field)?;
    }
    Ok(InstitutionSummary {
        institution_name: optional_text(value.institution_name),
        description: optional_text(value.description),
        position_value: value.position_value,
        last_position_value: value.last_position_value,
        position_value_change_pct: value.position_value_change_pct,
        total_holding_count: value.total_holding_count,
        holding_change_count: value.holding_change_count,
        new_count: value.new_count,
        sold_out_count: value.sold_out_count,
        increase_count: value.increase_count,
        decrease_count: value.decrease_count,
        top10_pct: value.top10_pct,
        top10_pct_change: value.top10_pct_change,
        disclosure_date: optional_text(value.disclosure_date),
        currency: optional_text(value.currency),
    })
}

pub(super) fn map_distribution(
    value: crate::trade_proto::qot_get_institution_distribution::IndustryDistributionItem,
) -> Result<InstitutionDistribution, InstitutionQueryError> {
    validate_finite_fields([
        ("positionValue", value.position_value),
        ("portfolioPct", value.portfolio_pct),
    ])?;
    Ok(InstitutionDistribution {
        industry_id: value.industry_id,
        industry_name: optional_text(value.industry_name),
        position_value: value.position_value,
        portfolio_pct: value.portfolio_pct,
    })
}

pub(super) fn map_dynamic(
    value: crate::trade_proto::qot_get_ark_stock_dynamic::S2c,
) -> Result<InstitutionDynamic, InstitutionQueryError> {
    if value
        .dynamic_type
        .is_some_and(|value| !(0..=4).contains(&value))
    {
        return Err(InstitutionQueryError::InvalidResponse(
            "OpenD ARK dynamicType is unsupported".to_owned(),
        ));
    }
    validate_non_negative_i32(value.transaction_count, "transactionCount")?;
    Ok(InstitutionDynamic {
        dynamic_type: value.dynamic_type,
        transaction_count: value.transaction_count,
        net_shares: value.net_shares,
        last_transaction_time: optional_text(value.last_transaction_time),
    })
}

fn map_security(
    value: crate::trade_proto::qot_common::Security,
) -> Result<InstitutionSecurity, InstitutionQueryError> {
    let market = market_label(value.market).ok_or_else(|| {
        InstitutionQueryError::InvalidResponse(
            "OpenD institution security market is unsupported".to_owned(),
        )
    })?;
    let code = value.code.trim();
    if code.is_empty()
        || code
            .chars()
            .any(|value| value.is_whitespace() || value.is_control())
    {
        return Err(InstitutionQueryError::InvalidResponse(
            "OpenD institution security code is invalid".to_owned(),
        ));
    }
    let code = code.to_ascii_uppercase();
    Ok(InstitutionSecurity {
        market: market.to_owned(),
        instrument_id: format!("{market}.{code}"),
        code,
    })
}

pub(super) fn ensure_success(
    ret_type: i32,
    err_code: Option<i32>,
    ret_msg: Option<String>,
    operation: InstitutionOperation,
) -> Result<(), InstitutionQueryError> {
    if ret_type == 0 {
        return Ok(());
    }
    Err(InstitutionQueryError::Rejected {
        operation: operation.as_str(),
        ret_type,
        err_code: err_code.unwrap_or_default(),
        message: ret_msg.unwrap_or_else(|| "OpenD institution request failed".to_owned()),
    })
}

pub(super) fn normalize_next_page(value: Option<String>) -> Option<String> {
    value
        .map(|value| value.trim().to_owned())
        .filter(|value| !value.is_empty() && value != "-1")
}

pub(super) fn optional_text(value: Option<String>) -> Option<String> {
    value
        .map(|value| value.trim().to_owned())
        .filter(|value| !value.is_empty())
}

fn validate_finite_fields<const N: usize>(
    values: [(&str, Option<f64>); N],
) -> Result<(), InstitutionQueryError> {
    for (field, value) in values {
        if value.is_some_and(|value| !value.is_finite()) {
            return Err(InstitutionQueryError::InvalidResponse(format!(
                "OpenD institution {field} must be finite"
            )));
        }
    }
    Ok(())
}

fn validate_non_negative_i32(value: Option<i32>, field: &str) -> Result<(), InstitutionQueryError> {
    if value.is_some_and(|value| value < 0) {
        return Err(InstitutionQueryError::InvalidResponse(format!(
            "OpenD institution {field} must be non-negative"
        )));
    }
    Ok(())
}
