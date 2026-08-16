"""AKShare-specific JSON models with lossless decimal strings."""

from __future__ import annotations

from pydantic import BaseModel, ConfigDict, Field

from .models import RankingsEntry


class AKWireModel(BaseModel):
    model_config = ConfigDict(extra="forbid")


class AKSearchEntry(AKWireModel):
    market: str
    resolved_market: str
    instrument_id: str
    code: str
    symbol: str
    name: str | None = None
    security_type: str | None = None
    exchange: str | None = None
    selectable: bool = True
    source: str = "akshare:eastmoney"
    supported_periods: list[str]


class AKSearchResponse(AKWireModel):
    entries: list[AKSearchEntry]


class AKSecurityResponse(AKWireModel):
    market: str
    symbol: str
    instrument_id: str
    name: str
    exchange: str | None = None
    currency: str | None = None
    timezone: str
    security_type: str | None = None
    industry: str | None = None
    market_cap: int | None = None
    trailing_pe: float | None = None
    price_to_book: float | None = None
    shares_outstanding: int | None = None
    supported_periods: list[str]
    source: str = "akshare:eastmoney"


class AKSnapshotQuote(AKWireModel):
    price: str
    high_price: str | None = None
    low_price: str | None = None
    volume: str | None = None
    turnover: str | None = None
    change_value: str | None = None
    change_rate: str | None = None
    quote_at: str | None = None


class AKSnapshotResponse(AKWireModel):
    market: str
    symbol: str
    instrument_id: str
    price: str
    bid: str | None = None
    ask: str | None = None
    open_price: str | None = None
    high_price: str | None = None
    low_price: str | None = None
    previous_close_price: str | None = None
    last_close_price: str | None = None
    regular_quote: AKSnapshotQuote | None = None
    pre_market_quote: None = None
    after_market_quote: None = None
    volume: str | None = None
    turnover: str | None = None
    quote_at: str | None = None
    observed_at: str
    delayed: bool = True
    delay_minutes: int = Field(default=15, ge=0)
    currency: str | None = None
    exchange: str | None = None
    source: str = "akshare:eastmoney"


class AKCandle(AKWireModel):
    at: str
    open: str
    high: str
    low: str
    close: str
    volume: str | None = None


class AKCandlesResponse(AKWireModel):
    market: str
    symbol: str
    instrument_id: str
    period: str
    extended_hours: bool = False
    candles: list[AKCandle]
    total_returned: int = Field(ge=0)
    has_more: bool
    next_before: str | None = None
    source: str = "akshare:eastmoney"
    adjustment: str = "none"


class AKIndexConstituent(AKWireModel):
    code: str
    name: str | None = None
    weight: float | None = None


class AKIndexConstituentsResponse(AKWireModel):
    market: str
    symbol: str
    instrument_id: str
    constituents: list[AKIndexConstituent]
    source: str = "akshare-index-constituents"


class AKIndustryBoard(AKWireModel):
    name: str
    change_rate: float | None = None
    turnover: float | None = None
    volume: float | None = None
    leading_stock_name: str | None = None
    leading_stock_change_rate: float | None = None


class AKIndustriesResponse(AKWireModel):
    market: str
    kind: str
    boards: list[AKIndustryBoard]
    source: str = "akshare-industries"


class AKIndustryMembersResponse(AKWireModel):
    market: str
    kind: str
    board: str
    entries: list[RankingsEntry]
    source: str = "akshare-industries"


class AKBatchRequest(AKWireModel):
    instrument_ids: list[str] = Field(min_length=1, max_length=100)


class AKBatchError(AKWireModel):
    instrument_id: str
    code: str
    message: str


class AKBatchResponse(AKWireModel):
    entries: list[AKSnapshotResponse]
    errors: list[AKBatchError]
