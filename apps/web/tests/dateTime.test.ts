import { describe, expect, it } from "vitest";

import { formatLocalDateTime } from "../src/utils/dateTime";

describe("formatLocalDateTime", () => {
  it("uses the caller fallback when no timestamp is available", () => {
    expect(formatLocalDateTime(null)).toBe("--");
    expect(formatLocalDateTime(undefined, "未知时间")).toBe("未知时间");
    expect(formatLocalDateTime("", "暂无时间")).toBe("暂无时间");
  });

  it("keeps a non-date string visible and protects callers from invalid non-string values", () => {
    expect(formatLocalDateTime("not-a-date")).toBe("not-a-date");
    expect(formatLocalDateTime(Number.NaN, "未知时间")).toBe("未知时间");
  });
});
