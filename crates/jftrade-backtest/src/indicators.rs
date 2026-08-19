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
