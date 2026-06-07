<script setup lang="ts">
import { ref } from "vue";

import AuthGate from "./components/AuthGate.vue";
import AppShell from "./layout/AppShell.vue";
import { resolveAuthRequired } from "./runtimeConfig";

const authenticated = ref(!resolveAuthRequired());
</script>

<template>
  <AuthGate v-if="!authenticated" @authenticated="authenticated = true" />
  <AppShell v-else />
</template>
