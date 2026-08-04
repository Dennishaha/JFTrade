"""Static JFTrade market profiles supported by this sidecar."""

from fastapi import APIRouter

from ..models import (
    MarketPrecision,
    MarketProfile,
    MarketsResponse,
    TradingWindow,
)
from .common import MARKET_SPECS

router = APIRouter()


def _profile(code: str) -> MarketProfile:
    spec = MARKET_SPECS[code]
    # SH and SZ remain separate request routes, while CN is the console's
    # market-picker aggregate. This lets the UI render one "沪深" option and
    # still preserve the exchange prefix required by Yahoo Finance.
    resolved_market = "CN" if spec.code in {"SH", "SZ"} else spec.code
    return MarketProfile(
        code=spec.code,
        resolved_market=resolved_market,
        preferred_prefix=spec.code,
        display_name=spec.display_name,
        quote_currency=spec.quote_currency,
        timezone=spec.timezone,
        supports_extended_hours=spec.supports_extended_hours,
        requires_exchange_prefix=spec.requires_exchange_prefix,
        aliases=list(spec.aliases),
        regular_sessions=[
            TradingWindow(start_minute=start, end_minute=end, label=label)
            for start, end, label in spec.regular_sessions
        ],
        precision=MarketPrecision(
            price=spec.price_precision,
            quote=spec.quote_precision,
        ),
        tick_size=spec.tick_size,
    )


US_PROFILE = _profile("US")


@router.get("/markets", response_model=MarketsResponse)
def markets() -> MarketsResponse:
    return MarketsResponse(markets=[_profile(code) for code in ("US", "HK", "SH", "SZ")])
