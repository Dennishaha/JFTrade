from __future__ import annotations

from types import SimpleNamespace
from typing import Any

import httpx
import pytest

from yfinance_sidecar import upstream
from yfinance_sidecar import main as sidecar_main


@pytest.mark.asyncio
async def test_data_routes_return_retryable_warming(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(
        upstream,
        "runtime_snapshot",
        lambda: upstream.RuntimeSnapshot("warming"),
    )

    response = await client.get("/search", params={"q": "AAPL"})

    assert response.status_code == 503
    assert response.headers["Retry-After"] == "1"
    assert response.json() == {
        "error": {
            "code": "YFINANCE_RUNTIME_WARMING",
            "message": "Yahoo Finance runtime is warming up",
        }
    }


@pytest.mark.asyncio
async def test_static_routes_stay_available_while_runtime_warms(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(
        upstream,
        "runtime_snapshot",
        lambda: upstream.RuntimeSnapshot("warming"),
    )

    health = await client.get("/health")
    markets = await client.get("/markets")

    assert health.status_code == 200
    assert health.json()["runtime_state"] == "warming"
    assert markets.status_code == 200


def test_runtime_warmup_imports_heavy_stack_once(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    imports: list[str] = []

    class FakeSession:
        def __init__(self, **_kwargs: Any) -> None:
            pass

        def request(self, *_args: Any, **_kwargs: Any) -> None:
            return None

    def fake_import(name: str) -> Any:
        imports.append(name)
        if name == "yfinance":
            return SimpleNamespace()
        if name == "curl_cffi.requests":
            return SimpleNamespace(Session=FakeSession)
        raise AssertionError(f"unexpected import: {name}")

    monkeypatch.setattr(upstream, "_runtime_started", False)
    monkeypatch.setattr(
        upstream,
        "_runtime_snapshot",
        upstream.RuntimeSnapshot("warming"),
    )
    monkeypatch.setattr(upstream, "_runtime_components", None)
    monkeypatch.setattr(upstream.importlib, "import_module", fake_import)

    upstream.warm_runtime()
    upstream.warm_runtime()

    assert imports == ["yfinance", "curl_cffi.requests"]
    assert upstream._runtime_snapshot.state == "ready"
    assert upstream._runtime_components is not None


def test_runtime_warmup_records_import_failure(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(upstream, "_runtime_started", False)
    monkeypatch.setattr(
        upstream,
        "_runtime_snapshot",
        upstream.RuntimeSnapshot("warming"),
    )
    monkeypatch.setattr(upstream, "_runtime_components", None)

    def fail_import(_name: str) -> Any:
        raise ImportError("missing runtime")

    monkeypatch.setattr(upstream.importlib, "import_module", fail_import)

    upstream.warm_runtime()

    assert upstream._runtime_snapshot.state == "failed"
    assert "ImportError" in upstream._runtime_snapshot.error


@pytest.mark.asyncio
async def test_lifespan_schedules_one_daemon_warmup(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    scheduled: list[dict[str, Any]] = []

    class FakeTimer:
        def __init__(self, interval: float, function: Any) -> None:
            scheduled.append({"interval": interval, "target": function})
            self.name = ""
            self.daemon = False

        def start(self) -> None:
            scheduled[-1].update(name=self.name, daemon=self.daemon, started=True)

    monkeypatch.setattr(sidecar_main.threading, "Timer", FakeTimer)
    async with sidecar_main._lifespan(sidecar_main.app):
        pass

    assert scheduled == [
        {
            "interval": sidecar_main.RUNTIME_WARMUP_DELAY_SECONDS,
            "target": upstream.warm_runtime,
            "name": "yfinance-runtime-warmup",
            "daemon": True,
            "started": True,
        }
    ]
