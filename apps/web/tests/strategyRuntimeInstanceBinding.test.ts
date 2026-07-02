import type {
  StrategyBrokerAccountBinding,
  StrategyInstanceItem,
} from "@/contracts";

import { describe, expect, it } from "vitest";

import type { BrokerAccountSelectionOption } from "../src/composables/consoleDataBrokerAccountSelection";
import { buildBrokerAccountSelectionKey } from "../src/composables/consoleDataBrokerAccountSelection";
import {
  bindingInstrumentsToSymbols,
  brokerAccountOptionSubtitle,
  buildStrategyBindingPayload,
  filterBrokerAccountOptions,
  formatBrokerAccountSummary,
  formatRuntimeObservationSymbols,
  formatStrategyInterval,
  formatStrategyRuntimeRiskMode,
  formatStrategyRuntimeRiskSummary,
  formatStrategySymbols,
  normalizeBindingInstruments,
  normalizeStrategyRuntimeRiskSettings,
  normalizeText,
  readStrategyBinding,
  resolveBrokerAccountOption,
  resolveBrokerAccountSelectionKey,
  splitSymbolsText,
} from "../src/components/strategy-runtime/strategyRuntimeInstanceBinding";

function createStrategy(
  overrides: Partial<StrategyInstanceItem> = {},
): StrategyInstanceItem {
  return {
    id: "instance-1",
    definition: {
      strategyId: "def-1",
      name: "Momentum",
      version: "1.0.0",
    },
    runtime: "paper",
    sourceFormat: "pine-v6",
    startable: true,
    params: {},
    status: "STOPPED",
    createdAt: "2026-07-01T00:00:00.000Z",
    logs: [],
    ...overrides,
  };
}

function createBrokerAccountOption(
  input: Partial<BrokerAccountSelectionOption> & {
    brokerId: string;
    accountId: string;
    tradingEnvironment: string;
    market: string;
  },
): BrokerAccountSelectionOption {
  return {
    selectionKey: buildBrokerAccountSelectionKey({
      brokerId: input.brokerId,
      tradingEnvironment: input.tradingEnvironment,
      accountId: input.accountId,
      market: input.market,
    }),
    source: input.source ?? "managed",
    brokerId: input.brokerId,
    accountId: input.accountId,
    displayName: input.displayName ?? input.accountId,
    tradingEnvironment: input.tradingEnvironment,
    market: input.market,
    securityFirm: input.securityFirm ?? null,
  };
}

