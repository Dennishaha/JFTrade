import { effectScope, nextTick, ref } from "vue";
import { afterEach, describe, expect, it, vi } from "vitest";

import type {
  NormalizeInstrumentRequest,
  StrategyDefinitionDocument,
  StrategyInstanceBindingDocument,
  StrategyInstanceItem,
} from "../src/contracts";
import type { BrokerAccountSelectionOption } from "../src/composables/consoleDataBrokerAccountSelection";
import { useStrategyRuntimeInstanceEditor } from "../src/components/strategy-runtime/useStrategyRuntimeInstanceEditor";

const definitions: StrategyDefinitionDocument[] = [
  {
    id: "def-1",
    name: "Breakout",
    version: "1.2.0",
    description: "",
    runtime: "pine-v6",
    script: "strategy('Breakout')",
    createdAt: "2026-07-01T00:00:00Z",
    updatedAt: "2026-07-01T00:00:00Z",
  },
  {
    id: "def-2",
    name: "Mean Revert",
    version: "2.0.0",
    description: "",
    runtime: "pine-v6",
    script: "strategy('Mean Revert')",
    createdAt: "2026-07-01T00:00:00Z",
    updatedAt: "2026-07-01T00:00:00Z",
  },
];

const accounts: BrokerAccountSelectionOption[] = [
  {
    selectionKey: "futu|SIMULATE|acc-us|US",
    source: "managed",
    brokerId: "futu",
    accountId: "acc-us",
    displayName: "US Paper",
    tradingEnvironment: "SIMULATE",
    market: "US",
    securityFirm: "FUTUSECURITIES",
  },
  {
    selectionKey: "futu|REAL|acc-hk|HK",
    source: "managed",
    brokerId: "futu",
    accountId: "acc-hk",
    displayName: "HK Live",
    tradingEnvironment: "REAL",
    market: "HK",
    securityFirm: null,
  },
];

const selectedStrategy: StrategyInstanceItem = {
  id: "inst-1",
  definition: { strategyId: "def-1", name: "Breakout", version: "1.1.0" },
  runtime: "pine-v6",
  sourceFormat: "pine-v6",
  startable: true,
  params: {},
  status: "STOPPED",
  createdAt: "2026-07-01T00:00:00Z",
  logs: [],
};

const selectedBinding: StrategyInstanceBindingDocument = {
  instruments: [{ market: "HK", code: "00700" }],
  symbols: ["HK.00700"],
  interval: "15m",
  executionMode: "notify_only",
  brokerAccount: {
    brokerId: "futu",
    accountId: "acc-hk",
    tradingEnvironment: "REAL",
    market: "HK",
  },
  runtimeRisk: {
    mode: "enforce",
    closeOnly: true,
    maxOrderQuantity: 100,
    maxOrderNotional: 50000,
    dailyMaxOrders: 8,
    pauseOnReject: true,
  },
};

const scopes: ReturnType<typeof effectScope>[] = [];

afterEach(() => {
  for (const scope of scopes.splice(0)) scope.stop();
});

