import { TickMarkType, type Time } from "lightweight-charts";

import { formatLocalDateTime } from "@/utils/dateTime";

function toDateFromChartTime(time: Time): Date | null {
  if (typeof time === "number") {
    return new Date(time * 1000);
  }

  if (typeof time === "string") {
    const parsed = new Date(time);
    return Number.isNaN(parsed.getTime()) ? null : parsed;
  }

  return new Date(Date.UTC(time.year, time.month - 1, time.day));
}

export function formatBacktestChartTime(time: Time): string {
  const date = toDateFromChartTime(time);
  return date == null ? "" : formatLocalDateTime(date, "");
}

export function formatBacktestTickMark(time: Time, tickMarkType: TickMarkType): string {
  const date = toDateFromChartTime(time);
  if (date == null) return "";

  const options: Intl.DateTimeFormatOptions =
    tickMarkType === TickMarkType.Year
      ? { year: "numeric" }
      : tickMarkType === TickMarkType.Month
        ? { month: "2-digit", year: "2-digit" }
        : tickMarkType === TickMarkType.DayOfMonth
          ? { month: "2-digit", day: "2-digit" }
          : { hour: "2-digit", minute: "2-digit", hour12: false };
  return new Intl.DateTimeFormat(undefined, options).format(date);
}
