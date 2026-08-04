"""AKShare catalog, quote, and candle normalization."""

from __future__ import annotations

import re
import threading
import time
from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
from decimal import Decimal, InvalidOperation
from typing import Any, Callable, Iterable, Mapping
from zoneinfo import ZoneInfo

from . import akshare_upstream
from .akshare_models import (
    AKCandle,
    AKCandlesResponse,
    AKSearchEntry,
    AKSecurityResponse,
    AKSnapshotQuote,
    AKSnapshotResponse,
)
from .conversion import clean_text, format_rfc3339, timestamp_as_utc
from .errors import SidecarError, invalid_request, not_found
from .routes.common import MARKET_SPECS

UTC = timezone.utc
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
HK_INDEX_IDS = {
    "HSI": "800000",
    "恒生指数": "800000",
    "HSCEI": "800100",
    "恒生中国企业指数": "800100",
    "国企指数": "800100",
    "HSTECH": "800700",
    "恒生科技指数": "800700",
}
US_INDEX_IDS = {
    "DJIA": ".DJI",
    "DJI": ".DJI",
    "道琼斯": ".DJI",
    "道琼斯指数": ".DJI",
    "SPX": ".SPX",
    "SP500": ".SPX",
    "标普500": ".SPX",
    "标普500指数": ".SPX",
    "NDX": ".NDX",
    "纳斯达克100": ".NDX",
    "纳斯达克100指数": ".NDX",
}
MARKET_CURRENCY = {"US": "USD", "HK": "HKD", "SH": "CNY", "SZ": "CNY"}
MARKET_EXCHANGE = {"US": None, "HK": "HKEX", "SH": "SSE", "SZ": "SZSE"}
CODE_PATTERN = re.compile(r"^[A-Z0-9.^=_-]{1,64}$")


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
        instruments.extend(_stock_catalog("US", "stock_us_spot_em"))
        instruments.extend(_us_index_catalog())
    elif normalized == "HK":
        instruments.extend(_stock_catalog("HK", "stock_hk_spot_em"))
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


def security(instrument: AKInstrument) -> AKSecurityResponse:
    return AKSecurityResponse(
        market=instrument.market,
        symbol=instrument.symbol,
        instrument_id=instrument.instrument_id,
        name=instrument.name or instrument.symbol,
        exchange=instrument.exchange,
        currency=MARKET_CURRENCY[instrument.market],
        timezone=instrument.timezone,
        security_type=instrument.security_type,
        supported_periods=list(instrument.supported_periods),
    )


def snapshot(instrument: AKInstrument) -> AKSnapshotResponse:
    row = instrument.row
    price = _required_decimal(row, "最新价", "最新", "price")
    quote_at = _row_timestamp(row, instrument.timezone)
    volume = _optional_decimal(row, "成交量", "volume", minimum=Decimal(0))
    if volume is not None:
        volume *= instrument.volume_multiplier
    turnover = _optional_decimal(row, "成交额", "turnover", minimum=Decimal(0))
    open_price = _optional_price(
        row,
        "今开",
        "开盘",
        "开盘价",
        "open",
    )
    high = _optional_price(row, "最高", "最高价", "high")
    low = _optional_price(row, "最低", "最低价", "low")
    _validate_snapshot_ohlc(price, open_price, high, low)
    regular = AKSnapshotQuote(
        price=_decimal_text(price),
        high_price=_decimal_text(high),
        low_price=_decimal_text(low),
        volume=_decimal_text(volume),
        turnover=_decimal_text(turnover),
        change_value=_decimal_text(_optional_decimal(row, "涨跌额", "change")),
        change_rate=_decimal_text(_optional_decimal(row, "涨跌幅", "change_rate")),
        quote_at=quote_at,
    )
    previous_close = _optional_price(
        row,
        "昨收",
        "昨收价",
        "previous_close",
    )
    return AKSnapshotResponse(
        market=instrument.market,
        symbol=instrument.symbol,
        instrument_id=instrument.instrument_id,
        price=_decimal_text(price) or "0",
        bid=_decimal_text(_optional_price(row, "买一", "买入", "bid")),
        ask=_decimal_text(_optional_price(row, "卖一", "卖出", "ask")),
        open_price=_decimal_text(open_price),
        high_price=_decimal_text(high),
        low_price=_decimal_text(low),
        previous_close_price=_decimal_text(previous_close),
        last_close_price=_decimal_text(previous_close),
        regular_quote=regular,
        volume=_decimal_text(volume),
        turnover=_decimal_text(turnover),
        quote_at=quote_at,
        observed_at=format_rfc3339(_utc_now()),
        currency=MARKET_CURRENCY[instrument.market],
        exchange=instrument.exchange,
    )


