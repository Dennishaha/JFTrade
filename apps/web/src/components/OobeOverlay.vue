<script setup lang="ts">
import { computed, defineAsyncComponent, ref, watch } from "vue";
import { useRouter } from "vue-router";

import type { RuntimeDependenciesResponse } from "@/contracts";
import RuntimeDependenciesSection from "./RuntimeDependenciesSection.vue";
import SettingsAccountDiscoverySection from "./SettingsAccountDiscoverySection.vue";
import SettingsManagedAccountsSection from "./SettingsManagedAccountsSection.vue";
import { createSettingsManagedAccountsController } from "../composables/settingsManagedAccounts";
import { useConsoleData } from "../composables/useConsoleData";

const FutuIntegrationSection = defineAsyncComponent(
  () =>
    import("./FutuIntegrationSection.vue").then(
      ({ default: component }) => component,
    ),
);

const console_ = useConsoleData();
const router = useRouter();

const activeStep = ref<"dependencies" | "broker" | "connection" | "account">(
  "dependencies",
);
const savingOnboarding = ref(false);
const runtimeDependenciesSatisfied = ref(false);
const dependencyWarningSkipped = ref(false);
const selectedBrokerId = ref(
  console_.onboardingState.value.state.lastBrokerId || "",
);

watch(
  () => console_.onboardingState.value.state.lastBrokerId,
  (brokerId) => {
    if (selectedBrokerId.value === "" && brokerId !== "") {
      selectedBrokerId.value = brokerId;
    }
  },
);

const selectedBroker = computed(
  () =>
    console_.onboardingState.value.brokers.find(
      (broker) => broker.descriptor.id === selectedBrokerId.value,
    ) ?? null,
);

const savedFutuIntegration = computed(
  () =>
    console_.brokerSettings.value.brokers.find(
      (broker) => broker.descriptor.id === "futu",
    )?.integration ?? null,
);

const hasSavedIntegration = computed(() => savedFutuIntegration.value != null);
const isSavedEnabled = computed(
  () => savedFutuIntegration.value?.enabled === true,
);
const hasBlockingIssue = computed(
  () =>
    selectedBrokerId.value === "futu" &&
    activeStep.value !== "broker" &&
    hasSavedIntegration.value &&
    isSavedEnabled.value &&
    (console_.futuOpenDHealth.value.diagnosis.manualRetryRequired ||
      console_.brokerRuntime.value.session.connectivity !== "connected"),
);

const connectionStatusLabel = computed(() => {
  if (!hasSavedIntegration.value) {
    return "待保存";
  }
  if (!isSavedEnabled.value) {
    return "已停用";
  }
  switch (console_.brokerRuntime.value.session.connectivity) {
    case "connected":
      return "已连接";
    case "degraded":
      return "部分可用";
    default:
      return "未连接";
  }
});

const canEnterAccountStep = computed(
  () =>
    selectedBrokerId.value === "futu" &&
    hasSavedIntegration.value &&
    isSavedEnabled.value &&
    console_.brokerRuntime.value.session.connectivity === "connected",
);

const accountStepHint = computed(() => {
  if (selectedBrokerId.value === "") {
    return "先选择一个券商。";
  }
  if (selectedBrokerId.value !== "futu") {
    return "";
  }
  if (!hasSavedIntegration.value) {
    return "先填写并保存富途连接配置，然后再检测 OpenD。";
  }
  if (!isSavedEnabled.value) {
    return "当前富途接入已停用。启用并保存后，才能继续确认账户。";
  }
  if (console_.brokerRuntime.value.session.connectivity !== "connected") {
    return "等待 OpenD 连接成功后，才能进入账户确认步骤。";
  }
  return "";
});

const accountDiscoveryUnavailableMessage = computed(() => {
  if (!hasSavedIntegration.value) {
    return "请先在上一步填写并保存富途连接配置，随后 JFTrade 才会尝试发现 OpenD 账户。";
  }
  if (!isSavedEnabled.value) {
    return "当前富途接入已停用。启用并保存后，JFTrade 才会尝试发现 OpenD 账户。";
  }
  if (console_.brokerRuntime.value.session.connectivity !== "connected") {
    return "OpenD 尚未连接成功。连接恢复后，这里会显示发现到的账户。";
  }
  return undefined;
});

