use jftrade_kernel::Fixed8;
use serde::{Deserialize, Serialize};

use crate::{OrderCommand, TradingEnvironment};

#[derive(Clone, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct HardStop {
    #[serde(default)]
    pub id: Option<String>,
    pub broker_id: Option<String>,
    /// Optional environment scope retained from the Go control-plane entry.
    /// An omitted, blank, or `*` value matches either environment; populated
    /// values are compared case-insensitively against the command's canonical
    /// environment string.
    pub trading_environment: Option<String>,
    pub account_id: Option<String>,
    pub market: Option<String>,
    pub symbol: Option<String>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct RiskConfig {
    pub real_trading_enabled: bool,
    pub kill_switch_active: bool,
    pub max_order_quantity: Option<Fixed8>,
    pub max_order_notional: Option<Fixed8>,
    #[serde(default)]
    pub hard_stops: Vec<HardStop>,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct RiskDecision {
    pub allowed: bool,
    pub reason_code: Option<String>,
}

impl RiskDecision {
    fn allow() -> Self {
        Self {
            allowed: true,
            reason_code: None,
        }
    }

    fn reject(reason: &str) -> Self {
        Self {
            allowed: false,
            reason_code: Some(reason.to_owned()),
        }
    }
}

pub const RUNTIME_RISK_MODE_OFF: &str = "off";
pub const RUNTIME_RISK_MODE_MONITOR: &str = "monitor";
pub const RUNTIME_RISK_MODE_ENFORCE: &str = "enforce";

/// Per-strategy runtime limits. These controls are intentionally separate from
/// [`RiskConfig`]: the latter protects real broker submission, while this
/// policy describes a strategy's close-only/monitor/enforce behavior before a
/// trading plan is handed to the broker-neutral port.
#[derive(Clone, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", default)]
pub struct RuntimeRiskSettings {
    pub mode: String,
    pub close_only: bool,
    pub max_order_quantity: Option<Fixed8>,
    pub max_order_notional: Option<Fixed8>,
    pub daily_max_orders: Option<i64>,
    pub pause_on_reject: bool,
}

impl RuntimeRiskSettings {
    /// Applies the same normalization used by the strategy runtime: unknown
    /// modes become `off`, non-positive limits are omitted, and an off policy
    /// cannot retain stale enforcement flags.
    pub fn normalize(mut self) -> Self {
        self.mode = match self.mode.trim().to_ascii_lowercase().as_str() {
            RUNTIME_RISK_MODE_MONITOR => RUNTIME_RISK_MODE_MONITOR.to_owned(),
            RUNTIME_RISK_MODE_ENFORCE => RUNTIME_RISK_MODE_ENFORCE.to_owned(),
            _ => RUNTIME_RISK_MODE_OFF.to_owned(),
        };
        self.max_order_quantity = normalize_positive_fixed(self.max_order_quantity);
        self.max_order_notional = normalize_positive_fixed(self.max_order_notional);
        self.daily_max_orders = self.daily_max_orders.filter(|maximum| *maximum > 0);
        if self.mode == RUNTIME_RISK_MODE_OFF {
            self.close_only = false;
            self.max_order_quantity = None;
            self.max_order_notional = None;
            self.daily_max_orders = None;
            self.pause_on_reject = false;
        }
        self
    }

    pub fn evaluate(
        &self,
        order: &RuntimeRiskOrder,
        context: &RuntimeRiskContext,
    ) -> RuntimeRiskDecision {
        evaluate_runtime_risk(self.clone(), order, context)
    }
}

/// The order fields needed by strategy runtime controls. This deliberately
/// does not carry broker/account ownership, which remains on `OrderCommand`.
#[derive(Clone, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct RuntimeRiskOrder {
    pub symbol: String,
    pub side: String,
    pub quantity: Fixed8,
    pub price: Option<Fixed8>,
}

impl RuntimeRiskOrder {
    pub fn from_command(command: &OrderCommand) -> Self {
        Self {
            symbol: command.symbol.clone(),
            side: command.side.as_str().to_owned(),
            quantity: command.quantity,
            price: command.price,
        }
    }
}

/// Dynamic values supplied by the strategy runtime when evaluating an order.
#[derive(Clone, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct RuntimeRiskContext {
    pub current_price: Option<Fixed8>,
    pub sellable_quantity: Fixed8,
    pub today_submitted_order_count: i64,
}

#[derive(Clone, Debug, Default, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct RuntimeRiskDecision {
    pub matched: bool,
    pub rejected: bool,
    pub pause_on_reject: bool,
    pub reason: Option<String>,
    pub detail: Option<String>,
}