def candles(
    instrument: AKInstrument,
    *,
    period: str,
    limit: int,
    from_time: datetime | None,
    to_time: datetime | None,
) -> AKCandlesResponse:
    normalized_period = period.strip().lower()
    validate_candle_query(normalized_period, from_time, to_time)
    if normalized_period not in instrument.supported_periods:
        raise invalid_request(
            "unsupported_period",
            f"unsupported candle period for {instrument.instrument_id}: {period}",
        )
    validate_candle_retention(
        instrument.market,
        normalized_period,
        from_time,
        to_time,
    )
    frame, fetched_period, source, volume_multiplier = _fetch_candle_frame(
        instrument,
        normalized_period,
        from_time,
        to_time,
    )
    converted = _convert_candle_frame(
        frame,
        instrument=instrument,
        from_time=from_time,
        to_time=to_time,
        volume_multiplier=volume_multiplier,
    )
    if fetched_period != normalized_period:
        converted = _aggregate_candles(
            converted,
            normalized_period,
            instrument.timezone,
        )
    converted = converted[-limit:]
    if not converted:
        raise not_found(
            "candles_not_found",
            f"candles not found: {instrument.instrument_id}",
        )
    return AKCandlesResponse(
        market=instrument.market,
        symbol=instrument.symbol,
        instrument_id=instrument.instrument_id,
        period=normalized_period,
        candles=converted,
        total_returned=len(converted),
        source=source,
    )


def validate_candle_query(
    period: str,
    from_time: datetime | None,
    to_time: datetime | None,
) -> None:
    if period.strip().lower() not in ALL_PERIODS:
        raise invalid_request("unsupported_period", f"unsupported candle period: {period}")
    if from_time is not None and to_time is not None and from_time > to_time:
        raise invalid_request("invalid_time_range", "from must be earlier than or equal to to")


def validate_candle_retention(
    market: str,
    period: str,
    from_time: datetime | None,
    to_time: datetime | None,
) -> None:
    normalized_period = period.strip().lower()
    uses_five_day_source = normalized_period == "1m" or (
        market.strip().upper() == "US" and normalized_period in INTRADAY_PERIODS
    )
    if not uses_five_day_source:
        return
    cutoff = _utc_now() - timedelta(days=5)
    if any(bound is not None and bound < cutoff for bound in (from_time, to_time)):
        raise invalid_request(
            "UNSUPPORTED_RANGE",
            "requested intraday data is only available for the last 5 days",
        )


def normalize_identity(market: str, symbol: str) -> tuple[str, str]:
    normalized_market = market.strip().upper()
    normalized_symbol = symbol.strip().upper()
    if normalized_market == "CN":
        prefix, separator, code = normalized_symbol.partition(".")
        if separator != "." or prefix not in {"SH", "SZ"}:
            raise invalid_request(
                "invalid_symbol",
                "CN symbols must use SH.<code> or SZ.<code>",
            )
        normalized_market, normalized_symbol = prefix, code
    normalized_market = _normalize_market(normalized_market)
    for prefix in (normalized_market, *MARKET_SPECS[normalized_market].aliases):
        if normalized_symbol.startswith(f"{prefix}."):
            normalized_symbol = normalized_symbol[len(prefix) + 1 :]
            break
    if normalized_market in {"SH", "SZ"}:
        if not re.fullmatch(r"\d{6}", normalized_symbol):
            raise invalid_request("invalid_symbol", "China symbols must be six digits")
    elif normalized_market == "HK" and normalized_symbol.isdigit():
        normalized_symbol = f"{int(normalized_symbol):05d}"
    elif not CODE_PATTERN.fullmatch(normalized_symbol):
        raise invalid_request("invalid_symbol", "symbol has an invalid format")
    return normalized_market, normalized_symbol


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


