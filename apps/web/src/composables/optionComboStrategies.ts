import type {
  OptionComboLegDraft,
  OptionComboSide,
  OptionComboStrategy,
  OptionContractChoice,
} from "./optionComboDraft";

export interface OptionComboQuote {
  bid: number | null;
  mid: number | null;
  ask: number | null;
}

const strategyLabels: Record<OptionComboStrategy, string> = {
  vertical: "垂直价差",
  straddle: "跨式",
  strangle: "宽跨式",
  calendar: "日历价差",
  butterfly: "蝶式",
};

function sameValue<T>(values: T[]): boolean {
  return values.length > 0 && values.every((value) => value === values[0]);
}

function sameRatio(legs: OptionComboLegDraft[]): boolean {
  return sameValue(legs.map((leg) => leg.ratio));
}

function oppositeSides(left: OptionComboSide, right: OptionComboSide): boolean {
  return left !== right;
}

export function optionComboStrategyLabel(
  strategy: OptionComboStrategy | null,
): string {
  return strategy == null ? "自定义选腿" : strategyLabels[strategy];
}

export function recognizeOptionComboStrategy(
  legs: OptionComboLegDraft[],
): OptionComboStrategy | null {
  if (legs.length === 2) {
    const [left, right] = legs;
    if (left == null || right == null || !sameRatio(legs)) return null;
    const sameExpiry = left.expiry === right.expiry;
    const sameStrike = left.strike === right.strike;
    const sameType = left.optionType === right.optionType;
    if (
      sameExpiry &&
      sameType &&
      !sameStrike &&
      oppositeSides(left.side, right.side)
    ) {
      return "vertical";
    }
    if (
      sameExpiry &&
      sameStrike &&
      !sameType &&
      left.side === right.side
    ) {
      return "straddle";
    }
    if (
      sameExpiry &&
      !sameStrike &&
      !sameType &&
      left.side === right.side
    ) {
      return "strangle";
    }
    if (
      !sameExpiry &&
      sameStrike &&
      sameType &&
      oppositeSides(left.side, right.side)
    ) {
      return "calendar";
    }
    return null;
  }
  if (legs.length !== 3) return null;
  const sorted = [...legs].sort((left, right) => left.strike - right.strike);
  const [left, middle, right] = sorted;
  if (left == null || middle == null || right == null) return null;
  const spacingMatches =
    Math.abs(middle.strike - left.strike - (right.strike - middle.strike)) <=
    1e-8;
  const shapeMatches =
    sameValue(sorted.map((leg) => leg.expiry)) &&
    sameValue(sorted.map((leg) => leg.optionType)) &&
    left.side === right.side &&
    middle.side !== left.side &&
    left.ratio === right.ratio &&
    middle.ratio === left.ratio * 2;
  return spacingMatches && shapeMatches ? "butterfly" : null;
}

export function optionComboValidationMessage(
  legs: OptionComboLegDraft[],
): string {
  if (legs.length < 2) return "至少选择两条期权腿";
  if (recognizeOptionComboStrategy(legs) != null) return "";
  return "当前腿结构不属于受支持的垂直、跨式、宽跨式、日历或蝶式组合";
}

function sortedContracts(
  contracts: OptionContractChoice[],
  expiry: string,
  optionType: "call" | "put",
): OptionContractChoice[] {
  return contracts
    .filter(
      (contract) =>
        contract.expiry === expiry && contract.optionType === optionType,
    )
    .sort((left, right) => left.strike - right.strike);
}

function nearestIndex(
  contracts: OptionContractChoice[],
  underlyingPrice: number | null,
): number {
  if (contracts.length === 0) return -1;
  if (underlyingPrice == null) return Math.floor(contracts.length / 2);
  let nearest = 0;
  for (let index = 1; index < contracts.length; index += 1) {
    if (
      Math.abs(contracts[index]!.strike - underlyingPrice) <
      Math.abs(contracts[nearest]!.strike - underlyingPrice)
    ) {
      nearest = index;
    }
  }
  return nearest;
}

function asLeg(
  contract: OptionContractChoice,
  side: OptionComboSide,
  ratio = 1,
): OptionComboLegDraft {
  return { ...contract, side, ratio };
}

function callAndPutAtStrike(
  contracts: OptionContractChoice[],
  expiry: string,
  strike: number,
): [OptionContractChoice, OptionContractChoice] | null {
  const call = contracts.find(
    (contract) =>
      contract.expiry === expiry &&
      contract.optionType === "call" &&
      contract.strike === strike,
  );
  const put = contracts.find(
    (contract) =>
      contract.expiry === expiry &&
      contract.optionType === "put" &&
      contract.strike === strike,
  );
  return call != null && put != null ? [call, put] : null;
}

