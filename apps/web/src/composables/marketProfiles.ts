import { computed, ref } from "vue";

import type {
  MarketProfileDto,
  MarketProfilesResponse,
  NormalizeInstrumentRequest,
  NormalizeInstrumentResponse,
} from "@/contracts";

import { fetchEnvelope, fetchEnvelopeWithInit } from "./apiClient";
import { formatMarketLabel } from "./consoleDataFormatting";

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

function marketOptionTitle(profile: MarketProfileDto): string {
  const code = normalizeMarketCode(profile.code);
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
    marketProfiles.value.map((profile) => ({
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
    supportsExtendedHoursForMarket,
    normalizeInstrumentRefWithMarketApi,
  };
}
