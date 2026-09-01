use std::collections::BTreeMap;

use jftrade_kernel::{Fixed8, WireTimestamp};
use serde::{Deserialize, Serialize};

use crate::TradingError;

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct PositionProjection {
    pub account_id: String,
    pub market: String,
    pub symbol: String,
    pub quantity: Fixed8,
    pub sellable_quantity: Fixed8,
    pub last_price: Fixed8,
}

impl PositionProjection {
    /// Matches the strategy runtime's market-qualified and bare-symbol forms.
    /// A position with no market can match the suffix of `US.AAPL`/`US:AAPL`,
    /// while a market-qualified position can match either separator.
    pub fn matches_symbol(&self, symbol: &str) -> bool {
        position_matches_symbol(self, symbol)
    }
}

/// Returns whether a position belongs to a strategy symbol after trimming and
/// case-folding the market-qualified forms used by the Go runtime.
pub fn position_matches_symbol(position: &PositionProjection, symbol: &str) -> bool {
    let strategy_symbol = symbol.trim().to_ascii_uppercase();
    if strategy_symbol.is_empty() {
        return false;
    }
    let position_symbol = position.symbol.trim().to_ascii_uppercase();
    if position_symbol.is_empty() {
        return false;
    }
    if position_symbol == strategy_symbol {
        return true;
    }
    let market = position.market.trim().to_ascii_uppercase();
    if !market.is_empty()
        && !position_symbol.contains('.')
        && !position_symbol.contains(':')
        && (strategy_symbol == format!("{market}.{position_symbol}")
            || strategy_symbol == format!("{market}:{position_symbol}"))
    {
        return true;
    }
    if market.is_empty() {
        if let Some((_, suffix)) = strategy_symbol.split_once('.')
            && suffix == position_symbol
        {
            return true;
        }
        if let Some((_, suffix)) = strategy_symbol.split_once(':')
            && suffix == position_symbol
        {
            return true;
        }
    }
    false
}

