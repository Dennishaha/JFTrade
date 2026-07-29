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

export type MarketDataCandlePaginationDto =
  components["schemas"]["marketdata.CandlePaginationData"];

export type MarketDataCandleRequestDto =
  components["schemas"]["marketdata.CandleRequestData"];

export type MarketDataCandlesDto =
  components["schemas"]["marketdata.CandlesData"];

export type MarketDataDepthDto =
  components["schemas"]["marketdata.DepthData"];

export type MarketDataDepthRequestDto =
  components["schemas"]["marketdata.DepthRequestData"];

export type MarketDataInstrumentCandidateDto =
  components["schemas"]["marketdata.InstrumentCandidate"];

export type MarketDataInstrumentResolutionDto =
  components["schemas"]["marketdata.InstrumentResolution"];

export type MarketDataInstrumentResolutionFailureDto =
  components["schemas"]["marketdata.InstrumentResolutionFailure"];

export type MarketDataInstrumentResolutionStatus =
  components["schemas"]["marketdata.InstrumentResolutionStatus"];

export type MarketDataInstrumentRefDto =
  components["schemas"]["marketdata.InstrumentRef"];

export type MarketDataInstrumentDto =
  components["schemas"]["marketdata.MarketInstrumentData"];

export type MarketDataQueryMeta =
  components["schemas"]["marketdata.MarketQueryMeta"];

export type MarketDataMarketsDto =
  components["schemas"]["marketdata.MarketsData"];

export type MarketDataProviderStatusDto =
  components["schemas"]["marketdata.ProviderStatusResponse"];

export type MarketDataRuntimeStateDto =
  components["schemas"]["marketdata.RuntimeState"];

export type MarketDataSecurityDetailsDto =
  components["schemas"]["marketdata.SecurityDetailsData"];

export type MarketDataSnapshotDto =
  components["schemas"]["marketdata.SnapshotData"];

export type MarketDataSubscriptionEntryDto =
  components["schemas"]["marketdata.SubscriptionEntryData"];

export type MarketDataSubscriptionQuotaDto =
  components["schemas"]["marketdata.SubscriptionQuotaData"];

export type MarketDataSubscriptionsDto =
  components["schemas"]["marketdata.SubscriptionsData"];
