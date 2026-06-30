import { mount } from "@vue/test-utils";
import { createPinia } from "pinia";
import { vi } from "vitest";
import { defineComponent, h, nextTick } from "vue";
import { createMemoryHistory } from "vue-router";

import {
  type ApiSuccessEnvelope,
  emptyBrokerCashFlows,
  emptyBrokerFunds,
  emptyBrokerOrders,
  emptyBrokerPositions,
  emptyBrokerRuntime,
  emptyBrokerSettings,
  emptyExecutionOrders,
  emptyOnboardingState,
  emptyPluginCatalog,
  emptyPortfolioCashBalances,
  emptyPortfolioCashReconciliation,
  emptyPortfolioPositions,
  emptyPortfolioReconciliation,
} from "@/contracts";

import App from "../src/App.vue";
import type { LiveEventEnvelope, LiveEventSource } from "../src/composables/liveEventBus";
import { queryClient } from "../src/composables/serverState";
import { resetSharedLiveSocketHubForTests } from "../src/composables/sharedLiveSocket";
import { createConsoleRouter } from "../src/router";

export const passthroughStub = {
  template: "<div><slot name='header' /><slot name='default' /><slot /></div>",
};

export const collapseStub = {
  template: "<div><slot /></div>",
};

export const collapseItemStub = {
  template: "<div><slot name='title' /><slot /></div>",
};

export const buttonStub = {
  template: "<button type='button' @click=\"$emit('click')\"><slot /></button>",
};

export function enabledFutuBrokerSettings(
  accounts: Array<{
    accountId: string;
    displayName: string;
    tradingEnvironment: string;
    market: string;
  }> = [],
) {
  return {
    brokers: [
      {
        descriptor: {
          id: "futu",
          displayName: "Futu OpenAPI via OpenD",
          environments: ["SIMULATE", "REAL"],
          capabilities: [
            { market: "HK", supportsQuote: true, supportsTrade: true },
          ],
          notes: [],
        },
        integration: {
          brokerId: "futu",
          enabled: true,
          config: {
            type: "futu",
            host: "127.0.0.1",
            apiPort: 11110,
            websocketPort: 11111,
            maxWebSocketConnections: 20,
            useEncryption: false,
            tradeMarket: "HK",
            securityFirm: "FUTUSECURITIES",
          },
          createdAt: "2026-06-07T00:00:00Z",
          updatedAt: "2026-06-07T00:00:00Z",
        },
      },
    ],
    accounts: accounts.map((account, index) => ({
      id: `test-account-${index + 1}`,
      brokerId: "futu",
      accountId: account.accountId,
      displayName: account.displayName,
      tradingEnvironment: account.tradingEnvironment,
      market: account.market,
      securityFirm: "FUTUSECURITIES",
      enabled: true,
      createdAt: "2026-06-07T00:00:00Z",
      updatedAt: "2026-06-07T00:00:00Z",
    })),
  };
}

export const inputStub = {
  props: ["modelValue"],
  emits: ["update:modelValue"],
  template:
    "<input :value=\"modelValue ?? ''\" @input=\"$emit('update:modelValue', $event.target.value)\" />",
};

export const textareaStub = {
  props: ["modelValue"],
  emits: ["update:modelValue"],
  template:
    "<textarea :value=\"modelValue ?? ''\" @input=\"$emit('update:modelValue', $event.target.value)\"></textarea>",
};

export const autocompleteStub = defineComponent({
  props: ["modelValue"],
  emits: ["update:modelValue", "select"],
  setup(props, { emit }) {
    return () =>
      h("input", {
        value: props.modelValue ?? "",
        onInput: (event: Event) => {
          emit("update:modelValue", (event.target as HTMLInputElement).value);
        },
      });
  },
});

export const inputNumberStub = {
  props: ["modelValue", "min"],
  emits: ["update:modelValue"],
  template:
    "<input type='number' :value=\"modelValue ?? ''\" @input=\"$emit('update:modelValue', $event.target.value)\" :min=\"min\" />",
};

export const switchStub = {
  props: ["modelValue", "activeText"],
  emits: ["update:modelValue"],
  template:
    '<label><input type=\'checkbox\' :checked="!!modelValue" @change="$emit(\'update:modelValue\', $event.target.checked)" /><span v-if="activeText">{{ activeText }}</span></label>',
};

export const selectStub = defineComponent({
  props: ["modelValue", "items", "itemTitle", "itemValue"],
  emits: ["update:modelValue"],
  setup(props, { slots, emit }) {
    return () => {
      const items = (props.items as Array<string | Record<string, unknown>>) ?? [];
      const titleKey = (props.itemTitle as string) ?? "title";
      const valueKey = (props.itemValue as string) ?? "value";
      const options = items.map((item, idx) => {
        const val = typeof item === "string" ? item : (item[valueKey] as string) ?? String(idx);
        const label = typeof item === "string" ? item : ((item[titleKey] as string) ?? val);
        return h("option", { key: val, value: val }, label);
      });
      return h(
        "select",
        {
          value: props.modelValue ?? "",
          onChange: (event: Event) => emit("update:modelValue", (event.target as HTMLSelectElement).value),
        },
        options.length > 0 ? options : slots.default?.(),
      );
    };
  },
});

