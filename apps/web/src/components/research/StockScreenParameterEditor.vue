<script setup lang="ts">
import { computed } from "vue";

import { parameterLabel } from "./stockScreenModel";
import type {
  StockScreenEnumOption,
  StockScreenFactorParameter,
  StockScreenFactorRef,
  StockScreenFactorParams,
} from "./stockScreenTypes";

const props = withDefaults(
  defineProps<{
    reference: StockScreenFactorRef;
    parameters: StockScreenFactorParameter[];
    enums: Record<string, StockScreenEnumOption[]>;
    labelPrefix?: string;
    errorPrefix?: string;
    validationErrors?: Array<{ path: string; message: string }>;
    compact?: boolean;
  }>(),
  {
    labelPrefix: "",
    errorPrefix: "",
    validationErrors: () => [],
    compact: false,
  },
);

const visibleParameters = computed(() =>
  props.parameters.filter((parameter) => {
    const dependency = parameter.visibleWhen;
    if (!dependency) return true;
    const values = props.reference.params as
      | Record<string, unknown>
      | undefined;
    return Object.entries(dependency).every(
      ([key, expected]) => values?.[key] === expected,
    );
  }),
);

function enumOptions(parameter: StockScreenFactorParameter) {
  return parameter.enum ? (props.enums[parameter.enum] ?? []) : [];
}

function parameterValue(parameter: StockScreenFactorParameter): string | number {
  const value = (
    props.reference.params as Record<string, unknown> | undefined
  )?.[parameter.name];
  return Array.isArray(value) ? value.join(",") : String(value ?? "");
}

function updateParameter(
  parameter: StockScreenFactorParameter,
  event: Event,
): void {
  const raw = (event.target as HTMLInputElement | HTMLSelectElement).value;
  props.reference.params ??= {};
  const params = props.reference.params as Record<string, unknown>;
  if (raw === "") {
    delete params[parameter.name];
    return;
  }
  if (
    parameter.type === "integer_array" ||
    parameter.editorType === "multiNumber"
  ) {
    params[parameter.name] = raw
      .split(",")
      .map((item) => Number(item.trim()))
      .filter(Number.isFinite);
    return;
  }
  if (
    parameter.type === "integer" ||
    parameter.type === "number" ||
    parameter.editorType === "number" ||
    parameter.enum
  ) {
    params[parameter.name] = Number(raw);
    return;
  }
  params[parameter.name] = raw;
}

function unionType(): number {
  return props.reference.params?.optionParamType ?? 1;
}

function updateUnionType(event: Event): void {
  const nextType = Number((event.target as HTMLSelectElement).value);
  props.reference.params ??= {};
  const params = props.reference.params;
  params.optionParamType = nextType;
  delete params.optionParamString;
  delete params.optionParamInteger;
  delete params.optionParamIntegers;
}

function unionValue(): string | number {
  const params = props.reference.params;
  switch (unionType()) {
    case 2:
      return params?.optionParamInteger ?? "";
    case 3:
      return params?.optionParamIntegers?.join(",") ?? "";
    default:
      return params?.optionParamString ?? "";
  }
}

function updateUnionValue(event: Event): void {
  const raw = (event.target as HTMLInputElement).value;
  props.reference.params ??= {};
  const params: StockScreenFactorParams = props.reference.params;
  delete params.optionParamString;
  delete params.optionParamInteger;
  delete params.optionParamIntegers;
  if (raw === "") return;
  switch (unionType()) {
    case 2:
      params.optionParamInteger = Number(raw);
      break;
    case 3:
      params.optionParamIntegers = raw
        .split(",")
        .map((item) => Number(item.trim()))
        .filter(Number.isFinite);
      break;
    default:
      params.optionParamString = raw;
      break;
  }
}

function errorFor(parameter: StockScreenFactorParameter): string {
  if (!props.errorPrefix) return "";
  const path = `${props.errorPrefix}.params.${parameter.name}`;
  return (
    props.validationErrors.find(
      (error) => error.path === path || error.path.startsWith(`${path}.`),
    )?.message ?? ""
  );
}
</script>

