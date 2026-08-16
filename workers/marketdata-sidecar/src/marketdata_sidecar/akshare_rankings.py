"""AKShare market rankings sorted locally from the cached spot catalog.

The catalog already caches the full-market Eastmoney spot frames (15s TTL
with singleflight), so a rankings request performs no new upstream call: it
reuses those rows and sorts them in-process.
"""

from __future__ import annotations

from typing import Any, Mapping

from .akshare_catalog import AKInstrument, catalog
from .akshare_identity import _normalize_market
from .akshare_provider_conversion import _optional_decimal
from .errors import SidecarError
from .models import RankingsEntry, RankingsResponse

RANKINGS_SOURCE = "akshare-rankings"


def rankings(market: str, kind: str, limit: int) -> RankingsResponse:
    normalized = _rankings_market(market)
    leaves = ("SH", "SZ") if normalized == "CN" else (normalized,)
    instruments: list[AKInstrument] = []
    for leaf in leaves:
        instruments.extend(catalog(leaf))
    entries = [
        entry
        for instrument in instruments
        if instrument.kind == "stock"
        if (entry := _ranking_entry(instrument)) is not None
    ]
    _sort_entries(entries, kind)
    return RankingsResponse(
        market=normalized,
        kind=kind,
        entries=entries[:limit],
        source=RANKINGS_SOURCE,
    )


def _rankings_market(market: str) -> str:
    token = market.strip().upper()
    if token == "CN":
        return "CN"
    normalized = _normalize_market(token)
    if normalized == "US":
        raise SidecarError(
            400,
            "AKSHARE_UNSUPPORTED",
            "AKShare rankings are only available for CN and HK markets",
        )
    return normalized


def _sort_entries(entries: list[RankingsEntry], kind: str) -> None:
    if kind == "gainers":
        entries.sort(key=lambda entry: entry.change_rate or 0.0, reverse=True)
    elif kind == "losers":
        entries.sort(key=lambda entry: entry.change_rate or 0.0)
    else:
        entries.sort(
            key=lambda entry: (
                entry.turnover if entry.turnover is not None else float("-inf")
            ),
            reverse=True,
        )


def _ranking_entry(instrument: AKInstrument) -> RankingsEntry | None:
    row = instrument.row
    price = _optional_decimal(row, "最新价", "price")
    change_rate = _optional_decimal(row, "涨跌幅", "change_rate", "changepercent")
    if price is None or change_rate is None:
        return None
    return RankingsEntry(
        instrument_id=instrument.instrument_id,
        name=instrument.name,
        price=float(price),
        change_rate=float(change_rate),
        change_amount=_row_float(row, "涨跌额", "change_amount"),
        volume=_row_float(row, "成交量", "volume"),
        turnover=_row_float(row, "成交额", "turnover"),
        turnover_ratio=_row_float(row, "换手率", "turnover_ratio"),
        pe_ttm=_row_float(row, "市盈率-动态", "市盈率", "pe"),
        market_cap=_row_float(row, "总市值", "market_cap"),
    )


def _row_float(row: Mapping[str, Any], *names: str) -> float | None:
    value = _optional_decimal(row, *names)
    return float(value) if value is not None else None
