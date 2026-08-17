"""akshare calendar routes: earnings, dividends, economic, ipos."""

from __future__ import annotations

from datetime import date, datetime, timedelta, timezone
from typing import Any
from zoneinfo import ZoneInfo

import httpx
import pandas as pd
import pytest

from marketdata_sidecar import akshare_news, akshare_upstream, upstream


def _empty() -> pd.DataFrame:
    return pd.DataFrame()


def _yysj_frame() -> pd.DataFrame:
    return pd.DataFrame(
        [
            {
                "股票代码": "600519",
                "股票简称": "贵州茅台",
                "首次预约时间": date(2026, 3, 28),
                "实际披露时间": date(2026, 3, 30),
            },
            {
                # No actual disclosure yet: falls back to the appointment date.
                "股票代码": "000001",
                "股票简称": "平安银行",
                "首次预约时间": date(2026, 4, 25),
                "实际披露时间": None,
            },
            {
                # Outside the requested window: filtered out.
                "股票代码": "601111",
                "股票简称": "中国国航",
                "首次预约时间": date(2026, 9, 1),
                "实际披露时间": None,
            },
            {
                # Beijing listings are outside the JFTrade market model.
                "股票代码": "830799",
                "股票简称": "北交所标的",
                "首次预约时间": date(2026, 3, 30),
                "实际披露时间": None,
            },
        ]
    )


