#![forbid(unsafe_code)]

//! Empirical stress test harness for `VirtualAccountState` in `crates/jftrade-trading/src/simulate_account.rs`.
//!
//! Verifies:
//! 1. Conservation of funds across 150+ sequential random fills.
//! 2. Multi-symbol concurrent portfolio tracking across markets.
//! 3. Precision and edge cases: tiny fractional quantities, huge sizes, zero-cash boundary, fee exhaustion.
//! 4. Deterministic rejection of invalid operations and malformed inputs.
//! 5. Checkpoint serialization round-trip fidelity under active trading state.
//! 6. Reopening positions after full close, dust limit precision, and high-frequency multi-symbol stress.

use jftrade_trading::{SimulateExecutionError, VirtualAccountState};
use std::collections::BTreeMap;

/// Minimal deterministic PRNG (Xorshift64) for reproducible empirical testing.
struct TestPrng(u64);

impl TestPrng {
    fn new(seed: u64) -> Self {
        Self(if seed == 0 { 0xdeadbeef } else { seed })
    }

    fn next_u64(&mut self) -> u64 {
        self.0 ^= self.0 << 13;
        self.0 ^= self.0 >> 7;
        self.0 ^= self.0 << 17;
        self.0
    }

    fn next_f64(&mut self) -> f64 {
        (self.next_u64() % 1_000_000) as f64 / 1_000_000.0
    }

    fn next_range(&mut self, min: f64, max: f64) -> f64 {
        min + self.next_f64() * (max - min)
    }

    fn next_bool(&mut self) -> bool {
        (self.next_u64() & 1) == 1
    }
}

#[test]
fn test_sequential_trades_conserve_funds_zero_fees() {
    let mut rng = TestPrng::new(42);
    let initial_cash = 100_000.0;
    let mut account = VirtualAccountState::new("sim-stress-1", initial_cash, 1000);

    let symbols = ["US.AAPL", "US.MSFT", "US.GOOG", "US.NVDA", "US.AMZN"];
    let mut cumulative_realized_pnl = 0.0;
    let mut current_prices: BTreeMap<String, f64> = BTreeMap::new();
    for s in &symbols {
        current_prices.insert(s.to_string(), 100.0);
    }

    let mut successful_trades = 0;
    for step in 1..=200 {
        let sym_idx = (rng.next_u64() as usize) % symbols.len();
        let sym = symbols[sym_idx];
        let price_delta = rng.next_range(-5.0, 5.0);
        let prev_price = *current_prices.get(sym).unwrap();
        let current_price = (prev_price + price_delta).max(10.0);
        current_prices.insert(sym.to_string(), current_price);

        let is_buy = rng.next_bool();
        if is_buy {
            let max_affordable_qty = (account.available_cash / current_price).floor();
            if max_affordable_qty >= 1.0 {
                let qty = rng
                    .next_range(1.0, max_affordable_qty.min(20.0))
                    .floor()
                    .max(1.0);
                account
                    .apply_fill("US", sym, "BUY", qty, current_price, 0.0, 1000 + step)
                    .expect("valid buy must succeed");
                successful_trades += 1;
            }
        } else if let Some(pos) = account
            .get_position(sym)
            .filter(|p| p.sellable_quantity >= 1.0)
        {
            let qty = rng.next_range(1.0, pos.sellable_quantity).floor().max(1.0);
            let pnl = account
                .apply_fill("US", sym, "SELL", qty, current_price, 0.0, 1000 + step)
                .expect("valid sell must succeed");
            cumulative_realized_pnl += pnl;
            successful_trades += 1;
        }

        let total_equity = account.total_equity(&current_prices);
        let unrealized_pnl: f64 = account
            .positions
            .values()
            .map(|p| {
                let mkt_price = current_prices
                    .get(&p.symbol)
                    .copied()
                    .unwrap_or(p.average_cost);
                (mkt_price - p.average_cost) * p.quantity
            })
            .sum();

        let expected_equity = initial_cash + cumulative_realized_pnl + unrealized_pnl;
        let delta = (total_equity - expected_equity).abs();
        assert!(
            delta < 1e-6,
            "Funds conservation violated at step {step}: total_equity={total_equity}, expected={expected_equity}, delta={delta}"
        );
    }

    assert!(successful_trades >= 100, "Must execute 100+ random fills");
}