def _fetch_candle_frame(
    instrument: AKInstrument,
    period: str,
    from_time: datetime | None,
    to_time: datetime | None,
) -> tuple[Any, str, str, Decimal]:
    start = from_time or (_utc_now() - timedelta(days=5 if period in INTRADAY_PERIODS else 365 * 5))
    end = to_time or _utc_now()
    if instrument.kind == "index" and instrument.market in {"US", "HK"}:
        frame, source, volume_multiplier = _global_index_daily_frame(instrument)
        return frame, "1d", source, volume_multiplier
    if period in INTRADAY_PERIODS:
        fetch_period = "1m" if instrument.market in {"US", "HK"} else period
        minute = "60" if fetch_period == "1h" else fetch_period.removesuffix("m")
        if instrument.market in {"SH", "SZ"}:
            fallback_name, fallback_kwargs = _intraday_call(instrument, minute, start, end)
            frame, source, volume_multiplier = _preferred_history_call(
                "stock_zh_a_minute",
                {
                    "symbol": _sina_cn_symbol(instrument),
                    "period": minute,
                    "adjust": "",
                },
                fallback_name,
                fallback_kwargs,
                fallback_volume_multiplier=instrument.volume_multiplier,
            )
            return frame, fetch_period, source, volume_multiplier
        if instrument.market == "US":
            return (
                akshare_upstream.us_minute_rows(instrument.symbol),
                fetch_period,
                "akshare:sina",
                Decimal(1),
            )
        if instrument.market == "HK":
            return (
                akshare_upstream.hk_minute_rows(instrument.symbol),
                fetch_period,
                "akshare:eastmoney",
                Decimal(1),
            )
        function_name, kwargs = _intraday_call(instrument, minute, start, end)
        return (
            akshare_upstream.call(function_name, **kwargs),
            fetch_period,
            "akshare:eastmoney",
            instrument.volume_multiplier,
        )
    frame, source, volume_multiplier = _daily_frame(instrument, start, end)
    return frame, "1d", source, volume_multiplier


def _daily_frame(
    instrument: AKInstrument,
    start: datetime,
    end: datetime,
) -> tuple[Any, str, Decimal]:
    fallback_name, fallback_kwargs = _daily_call(instrument, "1d", start, end)
    if instrument.kind == "etf":
        return _preferred_history_call(
            "fund_etf_hist_sina",
            {"symbol": _sina_cn_symbol(instrument)},
            fallback_name,
            fallback_kwargs,
            fallback_volume_multiplier=instrument.volume_multiplier,
        )
    if instrument.kind == "index":
        return _preferred_history_call(
            "stock_zh_index_daily",
            {"symbol": _sina_cn_symbol(instrument)},
            fallback_name,
            fallback_kwargs,
            fallback_volume_multiplier=instrument.volume_multiplier,
        )
    if instrument.market == "US":
        return _preferred_history_call(
            "stock_us_daily",
            {"symbol": instrument.symbol, "adjust": ""},
            fallback_name,
            fallback_kwargs,
        )
    if instrument.market == "HK":
        return _preferred_history_call(
            "stock_hk_daily",
            {"symbol": instrument.symbol, "adjust": ""},
            fallback_name,
            fallback_kwargs,
        )
    return _preferred_history_call(
        "stock_zh_a_daily",
        {
            "symbol": _sina_cn_symbol(instrument),
            "start_date": _daily_bound(start, instrument.timezone),
            "end_date": _daily_bound(end, instrument.timezone),
            "adjust": "",
        },
        fallback_name,
        fallback_kwargs,
        fallback_volume_multiplier=instrument.volume_multiplier,
    )


def _global_index_daily_frame(
    instrument: AKInstrument,
) -> tuple[Any, str, Decimal]:
    if instrument.market == "US":
        fallback_name = "index_global_hist_em"
        fallback_kwargs = {"symbol": instrument.upstream_symbol}
        return _preferred_history_call(
            "stock_us_daily",
            {"symbol": _sina_us_index_symbol(instrument.symbol), "adjust": ""},
            fallback_name,
            fallback_kwargs,
        )
    return _preferred_history_call(
        "stock_hk_index_daily_sina",
        {"symbol": instrument.upstream_symbol},
        "stock_hk_index_daily_em",
        {"symbol": instrument.upstream_symbol},
    )


