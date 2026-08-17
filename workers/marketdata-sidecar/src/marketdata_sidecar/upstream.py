"""Small, patchable boundary around blocking yfinance calls."""

from __future__ import annotations

import importlib
import threading
import time
from dataclasses import dataclass
from datetime import datetime
from typing import Any, Literal

from .conversion import clean_text, finite_float, timestamp_as_utc
from .errors import SidecarError

UPSTREAM_TIMEOUT_SECONDS = 10
UPSTREAM_IMPERSONATE = "chrome"
SNAPSHOT_CACHE_SECONDS = 15
SECURITY_CACHE_SECONDS = 86400
NEWS_CACHE_SECONDS = 300
ACTIONS_CACHE_SECONDS = 3600
SCREEN_CACHE_SECONDS = 60
RESEARCH_CACHE_SECONDS = 3600


RuntimeState = Literal["warming", "ready", "failed"]


@dataclass(frozen=True)
class RuntimeSnapshot:
    state: RuntimeState
    error: str = ""


@dataclass(frozen=True)
class _RuntimeComponents:
    yfinance: Any
    session: Any


_runtime_lock = threading.Lock()
_runtime_started = False
_runtime_snapshot = RuntimeSnapshot("warming")
_runtime_components: _RuntimeComponents | None = None
_runtime_thread: threading.Thread | None = None


def runtime_snapshot() -> RuntimeSnapshot:
    with _runtime_lock:
        return _runtime_snapshot


def request_runtime_warmup() -> RuntimeSnapshot:
    """Start the Yahoo import exactly once without blocking the HTTP thread."""
    global _runtime_thread
    with _runtime_lock:
        if _runtime_started:
            return _runtime_snapshot
        if _runtime_thread is None or not _runtime_thread.is_alive():
            _runtime_thread = threading.Thread(
                target=warm_runtime,
                name="yfinance-runtime-warmup",
                daemon=True,
            )
            _runtime_thread.start()
        return _runtime_snapshot


def warm_runtime() -> None:
    """Import the heavy Yahoo stack once without blocking process health."""
    global _runtime_started, _runtime_snapshot, _runtime_components
    with _runtime_lock:
        if _runtime_started:
            return
        _runtime_started = True
    try:
        yf = importlib.import_module("yfinance")
        requests = importlib.import_module("curl_cffi.requests")
        components = _RuntimeComponents(
            yfinance=yf,
            session=_build_session(requests),
        )
    except Exception as exc:
        with _runtime_lock:
            _runtime_snapshot = RuntimeSnapshot(
                "failed",
                f"{type(exc).__name__}: {exc}",
            )
        return
    with _runtime_lock:
        _runtime_components = components
        _runtime_snapshot = RuntimeSnapshot("ready")


def require_runtime() -> _RuntimeComponents:
    snapshot = runtime_snapshot()
    if snapshot.state == "warming":
        raise SidecarError(
            503,
            "YFINANCE_RUNTIME_WARMING",
            "Yahoo Finance runtime is warming up",
        )
    if snapshot.state == "failed":
        raise SidecarError(
            503,
            "YFINANCE_RUNTIME_FAILED",
            "Yahoo Finance runtime failed to initialize",
        )
    with _runtime_lock:
        components = _runtime_components
    if components is None:
        raise SidecarError(
            503,
            "YFINANCE_RUNTIME_FAILED",
            "Yahoo Finance runtime is unavailable",
        )
    return components


def _build_session(requests: Any) -> Any:
    class _BoundedSession(requests.Session):
        """Clamp every yfinance transport request to a finite upper bound."""

        def request(
            self,
            method: Any,
            url: str,
            *args: Any,
            **kwargs: Any,
        ) -> Any:
            requested = kwargs.get("timeout")
            if (
                not isinstance(requested, (int, float))
                or requested <= 0
                or requested > UPSTREAM_TIMEOUT_SECONDS
            ):
                kwargs["timeout"] = UPSTREAM_TIMEOUT_SECONDS
            return super().request(method, url, *args, **kwargs)

    return _BoundedSession(
        impersonate=UPSTREAM_IMPERSONATE,
        timeout=UPSTREAM_TIMEOUT_SECONDS,
    )


