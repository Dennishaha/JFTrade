"""AKShare calendar and macro routes under /providers/akshare."""

from __future__ import annotations

from datetime import date

from fastapi import APIRouter, Query

from .. import akshare_calendar, akshare_macro
from ..errors import invalid_request
from ..models import (
    CalendarDividendsResponse,
    CalendarEarningsResponse,
    CalendarEconomicResponse,
    CalendarIposResponse,
    MacroHistoryResponse,
    MacroIndicatorsResponse,
)
from .akshare import _translate

router = APIRouter(prefix="/providers/akshare")


@router.get("/calendar/earnings", response_model=CalendarEarningsResponse)
def earnings(
    begin_date: str = Query(),
    end_date: str = Query(),
) -> CalendarEarningsResponse:
    begin, end = _window(begin_date, end_date)
    return _translate("earnings calendar lookup", akshare_calendar.earnings, begin, end)


@router.get("/calendar/dividends", response_model=CalendarDividendsResponse)
def dividends(date_value: str = Query(alias="date")) -> CalendarDividendsResponse:
    return _translate(
        "dividends calendar lookup",
        akshare_calendar.dividends,
        _parse_date(date_value, "date"),
    )


@router.get("/calendar/economic", response_model=CalendarEconomicResponse)
def economic(
    begin_date: str = Query(),
    end_date: str = Query(),
) -> CalendarEconomicResponse:
    begin, end = _window(begin_date, end_date)
    return _translate("economic calendar lookup", akshare_calendar.economic, begin, end)


@router.get("/calendar/ipos", response_model=CalendarIposResponse)
def ipos() -> CalendarIposResponse:
    return _translate("IPO calendar lookup", akshare_calendar.ipos)


@router.get("/macro/indicators", response_model=MacroIndicatorsResponse)
def indicators() -> MacroIndicatorsResponse:
    return _translate("macro indicators lookup", akshare_macro.indicators)


@router.get("/macro/indicator-history", response_model=MacroHistoryResponse)
def indicator_history(
    indicator_id: str = Query(min_length=1, max_length=64),
    limit: int = Query(default=100, ge=1, le=500),
) -> MacroHistoryResponse:
    return _translate(
        "macro indicator history lookup",
        akshare_macro.indicator_history,
        indicator_id,
        limit,
    )


def _window(begin_value: str, end_value: str) -> tuple[date, date]:
    begin = _parse_date(begin_value, "begin_date")
    end = _parse_date(end_value, "end_date")
    if begin > end:
        raise invalid_request("invalid_request", "begin_date must not be after end_date")
    return begin, end


def _parse_date(value: str, field: str) -> date:
    try:
        return date.fromisoformat(value.strip())
    except (ValueError, AttributeError) as exc:
        raise invalid_request(
            "invalid_request",
            f"{field} must be a YYYY-MM-DD date",
        ) from exc
