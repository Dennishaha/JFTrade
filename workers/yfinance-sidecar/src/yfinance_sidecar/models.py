"""Pydantic wire models for the sidecar's private HTTP API."""

from __future__ import annotations

from pydantic import BaseModel, ConfigDict, Field


class WireModel(BaseModel):
    model_config = ConfigDict(extra="forbid")


class ErrorBody(WireModel):
    code: str
    message: str


class ErrorEnvelope(WireModel):
    error: ErrorBody


class HealthResponse(WireModel):
    ok: bool
    yfinance_version: str


class TradingWindow(WireModel):
    start_minute: int
    end_minute: int
    label: str


class MarketPrecision(WireModel):
    price: int
    quote: int


class MarketProfile(WireModel):
    code: str
    resolved_market: str
    preferred_prefix: str
    display_name: str
    quote_currency: str
    timezone: str
    supports_extended_hours: bool
    requires_exchange_prefix: bool
    aliases: list[str]
    regular_sessions: list[TradingWindow]
    precision: MarketPrecision
    tick_size: float


class MarketsResponse(WireModel):
    markets: list[MarketProfile]


class SearchEntry(WireModel):
    market: str
    resolved_market: str
    instrument_id: str
    code: str
    symbol: str
    name: str | None = None
    security_type: str | None = None
    exchange: str | None = None
    selectable: bool
    source: str


class SearchResponse(WireModel):
    entries: list[SearchEntry]


class SecurityResponse(WireModel):
    market: str
    symbol: str
    instrument_id: str
    name: str
    exchange: str | None = None
    currency: str | None = None
    timezone: str | None = None
    security_type: str | None = None
    industry: str | None = None
    sector: str | None = None
    website: str | None = None
    business_summary: str | None = None
    market_cap: int | None = None
    trailing_pe: float | None = None
    forward_pe: float | None = None
    trailing_eps: float | None = None
    forward_eps: float | None = None
    dividend_rate: float | None = None
    dividend_yield: float | None = None
    fifty_two_week_high: float | None = None
    fifty_two_week_low: float | None = None
    average_volume: int | None = None
    shares_outstanding: int | None = None
    source: str


class SnapshotQuote(WireModel):
    """One regular or extended-hours quote block from Yahoo metadata."""

    price: float | None = None
    high_price: float | None = None
    low_price: float | None = None
    volume: int | None = None
    turnover: float | None = None
    change_value: float | None = None
    change_rate: float | None = None
    quote_at: str | None = None


class SnapshotResponse(WireModel):
    market: str
    symbol: str
    instrument_id: str
    price: float
    bid: float | None = None
    ask: float | None = None
    open_price: float | None = None
    high_price: float | None = None
    low_price: float | None = None
    previous_close_price: float | None = None
    last_close_price: float | None = None
    regular_quote: SnapshotQuote | None = None
    pre_market_quote: SnapshotQuote | None = None
    after_market_quote: SnapshotQuote | None = None
    volume: int
    turnover: float | None = None
    quote_at: str | None = None
    observed_at: str
    delayed: bool
    delay_minutes: int = Field(ge=0)
    currency: str | None = None
    exchange: str | None = None
    source: str


class Candle(WireModel):
    at: str
    open: float
    high: float
    low: float
    close: float
    volume: int


class CandlesResponse(WireModel):
    market: str
    symbol: str
    instrument_id: str
    period: str
    extended_hours: bool
    candles: list[Candle]
    total_returned: int = Field(ge=0)
    source: str
