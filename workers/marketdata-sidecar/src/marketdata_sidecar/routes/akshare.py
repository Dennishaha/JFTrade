"""Namespaced AKShare HTTP routes."""

from __future__ import annotations

from importlib.metadata import PackageNotFoundError, version

from fastapi import APIRouter, Query
from fastapi.responses import JSONResponse

from .. import akshare_provider, akshare_upstream
from ..akshare_models import (
    AKBatchError,
    AKBatchRequest,
    AKBatchResponse,
    AKCandlesResponse,
    AKSearchResponse,
    AKSecurityResponse,
    AKSnapshotResponse,
)
from ..errors import SidecarError
from ..models import (
    MarketPrecision,
    MarketProfile,
    MarketsResponse,
    ProviderHealthResponse,
    TradingWindow,
)
from ..readiness import provider_health_response
from .common import MARKET_SPECS, parse_candle_sessions
from ..conversion import parse_rfc3339_utc

router = APIRouter(prefix="/providers/akshare")


@router.get("/health", response_model=ProviderHealthResponse)
def health() -> ProviderHealthResponse | JSONResponse:
    try:
        provider_version = version("akshare")
    except PackageNotFoundError:
        provider_version = "1.18.81"
    return provider_health_response(
        "akshare",
        akshare_upstream,
        lambda runtime: ProviderHealthResponse(
            ok=True,
            provider="akshare",
            provider_version=provider_version,
            runtime_state=runtime.state,
            warmup_error=runtime.error or None,
        ),
    )


@router.get("/markets", response_model=MarketsResponse)
def markets() -> MarketsResponse:
    return MarketsResponse(markets=[_profile(code) for code in ("US", "HK", "SH", "SZ")])


@router.get("/search", response_model=AKSearchResponse)
def search(
    q: str = Query(min_length=1, max_length=100),
    limit: int = Query(default=20, ge=1, le=100),
) -> AKSearchResponse:
    return AKSearchResponse(entries=_translate("search", akshare_provider.search, q.strip(), limit))


@router.get("/security/{market}/{symbol:path}", response_model=AKSecurityResponse)
def security(market: str, symbol: str) -> AKSecurityResponse:
    return _translate(
        "security lookup",
        _security,
        market,
        symbol,
    )


@router.get("/snapshot/{market}/{symbol:path}", response_model=AKSnapshotResponse)
def snapshot(market: str, symbol: str) -> AKSnapshotResponse:
    return _translate(
        "snapshot lookup",
        _snapshot,
        market,
        symbol,
    )


@router.post("/snapshots", response_model=AKBatchResponse)
def snapshots(request: AKBatchRequest) -> AKBatchResponse:
    return _translate("batch snapshot lookup", _snapshots, request)


def _snapshots(request: AKBatchRequest) -> AKBatchResponse:
    entries: list[AKSnapshotResponse] = []
    errors: list[AKBatchError] = []
    parsed: list[tuple[str, str, str]] = []
    for instrument_id in request.instrument_ids:
        try:
            market, symbol = _instrument_id_parts(instrument_id)
            normalized_market, _normalized_symbol = akshare_provider.normalize_identity(
                market,
                symbol,
            )
            parsed.append((instrument_id, normalized_market, symbol))
        except SidecarError as exc:
            errors.append(_batch_error(instrument_id, exc))

    catalogs: dict[str, list[akshare_provider.AKInstrument]] = {}
    failed_markets: dict[str, SidecarError] = {}
    symbols_by_market: dict[str, list[str]] = {}
    for _instrument_id, market, symbol in parsed:
        symbols_by_market.setdefault(market, []).append(symbol)
    for market, symbols in symbols_by_market.items():
        try:
            catalogs[market] = akshare_provider.snapshot_catalog(market, symbols)
        except SidecarError as exc:
            failed_markets[market] = exc
        except Exception:
            failure = SidecarError(
                502,
                "AKSHARE_UPSTREAM_ERROR",
                "AKShare batch snapshot lookup failed",
            )
            failed_markets[market] = failure

    for instrument_id, market, symbol in parsed:
        prior = failed_markets.get(market)
        if prior is not None:
            errors.append(_batch_error(instrument_id, prior))
            continue
        try:
            instrument = akshare_provider.resolve_from_catalog(
                market,
                symbol,
                catalogs[market],
            )
            entries.append(akshare_provider.snapshot(instrument))
        except SidecarError as exc:
            errors.append(_batch_error(instrument_id, exc))
    return AKBatchResponse(entries=entries, errors=errors)


