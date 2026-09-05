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
    pub execution_store: Option<&'a jftrade_store_sqlite::ExecutionOrderStore>,
    pub provider: &'a ActiveProviderState,
    pub store: &'a StrategyRuntimeStore,
    pub instance_id: &'a str,
    pub market: &'a str,
    pub symbol: &'a str,
    pub binding: &'a Value,
    pub expected_risk_revision: Option<i64>,
    pub fallback_price: Option<f64>,
    pub sellable_quantity: Option<f64>,
    pub current_position: Option<f64>,
    pub available_cash: Option<f64>,
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
    let mut placed = false;
    let mut submitted_count = 0_i64;
    for (index, intent) in intents.iter().enumerate() {
        let kind = intent.kind.trim().to_ascii_lowercase();
        if kind == "cancel" || kind == "cancel_all" {
            let cancelled = dispatch_cancel_intent(
                execution,
                ctx.execution_store,
                ctx.store,
                ctx.instance_id,
                &broker_id,
                &account_id,
                &trading_environment,
                ctx.market,
                ctx.symbol,
                intent,
            )?;
            if cancelled {
                placed = true;
            }
            continue;
        }

        let is_close = matches!(kind.as_str(), "close" | "close_all" | "exit");
        let current_pos = ctx.current_position.or(ctx.sellable_quantity);

        if is_close
            && current_pos.unwrap_or(0.0) == 0.0
            && (!intent.has_quantity || intent.quantity <= 0.0)
        {
            let _ = ctx.store.append_audit_event(
                ctx.instance_id,
                "INTENT_SKIPPED",
                "no open position to close",
                now_millis(),
            );
            continue;
        }

        let side = if is_close {
            if let Some(pos) = current_pos.filter(|p| *p != 0.0) {
                if pos > 0.0 { "SELL" } else { "BUY" }
            } else {
                match intent.direction.trim().to_ascii_lowercase().as_str() {
                    "short" | "sell" | "bear" | "bearish" | "cover" => "BUY",
                    _ => "SELL",
                }
            }
        } else {
            match intent.direction.trim().to_ascii_lowercase().as_str() {
                "buy" | "long" | "bull" | "bullish" => "BUY",
                "sell" | "short" | "bear" | "bearish" => "SELL",
                _ => return Err(format!("strategy order intent {index} has invalid direction")),
            }
        };

        let resolved_qty = resolve_strategy_intent_quantity(
            intent,
            ctx.binding,
            current_pos,
            ctx.fallback_price,
            ctx.available_cash,
            side,
            is_close,
            index,
            !execution_binding_is_real(ctx.binding),
        )?;
        let Some((quantity, reduce_only)) = resolved_qty else {
            let _ = ctx.store.append_audit_event(
                ctx.instance_id,
                "INTENT_SKIPPED",
                "quantity rounded down to 0",
                now_millis(),
            );
            continue;
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
            today_submitted_order_count: ctx
                .store
                .count_daily_orders(ctx.instance_id, today_midnight_ms)
                .map_err(|error| format!("query strategy daily orders: {error}"))?
                + submitted_count,
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

        if let Some(expected) = ctx.expected_risk_revision {
            let latest = ctx
                .store
                .get_instance(ctx.instance_id)
                .map_err(|error| format!("reload strategy risk revision: {error}"))?
                .ok_or_else(|| format!("strategy instance {} not found", ctx.instance_id))?;
            if latest.runtime_risk_revision != expected {
                let detail = format!(
                    "expected revision {expected} does not match stored revision {} before submit",
                    latest.runtime_risk_revision
                );
                let _ = ctx.store.append_audit_event(
                    ctx.instance_id,
                    "RUNTIME_RISK_REVISION_MISMATCH",
                    &detail,
                    now_millis(),
                );
                return Err(format!("runtime risk revision fence triggered: {detail}"));
            }
        }

        let reservation_key = format!(
            "{ctx_instance}:{symbol}:{bar}:{index}",
            ctx_instance = ctx.instance_id,
            symbol = ctx.symbol,
            bar = intent.bar_index,
        );
        let reservation_required =
            risk_settings.mode == "enforce" && risk_settings.daily_max_orders.is_some();
        if reservation_required {
            ctx.store
                .reserve_daily_order(
                    ctx.instance_id,
                    ctx.expected_risk_revision.unwrap_or(stored_instance.runtime_risk_revision),
                    today_midnight_ms,
                    risk_settings.daily_max_orders,
                    &reservation_key,
                    now_millis(),
                )
                .map_err(|error| format!("reserve strategy daily order slot: {error}"))?;
        }
        if let Err(error) = dispatch_place_order(
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
            Some(&reservation_key),
        ) {
            if reservation_required {
                let _ = ctx.store.release_order_reservation(
                    ctx.instance_id,
                    &reservation_key,
                    now_millis(),
                );
            }
            return Err(error);
        }
        placed = true;
        submitted_count += 1;
    }
    Ok(placed)
}

