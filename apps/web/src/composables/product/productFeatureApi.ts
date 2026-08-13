import {
  marketFeatureApi,
  marketFeatureTarget,
  type MarketFeatureRequest,
} from "./marketFeatureApi";
import {
  predictionApi,
  predictionTarget,
  type PredictionRequest,
} from "@/composables/research/predictionApi";
import {
  researchApi,
  researchTarget,
  type ResearchRequest,
} from "@/composables/research/researchApi";
import type { BrokerFeatureResultDto } from "@/contracts";

export type ProductFeatureRequest =
  | MarketFeatureRequest
  | ResearchRequest
  | PredictionRequest;

export function queryProductFeature(
  request: ProductFeatureRequest,
): Promise<BrokerFeatureResultDto> {
  switch (request.scope) {
    case "market-feature":
      return marketFeatureApi.query(request);
    case "research":
      return researchApi.query(request);
    case "prediction":
      return predictionApi.query(request);
  }
}

export function productFeaturePath(request: ProductFeatureRequest): string {
  switch (request.scope) {
    case "market-feature":
      return marketFeatureTarget(request).path;
    case "research":
      return researchTarget(request).path;
    case "prediction":
      return predictionTarget(request).path;
  }
}
