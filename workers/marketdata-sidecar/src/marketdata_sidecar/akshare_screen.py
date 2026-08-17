"""AKShare stock screener: local filter/sort/page over the cached spot catalog.

Like akshare_rankings this reuses the 15s TTL full-market Eastmoney spot
frames from akshare_catalog, so a screen request performs no new upstream
call.  Only stocks (kind == "stock") participate; ETFs and indexes are
excluded.

Factor mapping (Eastmoney spot column names, verified against akshare
1.18.91 stock_hist_em.py):

- simple.price      → 最新价
- simple.change_pct → 涨跌幅 (%)
- simple.volume     → 成交量；SH/SZ 单位是手，输出前 ×100 换成股
  (AKInstrument.volume_multiplier)；HK 东财现货帧成交量单位即股，不换算
- simple.market_cap → 总市值
- simple.pe_ttm     → 市盈率-动态
- simple.pb         → 市净率

US/HK 的 spot 帧由 akshare_spot_clist 直连东财 clist 构建,补齐了
akshare 封装丢弃的 f23(市净率)/f115(PE TTM) 与 HK 总市值,因此六个
因子在全部支持市场(含 HK 的市值)可用。
"""

from __future__ import annotations

from datetime import datetime
from typing import Any
from zoneinfo import ZoneInfo

from .akshare_catalog import AKInstrument, catalog
from .akshare_identity import _normalize_market
from .akshare_provider_conversion import _optional_decimal
from .errors import invalid_request
from .models import (
    ScreenCondition,
    ScreenEntry,
    ScreenRequest,
    ScreenResponse,
    ScreenSort,
)
from .routes.common import MARKET_SPECS

SCREEN_SOURCE = "akshare-screen"

FACTOR_COLUMNS = {
    "simple.price": ("最新价",),
    "simple.change_pct": ("涨跌幅",),
    "simple.volume": ("成交量",),
    "simple.market_cap": ("总市值",),
    "simple.pe_ttm": ("市盈率-动态",),
    "simple.pb": ("市净率",),
}

# basic.* identity factors sort on catalog text fields, not spot columns.
TEXT_SORT_KEYS = {"basic.code", "basic.name"}

SUPPORTED_MARKETS = frozenset({"CN", "SH", "SZ", "HK", "US"})


def screen(request: ScreenRequest) -> ScreenResponse:
    leaves = _screen_markets(request.market)
    conditions = [_translate_condition(condition) for condition in request.conditions]
    sort_key, sort_desc = _translate_sort(request.sorts)

    matched: list[tuple[AKInstrument, dict[str, float]]] = []
    for leaf in leaves:
        for instrument in catalog(leaf):
            if instrument.kind != "stock":
                continue
            values = _instrument_values(instrument)
            if _matches(values, conditions):
                matched.append((instrument, values))
    if sort_key is not None:
        _sort(matched, sort_key, sort_desc)

    total = len(matched)
    page = matched[request.offset : request.offset + request.limit]
    entries = [
        ScreenEntry(
            instrument_id=instrument.instrument_id,
            name=instrument.name,
            symbol=instrument.symbol,
            industry=None,  # 东财 spot 帧无行业列
            quote_currency=MARKET_SPECS[instrument.market].quote_currency,
            values=values,
        )
        for instrument, values in page
    ]
    return ScreenResponse(
        entries=entries,
        total=total,
        has_more=request.offset + len(page) < total,
        next_offset=(
            request.offset + len(page)
            if request.offset + len(page) < total
            else None
        ),
        as_of=datetime.now(
            ZoneInfo(MARKET_SPECS[leaves[0]].timezone)
        ).isoformat(timespec="seconds"),
        source=SCREEN_SOURCE,
    )


def _screen_markets(market: str) -> tuple[str, ...]:
    token = market.strip().upper()
    if token == "CN":
        return ("SH", "SZ")
    if token not in {"SH", "SZ", "HK", "US"}:
        raise invalid_request(
            "unsupported_market",
            "AKShare screen is only available for markets: CN, SH, SZ, HK, US",
        )
    return (_normalize_market(token),)


def _translate_condition(condition: ScreenCondition) -> ScreenCondition:
    if condition.in_values is not None:
        raise invalid_request(
            "unsupported_kind",
            "enumeration (in) conditions are not supported by this catalog",
        )
    if condition.factor_key.strip() not in FACTOR_COLUMNS:
        raise invalid_request(
            "unsupported_kind",
            f"unsupported screen factor: {condition.factor_key}",
        )
    if condition.min is None and condition.max is None:
        raise invalid_request(
            "invalid_request",
            f"screen condition {condition.factor_key} requires min or max",
        )
    if (
        condition.min is not None
        and condition.max is not None
        and condition.min > condition.max
    ):
        raise invalid_request("invalid_request", "condition min must not exceed max")
    return condition


def _translate_sort(sorts: list[ScreenSort]) -> tuple[str | None, bool]:
    if not sorts:
        return None, False
    if len(sorts) > 1:
        raise invalid_request(
            "unsupported_kind",
            "multiple sort keys are not supported by this catalog",
        )
    first = sorts[0]
    key = first.factor_key.strip()
    if key not in FACTOR_COLUMNS and key not in TEXT_SORT_KEYS:
        raise invalid_request(
            "unsupported_kind",
            f"unsupported screen sort factor: {first.factor_key}",
        )
    direction = first.direction.strip().lower()
    if direction not in ("asc", "desc"):
        raise invalid_request(
            "invalid_request",
            f"unsupported screen sort direction: {first.direction}",
        )
    return key, direction == "desc"


def _instrument_values(instrument: AKInstrument) -> dict[str, float]:
    values: dict[str, float] = {}
    for factor_key, columns in FACTOR_COLUMNS.items():
        number = _optional_decimal(instrument.row, *columns)
        if number is None:
            continue
        if factor_key == "simple.volume":
            number *= instrument.volume_multiplier
        values[factor_key] = float(number)
    return values


def _matches(values: dict[str, float], conditions: list[ScreenCondition]) -> bool:
    for condition in conditions:
        value = values.get(condition.factor_key.strip())
        if value is None:
            return False  # 缺失因子值的行不通过任何区间条件
        if condition.min is not None and value < condition.min:
            return False
        if condition.max is not None and value > condition.max:
            return False
    return True


def _sort(
    matched: list[tuple[AKInstrument, dict[str, float]]],
    sort_key: str,
    sort_desc: bool,
) -> None:
    if sort_key in TEXT_SORT_KEYS:
        _sort_text(matched, sort_key, sort_desc)
        return

    def marker(item: tuple[AKInstrument, dict[str, float]]) -> tuple[Any, ...]:
        instrument, values = item
        value = values.get(sort_key)
        # 缺排序值的行在两个方向都排最后；instrument_id 兜底保证稳定输出
        present = value is not None
        rank = value if present else 0.0
        if sort_desc:
            return (present, rank, instrument.instrument_id)
        return (not present, rank, instrument.instrument_id)

    matched.sort(key=marker, reverse=sort_desc)


def _sort_text(
    matched: list[tuple[AKInstrument, dict[str, float]]],
    sort_key: str,
    sort_desc: bool,
) -> None:
    """Sort by a basic.* identity text; empty names trail in both directions."""

    def marker(item: tuple[AKInstrument, dict[str, float]]) -> tuple[Any, ...]:
        instrument, _values = item
        text = instrument.symbol if sort_key == "basic.code" else (instrument.name or "")
        present = text != ""
        if sort_desc:
            return (present, text, instrument.instrument_id)
        return (not present, text, instrument.instrument_id)

    matched.sort(key=marker, reverse=sort_desc)
