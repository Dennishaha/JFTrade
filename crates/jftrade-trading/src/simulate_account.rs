use serde::{Deserialize, Serialize};
use std::collections::BTreeMap;
use thiserror::Error;

/// Error returned by virtual account operations in SIMULATE mode.
#[derive(Clone, Debug, Error, PartialEq)]
pub enum SimulateExecutionError {
    #[error("insufficient virtual cash: available {available}, required {required}")]
    InsufficientCash { available: f64, required: f64 },

    #[error("insufficient virtual position: sellable {sellable}, required {required}")]
    InsufficientPosition { sellable: f64, required: f64 },

    #[error("invalid quantity {0}: must be positive and finite")]
    InvalidQuantity(f64),

    #[error("invalid price {0}: must be positive and finite")]
    InvalidPrice(f64),

    #[error("invalid fee {0}: must be non-negative and finite")]
    InvalidFee(f64),

    #[error("symbol cannot be empty for virtual position")]
    EmptySymbol,

    #[error("unsupported order side: {0}")]
    UnsupportedSide(String),
}

/// A simulated position for a single instrument in a virtual account.
#[derive(Clone, Debug, Deserialize, Serialize, PartialEq)]
#[serde(rename_all = "camelCase")]
pub struct VirtualPosition {
    pub symbol: String,
    pub market: String,
    pub quantity: f64,
    pub sellable_quantity: f64,
    pub average_cost: f64,
    pub realized_pnl: f64,
}

/// In-memory state of a virtual trading account in SIMULATE mode.
#[derive(Clone, Debug, Deserialize, Serialize, PartialEq)]
#[serde(rename_all = "camelCase")]
pub struct VirtualAccountState {
    pub account_id: String,
    pub initial_cash: f64,
    pub available_cash: f64,
    pub frozen_cash: f64,
    pub positions: BTreeMap<String, VirtualPosition>,
    pub updated_at_ms: i64,
}

impl VirtualAccountState {
    /// Constructs a new virtual account with given initial cash.
    pub fn new(account_id: impl Into<String>, initial_cash: f64, now_ms: i64) -> Self {
        let cash = if initial_cash.is_finite() && initial_cash >= 0.0 {
            initial_cash
        } else {
            0.0
        };
        Self {
            account_id: account_id.into(),
            initial_cash: cash,
            available_cash: cash,
            frozen_cash: 0.0,
            positions: BTreeMap::new(),
            updated_at_ms: now_ms,
        }
    }

    /// Looks up a position by symbol (case-insensitive).
    pub fn get_position(&self, symbol: &str) -> Option<&VirtualPosition> {
        let key = symbol.trim().to_ascii_uppercase();
        self.positions.get(&key)
    }

    /// Looks up a mutable position by symbol (case-insensitive).
    pub fn get_position_mut(&mut self, symbol: &str) -> Option<&mut VirtualPosition> {
        let key = symbol.trim().to_ascii_uppercase();
        self.positions.get_mut(&key)
    }

    /// Calculates total account equity given a map of symbol -> current market prices.
    pub fn total_equity(&self, market_prices: &BTreeMap<String, f64>) -> f64 {
        let position_market_value: f64 = self
            .positions
            .iter()
            .map(|(key, pos)| {
                let price = market_prices
                    .get(key)
                    .copied()
                    .or_else(|| market_prices.get(&pos.symbol).copied())
                    .unwrap_or(pos.average_cost);
                pos.quantity * price
            })
            .sum();
        self.available_cash + self.frozen_cash + position_market_value
    }

