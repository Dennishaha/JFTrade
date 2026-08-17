"""Eastmoney clist direct fetcher for full-market US/HK spot frames.

为什么不用 akshare 的 ``stock_us_spot_em`` / ``stock_hk_spot_em``:akshare
1.18.91 请求东财 clist 时 fields 本已包含 f20(总市值)/f23(市净率)/f115
(PE TTM),但位置式列映射把它们标为 ``"_"`` 丢弃(``stock_hist_em.py:1593``
US 版、``:1225`` HK 版),筛选需要的 US 市净率/PE 与 HK 总市值因此不可得。
本模块直连同一 clist 端点,按 f-code 显式映射列名,输出是 akshare 帧的
超集(列名逐一对齐,只增不改)。

字段语义(东财 push2 clist 约定,与 akshare_upstream.EASTMONEY_SPOT_FIELDS
注释同源;fs/ut 照抄 akshare 的用法):

- f2 最新价 / f3 涨跌幅% / f4 涨跌额 / f5 成交量 / f6 成交额 / f7 振幅 /
  f8 换手率 / f9 市盈率(动态口径,与沪深详情一致) / f12 代码(US 为
  ticker 简称,HK 为数字代码) / f13 市场编号(US: 105/106/107;HK: 128) /
  f14 名称 / f15 最高 / f16 最低 / f17 开盘 / f18 昨收 / f20 总市值 /
  f23 市净率 / f115 PE TTM(输出为 市盈率-动态,与沪深 spot 帧同名同义)

成交量单位:东财美股与港股 clist 的 f5 均以股计(只有 A 股以手计),
与 akshare 封装输出一致,sidecar 消费方(快照/榜单/筛选)本就按股处理,
无需换算;``AKInstrument.volume_multiplier`` 也只对 SH/SZ 乘 100。

代码列形态:US 拼成 akshare 同形态 ``编码.简称``(如 ``105.AAPL``),HK 为
f12 本身(如 ``00700``)。序号列为本地按涨跌幅降序的 1 基排名,对齐
akshare ``fetch_paginated_data`` 的行为;无消费方读取,仅为超集保真。

分页照抄 akshare ``fetch_paginated_data`` 的方式(首页由 data.total 与实际
返回条数定页数,pn 翻页),并去掉礼貌性 sleep:目录请求运行在 12 秒的池化
deadline 内,akshare 版每页随机等待 0.5–1.5s 必然超时。所有页面复用同一
兼容 Session,避免每页重新建立连接。

传输层:直接 ``requests.get`` 复用 akshare_upstream 安装的全进程兼容
Session(自动把 push2 clist 重写为 webguest 访客端点);池化/取消由调用方
(catalog 经路由 ``_translate`` → ``akshare_upstream.run``)提供,本模块只做
``ensure_request_active``/``require_runtime`` 检查,与 ``spot_rows`` 先例一致。
"""

from __future__ import annotations

import math
from typing import Any

import pandas as pd

from . import akshare_upstream
from .conversion import clean_text
from .errors import SidecarError, invalid_request

CLIST_URL = "https://72.push2.eastmoney.com/api/qt/clist/get"
CLIST_FIELDS = "f2,f3,f4,f5,f6,f7,f8,f9,f12,f13,f14,f15,f16,f17,f18,f20,f23,f115"
CLIST_PAGE_SIZE = 100
CLIST_TIMEOUT_SECONDS = 10

_MARKET_FS = {
    "US": "m:105,m:106,m:107",
    "HK": "m:128 t:3,m:128 t:4,m:128 t:1,m:128 t:2",
}

# f-code → 输出列名;OHLC 列名按市场对齐 akshare 各自帧的既有叫法。
_COMMON_COLUMNS = {
    "f2": "最新价",
    "f3": "涨跌幅",
    "f4": "涨跌额",
    "f5": "成交量",
    "f6": "成交额",
    "f7": "振幅",
    "f8": "换手率",
    "f9": "市盈率",
    "f14": "名称",
    "f20": "总市值",
    "f23": "市净率",
    "f115": "市盈率-动态",
}
_US_OHLC = {"f15": "最高价", "f16": "最低价", "f17": "开盘价", "f18": "昨收价"}
_HK_OHLC = {"f15": "最高", "f16": "最低", "f17": "今开", "f18": "昨收"}

