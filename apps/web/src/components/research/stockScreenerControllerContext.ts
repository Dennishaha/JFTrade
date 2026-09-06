import { hasInjectionContext, inject, provide, type InjectionKey } from "vue";

import type { StockScreenerController } from "./useStockScreenerController";

const stockScreenerControllerKey: InjectionKey<StockScreenerController> =
  Symbol("stock-screener-controller");

export function provideStockScreenerController(
  controller: StockScreenerController,
): void {
  provide(stockScreenerControllerKey, controller);
}

export function useStockScreenerControllerContext(): StockScreenerController {
  const controller = hasInjectionContext()
    ? inject(stockScreenerControllerKey, null)
    : null;
  if (!controller) {
    throw new Error("Stock screener controller is not available");
  }
  return controller;
}
