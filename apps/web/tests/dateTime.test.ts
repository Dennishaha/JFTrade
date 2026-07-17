import { describe, expect, it } from "vitest";

import { formatLocalDateTime } from "../src/utils/dateTime";

describe("formatLocalDateTime", () => {
  it("keeps a non-date string visible and protects callers from invalid non-string values", () => {
    expect(formatLocalDateTime("not-a-date")).toBe("not-a-date");
    expect(formatLocalDateTime(Number.NaN, "未知时间")).toBe("未知时间");
  });
});
