import { describe, expect, it } from "vitest";

import {
    buildStrategyBindingPayload,
    formatStrategyRuntimeRiskSummary,
    normalizeStrategyRuntimeRiskSettings,
} from "../src/components/strategy-runtime/strategyRuntimeInstanceBinding";

describe("strategy runtime risk binding", () => {
    it("normalizes legacy or disabled settings to a safe default", () => {
        expect(normalizeStrategyRuntimeRiskSettings(undefined)).toEqual({
            mode: "off",
            closeOnly: false,
            maxOrderQuantity: null,
            maxOrderNotional: null,
            dailyMaxOrders: null,
            pauseOnReject: false,
        });
        expect(normalizeStrategyRuntimeRiskSettings({
            mode: "off",
            closeOnly: true,
            maxOrderQuantity: 10,
        })).toEqual(normalizeStrategyRuntimeRiskSettings(undefined));
    });

    it("preserves enforce limits in the strategy binding payload", () => {
        const payload = buildStrategyBindingPayload({
            brokerAccountOptions: [],
            instruments: [{ market: "US", code: "AAPL" }],
            interval: "5m",
            executionMode: "live",
            brokerAccountKey: "",
            runtimeRisk: {
                mode: "enforce",
                closeOnly: true,
                maxOrderQuantity: 5,
                maxOrderNotional: 500,
                dailyMaxOrders: 8,
                pauseOnReject: true,
            },
        });

        expect(payload.runtimeRisk).toEqual({
            mode: "enforce",
            closeOnly: true,
            maxOrderQuantity: 5,
            maxOrderNotional: 500,
            dailyMaxOrders: 8,
            pauseOnReject: true,
        });
        expect(formatStrategyRuntimeRiskSummary(payload.runtimeRisk)).toContain("仅平仓");
        expect(formatStrategyRuntimeRiskSummary(payload.runtimeRisk)).toContain("拒单后暂停");
    });
});