class _TickerInfoCache:
    """Per-symbol in-process cache for ticker_info results.

    Callers specify ``max_age_seconds`` to declare how stale they are willing
    to accept.  The cache stores data with the monotonic timestamp it was
    fetched; a caller with a *shorter* max_age will bypass a stale entry and
    refresh, making the freshened result available to subsequent callers with
    a *longer* max_age.  No expiry housekeeping is needed for a local
    single-process sidecar with a small symbol universe.
    """

    def __init__(self) -> None:
        self._lock = threading.Lock()
        # symbol → (data, fetched_at_monotonic)
        self._store: dict[str, tuple[dict[str, Any], float]] = {}
        self._inflight: dict[str, threading.Event] = {}

    def get(self, symbol: str, max_age_seconds: int) -> dict[str, Any] | None:
        with self._lock:
            entry = self._store.get(symbol)
        if entry is None:
            return None
        data, fetched_at = entry
        if time.monotonic() - fetched_at < max_age_seconds:
            return data
        return None

    def set(self, symbol: str, data: dict[str, Any]) -> None:
        with self._lock:
            self._store[symbol] = (data, time.monotonic())

    def clear(self) -> None:
        with self._lock:
            self._store.clear()

    def get_or_fetch(
        self,
        symbol: str,
        max_age_seconds: int,
        fetch: Any,
    ) -> dict[str, Any]:
        """Fetch one symbol once when concurrent callers miss the cache."""
        while True:
            cached = self.get(symbol, max_age_seconds)
            if cached is not None:
                return cached
            with self._lock:
                event = self._inflight.get(symbol)
                if event is None:
                    event = threading.Event()
                    self._inflight[symbol] = event
                    owner = True
                else:
                    owner = False
            if owner:
                break
            # A failed owner leaves no cache entry; loop once it has finished
            # so a waiting request can retry with the same bounded timeout.
            event.wait(timeout=UPSTREAM_TIMEOUT_SECONDS + 1)

        try:
            data = dict(fetch() or {})
            self.set(symbol, data)
            return data
        finally:
            with self._lock:
                self._inflight.pop(symbol, None)
                event.set()


_ticker_info_cache = _TickerInfoCache()
_ticker_fast_info_cache = _TickerInfoCache()
_ticker_news_cache = _TickerInfoCache()
_ticker_actions_cache = _TickerInfoCache()
_screener_cache = _TickerInfoCache()
_screen_custom_cache = _TickerInfoCache()


def screen_quotes(query_id: str, count: int) -> list[dict[str, Any]]:
    """Return cached Yahoo predefined-screener quotes.

    yfinance 1.6.0 exposes predefined screens through the module-level
    ``yfinance.screen`` function (there is no ``Screener`` class in this
    version); ``query_id`` is one of ``PREDEFINED_SCREENER_QUERIES``.
    """
    runtime = require_runtime()
    data = _screener_cache.get_or_fetch(
        f"{query_id}:{count}",
        SCREEN_CACHE_SECONDS,
        lambda: {"quotes": _fetch_screen_quotes(runtime, query_id, count)},
    )
    return list(data.get("quotes") or [])


def _fetch_screen_quotes(
    runtime: _RuntimeComponents,
    query_id: str,
    count: int,
) -> list[dict[str, Any]]:
    result = runtime.yfinance.screen(
        query_id,
        count=count,
        session=runtime.session,
    )
    quotes = result.get("quotes") if isinstance(result, dict) else None
    if not isinstance(quotes, list):
        raise SidecarError(
            502,
            "YFINANCE_SCHEMA_ERROR",
            "Yahoo Finance screener response has an invalid schema",
        )
    return [dict(quote) for quote in quotes if isinstance(quote, dict)]


