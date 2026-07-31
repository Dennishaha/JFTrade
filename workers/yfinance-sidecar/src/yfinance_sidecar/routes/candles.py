"""Historical candle route with interval and time-boundary validation."""

from __future__ import annotations

import calendar
from datetime import datetime, timedelta, timezone

from fastapi import APIRouter, Query

from .. import upstream
from ..conversion import convert_history, parse_rfc3339_utc
from ..errors import invalid_request, not_found, upstream_error
from ..models import CandlesResponse
from .common import normalize_instrument, quote_is_supported, quote_matches_instrument

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


@router.get("/candles/{market}/{symbol}", response_model=CandlesResponse)
def candles(
    market: str,
    symbol: str,
    period: str = Query(default="1d"),
    limit: int = Query(default=200, ge=1, le=1000),
    from_value: str | None = Query(default=None, alias="from"),
    to_value: str | None = Query(default=None, alias="to"),
) -> CandlesResponse:
    instrument = normalize_instrument(market, symbol)
    normalized_period = period.strip().lower()
    if normalized_period not in INTERVALS:
        raise invalid_request(
            "unsupported_period",
            f"unsupported candle period: {period}",
        )
    from_time = parse_rfc3339_utc(from_value, "from")
    to_time = parse_rfc3339_utc(to_value, "to")
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
    try:
        frame = upstream.ticker_history(
            instrument.yahoo_symbol,
            interval=INTERVALS[normalized_period],
            fetch_period=FETCH_PERIODS[normalized_period],
            start=history_start(from_time, to_time, normalized_period),
            end=inclusive_history_end(to_time, normalized_period),
            prepost=instrument.spec.supports_extended_hours,
        )
    except Exception as exc:
        raise upstream_error("Yahoo Finance candle lookup failed") from exc
    converted = convert_history(
        frame,
        period=normalized_period,
        limit=limit,
        from_time=from_time,
        to_time=to_time,
        exchange_timezone=instrument.spec.timezone,
        market=instrument.market,
    )
    if not converted:
        raise not_found(
            "candles_not_found",
            f"candles not found: {instrument.instrument_id}",
        )
    return CandlesResponse(
        market=instrument.market,
        symbol=instrument.symbol,
        instrument_id=instrument.instrument_id,
        period=normalized_period,
        extended_hours=instrument.spec.supports_extended_hours,
        candles=converted,
        total_returned=len(converted),
        source="yfinance",
    )


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
