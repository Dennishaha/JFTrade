use rust_decimal::Decimal;

use crate::BacktestError;
use crate::model::{DrawdownPoint, EquityPoint};

pub(crate) fn drawdown_metrics(
    equity_curve: &[EquityPoint],
) -> Result<(String, String, Vec<DrawdownPoint>), BacktestError> {
    let Some(first) = equity_curve.first() else {
        return Ok(("0".to_owned(), "0".to_owned(), Vec::new()));
    };
    let mut peak = first.equity.parse::<Decimal>()?;
    let mut maximum = Decimal::ZERO;
    let mut current = Decimal::ZERO;
    let mut curve = Vec::with_capacity(equity_curve.len());
    for point in equity_curve {
        let equity = point.equity.parse::<Decimal>()?;
        if equity > peak {
            peak = equity;
        }
        current = if peak > Decimal::ZERO && equity < peak {
            (peak - equity)
                .checked_div(peak)
                .ok_or_else(|| BacktestError::Arithmetic("peak division failed".into()))?
        } else {
            Decimal::ZERO
        };
        maximum = maximum.max(current);
        curve.push(DrawdownPoint {
            time: point.time.clone(),
            drawdown: metric_text(current),
        });
    }
    Ok((metric_text(maximum), metric_text(current), curve))
}

pub(crate) fn metric_text(value: Decimal) -> String {
    let rounded = value.round_dp(12);
    let text = format!("{rounded:.12}");
    let trimmed = text.trim_end_matches('0').trim_end_matches('.');
    if trimmed.is_empty() || trimmed == "-0" {
        "0".to_owned()
    } else {
        trimmed.to_owned()
    }
}