@pytest.mark.asyncio
async def test_earnings_window_maps_report_periods(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    calls: list[str] = []

    def fake_call(function_name: str, **kwargs: Any) -> pd.DataFrame:
        assert function_name == "stock_yysj_em"
        calls.append(kwargs["date"])
        return _yysj_frame() if kwargs["date"] == "20251231" else _empty()

    monkeypatch.setattr(akshare_upstream, "call", fake_call)

    response = await client.get(
        "/providers/akshare/calendar/earnings",
        params={"begin_date": "2026-03-01", "end_date": "2026-04-30"},
    )

    assert response.status_code == 200
    # The window intersects the FY2025 annual and 2026 Q1 disclosure seasons.
    assert calls == ["20260331", "20251231"]
    entries = response.json()["entries"]
    assert [entry["instrument_id"] for entry in entries] == ["SH.600519", "SZ.000001"]
    first = entries[0]
    assert first["name"] == "贵州茅台"
    assert first["symbol"] == "600519"
    assert first["event_date"] == "2026-03-30"
    assert first["period_text"] == "2025年报"
    assert first["market_cap"] is None
    assert first["price"] is None
    assert entries[1]["event_date"] == "2026-04-25"


@pytest.mark.asyncio
async def test_earnings_invalid_window_rejected(
    client: httpx.AsyncClient,
) -> None:
    bad_date = await client.get(
        "/providers/akshare/calendar/earnings",
        params={"begin_date": "2026-13-01", "end_date": "2026-04-30"},
    )
    inverted = await client.get(
        "/providers/akshare/calendar/earnings",
        params={"begin_date": "2026-05-01", "end_date": "2026-04-30"},
    )
    too_wide = await client.get(
        "/providers/akshare/calendar/earnings",
        params={"begin_date": "2024-01-01", "end_date": "2026-12-31"},
    )

    for response in (bad_date, inverted, too_wide):
        assert response.status_code == 400
        assert response.json()["error"]["code"] == "invalid_request"


@pytest.mark.asyncio
async def test_dividends_filter_ex_date_and_build_statement(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    def fake_call(function_name: str, **kwargs: Any) -> pd.DataFrame:
        assert function_name == "stock_fhps_em"
        if kwargs["date"] == "20251231":
            return pd.DataFrame(
                [
                    {
                        "代码": "600519",
                        "名称": "贵州茅台",
                        "送转股份-送转总比例": 0,
                        "现金分红-现金分红比例": 8.0,
                        "股权登记日": date(2026, 6, 12),
                        "除权除息日": date(2026, 6, 15),
                    },
                    {
                        # Different ex-date: filtered out.
                        "代码": "000001",
                        "名称": "平安银行",
                        "送转股份-送转总比例": 0,
                        "现金分红-现金分红比例": 2.0,
                        "股权登记日": date(2026, 6, 10),
                        "除权除息日": date(2026, 6, 11),
                    },
                    {
                        # No dividend and no split: not an ex-event.
                        "代码": "601111",
                        "名称": "中国国航",
                        "送转股份-送转总比例": 0,
                        "现金分红-现金分红比例": 0,
                        "股权登记日": date(2026, 6, 12),
                        "除权除息日": date(2026, 6, 15),
                    },
                ]
            )
        return _empty()

    monkeypatch.setattr(akshare_upstream, "call", fake_call)
    akshare_news._fhps_cache.clear()

    response = await client.get(
        "/providers/akshare/calendar/dividends",
        params={"date": "2026-06-15"},
    )

    assert response.status_code == 200
    entries = response.json()["entries"]
    assert entries == [
        {
            "instrument_id": "SH.600519",
            "name": "贵州茅台",
            "symbol": "600519",
            "statement": "10派8元(含税)",
            "ex_date": "2026-06-15",
            "record_date": "2026-06-12",
            "payable_date": None,
        }
    ]


@pytest.mark.asyncio
async def test_dividends_statement_includes_split_ratio(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    def fake_call(function_name: str, **kwargs: Any) -> pd.DataFrame:
        if kwargs["date"] == "20260630":
            return pd.DataFrame(
                [
                    {
                        "代码": "300750",
                        "名称": "宁德时代",
                        "送转股份-送转总比例": 2.5,
                        "现金分红-现金分红比例": 4.5,
                        "股权登记日": date(2026, 9, 18),
                        "除权除息日": date(2026, 9, 21),
                    }
                ]
            )
        return _empty()

    monkeypatch.setattr(akshare_upstream, "call", fake_call)

    response = await client.get(
        "/providers/akshare/calendar/dividends",
        params={"date": "2026-09-21"},
    )

    assert response.status_code == 200
    entry = response.json()["entries"][0]
    assert entry["statement"] == "10送转2.5派4.5元(含税)"
    assert entry["instrument_id"] == "SZ.300750"


@pytest.mark.asyncio
async def test_dividends_invalid_date_rejected(client: httpx.AsyncClient) -> None:
    response = await client.get(
        "/providers/akshare/calendar/dividends",
        params={"date": "2026/06/15"},
    )
    assert response.status_code == 400
    assert response.json()["error"]["code"] == "invalid_request"


def _economic_frame() -> pd.DataFrame:
    return pd.DataFrame(
        [
            {
                "日期": date(2026, 8, 17),
                "时间": "09:30",
                "国家": "中国",
                "地区": "中国",
                "事件": "中国7月CPI年率",
                "重要性": 3,
                "前值": 0.3,
                "预期": 0.4,
                "公布": 0.2,
            },
            {
                "日期": date(2026, 8, 17),
                "时间": None,
                "国家": "美国",
                "地区": "美国",
                "事件": "美国7月NFIB小型企业信心指数",
                "重要性": 1,
                "前值": 98.6,
                "预期": None,
                "公布": None,
            },
        ]
    )


@pytest.mark.asyncio
async def test_economic_events_map_fields_and_timestamp(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    calls: list[str] = []

    def fake_call(function_name: str, **kwargs: Any) -> pd.DataFrame:
        assert function_name == "news_economic_baidu"
        calls.append(kwargs["date"])
        return _economic_frame()

    monkeypatch.setattr(akshare_upstream, "call", fake_call)

    response = await client.get(
        "/providers/akshare/calendar/economic",
        params={"begin_date": "2026-08-17", "end_date": "2026-08-17"},
    )

    assert response.status_code == 200
    assert calls == ["20260817"]
    entries = response.json()["entries"]
    assert len(entries) == 2
    first = entries[0]
    assert first["title"] == "中国7月CPI年率"
    assert first["region"] == "中国"
    assert first["event_date"] == "2026-08-17"
    assert first["importance"] == 3
    expected_ts = int(
        datetime(2026, 8, 17, 9, 30, tzinfo=ZoneInfo("Asia/Shanghai")).timestamp()
    )
    assert first["event_timestamp"] == expected_ts
    assert first["previous_value"] == "0.3"
    assert first["forecast_value"] == "0.4"
    assert first["actual_value"] == "0.2"
    assert len(first["event_id"]) == 16
    second = entries[1]
    assert second["event_date"] == "2026-08-17"
    assert second["event_timestamp"] is None
    assert second["forecast_value"] is None
    assert second["actual_value"] is None
    assert second["importance"] == 1


@pytest.mark.asyncio
async def test_economic_range_over_31_days_rejected(
    client: httpx.AsyncClient,
) -> None:
    response = await client.get(
        "/providers/akshare/calendar/economic",
        params={"begin_date": "2026-01-01", "end_date": "2026-03-01"},
    )
    assert response.status_code == 400
    assert response.json()["error"]["code"] == "invalid_request"


def _ipo_frame() -> pd.DataFrame:
    today = datetime.now(timezone.utc).date()
    listed = (today - timedelta(days=5)).isoformat()
    return pd.DataFrame(
        [
            {
                "股票代码": "603777",
                "股票简称": "来伊份",
                "交易所": "上交所",
                "发行总数": 6000.0,
                "发行价格": 11.67,
                "上市日期": None,
                "申购日期": today.isoformat(),
            },
            {
                "股票代码": "301555",
                "股票简称": "已上市新股",
                "交易所": "深交所",
                "发行总数": 4000.0,
                "发行价格": 22.5,
                "上市日期": listed,
                "申购日期": (today - timedelta(days=12)).isoformat(),
            },
            {
                # Beijing listing: outside the JFTrade market model.
                "股票代码": "920001",
                "股票简称": "北交所新股",
                "交易所": "北交所",
                "发行总数": 1000.0,
                "发行价格": 5.0,
                "上市日期": None,
                "申购日期": today.isoformat(),
            },
        ]
    )


@pytest.mark.asyncio
async def test_ipos_status_and_market_mapping(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(
        akshare_upstream,
        "call",
        lambda function_name, **kwargs: _ipo_frame(),
    )

    response = await client.get("/providers/akshare/calendar/ipos")

    assert response.status_code == 200
    entries = response.json()["entries"]
    assert [entry["instrument_id"] for entry in entries] == ["SH.603777", "SZ.301555"]
    pending, listed = entries
    assert pending["status"] == "pending"
    assert pending["listing_date"] is None
    assert pending["issue_volume"] == 6000.0
    assert pending["issue_price"] == 11.67
    assert pending["issue_price_min"] is None
    assert pending["issue_price_max"] is None
    assert listed["status"] == "listed"
    assert listed["listing_date"] is not None


@pytest.mark.asyncio
async def test_ipos_empty_frame_returns_empty_entries(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(akshare_upstream, "call", lambda *_a, **_k: _empty())

    response = await client.get("/providers/akshare/calendar/ipos")

    assert response.status_code == 200
    assert response.json()["entries"] == []


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "path",
    [
        "/providers/akshare/calendar/earnings?begin_date=2026-03-01&end_date=2026-04-30",
        "/providers/akshare/calendar/dividends?date=2026-06-15",
        "/providers/akshare/calendar/economic?begin_date=2026-08-17&end_date=2026-08-17",
        "/providers/akshare/calendar/ipos",
        "/providers/akshare/macro/indicators",
        "/providers/akshare/macro/indicator-history?indicator_id=cn_cpi_yoy",
    ],
)
async def test_calendar_and_macro_paths_are_warming_gated(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
    path: str,
) -> None:
    monkeypatch.setattr(
        akshare_upstream,
        "runtime_snapshot",
        lambda: upstream.RuntimeSnapshot("warming"),
    )
    monkeypatch.setattr(
        akshare_upstream,
        "request_runtime_warmup",
        lambda: upstream.RuntimeSnapshot("warming"),
    )

    response = await client.get(path)

    assert response.status_code == 503
    assert response.json()["error"]["code"] == "AKSHARE_RUNTIME_WARMING"
