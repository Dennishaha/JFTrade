"""Shared AKShare instrument identity and symbol normalization."""

from __future__ import annotations

import re

from .errors import invalid_request
from .routes.common import MARKET_SPECS

CODE_PATTERN = re.compile(r"^[A-Z0-9.^=_-]{1,64}$")
MARKET_CURRENCY = {"US": "USD", "HK": "HKD", "SH": "CNY", "SZ": "CNY"}
MARKET_EXCHANGE = {"US": None, "HK": "HKEX", "SH": "SSE", "SZ": "SZSE"}

HK_INDEX_IDS = {
    "HSI": "800000",
    "恒生指数": "800000",
    "HSCEI": "800100",
    "恒生中国企业指数": "800100",
    "国企指数": "800100",
    "HSTECH": "800700",
    "恒生科技指数": "800700",
}
US_INDEX_IDS = {
    "DJIA": ".DJI",
    "DJI": ".DJI",
    "道琼斯": ".DJI",
    "道琼斯指数": ".DJI",
    "SPX": ".SPX",
    "SP500": ".SPX",
    "标普500": ".SPX",
    "标普500指数": ".SPX",
    "NDX": ".NDX",
    "纳斯达克100": ".NDX",
    "纳斯达克100指数": ".NDX",
}


def normalize_identity(market: str, symbol: str) -> tuple[str, str]:
    normalized_market = market.strip().upper()
    normalized_symbol = symbol.strip().upper()
    if normalized_market == "CN":
        prefix, separator, code = normalized_symbol.partition(".")
        if separator != "." or prefix not in {"SH", "SZ"}:
            raise invalid_request(
                "invalid_symbol",
                "CN symbols must use SH.<code> or SZ.<code>",
            )
        normalized_market, normalized_symbol = prefix, code
    normalized_market = _normalize_market(normalized_market)
    for prefix in (normalized_market, *MARKET_SPECS[normalized_market].aliases):
        if normalized_symbol.startswith(f"{prefix}."):
            normalized_symbol = normalized_symbol[len(prefix) + 1 :]
            break
    if normalized_market in {"SH", "SZ"}:
        if not re.fullmatch(r"\d{6}", normalized_symbol):
            raise invalid_request("invalid_symbol", "China symbols must be six digits")
    elif normalized_market == "HK" and normalized_symbol.isdigit():
        normalized_symbol = f"{int(normalized_symbol):05d}"
    elif not CODE_PATTERN.fullmatch(normalized_symbol):
        raise invalid_request("invalid_symbol", "symbol has an invalid format")
    return normalized_market, normalized_symbol


def _normalize_market(market: str) -> str:
    token = market.strip().upper()
    for code, spec in MARKET_SPECS.items():
        if token == code or token in spec.aliases:
            return code
    raise invalid_request("unsupported_market", f"unsupported market: {token or market}")


def _stock_symbols(market: str, raw_code: str) -> tuple[str | None, str]:
    token = raw_code.strip().upper()
    if market == "US":
        prefix, separator, suffix = token.partition(".")
        symbol = suffix if separator and prefix.isdigit() else token
        return (symbol if CODE_PATTERN.fullmatch(symbol) else None), token
    if market == "HK":
        if not token.isdigit():
            return None, token
        return f"{int(token):05d}", f"{int(token):05d}"
    if not re.fullmatch(r"\d{6}", token):
        return None, token
    return token, token


def _etf_market(code: str) -> str:
    return "SH" if code.startswith(("5", "6")) else "SZ"


def _hk_index_symbol(code: str, name: str | None) -> str | None:
    for token in (code.strip().upper(), (name or "").replace(" ", "").upper()):
        if token in HK_INDEX_IDS:
            return HK_INDEX_IDS[token]
    if code.isdigit():
        return f"{int(code):05d}"
    token = code.strip().upper()
    return token if CODE_PATTERN.fullmatch(token) else None


def _us_index_symbol(code: str | None, name: str | None) -> str | None:
    tokens = [
        (code or "").replace(" ", "").replace("&", "").upper(),
        (name or "").replace(" ", "").replace("&", "").upper(),
    ]
    for token in tokens:
        if token in US_INDEX_IDS:
            return US_INDEX_IDS[token]
        for alias, symbol in US_INDEX_IDS.items():
            if alias and alias in token:
                return symbol
    return None
