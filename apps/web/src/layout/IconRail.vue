<script setup lang="ts">
import { computed } from "vue";
import { useRoute, useRouter } from "vue-router";

import { useDocsLink } from "../composables/useDocsLink";

interface InternalNavItem {
  type: "route";
  to: string;
  label: string;
  icon: string;
}

interface ExternalNavItem {
  type: "external";
  href: string;
  label: string;
  icon: string;
}

type NavItem = InternalNavItem | ExternalNavItem;

const { docsHomeUrl } = useDocsLink();

const items: NavItem[] = [
  { type: "route", to: "/workspace", label: "Trade", icon: "fa-solid fa-display" },
  { type: "route", to: "/overview", label: "Overview", icon: "fa-solid fa-chart-column" },
  { type: "route", to: "/market", label: "Market", icon: "fa-solid fa-chart-line" },
  { type: "route", to: "/strategy", label: "Strategy", icon: "fa-solid fa-wand-magic-sparkles" },
  { type: "route", to: "/backtest", label: "回测", icon: "fa-solid fa-flask" },
  { type: "route", to: "/execution", label: "Execution", icon: "fa-solid fa-list" },
  { type: "route", to: "/portfolio", label: "Portfolio", icon: "fa-solid fa-wallet" },
  { type: "route", to: "/broker", label: "Broker", icon: "fa-solid fa-plug" },
  { type: "route", to: "/risk", label: "Risk", icon: "fa-solid fa-triangle-exclamation" },
  { type: "route", to: "/system", label: "System", icon: "fa-solid fa-microchip" },
  { type: "route", to: "/settings", label: "Settings", icon: "fa-solid fa-gear" },
  { type: "external", href: docsHomeUrl, label: "Docs", icon: "fa-solid fa-file-lines" },
];

const route = useRoute();
const router = useRouter();

const activeTo = computed(() => route.path);

function go(to: string): void {
  void router.push(to);
}
</script>

<template>
  <nav class="tv-iconrail" aria-label="primary">
    <template v-for="item in items" :key="item.label">
      <a
        v-if="item.type === 'external'"
        :href="item.href"
        target="_blank"
        rel="noopener noreferrer"
        class="tv-iconrail-btn"
        :title="item.label"
      >
        <v-icon class="tv-iconrail-glyph" :size="20">{{ item.icon }}</v-icon>
        <span>{{ item.label }}</span>
      </a>
      <button
        v-else
        type="button"
        class="tv-iconrail-btn"
        :class="{ 'is-active': activeTo === item.to }"
        :title="item.label"
        @click="go(item.to)"
      >
        <v-icon class="tv-iconrail-glyph" :size="18">{{ item.icon }}</v-icon>
        <span>{{ item.label }}</span>
      </button>
    </template>
  </nav>
</template>
