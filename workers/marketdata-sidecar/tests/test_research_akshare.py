"""akshare research routes: profile, financials, analyst, ownership."""

from __future__ import annotations

from datetime import datetime, timedelta, timezone
from typing import Any

import httpx
import pandas as pd
import pytest

from marketdata_sidecar import akshare_ownership, akshare_upstream, upstream


def _empty() -> pd.DataFrame:
    return pd.DataFrame()


@pytest.mark.asyncio
async def test_profile_cn_basic_fields(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    def fake_call(function_name: str, **kwargs: Any) -> pd.DataFrame:
        assert function_name == "stock_individual_info_em"
        assert kwargs["symbol"] == "600519"
        return pd.DataFrame(
            [
                {"item": "股票简称", "value": "贵州茅台"},
                {"item": "总市值", "value": 2.1e12},
                {"item": "行业", "value": "酿酒行业"},
                {"item": "上市时间", "value": "20010827"},
            ]
        )

    monkeypatch.setattr(akshare_upstream, "call", fake_call)

    response = await client.get("/providers/akshare/profile/CN/600519")

    assert response.status_code == 200
    body = response.json()
    assert body["instrument_id"] == "CN.600519"
    assert body["market"] == "CN"
    assert body["symbol"] == "600519"
    assert body["currency"] == "CNY"
    assert len(body["groups"]) == 1
    fields = body["groups"][0]["fields"]
    assert fields[0] == {"name": "股票简称", "value": "贵州茅台"}
    assert {"name": "行业", "value": "酿酒行业"} in fields


@pytest.mark.asyncio
async def test_profile_hk_company_profile(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    def fake_call(function_name: str, **kwargs: Any) -> pd.DataFrame:
        assert function_name == "stock_hk_company_profile_em"
        assert kwargs["symbol"] == "00700"
        return pd.DataFrame(
            [{"公司名称": "腾讯控股", "所属行业": "软件服务", "公司成立日期": "1998-11-11"}]
        )

    monkeypatch.setattr(akshare_upstream, "call", fake_call)

    response = await client.get("/providers/akshare/profile/HK/700")

    assert response.status_code == 200
    body = response.json()
    assert body["instrument_id"] == "HK.00700"
    assert body["currency"] == "HKD"
    assert {"name": "公司名称", "value": "腾讯控股"} in body["groups"][0]["fields"]


@pytest.mark.asyncio
async def test_profile_unsupported_market_and_empty(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(
        akshare_upstream, "call", lambda *_a, **_k: _empty()
    )

    us = await client.get("/providers/akshare/profile/US/AAPL")
    empty = await client.get("/providers/akshare/profile/SH/600519")

    assert us.status_code == 400
    assert us.json()["error"]["code"] == "unsupported_market"
    assert empty.status_code == 404
    assert empty.json()["error"]["code"] == "not_found"


def _income_frame() -> pd.DataFrame:
    return pd.DataFrame(
        [
            {
                "REPORT_DATE": "2024-12-31",
                "TOTAL_OPERATE_INCOME": 1.7e11,
                "OPERATE_COST": 3.0e10,
                "PARENT_NETPROFIT": 8.6e10,
            },
            {
                "REPORT_DATE": "2023-12-31",
                "TOTAL_OPERATE_INCOME": 1.5e11,
                "OPERATE_COST": 2.8e10,
                "PARENT_NETPROFIT": 7.4e10,
            },
            {
                "REPORT_DATE": "2022-12-31",
                "TOTAL_OPERATE_INCOME": 1.24e11,
                "PARENT_NETPROFIT": 6.2e10,
            },
            {"REPORT_DATE": "2021-12-31", "TOTAL_OPERATE_INCOME": 1.06e11},
            {"REPORT_DATE": "2020-12-31", "TOTAL_OPERATE_INCOME": 9.5e10},
        ]
    )


@pytest.mark.asyncio
async def test_financials_income_periods_and_yoy(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    calls: list[tuple[str, str]] = []

    def fake_call(function_name: str, **kwargs: Any) -> pd.DataFrame:
        calls.append((function_name, kwargs["symbol"]))
        return _income_frame()

    monkeypatch.setattr(akshare_upstream, "call", fake_call)

    response = await client.get(
        "/providers/akshare/financials/CN/600519",
        params={"statement": "income"},
    )

    assert response.status_code == 200
    assert calls == [("stock_profit_sheet_by_yearly_em", "SH600519")]
    body = response.json()
    assert body["instrument_id"] == "CN.600519"
    assert body["statement"] == "income"
    assert body["currency"] == "CNY"
    assert [f["field_id"] for f in body["fields"]] == [
        "total_revenue",
        "operating_cost",
        "operating_profit",
        "total_profit",
        "net_profit",
        "net_profit_attributable",
        "basic_eps",
    ]
    periods = body["periods"]
    # Only the newest four periods are emitted.
    assert [p["period_text"] for p in periods] == [
        "2024年报",
        "2023年报",
        "2022年报",
        "2021年报",
    ]
    latest = periods[0]["values"]
    assert latest["total_revenue"]["data"] == 1.7e11
    assert latest["total_revenue"]["yoy"] == pytest.approx((1.7e11 / 1.5e11 - 1) * 100)
    assert latest["total_revenue"]["qoq"] is None
    assert latest["net_profit_attributable"]["data"] == 8.6e10
    # Fields absent from the frame row are omitted from that period.
    assert "operating_profit" not in latest
    assert "operating_cost" not in periods[3]["values"]


@pytest.mark.asyncio
@pytest.mark.parametrize(
    ("statement", "function_name"),
    [
        ("balance", "stock_balance_sheet_by_yearly_em"),
        ("cashflow", "stock_cash_flow_sheet_by_yearly_em"),
    ],
)
async def test_financials_statement_function_mapping(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
    statement: str,
    function_name: str,
) -> None:
    calls: list[tuple[str, str]] = []

    def fake_call(name: str, **kwargs: Any) -> pd.DataFrame:
        calls.append((name, kwargs["symbol"]))
        return pd.DataFrame([{"REPORT_DATE": "2024-12-31", "TOTAL_ASSETS": 1.0e12}])

    monkeypatch.setattr(akshare_upstream, "call", fake_call)

    response = await client.get(
        "/providers/akshare/financials/SZ/000001",
        params={"statement": statement},
    )

    assert response.status_code == 200
    assert calls == [(function_name, "SZ000001")]
    assert response.json()["statement"] == statement


@pytest.mark.asyncio
async def test_financials_rejections(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(akshare_upstream, "call", lambda *_a, **_k: _empty())

    hk = await client.get("/providers/akshare/financials/HK/00700")
    bad_statement = await client.get(
        "/providers/akshare/financials/SH/600519",
        params={"statement": "quarterly"},
    )
    empty = await client.get("/providers/akshare/financials/SH/600519")

    assert hk.status_code == 400
    assert hk.json()["error"]["code"] == "unsupported_market"
    assert bad_statement.status_code == 400
    assert bad_statement.json()["error"]["code"] == "unsupported_statement"
    assert empty.status_code == 404
    assert empty.json()["error"]["code"] == "not_found"


def _report_frame(days_ago: list[int]) -> pd.DataFrame:
    today = datetime.now(timezone.utc).date()
    rows = []
    ratings = ["买入", "增持", "中性", "买入"]
    orgs = ["中信证券", "华泰证券", "中信建投", "中信证券"]
    for index, days in enumerate(days_ago):
        rows.append(
            {
                "日期": (today - timedelta(days=days)).isoformat(),
                "东财评级": ratings[index % len(ratings)],
                "机构": orgs[index % len(orgs)],
            }
        )
    return pd.DataFrame(rows)


@pytest.mark.asyncio
async def test_analyst_cn_rating_aggregation(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    def fake_call(function_name: str, **kwargs: Any) -> pd.DataFrame:
        assert function_name == "stock_research_report_em"
        assert kwargs["symbol"] == "600519"
        return _report_frame([5, 20, 40, 300])  # one report outside the 180d window

    monkeypatch.setattr(akshare_upstream, "call", fake_call)

    response = await client.get("/providers/akshare/analyst/CN/600519")

    assert response.status_code == 200
    body = response.json()
    assert body["instrument_id"] == "CN.600519"
    # In-window ratings: 买入(5), 增持(4), 中性(3).
    assert body["rating"] == pytest.approx((5 + 4 + 3) / 3)
    assert body["analyst_count"] == 3
    assert body["target_price"] is None
    distribution = body["distribution"]
    assert distribution["strong_buy"] == pytest.approx(100 / 3)
    assert distribution["buy"] == pytest.approx(100 / 3)
    assert distribution["hold"] == pytest.approx(100 / 3)
    assert distribution["underperform"] == 0.0
    assert distribution["sell"] == 0.0
    today = datetime.now(timezone.utc).date()
    assert body["update_time"] == (today - timedelta(days=5)).isoformat()


@pytest.mark.asyncio
async def test_analyst_cn_no_recent_reports_is_not_found(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(
        akshare_upstream,
        "call",
        lambda *_a, **_k: _report_frame([400]),
    )

    response = await client.get("/providers/akshare/analyst/SH/600519")

    assert response.status_code == 404
    assert response.json()["error"]["code"] == "not_found"


@pytest.mark.asyncio
async def test_analyst_rejects_hk_and_us(
    client: httpx.AsyncClient,
) -> None:
    for market, symbol in (("HK", "00700"), ("US", "AAPL")):
        response = await client.get(f"/providers/akshare/analyst/{market}/{symbol}")
        assert response.status_code == 400
        assert response.json()["error"]["code"] == "unsupported_market"


def _holders_frame() -> pd.DataFrame:
    return pd.DataFrame(
        [
            {"名次": 1, "股东名称": "中国贵州茅台酒厂(集团)有限责任公司", "占总股本持股比例": 54.07},
            {"名次": 2, "股东名称": "香港中央结算有限公司", "占总股本持股比例": 7.92},
        ]
    )


@pytest.mark.asyncio
async def test_ownership_probes_latest_report_period(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    candidates = akshare_ownership._candidate_periods()
    calls: list[tuple[str, str]] = []

    def fake_call(function_name: str, **kwargs: Any) -> pd.DataFrame:
        assert function_name == "stock_gdfx_top_10_em"
        assert kwargs["symbol"] == "sh600519"
        calls.append((kwargs["symbol"], kwargs["date"]))
        if kwargs["date"] == candidates[0]:
            return _empty()
        return _holders_frame()

    monkeypatch.setattr(akshare_upstream, "call", fake_call)

    response = await client.get("/providers/akshare/ownership/CN/600519")

    assert response.status_code == 200
    assert calls[0] == ("sh600519", candidates[0])
    assert calls[1] == ("sh600519", candidates[1])
    body = response.json()
    assert body["instrument_id"] == "CN.600519"
    group = body["groups"][0]
    assert group["kind"] == "major_holders"
    date = candidates[1]
    assert group["static_date"] == f"{date[:4]}-{date[4:6]}-{date[6:]}"
    assert group["items"][0] == {
        "name": "中国贵州茅台酒厂(集团)有限责任公司",
        "holder_pct": 54.07,
    }


@pytest.mark.asyncio
async def test_ownership_cache_avoids_reprobing(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    calls: list[str] = []

    def fake_call(function_name: str, **kwargs: Any) -> pd.DataFrame:
        calls.append(kwargs["date"])
        return _holders_frame()

    monkeypatch.setattr(akshare_upstream, "call", fake_call)

    first = await client.get("/providers/akshare/ownership/SH/600519")
    second = await client.get("/providers/akshare/ownership/SH/600519")

    assert first.status_code == 200
    assert second.status_code == 200
    assert len(calls) == 1


@pytest.mark.asyncio
async def test_ownership_empty_periods_is_not_found(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(akshare_upstream, "call", lambda *_a, **_k: _empty())

    response = await client.get("/providers/akshare/ownership/SH/600519")

    assert response.status_code == 404
    assert response.json()["error"]["code"] == "not_found"


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "path",
    [
        "/providers/akshare/profile/CN/600519",
        "/providers/akshare/financials/SH/600519",
        "/providers/akshare/analyst/SZ/000001",
        "/providers/akshare/ownership/CN/600519",
    ],
)
async def test_akshare_research_paths_are_warming_gated(
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
