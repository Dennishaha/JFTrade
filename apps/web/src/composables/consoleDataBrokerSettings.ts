import { computed, type Ref } from "vue";

import {
  type BrokerRuntimeResponse,
  type BrokerSettingsResponse,
  type SystemStatusResponse,
} from "@jftrade/ui-contracts";

import {
  fetchEnvelope,
  fetchEnvelopeWithInit,
} from "./apiClient";
import {
  buildBrokerAccountSelectionKey,
  resolveActiveBrokerId as resolveActiveBrokerIdFromSelection,
  resolveBrokerAccountOptions as resolveBrokerAccountOptionsFromSelection,
  resolveBrokerQuery as resolveBrokerQueryFromSelection,
  resolveSelectedBrokerAccountOption as resolveSelectedBrokerAccountOptionFromSelection,
} from "./consoleDataBrokerAccountSelection";
import type { WorkspacePreferences } from "./useWorkspaceLayout";

export type { BrokerAccountSelectionOption } from "./consoleDataBrokerAccountSelection";

interface CreateConsoleDataBrokerSettingsControllerOptions {
  prefs: Ref<WorkspacePreferences>;
  update: (patch: Partial<WorkspacePreferences>) => void;
  brokerSettings: Ref<BrokerSettingsResponse>;
  brokerRuntime: Ref<BrokerRuntimeResponse>;
  systemStatus: Ref<SystemStatusResponse>;
  reloadSystemState: (options?: {
    background?: boolean;
    bypassCooldown?: boolean;
  }) => Promise<void>;
}

export function createConsoleDataBrokerSettingsController(
  options: CreateConsoleDataBrokerSettingsControllerOptions,
) {
  function resolveActiveBrokerId(context?: {
    settings?: BrokerSettingsResponse;
    status?: SystemStatusResponse;
  }): string {
    return resolveActiveBrokerIdFromSelection({
      selectedBrokerAccountKey: options.prefs.value.selectedBrokerAccountKey,
      settings: context?.settings ?? options.brokerSettings.value,
      status: context?.status ?? options.systemStatus.value,
    });
  }

  function resolveBrokerAccountOptions(context: {
    activeBrokerId: string;
    settings: BrokerSettingsResponse;
    runtime: BrokerRuntimeResponse;
    fallbackMarket: string;
  }) {
    return resolveBrokerAccountOptionsFromSelection(context);
  }

  function resolveSelectedBrokerAccountOption(
    selectionOptions: readonly import("./consoleDataBrokerAccountSelection").BrokerAccountSelectionOption[],
    activeBrokerId: string,
    defaultTradingEnvironment: string,
  ) {
    return resolveSelectedBrokerAccountOptionFromSelection({
      selectionOptions,
      selectedBrokerAccountKey: options.prefs.value.selectedBrokerAccountKey,
      activeBrokerId,
      defaultTradingEnvironment,
    });
  }

  const availableBrokerAccounts = computed(() =>
    resolveBrokerAccountOptions({
      activeBrokerId: resolveActiveBrokerId(),
      settings: options.brokerSettings.value,
      runtime: options.brokerRuntime.value,
      fallbackMarket:
        options.brokerRuntime.value.descriptor.capabilities[0]?.market ??
        options.systemStatus.value.broker.capabilities[0]?.market ??
        "HK",
    }),
  );

  const selectedBrokerAccount = computed(() =>
    resolveSelectedBrokerAccountOption(
      availableBrokerAccounts.value,
      resolveActiveBrokerId(),
      options.systemStatus.value.defaultTradingEnvironment,
    ),
  );

  function resolveBrokerQuery(
    selection: import("./consoleDataBrokerAccountSelection").BrokerAccountSelectionOption | null,
  ) {
    return resolveBrokerQueryFromSelection({
      selection,
      runtime: options.brokerRuntime.value,
      status: options.systemStatus.value,
    });
  }

  async function loadBrokerSettings(): Promise<BrokerSettingsResponse> {
    const response = await fetchEnvelope<BrokerSettingsResponse>(
      "/api/v1/settings/brokers",
    );
    options.brokerSettings.value = response;
    return response;
  }

  async function requestFutuOpenDManualRetry(): Promise<void> {
    await fetchEnvelopeWithInit("/api/v1/system/futu-opend/manual-retry", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        brokerId: "futu",
      }),
    });

    await options.reloadSystemState();
  }

  async function saveBrokerIntegration(
    brokerId: string,
    payload: {
      enabled: boolean;
      config: NonNullable<
        BrokerSettingsResponse["brokers"][number]["defaults"]
      >;
    },
  ): Promise<void> {
    await fetchEnvelopeWithInit(
      `/api/v1/settings/brokers/${encodeURIComponent(brokerId)}/integration`,
      {
        method: "PUT",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify(payload),
      },
    );

    await options.reloadSystemState({ bypassCooldown: true });
  }

  async function createManagedBrokerAccount(payload: {
    brokerId: string;
    accountId: string;
    displayName: string;
    tradingEnvironment: string;
    market: string;
    securityFirm?: string;
    enabled: boolean;
  }): Promise<void> {
    await fetchEnvelopeWithInit("/api/v1/settings/broker-accounts", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(payload),
    });

    await options.reloadSystemState();
  }

  async function updateManagedBrokerAccount(
    accountRecordId: string,
    payload: {
      brokerId: string;
      accountId: string;
      displayName: string;
      tradingEnvironment: string;
      market: string;
      securityFirm?: string;
      enabled: boolean;
    },
  ): Promise<void> {
    await fetchEnvelopeWithInit(
      `/api/v1/settings/broker-accounts/${encodeURIComponent(accountRecordId)}`,
      {
        method: "PUT",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify(payload),
      },
    );

    await options.reloadSystemState();
  }

  async function deleteManagedBrokerAccount(
    accountRecordId: string,
  ): Promise<void> {
    await fetchEnvelopeWithInit(
      `/api/v1/settings/broker-accounts/${encodeURIComponent(accountRecordId)}`,
      {
        method: "DELETE",
      },
    );

    if (options.prefs.value.selectedBrokerAccountKey != null) {
      const selectedMatch = options.brokerSettings.value.accounts.find(
        (account) =>
          buildBrokerAccountSelectionKey({
            brokerId: account.brokerId,
            tradingEnvironment: account.tradingEnvironment,
            accountId: account.accountId,
            market: account.market,
          }) === options.prefs.value.selectedBrokerAccountKey,
      );

      if (selectedMatch?.id === accountRecordId) {
        options.update({ selectedBrokerAccountKey: null });
      }
    }

    await options.reloadSystemState();
  }

  async function selectBrokerAccount(selectionKey: string | null): Promise<void> {
    options.update({ selectedBrokerAccountKey: selectionKey });
    await options.reloadSystemState({ bypassCooldown: true });
  }

  return {
    availableBrokerAccounts,
    createManagedBrokerAccount,
    deleteManagedBrokerAccount,
    loadBrokerSettings,
    requestFutuOpenDManualRetry,
    resolveActiveBrokerId,
    resolveBrokerAccountOptions,
    resolveBrokerQuery,
    resolveSelectedBrokerAccountOption,
    saveBrokerIntegration,
    selectBrokerAccount,
    selectedBrokerAccount,
    updateManagedBrokerAccount,
  };
}