export const optionStub = {
  props: ["value", "label"],
  template: '<option :value="value">{{ label || value }}</option>',
};

// @ts-expect-error biome doesn't support Record-based access patterns
let currentTableData: Record<string, unknown>[] = [];

export const tableStub = defineComponent({
  props: ["data", "stripe"],
  setup(props, { slots }) {
    currentTableData = (props.data as Record<string, unknown>[]) || [];
    return () => {
      const rows = (props.data as Record<string, unknown>[]) || [];
      return h("div", { class: "el-table-stub" }, [
        h("div", { class: "el-table-header" }, slots.default?.()),
        rows.map((row, rowIdx) =>
          h("div", { key: rowIdx, class: "el-table-row" }, [
            slots.default?.({ row }),
          ]),
        ),
      ]);
    };
  },
});

export const tableColumnStub = defineComponent({
  props: ["prop", "label", "width", "minWidth", "align"],
  setup(props, { slots }) {
    return () => {
      if (slots.default) {
        return h("div", { class: "el-table-column-stub" }, [
          currentTableData.map((row, idx) =>
            h("div", { key: idx }, slots.default({ row })),
          ),
        ]);
      }

      if (props.prop) {
        return h("div", { class: "el-table-column-stub" }, [
          currentTableData.map((row, idx) => {
            const value = row[props.prop as string];
            return h("div", { key: idx }, value);
          }),
        ]);
      }

      return h("div", { class: "el-table-column-stub" });
    };
  },
});

export const drawerStub = {
  props: ["modelValue", "title"],
  emits: ["update:modelValue"],
  template: "<div v-if=\"modelValue\" class='el-drawer-stub'><slot /></div>",
};

export const dialogStub = {
  props: ["modelValue"],
  emits: ["update:modelValue"],
  template: "<div v-if=\"modelValue\" class='v-dialog-stub'><slot /></div>",
};

export const emptyStub = {
  props: ["text", "description"],
  template: "<div><slot>{{ text ?? description }}</slot></div>",
};

export const skeletonStub = defineComponent({
  props: ["type", "loading"],
  setup(_, { slots }) {
    return () => h("div", { class: "v-skeleton-loader-stub" }, slots.default?.());
  },
});

export const tabStub = {
  props: ["value"],
  template: "<div class='v-tab-stub'><slot /></div>",
};

export const tabsStub = defineComponent({
  template: "<div class='v-tabs-stub'><slot /></div>",
});

export const windowStub = defineComponent({
  template: "<div class='v-window-stub'><slot /></div>",
});

export const windowItemStub = defineComponent({
  props: ["value"],
  template: "<div class='v-window-item-stub'><slot /></div>",
});

export const iconStub = defineComponent({
  setup() {
    return () => h("span", { class: "v-icon-stub", "aria-hidden": "true" });
  },
});

export class MockWebSocket {
  static instances: MockWebSocket[] = [];
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSING = 2;
  static readonly CLOSED = 3;

  readonly sentMessages: string[] = [];
  onopen: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent<string>) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  onclose: ((event: CloseEvent) => void) | null = null;
  closed = false;
  readyState = MockWebSocket.CONNECTING;
  private readonly listeners = new Map<string, Set<(event: Event | MessageEvent<string>) => void>>();

  constructor(public readonly url: string) {
    MockWebSocket.instances.push(this);

    queueMicrotask(() => {
      this.readyState = MockWebSocket.OPEN;
      this.dispatchEvent("open", new Event("open"));
    });
  }

  addEventListener(
    type: string,
    listener: (event: Event | MessageEvent<string>) => void,
  ): void {
    const listeners = this.listeners.get(type) ?? new Set();
    listeners.add(listener);
    this.listeners.set(type, listeners);
  }

  removeEventListener(
    type: string,
    listener: (event: Event | MessageEvent<string>) => void,
  ): void {
    this.listeners.get(type)?.delete(listener);
  }

  send(data: string): void {
    this.sentMessages.push(data);
  }

  emitMessage(data: unknown): void {
    this.dispatchEvent(
      "message",
      { data: JSON.stringify(data) } as MessageEvent<string>,
    );
  }

  emitError(): void {
    this.dispatchEvent("error", new Event("error"));
  }

  close(): void {
    this.closed = true;
    this.readyState = MockWebSocket.CLOSED;
    this.dispatchEvent("close", new CloseEvent("close"));
  }

  private dispatchEvent(
    type: string,
    event: Event | MessageEvent<string>,
  ): void {
    if (type === "open") {
      this.onopen?.(event as Event);
    } else if (type === "message") {
      this.onmessage?.(event as MessageEvent<string>);
    } else if (type === "error") {
      this.onerror?.(event as Event);
    } else if (type === "close") {
      this.onclose?.(event as CloseEvent);
    }
    for (const listener of this.listeners.get(type) ?? []) {
      listener(event);
    }
  }
}

export function createSuccessEnvelope<T>(data: T): ApiSuccessEnvelope<T> {
  return {
    ok: true,
    data,
    timestamp: "2026-05-16T00:00:00.000Z",
  };
}

