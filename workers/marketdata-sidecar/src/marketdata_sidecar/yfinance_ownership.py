"""Yahoo Finance ownership breakdown for US/HK instruments.

``major_holders`` reports percentage-Held fractions (0-1); institutional and
mutualfund rows carry a ``pctHeld`` fraction.  All are emitted as 0-100
percentages on the wire.
"""

from __future__ import annotations

from typing import Any, Mapping

from . import upstream
from .conversion import finite_float
from .models import OwnershipGroup, OwnershipItem, OwnershipResponse
from .research_common import research_not_found, yfinance_instrument


def ownership(market: str, symbol: str) -> OwnershipResponse:
    instrument = yfinance_instrument(market, symbol)
    data = upstream.ticker_ownership(instrument.yahoo_symbol)
    groups = [
        group
        for group in (
            _major_group(data.get("major") or []),
            _holders_group("institutional_holders", data.get("institutional") or []),
            _holders_group("mutualfund_holders", data.get("mutualfund") or []),
        )
        if group is not None
    ]
    if not groups:
        raise research_not_found("ownership", instrument.instrument_id)
    return OwnershipResponse(instrument_id=instrument.instrument_id, groups=groups)


def _major_group(records: list[Mapping[str, Any]]) -> OwnershipGroup | None:
    items = [
        OwnershipItem(name=label, holder_pct=_percent(record.get("Value")))
        for record in records
        if (label := record.get("label"))
    ]
    if not items:
        return None
    return OwnershipGroup(kind="major_holders", static_date=None, items=items)


def _holders_group(
    kind: str,
    records: list[Mapping[str, Any]],
) -> OwnershipGroup | None:
    items = [
        OwnershipItem(name=name, holder_pct=_percent(record.get("pctHeld")))
        for record in records
        if (name := record.get("Holder"))
    ]
    if not items:
        return None
    dates = sorted(
        date for record in records if (date := record.get("Date Reported"))
    )
    return OwnershipGroup(
        kind=kind,
        static_date=dates[-1] if dates else None,
        items=items,
    )


def _percent(value: Any) -> float | None:
    number = finite_float(value)
    return number * 100 if number is not None else None
