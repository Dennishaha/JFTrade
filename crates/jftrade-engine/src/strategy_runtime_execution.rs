use jftrade_kernel::Fixed8;
use jftrade_store_sqlite::StrategyRuntimeStore;
use jftrade_trading::{RuntimeRiskContext, RuntimeRiskOrder, RuntimeRiskSettings};
use serde_json::{Value, json};
use time::OffsetDateTime;

use crate::product::product_active_provider_state::ActiveProviderState;
use crate::product::product_execution_write_port::{
    ExecutionWriteInput, ExecutionWriteOperation, ExecutionWritePort, ExecutionWritePortError,
};
use crate::product::{ProductNotificationPort, ProductNotificationRequest};
use jftrade_integration_pine::PineOrderIntent;

pub(super) struct StrategyExecutionContext<'a> {
    pub execution: Option<&'a dyn ExecutionWritePort>,
    pub provider: &'a ActiveProviderState,
    pub store: &'a StrategyRuntimeStore,
    pub instance_id: &'a str,
    pub market: &'a str,
    pub symbol: &'a str,
    pub binding: &'a Value,
    pub expected_risk_revision: Option<i64>,
    pub fallback_price: Option<f64>,
    pub sellable_quantity: Option<f64>,
}

pub(super) fn execute_strategy_intents(
    ctx: StrategyExecutionContext<'_>,
    intents: &[PineOrderIntent],
) -> Result<bool, String> {
    let Some(execution) = ctx.execution else {
        return Err("strategy execution order port is unavailable".to_owned());
    };
    let (broker_id, account_id, trading_environment) =
        strategy_execution_binding(ctx.binding, ctx.provider)?;

    let stored_instance = ctx
        .store
        .get_instance(ctx.instance_id)
        .map_err(|error| format!("load strategy instance: {error}"))?
        .ok_or_else(|| format!("strategy instance {} not found", ctx.instance_id))?;

    if let Some(expected) = ctx
        .expected_risk_revision
        .filter(|expected| stored_instance.runtime_risk_revision != *expected)
    {
        let detail = format!(
            "expected revision {expected} does not match stored revision {}",
            stored_instance.runtime_risk_revision
        );
        let _ = ctx.store.append_audit_event(
            ctx.instance_id,
            "RUNTIME_RISK_REVISION_MISMATCH",
            &detail,
            now_millis(),
        );
        return Err(format!("runtime risk revision fence triggered: {detail}"));
    }

    let raw_risk = &stored_instance.runtime_risk;
    let (risk_settings, raw_mode) = if raw_risk.is_null() {
        (RuntimeRiskSettings::default(), "off".to_owned())
    } else {
        let settings: RuntimeRiskSettings = serde_json::from_value(raw_risk.clone())
            .map_err(|error| format!("deserialize strategy runtime risk settings: {error}"))?;
        let mode = settings.mode.trim().to_ascii_lowercase();
        (settings, mode)
    };
    if !raw_mode.is_empty() && !matches!(raw_mode.as_str(), "off" | "monitor" | "enforce") {
        return Err(format!("unknown strategy runtime risk mode: {raw_mode}"));
    }
    let risk_settings = risk_settings.normalize();

    let now_utc = OffsetDateTime::now_utc();
    let today_midnight_ms = OffsetDateTime::new_utc(now_utc.date(), time::Time::MIDNIGHT)
        .unix_timestamp_nanos() as i64
        / 1_000_000;
    let initial_daily_orders = ctx
        .store
        .count_daily_orders(ctx.instance_id, today_midnight_ms)
        .map_err(|error| format!("query strategy daily orders: {error}"))?;

    let mut placed = false;
    for (index, intent) in intents.iter().enumerate() {
        let kind = intent.kind.trim().to_ascii_lowercase();
        if kind == "cancel" || kind == "cancel_all" {
            dispatch_cancel_intent(
                execution,
                ctx.store,
                ctx.instance_id,
                &broker_id,
                &account_id,
                &trading_environment,
                ctx.market,
                ctx.symbol,
                intent,
            )?;
            placed = true;
            continue;
        }

        let (quantity, reduce_only) =
            resolve_strategy_intent_quantity(intent, ctx.binding, ctx.sellable_quantity, index)?;
        let is_close = matches!(kind.as_str(), "close" | "close_all" | "exit");
        let side = match (is_close, intent.direction.trim().to_ascii_lowercase().as_str()) {
            (true, "short" | "sell" | "bear" | "bearish" | "cover") => "BUY",
            (true, "buy" | "long" | "bull" | "bullish" | "flat" | "" | "close") => "SELL",
            (false, "buy" | "long" | "bull" | "bullish") => "BUY",
            (false, "sell" | "short" | "bear" | "bearish") => "SELL",
            _ => return Err(format!("strategy order intent {index} has invalid direction")),
        };

        let order_price = if intent.has_limit_price {
            Fixed8::from_f64(intent.limit_price).ok()
        } else {
            ctx.fallback_price.and_then(|p| Fixed8::from_f64(p).ok())
        };
        let risk_order = RuntimeRiskOrder {
            symbol: ctx.symbol.to_owned(),
            side: side.to_owned(),
            quantity: Fixed8::from_f64(quantity).unwrap_or_default(),
            price: order_price,
        };
        let risk_sellable_qty = ctx
            .sellable_quantity
            .and_then(|q| Fixed8::from_f64(q).ok())
            .unwrap_or_else(|| {
                if reduce_only || is_close {
                    Fixed8::from_f64(quantity).unwrap_or(Fixed8::ZERO)
                } else {
                    Fixed8::ZERO
                }
            });
        let risk_context = RuntimeRiskContext {
            current_price: order_price,
            sellable_quantity: risk_sellable_qty,
            today_submitted_order_count: initial_daily_orders + index as i64,
        };

        let decision = risk_settings.evaluate(&risk_order, &risk_context);
        if decision.rejected {
            let detail = decision
                .detail
                .clone()
                .unwrap_or_else(|| decision.reason.clone().unwrap_or_default());
            let _ = ctx.store.append_audit_event(
                ctx.instance_id,
                "RUNTIME_RISK_REJECTED",
                &detail,
                now_millis(),
            );
            if decision.pause_on_reject {
                let rfc3339 = now_rfc3339()?;
                let _ = ctx.store.update_status_cas(ctx.instance_id, &["RUNNING"], "PAUSED", &rfc3339);
            }
            return Err(format!("runtime risk rejected: {detail}"));
        } else if decision.matched {
            let detail = decision
                .detail
                .clone()
                .unwrap_or_else(|| decision.reason.clone().unwrap_or_default());
            let _ = ctx.store.append_audit_event(
                ctx.instance_id,
                "RUNTIME_RISK_MATCHED",
                &detail,
                now_millis(),
            );
        }

        dispatch_place_order(
            execution,
            ctx.store,
            ctx.instance_id,
            ctx.market,
            ctx.symbol,
            &broker_id,
            &account_id,
            &trading_environment,
            side,
            quantity,
            reduce_only,
            intent,
            index,
        )?;
        placed = true;
    }
    Ok(placed)
}