#[test]
fn test_sequential_trades_conservation_with_fees() {
    let mut rng = TestPrng::new(999);
    let initial_cash = 100_000.0;
    let mut account = VirtualAccountState::new("sim-stress-fees", initial_cash, 1000);

    let symbols = ["US.TSLA", "US.META", "US.AMD"];
    let mut total_fees_paid = 0.0;
    let mut total_gross_realized_pnl = 0.0;
    let mut current_prices: BTreeMap<String, f64> = BTreeMap::new();
    for s in &symbols {
        current_prices.insert(s.to_string(), 150.0);
    }

    let mut fills_count = 0;
    for step in 1..=150 {
        let sym_idx = (rng.next_u64() as usize) % symbols.len();
        let sym = symbols[sym_idx];
        let price = rng.next_range(50.0, 250.0);
        current_prices.insert(sym.to_string(), price);
        let fee = rng.next_range(1.0, 5.0);

        let is_buy = rng.next_bool();
        if is_buy {
            let max_qty = ((account.available_cash - fee) / price).floor();
            if max_qty >= 1.0 {
                let qty = rng.next_range(1.0, max_qty.min(10.0)).floor().max(1.0);
                account
                    .apply_fill("US", sym, "BUY", qty, price, fee, 1000 + step)
                    .expect("buy fill succeeds");
                total_fees_paid += fee;
                fills_count += 1;
            }
        } else if let Some(pos) = account
            .get_position(sym)
            .filter(|p| p.sellable_quantity >= 1.0)
        {
            let qty = rng.next_range(1.0, pos.sellable_quantity).floor().max(1.0);
            let avg_cost = pos.average_cost;
            account
                .apply_fill("US", sym, "SELL", qty, price, fee, 1000 + step)
                .expect("sell fill succeeds");
            total_gross_realized_pnl += (price - avg_cost) * qty;
            total_fees_paid += fee;
            fills_count += 1;
        }

        let total_equity = account.total_equity(&current_prices);
        let unrealized_gross_pnl: f64 = account
            .positions
            .values()
            .map(|p| {
                let mkt_price = current_prices
                    .get(&p.symbol)
                    .copied()
                    .unwrap_or(p.average_cost);
                (mkt_price - p.average_cost) * p.quantity
            })
            .sum();

        let total_gross_pnl = total_gross_realized_pnl + unrealized_gross_pnl;
        let left_side = total_equity + total_fees_paid;
        let right_side = initial_cash + total_gross_pnl;
        let delta = (left_side - right_side).abs();
        assert!(
            delta < 1e-6,
            "Conservation of funds with fees violated at step {step}: left={left_side}, right={right_side}, delta={delta}"
        );
    }

    assert!(fills_count >= 100, "Must execute 100+ fills with fees");
}

#[test]
fn test_multi_symbol_portfolio_tracking_and_cross_market_isolation() {
    let mut account = VirtualAccountState::new("sim-multi", 500_000.0, 1000);

    account
        .apply_fill("US", "US.AAPL", "BUY", 100.0, 150.0, 1.0, 1001)
        .unwrap();
    account
        .apply_fill("HK", "HK.00700", "BUY", 500.0, 380.0, 15.0, 1002)
        .unwrap();
    account
        .apply_fill("CN", "CN.600519", "BUY", 100.0, 1600.0, 5.0, 1003)
        .unwrap();
    account
        .apply_fill("US", "US.NVDA", "BUY", 200.0, 120.0, 2.0, 1004)
        .unwrap();

    assert_eq!(account.positions.len(), 4);

    assert!(account.get_position("us.aapl").is_some());
    assert!(account.get_position("  HK.00700  ").is_some());
    assert_eq!(account.get_position("cn.600519").unwrap().quantity, 100.0);

    let pnl_aapl = account
        .apply_fill("US", "us.aapl", "SELL", 40.0, 180.0, 1.0, 1005)
        .unwrap();
    assert_eq!(pnl_aapl, (180.0 - 150.0) * 40.0 - 1.0);
    assert_eq!(account.get_position("US.AAPL").unwrap().quantity, 60.0);

    let pnl_nvda = account
        .apply_fill("US", "US.NVDA", "SELL", 200.0, 140.0, 2.0, 1006)
        .unwrap();
    assert_eq!(pnl_nvda, (140.0 - 120.0) * 200.0 - 2.0);
    assert!(account.get_position("US.NVDA").is_none());
    assert_eq!(account.positions.len(), 3);

    let hk = account.get_position("HK.00700").unwrap();
    assert_eq!(hk.quantity, 500.0);
    assert_eq!(hk.average_cost, 380.0);

    let cn = account.get_position("CN.600519").unwrap();
    assert_eq!(cn.quantity, 100.0);
    assert_eq!(cn.average_cost, 1600.0);

    let mut prices = BTreeMap::new();
    prices.insert("US.AAPL".to_string(), 200.0);
    prices.insert("HK.00700".to_string(), 400.0);
    prices.insert("CN.600519".to_string(), 1700.0);

    let expected_mv = (60.0 * 200.0) + (500.0 * 400.0) + (100.0 * 1700.0);
    let equity = account.total_equity(&prices);
    assert_eq!(equity, account.available_cash + expected_mv);
}

