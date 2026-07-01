<script setup lang="ts">
defineProps<{
  modelValue: boolean;
  secret: string;
}>();

defineEmits<{
  "update:modelValue": [value: boolean];
}>();
</script>

<template>
  <v-dialog
    :model-value="modelValue"
    max-width="720"
    @update:model-value="$emit('update:modelValue', Boolean($event))"
  >
    <v-card>
      <v-card-title>网络回调密钥</v-card-title>
      <v-card-text>
        <pre class="adk-workflow-secret">{{ secret }}</pre>
      </v-card-text>
      <v-card-actions class="justify-end">
        <v-btn color="primary" @click="$emit('update:modelValue', false)">关闭</v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>
