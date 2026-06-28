export {};

declare global {
  interface Window {
    setTimeout(
      handler: TimerHandler,
      timeout?: number,
      ...arguments: unknown[]
    ): any;
    clearTimeout(handle?: any): void;
  }
}
