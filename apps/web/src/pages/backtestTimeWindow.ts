const DAY_START_SUFFIX = "T00:00:00Z";
const DAY_INCLUSIVE_END_SUFFIX = "T23:59:59.999Z";

export function buildBacktestDayStartTime(date: string): string {
  return `${date.trim()}${DAY_START_SUFFIX}`;
}

export function buildBacktestDayInclusiveEndTime(date: string): string {
  return `${date.trim()}${DAY_INCLUSIVE_END_SUFFIX}`;
}