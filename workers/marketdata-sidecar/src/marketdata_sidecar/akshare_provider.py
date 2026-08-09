"""AKShare provider facade: catalog, search, quotes, and candles.

Routing and transport code should import this module so the internal
catalog/search/snapshot decomposition stays an implementation detail.
"""

from __future__ import annotations

from .akshare_candles import candles, validate_candle_query, validate_candle_retention
from .akshare_catalog import (
    ALL_PERIODS,
    CATALOG_CACHE_SECONDS,
    CATALOG_FAILURE_CACHE_SECONDS,
    CN_INDEX_SERIES,
    INDEX_PERIODS,
    INTRADAY_PERIODS,
    US_FAMOUS_CATEGORIES,
    AKInstrument,
    _TTLCache,
    _catalog_cache,
    _spot_identity,
    catalog,
    snapshot_catalog,
)
from .akshare_identity import (
    CODE_PATTERN,
    HK_INDEX_IDS,
    MARKET_CURRENCY,
    MARKET_EXCHANGE,
    US_INDEX_IDS,
    normalize_identity,
)
from .akshare_quotes import security, snapshot
from .akshare_search import (
    resolve_from_catalog,
    resolve_instrument,
    search,
)

__all__ = [
    "ALL_PERIODS",
    "CATALOG_CACHE_SECONDS",
    "CATALOG_FAILURE_CACHE_SECONDS",
    "CN_INDEX_SERIES",
    "INDEX_PERIODS",
    "INTRADAY_PERIODS",
    "US_FAMOUS_CATEGORIES",
    "AKInstrument",
    "CODE_PATTERN",
    "HK_INDEX_IDS",
    "MARKET_CURRENCY",
    "MARKET_EXCHANGE",
    "US_INDEX_IDS",
    "_TTLCache",
    "_catalog_cache",
    "_spot_identity",
    "catalog",
    "snapshot_catalog",
    "search",
    "resolve_instrument",
    "resolve_from_catalog",
    "normalize_identity",
    "security",
    "snapshot",
    "candles",
    "validate_candle_query",
    "validate_candle_retention",
]
