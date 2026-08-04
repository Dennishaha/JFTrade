"""Process and Yahoo runtime health routes; neither performs network I/O."""

from importlib.metadata import PackageNotFoundError, version

from fastapi import APIRouter
from fastapi.responses import JSONResponse

from .. import upstream
from .. import __version__
from ..models import HealthResponse, ProcessHealthResponse
from ..readiness import provider_health_response

router = APIRouter()


def _package_version(name: str) -> str | None:
    try:
        return version(name)
    except PackageNotFoundError:
        return None


@router.get("/healthz", response_model=ProcessHealthResponse)
async def process_health() -> ProcessHealthResponse:
    return ProcessHealthResponse(ok=True, version=__version__)


@router.get("/health", response_model=HealthResponse)
async def health() -> HealthResponse:
    runtime = upstream.runtime_snapshot()
    if runtime.state != "ready":
        upstream.request_runtime_warmup()
        runtime = upstream.runtime_snapshot()
    return HealthResponse(
        ok=True,
        yfinance_version=_package_version("yfinance") or "unavailable",
        runtime_state=runtime.state,
        warmup_error=runtime.error or None,
    )


@router.get("/providers/yfinance/health", response_model=HealthResponse)
async def provider_health() -> HealthResponse | JSONResponse:
    return provider_health_response(
        "yfinance",
        upstream,
        lambda runtime: HealthResponse(
            ok=True,
            yfinance_version=_package_version("yfinance") or "unavailable",
            runtime_state=runtime.state,
            warmup_error=runtime.error or None,
        ),
    )
