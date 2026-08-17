"""AKShare instrument catalog construction, caching, and batch resolution."""

from __future__ import annotations

import re
import threading
import time
from dataclasses import dataclass
from decimal import Decimal
from typing import Any, Callable, Iterable, Mapping

from . import akshare_upstream
from .akshare_identity import (
    CODE_PATTERN,
    HK_INDEX_IDS,
    MARKET_EXCHANGE,
    US_INDEX_IDS,
    _etf_market,
    _hk_index_symbol,
    _normalize_market,
    _stock_symbols,
    _us_index_symbol,
    normalize_identity,
)
from .akshare_provider_conversion import _frame_rows, _row_value
from .akshare_spot_clist import fetch_spot_frame_clist
from .conversion import clean_text
from .errors import SidecarError
from .routes.common import MARKET_SPECS

CATALOG_CACHE_SECONDS = 15
# A failed full-market request should not immediately be retried by every
# waiter that was released by the same singleflight.  This small negative
# cache keeps one upstream outage from turning into a request storm while the
# normal positive cache remains the contractual 15 seconds.
CATALOG_FAILURE_CACHE_SECONDS = 2
ALL_PERIODS = ("1m", "5m", "15m", "30m", "1h", "1d", "1w", "1mo")
INDEX_PERIODS = ("1d", "1w", "1mo")
INTRADAY_PERIODS = frozenset({"1m", "5m", "15m", "30m", "1h"})
US_FAMOUS_CATEGORIES = (
    "科技类",
    "金融类",
    "医药食品类",
    "媒体类",
    "汽车能源类",
    "制造零售类",
)
CN_INDEX_SERIES = {
    "SH": ("上证系列指数", "中证系列指数"),
    "SZ": ("深证系列指数",),
}


@dataclass(frozen=True)
class AKInstrument:
    market: str
    symbol: str
    instrument_id: str
    upstream_symbol: str
    name: str
    security_type: str | None
    exchange: str | None
    kind: str
    supported_periods: tuple[str, ...]
    row: Mapping[str, Any]

    @property
    def timezone(self) -> str:
        return MARKET_SPECS[self.market].timezone

    @property
    def volume_multiplier(self) -> Decimal:
        return Decimal(100) if self.market in {"SH", "SZ"} else Decimal(1)


class _TTLCache:
    """Small TTL cache with per-key singleflight for full-market frames."""

    def __init__(self) -> None:
        self._lock = threading.Lock()
        self._values: dict[str, tuple[Any, float]] = {}
        self._errors: dict[str, tuple[BaseException, float]] = {}
        self._inflight: dict[str, threading.Event] = {}

    def get_or_fetch(self, key: str, fetch: Callable[[], Any]) -> Any:
        while True:
            with self._lock:
                value = self._values.get(key)
                if value is not None and time.monotonic() - value[1] < CATALOG_CACHE_SECONDS:
                    return value[0]
                failure = self._errors.get(key)
                if failure is not None:
                    error, failed_at = failure
                    if time.monotonic() - failed_at < CATALOG_FAILURE_CACHE_SECONDS:
                        raise error
                    self._errors.pop(key, None)
                event = self._inflight.get(key)
                if event is None:
                    event = threading.Event()
                    self._inflight[key] = event
                    owner = True
                else:
                    owner = False
            if owner:
                break
            event.wait(timeout=0.1)
            akshare_upstream.ensure_request_active()
        try:
            value = fetch()
            with self._lock:
                self._values[key] = (value, time.monotonic())
                self._errors.pop(key, None)
            return value
        except BaseException as exc:
            with self._lock:
                self._errors[key] = (exc, time.monotonic())
            raise
        finally:
            with self._lock:
                self._inflight.pop(key, None)
                event.set()

    def clear(self) -> None:
        with self._lock:
            self._values.clear()
            self._errors.clear()


_catalog_cache = _TTLCache()


def catalog(market: str) -> list[AKInstrument]:
    normalized = _normalize_market(market)
    instruments: list[AKInstrument] = []
    if normalized == "US":
        instruments.extend(_stock_catalog_frame("US", _cached_clist_call("stock:US", "US")))
        instruments.extend(_us_index_catalog())
    elif normalized == "HK":
        instruments.extend(_stock_catalog_frame("HK", _cached_clist_call("stock:HK", "HK")))
        instruments.extend(_hk_index_catalog())
    else:
        function_name = "stock_sh_a_spot_em" if normalized == "SH" else "stock_sz_a_spot_em"
        instruments.extend(_stock_catalog(normalized, function_name))
        instruments.extend(_etf_catalog(normalized))
        instruments.extend(_cn_index_catalog(normalized))
    return instruments


