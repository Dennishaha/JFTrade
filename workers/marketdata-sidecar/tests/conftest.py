"""Test guardrails: route tests must never reach a real network."""

from __future__ import annotations

import socket

import httpx
import pytest

from marketdata_sidecar import (
    akshare_index_constituents,
    akshare_news,
    akshare_provider,
    akshare_quotes,
    akshare_upstream,
    upstream,
)
from marketdata_sidecar.main import app


@pytest.fixture(autouse=True)
def block_real_network(monkeypatch: pytest.MonkeyPatch) -> None:
    def blocked_connect(*_args: object, **_kwargs: object) -> None:
        raise AssertionError("real network access is forbidden in sidecar tests")

    def blocked_yfinance(*_args: object, **_kwargs: object) -> None:
        raise AssertionError("yfinance must be mocked in sidecar tests")

    def blocked_akshare(*_args: object, **_kwargs: object) -> None:
        raise AssertionError("AKShare must be mocked in sidecar tests")

    monkeypatch.setattr(socket.socket, "connect", blocked_connect)
    monkeypatch.setattr(
        upstream,
        "runtime_snapshot",
        lambda: upstream.RuntimeSnapshot("ready"),
    )
    monkeypatch.setattr(upstream, "search_quotes", blocked_yfinance)
    monkeypatch.setattr(upstream, "ticker_info", blocked_yfinance)
    monkeypatch.setattr(upstream, "ticker_history", blocked_yfinance)
    monkeypatch.setattr(upstream, "ticker_news", blocked_yfinance)
    monkeypatch.setattr(upstream, "ticker_actions", blocked_yfinance)
    monkeypatch.setattr(
        akshare_upstream,
        "runtime_snapshot",
        lambda: upstream.RuntimeSnapshot("ready"),
    )
    monkeypatch.setattr(akshare_upstream, "call", blocked_akshare)
    monkeypatch.setattr(akshare_upstream, "us_minute_rows", blocked_akshare)
    monkeypatch.setattr(akshare_upstream, "hk_minute_rows", blocked_akshare)
    akshare_provider._catalog_cache.clear()
    akshare_quotes._enrichment_cache.clear()
    akshare_news._news_cache.clear()
    akshare_news._fhps_cache.clear()
    akshare_index_constituents._constituents_cache.clear()
    upstream._ticker_info_cache.clear()
    upstream._ticker_fast_info_cache.clear()
    upstream._ticker_news_cache.clear()
    upstream._ticker_actions_cache.clear()


@pytest.fixture
async def client() -> httpx.AsyncClient:
    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(
        transport=transport,
        base_url="http://sidecar.test",
    ) as test_client:
        yield test_client
