"""Lazy, bounded execution boundary around blocking AKShare calls."""

from __future__ import annotations

import importlib
import json
import os
import re
import threading
from concurrent.futures import ThreadPoolExecutor, TimeoutError as FutureTimeout
from dataclasses import dataclass
from typing import Any
from urllib.parse import parse_qsl, urlencode, urlsplit, urlunsplit

from .errors import SidecarError, service_unavailable
from .upstream import RuntimeSnapshot

MAX_WORKERS = 4
CALL_TIMEOUT_SECONDS = 12
EASTMONEY_TOKEN = "bd1d9ddb04089700cf9c27f6f7426281"
EASTMONEY_SEARCH_TOKEN = "D43BF722C8E33BDC906FB84D85E326E8"
# Field codes follow Eastmoney's push2 list-endpoint conventions (ulist/clist),
# which differ from the single-quote ``stock/get`` endpoint: f9 is the dynamic
# PE ratio, f20/f21 are total/float market cap, f23 is PB, and f31/f32 are the
# level-1 bid/ask prices.  Unserved markets return ``-`` and normalize to None.
EASTMONEY_SPOT_FIELDS = (
    "f1,f2,f3,f4,f5,f6,f9,f12,f13,f14,f15,f16,f17,f18,f20,f21,f23,f31,f32"
)
SINA_US_MINUTE_URL = (
    "https://stock.finance.sina.com.cn/usstock/api/jsonp.php/"
    "var%20jftrade_us_minutes=/US_MinKService.getMinK"
)
EASTMONEY_HK_MINUTE_URL = (
    "https://push2delay.eastmoney.com/api/qt/stock/trends2/get"
)
SINA_US_MINUTE_CALLBACK = "jftrade_us_minutes"


@dataclass(frozen=True)
class _RuntimeComponents:
    akshare: Any


_runtime_lock = threading.Lock()
_runtime_started = False
_runtime_snapshot = RuntimeSnapshot("warming")
_runtime_components: _RuntimeComponents | None = None
_runtime_thread: threading.Thread | None = None
_executor = ThreadPoolExecutor(
    max_workers=MAX_WORKERS,
    thread_name_prefix="akshare-upstream",
)
_slots = threading.BoundedSemaphore(MAX_WORKERS)
_request_state = threading.local()


def runtime_snapshot() -> RuntimeSnapshot:
    with _runtime_lock:
        return _runtime_snapshot


def request_runtime_warmup() -> RuntimeSnapshot:
    """Begin the import in a daemon thread and return the current state."""
    global _runtime_thread
    with _runtime_lock:
        if _runtime_started:
            return _runtime_snapshot
        if _runtime_thread is None or not _runtime_thread.is_alive():
            _runtime_thread = threading.Thread(
                target=warm_runtime,
                name="akshare-runtime-warmup",
                daemon=True,
            )
            _runtime_thread.start()
        return _runtime_snapshot


def warm_runtime() -> None:
    """Import AKShare once. Importing does not perform a market-data request."""
    global _runtime_started, _runtime_snapshot, _runtime_components
    with _runtime_lock:
        if _runtime_started:
            return
        _runtime_started = True
    try:
        module = importlib.import_module("akshare")
        if hasattr(module, "__path__"):
            _configure_akshare_transport()
    except Exception as exc:
        with _runtime_lock:
            _runtime_snapshot = RuntimeSnapshot(
                "failed",
                f"{type(exc).__name__}: {exc}",
            )
        return
    with _runtime_lock:
        _runtime_components = _RuntimeComponents(akshare=module)
        _runtime_snapshot = RuntimeSnapshot("ready")


