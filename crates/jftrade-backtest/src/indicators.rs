use jftrade_kernel::Fixed8;

use crate::BacktestError;
use crate::model::IndicatorOutput;

pub(crate) fn calculate_indicators(
    closes: &[Fixed8],
    periods: &[usize],
) -> Result<Vec<IndicatorOutput>, BacktestError> {
    let mut output = Vec::with_capacity(periods.len() * 2);
    for &period in periods {
        if period == 0 {
            return Err(BacktestError::InvalidInput(
                "indicator period must be positive".to_owned(),
            ));
        }
        output.push(IndicatorOutput {
            kind: "sma",
            period,
            values: simple_moving_average(closes, period)?,
        });
        output.push(IndicatorOutput {
            kind: "ema",
            period,
            values: exponential_moving_average(closes, period)?,
        });
    }
    Ok(output)
}

fn simple_moving_average(
    values: &[Fixed8],
    period: usize,
) -> Result<Vec<Option<String>>, BacktestError> {
    let divisor = integer_fixed(period)?;
    let mut sum = Fixed8::ZERO;
    let mut output = Vec::with_capacity(values.len());
    for (index, value) in values.iter().copied().enumerate() {
        sum = sum.checked_add(value)?;
        if index >= period {
            sum = sum.checked_sub(values[index - period])?;
        }
        if index + 1 < period {
            output.push(None);
        } else {
            output.push(Some(sum.checked_div(divisor)?.storage_text()));
        }
    }
    Ok(output)
}

fn exponential_moving_average(
    values: &[Fixed8],
    period: usize,
) -> Result<Vec<Option<String>>, BacktestError> {
    if values.is_empty() {
        return Ok(Vec::new());
    }
    let alpha = integer_fixed(2)?.checked_div(integer_fixed(period + 1)?)?;
    let one_minus_alpha = "1".parse::<Fixed8>()?.checked_sub(alpha)?;
    let mut current = values[0];
    let mut output = Vec::with_capacity(values.len());
    output.push(Some(current.storage_text()));
    for value in &values[1..] {
        current = value
            .checked_mul(alpha)?
            .checked_add(current.checked_mul(one_minus_alpha)?)?;
        output.push(Some(current.storage_text()));
    }
    Ok(output)
}

/// Calculates an EMA with the initialization semantics used by the PineTS
/// shadow reference.
///
/// Unlike the stage 3 wire indicator above, this variant emits `NaN` until it
/// has seen `period` non-`NaN` inputs and seeds the EMA from their arithmetic
/// mean. `NaN` inputs are skipped and leave the corresponding output as
/// `NaN`; they do not reset an already initialized EMA.
pub fn pine_compatible_ema(values: &[f64], period: usize) -> Result<Vec<f64>, BacktestError> {
    let (_, output) = pine_compatible_ema_series(values, period)?;
    Ok(output)
}

/// Calculates both the unrounded EMA state and the value exposed by PineTS.
///
/// PineTS rounds each value returned by `ta.ema`, but keeps the unrounded value
/// in its incremental state. Keeping those two representations separate is
/// observable on later bars, so callers that compose indicators must retain
/// the raw series while exposing the rounded one.
fn pine_compatible_ema_series(
    values: &[f64],
    period: usize,
) -> Result<(Vec<f64>, Vec<f64>), BacktestError> {
    validate_pine_period("EMA", period)?;

    let mut raw_output = vec![f64::NAN; values.len()];
    let mut output = vec![f64::NAN; values.len()];
    if values.is_empty() {
        return Ok((raw_output, output));
    }

    let period_as_float = period as f64;
    let alpha = 2.0 / (period_as_float + 1.0);
    let one_minus_alpha = 1.0 - alpha;
    let mut initialization_sum = 0.0;
    let mut initialization_count = 0usize;
    let mut previous = 0.0;

    for (index, &value) in values.iter().enumerate() {
        if value.is_nan() {
            continue;
        }
        if initialization_count < period {
            initialization_sum += value;
            initialization_count += 1;
            if initialization_count == period {
                previous = initialization_sum / period_as_float;
                raw_output[index] = previous;
                output[index] = pine_precision(previous);
            }
            continue;
        }
        previous = alpha * value + one_minus_alpha * previous;
        raw_output[index] = previous;
        output[index] = pine_precision(previous);
    }

    Ok((raw_output, output))
}