<template>
  <div
    class="stock-screen-parameter-editor"
    :class="{ 'stock-screen-parameter-editor--compact': compact }"
  >
    <label
      v-for="parameter in visibleParameters"
      :key="parameter.name"
      :title="parameter.help"
    >
      <span>
        {{ labelPrefix }}{{ parameterLabel(parameter) }}
        <small v-if="parameter.unit">{{ parameter.unit }}</small>
      </span>
      <template
        v-if="parameter.editorType === 'union' || parameter.name === 'optionParam'"
      >
        <div class="stock-screen-parameter-editor__union">
          <select
            :value="unionType()"
            :aria-label="`${labelPrefix}${parameterLabel(parameter)}类型`"
            @change="updateUnionType"
          >
            <option :value="1">文本</option>
            <option :value="2">整数</option>
            <option :value="3">整数数组</option>
          </select>
          <input
            :type="unionType() === 2 ? 'number' : 'text'"
            :value="unionValue()"
            :placeholder="unionType() === 3 ? '逗号分隔' : ''"
            :aria-label="`${labelPrefix}${parameterLabel(parameter)}值`"
            @input="updateUnionValue"
          />
        </div>
      </template>
      <select
        v-else-if="enumOptions(parameter).length"
        :value="parameterValue(parameter)"
        :aria-label="`${labelPrefix}${parameterLabel(parameter)}`"
        @change="updateParameter(parameter, $event)"
      >
        <option v-if="!parameter.required" value="">默认</option>
        <option
          v-for="option in enumOptions(parameter)"
          :key="option.key"
          :value="option.value"
        >
          {{ option.label }}
        </option>
      </select>
      <input
        v-else
        :type="
          parameter.editorType === 'date'
            ? 'date'
            : parameter.type === 'integer' ||
                parameter.type === 'number' ||
                parameter.editorType === 'number'
              ? 'number'
              : 'text'
        "
        :value="parameterValue(parameter)"
        :min="parameter.minimum"
        :max="parameter.maximum"
        :step="parameter.step"
        :placeholder="
          parameter.type === 'integer_array' ||
          parameter.editorType === 'multiNumber'
            ? '逗号分隔'
            : ''
        "
        :aria-label="`${labelPrefix}${parameterLabel(parameter)}`"
        @input="updateParameter(parameter, $event)"
      />
      <small v-if="errorFor(parameter)" class="stock-screen-parameter-editor__error">
        {{ errorFor(parameter) }}
      </small>
    </label>
  </div>
</template>

<style scoped>
.stock-screen-parameter-editor {
  display: flex;
  width: 100%;
  max-width: 100%;
  min-width: 0;
  flex-wrap: wrap;
  gap: 6px;
}

.stock-screen-parameter-editor > label {
  display: grid;
  min-width: 0;
  flex: 1 1 110px;
  gap: 2px;
}

.stock-screen-parameter-editor > label > span {
  color: var(--tv-text-muted);
  font-size: 10px;
}

.stock-screen-parameter-editor > label > span small {
  margin-left: 4px;
  color: var(--tv-text-dim);
  font-size: 9px;
}

.stock-screen-parameter-editor input,
.stock-screen-parameter-editor select {
  min-width: 0;
  min-height: 28px;
  padding: 0 6px;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  background: var(--tv-bg-surface);
  color: var(--tv-text);
  font: inherit;
}

.stock-screen-parameter-editor__union {
  display: grid;
  min-width: 0;
  grid-template-columns: minmax(0, 0.65fr) minmax(0, 1fr);
  gap: 4px;
}

.stock-screen-parameter-editor__error {
  color: #d55353;
  font-size: 10px;
}

.stock-screen-parameter-editor--compact {
  width: 100%;
  flex-basis: 100%;
}
</style>