def _configure_akshare_transport() -> None:
    """Use the sidecar's browser-compatible transport when no proxy is set.

    Several Eastmoney endpoints close plain CPython ``requests`` connections
    on macOS while accepting the same request from curl/libcurl.  AKShare
    imports requests internally, so a tiny local shim is installed only for
    that module.  Explicit proxy environment variables retain the upstream
    requests behavior; yfinance uses its own curl_cffi session and is not
    affected by this patch.
    """
    proxy_names = (
        "HTTP_PROXY",
        "HTTPS_PROXY",
        "ALL_PROXY",
        "http_proxy",
        "https_proxy",
        "all_proxy",
    )
    proxy_configured = any(os.environ.get(name, "").strip() for name in proxy_names)
    try:
        import requests as requests_compat
    except Exception:
        return
    if getattr(requests_compat.Session, "_jftrade_akshare_transport", False):
        return

    if proxy_configured:
        # Keep requests' normal proxy behavior when the caller explicitly
        # supplied a proxy, but still route AKShare's retired catalog path to
        # Eastmoney's current guest endpoint.
        original_session = requests_compat.Session

        class _RewritingSession(original_session):  # type: ignore[misc,valid-type]
            _jftrade_akshare_transport = True

            def request(self, method: str, url: str, **kwargs: Any) -> Any:
                return super().request(method, _rewrite_eastmoney_url(url), **kwargs)

        requests_compat.Session = _RewritingSession  # type: ignore[assignment]
        original_get = requests_compat.get

        def rewriting_get(url: str, **kwargs: Any) -> Any:
            return original_get(_rewrite_eastmoney_url(url), **kwargs)

        requests_compat.get = rewriting_get  # type: ignore[assignment]
        return

    try:
        from curl_cffi import Curl, CurlOpt, requests as curl_requests
    except Exception:
        return

    request_error = requests_compat.RequestException

    class _DirectSession:
        _jftrade_akshare_transport = True

        def __init__(self, *_args: Any, **_kwargs: Any) -> None:
            self._sessions = {
                1: self._new_session(Curl, CurlOpt, curl_requests, 1),
                2: self._new_session(Curl, CurlOpt, curl_requests, 2),
            }

        @staticmethod
        def _new_session(curl_type: Any, curl_opt: Any, requests_type: Any, family: int) -> Any:
            curl = curl_type()
            # Eastmoney's realtime nodes are healthy over IPv4 on macOS,
            # while historical push2his nodes may only be reachable on IPv6.
            curl.setopt(curl_opt.IPRESOLVE, family)
            return requests_type.Session(
                curl=curl,
                use_thread_local_curl=False,
                impersonate="chrome",
                trust_env=False,
            )

        def __enter__(self) -> _DirectSession:
            return self

        def __exit__(self, *_args: Any) -> None:
            self.close()

        def mount(self, *_args: Any, **_kwargs: Any) -> None:
            return None

        def request(self, method: str, url: str, **kwargs: Any) -> Any:
            try:
                host = (urlsplit(url).hostname or "").lower()
                family = 2 if "push2his" in host else 1
                return self._sessions[family].request(
                    method,
                    _rewrite_eastmoney_url(url),
                    **kwargs,
                )
            except Exception as exc:
                raise request_error(str(exc)) from exc

        def get(self, url: str, **kwargs: Any) -> Any:
            return self.request("GET", url, **kwargs)

        def close(self) -> None:
            for session in self._sessions.values():
                session.close()

    def direct_get(url: str, **kwargs: Any) -> Any:
        with _DirectSession() as session:
            return session.get(url, **kwargs)

    requests_compat.Session = _DirectSession  # type: ignore[assignment]
    requests_compat.get = direct_get  # type: ignore[assignment]


def _rewrite_eastmoney_url(url: str) -> str:
    """Use Eastmoney's current guest catalog endpoint for AKShare 1.18.91.

    Eastmoney now closes the legacy public ``/api/qt/clist/get`` route for
    unauthenticated requests.  The web frontend uses the same data service
    through ``/webguest/api/qt/clist/get``.  Only numbered ``push2`` hosts
    and their shared alias are rewritten; historical, delay, and non-catalog
    endpoints retain their original URL.
    """
    parsed = urlsplit(url)
    host = (parsed.hostname or "").lower()
    if parsed.path != "/api/qt/clist/get":
        return url
    if not (
        host == "push2.eastmoney.com"
        or host.endswith(".push2.eastmoney.com")
    ):
        return url
    query = parse_qsl(parsed.query, keep_blank_values=True)
    if not any(key.lower() == "timil" for key, _value in query):
        query.append(("timil", "1"))
    return urlunsplit(
        parsed._replace(
            path="/webguest/api/qt/clist/get",
            query=urlencode(query),
        )
    )


def require_runtime() -> _RuntimeComponents:
    runtime = runtime_snapshot()
    if runtime.state == "warming":
        raise service_unavailable(
            "AKSHARE_RUNTIME_WARMING",
            "akshare runtime is warming up",
        )
    if runtime.state == "failed":
        raise service_unavailable(
            "AKSHARE_RUNTIME_FAILED",
            "akshare runtime failed to initialize",
        )
    with _runtime_lock:
        components = _runtime_components
    if components is None:
        raise service_unavailable(
            "AKSHARE_RUNTIME_FAILED",
            "akshare runtime is unavailable",
        )
    return components


