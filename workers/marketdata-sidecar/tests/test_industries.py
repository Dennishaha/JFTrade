"""AKShare industry/concept board routes behavior."""

from __future__ import annotations

from typing import Any

import httpx
import pandas as pd
import pytest

from marketdata_sidecar import akshare_upstream


def _industry_boards_frame() -> pd.DataFrame:
    return pd.DataFrame(
        [
            {
                "排名": 1,
                "板块名称": "半导体",
                "板块代码": "BK1036",
                "最新价": 1200.0,
                "涨跌幅": 1.2,
                "领涨股票": "中芯国际",
                "领涨股票-涨跌幅": 5.5,
            },
            {
                "排名": 2,
                "板块名称": "酿酒行业",
                "板块代码": "BK0477",
                "最新价": 900.0,
                "涨跌幅": -0.4,
            },
            {"排名": 3, "板块名称": None, "板块代码": None},
        ]
    )


def _concept_boards_frame() -> pd.DataFrame:
    return pd.DataFrame(
        [
            {
                "板块名称": "可燃冰",
                "板块代码": "BK0818",
                "涨跌幅": 2.4,
                "领涨股票": "石化机械",
                "领涨股票-涨跌幅": 9.9,
            },
        ]
    )


def _industry_members_frame() -> pd.DataFrame:
    return pd.DataFrame(
        [
            {
                "序号": 1,
                "代码": "688981",
                "名称": "中芯国际",
                "最新价": 90.0,
                "涨跌幅": 5.5,
                "涨跌额": 4.7,
                "成交量": 100000,
                "成交额": 9.0e9,
                "换手率": 3.1,
                "市盈率-动态": 60.0,
            },
            {
                "序号": 2,
                "代码": "002371",
                "名称": "北方华创",
                "最新价": 320.0,
                "涨跌幅": 2.1,
                "成交额": 4.0e9,
            },
            {
                # Beijing listings are outside the JFTrade market model.
                "序号": 3,
                "代码": "830799",
                "名称": "北交所标的",
                "最新价": 10.0,
                "涨跌幅": 1.0,
            },
            {
                # No usable quote: excluded from the wire response.
                "序号": 4,
                "代码": "600999",
                "名称": "停牌股",
                "最新价": None,
                "涨跌幅": None,
            },
        ]
    )


def _boards_call(calls: list[tuple[str, Any]], members_frame: pd.DataFrame | None = None):
    def fake_call(function_name: str, **kwargs: Any) -> pd.DataFrame:
        calls.append((function_name, kwargs.get("symbol")))
        if function_name == "stock_board_industry_name_em":
            return _industry_boards_frame()
        if function_name == "stock_board_concept_name_em":
            return _concept_boards_frame()
        if function_name in {"stock_board_industry_cons_em", "stock_board_concept_cons_em"}:
            return members_frame if members_frame is not None else pd.DataFrame()
        raise AssertionError(f"unexpected AKShare call: {function_name} {kwargs}")

    return fake_call


