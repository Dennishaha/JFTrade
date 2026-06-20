const DATE_LABEL_PATTERN = /^(\d{4})-(\d{2})-(\d{2})$/;

export function normalizeBacktestDateLabel(date: string): string {
  const normalized = date.trim();
  const match = DATE_LABEL_PATTERN.exec(normalized);
  if (match == null) {
    return "";
  }
  const year = Number(match[1]);
  const month = Number(match[2]);
  const day = Number(match[3]);
  const leapYear = year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0);
  const daysInMonth = [31, leapYear ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
  if (month < 1 || month > 12 || day < 1 || day > (daysInMonth[month - 1] ?? 0)) {
    return "";
  }
  return normalized;
}
