"""AKShare CN yearly financial statements (Eastmoney F10).

The ``*_by_yearly_em`` frames pass Eastmoney's raw uppercase column names
through; each wire field lists candidate columns and the first present one
wins.  The catalog below was written against akshare 1.18.91's pass-through
behavior for ``lrbAjaxNew``/``zcfzbAjaxNew``/``xjllbAjaxNew``.
"""

from __future__ import annotations

from typing import Any, Mapping

from . import akshare_upstream
from .akshare_provider_conversion import _frame_rows, _optional_decimal, _row_value
from .models import FinancialField, FinancialsResponse
from .research_common import (
    akshare_research_identity,
    financial_periods,
    require_cn_leaf,
    research_not_found,
    yearly_period_text,
)
from .upstream import _TickerInfoCache

FINANCIALS_CACHE_SECONDS = 3600

_STATEMENT_FUNCTIONS = {
    "income": "stock_profit_sheet_by_yearly_em",
    "balance": "stock_balance_sheet_by_yearly_em",
    "cashflow": "stock_cash_flow_sheet_by_yearly_em",
}

# field_id -> (localized label, candidate Eastmoney columns)
_FIELD_CATALOG = {
    "income": (
        ("total_revenue", "营业总收入", ("TOTAL_OPERATE_INCOME", "OPERATE_INCOME")),
        ("operating_cost", "营业成本", ("OPERATE_COST", "TOTAL_OPERATE_COST")),
        ("operating_profit", "营业利润", ("OPERATE_PROFIT",)),
        ("total_profit", "利润总额", ("TOTAL_PROFIT",)),
        ("net_profit", "净利润", ("NETPROFIT", "NET_PROFIT")),
        ("net_profit_attributable", "归母净利润", ("PARENT_NETPROFIT",)),
        ("basic_eps", "基本每股收益", ("BASIC_EPS", "EPS")),
    ),
    "balance": (
        ("total_assets", "总资产", ("TOTAL_ASSETS",)),
        ("total_liabilities", "总负债", ("TOTAL_LIABILITIES",)),
        ("total_equity", "股东权益合计", ("TOTAL_PARENT_EQUITY", "TOTAL_EQUITY")),
        ("monetary_funds", "货币资金", ("MONETARYFUNDS", "MONETARY_FUNDS")),
        ("inventory", "存货", ("INVENTORY",)),
        ("accounts_receivable", "应收账款", ("ACCOUNTS_RECE", "ACCOUNTS_RECEIVABLE")),
    ),
    "cashflow": (
        ("operating_cash_flow", "经营活动现金流净额", ("NETCASH_OPERATE",)),
        ("investing_cash_flow", "投资活动现金流净额", ("NETCASH_INVEST",)),
        ("financing_cash_flow", "筹资活动现金流净额", ("NETCASH_FINANCE",)),
        ("cash_net_increase", "现金及等价物净增加额", ("CCE_ADD", "CCE_ADD_BALANCE")),
        ("sales_cash", "销售商品提供劳务收到的现金", ("SALES_SERVICES",)),
    ),
}

_financials_cache = _TickerInfoCache()


def financials(market: str, symbol: str, statement: str) -> FinancialsResponse:
    requested, leaf, code = akshare_research_identity(market, symbol, "financials")
    require_cn_leaf(leaf, "financials", market)
    instrument_id = f"{requested}.{code}"
    rows = _financials_cache.get_or_fetch(
        f"{leaf}:{code}:{statement}",
        FINANCIALS_CACHE_SECONDS,
        lambda: {"rows": _fetch_rows(leaf, code, statement)},
    )["rows"]
    if not rows:
        raise research_not_found("financials", instrument_id)
    catalog = _FIELD_CATALOG[statement]
    entries = [
        (yearly_period_text(row["report_date"]), row["values"]) for row in rows
    ]
    return FinancialsResponse(
        instrument_id=instrument_id,
        statement=statement,
        currency="CNY",
        fields=[
            FinancialField(field_id=field_id, display_name=display)
            for field_id, display, _columns in catalog
        ],
        periods=financial_periods(entries, [field_id for field_id, _d, _c in catalog]),
    )


def _fetch_rows(leaf: str, code: str, statement: str) -> list[dict[str, Any]]:
    frame = akshare_upstream.call(
        _STATEMENT_FUNCTIONS[statement],
        symbol=f"{leaf}{code}",
    )
    catalog = _FIELD_CATALOG[statement]
    rows = []
    for raw in _frame_rows(frame):
        report_date = _row_value(raw, "REPORT_DATE", "report_date")
        if report_date is None:
            continue
        rows.append(
            {
                "report_date": str(report_date)[:10],
                "values": {
                    field_id: value
                    for field_id, _display, columns in catalog
                    if (value := _first_decimal(raw, columns)) is not None
                },
            }
        )
    rows.sort(key=lambda row: row["report_date"], reverse=True)
    return rows


def _first_decimal(row: Mapping[str, Any], columns: tuple[str, ...]) -> float | None:
    value = _optional_decimal(row, *columns)
    return float(value) if value is not None else None
