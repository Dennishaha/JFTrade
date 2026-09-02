use jftrade_kernel::Fixed8;
use serde::{Deserialize, Serialize};

use crate::{OrderCommand, TradingEnvironment};

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct HardStop {
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
#[serde(rename_all = "camelCase")]
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

#[cfg(test)]
mod tests {
    use std::str::FromStr;

    use jftrade_kernel::Fixed8;

    use super::{
        RUNTIME_RISK_MODE_ENFORCE, RUNTIME_RISK_MODE_MONITOR, RUNTIME_RISK_MODE_OFF, RiskConfig,
        RiskEngine, RuntimeRiskContext, RuntimeRiskOrder, RuntimeRiskSettings,
        evaluate_runtime_risk,
    };
    use crate::{OrderCommand, OrderSide, TradingEnvironment};

    fn command(environment: TradingEnvironment) -> OrderCommand {
        OrderCommand {
            idempotency_key: "key-1".to_owned(),
            trace_id: "trace-1".to_owned(),
            broker_id: "futu".to_owned(),
            account_id: "acc-1".to_owned(),
            environment,
            market: "US".to_owned(),
            symbol: "AAPL".to_owned(),
            side: OrderSide::Buy,
            quantity: Fixed8::from_str("11").expect("quantity"),
            price: Some(Fixed8::from_str("10").expect("price")),
            client_order_id: "client-1".to_owned(),
        }
    }

    #[test]
    fn simulate_bypasses_real_trade_controls_but_real_fails_closed() {
        let engine = RiskEngine::new(RiskConfig {
            real_trading_enabled: false,
            kill_switch_active: true,
            max_order_quantity: None,
            max_order_notional: None,
            hard_stops: Vec::new(),
        });
        assert!(
            engine
                .evaluate(&command(TradingEnvironment::Simulate))
                .allowed
        );
        assert_eq!(
            engine
                .evaluate(&command(TradingEnvironment::Real))
                .reason_code
                .as_deref(),
            Some("REAL_TRADING_DISABLED")
        );
    }

    #[test]
    fn real_notional_uses_fixed_point_arithmetic() {
        let engine = RiskEngine::new(RiskConfig {
            real_trading_enabled: true,
            kill_switch_active: false,
            max_order_quantity: Some(Fixed8::from_str("20").expect("maximum quantity")),
            max_order_notional: Some(Fixed8::from_str("100").expect("maximum notional")),
            hard_stops: Vec::new(),
        });
        assert_eq!(
            engine
                .evaluate(&command(TradingEnvironment::Real))
                .reason_code
                .as_deref(),
            Some("MAX_ORDER_NOTIONAL_EXCEEDED")
        );
    }

    fn runtime_order(side: &str, quantity: &str, price: Option<&str>) -> RuntimeRiskOrder {
        RuntimeRiskOrder {
            symbol: "US.AAPL".to_owned(),
            side: side.to_owned(),
            quantity: quantity.parse().expect("quantity"),
            price: price.map(|value| value.parse().expect("price")),
        }
    }

    fn runtime_context(
        sellable: &str,
        current_price: Option<&str>,
        count: i64,
    ) -> RuntimeRiskContext {
        RuntimeRiskContext {
            current_price: current_price.map(|value| value.parse().expect("current price")),
            sellable_quantity: sellable.parse().expect("sellable quantity"),
            today_submitted_order_count: count,
        }
    }

    #[test]
    fn runtime_risk_normalizes_modes_and_clears_off_limits() {
        let normalized = RuntimeRiskSettings {
            mode: " unknown ".to_owned(),
            close_only: true,
            max_order_quantity: Some("-1".parse().expect("quantity")),
            max_order_notional: Some(Fixed8::POS_INFINITY),
            daily_max_orders: Some(0),
            pause_on_reject: true,
        }
        .normalize();
        assert_eq!(normalized.mode, RUNTIME_RISK_MODE_OFF);
        assert!(!normalized.close_only);
        assert!(normalized.max_order_quantity.is_none());
        assert!(normalized.max_order_notional.is_none());
        assert!(normalized.daily_max_orders.is_none());
        assert!(!normalized.pause_on_reject);

        assert_eq!(
            RuntimeRiskSettings {
                mode: " MONITOR ".to_owned(),
                ..RuntimeRiskSettings::default()
            }
            .normalize()
            .mode,
            RUNTIME_RISK_MODE_MONITOR
        );
    }

    #[test]
    fn runtime_risk_off_ignores_configured_limits() {
        let decision = evaluate_runtime_risk(
            RuntimeRiskSettings {
                mode: RUNTIME_RISK_MODE_OFF.to_owned(),
                max_order_quantity: Some("1".parse().expect("quantity")),
                ..RuntimeRiskSettings::default()
            },
            &runtime_order("BUY", "10", None),
            &RuntimeRiskContext::default(),
        );
        assert_eq!(decision, super::RuntimeRiskDecision::default());
    }

    #[test]
    fn runtime_risk_enforce_applies_close_only_quantity_notional_and_daily_limits() {
        let settings = RuntimeRiskSettings {
            mode: RUNTIME_RISK_MODE_ENFORCE.to_owned(),
            close_only: true,
            max_order_quantity: Some("5".parse().expect("quantity")),
            max_order_notional: Some("500".parse().expect("notional")),
            daily_max_orders: Some(3),
            pause_on_reject: true,
        };
        let cases = [
            ("BUY", "1", None, "close_only"),
            ("SELL", "5", None, "close_only_insufficient_position"),
            ("SELL", "4", Some("130"), "max_order_notional"),
            ("SELL", "4", Some("100"), ""),
        ];
        for (side, quantity, price, reason) in cases {
            let decision = evaluate_runtime_risk(
                settings.clone(),
                &runtime_order(side, quantity, price),
                &runtime_context("4", Some("100"), 0),
            );
            assert_eq!(decision.reason.as_deref().unwrap_or(""), reason);
            if reason.is_empty() {
                assert_eq!(decision, super::RuntimeRiskDecision::default());
            } else {
                assert!(decision.matched && decision.rejected && decision.pause_on_reject);
            }
        }

        let quantity = evaluate_runtime_risk(
            RuntimeRiskSettings {
                close_only: false,
                ..settings.clone()
            },
            &runtime_order("BUY", "6", None),
            &runtime_context("4", Some("100"), 0),
        );
        assert_eq!(quantity.reason.as_deref(), Some("max_order_quantity"));

        let daily = evaluate_runtime_risk(
            RuntimeRiskSettings {
                close_only: false,
                max_order_quantity: None,
                max_order_notional: None,
                daily_max_orders: Some(3),
                ..settings
            },
            &runtime_order("BUY", "1", None),
            &runtime_context("0", None, 3),
        );
        assert_eq!(daily.reason.as_deref(), Some("daily_max_orders"));
    }

    #[test]
    fn runtime_risk_monitor_records_match_without_rejecting_and_uses_fallback_price() {
        let decision = evaluate_runtime_risk(
            RuntimeRiskSettings {
                mode: RUNTIME_RISK_MODE_MONITOR.to_owned(),
                max_order_notional: Some("100".parse().expect("notional")),
                ..RuntimeRiskSettings::default()
            },
            &runtime_order("BUY", "2", None),
            &runtime_context("0", Some("60"), 0),
        );
        assert!(decision.matched);
        assert!(!decision.rejected);
        assert!(!decision.pause_on_reject);
        assert_eq!(decision.reason.as_deref(), Some("max_order_notional"));
        assert!(
            decision
                .detail
                .as_deref()
                .is_some_and(|detail| detail.starts_with("rule=max_order_notional"))
        );
    }

    #[test]
    fn runtime_order_can_be_derived_from_canonical_order_command() {
        let command = command(TradingEnvironment::Real);
        let order = RuntimeRiskOrder::from_command(&command);
        assert_eq!(order.symbol, "AAPL");
        assert_eq!(order.side, "BUY");
        assert_eq!(order.quantity, command.quantity);
        assert_eq!(order.price, command.price);
    }
}
