"""Shared market routing, symbol validation, and Yahoo field normalization."""

from __future__ import annotations

import re
from dataclasses import dataclass
from datetime import date, datetime, timezone
from typing import Any, Mapping

from ..conversion import clean_text, parse_rfc3339_utc
from ..errors import invalid_request

CANDLE_SESSION_ORDER = ("regular", "extended", "overnight")
CANDLE_INTRADAY_PERIODS = frozenset({"1m", "5m", "15m", "30m", "1h"})
CANDLE_ADJUSTMENTS = ("none", "forward", "backward")


def parse_candle_adjustment(value: str | None) -> str:
    """Normalize the optional candle price-adjustment mode."""
    if value is None:
        return "none"
    normalized = value.strip().lower()
    if normalized not in CANDLE_ADJUSTMENTS:
        raise invalid_request(
            "unsupported_adjustment",
            f"unsupported candle adjustment: {value}",
        )
    return normalized


def parse_candle_sessions(
    values: list[str] | None,
    *,
    market: str,
    period: str,
    extended: bool = True,
    overnight: bool = False,
) -> tuple[str, ...]:
    intraday = period in CANDLE_INTRADAY_PERIODS and market == "US"
    available = {"regular"}
    if intraday and extended:
        available.add("extended")
        if overnight:
            available.add("overnight")
    if values is None:
        return tuple(session for session in CANDLE_SESSION_ORDER if session in available)
    seen: set[str] = set()
    for value in values:
        for token in value.split(","):
            normalized = token.strip().lower()
            if normalized not in CANDLE_SESSION_ORDER:
                raise invalid_request("invalid_sessions", f"unsupported candle session: {token}")
            seen.add(normalized)
    if not seen:
        raise invalid_request("invalid_sessions", "at least one candle session is required")
    unsupported = sorted(seen - available)
    if unsupported:
        raise invalid_request("unsupported_sessions", f"candle sessions are unavailable: {', '.join(unsupported)}")
    return tuple(session for session in CANDLE_SESSION_ORDER if session in seen)


@dataclass(frozen=True)
class MarketSpec:
    """The small amount of market metadata needed by all sidecar routes."""

    code: str
    aliases: tuple[str, ...]
    yahoo_suffix: str
    display_name: str
    quote_currency: str
    timezone: str
    supports_extended_hours: bool
    requires_exchange_prefix: bool
    regular_sessions: tuple[tuple[int, int, str], ...]
    price_precision: int
    quote_precision: int
    tick_size: float


MARKET_SPECS: dict[str, MarketSpec] = {
    "US": MarketSpec(
        code="US",
        aliases=("USA", "NYSE", "NASDAQ", "AMEX"),
        yahoo_suffix="",
        display_name="US",
        quote_currency="USD",
        timezone="America/New_York",
        supports_extended_hours=True,
        requires_exchange_prefix=False,
        regular_sessions=((570, 960, "09:30-16:00"),),
        price_precision=2,
        quote_precision=2,
        tick_size=0.01,
    ),
    "HK": MarketSpec(
        code="HK",
        aliases=("HKG", "HKEX"),
        yahoo_suffix=".HK",
        display_name="Hong Kong",
        quote_currency="HKD",
        timezone="Asia/Hong_Kong",
        supports_extended_hours=False,
        requires_exchange_prefix=False,
        regular_sessions=(
            (570, 720, "09:30-12:00"),
            (780, 960, "13:00-16:00"),
        ),
        price_precision=3,
        quote_precision=3,
        tick_size=0.01,
    ),
    "SH": MarketSpec(
        code="SH",
        aliases=("SSE", "SHH", "SHSE", "CNSH"),
        yahoo_suffix=".SS",
        display_name="Shanghai",
        quote_currency="CNY",
        timezone="Asia/Shanghai",
        supports_extended_hours=False,
        requires_exchange_prefix=True,
        regular_sessions=(
            (570, 690, "09:30-11:30"),
            (780, 900, "13:00-15:00"),
        ),
        price_precision=2,
        quote_precision=2,
        tick_size=0.01,
    ),
    "SZ": MarketSpec(
        code="SZ",
        aliases=("SZSE", "SHZ", "SHE", "CNSZ"),
        yahoo_suffix=".SZ",
        display_name="Shenzhen",
        quote_currency="CNY",
        timezone="Asia/Shanghai",
        supports_extended_hours=False,
        requires_exchange_prefix=True,
        regular_sessions=(
            (570, 690, "09:30-11:30"),
            (780, 900, "13:00-15:00"),
        ),
        price_precision=2,
        quote_precision=2,
        tick_size=0.01,
    ),
}

