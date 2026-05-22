import {
  type BrokerRuntimeResponse,
  type BrokerSettingsResponse,
  type SystemStatusResponse,
} from "@jftrade/ui-contracts";

export interface BrokerAccountSelectionOption {
  selectionKey: string;
  source: "managed" | "runtime";
  brokerId: string;
  accountId: string;
  displayName: string;
  tradingEnvironment: string;
  market: string;
  securityFirm: string | null;
}

export function buildBrokerAccountSelectionKey(input: {
  brokerId: string;
  tradingEnvironment: string;
  accountId: string;
  market: string;
}): string {
  return [
    input.brokerId,
    input.tradingEnvironment,
    input.accountId,
    input.market,
  ]
    .map((segment) => encodeURIComponent(segment))
    .join("|");
}

function parseBrokerAccountSelectionKey(key: string | null | undefined): {
  brokerId: string;
} | null {
  if (key == null || key === "") {
    return null;
  }

  const [brokerId] = key.split("|");

  if (brokerId == null || brokerId === "") {
    return null;
  }

  return {
    brokerId: decodeURIComponent(brokerId),
  };
}

export function resolveActiveBrokerId(input: {
  selectedBrokerAccountKey: string | null | undefined;
  settings: BrokerSettingsResponse;
  status: SystemStatusResponse;
}): string {
  const preferredSelection = parseBrokerAccountSelectionKey(
    input.selectedBrokerAccountKey,
  );

  if (preferredSelection != null) {
    return preferredSelection.brokerId;
  }

  return (
    input.settings.accounts.find((account) => account.enabled)?.brokerId ??
    input.settings.brokers[0]?.descriptor.id ??
    input.status.defaultBroker ??
    "futu"
  );
}

export function resolveBrokerAccountOptions(context: {
  activeBrokerId: string;
  settings: BrokerSettingsResponse;
  runtime: BrokerRuntimeResponse;
  fallbackMarket: string;
}): BrokerAccountSelectionOption[] {
  const selectionOptions: BrokerAccountSelectionOption[] = [];
  const seen = new Set<string>();

  for (const account of context.settings.accounts) {
    if (!account.enabled) {
      continue;
    }

    const selectionKey = buildBrokerAccountSelectionKey({
      brokerId: account.brokerId,
      tradingEnvironment: account.tradingEnvironment,
      accountId: account.accountId,
      market: account.market,
    });

    seen.add(selectionKey);
    selectionOptions.push({
      selectionKey,
      source: "managed",
      brokerId: account.brokerId,
      accountId: account.accountId,
      displayName: account.displayName,
      tradingEnvironment: account.tradingEnvironment,
      market: account.market,
      securityFirm: account.securityFirm,
    });
  }

  if (context.runtime.descriptor.id !== context.activeBrokerId) {
    return selectionOptions;
  }

  for (const account of context.runtime.accounts) {
    const market = account.marketAuthorities[0] ?? context.fallbackMarket;
    const selectionKey = buildBrokerAccountSelectionKey({
      brokerId: context.activeBrokerId,
      tradingEnvironment: account.tradingEnvironment,
      accountId: account.accountId,
      market,
    });

    if (seen.has(selectionKey)) {
      continue;
    }

    seen.add(selectionKey);
    selectionOptions.push({
      selectionKey,
      source: "runtime",
      brokerId: context.activeBrokerId,
      accountId: account.accountId,
      displayName: account.accountId,
      tradingEnvironment: account.tradingEnvironment,
      market,
      securityFirm: account.securityFirm,
    });
  }

  return selectionOptions;
}

export function resolveSelectedBrokerAccountOption(input: {
  selectionOptions: readonly BrokerAccountSelectionOption[];
  selectedBrokerAccountKey: string | null | undefined;
  activeBrokerId: string;
  defaultTradingEnvironment: string;
}): BrokerAccountSelectionOption | null {
  return (
    input.selectionOptions.find(
      (option) => option.selectionKey === input.selectedBrokerAccountKey,
    ) ??
    input.selectionOptions.find(
      (option) =>
        option.brokerId === input.activeBrokerId &&
        option.tradingEnvironment === input.defaultTradingEnvironment,
    ) ??
    input.selectionOptions.find(
      (option) => option.brokerId === input.activeBrokerId,
    ) ??
    input.selectionOptions[0] ??
    null
  );
}

export function resolveBrokerQuery(input: {
  selection: BrokerAccountSelectionOption | null;
  runtime: BrokerRuntimeResponse;
  status: SystemStatusResponse;
}): URLSearchParams {
  if (
    input.selection != null &&
    input.selection.brokerId === input.runtime.descriptor.id
  ) {
    return new URLSearchParams({
      tradingEnvironment: input.selection.tradingEnvironment,
      accountId: input.selection.accountId,
      market: input.selection.market,
    });
  }

  const runtimeAccount =
    input.runtime.accounts.find(
      (account) =>
        account.tradingEnvironment === input.status.defaultTradingEnvironment,
    ) ?? input.runtime.accounts[0];

  return new URLSearchParams({
    tradingEnvironment:
      runtimeAccount?.tradingEnvironment ?? input.status.defaultTradingEnvironment,
    accountId: runtimeAccount?.accountId ?? "0",
    market:
      runtimeAccount?.marketAuthorities[0] ??
      input.runtime.descriptor.capabilities[0]?.market ??
      input.status.broker.capabilities[0]?.market ??
      "HK",
  });
}