#[allow(clippy::too_many_arguments)]
fn dispatch_cancel_intent(
    execution: &dyn ExecutionWritePort,
    store: &StrategyRuntimeStore,
    instance_id: &str,
    broker_id: &str,
    account_id: &str,
    trading_environment: &str,
    market: &str,
    symbol: &str,
    intent: &PineOrderIntent,
) -> Result<(), String> {
    let target_order_id = if !intent.id.trim().is_empty() {
        intent.id.trim()
    } else if !intent.from_entry.trim().is_empty() {
        intent.from_entry.trim()
    } else {
        ""
    };
    let input = ExecutionWriteInput {
        operation: ExecutionWriteOperation::OrderCancel,
        internal_order_id: if target_order_id.is_empty() {
            None
        } else {
            Some(target_order_id.to_owned())
        },
        payload: json!({
            "brokerId": broker_id,
            "accountId": account_id,
            "tradingEnvironment": trading_environment,
            "market": market,
            "symbol": symbol,
            "code": symbol,
            "orderKind": intent.kind,
            "remark": format!("strategy runtime cancel {instance_id}"),
        }),
        context: crate::product::product_execution_write_port::ExecutionWriteContext::Normal,
    };
    match execution.mutate(&input) {
        Ok(_) => {
            let _ = store.append_audit_event(
                instance_id,
                "ORDER_CANCELLED",
                &format!("target order {target_order_id}"),
                now_millis(),
            );
            Ok(())
        }
        Err(err) => {
            let err_msg = execution_error_message(err);
            let _ = store.append_audit_event(
                instance_id,
                "ORDER_CANCEL_FAILED",
                &format!("target order {target_order_id}: {err_msg}"),
                now_millis(),
            );
            Err(err_msg)
        }
    }
}

