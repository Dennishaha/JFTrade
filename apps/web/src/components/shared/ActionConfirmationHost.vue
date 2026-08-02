<script setup lang="ts">
import type { ActionConfirmationController } from "@/composables/shared/useActionConfirmation";

import ActionConfirmDialog from "./ActionConfirmDialog.vue";

const props = defineProps<{
  controller: ActionConfirmationController;
}>();

const { pendingConfirmation, confirmationOpen } = props.controller;
</script>

<template>
  <ActionConfirmDialog
    :open="confirmationOpen"
    :title="pendingConfirmation?.title ?? ''"
    :message="pendingConfirmation?.message ?? ''"
    :confirm-label="pendingConfirmation?.confirmLabel ?? '确认'"
    :confirmation-text="pendingConfirmation?.confirmationText ?? ''"
    @close="props.controller.cancelConfirmation"
    @confirm="props.controller.confirmConfirmation"
  />
</template>
