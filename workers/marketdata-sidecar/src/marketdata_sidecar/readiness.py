"""Shared Provider runtime readiness responses."""

from __future__ import annotations

from collections.abc import Callable
from typing import Any

from fastapi.responses import JSONResponse

from .models import ErrorBody, ErrorEnvelope

_PROVIDER_LABELS = {
    "yfinance": "Yahoo Finance",
    "akshare": "AKShare",
}


def provider_health_response(
    provider: str,
    boundary: Any,
    ready_payload: Callable[[Any], Any],
) -> Any:
    """Warm one Provider lazily and serialize readiness consistently."""
    runtime = boundary.runtime_snapshot()
    if runtime.state != "ready":
        boundary.request_runtime_warmup()
        runtime = boundary.runtime_snapshot()
    if runtime.state == "ready":
        return ready_payload(runtime)
    return runtime_unavailable_response(provider, runtime.state)


def runtime_unavailable_response(provider: str, state: str) -> JSONResponse:
    normalized_provider = provider.strip().lower()
    warming = state == "warming"
    suffix = "WARMING" if warming else "FAILED"
    label = _PROVIDER_LABELS.get(normalized_provider, normalized_provider)
    message = (
        f"{label} runtime is warming up"
        if warming
        else f"{label} runtime failed to initialize"
    )
    envelope = ErrorEnvelope(
        error=ErrorBody(
            code=f"{normalized_provider.upper()}_RUNTIME_{suffix}",
            message=message,
        )
    )
    response = JSONResponse(
        status_code=503,
        content=envelope.model_dump(mode="json"),
    )
    if warming:
        response.headers["Retry-After"] = "1"
    return response
