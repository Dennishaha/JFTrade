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
  emptyPluginCatalog,
  emptyPortfolioCashBalances,
  emptyPortfolioCashReconciliation,
  emptyPortfolioPositions,
  emptyPortfolioReconciliation,
} from "@jftrade/ui-contracts";

import App from "../src/App.vue";
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

export const inputStub = {
  props: ["modelValue"],
  emits: ["update:modelValue"],
  template:
    "<input :value=\"modelValue ?? ''\" @input=\"$emit('update:modelValue', $event.target.value)\" />",
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

export const selectStub = {
  props: ["modelValue"],
  emits: ["update:modelValue"],
  template:
    '<select :value="modelValue" @change="$emit(\'update:modelValue\', $event.target.value)"><slot /></select>',
};

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

export const emptyStub = {
  props: ["description"],
  template: "<div><slot>{{ description }}</slot></div>",
};

export const skeletonStub = defineComponent({
  props: ["loading", "rows", "animated"],
  setup(_, { slots }) {
    return () => h("div", { class: "el-skeleton-stub" }, slots.default?.());
  },
});

export const tabPaneStub = {
  props: ["label"],
  template:
    "<div><div class='el-tab-pane-label'>{{ label }}</div><slot /></div>",
};

export const tabsStub = defineComponent({
  template: "<div class='el-tabs-stub'><slot /></div>",
});

export const iconStub = defineComponent({
  setup(_, { slots }) {
    return () => h("span", { class: "el-icon-stub" }, slots.default?.());
  },
});

export class MockEventSource {
  static instances: MockEventSource[] = [];

  onopen: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent<string>) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  closed = false;

  constructor(public readonly url: string) {
    MockEventSource.instances.push(this);

    queueMicrotask(() => {
      this.onopen?.(new Event("open"));
    });
  }

  emitMessage(data: unknown): void {
    this.onmessage?.({ data: JSON.stringify(data) } as MessageEvent<string>);
  }

  close(): void {
    this.closed = true;
  }
}

export class MockWebSocket {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSING = 2;
  static readonly CLOSED = 3;
  static instances: MockWebSocket[] = [];

  readonly CONNECTING = MockWebSocket.CONNECTING;
  readonly OPEN = MockWebSocket.OPEN;
  readonly CLOSING = MockWebSocket.CLOSING;
  readonly CLOSED = MockWebSocket.CLOSED;

  onopen: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent<string>) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  onclose: ((event: CloseEvent) => void) | null = null;
  readyState = MockWebSocket.CONNECTING;
  closed = false;

  constructor(public readonly url: string) {
    MockWebSocket.instances.push(this);

    queueMicrotask(() => {
      this.readyState = MockWebSocket.OPEN;
      this.onopen?.(new Event("open"));
    });
  }

  addEventListener(
    type: "open" | "message" | "error" | "close",
    listener: EventListener,
  ): void {
    switch (type) {
      case "open":
        this.onopen = listener as (event: Event) => void;
        break;
      case "message":
        this.onmessage = listener as (event: MessageEvent<string>) => void;
        break;
      case "error":
        this.onerror = listener as (event: Event) => void;
        break;
      case "close":
        this.onclose = listener as (event: CloseEvent) => void;
        break;
    }
  }

  emitMessage(data: unknown): void {
    this.onmessage?.({ data: JSON.stringify(data) } as MessageEvent<string>);
  }

  close(): void {
    this.readyState = MockWebSocket.CLOSED;
    this.closed = true;
    this.onclose?.({ code: 1000, reason: "", wasClean: true } as CloseEvent);
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

export async function flushRequests(): Promise<void> {
  for (let attempt = 0; attempt < 4; attempt += 1) {
    await Promise.resolve();
    await nextTick();
    await new Promise((resolve) => setTimeout(resolve, 0));
  }
}

export async function mountApp(path = "/system") {
  MockWebSocket.instances = [];
  vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);

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
        "el-card": passthroughStub,
        "el-tag": passthroughStub,
        "el-button": buttonStub,
        "el-alert": passthroughStub,
        "el-progress": passthroughStub,
        "el-timeline": passthroughStub,
        "el-timeline-item": passthroughStub,
        "el-menu": passthroughStub,
        "el-menu-item": passthroughStub,
        "el-collapse": collapseStub,
        "el-collapse-item": collapseItemStub,
        "el-autocomplete": autocompleteStub,
        "el-input": inputStub,
        "el-input-number": inputNumberStub,
        "el-switch": switchStub,
        "el-select": selectStub,
        "el-option": optionStub,
        "el-form": passthroughStub,
        "el-form-item": passthroughStub,
        "el-table": tableStub,
        "el-table-column": tableColumnStub,
        "el-drawer": drawerStub,
        "el-empty": emptyStub,
        "el-skeleton": skeletonStub,
        "el-page-header": passthroughStub,
        "el-breadcrumb": passthroughStub,
        "el-breadcrumb-item": passthroughStub,
        "el-tabs": tabsStub,
        "el-tab-pane": tabPaneStub,
        "el-icon": iconStub,
      },
    },
  });

  await flushRequests();

  return { router, wrapper };
}
