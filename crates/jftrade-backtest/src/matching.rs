use jftrade_kernel::Fixed8;

use crate::model::Candle;

#[derive(Clone, Copy)]
pub(crate) enum MatchMode {
    FullBar,
    ClosePoint,
}

pub(crate) fn limit_price(
    side: &str,
    limit: Fixed8,
    candle: &Candle,
    mode: MatchMode,
) -> Option<Fixed8> {
    if limit <= Fixed8::ZERO {
        return None;
    }
    match (mode, side) {
        (MatchMode::ClosePoint, "buy") if candle.close > Fixed8::ZERO && candle.close <= limit => {
            Some(candle.close)
        }
        (MatchMode::ClosePoint, "sell") if candle.close > Fixed8::ZERO && candle.close >= limit => {
            Some(candle.close)
        }
        (MatchMode::FullBar, "buy") if candle.open > Fixed8::ZERO && candle.open <= limit => {
            Some(candle.open)
        }
        (MatchMode::FullBar, "buy") if candle.low > Fixed8::ZERO && candle.low <= limit => {
            Some(limit)
        }
        (MatchMode::FullBar, "sell") if candle.open > Fixed8::ZERO && candle.open >= limit => {
            Some(candle.open)
        }
        (MatchMode::FullBar, "sell") if candle.high > Fixed8::ZERO && candle.high >= limit => {
            Some(limit)
        }
        _ => None,
    }
}

pub(crate) fn stop_market_price(
    side: &str,
    stop: Fixed8,
    candle: &Candle,
    mode: MatchMode,
) -> Option<Fixed8> {
    if stop <= Fixed8::ZERO {
        return None;
    }
    match (mode, side) {
        (MatchMode::ClosePoint, "buy") if candle.close > Fixed8::ZERO && candle.close >= stop => {
            Some(candle.close)
        }
        (MatchMode::ClosePoint, "sell") if candle.close > Fixed8::ZERO && candle.close <= stop => {
            Some(candle.close)
        }
        (MatchMode::FullBar, "buy") if candle.open > Fixed8::ZERO && candle.open >= stop => {
            Some(candle.open)
        }
        (MatchMode::FullBar, "buy") if candle.high > Fixed8::ZERO && candle.high >= stop => {
            Some(stop)
        }
        (MatchMode::FullBar, "sell") if candle.open > Fixed8::ZERO && candle.open <= stop => {
            Some(candle.open)
        }
        (MatchMode::FullBar, "sell") if candle.low > Fixed8::ZERO && candle.low <= stop => {
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
