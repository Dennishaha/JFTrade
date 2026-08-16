"""Historical candle route with interval and time-boundary validation."""

from __future__ import annotations

import calendar
from datetime import datetime, timedelta, timezone

from fastapi import APIRouter, Query

from .. import upstream
from ..conversion import convert_history, parse_rfc3339_utc
from ..errors import invalid_request, not_found, upstream_error
from ..models import Candle, CandlesResponse
from .common import (
    normalize_instrument,
    parse_candle_adjustment,
    parse_candle_sessions,
    quote_is_supported,
    quote_matches_instrument,
)

router = APIRouter()

INTERVALS = {
    "1m": "1m",
    "5m": "5m",
    "15m": "15m",
    "30m": "30m",
    "1h": "1h",
    "1d": "1d",
    "1w": "1wk",
    "1mo": "1mo",
}

FETCH_PERIODS = {
    "1m": "7d",
    "5m": "60d",
    "15m": "60d",
    "30m": "60d",
    "1h": "730d",
    "1d": "5y",
    "1w": "max",
    "1mo": "max",
}

INTERVAL_DELTAS = {
    "1m": timedelta(minutes=1),
    "5m": timedelta(minutes=5),
    "15m": timedelta(minutes=15),
    "30m": timedelta(minutes=30),
    "1h": timedelta(hours=1),
    "1d": timedelta(days=1),
    "1w": timedelta(days=7),
}

PAGED_RETENTION = {
    "1m": timedelta(days=7),
    "5m": timedelta(days=60),
    "15m": timedelta(days=60),
    "30m": timedelta(days=60),
    "1h": timedelta(days=730),
}

HISTORY_FLOOR = datetime(1900, 1, 1, tzinfo=timezone.utc)


@router.get("/candles/{market}/{symbol}", response_model=CandlesResponse)
def candles(
    market: str,
    symbol: str,
    period: str = Query(default="1d"),
    limit: int = Query(default=200, ge=1, le=1000),
    from_value: str | None = Query(default=None, alias="from"),
    to_value: str | None = Query(default=None, alias="to"),
    before_value: str | None = Query(default=None, alias="before"),
    sessions: list[str] | None = Query(default=None),
    adjustment: str | None = Query(default=None),
) -> CandlesResponse:
    instrument = normalize_instrument(market, symbol)
    normalized_period = period.strip().lower()
    if normalized_period not in INTERVALS:
        raise invalid_request(
            "unsupported_period",
            f"unsupported candle period: {period}",
        )
    selected_adjustment = parse_candle_adjustment(adjustment)
    if selected_adjustment == "backward":
        # Yahoo Finance only exposes forward-adjusted (auto_adjust) history;
        # backward-adjusted series cannot be reconstructed without actions.
        raise invalid_request(
            "unsupported_time_range",
            "Yahoo Finance does not provide backward-adjusted candles",
        )
    selected_sessions = parse_candle_sessions(
        sessions,
        market=instrument.market,
        period=normalized_period,
    )
    from_time = parse_rfc3339_utc(from_value, "from")
    to_time = parse_rfc3339_utc(to_value, "to")
    before_time = parse_rfc3339_utc(before_value, "before")
    if before_time is not None and (from_time is not None or to_time is not None):
        raise invalid_request(
            "invalid_time_range",
            "before cannot be combined with from or to",
        )
    if from_time is not None and to_time is not None and from_time > to_time:
        raise invalid_request(
            "invalid_time_range",
            "from must be earlier than or equal to to",
        )
    if normalized_period == "1m" and from_time is not None:
        cutoff = datetime.now(timezone.utc) - timedelta(days=7)
        if from_time < cutoff:
            raise invalid_request(
                "unsupported_time_range",
                "1m candle data is only available for the last 7 days",
            )
    try:
        info = upstream.ticker_info(
            instrument.yahoo_symbol,
            max_age_seconds=upstream.SECURITY_CACHE_SECONDS,
        )
    except Exception as exc:
        raise upstream_error("Yahoo Finance candle lookup failed") from exc
    if not quote_is_supported(info, instrument.market) or not quote_matches_instrument(
        info, instrument
    ):
        raise not_found(
            "candles_not_found",
            f"candles not found: {instrument.instrument_id}",
        )
    if from_time is not None or to_time is not None:
        converted = bounded_history(
            instrument.yahoo_symbol,
            normalized_period,
            limit,
            from_time,
            to_time,
            selected_sessions,
            instrument.spec.timezone,
            auto_adjust=selected_adjustment == "forward",
        )
        if not converted:
            raise not_found(
                "candles_not_found",
                f"candles not found: {instrument.instrument_id}",
            )
        return candle_response(
            instrument.market,
            instrument.symbol,
            instrument.instrument_id,
            normalized_period,
            selected_sessions,
            converted,
            False,
            selected_adjustment,
        )

    converted, has_more = paged_history(
        instrument.yahoo_symbol,
        normalized_period,
        limit,
        before_time,
        selected_sessions,
        instrument.spec.timezone,
        auto_adjust=selected_adjustment == "forward",
    )
    if not converted and before_time is None:
        raise not_found(
            "candles_not_found",
            f"candles not found: {instrument.instrument_id}",
        )
    return candle_response(
        instrument.market,
        instrument.symbol,
        instrument.instrument_id,
        normalized_period,
        selected_sessions,
        converted,
        has_more,
        selected_adjustment,
    )


