import type { BrokerFeatureCapabilityDto } from "@/contracts";
import type { BrokerCapabilityDescriptor } from "./brokerProviderModels";

export type BrokerCandleSession = "regular" | "extended" | "overnight";
export interface BrokerCandleSessionCapability {
  id: BrokerCandleSession;
  supportedPeriods: string[];
}

export function mapSupportedCandleSessions(
  value: BrokerFeatureCapabilityDto["supportedSessions"],
): BrokerCandleSessionCapability[] | undefined {
  return value
    ?.filter((session): session is typeof session & { id: BrokerCandleSession } =>
      ["regular", "extended", "overnight"].includes(session.id),
    )
    .map((session) => ({
      id: session.id,
      supportedPeriods: [...session.supportedPeriods],
    }));
}

function normalizedID(value: string | null | undefined): string {
  return value?.trim().toLowerCase() ?? "";
}

export function brokerSupportedChartSessions(
  brokerId: string,
  market: string,
  period = "",
  descriptors: readonly BrokerCapabilityDescriptor[] = [],
): BrokerCandleSession[] | null {
  const normalizedBroker = normalizedID(brokerId);
  const normalizedMarket = market.trim().toUpperCase();
  const normalizedPeriod = period.trim().toLowerCase();
  if (normalizedBroker === "akshare") return ["regular"];
  if (normalizedBroker === "yfinance") {
    const intraday = ["tick", "1m", "5m", "15m", "30m", "1h"].includes(normalizedPeriod);
    return normalizedMarket === "US" && intraday ? ["regular", "extended"] : ["regular"];
  }
  const descriptor = normalizedBroker
    ? descriptors.find((candidate) => normalizedID(candidate.id) === normalizedBroker)
    : descriptors.length === 1
      ? descriptors[0]
      : undefined;
  if (descriptor == null) return null;
  const capability = (descriptor.capabilities ?? []).find(
    (candidate) => candidate.market.trim().toUpperCase() === normalizedMarket,
  );
  if (capability == null) return [];
  const feature = (capability.features ?? []).find(
    (candidate) => candidate.id === "market.candles" &&
      (candidate.state === "available" || candidate.state === "degraded"),
  );
  if (feature == null) return [];
  const supported = new Set<BrokerCandleSession>();
  for (const session of feature.supportedSessions ?? []) {
    if (normalizedPeriod === "" || session.supportedPeriods.map((value) => value.toLowerCase()).includes(normalizedPeriod)) {
      supported.add(session.id);
    }
  }
  return (["regular", "extended", "overnight"] as const).filter((session) => supported.has(session));
}
