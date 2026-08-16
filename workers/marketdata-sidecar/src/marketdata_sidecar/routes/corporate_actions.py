"""Yahoo Finance dividend and split history."""

from __future__ import annotations

from datetime import date
from typing import Any, Mapping

from fastapi import APIRouter, Query

from .. import upstream
from ..errors import SidecarError, upstream_error
from ..models import CorporateActionEvent, CorporateActionsResponse
from .common import action_window, normalize_instrument

router = APIRouter()


@router.get(
    "/corporate-actions/{market}/{symbol}",
    response_model=CorporateActionsResponse,
)
def corporate_actions(
    market: str,
    symbol: str,
    from_value: str | None = Query(default=None, alias="from"),
    to_value: str | None = Query(default=None, alias="to"),
) -> CorporateActionsResponse:
    instrument = normalize_instrument(market, symbol)
    from_date, to_date = action_window(from_value, to_value)
    try:
        actions = upstream.ticker_actions(instrument.yahoo_symbol)
    except SidecarError:
        raise
    except Exception as exc:
        raise upstream_error("Yahoo Finance corporate actions lookup failed") from exc
    return CorporateActionsResponse(
        market=instrument.market,
        symbol=instrument.symbol,
        instrument_id=instrument.instrument_id,
        events=_merge_events(actions, from_date, to_date),
        source="yfinance-actions",
    )


def _merge_events(
    actions: Mapping[str, Any],
    from_date: date,
    to_date: date,
) -> list[CorporateActionEvent]:
    events: list[CorporateActionEvent] = []
    for kind, amount_key, ratio_key in (
        ("dividend", "amount", None),
        ("split", None, "ratio"),
    ):
        points = actions.get(f"{kind}s") or []
        for point in points:
            ex_date = point.get("date") if isinstance(point, Mapping) else None
            value = point.get("value") if isinstance(point, Mapping) else None
            if not isinstance(ex_date, str) or not isinstance(value, (int, float)):
                continue
            try:
                parsed = date.fromisoformat(ex_date)
            except ValueError:
                continue
            if not from_date <= parsed <= to_date:
                continue
            events.append(
                CorporateActionEvent(
                    kind=kind,
                    ex_date=ex_date,
                    amount=float(value) if amount_key else None,
                    ratio=float(value) if ratio_key else None,
                )
            )
    events.sort(key=lambda event: (event.ex_date, event.kind))
    return events
