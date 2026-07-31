"""Security details and stable fundamental fields."""

from __future__ import annotations

from typing import Any, Mapping

from fastapi import APIRouter

from .. import upstream
from ..conversion import (
    clean_text,
    finite_float,
    first_value,
    non_negative_int,
)
from ..errors import not_found, upstream_error
from ..models import SecurityResponse
from .common import (
    normalize_instrument,
    normalized_exchange,
    quote_is_supported,
    quote_matches_instrument,
)

router = APIRouter()


@router.get("/security/{market}/{symbol}", response_model=SecurityResponse)
def security(market: str, symbol: str) -> SecurityResponse:
    instrument = normalize_instrument(market, symbol)
    try:
        info = upstream.ticker_info(
            instrument.yahoo_symbol,
            max_age_seconds=upstream.SECURITY_CACHE_SECONDS,
        )
    except Exception as exc:
        raise upstream_error("Yahoo Finance security lookup failed") from exc
    if (
        not _looks_like_security(info)
        or not quote_is_supported(info, instrument.market)
        or not quote_matches_instrument(info, instrument)
    ):
        raise not_found(
            "security_not_found",
            f"security not found: {instrument.instrument_id}",
        )
    return _security_response(
        info,
        resolved_market=instrument.market,
        symbol=instrument.symbol,
        instrument_id=instrument.instrument_id,
    )


def _looks_like_security(info: Mapping[str, Any]) -> bool:
    return bool(
        clean_text(info.get("symbol"))
        or clean_text(info.get("shortName"))
        or clean_text(info.get("longName"))
        or clean_text(info.get("quoteType"))
    )


def _security_response(
    info: Mapping[str, Any],
    *,
    resolved_market: str,
    symbol: str,
    instrument_id: str,
) -> SecurityResponse:
    name = clean_text(first_value(info, "longName", "shortName", "displayName"))
    security_type = clean_text(info.get("quoteType"))
    return SecurityResponse(
        market=resolved_market,
        symbol=symbol,
        instrument_id=instrument_id,
        name=name or symbol,
        exchange=normalized_exchange(info),
        currency=clean_text(info.get("currency")),
        timezone=clean_text(
            first_value(info, "exchangeTimezoneName", "timeZoneFullName")
        ),
        security_type=security_type.upper() if security_type else None,
        industry=clean_text(info.get("industry")),
        sector=clean_text(info.get("sector")),
        website=clean_text(info.get("website")),
        business_summary=clean_text(info.get("longBusinessSummary")),
        market_cap=non_negative_int(info.get("marketCap")),
        trailing_pe=finite_float(info.get("trailingPE")),
        forward_pe=finite_float(info.get("forwardPE")),
        trailing_eps=finite_float(info.get("trailingEps")),
        forward_eps=finite_float(info.get("forwardEps")),
        dividend_rate=finite_float(info.get("dividendRate")),
        dividend_yield=finite_float(info.get("dividendYield")),
        fifty_two_week_high=finite_float(info.get("fiftyTwoWeekHigh")),
        fifty_two_week_low=finite_float(info.get("fiftyTwoWeekLow")),
        average_volume=non_negative_int(info.get("averageVolume")),
        shares_outstanding=non_negative_int(info.get("sharesOutstanding")),
        source="yfinance",
    )