#[test]
fn test_edge_case_tiny_fractional_quantities() {
    let mut account = VirtualAccountState::new("sim-crypto", 10_000.0, 1000);

    account
        .apply_fill("CRYPTO", "BTCUSDT", "BUY", 0.005, 60_000.0, 0.3, 1001)
        .unwrap();
    assert_eq!(account.available_cash, 10_000.0 - 300.0 - 0.3);

    account
        .apply_fill("CRYPTO", "BTCUSDT", "BUY", 0.015, 70_000.0, 1.05, 1002)
        .unwrap();
    let pos = account.get_position("BTCUSDT").unwrap();
    assert_eq!(pos.quantity, 0.020);
    assert!((pos.average_cost - 67_500.0).abs() < 1e-9);

    let pnl = account
        .apply_fill("CRYPTO", "BTCUSDT", "SELL", 0.008, 80_000.0, 0.64, 1003)
        .unwrap();
    let expected_pnl = (80_000.0 - 67_500.0) * 0.008 - 0.64;
    assert!((pnl - expected_pnl).abs() < 1e-9);

    account
        .apply_fill("CRYPTO", "BTCUSDT", "SELL", 0.012, 80_000.0, 0.0, 1004)
        .unwrap();
    assert!(account.get_position("BTCUSDT").is_none());
}

#[test]
fn test_edge_case_huge_quantities_and_notionals() {
    let huge_cash = 100_000_000_000.0;
    let mut account = VirtualAccountState::new("sim-whale", huge_cash, 1000);

    account
        .apply_fill("US", "MEGA", "BUY", 10_000_000.0, 500.0, 50_000.0, 1001)
        .unwrap();
    assert_eq!(
        account.available_cash,
        huge_cash - 5_000_000_000.0 - 50_000.0
    );

    let pos = account.get_position("MEGA").unwrap();
    assert_eq!(pos.quantity, 10_000_000.0);
    assert_eq!(pos.average_cost, 500.0);

    let pnl = account
        .apply_fill("US", "MEGA", "SELL", 10_000_000.0, 550.0, 55_000.0, 1002)
        .unwrap();
    assert_eq!(pnl, 50.0 * 10_000_000.0 - 55_000.0);
    assert_eq!(account.available_cash, huge_cash + pnl - 50_000.0);
    assert!(account.get_position("MEGA").is_none());
}

#[test]
fn test_edge_case_zero_cash_boundary_and_exact_exhaustion() {
    let mut account = VirtualAccountState::new("sim-tight", 1000.0, 1000);

    account
        .apply_fill("US", "STK", "BUY", 10.0, 99.50, 5.0, 1001)
        .unwrap();
    assert_eq!(account.available_cash, 0.0);

    let err = account
        .apply_fill("US", "STK", "BUY", 1.0, 10.0, 0.0, 1002)
        .unwrap_err();
    assert!(
        matches!(err, SimulateExecutionError::InsufficientCash { available, .. } if available == 0.0)
    );

    account
        .apply_fill("US", "STK", "SELL", 5.0, 100.0, 1.0, 1003)
        .unwrap();
    assert_eq!(account.available_cash, 500.0 - 1.0);
}

