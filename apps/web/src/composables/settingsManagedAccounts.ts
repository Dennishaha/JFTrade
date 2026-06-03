import { computed, reactive, ref, watch, type Ref } from "vue";

import {
  type BrokerRuntimeResponse,
  type BrokerSettingsResponse,
} from "@jftrade/ui-contracts";

export interface SettingsManagedAccountForm {
  brokerId: string;
  accountId: string;
  displayName: string;
  tradingEnvironment: string;
  market: string;
  securityFirm: string;
  enabled: boolean;
}

interface SettingsManagedAccountsControllerOptions {
  brokerRuntime: Ref<BrokerRuntimeResponse>;
  brokerSettings: Ref<BrokerSettingsResponse>;
  createManagedBrokerAccount: (
    payload: SettingsManagedAccountForm,
  ) => Promise<void>;
  updateManagedBrokerAccount: (
    accountRecordId: string,
    payload: SettingsManagedAccountForm,
  ) => Promise<void>;
  deleteManagedBrokerAccount: (accountRecordId: string) => Promise<void>;
}

export function createSettingsManagedAccountsController(
  options: SettingsManagedAccountsControllerOptions,
) {
  const savingAccount = ref(false);
  const deletingAccountId = ref("");
  const editingAccountId = ref<string | null>(null);

  const futuBroker = computed(
    () =>
      options.brokerSettings.value.brokers.find(
        (broker) => broker.descriptor.id === "futu",
      ) ?? null,
  );

  const futuBrokerDefaults = computed(
    () =>
      futuBroker.value?.integration?.config ?? futuBroker.value?.defaults ?? null,
  );

  const defaultFutuTradeMarket = computed(
    () => futuBrokerDefaults.value?.tradeMarket ?? "HK",
  );

  const defaultFutuSecurityFirm = computed(
    () => futuBrokerDefaults.value?.securityFirm ?? "FUTUSECURITIES",
  );

  const accountForm = reactive<SettingsManagedAccountForm>({
    brokerId: "futu",
    accountId: "",
    displayName: "",
    tradingEnvironment: "SIMULATE",
    market: "HK",
    securityFirm: "FUTUSECURITIES",
    enabled: true,
  });

  function resetAccountForm(): void {
    editingAccountId.value = null;
    accountForm.brokerId = futuBroker.value?.descriptor.id ?? "futu";
    accountForm.accountId = "";
    accountForm.displayName = "";
    accountForm.tradingEnvironment = "SIMULATE";
    accountForm.market = defaultFutuTradeMarket.value;
    accountForm.securityFirm = defaultFutuSecurityFirm.value;
    accountForm.enabled = true;
  }

  function populateAccountForm(
    account: BrokerSettingsResponse["accounts"][number],
  ): void {
    editingAccountId.value = account.id;
    accountForm.brokerId = account.brokerId;
    accountForm.accountId = account.accountId;
    accountForm.displayName = account.displayName;
    accountForm.tradingEnvironment = account.tradingEnvironment;
    accountForm.market = account.market;
    accountForm.securityFirm =
      account.securityFirm ?? defaultFutuSecurityFirm.value;
    accountForm.enabled = account.enabled;
  }

  async function importRuntimeAccount(
    account: BrokerRuntimeResponse["accounts"][number],
  ): Promise<void> {
    const brokerId = futuBroker.value?.descriptor.id ?? "futu";
    const market = account.marketAuthorities[0] ?? defaultFutuTradeMarket.value;
    const payload: SettingsManagedAccountForm = {
      brokerId,
      accountId: account.accountId,
      displayName: account.accountId,
      tradingEnvironment: account.tradingEnvironment,
      market,
      securityFirm: account.securityFirm ?? defaultFutuSecurityFirm.value,
      enabled: true,
    };

    const existingAccount = options.brokerSettings.value.accounts.find(
      (savedAccount) =>
        savedAccount.brokerId === payload.brokerId &&
        savedAccount.accountId === payload.accountId &&
        savedAccount.tradingEnvironment === payload.tradingEnvironment &&
        savedAccount.market === payload.market,
    );

    savingAccount.value = true;

    try {
      if (existingAccount == null) {
        await options.createManagedBrokerAccount(payload);
      } else {
        await options.updateManagedBrokerAccount(existingAccount.id, payload);
      }

      resetAccountForm();
    } finally {
      savingAccount.value = false;
    }
  }

  async function submitAccount(): Promise<void> {
    savingAccount.value = true;

    try {
      const payload: SettingsManagedAccountForm = {
        brokerId: accountForm.brokerId,
        accountId: accountForm.accountId,
        displayName: accountForm.displayName,
        tradingEnvironment: accountForm.tradingEnvironment,
        market: accountForm.market,
        securityFirm: accountForm.securityFirm,
        enabled: accountForm.enabled,
      };

      if (editingAccountId.value == null) {
        await options.createManagedBrokerAccount(payload);
      } else {
        await options.updateManagedBrokerAccount(editingAccountId.value, payload);
      }

      resetAccountForm();
    } finally {
      savingAccount.value = false;
    }
  }

  async function removeAccount(accountId: string): Promise<void> {
    deletingAccountId.value = accountId;

    try {
      await options.deleteManagedBrokerAccount(accountId);

      if (editingAccountId.value === accountId) {
        resetAccountForm();
      }
    } finally {
      deletingAccountId.value = "";
    }
  }

  watch(
    futuBroker,
    () => {
      if (editingAccountId.value == null && accountForm.accountId === "") {
        resetAccountForm();
      }
    },
    { immediate: true },
  );

  return {
    accountForm,
    deletingAccountId,
    editingAccountId,
    importRuntimeAccount,
    populateAccountForm,
    removeAccount,
    resetAccountForm,
    savingAccount,
    submitAccount,
  };
}