def screen_custom(
    conditions: list[tuple[str, str, tuple[Any, ...]]],
    sort_field: str | None,
    sort_asc: bool,
    size: int,
) -> dict[str, Any]:
    """Run a Yahoo custom equity screen and return the raw result dict.

    ``conditions`` are ``(operator, field, values)`` triples already
    translated into EquityQuery operator names (EQ/BTWN/GTE/LTE); the
    yfinance boundary owns query object construction so callers never import
    yfinance.  ``size`` is the upstream page window (Yahoo caps it at 250);
    the caller slices offset/limit locally from the returned quotes.
    """
    runtime = require_runtime()
    key = repr((conditions, sort_field, sort_asc, size))
    data = _screen_custom_cache.get_or_fetch(
        key,
        SCREEN_CACHE_SECONDS,
        lambda: {
            "result": _fetch_custom_screen(runtime, conditions, sort_field, sort_asc, size)
        },
    )
    return dict(data.get("result") or {})


def _fetch_custom_screen(
    runtime: _RuntimeComponents,
    conditions: list[tuple[str, str, tuple[Any, ...]]],
    sort_field: str | None,
    sort_asc: bool,
    size: int,
) -> dict[str, Any]:
    equity_query = runtime.yfinance.EquityQuery
    queries = [
        equity_query(operator, [field, *values])
        for operator, field, values in conditions
    ]
    if not queries:
        raise SidecarError(
            400,
            "invalid_request",
            "custom screen requires at least one condition",
        )
    query = queries[0] if len(queries) == 1 else equity_query("AND", queries)
    result = runtime.yfinance.screen(
        query,
        size=size,
        sortField=sort_field,
        sortAsc=sort_asc,
        session=runtime.session,
    )
    quotes = result.get("quotes") if isinstance(result, dict) else None
    if not isinstance(quotes, list):
        raise SidecarError(
            502,
            "YFINANCE_SCHEMA_ERROR",
            "Yahoo Finance screener response has an invalid schema",
        )
    return result


_ticker_financials_cache = _TickerInfoCache()
_ticker_analyst_cache = _TickerInfoCache()
_ticker_ownership_cache = _TickerInfoCache()

# Ticker financial statement property per statement selector.
_FINANCIAL_ACCESSORS = {
    "income": "income_stmt",
    "balance": "balance_sheet",
    "cashflow": "cashflow",
}


def ticker_financials(symbol: str, statement: str) -> dict[str, Any]:
    """Return cached yearly statement data as plain periods/rows records."""
    runtime = require_runtime()
    return _ticker_financials_cache.get_or_fetch(
        f"{symbol}:{statement}",
        RESEARCH_CACHE_SECONDS,
        lambda: _fetch_financials(runtime, symbol, statement),
    )


def _fetch_financials(
    runtime: _RuntimeComponents,
    symbol: str,
    statement: str,
) -> dict[str, Any]:
    ticker = runtime.yfinance.Ticker(symbol, session=runtime.session)
    frame = getattr(ticker, _FINANCIAL_ACCESSORS[statement], None)
    if frame is None or getattr(frame, "empty", True):
        return {"periods": [], "rows": {}}
    periods = [
        column.date().isoformat() if hasattr(column, "date") else str(column)
        for column in frame.columns
    ]
    rows = {
        str(label): [_plain_value(value) for value in series.tolist()]
        for label, series in frame.iterrows()
    }
    return {"periods": periods, "rows": rows}


def ticker_analyst(symbol: str) -> dict[str, Any]:
    """Return cached recommendation-trend rows and analyst price targets."""
    runtime = require_runtime()
    return _ticker_analyst_cache.get_or_fetch(
        symbol,
        RESEARCH_CACHE_SECONDS,
        lambda: _fetch_analyst(runtime, symbol),
    )


