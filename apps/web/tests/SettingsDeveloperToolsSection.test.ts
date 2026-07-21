// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import type { RequestObservabilitySummary } from "../src/contracts";
import SettingsDeveloperToolsSection from "../src/components/SettingsDeveloperToolsSection.vue";

const passthroughStub = { template: "<div><slot /></div>" };
const buttonStub = {
  props: ["to"],
  template: "<a :href='to'><slot /></a>",
};

function mountSection(observability: RequestObservabilitySummary) {
  return mount(SettingsDeveloperToolsSection, {
    props: { observability },
    global: {
      stubs: {
        "v-card": passthroughStub,
        "v-card-text": passthroughStub,
        "v-chip": passthroughStub,
        "v-btn": buttonStub,
      },
    },
  });
}

function createObservability(): RequestObservabilitySummary {
  return {
    slowThresholdMs: 900,
    minimumImportance: "normal",
    openD: {
      totalCalls: 8,
      failedCalls: 2,
      lastOperation: "proto_3006",
    },
    recentErrors: [
      {
        at: "2026-07-21T01:00:00.000Z",
        level: "error",
        importance: "critical",
        message: "ADK provider failed",
        error: "provider unavailable",
        requestId: "req-1",
        sessionId: "session-1",
        runId: "run-1",
        providerId: "provider-1",
        source: "adk",
      },
      {
        at: "2026-07-21T01:01:00.000Z",
        level: "error",
        importance: "high",
        message: "Backtest sync failed",
        taskId: "sync-1",
        instrumentId: "US.AAPL",
        source: "backtest",
      },
      ...[2, 3, 4, 5].map((index) => ({
        at: `2026-07-21T01:0${index}:00.000Z`,
        level: "error",
        importance: "high" as const,
        message: `error-${index}`,
      })),
    ],
    recentSlowRequests: [0, 1, 2, 3, 4, 5].map((index) => ({
      at: `2026-07-21T02:0${index}:00.000Z`,
      level: "info",
      importance: "low" as const,
      message: "api request",
      method: "GET",
      path: `/api/v1/slow-${index}`,
      latencyMs: 1000 + index,
      requestId: `slow-${index}`,
    })),
  };
}

describe("SettingsDeveloperToolsSection", () => {
  it("renders request observability and keeps the display limits and targets", () => {
    const wrapper = mountSection(createObservability());

    expect(wrapper.text()).toContain("记录阈值 普通重要性");
    expect(wrapper.text()).toContain("慢请求阈值 900ms");
    expect(wrapper.text()).toContain("6 / 8");
    expect(wrapper.text()).toContain("proto_3006");
    expect(wrapper.text()).toContain("关键重要性");
    expect(wrapper.text()).toContain("request req-1");
    expect(wrapper.text()).toContain("session session-1");
    expect(wrapper.text()).toContain("run run-1");
    expect(wrapper.text()).toContain("provider provider-1");
    expect(wrapper.text()).toContain("task sync-1");
    expect(wrapper.text()).toContain("instrument US.AAPL");
    expect(wrapper.get('a[href="/adk/agents"]').text()).toBe("ADK 运行");
    expect(wrapper.get('a[href="/backtest"]').text()).toBe("回测任务");
    expect(wrapper.text()).toContain("error-4");
    expect(wrapper.text()).not.toContain("error-5");
    expect(wrapper.text()).toContain("/api/v1/slow-4");
    expect(wrapper.text()).not.toContain("/api/v1/slow-5");
  });

  it("renders the empty error state", () => {
    const observability = createObservability();
    observability.recentErrors = [];
    observability.recentSlowRequests = [];
    const wrapper = mountSection(observability);

    expect(wrapper.text()).toContain("当前没有近期链路错误");
    expect(wrapper.text()).not.toContain("/api/v1/slow-");
  });
});
