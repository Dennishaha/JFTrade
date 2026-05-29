import type {
    StrategyApplyLinkedInstancesResponse,
    StrategyDefinitionDocument,
    StrategyInstanceItem,
} from "@jftrade/ui-contracts";
import type { ComputedRef, Ref } from "vue";
import { onBeforeUnmount, onMounted, ref } from "vue";
import { onBeforeRouteLeave } from "vue-router";

import {
    fetchEnvelope,
    fetchEnvelopeWithInit,
} from "../../composables/apiClient";

interface SaveLinkedInstancesSummary {
    definitionId: string;
    linkedCount: number;
    stoppedCount: number;
    busyCount: number;
}

interface PendingDeleteDefinition {
    definition: StrategyDefinitionDocument;
    linkedStrategies: StrategyInstanceItem[];
}

interface UseStrategyStageDefinitionPersistenceOptions {
    definitionForm: Ref<StrategyDefinitionDocument>;
    selectedDefinitionId: Ref<string>;
    selectedDefinition: ComputedRef<StrategyDefinitionDocument | null>;
    strategyDefinitions: Ref<StrategyDefinitionDocument[]>;
    hasUnsavedDefinitionChanges: ComputedRef<boolean>;
    definitionError: Ref<string>;
    definitionNotice: Ref<string>;
    clearDefinitionMessages: () => void;
    buildSavePayload: () => StrategyDefinitionDocument;
    loadStrategyDefinitions: (preferredDefinitionId?: string) => Promise<void>;
    emitSwitchToRuntime: (payload?: { notice?: string; definitionId?: string }) => void;
}

