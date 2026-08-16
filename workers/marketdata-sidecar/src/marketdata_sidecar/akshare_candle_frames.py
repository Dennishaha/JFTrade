"""AKShare candle frame loading, transport fallback, and aggregation."""

from __future__ import annotations

from datetime import datetime, timedelta, timezone
from decimal import Decimal
from typing import Any, Protocol
from zoneinfo import ZoneInfo

from . import akshare_upstream
from .akshare_models import AKCandle
from .akshare_provider_conversion import (
    _daily_bound,
    _decimal_text,
    _frame_is_empty,
    _frame_rows_with_index,
    _history_symbol,
    _intraday_bound,
    _optional_decimal,
    _row_value,
    _utc_now,
)
from .conversion import format_rfc3339, timestamp_as_utc
from .errors import SidecarError

UTC = timezone.utc
INTRADAY_PERIODS = frozenset({"1m", "5m", "15m", "30m", "1h"})
AKSHARE_ADJUST = {"none": "", "forward": "qfq", "backward": "hfq"}


def _require_unadjusted(adjustment: str, instrument: CandleInstrument) -> None:
    """Reject adjustment modes the instrument's candle source cannot serve."""
    if adjustment != "none":
        raise SidecarError(
            400,
            "UNSUPPORTED_RANGE",
            f"price adjustment {adjustment} is not supported for {instrument.instrument_id} candles",
        )


class CandleInstrument(Protocol):
    market: str
    symbol: str
    instrument_id: str
    upstream_symbol: str
    kind: str
    supported_periods: tuple[str, ...]

    @property
    def timezone(self) -> str: ...

    @property
    def volume_multiplier(self) -> Decimal: ...


def _uses_five_day_candle_source(market: str, period: str) -> bool:
    normalized_period = period.strip().lower()
    return normalized_period == "1m" or (
        market.strip().upper() == "US" and normalized_period in INTRADAY_PERIODS
    )


def _fetch_candle_frame(
    instrument: CandleInstrument,
    period: str,
    from_time: datetime | None,
    to_time: datetime | None,
    adjustment: str = "none",
) -> tuple[Any, str, str, Decimal]:
    start = from_time or (_utc_now() - timedelta(days=5 if period in INTRADAY_PERIODS else 365 * 5))
    end = to_time or _utc_now()
    if instrument.kind == "index" and instrument.market in {"US", "HK"}:
        # Global index history has no adjustment-capable daily function.
        _require_unadjusted(adjustment, instrument)
        frame, source, volume_multiplier = _global_index_daily_frame(instrument)
        return frame, "1d", source, volume_multiplier
    if period in INTRADAY_PERIODS:
        # Minute sources (Sina/Eastmoney raw and *_hist_min_em) serve
        # unadjusted prices only.
        _require_unadjusted(adjustment, instrument)
        fetch_period = "1m" if instrument.market in {"US", "HK"} else period
        minute = "60" if fetch_period == "1h" else fetch_period.removesuffix("m")
        if instrument.market in {"SH", "SZ"}:
            fallback_name, fallback_kwargs = _intraday_call(instrument, minute, start, end)
            frame, source, volume_multiplier = _preferred_history_call(
                "stock_zh_a_minute",
                {
                    "symbol": _sina_cn_symbol(instrument),
                    "period": minute,
                    "adjust": "",
                },
                fallback_name,
                fallback_kwargs,
                fallback_volume_multiplier=instrument.volume_multiplier,
            )
            return frame, fetch_period, source, volume_multiplier
        if instrument.market == "US":
            return (
                akshare_upstream.us_minute_rows(instrument.symbol),
                fetch_period,
                "akshare:sina",
                Decimal(1),
            )
        if instrument.market == "HK":
            return (
                akshare_upstream.hk_minute_rows(instrument.symbol),
                fetch_period,
                "akshare:eastmoney",
                Decimal(1),
            )
        function_name, kwargs = _intraday_call(instrument, minute, start, end)
        return (
            akshare_upstream.call(function_name, **kwargs),
            fetch_period,
            "akshare:eastmoney",
            instrument.volume_multiplier,
        )
    frame, source, volume_multiplier = _daily_frame(instrument, start, end, adjustment)
    return frame, "1d", source, volume_multiplier


