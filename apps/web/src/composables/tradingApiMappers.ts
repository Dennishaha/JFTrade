import type {
  ExecutionOrderDetailsResponse,
  ExecutionOrderErrorSource,
  ExecutionOrderEventsResponse,
  ExecutionOrderSource,
  ExecutionOrderSourceDetail,
  ExecutionOrderSummaryResponse,
  ExecutionOrdersResponse,
} from "@/types";
import type { components } from "@/generated/openapi";

type ExecutionOrderWire = components["schemas"]["trading.ExecutionOrder"];
type ExecutionOrdersWire = components["schemas"]["trading.ExecutionOrders"];
type ExecutionOrderDetailsWire =
  components["schemas"]["trading.ExecutionOrderDetails"];
type ExecutionOrderEventsWire =
  components["schemas"]["trading.ExecutionOrderEvents"];

function mapExecutionOrderSource(value: unknown): ExecutionOrderSource {
  const normalized = typeof value === "string" ? value.trim().toLowerCase() : "";
  switch (normalized) {
    case "system":
      return "system";
    case "broker":
      return "broker";
    case "":
      return "system";
    default:
      throw new Error(`Unsupported execution order source: ${String(value)}`);
  }
}

function mapSourceDetail(
  value: unknown,
  source: ExecutionOrderSource,
): ExecutionOrderSourceDetail {
  const normalized = typeof value === "string" ? value.trim() : "";
  if (normalized === "") {
    return source === "broker" ? "broker.current" : "command.place";
  }
  // sourceDetail is deliberately preserved for forward compatibility. The UI
  // formatter already renders broker extensions that are newer than its known
  // labels, while the legacy view-model union is narrower than the wire value.
  return normalized as ExecutionOrderSourceDetail;
}

function mapLastErrorSource(
  value: string | null,
): ExecutionOrderErrorSource | null {
  if (value == null) return null;
  const normalized = value.trim();
  return normalized === "" ? null : (normalized as ExecutionOrderErrorSource);
}

function mapExecutionOrder(
  order: ExecutionOrderWire,
): ExecutionOrderSummaryResponse {
  const source = mapExecutionOrderSource(order.source);
  return {
    ...order,
    source,
    sourceDetail: mapSourceDetail(order.sourceDetail, source),
    lastErrorSource: mapLastErrorSource(order.lastErrorSource),
    ...(order.legs == null
      ? {}
      : { legs: order.legs.map((leg) => ({ ...leg })) }),
  };
}

export function mapExecutionOrders(
  response: ExecutionOrdersWire,
): ExecutionOrdersResponse {
  return {
    orders: (response.orders ?? []).map(mapExecutionOrder),
  };
}

export function mapExecutionOrderDetails(
  response: ExecutionOrderDetailsWire,
): ExecutionOrderDetailsResponse {
  return {
    order: mapExecutionOrder(response.order),
    recentEvents: (response.recentEvents ?? []).map((event) => ({ ...event })),
    checkedAt: response.checkedAt,
  };
}

export function mapExecutionOrderEvents(
  response: ExecutionOrderEventsWire,
): ExecutionOrderEventsResponse {
  return {
    internalOrderId: response.internalOrderId,
    events: (response.events ?? []).map((event) => ({ ...event })),
  };
}
