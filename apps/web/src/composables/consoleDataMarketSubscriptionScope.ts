type MarketDataScopeChannel = "SNAPSHOT" | "TICK";

interface MarketDataSubscriptionTarget {
  market: string;
  symbol: string;
}

interface CreateMarketDataSubscriptionScopeOptions {
  consumerId: string;
  acquireMarketDataSubscription: (input: {
    consumerId: string;
    market: string;
    symbol: string;
    channel: MarketDataScopeChannel;
  }) => Promise<void>;
  releaseMarketDataSubscription: (input: {
    consumerId: string;
    market: string;
    symbol: string;
    channel: MarketDataScopeChannel;
    keepalive?: boolean;
  }) => Promise<void>;
  heartbeatMarketDataConsumer: (consumerId: string) => Promise<void>;
  channels?: readonly MarketDataScopeChannel[];
}

export function createMarketDataSubscriptionScope(
  options: CreateMarketDataSubscriptionScopeOptions,
) {
  const channels = options.channels ?? (["SNAPSHOT", "TICK"] as const);
  let heartbeatTimer: number | null = null;
  let heldTarget: MarketDataSubscriptionTarget | null = null;

  async function releaseTarget(
    target: MarketDataSubscriptionTarget,
    keepalive = false,
  ): Promise<void> {
    await Promise.all(
      channels.map((channel) =>
        options.releaseMarketDataSubscription({
          consumerId: options.consumerId,
          market: target.market,
          symbol: target.symbol,
          channel,
          keepalive,
        }),
      ),
    );
  }

  async function syncTarget(next: MarketDataSubscriptionTarget): Promise<void> {
    const previous = heldTarget;

    if (
      previous != null &&
      (previous.market !== next.market || previous.symbol !== next.symbol)
    ) {
      await releaseTarget(previous);
      heldTarget = null;
    }

    if (next.market === "" || next.symbol === "") {
      return;
    }

    await Promise.all(
      channels.map((channel) =>
        options.acquireMarketDataSubscription({
          consumerId: options.consumerId,
          market: next.market,
          symbol: next.symbol,
          channel,
        }),
      ),
    );
    await options.heartbeatMarketDataConsumer(options.consumerId);
    heldTarget = next;
  }

  function startHeartbeat(intervalMs = 15_000): void {
    stopHeartbeat();
    heartbeatTimer = window.setInterval(() => {
      void options.heartbeatMarketDataConsumer(options.consumerId);
    }, intervalMs);
  }

  function stopHeartbeat(): void {
    if (heartbeatTimer == null) {
      return;
    }

    window.clearInterval(heartbeatTimer);
    heartbeatTimer = null;
  }

  async function cleanup(options: { keepalive?: boolean } = {}): Promise<void> {
    stopHeartbeat();

    const target = heldTarget;
    heldTarget = null;

    if (target == null) {
      return;
    }

    await releaseTarget(target, options.keepalive ?? false);
  }

  return {
    cleanup,
    startHeartbeat,
    stopHeartbeat,
    syncTarget,
  };
}