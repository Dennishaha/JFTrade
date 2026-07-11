<script setup lang="ts">
import { computed } from "vue";
import { useRoute, useRouter } from "vue-router";

import { useDocsLink } from "../composables/useDocsLink";
import { useExternalLink } from "../composables/externalLink";

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
const { handleExternalLinkClick } = useExternalLink();

const items: NavItem[] = [
  { type: "route", to: "/workspace", label: "交易", icon: "fa-solid fa-display" },
  { type: "route", to: "/watchlist", label: "自选股", icon: "fa-regular fa-star" },
  { type: "route", to: "/adk/agents", label: "智能体", icon: "fa-solid fa-robot" },
  { type: "route", to: "/strategy/runtime", label: "策略执行", icon: "fa-solid fa-play" },
  { type: "route", to: "/strategy/design", label: "策略设计", icon: "fa-solid fa-wand-magic-sparkles" },
  { type: "route", to: "/backtest", label: "回测", icon: "fa-solid fa-flask" },
  { type: "route", to: "/account", label: "我的账户", icon: "fa-solid fa-wallet" },
  { type: "route", to: "/risk", label: "风控", icon: "fa-solid fa-shield-halved" },
  { type: "route", to: "/system", label: "系统", icon: "fa-solid fa-microchip" },
  { type: "route", to: "/settings", label: "设置", icon: "fa-solid fa-gear" },
  { type: "external", href: docsHomeUrl, label: "文档", icon: "fa-solid fa-file-lines" },
];

const route = useRoute();
const router = useRouter();

const activeTo = computed(() => route.path);

function isRouteActive(to: string): boolean {
  if (to === "/adk/agents") return activeTo.value.startsWith("/adk/");
  return activeTo.value === to;
}

function go(to: string): void {
  void router.push(to);
}
</script>

<template>
  <nav class="tv-iconrail" aria-label="主导航">
    <template v-for="item in items" :key="item.label">
      <a v-if="item.type === 'external'" :href="item.href" target="_blank" rel="noopener noreferrer"
        class="tv-iconrail-btn" :title="item.label" @click="handleExternalLinkClick($event, item.href)">
        <v-icon class="tv-iconrail-glyph tv-iconrail-glyph--external">{{ item.icon }}</v-icon>
        <span>{{ item.label }}</span>
      </a>
      <button v-else type="button" class="tv-iconrail-btn" :class="{ 'is-active': isRouteActive(item.to) }"
        :title="item.label" @click="go(item.to)">
        <v-icon class="tv-iconrail-glyph">{{ item.icon }}</v-icon>
        <span>{{ item.label }}</span>
      </button>
    </template>
  </nav>
</template>
