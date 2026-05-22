import { describe, expect, it } from "vitest";

import {
  emptyBrokerRuntime,
  emptyBrokerSettings,
  emptySystemStatus,
} from "@jftrade/ui-contracts";

import {
  buildBrokerAccountSelectionKey,
  resolveActiveBrokerId,
  resolveBrokerAccountOptions,
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
});