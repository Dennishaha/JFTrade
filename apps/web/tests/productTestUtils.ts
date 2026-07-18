import { defineComponent } from "vue";

const passthrough = (tag = "div") =>
  defineComponent({
    inheritAttrs: false,
    template: `<${tag} v-bind="$attrs"><slot /></${tag}>`,
  });

export const productGlobalStubs = {
  BrokerProviderTag: defineComponent({
    name: "BrokerProviderTag",
    props: {
      provider: { type: Object, default: null },
      featureId: { type: String, default: "" },
      market: { type: String, default: "" },
      menuLocation: { type: String, default: "bottom end" },
    },
    template: `
      <span
        class="broker-provider-tag-stub"
        :data-state="provider?.capability ?? ''"
        :data-feature-id="featureId"
        :data-market="market"
        :data-menu-location="menuLocation"
      >
        {{ provider?.brokerId?.toUpperCase() ?? '数据源' }}
      </span>
    `,
  }),
  "v-alert": passthrough(),
  "v-btn-toggle": defineComponent({
    name: "VBtnToggle",
    inheritAttrs: false,
    props: { modelValue: null },
    emits: ["update:modelValue"],
    template: `<div v-bind="$attrs"><slot /></div>`,
  }),
  "v-chip": passthrough("span"),
  "v-progress-linear": passthrough(),
  "v-menu": defineComponent({
    name: "VMenu",
    inheritAttrs: false,
    props: { modelValue: Boolean, location: String },
    emits: ["update:modelValue"],
    template: `
      <div class="v-menu-stub">
        <slot
          name="activator"
          :props="{
            'aria-expanded': modelValue ? 'true' : 'false',
            onClick: () => $emit('update:modelValue', !modelValue),
          }"
        />
        <slot v-if="modelValue" />
      </div>
    `,
  }),
  "v-table": passthrough("table"),
  "v-tabs": defineComponent({
    name: "VTabs",
    inheritAttrs: false,
    props: { modelValue: null },
    emits: ["update:modelValue"],
    template: `<div v-bind="$attrs"><slot /></div>`,
  }),
  "v-tab": defineComponent({
    emits: ["click"],
    template: `<button type="button" @click="$emit('click')"><slot /></button>`,
  }),
  "v-btn": defineComponent({
    inheritAttrs: false,
    props: {
      disabled: Boolean,
      loading: Boolean,
      modelValue: null,
      value: null,
    },
    emits: ["click", "update:modelValue"],
    template: `<button type="button" v-bind="$attrs" :disabled="disabled || loading" @click="$emit('click')"><slot /></button>`,
  }),
  "v-select": defineComponent({
    name: "VSelect",
    inheritAttrs: false,
    props: {
      modelValue: null,
      items: { type: Array, default: () => [] },
      itemTitle: { type: String, default: "title" },
      itemValue: { type: String, default: "value" },
    },
    emits: ["update:modelValue"],
    methods: {
      optionTitle(item: unknown): string {
        if (item == null || typeof item !== "object") return String(item ?? "");
        return String(
          (item as Record<string, unknown>)[this.itemTitle] ??
            (item as Record<string, unknown>).title ??
            item,
        );
      },
      optionValue(item: unknown): unknown {
        if (item == null || typeof item !== "object") return item;
        return (
          (item as Record<string, unknown>)[this.itemValue] ??
          (item as Record<string, unknown>).value
        );
      },
    },
    template: `
      <select v-bind="$attrs" :value="modelValue ?? ''" @change="$emit('update:modelValue', $event.target.value)">
        <option value=""></option>
        <option v-for="item in items" :key="String(optionValue(item))" :value="optionValue(item)">
          {{ optionTitle(item) }}
        </option>
      </select>
    `,
  }),
  "v-text-field": defineComponent({
    name: "VTextField",
    inheritAttrs: false,
    props: { modelValue: null, type: { type: String, default: "text" } },
    emits: ["update:modelValue"],
    template: `
      <input
        v-bind="$attrs"
        :type="type"
        :value="modelValue ?? ''"
        @input="$emit('update:modelValue', type === 'number' ? Number($event.target.value) : $event.target.value)"
      />
    `,
  }),
  "v-checkbox": defineComponent({
    name: "VCheckbox",
    inheritAttrs: false,
    props: { modelValue: Boolean },
    emits: ["update:modelValue"],
    template: `<input v-bind="$attrs" type="checkbox" :checked="modelValue" @change="$emit('update:modelValue', $event.target.checked)" />`,
  }),
  "v-pagination": defineComponent({
    name: "VPagination",
    inheritAttrs: false,
    props: { modelValue: Number },
    emits: ["update:modelValue"],
    template: `<button type="button" class="pagination-next" @click="$emit('update:modelValue', modelValue + 1)">next</button>`,
  }),
};

export function setupState<T extends object>(wrapper: {
  vm: { $: { setupState: object } };
}): T {
  return wrapper.vm.$.setupState as T;
}

export async function flushPromises(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
}
