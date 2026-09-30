//! Market-specific order-price normalization.
//!
//! Parity: `pkg/futu/exchange_trade_price.go`. OpenD rejects (or silently
//! mis-fills) prices that are not multiples of the venue tick, so the write
//! path rounds a positive submit/stop price to the market step before encoding
//! `Trd_PlaceOrder`. `TrdSecMarket_US` (2) uses a 0.0001 step below one dollar
//! and 0.01 otherwise; every other market keeps the caller's exact price.

const SEC_MARKET_US: i32 = 2;

/// Go `submitOrderPriceStep`: the minor-unit size for the order's market.
pub fn submit_order_price_step(sec_market: i32, price: f64) -> f64 {
    if sec_market == SEC_MARKET_US {
        if price > 0.0 && price < 1.0 {
            return 0.0001;
        }
        return 0.01;
    }
    0.0
}

/// Go `roundPriceToStep`: round to the nearest step multiple, then round the
/// result to the step's own decimal precision so float drift cannot leak into
/// the wire value.
///
/// Non-positive steps and non-finite/non-positive values are returned
/// untouched, matching Go's `step <= 0 || !isFinitePositive(value)` guard.
pub fn round_price_to_step(value: f64, step: f64) -> f64 {
    if step <= 0.0 || !is_finite_positive(value) {
        return value;
    }
    let decimals = count_step_decimals(step);
    let rounded = (value / step).round() * step;
    let unit = step_rounded_unit(decimals);
    let snapped = (rounded / unit).round() * unit;
    // Go writes the result through `fixedpoint.NewFromFloat`, which quantizes to
    // its eight-decimal representation and drops binary-float drift such as
    // `123.46000000000001`. Without this the protobuf `double` field would
    // carry a price the caller never requested.
    quantize_eight_decimals(snapped)
}

/// Mirrors `fixedpoint.NewFromFloat`'s eight-decimal quantization.
fn quantize_eight_decimals(value: f64) -> f64 {
    if !value.is_finite() {
        return value;
    }
    let scaled = value * 100_000_000.0;
    if !scaled.is_finite() {
        return value;
    }
    scaled.round() / 100_000_000.0
}

/// Go `stepRoundedUnit`: `1 / 10^decimals`, clamped to at least 1 so a
/// nonsensical negative precision cannot divide by a fraction larger than one.
pub fn step_rounded_unit(decimals: i32) -> f64 {
    let factor = 10_f64.powi(decimals);
    if factor <= 0.0 {
        return 1.0;
    }
    1.0 / factor
}

/// Go `countStepDecimals`: digits after the decimal point in `step`.
pub fn count_step_decimals(step: f64) -> i32 {
    let text = format_shortest_f64(step);
    match text.split_once('.') {
        Some((_, fraction)) => fraction.len() as i32,
        None => 0,
    }
}

/// Go `normalizeSubmitOrderPrice`: round a positive price to the market step.
///
/// Zero and negative prices are returned unchanged so an absent price stays
/// absent on the wire instead of becoming a zero-filled field.
pub fn normalize_submit_order_price(sec_market: i32, price: f64) -> f64 {
    if !is_finite_positive(price) {
        return price;
    }
    let step = submit_order_price_step(sec_market, price);
    if step <= 0.0 {
        return price;
    }
    round_price_to_step(price, step)
}

fn is_finite_positive(value: f64) -> bool {
    !value.is_nan() && !value.is_infinite() && value > 0.0
}

/// Rust's `{}` for f64 already prints the shortest representation that round
/// trips, matching Go's `strconv.FormatFloat(v, 'f', -1, 64)` digit selection
/// for the step values this module accepts.
fn format_shortest_f64(value: f64) -> String {
    format!("{value}")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    // Parity: go:452dea11:pkg/futu/adapter_failure_boundaries_test.go:77 TestAdapterAndDecimalConversionBoundaries
    // Parity: go:452dea11:pkg/futu/exchange_trade_price_test.go:24 TestNormalizeSubmitOrderPriceForUSMarkets
    fn normalize_submit_order_price_rounds_us_prices_to_their_tick() {
        // Parity: go:452dea11:pkg/futu/exchange_trade_price_test.go:25
        // TestNormalizeSubmitOrderPrice. A US order at or above one dollar uses
        // the cent tick; sub-dollar names use the $0.0001 step.
        assert_eq!(normalize_submit_order_price(SEC_MARKET_US, 123.456), 123.46);
        // Parity: go:452dea11:internal/app/apiserver/servercoretest/exec_validate_test.go:18
        // TestExecutionOrderRoutesNormalizeUSPricePrecision. The route test
        // submits TME at 10.123 and reads 10.12 back off the OpenD place-order
        // request, so the cent tick has to round half-down here as well.
        assert_eq!(normalize_submit_order_price(SEC_MARKET_US, 10.123), 10.12);
        assert_eq!(normalize_submit_order_price(SEC_MARKET_US, 0.12344), 0.1234);
        assert_eq!(normalize_submit_order_price(SEC_MARKET_US, 0.12345), 0.1235);
        // HK keeps the caller's exact price; there is no HK tick table here.
        assert_eq!(normalize_submit_order_price(1, 320.123), 320.123);
    }

    #[test]
    fn helper_boundaries_return_the_input_unchanged() {
        // Parity: go:452dea11:pkg/futu/adapter_failure_boundaries_test.go:115
        // TestKLineSessionAndPriceHelperBoundaries (price half).
        assert_eq!(normalize_submit_order_price(SEC_MARKET_US, 0.0), 0.0);
        assert!(round_price_to_step(f64::NAN, 0.01).is_nan());
        assert_eq!(round_price_to_step(1.0, 0.0), 1.0);
        assert_eq!(step_rounded_unit(-1000), 1.0);
    }

    #[test]
    fn price_step_helpers_cover_their_edge_cases() {
        // Parity: go:452dea11:pkg/futu/exchange_trade_price_test.go:39
        // TestPriceStepHelpersCoverEdgeCases.
        assert_eq!(submit_order_price_step(SEC_MARKET_US, 150.0), 0.01);
        assert_eq!(submit_order_price_step(SEC_MARKET_US, 0.55), 0.0001);
        // HK carries no tick table: the step stays zero and the price is kept.
        assert_eq!(submit_order_price_step(1, 380.0), 0.0);
        // Sub-dollar and cent rounding, then a non-decimal tick (0.05).
        assert_eq!(round_price_to_step(0.12344, 0.0001), 0.1234);
        assert_eq!(round_price_to_step(123.456, 0.01), 123.46);
        assert_eq!(round_price_to_step(10.03, 0.05), 10.05);
        // `step_rounded_unit` follows `10^-decimals`, clamped to >= 1.
        assert_eq!(step_rounded_unit(4), 0.0001);
        assert_eq!(step_rounded_unit(0), 1.0);
        // `isFinitePositive` rejects NaN/Inf and non-positive values.
        assert!(!is_finite_positive(0.0));
        assert!(!is_finite_positive(-1.0));
        assert!(!is_finite_positive(f64::NAN));
        assert!(!is_finite_positive(f64::INFINITY));
        assert!(is_finite_positive(0.01));
    }

    #[test]
    fn step_decimals_follow_the_shortest_representation() {
        assert_eq!(count_step_decimals(0.01), 2);
        assert_eq!(count_step_decimals(0.0001), 4);
        assert_eq!(count_step_decimals(1.0), 0);
        assert_eq!(count_step_decimals(0.05), 2);
    }
}
