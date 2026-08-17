"""AKShare calendar endpoints: earnings disclosure, dividends, economic events, IPOs.

Semantics notes:

- Earnings uses Eastmoney's 预约披露时间 (``stock_yysj_em``), which is keyed by
  报告期.  A date window is answered by probing the report periods whose
  statutory disclosure season intersects the window (annual: Jan-Apr of the
  following year; Q1: Apr; semi: Jul-Aug; Q3: Oct), then filtering each
  row's 实际披露时间/首次预约时间 into the window.  ``event_date`` prefers the
  actual disclosure date and falls back to the first appointment date.
- Dividends reuse the cached 分红送配 frames (``stock_fhps_em`` via
  ``akshare_news._fhps_rows``) for the report periods whose ex-dates can land
  on the requested day, and filter locally by 除权除息日.  The frame carries
  no 派息日 column, so ``payable_date`` stays null.
- Economic events are per-day Baidu finance calendar frames
  (``news_economic_baidu``); the range is capped at 31 days to bound the
  number of upstream calls inside the 12s pool deadline.
- IPOs come from ``stock_xgsglb_em`` (全部股票); the frame has no price-range
  columns, so ``issue_price_min``/``issue_price_max`` stay null.
"""

from __future__ import annotations

import hashlib
import re
from datetime import date, datetime, timedelta, timezone
from decimal import Decimal
from typing import Any, Mapping
from zoneinfo import ZoneInfo

from . import akshare_news, akshare_upstream
from .akshare_provider_conversion import _frame_rows, _optional_decimal, _row_value
from .conversion import clean_text, finite_float
from .errors import invalid_request
from .models import (
    CalendarDividendEntry,
    CalendarDividendsResponse,
    CalendarEarningsEntry,
    CalendarEarningsResponse,
    CalendarEconomicEntry,
    CalendarEconomicResponse,
    CalendarIpoEntry,
    CalendarIposResponse,
)
from .upstream import _TickerInfoCache

CALENDAR_CACHE_SECONDS = 600
ECONOMIC_MAX_DAYS = 31
MAX_EARNINGS_PERIODS = 6
CN_TIMEZONE = "Asia/Shanghai"

_PERIOD_LABELS = {"0331": "一季报", "0630": "中报", "0930": "三季报", "1231": "年报"}

_earnings_cache = _TickerInfoCache()
_economic_cache = _TickerInfoCache()
_ipos_cache = _TickerInfoCache()


def earnings(begin: date, end: date) -> CalendarEarningsResponse:
    periods = _earnings_periods(begin, end)
    entries: list[CalendarEarningsEntry] = []
    for period in periods:
        for row in _earnings_rows(period):
            entry = _earnings_entry(row, period, begin, end)
            if entry is not None:
                entries.append(entry)
    entries.sort(key=lambda entry: (entry.event_date or "", entry.symbol))
    return CalendarEarningsResponse(entries=entries)


def dividends(day: date) -> CalendarDividendsResponse:
    entries: list[CalendarDividendEntry] = []
    for report_date in _dividend_periods(day):
        for row in akshare_news._fhps_rows(report_date):
            entry = _dividend_entry(row, day)
            if entry is not None:
                entries.append(entry)
    entries.sort(key=lambda entry: entry.symbol)
    return CalendarDividendsResponse(entries=entries)


def economic(begin: date, end: date) -> CalendarEconomicResponse:
    if (end - begin).days + 1 > ECONOMIC_MAX_DAYS:
        raise invalid_request(
            "invalid_request",
            f"economic calendar range is limited to {ECONOMIC_MAX_DAYS} days",
        )
    entries: list[CalendarEconomicEntry] = []
    day = begin
    while day <= end:
        # 逐日升序；日内有时间的在前按时间排序，无时间的全天事件排最后。
        for row in sorted(_economic_rows(day), key=_economic_row_sort_key):
            entry = _economic_entry(row)
            if entry is not None:
                entries.append(entry)
        day += timedelta(days=1)
    return CalendarEconomicResponse(entries=entries)


def ipos() -> CalendarIposResponse:
    rows = _ipos_cache.get_or_fetch(
        "all",
        CALENDAR_CACHE_SECONDS,
        lambda: {
            "rows": [
                dict(row)
                for row in _frame_rows(
                    akshare_upstream.call("stock_xgsglb_em", symbol="全部股票")
                )
            ]
        },
    )["rows"]
    today = datetime.now(timezone.utc).date()
    entries = [
        entry
        for row in rows
        if (entry := _ipo_entry(row, today)) is not None
    ]
    return CalendarIposResponse(entries=entries)