_NUMERIC_COLUMNS = (
    "最新价",
    "涨跌幅",
    "涨跌额",
    "成交量",
    "成交额",
    "振幅",
    "换手率",
    "市盈率",
    "最高价",
    "最低价",
    "开盘价",
    "昨收价",
    "最高",
    "最低",
    "今开",
    "昨收",
    "总市值",
    "市净率",
    "市盈率-动态",
)


def fetch_spot_frame_clist(market: str) -> pd.DataFrame:
    """Fetch the full-market US/HK spot frame directly from Eastmoney clist."""
    token = market.strip().upper()
    fs = _MARKET_FS.get(token)
    if fs is None:
        raise invalid_request(
            "unsupported_market",
            f"clist spot frames are only available for: {sorted(_MARKET_FS)}",
        )
    records = _fetch_all_pages(fs)
    return _build_frame(token, records)


def _fetch_all_pages(fs: str) -> list[dict[str, Any]]:
    import requests

    with requests.Session() as session:
        total, records = _clist_page(fs, 1, session)
        if total and not records:
            raise _schema_error("Eastmoney clist first page is empty")
        actual_page_size = len(records) or CLIST_PAGE_SIZE
        pages = math.ceil(total / actual_page_size) if total else 1
        for page in range(2, pages + 1):
            records.extend(_clist_page(fs, page, session)[1])
    if len(records) < total:
        raise _schema_error("Eastmoney clist response is incomplete")
    return records


def _clist_page(
    fs: str,
    page: int,
    session: Any | None = None,
) -> tuple[int, list[dict[str, Any]]]:
    """Fetch one clist page; isolated so tests can mock the HTTP boundary."""
    akshare_upstream.ensure_request_active()
    akshare_upstream.require_runtime()
    import requests

    requester = session if session is not None else requests
    response = requester.get(
        CLIST_URL,
        params={
            "pn": str(page),
            "pz": str(CLIST_PAGE_SIZE),
            "po": "1",
            "np": "1",
            "ut": akshare_upstream.EASTMONEY_TOKEN,
            "fltt": "2",
            "invt": "2",
            "fid": "f12",
            "fs": fs,
            "fields": CLIST_FIELDS,
        },
        timeout=CLIST_TIMEOUT_SECONDS,
    )
    response.raise_for_status()
    payload = response.json()
    data = payload.get("data") if isinstance(payload, dict) else None
    diff = data.get("diff") if isinstance(data, dict) else None
    total = data.get("total") if isinstance(data, dict) else None
    if not isinstance(diff, list) or not all(isinstance(row, dict) for row in diff):
        raise _schema_error("Eastmoney clist response has an invalid schema")
    akshare_upstream.ensure_request_active()
    return int(total or 0), diff


def _schema_error(message: str) -> SidecarError:
    return SidecarError(502, "AKSHARE_SCHEMA_ERROR", message)


def _build_frame(market: str, records: list[dict[str, Any]]) -> pd.DataFrame:
    columns = dict(_COMMON_COLUMNS)
    columns.update(_US_OHLC if market == "US" else _HK_OHLC)
    rows: list[dict[str, Any]] = []
    for record in records:
        row = {name: record.get(code) for code, name in columns.items()}
        if market == "US":
            # 对齐 akshare 的 105.AAPL 形态:市场编码 + "." + ticker 简称
            code = clean_text(record.get("f13"))
            ticker = clean_text(record.get("f12"))
            row["代码"] = f"{code}.{ticker}" if code and ticker else None
        else:
            row["代码"] = clean_text(record.get("f12"))
        rows.append(row)
    # akshare 按涨跌幅降序交付并给出 1 基序号;无消费方依赖,仅超集保真
    rows.sort(
        key=lambda row: (
            pd.notna(_to_number(row["涨跌幅"])),
            _to_number(row["涨跌幅"]),
        ),
        reverse=True,
    )
    for index, row in enumerate(rows, start=1):
        row["序号"] = index
    frame = pd.DataFrame(rows)
    for name in _NUMERIC_COLUMNS:
        if name in frame.columns:
            frame[name] = pd.to_numeric(frame[name], errors="coerce")
    return frame


def _to_number(value: Any) -> float:
    number = pd.to_numeric(pd.Series([value]), errors="coerce").iloc[0]
    return float(number) if pd.notna(number) else float("nan")
