"""AKShare CN analyst ratings aggregated from Eastmoney research reports.

akshare 1.18.91 has no per-stock ``stock_analyst_rating_em``; the usable
per-symbol source is ``stock_research_report_em`` (个股研报), which carries a
text 东财评级 per report.  Reports from the trailing 180 days are aggregated.

Rating text -> (1-5 score, distribution bucket) mapping (higher = bullish):

    买入 -> 5 / strong_buy
    增持 -> 4 / buy
    中性 -> 3 / hold
    减持 -> 2 / underperform
    卖出 -> 1 / sell

Unmapped rating texts are ignored.  Eastmoney reports carry no usable price
target columns, so ``target_price`` stays null.
"""

from __future__ import annotations

from datetime import datetime, timedelta, timezone
from typing import Any

from . import akshare_upstream
from .akshare_provider_conversion import _frame_rows, _row_value
from .conversion import clean_text
from .models import AnalystDistribution, AnalystResponse
from .research_common import (
    akshare_research_identity,
    require_cn_leaf,
    research_not_found,
)
from .upstream import _TickerInfoCache

ANALYST_CACHE_SECONDS = 3600
REPORT_WINDOW_DAYS = 180

_RATING_MAP = {
    "买入": (5, "strong_buy"),
    "增持": (4, "buy"),
    "中性": (3, "hold"),
    "减持": (2, "underperform"),
    "卖出": (1, "sell"),
}
_BUCKETS = ("strong_buy", "buy", "hold", "underperform", "sell")

_analyst_cache = _TickerInfoCache()


def analyst(market: str, symbol: str) -> AnalystResponse:
    requested, leaf, code = akshare_research_identity(market, symbol, "analyst")
    require_cn_leaf(leaf, "analyst", market)
    instrument_id = f"{requested}.{code}"
    rows = _analyst_cache.get_or_fetch(
        f"{leaf}:{code}",
        ANALYST_CACHE_SECONDS,
        lambda: {"rows": _fetch_rows(code)},
    )["rows"]
    cutoff = (datetime.now(timezone.utc) - timedelta(days=REPORT_WINDOW_DAYS)).date()
    recent = [row for row in rows if row["date"] >= cutoff.isoformat()]
    rated = [row for row in recent if row["bucket"] is not None]
    if not rated:
        raise research_not_found("analyst", instrument_id)
    total = len(rated)
    orgs = {row["org"] for row in rated if row["org"]}
    return AnalystResponse(
        instrument_id=instrument_id,
        rating=sum(row["score"] for row in rated) / total,
        analyst_count=len(orgs) or None,
        target_price=None,
        distribution=AnalystDistribution(
            **{
                bucket: sum(1 for row in rated if row["bucket"] == bucket)
                / total
                * 100
                for bucket in _BUCKETS
            }
        ),
        update_time=max(row["date"] for row in rated),
    )


def _fetch_rows(code: str) -> list[dict[str, Any]]:
    frame = akshare_upstream.call("stock_research_report_em", symbol=code)
    rows = []
    for raw in _frame_rows(frame):
        date = _row_value(raw, "日期", "publishDate")
        if date is None:
            continue
        rows.append(
            {
                "date": str(date)[:10],
                "org": clean_text(_row_value(raw, "机构", "orgSName")),
                "bucket": _rating_bucket(_row_value(raw, "东财评级", "emRatingName")),
                "score": _rating_score(_row_value(raw, "东财评级", "emRatingName")),
            }
        )
    return rows


def _rating_parts(value: Any) -> tuple[int, str] | None:
    text = clean_text(value)
    if text is None:
        return None
    return _RATING_MAP.get(text)


def _rating_bucket(value: Any) -> str | None:
    parts = _rating_parts(value)
    return parts[1] if parts else None


def _rating_score(value: Any) -> int | None:
    parts = _rating_parts(value)
    return parts[0] if parts else None
