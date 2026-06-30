export interface StrategyInstanceCardModel {
  id: string;
  name: string;
  status: string;
  statusLabel: string;
  selected: boolean;
  disabled?: boolean;
  definitionStale: boolean;
  definitionSyncSummary: string;
  symbols: string;
  interval: string;
  brokerAccountSummary: string;
  currentBrokerAccount: boolean;
  createdAt: string;
  createdAtTooltip: string;
  runtimeLabel: string;
  sourceFormatLabel: string;
  eligibilityLabel: string;
  startable: boolean;
  executionModeLabel: string;
  notifyOnly: boolean;
}
