<script setup lang="ts">
import MarkdownIt from "markdown-it";
import { computed, nextTick, onMounted, reactive, ref } from "vue";

import type { ADKApproval, ADKApprovalResolution } from "@jftrade/ui-contracts";

import ADKRunTrace from "../components/shared/ADKRunTrace.vue";
import {
  createAssistantMessageState,
  runTerminalMessage,
  syncRunPresentationState,
  type ADKAssistantMessageState,
} from "../composables/adkChatPresentation";
import {
  formatGenericStatusLabel,
  formatMarketLabel,
  formatTradingEnvironment,
} from "../composables/consoleDataFormatting";
import {
  normalizeAssistantContent,
  streamADKChat,
} from "../composables/adkChatStream";
import { fetchEnvelope, fetchEnvelopeWithInit } from "../composables/apiClient";
import { useConsoleData } from "../composables/useConsoleData";
import { useWorkspaceTradingPrefs } from "../composables/useWorkspaceLayout";

interface Message {
  id: string;
  role: "user" | "assistant";
  content: string;
  reasoningContent?: string | undefined;
  reasoningExpanded?: boolean | undefined;
  toolProgress?: string | undefined;
  preToolContent?: string | undefined;
  preToolReasoning?: string | undefined;
  run?: ADKAssistantMessageState["run"];
  toolSummaryExpanded?: boolean | undefined;
  expandedToolCallIds?: string[] | undefined;
}

const markdown = new MarkdownIt({
  html: false,
  linkify: true,
  breaks: true,
});

const messages = ref<Message[]>([
  {
    id: "welcome",
    role: "assistant",
    content:
      "嗨，我是 JFTrade 助手。可以问我当前自选行情、风控状态、订单情况等问题。例如：「当前持仓盈亏」、「风控是否启用」、「最近的审计动作」。",
  },
]);

const draft = ref("");
const busy = ref(false);
const scrollHost = ref<HTMLElement | null>(null);
const pendingApprovals = ref<ADKApproval[]>([]);

interface ApprovalsResponse { approvals: ADKApproval[] }

onMounted(() => {
  void refreshApprovals();
});

const {
  selectedBrokerAccount,
  systemStatus,
  realTradeKillSwitchState,
  realTradeRiskState,
  brokerOrders,
  portfolioPositions,
  storageOverview,
  marketDataSubscriptions,
} = useConsoleData();
const { prefs } = useWorkspaceTradingPrefs();

const suggestions = computed(() => [
  "当前交易环境",
  "风控状态",
  "持仓概览",
  "最近订单",
  "活跃行情订阅",
]);

function localReply(question: string): string {
  const q = question.toLowerCase();
  if (q.includes("环境") || q.includes("env") || q.includes("trading")) {
    return selectedBrokerAccount.value == null
      ? `当前默认交易环境：${formatTradingEnvironment(systemStatus.value.defaultTradingEnvironment)}，券商层：${systemStatus.value.broker.displayName}。`
      : `当前查询作用域：${selectedBrokerAccount.value.brokerId} / ${selectedBrokerAccount.value.accountId} / ${formatTradingEnvironment(selectedBrokerAccount.value.tradingEnvironment)} / ${formatMarketLabel(selectedBrokerAccount.value.market)}。`;
  }
  if (q.includes("风控") || q.includes("kill") || q.includes("risk")) {
    return `熔断开关：${formatGenericStatusLabel(realTradeKillSwitchState.value.killSwitchActive ? "ACTIVE" : "CLEAR")}；风控限额：${realTradeRiskState.value.riskEnabled ? "已启用" : "未启用"}。`;
  }
  if (q.includes("持仓") || q.includes("position") || q.includes("portfolio")) {
    const n = portfolioPositions.value.positions.length;
    return `投影层共 ${n} 条持仓记录。打开账户页查看明细。`;
  }
  if (q.includes("订单") || q.includes("order")) {
    const n = brokerOrders.value.orders.length;
    return `券商订单视图共 ${n} 条记录。可在执行相关页面查看详情。`;
  }
  if (q.includes("订阅") || q.includes("sub") || q.includes("market")) {
    return `当前行情活跃订阅 ${marketDataSubscriptions.value.totalActiveSubscriptions} 条，配额 ${marketDataSubscriptions.value.quota.totalUsed}/${marketDataSubscriptions.value.quota.totalLimit ?? "∞"}。`;
  }
  if (q.includes("审计") || q.includes("audit") || q.includes("log")) {
    const n = storageOverview.value.recentAuditLogs.length;
    return `最近 ${n} 条审计日志可在右栏或系统页查看。`;
  }
  if (q.includes("标的") || q.includes("symbol")) {
    return `当前工作区标的：${formatMarketLabel(prefs.value.market)}:${prefs.value.symbol}，周期 ${prefs.value.period}。`;
  }
  return "（本地助手）目前我可以回答关于交易环境、风控、持仓、订单、订阅、审计的问题。后续会接入大模型 / 策略生成能力。";
}

