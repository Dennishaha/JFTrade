// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";
import { nextTick } from "vue";

import {
  buildPineV6WorkflowScript,
  createDefaultPineV6Workflow,
} from "../src/features/pineV6Workflow";
import { queryClient } from "../src/composables/serverState";
import StrategyDesignStage from "../src/components/StrategyDesignStage.vue";
import {
  MockWebSocket,
  buildFetchMock,
  mountStrategyPage,
  resetStrategyPageTestState,
  settleStrategyWorkspace,
} from "./strategyPageTestUtils";

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

describe("StrategyDesignStage business flows", () => {
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
});
