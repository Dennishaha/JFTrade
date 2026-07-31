"""Conversion helpers that keep yfinance/pandas values JSON-safe."""

from __future__ import annotations

import math
from datetime import datetime, timezone
from typing import Any, Mapping
from zoneinfo import ZoneInfo

from .errors import invalid_request
from .models import Candle

UTC = timezone.utc
US_EASTERN = "America/New_York"
MARKET_SESSION_WINDOWS: dict[str, tuple[tuple[int, int], ...]] = {
    "US": ((570, 960),),
    "HK": ((570, 720), (780, 960)),
    "SH": ((570, 690), (780, 900)),
    "SZ": ((570, 690), (780, 900)),
}


def clean_text(value: Any) -> str | None:
    if value is None:
        return None
    if isinstance(value, (int, float)) and not isinstance(value, bool):
        if finite_float(value) is None:
            return None
    text = str(value).strip()
    if text.lower() in {"nan", "nat", "<na>", "none", "null"}:
        return None
    return text or None


def finite_float(value: Any) -> float | None:
    if value is None or isinstance(value, bool):
        return None
    try:
        result = float(value)
    except (TypeError, ValueError, OverflowError):
        return None
    return result if math.isfinite(result) else None


def non_negative_int(value: Any) -> int | None:
    number = finite_float(value)
    if number is None or number < 0:
        return None
    return int(number)


def first_value(values: Mapping[str, Any], *keys: str) -> Any:
    for key in keys:
        value = values.get(key)
        if value is not None:
            return value
    return None


def parse_rfc3339_utc(value: str | None, field_name: str) -> datetime | None:
    if value is None:
        return None
    trimmed = value.strip()
    if not trimmed:
        return None
    try:
        parsed = datetime.fromisoformat(trimmed.replace("Z", "+00:00"))
    except ValueError as exc:
        raise invalid_request(
            "invalid_time",
            f"{field_name} must be an RFC3339 timestamp with timezone",
        ) from exc
    if parsed.tzinfo is None:
        raise invalid_request(
            "invalid_time",
            f"{field_name} must be an RFC3339 timestamp with timezone",
        )
    return parsed.astimezone(UTC)


def timestamp_as_utc(value: Any, assumed_timezone: str = US_EASTERN) -> datetime | None:
    if value is None:
        return None
    if hasattr(value, "to_pydatetime"):
        try:
            value = value.to_pydatetime()
        except (TypeError, ValueError, OverflowError):
            return None
    if isinstance(value, datetime):
        parsed = value
    elif isinstance(value, (int, float)) and not isinstance(value, bool):
        number = finite_float(value)
        if number is None:
            return None
        try:
            parsed = datetime.fromtimestamp(number, tz=UTC)
        except (OSError, OverflowError, ValueError):
            return None
    elif isinstance(value, str):
        try:
            parsed = datetime.fromisoformat(value.strip().replace("Z", "+00:00"))
        except ValueError:
            return None
    else:
        return None
    if parsed.tzinfo is None:
        try:
            parsed = parsed.replace(tzinfo=ZoneInfo(assumed_timezone))
        except (KeyError, ValueError):
            return None
    return parsed.astimezone(UTC)


def format_rfc3339(value: datetime) -> str:
    return value.astimezone(UTC).isoformat(timespec="seconds").replace("+00:00", "Z")


def timestamp_as_rfc3339(value: Any, assumed_timezone: str = US_EASTERN) -> str | None:
    parsed = timestamp_as_utc(value, assumed_timezone)
    return format_rfc3339(parsed) if parsed is not None else None


def session_for_timestamp(
    value: datetime,
    period: str,
    *,
    market: str = "US",
    exchange_timezone: str = US_EASTERN,
) -> str:
    if period in {"1d", "1w", "1mo"}:
        return "regular"
    canonical_market = market.strip().upper() or "US"
    fallback_timezone = exchange_timezone.strip() or US_EASTERN
    timezone_name = {
        "HK": "Asia/Hong_Kong",
        "SH": "Asia/Shanghai",
        "SZ": "Asia/Shanghai",
    }.get(canonical_market, fallback_timezone)
    local = value.astimezone(ZoneInfo(timezone_name))
    minute = local.hour * 60 + local.minute
    windows = MARKET_SESSION_WINDOWS.get(canonical_market, ((570, 960),))
    if any(start <= minute < end for start, end in windows):
        return "regular"
    if canonical_market != "US":
        # Yahoo does not provide a reliable pre/post session for HK/CN. Keep
        # auction and lunch-break bars visible without mislabelling them as
        # US-style after-hours or fabricated overnight data.
        return "closed"
    if 240 <= minute < 570:
        return "pre_market"
    if 960 <= minute < 1200:
        return "after_hours"
    # Yahoo's US extended-hours feed ends with the post-market session. It
    # does not provide a dependable overnight quote stream, so do not leak an
    # Futu-only overnight session into this provider's candle contract.
    return "closed"


def snapshot_session(market_state: Any) -> tuple[str, bool]:
    return snapshot_session_for_market(market_state, market="US")


def snapshot_session_for_market(
    market_state: Any,
    *,
    market: str,
) -> tuple[str, bool]:
    """Classify a Yahoo market state without inventing non-US extended hours."""
    state = (clean_text(market_state) or "").upper()
    canonical_market = market.strip().upper() or "US"
    if state == "REGULAR":
        return "regular", False
    if canonical_market != "US":
        # Yahoo's US-style PRE/POST labels do not establish a reliable
        # extended-hours contract for HK/CN. Their actual auction bars remain
        # available through candles, but a snapshot must not claim a US-style
        # pre-market or after-hours session.
        return "closed", False
    if state == "PRE":
        return "pre_market", True
    if state == "POST":
        return "after_hours", True
    if state in {"PREPRE", "POSTPOST", "CLOSED"}:
        return "closed", False
    if state.startswith("PRE"):
        return "pre_market", True
    if state.startswith("POST"):
        return "after_hours", True
    return "regular", False


def convert_history(
    frame: Any,
    *,
    period: str,
    limit: int,
    from_time: datetime | None = None,
    to_time: datetime | None = None,
    exchange_timezone: str = US_EASTERN,
    market: str = "US",
) -> list[Candle]:
    if frame is None or bool(getattr(frame, "empty", True)):
        return []
    columns = {
        _normalized_column_name(column): column
        for column in getattr(frame, "columns", [])
    }
    required = {"open", "high", "low", "close"}
    if not required.issubset(columns):
        return []

    candles: list[Candle] = []
    for index, row in frame.iterrows():
        at = timestamp_as_utc(index, exchange_timezone)
        if at is None or (from_time is not None and at < from_time):
            continue
        if to_time is not None and at > to_time:
            continue
        prices = [finite_float(row[columns[key]]) for key in ("open", "high", "low", "close")]
        if any(price is None for price in prices):
            continue
        volume_column = columns.get("volume")
        volume = non_negative_int(row[volume_column]) if volume_column is not None else None
        candles.append(
            Candle(
                at=format_rfc3339(at),
                open=prices[0],
                high=prices[1],
                low=prices[2],
                close=prices[3],
                volume=volume or 0,
                session=session_for_timestamp(
                    at,
                    period,
                    market=market,
                    exchange_timezone=exchange_timezone,
                ),
            )
        )
    candles.sort(key=lambda candle: candle.at)
    return candles[-limit:]


def _normalized_column_name(column: Any) -> str:
    if isinstance(column, tuple):
        column = column[0]
    return str(column).strip().lower().replace(" ", "_")
