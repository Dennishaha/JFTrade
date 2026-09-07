use jftrade_kernel::Decimal;

use crate::model::Candle;

#[derive(Clone, Copy)]
pub(crate) enum MatchMode {
    FullBar,
    ClosePoint,
}

pub(crate) fn limit_price(
    side: &str,
    limit: Decimal,
    candle: &Candle,
    mode: MatchMode,
) -> Option<Decimal> {
    if limit <= Decimal::ZERO {
        return None;
    }
    match (mode, side) {
        (MatchMode::ClosePoint, "buy") if candle.close > Decimal::ZERO && candle.close <= limit => {
            Some(candle.close)
        }
        (MatchMode::ClosePoint, "sell")
            if candle.close > Decimal::ZERO && candle.close >= limit =>
        {
            Some(candle.close)
        }
        (MatchMode::FullBar, "buy") if candle.open > Decimal::ZERO && candle.open <= limit => {
            Some(candle.open)
        }
        (MatchMode::FullBar, "buy") if candle.low > Decimal::ZERO && candle.low <= limit => {
            Some(limit)
        }
        (MatchMode::FullBar, "sell") if candle.open > Decimal::ZERO && candle.open >= limit => {
            Some(candle.open)
        }
        (MatchMode::FullBar, "sell") if candle.high > Decimal::ZERO && candle.high >= limit => {
            Some(limit)
        }
        _ => None,
    }
}

pub(crate) fn stop_market_price(
    side: &str,
    stop: Decimal,
    candle: &Candle,
    mode: MatchMode,
) -> Option<Decimal> {
    if stop <= Decimal::ZERO {
        return None;
    }
    match (mode, side) {
        (MatchMode::ClosePoint, "buy") if candle.close > Decimal::ZERO && candle.close >= stop => {
            Some(candle.close)
        }
        (MatchMode::ClosePoint, "sell") if candle.close > Decimal::ZERO && candle.close <= stop => {
            Some(candle.close)
        }
        (MatchMode::FullBar, "buy") if candle.open > Decimal::ZERO && candle.open >= stop => {
            Some(candle.open)
        }
        (MatchMode::FullBar, "buy") if candle.high > Decimal::ZERO && candle.high >= stop => {
            Some(stop)
        }
        (MatchMode::FullBar, "sell") if candle.open > Decimal::ZERO && candle.open <= stop => {
            Some(candle.open)
        }
        (MatchMode::FullBar, "sell") if candle.low > Decimal::ZERO && candle.low <= stop => {
            Some(stop)
        }
        _ => None,
    }
}

pub(crate) fn event_time(candle: &Candle, mode: MatchMode) -> String {
    match mode {
        MatchMode::FullBar => candle.start.to_string(),
        MatchMode::ClosePoint => candle.end.to_string(),
    }
}
