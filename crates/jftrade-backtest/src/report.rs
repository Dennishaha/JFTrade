use jftrade_kernel::Fixed8;

use crate::BacktestError;
use crate::model::{DrawdownPoint, EquityPoint};

pub(crate) fn drawdown_metrics(
    equity_curve: &[EquityPoint],
) -> Result<(String, String, Vec<DrawdownPoint>), BacktestError> {
    let Some(first) = equity_curve.first() else {
        return Ok(("0".to_owned(), "0".to_owned(), Vec::new()));
    };
    let mut peak = first.equity.parse::<Fixed8>()?.to_f64()?;
    let mut maximum = 0.0_f64;
    let mut current = 0.0_f64;
    let mut curve = Vec::with_capacity(equity_curve.len());
    for point in equity_curve {
        let equity = point.equity.parse::<Fixed8>()?.to_f64()?;
        if equity > peak {
            peak = equity;
        }
        current = if peak > 0.0 && equity < peak {
            (peak - equity) / peak
        } else {
            0.0
        };
        maximum = maximum.max(current);
        curve.push(DrawdownPoint {
            time: point.time.clone(),
            drawdown: metric_text(current),
        });
    }
    Ok((metric_text(maximum), metric_text(current), curve))
}

pub(crate) fn metric_text(value: f64) -> String {
    let text = format!("{value:.12}");
    let trimmed = text.trim_end_matches('0').trim_end_matches('.');
    if trimmed.is_empty() || trimmed == "-0" {
        "0".to_owned()
    } else {
        trimmed.to_owned()
    }
}