CANONICAL_MARKET = "US"
MARKET_ALIASES = frozenset(
    alias for spec in MARKET_SPECS.values() for alias in (spec.code, *spec.aliases)
)
CN_PREFIXES = frozenset({"SH", "SZ"})
SYMBOL_PATTERN = re.compile(r"^[A-Za-z0-9^][A-Za-z0-9.^=_-]{0,63}$")
HK_SYMBOL_PATTERN = re.compile(r"^\d{1,5}$")
CN_SYMBOL_PATTERN = re.compile(r"^\d{6}$")
CN_QUALIFIED_PATTERN = re.compile(r"^(SH|SZ)\.(\d{6})$")

EXCHANGE_NAMES = {
    "ASE": "AMEX",
    "AMEX": "AMEX",
    "NGM": "NASDAQ",
    "NCM": "NASDAQ",
    "NMS": "NASDAQ",
    "NAS": "NASDAQ",
    "NASDAQ": "NASDAQ",
    "NASDAQGS": "NASDAQ",
    "NASDAQCM": "NASDAQ",
    "NASDAQGM": "NASDAQ",
    "NASDAQ GLOBAL SELECT MARKET": "NASDAQ",
    "NASDAQ GLOBAL MARKET": "NASDAQ",
    "NYQ": "NYSE",
    "NYE": "NYSE",
    "NYSE": "NYSE",
    "PCX": "NYSE ARCA",
    "BTS": "CBOE BZX",
    "HKG": "HKEX",
    "HKEX": "HKEX",
    "HONG KONG STOCK EXCHANGE": "HKEX",
    "SHH": "SSE",
    "SHC": "SSE",
    "SSE": "SSE",
    "SHSE": "SSE",
    "SHANGHAI STOCK EXCHANGE": "SSE",
    "SHZ": "SZSE",
    "SZSE": "SZSE",
    "SHE": "SZSE",
    "SHENZHEN STOCK EXCHANGE": "SZSE",
}

EXCHANGE_MARKETS = {
    exchange: market
    for market, spec in MARKET_SPECS.items()
    for exchange in spec.aliases
}
EXCHANGE_MARKETS.update(
    {
        "ASE": "US",
        "AMEX": "US",
        "NGM": "US",
        "NCM": "US",
        "NMS": "US",
        "NAS": "US",
        "NASDAQ": "US",
        "NASDAQGS": "US",
        "NASDAQCM": "US",
        "NASDAQGM": "US",
        "NASDAQ GLOBAL SELECT MARKET": "US",
        "NASDAQ GLOBAL MARKET": "US",
        "NYQ": "US",
        "NYE": "US",
        "NYSE": "US",
        "PCX": "US",
        "BTS": "US",
        "HONG KONG STOCK EXCHANGE": "HK",
        "SHC": "SH",
        "SHANGHAI STOCK EXCHANGE": "SH",
        "SHENZHEN STOCK EXCHANGE": "SZ",
    }
)

SUPPORTED_SEARCH_EXCHANGES = frozenset(EXCHANGE_MARKETS)
SUPPORTED_QUOTE_TYPES = frozenset(
    {
        "EQUITY",
        "ETF",
        "MUTUALFUND",
        "INDEX",
    }
)
SUPPORTED_PERIODS = ("1m", "5m", "15m", "30m", "1h", "1d", "1w", "1mo")