async function send(): Promise<void> {
  const text = draft.value.trim();
  if (!text || busy.value) return;
  draft.value = "";
  messages.value.push({ id: `u-${Date.now()}`, role: "user", content: text });
  busy.value = true;
  const assistantMessage = reactive<Message>(createAssistantMessageState(`a-${Date.now()}`));
  messages.value.push(assistantMessage);

  try {
    const response = await streamADKChat({ message: text }, async (event) => {
      if (event.type === "session" && event.session?.id) {
        // Session created or resolved — no action needed in panel mode
      }
      if (event.type === "run" && event.run) {
        syncRunPresentationState(assistantMessage, event.run);
      }
      if (event.type === "delta") {
        if (event.toolProgress) {
          if (assistantMessage.preToolContent === undefined && assistantMessage.content.trim() !== "") {
            assistantMessage.preToolContent = assistantMessage.content;
            assistantMessage.content = "";
          }
          if (assistantMessage.preToolReasoning === undefined && (assistantMessage.reasoningContent ?? "").trim() !== "") {
            assistantMessage.preToolReasoning = assistantMessage.reasoningContent;
            assistantMessage.reasoningContent = "";
          }
          assistantMessage.toolProgress = event.toolProgress;
        }
        if (event.delta) {
          assistantMessage.content += event.delta;
          assistantMessage.toolProgress = "";
        }
        if (event.reasoningDelta) {
          assistantMessage.reasoningContent = (assistantMessage.reasoningContent ?? "") + event.reasoningDelta;
          if (!assistantMessage.reasoningExpanded && assistantMessage.reasoningContent.length > 0) {
            assistantMessage.reasoningExpanded = true;
          }
        }
        await nextTick();
        if (scrollHost.value) {
          scrollHost.value.scrollTop = scrollHost.value.scrollHeight;
        }
      }
      if (event.type === "final" && event.response) {
        if (assistantMessage.preToolContent !== undefined) {
          assistantMessage.content = event.response.reply.replace(assistantMessage.preToolContent, "").trim();
          if (event.response.reasoningContent && assistantMessage.preToolReasoning) {
            assistantMessage.reasoningContent = event.response.reasoningContent.replace(assistantMessage.preToolReasoning, "").trim();
          }
        } else {
          assistantMessage.content = event.response.reply;
          assistantMessage.reasoningContent = event.response.reasoningContent ?? assistantMessage.reasoningContent ?? "";
        }
        syncRunPresentationState(assistantMessage, event.response.run);
        assistantMessage.toolProgress = "";
        const terminalMessage = runTerminalMessage(event.response.run);
        if (terminalMessage) {
          assistantMessage.content += `\n\n⚠️ ${terminalMessage}`;
        }
      }
      if (event.type === "error") {
        throw new Error(event.message || "Agents chat failed");
      }
    });
    const normalized = normalizeAssistantContent(
      response.reply,
      response.reasoningContent ?? assistantMessage.reasoningContent,
    );
    if (assistantMessage.preToolContent !== undefined) {
      assistantMessage.content = normalized.content.replace(assistantMessage.preToolContent, "").trim();
      if (normalized.reasoningContent && assistantMessage.preToolReasoning) {
        assistantMessage.reasoningContent = normalized.reasoningContent.replace(assistantMessage.preToolReasoning, "").trim();
      }
    } else {
      assistantMessage.content = normalized.content;
      assistantMessage.reasoningContent = normalized.reasoningContent;
    }
    syncRunPresentationState(assistantMessage, response.run);
    pendingApprovals.value = response.pendingApprovals;
    const terminalMessage = runTerminalMessage(response.run);
    if (terminalMessage) {
      assistantMessage.content += `\n\n⚠️ ${terminalMessage}`;
    }
  } catch (err) {
    const errMsg = err instanceof Error ? err.message : "Agents chat failed";
    if (assistantMessage.content.trim() === "" && (assistantMessage.reasoningContent ?? "").trim() === "") {
      assistantMessage.content = errMsg;
    } else {
      assistantMessage.content += `\n\n⚠️ ${errMsg}`;
    }
  } finally {
    busy.value = false;
    await nextTick();
    if (scrollHost.value) {
      scrollHost.value.scrollTop = scrollHost.value.scrollHeight;
    }
  }
}

