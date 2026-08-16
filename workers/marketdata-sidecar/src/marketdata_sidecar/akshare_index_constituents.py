"""AKShare CN index constituent listings."""

from __future__ import annotations

from typing import Any

from . import akshare_upstream
from .akshare_catalog import AKInstrument, catalog
from .akshare_identity import normalize_identity
from .akshare_models import AKIndexConstituent, AKIndexConstituentsResponse
from .akshare_provider_conversion import (
    _frame_rows,
    _optional_decimal,
    _row_value,
)
from .conversion import clean_text
from .errors import SidecarError
from .upstream import _TickerInfoCache

# Constituent lists change on index review schedules, so a one-hour TTL is
# far fresher than the upstream data itself.
CONSTITUENTS_CACHE_SECONDS = 3600

_constituents_cache = _TickerInfoCache()


def index_constituents(
    market: str,
    symbol: str,
    limit: int,
) -> AKIndexConstituentsResponse:
    normalized_market, normalized_symbol = _cn_market(market, symbol)
    instrument = _resolve_cn_index(normalized_market, normalized_symbol)
    function_name, code = _cons_source(instrument)
    rows = _constituents_cache.get_or_fetch(
        instrument.instrument_id,
        CONSTITUENTS_CACHE_SECONDS,
        lambda: {"rows": _fetch_rows(function_name, code)},
    )["rows"]
    entries = [
        entry
        for row in rows
        if (entry := _constituent_entry(row)) is not None
    ][:limit]
    return AKIndexConstituentsResponse(
        market=normalized_market,
        symbol=normalized_symbol,
        instrument_id=instrument.instrument_id,
        constituents=entries,
    )


def _cn_market(market: str, symbol: str) -> tuple[str, str]:
    try:
        normalized_market, normalized_symbol = normalize_identity(market, symbol)
    except SidecarError as exc:
        if exc.code == "unsupported_market":
            raise SidecarError(
                400,
                "AKSHARE_UNSUPPORTED",
                f"AKShare index constituents are unavailable for market: {market}",
            ) from exc
        raise
    if normalized_market not in {"SH", "SZ"}:
        # AKShare 1.18.91 has no HK/US index constituents endpoint.
        raise SidecarError(
            400,
            "AKSHARE_UNSUPPORTED",
            "AKShare index constituents are only available for CN indices",
        )
    return normalized_market, normalized_symbol


def _resolve_cn_index(market: str, symbol: str) -> AKInstrument:
    """Resolve through the full catalog so index codes keep their identity.

    The live spot shortcut used by quote reads labels CSI codes such as
    000300 as equities; the constituents lookup must instead find the
    index-kind catalog entry that carries the ``csi:``/``sh:``/``sz:``
    upstream prefix.
    """
    instrument_id = f"{market}.{symbol}"
    matches = [
        item
        for item in catalog(market)
        if item.instrument_id == instrument_id and item.kind == "index"
    ]
    if not matches:
        raise SidecarError(
            400,
            "AKSHARE_UNSUPPORTED",
            f"instrument is not a supported index: {instrument_id}",
        )
    for item in matches:
        if item.upstream_symbol.startswith("csi:"):
            return item
    return matches[0]


def _cons_source(instrument: AKInstrument) -> tuple[str, str]:
    upstream_symbol = instrument.upstream_symbol
    prefix, separator, code = upstream_symbol.partition(":")
    if separator and prefix == "csi":
        return "index_stock_cons_csindex", code
    return "index_stock_cons", code if separator else upstream_symbol


def _fetch_rows(function_name: str, code: str) -> list[dict[str, Any]]:
    frame = akshare_upstream.call(function_name, symbol=code)
    return [dict(row) for row in _frame_rows(frame)]


def _constituent_entry(row: dict[str, Any]) -> AKIndexConstituent | None:
    code = clean_text(
        _row_value(row, "成分券代码", "品种代码", "code", "symbol")
    )
    if code is None:
        return None
    name = clean_text(
        _row_value(row, "成分券名称", "品种名称", "名称", "name")
    )
    weight = _optional_decimal(row, "权重", "weight")
    return AKIndexConstituent(
        code=code,
        name=name,
        weight=float(weight) if weight is not None else None,
    )
