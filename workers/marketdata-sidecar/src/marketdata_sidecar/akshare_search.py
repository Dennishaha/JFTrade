"""AKShare symbol search, identity resolution, and instrument lookup."""

from __future__ import annotations

from typing import Any, Iterable, Mapping

from . import akshare_upstream
from .akshare_catalog import (
    AKInstrument,
    _dedupe_instruments,
    _famous_catalog,
    _instrument,
    _live_spot_catalog,
    catalog,
)
from .akshare_identity import HK_INDEX_IDS, US_INDEX_IDS, normalize_identity
from .akshare_models import AKSearchEntry
from .akshare_provider_conversion import _row_value
from .conversion import clean_text
from .errors import SidecarError, invalid_request, not_found


def search(query: str, limit: int) -> list[AKSearchEntry]:
    token = query.strip()
    if not token:
        raise invalid_request("invalid_query", "q must not be blank")
    qualified = _qualified_query(token)
    live_entries = _search_live(query=token, limit=limit, qualified=qualified)
    if live_entries is not None:
        return live_entries
    markets = [qualified[0]] if qualified is not None else ["US", "HK", "SH", "SZ"]
    candidates: list[tuple[int, AKInstrument]] = []
    normalized_query = _search_token(token)
    for market in markets:
        for instrument in catalog(market):
            score = _search_score(instrument, normalized_query, qualified)
            if score is not None:
                candidates.append((score, instrument))
    candidates.sort(key=lambda item: (item[0], item[1].instrument_id, item[1].name))
    identities: dict[str, set[tuple[str, str]]] = {}
    for _score, instrument in candidates:
        identities.setdefault(instrument.instrument_id, set()).add(
            (instrument.kind, instrument.upstream_symbol)
        )
    entries: list[AKSearchEntry] = []
    seen: set[str] = set()
    for _score, instrument in candidates:
        if instrument.instrument_id in seen:
            continue
        seen.add(instrument.instrument_id)
        if len(identities[instrument.instrument_id]) > 1:
            if qualified is not None:
                raise invalid_request(
                    "ambiguous_instrument",
                    f"instrument is ambiguous: {instrument.instrument_id}",
                )
            continue
        entries.append(_search_entry(instrument))
        if len(entries) >= limit:
            break
    return entries


def resolve_instrument(market: str, symbol: str) -> AKInstrument:
    normalized_market, normalized_symbol = normalize_identity(market, symbol)
    return resolve_from_catalog(
        normalized_market,
        normalized_symbol,
        _resolution_catalog(normalized_market, normalized_symbol),
    )


def _resolution_catalog(market: str, symbol: str) -> list[AKInstrument]:
    """Load the smallest AKShare directory that can resolve one identity.

    The complete US/HK spot endpoints paginate the entire exchange and can
    take longer than the 12-second request budget.  Their "famous" views are
    still AKShare data, contain the same delayed quote fields, and cover the
    common symbols used by the workspace.  We use them first for exact reads;
    a previously cached full directory, or the full directory on a miss,
    preserves support for less common symbols and search results.
    """
    live = _live_spot_catalog(market, {symbol})
    if live is not None and any(item.instrument_id == f"{market}.{symbol}" for item in live):
        return live
    if market == "US" and symbol not in US_INDEX_IDS.values():
        try:
            famous = _famous_catalog("US", {symbol})
        except AssertionError:
            # Test doubles and older AKShare builds may not expose the
            # optional famous-market endpoint.  Keep the contractual full
            # directory path as the compatibility fallback.
            famous = catalog(market)
        if any(item.instrument_id == f"US.{symbol}" for item in famous):
            return famous
        return catalog(market)
    if market == "HK" and symbol not in set(HK_INDEX_IDS.values()):
        try:
            famous = _famous_catalog("HK", {symbol})
        except AssertionError:
            famous = catalog(market)
        if any(item.instrument_id == f"HK.{symbol}" for item in famous):
            return famous
        return catalog(market)
    return catalog(market)


def _search_live(
    *,
    query: str,
    limit: int,
    qualified: tuple[str, str] | None,
) -> list[AKSearchEntry] | None:
    try:
        lookup_query = query
        if qualified is not None:
            lookup_query = qualified[1].lstrip(".")
            lookup_query = {
                ".DJI": "道琼斯指数",
                ".SPX": "标普500指数",
                ".NDX": "纳斯达克100",
            }.get(
                qualified[1],
                lookup_query,
            )
        rows = akshare_upstream.search_rows(lookup_query)
    except AssertionError:
        return None
    except SidecarError as exc:
        if exc.code in {"AKSHARE_RUNTIME_FAILED", "AKSHARE_RUNTIME_WARMING"}:
            return None
        raise
    candidates: list[tuple[int, AKInstrument]] = []
    normalized_query = _search_token(query)
    for row in rows:
        instrument = _suggested_instrument(row)
        if instrument is None:
            continue
        score = _search_score(instrument, normalized_query, qualified)
        if score is not None:
            candidates.append((score, instrument))
    candidates.sort(key=lambda item: (item[0], item[1].instrument_id, item[1].name))
    return [_search_entry(item) for _score, item in candidates[:limit]]


