"""Yahoo Finance custom equity screener (US only).

Factor mapping is pinned against yfinance 1.6.0 ``EQUITY_SCREENER_FIELDS``
(const.py:660-763, merged with COMMON_SCREENER_FIELDS:642-650); EquityQuery
itself rejects fields outside that table, so anything missing here is refused
locally with 400 unsupported_kind instead of reaching Yahoo:

- simple.price      → intradayprice            (当日最新价；lastclose 是昨收)
- simple.change_pct → percentchange            (当日涨跌幅，%)
- simple.volume     → dayvolume                (当日成交量，股)
- simple.market_cap → intradaymarketcap        (盘中市值)
- simple.pe_ttm     → peratio.lasttwelvemonths (TTM 市盈率)
- simple.pb         → pricebookratio.quarterly (季度市净率)

The screen response carries standard quote keys rather than screener field
names, so values are projected back through QUOTE_VALUE_KEYS.  Yahoo caps a
custom screen page at size=250; offset and size are passed through to Yahoo
independently, so the window may extend past 250 via pagination.
"""

from __future__ import annotations

from datetime import datetime
from typing import Any, Mapping
from zoneinfo import ZoneInfo

from . import upstream
from .conversion import clean_text, finite_float
from .errors import invalid_request
from .models import (
    ScreenCondition,
    ScreenEntry,
    ScreenRequest,
    ScreenResponse,
    ScreenSort,
)
from .routes.common import market_spec

SCREEN_SOURCE = "yfinance-screen"
# Yahoo caps a single custom-screen page at 250 (yfinance screener.py
# ValueError); offset is independent, so only the page size is bounded.
MAX_SCREEN_PAGE = 250

FACTOR_FIELDS = {
    "simple.price": "intradayprice",
    "simple.change_pct": "percentchange",
    "simple.volume": "dayvolume",
    "simple.market_cap": "intradaymarketcap",
    "simple.pe_ttm": "peratio.lasttwelvemonths",
    "simple.pb": "pricebookratio.quarterly",
}

QUOTE_VALUE_KEYS = {
    "simple.price": ("regularMarketPrice",),
    "simple.change_pct": ("regularMarketChangePercent",),
    "simple.volume": ("regularMarketVolume", "regularMarketDayVolume"),
    "simple.market_cap": ("marketCap",),
    "simple.pe_ttm": ("trailingPE",),
    "simple.pb": ("priceToBook",),
}

# Yahoo's default screener sort field is the ticker; basic.code sorts on it.
TICKER_SORT_FIELD = "ticker"


def screen(request: ScreenRequest) -> ScreenResponse:
    spec = market_spec(request.market)
    if spec.code != "US":
        raise invalid_request(
            "unsupported_market",
            "Yahoo Finance screen is only available for market: US",
        )
    conditions = [_translate_condition(condition) for condition in request.conditions]
    sort_field, sort_asc = _translate_sort(request.sorts)
    if request.limit > MAX_SCREEN_PAGE:
        raise invalid_request(
            "invalid_request",
            f"screen limit must not exceed {MAX_SCREEN_PAGE}",
        )
    # Pin region=us so the custom query never scans non-US listings.
    result = upstream.screen_custom(
        [("EQ", "region", ("us",)), *conditions],
        sort_field,
        sort_asc,
        request.limit,
        request.offset,
    )
    quotes = [quote for quote in result["quotes"] if isinstance(quote, Mapping)]
    consumed = len(quotes)
    entries = [
        entry
        for quote in quotes
        if (entry := _screen_entry(quote, spec.quote_currency)) is not None
    ]
    total, has_more = _page_meta(result.get("total"), request, consumed)
    return ScreenResponse(
        entries=entries,
        total=total,
        has_more=has_more,
        next_offset=request.offset + consumed if has_more else None,
        as_of=datetime.now(ZoneInfo(spec.timezone)).isoformat(timespec="seconds"),
        source=SCREEN_SOURCE,
    )


def _translate_condition(condition: ScreenCondition) -> tuple[str, str, tuple[Any, ...]]:
    if condition.in_values is not None:
        raise invalid_request(
            "unsupported_kind",
            "enumeration (in) conditions are not supported by this catalog",
        )
    field = FACTOR_FIELDS.get(condition.factor_key.strip())
    if field is None:
        raise invalid_request(
            "unsupported_kind",
            f"unsupported screen factor: {condition.factor_key}",
        )
    if condition.min is not None and condition.max is not None:
        if condition.min > condition.max:
            raise invalid_request(
                "invalid_request",
                "condition min must not exceed max",
            )
        return ("BTWN", field, (condition.min, condition.max))
    if condition.min is not None:
        return ("GTE", field, (condition.min,))
    if condition.max is not None:
        return ("LTE", field, (condition.max,))
    raise invalid_request(
        "invalid_request",
        f"screen condition {condition.factor_key} requires min or max",
    )


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
    if key == "basic.code":
        field = TICKER_SORT_FIELD
    else:
        field = FACTOR_FIELDS.get(key)
        if field is None:
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
    return field, direction == "asc"


def _screen_entry(
    quote: Mapping[str, Any],
    fallback_currency: str,
) -> ScreenEntry | None:
    symbol = clean_text(quote.get("symbol"))
    if symbol is None:
        return None
    values: dict[str, float] = {}
    for factor_key, quote_keys in QUOTE_VALUE_KEYS.items():
        for quote_key in quote_keys:
            number = finite_float(quote.get(quote_key))
            if number is not None:
                values[factor_key] = number
                break
    return ScreenEntry(
        instrument_id=f"US.{symbol.upper()}",
        name=clean_text(
            quote.get("shortName") or quote.get("longName") or quote.get("displayName")
        ),
        symbol=symbol.upper(),
        industry=clean_text(quote.get("industry")),
        quote_currency=clean_text(quote.get("currency")) or fallback_currency,
        values=values,
    )


def _page_meta(
    raw_total: Any,
    request: ScreenRequest,
    fetched: int,
) -> tuple[int, bool]:
    """Yahoo custom screens return a ``total`` count; when it is absent fall
    back to the fetched window and treat a full page as "probably more"."""
    total = raw_total if isinstance(raw_total, (int, float)) else None
    if total is not None:
        return int(total), fetched > 0 and request.offset + fetched < int(total)
    return request.offset + fetched, fetched >= request.limit