def call(function_name: str, /, **kwargs: Any) -> Any:
    """Call AKShare inside an already bounded request worker."""
    ensure_request_active()
    components = require_runtime()
    function = getattr(components.akshare, function_name, None)
    if function is None:
        raise SidecarError(
            502,
            "AKSHARE_SCHEMA_ERROR",
            f"AKShare function is unavailable: {function_name}",
        )
    result = function(**kwargs)
    ensure_request_active()
    return result


def us_minute_rows(symbol: str) -> list[dict[str, Any]]:
    """Fetch Sina's real one-minute US OHLCV rows."""
    ensure_request_active()
    require_runtime()
    import requests

    response = requests.get(
        SINA_US_MINUTE_URL,
        params={"symbol": symbol, "type": "1"},
        timeout=10,
    )
    response.raise_for_status()
    match = re.fullmatch(
        rf"\s*(?:/\*.*?\*/\s*)?var\s+{SINA_US_MINUTE_CALLBACK}"
        rf"\s*=\s*\((.*)\)\s*;\s*",
        response.text,
        flags=re.DOTALL,
    )
    if match is None:
        raise _schema_error("Sina US minute response is not valid JSONP")
    try:
        payload = json.loads(match.group(1))
    except (TypeError, ValueError) as exc:
        raise _schema_error("Sina US minute response contains invalid JSON") from exc
    if not isinstance(payload, list) or not all(isinstance(row, dict) for row in payload):
        raise _schema_error("Sina US minute response has an invalid schema")
    rows = [_normalize_sina_minute_row(row) for row in payload]
    ensure_request_active()
    return rows


def hk_minute_rows(symbol: str) -> list[dict[str, Any]]:
    """Fetch Eastmoney's currently reachable one-minute HK OHLCV rows."""
    ensure_request_active()
    require_runtime()
    import requests

    response = requests.get(
        EASTMONEY_HK_MINUTE_URL,
        params={
            "fields1": "f1,f2,f3,f4,f5,f6,f7,f8,f9,f10,f11,f12,f13",
            "fields2": "f51,f52,f53,f54,f55,f56,f57,f58",
            "ut": EASTMONEY_TOKEN,
            "iscr": "0",
            "ndays": "5",
            "secid": f"116.{symbol}",
        },
        timeout=10,
    )
    response.raise_for_status()
    payload = response.json()
    data = payload.get("data") if isinstance(payload, dict) else None
    trends = data.get("trends") if isinstance(data, dict) else None
    if not isinstance(trends, list) or not all(isinstance(row, str) for row in trends):
        raise _schema_error("Eastmoney HK minute response has an invalid schema")
    rows = [_normalize_eastmoney_minute_row(row) for row in trends]
    ensure_request_active()
    return rows


def _normalize_sina_minute_row(row: dict[str, Any]) -> dict[str, Any]:
    required = ("d", "o", "h", "l", "c")
    if any(key not in row for key in required):
        raise _schema_error("Sina US minute row is missing OHLC fields")
    return {
        "时间": row["d"],
        "开盘": row["o"],
        "最高": row["h"],
        "最低": row["l"],
        "收盘": row["c"],
        "成交量": row.get("v"),
        "成交额": row.get("a"),
    }


def _normalize_eastmoney_minute_row(row: str) -> dict[str, Any]:
    fields = row.split(",")
    if len(fields) < 7:
        raise _schema_error("Eastmoney HK minute row is missing OHLCV fields")
    return {
        "时间": fields[0],
        "开盘": fields[1],
        "收盘": fields[2],
        "最高": fields[3],
        "最低": fields[4],
        "成交量": fields[5],
        "成交额": fields[6],
    }


def _schema_error(message: str) -> SidecarError:
    return SidecarError(502, "AKSHARE_SCHEMA_ERROR", message)


