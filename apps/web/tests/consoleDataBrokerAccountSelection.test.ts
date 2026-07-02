import { describe, expect, it } from "vitest";

import {
  emptyBrokerRuntime,
  emptyBrokerSettings,
  emptySystemStatus,
} from "@/contracts";

import {
  buildBrokerAccountSelectionKey,
  resolveActiveBrokerId,
  resolveBrokerAccountOptions,
  resolveBrokerQuery,
  resolveSelectedBrokerAccountOption,
} from "../src/composables/consoleDataBrokerAccountSelection";

describe("consoleDataBrokerAccountSelection", () => {
  it("deduplicates runtime accounts that match enabled managed accounts", () => {
    const options = resolveBrokerAccountOptions({
      activeBrokerId: "futu",
      settings: {
        ...emptyBrokerSettings,
        accounts: [
          {
            id: "acct-sim",
            brokerId: "futu",
            accountId: "SIM-001",
            displayName: "Primary sim",
            tradingEnvironment: "SIMULATE",
            market: "HK",
            securityFirm: "FUTUSECURITIES",
            enabled: true,
            updatedAt: "2026-05-17T00:00:00.000Z",
            createdAt: "2026-05-17T00:00:00.000Z",
          },
        ],
      },
      runtime: {
        ...emptyBrokerRuntime,
        descriptor: {
          ...emptyBrokerRuntime.descriptor,
          id: "futu",
        },
        accounts: [
          {
            accountId: "SIM-001",
            tradingEnvironment: "SIMULATE",
            accountType: "CASH",
            accountRole: null,
            securityFirm: "FUTUSECURITIES",
            marketAuthorities: ["HK"],
            simulatedAccountType: "STOCK",
          },
          {
            accountId: "REAL-001",
            tradingEnvironment: "REAL",
            accountType: "CASH",
            accountRole: null,
            securityFirm: "FUTUSECURITIES",
            marketAuthorities: ["US"],
            simulatedAccountType: null,
          },
        ],
      },
      fallbackMarket: "HK",
    });

    expect(options).toEqual([
      {
        selectionKey: buildBrokerAccountSelectionKey({
          brokerId: "futu",
          tradingEnvironment: "SIMULATE",
          accountId: "SIM-001",
          market: "HK",
        }),
        source: "managed",
        brokerId: "futu",
        accountId: "SIM-001",
        displayName: "Primary sim",
        tradingEnvironment: "SIMULATE",
        market: "HK",
        securityFirm: "FUTUSECURITIES",
      },
      {
        selectionKey: buildBrokerAccountSelectionKey({
          brokerId: "futu",
          tradingEnvironment: "REAL",
          accountId: "REAL-001",
          market: "US",
        }),
        source: "runtime",
        brokerId: "futu",
        accountId: "REAL-001",
        displayName: "REAL-001",
        tradingEnvironment: "REAL",
        market: "US",
        securityFirm: "FUTUSECURITIES",
      },
    ]);
  });

  it("expands runtime accounts into one option per market authority", () => {
    const options = resolveBrokerAccountOptions({
      activeBrokerId: "futu",
      settings: {
        ...emptyBrokerSettings,
        accounts: [],
      },
      runtime: {
        ...emptyBrokerRuntime,
        descriptor: {
          ...emptyBrokerRuntime.descriptor,
          id: "futu",
        },
        accounts: [
          {
            accountId: "REAL-UTA-001",
            tradingEnvironment: "REAL",
            accountType: "UNKNOWN",
            accountRole: null,
            securityFirm: "FUTUSECURITIES",
            marketAuthorities: ["HK", "US", "HK"],
            simulatedAccountType: null,
          },
        ],
      },
      fallbackMarket: "HK",
    });

    expect(options).toEqual([
      {
        selectionKey: buildBrokerAccountSelectionKey({
          brokerId: "futu",
          tradingEnvironment: "REAL",
          accountId: "REAL-UTA-001",
          market: "HK",
        }),
        source: "runtime",
        brokerId: "futu",
        accountId: "REAL-UTA-001",
        displayName: "REAL-UTA-001",
        tradingEnvironment: "REAL",
        market: "HK",
        securityFirm: "FUTUSECURITIES",
      },
      {
        selectionKey: buildBrokerAccountSelectionKey({
          brokerId: "futu",
          tradingEnvironment: "REAL",
          accountId: "REAL-UTA-001",
          market: "US",
        }),
        source: "runtime",
        brokerId: "futu",
        accountId: "REAL-UTA-001",
        displayName: "REAL-UTA-001",
        tradingEnvironment: "REAL",
        market: "US",
        securityFirm: "FUTUSECURITIES",
      },
    ]);
  });

  it("prefers persisted selection and uses its broker as the active broker", () => {
    const selectionOptions = [
      {
        selectionKey: buildBrokerAccountSelectionKey({
          brokerId: "futu",
          tradingEnvironment: "SIMULATE",
          accountId: "SIM-001",
          market: "HK",
        }),
        source: "managed" as const,
        brokerId: "futu",
        accountId: "SIM-001",
        displayName: "Primary sim",
        tradingEnvironment: "SIMULATE",
        market: "HK",
        securityFirm: "FUTUSECURITIES",
      },
      {
        selectionKey: buildBrokerAccountSelectionKey({
          brokerId: "ib",
          tradingEnvironment: "REAL",
          accountId: "U123",
          market: "US",
        }),
        source: "managed" as const,
        brokerId: "ib",
        accountId: "U123",
        displayName: "US real",
        tradingEnvironment: "REAL",
        market: "US",
        securityFirm: null,
      },
    ];
    const selectedBrokerAccountKey = selectionOptions[1].selectionKey;

    expect(
      resolveActiveBrokerId({
        selectedBrokerAccountKey,
        settings: emptyBrokerSettings,
        status: {
          ...emptySystemStatus,
          defaultBroker: "futu",
        },
      }),
    ).toBe("ib");

    expect(
      resolveSelectedBrokerAccountOption({
        selectionOptions,
        selectedBrokerAccountKey,
        activeBrokerId: "futu",
        defaultTradingEnvironment: "SIMULATE",
      }),
    ).toBe(selectionOptions[1]);
  });

  it("encodes account keys and falls back through configured broker priorities", () => {
    const encodedKey = buildBrokerAccountSelectionKey({
      brokerId: "ib/gateway",
      tradingEnvironment: "REAL",
      accountId: "U 123",
      market: "US",
    });
    expect(encodedKey).toBe("ib%2Fgateway|REAL|U%20123|US");
    expect(
      resolveActiveBrokerId({
        selectedBrokerAccountKey: encodedKey,
        settings: emptyBrokerSettings,
        status: emptySystemStatus,
      }),
    ).toBe("ib/gateway");

    expect(
      resolveActiveBrokerId({
        selectedBrokerAccountKey: "",
        settings: {
          ...emptyBrokerSettings,
          accounts: [
            {
              id: "disabled",
              brokerId: "disabled-broker",
              accountId: "1",
              displayName: "Disabled",
              tradingEnvironment: "SIMULATE",
              market: "HK",
              securityFirm: null,
              enabled: false,
              createdAt: "",
              updatedAt: "",
            },
            {
              id: "enabled",
              brokerId: "managed-broker",
              accountId: "2",
              displayName: "Enabled",
              tradingEnvironment: "REAL",
              market: "US",
              securityFirm: null,
              enabled: true,
              createdAt: "",
              updatedAt: "",
            },
          ],
        },
        status: emptySystemStatus,
      }),
    ).toBe("managed-broker");
  });

  it("skips disabled managed accounts and ignores runtime accounts from another broker", () => {
    const options = resolveBrokerAccountOptions({
      activeBrokerId: "ib",
      settings: {
        ...emptyBrokerSettings,
        accounts: [
          {
            id: "disabled",
            brokerId: "ib",
            accountId: "U1",
            displayName: "Disabled",
            tradingEnvironment: "REAL",
            market: "US",
            securityFirm: null,
            enabled: false,
            createdAt: "",
            updatedAt: "",
          },
        ],
      },
      runtime: {
        ...emptyBrokerRuntime,
        descriptor: { ...emptyBrokerRuntime.descriptor, id: "futu" },
        accounts: [
          {
            accountId: "SIM-1",
            tradingEnvironment: "SIMULATE",
            accountType: "CASH",
            accountRole: null,
            securityFirm: null,
            marketAuthorities: [],
            simulatedAccountType: "STOCK",
          },
        ],
      },
      fallbackMarket: "US",
    });

    expect(options).toEqual([]);
  });

  it("uses the fallback market for runtime accounts without authorities", () => {
    const options = resolveBrokerAccountOptions({
      activeBrokerId: "futu",
      settings: emptyBrokerSettings,
      runtime: {
        ...emptyBrokerRuntime,
        descriptor: { ...emptyBrokerRuntime.descriptor, id: "futu" },
        accounts: [
          {
            accountId: "SIM-NO-MARKET",
            tradingEnvironment: "SIMULATE",
            accountType: "CASH",
            accountRole: null,
            securityFirm: null,
            marketAuthorities: [],
            simulatedAccountType: "STOCK",
          },
        ],
      },
      fallbackMarket: "HK",
    });

    expect(options[0]).toMatchObject({
      source: "runtime",
      accountId: "SIM-NO-MARKET",
      market: "HK",
    });
  });

  it("selects the environment match, active broker fallback, then first account", () => {
    const options = [
      {
        selectionKey: "ib-real",
        source: "managed" as const,
        brokerId: "ib",
        accountId: "U1",
        displayName: "IB real",
        tradingEnvironment: "REAL",
        market: "US",
        securityFirm: null,
      },
      {
        selectionKey: "futu-sim",
        source: "managed" as const,
        brokerId: "futu",
        accountId: "SIM1",
        displayName: "Futu sim",
        tradingEnvironment: "SIMULATE",
        market: "HK",
        securityFirm: null,
      },
    ];

    expect(
      resolveSelectedBrokerAccountOption({
        selectionOptions: options,
        selectedBrokerAccountKey: "missing",
        activeBrokerId: "futu",
        defaultTradingEnvironment: "SIMULATE",
      }),
    ).toBe(options[1]);
    expect(
      resolveSelectedBrokerAccountOption({
        selectionOptions: options,
        selectedBrokerAccountKey: null,
        activeBrokerId: "missing",
        defaultTradingEnvironment: "REAL",
      }),
    ).toBe(options[0]);
    expect(
      resolveSelectedBrokerAccountOption({
        selectionOptions: [],
        selectedBrokerAccountKey: null,
        activeBrokerId: "futu",
        defaultTradingEnvironment: "SIMULATE",
      }),
    ).toBeNull();
  });

  it("builds broker queries from an exact runtime selection or runtime defaults", () => {
    const runtime = {
      ...emptyBrokerRuntime,
      descriptor: {
        ...emptyBrokerRuntime.descriptor,
        id: "futu",
        capabilities: [{ market: "US", supportsQuote: true, supportsTrade: true }],
      },
      accounts: [
        {
          accountId: "SIM-1",
          tradingEnvironment: "SIMULATE",
          accountType: "CASH",
          accountRole: null,
          securityFirm: null,
          marketAuthorities: ["HK"],
          simulatedAccountType: "STOCK",
        },
      ],
    };
    const selection = {
      selectionKey: "futu-real",
      source: "managed" as const,
      brokerId: "futu",
      accountId: "REAL-2",
      displayName: "Real",
      tradingEnvironment: "REAL",
      market: "US",
      securityFirm: null,
    };

    expect(
      resolveBrokerQuery({ selection, runtime, status: emptySystemStatus }).toString(),
    ).toBe("tradingEnvironment=REAL&accountId=REAL-2&market=US");
    expect(
      resolveBrokerQuery({
        selection: { ...selection, brokerId: "ib" },
        runtime,
        status: { ...emptySystemStatus, defaultTradingEnvironment: "SIMULATE" },
      }).toString(),
    ).toBe("tradingEnvironment=SIMULATE&accountId=SIM-1&market=HK");
  });
});
