<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import { RouterView, useRoute } from "vue-router";

import AuthGate from "@/components/auth/AuthGate.vue";
import DesktopStartupGate from "@/components/app-shell/DesktopStartupGate.vue";
import { WEB_AUTH_REQUIRED_EVENT } from "@/composables/shared/apiClient";
import AppShell from "./layout/AppShell.vue";
import {
  resolveAuthRequired,
  resolveDesktopBridgeAvailable,
  resolveDesktopMode,
} from "./runtimeConfig";

const desktopMode = resolveDesktopMode();
const desktopStartupGateEnabled =
  desktopMode && resolveDesktopBridgeAvailable();
const desktopReady = ref(!desktopStartupGateEnabled);
const authenticated = ref(desktopMode || !resolveAuthRequired());
const route = useRoute();
const standalone = computed(
  () => desktopMode && route.meta.standalone === true,
);

function handleWebAuthRequired(): void {
  if (!desktopMode) {
    authenticated.value = false;
  }
}

onMounted(() => {
  window.addEventListener(WEB_AUTH_REQUIRED_EVENT, handleWebAuthRequired);
});

onUnmounted(() => {
  window.removeEventListener(WEB_AUTH_REQUIRED_EVENT, handleWebAuthRequired);
});
</script>

<template>
  <AuthGate
    v-if="!authenticated && !standalone"
    @authenticated="authenticated = true"
  />
  <RouterView v-else-if="standalone" />
  <DesktopStartupGate
    v-else-if="!desktopReady"
    @ready="desktopReady = true"
  />
  <AppShell v-else />
</template>
