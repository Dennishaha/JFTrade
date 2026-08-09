"""AKShare security metadata and snapshot projection."""

from __future__ import annotations

from decimal import Decimal
from typing import Any, Mapping, Protocol

from .akshare_models import AKSecurityResponse, AKSnapshotQuote, AKSnapshotResponse
from .akshare_provider_conversion import (
    _decimal_text,
    _optional_decimal,
    _optional_price,
    _required_decimal,
    _row_timestamp,
    _utc_now,
    _validate_snapshot_ohlc,
)
from .conversion import format_rfc3339

MARKET_CURRENCY = {"US": "USD", "HK": "HKD", "SH": "CNY", "SZ": "CNY"}


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
    return AKSecurityResponse(
        market=instrument.market,
        symbol=instrument.symbol,
        instrument_id=instrument.instrument_id,
        name=instrument.name or instrument.symbol,
        exchange=instrument.exchange,
        currency=MARKET_CURRENCY[instrument.market],
        timezone=instrument.timezone,
        security_type=instrument.security_type,
        supported_periods=list(instrument.supported_periods),
    )


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
