<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";

import type { ADKInputAnswer, ADKInputRequest } from "@/contracts";

const props = defineProps<{
  request: ADKInputRequest;
  busy?: boolean;
  submit: (request: ADKInputRequest, answers: ADKInputAnswer[]) => void | Promise<void>;
}>();

interface DraftAnswer {
  mode: "option" | "other" | "";
  optionId: string;
  otherText: string;
}

const drafts = reactive<Record<string, DraftAnswer>>({});
const activeQuestionIndex = ref(0);

function hydrateDrafts(): void {
  const answers = new Map((props.request.answers ?? []).map((answer) => [answer.questionId, answer]));
  for (const question of props.request.questions) {
    const answer = answers.get(question.id);
    drafts[question.id] = answer?.optionId
      ? { mode: "option", optionId: answer.optionId, otherText: "" }
      : answer?.otherText
        ? { mode: "other", optionId: "", otherText: answer.otherText }
        : { mode: "", optionId: "", otherText: "" };
  }
}

watch(
  () => props.request,
  () => {
    hydrateDrafts();
    activeQuestionIndex.value = Math.min(activeQuestionIndex.value, Math.max(0, props.request.questions.length - 1));
  },
  { immediate: true, deep: true },
);

const pending = computed(() => props.request.status === "PENDING");
const hasMultipleQuestions = computed(() => props.request.questions.length > 1);
const activeQuestion = computed(() => props.request.questions[activeQuestionIndex.value]);
const complete = computed(() =>
  props.request.questions.every((question) => {
    const draft = drafts[question.id];
    if (!draft) return false;
    return draft.mode === "option"
      ? draft.optionId !== ""
      : draft.mode === "other" && draft.otherText.trim() !== "";
  }),
);

function chooseOption(questionId: string, optionId: string): void {
  if (!pending.value) return;
  drafts[questionId] = { mode: "option", optionId, otherText: "" };
}

function chooseOther(questionId: string): void {
  if (!pending.value) return;
  const current = drafts[questionId];
  drafts[questionId] = { mode: "other", optionId: "", otherText: current?.otherText ?? "" };
}

function showQuestion(index: number): void {
  activeQuestionIndex.value = Math.max(0, Math.min(index, props.request.questions.length - 1));
}

async function submitAnswers(): Promise<void> {
  if (!pending.value || !complete.value || props.busy) return;
  const answers = props.request.questions.map((question): ADKInputAnswer => {
    const draft = drafts[question.id]!;
    return draft.mode === "option"
      ? { questionId: question.id, optionId: draft.optionId }
      : { questionId: question.id, otherText: draft.otherText.trim() };
  });
  await props.submit(props.request, answers);
}
</script>

