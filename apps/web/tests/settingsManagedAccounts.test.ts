import { emptyBrokerRuntime, emptyBrokerSettings } from "@jftrade/ui-contracts";
import type {
  BrokerRuntimeResponse,
  BrokerSettingsResponse,
} from "@jftrade/ui-contracts";
import { describe, expect, it, vi } from "vitest";
import { ref } from "vue";

import { createSettingsManagedAccountsController } from "../src/composables/settingsManagedAccounts";

function createBrokerSettings(
  accounts: BrokerSettingsResponse["accounts"] = [],
): BrokerSettingsResponse {
  return {
    ...emptyBrokerSettings,
    brokers: [
      {
        descriptor: emptyBrokerRuntime.descriptor,
        integration: null,
        defaults: null,
      },
    ],
    accounts,
  };
}

function createRuntimeAccount(): BrokerRuntimeResponse["accounts"][number] {
  return {
    accountId: "REAL-001",
    tradingEnvironment: "REAL",
    accountType: "CASH",
    accountRole: null,
    securityFirm: "FUTUSECURITIES",
    marketAuthorities: ["HK"],
    simulatedAccountType: "STOCK",
  };
}

describe("createSettingsManagedAccountsController", () => {
  it("imports a discovered runtime account by creating a managed account directly", async () => {
    const createManagedBrokerAccount = vi.fn(async () => undefined);
    const updateManagedBrokerAccount = vi.fn(async () => undefined);

    const controller = createSettingsManagedAccountsController({
      brokerRuntime: ref(emptyBrokerRuntime),
      brokerSettings: ref(createBrokerSettings()),
      createManagedBrokerAccount,
      updateManagedBrokerAccount,
      deleteManagedBrokerAccount: vi.fn(async () => undefined),
    });

    await controller.importRuntimeAccount(createRuntimeAccount());

    expect(createManagedBrokerAccount).toHaveBeenCalledWith({
      brokerId: "futu",
      accountId: "REAL-001",
      displayName: "REAL-001",
      tradingEnvironment: "REAL",
      market: "HK",
      securityFirm: "FUTUSECURITIES",
      enabled: true,
    });
    expect(updateManagedBrokerAccount).not.toHaveBeenCalled();
    expect(controller.savingAccount.value).toBe(false);
    expect(controller.editingAccountId.value).toBeNull();
  });

  it("imports a discovered runtime account by updating the existing managed account when it already exists", async () => {
    const createManagedBrokerAccount = vi.fn(async () => undefined);
    const updateManagedBrokerAccount = vi.fn(async () => undefined);

    const controller = createSettingsManagedAccountsController({
      brokerRuntime: ref(emptyBrokerRuntime),
      brokerSettings: ref(
        createBrokerSettings([
          {
            id: "managed-1",
            brokerId: "futu",
            accountId: "REAL-001",
            displayName: "旧名称",
            tradingEnvironment: "REAL",
            market: "HK",
            securityFirm: "FUTUSECURITIES",
            enabled: false,
            updatedAt: "2026-06-03T00:00:00.000Z",
            createdAt: "2026-06-03T00:00:00.000Z",
          },
        ]),
      ),
      createManagedBrokerAccount,
      updateManagedBrokerAccount,
      deleteManagedBrokerAccount: vi.fn(async () => undefined),
    });

    await controller.importRuntimeAccount(createRuntimeAccount());

    expect(createManagedBrokerAccount).not.toHaveBeenCalled();
    expect(updateManagedBrokerAccount).toHaveBeenCalledWith("managed-1", {
      brokerId: "futu",
      accountId: "REAL-001",
      displayName: "REAL-001",
      tradingEnvironment: "REAL",
      market: "HK",
      securityFirm: "FUTUSECURITIES",
      enabled: true,
    });
  });
});