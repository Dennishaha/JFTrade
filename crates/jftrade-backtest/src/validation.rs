use jftrade_kernel::Fixed8;

use crate::BacktestError;
use crate::model::{BacktestCase, OrderIntent};

pub(crate) fn validate_case(case: &BacktestCase) -> Result<(), BacktestError> {
    if case.id.trim().is_empty() || case.symbol.trim().is_empty() {
        return Err(BacktestError::InvalidInput(
            "case id and symbol are required".to_owned(),
        ));
    }
    if case.initial_balance < Fixed8::ZERO {
        return Err(BacktestError::InvalidInput(
            "initial balance cannot be negative".to_owned(),
        ));
    }
    if case.market.quantity_step <= Fixed8::ZERO
        || case.market.tick_size <= Fixed8::ZERO
        || case.market.min_quantity < Fixed8::ZERO
    {
        return Err(BacktestError::InvalidInput(
            "market increments must be positive".to_owned(),
        ));
    }
    if case.candles.is_empty() {
        return Err(BacktestError::InvalidInput(
            "at least one candle is required".to_owned(),
        ));
    }
    for pair in case.candles.windows(2) {
        if pair[0].start >= pair[1].start || pair[0].end >= pair[1].end {
            return Err(BacktestError::InvalidInput(
                "candles must be strictly ordered".to_owned(),
            ));
        }
    }
    if let Some(intent) = case
        .intents
        .iter()
        .find(|intent| intent.bar_index >= case.candles.len())
    {
        return Err(BacktestError::InvalidInput(format!(
            "intent {} targets an unavailable bar",
            intent.id
        )));
    }
    Ok(())
}

pub(crate) fn validate_submit_intent(intent: &OrderIntent) -> Result<(), BacktestError> {
    if intent.action != "submit" {
        return Err(BacktestError::InvalidInput(format!(
            "unsupported intent action {}",
            intent.action
        )));
    }
    if intent.id.trim().is_empty() || intent.quantity <= Fixed8::ZERO {
        return Err(BacktestError::InvalidInput(
            "submit intent requires id and positive quantity".to_owned(),
        ));
    }
    if !matches!(intent.side.as_str(), "buy" | "sell") {
        return Err(BacktestError::InvalidInput(format!(
            "intent {} has unsupported side {}",
            intent.id, intent.side
        )));
    }
    match normalized_order_type(&intent.order_type) {
        "limit" | "limit_maker" if intent.limit_price <= Fixed8::ZERO => Err(
            BacktestError::InvalidInput(format!("intent {} requires a limit price", intent.id)),
        ),
        "stop_market" if intent.stop_price <= Fixed8::ZERO => Err(BacktestError::InvalidInput(
            format!("intent {} requires a stop price", intent.id),
        )),
        "stop_limit" if intent.stop_price <= Fixed8::ZERO || intent.limit_price <= Fixed8::ZERO => {
            Err(BacktestError::InvalidInput(format!(
                "intent {} requires stop and limit prices",
                intent.id
            )))
        }
        _ => Ok(()),
    }
}

pub(crate) fn normalized_order_type(value: &str) -> &str {
    if value.trim().is_empty() {
        "market"
    } else {
        value
    }
}