/// Sums sellable quantities for positions matching a strategy symbol.
pub fn sellable_quantity(
    positions: &[PositionProjection],
    symbol: &str,
) -> Result<Fixed8, TradingError> {
    positions.iter().try_fold(Fixed8::ZERO, |total, position| {
        if position.matches_symbol(symbol) {
            total
                .checked_add(position.sellable_quantity)
                .map_err(|_| TradingError::Arithmetic)
        } else {
            Ok(total)
        }
    })
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct AccountRefresh {
    pub refresh_id: String,
    pub trace_id: String,
    pub account_id: String,
    pub generation: u64,
    pub sequence: u64,
    pub observed_at: WireTimestamp,
    pub positions: Vec<PositionProjection>,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum PortfolioOutcome {
    Applied,
    Duplicate,
    Stale,
}

#[derive(Clone, Debug, Default, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct AccountPortfolio {
    account_id: Option<String>,
    generation: u64,
    sequence: u64,
    positions: BTreeMap<String, PositionProjection>,
    refreshes: BTreeMap<String, String>,
}

impl AccountPortfolio {
    pub fn apply_refresh(
        &mut self,
        refresh: &AccountRefresh,
    ) -> Result<PortfolioOutcome, TradingError> {
        if refresh.refresh_id.trim().is_empty() {
            return Err(TradingError::InvalidPortfolio("refreshId is required"));
        }
        if refresh.trace_id.trim().is_empty() {
            return Err(TradingError::InvalidPortfolio("traceId is required"));
        }
        if refresh.account_id.trim().is_empty() {
            return Err(TradingError::InvalidPortfolio("accountId is required"));
        }
        let fingerprint = refresh_fingerprint(refresh);
        if let Some(existing) = self.refreshes.get(&refresh.refresh_id) {
            return if existing == &fingerprint {
                Ok(PortfolioOutcome::Duplicate)
            } else {
                Err(TradingError::InvalidPortfolio(
                    "refreshId was reused with different content",
                ))
            };
        }
        if refresh.generation < self.generation
            || refresh.generation == self.generation && refresh.sequence <= self.sequence
        {
            return Ok(PortfolioOutcome::Stale);
        }

        let mut positions = BTreeMap::new();
        for position in &refresh.positions {
            validate_position(refresh, position)?;
            let key = format!(
                "{}.{}",
                position.market.trim().to_ascii_uppercase(),
                position.symbol.trim().to_ascii_uppercase()
            );
            if positions.insert(key, position.clone()).is_some() {
                return Err(TradingError::InvalidPortfolio(
                    "refresh contains a duplicate position",
                ));
            }
        }

        self.account_id = Some(refresh.account_id.clone());
        self.generation = refresh.generation;
        self.sequence = refresh.sequence;
        self.positions = positions;
        self.refreshes
            .insert(refresh.refresh_id.clone(), fingerprint);
        Ok(PortfolioOutcome::Applied)
    }

    pub fn positions(&self) -> Vec<&PositionProjection> {
        self.positions.values().collect()
    }

    pub fn sellable_quantity(&self, symbol: &str) -> Result<Fixed8, TradingError> {
        self.positions()
            .into_iter()
            .try_fold(Fixed8::ZERO, |total, position| {
                if position.matches_symbol(symbol) {
                    total
                        .checked_add(position.sellable_quantity)
                        .map_err(|_| TradingError::Arithmetic)
                } else {
                    Ok(total)
                }
            })
    }

    pub const fn generation(&self) -> u64 {
        self.generation
    }

    pub const fn sequence(&self) -> u64 {
        self.sequence
    }
}

fn refresh_fingerprint(refresh: &AccountRefresh) -> String {
    let mut fields = vec![
        refresh.account_id.trim().to_owned(),
        refresh.generation.to_string(),
        refresh.sequence.to_string(),
    ];
    fields.extend(refresh.positions.iter().map(|position| {
        format!(
            "{}|{}|{}|{}|{}|{}",
            position.account_id.trim(),
            position.market.trim().to_ascii_uppercase(),
            position.symbol.trim().to_ascii_uppercase(),
            position.quantity,
            position.sellable_quantity,
            position.last_price
        )
    }));
    fields.join(";")
}

fn validate_position(
    refresh: &AccountRefresh,
    position: &PositionProjection,
) -> Result<(), TradingError> {
    if position.account_id != refresh.account_id {
        return Err(TradingError::InvalidPortfolio(
            "position account does not match refresh account",
        ));
    }
    if position.market.trim().is_empty() || position.symbol.trim().is_empty() {
        return Err(TradingError::InvalidPortfolio(
            "position market and symbol are required",
        ));
    }
    if position.sellable_quantity.signum() < 0 || position.last_price.signum() < 0 {
        return Err(TradingError::InvalidPortfolio(
            "sellable quantity and last price cannot be negative",
        ));
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use std::str::FromStr;

    use jftrade_kernel::Fixed8;

    use super::{
        AccountPortfolio, AccountRefresh, PortfolioOutcome, PositionProjection,
        position_matches_symbol, sellable_quantity,
    };

    fn refresh(id: &str, generation: u64, sequence: u64) -> AccountRefresh {
        AccountRefresh {
            refresh_id: id.to_owned(),
            trace_id: format!("trace-{id}"),
            account_id: "acc-1".to_owned(),
            generation,
            sequence,
            observed_at: "2026-08-19T00:00:00Z".parse().expect("timestamp"),
            positions: vec![PositionProjection {
                account_id: "acc-1".to_owned(),
                market: "US".to_owned(),
                symbol: "AAPL".to_owned(),
                quantity: Fixed8::from_str("10").expect("quantity"),
                sellable_quantity: Fixed8::from_str("8").expect("sellable"),
                last_price: Fixed8::from_str("100").expect("price"),
            }],
        }
    }

    #[test]
    fn refreshes_are_atomic_deduplicated_and_generation_guarded() {
        let mut portfolio = AccountPortfolio::default();
        let first = refresh("refresh-1", 1, 1);
        assert_eq!(
            portfolio.apply_refresh(&first),
            Ok(PortfolioOutcome::Applied)
        );
        assert_eq!(
            portfolio.apply_refresh(&first),
            Ok(PortfolioOutcome::Duplicate)
        );
        let mut conflicting = first.clone();
        conflicting.positions.clear();
        assert!(portfolio.apply_refresh(&conflicting).is_err());
        assert_eq!(
            portfolio.apply_refresh(&refresh("stale", 0, 9)),
            Ok(PortfolioOutcome::Stale)
        );

        let mut invalid = refresh("invalid", 2, 1);
        invalid.positions.push(PositionProjection {
            account_id: "other-account".to_owned(),
            ..invalid.positions[0].clone()
        });
        assert!(portfolio.apply_refresh(&invalid).is_err());
        assert_eq!(portfolio.generation(), 1);
        assert_eq!(portfolio.sequence(), 1);
        assert_eq!(portfolio.positions().len(), 1);
    }

    #[test]
    fn positions_match_qualified_symbols_and_sum_sellable_quantity() {
        let positions = vec![
            PositionProjection {
                account_id: "acc-1".to_owned(),
                market: "US".to_owned(),
                symbol: "AAPL".to_owned(),
                quantity: Fixed8::from_str("5").expect("quantity"),
                sellable_quantity: Fixed8::from_str("3").expect("sellable"),
                last_price: Fixed8::from_str("100").expect("price"),
            },
            PositionProjection {
                account_id: "acc-1".to_owned(),
                market: "HK".to_owned(),
                symbol: "00700".to_owned(),
                quantity: Fixed8::from_str("10").expect("quantity"),
                sellable_quantity: Fixed8::from_str("10").expect("sellable"),
                last_price: Fixed8::from_str("300").expect("price"),
            },
        ];
        assert!(positions[0].matches_symbol("us.aapl"));
        assert!(position_matches_symbol(
            &PositionProjection {
                market: String::new(),
                symbol: "AAPL".to_owned(),
                ..positions[0].clone()
            },
            "US.AAPL"
        ));
        assert!(positions[0].matches_symbol("US:AAPL"));
        assert!(!positions[1].matches_symbol("US.AAPL"));
        assert_eq!(
            sellable_quantity(&positions, "US.AAPL").expect("sellable quantity"),
            Fixed8::from_str("3").expect("expected quantity")
        );
        assert_eq!(
            sellable_quantity(&positions, " ").expect("empty quantity"),
            Fixed8::ZERO
        );
    }
}
