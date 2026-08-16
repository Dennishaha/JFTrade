"""AKShare security metadata and snapshot projection."""

from __future__ import annotations

from decimal import Decimal
from typing import Any, Mapping, Protocol

from . import akshare_upstream
from .akshare_models import AKSecurityResponse, AKSnapshotQuote, AKSnapshotResponse
from .akshare_provider_conversion import (
    _decimal_text,
    _frame_rows,
    _optional_decimal,
    _optional_price,
    _required_decimal,
    _row_timestamp,
    _row_value,
    _utc_now,
    _validate_snapshot_ohlc,
)
from .conversion import clean_text, finite_float, format_rfc3339, non_negative_int
from .upstream import SECURITY_CACHE_SECONDS, _TickerInfoCache

MARKET_CURRENCY = {"US": "USD", "HK": "HKD", "SH": "CNY", "SZ": "CNY"}

# CN individual-info enrichment (行业/总股本/上市时间) changes at most daily.
CN_ENRICHMENT_CACHE_SECONDS = SECURITY_CACHE_SECONDS
_enrichment_cache = _TickerInfoCache()


class QuoteInstrument(Protocol):
    market: str
    symbol: str
    instrument_id: str
    name: str
    exchange: str | None
    security_type: str | None
    supported_periods: tuple[str, ...]
    row: Mapping[str, Any]

    @property
    def timezone(self) -> str: ...

    @property
    def volume_multiplier(self) -> Decimal: ...


def security(instrument: QuoteInstrument) -> AKSecurityResponse:
    row = instrument.row
    enrichment = _cn_enrichment(instrument)
    return AKSecurityResponse(
        market=instrument.market,
        symbol=instrument.symbol,
        instrument_id=instrument.instrument_id,
        name=instrument.name or instrument.symbol,
        exchange=instrument.exchange,
        currency=MARKET_CURRENCY[instrument.market],
        timezone=instrument.timezone,
        security_type=instrument.security_type,
        industry=enrichment.get("industry"),
        market_cap=non_negative_int(_row_value(row, "总市值")),
        trailing_pe=finite_float(_row_value(row, "市盈率", "市盈率-动态")),
        price_to_book=finite_float(_row_value(row, "市净率")),
        shares_outstanding=enrichment.get("shares_outstanding"),
        supported_periods=list(instrument.supported_periods),
    )


def _cn_enrichment(instrument: QuoteInstrument) -> dict[str, Any]:
    """Best-effort CN A-share fundamentals from Eastmoney's per-stock endpoint.

    Enrichment never fails the security response: any upstream, pool, or
    schema problem degrades to the spot-only projection.
    """
    if instrument.market not in {"SH", "SZ"}:
        return {}
    if instrument.security_type != "EQUITY":
        return {}
    try:
        return _enrichment_cache.get_or_fetch(
            instrument.symbol,
            CN_ENRICHMENT_CACHE_SECONDS,
            lambda: _fetch_cn_enrichment(instrument.symbol),
        )
    except Exception:
        return {}


def _fetch_cn_enrichment(symbol: str) -> dict[str, Any]:
    frame = akshare_upstream.call("stock_individual_info_em", symbol=symbol)
    items: dict[str, Any] = {}
    for row in _frame_rows(frame):
        item = clean_text(_row_value(row, "item", "项目"))
        if item is not None:
            items[item] = _row_value(row, "value", "值")
    industry = clean_text(items.get("行业"))
    return {
        # Eastmoney renders missing values as a bare dash.
        "industry": None if industry == "-" else industry,
        "shares_outstanding": _share_count(items.get("总股本")),
    }


def _share_count(value: Any) -> int | None:
    if value is None or isinstance(value, bool):
        return None
    if isinstance(value, (int, float)):
        return non_negative_int(value)
    text = str(value).strip().replace(",", "")
    multiplier = 1
    if text.endswith("亿"):
        multiplier, text = 100_000_000, text[:-1]
    elif text.endswith("万"):
        multiplier, text = 10_000, text[:-1]
    number = finite_float(text)
    if number is None:
        return None
    return non_negative_int(number * multiplier)


def snapshot(instrument: QuoteInstrument) -> AKSnapshotResponse:
    row = instrument.row
    price = _required_decimal(row, "最新价", "最新", "price")
    quote_at = _row_timestamp(row, instrument.timezone)
    volume = _optional_decimal(row, "成交量", "volume", minimum=Decimal(0))
    if volume is not None:
        volume *= instrument.volume_multiplier
    turnover = _optional_decimal(row, "成交额", "turnover", minimum=Decimal(0))
    open_price = _optional_price(
        row,
        "今开",
        "开盘",
        "开盘价",
        "open",
    )
    high = _optional_price(row, "最高", "最高价", "high")
    low = _optional_price(row, "最低", "最低价", "low")
    _validate_snapshot_ohlc(price, open_price, high, low)
    regular = AKSnapshotQuote(
        price=_decimal_text(price),
        high_price=_decimal_text(high),
        low_price=_decimal_text(low),
        volume=_decimal_text(volume),
        turnover=_decimal_text(turnover),
        change_value=_decimal_text(_optional_decimal(row, "涨跌额", "change")),
        change_rate=_decimal_text(_optional_decimal(row, "涨跌幅", "change_rate")),
        quote_at=quote_at,
    )
    previous_close = _optional_price(
        row,
        "昨收",
        "昨收价",
        "previous_close",
    )
    return AKSnapshotResponse(
        market=instrument.market,
        symbol=instrument.symbol,
        instrument_id=instrument.instrument_id,
        price=_decimal_text(price) or "0",
        bid=_decimal_text(_optional_price(row, "买一", "买入", "bid")),
        ask=_decimal_text(_optional_price(row, "卖一", "卖出", "ask")),
        open_price=_decimal_text(open_price),
        high_price=_decimal_text(high),
        low_price=_decimal_text(low),
        previous_close_price=_decimal_text(previous_close),
        last_close_price=_decimal_text(previous_close),
        regular_quote=regular,
        volume=_decimal_text(volume),
        turnover=_decimal_text(turnover),
        quote_at=quote_at,
        observed_at=format_rfc3339(_utc_now()),
        currency=MARKET_CURRENCY[instrument.market],
        exchange=instrument.exchange,
    )