function createEditor(input: {
  pendingDefinitionId?: string;
  selected?: StrategyInstanceItem | null;
  binding?: StrategyInstanceBindingDocument | null;
  resolver?: (request: NormalizeInstrumentRequest) => Promise<{
    market: string;
    prefix: string;
    code: string;
    symbol: string;
    instrumentId: string;
    resolvedMarket: string;
  }>;
} = {}) {
  const scope = effectScope();
  scopes.push(scope);
  const strategyDefinitions = ref([...definitions]);
  const selected = ref<StrategyInstanceItem | null>(input.selected ?? null);
  const binding = ref<StrategyInstanceBindingDocument | null>(input.binding ?? null);
  const brokerAccountOptions = ref([...accounts]);
  const selectedBrokerAccount = ref<BrokerAccountSelectionOption | null>(accounts[0] ?? null);
  const defaultBrokerAccountSelectionKey = ref(accounts[0]?.selectionKey ?? "");
  let pendingDefinitionId = input.pendingDefinitionId;
  const pendingSelected = vi.fn(() => {
    pendingDefinitionId = undefined;
  });
  const resolver = vi.fn(input.resolver ?? (async (request: NormalizeInstrumentRequest) => {
    const raw = String(request.instrumentId ?? request.code ?? "").trim().toUpperCase();
    if (raw === "BAD" || raw === "US.BAD") throw new Error("unknown instrument");
    const [qualifiedMarket, qualifiedCode] = raw.includes(".") ? raw.split(".", 2) : [];
    const prefix = qualifiedMarket || String(request.market ?? "HK").trim().toUpperCase();
    const code = qualifiedCode || raw;
    return {
      market: prefix,
      prefix,
      code,
      symbol: `${prefix}.${code}`,
      instrumentId: `${prefix}.${code}`,
      resolvedMarket: prefix,
    };
  }));

  const editor = scope.run(() => useStrategyRuntimeInstanceEditor({
    strategyDefinitions,
    selectedStrategy: selected,
    selectedStrategyBinding: binding,
    brokerAccountOptions,
    selectedBrokerAccount,
    defaultBrokerAccountSelectionKey,
    pendingDefinitionId: () => pendingDefinitionId,
    onPendingDefinitionSelected: pendingSelected,
    normalizeInstrumentRefWithMarketApi: resolver,
  }));
  if (editor == null) throw new Error("editor scope failed");
  return {
    editor,
    strategyDefinitions,
    selected,
    binding,
    brokerAccountOptions,
    selectedBrokerAccount,
    defaultBrokerAccountSelectionKey,
    resolver,
    pendingSelected,
  };
}

