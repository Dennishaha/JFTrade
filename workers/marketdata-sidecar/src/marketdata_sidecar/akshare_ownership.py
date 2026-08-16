"""AKShare CN top-10 shareholder listings (Eastmoney F10 股东研究).

``stock_gdfx_top_10_em`` requires an explicit 报告期, so the latest period
is probed from recent quarter ends (newest first, up to eight quarters
back).  The resolved period and rows share one cache entry, so a repeat
request never re-probes.
"""

from __future__ import annotations

from datetime import date, datetime, timezone
from typing import Any

from . import akshare_upstream
from .akshare_provider_conversion import _frame_rows, _optional_decimal, _row_value
from .conversion import clean_text
from .models import OwnershipGroup, OwnershipItem, OwnershipResponse
from .research_common import (
    akshare_research_identity,
    require_cn_leaf,
    research_not_found,
)
from .upstream import _TickerInfoCache

OWNERSHIP_CACHE_SECONDS = 3600
_PROBE_QUARTERS = 8
_QUARTER_ENDS = ((3, 31), (6, 30), (9, 30), (12, 31))

_ownership_cache = _TickerInfoCache()


def ownership(market: str, symbol: str) -> OwnershipResponse:
    requested, leaf, code = akshare_research_identity(market, symbol, "ownership")
    require_cn_leaf(leaf, "ownership", market)
    instrument_id = f"{requested}.{code}"
    data = _ownership_cache.get_or_fetch(
        f"{leaf}:{code}",
        OWNERSHIP_CACHE_SECONDS,
        lambda: _probe(leaf, code),
    )
    rows = data.get("rows") or []
    if not rows:
        raise research_not_found("ownership", instrument_id)
    items = [
        OwnershipItem(name=name, holder_pct=pct)
        for row in rows
        if (name := clean_text(_row_value(row, "股东名称", "holder"))) is not None
        for pct in [_pct(row)]
    ]
    if not items:
        raise research_not_found("ownership", instrument_id)
    report_date = data.get("date") or ""
    static_date = (
        f"{report_date[:4]}-{report_date[4:6]}-{report_date[6:]}"
        if len(report_date) == 8
        else None
    )
    return OwnershipResponse(
        instrument_id=instrument_id,
        groups=[
            OwnershipGroup(kind="major_holders", static_date=static_date, items=items)
        ],
    )


def _pct(row: dict[str, Any]) -> float | None:
    value = _optional_decimal(row, "占总股本持股比例", "holder_pct")
    return float(value) if value is not None else None


def _probe(leaf: str, code: str) -> dict[str, Any]:
    symbol = f"{leaf.lower()}{code}"
    for candidate in _candidate_periods():
        rows = [
            dict(row)
            for row in _frame_rows(
                akshare_upstream.call(
                    "stock_gdfx_top_10_em",
                    symbol=symbol,
                    date=candidate,
                )
            )
        ]
        if rows:
            return {"date": candidate, "rows": rows}
    return {"date": None, "rows": []}


def _candidate_periods() -> list[str]:
    today = datetime.now(timezone.utc).date()
    candidates: list[date] = []
    for year in range(today.year, today.year - 3, -1):
        for month, day in reversed(_QUARTER_ENDS):
            end = date(year, month, day)
            if end <= today:
                candidates.append(end)
    return [end.strftime("%Y%m%d") for end in candidates[:_PROBE_QUARTERS]]