const canEnterBrokerStep = computed(
  () => runtimeDependenciesSatisfied.value || dependencyWarningSkipped.value,
);

watch(canEnterAccountStep, (canEnter) => {
  if (!canEnter && activeStep.value === "account") {
    activeStep.value = "connection";
  }
});

const managedAccountsController = createSettingsManagedAccountsController({
  brokerRuntime: console_.brokerRuntime,
  brokerSettings: console_.brokerSettings,
  createManagedBrokerAccount: console_.createManagedBrokerAccount,
  updateManagedBrokerAccount: console_.updateManagedBrokerAccount,
  deleteManagedBrokerAccount: console_.deleteManagedBrokerAccount,
});
const {
  accountForm,
  deletingAccountId,
  editingAccountId,
  importRuntimeAccount,
  populateAccountForm,
  removeAccount,
  resetAccountForm,
  savingAccount,
  submitAccount,
} = managedAccountsController;

async function completeOnboarding(dismissed = false): Promise<void> {
  savingOnboarding.value = true;
  try {
    await console_.saveOnboardingState({
      completed: true,
      dismissed,
      lastBrokerId: selectedBrokerId.value,
    });
    await router.replace("/workspace");
  } finally {
    savingOnboarding.value = false;
  }
}

async function openSettings(): Promise<void> {
  await completeOnboarding(true);
  await router.push("/settings");
}

async function selectBroker(brokerId: string): Promise<void> {
  selectedBrokerId.value = brokerId;
  await console_.saveOnboardingState({
    completed: false,
    lastBrokerId: brokerId,
  });
}

function handleRuntimeDependencyStatus(
  response: RuntimeDependenciesResponse,
): void {
  runtimeDependenciesSatisfied.value = response.allRequiredSatisfied;
  if (response.allRequiredSatisfied) {
    dependencyWarningSkipped.value = false;
  }
}

function goToBrokerStep(): void {
  if (!runtimeDependenciesSatisfied.value) {
    dependencyWarningSkipped.value = true;
  }
  activeStep.value = "broker";
}

async function goToConnectionStep(): Promise<void> {
  if (!canEnterBrokerStep.value) {
    activeStep.value = "dependencies";
    return;
  }
  if (selectedBrokerId.value === "") {
    await selectBroker(
      console_.onboardingState.value.recommendedBrokerId || "futu",
    );
  }
  activeStep.value = "connection";
}

async function goToAccountStep(): Promise<void> {
  if (!canEnterBrokerStep.value) {
    activeStep.value = "dependencies";
    return;
  }
  if (selectedBrokerId.value === "") {
    await selectBroker(
      console_.onboardingState.value.recommendedBrokerId || "futu",
    );
  }
  if (!canEnterAccountStep.value) {
    activeStep.value = "connection";
    return;
  }
  activeStep.value = "account";
}
</script>

