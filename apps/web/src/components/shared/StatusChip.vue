<script setup lang="ts">
import { computed } from "vue";

import { statusTone } from "@/composables/shared/statusTone";

type StatusChipVariant = "flat" | "text" | "elevated" | "tonal" | "outlined" | "plain";

const props = withDefaults(
  defineProps<{
    /** 状态词（大小写/连字符不敏感），决定默认配色与默认文案。 */
    status: string;
    /** 展示文案；缺省时原样展示 status（保留各域现有原文）。 */
    label?: string;
    /** 域专属配色覆盖（如 ADK 的 CANCELLED→grey）；缺省时取 statusTone 共享配色。 */
    color?: string;
    size?: string;
    variant?: StatusChipVariant;
  }>(),
  {
    label: "",
    color: "",
    size: "small",
    variant: "tonal",
  },
);

const tone = computed(() => statusTone(props.status));
const chipColor = computed(() => (props.color !== "" ? props.color : tone.value.color));
const chipLabel = computed(() => (props.label !== "" ? props.label : props.status));
</script>

<template>
  <v-chip :color="chipColor" :size="size" :variant="variant">{{ chipLabel }}</v-chip>
</template>
