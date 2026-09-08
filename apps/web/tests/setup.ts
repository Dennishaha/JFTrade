import { config } from "@vue/test-utils";
import { vi } from "vitest";

function installStorage(name: "localStorage" | "sessionStorage"): void {
  const values = new Map<string, string>();
  const storage: Storage = {
    get length() {
      return values.size;
    },
    clear: () => values.clear(),
    getItem: (key) => values.get(String(key)) ?? null,
    key: (index) => Array.from(values.keys())[index] ?? null,
    removeItem: (key) => values.delete(String(key)),
    setItem: (key, value) => values.set(String(key), String(value)),
  };

  Object.defineProperty(window, name, {
    configurable: true,
    value: storage,
  });
}

if (typeof window !== "undefined") {
  installStorage("localStorage");
  installStorage("sessionStorage");
  vi.stubGlobal("localStorage", window.localStorage);
  vi.stubGlobal("sessionStorage", window.sessionStorage);
}

const passthroughStub = {
  template: "<div><slot name='header' /><slot /></div>",
};
const inlineStub = {
  template: "<span><slot /></span>",
};

const iconStub = {
  template: "<span class='v-icon-stub' aria-hidden='true'><slot /></span>",
};

const buttonStub = {
  props: ["disabled", "loading"],
  emits: ["click"],
  template:
    "<button type='button' :disabled='disabled' :class='$attrs.class' @click=\"$emit('click')\"><slot /></button>",
};

const dialogStub = {
  props: ["modelValue"],
  emits: ["update:modelValue"],
  template: "<div v-if='modelValue' class='v-dialog-stub'><slot /></div>",
};

const menuStub = {
  template: "<div><slot name='activator' :props='{}' /><slot /></div>",
};

const textFieldStub = {
  props: ["modelValue"],
  emits: ["update:modelValue"],
  template:
    "<input :value=\"modelValue ?? ''\" @input=\"$emit('update:modelValue', $event.target.value)\" />",
};

const textareaStub = {
  props: ["modelValue"],
  emits: ["update:modelValue"],
  template:
    "<textarea :value=\"modelValue ?? ''\" @input=\"$emit('update:modelValue', $event.target.value)\"></textarea>",
};

config.global.stubs = {
  ...config.global.stubs,
  "v-alert": passthroughStub,
  "v-badge": passthroughStub,
  "v-breadcrumbs": passthroughStub,
  "v-breadcrumbs-item": passthroughStub,
  "v-btn": buttonStub,
  "v-btn-toggle": passthroughStub,
  "v-card": passthroughStub,
  "v-card-actions": passthroughStub,
  "v-card-item": passthroughStub,
  "v-card-subtitle": passthroughStub,
  "v-card-text": passthroughStub,
  "v-card-title": passthroughStub,
  "v-checkbox": passthroughStub,
  "v-chip": inlineStub,
  "v-col": passthroughStub,
  "v-container": passthroughStub,
  "v-dialog": dialogStub,
  "v-divider": { template: "<hr class='v-divider-stub' />" },
  "v-empty-state": passthroughStub,
  "v-expansion-panel": passthroughStub,
  "v-expansion-panel-text": passthroughStub,
  "v-expansion-panel-title": passthroughStub,
  "v-expansion-panels": passthroughStub,
  "v-form": { template: "<form><slot /></form>" },
  "v-icon": iconStub,
  "v-list": passthroughStub,
  "v-list-item": passthroughStub,
  "v-list-item-subtitle": passthroughStub,
  "v-list-item-title": passthroughStub,
  "v-menu": menuStub,
  "v-navigation-drawer": passthroughStub,
  "v-pagination": passthroughStub,
  "v-progress-circular": { template: "<span class='v-progress-circular-stub' />" },
  "v-progress-linear": { template: "<span class='v-progress-linear-stub' />" },
  "v-radio": passthroughStub,
  "v-radio-group": passthroughStub,
  "v-row": passthroughStub,
  "v-skeleton-loader": passthroughStub,
  "v-spacer": { template: "<div class='v-spacer' />" },
  "v-switch": passthroughStub,
  "v-tab": passthroughStub,
  "v-table": passthroughStub,
  "v-tabs": passthroughStub,
  "v-text-field": textFieldStub,
  "v-textarea": textareaStub,
  "v-theme-provider": passthroughStub,
  "v-tooltip": passthroughStub,
  "v-window": passthroughStub,
  "v-window-item": passthroughStub,
};
