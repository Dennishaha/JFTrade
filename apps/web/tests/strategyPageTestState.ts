import type { SystemStatusResponse } from "@/contracts"
import { emptySystemStatus } from "@/contracts"

import type { provideConsoleDataStore } from "../src/composables/useConsoleData"

let currentStrategySystemStatus: SystemStatusResponse = emptySystemStatus
let currentConsoleDataStore: ReturnType<typeof provideConsoleDataStore> | null = null

export function resetStrategyPageSharedState() {
  currentStrategySystemStatus = emptySystemStatus
  currentConsoleDataStore = null
}

export function setCurrentStrategySystemStatus(systemStatus: SystemStatusResponse) {
  currentStrategySystemStatus = systemStatus
}

export function getCurrentStrategySystemStatus() {
  return currentStrategySystemStatus
}

export function setCurrentConsoleDataStore(store: ReturnType<typeof provideConsoleDataStore> | null) {
  currentConsoleDataStore = store
}

export function getCurrentConsoleDataStore() {
  return currentConsoleDataStore
}