export function buildOptionComboTemplate(
  strategy: OptionComboStrategy,
  contracts: OptionContractChoice[],
  selectedExpiry: string,
  underlyingPrice: number | null,
): OptionComboLegDraft[] {
  const expiries = [
    ...new Set(contracts.map((contract) => contract.expiry).filter(Boolean)),
  ].sort();
  const expiry = expiries.includes(selectedExpiry)
    ? selectedExpiry
    : (expiries[0] ?? "");
  const calls = sortedContracts(contracts, expiry, "call");
  const atmIndex = nearestIndex(calls, underlyingPrice);
  const atm = calls[atmIndex];
  if (atm == null) return [];

  switch (strategy) {
    case "vertical": {
      const nextIndex = atmIndex < calls.length - 1 ? atmIndex + 1 : atmIndex - 1;
      const other = calls[nextIndex];
      if (other == null) return [];
      const [lower, upper] =
        atm.strike < other.strike ? [atm, other] : [other, atm];
      return [asLeg(lower, "BUY"), asLeg(upper, "SELL")];
    }
    case "straddle": {
      const pair = callAndPutAtStrike(contracts, expiry, atm.strike);
      return pair == null
        ? []
        : [asLeg(pair[0], "BUY"), asLeg(pair[1], "BUY")];
    }
    case "strangle": {
      const puts = sortedContracts(contracts, expiry, "put");
      const lowerPut = [...puts]
        .reverse()
        .find((contract) => contract.strike < atm.strike);
      const upperCall = calls.find(
        (contract) => contract.strike > atm.strike,
      );
      return lowerPut == null || upperCall == null
        ? []
        : [asLeg(lowerPut, "BUY"), asLeg(upperCall, "BUY")];
    }
    case "calendar": {
      const expiryIndex = expiries.indexOf(expiry);
      const farExpiry = expiries[expiryIndex + 1];
      const far = contracts.find(
        (contract) =>
          contract.expiry === farExpiry &&
          contract.optionType === atm.optionType &&
          contract.strike === atm.strike,
      );
      return far == null
        ? []
        : [asLeg(atm, "SELL"), asLeg(far, "BUY")];
    }
    case "butterfly": {
      if (atmIndex <= 0 || atmIndex >= calls.length - 1) return [];
      const left = calls[atmIndex - 1];
      const right = calls[atmIndex + 1];
      if (
        left == null ||
        right == null ||
        Math.abs(atm.strike - left.strike - (right.strike - atm.strike)) >
          1e-8
      ) {
        return [];
      }
      return [
        asLeg(left, "BUY"),
        asLeg(atm, "SELL", 2),
        asLeg(right, "BUY"),
      ];
    }
  }
}

export function optionComboLocalQuote(
  legs: OptionComboLegDraft[],
): OptionComboQuote {
  if (
    legs.length === 0 ||
    legs.some((leg) => leg.bidPrice == null || leg.askPrice == null)
  ) {
    return { bid: null, mid: null, ask: null };
  }
  let naturalBid = 0;
  let naturalAsk = 0;
  for (const leg of legs) {
    const bid = leg.bidPrice ?? 0;
    const ask = leg.askPrice ?? 0;
    if (leg.side === "BUY") {
      naturalBid += bid * leg.ratio;
      naturalAsk += ask * leg.ratio;
    } else {
      naturalBid -= ask * leg.ratio;
      naturalAsk -= bid * leg.ratio;
    }
  }
  const values = [Math.abs(naturalBid), Math.abs(naturalAsk)].sort(
    (left, right) => left - right,
  );
  const bid = values[0] ?? null;
  const ask = values[1] ?? null;
  const roundedBid = bid == null ? null : Number(bid.toFixed(6));
  const roundedAsk = ask == null ? null : Number(ask.toFixed(6));
  return {
    bid: roundedBid,
    ask: roundedAsk,
    mid:
      roundedBid == null || roundedAsk == null
        ? null
        : Number(((roundedBid + roundedAsk) / 2).toFixed(6)),
  };
}

export function optionComboSpread(
  strategy: OptionComboStrategy | null,
  legs: OptionComboLegDraft[],
): number | undefined {
  if (
    strategy !== "vertical" &&
    strategy !== "strangle" &&
    strategy !== "butterfly"
  ) {
    return undefined;
  }
  const strikes = [...new Set(legs.map((leg) => leg.strike))].sort(
    (left, right) => left - right,
  );
  if (strikes.length < 2) return undefined;
  return strategy === "butterfly"
    ? strikes[1]! - strikes[0]!
    : strikes.at(-1)! - strikes[0]!;
}
