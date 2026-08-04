import type {
  MarketDataInstrumentCandidateDto,
  MarketDataInstrumentResolutionDto,
  MarketDataInstrumentResolutionFailureDto,
  MarketDataInstrumentResolutionStatus,
} from "@/contracts";

export interface MarketTradingWindowDto {
  startMinute: number;
  endMinute: number;
  label: string;
}

export interface MarketPrecisionDto {
  price: number;
  quote: number;
}

export interface MarketProfileDto {
  code: string;
  resolvedMarket: string;
  preferredPrefix: string;
  displayName: string;
  quoteCurrency: string;
  timezone: string;
  supportsExtendedHours: boolean;
  requiresExchangePrefix: boolean;
  aliases: string[];
  regularSessions: MarketTradingWindowDto[];
  precision: MarketPrecisionDto;
  tickSize: number;
}

export interface MarketProfilesResponse {
  markets: MarketProfileDto[];
  defaultMarket: string;
  updatedAt: string;
}

export type InstrumentResolutionStatus = MarketDataInstrumentResolutionStatus;

export type InstrumentResolutionFailure =
  MarketDataInstrumentResolutionFailureDto;

export type InstrumentResolutionCandidate = Omit<
  MarketDataInstrumentCandidateDto,
  | "isWatched"
  | "lotSize"
  | "name"
  | "securityType"
  | "source"
  | "supportedPeriods"
  | "unavailableReason"
> & {
  name: string | null;
  securityType: string | null;
  lotSize: number | null;
  source: string;
  isWatched: boolean;
  unavailableReason: string | null;
  supportedPeriods?: string[];
};

export type InstrumentResolutionResponse = Omit<
  MarketDataInstrumentResolutionDto,
  "entries" | "failures"
> & {
  entries: InstrumentResolutionCandidate[];
  failures: InstrumentResolutionFailure[];
};