#[test]
fn test_edge_case_initial_cash_sanitization() {
    let acc1 = VirtualAccountState::new("sim-neg", -500.0, 1000);
    assert_eq!(acc1.initial_cash, 0.0);
    assert_eq!(acc1.available_cash, 0.0);

    let acc2 = VirtualAccountState::new("sim-nan", f64::NAN, 1000);
    assert_eq!(acc2.initial_cash, 0.0);

    let acc3 = VirtualAccountState::new("sim-inf", f64::INFINITY, 1000);
    assert_eq!(acc3.initial_cash, 0.0);
}

#[test]
fn test_edge_case_fee_exceeding_notional_on_sell() {
    let mut account = VirtualAccountState::new("sim-fee-edge", 100.0, 1000);
    account
        .apply_fill("US", "PENNY", "BUY", 10.0, 5.0, 0.0, 1001)
        .unwrap();
    assert_eq!(account.available_cash, 50.0);

    let pnl = account
        .apply_fill("US", "PENNY", "SELL", 1.0, 2.0, 10.0, 1002)
        .unwrap();
    assert_eq!(pnl, -13.0);
    assert_eq!(account.available_cash, 42.0);
}

#[test]
fn test_invalid_operations_rejected_strictly() {
    let mut account = VirtualAccountState::new("sim-invalids", 1000.0, 1000);

    let err_cash = account
        .apply_fill("US", "AAPL", "BUY", 100.0, 150.0, 0.0, 1001)
        .unwrap_err();
    assert!(matches!(
        err_cash,
        SimulateExecutionError::InsufficientCash {
            available: 1000.0,
            required: 15000.0
        }
    ));

    let err_no_pos = account
        .apply_fill("US", "UNKNOWN", "SELL", 10.0, 50.0, 0.0, 1002)
        .unwrap_err();
    assert!(matches!(
        err_no_pos,
        SimulateExecutionError::InsufficientPosition {
            sellable: 0.0,
            required: 10.0
        }
    ));

    account
        .apply_fill("US", "AAPL", "BUY", 10.0, 50.0, 0.0, 1003)
        .unwrap();
    let err_over_sell = account
        .apply_fill("US", "AAPL", "SELL", 15.0, 60.0, 0.0, 1004)
        .unwrap_err();
    assert!(matches!(
        err_over_sell,
        SimulateExecutionError::InsufficientPosition {
            sellable: 10.0,
            required: 15.0
        }
    ));

    assert!(matches!(
        account.apply_fill("US", "", "BUY", 1.0, 10.0, 0.0, 1005),
        Err(SimulateExecutionError::EmptySymbol)
    ));
    assert!(matches!(
        account.apply_fill("US", "   ", "BUY", 1.0, 10.0, 0.0, 1006),
        Err(SimulateExecutionError::EmptySymbol)
    ));

    assert!(matches!(
        account.apply_fill("US", "AAPL", "BUY", 0.0, 10.0, 0.0, 1007),
        Err(SimulateExecutionError::InvalidQuantity(_))
    ));
    assert!(matches!(
        account.apply_fill("US", "AAPL", "BUY", -1.0, 10.0, 0.0, 1008),
        Err(SimulateExecutionError::InvalidQuantity(_))
    ));
    assert!(matches!(
        account.apply_fill("US", "AAPL", "BUY", f64::NAN, 10.0, 0.0, 1009),
        Err(SimulateExecutionError::InvalidQuantity(_))
    ));
    assert!(matches!(
        account.apply_fill("US", "AAPL", "BUY", f64::INFINITY, 10.0, 0.0, 1010),
        Err(SimulateExecutionError::InvalidQuantity(_))
    ));

    assert!(matches!(
        account.apply_fill("US", "AAPL", "BUY", 1.0, 0.0, 0.0, 1011),
        Err(SimulateExecutionError::InvalidPrice(_))
    ));
    assert!(matches!(
        account.apply_fill("US", "AAPL", "BUY", 1.0, -10.0, 0.0, 1012),
        Err(SimulateExecutionError::InvalidPrice(_))
    ));
    assert!(matches!(
        account.apply_fill("US", "AAPL", "BUY", 1.0, f64::NAN, 0.0, 1013),
        Err(SimulateExecutionError::InvalidPrice(_))
    ));

    assert!(matches!(
        account.apply_fill("US", "AAPL", "BUY", 1.0, 10.0, -0.01, 1014),
        Err(SimulateExecutionError::InvalidFee(_))
    ));
    assert!(matches!(
        account.apply_fill("US", "AAPL", "BUY", 1.0, 10.0, f64::NAN, 1015),
        Err(SimulateExecutionError::InvalidFee(_))
    ));

    assert!(matches!(
        account.apply_fill("US", "AAPL", "SHORT", 1.0, 10.0, 0.0, 1016),
        Err(SimulateExecutionError::UnsupportedSide(_))
    ));
    assert!(matches!(
        account.apply_fill("US", "AAPL", "CANCEL", 1.0, 10.0, 0.0, 1017),
        Err(SimulateExecutionError::UnsupportedSide(_))
    ));
}

