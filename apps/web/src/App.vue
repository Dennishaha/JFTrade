<script setup lang="ts">
import { computed, ref } from "vue";
import { RouterView, useRoute } from "vue-router";

import AuthGate from "./components/AuthGate.vue";
import AppShell from "./layout/AppShell.vue";
import { resolveAuthRequired } from "./runtimeConfig";

const authenticated = ref(!resolveAuthRequired());
const route = useRoute();
const standalone = computed(() => route.meta.standalone === true);
</script>

<template>
  <AuthGate
    v-if="!authenticated && !standalone"
    @authenticated="authenticated = true"
  />
  <RouterView v-else-if="standalone" />
  <AppShell v-else />
</template>
