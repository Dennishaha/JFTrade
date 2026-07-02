// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";
import { ref } from "vue";

import {
  emptyBrokerRuntime,
  emptyBrokerSettings,
  emptySystemStatus,
  type BrokerSettingsResponse,
} from "../src/contracts";
import { buildBrokerAccountSelectionKey } from "../src/composables/consoleDataBrokerAccountSelection";
import { createConsoleDataBrokerSettingsController } from "../src/composables/consoleDataBrokerSettings";
import type { WorkspaceTradingPreferences } from "../src/composables/useWorkspaceLayout";
import { createResponse } from "./helpers";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("createConsoleDataBrokerSettingsController", () => {
  it("resolves available and selected accounts from persisted workspace preferences", () => {
    const settings = buildSettings();
    const selectedKey = accountKey(settings.accounts[0]!);
    const harness = createHarness(settings, selectedKey);

    expect(harness.controller.resolveActiveBrokerId()).toBe("futu");
    expect(harness.controller.availableBrokerAccounts.value).toHaveLength(1);
    expect(harness.controller.selectedBrokerAccount.value).toMatchObject({
      accountId: "SIM-1",
      market: "HK",
      source: "managed",
    });
    expect(harness.controller.resolveBrokerQuery(harness.controller.selectedBrokerAccount.value).toString()).toBe(
      "tradingEnvironment=SIMULATE&accountId=SIM-1&market=HK",
    );
  });

  it("loads broker settings and sends manual OpenD retry before refreshing system state", async () => {
    const settings = buildSettings();
    const fetchMock = installFetch(settings);
    const harness = createHarness();

    expect(await harness.controller.loadBrokerSettings()).toEqual(settings);
    expect(harness.brokerSettings.value).toEqual(settings);

    await harness.controller.requestFutuOpenDManualRetry();

    expect(requestPath(fetchMock, 1)).toBe("/api/v1/system/futu-opend/manual-retry");
    expect(requestInit(fetchMock, 1)).toMatchObject({ method: "POST" });
    expect(JSON.parse(String(requestInit(fetchMock, 1).body))).toEqual({ brokerId: "futu" });
    expect(harness.reloadSystemState).toHaveBeenCalledWith();
  });

  it("saves an encoded broker integration and bypasses the refresh cooldown", async () => {
    const fetchMock = installFetch(undefined);
    const harness = createHarness();
    const config = {
      type: "futu" as const,
      host: "127.0.0.1",
      apiPort: 11110,
      websocketPort: 11111,
      maxWebSocketConnections: 20,
      useEncryption: false,
      websocketKey: "",
      tradeMarket: "HK",
      securityFirm: "FUTUSECURITIES",
    };

    await harness.controller.saveBrokerIntegration("futu/paper", {
      enabled: true,
      config,
    });

    expect(requestPath(fetchMock, 0)).toBe(
      "/api/v1/settings/brokers/futu%2Fpaper/integration",
    );
    expect(requestInit(fetchMock, 0).method).toBe("PUT");
    expect(JSON.parse(String(requestInit(fetchMock, 0).body))).toEqual({
      enabled: true,
      config,
    });
    expect(harness.reloadSystemState).toHaveBeenCalledWith({ bypassCooldown: true });
  });

  it("creates and updates managed accounts with exact broker payloads", async () => {
    const fetchMock = installFetch(undefined);
    const harness = createHarness();
    const payload = {
      brokerId: "futu",
      accountId: "REAL-2",
      displayName: "US real",
      tradingEnvironment: "REAL",
      market: "US",
      securityFirm: "FUTUINC",
      enabled: true,
    };

    await harness.controller.createManagedBrokerAccount(payload);
    await harness.controller.updateManagedBrokerAccount("account/2", {
      ...payload,
      enabled: false,
    });

    expect(requestPath(fetchMock, 0)).toBe("/api/v1/settings/broker-accounts");
    expect(requestInit(fetchMock, 0).method).toBe("POST");
    expect(JSON.parse(String(requestInit(fetchMock, 0).body))).toEqual(payload);
    expect(requestPath(fetchMock, 1)).toBe(
      "/api/v1/settings/broker-accounts/account%2F2",
    );
    expect(requestInit(fetchMock, 1).method).toBe("PUT");
    expect(JSON.parse(String(requestInit(fetchMock, 1).body))).toEqual({
      ...payload,
      enabled: false,
    });
    expect(harness.reloadSystemState).toHaveBeenCalledTimes(2);
  });

  it("clears a deleted selected account before refreshing broker data", async () => {
    const settings = buildSettings();
    const selectedKey = accountKey(settings.accounts[0]!);
    const fetchMock = installFetch(undefined);
    const harness = createHarness(settings, selectedKey);

    await harness.controller.deleteManagedBrokerAccount("managed-1");

    expect(requestPath(fetchMock, 0)).toBe(
      "/api/v1/settings/broker-accounts/managed-1",
    );
    expect(requestInit(fetchMock, 0).method).toBe("DELETE");
    expect(harness.update).toHaveBeenCalledWith({ selectedBrokerAccountKey: null });
    expect(harness.prefs.value.selectedBrokerAccountKey).toBeNull();
    expect(harness.reloadSystemState).toHaveBeenCalledOnce();
  });

  it("keeps unrelated account selection and refreshes after explicit selection", async () => {
    const settings = buildSettings();
    const fetchMock = installFetch(undefined);
    const harness = createHarness(settings, "other-key");

    await harness.controller.deleteManagedBrokerAccount("managed-1");
    expect(harness.update).not.toHaveBeenCalled();

    await harness.controller.selectBrokerAccount(accountKey(settings.accounts[0]!));
    expect(harness.prefs.value.selectedBrokerAccountKey).toBe(accountKey(settings.accounts[0]!));
    expect(harness.reloadSystemState).toHaveBeenLastCalledWith({ bypassCooldown: true });
    expect(fetchMock).toHaveBeenCalledOnce();
  });
});

