// @vitest-environment jsdom

import { describe, expect, it } from "vitest";

import { normalizeStatusWord, statusTone } from "@/composables/shared/statusTone";

describe("statusTone", () => {
  it("maps cross-domain success statuses to success color with Chinese labels", () => {
    for (const status of ["COMPLETED", "DONE", "SUCCEEDED", "APPROVED", "ENABLED"]) {
      expect(statusTone(status).color).toBe("success");
    }
    expect(statusTone("COMPLETED").label).toBe("已完成");
    expect(statusTone("DONE").label).toBe("已完成");
    expect(statusTone("APPROVED").label).toBe("已批准");
    expect(statusTone("ENABLED").label).toBe("已启用");
  });

  it("maps active statuses to info color", () => {
    for (const status of ["RUNNING", "IN_PROGRESS", "PENDING"]) {
      expect(statusTone(status).color).toBe("info");
    }
    expect(statusTone("RUNNING").label).toBe("运行中");
    expect(statusTone("IN_PROGRESS").label).toBe("进行中");
    expect(statusTone("PENDING").label).toBe("待处理");
  });

  it("maps failure statuses to error color", () => {
    for (const status of ["FAILED", "TIMED_OUT", "DENIED"]) {
      expect(statusTone(status).color).toBe("error");
    }
    expect(statusTone("FAILED").label).toBe("失败");
  });

  it("maps waiting and blocked statuses to warning color", () => {
    for (const status of ["PENDING_APPROVAL", "PENDING_INPUT", "BLOCKED", "PAUSED"]) {
      expect(statusTone(status).color).toBe("warning");
    }
    expect(statusTone("PENDING_APPROVAL").label).toBe("等待审批");
    expect(statusTone("BLOCKED").label).toBe("已阻断");
  });

  it("passes labels through unchanged for statuses without a shared Chinese label", () => {
    // label 完全沿用 formatGenericStatusLabel：未收录状态原样返回。
    expect(statusTone("DENIED").label).toBe("DENIED");
    expect(statusTone("TIMED_OUT").label).toBe("TIMED_OUT");
    expect(statusTone("PAUSED").label).toBe("PAUSED");
  });

  it("normalizes case, surrounding whitespace and hyphen separators", () => {
    expect(statusTone("completed").color).toBe("success");
    expect(statusTone("  Running  ").color).toBe("info");
    expect(statusTone("timed-out").color).toBe("error");
    expect(statusTone("pending approval").color).toBe("warning");
    expect(normalizeStatusWord(" in_progress ")).toBe("IN_PROGRESS");
  });

  it("falls back to the default color for statuses that domains color differently", () => {
    // CANCELLED / QUEUED / TODO 的配色在 ADK 与回测域间不一致，共享映射保持中立。
    expect(statusTone("CANCELLED").color).toBe("default");
    expect(statusTone("QUEUED").color).toBe("default");
    expect(statusTone("TODO").color).toBe("default");
    expect(statusTone("CANCELLED").label).toBe("已取消");
    expect(statusTone("QUEUED").label).toBe("排队中");
  });

  it("falls back to the default color for unknown and empty statuses", () => {
    expect(statusTone("SOMETHING_ELSE").color).toBe("default");
    expect(statusTone("SOMETHING_ELSE").label).toBe("SOMETHING_ELSE");
    expect(statusTone("").color).toBe("default");
    expect(statusTone(null).color).toBe("default");
    expect(statusTone(undefined).color).toBe("default");
    expect(statusTone("").label).toBe("未知");
  });
});
