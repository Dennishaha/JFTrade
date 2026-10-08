use jftrade_integration_pine::PineOrderIntent;

/// Validate the whole worker response before any order, cancellation, or local
/// fill. The execution port cannot enforce parent activation or atomic OCO, so
/// relationships must never be silently discarded during dispatch.
pub(super) fn validate_strategy_intents(intents: &[PineOrderIntent]) -> Result<(), String> {
    for (index, intent) in intents.iter().enumerate() {
        let kind = intent.kind.trim().to_ascii_lowercase();
        if !matches!(
            kind.as_str(),
            "order" | "entry" | "close" | "close_all" | "exit" | "cancel" | "cancel_all"
        ) {
            return Err(format!(
                "unsupported strategy order intent kind at index {index}: {}",
                intent.kind
            ));
        }
        validate_prices(intent, &kind, index)?;
        validate_relationships(intent, &kind, index)?;
        if matches!(kind.as_str(), "entry" | "order")
            && !matches!(
                intent.direction.trim().to_ascii_lowercase().as_str(),
                "buy" | "long" | "bull" | "bullish" | "sell" | "short" | "bear" | "bearish"
            )
        {
            return Err(format!(
                "strategy order intent {index} requires long/short direction"
            ));
        }
        if kind == "cancel" && intent.id.trim().is_empty() && intent.from_entry.trim().is_empty() {
            return Err(format!("cancel command id is required at index {index}"));
        }
    }
    Ok(())
}

fn validate_prices(intent: &PineOrderIntent, kind: &str, index: usize) -> Result<(), String> {
    for (present, price, name) in [
        (intent.has_limit_price, intent.limit_price, "limit"),
        (intent.has_stop_price, intent.stop_price, "stop"),
    ] {
        if present && (!price.is_finite() || price <= 0.0) {
            return Err(format!(
                "strategy order intent {index} {name} price must be positive and finite"
            ));
        }
    }
    if intent.has_limit_price && intent.has_stop_price {
        if kind == "exit" {
            return Err(format!(
                "strategy exit intent {index} is an OCO bracket requiring an atomic execution port"
            ));
        }
        if !matches!(kind, "entry" | "order") {
            return Err(format!(
                "strategy {kind} intent {index} cannot combine limit and stop prices"
            ));
        }
    }
    Ok(())
}

fn validate_relationships(
    intent: &PineOrderIntent,
    kind: &str,
    index: usize,
) -> Result<(), String> {
    let has_parent = !intent.parent_id.trim().is_empty();
    let has_atomic = !intent.atomic_group_id.trim().is_empty();
    let has_oco = !intent.oco_group_id.trim().is_empty();
    if has_oco && !has_atomic {
        return Err(format!(
            "strategy OCO intent {index} requires an atomic group id"
        ));
    }
    if matches!(kind, "cancel" | "cancel_all")
        && (has_parent || has_atomic || has_oco || intent.reduce_only)
    {
        return Err(format!(
            "strategy cancellation intent {index} cannot carry placement relationships"
        ));
    }
    if matches!(kind, "entry" | "order") && (has_parent || has_oco || intent.reduce_only) {
        return Err(format!(
            "strategy entry intent {index} has invalid protective-order metadata"
        ));
    }
    if has_parent || has_atomic || has_oco {
        return Err(format!(
            "strategy intent {index} has unsupported parent or atomic group relationships"
        ));
    }
    Ok(())
}
