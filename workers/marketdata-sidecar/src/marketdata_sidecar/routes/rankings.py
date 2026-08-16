"""Yahoo Finance market rankings routes."""

from __future__ import annotations

from fastapi import APIRouter, Query

from .. import yfinance_rankings
from ..errors import SidecarError, upstream_error
from ..models import RankingsResponse
from .common import parse_ranking_kind

router = APIRouter()


@router.get("/rankings", response_model=RankingsResponse)
def rankings(
    market: str = Query(),
    kind: str = Query(),
    limit: int = Query(default=20, ge=1, le=100),
) -> RankingsResponse:
    selected_kind = parse_ranking_kind(kind)
    try:
        return yfinance_rankings.rankings(market, selected_kind, limit)
    except SidecarError:
        raise
    except Exception as exc:
        raise upstream_error("Yahoo Finance rankings lookup failed") from exc
