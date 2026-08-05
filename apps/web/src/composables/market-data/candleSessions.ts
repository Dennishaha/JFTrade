export type CandleSession = "regular" | "extended" | "overnight";

export const CANDLE_SESSION_ORDER: readonly CandleSession[] = [
  "regular",
  "extended",
  "overnight",
];

export const CANDLE_SESSION_LABELS: Record<CandleSession, string> = {
  regular: "盘中",
  extended: "盘前后",
  overnight: "夜盘",
};

export function normalizeCandleSessions(
  values: readonly (string | null | undefined)[] | null | undefined,
): CandleSession[] {
  const requested = new Set(values?.flatMap((value) =>
    String(value ?? "")
      .split(",")
      .map((token) => token.trim().toLowerCase()),
  ));
  return CANDLE_SESSION_ORDER.filter((session) => requested.has(session));
}

export function intersectCandleSessions(
  selected: readonly CandleSession[],
  available: readonly CandleSession[],
): CandleSession[] {
  const availableSet = new Set(available);
  return CANDLE_SESSION_ORDER.filter(
    (session) => availableSet.has(session) && selected.includes(session),
  );
}

export function summarizeCandleSessions(
  sessions: readonly CandleSession[],
): string {
  const normalized = normalizeCandleSessions(sessions);
  if (normalized.length === CANDLE_SESSION_ORDER.length) return "全天";
  if (normalized.length === 1) return CANDLE_SESSION_LABELS[normalized[0]!];
  if (normalized.length === 2 && normalized[0] === "regular" && normalized[1] === "extended") {
    return "盘中+盘前后";
  }
  return normalized.map((session) => CANDLE_SESSION_LABELS[session]).join("+");
}

export function candleSessionForValue(
  value: string | null | undefined,
): CandleSession | null {
  const normalized = value?.trim().toLowerCase();
  if (normalized === "regular" || normalized === "rth") return "regular";
  if (normalized === "extended" || normalized === "pre" || normalized === "after" || normalized === "eth") return "extended";
  if (normalized === "overnight") return "overnight";
  return null;
}

export function candleMatchesSessions(
  value: string | null | undefined,
  selected: readonly CandleSession[],
): boolean {
  const session = candleSessionForValue(value) ?? "regular";
  return selected.includes(session);
}
