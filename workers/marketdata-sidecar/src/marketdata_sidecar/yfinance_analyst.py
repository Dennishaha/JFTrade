"""Yahoo Finance analyst ratings and price targets for US/HK instruments.

The numeric rating is the count-weighted average of the current-period
recommendation trend using the bucket scores below (higher = more bullish):

    strongBuy -> 5, buy -> 4, hold -> 3, sell -> 2, strongSell -> 1
"""

from __future__ import annotations

from typing import Any, Mapping

from . import upstream
from .conversion import finite_float
from .models import (
    AnalystDistribution,
    AnalystResponse,
    AnalystTargetPrice,
)
from .research_common import research_not_found, yfinance_instrument

# yfinance recommendationTrend rows keyed by bucket -> (score, wire key).
_RATING_BUCKETS = (
    ("strongBuy", 5, "strong_buy"),
    ("buy", 4, "buy"),
    ("hold", 3, "hold"),
    ("sell", 2, "underperform"),
    ("strongSell", 1, "sell"),
)


def analyst(market: str, symbol: str) -> AnalystResponse:
    instrument = yfinance_instrument(market, symbol)
    data = upstream.ticker_analyst(instrument.yahoo_symbol)
    current = _current_trend(data.get("trend") or [])
    targets = data.get("targets") or {}
    if current is None and not targets:
        raise research_not_found("analyst", instrument.instrument_id)
    return AnalystResponse(
        instrument_id=instrument.instrument_id,
        rating=_weighted_rating(current) if current is not None else None,
        analyst_count=_analyst_count(current) if current is not None else None,
        target_price=_target_price(targets),
        distribution=_distribution(current) if current is not None else None,
        update_time=None,
    )


def _current_trend(trend: list[Mapping[str, Any]]) -> Mapping[str, Any] | None:
    for row in trend:
        if row.get("period") == "0m":
            return row
    return trend[0] if trend else None


def _bucket_counts(row: Mapping[str, Any]) -> dict[str, float]:
    return {
        wire_key: count
        for key, _score, wire_key in _RATING_BUCKETS
        if (count := finite_float(row.get(key))) is not None
    }


def _weighted_rating(row: Mapping[str, Any]) -> float | None:
    total = 0.0
    weighted = 0.0
    for key, score, wire_key in _RATING_BUCKETS:
        count = finite_float(row.get(key))
        if count is None:
            continue
        total += count
        weighted += count * score
    return weighted / total if total > 0 else None


def _analyst_count(row: Mapping[str, Any]) -> int | None:
    counts = _bucket_counts(row)
    return int(sum(counts.values())) if counts else None


def _distribution(row: Mapping[str, Any]) -> AnalystDistribution | None:
    counts = _bucket_counts(row)
    total = sum(counts.values())
    if total <= 0:
        return None
    percentages = {
        wire_key: counts.get(wire_key, 0.0) / total * 100
        for _key, _score, wire_key in _RATING_BUCKETS
    }
    return AnalystDistribution(**percentages)


def _target_price(targets: Mapping[str, Any]) -> AnalystTargetPrice | None:
    if not targets:
        return None
    target = AnalystTargetPrice(
        lowest=targets.get("low"),
        average=targets.get("mean"),
        highest=targets.get("high"),
    )
    if target.lowest is None and target.average is None and target.highest is None:
        return None
    return target
