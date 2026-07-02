import { emptyBrokerRuntime, emptyBrokerSettings } from "@/contracts";
import type {
  BrokerRuntimeResponse,
  BrokerSettingsResponse,
} from "@/contracts";
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

  it("uses configured Futu defaults when resetting and importing incomplete runtime accounts", async () => {
    const settings = createBrokerSettings();
    settings.brokers[0] = {
      descriptor: { ...emptyBrokerRuntime.descriptor, id: "futu" },
      integration: null,
      defaults: {
        type: "futu",
        host: "127.0.0.1",
        apiPort: 11110,
        websocketPort: 11111,
        maxWebSocketConnections: 20,
        useEncryption: false,
        websocketKey: "",
        tradeMarket: "US",
        securityFirm: "FUTUINC",
      },
    };
    const createManagedBrokerAccount = vi.fn(async () => undefined);
    const controller = createSettingsManagedAccountsController({
      brokerRuntime: ref(emptyBrokerRuntime),
      brokerSettings: ref(settings),
      createManagedBrokerAccount,
      updateManagedBrokerAccount: vi.fn(async () => undefined),
      deleteManagedBrokerAccount: vi.fn(async () => undefined),
    });
    const runtimeAccount = {
      ...createRuntimeAccount(),
      securityFirm: null,
      marketAuthorities: [],
    };

    await controller.importRuntimeAccount(runtimeAccount);

    expect(createManagedBrokerAccount).toHaveBeenCalledWith(
      expect.objectContaining({ market: "US", securityFirm: "FUTUINC" }),
    );
    expect(controller.accountForm).toMatchObject({
      accountId: "",
      market: "US",
      securityFirm: "FUTUINC",
      enabled: true,
    });
  });

  it("populates an existing account and submits an update with editable fields", async () => {
    const updateManagedBrokerAccount = vi.fn(async () => undefined);
    const controller = createSettingsManagedAccountsController({
      brokerRuntime: ref(emptyBrokerRuntime),
      brokerSettings: ref(createBrokerSettings()),
      createManagedBrokerAccount: vi.fn(async () => undefined),
      updateManagedBrokerAccount,
      deleteManagedBrokerAccount: vi.fn(async () => undefined),
    });
    controller.populateAccountForm({
      id: "managed-edit",
      brokerId: "futu",
      accountId: "REAL-2",
      displayName: "Trading account",
      tradingEnvironment: "REAL",
      market: "US",
      securityFirm: null,
      enabled: false,
      createdAt: "",
      updatedAt: "",
    });
    controller.accountForm.displayName = "Updated account";
    controller.accountForm.enabled = true;

    await controller.submitAccount();

    expect(updateManagedBrokerAccount).toHaveBeenCalledWith("managed-edit", {
      brokerId: "futu",
      accountId: "REAL-2",
      displayName: "Updated account",
      tradingEnvironment: "REAL",
      market: "US",
      securityFirm: "FUTUSECURITIES",
      enabled: true,
    });
    expect(controller.editingAccountId.value).toBeNull();
    expect(controller.savingAccount.value).toBe(false);
  });

  it("creates a manually entered account and releases the saving flag on failure", async () => {
    const createManagedBrokerAccount = vi
      .fn()
      .mockResolvedValueOnce(undefined)
      .mockRejectedValueOnce(new Error("duplicate account"));
    const controller = createSettingsManagedAccountsController({
      brokerRuntime: ref(emptyBrokerRuntime),
      brokerSettings: ref(createBrokerSettings()),
      createManagedBrokerAccount,
      updateManagedBrokerAccount: vi.fn(async () => undefined),
      deleteManagedBrokerAccount: vi.fn(async () => undefined),
    });
    Object.assign(controller.accountForm, {
      accountId: "SIM-2",
      displayName: "Secondary sim",
      market: "HK",
    });

    await controller.submitAccount();
    expect(createManagedBrokerAccount).toHaveBeenCalledWith(
      expect.objectContaining({ accountId: "SIM-2", displayName: "Secondary sim" }),
    );

    controller.accountForm.accountId = "SIM-2";
    await expect(controller.submitAccount()).rejects.toThrow("duplicate account");
    expect(controller.savingAccount.value).toBe(false);
  });

  it("removes accounts and resets an editor only when it targets that account", async () => {
    const deleteManagedBrokerAccount = vi.fn(async () => undefined);
    const controller = createSettingsManagedAccountsController({
      brokerRuntime: ref(emptyBrokerRuntime),
      brokerSettings: ref(createBrokerSettings()),
      createManagedBrokerAccount: vi.fn(async () => undefined),
      updateManagedBrokerAccount: vi.fn(async () => undefined),
      deleteManagedBrokerAccount,
    });
    controller.populateAccountForm({
      id: "managed-delete",
      brokerId: "futu",
      accountId: "REAL-3",
      displayName: "Delete me",
      tradingEnvironment: "REAL",
      market: "HK",
      securityFirm: "FUTUSECURITIES",
      enabled: true,
      createdAt: "",
      updatedAt: "",
    });

    await controller.removeAccount("other-account");
    expect(controller.editingAccountId.value).toBe("managed-delete");
    await controller.removeAccount("managed-delete");

    expect(deleteManagedBrokerAccount).toHaveBeenCalledTimes(2);
    expect(controller.editingAccountId.value).toBeNull();
    expect(controller.deletingAccountId.value).toBe("");
  });

  it("clears the deleting flag when backend deletion fails", async () => {
    const controller = createSettingsManagedAccountsController({
      brokerRuntime: ref(emptyBrokerRuntime),
      brokerSettings: ref(createBrokerSettings()),
      createManagedBrokerAccount: vi.fn(async () => undefined),
      updateManagedBrokerAccount: vi.fn(async () => undefined),
      deleteManagedBrokerAccount: vi.fn(async () => {
        throw new Error("account in use");
      }),
    });

    await expect(controller.removeAccount("managed-in-use")).rejects.toThrow(
      "account in use",
    );
    expect(controller.deletingAccountId.value).toBe("");
  });
});