def _fetch_analyst(runtime: _RuntimeComponents, symbol: str) -> dict[str, Any]:
    ticker = runtime.yfinance.Ticker(symbol, session=runtime.session)
    trend = getattr(ticker, "recommendations", None)
    records = (
        [
            {str(key): _plain_value(value) for key, value in row.items()}
            for _index, row in trend.iterrows()
        ]
        if trend is not None and not getattr(trend, "empty", True)
        else []
    )
    targets = getattr(ticker, "analyst_price_targets", None) or {}
    return {
        "trend": records,
        "targets": {
            str(key): finite_float(value) for key, value in targets.items()
        },
    }


def ticker_ownership(symbol: str) -> dict[str, list[dict[str, Any]]]:
    """Return cached major/institutional/mutualfund holder records."""
    runtime = require_runtime()
    return _ticker_ownership_cache.get_or_fetch(
        symbol,
        RESEARCH_CACHE_SECONDS,
        lambda: _fetch_ownership(runtime, symbol),
    )


def _fetch_ownership(
    runtime: _RuntimeComponents,
    symbol: str,
) -> dict[str, list[dict[str, Any]]]:
    ticker = runtime.yfinance.Ticker(symbol, session=runtime.session)
    return {
        "major": _holder_records(getattr(ticker, "major_holders", None)),
        "institutional": _holder_records(getattr(ticker, "institutional_holders", None)),
        "mutualfund": _holder_records(getattr(ticker, "mutualfund_holders", None)),
    }


def _holder_records(frame: Any) -> list[dict[str, Any]]:
    if frame is None or getattr(frame, "empty", True):
        return []
    records: list[dict[str, Any]] = []
    for index, row in frame.iterrows():
        record = {str(key): _plain_value(value) for key, value in row.items()}
        record["label"] = str(index)
        records.append(record)
    return records


def _plain_value(value: Any) -> Any:
    if hasattr(value, "date") and not isinstance(value, str):
        try:
            return value.date().isoformat()
        except (TypeError, ValueError):
            return None
    number = finite_float(value)
    if number is not None:
        return number
    return clean_text(value)


def ticker_fast_info(symbol: str) -> dict[str, Any] | None:
    """Return Yahoo-keyed quote metadata from ``fast_info``, or None.

    The fast path is only usable when it can supply a price plus the
    exchange/quote-type fields the snapshot validators require.  A ``None``
    result (missing keys, upstream failure, or an unavailable runtime) sends
    the caller to the regular :func:`ticker_info` path, which owns the
    contractual error reporting.
    """
    try:
        runtime = require_runtime()
    except SidecarError:
        return None
    data = _ticker_fast_info_cache.get_or_fetch(
        symbol,
        SNAPSHOT_CACHE_SECONDS,
        lambda: _fetch_fast_info(runtime, symbol),
    )
    return data or None


def _fetch_fast_info(runtime: _RuntimeComponents, symbol: str) -> dict[str, Any]:
    try:
        fast = runtime.yfinance.Ticker(
            symbol,
            session=runtime.session,
        ).get_fast_info()
        mapped: dict[str, Any] = {}
        for fast_key, info_key in FAST_INFO_KEY_MAP.items():
            try:
                value = fast[fast_key]
            except Exception:
                continue
            if value is not None:
                mapped[info_key] = value
    except Exception:
        return {}
    if finite_float(mapped.get("regularMarketPrice")) is None:
        return {}
    if not clean_text(mapped.get("quoteType")) or not clean_text(
        mapped.get("exchange")
    ):
        return {}
    return mapped


def ticker_news(symbol: str, limit: int) -> list[dict[str, Any]]:
    """Return cached Yahoo news items for one ticker."""
    runtime = require_runtime()
    data = _ticker_news_cache.get_or_fetch(
        f"{symbol}:{limit}",
        NEWS_CACHE_SECONDS,
        lambda: {
            "items": runtime.yfinance.Ticker(
                symbol,
                session=runtime.session,
            ).get_news(count=limit)
            or []
        },
    )
    return list(data.get("items") or [])


