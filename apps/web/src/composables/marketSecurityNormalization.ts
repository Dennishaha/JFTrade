import type {
  MarketDataExtendedQuote,
  MarketDataExtendedQuoteBlocks,
  MarketSecurityDetails,
  MarketSecurityDetailsQueryResult,
  MarketSecurityEquityDetails,
  MarketSecurityFutureDetails,
  MarketSecurityIndexDetails,
  MarketSecurityOptionDetails,
  MarketSecurityPlateDetails,
  MarketSecurityRef,
  MarketSecurityTrustDetails,
  MarketSecurityWarrantDetails,
} from "@/contracts";

const extendedQuoteNumberKeys = [
  "price",
  "highPrice",
  "lowPrice",
  "volume",
  "turnover",
  "changeVal",
  "changeRate",
  "amplitude",
] as const;

const securityNumberKeys = [
  "securityId",
  "listTimestamp",
  "lotSize",
  "priceSpread",
  "updateTimestamp",
  "highPrice",
  "openPrice",
  "lowPrice",
  "lastClosePrice",
  "currentPrice",
  "volume",
  "turnover",
  "turnoverRate",
  "askPrice",
  "bidPrice",
  "askVolume",
  "bidVolume",
  "amplitude",
  "averagePrice",
  "bidAskRatio",
  "volumeRatio",
  "highest52WeeksPrice",
  "lowest52WeeksPrice",
  "highestHistoryPrice",
  "lowestHistoryPrice",
  "closePrice5Minute",
  "highPrecisionVolume",
  "highPrecisionAskVol",
  "highPrecisionBidVol",
] as const;

const equityNumberKeys = [
  "issuedShares",
  "issuedMarketValue",
  "netAsset",
  "netProfit",
  "earningsPerShare",
  "outstandingShares",
  "outstandingMarketVal",
  "netAssetPerShare",
  "earningsYieldRate",
  "peRate",
  "pbRate",
  "peTTMRate",
  "dividendTTM",
  "dividendRatioTTM",
  "dividendLFY",
  "dividendLFYRatio",
] as const;

const warrantNumberKeys = [
  "conversionRate",
  "strikePrice",
  "recoveryPrice",
  "streetVolume",
  "issueVolume",
  "streetRate",
  "delta",
  "impliedVolatility",
  "premium",
  "maturityTimestamp",
  "endTradeTimestamp",
  "leverage",
  "inOutPriceRatio",
  "breakEvenPoint",
  "conversionPrice",
  "priceRecoveryRatio",
  "score",
  "upperStrikePrice",
  "lowerStrikePrice",
] as const;

const optionNumberKeys = [
  "strikePrice",
  "contractSize",
  "contractSizeFloat",
  "openInterest",
  "impliedVolatility",
  "premium",
  "delta",
  "gamma",
  "vega",
  "theta",
  "rho",
  "strikeTimestamp",
  "netOpenInterest",
  "expiryDateDistance",
  "contractNominalValue",
  "ownerLotMultiplier",
  "contractMultiplier",
] as const;

const indexNumberKeys = ["raiseCount", "fallCount", "equalCount"] as const;
const futureNumberKeys = [
  "lastSettlePrice",
  "position",
  "positionChange",
  "lastTradeTimestamp",
] as const;
const trustNumberKeys = [
  "dividendYield",
  "aum",
  "outstandingUnit",
  "netAssetValue",
  "premium",
] as const;

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function normalizeNumberish(value: unknown): number | undefined {
  if (typeof value === "number") {
    return Number.isFinite(value) ? value : undefined;
  }
  if (typeof value !== "string") {
    return undefined;
  }
  const trimmed = value.trim();
  if (trimmed === "") {
    return undefined;
  }
  const parsed = Number(trimmed);
  return Number.isFinite(parsed) ? parsed : undefined;
}

function normalizeFields<T extends Record<string, unknown> | null | undefined>(
  value: T,
  keys: readonly string[],
): T {
  if (!isRecord(value)) {
    return value;
  }
  const normalized: Record<string, unknown> = { ...value };
  for (const key of keys) {
    if (!(key in normalized)) {
      continue;
    }
    const current = normalized[key];
    if (current == null) {
      continue;
    }
    const parsed = normalizeNumberish(current);
    if (parsed !== undefined) {
      normalized[key] = parsed;
    }
  }
  return normalized as T;
}

function normalizeSecurityRef(value: unknown): MarketSecurityRef | null {
  if (!isRecord(value)) {
    return null;
  }
  return {
    instrumentId: typeof value.instrumentId === "string" ? value.instrumentId : "",
    market: typeof value.market === "string" ? value.market : "",
    symbol: typeof value.symbol === "string" ? value.symbol : "",
  };
}

function normalizeExtendedQuote(value: unknown): MarketDataExtendedQuote | null {
  return normalizeFields(
    isRecord(value) ? value : null,
    extendedQuoteNumberKeys,
  ) as MarketDataExtendedQuote | null;
}

function normalizeExtendedQuoteBlocks(
  value: unknown,
): MarketDataExtendedQuoteBlocks | null {
  if (!isRecord(value)) {
    return null;
  }
  return {
    ...value,
    preMarket: normalizeExtendedQuote(value.preMarket),
    afterMarket: normalizeExtendedQuote(value.afterMarket),
    overnight: normalizeExtendedQuote(value.overnight),
  } as MarketDataExtendedQuoteBlocks;
}

function normalizeSecurityDetails(value: unknown): MarketSecurityDetails | null {
  const security = normalizeFields(
    isRecord(value) ? value : null,
    securityNumberKeys,
  );
  if (!isRecord(security)) {
    return null;
  }

  const warrant = normalizeFields(
    isRecord(security.warrant) ? security.warrant : null,
    warrantNumberKeys,
  ) as MarketSecurityWarrantDetails | null;
  const option = normalizeFields(
    isRecord(security.option) ? security.option : null,
    optionNumberKeys,
  ) as MarketSecurityOptionDetails | null;

  return {
    ...security,
    securityType:
      typeof security.securityType === "string" ? security.securityType : "",
    extended: normalizeExtendedQuoteBlocks(security.extended),
    equity: normalizeFields(
      isRecord(security.equity) ? security.equity : null,
      equityNumberKeys,
    ) as MarketSecurityEquityDetails | null,
    warrant:
      warrant == null
        ? null
        : {
            ...warrant,
            owner: normalizeSecurityRef(warrant.owner),
          },
    option:
      option == null
        ? null
        : {
            ...option,
            owner: normalizeSecurityRef(option.owner),
          },
    index: normalizeFields(
      isRecord(security.index) ? security.index : null,
      indexNumberKeys,
    ) as MarketSecurityIndexDetails | null,
    plate: normalizeFields(
      isRecord(security.plate) ? security.plate : null,
      indexNumberKeys,
    ) as MarketSecurityPlateDetails | null,
    future: normalizeFields(
      isRecord(security.future) ? security.future : null,
      futureNumberKeys,
    ) as MarketSecurityFutureDetails | null,
    trust: normalizeFields(
      isRecord(security.trust) ? security.trust : null,
      trustNumberKeys,
    ) as MarketSecurityTrustDetails | null,
  } as MarketSecurityDetails;
}

export function normalizeMarketSecurityDetailsQueryResult(
  result: MarketSecurityDetailsQueryResult,
): MarketSecurityDetailsQueryResult {
  return {
    ...result,
    security: normalizeSecurityDetails(result.security),
  };
}
