function padTwoDigits(value: number): string {
  return String(value).padStart(2, "0");
}

type DateTimeInput = string | number | Date | null | undefined;

function parseDateTime(
  value: DateTimeInput,
  fallback: string,
): Date | string {
  if (value == null || value === "") return fallback;
  const date = value instanceof Date ? value : new Date(value);
  if (Number.isNaN(date.getTime())) {
    return typeof value === "string" ? value : fallback;
  }
  return date;
}

export function formatLocalDateTime(
  value: DateTimeInput,
  fallback = "--",
): string {
  const parsed = parseDateTime(value, fallback);
  if (typeof parsed === "string") return parsed;

  return `${parsed.getFullYear()}-${padTwoDigits(parsed.getMonth() + 1)}-${padTwoDigits(parsed.getDate())} ${padTwoDigits(parsed.getHours())}:${padTwoDigits(parsed.getMinutes())}:${padTwoDigits(parsed.getSeconds())}`;
}

export function formatDateTime(
  value: DateTimeInput,
  options: {
    fallback?: string;
    locale?: string | string[];
    timeZoneName?: Intl.DateTimeFormatOptions["timeZoneName"];
  } = {},
): string {
  const fallback = options.fallback ?? "—";
  const parsed = parseDateTime(value, fallback);
  if (typeof parsed === "string") return parsed;
  return new Intl.DateTimeFormat(options.locale, {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
    ...(options.timeZoneName == null
      ? {}
      : { timeZoneName: options.timeZoneName }),
  }).format(parsed);
}