impl RuntimeRiskDecision {
    pub fn reason_code(&self) -> Option<&str> {
        self.reason.as_deref()
    }
}

/// Evaluates strategy-scoped limits without mutating broker or portfolio
/// state. Monitor mode records a match while enforce mode rejects it.
pub fn evaluate_runtime_risk(
    settings: RuntimeRiskSettings,
    order: &RuntimeRiskOrder,
    context: &RuntimeRiskContext,
) -> RuntimeRiskDecision {
    let settings = settings.normalize();
    if settings.mode == RUNTIME_RISK_MODE_OFF {
        return RuntimeRiskDecision::default();
    }
    let Some(reason) = runtime_reject_reason(&settings, order, context) else {
        return RuntimeRiskDecision::default();
    };
    RuntimeRiskDecision {
        matched: true,
        rejected: settings.mode == RUNTIME_RISK_MODE_ENFORCE,
        pause_on_reject: settings.pause_on_reject,
        reason: Some(reason.to_owned()),
        detail: Some(runtime_risk_detail(&settings, order, reason)),
    }
}

fn runtime_reject_reason<'a>(
    settings: &RuntimeRiskSettings,
    order: &RuntimeRiskOrder,
    context: &RuntimeRiskContext,
) -> Option<&'a str> {
    let side = order.side.trim().to_ascii_uppercase();
    if settings.close_only {
        if side != "SELL" {
            return Some("close_only");
        }
        let tolerance = Fixed8::from_scaled(10);
        let sellable = context
            .sellable_quantity
            .checked_add(tolerance)
            .unwrap_or(Fixed8::POS_INFINITY);
        if order.quantity > sellable {
            return Some("close_only_insufficient_position");
        }
    }
    if settings
        .max_order_quantity
        .is_some_and(|maximum| order.quantity > maximum)
    {
        return Some("max_order_quantity");
    }
    if let Some(maximum) = settings.max_order_notional {
        let price = order
            .price
            .filter(|value| value.signum() > 0)
            .or_else(|| context.current_price.filter(|value| value.signum() > 0));
        let Some(price) = price else {
            return Some("max_order_notional_missing_price");
        };
        let Ok(notional) = order.quantity.checked_mul(price) else {
            return Some("max_order_notional");
        };
        if notional > maximum {
            return Some("max_order_notional");
        }
    }
    if settings
        .daily_max_orders
        .is_some_and(|maximum| context.today_submitted_order_count >= maximum)
    {
        return Some("daily_max_orders");
    }
    None
}

fn runtime_risk_detail(
    settings: &RuntimeRiskSettings,
    order: &RuntimeRiskOrder,
    reason: &str,
) -> String {
    format!(
        "rule={reason} symbol={} side={} qty={} mode={} closeOnly={} maxQty={} maxNotional={} dailyMaxOrders={}",
        order.symbol,
        order.side,
        format_fixed8(order.quantity),
        settings.mode,
        settings.close_only,
        optional_fixed_label(settings.max_order_quantity),
        optional_fixed_label(settings.max_order_notional),
        settings
            .daily_max_orders
            .map_or_else(|| "none".to_owned(), |value| value.to_string()),
    )
}

fn normalize_positive_fixed(value: Option<Fixed8>) -> Option<Fixed8> {
    value.filter(|value| {
        value.signum() > 0 && *value != Fixed8::POS_INFINITY && *value != Fixed8::NEG_INFINITY
    })
}

fn optional_fixed_label(value: Option<Fixed8>) -> String {
    value.map_or_else(|| "none".to_owned(), format_fixed8)
}

fn format_fixed8(value: Fixed8) -> String {
    let Ok(number) = value.to_f64() else {
        return value.storage_text();
    };
    if number == 0.0 {
        return "0".to_owned();
    }
    let text = format!("{number:.4}");
    text.trim_end_matches('0').trim_end_matches('.').to_owned()
}

#[derive(Clone, Debug)]
pub struct RiskEngine {
    config: RiskConfig,
}

impl RiskEngine {
    pub fn new(config: RiskConfig) -> Self {
        Self { config }
    }

