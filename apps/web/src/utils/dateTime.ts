function padTwoDigits(value: number): string {
  return String(value).padStart(2, "0");
}

export function formatLocalDateTime(
  value: string | number | Date | null | undefined,
  fallback = "--",
): string {
  if (value == null || value === "") {
    return fallback;
  }

  const date = value instanceof Date ? value : new Date(value);
  if (Number.isNaN(date.getTime())) {
    return typeof value === "string" ? value : fallback;
  }

  return `${date.getFullYear()}-${padTwoDigits(date.getMonth() + 1)}-${padTwoDigits(date.getDate())} ${padTwoDigits(date.getHours())}:${padTwoDigits(date.getMinutes())}:${padTwoDigits(date.getSeconds())}`;
}