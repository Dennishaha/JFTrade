"""AKShare CN industry/concept board listings and board members.

Eastmoney boards cover A-shares only, so both endpoints reject US/HK
markets.  Board and member frames change slowly, so they sit behind a
one-hour in-process cache mirroring the index-constituents pattern.
"""

from __future__ import annotations

import re
from typing import Any, Mapping

from . import akshare_upstream
from .akshare_identity import _normalize_market
from .akshare_models import (
    AKIndustriesResponse,
    AKIndustryBoard,
    AKIndustryMembersResponse,
)
from .akshare_provider_conversion import _frame_rows, _optional_decimal, _row_value
from .conversion import clean_text
from .errors import SidecarError, invalid_request, not_found
from .models import RankingsEntry
from .upstream import _TickerInfoCache

INDUSTRIES_CACHE_SECONDS = 3600
INDUSTRIES_SOURCE = "akshare-industries"

_BOARD_FUNCTIONS = {
    "industry": "stock_board_industry_name_em",
    "concept": "stock_board_concept_name_em",
}
_MEMBER_FUNCTIONS = {
    "industry": "stock_board_industry_cons_em",
    "concept": "stock_board_concept_cons_em",
}

_boards_cache = _TickerInfoCache()
_members_cache = _TickerInfoCache()


def industries(market: str, kind: str) -> AKIndustriesResponse:
    normalized = _cn_boards_market(market)
    rows = _board_rows(kind)
    boards = [board for row in rows if (board := _board_entry(row)) is not None]
    return AKIndustriesResponse(
        market=normalized,
        kind=kind,
        boards=boards,
        source=INDUSTRIES_SOURCE,
    )


def industry_members(
    market: str,
    kind: str | None,
    name: str,
    limit: int,
) -> AKIndustryMembersResponse:
    normalized = _cn_boards_market(market)
    board = clean_text(name)
    if board is None:
        raise invalid_request("invalid_board", "board name must not be blank")
    resolved_kind, code, canonical = _resolve_board(kind, board)
    rows = _members_cache.get_or_fetch(
        f"{resolved_kind}:{code}",
        INDUSTRIES_CACHE_SECONDS,
        lambda: {"rows": _fetch_member_rows(resolved_kind, code)},
    )["rows"]
    entries = [
        entry
        for row in rows
        if (entry := _member_entry(row)) is not None
    ][:limit]
    return AKIndustryMembersResponse(
        market=normalized,
        kind=resolved_kind,
        board=canonical,
        entries=entries,
        source=INDUSTRIES_SOURCE,
    )


def _cn_boards_market(market: str) -> str:
    token = market.strip().upper()
    if token == "CN":
        return "CN"
    normalized = _normalize_market(token)
    if normalized not in {"SH", "SZ"}:
        # Eastmoney industry/concept boards only cover A-shares.
        raise SidecarError(
            400,
            "AKSHARE_UNSUPPORTED",
            f"AKShare industry boards are unavailable for market: {market}",
        )
    return normalized


def _board_rows(kind: str) -> list[dict[str, Any]]:
    return _boards_cache.get_or_fetch(
        kind,
        INDUSTRIES_CACHE_SECONDS,
        lambda: {"rows": _fetch_board_rows(kind)},
    )["rows"]


def _fetch_board_rows(kind: str) -> list[dict[str, Any]]:
    frame = akshare_upstream.call(_BOARD_FUNCTIONS[kind])
    return [dict(row) for row in _frame_rows(frame)]


def _fetch_member_rows(kind: str, code: str) -> list[dict[str, Any]]:
    frame = akshare_upstream.call(_MEMBER_FUNCTIONS[kind], symbol=code)
    return [dict(row) for row in _frame_rows(frame)]


def _resolve_board(kind: str | None, name: str) -> tuple[str, str, str]:
    """Resolve ``(kind, BK code, canonical name)`` for a member lookup.

    When ``kind`` is omitted the board is resolved by name alone: the
    industry frame is searched first, then the concept frame.  Name matching
    is case-insensitive for ASCII letters (the JFTrade API layer uppercases
    instrument ids, so ``ChatGPT概念`` may arrive as ``CHATGPT概念``); an
    exact match always wins over a case-variant match.  Pure-Chinese names
    are unaffected.
    """
    if re.fullmatch(r"BK\d+", name):
        # A raw BK code skips AKShare's name lookup; without a kind hint the
        # industry member endpoint is the documented default.
        return (kind or "industry"), name, name
    candidates = (kind,) if kind is not None else ("industry", "concept")
    for candidate in candidates:
        row = _find_board_row(_board_rows(candidate), name)
        if row is not None:
            code = clean_text(_row_value(row, "板块代码"))
            canonical = clean_text(_row_value(row, "板块名称"))
            if code is not None and canonical is not None:
                return candidate, code, canonical
    label = kind or "industry/concept"
    raise not_found("board_not_found", f"unknown {label} board: {name}")


def _find_board_row(
    rows: list[dict[str, Any]],
    name: str,
) -> dict[str, Any] | None:
    case_insensitive: dict[str, Any] | None = None
    folded = _ascii_upper(name)
    for row in rows:
        board_name = clean_text(_row_value(row, "板块名称"))
        if board_name is None:
            continue
        if board_name == name:
            return row
        if case_insensitive is None and _ascii_upper(board_name) == folded:
            case_insensitive = row
    return case_insensitive


def _ascii_upper(value: str) -> str:
    """Uppercase ASCII letters only; CJK and other scripts stay untouched."""
    return "".join(
        chr(ord(char) - 32) if "a" <= char <= "z" else char for char in value
    )


def _board_entry(row: Mapping[str, Any]) -> AKIndustryBoard | None:
    name = clean_text(_row_value(row, "板块名称", "name"))
    if name is None:
        return None
    return AKIndustryBoard(
        name=name,
        change_rate=_row_float(row, "涨跌幅", "change_rate"),
        turnover=_row_float(row, "成交额", "turnover"),
        volume=_row_float(row, "成交量", "volume"),
        leading_stock_name=clean_text(_row_value(row, "领涨股票", "leading_stock")),
        leading_stock_change_rate=_row_float(row, "领涨股票-涨跌幅"),
    )


def _member_entry(row: Mapping[str, Any]) -> RankingsEntry | None:
    code = clean_text(_row_value(row, "代码", "code", "symbol"))
    if code is None or not re.fullmatch(r"\d{6}", code):
        return None
    market = _member_market(code)
    if market is None:
        # Boards may include Beijing listings, which JFTrade does not model.
        return None
    price = _optional_decimal(row, "最新价", "price")
    change_rate = _optional_decimal(row, "涨跌幅", "change_rate")
    if price is None or change_rate is None:
        return None
    return RankingsEntry(
        instrument_id=f"{market}.{code}",
        name=clean_text(_row_value(row, "名称", "name")),
        price=float(price),
        change_rate=float(change_rate),
        change_amount=_row_float(row, "涨跌额", "change_amount"),
        volume=_row_float(row, "成交量", "volume"),
        turnover=_row_float(row, "成交额", "turnover"),
        turnover_ratio=_row_float(row, "换手率", "turnover_ratio"),
        pe_ttm=_row_float(row, "市盈率-动态", "市盈率", "pe"),
        market_cap=_row_float(row, "总市值", "market_cap"),
    )


def _member_market(code: str) -> str | None:
    if code.startswith("6"):
        return "SH"
    if code.startswith(("0", "3")):
        return "SZ"
    return None


def _row_float(row: Mapping[str, Any], *names: str) -> float | None:
    value = _optional_decimal(row, *names)
    return float(value) if value is not None else None