def _suggested_instrument(row: Mapping[str, Any]) -> AKInstrument | None:
    market_id = clean_text(_row_value(row, "MktNum", "market_id")) or ""
    code = clean_text(_row_value(row, "Code", "代码", "code"))
    name = clean_text(_row_value(row, "Name", "名称", "name"))
    if not code:
        return None
    if market_id in {"105", "106", "107"}:
        market, symbol, kind, upstream_symbol = "US", code.upper(), "stock", code.upper()
    elif market_id == "116":
        market, symbol, kind, upstream_symbol = "HK", f"{int(code):05d}", "stock", f"{int(code):05d}"
    elif market_id in {"0", "1"}:
        market, symbol, kind, upstream_symbol = market_id == "1" and "SH" or "SZ", code, "stock", code
    elif market_id in {"100", "124"} and code.upper() in {"DJIA", "SPX", "NDX", "NDX100"}:
        mapping = {
            "DJIA": (".DJI", "道琼斯"),
            "SPX": (".SPX", "标普500"),
            "NDX": (".NDX", "纳斯达克"),
            "NDX100": (".NDX", "纳斯达克100"),
        }
        symbol, upstream_symbol = mapping[code.upper()]
        market, kind = "US", "index"
    elif market_id in {"100", "124"} and code.upper() in {"HSI", "HSCEI", "HSTECH"}:
        mapping = {"HSI": "800000", "HSCEI": "800100", "HSTECH": "800700"}
        market, symbol, upstream_symbol, kind = "HK", mapping[code.upper()], code.upper(), "index"
    else:
        return None
    classify = clean_text(_row_value(row, "Classify", "classify")) or ""
    security_type = "ETF" if classify.lower() == "fund" else ("INDEX" if kind == "index" else ("EQUITY" if market in {"SH", "SZ"} else None))
    return _instrument(
        market,
        symbol,
        upstream_symbol,
        {"代码": symbol, "名称": name or symbol},
        kind=kind,
        security_type=security_type,
    )


def resolve_from_catalog(
    market: str,
    symbol: str,
    instruments: Iterable[AKInstrument],
) -> AKInstrument:
    normalized_market, normalized_symbol = normalize_identity(market, symbol)
    instrument_id = f"{normalized_market}.{normalized_symbol}"
    matches = [
        item for item in instruments if item.instrument_id == instrument_id
    ]
    if not matches:
        raise not_found("instrument_not_found", f"instrument not found: {instrument_id}")
    source_identities = {(item.kind, item.upstream_symbol) for item in matches}
    if len(source_identities) > 1:
        raise invalid_request(
            "ambiguous_instrument",
            f"instrument is ambiguous: {instrument_id}",
        )
    return matches[0]


def _search_entry(instrument: AKInstrument) -> AKSearchEntry:
    return AKSearchEntry(
        market=instrument.market,
        resolved_market=instrument.market,
        instrument_id=instrument.instrument_id,
        code=instrument.symbol,
        symbol=instrument.symbol,
        name=instrument.name,
        security_type=instrument.security_type,
        exchange=instrument.exchange,
        supported_periods=list(instrument.supported_periods),
    )


def _qualified_query(query: str) -> tuple[str, str] | None:
    token = query.strip().upper()
    if token.startswith("CN."):
        parts = token.split(".", 2)
        if len(parts) == 3 and parts[1] in {"SH", "SZ"}:
            return normalize_identity("CN", f"{parts[1]}.{parts[2]}")
    prefix, separator, symbol = token.partition(".")
    if separator and prefix in {"US", "HK", "SH", "SZ"}:
        return normalize_identity(prefix, symbol)
    return None


def _search_token(query: str) -> str:
    return query.strip().upper().replace(" ", "")


def _search_score(
    instrument: AKInstrument,
    query: str,
    qualified: tuple[str, str] | None,
) -> int | None:
    if qualified is not None:
        return 0 if (instrument.market, instrument.symbol) == qualified else None
    symbol = instrument.symbol.upper().replace(" ", "")
    name = instrument.name.upper().replace(" ", "")
    instrument_id = instrument.instrument_id.upper().replace(" ", "")
    if query in {symbol, instrument_id}:
        return 0
    if query == name:
        return 1
    if symbol.startswith(query) or instrument_id.startswith(query):
        return 2
    if query in name:
        return 3
    return None
