import { computed, ref, type ComputedRef } from "vue";

export interface StrategyUndoRedoState {
  readonly canUndo: ComputedRef<boolean>;
  readonly canRedo: ComputedRef<boolean>;
  undo(): string | null;
  redo(): string | null;
  pushSnapshot(snapshot: string): void;
  clear(): void;
  readonly stackDepth: ComputedRef<number>;
}

/**
 * Manages undo/redo for strategy design.  Tracks serialized snapshots
 * of the definition state so the user can step backward / forward
 * through recent edits.
 *
 * Usage:
 *   const history = useStrategyUndoRedo();
 *   history.pushSnapshot(JSON.stringify(definitionForm));
 *   const prev = history.undo(); // returns previous snapshot or null
 *   const next = history.redo(); // returns next snapshot or null
 */
export function useStrategyUndoRedo(options?: {
  maxDepth?: number;
}): StrategyUndoRedoState {
  const maxDepth = options?.maxDepth ?? 80;

  const stack = ref<string[]>([]);
  const pointer = ref(-1);

  let coalesceTimer: ReturnType<typeof setTimeout> | null = null;
  let pendingSnapshot = "";

  const canUndo = computed(() => pointer.value > 0);
  const canRedo = computed(() => pointer.value < stack.value.length - 1);
  const stackDepth = computed(() => stack.value.length);

  function pushSnapshot(snapshot: string): void {
    if (snapshot === "") {
      return;
    }

    // Coalesce rapid changes so we don't record every keystroke.
    pendingSnapshot = snapshot;

    if (coalesceTimer !== null) {
      clearTimeout(coalesceTimer);
    }

    coalesceTimer = setTimeout(() => {
      coalesceTimer = null;
      commitSnapshot(pendingSnapshot);
    }, 400);
  }

  function flushPending(): void {
    if (coalesceTimer !== null) {
      clearTimeout(coalesceTimer);
      coalesceTimer = null;
      if (pendingSnapshot !== "") {
        commitSnapshot(pendingSnapshot);
        pendingSnapshot = "";
      }
    }
  }

  function commitSnapshot(snapshot: string): void {
    // Discard redo future when a new edit is made.
    if (pointer.value < stack.value.length - 1) {
      stack.value = stack.value.slice(0, pointer.value + 1);
    }

    // Don't push duplicate consecutive snapshots.
    const last = stack.value[pointer.value];
    if (last === snapshot) {
      return;
    }

    stack.value.push(snapshot);
    pointer.value = stack.value.length - 1;

    // Trim history.
    while (stack.value.length > maxDepth) {
      stack.value.shift();
      pointer.value = Math.max(0, pointer.value - 1);
    }
  }

  function undo(): string | null {
    // Flush any pending snapshot first so we capture the state before undo.
    flushPending();

    if (pointer.value <= 0) {
      return null;
    }

    pointer.value -= 1;
    return stack.value[pointer.value] ?? null;
  }

  function redo(): string | null {
    flushPending();

    if (pointer.value >= stack.value.length - 1) {
      return null;
    }

    pointer.value += 1;
    return stack.value[pointer.value] ?? null;
  }

  function clear(): void {
    stack.value = [];
    pointer.value = -1;
    pendingSnapshot = "";
    if (coalesceTimer !== null) {
      clearTimeout(coalesceTimer);
      coalesceTimer = null;
    }
  }

  return {
    canUndo,
    canRedo,
    undo,
    redo,
    pushSnapshot,
    clear,
    stackDepth,
  };
}
