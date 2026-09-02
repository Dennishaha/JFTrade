use std::collections::BTreeMap;

use jftrade_kernel::Fixed8;

use crate::BacktestError;
use crate::model::{FeeBreakdown, FeeRule};

#[derive(Clone, Copy, Debug, Default)]
pub(crate) struct AppliedFees {
    pub broker: Fixed8,
    pub market: Fixed8,
    pub total: Fixed8,
}

#[derive(Clone, Copy, Debug, Default)]
struct RuleAccumulator {
    raw: Fixed8,
    notional: Fixed8,
    charged: Fixed8,
    order_used: bool,
}

#[derive(Clone, Debug)]
struct BreakdownAccumulator {
    label: String,
    group: String,
    amount: Fixed8,
    count: usize,
}

pub(crate) struct FeeEngine {
    rules: Vec<FeeRule>,
    orders: BTreeMap<String, RuleAccumulator>,
    breakdown: BTreeMap<String, BreakdownAccumulator>,
    broker_total: Fixed8,
    market_total: Fixed8,
}

impl FeeEngine {
    pub fn new(rules: &[FeeRule]) -> Self {
        Self {
            rules: rules.to_vec(),
            orders: BTreeMap::new(),
            breakdown: BTreeMap::new(),
            broker_total: Fixed8::ZERO,
            market_total: Fixed8::ZERO,
        }
    }

    pub fn apply(
        &mut self,
        order_id: u64,
        side: &str,
        price: Fixed8,
        quantity: Fixed8,
    ) -> Result<AppliedFees, BacktestError> {
        let notional = price.checked_mul(quantity)?;
        // A trade without a positive notional or quantity is not billable.
        // Keep this guard at the fee boundary so an order-basis fixed charge
        // cannot turn a zero-value trade into a fee-bearing event.
        if notional <= Fixed8::ZERO || quantity <= Fixed8::ZERO {
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
            if raw <= Fixed8::ZERO {
                continue;
            }
            accumulator.raw = accumulator.raw.checked_add(raw)?;
            accumulator.notional = accumulator.notional.checked_add(notional)?;
            let mut target = accumulator.raw;
            if rule.min_amount > Fixed8::ZERO && target < rule.min_amount {
                target = rule.min_amount;
            }
            let cap = cap_amount(&rule, accumulator.notional)?;
            if cap > Fixed8::ZERO && target > cap {
                target = cap;
            }
            target = round_fee(target, &rule.rounding)?;
            let incremental = target.checked_sub(accumulator.charged)?;
            if incremental <= Fixed8::ZERO {
                continue;
            }
            accumulator.charged = target;
            match rule.group.trim().to_ascii_lowercase().as_str() {
                "broker" => applied.broker = applied.broker.checked_add(incremental)?,
                "market" => applied.market = applied.market.checked_add(incremental)?,
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
                        amount: Fixed8::ZERO,
                        count: 0,
                    });
            entry.amount = entry.amount.checked_add(incremental)?;
            entry.count += 1;
        }
        applied.total = applied.broker.checked_add(applied.market)?;
        self.broker_total = self.broker_total.checked_add(applied.broker)?;
        self.market_total = self.market_total.checked_add(applied.market)?;
        Ok(applied)
    }

    pub const fn broker_total(&self) -> Fixed8 {
        self.broker_total
    }

    pub const fn market_total(&self) -> Fixed8 {
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
                amount: value.amount.storage_text(),
                count: value.count,
            })
            .collect()
    }
}

fn raw_fee(
    rule: &FeeRule,
    accumulator: &mut RuleAccumulator,
    notional: Fixed8,
    quantity: Fixed8,
) -> Result<Fixed8, BacktestError> {
    match rule.basis.trim().to_ascii_lowercase().as_str() {
        "share" | "contract" | "quantity" => quantity
            .checked_mul(rule.fixed_amount)?
            .checked_add(quantity.checked_mul(rule.rate)?)
            .map_err(BacktestError::from),
        "order" => {
            if accumulator.order_used {
                Ok(Fixed8::ZERO)
            } else {
                accumulator.order_used = true;
                Ok(rule.fixed_amount)
            }
        }
        "notional" | "" => notional
            .checked_mul(rule.rate)?
            .checked_add(rule.fixed_amount)
            .map_err(BacktestError::from),
        other => Err(BacktestError::InvalidInput(format!(
            "fee rule {} has unsupported basis {other}",
            rule.id
        ))),
    }
}

fn cap_amount(rule: &FeeRule, notional: Fixed8) -> Result<Fixed8, BacktestError> {
    let mut cap = rule.max_amount;
    if rule.max_rate > Fixed8::ZERO {
        let rate_cap = notional.checked_mul(rule.max_rate)?;
        if cap.is_zero() || rate_cap < cap {
            cap = rate_cap;
        }
    }
    Ok(cap)
}

fn round_fee(amount: Fixed8, rounding: &str) -> Result<Fixed8, BacktestError> {
    match rounding.trim().to_ascii_lowercase().as_str() {
        "ceil_currency_unit" | "ceil_hkd" => amount
            .ceil_to_increment("1".parse()?)
            .map_err(BacktestError::from),
        "ceil_cent" => amount
            .ceil_to_increment("0.01".parse()?)
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
    use jftrade_kernel::Fixed8;

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
            fixed_amount: Fixed8::ZERO,
            min_amount: "1".parse().expect("minimum"),
            max_amount: Fixed8::ZERO,
            max_rate: Fixed8::ZERO,
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
        assert_eq!(first.total.storage_text(), "1");
        assert_eq!(second.total.storage_text(), "1.2");
        assert_eq!(engine.broker_total().storage_text(), "2.2");
    }

    #[test]
    fn non_billable_trade_does_not_consume_per_order_fee() {
        let rule = FeeRule {
            id: "per-order".to_owned(),
            label: "Per order".to_owned(),
            group: "broker".to_owned(),
            side: "both".to_owned(),
            basis: "order".to_owned(),
            rate: Fixed8::ZERO,
            fixed_amount: "2".parse().expect("fixed amount"),
            min_amount: Fixed8::ZERO,
            max_amount: Fixed8::ZERO,
            max_rate: Fixed8::ZERO,
            rounding: String::new(),
        };
        let mut engine = FeeEngine::new(&[rule]);

        let zero_notional = engine
            .apply(9, "buy", Fixed8::ZERO, "1".parse().expect("quantity"))
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

        assert_eq!(zero_notional.total, Fixed8::ZERO);
        assert_eq!(first_billable.total.storage_text(), "2");
        assert_eq!(second_billable.total, Fixed8::ZERO);
        assert_eq!(engine.broker_total().storage_text(), "2");
        let breakdown = engine.breakdown();
        assert_eq!(breakdown.len(), 1);
        assert_eq!(breakdown[0].amount, "2");
        assert_eq!(breakdown[0].count, 1);
    }
}
