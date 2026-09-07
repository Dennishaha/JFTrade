use std::collections::BTreeMap;

use jftrade_kernel::{Decimal, DecimalTradingExt};

use crate::BacktestError;
use crate::model::{FeeBreakdown, FeeRule};

#[derive(Clone, Copy, Debug, Default)]
pub(crate) struct AppliedFees {
    pub broker: Decimal,
    pub market: Decimal,
    pub total: Decimal,
}

#[derive(Clone, Copy, Debug, Default)]
struct RuleAccumulator {
    raw: Decimal,
    notional: Decimal,
    charged: Decimal,
    order_used: bool,
}

#[derive(Clone, Debug)]
struct BreakdownAccumulator {
    label: String,
    group: String,
    amount: Decimal,
    count: usize,
}

pub(crate) struct FeeEngine {
    rules: Vec<FeeRule>,
    orders: BTreeMap<String, RuleAccumulator>,
    breakdown: BTreeMap<String, BreakdownAccumulator>,
    broker_total: Decimal,
    market_total: Decimal,
}

impl FeeEngine {
    pub fn new(rules: &[FeeRule]) -> Self {
        Self {
            rules: rules.to_vec(),
            orders: BTreeMap::new(),
            breakdown: BTreeMap::new(),
            broker_total: Decimal::ZERO,
            market_total: Decimal::ZERO,
        }
    }

    pub fn apply(
        &mut self,
        order_id: u64,
        side: &str,
        price: Decimal,
        quantity: Decimal,
    ) -> Result<AppliedFees, BacktestError> {
        let notional = price
            .checked_mul(quantity)
            .ok_or_else(|| BacktestError::Arithmetic("fee notional overflow".to_owned()))?;
        // A trade without a positive notional or quantity is not billable.
        // Keep this guard at the fee boundary so an order-basis fixed charge
        // cannot turn a zero-value trade into a fee-bearing event.
        if notional <= Decimal::ZERO || quantity <= Decimal::ZERO {
            return Ok(AppliedFees::default());
        }
        let mut applied = AppliedFees::default();
        for rule in self.rules.clone() {
            if !side_matches(&rule.side, side) {
                continue;
            }
            let key = format!("{}|{}|{order_id}", rule.group, rule.id);
            let accumulator = self.orders.entry(key).or_default();
            let raw = raw_fee(&rule, accumulator, notional, quantity)?;
            if raw <= Decimal::ZERO {
                continue;
            }
            accumulator.raw = accumulator.raw.checked_add(raw).ok_or_else(|| {
                BacktestError::Arithmetic("fee raw accumulator overflow".to_owned())
            })?;
            accumulator.notional = accumulator.notional.checked_add(notional).ok_or_else(|| {
                BacktestError::Arithmetic("fee notional accumulator overflow".to_owned())
            })?;
            let mut target = accumulator.raw;
            if rule.min_amount > Decimal::ZERO && target < rule.min_amount {
                target = rule.min_amount;
            }
            let cap = cap_amount(&rule, accumulator.notional)?;
            if cap > Decimal::ZERO && target > cap {
                target = cap;
            }
            target = round_fee(target, &rule.rounding)?;
            let incremental = target
                .checked_sub(accumulator.charged)
                .ok_or_else(|| BacktestError::Arithmetic("fee incremental underflow".to_owned()))?;
            if incremental <= Decimal::ZERO {
                continue;
            }
            accumulator.charged = target;
            match rule.group.trim().to_ascii_lowercase().as_str() {
                "broker" => {
                    applied.broker = applied.broker.checked_add(incremental).ok_or_else(|| {
                        BacktestError::Arithmetic("applied broker fee overflow".to_owned())
                    })?;
                }
                "market" => {
                    applied.market = applied.market.checked_add(incremental).ok_or_else(|| {
                        BacktestError::Arithmetic("applied market fee overflow".to_owned())
                    })?;
                }
                _ => {
                    return Err(BacktestError::InvalidInput(format!(
                        "fee rule {} has unsupported group {}",
                        rule.id, rule.group
                    )));
                }
            }
            let breakdown_key = format!("{}|{}", rule.group, rule.id);
            let entry =
                self.breakdown
                    .entry(breakdown_key)
                    .or_insert_with(|| BreakdownAccumulator {
                        label: rule.label.clone(),
                        group: rule.group.clone(),
                        amount: Decimal::ZERO,
                        count: 0,
                    });
            entry.amount = entry
                .amount
                .checked_add(incremental)
                .ok_or_else(|| BacktestError::Arithmetic("fee breakdown overflow".to_owned()))?;
            entry.count += 1;
        }
        applied.total = applied
            .broker
            .checked_add(applied.market)
            .ok_or_else(|| BacktestError::Arithmetic("applied total fee overflow".to_owned()))?;
        self.broker_total = self
            .broker_total
            .checked_add(applied.broker)
            .ok_or_else(|| BacktestError::Arithmetic("broker total overflow".to_owned()))?;
        self.market_total = self
            .market_total
            .checked_add(applied.market)
            .ok_or_else(|| BacktestError::Arithmetic("market total overflow".to_owned()))?;
        Ok(applied)
    }

    pub const fn broker_total(&self) -> Decimal {
        self.broker_total
    }

    pub const fn market_total(&self) -> Decimal {
        self.market_total
    }

    pub fn breakdown(&self) -> Vec<FeeBreakdown> {
        self.breakdown
            .iter()
            .map(|(key, value)| FeeBreakdown {
                rule_id: key
                    .split_once('|')
                    .map_or(key.as_str(), |(_, id)| id)
                    .to_owned(),
                label: value.label.clone(),
                group: value.group.clone(),
                amount: value.amount.to_storage_text(),
                count: value.count,
            })
            .collect()
    }
}

