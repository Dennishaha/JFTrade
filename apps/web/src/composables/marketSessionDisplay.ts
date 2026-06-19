import type { MarketDataExtendedQuote, MarketDataExtendedQuoteBlocks } from "@/contracts";

type MarketSnapshotLike = {
  price?: number | null;
  previousClosePrice?: number | null;
  lastClosePrice?: number | null;
  session?: string | null;
  extended?: MarketDataExtendedQuoteBlocks | null;
};

type SupportedSession =
  | "regular"
  | "pre"
  | "after"
  | "overnight"
  | "closed"
  | "unknown";

export interface MarketSnapshotDisplayCard {
  key: "pre" | "after";
  label: string;
  price: number;
  changeRate: number | null;
  quoteTime: string | null;
}

export interface MarketSnapshotDisplayModel {
  session: SupportedSession | null;
  sessionLabel: string;
  mainPriceLabel: string;
  mainDisplayPrice: number | null;
  mainChangePercent: number | null;
  extendedCards: MarketSnapshotDisplayCard[];
}

const supportedSessions = new Set<SupportedSession>([
  "regular",
  "pre",
  "after",
  "overnight",
  "closed",
  "unknown",
]);

export function normalizeMarketSession(session: string | null | undefined): SupportedSession | null {
  const normalized = session?.trim().toLowerCase();
  if (normalized == null || normalized === "") {
    return null;
  }
  return supportedSessions.has(normalized as SupportedSession)
    ? (normalized as SupportedSession)
    : null;
}

export function formatMarketSessionLabel(session: string | null | undefined): string {
  const normalized = normalizeMarketSession(session);
  if (normalized === "regular") return "盘中";
  if (normalized === "pre") return "盘前";
  if (normalized === "after") return "盘后";
  if (normalized === "overnight") return "夜盘";
  if (normalized === "closed") return "休市";
  if (normalized === "unknown") return "未知";
  return typeof session === "string" ? session : "";
}

export function resolveMarketSnapshotDisplay(
  snapshot: MarketSnapshotLike | null | undefined,
  supportsExtendedHours: boolean,
): MarketSnapshotDisplayModel {
  const session = normalizeMarketSession(snapshot?.session);
  const mainPriceLabel =
    supportsExtendedHours && session != null && session !== "regular" ? "最近常规收盘" : "最新价";
  const mainDisplayPrice =
    snapshot == null
      ? null
      : supportsExtendedHours && session != null && session !== "regular"
        ? (snapshot.previousClosePrice ?? snapshot.price ?? null)
        : (snapshot.price ?? null);

  return {
    session,
    sessionLabel: formatMarketSessionLabel(session),
    mainPriceLabel,
    mainDisplayPrice,
    mainChangePercent: resolveMainChangePercent(snapshot, supportsExtendedHours, session),
    extendedCards: resolveExtendedCards(snapshot, session),
  };
}

function resolveMainChangePercent(
  snapshot: MarketSnapshotLike | null | undefined,
  supportsExtendedHours: boolean,
  session: SupportedSession | null,
): number | null {
  if (snapshot == null) {
    return null;
  }
  if (!supportsExtendedHours || session == null || session === "regular") {
    return percentChange(snapshot.price, snapshot.previousClosePrice);
  }
  const close = snapshot.previousClosePrice;
  const lastClose = snapshot.lastClosePrice;
  if (close == null || lastClose == null || lastClose === 0 || close === lastClose) {
    return null;
  }
  return ((close - lastClose) / lastClose) * 100;
}

function resolveExtendedCards(
  snapshot: MarketSnapshotLike | null | undefined,
  session: SupportedSession | null,
): MarketSnapshotDisplayCard[] {
  const extended = snapshot?.extended;
  if (extended == null) {
    return [];
  }
  const livePrice = positiveNumber(snapshot?.price) ? snapshot?.price ?? null : null;
  const liveChangeRate = percentChange(snapshot?.price, snapshot?.previousClosePrice);
  const cards: MarketSnapshotDisplayCard[] = [];

  if (session === "pre" && positiveNumber(extended.preMarket?.price)) {
    cards.push({
      key: "pre",
      label: "盘前价格",
      price: livePrice ?? (extended.preMarket?.price as number),
      changeRate: liveChangeRate ?? normalizeNullableNumber(extended.preMarket?.changeRate),
      quoteTime: normalizeQuoteTime(extended.preMarket),
    });
  }

  const afterMarketPrice = positiveNumber(extended.afterMarket?.price)
    ? (extended.afterMarket?.price as number)
    : null;
  if (afterMarketPrice != null) {
    if (session === "after") {
      cards.push({
        key: "after",
        label: "盘后价格",
        price: livePrice ?? afterMarketPrice,
        changeRate: liveChangeRate ?? normalizeNullableNumber(extended.afterMarket?.changeRate),
        quoteTime: normalizeQuoteTime(extended.afterMarket),
      });
    } else if (
      (session === "closed" || session === "overnight") &&
      normalizeQuoteTime(extended.afterMarket) != null
    ) {
      cards.push({
        key: "after",
        label: "最近盘后价格",
        price: afterMarketPrice,
        changeRate: normalizeNullableNumber(extended.afterMarket?.changeRate),
        quoteTime: normalizeQuoteTime(extended.afterMarket),
      });
    }
  }

  return cards;
}

function percentChange(
  value: number | null | undefined,
  reference: number | null | undefined,
): number | null {
  if (value == null || reference == null || reference === 0) {
    return null;
  }
  return ((value - reference) / reference) * 100;
}

function positiveNumber(value: number | null | undefined): boolean {
  return typeof value === "number" && Number.isFinite(value) && value > 0;
}

function normalizeNullableNumber(value: number | null | undefined): number | null {
  return typeof value === "number" && Number.isFinite(value) ? value : null;
}

function normalizeQuoteTime(quote: MarketDataExtendedQuote | null | undefined): string | null {
  if (quote == null || typeof quote.quoteTime !== "string") {
    return null;
  }
  const trimmed = quote.quoteTime.trim();
  return trimmed === "" ? null : trimmed;
}
