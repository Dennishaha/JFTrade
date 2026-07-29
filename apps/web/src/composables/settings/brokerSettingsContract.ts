import type {
  BrokerIntegrationConfig,
  BrokerSettingsResponse,
} from "@/types";
import type {
  BrokerSettingsResponse as BrokerSettingsResponseDto,
  FutuIntegrationConfig,
} from "@/contracts";

import { isBrokerDescriptor } from "@/composables/settings/onboardingContract";

type BrokerSettingsWire = BrokerSettingsResponseDto;
type FutuConfigWire = FutuIntegrationConfig;

function mapFutuConfig(value: FutuConfigWire): BrokerIntegrationConfig {
  return {
    type: "futu",
    host: value.host ?? "127.0.0.1",
    apiPort: value.apiPort ?? 11110,
    websocketPort: value.websocketPort ?? 11111,
    maxWebSocketConnections: value.maxWebSocketConnections ?? 20,
    useEncryption: value.useEncryption ?? false,
    websocketKey: value.websocketKey ?? "",
    tradeMarket: value.tradeMarket ?? "",
    securityFirm: value.securityFirm ?? "",
  };
}

export function mapBrokerSettings(value: BrokerSettingsWire): BrokerSettingsResponse {
  return {
    brokers: (value.brokers ?? []).flatMap((broker) => {
      if (!isBrokerDescriptor(broker.descriptor)) {
        return [];
      }
      return [
        {
          descriptor: broker.descriptor,
          integration:
            broker.integration == null
              ? null
              : {
                  brokerId: broker.integration.brokerId ?? broker.descriptor.id,
                  enabled: broker.integration.enabled ?? false,
                  config: mapFutuConfig(broker.integration.config ?? {}),
                  updatedAt: broker.integration.updatedAt ?? "",
                  createdAt: broker.integration.createdAt ?? "",
                },
          defaults:
            broker.defaults == null ? null : mapFutuConfig(broker.defaults),
        },
      ];
    }),
    accounts: (value.accounts ?? []).map((account) => ({
      id: account.id ?? "",
      brokerId: account.brokerId ?? "",
      accountId: account.accountId ?? "",
      displayName: account.displayName ?? "",
      tradingEnvironment: account.tradingEnvironment ?? "",
      market: account.market ?? "",
      securityFirm: account.securityFirm ?? null,
      enabled: account.enabled ?? false,
      updatedAt: account.updatedAt ?? "",
      createdAt: account.createdAt ?? "",
    })),
  };
}