def _preferred_history_call(
    function_name: str,
    kwargs: dict[str, Any],
    fallback_name: str,
    fallback_kwargs: dict[str, Any],
    *,
    fallback_volume_multiplier: Decimal = Decimal(1),
) -> tuple[Any, str, Decimal]:
    """Use another AKShare transport when Eastmoney history is unreachable."""
    try:
        frame = akshare_upstream.call(function_name, **kwargs)
        if not _frame_is_empty(frame):
            return frame, "akshare:sina", Decimal(1)
    except SidecarError as exc:
        if exc.status_code == 503:
            raise
    except Exception:
        akshare_upstream.ensure_request_active()
    frame = akshare_upstream.call(fallback_name, **fallback_kwargs)
    return frame, "akshare:eastmoney", fallback_volume_multiplier


def _sina_cn_symbol(instrument: AKInstrument) -> str:
    prefix = "sh" if instrument.market == "SH" else "sz"
    return f"{prefix}{instrument.symbol}"


def _sina_us_index_symbol(symbol: str) -> str:
    return {".DJI": ".DJI", ".SPX": ".INX", ".NDX": ".NDX"}[symbol]


def _intraday_call(
    instrument: AKInstrument,
    minute: str,
    start: datetime,
    end: datetime,
) -> tuple[str, dict[str, Any]]:
    if instrument.kind == "etf":
        return "fund_etf_hist_min_em", {
            "symbol": _history_symbol(instrument),
            "period": minute,
            "start_date": _intraday_bound(start, instrument.timezone),
            "end_date": _intraday_bound(end, instrument.timezone),
            "adjust": "",
        }
    if instrument.kind == "index":
        return "index_zh_a_hist_min_em", {
            "symbol": _history_symbol(instrument),
            "period": minute,
            "start_date": _intraday_bound(start, instrument.timezone),
            "end_date": _intraday_bound(end, instrument.timezone),
        }
    if instrument.market == "US":
        return "stock_us_hist_min_em", {
            "symbol": _history_symbol(instrument),
            "start_date": _intraday_bound(start, instrument.timezone),
            "end_date": _intraday_bound(end, instrument.timezone),
        }
    if instrument.market == "HK":
        return "stock_hk_hist_min_em", {
            "symbol": _history_symbol(instrument),
            "period": minute,
            "start_date": _intraday_bound(start, instrument.timezone),
            "end_date": _intraday_bound(end, instrument.timezone),
            "adjust": "",
        }
    return "stock_zh_a_hist_min_em", {
        "symbol": _history_symbol(instrument),
        "period": minute,
        "start_date": _intraday_bound(start, instrument.timezone),
        "end_date": _intraday_bound(end, instrument.timezone),
        "adjust": "",
    }


def _daily_call(
    instrument: AKInstrument,
    period: str,
    start: datetime,
    end: datetime,
) -> tuple[str, dict[str, Any]]:
    upstream_period = {"1d": "daily", "1w": "weekly", "1mo": "monthly"}[period]
    kwargs = {
        "symbol": _history_symbol(instrument),
        "period": upstream_period,
        "start_date": _daily_bound(start, instrument.timezone),
        "end_date": _daily_bound(end, instrument.timezone),
    }
    if instrument.kind == "etf":
        kwargs["adjust"] = ""
        return "fund_etf_hist_em", kwargs
    if instrument.kind == "index":
        return "index_zh_a_hist", kwargs
    kwargs["adjust"] = ""
    if instrument.market == "US":
        return "stock_us_hist", kwargs
    if instrument.market == "HK":
        return "stock_hk_hist", kwargs
    return "stock_zh_a_hist", kwargs