    pub fn evaluate(&self, command: &OrderCommand) -> RiskDecision {
        if command.environment == TradingEnvironment::Simulate {
            return RiskDecision::allow();
        }
        if !self.config.real_trading_enabled {
            return RiskDecision::reject("REAL_TRADING_DISABLED");
        }
        if self.config.kill_switch_active {
            return RiskDecision::reject("REAL_TRADE_KILL_SWITCH_ACTIVE");
        }
        if self
            .config
            .hard_stops
            .iter()
            .any(|hard_stop| hard_stop.matches(command))
        {
            return RiskDecision::reject("REAL_TRADE_HARD_STOP_ACTIVE");
        }
        if self
            .config
            .max_order_quantity
            .is_some_and(|maximum| command.quantity > maximum)
        {
            return RiskDecision::reject("MAX_ORDER_QUANTITY_EXCEEDED");
        }
        if let Some(maximum) = self.config.max_order_notional {
            let Some(price) = command.price else {
                return RiskDecision::reject("RISK_PRICE_UNAVAILABLE");
            };
            let Ok(notional) = command.quantity.checked_mul(price) else {
                return RiskDecision::reject("INVALID_ORDER_RISK_SHAPE");
            };
            if notional > maximum {
                return RiskDecision::reject("MAX_ORDER_NOTIONAL_EXCEEDED");
            }
        }
        RiskDecision::allow()
    }
}

impl HardStop {
    fn matches(&self, command: &OrderCommand) -> bool {
        owner_matches(&self.broker_id, &command.broker_id)
            && owner_matches(&self.trading_environment, command.environment.as_str())
            && owner_matches(&self.account_id, &command.account_id)
            && exact_matches(&self.market, &command.market)
            && symbol_matches(&self.symbol, &command.market, &command.symbol)
    }

    pub fn matches_pre_trade(&self, order: &PreTradeRiskOrder) -> bool {
        owner_matches(&self.broker_id, &order.broker_id)
            && owner_matches(
                &self.trading_environment,
                order.trading_environment.as_str(),
            )
            && owner_matches(&self.account_id, &order.account_id)
            && exact_matches(&self.market, &order.market)
            && symbol_matches(&self.symbol, &order.market, &order.symbol)
    }
}

/// Pure domain order payload for pre-trade risk evaluation.
#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct PreTradeRiskOrder {
    pub broker_id: String,
    pub trading_environment: TradingEnvironment,
    pub account_id: String,
    pub market: String,
    pub symbol: String,
    pub side: String,
    pub order_type: String,
    pub order_kind: String,
    pub product_class: String,
    pub quantity_mode: String,
    pub quantity: Fixed8,
    pub price: Option<Fixed8>,
    pub amount: Option<Fixed8>,
    #[serde(default)]
    pub legs: Vec<PreTradeRiskComboLeg>,
}

/// Single leg in a multi-leg combo order for pre-trade risk evaluation.
#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct PreTradeRiskComboLeg {
    pub symbol: String,
    pub market: String,
    pub side: String,
    pub quantity: Fixed8,
    pub multiplier: Fixed8,
    pub price: Option<Fixed8>,
    #[serde(default)]
    pub product_class: String,
}

/// Domain-neutral snapshot of the control-plane policy.
#[derive(Clone, Debug, Default, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct PreTradeRiskPolicy {
    pub control_plane_available: bool,
    pub real_trading_enabled: bool,
    pub kill_switch_active: bool,
    pub effective_max_order_quantity: Option<Fixed8>,
    pub effective_max_order_notional: Option<Fixed8>,
    #[serde(default)]
    pub hard_stops: Vec<HardStop>,
}

/// Evaluation outcome for pre-trade submission gates.
#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct PreTradeRiskDecision {
    pub allowed: bool,
    pub reason_code: Option<String>,
    pub reason_message: Option<String>,
    #[serde(default)]
    pub matched_hard_stop_id: Option<String>,
}

impl PreTradeRiskDecision {
    pub fn allow() -> Self {
        Self {
            allowed: true,
            reason_code: None,
            reason_message: None,
            matched_hard_stop_id: None,
        }
    }

    pub fn reject(code: impl Into<String>, message: impl Into<String>) -> Self {
        Self {
            allowed: false,
            reason_code: Some(code.into()),
            reason_message: Some(message.into()),
            matched_hard_stop_id: None,
        }
    }
}

