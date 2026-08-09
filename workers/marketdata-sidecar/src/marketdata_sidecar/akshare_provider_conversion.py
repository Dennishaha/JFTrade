"""Low-level AKShare row, decimal, and time conversion helpers."""

from __future__ import annotations

import re
from datetime import datetime, timezone
from decimal import Decimal, InvalidOperation
from typing import Any, Iterable, Mapping, Protocol
from zoneinfo import ZoneInfo

from .conversion import format_rfc3339, timestamp_as_utc
from .errors import SidecarError, not_found

UTC = timezone.utc


class InstrumentIdentity(Protocol):
    upstream_symbol: str


def _frame_rows(frame: Any) -> Iterable[dict[str, Any]]:
    return (row for _index, row in _frame_rows_with_index(frame))


def _frame_is_empty(frame: Any) -> bool:
    if frame is None:
        return True
    if isinstance(frame, (list, tuple)):
        return not frame
    return bool(getattr(frame, "empty", True))


def _frame_rows_with_index(frame: Any) -> Iterable[tuple[Any, dict[str, Any]]]:
    if _frame_is_empty(frame):
        return []
    if isinstance(frame, (list, tuple)):
        return (
            (index, dict(row))
            for index, row in enumerate(frame)
            if isinstance(row, Mapping)
        )
    return ((index, dict(row)) for index, row in frame.iterrows())


def _row_value(row: Mapping[str, Any], *names: str) -> Any:
    normalized = {_column_key(key): value for key, value in row.items()}
    for name in names:
        value = normalized.get(_column_key(name))
        if value is not None:
            return value
    return None


def _column_key(value: Any) -> str:
    return str(value).strip().lower().replace(" ", "").replace("_", "")


def _optional_decimal(
    row: Mapping[str, Any],
    *names: str,
    minimum: Decimal | None = None,
) -> Decimal | None:
    value = _row_value(row, *names)
    if value is None or isinstance(value, bool):
        return None
    try:
        result = Decimal(str(value).strip())
    except (InvalidOperation, ValueError):
        return None
    if not result.is_finite() or (minimum is not None and result < minimum):
        return None
    return result


def _required_decimal(row: Mapping[str, Any], *names: str) -> Decimal:
    value = _optional_decimal(row, *names, minimum=Decimal(0))
    if value is None or value <= 0:
        raise not_found("snapshot_not_found", "snapshot does not contain a valid price")
    return value


def _optional_price(row: Mapping[str, Any], *names: str) -> Decimal | None:
    value = _optional_decimal(row, *names, minimum=Decimal(0))
    return value if value is not None and value > 0 else None


def _decimal_text(value: Decimal | None) -> str | None:
    if value is None:
        return None
    rendered = format(value, "f")
    if "." in rendered:
        rendered = rendered.rstrip("0").rstrip(".")
    return rendered or "0"


def _row_timestamp(row: Mapping[str, Any], timezone_name: str) -> str | None:
    value = _row_value(
        row,
        "更新时间",
        "最新时间",
        "最新行情时间",
        "时间",
        "timestamp",
    )
    if isinstance(value, str) and not re.search(r"(?:T|\s)\d{1,2}:\d{2}", value.strip()):
        return None
    parsed = timestamp_as_utc(value, timezone_name)
    return format_rfc3339(parsed) if parsed is not None else None


def _intraday_bound(value: datetime, timezone_name: str) -> str:
    return value.astimezone(ZoneInfo(timezone_name)).strftime("%Y-%m-%d %H:%M:%S")


def _daily_bound(value: datetime, timezone_name: str) -> str:
    return value.astimezone(ZoneInfo(timezone_name)).strftime("%Y%m%d")


def _history_symbol(instrument: InstrumentIdentity) -> str:
    """Strip the private CN series key only at the AKShare call boundary."""
    _series, separator, symbol = instrument.upstream_symbol.partition(":")
    return symbol if separator else instrument.upstream_symbol


def _validate_snapshot_ohlc(
    price: Decimal,
    open_price: Decimal | None,
    high: Decimal | None,
    low: Decimal | None,
) -> None:
    if high is not None and low is not None and high < low:
        raise SidecarError(
            502,
            "AKSHARE_SCHEMA_ERROR",
            "AKShare snapshot contains inconsistent OHLC",
        )
    comparable = [price]
    if open_price is not None:
        comparable.append(open_price)
    if high is not None and high < max(comparable):
        raise SidecarError(
            502,
            "AKSHARE_SCHEMA_ERROR",
            "AKShare snapshot contains inconsistent OHLC",
        )
    if low is not None and low > min(comparable):
        raise SidecarError(
            502,
            "AKSHARE_SCHEMA_ERROR",
            "AKShare snapshot contains inconsistent OHLC",
        )


def _utc_now() -> datetime:
    return datetime.now(UTC)
