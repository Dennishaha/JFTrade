export function normalizeSupportedPeriods(
  value: unknown,
): string[] | undefined {
  if (!Array.isArray(value)) return undefined;
  return [
    ...new Set(
      value.flatMap((period) => {
        if (typeof period !== "string") return [];
        const normalized = period.trim().toLowerCase();
        return normalized ? [normalized] : [];
      }),
    ),
  ];
}