#[test]
fn test_checkpoint_roundtrip_preserves_full_state() {
    let mut account = VirtualAccountState::new("sim-checkpoint-test", 75_000.0, 1000);
    account
        .apply_fill("US", "AAPL", "BUY", 50.0, 150.0, 2.0, 2000)
        .unwrap();
    account
        .apply_fill("US", "MSFT", "BUY", 30.0, 300.0, 1.5, 3000)
        .unwrap();
    account
        .apply_fill("US", "AAPL", "SELL", 20.0, 160.0, 1.0, 4000)
        .unwrap();

    let json_bytes = serde_json::to_vec(&account).expect("serialize");
    let mut restored: VirtualAccountState =
        serde_json::from_slice(&json_bytes).expect("deserialize");

    assert_eq!(account, restored);
    assert_eq!(restored.available_cash, account.available_cash);
    assert_eq!(restored.positions.len(), 2);

    restored
        .apply_fill("US", "MSFT", "SELL", 30.0, 310.0, 2.0, 5000)
        .unwrap();
    assert!(restored.get_position("MSFT").is_none());
    assert_eq!(restored.positions.len(), 1);
}

#[test]
fn test_high_frequency_multi_symbol_random_walk_portfolio_stress() {
    let mut rng = TestPrng::new(12345);
    let initial_cash = 250_000.0;
    let mut account = VirtualAccountState::new("sim-hf-multi", initial_cash, 1000);

    let symbols = [
        "US.AAPL",
        "US.MSFT",
        "US.GOOGL",
        "US.AMZN",
        "US.NVDA",
        "HK.00700",
        "HK.09988",
        "CN.600519",
        "CN.000858",
        "CRYPTO.BTC",
    ];
    let mut current_prices = BTreeMap::new();
    for s in &symbols {
        current_prices.insert(s.to_string(), 100.0);
    }

    let mut total_fees = 0.0;
    let mut gross_pnl = 0.0;

    for step in 1..=300 {
        let sym_idx = (rng.next_u64() as usize) % symbols.len();
        let sym = symbols[sym_idx];
        let prev = *current_prices.get(sym).unwrap();
        let new_price = (prev + rng.next_range(-2.0, 2.0)).max(5.0);
        current_prices.insert(sym.to_string(), new_price);
        let fee = rng.next_range(0.5, 2.5);

        if rng.next_bool() {
            let max_qty = ((account.available_cash - fee) / new_price).floor();
            if max_qty >= 1.0 {
                let qty = rng.next_range(1.0, max_qty.min(10.0)).floor().max(1.0);
                if account
                    .apply_fill("MKT", sym, "BUY", qty, new_price, fee, 1000 + step)
                    .is_ok()
                {
                    total_fees += fee;
                }
            }
        } else if let Some(pos) = account
            .get_position(sym)
            .filter(|p| p.sellable_quantity >= 1.0)
        {
            let qty = rng.next_range(1.0, pos.sellable_quantity).floor().max(1.0);
            let avg = pos.average_cost;
            if account
                .apply_fill("MKT", sym, "SELL", qty, new_price, fee, 1000 + step)
                .is_ok()
            {
                gross_pnl += (new_price - avg) * qty;
                total_fees += fee;
            }
        }

        let equity = account.total_equity(&current_prices);
        let open_unrealized: f64 = account
            .positions
            .values()
            .map(|p| {
                let p_mkt = current_prices
                    .get(&p.symbol)
                    .copied()
                    .unwrap_or(p.average_cost);
                (p_mkt - p.average_cost) * p.quantity
            })
            .sum();

        let delta = (equity + total_fees - (initial_cash + gross_pnl + open_unrealized)).abs();
        assert!(
            delta < 1e-5,
            "Identity violated at step {step}: delta={delta}"
        );
    }
}

