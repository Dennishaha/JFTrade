"""FastAPI application assembly and stable error serialization."""

from __future__ import annotations

import argparse
import logging
from collections.abc import Sequence

from fastapi import FastAPI, Request
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse
from starlette.exceptions import HTTPException as StarletteHTTPException

from . import __version__
from . import akshare_upstream, upstream
from .errors import SidecarError
from .models import ErrorBody, ErrorEnvelope
from .readiness import runtime_unavailable_response
from .routes import (
    akshare,
    calendar,
    candles,
    corporate_actions,
    health,
    markets,
    news,
    rankings,
    research,
    search,
    security,
    snapshot,
)

logger = logging.getLogger(__name__)


def create_app() -> FastAPI:
    application = FastAPI(
        title="JFTrade market-data sidecar",
        version=__version__,
        docs_url=None,
        redoc_url=None,
        openapi_url=None,
    )
    application.add_exception_handler(SidecarError, _sidecar_error_handler)
    application.add_exception_handler(
        RequestValidationError,
        _validation_error_handler,
    )
    application.add_exception_handler(
        StarletteHTTPException,
        _http_error_handler,
    )
    application.add_exception_handler(Exception, _unexpected_error_handler)
    application.middleware("http")(_runtime_readiness_middleware)
    application.include_router(health.router)
    application.include_router(markets.router)
    for yahoo_router in (
        markets.router,
        search.router,
        security.router,
        snapshot.router,
        candles.router,
        news.router,
        corporate_actions.router,
        rankings.router,
        research.router,
    ):
        application.include_router(yahoo_router, prefix="/providers/yfinance")
    application.include_router(search.router)
    application.include_router(security.router)
    application.include_router(snapshot.router)
    application.include_router(candles.router)
    application.include_router(news.router)
    application.include_router(corporate_actions.router)
    application.include_router(rankings.router)
    application.include_router(research.router)
    application.include_router(akshare.router)
    application.include_router(research.akshare_router)
    application.include_router(calendar.router)
    return application


async def _runtime_readiness_middleware(
    request: Request,
    call_next,
):
    provider = _provider_for_data_path(request.url.path)
    if provider is not None:
        boundary = upstream if provider == "yfinance" else akshare_upstream
        runtime = boundary.runtime_snapshot()
        if runtime.state != "ready":
            boundary.request_runtime_warmup()
            runtime = boundary.runtime_snapshot()
        if runtime.state != "ready":
            return runtime_unavailable_response(provider, runtime.state)
    return await call_next(request)


def _provider_for_data_path(path: str) -> str | None:
    for provider in ("akshare", "yfinance"):
        prefix = f"/providers/{provider}/"
        if path.startswith(prefix):
            suffix = path.removeprefix(prefix)
            return None if suffix in {"health", "markets"} else provider
    if path in {"/health", "/healthz", "/markets"}:
        return None
    if path.startswith(
        (
            "/search",
            "/security/",
            "/snapshot/",
            "/candles/",
            "/news/",
            "/corporate-actions/",
            "/rankings",
            "/profile/",
            "/financials/",
            "/analyst/",
            "/ownership/",
        )
    ):
        return "yfinance"
    return None


async def _sidecar_error_handler(
    _request: Request,
    exc: SidecarError,
) -> JSONResponse:
    response = _error_response(exc.status_code, exc.code, exc.message)
    if exc.status_code == 503 and exc.code in {
        "AKSHARE_POOL_BUSY",
        "AKSHARE_UPSTREAM_TIMEOUT",
    }:
        response.headers["Retry-After"] = "2"
    return response


async def _validation_error_handler(
    _request: Request,
    _exc: RequestValidationError,
) -> JSONResponse:
    return _error_response(400, "invalid_request", "request validation failed")


async def _http_error_handler(
    _request: Request,
    exc: StarletteHTTPException,
) -> JSONResponse:
    code = "not_found" if exc.status_code == 404 else "http_error"
    return _error_response(exc.status_code, code, str(exc.detail))


async def _unexpected_error_handler(
    _request: Request,
    exc: Exception,
) -> JSONResponse:
    logger.exception("unhandled market-data sidecar error", exc_info=exc)
    return _error_response(500, "internal_error", "internal sidecar error")


def _error_response(status_code: int, code: str, message: str) -> JSONResponse:
    envelope = ErrorEnvelope(error=ErrorBody(code=code, message=message))
    return JSONResponse(
        status_code=status_code,
        content=envelope.model_dump(mode="json"),
    )


app = create_app()


def _port(value: str) -> int:
    try:
        port = int(value)
    except ValueError as exc:
        raise argparse.ArgumentTypeError("port must be an integer") from exc
    if not 1 <= port <= 65535:
        raise argparse.ArgumentTypeError("port must be between 1 and 65535")
    return port


def parse_args(argv: Sequence[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Run the local JFTrade market-data sidecar.",
    )
    parser.add_argument(
        "--host",
        default="127.0.0.1",
        help="host interface to bind (default: 127.0.0.1)",
    )
    parser.add_argument(
        "--port",
        default=7788,
        type=_port,
        help="TCP port to bind (default: 7788)",
    )
    parser.add_argument(
        "--version",
        action="version",
        version=f"marketdata-sidecar {__version__}",
    )
    return parser.parse_args(argv)


def main(argv: Sequence[str] | None = None) -> None:
    args = parse_args(argv)
    import uvicorn

    uvicorn.run(
        app,
        host=args.host,
        port=args.port,
        loop="asyncio",
        http="h11",
        ws="none",
        lifespan="on",
    )


if __name__ == "__main__":
    main()
