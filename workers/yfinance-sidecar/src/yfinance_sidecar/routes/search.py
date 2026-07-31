"""Yahoo Finance instrument search."""

from __future__ import annotations

import re
from typing import Any, Mapping

from fastapi import APIRouter, Query

from .. import upstream
from ..conversion import clean_text
from ..errors import SidecarError, upstream_error
from ..models import SearchEntry, SearchResponse
from .common import (
    MARKET_ALIASES,
    from_yahoo_symbol,
    market_for_quote,
    normalize_instrument,
    normalized_exchange,
    quote_is_supported,
    quote_matches_instrument,
)

router = APIRouter()


_SUFFIX_QUERY_PATTERNS = (
    (re.compile(r"^\d+\.HK$"), "HK"),
    (re.compile(r"^\d+\.SS$"), "SH"),
    (re.compile(r"^\d+\.SZ$"), "SZ"),
)


@router.get("/search", response_model=SearchResponse)
def search(
    q: str = Query(min_length=1, max_length=100),
    limit: int = Query(default=20, ge=1, le=100),
) -> SearchResponse:
    query = q.strip()
    if not query:
        from ..errors import invalid_request

        raise invalid_request("invalid_query", "q must not be blank")
    if _is_qualified_query(query):
        exact = _exact_search_entry(query)
        return SearchResponse(entries=[exact] if exact is not None else [])
    try:
        quotes = upstream.search_quotes(query, limit)
    except Exception as exc:
        raise upstream_error("Yahoo Finance search failed") from exc
    entries = [
        entry
        for quote in quotes
        if (entry := _search_entry(quote)) is not None
    ]
    return SearchResponse(entries=entries[:limit])


def _exact_search_entry(query: str) -> SearchEntry | None:
    """Resolve qualified codes without relying on Yahoo's text search index."""
    token = query.strip().upper()
    try:
        market, symbol = _exact_query_parts(token)
        instrument = normalize_instrument(market, symbol)
    except (SidecarError, ValueError):
        return None
    try:
        info = upstream.ticker_info(
            instrument.yahoo_symbol,
            max_age_seconds=upstream.SECURITY_CACHE_SECONDS,
        )
    except Exception as exc:
        raise upstream_error("Yahoo Finance search failed") from exc
    if not quote_is_supported(info, instrument.market) or not quote_matches_instrument(
        info, instrument
    ):
        return None
    quote = dict(info)
    quote.setdefault("symbol", instrument.yahoo_symbol)
    return _search_entry(quote)


def _is_qualified_query(query: str) -> bool:
    token = query.strip().upper()
    if _has_market_prefix(token):
        return True
    return any(pattern.fullmatch(token) for pattern, _market in _SUFFIX_QUERY_PATTERNS)


def _exact_query_parts(token: str) -> tuple[str, str]:
    """Parse JFTrade-qualified and numeric Yahoo-suffix code forms."""
    if _has_market_prefix(token):
        return token.split(".", 1)
    for pattern, market in _SUFFIX_QUERY_PATTERNS:
        if pattern.fullmatch(token):
            symbol, _suffix = token.rsplit(".", 1)
            return market, symbol
    raise ValueError("query is not a qualified instrument")


def _has_market_prefix(token: str) -> bool:
    prefix, separator, _symbol = token.partition(".")
    return separator == "." and (prefix in MARKET_ALIASES or prefix == "CN")


def _search_entry(quote: Mapping[str, Any]) -> SearchEntry | None:
    yahoo_symbol = (clean_text(quote.get("symbol")) or "").upper()
    market = market_for_quote(quote)
    if market is None or not quote_is_supported(quote, market):
        return None
    converted = from_yahoo_symbol(market, yahoo_symbol)
    if converted is None:
        return None
    symbol, resolved_market = converted
    name = clean_text(
        quote.get("longname")
        or quote.get("shortname")
        or quote.get("longName")
        or quote.get("shortName")
        or quote.get("name")
    )
    security_type = clean_text(quote.get("quoteType") or quote.get("typeDisp"))
    return SearchEntry(
        market=resolved_market,
        resolved_market=resolved_market,
        instrument_id=f"{resolved_market}.{symbol}",
        code=symbol,
        symbol=symbol,
        name=name,
        security_type=security_type.upper() if security_type else None,
        exchange=normalized_exchange(quote),
        selectable=True,
        source="yfinance",
    )
