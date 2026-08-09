import type { BrokerCandleSessionCapability } from "./brokerCandleSessions";

export type BrokerCapabilityState = "available" | "degraded" | "unavailable";

/**
 * Presentation state intentionally differs from capability state. A provider
 * may advertise a degraded capability while still operating normally for the
 * feature currently being viewed.
 */
export type BrokerProviderDisplayState = BrokerCapabilityState;
export type BrokerProviderDisplayTone = "success" | "warning" | "error";

export interface BrokerCapabilityPresentation {
  displayState: BrokerProviderDisplayState;
  tone: BrokerProviderDisplayTone;
}

export interface BrokerFeatureCapability {
  id: string;
  markets?: string[];
  supportedPeriods?: string[];
  supportedSessions?: BrokerCandleSessionCapability[];
  state: BrokerCapabilityState;
  reasonCode?: string;
  reason?: string;
}

export interface BrokerMarketCapability {
  market: string;
  supportsQuote: boolean;
  supportsTrade: boolean;
  features?: BrokerFeatureCapability[];
}

export interface BrokerCapabilityDescriptor {
  id: string;
  displayName: string;
  securityFirm?: string;
  capabilityVersion?: string;
  capabilities?: BrokerMarketCapability[];
}

export interface BrokerRuntimeCapabilityEvaluation {
  state: BrokerCapabilityState;
  code?: string;
  reason?: string;
  checkedAt?: string;
}

export interface BrokerRuntimeCapabilityStatus {
  brokerId: string;
  securityFirm?: string;
  market: string;
  featureId: string;
  capability: BrokerFeatureCapability;
  evaluation?: BrokerRuntimeCapabilityEvaluation;
}

export type BrokerFeatureSelector = string | readonly string[];

export interface BrokerCapabilitySummary {
  state: BrokerCapabilityState;
  reason: string;
}

export interface BrokerProviderOption {
  id: string;
  label: string;
  shortLabel: string;
  securityFirm: string;
  state: BrokerCapabilityState;
  reason: string;
  /** UI-only state; `state` remains the raw capability state for selection. */
  displayState?: BrokerProviderDisplayState;
  /** UI-only semantic color, independent of capability selection semantics. */
  tone?: BrokerProviderDisplayTone;
  /** Static capability gate; runtime health may additionally disable selection. */
  selectable: boolean;
}

export type BrokerProviderNameInput =
  | Pick<BrokerCapabilityDescriptor, "id" | "displayName">
  | string
  | null
  | undefined;
