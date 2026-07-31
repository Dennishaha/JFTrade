"""Process health route; this endpoint never calls Yahoo Finance."""

import yfinance as yf
from fastapi import APIRouter

from ..models import HealthResponse

router = APIRouter()


@router.get("/health", response_model=HealthResponse)
def health() -> HealthResponse:
    return HealthResponse(ok=True, yfinance_version=yf.__version__)
