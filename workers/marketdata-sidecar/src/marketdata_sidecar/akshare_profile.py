"""AKShare company profiles for CN (Eastmoney F10) and HK instruments."""

from __future__ import annotations

from typing import Any, Mapping

from . import akshare_upstream
from .akshare_provider_conversion import _frame_rows, _row_value
from .conversion import clean_text
from .models import ProfileField, ProfileGroup, ProfileResponse
from .research_common import akshare_research_identity, research_not_found
from .upstream import _TickerInfoCache

PROFILE_CACHE_SECONDS = 3600

# CN item labels emitted in this order when present.
_CN_FIELD_ORDER = (
    "股票简称",
    "总股本",
    "流通股",
    "总市值",
    "流通市值",
    "行业",
    "上市时间",
)
_MARKET_CURRENCY = {"SH": "CNY", "SZ": "CNY", "HK": "HKD"}

_profile_cache = _TickerInfoCache()


def profile(market: str, symbol: str) -> ProfileResponse:
    requested, leaf, code = akshare_research_identity(market, symbol, "profile")
    instrument_id = f"{requested}.{code}"
    data = _profile_cache.get_or_fetch(
        f"{leaf}:{code}",
        PROFILE_CACHE_SECONDS,
        lambda: {"rows": _fetch_profile(leaf, code)},
    )["rows"]
    groups = _hk_groups(data) if leaf == "HK" else _cn_groups(data)
    if not groups:
        raise research_not_found("profile", instrument_id)
    return ProfileResponse(
        instrument_id=instrument_id,
        market=requested,
        symbol=code,
        currency=_MARKET_CURRENCY[leaf],
        groups=groups,
    )


def _fetch_profile(leaf: str, code: str) -> list[dict[str, Any]]:
    if leaf == "HK":
        frame = akshare_upstream.call("stock_hk_company_profile_em", symbol=code)
        rows = list(_frame_rows(frame))
        # The HK company profile arrives as one wide row; transpose it.
        if not rows:
            return []
        return [{"name": str(key), "value": value} for key, value in rows[0].items()]
    frame = akshare_upstream.call("stock_individual_info_em", symbol=code)
    return [
        {"name": name, "value": _row_value(row, "value")}
        for row in _frame_rows(frame)
        if (name := clean_text(_row_value(row, "item"))) is not None
    ]


def _cn_groups(rows: list[Mapping[str, Any]]) -> list[ProfileGroup]:
    values = {row["name"]: clean_text(row.get("value")) for row in rows}
    fields = [
        ProfileField(name=label, value=text)
        for label in _CN_FIELD_ORDER
        if (text := values.get(label)) is not None
    ]
    return [ProfileGroup(title="基本资料", fields=fields)] if fields else []


def _hk_groups(rows: list[Mapping[str, Any]]) -> list[ProfileGroup]:
    fields = [
        ProfileField(name=row["name"], value=text)
        for row in rows
        if (text := clean_text(row.get("value"))) is not None
    ]
    return [ProfileGroup(title="公司资料", fields=fields)] if fields else []
