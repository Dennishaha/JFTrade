// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  emptyRealTradeApprovals,
  emptyRealTradeHardStopEvents,
  emptyRealTradeHardStops,
  emptyRealTradeKillSwitchEvents,
  emptyRealTradeKillSwitchState,
  emptyRealTradeRiskEvents,
  emptyRealTradeRiskState,
  emptySystemStatus,
} from "@/contracts";
import type { SystemStatusResponse } from "@/contracts";

import {
  MockWebSocket,
  createLiveEnvelope,
  createResponse,
  flushRequests,
  mountApp,
} from "./helpers";

function findLiveEventStream(): MockWebSocket | undefined {
  return MockWebSocket.instances.find((instance) =>
    instance.url.includes("/api/v1/ws/live"),
  );
}

const systemStatus: SystemStatusResponse = {
  ...emptySystemStatus,
  message: "runtime ready",
  observability: {
    requests: {
      slowThresholdMs: 750,
      minimumImportance: "low",
      recentErrors: [
        {
          at: "2026-05-16T00:31:00.000Z",
          level: "error",
          importance: "high",
          message: "backtest sync task failed",
          error: "OpenD history unavailable",
          requestId: "request-sync-1",
          taskId: "sync-1",
          instrumentId: "US.AAPL",
          source: "backtest",
        },
      ],
      recentSlowRequests: [
        {
          at: "2026-05-16T00:32:00.000Z",
          level: "info",
          importance: "low",
          message: "api request",
          method: "GET",
          path: "/api/v1/backtests",
          requestId: "request-slow-1",
          latencyMs: 900,
          source: "api",
        },
      ],
      openD: {
        totalCalls: 5,
        failedCalls: 1,
        lastOperation: "proto_3006",
        lastRequestId: "request-sync-1",
      },
    },
  },
};

afterEach(() => {
  vi.unstubAllGlobals();
  MockWebSocket.instances = [];
});

describe("application notifications", () => {
  it("renders developer observability and structured live notifications", async () => {
    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      const url = String(input);

      if (url.includes("/api/v1/system/status"))
        return createResponse(systemStatus);
      if (url.includes("/api/v1/system/real-trade-approvals"))
        return createResponse(emptyRealTradeApprovals);
      if (url.includes("/api/v1/system/real-trade-hard-stops"))
        return createResponse(emptyRealTradeHardStops);
      if (url.includes("/api/v1/system/real-trade-hard-stop-events"))
        return createResponse(emptyRealTradeHardStopEvents);
      if (url.includes("/api/v1/system/real-trade-kill-switch-events"))
        return createResponse(emptyRealTradeKillSwitchEvents);
      if (url.includes("/api/v1/system/real-trade-kill-switch"))
        return createResponse(emptyRealTradeKillSwitchState);
      if (url.includes("/api/v1/system/real-trade-risk-events"))
        return createResponse(emptyRealTradeRiskEvents);
      if (url.includes("/api/v1/system/real-trade-risk-limits"))
        return createResponse(emptyRealTradeRiskState);

      throw new Error(`Unexpected request: ${url}`);
    });

    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal(
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const { wrapper } = await mountApp("/settings/developer-tools");
    const liveStream = findLiveEventStream();

    expect(wrapper.text()).toContain("开发者工具");
    expect(wrapper.text()).toContain("链路观测");
    expect(wrapper.text()).toContain("backtest sync task failed");
    expect(wrapper.text()).toContain("proto_3006");
    expect(wrapper.find(".tv-rightdock-toggle").exists()).toBe(false);
    expect(wrapper.find(".tv-app-content-split").exists()).toBe(false);
    expect(wrapper.find(".tv-rightdock-slot").exists()).toBe(false);

    const mainElement = wrapper.get(".tv-main").element;
    await wrapper.get('button[title="通知"]').trigger("click");
    expect(wrapper.get(".tv-main").element).toBe(mainElement);
    expect(wrapper.find(".tv-app-content-split").exists()).toBe(false);
    expect(wrapper.find(".tv-rightdock-slot").exists()).toBe(true);
    await wrapper.get('[data-testid="rightdock-tab-notifications"]').trigger(
      "click",
    );

    const notification = {
      type: "system.notification",
      id: "system-notification-1",
      at: "2026-05-16T00:31:00.000Z",
      level: "warn",
      title: "OpenD 连接状态变化",
      message: "行情未登录，交易未登录。",
      source: "futu-opend",
      brokerId: "futu",
      category: "broker.connection",
    };
    liveStream?.emitMessage(createLiveEnvelope(notification, {
      source: "notification",
      entityId: "system-notification-1",
      eventId: "system-notification-1",
    }));
    await flushRequests();

    expect(wrapper.text()).toContain("OpenD 连接状态变化");
    expect(wrapper.text()).toContain("行情未登录，交易未登录。");
    expect(wrapper.text()).toContain("富途 OpenD");
    expect(wrapper.text()).toContain("券商连接");
    expect(wrapper.text()).not.toContain("WS: system.notification");

    wrapper.unmount();
  });
});