def snapshot_catalog(market: str, symbols: Iterable[str]) -> list[AKInstrument]:
    """Resolve a batch from one market directory without needless pagination.

    The complete US/HK spot directories are still used by ``search`` and as
    a fallback for uncommon symbols.  A watchlist batch containing ordinary
    well-known securities can use AKShare's compact famous directory, so one
    slow full-market pagination cannot occupy every request slot while the
    chart is loading.
    """
    normalized = _normalize_market(market)
    requested = {normalize_identity(normalized, symbol)[1] for symbol in symbols}
    live = _live_spot_catalog(normalized, requested)
    if live is not None:
        return live
    if normalized not in {"US", "HK"}:
        return catalog(normalized)
    try:
        index_symbols = set(US_INDEX_IDS.values()) if normalized == "US" else set(HK_INDEX_IDS.values())
        result = _famous_catalog(normalized, requested - index_symbols)
    except AssertionError:
        return catalog(normalized)
    known = {item.symbol for item in result}
    missing = requested - known
    index_symbols = set(US_INDEX_IDS.values()) if normalized == "US" else set(HK_INDEX_IDS.values())
    missing -= index_symbols
    if missing:
        return catalog(normalized)
    if requested & index_symbols:
        result.extend(
            _us_index_catalog() if normalized == "US" else _hk_index_catalog()
        )
    return _dedupe_instruments(result)


def _famous_catalog(
    market: str,
    symbols: set[str] | None = None,
) -> list[AKInstrument]:
    if market == "US":
        result: list[AKInstrument] = []
        for category in US_FAMOUS_CATEGORIES:
            result.extend(_stock_catalog(
                "US",
                "stock_us_famous_spot_em",
                cache_key=f"famous:US:{category}",
                call_kwargs={"symbol": category},
                kind="stock",
            ))
            if symbols and symbols.issubset({item.symbol for item in result}):
                break
        return _dedupe_instruments(result)
    if market == "HK":
        return _stock_catalog(
            "HK",
            "stock_hk_famous_spot_em",
            cache_key="famous:HK",
            kind="stock",
        )
    return []


def _live_spot_catalog(
    market: str,
    symbols: set[str],
) -> list[AKInstrument] | None:
    """Use the current Eastmoney batch endpoint for exact reads.

    The helper is deliberately optional: fixture tests and older packaged
    runtimes that do not expose the compatibility transport continue through
    the AKShare catalog functions below.
    """
    try:
        rows = akshare_upstream.spot_rows(market, sorted(symbols))
    except AssertionError:
        return None
    except SidecarError as exc:
        if exc.code in {"AKSHARE_RUNTIME_FAILED", "AKSHARE_RUNTIME_WARMING"}:
            return None
        raise
    return _spot_instruments(market, symbols, rows)


def _spot_instruments(
    market: str,
    symbols: set[str],
    rows: Iterable[Mapping[str, Any]],
) -> list[AKInstrument]:
    result: list[AKInstrument] = []
    requested = {normalize_identity(market, symbol)[1] for symbol in symbols}
    for row in rows:
        code = clean_text(_row_value(row, "代码", "code"))
        if code is None:
            continue
        market_id = str(_row_value(row, "market_id", "market") or "")
        symbol, upstream_symbol, kind, security_type = _spot_identity(
            market,
            code,
            market_id,
            row,
        )
        if symbol is None or symbol not in requested:
            continue
        result.append(
            _instrument(
                market,
                symbol,
                upstream_symbol,
                row,
                kind=kind,
                security_type=security_type,
            )
        )
    return _dedupe_instruments(result)


def _spot_identity(
    market: str,
    code: str,
    market_id: str,
    row: Mapping[str, Any],
) -> tuple[str | None, str, str, str | None]:
    token = code.strip().upper()
    if market == "US":
        if market_id == "100":
            mapping = {"DJIA": ".DJI", "SPX": ".SPX", "NDX": ".NDX"}
            symbol = mapping.get(token)
            names = {"DJIA": "道琼斯", "SPX": "标普500", "NDX": "纳斯达克"}
            return symbol, names.get(token, token), "index", "INDEX"
        return (token if CODE_PATTERN.fullmatch(token) else None), token, "stock", None
    if market == "HK":
        core = {"HSI": "800000", "HSCEI": "800100", "HSTECH": "800700"}
        if market_id in {"100", "124"} and token in core:
            return core[token], token, "index", "INDEX"
        if not token.isdigit():
            return None, token, "stock", None
        return f"{int(token):05d}", f"{int(token):05d}", "stock", None
    if not re.fullmatch(r"\d{6}", token):
        return None, token, "stock", None
    is_etf = str(_row_value(row, "instrument_kind", "kind") or "") == "3"
    index_codes = {"SH": {"000001"}, "SZ": {"399001", "399006"}}
    if token in index_codes[market]:
        return token, token, "index", "INDEX"
    return token, token, "etf" if is_etf else "stock", "ETF" if is_etf else "EQUITY"


def _stock_catalog(
    market: str,
    function_name: str,
    *,
    cache_key: str | None = None,
    call_kwargs: Mapping[str, Any] | None = None,
    kind: str = "stock",
) -> list[AKInstrument]:
    frame = _cached_call(
        cache_key or f"stock:{market}",
        function_name,
        **dict(call_kwargs or {}),
    )
    return _stock_catalog_frame(market, frame, kind=kind)


