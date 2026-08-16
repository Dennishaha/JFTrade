"""AKShare CN news headlines and dividend/split events."""

from __future__ import annotations

import re
from datetime import date, datetime, timezone
from decimal import Decimal, InvalidOperation
from typing import Any, Mapping
from zoneinfo import ZoneInfo

from . import akshare_upstream
from .akshare_identity import normalize_identity
from .akshare_provider_conversion import _frame_rows, _row_value
from .conversion import clean_text, format_rfc3339, timestamp_as_utc
from .errors import SidecarError
from .models import (
    CorporateActionEvent,
    CorporateActionsResponse,
    NewsEntry,
    NewsResponse,
)
from .upstream import ACTIONS_CACHE_SECONDS, NEWS_CACHE_SECONDS, _TickerInfoCache

CN_TIMEZONE = "Asia/Shanghai"

_news_cache = _TickerInfoCache()
_fhps_cache = _TickerInfoCache()


def news(market: str, symbol: str, limit: int) -> NewsResponse:
    normalized_market, normalized_symbol = _cn_identity(market, symbol, "news")
    rows = _news_cache.get_or_fetch(
        normalized_symbol,
        NEWS_CACHE_SECONDS,
        lambda: {"rows": _fetch_news_rows(normalized_symbol)},
    )["rows"]
    entries = [
        entry
        for row in rows[:limit]
        if (entry := _news_entry(row)) is not None
    ]
    return NewsResponse(
        market=normalized_market,
        symbol=normalized_symbol,
        instrument_id=f"{normalized_market}.{normalized_symbol}",
        entries=entries,
        source="akshare-news",
    )


def corporate_actions(
    market: str,
    symbol: str,
    from_date: date,
    to_date: date,
) -> CorporateActionsResponse:
    normalized_market, normalized_symbol = _cn_identity(
        market,
        symbol,
        "corporate actions",
    )
    events: list[CorporateActionEvent] = []
    for report_date in _report_dates(from_date, to_date):
        for row in _fhps_rows(report_date):
            if clean_text(_row_value(row, "代码")) != normalized_symbol:
                continue
            events.extend(_row_events(row))
    selected = sorted(
        {
            (event.kind, event.ex_date, event.amount, event.ratio): event
            for event in events
            if from_date.isoformat() <= event.ex_date <= to_date.isoformat()
        }.values(),
        key=lambda event: (event.ex_date, event.kind),
    )
    return CorporateActionsResponse(
        market=normalized_market,
        symbol=normalized_symbol,
        instrument_id=f"{normalized_market}.{normalized_symbol}",
        events=selected,
        source="akshare-actions",
    )


def _cn_identity(market: str, symbol: str, operation: str) -> tuple[str, str]:
    normalized_market, normalized_symbol = normalize_identity(market, symbol)
    if normalized_market not in {"SH", "SZ"}:
        raise SidecarError(
            400,
            "AKSHARE_UNSUPPORTED",
            f"AKShare {operation} is only available for CN markets",
        )
    return normalized_market, normalized_symbol


def _fetch_news_rows(symbol: str) -> list[dict[str, Any]]:
    frame = akshare_upstream.call("stock_news_em", symbol=symbol)
    return [dict(row) for row in _frame_rows(frame)]


def _news_entry(row: Mapping[str, Any]) -> NewsEntry | None:
    title = clean_text(_row_value(row, "新闻标题", "title"))
    link = clean_text(_row_value(row, "新闻链接", "url", "link"))
    publisher = clean_text(_row_value(row, "文章来源", "mediaName", "publisher"))
    published = timestamp_as_utc(
        _row_value(row, "发布时间", "date"),
        CN_TIMEZONE,
    )
    summary = clean_text(_row_value(row, "新闻内容", "content"))
    if title is None and link is None:
        return None
    return NewsEntry(
        title=title,
        link=link,
        publisher=publisher,
        published_at=format_rfc3339(published) if published is not None else None,
        summary=summary,
    )


def _report_dates(from_date: date, to_date: date) -> list[str]:
    """CN distributions are declared against interim and annual reports.

    The ex-date of an annual plan usually lands in the following year, so the
    scan starts one report year before ``from_date``.
    """
    today = datetime.now(timezone.utc).date()
    dates: list[str] = []
    for year in range(from_date.year - 1, to_date.year + 1):
        for month, day in ((6, 30), (12, 31)):
            report_date = date(year, month, day)
            if report_date <= today:
                dates.append(f"{year}{month:02d}{day:02d}")
    return dates


def _fhps_rows(report_date: str) -> list[dict[str, Any]]:
    return _fhps_cache.get_or_fetch(
        report_date,
        ACTIONS_CACHE_SECONDS,
        lambda: {"rows": _fetch_fhps_rows(report_date)},
    )["rows"]


def _fetch_fhps_rows(report_date: str) -> list[dict[str, Any]]:
    try:
        frame = akshare_upstream.call("stock_fhps_em", date=report_date)
    except SidecarError:
        raise
    except Exception:
        # A report period without any published plans (for example the most
        # recent one) raises inside AKShare; it is an empty page, not a
        # failure of the whole lookup.
        return []
    return [dict(row) for row in _frame_rows(frame)]


def _row_events(row: Mapping[str, Any]) -> list[CorporateActionEvent]:
    ex_date = _cn_event_date(_row_value(row, "除权除息日"))
    if ex_date is None:
        return []
    events: list[CorporateActionEvent] = []
    dividend = _decimal(_row_value(row, "现金分红-现金分红比例"))
    if dividend is not None and dividend > 0:
        # The Eastmoney plan quotes cash per 10 shares; the wire amount is
        # the per-share value.
        events.append(
            CorporateActionEvent(
                kind="dividend",
                ex_date=ex_date,
                amount=float(dividend / 10),
            )
        )
    gift = _decimal(_row_value(row, "送转股份-送转总比例"))
    if gift is not None and gift > 0:
        # 10送转X turns one share into 1 + X/10 shares.
        events.append(
            CorporateActionEvent(
                kind="split",
                ex_date=ex_date,
                ratio=float(1 + gift / 10),
            )
        )
    return events


def _cn_event_date(value: Any) -> str | None:
    """Keep the upstream calendar date; ex-dates must not shift across UTC."""
    if (
        isinstance(value, (int, float))
        and not isinstance(value, bool)
        and value > 100_000_000_000
    ):
        # Eastmoney date columns are occasionally epoch milliseconds.
        parsed = timestamp_as_utc(value / 1000, CN_TIMEZONE)
        if parsed is None:
            return None
        return parsed.astimezone(ZoneInfo(CN_TIMEZONE)).date().isoformat()
    if hasattr(value, "date") and not isinstance(value, str):
        try:
            return value.date().isoformat()
        except (TypeError, ValueError):
            return None
    if isinstance(value, str):
        match = re.search(r"(\d{4})-(\d{2})-(\d{2})", value)
        if match is not None:
            return match.group(0)
    return None


def _decimal(value: Any) -> Decimal | None:
    if value is None or isinstance(value, bool):
        return None
    try:
        result = Decimal(str(value).strip())
    except (InvalidOperation, ValueError):
        return None
    return result if result.is_finite() else None