def ticker_actions(symbol: str) -> dict[str, list[dict[str, Any]]]:
    """Return cached dividend/split points as plain dated records."""
    runtime = require_runtime()
    return _ticker_actions_cache.get_or_fetch(
        symbol,
        ACTIONS_CACHE_SECONDS,
        lambda: _fetch_actions(runtime, symbol),
    )


def _fetch_actions(
    runtime: _RuntimeComponents,
    symbol: str,
) -> dict[str, list[dict[str, Any]]]:
    ticker = runtime.yfinance.Ticker(symbol, session=runtime.session)
    return {
        "dividends": _series_points(ticker.dividends),
        "splits": _series_points(ticker.splits),
    }


def _series_points(series: Any) -> list[dict[str, Any]]:
    points: list[dict[str, Any]] = []
    for index, value in getattr(series, "items", lambda: [])():
        stamp = timestamp_as_utc(index)
        number = finite_float(value)
        if stamp is None or number is None:
            continue
        points.append({"date": stamp.date().isoformat(), "value": number})
    return points

# fast_info keys mapped onto the regular get_info key names so the snapshot
# route keeps a single projection for both accessors.
FAST_INFO_KEY_MAP = {
    "last_price": "regularMarketPrice",
    "previous_close": "regularMarketPreviousClose",
    "open": "regularMarketOpen",
    "day_high": "regularMarketDayHigh",
    "day_low": "regularMarketDayLow",
    "last_volume": "regularMarketVolume",
    "market_cap": "marketCap",
    "currency": "currency",
    "exchange": "exchange",
    "quote_type": "quoteType",
    "timezone": "exchangeTimezoneName",
}


def search_quotes(query: str, limit: int) -> list[dict[str, Any]]:
    runtime = require_runtime()
    search = runtime.yfinance.Search(
        query,
        max_results=limit,
        news_count=0,
        session=runtime.session,
        timeout=UPSTREAM_TIMEOUT_SECONDS,
    )
    return list(search.quotes or [])


def ticker_info(
    symbol: str,
    max_age_seconds: int = SECURITY_CACHE_SECONDS,
) -> dict[str, Any]:
    """Return cached ticker info, fetching from Yahoo Finance on a cache miss.

    ``max_age_seconds`` controls how old a cached result can be before this
    caller triggers a fresh fetch. Snapshot routes use
    :data:`SNAPSHOT_CACHE_SECONDS`; security details use
    :data:`SECURITY_CACHE_SECONDS`. Concurrent misses for the same Yahoo
    ticker share one upstream request.
    """
    runtime = require_runtime()
    return _ticker_info_cache.get_or_fetch(
        symbol,
        max_age_seconds,
        lambda: runtime.yfinance.Ticker(
            symbol,
            session=runtime.session,
        ).get_info(),
    )


def ticker_history(
    symbol: str,
    *,
    interval: str,
    fetch_period: str,
    start: datetime | None,
    end: datetime | None,
    prepost: bool = True,
    auto_adjust: bool = False,
) -> Any:
    runtime = require_runtime()
    options: dict[str, Any] = {
        "interval": interval,
        "prepost": prepost,
        "auto_adjust": auto_adjust,
        "actions": False,
        "repair": False,
        "timeout": UPSTREAM_TIMEOUT_SECONDS,
        # yfinance otherwise logs transport, parsing, rate-limit, and timezone
        # failures and returns an empty frame, which would be misreported as a
        # legitimate 404 by the HTTP route.
        "raise_errors": True,
    }
    if start is None and end is None:
        options["period"] = fetch_period
    else:
        # Ticker.history defaults period to "1mo". Explicitly disable it so a
        # bounded request always honors start/end, especially an older page
        # supplied through an end boundary.
        options["period"] = None
        if start is not None:
            options["start"] = start
        if end is not None:
            options["end"] = end
    return runtime.yfinance.Ticker(
        symbol,
        session=runtime.session,
    ).history(**options)
