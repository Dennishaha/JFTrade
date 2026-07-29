// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent, nextTick } from "vue";

import {
  buildPineV6WorkflowScript,
  createDefaultPineV6Workflow,
} from "../src/features/pineV6Workflow";
import { queryClient } from "@/composables/settings/serverState";
import StrategyDesignStage from "@/components/strategy-design/StrategyDesignStage.vue";
import { useStrategyDesignContext } from "../src/components/strategy-design/strategyDesignContext";
import {
  MockWebSocket,
  buildFetchMock,
  mountStrategyPage,
  resetStrategyPageTestState,
  settleStrategyWorkspace,
} from "./strategyPageTestUtils";
import { createResponse } from "./helpers";

afterEach(() => {
  queryClient.setDefaultOptions({
    queries: {
      gcTime: 5 * 60 * 1000,
      refetchOnMount: false,
      refetchOnWindowFocus: false,
      retry: 1,
      staleTime: 30 * 1000,
    },
  });
  vi.useRealTimers();
  vi.unstubAllGlobals();
  resetStrategyPageTestState();
});

function requestMethod(
  input: string | URL | Request,
  init?: RequestInit,
): string {
  return input instanceof Request ? input.method : init?.method ?? "GET";
}

function findButtonByLabels(
  wrapper: Awaited<ReturnType<typeof mountStrategyPage>>["wrapper"],
  labels: string[],
) {
  const button = wrapper.findAll("button").find((candidate) =>
    labels.includes(candidate.text().trim()),
  );
  if (button == null) {
    throw new Error(`Button not found: ${labels.join(", ")}`);
  }
  return button;
}

function findDefinitionSelect(
  wrapper: Awaited<ReturnType<typeof mountStrategyPage>>["wrapper"],
) {
  const select = wrapper.findAll("select").find((candidate) =>
    candidate.findAll("option").some((option) => option.text() === "新建草稿"),
  );
  if (select == null) {
    throw new Error("Strategy definition select not found.");
  }
  return select;
}

function findFieldByLabel(
  wrapper: Awaited<ReturnType<typeof mountStrategyPage>>["wrapper"],
  labelText: string,
  selector: "input" | "textarea" = "input",
) {
  const label = wrapper.findAll("label").find((candidate) =>
    candidate.text().includes(labelText) && candidate.find(selector).exists(),
  );
  if (label == null) {
    throw new Error(`Field not found for label: ${labelText}`);
  }
  return label.get(selector);
}

function strategySourceEditor(
  wrapper: Awaited<ReturnType<typeof mountStrategyPage>>["wrapper"],
) {
  return wrapper.get('[data-testid="strategy-script-editor"]');
}

async function settleWithFakeTimers(): Promise<void> {
  for (let attempt = 0; attempt < 4; attempt += 1) {
    await Promise.resolve();
    await nextTick();
    await vi.advanceTimersByTimeAsync(0);
  }
}

function readStrategySetupValue<T>(value: unknown): T {
  if (value !== null && typeof value === "object" && "value" in value) {
    return (value as { value: T }).value;
  }
  return value as T;
}

function writeStrategySetupValue(
  setup: Record<string, unknown>,
  key: string,
  value: unknown,
): void {
  const current = setup[key];
  if (current !== null && typeof current === "object" && "value" in current) {
    (current as { value: unknown }).value = value;
    return;
  }
  setup[key] = value;
}

