<script setup lang="ts">
export interface AsyncPanelPartialError {
  code: string;
  message: string;
  scope: string;
}

withDefaults(
  defineProps<{
    loading?: boolean;
    error?: string | null;
    warnings?: string[];
    partialErrors?: AsyncPanelPartialError[];
    warningType?: "error" | "info" | "success" | "warning";
    progressClass?: string;
  }>(),
  {
    loading: false,
    error: null,
    warnings: () => [],
    partialErrors: () => [],
    warningType: "warning",
    progressClass: "",
  },
);
</script>

<template>
  <v-progress-linear
    v-if="loading"
    :class="progressClass || undefined"
    indeterminate
  />
  <v-alert v-if="error" type="warning" variant="tonal" density="compact">
    {{ error }}
  </v-alert>
  <!-- 默认插槽：渲染在错误告警之后、warnings 之前的附加内容（额外告警、指标等） -->
  <slot />
  <v-alert
    v-for="warning in warnings"
    :key="warning"
    :type="warningType"
    variant="tonal"
    density="compact"
  >
    {{ warning }}
  </v-alert>
  <v-alert
    v-for="partialError in partialErrors"
    :key="`${partialError.scope}-${partialError.code}`"
    type="warning"
    variant="outlined"
    density="compact"
  >
    <slot name="partial-error" :partial-error="partialError">
      {{ partialError.scope }} · {{ partialError.message }}
    </slot>
  </v-alert>
</template>
