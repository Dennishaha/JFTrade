"""AKShare spot fundamentals and level-1 bid/ask projections."""

from __future__ import annotations

from typing import Any

import httpx
import pandas as pd
import pytest

from marketdata_sidecar import akshare_upstream


def _spot_row(**overrides: Any) -> dict[str, Any]:
    row = {
        "market_id": 1,
        "instrument_kind": 2,
        "代码": "600519",
        "名称": "贵州茅台",
        "最新价": 1425.5,
        "涨跌幅": 1.46,
        "涨跌额": 20.5,
        "成交量": 12.5,
        "成交额": 17819.25,
        "最高": 1430.0,
        "最低": 1400.0,
        "今开": 1410.0,
        "昨收": 1405.0,
        "市盈率": 21.5,
        "市净率": 7.8,
        "总市值": 1_790_000_000_000.0,
        "流通市值": 1_790_000_000_000.0,
        "买一": 1424.9,
        "卖一": 1425.6,
    }
    row.update(overrides)
    return row


def _cn_catalog_frame() -> pd.DataFrame:
    return pd.DataFrame(
        [
            {
                "代码": "600519",
                "名称": "贵州茅台",
                "最新价": "1425.50",
                "今开": "1410.00",
                "最高": "1430.00",
                "最低": "1400.00",
                "昨收": "1405.00",
                "成交量": "12.5",
                "成交额": "17819.25",
                "市盈率-动态": "21.5",
                "市净率": "7.8",
                "总市值": "1790000000000",
            }
        ]
    )


def _catalog_call(function_name: str, **kwargs: Any) -> pd.DataFrame:
    if function_name == "stock_sh_a_spot_em":
        return _cn_catalog_frame()
    if function_name == "stock_sz_a_spot_em":
        return pd.DataFrame()
    if function_name == "fund_etf_spot_em":
        return pd.DataFrame()
    if function_name == "stock_zh_index_spot_em":
        return pd.DataFrame()
    raise AssertionError(f"unexpected AKShare call: {function_name} {kwargs}")


def test_spot_row_normalization_exposes_fundamental_and_book_fields() -> None:
    raw = {
        "f1": 2,
        "f2": 1425.5,
        "f9": 21.5,
        "f12": "600519",
        "f13": 1,
        "f14": "贵州茅台",
        "f18": 1405.0,
        "f20": 1_790_000_000_000.0,
        "f21": 1_780_000_000_000.0,
        "f23": 7.8,
        "f31": 1424.9,
        "f32": 1425.6,
    }

    row = akshare_upstream._normalize_spot_row(raw)

    assert row["市盈率"] == 21.5
    assert row["市净率"] == 7.8
    assert row["总市值"] == 1_790_000_000_000.0
    assert row["流通市值"] == 1_780_000_000_000.0
    assert row["买一"] == 1424.9
    assert row["卖一"] == 1425.6


@pytest.mark.asyncio
async def test_akshare_security_projects_spot_fundamentals(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    enrichment_calls: list[str] = []

    def fake_call(function_name: str, **kwargs: Any) -> pd.DataFrame:
        if function_name == "stock_individual_info_em":
            enrichment_calls.append(kwargs["symbol"])
            return pd.DataFrame(
                {
                    "item": ["行业", "总股本", "上市时间"],
                    "value": ["白酒", 1_256_197_800.0, "2001-08-27"],
                }
            )
        return _catalog_call(function_name, **kwargs)

    monkeypatch.setattr(akshare_upstream, "call", fake_call)
    response = await client.get("/providers/akshare/security/SH/600519")

    assert response.status_code == 200
    body = response.json()
    assert body["instrument_id"] == "SH.600519"
    assert body["trailing_pe"] == 21.5
    assert body["price_to_book"] == 7.8
    assert body["market_cap"] == 1_790_000_000_000
    assert body["industry"] == "白酒"
    assert body["shares_outstanding"] == 1_256_197_800
    assert enrichment_calls == ["600519"]


@pytest.mark.asyncio
async def test_akshare_security_enrichment_failure_degrades_to_spot_only(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    def fake_call(function_name: str, **kwargs: Any) -> pd.DataFrame:
        if function_name == "stock_individual_info_em":
            raise RuntimeError("private enrichment failure")
        return _catalog_call(function_name, **kwargs)

    monkeypatch.setattr(akshare_upstream, "call", fake_call)
    response = await client.get("/providers/akshare/security/SH/600519")

    assert response.status_code == 200
    body = response.json()
    assert body["industry"] is None
    assert body["shares_outstanding"] is None
    assert body["trailing_pe"] == 21.5
    assert body["price_to_book"] == 7.8
    assert "private enrichment failure" not in response.text


@pytest.mark.asyncio
async def test_akshare_etf_security_skips_cn_stock_enrichment(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    def fake_call(function_name: str, **kwargs: Any) -> pd.DataFrame:
        if function_name == "stock_individual_info_em":
            raise AssertionError("ETFs must not trigger the stock info endpoint")
        if function_name == "fund_etf_spot_em":
            return pd.DataFrame(
                [
                    {
                        "代码": "510300",
                        "名称": "沪深300ETF",
                        "最新价": "4.123",
                        "总市值": "90000000000",
                    }
                ]
            )
        if function_name in {"stock_sh_a_spot_em", "stock_zh_index_spot_em"}:
            return pd.DataFrame()
        raise AssertionError(function_name)

    monkeypatch.setattr(akshare_upstream, "call", fake_call)
    response = await client.get("/providers/akshare/security/SH/510300")

    assert response.status_code == 200
    body = response.json()
    assert body["security_type"] == "ETF"
    assert body["market_cap"] == 90_000_000_000
    assert body["industry"] is None
    assert body["trailing_pe"] is None


@pytest.mark.asyncio
async def test_akshare_live_spot_snapshot_populates_bid_ask_and_fundamentals(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(
        akshare_upstream,
        "spot_rows",
        lambda market, symbols: [_spot_row()] if market == "SH" else [],
    )

    def fake_call(function_name: str, **kwargs: Any) -> pd.DataFrame:
        if function_name == "stock_individual_info_em":
            return pd.DataFrame({"item": ["行业"], "value": ["白酒"]})
        return _catalog_call(function_name, **kwargs)

    monkeypatch.setattr(akshare_upstream, "call", fake_call)

    snapshot = await client.get("/providers/akshare/snapshot/SH/600519")
    security = await client.get("/providers/akshare/security/SH/600519")

    assert snapshot.status_code == 200
    body = snapshot.json()
    assert body["bid"] == "1424.9"
    assert body["ask"] == "1425.6"
    assert body["price"] == "1425.5"
    assert security.status_code == 200
    assert security.json()["trailing_pe"] == 21.5
    assert security.json()["price_to_book"] == 7.8
    assert security.json()["market_cap"] == 1_790_000_000_000


@pytest.mark.asyncio
async def test_akshare_live_spot_without_book_fields_keeps_bid_ask_null(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(
        akshare_upstream,
        "spot_rows",
        lambda _market, _symbols: [_spot_row(买一="-", 卖一="-", 市盈率="-")],
    )
    monkeypatch.setattr(akshare_upstream, "call", _catalog_call)

    snapshot = await client.get("/providers/akshare/snapshot/SH/600519")
    security = await client.get("/providers/akshare/security/SH/600519")

    assert snapshot.status_code == 200
    assert snapshot.json()["bid"] is None
    assert snapshot.json()["ask"] is None
    assert security.status_code == 200
    assert security.json()["trailing_pe"] is None