describe("StrategyDesignStage business flows", () => {
  it("auto-collapses metadata at medium widths and closes the drawer with Escape", async () => {
    let mediaChangeListener: ((event: MediaQueryListEvent) => void) | undefined;
    const removeEventListener = vi.fn();
    const mediaQuery = {
      matches: true,
      media: "(min-width: 769px) and (max-width: 1180px)",
      onchange: null,
      addEventListener: vi.fn(
        (_type: string, listener: (event: MediaQueryListEvent) => void) => {
          mediaChangeListener = listener;
        },
      ),
      removeEventListener,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    } as unknown as MediaQueryList;
    vi.stubGlobal("matchMedia", vi.fn(() => mediaQuery));
    vi.stubGlobal("fetch", buildFetchMock({ definitions: [], strategies: [] }));
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);

    const { wrapper } = await mountStrategyPage("/strategy/design");
    await settleStrategyWorkspace();

    const stage = wrapper.get('[data-testid="strategy-design-stage"]');
    const toggle = wrapper.get('[data-testid="strategy-metadata-toggle"]');
    expect(stage.classes()).toContain("strategy-native-page--medium");
    expect(stage.classes()).toContain("strategy-native-page--metadata-closed");
    expect(toggle.attributes("aria-expanded")).toBe("false");
    expect(
      wrapper.get('[data-testid="strategy-side-panel-definition-title"]').attributes("draggable"),
    ).toBe("false");
    expect(wrapper.find('[data-testid="strategy-metadata-backdrop"]').exists()).toBe(false);

    await toggle.trigger("click");
    expect(stage.classes()).toContain("strategy-native-page--metadata-open");
    expect(toggle.attributes("aria-expanded")).toBe("true");
    expect(wrapper.find('[data-testid="strategy-metadata-backdrop"]').exists()).toBe(true);

    await wrapper.get('[data-testid="strategy-metadata-backdrop"]').trigger("click");
    expect(stage.classes()).toContain("strategy-native-page--metadata-closed");
    expect(wrapper.find('[data-testid="strategy-metadata-backdrop"]').exists()).toBe(false);

    await toggle.trigger("click");
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    await nextTick();
    expect(stage.classes()).toContain("strategy-native-page--metadata-closed");
    expect(wrapper.find('[data-testid="strategy-metadata-backdrop"]').exists()).toBe(false);

    mediaChangeListener?.({ matches: false } as MediaQueryListEvent);
    await nextTick();
    expect(stage.classes()).not.toContain("strategy-native-page--medium");
    expect(stage.classes()).toContain("strategy-native-page--metadata-open");

    wrapper.unmount();
    expect(removeEventListener).toHaveBeenCalledWith("change", expect.any(Function));
  });

  it("reorders desktop side panels by drag without changing the expanded panel", async () => {
    vi.stubGlobal("fetch", buildFetchMock({ definitions: [], strategies: [] }));
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);

    const { wrapper } = await mountStrategyPage("/strategy/design");
    await settleStrategyWorkspace();

    const definitionPanel = wrapper.get('[data-testid="strategy-side-panel-definition"]');
    const historyPanel = wrapper.get('[data-testid="strategy-side-panel-history"]');
    const definitionTitle = wrapper.get('[data-testid="strategy-side-panel-definition-title"]');
    const historyTitle = wrapper.get('[data-testid="strategy-side-panel-history-title"]');

    expect(definitionTitle.attributes("draggable")).toBe("true");
    expect((definitionPanel.element as HTMLElement).style.order).toBe("0");
    expect((historyPanel.element as HTMLElement).style.order).toBe("1");

    const dataTransfer = {
      dropEffect: "none",
      effectAllowed: "none",
      setData: vi.fn(),
    };
    await definitionTitle.trigger("dragstart", { dataTransfer });
    await definitionTitle.trigger("dragover", { clientY: 1, dataTransfer });
    await historyTitle.trigger("dragover", { clientY: 1, dataTransfer });
    await historyTitle.trigger("drop", { dataTransfer });
    await historyTitle.trigger("dragstart", { dataTransfer });

    await wrapper.get('[data-testid="strategy-side-panel-declaration-title"]').trigger(
      "dragover",
      { clientY: 1, dataTransfer },
    );
    await wrapper.get('[data-testid="strategy-side-panel-diagnostics-title"]').trigger(
      "dragstart",
      { dataTransfer },
    );
    await wrapper.get('[data-testid="strategy-side-panel-instances-title"]').trigger(
      "dragover",
      { clientY: 1, dataTransfer },
    );
    await wrapper.get(".strategy-native-side-panels").trigger("dragend");

    expect(dataTransfer.setData).toHaveBeenCalledWith("text/plain", "definition");
    expect((definitionPanel.element as HTMLElement).style.order).toBe("1");
    expect((historyPanel.element as HTMLElement).style.order).toBe("0");
    expect(historyPanel.classes()).toContain("is-first-panel");
    expect(definitionPanel.classes()).not.toContain("is-first-panel");
    expect(definitionPanel.classes()).toContain("is-fill-panel");

    await wrapper.get('button[aria-label="关闭策略信息"]').trigger("click");
    expect(wrapper.get('[data-testid="strategy-design-stage"]').classes()).toContain(
      "strategy-native-page--metadata-closed",
    );

    const setup = wrapper.getComponent(StrategyDesignStage).vm.$.setupState as Record<string, unknown>;
    setup.expandedStrategySidePanels = ["definition", "history", "declaration"];
    await nextTick();
    expect(wrapper.get(".strategy-native-side-panels").classes()).toContain(
      "is-space-constrained",
    );
    setup.expandedStrategySidePanels = ["definition", "history"];
    await nextTick();
    expect(wrapper.get(".strategy-native-side-panels").classes()).not.toContain(
      "is-space-constrained",
    );
  });

  it("shows immutable strategy history and opens a two-version comparison URL", async () => {
    const workflow = createDefaultPineV6Workflow("Versioned strategy");
    const baseFetch = buildFetchMock({
      definitions: [{
        id: "versioned",
        name: "Versioned strategy",
        version: "0.1.1",
        description: "Version history",
        runtime: "pine-pinets",
        sourceFormat: "pine-v6",
        script: buildPineV6WorkflowScript(workflow),
        visualModel: workflow,
        createdAt: "2026-07-01T00:00:00.000Z",
        updatedAt: "2026-07-03T00:00:00.000Z",
      }],
      versionsByDefinitionId: {
        versioned: [
          {
            version: "0.1.1",
            savedAt: "2026-07-03T00:00:00.000Z",
            isCurrent: true,
          },
          {
            version: "0.1.0",
            savedAt: "2026-07-01T00:00:00.000Z",
            snapshot: { script: '//@version=6\nstrategy("Versioned strategy")' },
          },
        ],
      },
      strategies: [],
    });
    vi.stubGlobal("fetch", baseFetch);
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);

    const { router, wrapper } = await mountStrategyPage("/strategy/design");
    await settleStrategyWorkspace();

    const versionInput = findFieldByLabel(wrapper, "版本");
    expect(versionInput.attributes("readonly")).toBeDefined();
    expect(wrapper.text()).toContain("版本历史");
    expect(wrapper.text()).toContain("v0.1.0");
    expect(wrapper.text()).toContain("v0.1.1");

    await wrapper.get('button[aria-label="刷新版本历史"]').trigger("click");
    await settleStrategyWorkspace();

    const latestEntry = wrapper.get('[data-testid="strategy-version-entry-0.1.1"]');
    const baselineEntry = wrapper.get('[data-testid="strategy-version-entry-0.1.0"]');
    await baselineEntry.get(".strategy-native-version-entry__view").trigger("click");
    await settleStrategyWorkspace();
    expect(wrapper.text()).toContain("strategy(\"Versioned strategy\")");
    await latestEntry.get('input[type="checkbox"]').setValue(true);
    await baselineEntry.get('input[type="checkbox"]').setValue(true);
    await settleStrategyWorkspace();

    const compareButton = wrapper.get('[data-testid="strategy-open-version-comparison"]');
    expect(compareButton.attributes("disabled")).toBeUndefined();
    await compareButton.trigger("click");
    await settleStrategyWorkspace();

    expect(router.currentRoute.value.path).toBe("/backtest");
    expect(router.currentRoute.value.query).toMatchObject({
      mode: "compare",
      definitionId: "versioned",
      leftVersion: "0.1.0",
      rightVersion: "0.1.1",
    });
  });

  it("reloads version history after saving a changed strategy", async () => {
    const workflow = createDefaultPineV6Workflow("Fresh history");
    const baseFetch = buildFetchMock({
      definitions: [{
        id: "fresh-history",
        name: "Fresh history",
        version: "0.1.0",
        description: "before save",
        runtime: "pine-pinets",
        sourceFormat: "pine-v6",
        script: buildPineV6WorkflowScript(workflow),
        visualModel: workflow,
        createdAt: "2026-07-01T00:00:00.000Z",
        updatedAt: "2026-07-01T00:00:00.000Z",
      }],
      strategies: [],
    });
    let historyRequests = 0;
    const fetchMock = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      if (
        url.endsWith("/api/v1/strategy-definitions/fresh-history/versions")
        && requestMethod(input, init) === "GET"
      ) {
        historyRequests += 1;
      }
      return baseFetch(input, init);
    });
    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);

    const { wrapper } = await mountStrategyPage("/strategy/design");
    await settleStrategyWorkspace();
    expect(wrapper.text()).toContain("v0.1.0");
    expect(historyRequests).toBeGreaterThanOrEqual(1);

    await findFieldByLabel(wrapper, "说明", "textarea").setValue("after save");
    await findButtonByLabels(wrapper, ["保存", "保存中", "已保存"]).trigger("click");
    await settleStrategyWorkspace();

    expect(historyRequests).toBeGreaterThanOrEqual(2);
    expect(wrapper.text()).toContain("v0.1.1");
  });

  it("handles version-history failures, snapshot boundaries, and panel drag guards", async () => {
    const workflow = createDefaultPineV6Workflow("History boundaries");
    const baseFetch = buildFetchMock({
      definitions: [{
        id: "history-boundaries",
        name: "History boundaries",
        version: "0.2.0",
        runtime: "pine-pinets",
        sourceFormat: "pine-v6",
        script: buildPineV6WorkflowScript(workflow),
        visualModel: workflow,
        createdAt: "2026-07-01T00:00:00.000Z",
        updatedAt: "2026-07-03T00:00:00.000Z",
      }],
      versionsByDefinitionId: {
        "history-boundaries": [
          { version: "0.2.0", savedAt: "invalid", isCurrent: true, snapshot: { script: "strategy('current')" } },
          { version: "0.1.0", savedAt: "2026-07-01T00:00:00.000Z", snapshot: { script: "strategy('initial')" } },
        ],
      },
      strategies: [],
    });
    let historyFailure: unknown = null;
    let snapshotFailure: unknown = null;
    const fetchMock = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      if (historyFailure != null && url.endsWith("/history-boundaries/versions")) {
        throw historyFailure;
      }
      if (snapshotFailure != null && url.includes("/history-boundaries/versions/broken")) {
        throw snapshotFailure;
      }
      return baseFetch(input, init);
    });
    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);

    const { router, wrapper } = await mountStrategyPage("/strategy/design");
    await settleStrategyWorkspace();
    const stage = wrapper.getComponent(StrategyDesignStage);
    const setup = stage.vm.$.setupState as Record<string, unknown>;
    const call = <T>(name: string, ...args: unknown[]) =>
      (setup[name] as (...values: unknown[]) => T)(...args);

    expect(call("formatVersionSavedAt", "")).toBe("保存时间未知");
    expect(call<string>("formatVersionSavedAt", "2026-07-01T00:00:00.000Z")).not.toBe("保存时间未知");
    expect(call("isVersionSelectedForComparison", "0.1.0")).toBe(false);
    expect(call("versionSelectionDisabled", "0.1.0")).toBe(false);
    call("toggleVersionForComparison", " ");
    call("toggleVersionForComparison", "0.1.0");
    expect(call("isVersionSelectedForComparison", "0.1.0")).toBe(true);
    call("toggleVersionForComparison", "0.1.0");
    expect(call("isVersionSelectedForComparison", "0.1.0")).toBe(false);
    writeStrategySetupValue(setup, "comparisonVersionSelection", ["0.1.0", "0.2.0"]);
    expect(call("versionSelectionDisabled", "0.3.0")).toBe(true);
    expect(call("versionSelectionDisabled", "0.1.0")).toBe(false);
    call("toggleVersionForComparison", "0.3.0");
    expect(readStrategySetupValue<string[]>(setup.comparisonVersionSelection)).toEqual(["0.1.0", "0.2.0"]);

    await call<Promise<void>>("showVersionSnapshot", "0.1.0");
    expect(readStrategySetupValue<Record<string, unknown> | null>(setup.selectedVersionSnapshot)).toMatchObject({
      version: "0.1.0",
      script: "strategy('initial')",
    });
    expect(wrapper.text()).toContain("strategy('initial')");

    await call<Promise<void>>("showVersionSnapshot", " ");
    snapshotFailure = new Error("snapshot unavailable");
    await call<Promise<void>>("showVersionSnapshot", "broken");
    expect(readStrategySetupValue<string>(setup.versionSnapshotError)).toContain("snapshot unavailable");
    expect(readStrategySetupValue(setup.selectedVersionSnapshot)).toBeNull();
    snapshotFailure = "snapshot rejected";
    await call<Promise<void>>("showVersionSnapshot", "broken-string");
    expect(readStrategySetupValue<string>(setup.versionSnapshotError)).toContain("snapshot rejected");

    historyFailure = new Error("history unavailable");
    await call<Promise<void>>("loadDefinitionVersions", "history-boundaries");
    expect(readStrategySetupValue<unknown[]>(setup.definitionVersions)).toEqual([]);
    expect(readStrategySetupValue<string>(setup.definitionVersionsError)).toContain("history unavailable");
    historyFailure = "history rejected";
    await call<Promise<void>>("loadDefinitionVersions", "history-boundaries");
    expect(readStrategySetupValue<string>(setup.definitionVersionsError)).toContain("history rejected");

    writeStrategySetupValue(setup, "selectedDefinitionId", "");
    await call<Promise<void>>("loadDefinitionVersions", " ");
    expect(readStrategySetupValue<unknown[]>(setup.definitionVersions)).toEqual([]);
    call("openVersionComparison");
    expect(router.currentRoute.value.path).toBe("/strategy/design");

    call("moveStrategySidePanel", "missing-panel", 2);
    writeStrategySetupValue(setup, "isWideWorkbench", false);
    const preventDefault = vi.fn();
    call("handleStrategySidePanelDragStart", { preventDefault, dataTransfer: null }, "definition");
    expect(preventDefault).toHaveBeenCalled();
    writeStrategySetupValue(setup, "isWideWorkbench", true);
    call("handleStrategySidePanelDragOver", { currentTarget: document.createElement("div"), clientY: 0, dataTransfer: null }, "definition");
    expect(readStrategySetupValue(setup.strategySidePanelDropTarget)).toBeNull();
    call("handleStrategySidePanelDrop", { preventDefault });
    call("addSourceBlock", "invalid-kind");
    call("changeSourceBlockKind", { id: "missing" }, "invalid-kind");
    call("rememberSourceSnapshot", readStrategySetupValue<string>(setup.activeScript));
    call("commitSourceChange", readStrategySetupValue<string>(setup.activeScript));

    wrapper.unmount();
  });

  it("edits declarations, manages analyze/save feedback, and refreshes strategy instances", async () => {
    const alphaWorkflow = createDefaultPineV6Workflow("Alpha Existing");
    const betaWorkflow = createDefaultPineV6Workflow("Beta Definition");
    const baseFetch = buildFetchMock({
      definitions: [
        {
          id: "alpha",
          name: "Alpha Existing",
          version: "0.1.0",
          description: "Alpha strategy",
          runtime: "pine-pinets",
          sourceFormat: "pine-v6",
          symbol: "00700",
          interval: "5m",
          script: "",
          visualModel: alphaWorkflow,
          createdAt: "2026-07-01T00:00:00.000Z",
          updatedAt: "2026-07-01T00:00:00.000Z",
        },
        {
          id: "beta",
          name: "Beta Definition",
          version: "0.2.0",
          description: "Beta strategy",
          runtime: "pine-pinets",
          sourceFormat: "pine-v6",
          symbol: "AAPL",
          interval: "15m",
          script: buildPineV6WorkflowScript(betaWorkflow),
          visualModel: betaWorkflow,
          createdAt: "2026-07-02T00:00:00.000Z",
          updatedAt: "2026-07-02T00:00:00.000Z",
        },
      ],
      strategies: [
        {
          id: "alpha-instance",
          definition: {
            strategyId: "alpha",
            name: "Alpha Existing",
            version: "0.1.0",
          },
          binding: {
            symbols: ["SH.600519", "SZ.000001"],
            interval: "5m",
            executionMode: "live",
          },
          params: {
            definitionId: "alpha",
          },
          status: "SYNCING" as any,
          createdAt: "2026-07-01T00:00:00.000Z",
          logs: [],
        },
        {
          id: "other-instance",
          definition: {
            strategyId: "other",
            name: "Other Strategy",
            version: "0.1.0",
          },
          binding: {
            symbols: ["US.AAPL"],
            interval: "1d",
            executionMode: "live",
          },
          params: {
            definitionId: "other",
          },
          status: "STOPPED",
          createdAt: "2026-07-01T00:00:00.000Z",
          logs: [],
        },
      ],
    });

    let strategyFetchCount = 0;
    let resolveAnalyze: null | (() => Promise<void>) = null;
    let resolveSave: null | (() => Promise<void>) = null;

    const fetchMock = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      const method = requestMethod(input, init);

      if (url.endsWith("/api/v1/strategies") && method === "GET") {
        strategyFetchCount += 1;
      }

      if (url.includes("/api/v1/strategy-pine/analyze") && method === "POST") {
        return new Promise((resolve) => {
          resolveAnalyze = async () => resolve(await baseFetch(input, init));
        });
      }

      if (url.endsWith("/api/v1/strategy-definitions/alpha") && method === "PUT") {
        return new Promise((resolve) => {
          resolveSave = async () => resolve(await baseFetch(input, init));
        });
      }

      return baseFetch(input, init);
    });

    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);

    const { wrapper } = await mountStrategyPage("/strategy/design");
    await settleStrategyWorkspace();
    vi.useFakeTimers();

    expect(strategySourceEditor(wrapper).element.value).toContain(
      'strategy("Alpha Existing"',
    );
    expect(wrapper.text()).toContain("SYNCING");
    expect(wrapper.text()).toContain("Alpha Existing");

    const sourceEditor = strategySourceEditor(wrapper);
    const originalSource = sourceEditor.element.value;
    await sourceEditor.setValue(`${originalSource}\n// staged change`);
    await settleWithFakeTimers();

    const undoButton = wrapper.get('[data-testid="strategy-source-undo"]');
    const redoButton = wrapper.get('[data-testid="strategy-source-redo"]');
    await undoButton.trigger("click");
    await settleWithFakeTimers();
    await redoButton.trigger("click");
    await settleWithFakeTimers();
    expect(sourceEditor.element.value).toContain("Alpha Existing");

    await wrapper.get('[data-testid="strategy-display-mode-code"]').trigger("click");
    expect(
      wrapper.get('[data-testid="strategy-display-mode-code"]').classes(),
    ).toContain("is-active");
    await wrapper.get('[data-testid="strategy-display-mode-split"]').trigger("click");
    expect(
      wrapper.get('[data-testid="strategy-display-mode-split"]').classes(),
    ).toContain("is-active");
    await wrapper.get('[data-testid="strategy-display-mode-instruction"]').trigger("click");
    expect(
      wrapper.get('[data-testid="strategy-display-mode-instruction"]').classes(),
    ).toContain("is-active");
    await wrapper.get('[data-testid="strategy-display-mode-split"]').trigger("click");
    expect(
      wrapper.get('[data-testid="strategy-display-mode-split"]').classes(),
    ).toContain("is-active");

    const definitionSelect = findDefinitionSelect(wrapper);
    await definitionSelect.setValue("beta");
    await settleWithFakeTimers();
    expect(strategySourceEditor(wrapper).element.value).toContain(
      'strategy("Beta Definition"',
    );

    await definitionSelect.setValue("alpha");
    await settleWithFakeTimers();

    await findFieldByLabel(wrapper, "名称").setValue("");
    await findFieldByLabel(wrapper, "版本").setValue("2.0.0");
    await findFieldByLabel(wrapper, "说明", "textarea").setValue(
      "Updated existing strategy",
    );
    await wrapper.get('[data-testid="strategy-declaration-title"]').setValue(
      "Fallback Title",
    );
    await settleWithFakeTimers();

    expect(
      (findFieldByLabel(wrapper, "名称").element as HTMLInputElement).value,
    ).toBe("Fallback Title");

    const overlayToggle = wrapper
      .findAll('input[type="checkbox"]')
      .find((candidate) =>
        candidate.element.parentElement?.textContent?.includes("叠加到主图"),
      );
    if (overlayToggle == null) {
      throw new Error("Overlay toggle not found.");
    }
    await overlayToggle.setValue(false);
    await findFieldByLabel(wrapper, "初始资金").setValue("12345");
    await findFieldByLabel(wrapper, "币种").setValue("USD");
    await findFieldByLabel(wrapper, "允许加仓次数").setValue("2");
    await settleWithFakeTimers();

    const updatedScript = strategySourceEditor(wrapper).element.value;
    expect(updatedScript).toContain(
      'strategy("Fallback Title", overlay=false, initial_capital=12345, currency=USD, pyramiding=2',
    );

    await findButtonByLabels(wrapper, ["分析", "已分析", "分析中"]).trigger("click");
    await Promise.resolve();
    expect(findButtonByLabels(wrapper, ["分析中"]).exists()).toBe(true);

    await resolveAnalyze?.();
    await settleWithFakeTimers();
    expect(findButtonByLabels(wrapper, ["已分析"]).exists()).toBe(true);

    await findButtonByLabels(wrapper, ["保存", "保存中", "已保存"]).trigger("click");
    await Promise.resolve();
    expect(findButtonByLabels(wrapper, ["保存中"]).exists()).toBe(true);

    await resolveSave?.();
    await settleWithFakeTimers();
    expect(findButtonByLabels(wrapper, ["已保存"]).exists()).toBe(true);

    await vi.advanceTimersByTimeAsync(1599);
    await settleWithFakeTimers();
    expect(findButtonByLabels(wrapper, ["已保存"]).exists()).toBe(true);

    await vi.advanceTimersByTimeAsync(1);
    await settleWithFakeTimers();
    expect(findButtonByLabels(wrapper, ["保存"]).exists()).toBe(true);

    expect(strategyFetchCount).toBeGreaterThanOrEqual(1);
    const instanceSymbols = wrapper.get(
      '[data-testid="strategy-design-instance-symbols-alpha-instance"]',
    );
    expect(instanceSymbols.text()).toContain("600519");
    expect(instanceSymbols.text()).toContain("上证");
    expect(instanceSymbols.text()).toContain("000001");
    expect(instanceSymbols.text()).toContain("深证");
    expect(instanceSymbols.text()).not.toContain("SH.600519");
    expect(instanceSymbols.get('[data-instrument-id="SH.600519"]').attributes("title")).toBe(
      "SH.600519",
    );
    await wrapper.get('button[aria-label="刷新策略实例"]').trigger("click");
    await settleWithFakeTimers();
    expect(strategyFetchCount).toBeGreaterThanOrEqual(2);
  });

  it("surfaces analysis diagnostics, falls back from raw source, and reports load/save failures", async () => {
    queryClient.setDefaultOptions({
      queries: {
        gcTime: 5 * 60 * 1000,
        refetchOnMount: false,
        refetchOnWindowFocus: false,
        retry: false,
        staleTime: 30 * 1000,
      },
    });

    const baseFetch = buildFetchMock({
      definitions: [],
      strategies: [],
    });

    const fetchMock = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      const method = requestMethod(input, init);

      if (url.endsWith("/api/v1/strategy-definitions") && method === "GET") {
        throw new Error("definitions offline");
      }

      if (url.endsWith("/api/v1/strategies") && method === "GET") {
        throw new Error("strategies offline");
      }

      if (url.endsWith("/api/v1/strategy-definitions") && method === "POST") {
        throw new Error("save offline");
      }

      return baseFetch(input, init);
    });

    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);

    const { wrapper } = await mountStrategyPage("/strategy/design");
    await settleStrategyWorkspace();

    expect(wrapper.text()).toContain("加载策略定义失败: definitions offline");
    expect(wrapper.text()).toContain("暂无实例。");

    const errorBanner = wrapper.get(".strategy-native-banner--error");
    expect(errorBanner.attributes("aria-expanded")).toBe("false");
    await errorBanner.trigger("click");
    expect(errorBanner.attributes("aria-expanded")).toBe("true");

    await wrapper.get('[data-testid="strategy-source-override-toggle"]').setValue(true);
    await settleStrategyWorkspace();
    await strategySourceEditor(wrapper).setValue(
      '//@version=6\nstrategy("Collections", overlay=true)\narr = array.new_float()\narray.push(arr, close)\n',
    );
    await settleStrategyWorkspace();
    await findButtonByLabels(wrapper, ["分析", "已分析", "分析中"]).trigger("click");
    await settleStrategyWorkspace();

    expect(wrapper.text()).toContain("Pine v6 分析未通过，请先处理错误诊断。");
    expect(wrapper.text()).toContain("PINE_COLLECTION_UNSUPPORTED");
    expect(wrapper.text()).toContain("第 3 行");
    expect(wrapper.text()).toContain("Pine 分析错误 1 个");

    await strategySourceEditor(wrapper).setValue("//@version=6\n// raw only\n");
    await settleStrategyWorkspace();
    await findFieldByLabel(wrapper, "名称").setValue("");
    await wrapper.get('[data-testid="strategy-declaration-title"]').setValue(
      "Recovered Workflow",
    );
    await settleStrategyWorkspace();

    expect(
      (findFieldByLabel(wrapper, "名称").element as HTMLInputElement).value,
    ).toBe("Recovered Workflow");
    expect(strategySourceEditor(wrapper).element.value).toContain(
      'strategy("Recovered Workflow"',
    );

    await findButtonByLabels(wrapper, ["新建 Pine v6"]).trigger("click");
    await settleStrategyWorkspace();
    expect(strategySourceEditor(wrapper).element.readOnly).toBe(true);
    expect(strategySourceEditor(wrapper).element.value).toContain(
      'strategy("Pine v6 原生策略"',
    );
    expect(wrapper.text()).not.toContain("PINE_COLLECTION_UNSUPPORTED");

    await findButtonByLabels(wrapper, ["保存", "保存中", "已保存"]).trigger("click");
    await settleStrategyWorkspace();
    expect(wrapper.text()).toContain("保存策略定义失败: save offline");
  });

  it("guards source-editing no-ops and preserves a recoverable analyzer failure", async () => {
    const baseFetch = buildFetchMock({ definitions: [], strategies: [] });
    vi.stubGlobal("fetch", async (input: string | URL | Request, init?: RequestInit) => {
      if (String(input).includes("/api/v1/strategy-pine/analyze")) {
        throw new Error("analyzer transport unavailable");
      }
      return baseFetch(input, init);
    });
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);

    const { wrapper } = await mountStrategyPage("/strategy/design");
    await settleStrategyWorkspace();
    const stage = wrapper.getComponent(StrategyDesignStage);
    const setup = stage.vm.$.setupState as Record<string, unknown>;
    const call = <T>(name: string, ...args: unknown[]) =>
      (setup[name] as (...values: unknown[]) => T)(...args);
    const read = <T>(value: unknown): T =>
      value !== null && typeof value === "object" && "value" in value
        ? (value as { value: T }).value
        : value as T;

    expect(call<string>("statusLabel", "RUNNING")).toBe("运行中");
    expect(call<string>("statusLabel", "PAUSED")).toBe("已暂停");
    expect(call<string>("statusLabel", "STOPPED")).toBe("已停止");
    expect(call<string>("statusClass", "PAUSED")).toBe(
      "strategy-native-status--paused",
    );

    const before = read<string>(setup.activeScript);
	call<void>("applyDefinition", {
		visualModel: null,
		script: "",
	});
	expect(read<string>(setup.selectedDefinitionId)).toBe("");
	expect(read<string>(setup.definitionName)).toBe("");
	expect(read<string>(setup.definitionVersion)).toBe("");
	expect(read<string>(setup.definitionDescription)).toBe("");
    call<void>("addSourceBlock", "not-a-pine-block");
    call<void>("changeSourceBlockKind", { id: "missing" }, "not-a-pine-block");
    call<void>("applySourceEdit", { source: before, changed: false });
    call<void>("commitSourceChange", before);
    call<void>("undoSourceChange");
    call<void>("redoSourceChange");
    call<void>("updateSourceBlockField", { match: { type: "raw" } }, "title", "ignored");
    expect(read<string>(setup.activeScript)).toBe(before);

    await wrapper.get('[data-testid="strategy-mobile-section-code"]').trigger("click");
    expect(read<string>(setup.strategyMobileSection)).toBe("code");
    expect(read<string>(setup.strategyDisplayMode)).toBe("code");
    await wrapper.get('[data-testid="strategy-mobile-section-instruction"]').trigger("click");
    expect(read<string>(setup.strategyMobileSection)).toBe("instruction");
    expect(read<string>(setup.strategyDisplayMode)).toBe("instruction");

    await expect(call<Promise<boolean>>("analyzeCurrentScript")).resolves.toBe(false);
    expect(read<string>(setup.error)).toContain("Pine v6 分析失败: analyzer transport unavailable");
    expect(read<{ diagnostics: Array<{ message: string }> } | null>(setup.analyzeResult))
      .toMatchObject({ diagnostics: [{ message: "analyzer transport unavailable" }] });
    await expect(
      call<Promise<unknown>>("saveDefinition", { requireAnalysis: true }),
    ).resolves.toBeNull();
  });

  it("normalizes sparse and malformed analyzer diagnostics at the API boundary", async () => {
    const baseFetch = buildFetchMock({ definitions: [], strategies: [] });
    let analyzeCalls = 0;
    vi.stubGlobal("fetch", async (input: string | URL | Request, init?: RequestInit) => {
      if (String(input).includes("/api/v1/strategy-pine/analyze")) {
        analyzeCalls += 1;
        if (analyzeCalls === 1) {
          return createResponse({
            ok: true,
            diagnostics: [
              null,
              "not-an-object",
              [],
              {
                severity: "error",
                code: "PINE_ERROR",
                message: "invalid order",
                line: 3,
                column: 4,
                endLine: 3,
                endColumn: 8,
              },
              {
                severity: "warning",
                code: 42,
                message: 17,
                line: "unknown",
                column: null,
                endLine: false,
                endColumn: {},
              },
              {
                severity: "info",
                message: "analysis note",
              },
              {
                severity: "unsupported",
                message: "unknown severity",
              },
            ],
            features: ["strategy"],
          });
        }
        return createResponse({ ok: true });
      }
      return baseFetch(input, init);
    });
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);

    const { wrapper } = await mountStrategyPage("/strategy/design");
    await settleStrategyWorkspace();
    const setup = wrapper.getComponent(StrategyDesignStage).vm.$.setupState as Record<string, unknown>;
    const analyze = setup.analyzeCurrentScript as () => Promise<boolean>;

    await expect(analyze()).resolves.toBe(false);
    expect(readStrategySetupValue<unknown>(setup.analyzeResult)).toEqual({
      ok: true,
      diagnostics: [
        {
          severity: "error",
          code: "PINE_ERROR",
          message: "invalid order",
          line: 3,
          column: 4,
          endLine: 3,
          endColumn: 8,
        },
        {
          severity: "warning",
          message: "",
          line: 0,
          column: 0,
          endLine: 0,
          endColumn: 0,
        },
        {
          severity: "info",
          message: "analysis note",
          line: 0,
          column: 0,
          endLine: 0,
          endColumn: 0,
        },
        {
          severity: "info",
          message: "unknown severity",
          line: 0,
          column: 0,
          endLine: 0,
          endColumn: 0,
        },
      ],
      features: ["strategy"],
    });

    await expect(analyze()).resolves.toBe(true);
    expect(readStrategySetupValue<unknown>(setup.analyzeResult)).toEqual({
      ok: true,
      diagnostics: [],
      features: [],
    });
  });

  it("fails fast when a split strategy component is mounted without its context", () => {
    const ContextProbe = defineComponent({
      setup() {
        useStrategyDesignContext();
        return () => null;
      },
    });

    expect(() => mount(ContextProbe)).toThrow("Strategy design context is unavailable");
  });
});
