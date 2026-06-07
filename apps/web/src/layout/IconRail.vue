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
  { type: "route", to: "/workspace", label: "交易", icon: "fa-solid fa-display" },
  { type: "route", to: "/overview", label: "概览", icon: "fa-solid fa-chart-column" },
  { type: "route", to: "/market", label: "行情", icon: "fa-solid fa-chart-line" },
  { type: "route", to: "/strategy", label: "策略", icon: "fa-solid fa-wand-magic-sparkles" },
  { type: "route", to: "/adk", label: "Agents", icon: "fa-solid fa-robot" },
  { type: "route", to: "/backtest", label: "回测", icon: "fa-solid fa-flask" },
  { type: "route", to: "/account", label: "我的账户", icon: "fa-solid fa-wallet" },
  { type: "route", to: "/risk", label: "风控", icon: "fa-solid fa-triangle-exclamation" },
  { type: "route", to: "/system", label: "系统", icon: "fa-solid fa-microchip" },
  { type: "route", to: "/settings", label: "设置", icon: "fa-solid fa-gear" },
  { type: "external", href: docsHomeUrl, label: "文档", icon: "fa-solid fa-file-lines" },
];

const route = useRoute();
const router = useRouter();

const activeTo = computed(() => route.path);

function go(to: string): void {
  void router.push(to);
}
</script>

<template>
  <nav class="tv-iconrail" aria-label="主导航">
    <template v-for="item in items" :key="item.label">
      <a
        v-if="item.type === 'external'"
        :href="item.href"
        target="_blank"
        rel="noopener noreferrer"
        class="tv-iconrail-btn"
        :title="item.label"
      >
        <v-icon class="tv-iconrail-glyph tv-iconrail-glyph--external">{{ item.icon }}</v-icon>
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
        <v-icon class="tv-iconrail-glyph">{{ item.icon }}</v-icon>
        <span>{{ item.label }}</span>
      </button>
    </template>
  </nav>
</template>