def _stock_catalog_frame(
    market: str,
    frame: Any,
    *,
    kind: str = "stock",
) -> list[AKInstrument]:
    result: list[AKInstrument] = []
    for row in _frame_rows(frame):
        raw_code = clean_text(_row_value(row, "代码", "symbol", "code"))
        if raw_code is None:
            continue
        symbol, upstream_symbol = _stock_symbols(market, raw_code)
        if symbol is None:
            continue
        result.append(
            _instrument(
                market,
                symbol,
                upstream_symbol,
                row,
                kind=kind,
                security_type="EQUITY" if market in {"SH", "SZ"} else None,
            )
        )
    return result


def _dedupe_instruments(values: Iterable[AKInstrument]) -> list[AKInstrument]:
    result: list[AKInstrument] = []
    seen: set[tuple[str, str, str]] = set()
    for instrument in values:
        key = (instrument.instrument_id, instrument.kind, instrument.upstream_symbol)
        if key in seen:
            continue
        seen.add(key)
        result.append(instrument)
    return result


def _etf_catalog(market: str) -> list[AKInstrument]:
    frame = _cached_call("etf:CN", "fund_etf_spot_em")
    result: list[AKInstrument] = []
    for row in _frame_rows(frame):
        code = clean_text(_row_value(row, "代码", "symbol", "code"))
        if code is None or not re.fullmatch(r"\d{6}", code):
            continue
        if _etf_market(code) != market:
            continue
        result.append(_instrument(market, code, code, row, kind="etf", security_type="ETF"))
    return result


def _cn_index_catalog(market: str) -> list[AKInstrument]:
    result: list[AKInstrument] = []
    for series in CN_INDEX_SERIES[market]:
        frame = _cached_call(
            f"index:{series}",
            "stock_zh_index_spot_em",
            symbol=series,
        )
        for row in _frame_rows(frame):
            code = clean_text(_row_value(row, "代码", "symbol", "code"))
            if code is None:
                continue
            digits = code.rsplit(".", 1)[-1]
            if not re.fullmatch(r"\d{6}", digits):
                continue
            series_key = {
                "上证系列指数": "sh",
                "深证系列指数": "sz",
                "中证系列指数": "csi",
            }[series]
            result.append(
                _instrument(
                    market,
                    digits,
                    f"{series_key}:{digits}",
                    row,
                    kind="index",
                    security_type="INDEX",
                )
            )
    return result


def _hk_index_catalog() -> list[AKInstrument]:
    frame = _cached_call("index:HK", "stock_hk_index_spot_em")
    result: list[AKInstrument] = []
    for row in _frame_rows(frame):
        code = clean_text(_row_value(row, "代码", "symbol", "code"))
        name = clean_text(_row_value(row, "名称", "name"))
        if code is None:
            continue
        symbol = _hk_index_symbol(code, name)
        if symbol is None:
            continue
        result.append(
            _instrument(
                "HK",
                symbol,
                code,
                row,
                kind="index",
                security_type="INDEX",
            )
        )
    return result


def _us_index_catalog() -> list[AKInstrument]:
    frame = _cached_call("index:US", "index_global_spot_em")
    result: list[AKInstrument] = []
    for row in _frame_rows(frame):
        code = clean_text(_row_value(row, "代码", "symbol", "code"))
        name = clean_text(_row_value(row, "名称", "name"))
        symbol = _us_index_symbol(code, name)
        if symbol is None:
            continue
        result.append(
            _instrument(
                "US",
                symbol,
                name or code or symbol,
                row,
                kind="index",
                security_type="INDEX",
            )
        )
    return result


def _instrument(
    market: str,
    symbol: str,
    upstream_symbol: str,
    row: Mapping[str, Any],
    *,
    kind: str,
    security_type: str | None,
) -> AKInstrument:
    name = clean_text(_row_value(row, "名称", "name", "简称")) or symbol
    exchange = MARKET_EXCHANGE[market]
    if market == "US":
        exchange = clean_text(_row_value(row, "交易所", "exchange"))
    return AKInstrument(
        market=market,
        symbol=symbol,
        instrument_id=f"{market}.{symbol}",
        upstream_symbol=upstream_symbol,
        name=name,
        security_type=security_type,
        exchange=exchange,
        kind=kind,
        supported_periods=INDEX_PERIODS if kind == "index" and market in {"US", "HK"} else ALL_PERIODS,
        row=row,
    )


def _cached_call(key: str, function_name: str, **kwargs: Any) -> Any:
    return _catalog_cache.get_or_fetch(
        key,
        lambda: akshare_upstream.call(function_name, **kwargs),
    )


def _cached_clist_call(key: str, market: str) -> Any:
    """Full-market US/HK spot frames fetched via the clist direct fetcher.

    ``fetch_spot_frame_clist`` issues its own HTTP requests (see its module
    docstring), so it is bound to the worker pool through ``run`` instead of
    ``call``.
    """
    return _catalog_cache.get_or_fetch(
        key,
        lambda: akshare_upstream.run(fetch_spot_frame_clist, market),
    )