<template>
  <section class="adk-input-card" :class="`is-${request.status.toLowerCase()}`">
    <header>
      <div>
        <span class="adk-input-card__eyebrow">需要你的选择</span>
        <h3>{{ request.title || "请确认以下问题" }}</h3>
      </div>
      <span class="adk-input-card__status">
        {{ request.status === "PENDING" ? "待回答" : request.status === "ANSWERED" ? "已回答" : "已取消" }}
      </span>
    </header>

    <nav v-if="hasMultipleQuestions" class="adk-input-card__question-nav" aria-label="问题切换">
      <button
        v-for="(question, index) in request.questions"
        :key="question.id"
        type="button"
        :class="{ 'is-active': index === activeQuestionIndex, 'is-answered': drafts[question.id]?.mode !== '' }"
        :aria-label="`第 ${index + 1} 个问题`"
        @click="showQuestion(index)"
      >
        {{ index + 1 }}
      </button>
      <span>{{ activeQuestionIndex + 1 }} / {{ request.questions.length }}</span>
    </nav>

    <div v-if="activeQuestion" class="adk-input-card__questions">
      <fieldset :key="activeQuestion.id" :disabled="!pending || busy">
        <legend>{{ activeQuestion.question }}</legend>
        <button
          v-for="option in activeQuestion.options"
          :key="option.id"
          type="button"
          class="adk-input-option"
          :class="{ 'is-selected': drafts[activeQuestion.id]?.mode === 'option' && drafts[activeQuestion.id]?.optionId === option.id }"
          @click="chooseOption(activeQuestion.id, option.id)"
        >
          <span class="adk-input-option__radio" />
          <span class="adk-input-option__body">
            <strong>{{ option.label }} <small v-if="option.recommended">推荐</small></strong>
            <span v-if="option.description">{{ option.description }}</span>
          </span>
        </button>
        <div v-if="activeQuestion.allowOther" class="adk-input-other" :class="{ 'is-selected': drafts[activeQuestion.id]?.mode === 'other' }">
          <button type="button" @click="chooseOther(activeQuestion.id)">
            <span class="adk-input-option__radio" />
            <strong>其他</strong>
          </button>
          <textarea
            :value="drafts[activeQuestion.id]?.otherText ?? ''"
            rows="2"
            placeholder="输入方案外的回答"
            :disabled="!pending || busy"
            @focus="chooseOther(activeQuestion.id)"
            @input="drafts[activeQuestion.id] = { mode: 'other', optionId: '', otherText: ($event.target as HTMLTextAreaElement).value }"
          />
        </div>
      </fieldset>
    </div>

    <footer v-if="pending">
      <span>{{ hasMultipleQuestions ? "提交前可随时返回修改" : "选择你的回答" }}</span>
      <div class="adk-input-card__actions">
        <button v-if="hasMultipleQuestions" type="button" class="is-secondary" :disabled="activeQuestionIndex === 0 || busy" @click="showQuestion(activeQuestionIndex - 1)">上一个</button>
        <button v-if="hasMultipleQuestions && activeQuestionIndex < request.questions.length - 1" type="button" :disabled="busy" @click="showQuestion(activeQuestionIndex + 1)">下一个</button>
        <button v-else type="button" :disabled="!complete || busy" @click="submitAnswers">
          {{ busy ? "正在提交…" : "提交并继续" }}
        </button>
      </div>
    </footer>
  </section>
</template>

<style scoped>
.adk-input-card {
  width: min(680px, 100%);
  border: 1px solid color-mix(in srgb, var(--tv-accent) 38%, var(--tv-border));
  border-radius: 14px;
  background: var(--tv-bg-surface);
  box-shadow: 0 12px 32px rgba(2, 6, 23, .18);
  color: var(--tv-text);
  overflow: hidden;
}

.adk-input-card > header {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 14px;
  border-bottom: 1px solid var(--tv-border);
}

