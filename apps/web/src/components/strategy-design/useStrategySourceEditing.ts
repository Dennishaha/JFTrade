import { computed, ref, type Ref } from "vue";

import type PineSourceCodePane from "@/components/strategy-design/PineSourceCodePane.vue";
import {
  deleteSourceBlock,
  duplicateSourceBlock,
  insertSourceBlock,
  isPineV6WorkflowBlockKind,
  moveSourceBlock,
  renderBlockToSource,
  replaceSourceBlockKind,
  replaceSourceRange,
  sourceBlockEditableFields,
  updateInstructionBlockParam,
  type PineSourceBlock,
  type PineSourceEditResult,
} from "@/features/pine-structure";

export function useStrategySourceEditing(
  activeScript: Readonly<Ref<string>>,
  sourceOverride: Ref<string>,
  useSourceOverride: Ref<boolean>,
) {
  const selectedSourceNodeId = ref("");
  const expandedSourceNodeId = ref<string | null>(null);
  const sourceEditorRef = ref<InstanceType<typeof PineSourceCodePane> | null>(null);
  const sourceUndoStack = ref<string[]>([]);
  const sourceRedoStack = ref<string[]>([]);
  const canUndoSourceChange = computed(() => sourceUndoStack.value.length > 0);
  const canRedoSourceChange = computed(() => sourceRedoStack.value.length > 0);

  function sourceBlockIsEditable(block: PineSourceBlock): boolean {
    return sourceBlockEditableFields(block).length > 0;
  }

  function addSourceBlock(kind: string): void {
    if (!isPineV6WorkflowBlockKind(kind)) return;
    applySourceEdit(insertSourceBlock(activeScript.value, selectedSourceNodeId.value || null, kind));
  }

  function changeSourceBlockKind(block: PineSourceBlock, kind: string): void {
    if (!isPineV6WorkflowBlockKind(kind)) return;
    applySourceEdit(replaceSourceBlockKind(activeScript.value, block.id, kind));
  }

  function deleteSourceStructureBlock(block: PineSourceBlock): void {
    applySourceEdit(deleteSourceBlock(activeScript.value, block.id));
  }

  function duplicateSourceStructureBlock(block: PineSourceBlock): void {
    applySourceEdit(duplicateSourceBlock(activeScript.value, block.id));
  }

  function moveSourceStructureBlock(block: PineSourceBlock, direction: -1 | 1): void {
    applySourceEdit(moveSourceBlock(activeScript.value, block.id, direction));
  }

  function rememberSourceSnapshot(snapshot: string): void {
    if (sourceUndoStack.value.at(-1) === snapshot) return;
    sourceUndoStack.value = [...sourceUndoStack.value.slice(-79), snapshot];
    sourceRedoStack.value = [];
  }

  function commitSourceChange(nextSource: string): void {
    if (nextSource === activeScript.value) return;
    rememberSourceSnapshot(activeScript.value);
    useSourceOverride.value = true;
    sourceOverride.value = nextSource;
  }

  function resetSourceHistory(): void {
    sourceUndoStack.value = [];
    sourceRedoStack.value = [];
  }

  function restoreSourceSnapshot(nextSource: string): void {
    useSourceOverride.value = true;
    sourceOverride.value = nextSource;
    selectedSourceNodeId.value = "";
    expandedSourceNodeId.value = null;
  }

  function undoSourceChange(): void {
    const previousSource = sourceUndoStack.value.at(-1);
    if (previousSource === undefined) return;
    sourceUndoStack.value = sourceUndoStack.value.slice(0, -1);
    sourceRedoStack.value = [...sourceRedoStack.value, activeScript.value];
    restoreSourceSnapshot(previousSource);
  }

  function redoSourceChange(): void {
    const nextSource = sourceRedoStack.value.at(-1);
    if (nextSource === undefined) return;
    sourceRedoStack.value = sourceRedoStack.value.slice(0, -1);
    sourceUndoStack.value = [...sourceUndoStack.value, activeScript.value];
    restoreSourceSnapshot(nextSource);
  }

  function applySourceEdit(result: PineSourceEditResult): void {
    if (!result.changed) return;
    commitSourceChange(result.source);
    selectedSourceNodeId.value = "";
    expandedSourceNodeId.value = null;
  }

  function toggleSourceBlockExpansion(block: PineSourceBlock): void {
    selectedSourceNodeId.value = block.id;
    expandedSourceNodeId.value = expandedSourceNodeId.value === block.id ? null : block.id;
    sourceEditorRef.value?.revealOffsetRange({
      start: block.sourceRange.start,
      end: Math.max(block.sourceRange.start + 1, block.sourceRange.end),
    });
  }

  function updateSourceBlockField(block: PineSourceBlock, key: string, value: unknown): void {
    if (!sourceBlockIsEditable(block)) return;
    const nextBlock = updateInstructionBlockParam(block, key, value);
    const nextSource = replaceSourceRange(
      activeScript.value,
      block.sourceRange,
      renderBlockToSource(nextBlock),
    );
    commitSourceChange(nextSource);
    selectedSourceNodeId.value = block.id;
    expandedSourceNodeId.value = block.id;
  }

  return {
    selectedSourceNodeId,
    expandedSourceNodeId,
    sourceEditorRef,
    sourceUndoStack,
    sourceRedoStack,
    canUndoSourceChange,
    canRedoSourceChange,
    sourceBlockIsEditable,
    addSourceBlock,
    changeSourceBlockKind,
    deleteSourceStructureBlock,
    duplicateSourceStructureBlock,
    moveSourceStructureBlock,
    rememberSourceSnapshot,
    commitSourceChange,
    resetSourceHistory,
    restoreSourceSnapshot,
    undoSourceChange,
    redoSourceChange,
    applySourceEdit,
    toggleSourceBlockExpansion,
    updateSourceBlockField,
  };
}