export function useStrategyStageDefinitionPersistence(
    options: UseStrategyStageDefinitionPersistenceOptions,
) {
    const isSavingDefinition = ref(false);
    const isInstantiatingDefinition = ref(false);
    const deletingDefinitionId = ref("");
    const isSaveLinkedInstancesDialogOpen = ref(false);
    const pendingSaveLinkedInstancesSummary = ref<SaveLinkedInstancesSummary | null>(null);
    const isDeleteDefinitionDialogOpen = ref(false);
    const pendingDeleteDefinition = ref<PendingDeleteDefinition | null>(null);

    const isStrategyLinkedToDefinition = (
        strategy: StrategyInstanceItem,
        definitionId: string,
    ): boolean => {
        const normalizedDefinitionID = definitionId.trim();
        if (normalizedDefinitionID === "") {
            return false;
        }

        if (strategy.definition.strategyId.trim() === normalizedDefinitionID) {
            return true;
        }

        return typeof strategy.params.definitionId === "string"
            && strategy.params.definitionId.trim() === normalizedDefinitionID;
    };

    const buildApplyLinkedInstancesNotice = (
        result: StrategyApplyLinkedInstancesResponse,
    ): string => {
        if (result.totalLinked === 0) {
            return "当前没有关联实例需要同步。";
        }

        const parts: string[] = [];
        if (result.applied.length > 0) {
            parts.push(`已同步 ${result.applied.length} 个关联实例`);
        }
        if (result.alreadyLatest.length > 0) {
            parts.push(`${result.alreadyLatest.length} 个实例本来就是最新版本`);
        }
        if (result.skippedBusy.length > 0) {
            parts.push(`${result.skippedBusy.length} 个实例因未停止而跳过`);
        }
        return parts.join("，") || "关联实例状态未变化。";
    };

    const closeSaveLinkedInstancesDialog = (): void => {
        isSaveLinkedInstancesDialogOpen.value = false;
        pendingSaveLinkedInstancesSummary.value = null;
    };

    const closeDeleteDefinitionDialog = (options?: { force?: boolean }): void => {
        if (!options?.force && deletingDefinitionId.value !== "") {
            return;
        }
        isDeleteDefinitionDialogOpen.value = false;
        pendingDeleteDefinition.value = null;
    };

    const summarizeLinkedStrategyForDeleteDialog = (
        strategy: StrategyInstanceItem,
    ): string => {
        const symbols = (strategy.binding?.symbols ?? []).join(", ");
        if (symbols !== "") {
            return `${strategy.status} · ${symbols}`;
        }
        return `${strategy.status} · 未绑定标的`;
    };

    const jumpToRuntimeForDeletingLinkedInstances = (): void => {
        const pending = pendingDeleteDefinition.value;
        if (pending === null || pending.linkedStrategies.length === 0) {
            return;
        }
        closeDeleteDefinitionDialog({ force: true });
        options.emitSwitchToRuntime({
            notice: `已切换到运行面板，请先删除策略「${pending.definition.name}」关联的 ${pending.linkedStrategies.length} 个实例。`,
            definitionId: pending.definition.id,
        });
    };

    const persistStrategyDefinition = async (
        payload: StrategyDefinitionDocument,
        persistOptions: {
            applyLinkedInstances: boolean;
            promptedLinkedCount?: number;
        },
    ): Promise<boolean> => {
        const isExisting =
            options.selectedDefinition.value !== null
            && options.selectedDefinition.value.id === options.selectedDefinitionId.value;
        const path = isExisting
            ? `/api/v1/strategy-definitions/${encodeURIComponent(options.selectedDefinitionId.value)}`
            : "/api/v1/strategy-definitions";

        isSavingDefinition.value = true;

        try {
            const response = await fetchEnvelopeWithInit<StrategyDefinitionDocument>(path, {
                method: isExisting ? "PUT" : "POST",
                headers: {
                    "Content-Type": "application/json",
                },
                body: JSON.stringify(payload),
            });

            const savedId = response.id;
            let notice = isExisting ? "策略已保存。" : "策略已创建。";

            if (persistOptions.applyLinkedInstances && savedId.trim() !== "") {
                const applyResult = await fetchEnvelopeWithInit<StrategyApplyLinkedInstancesResponse>(
                    `/api/v1/strategy-definitions/${encodeURIComponent(savedId)}/apply-linked-instances`,
                    {
                        method: "POST",
                        headers: {
                            "Content-Type": "application/json",
                        },
                        body: JSON.stringify({}),
                    },
                );
                notice = `${notice} ${buildApplyLinkedInstancesNotice(applyResult)}`;
            } else if ((persistOptions.promptedLinkedCount ?? 0) > 0) {
                notice = `${notice} 已保留 ${persistOptions.promptedLinkedCount ?? 0} 个关联实例的当前版本。`;
            }

            await options.loadStrategyDefinitions(savedId);
            options.definitionNotice.value = notice;
            return true;
        } catch (error) {
            options.definitionError.value =
                error instanceof Error ? error.message : "保存策略失败。";
            return false;
        } finally {
            isSavingDefinition.value = false;
        }
    };

    const maybePromptLinkedInstanceSave = async (): Promise<boolean> => {
        const definitionId = options.selectedDefinitionId.value.trim();
        if (definitionId === "" || !options.hasUnsavedDefinitionChanges.value) {
            return false;
        }

        isSavingDefinition.value = true;

        try {
            const strategies = await fetchEnvelope<StrategyInstanceItem[]>("/api/v1/strategies");
            const linkedStrategies = strategies.filter((item) => isStrategyLinkedToDefinition(item, definitionId));
            if (linkedStrategies.length === 0) {
                return false;
            }

            pendingSaveLinkedInstancesSummary.value = {
                definitionId,
                linkedCount: linkedStrategies.length,
                stoppedCount: linkedStrategies.filter((item) => item.status === "STOPPED").length,
                busyCount: linkedStrategies.filter((item) => item.status !== "STOPPED").length,
            };
            isSaveLinkedInstancesDialogOpen.value = true;
            return true;
        } catch (error) {
            options.definitionError.value =
                error instanceof Error
                    ? error.message
                    : "加载关联实例失败。";
            return true;
        } finally {
            isSavingDefinition.value = false;
        }
    };

    const saveStrategyDefinition = async (): Promise<boolean> => {
        options.clearDefinitionMessages();

        const payload = options.buildSavePayload();

        if (payload.name === "") {
            options.definitionError.value = "策略名称不能为空。";
            return false;
        }

        if (payload.script.trim() === "") {
            options.definitionError.value = "策略 DSL 不能为空。";
            return false;
        }

        if (await maybePromptLinkedInstanceSave()) {
            return false;
        }

        return persistStrategyDefinition(payload, { applyLinkedInstances: false });
    };

    const saveStrategyDefinitionWithLinkedInstanceChoice = async (
        action: "save-only" | "save-and-apply",
    ): Promise<void> => {
        const summary = pendingSaveLinkedInstancesSummary.value;
        closeSaveLinkedInstancesDialog();

        const payload = options.buildSavePayload();
        if (payload.name === "") {
            options.definitionError.value = "策略名称不能为空。";
            return;
        }
        if (payload.script.trim() === "") {
            options.definitionError.value = "策略 DSL 不能为空。";
            return;
        }

        await persistStrategyDefinition(payload, {
            applyLinkedInstances: action === "save-and-apply",
            promptedLinkedCount: summary?.linkedCount ?? 0,
        });
    };

    const confirmStrategyDesignExit = async (): Promise<boolean> => {
        if (!options.hasUnsavedDefinitionChanges.value) {
            return true;
        }

        if (typeof window === "undefined" || typeof window.confirm !== "function") {
            return false;
        }

        const shouldSave = window.confirm(
            "当前策略有未保存更改。\n选择“确定”将先保存再离开当前编辑界面。",
        );

        if (shouldSave) {
            return await saveStrategyDefinition();
        }

        return window.confirm("要放弃未保存更改并离开当前编辑界面吗？");
    };

    const instantiateStrategyDefinition = async (): Promise<void> => {
        options.clearDefinitionMessages();

        if (options.selectedDefinition.value === null) {
            options.definitionError.value = "请先选择已保存的策略定义，再创建运行实例。";
            return;
        }

        if (!(await confirmStrategyDesignExit())) {
            return;
        }

        isInstantiatingDefinition.value = true;

        try {
            options.definitionNotice.value = "已切换到运行面板，请先完成实例绑定再创建实例。";
            options.emitSwitchToRuntime({
                notice: options.definitionNotice.value,
                definitionId: options.selectedDefinition.value.id,
            });
        } catch (error) {
            options.definitionError.value =
                error instanceof Error
                    ? error.message
                    : "创建运行实例失败。";
        } finally {
            isInstantiatingDefinition.value = false;
        }
    };

    const deleteStrategyDefinition = async (definition: StrategyDefinitionDocument): Promise<void> => {
        options.clearDefinitionMessages();
        deletingDefinitionId.value = definition.id;
        try {
            const strategies = await fetchEnvelope<StrategyInstanceItem[]>("/api/v1/strategies");
            const linkedStrategies = strategies
                .filter((item) => isStrategyLinkedToDefinition(item, definition.id))
                .sort((left, right) => {
                    if (left.createdAt === right.createdAt) {
                        return left.id.localeCompare(right.id);
                    }
                    return left.createdAt.localeCompare(right.createdAt);
                });

            pendingDeleteDefinition.value = {
                definition,
                linkedStrategies,
            };
            isDeleteDefinitionDialogOpen.value = true;
        } catch (error) {
            options.definitionError.value =
                error instanceof Error ? error.message : "加载关联实例失败。";
        } finally {
            deletingDefinitionId.value = "";
        }
    };

    const confirmDeleteStrategyDefinition = async (): Promise<void> => {
        const pending = pendingDeleteDefinition.value;
        if (pending === null || pending.linkedStrategies.length > 0) {
            return;
        }

        deletingDefinitionId.value = pending.definition.id;
        try {
            await fetchEnvelopeWithInit<StrategyDefinitionDocument>(
                `/api/v1/strategy-definitions/${encodeURIComponent(pending.definition.id)}`,
                {
                    method: "DELETE",
                },
            );

            const nextPreferredId = options.selectedDefinitionId.value === pending.definition.id
                ? options.strategyDefinitions.value.find((item) => item.id !== pending.definition.id)?.id ?? ""
                : options.selectedDefinitionId.value;
            closeDeleteDefinitionDialog({ force: true });
            await options.loadStrategyDefinitions(nextPreferredId);
            options.definitionNotice.value = `策略已删除：${pending.definition.name}。`;
        } catch (error) {
            options.definitionError.value =
                error instanceof Error ? error.message : "删除策略定义失败。";
        } finally {
            deletingDefinitionId.value = "";
        }
    };

    const handleBeforeUnload = (event: BeforeUnloadEvent): void => {
        if (!options.hasUnsavedDefinitionChanges.value) {
            return;
        }

        event.preventDefault();
        event.returnValue = "";
    };

    onMounted(() => {
        if (typeof window !== "undefined") {
            window.addEventListener("beforeunload", handleBeforeUnload);
        }
    });

    onBeforeUnmount(() => {
        if (typeof window !== "undefined") {
            window.removeEventListener("beforeunload", handleBeforeUnload);
        }
    });

    onBeforeRouteLeave(async () => await confirmStrategyDesignExit());

    return {
        closeDeleteDefinitionDialog,
        closeSaveLinkedInstancesDialog,
        confirmDeleteStrategyDefinition,
        confirmStrategyDesignExit,
        deleteStrategyDefinition,
        deletingDefinitionId,
        instantiateStrategyDefinition,
        isDeleteDefinitionDialogOpen,
        isInstantiatingDefinition,
        isSaveLinkedInstancesDialogOpen,
        isSavingDefinition,
        jumpToRuntimeForDeletingLinkedInstances,
        pendingDeleteDefinition,
        pendingSaveLinkedInstancesSummary,
        saveStrategyDefinition,
        saveStrategyDefinitionWithLinkedInstanceChoice,
        summarizeLinkedStrategyForDeleteDialog,
    };
}