/// MACD output in line, signal, and histogram order.
pub type PineCompatibleMacd = (Vec<f64>, Vec<f64>, Vec<f64>);

/// Calculates the PineTS-compatible MACD line, signal line, and histogram.
///
/// The signal EMA receives the MACD line including its leading `NaN` values,
/// so signal initialization starts after `signal_period` valid MACD values,
/// matching the PineTS shadow reference.
pub fn pine_compatible_macd(
    values: &[f64],
    fast_period: usize,
    slow_period: usize,
    signal_period: usize,
) -> Result<PineCompatibleMacd, BacktestError> {
    validate_pine_period("MACD fast", fast_period)?;
    validate_pine_period("MACD slow", slow_period)?;
    validate_pine_period("MACD signal", signal_period)?;

    let (_, fast_ema) = pine_compatible_ema_series(values, fast_period)?;
    let (_, slow_ema) = pine_compatible_ema_series(values, slow_period)?;
    let mut raw_macd = vec![f64::NAN; values.len()];
    let mut macd = vec![f64::NAN; values.len()];
    for index in 0..values.len() {
        if !fast_ema[index].is_nan() && !slow_ema[index].is_nan() {
            let macd_line = fast_ema[index] - slow_ema[index];
            raw_macd[index] = macd_line;
            macd[index] = pine_precision(macd_line);
        }
    }

    let (_, signal) = pine_compatible_ema_series(&raw_macd, signal_period)?;
    let mut histogram = vec![f64::NAN; values.len()];
    for index in 0..values.len() {
        if !raw_macd[index].is_nan() && !signal[index].is_nan() {
            histogram[index] = pine_precision(raw_macd[index] - signal[index]);
        }
    }
    Ok((macd, signal, histogram))
}

const PINE_PRECISION_SCALE: f64 = 10_000_000_000.0;

/// Matches PineTS's `Math.round(value * 1e10) / 1e10` operation.
///
/// JavaScript rounds halfway cases toward positive infinity, unlike Rust's
/// `f64::round`, which rounds halfway cases away from zero. The explicit
/// negative-zero branch also preserves `Math.round(-0.5) === -0`.
fn pine_precision(value: f64) -> f64 {
    if value.is_nan() || value.is_infinite() {
        return value;
    }

    js_math_round(value * PINE_PRECISION_SCALE) / PINE_PRECISION_SCALE
}

fn js_math_round(value: f64) -> f64 {
    if value.is_nan() || value.is_infinite() || value == 0.0 {
        return value;
    }

    let lower = value.floor();
    let rounded = if value - lower >= 0.5 {
        lower + 1.0
    } else {
        lower
    };
    if rounded == 0.0 && value.is_sign_negative() {
        -0.0
    } else {
        rounded
    }
}

fn validate_pine_period(indicator: &str, period: usize) -> Result<(), BacktestError> {
    if period == 0 {
        return Err(BacktestError::InvalidInput(format!(
            "Pine-compatible {indicator} period must be positive"
        )));
    }
    Ok(())
}

fn integer_fixed(value: usize) -> Result<Fixed8, BacktestError> {
    value
        .to_string()
        .parse::<Fixed8>()
        .map_err(BacktestError::from)
}

#[cfg(test)]
mod tests {
    use jftrade_kernel::Fixed8;

    use super::calculate_indicators;

    #[test]
    fn indicator_properties_stay_within_go_compatibility_bounds() {
        for period in 1..=32 {
            let values = vec!["42".parse::<Fixed8>().expect("value"); period * 3];
            let indicators = calculate_indicators(&values, &[period]).expect("indicators");
            for indicator in indicators {
                for value in indicator.values.into_iter().flatten() {
                    if indicator.kind == "sma" {
                        assert_eq!(value, "42");
                    } else {
                        let scaled = value.parse::<Fixed8>().expect("EMA value").scaled();
                        assert!(scaled <= 4_200_000_000);
                        assert!(scaled >= 4_199_999_000);
                    }
                }
            }
        }
    }
}