describe("strategy runtime instance editor", () => {
  it("opens a pending definition and maintains create-form summaries", async () => {
    const { editor, pendingSelected } = createEditor({ pendingDefinitionId: " def-2 " });
    await nextTick();

    expect(editor.instanceEditorMode.value).toBe("create");
    expect(editor.createDefinitionId.value).toBe("def-2");
    expect(editor.instanceEditorPreviewDefinitionLabel.value).toBe("Mean Revert / v2.0.0");
    expect(editor.instanceEditorTitle.value).toBe("新增实例");
    expect(editor.instanceEditorHint.value).toContain("实例负责绑定");
    expect(editor.activeInstanceEditorSymbolsSummary.value).toBe("暂未绑定交易代码");
    expect(editor.activeInstanceEditorBrokerAccountSummary.value).toContain("acc-us");
    expect(pendingSelected).toHaveBeenCalledOnce();

    editor.instanceEditorOpen.value = false;
    expect(editor.instanceEditorMode.value).toBeNull();
    editor.openCreateInstanceForm();
    expect(editor.activeSymbolDraftMarket.value).toBe("US");
  });

  it("normalizes mixed symbol drafts and reports only invalid instruments", async () => {
    const { editor, resolver } = createEditor();
    editor.openCreateInstanceForm();
    editor.updateActiveSymbolDraftMarket(" us ");
    editor.updateActiveSymbolDraft("AAPL, bad; HK.00700");

    await expect(editor.commitSymbolDraft("create")).resolves.toBe(false);

    expect(editor.activeSymbolTags.value).toEqual(["US.AAPL", "HK.00700"]);
    expect(editor.activeSymbolDraft.value).toBe("");
    expect(editor.activeSymbolDraftMarket.value).toBe("HK");
    expect(editor.activeSymbolValidationMessage.value).toContain("BAD");
    expect(resolver).toHaveBeenCalledTimes(3);

    editor.removeActiveSymbol("US.AAPL");
    expect(editor.activeSymbolTags.value).toEqual(["HK.00700"]);
    editor.updateActiveSymbolDraft("MSFT");
    await expect(editor.commitSymbolDraft("create")).resolves.toBe(true);
    expect(editor.activeSymbolValidationMessage.value).toBe("");
    expect(editor.activeSymbolTags.value).toEqual(["HK.00700", "HK.MSFT"]);
  });

  it("stores the actual A-share exchange after an explicit resolver selection", () => {
    const { editor } = createEditor();
    editor.openCreateInstanceForm();

    editor.acceptActiveResolvedInstrument({
      market: "SZ",
      resolvedMarket: "CN",
      instrumentId: "SZ.000001",
      code: "000001",
      symbol: "000001",
      name: "平安银行",
      securityType: "STOCK",
      lotSize: 100,
      source: "test-static",
    });

    expect(editor.activeSymbolDraftMarket.value).toBe("CN");
    expect(editor.createBindingInstruments.value).toEqual([
      { market: "SZ", code: "000001" },
    ]);
    expect(editor.activeSymbolTags.value).toEqual(["SZ.000001"]);
  });

  it("handles keyboard and paste editing semantics", async () => {
    const { editor } = createEditor();
    editor.openCreateInstanceForm();
    editor.updateActiveSymbolDraftMarket("US");
    editor.updateActiveSymbolDraft("AAPL");

    const composingPrevent = vi.fn();
    editor.handleActiveSymbolDraftKeydown({ isComposing: true, key: "Enter", preventDefault: composingPrevent } as unknown as KeyboardEvent);
    expect(composingPrevent).not.toHaveBeenCalled();

    const enterPrevent = vi.fn();
    editor.handleActiveSymbolDraftKeydown({ isComposing: false, key: "Enter", preventDefault: enterPrevent } as unknown as KeyboardEvent);
    expect(enterPrevent).toHaveBeenCalledOnce();
    await vi.waitFor(() => expect(editor.activeSymbolTags.value).toContain("US.AAPL"));

    editor.updateActiveSymbolDraft("");
    const backspacePrevent = vi.fn();
    editor.handleActiveSymbolDraftKeydown({ isComposing: false, key: "Backspace", preventDefault: backspacePrevent } as unknown as KeyboardEvent);
    expect(backspacePrevent).toHaveBeenCalledOnce();
    expect(editor.activeSymbolTags.value).toEqual([]);

    const pastePrevent = vi.fn();
    editor.handleActiveSymbolDraftPaste({
      clipboardData: { getData: () => "AAPL\nMSFT" },
      preventDefault: pastePrevent,
    } as unknown as ClipboardEvent);
    expect(pastePrevent).toHaveBeenCalledOnce();
    await vi.waitFor(() => expect(editor.activeSymbolTags.value).toEqual(["US.AAPL", "US.MSFT"]));

    const singlePastePrevent = vi.fn();
    editor.handleActiveSymbolDraftPaste({
      clipboardData: { getData: () => "NVDA" },
      preventDefault: singlePastePrevent,
    } as unknown as ClipboardEvent);
    expect(singlePastePrevent).not.toHaveBeenCalled();
  });

  it("updates create risk, execution, interval, and broker account controls", () => {
    const { editor } = createEditor();
    editor.openCreateInstanceForm();

    editor.updateActiveIntervalValue("1h");
    editor.updateActiveExecutionMode("notify_only");
    editor.updateActiveRuntimeRiskMode("enforce");
    editor.updateActiveRuntimeRiskCloseOnly(true);
    editor.updateActiveRuntimeRiskPauseOnReject(true);
    editor.updateActiveRuntimeRiskNumber("maxOrderQuantity", "25");
    editor.updateActiveRuntimeRiskNumber("maxOrderNotional", "bad");
    editor.updateActiveRuntimeRiskNumber("dailyMaxOrders", "");
    expect(editor.activeIntervalValue.value).toBe("1h");
    expect(editor.activeExecutionMode.value).toBe("notify_only");
    expect(editor.activeRuntimeRisk.value).toMatchObject({
      mode: "enforce",
      closeOnly: true,
      pauseOnReject: true,
      maxOrderQuantity: 25,
      maxOrderNotional: null,
      dailyMaxOrders: null,
    });

    editor.toggleActiveBrokerAccountPicker();
    expect(editor.activeIsBrokerAccountPickerOpen.value).toBe(true);
    editor.updateActiveBrokerAccountQuery("hk live");
    expect(editor.activeFilteredBrokerAccountOptions.value.map((item) => item.selectionKey)).toEqual([
      accounts[1]?.selectionKey,
    ]);
    editor.selectActiveBrokerAccount(accounts[1]?.selectionKey ?? "");
    expect(editor.activeSelectedBrokerAccountKey.value).toBe(accounts[1]?.selectionKey);
    expect(editor.activeIsBrokerAccountPickerOpen.value).toBe(false);
    editor.clearActiveBrokerAccountSelection();
    expect(editor.activeSelectedBrokerAccountKey.value).toBe("");
    expect(editor.activeInstanceEditorBrokerAccountSummary.value).toBe("暂不绑定账号");
  });

  it("loads and edits an existing binding, then resets when selection disappears", async () => {
    const state = createEditor({ selected: selectedStrategy, binding: selectedBinding });
    const { editor } = state;
    editor.openEditInstanceForm();

    expect(editor.instanceEditorMode.value).toBe("edit");
    expect(editor.instanceEditorTitle.value).toBe("实例绑定");
    expect(editor.instanceEditorHint.value).toContain("独立绑定");
    expect(editor.instanceEditorPreviewDefinitionLabel.value).toBe("Breakout / v1.1.0");
    expect(editor.activeSymbolTags.value).toEqual(["HK.00700"]);
    expect(editor.activeIntervalValue.value).toBe("15m");
    expect(editor.activeExecutionMode.value).toBe("notify_only");
    expect(editor.activeSelectedBrokerAccountKey.value).toBe(accounts[1]?.selectionKey);

    editor.updateActiveSymbolDraftMarket("");
    editor.updateActiveSymbolDraft("US.AAPL");
    await expect(editor.commitSymbolDraft("edit")).resolves.toBe(true);
    editor.updateActiveIntervalValue("30m");
    editor.updateActiveExecutionMode("unsupported");
    editor.updateActiveRuntimeRiskMode("unexpected");
    expect(editor.editBindingInstruments.value).toHaveLength(2);
    expect(editor.editInterval.value).toBe("30m");
    expect(editor.editExecutionMode.value).toBe("live");
    expect(editor.editRuntimeRisk.value.mode).toBe("off");

    state.selected.value = null;
    await nextTick();
    expect(editor.instanceEditorMode.value).toBeNull();

    state.binding.value = null;
    await nextTick();
    expect(editor.editBindingInstruments.value).toEqual([]);
    expect(editor.editInterval.value).toBe("5m");
    expect(editor.editExecutionMode.value).toBe("live");
  });

  it("tracks definition and account option changes without retaining stale selections", async () => {
    const state = createEditor();
    expect(state.editor.createDefinitionId.value).toBe("def-1");

    state.editor.createDefinitionId.value = "missing";
    state.strategyDefinitions.value = [definitions[1]!];
    await nextTick();
    expect(state.editor.createDefinitionId.value).toBe("def-2");

    state.strategyDefinitions.value = [];
    await nextTick();
    expect(state.editor.createDefinitionId.value).toBe("");
    expect(state.editor.instanceEditorHint.value).toContain("先回到设计区");

    state.defaultBrokerAccountSelectionKey.value = accounts[1]?.selectionKey ?? "";
    state.brokerAccountOptions.value = [accounts[1]!];
    await nextTick();
    expect(state.editor.createBrokerAccountKey.value).toBe(accounts[1]?.selectionKey);

    state.editor.openCreateInstanceForm();
    state.editor.closeInstanceEditorDialog();
    expect(state.editor.instanceEditorMode.value).toBeNull();
  });
});
