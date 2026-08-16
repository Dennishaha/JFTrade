"""yfinance snapshot fast path: fast_info first, get_info fallback."""

from __future__ import annotations

from types import SimpleNamespace
from typing import Any

import httpx
import pytest

from marketdata_sidecar import upstream


class _FakeTicker:
    def __init__(self, fast_info: Any = None, info: Any = None) -> None:
        self._fast_info = fast_info
        self._info = info

    def get_fast_info(self) -> Any:
        if isinstance(self._fast_info, Exception):
            raise self._fast_info
        return self._fast_info

    def get_info(self) -> Any:
        if isinstance(self._info, Exception):
            raise self._info
        return self._info


def _runtime(ticker: _FakeTicker) -> SimpleNamespace:
    return SimpleNamespace(
        yfinance=SimpleNamespace(Ticker=lambda _symbol, **_kw: ticker),
        session=object(),
    )


def test_fast_info_maps_price_fields_onto_info_keys(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    ticker = _FakeTicker(
        fast_info={
            "last_price": 210.5,
            "previous_close": 208.0,
            "open": 209.0,
            "day_high": 211.0,
            "day_low": 207.5,
            "last_volume": 41_000_000,
            "market_cap": 3_200_000_000_000,
            "currency": "USD",
            "exchange": "NMS",
            "quote_type": "EQUITY",
            "timezone": "America/New_York",
        }
    )
    monkeypatch.setattr(upstream, "require_runtime", lambda: _runtime(ticker))

    info = upstream.ticker_fast_info("FAST1")

    assert info is not None
    assert info["regularMarketPrice"] == 210.5
    assert info["regularMarketPreviousClose"] == 208.0
    assert info["regularMarketOpen"] == 209.0
    assert info["regularMarketDayHigh"] == 211.0
    assert info["regularMarketDayLow"] == 207.5
    assert info["regularMarketVolume"] == 41_000_000
    assert info["marketCap"] == 3_200_000_000_000
    assert info["quoteType"] == "EQUITY"
    assert info["exchange"] == "NMS"
    assert info["currency"] == "USD"


@pytest.mark.parametrize(
    "fast_info",
    [
        # Missing price: the snapshot contract requires one.
        {"quote_type": "EQUITY", "exchange": "NMS", "currency": "USD"},
        # Missing identity fields: the snapshot validators require them.
        {"last_price": 210.5},
        # A fast_info key that raises mid-read must not fail the request.
        None,
    ],
)
def test_fast_info_unusable_payloads_fall_back(
    monkeypatch: pytest.MonkeyPatch,
    fast_info: Any,
) -> None:
    ticker = _FakeTicker(fast_info=fast_info)
    monkeypatch.setattr(upstream, "require_runtime", lambda: _runtime(ticker))

    assert upstream.ticker_fast_info("FAST2") is None


def test_fast_info_transport_failure_falls_back(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    ticker = _FakeTicker(fast_info=RuntimeError("yahoo down"))
    monkeypatch.setattr(upstream, "require_runtime", lambda: _runtime(ticker))

    assert upstream.ticker_fast_info("FAST3") is None


def _fast_path_info() -> dict[str, Any]:
    return {
        "regularMarketPrice": 210.0,
        "regularMarketPreviousClose": 206.0,
        "regularMarketOpen": 205.0,
        "regularMarketDayHigh": 212.0,
        "regularMarketDayLow": 204.0,
        "regularMarketVolume": 42_000_000,
        "quoteType": "EQUITY",
        "exchange": "NMS",
        "currency": "USD",
    }


@pytest.mark.asyncio
async def test_snapshot_uses_fast_info_without_calling_get_info(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(
        upstream,
        "ticker_fast_info",
        lambda symbol: _fast_path_info() if symbol == "AAPL" else None,
    )

    def forbidden_info(_symbol: str, **_kw: Any) -> dict[str, Any]:
        raise AssertionError("get_info must not run when fast_info succeeds")

    monkeypatch.setattr(upstream, "ticker_info", forbidden_info)

    response = await client.get("/snapshot/US/AAPL")

    assert response.status_code == 200
    body = response.json()
    assert body["price"] == 210.0
    assert body["previous_close_price"] == 206.0
    assert body["open_price"] == 205.0
    assert body["volume"] == 42_000_000
    assert body["delayed"] is True
    assert body["delay_minutes"] == 15
    # fast_info has no extended-hours payload; the blocks stay null.
    assert body["pre_market_quote"] is None
    assert body["after_market_quote"] is None


@pytest.mark.asyncio
async def test_snapshot_falls_back_to_get_info_when_fast_info_is_unusable(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(upstream, "ticker_fast_info", lambda _symbol: None)
    info_calls: list[str] = []

    def fake_info(symbol: str, **_kw: Any) -> dict[str, Any]:
        info_calls.append(symbol)
        return {
            "symbol": "AAPL",
            "exchange": "NMS",
            "quoteType": "EQUITY",
            "regularMarketPrice": 210.0,
            "regularMarketTime": 1_753_812_000,
        }

    monkeypatch.setattr(upstream, "ticker_info", fake_info)

    response = await client.get("/snapshot/US/AAPL")

    assert response.status_code == 200
    assert response.json()["price"] == 210.0
    assert info_calls == ["AAPL"]
