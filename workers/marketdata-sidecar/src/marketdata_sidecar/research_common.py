"""Shared helpers for the four stock-research capabilities.

Both providers emit the same wire shapes: profile groups, yearly financial
statements with a stable field catalog, analyst aggregates, and ownership
groups.  This module owns the market/identity normalization and the
period/yoy assembly so each provider module only fetches and maps rows.
"""

from __future__ import annotations

from typing import Any, Iterable, Mapping

from .akshare_identity import normalize_identity
from .errors import SidecarError, invalid_request
from .models import FinancialPeriod, FinancialValue
from .routes.common import normalize_instrument

STATEMENT_KINDS = ("income", "balance", "cashflow")
FINANCIAL_PERIODS = 4


def parse_statement(value: str) -> str:
    normalized = value.strip().lower()
    if normalized not in STATEMENT_KINDS:
        raise invalid_request(
            "unsupported_statement",
            f"unsupported financial statement: {value}",
        )
    return normalized


def yfinance_instrument(market: str, symbol: str):
    """Normalize a US/HK route into a canonical symbol and Yahoo ticker."""
    instrument = normalize_instrument(market, symbol)
    if instrument.market not in {"US", "HK"}:
        raise invalid_request(
            "unsupported_market",
            f"Yahoo Finance research data is unavailable for market: {market}",
        )
    return instrument


def akshare_research_identity(
    market: str,
    symbol: str,
    capability: str,
) -> tuple[str, str, str]:
    """Resolve ``(requested_market, leaf_market, code)`` for CN/HK research.

    ``CN`` accepts a plain six-digit code (leaf inferred from the leading
    digit) or an ``SH.``/``SZ.`` qualified symbol; the wire identity echoes
    the requested market (``CN.600519`` for a CN request).
    """
    token = market.strip().upper()
    text = symbol.strip().upper()
    if token == "CN":
        prefix, separator, code = text.partition(".")
        if separator == "." and prefix in {"SH", "SZ"}:
            leaf, plain = prefix, code
        else:
            plain = text
            if not plain.isdigit() or len(plain) != 6:
                raise invalid_request(
                    "invalid_symbol",
                    "CN symbols must be six digits or SH.<code>/SZ.<code>",
                )
            leaf = "SH" if plain.startswith("6") else "SZ"
        if plain.startswith(("4", "8")):
            raise invalid_request(
                "unsupported_market",
                f"AKShare {capability} is unavailable for Beijing listings",
            )
        return "CN", leaf, plain
    if token in {"SH", "SZ", "HK"}:
        leaf, code = normalize_identity(token, text)
        return leaf, leaf, code
    raise invalid_request(
        "unsupported_market",
        f"AKShare {capability} is unavailable for market: {market}",
    )


def require_cn_leaf(leaf: str, capability: str, market: str) -> None:
    if leaf not in {"SH", "SZ"}:
        raise invalid_request(
            "unsupported_market",
            f"AKShare {capability} is only available for CN markets: {market}",
        )


def yearly_period_text(iso_date: str) -> str:
    """Render a fiscal-year-end date as the Chinese yearly period label."""
    year = iso_date.strip()[:4]
    return f"{year}年报" if year.isdigit() else iso_date


def financial_periods(
    entries: Iterable[tuple[str, Mapping[str, float | None]]],
    field_ids: Iterable[str],
) -> list[FinancialPeriod]:
    """Assemble newest-first periods with year-over-year growth.

    ``entries`` are ``(period_text, {field_id: value})`` pairs ordered from
    newest to oldest.  A field missing from one period is omitted from that
    period's ``values`` entirely; yearly statements have no quarter-over-
    quarter figure, so ``qoq`` stays null.
    """
    rows = list(entries)
    known = tuple(field_ids)
    periods: list[FinancialPeriod] = []
    for index, (text, values) in enumerate(rows[:FINANCIAL_PERIODS]):
        older = rows[index + 1][1] if index + 1 < len(rows) else {}
        assembled: dict[str, FinancialValue] = {}
        for field_id in known:
            value = values.get(field_id)
            if value is None:
                continue
            assembled[field_id] = FinancialValue(
                data=value,
                yoy=_growth(value, older.get(field_id)),
                qoq=None,
            )
        periods.append(FinancialPeriod(period_text=text, values=assembled))
    return periods


def _growth(current: float, previous: float | None) -> float | None:
    """Year-over-year growth in percent points (e.g. 0.0526 -> 5.26)."""
    if previous is None or previous == 0:
        return None
    return (current / previous - 1) * 100


def research_not_found(capability: str, instrument_id: str) -> SidecarError:
    return SidecarError(
        404,
        "not_found",
        f"no {capability} data available for {instrument_id}",
    )