fn raw_fee(
    rule: &FeeRule,
    accumulator: &mut RuleAccumulator,
    notional: Decimal,
    quantity: Decimal,
) -> Result<Decimal, BacktestError> {
    match rule.basis.trim().to_ascii_lowercase().as_str() {
        "share" | "contract" | "quantity" => {
            let fixed_part = quantity
                .checked_mul(rule.fixed_amount)
                .ok_or_else(|| BacktestError::Arithmetic("fee share fixed overflow".to_owned()))?;
            let rate_part = quantity
                .checked_mul(rule.rate)
                .ok_or_else(|| BacktestError::Arithmetic("fee share rate overflow".to_owned()))?;
            fixed_part
                .checked_add(rate_part)
                .ok_or_else(|| BacktestError::Arithmetic("fee share total overflow".to_owned()))
        }
        "order" => {
            if accumulator.order_used {
                Ok(Decimal::ZERO)
            } else {
                accumulator.order_used = true;
                Ok(rule.fixed_amount)
            }
        }
        "notional" | "" => {
            let rate_part = notional.checked_mul(rule.rate).ok_or_else(|| {
                BacktestError::Arithmetic("fee notional rate overflow".to_owned())
            })?;
            rate_part
                .checked_add(rule.fixed_amount)
                .ok_or_else(|| BacktestError::Arithmetic("fee notional total overflow".to_owned()))
        }
        other => Err(BacktestError::InvalidInput(format!(
            "fee rule {} has unsupported basis {other}",
            rule.id
        ))),
    }
}

fn cap_amount(rule: &FeeRule, notional: Decimal) -> Result<Decimal, BacktestError> {
    let mut cap = rule.max_amount;
    if rule.max_rate > Decimal::ZERO {
        let rate_cap = notional
            .checked_mul(rule.max_rate)
            .ok_or_else(|| BacktestError::Arithmetic("fee max_rate overflow".to_owned()))?;
        if cap.is_zero() || rate_cap < cap {
            cap = rate_cap;
        }
    }
    Ok(cap)
}

fn round_fee(amount: Decimal, rounding: &str) -> Result<Decimal, BacktestError> {
    match rounding.trim().to_ascii_lowercase().as_str() {
        "ceil_currency_unit" | "ceil_hkd" => amount
            .ceil_to_increment(Decimal::ONE)
            .map_err(BacktestError::from),
        "ceil_cent" => amount
            .ceil_to_increment(Decimal::new(1, 2))
            .map_err(BacktestError::from),
        "" => Ok(amount),
        other => Err(BacktestError::InvalidInput(format!(
            "unsupported fee rounding {other}"
        ))),
    }
}

fn side_matches(rule_side: &str, trade_side: &str) -> bool {
    let normalized = rule_side.trim().to_ascii_lowercase();
    normalized.is_empty() || normalized == "both" || normalized == trade_side
}

#[cfg(test)]
mod tests {
    use jftrade_kernel::{Decimal, DecimalTradingExt};

    use super::FeeEngine;
    use crate::model::FeeRule;

    #[test]
    fn order_minimum_is_incremental_across_partial_fills() {
        let rule = FeeRule {
            id: "commission".to_owned(),
            label: "Commission".to_owned(),
            group: "broker".to_owned(),
            side: "both".to_owned(),
            basis: "notional".to_owned(),
            rate: "0.001".parse().expect("rate"),
            fixed_amount: Decimal::ZERO,
            min_amount: "1".parse().expect("minimum"),
            max_amount: Decimal::ZERO,
            max_rate: Decimal::ZERO,
            rounding: String::new(),
        };
        let mut engine = FeeEngine::new(&[rule]);
        let first = engine
            .apply(
                1,
                "buy",
                "100".parse().expect("price"),
                "2".parse().expect("qty"),
            )
            .expect("first");
        let second = engine
            .apply(
                1,
                "buy",
                "100".parse().expect("price"),
                "20".parse().expect("qty"),
            )
            .expect("second");
        assert_eq!(first.total.to_storage_text(), "1");
        assert_eq!(second.total.to_storage_text(), "1.2");
        assert_eq!(engine.broker_total().to_storage_text(), "2.2");
    }

    #[test]
    fn non_billable_trade_does_not_consume_per_order_fee() {
        let rule = FeeRule {
            id: "per-order".to_owned(),
            label: "Per order".to_owned(),
            group: "broker".to_owned(),
            side: "both".to_owned(),
            basis: "order".to_owned(),
            rate: Decimal::ZERO,
            fixed_amount: "2".parse().expect("fixed amount"),
            min_amount: Decimal::ZERO,
            max_amount: Decimal::ZERO,
            max_rate: Decimal::ZERO,
            rounding: String::new(),
        };
        let mut engine = FeeEngine::new(&[rule]);

        let zero_notional = engine
            .apply(9, "buy", Decimal::ZERO, "1".parse().expect("quantity"))
            .expect("zero-notional trade");
        let first_billable = engine
            .apply(
                9,
                "buy",
                "100".parse().expect("price"),
                "1".parse().expect("quantity"),
            )
            .expect("first billable trade");
        let second_billable = engine
            .apply(
                9,
                "buy",
                "100".parse().expect("price"),
                "1".parse().expect("quantity"),
            )
            .expect("second billable trade");

        assert_eq!(zero_notional.total, Decimal::ZERO);
        assert_eq!(first_billable.total.to_storage_text(), "2");
        assert_eq!(second_billable.total, Decimal::ZERO);
        assert_eq!(engine.broker_total().to_storage_text(), "2");
        let breakdown = engine.breakdown();
        assert_eq!(breakdown.len(), 1);
        assert_eq!(breakdown[0].amount, "2");
        assert_eq!(breakdown[0].count, 1);
    }
}
