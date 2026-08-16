"""Yahoo Finance news headlines."""

from __future__ import annotations

from typing import Any, Mapping

from fastapi import APIRouter, Query

from .. import upstream
from ..conversion import clean_text, first_value, timestamp_as_rfc3339
from ..errors import SidecarError, upstream_error
from ..models import NewsEntry, NewsResponse
from .common import normalize_instrument

router = APIRouter()


@router.get("/news/{market}/{symbol}", response_model=NewsResponse)
def news(
    market: str,
    symbol: str,
    limit: int = Query(default=10, ge=1, le=50),
) -> NewsResponse:
    instrument = normalize_instrument(market, symbol)
    try:
        items = upstream.ticker_news(instrument.yahoo_symbol, limit)
    except SidecarError:
        raise
    except Exception as exc:
        raise upstream_error("Yahoo Finance news lookup failed") from exc
    entries = [
        entry
        for item in items[:limit]
        if (entry := _news_entry(item)) is not None
    ]
    return NewsResponse(
        market=instrument.market,
        symbol=instrument.symbol,
        instrument_id=instrument.instrument_id,
        entries=entries,
        source="yfinance-news",
    )


def _news_entry(item: Mapping[str, Any]) -> NewsEntry | None:
    content = item.get("content")
    if not isinstance(content, Mapping):
        content = item
    title = clean_text(content.get("title"))
    link = _url_text(first_value(content, "canonicalUrl", "clickThroughUrl"))
    if link is None:
        link = clean_text(content.get("link"))
    publisher = _provider_text(content.get("provider"))
    if publisher is None:
        publisher = clean_text(content.get("publisher"))
    published_at = timestamp_as_rfc3339(
        first_value(content, "pubDate", "providerPublishTime", "displayTime")
    )
    summary = clean_text(content.get("summary"))
    if title is None and link is None:
        return None
    return NewsEntry(
        title=title,
        link=link,
        publisher=publisher,
        published_at=published_at,
        summary=summary,
    )


def _url_text(value: Any) -> str | None:
    if isinstance(value, Mapping):
        return clean_text(value.get("url"))
    return clean_text(value)


def _provider_text(value: Any) -> str | None:
    if isinstance(value, Mapping):
        return clean_text(value.get("displayName"))
    return clean_text(value)