describe("strategyRuntimeInstanceBinding", () => {
  it("normalizes instruments, symbols, and free-form symbol text", () => {
    expect(normalizeText("  momentum  ")).toBe("momentum");
    expect(normalizeText(42)).toBe("");

    const normalized = normalizeBindingInstruments([
      { market: " hk ", code: "00700 " },
      { market: "HK", code: "00700" },
      { market: "us", code: " aapl " },
      { market: "", code: "NVDA" },
    ]);

    expect(normalized).toEqual([
      { market: "HK", code: "00700" },
      { market: "US", code: "AAPL" },
    ]);
    expect(bindingInstrumentsToSymbols(normalized)).toEqual([
      "HK.00700",
      "US.AAPL",
    ]);
    expect(splitSymbolsText(" HK:00700,\nUS.AAPL；  US.TSLA\t;")).toEqual([
      "HK:00700",
      "US.AAPL",
      "US.TSLA",
    ]);
    expect(
      formatRuntimeObservationSymbols([
        " hk.00700 ",
        "HK:00700",
        "US.aapl",
        "",
        "US.AAPL",
      ]),
    ).toBe("HK.00700, US.AAPL");
    expect(formatRuntimeObservationSymbols(null)).toBe("暂无");
  });

  it("prefers explicit binding values and normalizes broker account and runtime risk", () => {
    const strategy = createStrategy({
      binding: {
        instruments: [
          { market: " hk ", code: "00700" },
          { market: "HK", code: "00700" },
        ],
        symbols: ["US.TSLA"],
        interval: " 15m ",
        executionMode: "notify_only",
        brokerAccount: {
          brokerId: " FUTU ",
          accountId: " 123456 ",
          tradingEnvironment: " real ",
          market: " hk ",
        },
        runtimeRisk: {
          mode: "monitor",
          closeOnly: true,
          maxOrderQuantity: 12.5,
          maxOrderNotional: null,
          dailyMaxOrders: 3.9,
          pauseOnReject: true,
        },
      },
      params: {
        symbols: ["US.AAPL"],
        interval: "1m",
        executionMode: "live",
      },
    });

    const binding = readStrategyBinding(strategy);

    expect(binding).toEqual({
      instruments: [{ market: "HK", code: "00700" }],
      symbols: ["HK.00700"],
      interval: "15m",
      executionMode: "notify_only",
      brokerAccount: {
        brokerId: "futu",
        accountId: "123456",
        tradingEnvironment: "REAL",
        market: "HK",
      },
      runtimeRisk: {
        mode: "monitor",
        closeOnly: true,
        maxOrderQuantity: 12.5,
        maxOrderNotional: null,
        dailyMaxOrders: 3,
        pauseOnReject: true,
      },
    });
    expect(formatStrategySymbols(strategy)).toBe("HK.00700");
    expect(formatStrategyInterval(strategy)).toBe("15m");
    expect(formatBrokerAccountSummary(binding.brokerAccount)).toBe(
      "FUTU / 实盘 / 123456 / HK",
    );
    expect(formatStrategyRuntimeRiskSummary(binding.runtimeRisk)).toContain(
      "观察",
    );
  });

  it("falls back through params instruments, symbols, and symbol when binding is absent", () => {
    const paramsInstrumentStrategy = createStrategy({
      params: {
        instruments: [
          { market: "us", code: "msft" },
          { market: "US", code: "MSFT" },
          { market: "", code: "bad" },
        ],
        interval: "",
        brokerAccount: {
          brokerId: " ib ",
          accountId: " DU123 ",
          tradingEnvironment: " simulate ",
          market: " us ",
        },
        runtimeRisk: {
          mode: "enforce",
          closeOnly: true,
          maxOrderQuantity: "15.5",
          maxOrderNotional: "20000",
          dailyMaxOrders: "8.8",
          pauseOnReject: true,
        },
      },
    });

    const symbolsStrategy = createStrategy({
      params: {
        symbols: [" hk:00005 ", "HK.00005", ""],
      },
    });

    const singleSymbolStrategy = createStrategy({
      params: {
        symbol: " us:tsla ",
      },
    });

    expect(readStrategyBinding(paramsInstrumentStrategy)).toEqual({
      instruments: [{ market: "US", code: "MSFT" }],
      symbols: ["US.MSFT"],
      interval: "5m",
      executionMode: "live",
      brokerAccount: {
        brokerId: "ib",
        accountId: "DU123",
        tradingEnvironment: "SIMULATE",
        market: "US",
      },
      runtimeRisk: {
        mode: "enforce",
        closeOnly: true,
        maxOrderQuantity: 15.5,
        maxOrderNotional: 20000,
        dailyMaxOrders: 8,
        pauseOnReject: true,
      },
    });
    expect(readStrategyBinding(symbolsStrategy).symbols).toEqual(["HK.00005"]);
    expect(readStrategyBinding(singleSymbolStrategy).symbols).toEqual([
      "US.TSLA",
    ]);
    expect(formatStrategySymbols(createStrategy())).toBe("未绑定交易代码");
    expect(formatStrategyInterval(createStrategy())).toBe("5m");
    expect(formatBrokerAccountSummary(null)).toBe("未绑定账号");
  });

  it("normalizes runtime risk settings to safe defaults when disabled or malformed", () => {
    expect(normalizeStrategyRuntimeRiskSettings(undefined)).toEqual({
      mode: "off",
      closeOnly: false,
      maxOrderQuantity: null,
      maxOrderNotional: null,
      dailyMaxOrders: null,
      pauseOnReject: false,
    });
    expect(
      normalizeStrategyRuntimeRiskSettings({
        mode: "off",
        closeOnly: true,
        maxOrderQuantity: 10,
      }),
    ).toEqual(normalizeStrategyRuntimeRiskSettings(undefined));
    expect(
      normalizeStrategyRuntimeRiskSettings({
        mode: "broken",
        closeOnly: true,
        maxOrderQuantity: -1,
        dailyMaxOrders: "0",
      }),
    ).toEqual(normalizeStrategyRuntimeRiskSettings(undefined));
    expect(formatStrategyRuntimeRiskMode("monitor")).toBe("观察");
    expect(formatStrategyRuntimeRiskMode("enforce")).toBe("执行");
    expect(formatStrategyRuntimeRiskMode("anything-else")).toBe("关闭");
  });

  it("filters and resolves broker account options using the normalized selection key", () => {
    const options = [
      createBrokerAccountOption({
        brokerId: "futu",
        accountId: "REAL-001",
        tradingEnvironment: "REAL",
        market: "HK",
        displayName: "Primary HK",
      }),
      createBrokerAccountOption({
        brokerId: "futu",
        accountId: "SIM-002",
        tradingEnvironment: "SIMULATE",
        market: "US",
        displayName: "US Paper",
      }),
    ];

    expect(brokerAccountOptionSubtitle(options[1])).toBe(
      "FUTU / 模拟盘 / SIM-002 / US",
    );
    expect(filterBrokerAccountOptions(options, "")).toEqual(options);
    expect(filterBrokerAccountOptions(options, "paper")).toEqual([options[1]]);
    expect(filterBrokerAccountOptions(options, "模拟盘")).toEqual([options[1]]);
    expect(
      resolveBrokerAccountSelectionKey(options, {
        brokerId: " FUTU ",
        accountId: " SIM-002 ",
        tradingEnvironment: " simulate ",
        market: " us ",
      }),
    ).toBe(options[1].selectionKey);
    expect(resolveBrokerAccountSelectionKey(options, null)).toBe("");
    expect(resolveBrokerAccountOption(options, options[0].selectionKey)).toEqual(
      options[0],
    );
    expect(resolveBrokerAccountOption(options, "missing")).toBeNull();
  });

  it("builds strategy binding payloads from selected or fallback broker accounts", () => {
    const options = [
      createBrokerAccountOption({
        brokerId: "futu",
        accountId: "REAL-001",
        tradingEnvironment: "REAL",
        market: "HK",
      }),
    ];

    const payload = buildStrategyBindingPayload({
      brokerAccountOptions: options,
      instruments: [
        { market: "us", code: "aapl" },
        { market: "US", code: "AAPL" },
      ],
      interval: " ",
      executionMode: "notify_only",
      brokerAccountKey: options[0].selectionKey,
      runtimeRisk: {
        mode: "enforce",
        closeOnly: true,
        maxOrderQuantity: 5,
        maxOrderNotional: 500,
        dailyMaxOrders: 8,
        pauseOnReject: true,
      },
    });

    expect(payload).toEqual({
      instruments: [{ market: "US", code: "AAPL" }],
      symbols: ["US.AAPL"],
      interval: "5m",
      executionMode: "notify_only",
      brokerAccount: {
        brokerId: "futu",
        accountId: "REAL-001",
        tradingEnvironment: "REAL",
        market: "HK",
      },
      runtimeRisk: {
        mode: "enforce",
        closeOnly: true,
        maxOrderQuantity: 5,
        maxOrderNotional: 500,
        dailyMaxOrders: 8,
        pauseOnReject: true,
      },
    });
    expect(formatStrategyRuntimeRiskSummary(payload.runtimeRisk)).toContain(
      "仅平仓",
    );
    expect(formatStrategyRuntimeRiskSummary(payload.runtimeRisk)).toContain(
      "拒单后暂停",
    );

    const fallbackBrokerAccount: StrategyBrokerAccountBinding = {
      brokerId: "futu",
      accountId: "FALLBACK-1",
      tradingEnvironment: "REAL",
      market: "US",
    };

    expect(
      buildStrategyBindingPayload({
        brokerAccountOptions: options,
        instruments: [],
        interval: "30m",
        executionMode: "live",
        brokerAccountKey: "missing",
        fallbackBrokerAccount,
        runtimeRisk: null,
      }),
    ).toEqual({
      instruments: [],
      symbols: [],
      interval: "30m",
      executionMode: "live",
      brokerAccount: fallbackBrokerAccount,
      runtimeRisk: normalizeStrategyRuntimeRiskSettings(null),
    });
  });
});
