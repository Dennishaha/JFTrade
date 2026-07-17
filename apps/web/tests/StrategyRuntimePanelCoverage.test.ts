// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

import type { StrategyDefinitionDocument } from "@/contracts";
import StrategyRuntimePanel from "../src/components/StrategyRuntimePanel.vue";
import { PINE_WORKER_RUNTIME } from "../src/components/strategy-runtime/strategyRuntimeIdentity";
import {
  MockWebSocket,
  buildFetchMock,
  mountStrategyPage,
  resetStrategyPageTestState,
  settleStrategyWorkspace,
} from "./strategyPageTestUtils";

function read<T>(value: unknown): T {
  return value !== null && typeof value === "object" && "value" in value
    ? (value as { value: T }).value
    : value as T;
}

function write(setup: Record<string, unknown>, key: string, value: unknown): void {
  const current = setup[key];
  if (current !== null && typeof current === "object" && "value" in current) {
    (current as { value: unknown }).value = value;
    return;
  }
  setup[key] = value;
}

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  resetStrategyPageTestState();
});

describe("StrategyRuntimePanel boundary behavior", () => {
  it("keeps an empty runtime on the instance list", async () => {
    const fetchSpy = vi.fn(buildFetchMock({ definitions: [], strategies: [] }));
    vi.stubGlobal("fetch", fetchSpy);
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);

    const { wrapper } = await mountStrategyPage("/strategy/runtime");
    await settleStrategyWorkspace();
    const setup = wrapper.getComponent(StrategyRuntimePanel).vm.$.setupState as Record<string, unknown>;

    expect(read<unknown>(setup.selectedStrategyDefinitionDocument)).toBeNull();
    (setup.selectStrategyRuntimeMobileSection as (section: string) => void)("workbench");
    expect(read<string>(setup.strategyRuntimeMobileSection)).toBe("instances");

    expect(fetchSpy.mock.calls.some((call) => call[1]?.method === "POST")).toBe(false);
    wrapper.unmount();
  });

  it("requires raw create and edit symbol drafts to be confirmed before mutation", async () => {
    const fetchSpy = vi.fn(buildFetchMock({
      definitions: [buildDefinition()],
      strategies: [buildStoppedStrategy()],
    }));
    vi.stubGlobal("fetch", fetchSpy);
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);

    const { wrapper } = await mountStrategyPage("/strategy/runtime");
    await settleStrategyWorkspace();
    const setup = wrapper.getComponent(StrategyRuntimePanel).vm.$.setupState as Record<string, unknown>;

    (setup.openCreateInstanceForm as () => void)();
    write(setup, "createDefinitionId", "mean-revert");
    (setup.updateActiveSymbolDraft as (value: string) => void)("HK.00700");
    await (setup.createStrategyInstance as () => Promise<void>)();
    expect(read<string>(setup.instanceMutationError)).toBe(
      "请先解析并确认待添加的交易代码。",
    );

    (setup.openEditInstanceForm as () => void)();
    (setup.updateActiveSymbolDraft as (value: string) => void)("US.AAPL");
    await (setup.updateSelectedStrategyBinding as () => Promise<void>)();
    expect(read<string>(setup.instanceMutationError)).toBe(
      "请先解析并确认待添加的交易代码。",
    );
    expect(fetchSpy.mock.calls.some((call) => call[1]?.method === "POST")).toBe(false);
    wrapper.unmount();
  });

  it("defers background refreshes while hidden or busy instead of racing instance mutations", async () => {
    vi.stubGlobal("fetch", buildFetchMock({ definitions: [], strategies: [] }));
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);
    const originalVisibility = Object.getOwnPropertyDescriptor(
      document,
      "visibilityState",
    );
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      value: "visible",
    });

    const { wrapper } = await mountStrategyPage("/strategy/runtime");
    await settleStrategyWorkspace();
    const setup = wrapper.getComponent(StrategyRuntimePanel).vm.$.setupState as Record<string, unknown>;
    (setup.clearStrategyRuntimeRefreshTimer as () => void)();
    vi.useFakeTimers();

    write(setup, "isLoadingDetails", true);
    await (setup.refreshStrategyRuntimeContent as () => Promise<void>)();
    expect(vi.getTimerCount()).toBeGreaterThan(0);

    (setup.clearStrategyRuntimeRefreshTimer as () => void)();
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      value: "hidden",
    });
    await (setup.refreshStrategyRuntimeContent as () => Promise<void>)();
    (setup.scheduleStrategyRuntimeRefresh as () => void)();
    expect(vi.getTimerCount()).toBe(0);

    wrapper.unmount();
    if (originalVisibility) {
      Object.defineProperty(document, "visibilityState", originalVisibility);
    }
  });
});

function buildDefinition(): StrategyDefinitionDocument {
  return {
    id: "mean-revert",
    name: "Mean Revert",
    version: "0.1.0",
    description: "EMA mean reversion",
    runtime: PINE_WORKER_RUNTIME,
    sourceFormat: "pine-v6",
    symbol: "HK.00700",
    interval: "5m",
    script: '//@version=6\nstrategy("Mean Revert")\n',
    visualModel: null,
    createdAt: "2026-06-01T00:00:00.000Z",
    updatedAt: "2026-06-01T00:00:00.000Z",
  };
}

function buildStoppedStrategy() {
  return {
    id: "instance-1",
    definition: { strategyId: "mean-revert", name: "Mean Revert", version: "0.1.0" },
    runtime: PINE_WORKER_RUNTIME,
    sourceFormat: "pine-v6" as const,
    startable: true,
    binding: { symbols: ["HK.00700"], interval: "5m", executionMode: "live" as const },
    params: {
      definitionId: "mean-revert",
      symbol: "HK.00700",
      symbols: ["HK.00700"],
      interval: "5m",
      executionMode: "live",
    },
    status: "STOPPED" as const,
    createdAt: "2026-06-01T00:00:00.000Z",
    logs: [],
  };
}
