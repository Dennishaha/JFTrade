import { computed, ref } from "vue";

import type {
  MarketProfileDto,
  MarketProfilesResponse,
  NormalizeInstrumentRequest,
  NormalizeInstrumentResponse,
} from "@/contracts";

import { fetchEnvelope, fetchEnvelopeWithInit } from "./apiClient";
import { formatMarketLabel } from "./consoleDataFormatting";
import { formatUserMarketLabel } from "./instrumentPresentation";
import { marketPricePrecision } from "../utils/numberFormat";

const marketProfiles = ref<MarketProfileDto[]>([]);
const defaultMarket = ref("HK");
const isLoadingMarketProfiles = ref(false);
const marketProfilesError = ref("");
let marketProfilesLoadPromise: Promise<void> | null = null;

function normalizeMarketCode(value: string | null | undefined): string {
  return (value ?? "").trim().toUpperCase();
}

function profileMatchesMarket(profile: MarketProfileDto, market: string): boolean {
  const normalized = normalizeMarketCode(market);
  if (normalized === "") {
    return false;
  }
  return [
    profile.code,
    profile.resolvedMarket,
    profile.preferredPrefix,
    ...(profile.aliases ?? []),
  ]
    .map(normalizeMarketCode)
    .some((candidate) => candidate === normalized);
}

function selectableProfiles(profiles: MarketProfileDto[]): MarketProfileDto[] {
  const result: MarketProfileDto[] = [];
  const indexByCode = new Map<string, number>();

  for (const profile of profiles) {
    const code = normalizeMarketCode(profile.code);
    const resolvedMarket =
      normalizeMarketCode(profile.resolvedMarket) || code;
    if (code === "" || resolvedMarket === "") {
      continue;
    }

    if (code === resolvedMarket) {
      const existingIndex = indexByCode.get(code);
      if (existingIndex == null) {
        indexByCode.set(code, result.length);
        result.push(profile);
      } else {
        const synthesized = result[existingIndex];
        result[existingIndex] = {
          ...profile,
          aliases: Array.from(
            new Set([
              ...(profile.aliases ?? []),
              ...(synthesized?.aliases ?? []),
            ]),
          ),
        };
      }
      continue;
    }

    const existingIndex = indexByCode.get(resolvedMarket);
    const childAliases = [code, ...(profile.aliases ?? [])];
    if (existingIndex != null) {
      const existing = result[existingIndex];
      if (existing != null) {
        result[existingIndex] = {
          ...existing,
          aliases: Array.from(
            new Set([...(existing.aliases ?? []), ...childAliases]),
          ),
        };
      }
      continue;
    }

    indexByCode.set(resolvedMarket, result.length);
    result.push({
      ...profile,
      code: resolvedMarket,
      resolvedMarket,
      preferredPrefix: "",
      displayName: resolvedMarket === "CN" ? "沪深" : profile.displayName,
      aliases: Array.from(new Set(childAliases)),
    });
  }

  return result;
}

function marketOptionTitle(profile: MarketProfileDto): string {
  const code = normalizeMarketCode(profile.code);
  if (code === "CN") {
    return formatUserMarketLabel(code);
  }
  const localized = formatMarketLabel(code);
  if (localized !== "" && localized !== code) {
    return `${localized} ${code}`;
  }
  const displayName = profile.displayName.trim();
  if (displayName === "" || normalizeMarketCode(displayName) === code) {
    return code;
  }
  return `${displayName} ${code}`;
}

export async function loadMarketProfiles(): Promise<void> {
  if (marketProfilesLoadPromise != null) {
    return marketProfilesLoadPromise;
  }
  isLoadingMarketProfiles.value = true;
  marketProfilesError.value = "";
  marketProfilesLoadPromise = (async () => {
    try {
      const response = await fetchEnvelope<MarketProfilesResponse>(
        "/api/v1/market-data/markets",
      );
      if (Array.isArray(response.markets) && response.markets.length > 0) {
        marketProfiles.value = response.markets;
      }
      defaultMarket.value = normalizeMarketCode(response.defaultMarket) || "HK";
    } catch (cause) {
      marketProfiles.value = [];
      defaultMarket.value = "HK";
      marketProfilesError.value =
        cause instanceof Error ? cause.message : String(cause);
    } finally {
      isLoadingMarketProfiles.value = false;
      marketProfilesLoadPromise = null;
    }
  })();
  return marketProfilesLoadPromise;
}

export function findMarketProfile(
  market: string | null | undefined,
): MarketProfileDto | null {
  const normalized = normalizeMarketCode(market);
  return (
    marketProfiles.value.find((profile) => profileMatchesMarket(profile, normalized)) ??
    null
  );
}

export function quoteCurrencyForMarket(
  market: string | null | undefined,
): string {
  return findMarketProfile(market)?.quoteCurrency ?? "HKD";
}

export function pricePrecisionForMarket(
  market: string | null | undefined,
): number | null {
  const precision = findMarketProfile(market)?.precision?.price;
  return typeof precision === "number" && Number.isFinite(precision)
    ? precision
    : marketPricePrecision(market);
}

export function supportsExtendedHoursForMarket(
  market: string | null | undefined,
): boolean {
  return findMarketProfile(market)?.supportsExtendedHours === true;
}

export async function normalizeInstrumentRefWithMarketApi(
  input: NormalizeInstrumentRequest,
): Promise<NormalizeInstrumentResponse> {
  return fetchEnvelopeWithInit<NormalizeInstrumentResponse>(
    "/api/v1/market-data/instruments/normalize",
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input),
    },
  );
}

export function useMarketProfiles() {
  const marketOptions = computed(() =>
    selectableProfiles(marketProfiles.value).map((profile) => ({
      value: profile.code,
      title: marketOptionTitle(profile),
    })),
  );

  return {
    marketProfiles,
    defaultMarket,
    isLoadingMarketProfiles,
    marketProfilesError,
    marketOptions,
    loadMarketProfiles,
    findMarketProfile,
    quoteCurrencyForMarket,
    pricePrecisionForMarket,
    supportsExtendedHoursForMarket,
    normalizeInstrumentRefWithMarketApi,
  };
}
