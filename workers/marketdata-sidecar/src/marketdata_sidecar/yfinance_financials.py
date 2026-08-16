"""Yahoo Finance yearly financial statements for US/HK instruments."""

from __future__ import annotations

from . import upstream
from .conversion import clean_text
from .models import FinancialField, FinancialsResponse
from .research_common import (
    financial_periods,
    research_not_found,
    yearly_period_text,
    yfinance_instrument,
)

# Stable field catalog: field_id -> (localized label, Yahoo row label).
_FIELD_CATALOG = {
    "income": (
        ("total_revenue", "营业总收入", "Total Revenue"),
        ("gross_profit", "毛利", "Gross Profit"),
        ("operating_income", "营业利润", "Operating Income"),
        ("net_income", "净利润", "Net Income"),
        ("basic_eps", "基本每股收益", "Basic EPS"),
    ),
    "balance": (
        ("total_assets", "总资产", "Total Assets"),
        ("total_liabilities", "总负债", "Total Liabilities Net Minority Interest"),
        ("total_equity", "股东权益", "Total Equity Gross Minority Interest"),
        ("cash_and_equivalents", "现金及等价物", "Cash And Cash Equivalents"),
        ("total_debt", "总债务", "Total Debt"),
    ),
    "cashflow": (
        ("operating_cash_flow", "经营现金流净额", "Operating Cash Flow"),
        ("investing_cash_flow", "投资现金流净额", "Investing Cash Flow"),
        ("financing_cash_flow", "筹资现金流净额", "Financing Cash Flow"),
        ("free_cash_flow", "自由现金流", "Free Cash Flow"),
        ("capital_expenditure", "资本开支", "Capital Expenditure"),
    ),
}


def financials(market: str, symbol: str, statement: str) -> FinancialsResponse:
    instrument = yfinance_instrument(market, symbol)
    data = upstream.ticker_financials(instrument.yahoo_symbol, statement)
    periods = data.get("periods") or []
    rows = data.get("rows") or {}
    if not periods:
        raise research_not_found("financials", instrument.instrument_id)
    catalog = _FIELD_CATALOG[statement]
    entries = []
    for index, period in enumerate(periods):
        values = {
            field_id: _row_value(rows.get(label), index)
            for field_id, _display, label in catalog
        }
        entries.append((yearly_period_text(period), values))
    currency = None
    info = upstream.ticker_info(
        instrument.yahoo_symbol,
        max_age_seconds=upstream.SECURITY_CACHE_SECONDS,
    )
    if info:
        currency = clean_text(info.get("currency"))
    return FinancialsResponse(
        instrument_id=instrument.instrument_id,
        statement=statement,
        currency=currency,
        fields=[
            FinancialField(field_id=field_id, display_name=display)
            for field_id, display, _label in catalog
        ],
        periods=financial_periods(entries, [field_id for field_id, _d, _l in catalog]),
    )


def _row_value(series: list | None, index: int) -> float | None:
    if series is None or index >= len(series):
        return None
    value = series[index]
    return float(value) if isinstance(value, (int, float)) else None
