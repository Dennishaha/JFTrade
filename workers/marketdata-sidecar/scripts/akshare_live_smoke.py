"""Opt-in live AKShare contract smoke; never used by ordinary tests."""

from __future__ import annotations

import asyncio
import json
import os
import sys
from pathlib import Path

import httpx

WORKER_ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(WORKER_ROOT / "src"))

from marketdata_sidecar import akshare_upstream  # noqa: E402
from marketdata_sidecar.main import app  # noqa: E402

ENABLE_ENV = "JFTRADE_AKSHARE_LIVE_SMOKE"


async def smoke() -> None:
    if os.environ.get(ENABLE_ENV) != "1":
        print(f"SKIP: set {ENABLE_ENV}=1 to run the live AKShare smoke")
        return

    akshare_upstream.warm_runtime()
    runtime = akshare_upstream.runtime_snapshot()
    if runtime.state != "ready":
        raise RuntimeError(f"AKShare runtime is {runtime.state}: {runtime.error}")

    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(
        transport=transport,
        base_url="http://sidecar.live",
        timeout=20,
    ) as client:
        health = await _require_ok(client, "/providers/akshare/health")
        searched = await _require_ok(
            client,
            "/providers/akshare/search",
            params={"q": "US.AAPL", "limit": 5},
        )
        entries = searched.get("entries") or []
        if not entries:
            raise RuntimeError("AKShare live search returned no US.AAPL entry")
        snapshot = await _require_ok(client, "/providers/akshare/snapshot/US/AAPL")
        candles = await _require_ok(
            client,
            "/providers/akshare/candles/US/AAPL",
            params={"period": "1d", "limit": 2},
        )
        if not candles.get("candles"):
            raise RuntimeError("AKShare live candle lookup returned no rows")
        print(
            json.dumps(
                {
                    "health": health.get("runtime_state"),
                    "instrument_id": entries[0].get("instrument_id"),
                    "price": snapshot.get("price"),
                    "candles": candles.get("total_returned"),
                },
                ensure_ascii=False,
                sort_keys=True,
            )
        )


async def _require_ok(
    client: httpx.AsyncClient,
    path: str,
    *,
    params: dict[str, object] | None = None,
) -> dict[str, object]:
    response = await client.get(path, params=params)
    if response.status_code != 200:
        raise RuntimeError(
            f"{path} returned HTTP {response.status_code}: {response.text[:500]}"
        )
    body = response.json()
    if not isinstance(body, dict):
        raise RuntimeError(f"{path} returned a non-object JSON response")
    return body


if __name__ == "__main__":
    asyncio.run(smoke())