<template>
  <div class="oobe-page">
    <section class="oobe-shell" aria-label="首次配置">
      <header class="oobe-header">
        <div>
          <div class="oobe-eyebrow">JFTrade OOBE</div>
          <h1>运行时依赖与券商接入配置</h1>
          <p>
            先确认策略系统需要的本机依赖，再选择券商、保存 OpenD
            连接信息，并确认一个可用账户。
          </p>
        </div>
        <div class="oobe-header-actions">
          <v-btn
            variant="text"
            color="primary"
            :loading="savingOnboarding"
            @click="completeOnboarding(true)"
          >
            跳过
          </v-btn>
          <v-btn
            color="primary"
            :loading="savingOnboarding"
            @click="completeOnboarding(false)"
          >
            进入工作台
          </v-btn>
        </div>
      </header>

      <v-alert
        v-if="hasBlockingIssue"
        class="oobe-alert"
        type="warning"
        :closable="false"
      >
        <div class="oobe-alert-title">检测到券商连接中断</div>
        <p>
          {{
            console_.futuOpenDHealth.value.diagnosis.summary ||
            console_.brokerRuntime.value.session.lastError ||
            "OpenD 尚未连接或尚未登录。"
          }}
        </p>
      </v-alert>

      <div class="oobe-steps">
        <button
          type="button"
          :class="{ active: activeStep === 'dependencies' }"
          @click="activeStep = 'dependencies'"
        >
          <span>1</span>
          运行时依赖
        </button>
        <button
          type="button"
          :class="{ active: activeStep === 'broker' }"
          :disabled="!canEnterBrokerStep"
          @click="activeStep = 'broker'"
        >
          <span>2</span>
          选择券商
        </button>
        <button
          type="button"
          :class="{ active: activeStep === 'connection' }"
          :disabled="!canEnterBrokerStep"
          @click="goToConnectionStep"
        >
          <span>3</span>
          连接配置
        </button>
        <button
          type="button"
          :class="{ active: activeStep === 'account' }"
          :disabled="!canEnterBrokerStep"
          @click="goToAccountStep"
        >
          <span>4</span>
          确认账户
        </button>
      </div>

      <main class="oobe-content">
        <section v-show="activeStep === 'dependencies'" class="oobe-panel">
          <RuntimeDependenciesSection
            mode="oobe"
            @status-change="handleRuntimeDependencyStatus"
          />
          <div class="oobe-footer-actions">
            <v-btn color="primary" @click="goToBrokerStep">
              {{
                runtimeDependenciesSatisfied ? "下一步" : "继续配置券商"
              }}
            </v-btn>
          </div>
        </section>

        <section v-show="activeStep === 'broker'" class="oobe-panel">
          <div class="oobe-section-title">选择券商</div>
          <div class="oobe-broker-grid">
            <button
              v-for="broker in console_.onboardingState.value.brokers"
              :key="broker.descriptor.id"
              type="button"
              class="oobe-broker-card"
              :class="{ selected: broker.descriptor.id === selectedBrokerId }"
              :disabled="!broker.available"
              @click="selectBroker(broker.descriptor.id)"
            >
              <span>{{ broker.descriptor.displayName }}</span>
              <small
                >{{ broker.descriptor.id }} /
                {{ broker.configured ? "已配置" : "待配置" }}</small
              >
            </button>
          </div>
          <div v-if="selectedBroker" class="oobe-status-strip">
            <span>当前券商：{{ selectedBroker.descriptor.displayName }}</span>
            <span>连接：{{ connectionStatusLabel }}</span>
            <span
              >托管账户：{{
                console_.brokerSettings.value.accounts.length
              }}</span
            >
          </div>
          <div class="oobe-footer-actions">
            <v-btn variant="outlined" @click="activeStep = 'dependencies'"
              >上一步</v-btn
            >
            <v-btn color="primary" @click="goToConnectionStep">下一步</v-btn>
          </div>
        </section>

        <section v-show="activeStep === 'connection'" class="oobe-panel">
          <div class="oobe-section-title">连接配置</div>
          <FutuIntegrationSection mode="oobe" />
          <p v-if="accountStepHint" class="oobe-step-hint">
            {{ accountStepHint }}
          </p>
          <div class="oobe-footer-actions">
            <v-btn variant="outlined" @click="activeStep = 'broker'"
              >上一步</v-btn
            >
            <v-btn
              color="primary"
              :disabled="!canEnterAccountStep"
              @click="goToAccountStep"
            >
              下一步
            </v-btn>
          </div>
        </section>

        <section v-show="activeStep === 'account'" class="oobe-panel">
          <div class="oobe-section-title">确认账户</div>
          <SettingsAccountDiscoverySection
            :accounts="console_.brokerRuntime.value.accounts"
            :import-runtime-account="importRuntimeAccount"
            :unavailable-message="accountDiscoveryUnavailableMessage"
          />
          <SettingsManagedAccountsSection
            :accounts="console_.brokerSettings.value.accounts"
            :account-form="accountForm"
            :deleting-account-id="deletingAccountId"
            :editing-account-id="editingAccountId"
            :saving-account="savingAccount"
            :populate-account-form="populateAccountForm"
            :remove-account="removeAccount"
            :reset-account-form="resetAccountForm"
            :submit-account="submitAccount"
          />
          <div class="oobe-footer-actions">
            <v-btn variant="outlined" @click="activeStep = 'connection'"
              >上一步</v-btn
            >
            <v-btn variant="outlined" color="primary" @click="openSettings">
              去设置页继续配置
            </v-btn>
            <v-btn
              color="primary"
              :loading="savingOnboarding"
              @click="completeOnboarding(false)"
            >
              完成
            </v-btn>
          </div>
        </section>
      </main>
    </section>
  </div>
