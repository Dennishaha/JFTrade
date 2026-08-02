import type { Ref } from "vue";

import {
  deleteADKWorkflow,
  deleteADKWorkflowTrigger,
} from "@/composables/adk/adkWorkflowsApi";
import { useActionConfirmation } from "@/composables/shared/useActionConfirmation";
import type {
  TriggerFormModel,
  WorkflowFormModel,
} from "@/features/adkWorkflowForms";

/**
 * Workflow Studio 的删除确认与执行。确认对话框由宿主组件渲染
 * （ActionConfirmationHost），此处只持有 pending 状态与确认后的删除逻辑。
 */
export function useADKWorkflowStudioDelete(options: {
  workflowForm: WorkflowFormModel;
  triggerForm: TriggerFormModel;
  selectedWorkflowId: Ref<string>;
  selectedNodeId: Ref<string>;
  errorMessage: Ref<string>;
  removeWorkflow: (workflowId: string) => void;
  removeTrigger: (triggerId: string) => void;
  removeFlowNode: (nodeId: string) => void;
  removeDraftTriggerNode: () => void;
}) {
  const deleteConfirmation = useActionConfirmation();

  async function removeSelectedWorkflow(): Promise<void> {
    if (options.workflowForm.id.trim() === "") return;
    const confirmed = await deleteConfirmation.requestConfirmation({
      title: "删除工作流",
      message: `删除工作流「${options.workflowForm.name}」？`,
      confirmLabel: "删除",
    });
    if (confirmed === null) return;
    try {
      await deleteADKWorkflow(options.workflowForm.id);
      options.removeWorkflow(options.workflowForm.id);
    } catch (error) {
      options.errorMessage.value =
        error instanceof Error ? error.message : "删除工作流失败";
    }
  }

  async function removeSelectedTrigger(): Promise<void> {
    if (options.triggerForm.id.trim() === "") {
      options.removeDraftTriggerNode();
      return;
    }
    const confirmed = await deleteConfirmation.requestConfirmation({
      title: "删除触发器",
      message: `删除触发器「${options.triggerForm.title || options.triggerForm.id}」？`,
      confirmLabel: "删除",
    });
    if (confirmed === null) return;
    try {
      await deleteADKWorkflowTrigger(options.selectedWorkflowId.value, options.triggerForm.id);
      options.removeTrigger(options.triggerForm.id);
      options.removeFlowNode(options.selectedNodeId.value);
      options.selectedNodeId.value = "start";
    } catch (error) {
      options.errorMessage.value =
        error instanceof Error ? error.message : "删除触发器失败";
    }
  }

  return {
    deleteConfirmation,
    removeSelectedWorkflow,
    removeSelectedTrigger,
  };
}