#[allow(clippy::too_many_arguments)]
fn dispatch_place_order(
    execution: &dyn ExecutionWritePort,
    store: &StrategyRuntimeStore,
    instance_id: &str,
    market: &str,
    symbol: &str,
    broker_id: &str,
    account_id: &str,
    trading_environment: &str,
    side: &str,
    quantity: f64,
    reduce_only: bool,
    intent: &PineOrderIntent,
    index: usize,
) -> Result<(), String> {
    let order_type = if intent.has_limit_price {
        "LIMIT"
    } else if intent.has_stop_price {
        "STOP"
    } else {
        "MARKET"
    };
    let client_order_id = strategy_client_order_id(instance_id, symbol, intent, index);
    let mut payload = json!({
        "brokerId": broker_id,
        "accountId": account_id,
        "tradingEnvironment": trading_environment,
        "market": market,
        "symbol": symbol,
        "code": symbol,
        "side": side,
        "orderType": order_type,
        "quantity": quantity,
        "orderKind": intent.kind,
        "remark": format!("strategy runtime {instance_id}"),
        "clientOrderId": client_order_id,
        "reduceOnly": reduce_only,
    });
    if intent.has_limit_price {
        payload["price"] = json!(intent.limit_price);
    }
    if intent.has_stop_price {
        payload["stopPrice"] = json!(intent.stop_price);
    }
    let input = ExecutionWriteInput {
        operation: ExecutionWriteOperation::OrderPlace,
        internal_order_id: None,
        payload,
        context: crate::product::product_execution_write_port::ExecutionWriteContext::Normal,
    };
    execution.mutate(&input).map_err(execution_error_message)?;

    let _ = store.append_audit_event(
        instance_id,
        "ORDER_SUBMITTED",
        &format!("{symbol} {side} {quantity} (clientOrderId: {client_order_id})"),
        now_millis(),
    );
    Ok(())
}

pub(super) fn notify_strategy_intents(
    notification: Option<&dyn ProductNotificationPort>,
    store: &StrategyRuntimeStore,
    instance_id: &str,
    symbol: &str,
    intents: &[PineOrderIntent],
) -> Result<(), String> {
    for intent in intents {
        let side = match intent.direction.trim().to_ascii_lowercase().as_str() {
            "buy" | "long" | "bull" | "bullish" => "BUY",
            "sell" | "short" | "bear" | "bearish" => "SELL",
            _ => "UNKNOWN",
        };
        let body = format!("{symbol} {side} {} (仅通知模式)", intent.quantity);
        let now = now_millis();
        let _ = store.append_log_event(
            instance_id,
            &format!("Signal detected: {symbol} {side} {} (notify_only)", intent.quantity),
            "INFO",
            now,
        );
        let delivered = if let Some(notifier) = notification {
            let request = ProductNotificationRequest {
                title: "策略下单信号".to_owned(),
                body: body.clone(),
                sound_enabled: true,
            };
            notifier.deliver(request).delivered
        } else {
            false
        };
        let event_kind = if delivered {
            "SIGNAL_NOTIFIED"
        } else {
            "SIGNAL_DETECTED"
        };
        let _ = store.append_audit_event(instance_id, event_kind, &body, now);
    }
    Ok(())
}

pub(super) fn validate_strategy_execution_binding(
    binding: &Value,
    provider: &ActiveProviderState,
) -> Result<(), String> {
    strategy_execution_binding(binding, provider).map(|_| ())
}

pub(super) fn strategy_execution_binding(
    binding: &Value,
    provider: &ActiveProviderState,
) -> Result<(String, String, String), String> {
    let broker_id = binding_scalar_string(binding, &["brokerId", "broker"])
        .or_else(|| {
            nested_binding_scalar_string(binding, "brokerAccount", &["brokerId", "broker"])
        })
        .or_else(|| {
            (provider.snapshot().provider == Some(jftrade_settings::MarketDataProvider::Futu))
                .then_some("futu".to_owned())
        })
        .ok_or_else(|| "strategy execution broker is not configured".to_owned())?;
    let account_id = binding_scalar_string(binding, &["accountId", "account"])
        .or_else(|| {
            nested_binding_scalar_string(binding, "brokerAccount", &["accountId", "account"])
        })
        .ok_or_else(|| "strategy execution accountId is not configured".to_owned())?;
    if account_id.parse::<u64>().is_err() {
        return Err("strategy execution accountId must be numeric for Futu".to_owned());
    }
    let trading_environment = binding_scalar_string(
        binding,
        &["tradingEnvironment", "environment", "env"],
    )
    .or_else(|| {
        nested_binding_scalar_string(
            binding,
            "brokerAccount",
            &["tradingEnvironment", "environment", "env"],
        )
    })
    .ok_or_else(|| "strategy execution tradingEnvironment is not configured".to_owned())?
    .to_ascii_uppercase();
    if !matches!(trading_environment.as_str(), "REAL" | "SIMULATE") {
        return Err("strategy execution tradingEnvironment must be REAL or SIMULATE".to_owned());
    }
    Ok((broker_id, account_id, trading_environment))
}

