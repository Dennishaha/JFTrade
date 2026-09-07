import { inject, provide, type InjectionKey } from "vue";

const strategyDesignContextKey: InjectionKey<object> = Symbol("strategy-design-context");

export function provideStrategyDesignContext<T extends object>(context: T): T {
  provide(strategyDesignContextKey, context);
  return context;
}

export function useStrategyDesignContext<T extends object>(): T {
  const context = inject(strategyDesignContextKey);
  if (context === undefined) {
    throw new Error("Strategy design context is unavailable");
  }
  return context as T;
}