    /// Applies a trade fill (BUY or SELL) to the virtual account.
    ///
    /// For BUY:
    /// - Checks cash sufficiency (`available_cash >= notional + fee`).
    /// - Deducts cash (`notional + fee`).
    /// - Updates position quantity, sellable quantity, and weighted average cost.
    /// - Returns `Ok(0.0)`.
    ///
    /// For SELL:
    /// - Checks position sufficiency (`sellable_quantity >= quantity`).
    /// - Credits cash (`notional - fee`).
    /// - Computes realized PnL: `(price - average_cost) * quantity - fee`.
    /// - Decrements position quantity and sellable quantity.
    /// - Removes the position if closed (`quantity <= 0.0`).
    /// - Returns `Ok(realized_pnl)`.
    #[allow(clippy::too_many_arguments)]
    pub fn apply_fill(
        &mut self,
        market: &str,
        symbol: &str,
        side: &str,
        quantity: f64,
        price: f64,
        fee: f64,
        now_ms: i64,
    ) -> Result<f64, SimulateExecutionError> {
        let trimmed_symbol = symbol.trim();
        if trimmed_symbol.is_empty() {
            return Err(SimulateExecutionError::EmptySymbol);
        }
        if !quantity.is_finite() || quantity <= 0.0 {
            return Err(SimulateExecutionError::InvalidQuantity(quantity));
        }
        if !price.is_finite() || price <= 0.0 {
            return Err(SimulateExecutionError::InvalidPrice(price));
        }
        if !fee.is_finite() || fee < 0.0 {
            return Err(SimulateExecutionError::InvalidFee(fee));
        }

        let key = trimmed_symbol.to_ascii_uppercase();
        let notional = quantity * price;

        let normalized_side = side.trim().to_ascii_uppercase();
        match normalized_side.as_str() {
            "BUY" => {
                let total_cost = notional + fee;
                if self.available_cash < total_cost {
                    return Err(SimulateExecutionError::InsufficientCash {
                        available: self.available_cash,
                        required: total_cost,
                    });
                }
                self.available_cash -= total_cost;

                let pos = self
                    .positions
                    .entry(key)
                    .or_insert_with(|| VirtualPosition {
                        symbol: trimmed_symbol.to_owned(),
                        market: market.trim().to_ascii_uppercase(),
                        quantity: 0.0,
                        sellable_quantity: 0.0,
                        average_cost: 0.0,
                        realized_pnl: 0.0,
                    });

                let prev_notional = pos.quantity * pos.average_cost;
                pos.quantity += quantity;
                pos.sellable_quantity += quantity;
                if pos.quantity > 0.0 {
                    pos.average_cost = (prev_notional + notional) / pos.quantity;
                }
                self.updated_at_ms = now_ms;
                Ok(0.0)
            }
            "SELL" => {
                let pos = self.positions.get_mut(&key).ok_or(
                    SimulateExecutionError::InsufficientPosition {
                        sellable: 0.0,
                        required: quantity,
                    },
                )?;

                if pos.sellable_quantity < quantity {
                    return Err(SimulateExecutionError::InsufficientPosition {
                        sellable: pos.sellable_quantity,
                        required: quantity,
                    });
                }

                let pnl = (price - pos.average_cost) * quantity - fee;
                pos.quantity -= quantity;
                pos.sellable_quantity -= quantity;
                pos.realized_pnl += pnl;

                let net_credit = notional - fee;
                self.available_cash += net_credit;

                if pos.quantity <= 1e-9 {
                    self.positions.remove(&key);
                }
                self.updated_at_ms = now_ms;
                Ok(pnl)
            }
            _ => Err(SimulateExecutionError::UnsupportedSide(side.to_owned())),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_virtual_account_initialization() {
        let account = VirtualAccountState::new("sim-1", 100_000.0, 1000);
        assert_eq!(account.account_id, "sim-1");
        assert_eq!(account.initial_cash, 100_000.0);
        assert_eq!(account.available_cash, 100_000.0);
        assert_eq!(account.frozen_cash, 0.0);
        assert!(account.positions.is_empty());
        assert_eq!(account.updated_at_ms, 1000);
    }

    #[test]
    fn test_buy_fill_deducts_cash_and_creates_position() {
        let mut account = VirtualAccountState::new("sim-1", 100_000.0, 1000);
        let pnl = account
            .apply_fill("US", "AAPL", "BUY", 100.0, 150.0, 5.0, 2000)
            .expect("buy fill succeeds");
        assert_eq!(pnl, 0.0);
        assert_eq!(account.available_cash, 100_000.0 - 15_000.0 - 5.0);

        let pos = account.get_position("AAPL").expect("AAPL position exists");
        assert_eq!(pos.symbol, "AAPL");
        assert_eq!(pos.market, "US");
        assert_eq!(pos.quantity, 100.0);
        assert_eq!(pos.sellable_quantity, 100.0);
        assert_eq!(pos.average_cost, 150.0);
        assert_eq!(pos.realized_pnl, 0.0);
        assert_eq!(account.updated_at_ms, 2000);
    }

    #[test]
    fn test_multiple_buys_computes_weighted_average_cost() {
        let mut account = VirtualAccountState::new("sim-1", 100_000.0, 1000);
        account
            .apply_fill("US", "AAPL", "BUY", 100.0, 100.0, 0.0, 2000)
            .unwrap();
        account
            .apply_fill("US", "AAPL", "BUY", 100.0, 200.0, 0.0, 3000)
            .unwrap();

        let pos = account.get_position("AAPL").unwrap();
        assert_eq!(pos.quantity, 200.0);
        assert_eq!(pos.sellable_quantity, 200.0);
        assert_eq!(pos.average_cost, 150.0); // (100*100 + 100*200) / 200 = 150.0
    }

    #[test]
    fn test_buy_insufficient_cash_rejected() {
        let mut account = VirtualAccountState::new("sim-1", 500.0, 1000);
        let err = account
            .apply_fill("US", "AAPL", "BUY", 10.0, 100.0, 0.0, 2000)
            .expect_err("should fail due to insufficient cash");
        assert_eq!(
            err,
            SimulateExecutionError::InsufficientCash {
                available: 500.0,
                required: 1000.0,
            }
        );
        assert_eq!(account.available_cash, 500.0);
        assert!(account.positions.is_empty());
    }

    #[test]
    fn test_sell_fill_credits_cash_and_computes_pnl() {
        let mut account = VirtualAccountState::new("sim-1", 100_000.0, 1000);
        account
            .apply_fill("US", "AAPL", "BUY", 100.0, 150.0, 0.0, 2000)
            .unwrap();

        // Sell partial: 40 shares at 180.0, fee 2.0
        let pnl = account
            .apply_fill("US", "AAPL", "SELL", 40.0, 180.0, 2.0, 3000)
            .expect("sell fill succeeds");
        // PnL = (180 - 150) * 40 - 2 = 1200 - 2 = 1198.0
        assert_eq!(pnl, 1198.0);
        // Cash = 85_000 + 40 * 180 - 2 = 85_000 + 7200 - 2 = 92_198.0
        assert_eq!(account.available_cash, 92_198.0);

        let pos = account.get_position("AAPL").unwrap();
        assert_eq!(pos.quantity, 60.0);
        assert_eq!(pos.sellable_quantity, 60.0);
        assert_eq!(pos.average_cost, 150.0);
        assert_eq!(pos.realized_pnl, 1198.0);
    }

    #[test]
    fn test_sell_full_close_removes_position() {
        let mut account = VirtualAccountState::new("sim-1", 100_000.0, 1000);
        account
            .apply_fill("US", "AAPL", "BUY", 100.0, 150.0, 0.0, 2000)
            .unwrap();

        let pnl = account
            .apply_fill("US", "AAPL", "SELL", 100.0, 160.0, 0.0, 3000)
            .unwrap();
        assert_eq!(pnl, 1000.0);
        assert_eq!(account.available_cash, 101_000.0);
        assert!(account.get_position("AAPL").is_none());
    }

    #[test]
    fn test_sell_insufficient_position_rejected() {
        let mut account = VirtualAccountState::new("sim-1", 100_000.0, 1000);
        account
            .apply_fill("US", "AAPL", "BUY", 50.0, 150.0, 0.0, 2000)
            .unwrap();

        let err = account
            .apply_fill("US", "AAPL", "SELL", 100.0, 160.0, 0.0, 3000)
            .expect_err("should fail due to insufficient position");
        assert_eq!(
            err,
            SimulateExecutionError::InsufficientPosition {
                sellable: 50.0,
                required: 100.0,
            }
        );
    }

    #[test]
    fn test_sell_without_position_rejected() {
        let mut account = VirtualAccountState::new("sim-1", 100_000.0, 1000);
        let err = account
            .apply_fill("US", "TSLA", "SELL", 10.0, 200.0, 0.0, 2000)
            .expect_err("should fail when no position");
        assert_eq!(
            err,
            SimulateExecutionError::InsufficientPosition {
                sellable: 0.0,
                required: 10.0,
            }
        );
    }

    #[test]
    fn test_serialization_and_deserialization_checkpoint() {
        let mut account = VirtualAccountState::new("sim-check", 50_000.0, 5000);
        account
            .apply_fill("US", "NVDA", "BUY", 20.0, 120.0, 1.5, 6000)
            .unwrap();

        let json_str = serde_json::to_string(&account).expect("serialize");
        let restored: VirtualAccountState = serde_json::from_str(&json_str).expect("deserialize");
        assert_eq!(account, restored);
        assert_eq!(restored.get_position("NVDA").unwrap().average_cost, 120.0);
    }

    #[test]
    fn test_invalid_inputs_rejected() {
        let mut account = VirtualAccountState::new("sim-1", 100_000.0, 1000);
        assert!(matches!(
            account.apply_fill("US", "", "BUY", 10.0, 100.0, 0.0, 2000),
            Err(SimulateExecutionError::EmptySymbol)
        ));
        assert!(matches!(
            account.apply_fill("US", "AAPL", "BUY", -5.0, 100.0, 0.0, 2000),
            Err(SimulateExecutionError::InvalidQuantity(_))
        ));
        assert!(matches!(
            account.apply_fill("US", "AAPL", "BUY", 10.0, -100.0, 0.0, 2000),
            Err(SimulateExecutionError::InvalidPrice(_))
        ));
        assert!(matches!(
            account.apply_fill("US", "AAPL", "BUY", 10.0, 100.0, -1.0, 2000),
            Err(SimulateExecutionError::InvalidFee(_))
        ));
        assert!(matches!(
            account.apply_fill("US", "AAPL", "HOLD", 10.0, 100.0, 0.0, 2000),
            Err(SimulateExecutionError::UnsupportedSide(_))
        ));
    }
}
