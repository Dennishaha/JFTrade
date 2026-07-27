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

export type InstrumentResolutionStatus =
  | "resolved"
  | "ambiguous"
  | "not_found"
  | "incomplete"
  | "unavailable";

export interface InstrumentResolutionFailure {
  market: string;
  code: string;
  message: string;
}

export interface InstrumentResolutionCandidate {
  market: string;
  resolvedMarket: string;
  instrumentId: string;
  code: string;
  symbol: string;
  name: string | null;
  securityType: string | null;
  lotSize: number | null;
  source: string;
  isWatched: boolean;
  selectable: boolean;
  unavailableReason: string | null;
}

export interface InstrumentResolutionResponse {
  requestedMarket: string;
  query: string;
  resolutionStatus: InstrumentResolutionStatus;
  totalReturned: number;
  entries: InstrumentResolutionCandidate[];
  failures: InstrumentResolutionFailure[];
}
