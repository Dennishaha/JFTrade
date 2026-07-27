import type { components } from "@/generated/openapi";

export type MarketDataProviderCapabilities =
  components["schemas"]["marketdata.ProviderCapabilities"];

export type MarketDataProviderConstraints =
  components["schemas"]["marketdata.ProviderConstraints"];

export type MarketDataProviderDescriptor =
  components["schemas"]["marketdata.ProviderDescriptor"];

export type MarketDataProviderHealth =
  components["schemas"]["marketdata.HealthStatus"];

export type MarketDataSubscriptionQuotaBucketDto =
  components["schemas"]["marketdata.SubscriptionQuotaBucketData"];
