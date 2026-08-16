"""Yahoo Finance US market rankings via predefined screener queries."""

from __future__ import annotations

from typing import Any, Mapping

from . import upstream
from .conversion import clean_text, finite_float
from .errors import invalid_request
from .models import RankingsEntry, RankingsResponse
from .routes.common import market_spec

RANKINGS_SOURCE = "yfinance-rankings"

# Predefined query ids from yfinance 1.6.0 PREDEFINED_SCREENER_QUERIES.
YF_RANKING_QUERIES = {
    "gainers": "day_gainers",
    "losers": "day_losers",
    "active": "most_actives",
}


def rankings(market: str, kind: str, limit: int) -> RankingsResponse:
    spec = market_spec(market)
    if spec.code != "US":
        raise invalid_request(
            "unsupported_market",
            "Yahoo Finance rankings are only available for market: US",
        )
    quotes = upstream.screen_quotes(YF_RANKING_QUERIES[kind], limit)
    entries = [
        entry
        for quote in quotes
        if (entry := _ranking_entry(quote)) is not None
    ]
    return RankingsResponse(
        market="US",
        kind=kind,
        entries=entries[:limit],
        source=RANKINGS_SOURCE,
    )


def _ranking_entry(quote: Mapping[str, Any]) -> RankingsEntry | None:
    symbol = clean_text(quote.get("symbol"))
    price = finite_float(quote.get("regularMarketPrice"))
    change_rate = finite_float(quote.get("regularMarketChangePercent"))
    if symbol is None or price is None or change_rate is None:
        return None
    volume = quote.get("regularMarketVolume")
    if volume is None:
        volume = quote.get("regularMarketDayVolume")
    return RankingsEntry(
        instrument_id=f"US.{symbol.upper()}",
        name=clean_text(
            quote.get("shortName")
            or quote.get("longName")
            or quote.get("displayName")
        ),
        price=price,
        change_rate=change_rate,
        change_amount=finite_float(quote.get("regularMarketChange")),
        volume=finite_float(volume),
        turnover=None,
        turnover_ratio=None,
        pe_ttm=finite_float(quote.get("trailingPE")),
        market_cap=finite_float(quote.get("marketCap")),
    )