def spot_rows(market: str, symbols: list[str]) -> list[dict[str, Any]]:
    """Fetch one delayed Eastmoney batch used by AKShare spot functions."""
    ensure_request_active()
    require_runtime()
    secids = _spot_secids(market, symbols)
    if not secids:
        return []
    import requests

    response = requests.get(
        "https://push2.eastmoney.com/api/qt/ulist.np/get",
        params={
            "fltt": "2",
            "invt": "2",
            "ut": EASTMONEY_TOKEN,
            "fields": EASTMONEY_SPOT_FIELDS,
            "secids": ",".join(secids),
        },
        timeout=10,
    )
    response.raise_for_status()
    payload = response.json()
    data = payload.get("data") if isinstance(payload, dict) else None
    rows = data.get("diff") if isinstance(data, dict) else None
    if rows is None:
        return []
    if not isinstance(rows, list) or not all(isinstance(row, dict) for row in rows):
        raise ValueError("Eastmoney batch spot response has an invalid schema")
    ensure_request_active()
    return [_normalize_spot_row(row) for row in rows]


def search_rows(query: str) -> list[dict[str, Any]]:
    """Query Eastmoney's current instrument suggestion directory."""
    ensure_request_active()
    require_runtime()
    import requests

    response = requests.get(
        "https://searchapi.eastmoney.com/api/suggest/get",
        params={
            "input": query,
            "type": "14",
            "token": EASTMONEY_SEARCH_TOKEN,
        },
        timeout=10,
    )
    response.raise_for_status()
    payload = response.json()
    table = payload.get("QuotationCodeTable") if isinstance(payload, dict) else None
    rows = table.get("Data") if isinstance(table, dict) else None
    if rows is None:
        return []
    if not isinstance(rows, list) or not all(isinstance(row, dict) for row in rows):
        raise ValueError("Eastmoney search response has an invalid schema")
    ensure_request_active()
    return rows


def _spot_secids(market: str, symbols: list[str]) -> list[str]:
    normalized = market.strip().upper()
    result: list[str] = []
    for raw_symbol in symbols:
        symbol = raw_symbol.strip().upper()
        if normalized == "US":
            index_code = {".DJI": "DJIA", ".SPX": "SPX", ".NDX": "NDX"}.get(symbol)
            if index_code is not None:
                result.append(f"100.{index_code}")
            else:
                result.extend(f"{market_id}.{symbol}" for market_id in (105, 106, 107))
        elif normalized == "HK":
            index_code = {"800000": "100.HSI", "800100": "100.HSCEI", "800700": "124.HSTECH"}.get(symbol)
            result.append(index_code or f"116.{symbol}")
        elif normalized == "SH":
            result.append(f"1.{symbol}")
        elif normalized == "SZ":
            result.append(f"0.{symbol}")
    return list(dict.fromkeys(result))


def _normalize_spot_row(row: dict[str, Any]) -> dict[str, Any]:
    return {
        "market_id": row.get("f13"),
        "instrument_kind": row.get("f1"),
        "代码": row.get("f12"),
        "名称": row.get("f14"),
        "最新价": row.get("f2"),
        "涨跌幅": row.get("f3"),
        "涨跌额": row.get("f4"),
        "成交量": row.get("f5"),
        "成交额": row.get("f6"),
        "最高": row.get("f15"),
        "最低": row.get("f16"),
        "今开": row.get("f17"),
        "昨收": row.get("f18"),
        "市盈率": row.get("f9"),
        "市净率": row.get("f23"),
        "总市值": row.get("f20"),
        "流通市值": row.get("f21"),
        "买一": row.get("f31"),
        "卖一": row.get("f32"),
    }


def run(function: Any, /, *args: Any, **kwargs: Any) -> Any:
    """Bound one complete HTTP operation to one slot and one deadline."""
    slots = _slots
    if not slots.acquire(blocking=False):
        raise service_unavailable(
            "AKSHARE_POOL_BUSY",
            "AKShare worker pool is busy",
        )
    cancelled = threading.Event()

    def invoke() -> Any:
        _request_state.cancelled = cancelled
        try:
            return function(*args, **kwargs)
        finally:
            _request_state.cancelled = None

    future = _executor.submit(invoke)
    future.add_done_callback(lambda _future: slots.release())
    try:
        return future.result(timeout=CALL_TIMEOUT_SECONDS)
    except FutureTimeout as exc:
        cancelled.set()
        raise service_unavailable(
            "AKSHARE_UPSTREAM_TIMEOUT",
            "AKShare request timed out",
        ) from exc


def ensure_request_active() -> None:
    cancelled = getattr(_request_state, "cancelled", None)
    if cancelled is not None and cancelled.is_set():
        raise service_unavailable(
            "AKSHARE_UPSTREAM_TIMEOUT",
            "AKShare request timed out",
        )