def action_window(
    from_value: str | None,
    to_value: str | None,
) -> tuple[date, date]:
    """Resolve inclusive corporate-action bounds; defaults to the last 2 years."""
    from_time = parse_rfc3339_utc(from_value, "from")
    to_time = parse_rfc3339_utc(to_value, "to")
    to_date = to_time.date() if to_time is not None else datetime.now(timezone.utc).date()
    if from_time is not None:
        from_date = from_time.date()
    else:
        try:
            from_date = to_date.replace(year=to_date.year - 2)
        except ValueError:
            # February 29 has no counterpart two years earlier.
            from_date = to_date.replace(year=to_date.year - 2, day=28)
    if from_date > to_date:
        raise invalid_request("invalid_time_range", "from must not be after to")
    return from_date, to_date


@dataclass(frozen=True)
class Instrument:
    market: str
    symbol: str
    instrument_id: str
    yahoo_symbol: str

    @property
    def spec(self) -> MarketSpec:
        return MARKET_SPECS[self.market]

    def __iter__(self):
        """Keep the original three-value helper contract unpackable."""
        yield self.market
        yield self.symbol
        yield self.instrument_id


def quote_matches_instrument(values: Mapping[str, Any], instrument: Instrument) -> bool:
    """Reject a Yahoo metadata response for a different ticker.

    The sidecar owns the public JFTrade identity, so a mismatched Yahoo
    response would otherwise be indistinguishable from a valid snapshot or
    security detail response after the route rewrites its identifier.
    """
    returned_symbol = clean_text(values.get("symbol"))
    if returned_symbol is None:
        # Some metadata payloads omit ``symbol``. Exchange/type validation is
        # still useful in that case, while a present ticker must match exactly.
        return True
    expected_symbol = instrument.yahoo_symbol.upper()
    returned_symbol = returned_symbol.upper()
    if instrument.market == "US":
        # Yahoo represents some US share classes with a dash while Futu and
        # callers may use a dot (for example BRK.B versus BRK-B).
        return returned_symbol.replace(".", "-") == expected_symbol.replace(".", "-")
    # Yahoo normally returns four-digit Hong Kong tickers (0700.HK), while
    # some metadata/search responses include the canonical five-digit form
    # (00700.HK). Compare the normalized JFTrade identity in both cases.
    converted = from_yahoo_symbol(instrument.market, returned_symbol)
    return converted == (instrument.symbol, instrument.market)


def market_spec(market: str) -> MarketSpec:
    """Resolve a canonical market or any supported exchange alias."""
    normalized = market.strip().upper()
    if normalized == "CN":
        raise invalid_request(
            "unsupported_market",
            "CN requires a qualified SH.<code> or SZ.<code> symbol",
        )
    try:
        return MARKET_SPECS[normalized]
    except KeyError:
        for spec in MARKET_SPECS.values():
            if normalized in spec.aliases:
                return spec
    raise invalid_request(
        "unsupported_market",
        f"unsupported market: {normalized or market}",
    )


def normalize_instrument(market: str, symbol: str) -> Instrument:
    """Normalize a JFTrade route into a canonical symbol and Yahoo ticker.

    ``CN`` is a UI aggregate rather than a Yahoo route.  It is accepted only
    when the symbol explicitly carries its leaf exchange prefix, e.g.
    ``CN/SH.600519``.
    """
    normalized_market = market.strip().upper()
    normalized_symbol = symbol.strip().upper()
    if normalized_market == "CN":
        qualified = CN_QUALIFIED_PATTERN.fullmatch(normalized_symbol)
        if qualified is None:
            raise invalid_request(
                "invalid_symbol",
                "CN symbols must use SH.<code> or SZ.<code>",
            )
        normalized_market = qualified.group(1)
        normalized_symbol = qualified.group(2)
    spec = market_spec(normalized_market)
    canonical_symbol = _normalize_symbol(spec, normalized_symbol)
    instrument_id = f"{spec.code}.{canonical_symbol}"
    return Instrument(
        market=spec.code,
        symbol=canonical_symbol,
        instrument_id=instrument_id,
        yahoo_symbol=to_yahoo_symbol(spec.code, canonical_symbol),
    )


def to_yahoo_symbol(market: str, symbol: str) -> str:
    """Convert a canonical JFTrade symbol to yfinance's Yahoo ticker."""
    spec = market_spec(market)
    if spec.code == "US":
        return symbol.upper()
    if spec.code == "HK":
        return f"{int(symbol):04d}{spec.yahoo_suffix}"
    return f"{symbol.upper()}{spec.yahoo_suffix}"


