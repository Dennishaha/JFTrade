use jftrade_integration_pine::PineOrderIntent;
use jftrade_store_sqlite::{StoredExecutionOrder, StrategyRuntimeStore};
use jftrade_trading::VirtualAccountState;
use serde_json::Value;

use crate::product::product_active_provider_state::ActiveProviderState;

use super::strategy_runtime_execution::{now_millis, now_rfc3339, strategy_client_order_id};

pub(crate) fn binding_scalar_string(binding: &Value, keys: &[&str]) -> Option<String> {
    keys.iter()
        .find_map(|key| binding.get(*key).and_then(value_scalar_string))
}

pub(crate) fn nested_binding_scalar_string(
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

pub(crate) fn value_scalar_string(value: &Value) -> Option<String> {
    match value {
        Value::String(value) => {
            let value = value.trim();
            (!value.is_empty()).then_some(value.to_owned())
        }
        Value::Number(value) => Some(value.to_string()),
        _ => None,
    }
}

pub(crate) fn execution_binding_is_real(binding: &Value) -> bool {
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

pub(crate) fn binding_scalar_f64(binding: &Value, keys: &[&str]) -> Option<f64> {
    keys.iter().find_map(|key| {
        binding.get(*key).and_then(|v| {
            v.as_f64()
                .or_else(|| v.as_str().and_then(|s| s.parse::<f64>().ok()))
        })
    })
}

pub(crate) fn validate_strategy_execution_binding(
    binding: &Value,
    provider: &ActiveProviderState,
) -> Result<(), String> {
    if binding_scalar_string(binding, &["brokerId", "broker"])
        .or_else(|| nested_binding_scalar_string(binding, "brokerAccount", &["brokerId", "broker"]))
        .is_none()
        && provider.snapshot().provider != Some(jftrade_settings::MarketDataProvider::Futu)
    {
        return Err("strategy execution broker is not configured".to_owned());
    }
    for keys in [
        &["accountId", "account"][..],
        &["tradingEnvironment", "environment", "env"][..],
    ] {
        if binding_scalar_string(binding, keys)
            .or_else(|| nested_binding_scalar_string(binding, "brokerAccount", keys))
            .is_none()
        {
            return Err(format!("strategy execution {} is not configured", keys[0]));
        }
    }
    let (_, account, _) = strategy_execution_binding(binding, provider)?;
    account
        .parse::<u64>()
        .map_err(|_| "strategy execution accountId must be numeric for Futu".to_owned())?;
    strategy_execution_binding(binding, provider).map(|_| ())
}

pub(crate) fn strategy_execution_binding(
    binding: &Value,
    provider: &ActiveProviderState,
) -> Result<(String, String, String), String> {
    let is_real = execution_binding_is_real(binding);
    let broker_id = binding_scalar_string(binding, &["brokerId", "broker"])
        .or_else(|| nested_binding_scalar_string(binding, "brokerAccount", &["brokerId", "broker"]))
        .or_else(|| {
            (provider.snapshot().provider == Some(jftrade_settings::MarketDataProvider::Futu))
                .then_some("futu".to_owned())
        })
        .or_else(|| (!is_real).then(|| "simulated".to_owned()))
        .ok_or_else(|| "strategy execution broker is not configured".to_owned())?;
    let account_id = binding_scalar_string(binding, &["accountId", "account"])
        .or_else(|| {
            nested_binding_scalar_string(binding, "brokerAccount", &["accountId", "account"])
        })
        .or_else(|| (!is_real).then(|| "10001".to_owned()))
        .ok_or_else(|| "strategy execution accountId is not configured".to_owned())?;
    if is_real && broker_id == "futu" && account_id.parse::<u64>().is_err() {
        return Err("strategy execution accountId must be numeric for Futu".to_owned());
    }
    let trading_environment =
        binding_scalar_string(binding, &["tradingEnvironment", "environment", "env"])
            .or_else(|| {
                nested_binding_scalar_string(
                    binding,
                    "brokerAccount",
                    &["tradingEnvironment", "environment", "env"],
                )
            })
            .unwrap_or_else(|| {
                if is_real {
                    "REAL".to_owned()
                } else {
                    "SIMULATE".to_owned()
                }
            })
            .to_ascii_uppercase();
    if !matches!(trading_environment.as_str(), "REAL" | "SIMULATE") {
        return Err("strategy execution tradingEnvironment must be REAL or SIMULATE".to_owned());
    }
    Ok((broker_id, account_id, trading_environment))
}

/// Restores the virtual account state from the latest `SIMULATE_ACCOUNT_CHECKPOINT`
/// audit event for the strategy instance. If no checkpoint exists, initializes a fresh
/// virtual account using configured initial cash (default 100,000.0).
#[cfg(test)]
pub(super) fn restore_or_init_virtual_account(
    store: &StrategyRuntimeStore,
    instance_id: &str,
    binding: &Value,
) -> Result<VirtualAccountState, String> {
    let events = store
        .list_audit_events(instance_id)
        .map_err(|error| format!("read simulate recovery checkpoints: {error}"))?;

    for event in events {
        if event.kind == "SIMULATE_ACCOUNT_CHECKPOINT" {
            let state: VirtualAccountState = serde_json::from_str(&event.detail)
                .map_err(|error| format!("invalid simulate recovery checkpoint: {error}"))?;
            return Ok(state);
        }
    }

    let account_id = binding_scalar_string(binding, &["accountId", "account"])
        .or_else(|| {
            nested_binding_scalar_string(binding, "brokerAccount", &["accountId", "account"])
        })
        .unwrap_or_else(|| format!("sim-{instance_id}"));

    let initial_cash =
        binding_scalar_f64(binding, &["initialCash", "initial_cash", "capital", "cash"])
            .unwrap_or(100_000.0);

    Ok(VirtualAccountState::new(
        account_id,
        initial_cash,
        now_millis(),
    ))
}

/// Persists the virtual account state as a `SIMULATE_ACCOUNT_CHECKPOINT` audit event.
pub(super) fn persist_simulate_account_checkpoint(
    store: &StrategyRuntimeStore,
    instance_id: &str,
    account: &VirtualAccountState,
) -> Result<(), String> {
    let detail = serde_json::to_string(account)
        .map_err(|error| format!("serialize simulate account checkpoint: {error}"))?;
    store
        .append_audit_event(
            instance_id,
            "SIMULATE_ACCOUNT_CHECKPOINT",
            &detail,
            now_millis(),
        )
        .map_err(|error| format!("persist simulate account checkpoint: {error}"))
}

/// Matches an order locally at fallback price in offline simulate mode, updates
/// `VirtualAccountState`, records an `ORDER_FILLED` audit event, persists a
/// `SIMULATE_ACCOUNT_CHECKPOINT`, and persists the order to `execution_store` if present.
#[allow(clippy::too_many_arguments)]
pub(super) fn simulate_fill_order(
    store: &StrategyRuntimeStore,
    execution_store: Option<&jftrade_store_sqlite::ExecutionOrderStore>,
    instance_id: &str,
    market: &str,
    symbol: &str,
    side: &str,
    quantity: f64,
    fill_price: f64,
    fee: f64,
    intent: &PineOrderIntent,
    index: usize,
    virtual_account: &mut VirtualAccountState,
) -> Result<f64, String> {
    let now = now_millis();
    let pnl = virtual_account
        .apply_fill(market, symbol, side, quantity, fill_price, fee, now)
        .map_err(|error| format!("simulate fill: {error}"))?;

    let client_order_id = strategy_client_order_id(instance_id, symbol, intent, index);
    let detail = format!(
        "{symbol} {side} {quantity} @ {fill_price} (clientOrderId: {client_order_id}; realizedPnl: {pnl})"
    );

    store
        .append_audit_event(instance_id, "ORDER_FILLED", &detail, now)
        .map_err(|error| format!("persist ORDER_FILLED audit: {error}"))?;

    persist_simulate_account_checkpoint(store, instance_id, virtual_account)?;

    if let Some(ord_store) = execution_store {
        let now_str = now_rfc3339()?;
        let internal_order_id = format!(
            "sim-ord-{instance_id}-{symbol}-{}-{index}",
            intent.bar_index
        );
        let stored_order = StoredExecutionOrder {
            internal_order_id: internal_order_id.clone(),
            broker_id: "simulated".to_owned(),
            broker_order_id: Some(internal_order_id.clone()),
            broker_order_id_ex: None,
            source: "strategy-runtime".to_owned(),
            source_detail: instance_id.to_owned(),
            trading_environment: "SIMULATE".to_owned(),
            account_id: virtual_account.account_id.clone(),
            market: market.to_owned(),
            symbol: Some(symbol.to_owned()),
            side: Some(side.to_owned()),
            order_type: Some(if intent.has_limit_price {
                "LIMIT".to_owned()
            } else {
                "MARKET".to_owned()
            }),
            status: "FILLED".to_owned(),
            raw_broker_status: Some("FILLED".to_owned()),
            requested_quantity: Some(quantity),
            requested_price: Some(fill_price),
            filled_quantity: Some(quantity),
            filled_average_price: Some(fill_price),
            remark: Some(format!("strategy simulate execution {instance_id}")),
            last_error: None,
            last_error_code: None,
            last_error_source: None,
            submitted_at: Some(now_str.clone()),
            updated_at: now_str.clone(),
            created_at: now_str.clone(),
            order_kind: intent.kind.clone(),
            product_class: "STOCK".to_owned(),
            quantity_mode: "UNITS".to_owned(),
            client_order_id: Some(client_order_id),
            preview_id: None,
            normalized_request: "{}".to_owned(),
            requested_amount: Some(quantity * fill_price),
            payout: None,
            fees: Some(fee),
        };
        let _ = ord_store.save_order(stored_order, &now_str);
    }

    Ok(pnl)
}

/// Dispatches an offline simulated fill or records the fill against the virtual account.
#[allow(clippy::too_many_arguments)]
pub(super) fn dispatch_simulated_fill(
    store: &StrategyRuntimeStore,
    execution_store: Option<&jftrade_store_sqlite::ExecutionOrderStore>,
    instance_id: &str,
    market: &str,
    symbol: &str,
    side: &str,
    quantity: f64,
    fallback_price: Option<f64>,
    intent: &PineOrderIntent,
    index: usize,
    virtual_account: Option<&mut VirtualAccountState>,
) -> Result<(), String> {
    if let Some(vaccount) = virtual_account {
        let fill_price = if intent.has_limit_price && intent.limit_price > 0.0 {
            intent.limit_price
        } else {
            fallback_price.unwrap_or(0.0)
        };
        simulate_fill_order(
            store,
            execution_store,
            instance_id,
            market,
            symbol,
            side,
            quantity,
            fill_price,
            0.0,
            intent,
            index,
            vaccount,
        )
        .map(|_| ())
    } else {
        Err("strategy execution order port is unavailable".to_owned())
    }
}
