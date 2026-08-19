"""Private helper authentication remains opt-in during Go/Rust coexistence."""

from __future__ import annotations

import httpx
import pytest

from marketdata_sidecar.main import create_app


@pytest.mark.asyncio
async def test_configured_bearer_token_fails_closed() -> None:
    token = "stage4_marketdata_token_0123456789abcdef"
    transport = httpx.ASGITransport(app=create_app(token))
    async with httpx.AsyncClient(
        transport=transport,
        base_url="http://sidecar.test",
    ) as client:
        missing = await client.get("/healthz")
        assert missing.status_code == 401
        assert missing.json() == {
            "error": {
                "code": "unauthenticated",
                "message": "missing or invalid market-data helper bearer token",
            },
        }

        authorized = await client.get(
            "/healthz",
            headers={"Authorization": f"Bearer {token}"},
        )
        assert authorized.status_code == 200


def test_configured_bearer_token_rejects_weak_secret() -> None:
    with pytest.raises(ValueError, match="at least 32"):
        create_app("short")