def from_yahoo_symbol(market: str, symbol: str) -> tuple[str, str] | None:
    """Return ``(canonical_symbol, Yahoo market)`` for a search quote."""
    spec = MARKET_SPECS.get(market.upper())
    if spec is None:
        return None
    normalized = symbol.strip().upper()
    suffix = spec.yahoo_suffix
    if spec.code == "US":
        if not SYMBOL_PATTERN.fullmatch(normalized) or normalized.endswith(
            (".HK", ".SS", ".SZ")
        ):
            return None
        return normalized, spec.code
    if not normalized.endswith(suffix):
        return None
    base = normalized[: -len(suffix)]
    if spec.code == "HK":
        if not HK_SYMBOL_PATTERN.fullmatch(base):
            return None
        return f"{int(base):05d}", spec.code
    if not CN_SYMBOL_PATTERN.fullmatch(base):
        return None
    return base, spec.code


def market_for_quote(values: Mapping[str, Any]) -> str | None:
    """Infer a supported JFTrade market from Yahoo quote metadata."""
    raw_exchange = clean_text(
        values.get("exchange")
        or values.get("exchangeDisplay")
        or values.get("exchDisp")
        or values.get("fullExchangeName")
    )
    if raw_exchange:
        market = EXCHANGE_MARKETS.get(raw_exchange.upper())
        if market:
            return market
    raw_symbol = (clean_text(values.get("symbol")) or "").upper()
    if raw_symbol.endswith(".HK"):
        return "HK"
    if raw_symbol.endswith(".SS"):
        return "SH"
    if raw_symbol.endswith(".SZ"):
        return "SZ"
    return None


def normalized_exchange(values: Mapping[str, Any]) -> str | None:
    raw = clean_text(
        values.get("exchange")
        or values.get("exchangeDisplay")
        or values.get("exchDisp")
        or values.get("fullExchangeName")
    )
    if raw is None:
        return None
    return EXCHANGE_NAMES.get(raw.upper(), raw.upper())


def quote_is_supported(
    values: Mapping[str, Any],
    expected_market: str | None = None,
) -> bool:
    quote_type = clean_text(values.get("quoteType") or values.get("typeDisp"))
    if quote_type is None or quote_type.upper() not in SUPPORTED_QUOTE_TYPES:
        return False
    market = market_for_quote(values)
    if market is None:
        return False
    if expected_market is not None:
        try:
            expected = market_spec(expected_market).code
        except Exception:
            return False
        if market != expected:
            return False
    return True


def _normalize_symbol(spec: MarketSpec, value: str) -> str:
    symbol = value.strip().upper()
    for prefix in (spec.code, *spec.aliases):
        qualified_prefix = f"{prefix}."
        if symbol.startswith(qualified_prefix):
            symbol = symbol[len(qualified_prefix) :]
            break
    if spec.code == "US":
        # Yahoo search and user input occasionally include an exchange prefix.
        if ":" in symbol:
            prefix, symbol = symbol.rsplit(":", 1)
            if prefix not in spec.aliases and prefix != spec.code:
                raise invalid_request("invalid_symbol", "symbol has an invalid exchange prefix")
        if not SYMBOL_PATTERN.fullmatch(symbol):
            raise invalid_request("invalid_symbol", "symbol has an invalid format")
        return symbol
    if spec.code == "HK":
        if symbol.endswith(spec.yahoo_suffix):
            symbol = symbol[: -len(spec.yahoo_suffix)]
        if not HK_SYMBOL_PATTERN.fullmatch(symbol):
            raise invalid_request("invalid_symbol", "Hong Kong symbols must be 1-5 digits")
        return f"{int(symbol):05d}"
    if symbol.endswith(spec.yahoo_suffix):
        symbol = symbol[: -len(spec.yahoo_suffix)]
    if not CN_SYMBOL_PATTERN.fullmatch(symbol):
        raise invalid_request("invalid_symbol", f"{spec.code} symbols must be six digits")
    return symbol
