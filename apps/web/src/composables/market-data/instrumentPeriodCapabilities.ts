import type { MarketSecurityDetailsQueryResult } from "@/types";

function providerIdentityMatches(expected: string, observed: string): boolean {
  if (expected === "yfinance") {
    return observed === "yfinance" || observed === "yahoo-finance";
  }
  if (expected === "futu") {
    return observed === "futu" || observed === "futu-opend";
  }
  return expected !== "" && observed === expected;
}

function providerSourceMatches(expected: string, source: string): boolean {
  if (expected === "akshare") {
    return source === "akshare" || source.startsWith("akshare:");
  }
  if (expected === "yfinance") {
    return source === "yfinance" || source === "yahoo-finance";
  }
  if (expected === "futu") {
    return (
      source === "futu" ||
      source === "futu-opend" ||
      source.startsWith("futu:") ||
      source.startsWith("bbgo:futu")
    );
  }
  return (
    expected !== "" &&
    (source === expected || source.startsWith(`${expected}:`))
  );
}

function matchingDetails(input: {
  providerID: string;
  instrumentID: string;
  details: MarketSecurityDetailsQueryResult | null;
}): MarketSecurityDetailsQueryResult | null {
  const details = input.details;
  if (
    details == null ||
    details.request.instrumentId.trim().toUpperCase() !== input.instrumentID
  ) {
    return null;
  }
  const expectedProvider = input.providerID.trim().toLowerCase();
  const brokerID = details.meta.brokerId?.trim().toLowerCase() ?? "";
  if (brokerID !== "") {
    return providerIdentityMatches(expectedProvider, brokerID) ? details : null;
  }
  const source = details.meta.source?.trim().toLowerCase() ?? "";
  return providerSourceMatches(expectedProvider, source) ? details : null;
}

export function resolveInstrumentSupportedPeriods(input: {
  providerID: string;
  instrumentID: string;
  providerPeriods: string[] | null;
  details: MarketSecurityDetailsQueryResult | null;
  requireDetails: boolean;
}): string[] | null {
  if (input.providerPeriods == null) return null;
  const details = matchingDetails(input);
  const instrumentPeriods = details?.security?.supportedPeriods;
  if (Array.isArray(instrumentPeriods)) {
    const providerSet = new Set(input.providerPeriods);
    return [
      ...new Set(
        instrumentPeriods
          .map((period) => period.trim().toLowerCase())
          .filter((period) => period !== "" && providerSet.has(period)),
      ),
    ];
  }
  if (!input.requireDetails) return input.providerPeriods;
  return details == null ? null : [];
}

export function fallbackInstrumentPeriod(values: readonly string[]): string {
  for (const candidate of ["1m", "5m", "1d"]) {
    if (values.includes(candidate)) return candidate;
  }
  return values.find((period) => period !== "tick") ?? "tick";
}

export function resolveInstrumentRequestPeriod(input: {
  preferredPeriod: string;
  providerPeriods: string[] | null;
  supportedPeriods: string[] | null;
  requireDetails: boolean;
  renderablePeriods: ReadonlySet<string>;
}): string {
  const providerPeriods = (input.providerPeriods ?? []).filter((period) =>
    input.renderablePeriods.has(period),
  );
  const requestable = new Set(input.supportedPeriods ?? providerPeriods);
  if (requestable.has(input.preferredPeriod)) return input.preferredPeriod;
  return input.requireDetails && input.supportedPeriods == null && providerPeriods.length
    ? fallbackInstrumentPeriod(providerPeriods)
    : "";
}
