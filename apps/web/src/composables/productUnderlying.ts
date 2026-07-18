import type { MarketSecurityDetails, MarketSecurityRef } from "../contracts";
import type { WorkspaceProductClass } from "./workspaceProductTabs";

export interface ProductUnderlyingResolution {
  instrumentId: string;
  source: "current_instrument" | "option_owner" | "warrant_owner" | "unresolved";
  pending: boolean;
}

function instrumentIdFromRef(owner: MarketSecurityRef | null | undefined): string {
  const direct = String(owner?.instrumentId ?? "")
    .trim()
    .toUpperCase();
  if (direct) return direct;
  const market = String(owner?.market ?? "")
    .trim()
    .toUpperCase();
  const symbol = String(owner?.symbol ?? "")
    .trim()
    .toUpperCase();
  return market && symbol ? `${market}.${symbol}` : "";
}

export function resolveProductUnderlying(
  currentInstrumentId: string,
  productClass: WorkspaceProductClass,
  details: MarketSecurityDetails | null | undefined,
  productIdentityPending: boolean,
): ProductUnderlyingResolution {
  const current = currentInstrumentId.trim().toUpperCase();
  if (productClass === "unknown" && productIdentityPending) {
    return {
      instrumentId: "",
      source: "unresolved",
      pending: true,
    };
  }
  if (productClass === "option") {
    const instrumentId = instrumentIdFromRef(details?.option?.owner);
    return {
      instrumentId,
      source: instrumentId ? "option_owner" : "unresolved",
      pending: productIdentityPending || details == null,
    };
  }
  if (productClass === "warrant" || productClass === "cbbc") {
    const instrumentId = instrumentIdFromRef(details?.warrant?.owner);
    return {
      instrumentId,
      source: instrumentId ? "warrant_owner" : "unresolved",
      pending: productIdentityPending || details == null,
    };
  }
  return {
    instrumentId: current,
    source: current ? "current_instrument" : "unresolved",
    pending: productIdentityPending,
  };
}