</template>

<style scoped>
.oobe-page {
  min-height: 100vh;
  overflow: auto;
  background: color-mix(in srgb, var(--tv-bg-app) 90%, #ffffff 10%);
  padding: 24px;
}

.oobe-shell {
  margin: 0 auto;
  max-width: 1180px;
  min-height: calc(100vh - 48px);
  display: grid;
  gap: 18px;
  align-content: start;
}

.oobe-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 20px;
  border: 1px solid var(--card-border);
  background: var(--card-surface);
  border-radius: 8px;
  padding: 22px;
}

.oobe-eyebrow {
  color: var(--card-text-2);
  font-size: 12px;
  font-weight: 700;
  text-transform: uppercase;
}

.oobe-header h1 {
  margin: 6px 0 8px;
  color: var(--card-text-1);
  font-size: 28px;
  font-weight: 800;
}

.oobe-header p {
  max-width: 680px;
  color: var(--card-text-2);
  line-height: 1.7;
}

.oobe-header-actions,
.oobe-footer-actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 10px;
}

.oobe-alert {
  border-radius: 8px;
}

.oobe-alert-title {
  margin-bottom: 4px;
  font-weight: 800;
}

.oobe-steps {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 10px;
}

.oobe-steps button {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 48px;
  border: 1px solid var(--card-border);
  background: var(--card-surface);
  color: var(--card-text-2);
  border-radius: 8px;
  padding: 0 14px;
  text-align: left;
}

.oobe-steps button.active {
  border-color: #2563eb;
  color: var(--card-text-1);
}

.oobe-steps button:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

.oobe-steps span {
  display: inline-grid;
  width: 24px;
  height: 24px;
  place-items: center;
  border-radius: 999px;
  background: #2563eb;
  color: #ffffff;
  font-size: 12px;
  font-weight: 800;
}

.oobe-content,
.oobe-panel {
  display: grid;
  gap: 16px;
}

.oobe-panel {
  border: 1px solid var(--card-border);
  background: var(--card-surface);
  border-radius: 8px;
  padding: 18px;
}

.oobe-section-title {
  color: var(--card-text-1);
  font-size: 18px;
  font-weight: 800;
}

.oobe-broker-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  gap: 12px;
}

.oobe-broker-card {
  display: grid;
  gap: 6px;
  min-height: 92px;
  border: 1px solid var(--card-border);
  background: var(--card-surface-raised);
  border-radius: 8px;
  padding: 16px;
  text-align: left;
}

.oobe-broker-card.selected {
  border-color: #2563eb;
  background: color-mix(in srgb, #2563eb 10%, var(--card-surface));
}

.oobe-broker-card span {
  color: var(--card-text-1);
  font-weight: 800;
}

.oobe-broker-card small {
  color: var(--card-text-2);
}

.oobe-status-strip {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  color: var(--card-text-2);
  font-size: 13px;
}

.oobe-status-strip span {
  border: 1px solid var(--card-border);
  border-radius: 999px;
  padding: 6px 10px;
}

.oobe-step-hint {
  color: #475569;
  font-size: 13px;
  line-height: 1.6;
}

@media (max-width: 760px) {
  .oobe-page {
    padding: 12px;
  }

  .oobe-header {
    display: grid;
  }

  .oobe-steps {
    grid-template-columns: 1fr;
  }
}
</style>
