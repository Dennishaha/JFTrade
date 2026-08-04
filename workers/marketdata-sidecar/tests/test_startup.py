from __future__ import annotations

from types import SimpleNamespace
from typing import Any

import httpx
import pytest

from marketdata_sidecar import upstream
from marketdata_sidecar import main as sidecar_main


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


@pytest.mark.asyncio
async def test_legacy_health_triggers_lazy_yfinance_warmup(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    calls: list[str] = []
    monkeypatch.setattr(
        upstream,
        "runtime_snapshot",
        lambda: upstream.RuntimeSnapshot("warming"),
    )
    monkeypatch.setattr(
        upstream,
        "request_runtime_warmup",
        lambda: calls.append("warm") or upstream.RuntimeSnapshot("warming"),
    )

    response = await client.get("/health")

    assert response.status_code == 200
    assert response.json()["runtime_state"] == "warming"
    assert calls == ["warm"]


@pytest.mark.asyncio
@pytest.mark.parametrize(
    ("state", "code", "message", "retry_after"),
    [
        (
            "warming",
            "YFINANCE_RUNTIME_WARMING",
            "Yahoo Finance runtime is warming up",
            "1",
        ),
        (
            "failed",
            "YFINANCE_RUNTIME_FAILED",
            "Yahoo Finance runtime failed to initialize",
            None,
        ),
    ],
)
async def test_namespaced_yfinance_health_requires_ready_runtime(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
    state: str,
    code: str,
    message: str,
    retry_after: str | None,
) -> None:
    monkeypatch.setattr(
        upstream,
        "runtime_snapshot",
        lambda: upstream.RuntimeSnapshot(state, "private import failure"),
    )
    monkeypatch.setattr(
        upstream,
        "request_runtime_warmup",
        lambda: upstream.RuntimeSnapshot(state),
    )

    response = await client.get("/providers/yfinance/health")

    assert response.status_code == 503
    assert response.json() == {"error": {"code": code, "message": message}}
    assert response.headers.get("Retry-After") == retry_after
    assert "private import failure" not in response.text


@pytest.mark.asyncio
async def test_namespaced_yfinance_health_ready_is_200(
    client: httpx.AsyncClient,
) -> None:
    response = await client.get("/providers/yfinance/health")

    assert response.status_code == 200
    assert response.json()["ok"] is True
    assert response.json()["runtime_state"] == "ready"
    assert response.json()["yfinance_version"]


@pytest.mark.asyncio
async def test_legacy_health_keeps_200_when_yfinance_runtime_failed(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(
        upstream,
        "runtime_snapshot",
        lambda: upstream.RuntimeSnapshot("failed", "missing runtime"),
    )
    monkeypatch.setattr(
        upstream,
        "request_runtime_warmup",
        lambda: upstream.RuntimeSnapshot("failed"),
    )

    response = await client.get("/health")

    assert response.status_code == 200
    assert response.json()["runtime_state"] == "failed"
    assert response.json()["warmup_error"] == "missing runtime"


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


def test_app_assembly_does_not_eagerly_warm_any_provider(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    calls: list[str] = []
    monkeypatch.setattr(upstream, "warm_runtime", lambda: calls.append("yfinance"))

    sidecar_main.create_app()

    assert calls == []
