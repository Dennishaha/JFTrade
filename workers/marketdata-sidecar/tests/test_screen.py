"""Screener route behavior for the yfinance and akshare namespaces."""

from __future__ import annotations

from typing import Any

import httpx
import pandas as pd
import pytest

from marketdata_sidecar import akshare_upstream, upstream


def _screen_payload(**overrides: Any) -> dict[str, Any]:
    payload: dict[str, Any] = {
        "market": "US",
        "conditions": [{"factor_key": "simple.market_cap", "min": 1_000_000_000, "max": None}],
        "sorts": [{"factor_key": "simple.market_cap", "direction": "desc"}],
        "offset": 0,
        "limit": 50,
    }
    payload.update(overrides)
    return payload


def _yahoo_quotes() -> list[dict[str, Any]]:
    return [
        {
            "symbol": "AAPL",
            "shortName": "Apple Inc.",
            "regularMarketPrice": 123.4,
            "regularMarketChangePercent": 1.12,
            "regularMarketVolume": 11_660_806,
            "marketCap": 2.9676e12,
            "trailingPE": 18.99,
            "priceToBook": 2.1,
            "currency": "USD",
        },
        {
            "symbol": "MSFT",
            "shortName": "Microsoft Corporation",
            "regularMarketPrice": 400.0,
            "regularMarketChangePercent": -0.5,
            "regularMarketDayVolume": 20_000_000,
            "marketCap": 3.1e12,
            "trailingPE": 30.0,
            # priceToBook 缺失：values 整键省略
        },
        {
            # 无 symbol 的 quote 被丢弃
            "regularMarketPrice": 1.0,
        },
    ]


def _fake_screen_custom(
    calls: list[tuple[Any, ...]],
    result: dict[str, Any] | None = None,
):
    def fake(
        conditions: list[tuple[str, str, tuple[Any, ...]]],
        sort_field: str | None,
        sort_asc: bool,
        size: int,
    ) -> dict[str, Any]:
        calls.append((conditions, sort_field, sort_asc, size))
        return result if result is not None else {
            "quotes": _yahoo_quotes(),
            "total": 1234,
        }

    return fake