fn binding_scalar_string(binding: &Value, keys: &[&str]) -> Option<String> {
    keys.iter()
        .find_map(|key| binding.get(*key).and_then(value_scalar_string))
}

fn nested_binding_scalar_string(
    binding: &Value,
    object_key: &str,
    keys: &[&str],
) -> Option<String> {
    binding
        .get(object_key)
        .and_then(Value::as_object)
        .and_then(|object| {
            keys.iter()
                .find_map(|key| object.get(*key).and_then(value_scalar_string))
        })
}

fn value_scalar_string(value: &Value) -> Option<String> {
    match value {
        Value::String(value) => {
            let value = value.trim();
            (!value.is_empty()).then_some(value.to_owned())
        }
        Value::Number(value) => Some(value.to_string()),
        _ => None,
    }
}

fn resolve_strategy_intent_quantity(
    intent: &PineOrderIntent,
    binding: &Value,
    sellable_quantity: Option<f64>,
    index: usize,
) -> Result<(f64, bool), String> {
    let kind = intent.kind.trim().to_ascii_lowercase();
    let is_close = matches!(kind.as_str(), "close" | "close_all" | "exit");
    let reduce_only = intent.reduce_only || is_close;

    let mut qty = if intent.has_quantity && intent.quantity.is_finite() && intent.quantity > 0.0 {
        intent.quantity
    } else if is_close {
        if intent.quantity.is_finite() && intent.quantity > 0.0 {
            intent.quantity
        } else {
            sellable_quantity.filter(|q| *q > 0.0).unwrap_or(1.0)
        }
    } else if intent.has_quantity_pct {
        let base_capital = binding_scalar_f64(
            binding,
            &["capital", "initialCapital", "orderSize", "defaultOrderSize", "accountSize"],
        )
        .or_else(|| {
            nested_binding_scalar_f64(
                binding,
                "brokerAccount",
                &["capital", "orderSize", "accountSize"],
            )
        })
        .unwrap_or(100.0);
        let pct = if intent.quantity_pct > 0.0 {
            intent.quantity_pct
        } else {
            intent.quantity
        };
        (base_capital * (pct / 100.0)).floor().max(1.0)
    } else if matches!(kind.as_str(), "entry" | "order") && intent.quantity <= 0.0 {
        1.0
    } else {
        return Err(format!(
            "strategy order intent {index} requires a positive finite quantity"
        ));
    };

    if let Some(lot) =
        binding_scalar_f64(binding, &["lotSize", "lot_size"]).filter(|lot| *lot > 0.0)
    {
        qty = (qty / lot).round() * lot;
        if qty < lot {
            qty = lot;
        }
    }

    Ok((qty, reduce_only))
}

fn binding_scalar_f64(binding: &Value, keys: &[&str]) -> Option<f64> {
    keys.iter().find_map(|key| {
        binding.get(*key).and_then(|v| {
            v.as_f64()
                .or_else(|| v.as_str().and_then(|s| s.parse::<f64>().ok()))
        })
    })
}

fn nested_binding_scalar_f64(
    binding: &Value,
    object_key: &str,
    keys: &[&str],
) -> Option<f64> {
    binding
        .get(object_key)
        .and_then(Value::as_object)
        .and_then(|object| {
            keys.iter().find_map(|key| {
                object.get(*key).and_then(|v| {
                    v.as_f64()
                        .or_else(|| v.as_str().and_then(|s| s.parse::<f64>().ok()))
                })
            })
        })
}

fn strategy_client_order_id(
    instance_id: &str,
    symbol: &str,
    intent: &PineOrderIntent,
    index: usize,
) -> String {
    let intent_id = if intent.id.trim().is_empty() {
        format!("intent-{index}")
    } else {
        intent.id.trim().to_owned()
    };
    format!(
        "strategy-{instance_id}-{symbol}-{intent_id}-{}-{candle_time}",
        intent.bar_index,
        candle_time = intent.time
    )
}

fn execution_error_message(error: ExecutionWritePortError) -> String {
    match error {
        ExecutionWritePortError::Unavailable(message) => {
            format!("strategy execution unavailable: {message}")
        }
        ExecutionWritePortError::Failed {
            status,
            code,
            message,
        } => format!("strategy execution failed ({status} {code}): {message}"),
    }
}

pub(super) fn now_millis() -> i64 {
    time::OffsetDateTime::now_utc().unix_timestamp_nanos() as i64 / 1_000_000
}

pub(super) fn now_rfc3339() -> Result<String, String> {
    time::OffsetDateTime::now_utc()
        .format(&time::format_description::well_known::Rfc3339)
        .map_err(|error| format!("format strategy runtime timestamp: {error}"))
}

#[cfg(test)]
#[path = "strategy_runtime_execution_tests.rs"]
mod tests;

