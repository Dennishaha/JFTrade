"""Small, patchable boundary around blocking yfinance calls."""

from __future__ import annotations

import threading
import time
from datetime import datetime
from typing import Any

import yfinance as yf
from curl_cffi import requests

UPSTREAM_TIMEOUT_SECONDS = 10
UPSTREAM_IMPERSONATE = "chrome"
SNAPSHOT_CACHE_SECONDS = 15
SECURITY_CACHE_SECONDS = 86400


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


# Yahoo frequently rate-limits the default curl_cffi fingerprint. Use a
# stable browser profile for all yfinance requests while keeping the
# transport local to this sidecar.
_SESSION = _BoundedSession(
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


def search_quotes(query: str, limit: int) -> list[dict[str, Any]]:
    search = yf.Search(
        query,
        max_results=limit,
        news_count=0,
        session=_SESSION,
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
    return _ticker_info_cache.get_or_fetch(
        symbol,
        max_age_seconds,
        lambda: yf.Ticker(symbol, session=_SESSION).get_info(),
    )


def ticker_history(
    symbol: str,
    *,
    interval: str,
    fetch_period: str,
    start: datetime | None,
    end: datetime | None,
    prepost: bool = True,
) -> Any:
    options: dict[str, Any] = {
        "interval": interval,
        "prepost": prepost,
        "auto_adjust": False,
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
    return yf.Ticker(symbol, session=_SESSION).history(**options)