/// Evaluates pre-trade risk against a pure domain policy snapshot.
///
/// Shape validation applies to both SIMULATE and REAL environments.
/// Real trading controls fail closed if the control plane is unavailable.
pub fn evaluate_pre_trade_risk(
    policy: &PreTradeRiskPolicy,
    order: &PreTradeRiskOrder,
) -> PreTradeRiskDecision {
    let is_amount_mode = order.quantity_mode.eq_ignore_ascii_case("amount");
    if is_amount_mode {
        let Some(amount) = order.amount else {
            return PreTradeRiskDecision::reject(
                "INVALID_ORDER_RISK_SHAPE",
                "order amount is required in amount mode",
            );
        };
        if amount.signum() <= 0 {
            return PreTradeRiskDecision::reject(
                "INVALID_ORDER_RISK_SHAPE",
                "order amount must be positive in amount mode",
            );
        }
    } else {
        if order.quantity.signum() <= 0 {
            return PreTradeRiskDecision::reject(
                "INVALID_ORDER_RISK_SHAPE",
                "order quantity must be positive",
            );
        }
        for leg in &order.legs {
            if leg.quantity.signum() <= 0 {
                return PreTradeRiskDecision::reject(
                    "INVALID_ORDER_RISK_SHAPE",
                    "combo leg quantity must be positive",
                );
            }
        }
    }

    if order.trading_environment == TradingEnvironment::Simulate {
        return PreTradeRiskDecision::allow();
    }

    if !policy.control_plane_available {
        return PreTradeRiskDecision::reject(
            "PRE_TRADE_RISK_UNAVAILABLE",
            "pre-trade risk gateway is unavailable; REAL orders are blocked",
        );
    }

    if !policy.real_trading_enabled {
        return PreTradeRiskDecision::reject(
            "REAL_TRADING_DISABLED",
            "real trading is disabled; enable runtime real-trade risk config before placing REAL orders",
        );
    }

    if policy.kill_switch_active {
        return PreTradeRiskDecision::reject(
            "REAL_TRADE_KILL_SWITCH_ACTIVE",
            "real-trade kill switch is active; PLACE orders are blocked",
        );
    }

    if let Some(hs) = policy
        .hard_stops
        .iter()
        .find(|hs| hs.matches_pre_trade(order))
    {
        let mut decision = PreTradeRiskDecision::reject(
            "REAL_TRADE_HARD_STOP_ACTIVE",
            "real-trade hard stop is active for this order scope; PLACE orders are blocked",
        );
        decision.matched_hard_stop_id = hs.id.clone();
        return decision;
    }

    for leg in &order.legs {
        let leg_order = PreTradeRiskOrder {
            broker_id: order.broker_id.clone(),
            trading_environment: order.trading_environment,
            account_id: order.account_id.clone(),
            market: leg.market.clone(),
            symbol: leg.symbol.clone(),
            side: leg.side.clone(),
            order_type: order.order_type.clone(),
            order_kind: order.order_kind.clone(),
            product_class: leg.product_class.clone(),
            quantity_mode: order.quantity_mode.clone(),
            quantity: leg.quantity,
            price: leg.price,
            amount: None,
            legs: Vec::new(),
        };
        if let Some(hs) = policy
            .hard_stops
            .iter()
            .find(|hs| hs.matches_pre_trade(&leg_order))
        {
            let mut decision = PreTradeRiskDecision::reject(
                "REAL_TRADE_HARD_STOP_ACTIVE",
                "real-trade hard stop is active for combo leg scope; PLACE orders are blocked",
            );
            decision.matched_hard_stop_id = hs.id.clone();
            return decision;
        }
    }

    if !is_amount_mode && let Some(maximum) = policy.effective_max_order_quantity {
        if order.quantity > maximum {
            return PreTradeRiskDecision::reject(
                "MAX_ORDER_QUANTITY_EXCEEDED",
                "order quantity exceeds the configured real-trade limit",
            );
        }
        for leg in &order.legs {
            if leg.quantity > maximum {
                return PreTradeRiskDecision::reject(
                    "MAX_ORDER_QUANTITY_EXCEEDED",
                    "combo leg quantity exceeds the configured real-trade limit",
                );
            }
        }
    }

    if let Some(maximum) = policy.effective_max_order_notional {
        let notional = if is_amount_mode {
            order.amount.ok_or("RISK_AMOUNT_UNAVAILABLE")
        } else if !order.legs.is_empty() {
            match calculate_combo_notional(order) {
                Ok(val) => Ok(val),
                Err(decision) => return decision,
            }
        } else {
            let Some(price) = order.price else {
                return PreTradeRiskDecision::reject(
                    "RISK_PRICE_UNAVAILABLE",
                    "order price is required to enforce the configured real-trade notional limit",
                );
            };
            let multiplier = if order.product_class.eq_ignore_ascii_case("option") {
                Fixed8::from_scaled(100 * 100_000_000)
            } else {
                Fixed8::from_scaled(100_000_000)
            };
            order
                .quantity
                .checked_mul(price)
                .and_then(|val| val.checked_mul(multiplier))
                .map_err(|_| "INVALID_ORDER_RISK_SHAPE")
        };
        match notional {
            Ok(val) => {
                if val > maximum {
                    return PreTradeRiskDecision::reject(
                        "MAX_ORDER_NOTIONAL_EXCEEDED",
                        "order notional exceeds the configured real-trade limit",
                    );
                }
            }
            Err(code) => {
                let msg = if code == "RISK_AMOUNT_UNAVAILABLE" {
                    "order amount is required to enforce the configured real-trade limit"
                } else if code == "RISK_PRICE_UNAVAILABLE" {
                    "order price is required to enforce the configured real-trade notional limit"
                } else {
                    "invalid order risk shape for notional calculation"
                };
                return PreTradeRiskDecision::reject(code, msg);
            }
        }
    }

    PreTradeRiskDecision::allow()
}