def _convert_candle_frame(
    frame: Any,
    *,
    instrument: AKInstrument,
    from_time: datetime | None,
    to_time: datetime | None,
    volume_multiplier: Decimal,
) -> list[AKCandle]:
    result: list[AKCandle] = []
    for index, row in _frame_rows_with_index(frame):
        at_value = _row_value(row, "时间", "日期", "datetime", "date", "time", "day")
        at = timestamp_as_utc(at_value if at_value is not None else index, instrument.timezone)
        if at is None or (from_time is not None and at < from_time) or (to_time is not None and at > to_time):
            continue
        open_price = _optional_decimal(row, "开盘", "开盘价", "open", minimum=Decimal(0))
        high = _optional_decimal(row, "最高", "最高价", "high", minimum=Decimal(0))
        low = _optional_decimal(row, "最低", "最低价", "low", minimum=Decimal(0))
        close = _optional_decimal(
            row,
            "收盘",
            "收盘价",
            "最新价",
            "latest",
            "close",
            minimum=Decimal(0),
        )
        if None in {open_price, high, low, close}:
            continue
        assert open_price is not None and high is not None and low is not None and close is not None
        if min(open_price, high, low, close) <= 0:
            continue
        if high < max(open_price, low, close) or low > min(open_price, high, close):
            continue
        volume = _optional_decimal(row, "成交量", "volume", minimum=Decimal(0))
        if volume is not None:
            volume *= volume_multiplier
        result.append(
            AKCandle(
                at=format_rfc3339(at),
                open=_decimal_text(open_price) or "0",
                high=_decimal_text(high) or "0",
                low=_decimal_text(low) or "0",
                close=_decimal_text(close) or "0",
                volume=_decimal_text(volume),
            )
        )
    result.sort(key=lambda item: item.at)
    return result


