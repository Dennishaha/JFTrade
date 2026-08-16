"""AKShare index-constituents route behavior."""

from __future__ import annotations

from typing import Any

import httpx
import pandas as pd
import pytest

from marketdata_sidecar import akshare_upstream, upstream


def _empty() -> pd.DataFrame:
    return pd.DataFrame()


def _csindex_cons_frame() -> pd.DataFrame:
    return pd.DataFrame(
        [
            {
                "日期": "2026-07-31",
                "指数代码": "000300",
                "指数名称": "沪深300",
                "成分券代码": "600519",
                "成分券名称": "贵州茅台",
                "交易所": "上海证券交易所",
            },
            {
                "日期": "2026-07-31",
                "指数代码": "000300",
                "指数名称": "沪深300",
                "成分券代码": "300750",
                "成分券名称": "宁德时代",
                "交易所": "深圳证券交易所",
            },
            {
                "日期": "2026-07-31",
                "指数代码": "000300",
                "指数名称": "沪深300",
                "成分券代码": None,
                "成分券名称": None,
                "交易所": None,
            },
        ]
    )


def _sina_cons_frame() -> pd.DataFrame:
    return pd.DataFrame(
        [
            {"品种代码": "600000", "品种名称": "浦发银行", "最新价": "10.0"},
            {"品种代码": "600030", "品种名称": "中信证券", "最新价": "25.0"},
        ]
    )


def _catalog_call(cons_calls: list[tuple[str, str]], cons_frame: pd.DataFrame):
    def fake_call(function_name: str, **kwargs: Any) -> pd.DataFrame:
        if function_name == "stock_sh_a_spot_em":
            return pd.DataFrame([{"代码": "600519", "名称": "贵州茅台", "最新价": "1700"}])
        if function_name == "stock_sz_a_spot_em":
            return _empty()
        if function_name == "fund_etf_spot_em":
            return _empty()
        if function_name == "stock_zh_index_spot_em":
            if kwargs["symbol"] == "上证系列指数":
                return pd.DataFrame([{"代码": "000001", "名称": "上证指数", "最新价": "3300"}])
            if kwargs["symbol"] == "中证系列指数":
                return pd.DataFrame([{"代码": "000300", "名称": "沪深300", "最新价": "3900"}])
            return _empty()
        if function_name in {"index_stock_cons_csindex", "index_stock_cons"}:
            cons_calls.append((function_name, kwargs["symbol"]))
            return cons_frame
        raise AssertionError(f"unexpected AKShare call: {function_name} {kwargs}")

    return fake_call


@pytest.mark.asyncio
async def test_csi_index_constituents_come_from_csindex(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    cons_calls: list[tuple[str, str]] = []
    monkeypatch.setattr(
        akshare_upstream,
        "call",
        _catalog_call(cons_calls, _csindex_cons_frame()),
    )

    response = await client.get("/providers/akshare/index-constituents/SH/000300")

    assert response.status_code == 200
    body = response.json()
    assert body["market"] == "SH"
    assert body["symbol"] == "000300"
    assert body["instrument_id"] == "SH.000300"
    assert body["source"] == "akshare-index-constituents"
    assert body["constituents"] == [
        {"code": "600519", "name": "贵州茅台", "weight": None},
        {"code": "300750", "name": "宁德时代", "weight": None},
    ]
    assert cons_calls == [("index_stock_cons_csindex", "000300")]


@pytest.mark.asyncio
async def test_exchange_index_constituents_come_from_sina(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    cons_calls: list[tuple[str, str]] = []
    monkeypatch.setattr(
        akshare_upstream,
        "call",
        _catalog_call(cons_calls, _sina_cons_frame()),
    )

    response = await client.get("/providers/akshare/index-constituents/SH/000001")

    assert response.status_code == 200
    body = response.json()
    assert body["instrument_id"] == "SH.000001"
    assert body["constituents"] == [
        {"code": "600000", "name": "浦发银行", "weight": None},
        {"code": "600030", "name": "中信证券", "weight": None},
    ]
    assert cons_calls == [("index_stock_cons", "000001")]


@pytest.mark.asyncio
async def test_constituent_weight_is_null_when_upstream_has_no_weight(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    frame = pd.DataFrame(
        [
            {"成分券代码": "600519", "成分券名称": "贵州茅台", "权重": 4.85},
            {"成分券代码": "300750", "成分券名称": "宁德时代"},
        ]
    )
    monkeypatch.setattr(akshare_upstream, "call", _catalog_call([], frame))

    response = await client.get("/providers/akshare/index-constituents/SH/000300")

    assert response.status_code == 200
    assert response.json()["constituents"] == [
        {"code": "600519", "name": "贵州茅台", "weight": 4.85},
        {"code": "300750", "name": "宁德时代", "weight": None},
    ]


@pytest.mark.asyncio
async def test_constituents_limit_clamps_entries_after_fetch(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(
        akshare_upstream,
        "call",
        _catalog_call([], _csindex_cons_frame()),
    )

    limited = await client.get(
        "/providers/akshare/index-constituents/SH/000300",
        params={"limit": 1},
    )
    too_small = await client.get(
        "/providers/akshare/index-constituents/SH/000300",
        params={"limit": 0},
    )
    too_large = await client.get(
        "/providers/akshare/index-constituents/SH/000300",
        params={"limit": 1001},
    )

    assert limited.status_code == 200
    assert len(limited.json()["constituents"]) == 1
    assert too_small.status_code == 400
    assert too_small.json()["error"]["code"] == "invalid_request"
    assert too_large.status_code == 400


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "path",
    [
        # A CN equity is not an index.
        "/providers/akshare/index-constituents/SH/600519",
        # Unknown CN code.
        "/providers/akshare/index-constituents/SZ/399999",
        # AKShare exposes no HK/US index constituents endpoint.
        "/providers/akshare/index-constituents/HK/800000",
        "/providers/akshare/index-constituents/US/.SPX",
        # Unsupported market token.
        "/providers/akshare/index-constituents/XX/000300",
    ],
)
async def test_non_index_and_unsupported_markets_are_rejected(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
    path: str,
) -> None:
    monkeypatch.setattr(akshare_upstream, "call", _catalog_call([], _empty()))

    response = await client.get(path)

    assert response.status_code == 400
    assert response.json()["error"]["code"] == "AKSHARE_UNSUPPORTED"


@pytest.mark.asyncio
async def test_index_constituents_path_is_warming_gated(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
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

    response = await client.get("/providers/akshare/index-constituents/SH/000300")

    assert response.status_code == 503
    assert response.json()["error"]["code"] == "AKSHARE_RUNTIME_WARMING"


@pytest.mark.asyncio
async def test_constituents_cache_hit_avoids_second_upstream_call(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    cons_calls: list[tuple[str, str]] = []
    monkeypatch.setattr(
        akshare_upstream,
        "call",
        _catalog_call(cons_calls, _csindex_cons_frame()),
    )

    first = await client.get("/providers/akshare/index-constituents/SH/000300")
    second = await client.get(
        "/providers/akshare/index-constituents/SH/000300",
        params={"limit": 1},
    )

    assert first.status_code == 200
    assert second.status_code == 200
    assert len(first.json()["constituents"]) == 2
    assert len(second.json()["constituents"]) == 1
    assert cons_calls == [("index_stock_cons_csindex", "000300")]