#[allow(clippy::too_many_arguments)]
fn dispatch_cancel_intent(
    execution: &dyn ExecutionWritePort,
    execution_store: Option<&jftrade_store_sqlite::ExecutionOrderStore>,
    store: &StrategyRuntimeStore,
    instance_id: &str,
    broker_id: &str,
    account_id: &str,
    trading_environment: &str,
    market: &str,
    symbol: &str,
    intent: &PineOrderIntent,
) -> Result<bool, String> {
    let kind = intent.kind.trim().to_ascii_lowercase();
    if kind == "cancel_all" {
        let mut cancelled_any = false;
        let ord_store = execution_store
            .ok_or_else(|| "execution order store is unavailable for cancel_all".to_owned())?;
        let active = ord_store
            .list_active_orders_for_instance(instance_id)
            .map_err(|e| format!("query active orders for cancel: {e}"))?;
        let mut failures = Vec::new();
        {
            for order in active {
                let input = ExecutionWriteInput {
                    operation: ExecutionWriteOperation::OrderCancel,
                    internal_order_id: Some(order.internal_order_id.clone()),
                    payload: json!({
                        "brokerId": broker_id,
                        "accountId": account_id,
                        "tradingEnvironment": trading_environment,
                        "market": market,
                        "symbol": symbol,
                        "code": symbol,
                        "orderKind": "cancel",
                        "remark": format!("strategy runtime cancel {instance_id}"),
                    }),
                    context:
                        crate::product::product_execution_write_port::ExecutionWriteContext::Normal,
                };
                if let Err(e) = execution.mutate(&input) {
                    let err_msg = execution_error_message(e);
                    failures.push(format!("{}: {err_msg}", order.internal_order_id));
                    let _ = store.append_audit_event(
                        instance_id,
                        "ORDER_CANCEL_FAILED",
                        &format!("target order {}: {err_msg}", order.internal_order_id),
                        now_millis(),
                    );
                } else {
                    cancelled_any = true;
                    let _ = store.append_audit_event(
                        instance_id,
                        "ORDER_CANCELLED",
                        &format!("target order {}", order.internal_order_id),
                        now_millis(),
                    );
                }
            }
        }
        if !failures.is_empty() {
            return Err(format!("cancel_all partially failed: {}", failures.join("; ")));
        }
        return Ok(cancelled_any);
    }

    let target_order_id = if !intent.id.trim().is_empty() {
        intent.id.trim()
    } else if !intent.from_entry.trim().is_empty() {
        intent.from_entry.trim()
    } else {
        ""
    };

    let resolved_id = if let Some(ord_store) = execution_store {
        if !target_order_id.is_empty() {
            if let Ok(Some(existing)) = ord_store.find_order_by_client_identity(
                broker_id,
                trading_environment,
                account_id,
                target_order_id,
            ) {
                if existing.source == "strategy-runtime"
                    && existing.source_detail == instance_id
                    && matches!(
                        existing.status.as_str(),
                        "SUBMITTING" | "SUBMITTED" | "WAITING" | "OPEN"
                    )
                {
                    existing.internal_order_id
                } else {
                    return Err(format!(
                        "strategy order {target_order_id} is not owned by instance {instance_id}"
                    ));
                }
            } else {
                return Err(format!("strategy order {target_order_id} is not owned by instance {instance_id}"));
            }
        } else {
            String::new()
        }
    } else {
        return Err("execution order store is unavailable for targeted cancel".to_owned());
    };

    let input = ExecutionWriteInput {
        operation: ExecutionWriteOperation::OrderCancel,
        internal_order_id: if resolved_id.is_empty() {
            None
        } else {
            Some(resolved_id.clone())
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
                &format!("target order {resolved_id}"),
                now_millis(),
            );
            Ok(true)
        }
        Err(err) => {
            let err_msg = execution_error_message(err);
            let _ = store.append_audit_event(
                instance_id,
                "ORDER_CANCEL_FAILED",
                &format!("target order {resolved_id}: {err_msg}"),
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
    reservation_key: Option<&str>,
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
        "source": "strategy-runtime",
        "sourceDetail": instance_id,
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

    let detail = match reservation_key {
        Some(key) => format!("{symbol} {side} {quantity} (clientOrderId: {client_order_id}; reservation: {key})"),
        None => format!("{symbol} {side} {quantity} (clientOrderId: {client_order_id})"),
    };
    let _ = store.append_audit_event(
        instance_id,
        "ORDER_SUBMITTED",
        &detail,
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

fn execution_binding_is_real(binding: &Value) -> bool {
    binding_scalar_string(binding, &["tradingEnvironment", "environment", "env"])
        .or_else(|| {
            nested_binding_scalar_string(
                binding,
                "brokerAccount",
                &["tradingEnvironment", "environment", "env"],
            )
        })
        .is_some_and(|value| value.eq_ignore_ascii_case("REAL"))
}

#[allow(clippy::too_many_arguments)]
fn resolve_strategy_intent_quantity(
    intent: &PineOrderIntent,
    binding: &Value,
    current_position: Option<f64>,
    current_price: Option<f64>,
    available_cash: Option<f64>,
    side: &str,
    is_close: bool,
    index: usize,
    allow_default_quantity: bool,
) -> Result<Option<(f64, bool)>, String> {
    let kind = intent.kind.trim().to_ascii_lowercase();
    let reduce_only = intent.reduce_only || is_close;

    let mut qty = if intent.has_quantity && intent.quantity.is_finite() && intent.quantity > 0.0 {
        intent.quantity
    } else if is_close {
        let pos = current_position.unwrap_or(0.0).abs();
        if pos <= 0.0 {
            return Ok(None);
        }
        pos
    } else if intent.has_quantity_pct {
        let pct = if intent.quantity_pct > 0.0 {
            intent.quantity_pct
        } else {
            intent.quantity
        };
        if side == "BUY" {
            let base_capital = available_cash.ok_or_else(|| {
                "available cash is required to resolve quantity percentage".to_owned()
            })?;
            let notional = base_capital * (pct / 100.0);
            let price = if intent.has_limit_price && intent.limit_price > 0.0 {
                Some(intent.limit_price)
            } else {
                current_price.filter(|p| *p > 0.0)
            };
            let p = price.ok_or_else(|| {
                "price required to resolve quantity percentage".to_owned()
            })?;
            (notional / p).floor()
        } else {
            let pos = current_position
                .map(|p| p.abs())
                .filter(|p| *p > 0.0)
                .ok_or_else(|| {
                    "sellable position is required to resolve quantity percentage".to_owned()
                })?;
            (pos * (pct / 100.0)).floor()
        }
    } else if allow_default_quantity
        && matches!(kind.as_str(), "entry" | "order")
        && intent.quantity <= 0.0
    {
        1.0
    } else {
        return Err(format!(
            "strategy order intent {index} requires a positive finite quantity"
        ));
    };

    if let Some(lot) =
        binding_scalar_f64(binding, &["lotSize", "lot_size"]).filter(|lot| *lot > 0.0)
    {
        qty = (qty / lot).floor() * lot;
    }

    if qty <= 0.0 {
        return Ok(None);
    }

    Ok(Some((qty, reduce_only)))
}

fn binding_scalar_f64(binding: &Value, keys: &[&str]) -> Option<f64> {
    keys.iter().find_map(|key| {
        binding.get(*key).and_then(|v| {
            v.as_f64()
                .or_else(|| v.as_str().and_then(|s| s.parse::<f64>().ok()))
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
