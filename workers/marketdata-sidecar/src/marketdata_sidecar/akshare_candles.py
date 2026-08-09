"""AKShare candle loading, retention, and aggregation entry points."""

from __future__ import annotations

from datetime import datetime, timedelta

from .akshare_candle_frames import (
    UTC,
    CandleInstrument,
    _aggregate_candles,
    _convert_candle_frame,
    _fetch_candle_frame,
    _uses_five_day_candle_source,
)
from .akshare_models import AKCandle, AKCandlesResponse
from .akshare_provider_conversion import _utc_now
from .errors import invalid_request, not_found

ALL_PERIODS = ("1m", "5m", "15m", "30m", "1h", "1d", "1w", "1mo")


def candles(
    instrument: CandleInstrument,
    *,
    period: str,
    limit: int,
    from_time: datetime | None,
    to_time: datetime | None,
    before_time: datetime | None = None,
) -> AKCandlesResponse:
    normalized_period = period.strip().lower()
    validate_candle_query(normalized_period, from_time, to_time)
    if normalized_period not in instrument.supported_periods:
        raise invalid_request(
            "unsupported_period",
            f"unsupported candle period for {instrument.instrument_id}: {period}",
        )
    validate_candle_retention(
        instrument.market,
        normalized_period,
        from_time,
        to_time,
    )
    if before_time is not None and (from_time is not None or to_time is not None):
        raise invalid_request("invalid_time_range", "before cannot be combined with from or to")

    if from_time is not None or to_time is not None:
        converted, source = _load_candle_window(
            instrument,
            normalized_period,
            from_time,
            to_time,
            None,
        )
        converted = converted[-limit:]
        if not converted:
            raise not_found(
                "candles_not_found",
                f"candles not found: {instrument.instrument_id}",
            )
        return _candle_response(instrument, normalized_period, converted, source, False)

    converted, source, has_more = _load_candle_page(
        instrument,
        normalized_period,
        limit,
        before_time,
    )
    if not converted and before_time is None:
        raise not_found(
            "candles_not_found",
            f"candles not found: {instrument.instrument_id}",
        )

    return _candle_response(instrument, normalized_period, converted, source, has_more)


def _candle_response(
    instrument: CandleInstrument,
    period: str,
    candles: list[AKCandle],
    source: str,
    has_more: bool,
) -> AKCandlesResponse:
    return AKCandlesResponse(
        market=instrument.market,
        symbol=instrument.symbol,
        instrument_id=instrument.instrument_id,
        period=period,
        candles=candles,
        total_returned=len(candles),
        has_more=has_more,
        next_before=candles[0].at if has_more else None,
        source=source,
    )


def _load_candle_page(
    instrument: CandleInstrument,
    period: str,
    limit: int,
    before_time: datetime | None,
) -> tuple[list[AKCandle], str, bool]:
    now = _utc_now()
    end_time = before_time or now
    lower_bound = _candle_history_lower_bound(instrument.market, period, now)
    if before_time is not None and before_time <= lower_bound:
        return [], "akshare:eastmoney", False

    start_time = max(lower_bound, end_time - _candle_page_lookback(period, limit))
    converted, source = _load_candle_window(
        instrument,
        period,
        start_time,
        end_time,
        before_time,
    )
    if len(converted) <= limit and start_time > lower_bound:
        converted, source = _load_candle_window(
            instrument,
            period,
            lower_bound,
            end_time,
            before_time,
        )
    has_more = len(converted) > limit
    if has_more:
        converted = converted[-limit:]
    return converted, source, has_more


def _load_candle_window(
    instrument: CandleInstrument,
    period: str,
    from_time: datetime | None,
    to_time: datetime | None,
    before_time: datetime | None,
) -> tuple[list[AKCandle], str]:
    frame, fetched_period, source, volume_multiplier = _fetch_candle_frame(
        instrument,
        period,
        from_time,
        to_time,
    )
    converted = _convert_candle_frame(
        frame,
        instrument=instrument,
        from_time=from_time,
        to_time=to_time,
        before_time=before_time,
        volume_multiplier=volume_multiplier,
    )
    if fetched_period != period:
        converted = _aggregate_candles(converted, period, instrument.timezone)
        if before_time is not None:
            converted = [
                candle
                for candle in converted
                if datetime.fromisoformat(candle.at.replace("Z", "+00:00")) < before_time
            ]
    return converted, source


def _candle_history_lower_bound(market: str, period: str, now: datetime) -> datetime:
    if _uses_five_day_candle_source(market, period):
        return now - timedelta(days=5)
    return datetime(1900, 1, 1, tzinfo=UTC)


def _candle_page_lookback(period: str, limit: int) -> timedelta:
    interval = {
        "1m": timedelta(minutes=1),
        "5m": timedelta(minutes=5),
        "15m": timedelta(minutes=15),
        "30m": timedelta(minutes=30),
        "1h": timedelta(hours=1),
        "1d": timedelta(days=1),
        "1w": timedelta(days=7),
        "1mo": timedelta(days=31),
    }[period]
    return max(interval * (limit + 1) * 2, timedelta(days=7))


def validate_candle_query(
    period: str,
    from_time: datetime | None,
    to_time: datetime | None,
) -> None:
    if period.strip().lower() not in ALL_PERIODS:
        raise invalid_request("unsupported_period", f"unsupported candle period: {period}")
    if from_time is not None and to_time is not None and from_time > to_time:
        raise invalid_request("invalid_time_range", "from must be earlier than or equal to to")


def validate_candle_retention(
    market: str,
    period: str,
    from_time: datetime | None,
    to_time: datetime | None,
) -> None:
    if not _uses_five_day_candle_source(market, period):
        return
    cutoff = _utc_now() - timedelta(days=5)
    if any(bound is not None and bound < cutoff for bound in (from_time, to_time)):
        raise invalid_request(
            "UNSUPPORTED_RANGE",
            "requested intraday data is only available for the last 5 days",
        )
