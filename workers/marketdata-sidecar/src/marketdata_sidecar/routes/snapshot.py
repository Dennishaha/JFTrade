"""Delayed Yahoo Finance quote snapshots."""

from __future__ import annotations

from datetime import datetime, timezone
from typing import Any, Mapping

from fastapi import APIRouter

from .. import upstream
from ..conversion import (
    clean_text,
    finite_float,
    first_value,
    format_rfc3339,
    non_negative_int,
    timestamp_as_rfc3339,
)
from ..errors import not_found, upstream_error
from ..models import SnapshotQuote, SnapshotResponse
from .common import (
    normalize_instrument,
    normalized_exchange,
    quote_is_supported,
    quote_matches_instrument,
)

router = APIRouter()


@router.get("/snapshot/{market}/{symbol}", response_model=SnapshotResponse)
def snapshot(market: str, symbol: str) -> SnapshotResponse:
    instrument = normalize_instrument(market, symbol)
    try:
        info = upstream.ticker_info(
            instrument.yahoo_symbol,
            max_age_seconds=upstream.SNAPSHOT_CACHE_SECONDS,
        )
    except Exception as exc:
        raise upstream_error("Yahoo Finance snapshot lookup failed") from exc
    if not quote_is_supported(info, instrument.market) or not quote_matches_instrument(
        info, instrument
    ):
        raise not_found(
            "snapshot_not_found",
            f"snapshot not found: {instrument.instrument_id}",
        )
    price, quote_time = _regular_price_and_time(info)
    if price is None:
        raise not_found(
            "snapshot_not_found",
            f"snapshot not found: {instrument.instrument_id}",
        )
    observed_at = format_rfc3339(datetime.now(timezone.utc))
    previous_close = finite_float(
        first_value(info, "regularMarketPreviousClose", "previousClose")
    )
    last_close = finite_float(
        first_value(info, "previousClose", "regularMarketPreviousClose")
    )
    return SnapshotResponse(
        market=instrument.market,
        symbol=instrument.symbol,
        instrument_id=instrument.instrument_id,
        price=price,
        bid=finite_float(info.get("bid")),
        ask=finite_float(info.get("ask")),
        open_price=finite_float(info.get("regularMarketOpen")),
        high_price=finite_float(info.get("regularMarketDayHigh")),
        low_price=finite_float(info.get("regularMarketDayLow")),
        previous_close_price=previous_close,
        last_close_price=last_close,
        regular_quote=_quote_block(info, "regular"),
        pre_market_quote=_quote_block(info, "pre_market"),
        after_market_quote=_quote_block(info, "after_market"),
        volume=non_negative_int(info.get("regularMarketVolume")),
        turnover=finite_float(info.get("regularMarketTurnover")),
        quote_at=timestamp_as_rfc3339(quote_time),
        observed_at=observed_at,
        delayed=True,
        delay_minutes=15,
        currency=clean_text(info.get("currency")),
        exchange=normalized_exchange(info),
        source="yfinance",
    )


def _regular_price_and_time(info: Mapping[str, Any]) -> tuple[float | None, Any]:
    candidates = (
        ("regularMarketPrice", "regularMarketTime"),
        ("currentPrice", "regularMarketTime"),
    )
    for price_key, time_key in candidates:
        price = finite_float(info.get(price_key))
        if price is not None:
            return price, info.get(time_key)
    return None, None


def _quote_block(info: Mapping[str, Any], session: str) -> SnapshotQuote | None:
    prefix = {
        "regular": "regularMarket",
        "pre_market": "preMarket",
        "after_market": "postMarket",
    }[session]
    price = finite_float(info.get(f"{prefix}Price"))
    if price is None and session == "regular":
        price = finite_float(info.get("currentPrice"))
    if price is None:
        return None
    return SnapshotQuote(
        price=price,
        high_price=finite_float(
            first_value(info, f"{prefix}DayHigh", "regularMarketDayHigh")
        ),
        low_price=finite_float(
            first_value(info, f"{prefix}DayLow", "regularMarketDayLow")
        ),
        volume=non_negative_int(info.get(f"{prefix}Volume")),
        turnover=finite_float(info.get(f"{prefix}Turnover")),
        change_value=finite_float(info.get(f"{prefix}Change")),
        change_rate=finite_float(info.get(f"{prefix}ChangePercent")),
        quote_at=timestamp_as_rfc3339(
            info.get(
                "regularMarketTime"
                if session == "regular"
                else f"{prefix}Time"
            )
        ),
    )
