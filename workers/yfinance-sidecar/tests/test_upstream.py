"""Contracts for translating sidecar candle bounds into yfinance options."""

from __future__ import annotations

from concurrent.futures import ThreadPoolExecutor
from datetime import datetime, timezone
import threading
from typing import Any

import pytest

from yfinance_sidecar import upstream
from yfinance_sidecar.routes.candles import inclusive_history_end

TICKER_HISTORY = upstream.ticker_history
TICKER_INFO = upstream.ticker_info


@pytest.mark.parametrize(
    ("start", "end"),
    [
        (datetime(2026, 7, 1, tzinfo=timezone.utc), None),
        (
            datetime(2026, 7, 1, tzinfo=timezone.utc),
            datetime(2026, 7, 2, tzinfo=timezone.utc),
        ),
        (
            None,
            datetime(2026, 7, 1, tzinfo=timezone.utc),
        ),
    ],
)
def test_bounded_history_never_uses_a_period(
    monkeypatch: pytest.MonkeyPatch,
    start: datetime | None,
    end: datetime | None,
) -> None:
    calls: list[dict[str, Any]] = []

    class FakeTicker:
        def __init__(self, symbol: str, **kwargs: Any) -> None:
            assert symbol == "AAPL"
            assert kwargs["session"] is upstream._SESSION

        def history(self, **options: Any) -> str:
            calls.append(options)
            return "frame"

    monkeypatch.setattr(upstream.yf, "Ticker", FakeTicker)
    result = TICKER_HISTORY(
        "AAPL",
        interval="1d",
        fetch_period="5y",
        start=start,
        end=end,
    )

    assert result == "frame"
    assert calls[0]["period"] is None
    assert calls[0]["raise_errors"] is True
    assert calls[0]["timeout"] == upstream.UPSTREAM_TIMEOUT_SECONDS
    assert calls[0].get("start") == start
    assert calls[0].get("end") == end


def test_unbounded_history_uses_the_configured_fetch_period(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    calls: list[dict[str, Any]] = []

    class FakeTicker:
        def __init__(self, _symbol: str, **kwargs: Any) -> None:
            assert kwargs["session"] is upstream._SESSION

        def history(self, **options: Any) -> str:
            calls.append(options)
            return "frame"

    monkeypatch.setattr(upstream.yf, "Ticker", FakeTicker)

    TICKER_HISTORY(
        "AAPL",
        interval="5m",
        fetch_period="60d",
        start=None,
        end=None,
    )

    assert calls[0]["period"] == "60d"
    assert calls[0]["prepost"] is True
    assert calls[0]["raise_errors"] is True
    assert calls[0]["timeout"] == upstream.UPSTREAM_TIMEOUT_SECONDS
    assert "start" not in calls[0]
    assert "end" not in calls[0]


def test_history_can_disable_extended_hours_for_hk_and_cn_markets(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    calls: list[dict[str, Any]] = []

    class FakeTicker:
        def __init__(self, _symbol: str, **_kwargs: Any) -> None:
            pass

        def history(self, **options: Any) -> str:
            calls.append(options)
            return "frame"

    monkeypatch.setattr(upstream.yf, "Ticker", FakeTicker)
    monkeypatch.setattr(upstream, "ticker_history", TICKER_HISTORY)
    TICKER_HISTORY(
        "0700.HK",
        interval="5m",
        fetch_period="60d",
        start=None,
        end=None,
        prepost=False,
    )

    assert calls[0]["prepost"] is False


def test_bounded_session_clamps_yfinance_transport_timeouts(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    calls: list[Any] = []

    def fake_request(
        _self: Any,
        _method: Any,
        _url: str,
        *_args: Any,
        **kwargs: Any,
    ) -> str:
        calls.append(kwargs["timeout"])
        return "response"

    monkeypatch.setattr(upstream.requests.Session, "request", fake_request)
    session = upstream._BoundedSession(timeout=60)

    assert session.request("GET", "https://example.test", timeout=30) == "response"
    assert session.request("GET", "https://example.test", timeout=3) == "response"
    assert session.request("GET", "https://example.test") == "response"
    assert calls == [upstream.UPSTREAM_TIMEOUT_SECONDS, 3, upstream.UPSTREAM_TIMEOUT_SECONDS]


def test_shared_session_uses_browser_transport_profile() -> None:
    assert upstream._SESSION.impersonate == upstream.UPSTREAM_IMPERSONATE


def test_ticker_info_snapshot_cache_singleflights_concurrent_misses(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    upstream._ticker_info_cache.clear()
    calls = 0
    entered = threading.Event()
    release = threading.Event()

    class FakeTicker:
        def __init__(self, _symbol: str, **_kwargs: Any) -> None:
            pass

        def get_info(self) -> dict[str, Any]:
            nonlocal calls
            calls += 1
            entered.set()
            assert release.wait(timeout=2)
            return {"symbol": "AAPL", "exchange": "NMS", "quoteType": "EQUITY"}

    monkeypatch.setattr(upstream.yf, "Ticker", FakeTicker)
    with ThreadPoolExecutor(max_workers=4) as executor:
        futures = [
            executor.submit(
                TICKER_INFO,
                "AAPL",
                upstream.SNAPSHOT_CACHE_SECONDS,
            )
            for _ in range(4)
        ]
        assert entered.wait(timeout=2)
        release.set()
        results = [future.result(timeout=3) for future in futures]

    assert calls == 1
    assert all(result["symbol"] == "AAPL" for result in results)
    upstream._ticker_info_cache.clear()


@pytest.mark.parametrize(
    ("period", "to_time", "expected"),
    [
        (
            "1w",
            datetime(2026, 7, 29, 13, 30, tzinfo=timezone.utc),
            datetime(2026, 8, 5, 13, 30, tzinfo=timezone.utc),
        ),
        (
            "1mo",
            datetime(2024, 1, 31, 13, 30, tzinfo=timezone.utc),
            datetime(2024, 2, 29, 13, 30, tzinfo=timezone.utc),
        ),
        (
            "1mo",
            datetime(2026, 12, 31, 13, 30, tzinfo=timezone.utc),
            datetime(2027, 1, 31, 13, 30, tzinfo=timezone.utc),
        ),
    ],
)
def test_inclusive_end_advances_one_complete_interval(
    period: str,
    to_time: datetime,
    expected: datetime,
) -> None:
    assert inclusive_history_end(to_time, period) == expected