def _daily_frame(
    instrument: CandleInstrument,
    start: datetime,
    end: datetime,
    adjustment: str = "none",
) -> tuple[Any, str, Decimal]:
    adjust = AKSHARE_ADJUST[adjustment]
    fallback_name, fallback_kwargs = _daily_call(instrument, "1d", start, end, adjust)
    if instrument.kind == "etf":
        if adjust:
            # fund_etf_hist_sina has no adjust parameter; Eastmoney history
            # is the only adjusted ETF daily source.
            frame = akshare_upstream.call(fallback_name, **fallback_kwargs)
            return frame, "akshare:eastmoney", instrument.volume_multiplier
        return _preferred_history_call(
            "fund_etf_hist_sina",
            {"symbol": _sina_cn_symbol(instrument)},
            fallback_name,
            fallback_kwargs,
            fallback_volume_multiplier=instrument.volume_multiplier,
        )
    if instrument.kind == "index":
        # Neither stock_zh_index_daily nor index_zh_a_hist accepts adjust.
        _require_unadjusted(adjustment, instrument)
        return _preferred_history_call(
            "stock_zh_index_daily",
            {"symbol": _sina_cn_symbol(instrument)},
            fallback_name,
            fallback_kwargs,
            fallback_volume_multiplier=instrument.volume_multiplier,
        )
    if instrument.market == "US":
        return _preferred_history_call(
            "stock_us_daily",
            {"symbol": instrument.symbol, "adjust": adjust},
            fallback_name,
            fallback_kwargs,
        )
    if instrument.market == "HK":
        return _preferred_history_call(
            "stock_hk_daily",
            {"symbol": instrument.symbol, "adjust": adjust},
            fallback_name,
            fallback_kwargs,
        )
    return _preferred_history_call(
        "stock_zh_a_daily",
        {
            "symbol": _sina_cn_symbol(instrument),
            "start_date": _daily_bound(start, instrument.timezone),
            "end_date": _daily_bound(end, instrument.timezone),
            "adjust": adjust,
        },
        fallback_name,
        fallback_kwargs,
        fallback_volume_multiplier=instrument.volume_multiplier,
    )


def _global_index_daily_frame(
    instrument: CandleInstrument,
) -> tuple[Any, str, Decimal]:
    if instrument.market == "US":
        fallback_name = "index_global_hist_em"
        fallback_kwargs = {"symbol": instrument.upstream_symbol}
        return _preferred_history_call(
            "stock_us_daily",
            {"symbol": _sina_us_index_symbol(instrument.symbol), "adjust": ""},
            fallback_name,
            fallback_kwargs,
        )
    return _preferred_history_call(
        "stock_hk_index_daily_sina",
        {"symbol": instrument.upstream_symbol},
        "stock_hk_index_daily_em",
        {"symbol": instrument.upstream_symbol},
    )


def _preferred_history_call(
    function_name: str,
    kwargs: dict[str, Any],
    fallback_name: str,
    fallback_kwargs: dict[str, Any],
    *,
    fallback_volume_multiplier: Decimal = Decimal(1),
) -> tuple[Any, str, Decimal]:
    """Use another AKShare transport when Eastmoney history is unreachable."""
    try:
        frame = akshare_upstream.call(function_name, **kwargs)
        if not _frame_is_empty(frame):
            return frame, "akshare:sina", Decimal(1)
    except SidecarError as exc:
        if exc.status_code == 503:
            raise
    except Exception:
        akshare_upstream.ensure_request_active()
    frame = akshare_upstream.call(fallback_name, **fallback_kwargs)
    return frame, "akshare:eastmoney", fallback_volume_multiplier


def _sina_cn_symbol(instrument: CandleInstrument) -> str:
    prefix = "sh" if instrument.market == "SH" else "sz"
    return f"{prefix}{instrument.symbol}"


def _sina_us_index_symbol(symbol: str) -> str:
    return {".DJI": ".DJI", ".SPX": ".INX", ".NDX": ".NDX"}[symbol]


def _intraday_call(
    instrument: CandleInstrument,
    minute: str,
    start: datetime,
    end: datetime,
) -> tuple[str, dict[str, Any]]:
    if instrument.kind == "etf":
        return "fund_etf_hist_min_em", {
            "symbol": _history_symbol(instrument),
            "period": minute,
            "start_date": _intraday_bound(start, instrument.timezone),
            "end_date": _intraday_bound(end, instrument.timezone),
            "adjust": "",
        }
    if instrument.kind == "index":
        return "index_zh_a_hist_min_em", {
            "symbol": _history_symbol(instrument),
            "period": minute,
            "start_date": _intraday_bound(start, instrument.timezone),
            "end_date": _intraday_bound(end, instrument.timezone),
        }
    if instrument.market == "US":
        return "stock_us_hist_min_em", {
            "symbol": _history_symbol(instrument),
            "start_date": _intraday_bound(start, instrument.timezone),
            "end_date": _intraday_bound(end, instrument.timezone),
        }
    if instrument.market == "HK":
        return "stock_hk_hist_min_em", {
            "symbol": _history_symbol(instrument),
            "period": minute,
            "start_date": _intraday_bound(start, instrument.timezone),
            "end_date": _intraday_bound(end, instrument.timezone),
            "adjust": "",
        }
    return "stock_zh_a_hist_min_em", {
        "symbol": _history_symbol(instrument),
        "period": minute,
        "start_date": _intraday_bound(start, instrument.timezone),
        "end_date": _intraday_bound(end, instrument.timezone),
        "adjust": "",
    }