function createHarness(
  settings: BrokerSettingsResponse = emptyBrokerSettings,
  selectedBrokerAccountKey: string | null = null,
) {
  const prefs = ref({ selectedBrokerAccountKey } as WorkspaceTradingPreferences);
  const update = vi.fn((patch: Partial<WorkspaceTradingPreferences>) => {
    Object.assign(prefs.value, patch);
  });
  const brokerSettings = ref(settings);
  const reloadSystemState = vi.fn(async () => {});
  return {
    controller: createConsoleDataBrokerSettingsController({
      prefs,
      update,
      brokerSettings,
      brokerRuntime: ref({
        ...emptyBrokerRuntime,
        descriptor: { ...emptyBrokerRuntime.descriptor, id: "futu" },
        accounts: [
          {
            accountId: "SIM-1",
            tradingEnvironment: "SIMULATE",
            accountType: "CASH",
            accountRole: null,
            securityFirm: "FUTUSECURITIES",
            marketAuthorities: ["HK"],
            simulatedAccountType: "STOCK",
          },
        ],
      }),
      systemStatus: ref({
        ...emptySystemStatus,
        defaultBroker: "futu",
        defaultTradingEnvironment: "SIMULATE",
      }),
      reloadSystemState,
    }),
    prefs,
    update,
    brokerSettings,
    reloadSystemState,
  };
}

function buildSettings(): BrokerSettingsResponse {
  return {
    brokers: [],
    accounts: [
      {
        id: "managed-1",
        brokerId: "futu",
        accountId: "SIM-1",
        displayName: "Simulation",
        tradingEnvironment: "SIMULATE",
        market: "HK",
        securityFirm: "FUTUSECURITIES",
        enabled: true,
        createdAt: "2026-01-01T00:00:00Z",
        updatedAt: "2026-01-01T00:00:00Z",
      },
    ],
  };
}

function accountKey(account: BrokerSettingsResponse["accounts"][number]): string {
  return buildBrokerAccountSelectionKey(account);
}

type FetchMock = ReturnType<typeof vi.fn>;

function installFetch(settings: BrokerSettingsResponse | undefined): FetchMock {
  const fetchMock = vi.fn(async (input: RequestInfo | URL) =>
    createResponse(
      String(input).endsWith("/api/v1/settings/brokers") ? settings : undefined,
    ),
  );
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

function requestPath(fetchMock: FetchMock, index: number): string {
  return new URL(String(fetchMock.mock.calls[index]?.[0]), "http://localhost").pathname;
}

function requestInit(fetchMock: FetchMock, index: number): RequestInit {
  return (fetchMock.mock.calls[index]?.[1] ?? {}) as RequestInit;
}