async function refreshApprovals(): Promise<void> {
  try {
    const response = await fetchEnvelope<ApprovalsResponse>("/api/v1/adk/approvals?status=PENDING&limit=20");
    pendingApprovals.value = response.approvals;
  } catch {
    pendingApprovals.value = [];
  }
}

async function resolveApproval(approval: ADKApproval, approved: boolean): Promise<void> {
  const action = approved ? "approve" : "deny";
  try {
      const resolution = await fetchEnvelopeWithInit<ADKApprovalResolution>(
      `/api/v1/adk/approvals/${encodeURIComponent(approval.id)}/${action}`,
      { method: "POST" },
    );
    if (resolution.message) {
      const message: Message = {
        ...(createAssistantMessageState(resolution.message.id)),
        id: resolution.message.id,
        role: "assistant",
        content: resolution.message.content,
      };
      if (resolution.message.reasoningContent !== undefined) {
        message.reasoningContent = resolution.message.reasoningContent;
      }
      if (resolution.run) {
        syncRunPresentationState(message, resolution.run);
      }
      const messageIndex = resolution.run
        ? messages.value.findIndex((item) => item.run?.id === resolution.run?.id)
        : -1;
      if (messageIndex >= 0) {
        messages.value[messageIndex] = {
          ...messages.value[messageIndex]!,
          content: message.content,
          reasoningContent: message.reasoningContent ?? "",
          reasoningExpanded: false,
          run: resolution.run,
        };
      } else {
        messages.value.push(message);
      }
    }
    await refreshApprovals();
  } catch (error) {
    messages.value.push({
      id: `approval-error-${Date.now()}`,
      role: "assistant",
      content: error instanceof Error ? error.message : "审批处理失败",
    });
  }
}

function pick(s: string): void {
  draft.value = s;
  void send();
}

function renderMarkdown(content: string): string {
  return markdown
    .render(content)
    .replace(/<a /g, '<a target="_blank" rel="noopener noreferrer" ');
}
</script>