@router.get("/candles/{market}/{symbol:path}", response_model=AKCandlesResponse)
def candles(
    market: str,
    symbol: str,
    period: str = Query(default="1d"),
    limit: int = Query(default=200, ge=1, le=1000),
    from_value: str | None = Query(default=None, alias="from"),
    to_value: str | None = Query(default=None, alias="to"),
    before_value: str | None = Query(default=None, alias="before"),
    sessions: list[str] | None = Query(default=None),
) -> AKCandlesResponse:
    from_time = parse_rfc3339_utc(from_value, "from")
    to_time = parse_rfc3339_utc(to_value, "to")
    before_time = parse_rfc3339_utc(before_value, "before")
    if before_time is not None and (from_time is not None or to_time is not None):
        raise SidecarError(400, "invalid_time_range", "before cannot be combined with from or to")
    return _translate(
        "candle lookup",
        _candles,
        market,
        symbol,
        period=period,
        limit=limit,
        from_time=from_time,
        to_time=to_time,
        before_time=before_time,
        sessions=sessions,
    )


def _translate(operation: str, function, *args, **kwargs):
    try:
        return akshare_upstream.run(function, *args, **kwargs)
    except SidecarError:
        raise
    except Exception as exc:
        raise SidecarError(
            502,
            "AKSHARE_UPSTREAM_ERROR",
            f"AKShare {operation} failed",
        ) from exc


def _security(market: str, symbol: str) -> AKSecurityResponse:
    return akshare_provider.security(
        akshare_provider.resolve_instrument(market, symbol)
    )


def _snapshot(market: str, symbol: str) -> AKSnapshotResponse:
    return akshare_provider.snapshot(
        akshare_provider.resolve_instrument(market, symbol)
    )


def _candles(
    market: str,
    symbol: str,
    *,
    period: str,
    limit: int,
    from_time,
    to_time,
    before_time,
    sessions,
) -> AKCandlesResponse:
    selected_sessions = parse_candle_sessions(
        sessions,
        market=akshare_provider.normalize_identity(market, symbol)[0],
        period=period.strip().lower(),
        extended=False,
    )
    if selected_sessions != ("regular",):
        raise SidecarError(400, "unsupported_sessions", "AKShare only provides regular-session candles")
    akshare_provider.validate_candle_query(period, from_time, to_time)
    normalized_market, _normalized_symbol = akshare_provider.normalize_identity(
        market,
        symbol,
    )
    akshare_provider.validate_candle_retention(
        normalized_market,
        period,
        from_time,
        to_time,
    )
    return akshare_provider.candles(
        akshare_provider.resolve_instrument(market, symbol),
        period=period,
        limit=limit,
        from_time=from_time,
        to_time=to_time,
        before_time=before_time,
    )


def _instrument_id_parts(instrument_id: str) -> tuple[str, str]:
    market, separator, symbol = instrument_id.strip().upper().partition(".")
    if not separator or not market or not symbol:
        raise SidecarError(
            400,
            "invalid_instrument_id",
            f"invalid instrument id: {instrument_id}",
        )
    return market, symbol


def _batch_error(instrument_id: str, error: SidecarError) -> AKBatchError:
    return AKBatchError(
        instrument_id=instrument_id,
        code=error.code,
        message=error.message,
    )


def _profile(code: str) -> MarketProfile:
    spec = MARKET_SPECS[code]
    return MarketProfile(
        code=code,
        resolved_market="CN" if code in {"SH", "SZ"} else code,
        preferred_prefix=code,
        display_name=spec.display_name,
        quote_currency=spec.quote_currency,
        timezone=spec.timezone,
        supports_extended_hours=False,
        requires_exchange_prefix=spec.requires_exchange_prefix,
        aliases=list(spec.aliases),
        regular_sessions=[
            TradingWindow(start_minute=start, end_minute=end, label=label)
            for start, end, label in spec.regular_sessions
        ],
        precision=MarketPrecision(price=spec.price_precision, quote=spec.quote_precision),
        tick_size=spec.tick_size,
    )