def _earnings_periods(begin: date, end: date) -> list[date]:
    """Report periods whose disclosure season intersects the window."""
    periods: set[date] = set()
    for year in range(begin.year - 1, end.year + 1):
        spans = (
            (date(year - 1, 12, 31), date(year, 1, 1), date(year, 4, 30)),
            (date(year, 3, 31), date(year, 4, 1), date(year, 4, 30)),
            (date(year, 6, 30), date(year, 7, 1), date(year, 8, 31)),
            (date(year, 9, 30), date(year, 10, 1), date(year, 10, 31)),
        )
        for period, lo, hi in spans:
            if lo <= end and begin <= hi:
                periods.add(period)
    if len(periods) > MAX_EARNINGS_PERIODS:
        raise invalid_request(
            "invalid_request",
            "earnings calendar range spans too many report periods",
        )
    return sorted(periods, reverse=True)


def _earnings_rows(period: date) -> list[dict[str, Any]]:
    key = period.strftime("%Y%m%d")
    return _earnings_cache.get_or_fetch(
        key,
        CALENDAR_CACHE_SECONDS,
        lambda: {
            "rows": [
                dict(row)
                for row in _frame_rows(
                    akshare_upstream.call(
                        "stock_yysj_em",
                        symbol="沪深A股",
                        date=key,
                    )
                )
            ]
        },
    )["rows"]


def _earnings_entry(
    row: Mapping[str, Any],
    period: date,
    begin: date,
    end: date,
) -> CalendarEarningsEntry | None:
    code = clean_text(_row_value(row, "股票代码", "代码"))
    if code is None:
        return None
    market = _cn_market(code)
    if market is None:
        return None
    event = _date_text(
        _row_value(row, "实际披露时间") or _row_value(row, "首次预约时间")
    )
    if event is None or not (begin.isoformat() <= event <= end.isoformat()):
        return None
    return CalendarEarningsEntry(
        instrument_id=f"{market}.{code}",
        name=clean_text(_row_value(row, "股票简称", "名称")),
        symbol=code,
        event_date=event,
        period_text=f"{period.year}{_PERIOD_LABELS[period.strftime('%m%d')]}",
        market_cap=None,
        price=None,
    )


def _dividend_periods(day: date) -> list[str]:
    """Report periods whose ex-dates can land on the requested day."""
    return [
        f"{day.year}0630",
        f"{day.year - 1}1231",
        f"{day.year - 1}0630",
    ]


def _dividend_entry(row: Mapping[str, Any], day: date) -> CalendarDividendEntry | None:
    ex_date = _date_text(_row_value(row, "除权除息日"))
    if ex_date != day.isoformat():
        return None
    code = clean_text(_row_value(row, "代码"))
    if code is None:
        return None
    market = _cn_market(code)
    if market is None:
        return None
    statement = _dividend_statement(row)
    if statement is None:
        return None
    return CalendarDividendEntry(
        instrument_id=f"{market}.{code}",
        name=clean_text(_row_value(row, "名称")),
        symbol=code,
        statement=statement,
        ex_date=ex_date,
        record_date=_date_text(_row_value(row, "股权登记日")),
        payable_date=None,
    )


def _dividend_statement(row: Mapping[str, Any]) -> str | None:
    parts: list[str] = []
    gift = _optional_decimal(row, "送转股份-送转总比例")
    if gift is not None and gift > 0:
        parts.append(f"10送转{_decimal_plain(gift)}")
    dividend = _optional_decimal(row, "现金分红-现金分红比例")
    if dividend is not None and dividend > 0:
        # 惯例写法 "10送转x派y元(含税)"：有送转时派息段省略重复的 "10"
        prefix = "派" if parts else "10派"
        parts.append(f"{prefix}{_decimal_plain(dividend)}元(含税)")
    return "".join(parts) or None


def _decimal_plain(value: Decimal) -> str:
    rendered = format(value, "f")
    if "." in rendered:
        rendered = rendered.rstrip("0").rstrip(".")
    return rendered