#[test]
fn test_subsequent_reopen_after_complete_close() {
    let mut account = VirtualAccountState::new("sim-reopen", 10_000.0, 1000);

    account
        .apply_fill("US", "AAPL", "BUY", 10.0, 100.0, 0.0, 1001)
        .unwrap();
    let pnl1 = account
        .apply_fill("US", "AAPL", "SELL", 10.0, 120.0, 0.0, 1002)
        .unwrap();
    assert_eq!(pnl1, 200.0);
    assert_eq!(account.available_cash, 10_200.0);
    assert!(account.get_position("AAPL").is_none());

    account
        .apply_fill("US", "AAPL", "BUY", 5.0, 150.0, 0.0, 1003)
        .unwrap();
    let pos = account.get_position("AAPL").unwrap();
    assert_eq!(pos.quantity, 5.0);
    assert_eq!(
        pos.average_cost, 150.0,
        "Cost basis must be reset to 150.0, not contaminated"
    );

    let pnl2 = account
        .apply_fill("US", "AAPL", "SELL", 5.0, 140.0, 0.0, 1004)
        .unwrap();
    assert_eq!(pnl2, -50.0);
    assert_eq!(account.available_cash, 10_150.0);
    assert!(account.get_position("AAPL").is_none());
}

#[test]
fn test_total_equity_fallback_to_cost_basis_when_prices_missing() {
    let mut account = VirtualAccountState::new("sim-fallback", 50_000.0, 1000);
    account
        .apply_fill("US", "AAPL", "BUY", 100.0, 150.0, 0.0, 1001)
        .unwrap();
    account
        .apply_fill("US", "NVDA", "BUY", 50.0, 120.0, 0.0, 1002)
        .unwrap();

    let empty_prices = BTreeMap::new();
    let equity = account.total_equity(&empty_prices);
    let expected_equity = account.available_cash + (100.0 * 150.0) + (50.0 * 120.0);
    assert_eq!(equity, expected_equity);
    assert_eq!(equity, 50_000.0);
}

#[test]
fn test_repeated_partial_sells_precision_down_to_dust_limit() {
    let mut account = VirtualAccountState::new("sim-dust", 10_000.0, 1000);
    account
        .apply_fill("US", "AAPL", "BUY", 10.0, 100.0, 0.0, 1001)
        .unwrap();

    for i in 1..=9 {
        account
            .apply_fill("US", "AAPL", "SELL", 1.0, 100.0, 0.0, 1001 + i)
            .unwrap();
        let remaining = account.get_position("AAPL").unwrap().quantity;
        let expected = 10.0 - (i as f64);
        assert!((remaining - expected).abs() < 1e-12);
    }

    assert!((account.get_position("AAPL").unwrap().quantity - 1.0).abs() < 1e-12);

    account
        .apply_fill("US", "AAPL", "SELL", 1.0, 100.0, 0.0, 1020)
        .unwrap();
    assert!(account.get_position("AAPL").is_none());

    let err = account
        .apply_fill("US", "AAPL", "SELL", 0.001, 100.0, 0.0, 1021)
        .unwrap_err();
    assert!(
        matches!(err, SimulateExecutionError::InsufficientPosition { sellable, .. } if sellable == 0.0)
    );
}

#[test]
fn test_extreme_price_and_spread_volatility() {
    let mut account = VirtualAccountState::new("sim-volatile", 10_000.0, 1000);

    account
        .apply_fill("US", "MOON", "BUY", 1000.0, 1.0, 0.0, 1001)
        .unwrap();
    let pnl_gain = account
        .apply_fill("US", "MOON", "SELL", 1000.0, 1000.0, 0.0, 1002)
        .unwrap();
    assert_eq!(pnl_gain, 999_000.0);
    assert_eq!(account.available_cash, 10_000.0 - 1000.0 + 1_000_000.0);

    account
        .apply_fill("US", "CRASH", "BUY", 100.0, 1000.0, 0.0, 1003)
        .unwrap();
    let pnl_loss = account
        .apply_fill("US", "CRASH", "SELL", 100.0, 0.01, 0.0, 1004)
        .unwrap();
    assert!((pnl_loss - (-99_999.0)).abs() < 1e-6);
    assert!(account.available_cash.is_finite());
}