export function createResponse<T>(data: T): Response {
  return {
    ok: true,
    json: async () => createSuccessEnvelope(data),
  } as Response;
}

export function createLiveEnvelope<TPayload extends { type: string; at?: string }>(
  payload: TPayload,
  options: {
    source: LiveEventSource;
    entityId: string;
    eventId?: string;
    serverTime?: string;
  },
): LiveEventEnvelope<TPayload> {
  const serverTime = options.serverTime ?? payload.at ?? "2026-05-16T00:00:00.000Z";
  return {
    eventId: options.eventId ?? `${payload.type}|${options.entityId}|${serverTime}`,
    type: payload.type,
    source: options.source,
    entityId: options.entityId,
    serverTime,
    payload,
  };
}

export async function flushRequests(): Promise<void> {
  for (let attempt = 0; attempt < 4; attempt += 1) {
    await Promise.resolve();
    await nextTick();
    await new Promise((resolve) => setTimeout(resolve, 0));
  }
}

export async function mountApp(path = "/system") {
  queryClient.clear();
  resetSharedLiveSocketHubForTests();
  MockWebSocket.instances = [];
  vi.stubGlobal(
    "WebSocket",
    MockWebSocket as unknown as typeof WebSocket,
  );

  const existingFetch = globalThis.fetch;
  if (existingFetch != null) {
    vi.stubGlobal("fetch", (async (
      input: string | URL | Request,
      init?: RequestInit,
    ) => {
      const url = String(input);

      try {
        return await existingFetch(input, init);
      } catch (error) {
        if (url.includes("/api/v1/settings/brokers")) {
          return createResponse(emptyBrokerSettings);
        }
        if (url.includes("/api/v1/settings/onboarding")) {
          return createResponse(emptyOnboardingState);
        }
        if (url.includes("/api/v1/plugins")) {
          return createResponse(emptyPluginCatalog);
        }
        if (/\/api\/v1\/brokers\/[^/]+\/runtime/.test(url)) {
          return createResponse(emptyBrokerRuntime);
        }
        if (/\/api\/v1\/brokers\/[^/]+\/funds/.test(url)) {
          return createResponse(emptyBrokerFunds);
        }
        if (/\/api\/v1\/brokers\/[^/]+\/positions/.test(url)) {
          return createResponse(emptyBrokerPositions);
        }
        if (/\/api\/v1\/brokers\/[^/]+\/orders/.test(url)) {
          return createResponse(emptyBrokerOrders);
        }
        if (/\/api\/v1\/brokers\/[^/]+\/cash-flows/.test(url)) {
          return createResponse(emptyBrokerCashFlows);
        }
        if (/\/api\/v1\/portfolio\/[^/]+\/cash-balances/.test(url)) {
          return createResponse(emptyPortfolioCashBalances);
        }
        if (/\/api\/v1\/portfolio\/[^/]+\/positions/.test(url)) {
          return createResponse(emptyPortfolioPositions);
        }
        if (/\/api\/v1\/portfolio\/[^/]+\/cash-reconciliation/.test(url)) {
          return createResponse(emptyPortfolioCashReconciliation);
        }
        if (/\/api\/v1\/portfolio\/[^/]+\/reconciliation/.test(url)) {
          return createResponse(emptyPortfolioReconciliation);
        }
        if (url.includes("/api/v1/execution/orders")) {
          return createResponse(emptyExecutionOrders);
        }

        throw error;
      }
    }) as typeof fetch);
  }

  const router = createConsoleRouter(createMemoryHistory());
  await router.push(path);
  await router.isReady();

  const wrapper = mount(App, {
    global: {
      plugins: [createPinia(), router],
      stubs: {
        "v-card": passthroughStub,
        "v-card-item": passthroughStub,
        "v-card-title": passthroughStub,
        "v-card-text": passthroughStub,
        "v-card-actions": passthroughStub,
        "v-chip": passthroughStub,
        "v-btn": buttonStub,
        "v-btn-toggle": passthroughStub,
        "v-alert": passthroughStub,
        "v-progress-linear": passthroughStub,
        "v-progress-circular": passthroughStub,
        "v-expansion-panels": collapseStub,
        "v-expansion-panel": collapseItemStub,
        "v-autocomplete": autocompleteStub,
        "v-text-field": inputStub,
        "v-textarea": textareaStub,
        "v-switch": switchStub,
        "v-select": selectStub,
        "v-form": passthroughStub,
        "v-menu": passthroughStub,
        "v-list-item": passthroughStub,
        "v-data-table": tableStub,
        "v-navigation-drawer": drawerStub,
        "v-dialog": dialogStub,
        "v-empty-state": emptyStub,
        "v-skeleton-loader": skeletonStub,
        "v-breadcrumbs": passthroughStub,
        "v-breadcrumbs-item": passthroughStub,
        "v-tabs": tabsStub,
        "v-tab": tabStub,
        "v-window": windowStub,
        "v-window-item": windowItemStub,
        "v-icon": iconStub,
      },
    },
  });

  await flushRequests();

  return { router, wrapper };
}
