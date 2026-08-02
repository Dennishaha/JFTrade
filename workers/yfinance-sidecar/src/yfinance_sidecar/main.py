"""FastAPI application assembly and stable error serialization."""

from __future__ import annotations

import argparse
import logging
import threading
from collections.abc import Sequence
from contextlib import asynccontextmanager

from fastapi import FastAPI, Request
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse
from starlette.exceptions import HTTPException as StarletteHTTPException

from . import __version__
from . import upstream
from .errors import SidecarError
from .models import ErrorBody, ErrorEnvelope
from .routes import candles, health, markets, search, security, snapshot

logger = logging.getLogger(__name__)
RUNTIME_WARMUP_DELAY_SECONDS = 0.5


@asynccontextmanager
async def _lifespan(_application: FastAPI):
    warmup = threading.Timer(
        RUNTIME_WARMUP_DELAY_SECONDS,
        function=upstream.warm_runtime,
    )
    warmup.name = "yfinance-runtime-warmup"
    warmup.daemon = True
    warmup.start()
    yield


def create_app() -> FastAPI:
    application = FastAPI(
        title="JFTrade yfinance sidecar",
        version=__version__,
        docs_url=None,
        redoc_url=None,
        openapi_url=None,
        lifespan=_lifespan,
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
    application.include_router(search.router)
    application.include_router(security.router)
    application.include_router(snapshot.router)
    application.include_router(candles.router)
    return application


async def _runtime_readiness_middleware(
    request: Request,
    call_next,
):
    if request.url.path not in {"/health", "/markets"}:
        runtime = upstream.runtime_snapshot()
        if runtime.state != "ready":
            code = (
                "YFINANCE_RUNTIME_WARMING"
                if runtime.state == "warming"
                else "YFINANCE_RUNTIME_FAILED"
            )
            message = (
                "Yahoo Finance runtime is warming up"
                if runtime.state == "warming"
                else "Yahoo Finance runtime failed to initialize"
            )
            response = _error_response(503, code, message)
            if runtime.state == "warming":
                response.headers["Retry-After"] = "1"
            return response
    return await call_next(request)


async def _sidecar_error_handler(
    _request: Request,
    exc: SidecarError,
) -> JSONResponse:
    return _error_response(exc.status_code, exc.code, exc.message)


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
    logger.exception("unhandled yfinance sidecar error", exc_info=exc)
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
        description="Run the local JFTrade yfinance sidecar.",
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
        version=f"yfinance-sidecar {__version__}",
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
