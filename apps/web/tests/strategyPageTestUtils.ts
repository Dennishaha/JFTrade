import { mount } from "@vue/test-utils"
import { createPinia } from "pinia"
import { defineComponent, h, nextTick } from "vue"
import { createMemoryHistory, createRouter, RouterView } from "vue-router"

import { provideConsoleDataStore } from "../src/composables/useConsoleData"
import { queryClient } from "../src/composables/serverState"
import { provideThemeStore } from "../src/composables/useTheme"
import { provideUIColorPreferencesStore } from "../src/composables/useUIColorPreferences"
import { provideWorkspaceLayoutStore } from "../src/composables/useWorkspaceLayout"
import StrategyDesignPage from "../src/pages/StrategyDesignPage.vue"
import StrategyRuntimePage from "../src/pages/StrategyRuntimePage.vue"

import {
  MockWebSocket,
  collapseItemStub,
  collapseStub,
  dialogStub,
  flushRequests,
  passthroughStub,
} from "./helpers"
import {
  getCurrentConsoleDataStore as readCurrentConsoleDataStore,
  getCurrentStrategySystemStatus,
  resetStrategyPageSharedState,
  setCurrentConsoleDataStore,
} from "./strategyPageTestState"

const expansionTitleStub = defineComponent({
  emits: ["click"],
  template: "<button type='button' class='v-expansion-panel-title' @click=\"$emit('click')\"><slot /></button>",
})

const StrategyPageTestRoot = defineComponent({
  setup() {
    const themeStore = provideThemeStore()
    provideUIColorPreferencesStore(themeStore.theme)
    const workspaceLayout = provideWorkspaceLayoutStore()
    const consoleData = provideConsoleDataStore(workspaceLayout)
    setCurrentConsoleDataStore(consoleData)
    consoleData.systemStatus.value = getCurrentStrategySystemStatus()
    return () => h(RouterView)
  },
})

const RouteLeaveTargetPage = defineComponent({
  setup() {
    return () => h("div", "Route leave target")
  },
})

export async function mountStrategyPage(path = "/strategy") {
  queryClient.clear()
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/strategy", redirect: "/strategy/runtime" },
      { path: "/strategy/runtime", component: StrategyRuntimePage as never },
      { path: "/strategy/design", component: StrategyDesignPage as never },
      { path: "/route-leave-target", component: RouteLeaveTargetPage as never },
    ],
  })
  await router.push(path)
  await router.isReady()

  const wrapper = mount(StrategyPageTestRoot, {
    global: {
      plugins: [createPinia(), router],
      stubs: {
        "v-dialog": dialogStub,
        "v-expansion-panels": collapseStub,
        "v-expansion-panel": collapseItemStub,
        "v-expansion-panel-title": expansionTitleStub,
        "v-expansion-panel-text": passthroughStub,
      },
    },
  })

  await flushRequests()

  return { router, wrapper }
}

export type StrategyPageWrapper = Awaited<ReturnType<typeof mountStrategyPage>>["wrapper"]

export function resetStrategyPageTestState() {
  queryClient.clear()
  MockWebSocket.instances = []
  resetStrategyPageSharedState()
}

export function getCurrentConsoleDataStore() {
  return readCurrentConsoleDataStore()
}

export async function settleStrategyWorkspace() {
  await flushRequests()
  await flushRequests()
  await nextTick()
  await nextTick()
}

export async function showStrategyCodeEditor(
  wrapper: StrategyPageWrapper,
  mode: "split" | "code" = "code",
) {
  await wrapper
    .get(`[data-testid="strategy-display-mode-${mode}"]`)
    .trigger("click")
  await settleStrategyWorkspace()
}

export async function openNewStrategyFromRuntime(
  wrapper: StrategyPageWrapper,
) {
  for (let attempt = 0; attempt < 4; attempt += 1) {
    if (wrapper.find('[data-testid="strategy-templates-section"]').exists()) {
      return
    }
    await wrapper.get('[data-testid="strategy-create-menu-toggle"]').trigger("click")
    await settleStrategyWorkspace()
    await wrapper.get('[data-testid="strategy-new-definition"]').trigger("click")
    await settleStrategyWorkspace()
    if (wrapper.find('[data-testid="strategy-templates-section"]').exists()) {
      return
    }
  }
}

export async function openCreateInstancePanel(
  wrapper: StrategyPageWrapper,
) {
  for (let attempt = 0; attempt < 4; attempt += 1) {
    if (wrapper.find('[data-testid="strategy-create-instance-panel"]').exists()) {
      return
    }
    await wrapper.get('[data-testid="strategy-create-menu-toggle"]').trigger("click")
    await settleStrategyWorkspace()
    await wrapper.get('[data-testid="strategy-new-instance"]').trigger("click")
    await settleStrategyWorkspace()
    if (wrapper.find('[data-testid="strategy-create-instance-panel"]').exists()) {
      return
    }
  }
}

export async function appendSymbolTags(
  wrapper: StrategyPageWrapper,
  selector: string,
  symbols: string[],
) {
  for (const symbol of symbols) {
    const input = wrapper.get(selector)
    await input.setValue(symbol)
    await input.trigger("keydown", { key: "Enter" })
    await settleStrategyWorkspace()
  }
}

export async function appendInstrumentTags(
  wrapper: StrategyPageWrapper,
  selectors: {
    market: string
    code: string
  },
  instruments: Array<{
    market: string
    code: string
  }>,
) {
  for (const instrument of instruments) {
    await wrapper.get(selectors.market).setValue(instrument.market)
    await wrapper.get(selectors.code).setValue(instrument.code)
    await wrapper.get(selectors.code).trigger("keydown", { key: "Enter" })
    await settleStrategyWorkspace()
  }
}

export async function openStrategyTemplatesPanel(
  wrapper: StrategyPageWrapper,
) {
  await ensureStrategyDesignWorkspace(wrapper)
  if (wrapper.find('[data-testid="strategy-templates-section"]').exists()) {
    return
  }
  await wrapper.get('[data-testid="toggle-strategy-templates-section"]').trigger("click")
  await settleStrategyWorkspace()
}

export async function waitForSelector(
  wrapper: StrategyPageWrapper,
  selector: string,
  attempts = 40,
) {
  for (let index = 0; index < attempts; index += 1) {
    if (wrapper.find(selector).exists()) {
      return
    }
    await settleStrategyWorkspace()
  }
}

async function ensureStrategyDesignWorkspace(wrapper: StrategyPageWrapper) {
  if (wrapper.find('[data-testid="strategy-display-mode-code"]').exists()) {
    return
  }
  await wrapper.get('[data-testid="strategy-workspace-tab-design"]').trigger("click")
  await settleStrategyWorkspace()
}

export {
  buildFetchMock,
  buildPineScript,
  buildRuntimeAccount,
  MockWebSocket,
  flushRequests,
} from "./strategyPageMockApi"