.adk-input-card h3 { margin: 2px 0 0; color: var(--tv-text); font-size: 14px; line-height: 1.35; }
.adk-input-card__eyebrow { color: var(--adk-accent-fg); font-size: 11px; font-weight: 700; }
.adk-input-card__status { align-self: flex-start; border-radius: 999px; background: var(--adk-accent-bg-soft); color: var(--adk-accent-fg); padding: 3px 8px; font-size: 11px; font-weight: 700; white-space: nowrap; }
.adk-input-card__questions { display: grid; gap: 12px; padding: 14px; }
.adk-input-card__question-nav { display: flex; align-items: center; gap: 6px; padding: 10px 14px 0; }
.adk-input-card__question-nav button { display: grid; place-items: center; width: 24px; height: 24px; border: 1px solid var(--tv-border); border-radius: 7px; background: var(--tv-bg-app); color: var(--tv-text-muted); font-size: 11px; font-weight: 700; }
.adk-input-card__question-nav button.is-answered { border-color: var(--adk-accent-border); color: var(--adk-accent-fg); }
.adk-input-card__question-nav button.is-active { background: var(--adk-accent-bg); color: var(--adk-accent-fg); }
.adk-input-card__question-nav > span { margin-left: auto; color: var(--tv-text-muted); font-size: 11px; font-variant-numeric: tabular-nums; }
fieldset { min-width: 0; margin: 0; padding: 0; border: 0; display: grid; gap: 6px; }
legend { width: 100%; margin-bottom: 2px; color: var(--tv-text); font-size: 13px; font-weight: 700; line-height: 1.4; }
.adk-input-option { width: 100%; display: flex; gap: 8px; align-items: flex-start; padding: 8px 9px; border: 1px solid var(--tv-border); border-radius: 9px; background: color-mix(in srgb, var(--tv-bg-surface) 94%, transparent); text-align: left; color: var(--tv-text-muted); transition: border-color .15s ease, background .15s ease; }
.adk-input-option:hover:not(:disabled) { border-color: color-mix(in srgb, var(--tv-accent) 48%, var(--tv-border)); }
.adk-input-option.is-selected, .adk-input-other.is-selected { border-color: var(--adk-accent-border); background: var(--adk-accent-bg-soft); }
.adk-input-option__radio { flex: 0 0 auto; width: 14px; height: 14px; margin-top: 2px; border: 2px solid color-mix(in srgb, var(--tv-accent) 65%, var(--tv-border)); border-radius: 50%; box-shadow: inset 0 0 0 2px var(--tv-bg-surface); }
.is-selected .adk-input-option__radio { background: var(--tv-accent); }
.adk-input-option__body { display: grid; gap: 2px; min-width: 0; }
.adk-input-option__body strong { color: var(--tv-text); font-size: 12px; line-height: 1.35; }
.adk-input-option__body small { margin-left: 4px; color: var(--adk-accent-fg); font-size: 10px; }
.adk-input-option__body > span { color: var(--tv-text-muted); font-size: 11px; line-height: 1.4; }
.adk-input-other { border: 1px solid var(--tv-border); border-radius: 9px; background: color-mix(in srgb, var(--tv-bg-surface) 94%, transparent); padding: 8px 9px; }
.adk-input-other > button { display: flex; align-items: center; gap: 8px; width: 100%; border: 0; background: transparent; padding: 1px 0 6px; color: var(--tv-text); text-align: left; font-size: 12px; }
.adk-input-other textarea { width: 100%; resize: vertical; border: 1px solid var(--tv-border-strong); border-radius: 7px; padding: 7px 8px; background: var(--tv-bg-app); color: var(--tv-text); font: inherit; font-size: 12px; line-height: 1.4; }
.adk-input-other textarea:focus { outline: 2px solid color-mix(in srgb, var(--tv-accent) 55%, transparent); outline-offset: 1px; }
.adk-input-card footer { display: flex; justify-content: space-between; align-items: center; gap: 12px; padding: 10px 14px; border-top: 1px solid var(--tv-border); color: var(--tv-text-muted); font-size: 11px; }
.adk-input-card footer button { border: 1px solid var(--adk-accent-border); border-radius: 8px; background: var(--adk-accent-bg); color: var(--adk-accent-fg); padding: 7px 11px; font-size: 12px; font-weight: 700; }
.adk-input-card__actions { display: flex; gap: 7px; }
.adk-input-card footer button.is-secondary { border-color: var(--tv-border); background: var(--tv-bg-app); color: var(--tv-text-muted); }
button:disabled, fieldset:disabled button { cursor: not-allowed; opacity: .62; }
.is-answered .adk-input-card__status { background: var(--adk-success-bg); color: var(--adk-success-fg); }
.is-cancelled .adk-input-card__status { background: var(--adk-muted-bg); color: var(--adk-muted-fg); }

@media (max-width: 640px) {
  .adk-input-card > header, .adk-input-card__questions, .adk-input-card footer { padding-left: 11px; padding-right: 11px; }
  .adk-input-card footer { align-items: stretch; flex-direction: column; }
  .adk-input-card footer button { min-height: 34px; }
}
</style>
