"""Stock screener routes for the yfinance and akshare namespaces."""

from __future__ import annotations

from fastapi import APIRouter

from .. import akshare_screen, yfinance_screen
from ..errors import SidecarError, upstream_error
from ..models import ScreenRequest, ScreenResponse
from .akshare import _translate

router = APIRouter()
akshare_router = APIRouter(prefix="/providers/akshare")


@router.post("/screen", response_model=ScreenResponse)
def yfinance_screen_route(request: ScreenRequest) -> ScreenResponse:
    try:
        return yfinance_screen.screen(request)
    except SidecarError:
        raise
    except Exception as exc:
        raise upstream_error("Yahoo Finance screen failed") from exc


@akshare_router.post("/screen", response_model=ScreenResponse)
def akshare_screen_route(request: ScreenRequest) -> ScreenResponse:
    return _translate("screen", akshare_screen.screen, request)
