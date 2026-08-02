"""Test guardrails: route tests must never reach a real network."""

from __future__ import annotations

import socket

import httpx
import pytest

from yfinance_sidecar import upstream
from yfinance_sidecar.main import app


@pytest.fixture(autouse=True)
def block_real_network(monkeypatch: pytest.MonkeyPatch) -> None:
    def blocked_connect(*_args: object, **_kwargs: object) -> None:
        raise AssertionError("real network access is forbidden in sidecar tests")

    def blocked_yfinance(*_args: object, **_kwargs: object) -> None:
        raise AssertionError("yfinance must be mocked in sidecar tests")

    monkeypatch.setattr(socket.socket, "connect", blocked_connect)
    monkeypatch.setattr(
        upstream,
        "runtime_snapshot",
        lambda: upstream.RuntimeSnapshot("ready"),
    )
    monkeypatch.setattr(upstream, "search_quotes", blocked_yfinance)
    monkeypatch.setattr(upstream, "ticker_info", blocked_yfinance)
    monkeypatch.setattr(upstream, "ticker_history", blocked_yfinance)


@pytest.fixture
async def client() -> httpx.AsyncClient:
    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(
        transport=transport,
        base_url="http://sidecar.test",
    ) as test_client:
        yield test_client
