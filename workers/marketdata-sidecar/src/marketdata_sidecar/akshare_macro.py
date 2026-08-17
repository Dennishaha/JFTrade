"""AKShare macro indicator catalog and history endpoints.

The catalog is a curated static table (``akshare_macro_catalog``); history
fetches go through the akshare pool and are cached for one hour per
indicator.  Values arrive numeric from the upstream frames, so ``value`` is
emitted as a plain JSON number with ``predict_value``/``previous_value``
filled when the source frame carries them.
"""

from __future__ import annotations

import re
from typing import Any

from . import akshare_upstream
from .akshare_macro_catalog import INDICATORS, category_order, indicator_by_id
from .akshare_provider_conversion import _frame_rows, _optional_decimal, _row_value
from .errors import not_found
from .models import (
    MacroCategory,
    MacroHistoryEntry,
    MacroHistoryResponse,
    MacroIndicatorInfo,
    MacroIndicatorsResponse,
)
from .upstream import _TickerInfoCache

MACRO_HISTORY_CACHE_SECONDS = 3600

_history_cache = _TickerInfoCache()


def indicators() -> MacroIndicatorsResponse:
    # The catalog is static; assembly is cheap enough to serve per request.
    categories: list[MacroCategory] = []
    for category in category_order():
        specs = [spec for spec in INDICATORS if spec.category == category]
        categories.append(
            MacroCategory(
                category_name=category,
                indicators=[
                    MacroIndicatorInfo(
                        indicator_id=spec.indicator_id,
                        name=spec.name,
                        region=spec.region,
                        unit=spec.unit,
                        unit_type=spec.unit_type,
                        frequency=spec.frequency,
                    )
                    for spec in specs
                ],
            )
        )
    return MacroIndicatorsResponse(categories=categories)


def indicator_history(indicator_id: str, limit: int) -> MacroHistoryResponse:
    spec = indicator_by_id(indicator_id)
    if spec is None:
        raise not_found("not_found", f"unknown macro indicator: {indicator_id}")
    rows = _history_cache.get_or_fetch(
        indicator_id,
        MACRO_HISTORY_CACHE_SECONDS,
        lambda: {"rows": _fetch_rows(spec)},
    )["rows"]
    entries = sorted(rows, key=lambda row: row["data_time"], reverse=True)[:limit]
    return MacroHistoryResponse(
        indicator_id=spec.indicator_id,
        entries=[
            MacroHistoryEntry(
                data_time=row["data_time"],
                value=row["value"],
                predict_value=row["predict_value"],
                previous_value=row["previous_value"],
                unit=spec.unit,
                unit_type=spec.unit_type,
            )
            for row in entries
        ],
    )


def _fetch_rows(spec) -> list[dict[str, Any]]:
    frame = akshare_upstream.call(spec.function)
    rows: list[dict[str, Any]] = []
    for raw in _frame_rows(frame):
        data_time = _data_time(_row_value(raw, spec.date_column))
        if data_time is None:
            continue
        rows.append(
            {
                "data_time": data_time,
                "value": _float(raw, spec.value_column),
                "predict_value": _float(raw, spec.predict_column)
                if spec.predict_column
                else None,
                "previous_value": _float(raw, spec.previous_column)
                if spec.previous_column
                else None,
            }
        )
    return rows


def _data_time(value: Any) -> str | None:
    if value is None:
        return None
    if hasattr(value, "isoformat") and not isinstance(value, str):
        try:
            return value.isoformat()[:7]
        except (TypeError, ValueError):
            return None
    text = str(value).strip()
    match = re.search(r"(\d{4})\D?(\d{1,2})", text)
    if match is None:
        return None
    return f"{match.group(1)}-{int(match.group(2)):02d}"


def _float(row: Any, column: str) -> float | None:
    value = _optional_decimal(row, column)
    return float(value) if value is not None else None
