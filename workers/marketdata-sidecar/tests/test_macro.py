"""akshare macro indicator catalog and history routes."""

from __future__ import annotations

from datetime import date
from typing import Any

import httpx
import pandas as pd
import pytest

from marketdata_sidecar import akshare_upstream
from marketdata_sidecar.akshare_macro_catalog import INDICATORS


@pytest.mark.asyncio
async def test_indicators_catalog_groups_and_ids(client: httpx.AsyncClient) -> None:
    response = await client.get("/providers/akshare/macro/indicators")

    assert response.status_code == 200
    categories = response.json()["categories"]
    names = [category["category_name"] for category in categories]
    assert names == [
        "中国·物价",
        "中国·景气",
        "中国·经济总量",
        "中国·货币信贷",
        "美国·物价",
        "美国·就业",
        "美国·消费与景气",
    ]
    indicators = [
        indicator
        for category in categories
        for indicator in category["indicators"]
    ]
    ids = [indicator["indicator_id"] for indicator in indicators]
    assert len(ids) == len(set(ids)) == 16
    cpi = next(item for item in indicators if item["indicator_id"] == "cn_cpi_yoy")
    assert cpi == {
        "indicator_id": "cn_cpi_yoy",
        "name": "CPI同比",
        "region": "中国",
        "unit": "%",
        "unit_type": 1,
        "frequency": "monthly",
    }
    pmi = next(item for item in indicators if item["indicator_id"] == "cn_pmi")
    assert pmi["unit_type"] == 3


def test_catalog_entries_point_at_real_akshare_functions() -> None:
    import akshare as ak

    for spec in INDICATORS:
        assert callable(getattr(ak, spec.function, None)), spec.function


def _cpi_frame() -> pd.DataFrame:
    return pd.DataFrame(
        [
            {"商品": "中国CPI年率报告", "日期": date(2026, 7, 9), "今值": 0.3, "预测值": 0.4, "前值": 0.5},
            {"商品": "中国CPI年率报告", "日期": date(2026, 6, 9), "今值": 0.5, "预测值": None, "前值": 0.1},
        ]
    )


@pytest.mark.asyncio
async def test_indicator_history_maps_jin10_columns(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    calls: list[str] = []

    def fake_call(function_name: str, **_kwargs: Any) -> pd.DataFrame:
        calls.append(function_name)
        return _cpi_frame()

    monkeypatch.setattr(akshare_upstream, "call", fake_call)

    response = await client.get(
        "/providers/akshare/macro/indicator-history",
        params={"indicator_id": "cn_cpi_yoy"},
    )

    assert response.status_code == 200
    assert calls == ["macro_china_cpi_yearly"]
    body = response.json()
    assert body["indicator_id"] == "cn_cpi_yoy"
    # Rows are emitted newest first.
    assert body["entries"] == [
        {
            "data_time": "2026-07",
            "value": 0.3,
            "predict_value": 0.4,
            "previous_value": 0.5,
            "unit": "%",
            "unit_type": 1,
        },
        {
            "data_time": "2026-06",
            "value": 0.5,
            "predict_value": None,
            "previous_value": 0.1,
            "unit": "%",
            "unit_type": 1,
        },
    ]


@pytest.mark.asyncio
async def test_indicator_history_limit_and_cache(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    calls: list[str] = []
    monkeypatch.setattr(
        akshare_upstream,
        "call",
        lambda function_name, **_k: calls.append(function_name) or _cpi_frame(),
    )

    limited = await client.get(
        "/providers/akshare/macro/indicator-history",
        params={"indicator_id": "cn_cpi_yoy", "limit": 1},
    )
    again = await client.get(
        "/providers/akshare/macro/indicator-history",
        params={"indicator_id": "cn_cpi_yoy"},
    )

    assert limited.status_code == 200
    assert len(limited.json()["entries"]) == 1
    assert again.status_code == 200
    assert len(again.json()["entries"]) == 2
    assert calls == ["macro_china_cpi_yearly"]


@pytest.mark.asyncio
async def test_indicator_history_lpr_columns(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    def fake_call(function_name: str, **_kwargs: Any) -> pd.DataFrame:
        assert function_name == "macro_china_lpr"
        return pd.DataFrame(
            [
                {"TRADE_DATE": date(2026, 7, 20), "LPR1Y": 3.0, "LPR5Y": 3.5},
                {"TRADE_DATE": date(2026, 6, 22), "LPR1Y": 3.1, "LPR5Y": 3.6},
            ]
        )

    monkeypatch.setattr(akshare_upstream, "call", fake_call)

    response = await client.get(
        "/providers/akshare/macro/indicator-history",
        params={"indicator_id": "cn_lpr_1y"},
    )

    assert response.status_code == 200
    entries = response.json()["entries"]
    assert entries[0]["data_time"] == "2026-07"
    assert entries[0]["value"] == 3.0
    assert entries[0]["predict_value"] is None
    assert entries[0]["previous_value"] is None
    assert entries[0]["unit"] == "%"


@pytest.mark.asyncio
async def test_indicator_history_new_credit_month_text(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    def fake_call(function_name: str, **_kwargs: Any) -> pd.DataFrame:
        assert function_name == "macro_china_new_financial_credit"
        return pd.DataFrame([{"月份": "2026年07月份", "当月": 9500.0}])

    monkeypatch.setattr(akshare_upstream, "call", fake_call)

    response = await client.get(
        "/providers/akshare/macro/indicator-history",
        params={"indicator_id": "cn_new_credit"},
    )

    assert response.status_code == 200
    entry = response.json()["entries"][0]
    assert entry["data_time"] == "2026-07"
    assert entry["value"] == 9500.0
    assert entry["unit"] == "亿元"
    assert entry["unit_type"] == 3


@pytest.mark.asyncio
async def test_indicator_history_unknown_id_is_not_found(
    client: httpx.AsyncClient,
) -> None:
    response = await client.get(
        "/providers/akshare/macro/indicator-history",
        params={"indicator_id": "cn_nonexistent"},
    )

    assert response.status_code == 404
    assert response.json()["error"]["code"] == "not_found"