<template>
  <div style="display: flex; flex-direction: column; height: 100%; min-height: 0">
    <div ref="scrollHost" style="flex: 1; overflow-y: auto; padding: 10px">
      <div v-for="m in messages" :key="m.id" class="tv-ai-msg" :class="{ 'is-user': m.role === 'user' }">
        <div style="font-size: 10px; text-transform: uppercase; letter-spacing: 0.08em; color: var(--tv-text-muted); margin-bottom: 4px">
          {{ m.role === "user" ? "你" : "助手" }}
        </div>
        <button
          v-if="m.role === 'assistant' && m.preToolContent === undefined && (m.reasoningContent ?? '').trim() !== ''"
          type="button"
          class="tv-ai-reasoning-toggle"
          @click="m.reasoningExpanded = !m.reasoningExpanded"
        >
          {{ m.reasoningExpanded ? "隐藏深度思考" : "查看深度思考" }}
        </button>
        <div
          v-if="m.role === 'assistant' && m.preToolContent === undefined && m.reasoningExpanded && (m.reasoningContent ?? '').trim() !== ''"
          class="tv-ai-reasoning-body"
          v-html="renderMarkdown(m.reasoningContent ?? '')"
        />
        <div
          v-if="m.role === 'assistant' && (m.preToolContent ?? '').trim() !== ''"
          v-html="renderMarkdown(m.preToolContent ?? '')"
        />
        <button
          v-if="m.role === 'assistant' && (m.preToolReasoning ?? '').trim() !== ''"
          type="button"
          class="tv-ai-reasoning-toggle"
          @click="m.reasoningExpanded = !m.reasoningExpanded"
        >
          {{ m.reasoningExpanded ? "隐藏深度思考" : "查看深度思考" }}
        </button>
        <div
          v-if="m.role === 'assistant' && m.reasoningExpanded && (m.preToolReasoning ?? '').trim() !== ''"
          class="tv-ai-reasoning-body"
          v-html="renderMarkdown(m.preToolReasoning ?? '')"
        />
        <div
          v-if="m.role === 'assistant' && busy && !m.toolProgress && !m.run && m.content === '' && (m.reasoningContent ?? '') === '' && messages[messages.length - 1] === m"
          class="tv-ai-tool-progress"
        >
          <span class="tv-ai-thinking-dot"></span>
          思考中…
        </div>
        <ADKRunTrace
          v-if="m.role === 'assistant'"
          :run="m.run"
          :tool-progress="m.toolProgress"
          :busy="busy && messages[messages.length - 1] === m"
          compact
          :summary-expanded="m.toolSummaryExpanded"
          :expanded-tool-call-ids="m.expandedToolCallIds"
          @update:summary-expanded="m.toolSummaryExpanded = $event"
          @update:expanded-tool-call-ids="m.expandedToolCallIds = $event"
        />
        <div
          v-if="m.role === 'assistant'"
          v-html="renderMarkdown(m.content)"
        />
        <div v-if="m.role === 'user'">{{ m.content }}</div>
      </div>
      <div v-if="pendingApprovals.length > 0" class="tv-ai-approvals">
        <div class="tv-ai-approval-title">待审批动作</div>
        <div v-for="approval in pendingApprovals" :key="approval.id" class="tv-ai-approval-card">
          <strong>{{ approval.toolName }}</strong>
          <span>{{ approval.reason }}</span>
          <div class="tv-ai-approval-actions">
            <button type="button" @click="resolveApproval(approval, true)">批准</button>
            <button type="button" @click="resolveApproval(approval, false)">拒绝</button>
          </div>
        </div>
      </div>
    </div>
    <div style="border-top: 1px solid var(--tv-border); padding: 8px 10px">
      <div style="display: flex; gap: 4px; flex-wrap: wrap; margin-bottom: 6px">
        <button
          v-for="s in suggestions"
          :key="s"
          class="tv-btn tv-btn-ghost"
          style="height: 22px; font-size: 10px; padding: 0 8px"
          @click="pick(s)"
        >
          {{ s }}
        </button>
      </div>
      <form style="display: flex; gap: 6px" @submit.prevent="send">
        <input
          v-model="draft"
          class="tv-input"
          placeholder="问点什么…"
          :disabled="busy"
        />
        <button type="submit" class="tv-btn" :disabled="busy || !draft.trim()">
          {{ busy ? "…" : "发送" }}
        </button>
      </form>
    </div>
  </div>
</template>