@pytest.mark.asyncio
async def test_industry_boards_map_available_fields(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(akshare_upstream, "call", _boards_call([]))

    response = await client.get(
        "/providers/akshare/industries",
        params={"kind": "industry"},
    )

    assert response.status_code == 200
    body = response.json()
    assert body["market"] == "CN"
    assert body["kind"] == "industry"
    assert body["source"] == "akshare-industries"
    assert body["boards"] == [
        {
            "name": "半导体",
            "change_rate": 1.2,
            "turnover": None,
            "volume": None,
            "leading_stock_name": "中芯国际",
            "leading_stock_change_rate": 5.5,
        },
        {
            "name": "酿酒行业",
            "change_rate": -0.4,
            "turnover": None,
            "volume": None,
            "leading_stock_name": None,
            "leading_stock_change_rate": None,
        },
    ]


@pytest.mark.asyncio
async def test_concept_boards_use_concept_listing(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    calls: list[tuple[str, Any]] = []
    monkeypatch.setattr(akshare_upstream, "call", _boards_call(calls))

    response = await client.get(
        "/providers/akshare/industries",
        params={"kind": "concept"},
    )

    assert response.status_code == 200
    body = response.json()
    assert body["kind"] == "concept"
    assert [board["name"] for board in body["boards"]] == ["可燃冰"]
    assert calls == [("stock_board_concept_name_em", None)]


@pytest.mark.asyncio
async def test_industry_members_resolve_board_code_and_rankings_shape(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    calls: list[tuple[str, Any]] = []
    monkeypatch.setattr(
        akshare_upstream,
        "call",
        _boards_call(calls, _industry_members_frame()),
    )

    response = await client.get(
        "/providers/akshare/industries/半导体/members",
        params={"kind": "industry"},
    )

    assert response.status_code == 200
    body = response.json()
    assert body["market"] == "CN"
    assert body["kind"] == "industry"
    assert body["board"] == "半导体"
    assert body["source"] == "akshare-industries"
    assert [entry["instrument_id"] for entry in body["entries"]] == [
        "SH.688981",
        "SZ.002371",
    ]
    first = body["entries"][0]
    assert first["name"] == "中芯国际"
    assert first["price"] == 90.0
    assert first["change_rate"] == 5.5
    assert first["change_amount"] == 4.7
    assert first["volume"] == 100000
    assert first["turnover"] == 9.0e9
    assert first["turnover_ratio"] == 3.1
    assert first["pe_ttm"] == 60.0
    assert first["market_cap"] is None
    # The board code is resolved once so AKShare skips its own name lookup.
    assert ("stock_board_industry_cons_em", "BK1036") in calls


@pytest.mark.asyncio
async def test_concept_members_use_concept_function(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    calls: list[tuple[str, Any]] = []
    monkeypatch.setattr(akshare_upstream, "call", _boards_call(calls))

    response = await client.get(
        "/providers/akshare/industries/可燃冰/members",
        params={"kind": "concept"},
    )

    assert response.status_code == 200
    assert response.json()["entries"] == []
    assert ("stock_board_concept_cons_em", "BK0818") in calls


@pytest.mark.asyncio
async def test_members_limit_clamps_entries(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(
        akshare_upstream,
        "call",
        _boards_call([], _industry_members_frame()),
    )

    limited = await client.get(
        "/providers/akshare/industries/半导体/members",
        params={"kind": "industry", "limit": 1},
    )
    too_large = await client.get(
        "/providers/akshare/industries/半导体/members",
        params={"kind": "industry", "limit": 501},
    )

    assert limited.status_code == 200
    assert len(limited.json()["entries"]) == 1
    assert too_large.status_code == 400
    assert too_large.json()["error"]["code"] == "invalid_request"


@pytest.mark.asyncio
async def test_unknown_board_is_not_found(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(akshare_upstream, "call", _boards_call([]))

    response = await client.get(
        "/providers/akshare/industries/不存在板块/members",
        params={"kind": "industry"},
    )

    assert response.status_code == 404
    assert response.json()["error"]["code"] == "board_not_found"


@pytest.mark.asyncio
@pytest.mark.parametrize("market", ["US", "HK"])
async def test_boards_and_members_reject_us_hk_markets(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
    market: str,
) -> None:
    monkeypatch.setattr(akshare_upstream, "call", _boards_call([]))

    boards = await client.get(
        "/providers/akshare/industries",
        params={"kind": "industry", "market": market},
    )
    members = await client.get(
        "/providers/akshare/industries/半导体/members",
        params={"kind": "industry", "market": market},
    )

    for response in (boards, members):
        assert response.status_code == 400
        assert response.json()["error"]["code"] == "AKSHARE_UNSUPPORTED"


@pytest.mark.asyncio
async def test_invalid_board_kind_is_rejected(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(akshare_upstream, "call", _boards_call([]))

    response = await client.get(
        "/providers/akshare/industries",
        params={"kind": "sector"},
    )

    assert response.status_code == 400
    assert response.json()["error"]["code"] == "unsupported_kind"


@pytest.mark.asyncio
async def test_boards_and_members_caches_avoid_second_fetch(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    calls: list[tuple[str, Any]] = []
    monkeypatch.setattr(
        akshare_upstream,
        "call",
        _boards_call(calls, _industry_members_frame()),
    )

    await client.get("/providers/akshare/industries", params={"kind": "industry"})
    await client.get("/providers/akshare/industries", params={"kind": "industry"})
    await client.get(
        "/providers/akshare/industries/半导体/members",
        params={"kind": "industry"},
    )
    await client.get(
        "/providers/akshare/industries/半导体/members",
        params={"kind": "industry", "limit": 1},
    )

    assert calls.count(("stock_board_industry_name_em", None)) == 1
    assert calls.count(("stock_board_industry_cons_em", "BK1036")) == 1


def _ascii_boards_frame() -> pd.DataFrame:
    return pd.DataFrame(
        [
            {
                "板块名称": "ChatGPT概念",
                "板块代码": "BK1000",
                "涨跌幅": 3.3,
                "领涨股票": "科大讯飞",
                "领涨股票-涨跌幅": 7.7,
            },
        ]
    )


def _ascii_boards_call(calls: list[tuple[str, Any]]):
    def fake_call(function_name: str, **kwargs: Any) -> pd.DataFrame:
        calls.append((function_name, kwargs.get("symbol")))
        if function_name == "stock_board_industry_name_em":
            return pd.DataFrame()
        if function_name == "stock_board_concept_name_em":
            return _ascii_boards_frame()
        if function_name == "stock_board_concept_cons_em":
            return pd.DataFrame(
                [{"代码": "002230", "名称": "科大讯飞", "最新价": 50.0, "涨跌幅": 7.7}]
            )
        if function_name == "stock_board_industry_cons_em":
            return pd.DataFrame()
        raise AssertionError(f"unexpected AKShare call: {function_name} {kwargs}")

    return fake_call


@pytest.mark.asyncio
async def test_members_resolve_uppercased_ascii_board_name(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    calls: list[tuple[str, Any]] = []
    monkeypatch.setattr(akshare_upstream, "call", _ascii_boards_call(calls))

    response = await client.get(
        "/providers/akshare/industries/CHATGPT概念/members",
        params={"kind": "concept"},
    )

    assert response.status_code == 200
    body = response.json()
    assert body["kind"] == "concept"
    # The response echoes the canonical board name from the listing frame.
    assert body["board"] == "ChatGPT概念"
    assert [entry["instrument_id"] for entry in body["entries"]] == ["SZ.002230"]
    assert ("stock_board_concept_cons_em", "BK1000") in calls


@pytest.mark.asyncio
async def test_members_prefer_exact_match_over_case_variant(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    def fake_call(function_name: str, **kwargs: Any) -> pd.DataFrame:
        if function_name == "stock_board_concept_name_em":
            return pd.DataFrame(
                [
                    {"板块名称": "CHATGPT概念", "板块代码": "BK1001", "涨跌幅": 1.0},
                    {"板块名称": "ChatGPT概念", "板块代码": "BK1000", "涨跌幅": 3.3},
                ]
            )
        if function_name == "stock_board_concept_cons_em":
            return pd.DataFrame()
        if function_name == "stock_board_industry_name_em":
            return pd.DataFrame()
        raise AssertionError(f"unexpected AKShare call: {function_name} {kwargs}")

    monkeypatch.setattr(akshare_upstream, "call", fake_call)

    exact = await client.get(
        "/providers/akshare/industries/ChatGPT概念/members",
        params={"kind": "concept"},
    )
    variant = await client.get(
        "/providers/akshare/industries/chatgpt概念/members",
        params={"kind": "concept"},
    )

    assert exact.status_code == 200
    assert exact.json()["board"] == "ChatGPT概念"
    assert variant.status_code == 200
    # The lowercase request matches neither exactly; the first case-variant
    # in listing order wins.
    assert variant.json()["board"] == "CHATGPT概念"


@pytest.mark.asyncio
async def test_members_without_kind_search_industry_then_concept(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    calls: list[tuple[str, Any]] = []
    monkeypatch.setattr(akshare_upstream, "call", _ascii_boards_call(calls))

    concept = await client.get("/providers/akshare/industries/CHATGPT概念/members")

    assert concept.status_code == 200
    body = concept.json()
    assert body["kind"] == "concept"
    assert body["board"] == "ChatGPT概念"
    assert [entry["instrument_id"] for entry in body["entries"]] == ["SZ.002230"]
    assert ("stock_board_concept_cons_em", "BK1000") in calls


@pytest.mark.asyncio
async def test_members_without_kind_resolve_industry_board_first(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    calls: list[tuple[str, Any]] = []
    monkeypatch.setattr(
        akshare_upstream,
        "call",
        _boards_call(calls, _industry_members_frame()),
    )

    response = await client.get("/providers/akshare/industries/半导体/members")

    assert response.status_code == 200
    body = response.json()
    assert body["kind"] == "industry"
    assert body["board"] == "半导体"
    assert ("stock_board_industry_cons_em", "BK1036") in calls
    # The industry frame matched, so the concept listing is never fetched.
    assert ("stock_board_concept_name_em", None) not in calls


@pytest.mark.asyncio
async def test_members_without_kind_unknown_board_is_not_found(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(akshare_upstream, "call", _ascii_boards_call([]))

    response = await client.get("/providers/akshare/industries/不存在板块/members")

    assert response.status_code == 404
    assert response.json()["error"]["code"] == "board_not_found"
