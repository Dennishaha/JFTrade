"""Pydantic wire models for the sidecar's private HTTP API."""

from __future__ import annotations

from typing import Literal

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
    runtime_state: Literal["warming", "ready", "failed"]
    warmup_error: str | None = None


class ProcessHealthResponse(WireModel):
    ok: bool
    version: str


class ProviderHealthResponse(WireModel):
    ok: bool
    provider: str
    provider_version: str | None = None
    runtime_state: Literal["warming", "ready", "failed"]
    warmup_error: str | None = None


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
    supported_periods: list[str]


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
    supported_periods: list[str]
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
    volume: int | None = None
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
    has_more: bool
    next_before: str | None = None
    source: str
    adjustment: str = "none"


class RankingsEntry(WireModel):
    instrument_id: str
    name: str | None = None
    price: float | None = None
    change_rate: float | None = None
    change_amount: float | None = None
    volume: float | None = None
    turnover: float | None = None
    turnover_ratio: float | None = None
    pe_ttm: float | None = None
    market_cap: float | None = None


class RankingsResponse(WireModel):
    market: str
    kind: str
    entries: list[RankingsEntry]
    source: str


class ProfileField(WireModel):
    name: str
    value: str


class ProfileGroup(WireModel):
    title: str
    fields: list[ProfileField]


class ProfileResponse(WireModel):
    instrument_id: str
    market: str
    symbol: str
    currency: str | None = None
    groups: list[ProfileGroup]


class FinancialField(WireModel):
    field_id: str
    display_name: str


class FinancialValue(WireModel):
    data: float | None = None
    yoy: float | None = None
    qoq: float | None = None


class FinancialPeriod(WireModel):
    period_text: str
    values: dict[str, FinancialValue]


class FinancialsResponse(WireModel):
    instrument_id: str
    statement: str
    currency: str | None = None
    fields: list[FinancialField]
    periods: list[FinancialPeriod]


class AnalystTargetPrice(WireModel):
    lowest: float | None = None
    average: float | None = None
    highest: float | None = None


class AnalystDistribution(WireModel):
    strong_buy: float
    buy: float
    hold: float
    underperform: float
    sell: float


class AnalystResponse(WireModel):
    instrument_id: str
    rating: float | None = None
    analyst_count: int | None = None
    target_price: AnalystTargetPrice | None = None
    distribution: AnalystDistribution | None = None
    update_time: str | None = None


class OwnershipItem(WireModel):
    name: str
    holder_pct: float | None = None


class OwnershipGroup(WireModel):
    kind: str
    static_date: str | None = None
    items: list[OwnershipItem]


class OwnershipResponse(WireModel):
    instrument_id: str
    groups: list[OwnershipGroup]


class CalendarEarningsEntry(WireModel):
    instrument_id: str
    name: str | None = None
    symbol: str
    event_date: str | None = None
    period_text: str | None = None
    market_cap: float | None = None
    price: float | None = None


class CalendarEarningsResponse(WireModel):
    entries: list[CalendarEarningsEntry]


class CalendarDividendEntry(WireModel):
    instrument_id: str
    name: str | None = None
    symbol: str
    statement: str | None = None
    ex_date: str | None = None
    record_date: str | None = None
    payable_date: str | None = None


class CalendarDividendsResponse(WireModel):
    entries: list[CalendarDividendEntry]


class CalendarEconomicEntry(WireModel):
    event_id: str
    title: str | None = None
    region: str | None = None
    event_timestamp: int | None = None
    importance: int | None = None
    previous_value: str | None = None
    forecast_value: str | None = None
    actual_value: str | None = None


class CalendarEconomicResponse(WireModel):
    entries: list[CalendarEconomicEntry]


class CalendarIpoEntry(WireModel):
    instrument_id: str
    name: str | None = None
    symbol: str
    status: str
    listing_date: str | None = None
    issue_volume: float | None = None
    issue_price: float | None = None
    issue_price_min: float | None = None
    issue_price_max: float | None = None


class CalendarIposResponse(WireModel):
    entries: list[CalendarIpoEntry]


class MacroIndicatorInfo(WireModel):
    indicator_id: str
    name: str
    region: str
    unit: str
    unit_type: int
    frequency: str


class MacroCategory(WireModel):
    category_name: str
    indicators: list[MacroIndicatorInfo]


class MacroIndicatorsResponse(WireModel):
    categories: list[MacroCategory]


class MacroHistoryEntry(WireModel):
    data_time: str
    value: float | None = None
    predict_value: float | None = None
    previous_value: float | None = None
    unit: str
    unit_type: int


class MacroHistoryResponse(WireModel):
    indicator_id: str
    entries: list[MacroHistoryEntry]


class NewsEntry(WireModel):
    title: str | None = None
    link: str | None = None
    publisher: str | None = None
    published_at: str | None = None
    summary: str | None = None


class NewsResponse(WireModel):
    market: str
    symbol: str
    instrument_id: str
    entries: list[NewsEntry]
    source: str


class CorporateActionEvent(WireModel):
    kind: Literal["dividend", "split"]
    ex_date: str
    amount: float | None = None
    ratio: float | None = None


class CorporateActionsResponse(WireModel):
    market: str
    symbol: str
    instrument_id: str
    events: list[CorporateActionEvent]
    source: str
