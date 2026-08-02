"""Process health route; this endpoint never calls Yahoo Finance."""

from importlib.metadata import version

from fastapi import APIRouter

from .. import upstream
from ..models import HealthResponse

router = APIRouter()


@router.get("/health", response_model=HealthResponse)
async def health() -> HealthResponse:
    runtime = upstream.runtime_snapshot()
    return HealthResponse(
        ok=True,
        yfinance_version=version("yfinance"),
        runtime_state=runtime.state,
        warmup_error=runtime.error or None,
    )