@pytest.mark.asyncio
async def test_yfinance_screen_translates_range_condition_and_sort(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    calls: list[tuple[Any, ...]] = []
    monkeypatch.setattr(upstream, "screen_custom", _fake_screen_custom(calls))

    response = await client.post("/providers/yfinance/screen", json=_screen_payload())

    assert response.status_code == 200
    conditions, sort_field, sort_asc, size = calls[0]
    assert conditions == [
        ("EQ", "region", ("us",)),
        ("GTE", "intradaymarketcap", (1_000_000_000.0,)),
    ]
    assert sort_field == "intradaymarketcap"
    assert sort_asc is False
    assert size == 50

    body = response.json()
    assert body["total"] == 1234
    assert body["has_more"] is True
    assert body["source"] == "yfinance-screen"
    assert body["as_of"]
    entries = body["entries"]
    assert [entry["instrument_id"] for entry in entries] == ["US.AAPL", "US.MSFT"]
    first = entries[0]
    assert first["name"] == "Apple Inc."
    assert first["symbol"] == "AAPL"
    assert first["industry"] is None
    assert first["quote_currency"] == "USD"
    assert first["values"] == {
        "simple.price": 123.4,
        "simple.change_pct": 1.12,
        "simple.volume": 11_660_806,
        "simple.market_cap": 2.9676e12,
        "simple.pe_ttm": 18.99,
        "simple.pb": 2.1,
    }
    second = entries[1]
    # regularMarketDayVolume 兜底 + 缺失因子整键省略
    assert second["values"]["simple.volume"] == 20_000_000
    assert "simple.pb" not in second["values"]


@pytest.mark.asyncio
async def test_yfinance_screen_translates_double_bound_to_btwn(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    calls: list[tuple[Any, ...]] = []
    monkeypatch.setattr(upstream, "screen_custom", _fake_screen_custom(calls))

    response = await client.post(
        "/providers/yfinance/screen",
        json=_screen_payload(
            conditions=[{"factor_key": "simple.pe_ttm", "min": 5, "max": 20}],
            sorts=[{"factor_key": "simple.price", "direction": "asc"}],
        ),
    )

    assert response.status_code == 200
    conditions, sort_field, sort_asc, _size = calls[0]
    assert conditions == [
        ("EQ", "region", ("us",)),
        ("BTWN", "peratio.lasttwelvemonths", (5.0, 20.0)),
    ]
    assert (sort_field, sort_asc) == ("intradayprice", True)


@pytest.mark.asyncio
async def test_yfinance_screen_translates_single_bound_to_gte_lte(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    calls: list[tuple[Any, ...]] = []
    monkeypatch.setattr(upstream, "screen_custom", _fake_screen_custom(calls))

    response = await client.post(
        "/providers/yfinance/screen",
        json=_screen_payload(
            conditions=[
                {"factor_key": "simple.volume", "min": 5000},
                {"factor_key": "simple.pb", "max": 3},
            ],
            sorts=[],
        ),
    )

    assert response.status_code == 200
    conditions, sort_field, sort_asc, _size = calls[0]
    assert conditions == [
        ("EQ", "region", ("us",)),
        ("GTE", "dayvolume", (5000.0,)),
        ("LTE", "pricebookratio.quarterly", (3.0,)),
    ]
    assert sort_field is None
    assert sort_asc is False


@pytest.mark.asyncio
async def test_yfinance_screen_slices_offset_window_locally(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    calls: list[tuple[Any, ...]] = []
    monkeypatch.setattr(upstream, "screen_custom", _fake_screen_custom(calls))

    response = await client.post(
        "/providers/yfinance/screen",
        json=_screen_payload(offset=1, limit=1),
    )

    assert response.status_code == 200
    assert calls[0][3] == 2  # size = offset + limit
    body = response.json()
    assert [entry["instrument_id"] for entry in body["entries"]] == ["US.MSFT"]
    assert body["has_more"] is True  # 1 + 1 < 1234


@pytest.mark.asyncio
async def test_yfinance_screen_falls_back_when_total_absent(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    calls: list[tuple[Any, ...]] = []
    monkeypatch.setattr(
        upstream,
        "screen_custom",
        _fake_screen_custom(calls, {"quotes": _yahoo_quotes()}),
    )

    response = await client.post(
        "/providers/yfinance/screen",
        json=_screen_payload(limit=3),
    )

    assert response.status_code == 200
    body = response.json()
    assert body["total"] == 3  # offset + 已取页大小
    assert body["has_more"] is True  # 取满 size=3 的整页 → 按截断语义视为可能还有更多


@pytest.mark.asyncio
async def test_yfinance_screen_rejects_window_beyond_yahoo_cap(
    client: httpx.AsyncClient,
) -> None:
    response = await client.post(
        "/providers/yfinance/screen",
        json=_screen_payload(offset=210, limit=50),
    )

    assert response.status_code == 400
    assert response.json()["error"]["code"] == "invalid_request"


@pytest.mark.asyncio
async def test_yfinance_screen_rejects_unsupported_market(
    client: httpx.AsyncClient,
) -> None:
    response = await client.post(
        "/providers/yfinance/screen",
        json=_screen_payload(market="HK"),
    )

    assert response.status_code == 400
    assert response.json()["error"]["code"] == "unsupported_market"


@pytest.mark.asyncio
async def test_yfinance_screen_rejects_unknown_factor_and_in_condition(
    client: httpx.AsyncClient,
) -> None:
    unknown = await client.post(
        "/providers/yfinance/screen",
        json=_screen_payload(conditions=[{"factor_key": "simple.roe", "min": 10}]),
    )
    assert unknown.status_code == 400
    assert unknown.json()["error"]["code"] == "unsupported_kind"

    enumeration = await client.post(
        "/providers/yfinance/screen",
        json=_screen_payload(conditions=[{"factor_key": "simple.price", "in": [1, 2]}]),
    )
    assert enumeration.status_code == 400
    assert enumeration.json()["error"]["code"] == "unsupported_kind"

    bad_sort = await client.post(
        "/providers/yfinance/screen",
        json=_screen_payload(sorts=[{"factor_key": "simple.roe", "direction": "desc"}]),
    )
    assert bad_sort.status_code == 400
    assert bad_sort.json()["error"]["code"] == "unsupported_kind"


@pytest.mark.asyncio
async def test_yfinance_screen_rejects_bad_direction_and_boundless_condition(
    client: httpx.AsyncClient,
) -> None:
    bad_direction = await client.post(
        "/providers/yfinance/screen",
        json=_screen_payload(
            sorts=[{"factor_key": "simple.price", "direction": "sideways"}],
        ),
    )
    assert bad_direction.status_code == 400
    assert bad_direction.json()["error"]["code"] == "invalid_request"

    boundless = await client.post(
        "/providers/yfinance/screen",
        json=_screen_payload(conditions=[{"factor_key": "simple.price"}]),
    )
    assert boundless.status_code == 400
    assert boundless.json()["error"]["code"] == "invalid_request"


@pytest.mark.asyncio
async def test_yfinance_screen_empty_result(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(
        upstream,
        "screen_custom",
        _fake_screen_custom([], {"quotes": [], "total": 0}),
    )

    response = await client.post("/providers/yfinance/screen", json=_screen_payload())

    assert response.status_code == 200
    body = response.json()
    assert body["entries"] == []
    assert body["total"] == 0
    assert body["has_more"] is False


def _sh_spot_frame() -> pd.DataFrame:
    return pd.DataFrame(
        [
            {
                "代码": "600519",
                "名称": "贵州茅台",
                "最新价": 1700.0,
                "涨跌幅": 1.5,
                "成交量": 12345,
                "市盈率-动态": 28.5,
                "市净率": 9.8,
                "总市值": 2.1e12,
            },
            {
                "代码": "601111",
                "名称": "中国国航",
                "最新价": 7.0,
                "涨跌幅": -2.0,
                "成交量": 99999,
                "市盈率-动态": None,
                "市净率": 1.2,
                "总市值": 1.0e11,
            },
            {
                # 停牌行无价格：任何区间条件都不通过
                "代码": "600000",
                "名称": "浦发银行",
                "最新价": None,
                "涨跌幅": None,
            },
        ]
    )


def _sz_spot_frame() -> pd.DataFrame:
    return pd.DataFrame(
        [
            {
                "代码": "300750",
                "名称": "宁德时代",
                "最新价": 200.0,
                "涨跌幅": 3.0,
                "成交量": 54321,
                "市盈率-动态": 22.0,
                "市净率": 4.5,
                "总市值": 9.0e11,
            },
            {
                "代码": "000001",
                "名称": "平安银行",
                "最新价": 10.0,
                "涨跌幅": -5.0,
                "成交量": 88888,
                "市盈率-动态": 5.0,
                "市净率": 0.6,
                "总市值": 2.0e11,
            },
        ]
    )


def _hk_spot_frame() -> pd.DataFrame:
    return pd.DataFrame(
        [
            {
                "代码": "00700",
                "名称": "腾讯控股",
                "最新价": 380.0,
                "涨跌幅": 2.0,
                "成交量": 1000,
                "市盈率-动态": 18.0,
                "市净率": 3.4,
            },
            {
                "代码": "00005",
                "名称": "汇丰控股",
                "最新价": 70.0,
                "涨跌幅": -1.0,
                "成交量": 2000,
                "市盈率-动态": 9.0,
                "市净率": 0.9,
            },
        ]
    )


def _catalog_call(calls: list[str]):
    def fake_call(function_name: str, **_kwargs: Any) -> pd.DataFrame:
        calls.append(function_name)
        if function_name == "stock_sh_a_spot_em":
            return _sh_spot_frame()
        if function_name == "stock_sz_a_spot_em":
            return _sz_spot_frame()
        if function_name == "stock_hk_spot_em":
            return _hk_spot_frame()
        if function_name in {
            "fund_etf_spot_em",
            "stock_zh_index_spot_em",
            "stock_hk_index_spot_em",
        }:
            return pd.DataFrame()
        raise AssertionError(f"unexpected AKShare call: {function_name}")

    return fake_call


def _ak_payload(**overrides: Any) -> dict[str, Any]:
    payload: dict[str, Any] = {
        "market": "CN",
        "conditions": [],
        "sorts": [],
        "offset": 0,
        "limit": 50,
    }
    payload.update(overrides)
    return payload


@pytest.mark.asyncio
async def test_akshare_screen_filters_sorts_and_pages_cn_catalog(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    calls: list[str] = []
    monkeypatch.setattr(akshare_upstream, "call", _catalog_call(calls))

    response = await client.post(
        "/providers/akshare/screen",
        json=_ak_payload(
            conditions=[{"factor_key": "simple.market_cap", "min": 1.5e11}],
            sorts=[{"factor_key": "simple.market_cap", "direction": "desc"}],
            offset=1,
            limit=2,
        ),
    )

    assert response.status_code == 200
    body = response.json()
    assert body["source"] == "akshare-screen"
    assert body["total"] == 3  # 600519 / 300750 / 000001 通过；601111 与停牌行被过滤
    assert body["has_more"] is False  # offset 1 + 2 条 == total 3
    assert [entry["instrument_id"] for entry in body["entries"]] == [
        "SZ.300750",
        "SZ.000001",
    ]
    first = body["entries"][0]
    assert first["name"] == "宁德时代"
    assert first["quote_currency"] == "CNY"
    # CN 成交量单位是手，输出换算成股
    assert first["values"]["simple.volume"] == 5_432_100
    assert first["values"]["simple.pe_ttm"] == 22.0
    assert first["values"]["simple.pb"] == 4.5


@pytest.mark.asyncio
async def test_akshare_screen_sort_ascending_sends_missing_values_last(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(akshare_upstream, "call", _catalog_call([]))

    response = await client.post(
        "/providers/akshare/screen",
        json=_ak_payload(
            market="SH",
            sorts=[{"factor_key": "simple.pe_ttm", "direction": "asc"}],
        ),
    )

    assert response.status_code == 200
    body = response.json()
    assert body["total"] == 3
    entries = body["entries"]
    assert [entry["instrument_id"] for entry in entries] == [
        "SH.600519",
        "SH.600000",
        "SH.601111",
    ]
    # 600000 停牌行与 601111 的 pe_ttm 均缺失 → values 整键省略且排在有值行之后
    assert "simple.pe_ttm" not in entries[1]["values"]
    assert "simple.pe_ttm" not in entries[2]["values"]
    assert entries[2]["values"]["simple.pb"] == 1.2


@pytest.mark.asyncio
async def test_akshare_screen_hk_volume_stays_in_shares(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(akshare_upstream, "call", _catalog_call([]))

    response = await client.post(
        "/providers/akshare/screen",
        json=_ak_payload(
            market="HK",
            conditions=[{"factor_key": "simple.volume", "min": 1500}],
        ),
    )

    assert response.status_code == 200
    body = response.json()
    assert body["total"] == 1
    entry = body["entries"][0]
    assert entry["instrument_id"] == "HK.00005"
    assert entry["quote_currency"] == "HKD"
    assert entry["values"]["simple.volume"] == 2000  # HK 帧单位即股，不换算
    assert "simple.market_cap" not in entry["values"]  # HK spot 无总市值列


@pytest.mark.asyncio
async def test_akshare_screen_rejects_hk_market_cap_factor(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(akshare_upstream, "call", _catalog_call([]))

    response = await client.post(
        "/providers/akshare/screen",
        json=_ak_payload(
            market="HK",
            conditions=[{"factor_key": "simple.market_cap", "min": 1}],
        ),
    )

    assert response.status_code == 400
    assert response.json()["error"]["code"] == "unsupported_kind"


@pytest.mark.asyncio
async def test_akshare_screen_rejects_unsupported_market_and_factor(
    client: httpx.AsyncClient,
) -> None:
    us = await client.post("/providers/akshare/screen", json=_ak_payload(market="US"))
    assert us.status_code == 400
    assert us.json()["error"]["code"] == "unsupported_market"

    unknown = await client.post(
        "/providers/akshare/screen",
        json=_ak_payload(conditions=[{"factor_key": "simple.roe", "min": 10}]),
    )
    assert unknown.status_code == 400
    assert unknown.json()["error"]["code"] == "unsupported_kind"

    enumeration = await client.post(
        "/providers/akshare/screen",
        json=_ak_payload(conditions=[{"factor_key": "simple.price", "in": [1, 2]}]),
    )
    assert enumeration.status_code == 400
    assert enumeration.json()["error"]["code"] == "unsupported_kind"


@pytest.mark.asyncio
async def test_akshare_screen_empty_result(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(akshare_upstream, "call", _catalog_call([]))

    response = await client.post(
        "/providers/akshare/screen",
        json=_ak_payload(conditions=[{"factor_key": "simple.price", "min": 99999}]),
    )

    assert response.status_code == 200
    body = response.json()
    assert body["entries"] == []
    assert body["total"] == 0
    assert body["has_more"] is False
