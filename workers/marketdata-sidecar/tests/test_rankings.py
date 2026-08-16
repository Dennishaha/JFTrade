"""Rankings route behavior for the AKShare and yfinance namespaces."""

from __future__ import annotations

from typing import Any

import httpx
import pandas as pd
import pytest

from marketdata_sidecar import akshare_upstream, upstream

# Captured before conftest installs its network guard on the module.
_REAL_SCREEN_QUOTES = upstream.screen_quotes


def _empty() -> pd.DataFrame:
    return pd.DataFrame()


def _sh_spot_frame() -> pd.DataFrame:
    return pd.DataFrame(
        [
            {
                "代码": "600519",
                "名称": "贵州茅台",
                "最新价": 1700.0,
                "涨跌幅": 1.5,
                "涨跌额": 25.0,
                "成交量": 12345,
                "成交额": 2.1e9,
                "换手率": 0.5,
                "市盈率-动态": 28.5,
                "总市值": 2.1e12,
            },
            {
                "代码": "601111",
                "名称": "中国国航",
                "最新价": 7.0,
                "涨跌幅": -2.0,
                "涨跌额": -0.14,
                "成交量": 99999,
                "成交额": 7.0e8,
                "换手率": 1.2,
                "市盈率-动态": None,
                "总市值": 1.0e11,
            },
            {
                # Suspended rows carry no price/change and must be excluded.
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
                "涨跌额": 5.8,
                "成交量": 54321,
                "成交额": 1.1e10,
                "换手率": 2.3,
                "市盈率-动态": 22.0,
                "总市值": 9.0e11,
            },
            {
                "代码": "000001",
                "名称": "平安银行",
                "最新价": 10.0,
                "涨跌幅": -5.0,
                "涨跌额": -0.5,
                "成交量": 88888,
                "成交额": 8.9e8,
                "换手率": 0.9,
                "市盈率-动态": 5.0,
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
                "涨跌额": 7.4,
                "成交量": 1000,
                "成交额": 3.8e8,
            },
            {
                "代码": "00005",
                "名称": "汇丰控股",
                "最新价": 70.0,
                "涨跌幅": -1.0,
                "涨跌额": -0.7,
                "成交量": 2000,
                "成交额": 1.4e8,
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
            return _empty()
        raise AssertionError(f"unexpected AKShare call: {function_name}")

    return fake_call


def _entry_ids(body: dict[str, Any]) -> list[str]:
    return [entry["instrument_id"] for entry in body["entries"]]


@pytest.mark.asyncio
async def test_akshare_gainers_sort_cn_merge_by_change_rate_desc(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(akshare_upstream, "call", _catalog_call([]))

    response = await client.get(
        "/providers/akshare/rankings",
        params={"market": "CN", "kind": "gainers"},
    )

    assert response.status_code == 200
    body = response.json()
    assert body["market"] == "CN"
    assert body["kind"] == "gainers"
    assert body["source"] == "akshare-rankings"
    assert _entry_ids(body) == [
        "SZ.300750",
        "SH.600519",
        "SH.601111",
        "SZ.000001",
    ]
    first = body["entries"][0]
    assert first["name"] == "宁德时代"
    assert first["price"] == 200.0
    assert first["change_rate"] == 3.0
    assert first["change_amount"] == 5.8
    assert first["volume"] == 54321
    assert first["turnover"] == 1.1e10
    assert first["turnover_ratio"] == 2.3
    assert first["pe_ttm"] == 22.0
    assert first["market_cap"] == 9.0e11


@pytest.mark.asyncio
async def test_akshare_losers_sort_by_change_rate_asc(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(akshare_upstream, "call", _catalog_call([]))

    response = await client.get(
        "/providers/akshare/rankings",
        params={"market": "CN", "kind": "losers"},
    )

    assert response.status_code == 200
    assert _entry_ids(response.json()) == [
        "SZ.000001",
        "SH.601111",
        "SH.600519",
        "SZ.300750",
    ]


@pytest.mark.asyncio
async def test_akshare_active_sort_by_turnover_desc(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(akshare_upstream, "call", _catalog_call([]))

    response = await client.get(
        "/providers/akshare/rankings",
        params={"market": "SZ", "kind": "active"},
    )

    assert response.status_code == 200
    body = response.json()
    assert body["market"] == "SZ"
    assert _entry_ids(body) == ["SZ.300750", "SZ.000001"]


@pytest.mark.asyncio
async def test_akshare_hk_rankings_use_hk_identity(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(akshare_upstream, "call", _catalog_call([]))

    response = await client.get(
        "/providers/akshare/rankings",
        params={"market": "HK", "kind": "gainers"},
    )

    assert response.status_code == 200
    body = response.json()
    assert _entry_ids(body) == ["HK.00700", "HK.00005"]
    entry = body["entries"][0]
    assert entry["turnover_ratio"] is None
    assert entry["pe_ttm"] is None
    assert entry["market_cap"] is None


@pytest.mark.asyncio
async def test_akshare_rankings_limit_clamps_entries(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(akshare_upstream, "call", _catalog_call([]))

    limited = await client.get(
        "/providers/akshare/rankings",
        params={"market": "CN", "kind": "gainers", "limit": 2},
    )
    too_small = await client.get(
        "/providers/akshare/rankings",
        params={"market": "CN", "kind": "gainers", "limit": 0},
    )
    too_large = await client.get(
        "/providers/akshare/rankings",
        params={"market": "CN", "kind": "gainers", "limit": 101},
    )

    assert limited.status_code == 200
    assert len(limited.json()["entries"]) == 2
    assert too_small.status_code == 400
    assert too_small.json()["error"]["code"] == "invalid_request"
    assert too_large.status_code == 400


@pytest.mark.asyncio
async def test_akshare_rankings_reject_invalid_kind_and_us_market(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(akshare_upstream, "call", _catalog_call([]))

    bad_kind = await client.get(
        "/providers/akshare/rankings",
        params={"market": "CN", "kind": "trending"},
    )
    us_market = await client.get(
        "/providers/akshare/rankings",
        params={"market": "US", "kind": "gainers"},
    )

    assert bad_kind.status_code == 400
    assert bad_kind.json()["error"]["code"] == "unsupported_kind"
    assert us_market.status_code == 400
    assert us_market.json()["error"]["code"] == "AKSHARE_UNSUPPORTED"


@pytest.mark.asyncio
async def test_akshare_rankings_reuse_cached_catalog_frames(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    calls: list[str] = []
    monkeypatch.setattr(akshare_upstream, "call", _catalog_call(calls))

    first = await client.get(
        "/providers/akshare/rankings",
        params={"market": "CN", "kind": "gainers"},
    )
    calls_after_first = len(calls)
    second = await client.get(
        "/providers/akshare/rankings",
        params={"market": "CN", "kind": "losers"},
    )

    assert first.status_code == 200
    assert second.status_code == 200
    # The second request is served entirely from the 15s catalog cache.
    assert len(calls) == calls_after_first


def _screen_quotes() -> list[dict[str, Any]]:
    return [
        {
            "symbol": "NVDA",
            "shortName": "NVIDIA Corporation",
            "regularMarketPrice": 180.5,
            "regularMarketChangePercent": 4.2,
            "regularMarketChange": 7.3,
            "regularMarketVolume": 12345678,
            "marketCap": 4.4e12,
            "trailingPE": 55.1,
        },
        {
            "symbol": "ABC",
            "longName": "ABC Holdings",
            "regularMarketPrice": 12.0,
            "regularMarketChangePercent": 1.0,
            "regularMarketChange": 0.12,
            "regularMarketDayVolume": 777,
        },
        {
            # No usable price/change: excluded from the wire response.
            "symbol": "BROKEN",
        },
    ]


@pytest.mark.asyncio
@pytest.mark.parametrize(
    ("kind", "query_id"),
    [
        ("gainers", "day_gainers"),
        ("losers", "day_losers"),
        ("active", "most_actives"),
    ],
)
async def test_yfinance_rankings_map_predefined_queries(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
    kind: str,
    query_id: str,
) -> None:
    calls: list[tuple[str, int]] = []

    def fake_screen(requested: str, count: int) -> list[dict[str, Any]]:
        calls.append((requested, count))
        return _screen_quotes()

    monkeypatch.setattr(upstream, "screen_quotes", fake_screen)

    response = await client.get(
        "/providers/yfinance/rankings",
        params={"market": "US", "kind": kind, "limit": 5},
    )

    assert response.status_code == 200
    assert calls == [(query_id, 5)]
    body = response.json()
    assert body["market"] == "US"
    assert body["kind"] == kind
    assert body["source"] == "yfinance-rankings"
    assert _entry_ids(body) == ["US.NVDA", "US.ABC"]
    first = body["entries"][0]
    assert first["name"] == "NVIDIA Corporation"
    assert first["price"] == 180.5
    assert first["change_rate"] == 4.2
    assert first["change_amount"] == 7.3
    assert first["volume"] == 12345678
    assert first["turnover"] is None
    assert first["turnover_ratio"] is None
    assert first["pe_ttm"] == 55.1
    assert first["market_cap"] == 4.4e12
    second = body["entries"][1]
    assert second["name"] == "ABC Holdings"
    assert second["volume"] == 777
    assert second["pe_ttm"] is None


@pytest.mark.asyncio
async def test_yfinance_rankings_reject_non_us_markets(
    client: httpx.AsyncClient,
) -> None:
    for market in ("HK", "SH", "CN"):
        response = await client.get(
            "/providers/yfinance/rankings",
            params={"market": market, "kind": "gainers"},
        )
        assert response.status_code == 400
        assert response.json()["error"]["code"] == "unsupported_market"


@pytest.mark.asyncio
async def test_yfinance_screen_cache_avoids_second_upstream_fetch(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    class _FakeYfinance:
        def __init__(self) -> None:
            self.calls: list[tuple[str, int]] = []

        def screen(self, query_id: str, count: int, session: Any = None) -> dict[str, Any]:
            self.calls.append((query_id, count))
            return {"quotes": [{"symbol": "NVDA", "regularMarketPrice": 1.0}]}

    fake = _FakeYfinance()
    # conftest replaces screen_quotes with a network guard; restore the real
    # implementation here while keeping the runtime itself fully mocked.
    monkeypatch.setattr(upstream, "screen_quotes", _REAL_SCREEN_QUOTES)
    monkeypatch.setattr(
        upstream,
        "require_runtime",
        lambda: upstream._RuntimeComponents(yfinance=fake, session=None),
    )

    first = upstream.screen_quotes("day_gainers", 20)
    second = upstream.screen_quotes("day_gainers", 20)

    assert first == second == [{"symbol": "NVDA", "regularMarketPrice": 1.0}]
    assert fake.calls == [("day_gainers", 20)]


@pytest.mark.asyncio
@pytest.mark.parametrize(
    ("path", "params"),
    [
        ("/providers/akshare/rankings", {"market": "CN", "kind": "gainers"}),
        ("/providers/akshare/industries", {"kind": "industry"}),
        ("/providers/akshare/industries/半导体/members", {"kind": "industry"}),
    ],
)
async def test_akshare_rankings_paths_are_warming_gated(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
    path: str,
    params: dict[str, str],
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

    response = await client.get(path, params=params)

    assert response.status_code == 503
    assert response.json()["error"]["code"] == "AKSHARE_RUNTIME_WARMING"


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "path",
    ["/rankings", "/providers/yfinance/rankings"],
)
async def test_yfinance_rankings_paths_are_warming_gated(
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

    response = await client.get(path, params={"market": "US", "kind": "gainers"})

    assert response.status_code == 503
    assert response.json()["error"]["code"] == "YFINANCE_RUNTIME_WARMING"
