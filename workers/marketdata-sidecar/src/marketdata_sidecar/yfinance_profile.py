"""Yahoo Finance company profile for US/HK instruments."""

from __future__ import annotations

from typing import Any, Mapping

from . import upstream
from .conversion import clean_text
from .models import ProfileField, ProfileGroup, ProfileResponse
from .research_common import research_not_found, yfinance_instrument

# (info key, localized field label) for the basic-info group.
_BASIC_FIELDS = (
    ("longName", "公司名称"),
    ("sector", "所属板块"),
    ("industry", "所属行业"),
    ("country", "国家/地区"),
    ("city", "城市"),
    ("website", "公司网址"),
    ("fullTimeEmployees", "员工人数"),
)


def profile(market: str, symbol: str) -> ProfileResponse:
    instrument = yfinance_instrument(market, symbol)
    info = upstream.ticker_info(
        instrument.yahoo_symbol,
        max_age_seconds=upstream.SECURITY_CACHE_SECONDS,
    )
    if not info or not clean_text(info.get("longName") or info.get("shortName")):
        raise research_not_found("profile", instrument.instrument_id)
    groups = [
        group
        for group in (_basic_group(info), _summary_group(info))
        if group is not None
    ]
    return ProfileResponse(
        instrument_id=instrument.instrument_id,
        market=instrument.market,
        symbol=instrument.symbol,
        currency=clean_text(info.get("currency")),
        groups=groups,
    )


def _basic_group(info: Mapping[str, Any]) -> ProfileGroup | None:
    fields = [
        ProfileField(name=label, value=text)
        for key, label in _BASIC_FIELDS
        if (text := clean_text(info.get(key))) is not None
    ]
    return ProfileGroup(title="基本资料", fields=fields) if fields else None


def _summary_group(info: Mapping[str, Any]) -> ProfileGroup | None:
    summary = clean_text(info.get("longBusinessSummary"))
    if summary is None:
        return None
    return ProfileGroup(
        title="公司简介",
        fields=[ProfileField(name="简介", value=summary)],
    )
