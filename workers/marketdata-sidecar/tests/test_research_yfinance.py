"""yfinance research routes: profile, financials, analyst, ownership."""

from __future__ import annotations

from typing import Any

import httpx
import pytest

from marketdata_sidecar import upstream


def _info() -> dict[str, Any]:
    return {
        "longName": "Apple Inc.",
        "sector": "Technology",
        "industry": "Consumer Electronics",
        "country": "United States",
        "city": "Cupertino",
        "website": "https://www.apple.com",
        "fullTimeEmployees": 164000,
        "currency": "USD",
        "longBusinessSummary": "Apple designs smartphones.",
    }


@pytest.mark.asyncio
async def test_profile_groups_and_currency(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(upstream, "ticker_info", lambda *_a, **_k: _info())

    response = await client.get("/providers/yfinance/profile/US/AAPL")

    assert response.status_code == 200
    body = response.json()
    assert body["instrument_id"] == "US.AAPL"
    assert body["market"] == "US"
    assert body["symbol"] == "AAPL"
    assert body["currency"] == "USD"
    assert [group["title"] for group in body["groups"]] == ["基本资料", "公司简介"]
    basic = body["groups"][0]["fields"]
    assert {"name": "公司名称", "value": "Apple Inc."} in basic
    assert {"name": "员工人数", "value": "164000"} in basic
    assert body["groups"][1]["fields"][0]["value"] == "Apple designs smartphones."


@pytest.mark.asyncio
async def test_profile_hk_supported_when_info_present(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(
        upstream,
        "ticker_info",
        lambda *_a, **_k: {"longName": "Tencent", "currency": "HKD"},
    )

    response = await client.get("/providers/yfinance/profile/HK/00700")

    assert response.status_code == 200
    body = response.json()
    assert body["instrument_id"] == "HK.00700"
    assert body["currency"] == "HKD"


@pytest.mark.asyncio
async def test_profile_empty_info_is_not_found(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(upstream, "ticker_info", lambda *_a, **_k: {})

    response = await client.get("/providers/yfinance/profile/US/NOPE")

    assert response.status_code == 404
    assert response.json()["error"]["code"] == "not_found"


@pytest.mark.asyncio
async def test_yfinance_research_rejects_cn_markets(
    client: httpx.AsyncClient,
) -> None:
    for path in (
        "/providers/yfinance/profile/SH/600519",
        "/providers/yfinance/financials/SZ/000001",
        "/providers/yfinance/analyst/CN/SH.600519",
        "/providers/yfinance/ownership/SH/600519",
    ):
        response = await client.get(path)
        assert response.status_code == 400
        assert response.json()["error"]["code"] == "unsupported_market"


def _income_data() -> dict[str, Any]:
    return {
        "periods": ["2025-09-27", "2024-09-28", "2023-09-30"],
        "rows": {
            "Total Revenue": [400.0, 380.0, 360.0],
            "Gross Profit": [180.0, 170.0, 160.0],
            "Operating Income": [120.0, 110.0, 100.0],
            "Net Income": [100.0, 90.0, 80.0],
            # No "Basic EPS" row: the key must be omitted per period.
        },
    }


@pytest.mark.asyncio
async def test_financials_periods_yoy_and_field_omission(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(
        upstream, "ticker_financials", lambda _s, _st: _income_data()
    )
    monkeypatch.setattr(upstream, "ticker_info", lambda *_a, **_k: _info())

    response = await client.get(
        "/providers/yfinance/financials/US/AAPL",
        params={"statement": "income"},
    )

    assert response.status_code == 200
    body = response.json()
    assert body["instrument_id"] == "US.AAPL"
    assert body["statement"] == "income"
    assert body["currency"] == "USD"
    assert [f["field_id"] for f in body["fields"]] == [
        "total_revenue",
        "gross_profit",
        "operating_income",
        "net_income",
        "basic_eps",
    ]
    assert body["fields"][0]["display_name"] == "营业总收入"
    periods = body["periods"]
    assert [p["period_text"] for p in periods] == ["2025年报", "2024年报", "2023年报"]
    latest = periods[0]["values"]
    assert latest["total_revenue"]["data"] == 400.0
    assert latest["total_revenue"]["yoy"] == pytest.approx((400.0 / 380.0 - 1) * 100)
    assert latest["total_revenue"]["qoq"] is None
    assert "basic_eps" not in latest
    # The oldest period has no prior year to compare against.
    assert periods[-1]["values"]["total_revenue"]["yoy"] is None


@pytest.mark.asyncio
async def test_financials_empty_frame_is_not_found(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(
        upstream,
        "ticker_financials",
        lambda _s, _st: {"periods": [], "rows": {}},
    )

    response = await client.get("/providers/yfinance/financials/US/NOPE")

    assert response.status_code == 404
    assert response.json()["error"]["code"] == "not_found"


@pytest.mark.asyncio
async def test_financials_invalid_statement_rejected(
    client: httpx.AsyncClient,
) -> None:
    response = await client.get(
        "/providers/yfinance/financials/US/AAPL",
        params={"statement": "quarterly"},
    )

    assert response.status_code == 400
    assert response.json()["error"]["code"] == "unsupported_statement"


def _analyst_data() -> dict[str, Any]:
    return {
        "trend": [
            {
                "period": "0m",
                "strongBuy": 8,
                "buy": 10,
                "hold": 4,
                "sell": 0,
                "strongSell": 0,
            },
            {"period": "-1m", "strongBuy": 7, "buy": 10, "hold": 5},
        ],
        "targets": {"low": 150.0, "mean": 200.0, "high": 250.0, "current": 180.0},
    }


@pytest.mark.asyncio
async def test_analyst_rating_distribution_and_targets(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(upstream, "ticker_analyst", lambda _s: _analyst_data())

    response = await client.get("/providers/yfinance/analyst/US/AAPL")

    assert response.status_code == 200
    body = response.json()
    assert body["instrument_id"] == "US.AAPL"
    assert body["rating"] == pytest.approx((8 * 5 + 10 * 4 + 4 * 3) / 22)
    assert body["analyst_count"] == 22
    assert body["target_price"] == {"lowest": 150.0, "average": 200.0, "highest": 250.0}
    distribution = body["distribution"]
    assert distribution["strong_buy"] == pytest.approx(8 / 22 * 100)
    assert distribution["buy"] == pytest.approx(10 / 22 * 100)
    assert distribution["hold"] == pytest.approx(4 / 22 * 100)
    assert distribution["underperform"] == 0.0
    assert distribution["sell"] == 0.0
    assert body["update_time"] is None


@pytest.mark.asyncio
async def test_analyst_empty_data_is_not_found(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(
        upstream, "ticker_analyst", lambda _s: {"trend": [], "targets": {}}
    )

    response = await client.get("/providers/yfinance/analyst/US/NOPE")

    assert response.status_code == 404


def _ownership_data() -> dict[str, list[dict[str, Any]]]:
    return {
        "major": [
            {"label": "insidersPercentHeld", "Value": 0.072},
            {"label": "institutionsPercentHeld", "Value": 0.61},
        ],
        "institutional": [
            {
                "label": "0",
                "Holder": "Vanguard Group",
                "pctHeld": 0.086,
                "Date Reported": "2025-06-30",
                "Shares": 1300000000.0,
            }
        ],
        "mutualfund": [],
    }


@pytest.mark.asyncio
async def test_ownership_groups_emit_percentages(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(upstream, "ticker_ownership", lambda _s: _ownership_data())

    response = await client.get("/providers/yfinance/ownership/US/AAPL")

    assert response.status_code == 200
    groups = response.json()["groups"]
    assert [group["kind"] for group in groups] == [
        "major_holders",
        "institutional_holders",
    ]
    major = groups[0]
    assert major["static_date"] is None
    assert {"name": "insidersPercentHeld", "holder_pct": pytest.approx(7.2)} in [
        {"name": item["name"], "holder_pct": item["holder_pct"]}
        for item in major["items"]
    ]
    institutional = groups[1]
    assert institutional["static_date"] == "2025-06-30"
    assert institutional["items"][0]["holder_pct"] == pytest.approx(8.6)


@pytest.mark.asyncio
async def test_ownership_empty_is_not_found(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(
        upstream,
        "ticker_ownership",
        lambda _s: {"major": [], "institutional": [], "mutualfund": []},
    )

    response = await client.get("/providers/yfinance/ownership/US/NOPE")

    assert response.status_code == 404


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "path",
    [
        "/profile/US/AAPL",
        "/financials/US/AAPL",
        "/analyst/US/AAPL",
        "/ownership/US/AAPL",
        "/providers/yfinance/profile/US/AAPL",
    ],
)
async def test_yfinance_research_paths_are_warming_gated(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
    path: str,
) -> None:
    monkeypatch.setattr(
        upstream,
        "runtime_snapshot",
        lambda: upstream.RuntimeSnapshot("warming"),
    )
    monkeypatch.setattr(
        upstream,
        "request_runtime_warmup",
        lambda: upstream.RuntimeSnapshot("warming"),
    )

    response = await client.get(path)

    assert response.status_code == 503
    assert response.json()["error"]["code"] == "YFINANCE_RUNTIME_WARMING"