fn calculate_combo_notional(order: &PreTradeRiskOrder) -> Result<Fixed8, PreTradeRiskDecision> {
    let all_legs_have_price = order.legs.iter().all(|leg| leg.price.is_some());
    if all_legs_have_price {
        let mut net_notional = Fixed8::ZERO;
        for leg in &order.legs {
            let price = leg.price.unwrap();
            let multiplier = if leg.multiplier > Fixed8::ZERO {
                leg.multiplier
            } else if leg.product_class.eq_ignore_ascii_case("option") {
                Fixed8::from_scaled(100 * 100_000_000)
            } else {
                Fixed8::from_scaled(100_000_000)
            };
            let leg_notional = leg
                .quantity
                .checked_mul(price)
                .and_then(|val| val.checked_mul(multiplier))
                .map_err(|_| {
                    PreTradeRiskDecision::reject(
                        "INVALID_ORDER_RISK_SHAPE",
                        "invalid order risk shape for notional calculation",
                    )
                })?;
            let is_sell = leg.side.eq_ignore_ascii_case("sell");
            let next_net = if is_sell {
                net_notional.checked_sub(leg_notional)
            } else {
                net_notional.checked_add(leg_notional)
            };
            net_notional = next_net.map_err(|_| {
                PreTradeRiskDecision::reject(
                    "INVALID_ORDER_RISK_SHAPE",
                    "invalid order risk shape for notional calculation",
                )
            })?;
        }
        let abs_notional = if net_notional < Fixed8::ZERO {
            Fixed8::ZERO.checked_sub(net_notional).map_err(|_| {
                PreTradeRiskDecision::reject("INVALID_ORDER_RISK_SHAPE", "notional overflow")
            })?
        } else {
            net_notional
        };
        Ok(abs_notional)
    } else if let Some(combo_price) = order.price {
        let multiplier = if order
            .legs
            .iter()
            .any(|l| l.product_class.eq_ignore_ascii_case("option"))
            || order.product_class.eq_ignore_ascii_case("option")
        {
            Fixed8::from_scaled(100 * 100_000_000)
        } else {
            Fixed8::from_scaled(100_000_000)
        };
        let price_abs = if combo_price < Fixed8::ZERO {
            Fixed8::ZERO.checked_sub(combo_price).map_err(|_| {
                PreTradeRiskDecision::reject("INVALID_ORDER_RISK_SHAPE", "notional overflow")
            })?
        } else {
            combo_price
        };
        order
            .quantity
            .checked_mul(price_abs)
            .and_then(|val| val.checked_mul(multiplier))
            .map_err(|_| {
                PreTradeRiskDecision::reject(
                    "INVALID_ORDER_RISK_SHAPE",
                    "invalid order risk shape for notional calculation",
                )
            })
    } else {
        Err(PreTradeRiskDecision::reject(
            "RISK_PRICE_UNAVAILABLE",
            "order price or leg prices are required to enforce the configured real-trade notional limit",
        ))
    }
}

fn owner_matches(expected: &Option<String>, actual: &str) -> bool {
    expected.as_ref().is_none_or(|value| {
        let value = value.trim();
        value.is_empty() || value == "*" || value.eq_ignore_ascii_case(actual.trim())
    })
}

fn exact_matches(expected: &Option<String>, actual: &str) -> bool {
    expected
        .as_ref()
        .is_none_or(|value| value.trim().is_empty() || value.trim().eq_ignore_ascii_case(actual))
}

fn symbol_matches(expected: &Option<String>, market: &str, symbol: &str) -> bool {
    let Some(expected) = expected
        .as_ref()
        .map(|value| value.trim())
        .filter(|value| !value.is_empty())
    else {
        return true;
    };
    let expected = expected.to_ascii_uppercase();
    let market = market.trim().to_ascii_uppercase();
    let symbol = symbol.trim().to_ascii_uppercase();
    expected == symbol
        || !market.is_empty() && expected == format!("{market}.{symbol}")
        || !market.is_empty() && symbol == format!("{market}.{expected}")
}