def _daily_call(
    instrument: CandleInstrument,
    period: str,
    start: datetime,
    end: datetime,
    adjust: str = "",
) -> tuple[str, dict[str, Any]]:
    upstream_period = {"1d": "daily", "1w": "weekly", "1mo": "monthly"}[period]
    kwargs = {
        "symbol": _history_symbol(instrument),
        "period": upstream_period,
        "start_date": _daily_bound(start, instrument.timezone),
        "end_date": _daily_bound(end, instrument.timezone),
    }
    if instrument.kind == "etf":
        kwargs["adjust"] = adjust
        return "fund_etf_hist_em", kwargs
    if instrument.kind == "index":
        return "index_zh_a_hist", kwargs
    kwargs["adjust"] = adjust
    if instrument.market == "US":
        return "stock_us_hist", kwargs
    if instrument.market == "HK":
        return "stock_hk_hist", kwargs
    return "stock_zh_a_hist", kwargs


def _convert_candle_frame(
    frame: Any,
    *,
    instrument: CandleInstrument,
    from_time: datetime | None,
    to_time: datetime | None,
    before_time: datetime | None = None,
    volume_multiplier: Decimal,
) -> list[AKCandle]:
    result: list[AKCandle] = []
    for index, row in _frame_rows_with_index(frame):
        at_value = _row_value(row, "时间", "日期", "datetime", "date", "time", "day")
        at = timestamp_as_utc(at_value if at_value is not None else index, instrument.timezone)
        if at is None or (from_time is not None and at < from_time) or (to_time is not None and at > to_time):
            continue
        if before_time is not None and at >= before_time:
            continue
        open_price = _optional_decimal(row, "开盘", "开盘价", "open", minimum=Decimal(0))
        high = _optional_decimal(row, "最高", "最高价", "high", minimum=Decimal(0))
        low = _optional_decimal(row, "最低", "最低价", "low", minimum=Decimal(0))
        close = _optional_decimal(
            row,
            "收盘",
            "收盘价",
            "最新价",
            "latest",
            "close",
            minimum=Decimal(0),
        )
        if None in {open_price, high, low, close}:
            continue
        assert open_price is not None and high is not None and low is not None and close is not None
        if min(open_price, high, low, close) <= 0:
            continue
        if high < max(open_price, low, close) or low > min(open_price, high, close):
            continue
        volume = _optional_decimal(row, "成交量", "volume", minimum=Decimal(0))
        if volume is not None:
            volume *= volume_multiplier
        result.append(
            AKCandle(
                at=format_rfc3339(at),
                open=_decimal_text(open_price) or "0",
                high=_decimal_text(high) or "0",
                low=_decimal_text(low) or "0",
                close=_decimal_text(close) or "0",
                volume=_decimal_text(volume),
            )
        )
    result.sort(key=lambda item: item.at)
    return result


def _aggregate_candles(
    candles: list[AKCandle],
    period: str,
    timezone_name: str,
) -> list[AKCandle]:
    groups: dict[datetime, list[AKCandle]] = {}
    for candle in candles:
        at = datetime.fromisoformat(candle.at.replace("Z", "+00:00"))
        local = at.astimezone(ZoneInfo(timezone_name))
        if period in {"5m", "15m", "30m", "1h"}:
            minutes = 60 if period == "1h" else int(period.removesuffix("m"))
            bucket = local.replace(minute=(local.minute // minutes) * minutes, second=0, microsecond=0)
        elif period == "1w":
            bucket = (local - timedelta(days=local.weekday())).replace(hour=0, minute=0, second=0, microsecond=0)
        elif period == "1mo":
            bucket = local.replace(day=1, hour=0, minute=0, second=0, microsecond=0)
        else:
            bucket = local
        groups.setdefault(bucket.astimezone(UTC), []).append(candle)
    result: list[AKCandle] = []
    for bucket, items in sorted(groups.items()):
        volumes = [Decimal(item.volume) for item in items if item.volume is not None]
        result.append(
            AKCandle(
                at=format_rfc3339(bucket),
                open=items[0].open,
                high=_decimal_text(max(Decimal(item.high) for item in items)) or "0",
                low=_decimal_text(min(Decimal(item.low) for item in items)) or "0",
                close=items[-1].close,
                volume=_decimal_text(sum(volumes, Decimal(0))) if volumes else None,
            )
        )
    return result
