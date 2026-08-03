import { ref } from "vue";
import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  deleteWorkflow: vi.fn(),
  deleteTrigger: vi.fn(),
  requestConfirmation: vi.fn(),
}));

vi.mock("@/composables/adk/adkWorkflowsApi", () => ({
  deleteADKWorkflow: mocks.deleteWorkflow,
  deleteADKWorkflowTrigger: mocks.deleteTrigger,
}));

vi.mock("@/composables/shared/useActionConfirmation", () => ({
  useActionConfirmation: () => ({
    requestConfirmation: mocks.requestConfirmation,
  }),
}));

import { useADKWorkflowStudioDelete } from "../../src/composables/adk/useADKWorkflowStudioDelete";

beforeEach(() => {
  vi.clearAllMocks();
  mocks.requestConfirmation.mockResolvedValue({});
});

describe("useADKWorkflowStudioDelete", () => {
  it("uses stable fallback messages for non-Error deletion failures", async () => {
    const errorMessage = ref("");
    const options = {
      workflowForm: { id: "workflow-1", name: "复盘", status: "ENABLED" },
      triggerForm: {
        id: "trigger-1",
        workflowId: "workflow-1",
        title: "",
        type: "schedule",
        status: "ENABLED",
        config: {},
      },
      selectedWorkflowId: ref("workflow-1"),
      selectedNodeId: ref("trigger:trigger-1"),
      errorMessage,
      removeWorkflow: vi.fn(),
      removeTrigger: vi.fn(),
      removeFlowNode: vi.fn(),
      removeDraftTriggerNode: vi.fn(),
    } as never;
    const controller = useADKWorkflowStudioDelete(options);

    mocks.deleteWorkflow.mockRejectedValueOnce("workflow failed");
    await controller.removeSelectedWorkflow();
    expect(errorMessage.value).toBe("删除工作流失败");

    mocks.deleteTrigger.mockRejectedValueOnce("trigger failed");
    await controller.removeSelectedTrigger();
    expect(mocks.requestConfirmation).toHaveBeenLastCalledWith(
      expect.objectContaining({ message: "删除触发器「trigger-1」？" }),
    );
    expect(errorMessage.value).toBe("删除触发器失败");
  });
});
