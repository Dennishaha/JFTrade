import { describe, expect, it } from "vitest";

import {
  formatPermission,
  isInternalSkill,
  preview,
  riskColor,
  riskLabel,
  toolCallStatusColor,
} from "../src/composables/adkSettingsPresentation";

describe("adkSettingsPresentation", () => {
  it("maps permission, tool status, and risk labels for real ADK states", () => {
    expect(formatPermission("approval")).toBe("请求批准");
    expect(formatPermission("less_approval")).toBe("减少审批");
    expect(formatPermission("all")).toBe("全部允许");

    expect(toolCallStatusColor("COMPLETED")).toBe("success");
    expect(toolCallStatusColor("FAILED")).toBe("error");
    expect(toolCallStatusColor("PENDING_APPROVAL")).toBe("warning");
    expect(toolCallStatusColor("RUNNING")).toBe("info");
    expect(toolCallStatusColor("CANCELLED")).toBe("default");
    expect(toolCallStatusColor("UNKNOWN")).toBe("default");

    expect(riskColor("low")).toBe("success");
    expect(riskColor("medium")).toBe("info");
    expect(riskColor("high")).toBe("warning");
    expect(riskColor("critical")).toBe("error");
    expect(riskColor("")).toBe("default");

    expect(riskLabel("low")).toBe("低风险");
    expect(riskLabel("medium")).toBe("中风险");
    expect(riskLabel("high")).toBe("高风险");
    expect(riskLabel("critical")).toBe("关键风险");
    expect(riskLabel()).toBe("未标注");
  });

  it("formats previews and detects builtin skills robustly", () => {
    expect(preview({ foo: "bar" })).toContain('"foo": "bar"');
    const circular: { self?: unknown; toString: () => string } = {
      toString: () => "circular preview",
    };
    circular.self = circular;
    expect(preview(circular)).toBe("circular preview");

    expect(
      isInternalSkill({
        id: "builtin",
        displayName: "Builtin",
        description: "",
        source: "  BUILTIN  ",
        installPath: "",
        createdAt: "2026-07-01T00:00:00Z",
        updatedAt: "2026-07-01T00:00:00Z",
      }),
    ).toBe(true);
    expect(
      isInternalSkill({
        id: "external",
        displayName: "External",
        description: "",
        source: "https://skills.example/research",
        installPath: "",
        createdAt: "2026-07-01T00:00:00Z",
        updatedAt: "2026-07-01T00:00:00Z",
      }),
    ).toBe(false);
  });
});
