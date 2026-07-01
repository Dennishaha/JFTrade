<script setup lang="ts">
defineProps<{
  errorMessage: string;
  successMessage: string;
  runHref: string;
}>();

defineEmits<{
  dismissError: [];
  dismissSuccess: [];
}>();
</script>

<template>
  <div v-if="errorMessage || successMessage" class="adk-workflow-notices">
    <div v-if="errorMessage" class="adk-workflow-notice is-warning" role="status" aria-live="polite">
      <v-icon size="13">fa-solid fa-triangle-exclamation</v-icon>
      <span>{{ errorMessage }}</span>
      <button type="button" class="adk-workflow-notice__close" @click="$emit('dismissError')">
        <v-icon size="12">fa-solid fa-xmark</v-icon>
      </button>
    </div>
    <div v-if="successMessage" class="adk-workflow-notice is-success" role="status" aria-live="polite">
      <v-icon size="13">fa-solid fa-circle-check</v-icon>
      <span>{{ successMessage }}</span>
      <a v-if="runHref" class="adk-workflow-link" :href="runHref">打开运行</a>
      <button type="button" class="adk-workflow-notice__close" @click="$emit('dismissSuccess')">
        <v-icon size="12">fa-solid fa-xmark</v-icon>
      </button>
    </div>
  </div>
</template>