def candle_response(
    market: str,
    symbol: str,
    instrument_id: str,
    period: str,
    sessions: tuple[str, ...],
    candles: list[Candle],
    has_more: bool,
    adjustment: str,
) -> CandlesResponse:
    return CandlesResponse(
        market=market,
        symbol=symbol,
        instrument_id=instrument_id,
        period=period,
        extended_hours="extended" in sessions,
        candles=candles,
        total_returned=len(candles),
        has_more=has_more,
        next_before=candles[0].at if has_more else None,
        source="yfinance",
        adjustment=adjustment,
    )


def bounded_history(
    symbol: str,
    period: str,
    limit: int,
    from_time: datetime | None,
    to_time: datetime | None,
    sessions: tuple[str, ...],
    exchange_timezone: str,
    *,
    auto_adjust: bool = False,
) -> list[Candle]:
    return read_history(
        symbol,
        period,
        limit,
        history_start(from_time, to_time, period),
        inclusive_history_end(to_time, period),
        from_time,
        to_time,
        None,
        sessions,
        exchange_timezone,
        auto_adjust=auto_adjust,
    )


def paged_history(
    symbol: str,
    period: str,
    limit: int,
    before_time: datetime | None,
    sessions: tuple[str, ...],
    exchange_timezone: str,
    *,
    auto_adjust: bool = False,
) -> tuple[list[Candle], bool]:
    now = datetime.now(timezone.utc)
    end_time = before_time or now
    lower_bound = history_lower_bound(period, now)
    if before_time is not None and before_time <= lower_bound:
        return [], False

    start_time = max(lower_bound, end_time - page_lookback(period, limit))
    if FETCH_PERIODS[period] == "max":
        start_time = lower_bound
    converted = read_history(
        symbol,
        period,
        limit + 1,
        start_time,
        end_time,
        start_time,
        end_time,
        before_time,
        sessions,
        exchange_timezone,
        auto_adjust=auto_adjust,
    )
    if len(converted) <= limit and start_time > lower_bound:
        converted = read_history(
            symbol,
            period,
            limit + 1,
            lower_bound,
            end_time,
            lower_bound,
            end_time,
            before_time,
            sessions,
            exchange_timezone,
            auto_adjust=auto_adjust,
        )
    has_more = len(converted) > limit
    if has_more:
        converted = converted[-limit:]
    return converted, has_more


def read_history(
    symbol: str,
    period: str,
    limit: int,
    start: datetime | None,
    end: datetime | None,
    from_time: datetime | None,
    to_time: datetime | None,
    before_time: datetime | None,
    sessions: tuple[str, ...],
    exchange_timezone: str,
    *,
    auto_adjust: bool = False,
) -> list[Candle]:
    try:
        frame = upstream.ticker_history(
            symbol,
            interval=INTERVALS[period],
            fetch_period=FETCH_PERIODS[period],
            start=start,
            end=end,
            prepost="extended" in sessions,
            auto_adjust=auto_adjust,
        )
    except Exception as exc:
        raise upstream_error("Yahoo Finance candle lookup failed") from exc
    return convert_history(
        frame,
        limit=limit,
        from_time=from_time,
        to_time=to_time,
        before_time=before_time,
        exchange_timezone=exchange_timezone,
        sessions=sessions,
        period=period,
    )


def history_lower_bound(period: str, now: datetime) -> datetime:
    retention = PAGED_RETENTION.get(period)
    if retention is None:
        return HISTORY_FLOOR
    return now - retention


def page_lookback(period: str, limit: int) -> timedelta:
    if period == "1mo":
        interval = timedelta(days=31)
    else:
        interval = INTERVAL_DELTAS[period]
    return max(interval * (limit + 1) * 2, timedelta(days=7))


def history_start(
    from_time: datetime | None,
    to_time: datetime | None,
    period: str,
) -> datetime | None:
    """Give a to-only request an explicit bounded start for yfinance."""
    if from_time is not None or to_time is None:
        return from_time
    fetch_period = FETCH_PERIODS[period]
    if fetch_period == "max":
        return shift_year(to_time, -99)
    amount = int(fetch_period[:-1])
    if fetch_period.endswith("d"):
        return to_time - timedelta(days=amount)
    return shift_year(to_time, -amount)


def inclusive_history_end(to_time: datetime | None, period: str) -> datetime | None:
    """Translate JFTrade's inclusive `to` into yfinance's exclusive `end`."""
    if to_time is None:
        return None
    if period != "1mo":
        return to_time + INTERVAL_DELTAS[period]
    year = to_time.year + (to_time.month // 12)
    month = (to_time.month % 12) + 1
    day = min(to_time.day, calendar.monthrange(year, month)[1])
    return to_time.replace(year=year, month=month, day=day)


def shift_year(value: datetime, years: int) -> datetime:
    """Shift a timestamp by whole calendar years, clamping leap days."""
    year = value.year + years
    day = min(value.day, calendar.monthrange(year, value.month)[1])
    return value.replace(year=year, day=day)
