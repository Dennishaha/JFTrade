"""Stock research routes: profile, financials, analyst, ownership.

``router`` carries the yfinance-backed endpoints (mounted bare and under
``/providers/yfinance``); ``akshare_router`` carries the AKShare-backed CN/HK
variants under ``/providers/akshare``.
"""

from __future__ import annotations

from fastapi import APIRouter, Query

from .. import (
    akshare_analyst,
    akshare_financials,
    akshare_ownership,
    akshare_profile,
    yfinance_analyst,
    yfinance_financials,
    yfinance_ownership,
    yfinance_profile,
)
from ..errors import SidecarError, upstream_error
from ..models import (
    AnalystResponse,
    FinancialsResponse,
    OwnershipResponse,
    ProfileResponse,
)
from ..research_common import parse_statement
from .akshare import _translate

router = APIRouter()
akshare_router = APIRouter(prefix="/providers/akshare")


@router.get("/profile/{market}/{symbol:path}", response_model=ProfileResponse)
def profile(market: str, symbol: str) -> ProfileResponse:
    return _yfinance("profile lookup", yfinance_profile.profile, market, symbol)


@router.get("/financials/{market}/{symbol:path}", response_model=FinancialsResponse)
def financials(
    market: str,
    symbol: str,
    statement: str = Query(default="income"),
) -> FinancialsResponse:
    return _yfinance(
        "financials lookup",
        yfinance_financials.financials,
        market,
        symbol,
        parse_statement(statement),
    )


@router.get("/analyst/{market}/{symbol:path}", response_model=AnalystResponse)
def analyst(market: str, symbol: str) -> AnalystResponse:
    return _yfinance("analyst lookup", yfinance_analyst.analyst, market, symbol)


@router.get("/ownership/{market}/{symbol:path}", response_model=OwnershipResponse)
def ownership(market: str, symbol: str) -> OwnershipResponse:
    return _yfinance("ownership lookup", yfinance_ownership.ownership, market, symbol)


def _yfinance(operation: str, function, *args):
    try:
        return function(*args)
    except SidecarError:
        raise
    except Exception as exc:
        raise upstream_error(f"Yahoo Finance {operation} failed") from exc


@akshare_router.get("/profile/{market}/{symbol:path}", response_model=ProfileResponse)
def akshare_profile_route(market: str, symbol: str) -> ProfileResponse:
    return _translate("profile lookup", akshare_profile.profile, market, symbol)


@akshare_router.get("/financials/{market}/{symbol:path}", response_model=FinancialsResponse)
def akshare_financials_route(
    market: str,
    symbol: str,
    statement: str = Query(default="income"),
) -> FinancialsResponse:
    return _translate(
        "financials lookup",
        akshare_financials.financials,
        market,
        symbol,
        parse_statement(statement),
    )


@akshare_router.get("/analyst/{market}/{symbol:path}", response_model=AnalystResponse)
def akshare_analyst_route(market: str, symbol: str) -> AnalystResponse:
    return _translate("analyst lookup", akshare_analyst.analyst, market, symbol)


@akshare_router.get("/ownership/{market}/{symbol:path}", response_model=OwnershipResponse)
def akshare_ownership_route(market: str, symbol: str) -> OwnershipResponse:
    return _translate("ownership lookup", akshare_ownership.ownership, market, symbol)