def _economic_rows(day: date) -> list[dict[str, Any]]:
    key = day.strftime("%Y%m%d")
    return _economic_cache.get_or_fetch(
        key,
        CALENDAR_CACHE_SECONDS,
        lambda: {
            "rows": [
                dict(row)
                for row in _frame_rows(
                    akshare_upstream.call("news_economic_baidu", date=key)
                )
            ]
        },
    )["rows"]


def _economic_entry(row: Mapping[str, Any]) -> CalendarEconomicEntry | None:
    day = _date_text(_row_value(row, "日期"))
    title = clean_text(_row_value(row, "事件", "title"))
    if day is None or title is None:
        return None
    region = clean_text(_row_value(row, "地区", "国家", "region"))
    time_text = clean_text(_row_value(row, "时间")) or ""
    timestamp = _event_timestamp(day, time_text)
    importance = _importance(_row_value(row, "重要性"))
    return CalendarEconomicEntry(
        event_id=hashlib.sha1(
            f"{day}|{time_text}|{title}|{region}".encode("utf-8")
        ).hexdigest()[:16],
        title=title,
        region=region,
        event_date=day,
        event_timestamp=timestamp,
        importance=importance,
        previous_value=_value_text(_row_value(row, "前值")),
        forecast_value=_value_text(_row_value(row, "预期")),
        actual_value=_value_text(_row_value(row, "公布")),
    )


def _economic_row_sort_key(row: Mapping[str, Any]) -> tuple[int, str]:
    time_text = clean_text(_row_value(row, "时间")) or ""
    if re.fullmatch(r"\d{1,2}:\d{2}(?::\d{2})?", time_text.strip()):
        return (0, time_text.strip())
    return (1, "")


def _event_timestamp(day: str, time_text: str) -> int | None:
    match = re.fullmatch(r"(\d{1,2}):(\d{2})(?::\d{2})?", time_text.strip())
    if match is None:
        return None  # 全天事件无具体时刻，timestamp 置 null 而非伪造零点
    try:
        moment = datetime.fromisoformat(day).replace(
            hour=int(match.group(1)),
            minute=int(match.group(2)),
            tzinfo=ZoneInfo(CN_TIMEZONE),
        )
    except ValueError:
        return None
    return int(moment.timestamp())


def _importance(value: Any) -> int | None:
    number = finite_float(value)
    if number is None:
        return None
    return max(1, min(3, int(number)))


def _value_text(value: Any) -> str | None:
    number = finite_float(value)
    if number is not None:
        return f"{number:g}"
    return clean_text(value)


def _ipo_entry(row: Mapping[str, Any], today: date) -> CalendarIpoEntry | None:
    code = clean_text(_row_value(row, "股票代码", "代码"))
    if code is None:
        return None
    market = _ipo_market(row, code)
    if market is None:
        return None
    listing = _date_text(_row_value(row, "上市日期"))
    listed = listing is not None and listing <= today.isoformat()
    return CalendarIpoEntry(
        instrument_id=f"{market}.{code}",
        name=clean_text(_row_value(row, "股票简称", "名称")),
        symbol=code,
        status="listed" if listed else "pending",
        listing_date=listing,
        issue_volume=_float(row, "发行总数"),
        issue_price=_float(row, "发行价格"),
        issue_price_min=None,
        issue_price_max=None,
    )


def _ipo_market(row: Mapping[str, Any], code: str) -> str | None:
    exchange = clean_text(_row_value(row, "交易所")) or ""
    if "上海" in exchange:
        return "SH"
    if "深圳" in exchange:
        return "SZ"
    if "北京" in exchange:
        return None
    return _cn_market(code)


def _cn_market(code: str) -> str | None:
    if not re.fullmatch(r"\d{6}", code):
        return None
    if code.startswith("6"):
        return "SH"
    if code.startswith(("0", "3")):
        return "SZ"
    return None


def _float(row: Mapping[str, Any], *names: str) -> float | None:
    value = _optional_decimal(row, *names)
    return float(value) if value is not None else None


def _date_text(value: Any) -> str | None:
    if value is None:
        return None
    if hasattr(value, "isoformat") and not isinstance(value, str):
        try:
            return value.isoformat()[:10]
        except (TypeError, ValueError):
            return None
    match = re.search(r"\d{4}-\d{2}-\d{2}", str(value))
    return match.group(0) if match else None