def _aggregate_candles(
    candles: list[AKCandle],
    period: str,
    timezone_name: str,
) -> list[AKCandle]:
    groups: dict[datetime, list[AKCandle]] = {}
    for candle in candles:
        at = datetime.fromisoformat(candle.at.replace("Z", "+00:00"))
        local = at.astimezone(ZoneInfo(timezone_name))
        if period in {"5m", "15m", "30m", "1h"}:
            minutes = 60 if period == "1h" else int(period.removesuffix("m"))
            bucket = local.replace(minute=(local.minute // minutes) * minutes, second=0, microsecond=0)
        elif period == "1w":
            bucket = (local - timedelta(days=local.weekday())).replace(hour=0, minute=0, second=0, microsecond=0)
        elif period == "1mo":
            bucket = local.replace(day=1, hour=0, minute=0, second=0, microsecond=0)
        else:
            bucket = local
        groups.setdefault(bucket.astimezone(UTC), []).append(candle)
    result: list[AKCandle] = []
    for bucket, items in sorted(groups.items()):
        volumes = [Decimal(item.volume) for item in items if item.volume is not None]
        result.append(
            AKCandle(
                at=format_rfc3339(bucket),
                open=items[0].open,
                high=_decimal_text(max(Decimal(item.high) for item in items)) or "0",
                low=_decimal_text(min(Decimal(item.low) for item in items)) or "0",
                close=items[-1].close,
                volume=_decimal_text(sum(volumes, Decimal(0))) if volumes else None,
            )
        )
    return result


def _cached_call(key: str, function_name: str, **kwargs: Any) -> Any:
    return _catalog_cache.get_or_fetch(
        key,
        lambda: akshare_upstream.call(function_name, **kwargs),
    )


def _frame_rows(frame: Any) -> Iterable[dict[str, Any]]:
    return (row for _index, row in _frame_rows_with_index(frame))


def _frame_is_empty(frame: Any) -> bool:
    if frame is None:
        return True
    if isinstance(frame, (list, tuple)):
        return not frame
    return bool(getattr(frame, "empty", True))


def _frame_rows_with_index(frame: Any) -> Iterable[tuple[Any, dict[str, Any]]]:
    if _frame_is_empty(frame):
        return []
    if isinstance(frame, (list, tuple)):
        return (
            (index, dict(row))
            for index, row in enumerate(frame)
            if isinstance(row, Mapping)
        )
    return ((index, dict(row)) for index, row in frame.iterrows())


def _row_value(row: Mapping[str, Any], *names: str) -> Any:
    normalized = {_column_key(key): value for key, value in row.items()}
    for name in names:
        value = normalized.get(_column_key(name))
        if value is not None:
            return value
    return None


def _column_key(value: Any) -> str:
    return str(value).strip().lower().replace(" ", "").replace("_", "")


def _optional_decimal(
    row: Mapping[str, Any],
    *names: str,
    minimum: Decimal | None = None,
) -> Decimal | None:
    value = _row_value(row, *names)
    if value is None or isinstance(value, bool):
        return None
    try:
        result = Decimal(str(value).strip())
    except (InvalidOperation, ValueError):
        return None
    if not result.is_finite() or (minimum is not None and result < minimum):
        return None
    return result


def _required_decimal(row: Mapping[str, Any], *names: str) -> Decimal:
    value = _optional_decimal(row, *names, minimum=Decimal(0))
    if value is None or value <= 0:
        raise not_found("snapshot_not_found", "snapshot does not contain a valid price")
    return value


def _optional_price(row: Mapping[str, Any], *names: str) -> Decimal | None:
    value = _optional_decimal(row, *names, minimum=Decimal(0))
    return value if value is not None and value > 0 else None


def _decimal_text(value: Decimal | None) -> str | None:
    if value is None:
        return None
    rendered = format(value, "f")
    if "." in rendered:
        rendered = rendered.rstrip("0").rstrip(".")
    return rendered or "0"


def _row_timestamp(row: Mapping[str, Any], timezone_name: str) -> str | None:
    value = _row_value(
        row,
        "更新时间",
        "最新时间",
        "最新行情时间",
        "时间",
        "timestamp",
    )
    if isinstance(value, str) and not re.search(r"(?:T|\s)\d{1,2}:\d{2}", value.strip()):
        return None
    parsed = timestamp_as_utc(value, timezone_name)
    return format_rfc3339(parsed) if parsed is not None else None


def _normalize_market(market: str) -> str:
    token = market.strip().upper()
    for code, spec in MARKET_SPECS.items():
        if token == code or token in spec.aliases:
            return code
    raise invalid_request("unsupported_market", f"unsupported market: {token or market}")


def _stock_symbols(market: str, raw_code: str) -> tuple[str | None, str]:
    token = raw_code.strip().upper()
    if market == "US":
        prefix, separator, suffix = token.partition(".")
        symbol = suffix if separator and prefix.isdigit() else token
        return (symbol if CODE_PATTERN.fullmatch(symbol) else None), token
    if market == "HK":
        if not token.isdigit():
            return None, token
        return f"{int(token):05d}", f"{int(token):05d}"
    if not re.fullmatch(r"\d{6}", token):
        return None, token
    return token, token


def _etf_market(code: str) -> str:
    return "SH" if code.startswith(("5", "6")) else "SZ"


def _hk_index_symbol(code: str, name: str | None) -> str | None:
    for token in (code.strip().upper(), (name or "").replace(" ", "").upper()):
        if token in HK_INDEX_IDS:
            return HK_INDEX_IDS[token]
    if code.isdigit():
        return f"{int(code):05d}"
    token = code.strip().upper()
    return token if CODE_PATTERN.fullmatch(token) else None


def _us_index_symbol(code: str | None, name: str | None) -> str | None:
    tokens = [
        (code or "").replace(" ", "").replace("&", "").upper(),
        (name or "").replace(" ", "").replace("&", "").upper(),
    ]
    for token in tokens:
        if token in US_INDEX_IDS:
            return US_INDEX_IDS[token]
        for alias, symbol in US_INDEX_IDS.items():
            if alias and alias in token:
                return symbol
    return None


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


def _intraday_bound(value: datetime, timezone_name: str) -> str:
    return value.astimezone(ZoneInfo(timezone_name)).strftime("%Y-%m-%d %H:%M:%S")


def _daily_bound(value: datetime, timezone_name: str) -> str:
    return value.astimezone(ZoneInfo(timezone_name)).strftime("%Y%m%d")


def _history_symbol(instrument: AKInstrument) -> str:
    """Strip the private CN series key only at the AKShare call boundary."""
    _series, separator, symbol = instrument.upstream_symbol.partition(":")
    return symbol if separator else instrument.upstream_symbol


def _validate_snapshot_ohlc(
    price: Decimal,
    open_price: Decimal | None,
    high: Decimal | None,
    low: Decimal | None,
) -> None:
    if high is not None and low is not None and high < low:
        raise SidecarError(
            502,
            "AKSHARE_SCHEMA_ERROR",
            "AKShare snapshot contains inconsistent OHLC",
        )
    comparable = [price]
    if open_price is not None:
        comparable.append(open_price)
    if high is not None and high < max(comparable):
        raise SidecarError(
            502,
            "AKSHARE_SCHEMA_ERROR",
            "AKShare snapshot contains inconsistent OHLC",
        )
    if low is not None and low > min(comparable):
        raise SidecarError(
            502,
            "AKSHARE_SCHEMA_ERROR",
            "AKShare snapshot contains inconsistent OHLC",
        )


def _utc_now() -> datetime:
    return datetime.now(UTC)
